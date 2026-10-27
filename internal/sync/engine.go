// Package sync est le moteur de synchronisation de Watchoo (cœur métier) : il lit les dépôts suivis
// dans le store, interroge la forge par provider.Provider et enregistre de façon incrémentale
// branches, commits et contributeurs (documentation/03-architecture.md, « Synchronisation
// incrémentale », « Branches suivies », « Fréquence de synchronisation »).
//
// Le nom du package masque le package sync de la bibliothèque standard : ses fichiers importent
// celui-ci sous le nom gosync, et les appelants importent ce package sous le nom wsync, par exemple :
//
//	import wsync "github.com/studoo-app/watchoo/internal/sync"
//
// Token : redemandé à auth.Client.Token au début de CHAQUE synchro (et de chaque AddRepositories ou
// RateLimit), passé à la fabrique de Provider, puis oublié ; jamais stocké, journalisé ni inclus dans
// une erreur ou un rapport.
//
// Source (V2, voir Source) : choisie à chaque fois, uniquement selon gh. Token lu : API GitHub. gh
// introuvable ou connecté à aucun compte (auth.Absent) : clones Git locaux (NewGitProvider), sans
// token. Toute autre erreur de gh (délai, réponse illisible) reste une erreur : elle ne fait pas
// basculer vers git.
//
// Logs (slog, Options.Logger) : identifiants de dépôt (owner/name), SHA courts, compteurs et durées
// seulement ; jamais le token, un message de commit, un nom ou un e-mail.
package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Règles fixées du moteur.
const (
	// LockStaleAfter : un sync_run « running » plus ancien est considéré comme interrompu
	// (store.BeginSyncRun) ; plus récent, il verrouille le dépôt.
	LockStaleAfter = 30 * time.Minute
	// QuotaReservePercent : sous ce pourcentage de quota restant (Remaining*100 < Limit*10), les
	// dépôts restants sont reportés et aucune statistique de commit n'est demandée.
	QuotaReservePercent = 10
	// MaxStatsPerRepo : nombre maximum d'appels CommitStats par dépôt et par synchro.
	MaxStatsPerRepo = 100
)

// ProviderFactory construit un Provider authentifié par token. Le moteur l'appelle au début de chaque
// synchro avec le token fraîchement lu ; le Provider ne vit que le temps de cette synchro.
type ProviderFactory func(token string) (provider.Provider, error)

// GitProviderFactory construit le Provider des clones Git locaux (V2), utilisé sans gh. Le moteur
// l'appelle au début de chaque synchro faite sans gh.
type GitProviderFactory func() (provider.Provider, error)

// SourceKind est le moyen d'accès aux dépôts. Valeurs stables (sorties --json).
type SourceKind string

// Valeurs de SourceKind.
const (
	SourceAPI SourceKind = "api" // API GitHub, avec le token de gh
	SourceGit SourceKind = "git" // clones Git locaux, sans token (gh absent ou non connecté)
)

// Source décrit la source choisie pour la prochaine synchro.
type Source struct {
	Kind SourceKind
	// Reason explique le choix de SourceGit (message de gh : introuvable, non connecté), en français,
	// sans token ; vide pour SourceAPI.
	Reason string
}

// Label renvoie le nom de la source à afficher : « API GitHub » ou « clones Git locaux ».
func (s Source) Label() string {
	if s.Kind == SourceGit {
		return "clones Git locaux"
	}
	return "API GitHub"
}

// Options règle New.
type Options struct {
	Store *store.Store // base de l'espace (obligatoire)
	Auth  auth.Client  // accès à gh, seule source du token (obligatoire)
	// Config est la configuration de l'espace : sync.parallelism (ramené entre 1 et
	// workspace.MaxSyncParallelism) et branches.exclude.
	Config workspace.Config
	// NewProvider construit le Provider ; nil = github.New(token, github.Options{Cache, Quota,
	// UserAgent, Logger}) avec un *github.Cache et un *github.Quota créés par New et partagés par
	// toutes les synchros de l'Engine (donc de tout le processus). Les tests y renvoient un
	// providertest.Fake ou un provider GitHub branché sur githubtest.
	NewProvider ProviderFactory
	// NewGitProvider construit le Provider des clones Git locaux, utilisé quand gh n'est pas installé
	// ou pas connecté ; nil = git.New(git.Options{Dir: ReposDir, Protocol: Config.Git.Protocol,
	// Logger}). Les tests y branchent un provider Git sur gittest.
	NewGitProvider GitProviderFactory
	// ReposDir est le répertoire des clones Git locaux de l'espace (workspace.Workspace.ReposDir), pour
	// le provider Git par défaut ; vide (et NewGitProvider nil) = suivi sans gh impossible.
	ReposDir string
	// UserAgent est passé au provider GitHub par défaut (« watchoo/<version> ») ; vide = défaut.
	UserAgent string
	// Now est l'horloge (time.Now par défaut) : dates des sync_runs, verrou, durées, garde-fou de quota.
	Now func() time.Time
	// Logger reçoit les logs du moteur ; nil = slog.Default().
	Logger *slog.Logger
}

// Engine est le moteur de synchro d'un espace. Construire avec New. Sûr pour un usage concurrent :
// le TUI peut lancer une synchro manuelle pendant une synchro automatique (le verrou en base évite de
// synchroniser deux fois le même dépôt en même temps).
type Engine struct {
	store       *store.Store
	auth        auth.Client
	cfg         workspace.Config
	newProvider ProviderFactory
	newGit      GitProviderFactory
	now         func() time.Time
	logger      *slog.Logger
}

// New vérifie les options et construit un Engine (aucun appel à gh ni à la forge).
func New(opts Options) (*Engine, error) {
	if opts.Store == nil {
		return nil, errors.New("moteur de synchro : base non renseignée")
	}
	if opts.Auth == nil {
		return nil, errors.New("moteur de synchro : accès à gh non renseigné")
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	newProvider := opts.NewProvider
	if newProvider == nil {
		cache, quota := github.NewCache(0, 0), github.NewQuota()
		userAgent := opts.UserAgent
		newProvider = func(token string) (provider.Provider, error) {
			return github.New(token, github.Options{Cache: cache, Quota: quota, UserAgent: userAgent, Logger: logger})
		}
	}
	newGit := opts.NewGitProvider
	if newGit == nil {
		dir, protocol := opts.ReposDir, opts.Config.Git.Protocol
		newGit = func() (provider.Provider, error) {
			if dir == "" {
				return nil, errors.New("répertoire des clones Git locaux non renseigné")
			}
			return gitprov.New(gitprov.Options{Dir: dir, Protocol: protocol, Logger: logger})
		}
	}
	return &Engine{
		store:       opts.Store,
		auth:        opts.Auth,
		cfg:         opts.Config,
		newProvider: newProvider,
		newGit:      newGit,
		now:         now,
		logger:      logger,
	}, nil
}

// SyncOptions règle une synchro.
type SyncOptions struct {
	// Progress, si non nil, reçoit les événements de progression. Les appels sont sérialisés par le
	// moteur (jamais deux en même temps) mais peuvent venir de goroutines différentes ; Progress ne
	// doit pas bloquer longtemps (le TUI y fait un tea.Program.Send).
	Progress func(Event)
}

// SyncAll synchronise tous les dépôts suivis (Enabled), au plus sync.parallelism à la fois (errgroup),
// dans l'ordre de store.ListRepositories, et renvoie le rapport (un RepoReport par dépôt, dans cet
// ordre).
//
// Déroulement :
//  1. token lu avec Auth.Token, Provider construit (NewProvider) ; un échec ici est renvoyé comme
//     erreur (aucun dépôt n'est synchronisé, rapport vide) ;
//  2. si aucun quota n'a encore été observé (LastRateLimit), un appel RateLimit (gratuit) ; son
//     échec est ignoré ;
//  3. avant chaque dépôt, garde-fou : si LastRateLimit indique Remaining*100 < Limit*
//     QuotaReservePercent, ce dépôt et les suivants sont reportés (StatusDeferred, ResumeAt = Reset) ;
//  4. verrou store.BeginSyncRun(LockStaleAfter) : store.ErrLocked → StatusLocked, rien d'autre ;
//  5. synchro du dépôt (voir SyncRepo) ; une erreur est enregistrée avec store.FinishSyncRun
//     (message en français, sans donnée personnelle) et n'arrête pas les autres dépôts ;
//  6. une *provider.RateLimitError arrête la synchro des dépôts pas encore commencés : ils sont
//     reportés (StatusDeferred, ResumeAt = RateLimitError.Reset) ; celui qui l'a reçue est en
//     StatusError.
//
// L'erreur renvoyée est réservée aux échecs globaux (token, store, ctx annulé : les dépôts en cours
// sont alors clos en erreur) ; les échecs par dépôt sont dans le rapport.
func (e *Engine) SyncAll(ctx context.Context, opts SyncOptions) (Report, error) {
	all, err := e.store.ListRepositories(ctx)
	if err != nil {
		return e.emptyReport(opts), fmt.Errorf("synchronisation : %w", err)
	}
	repos := make([]model.Repository, 0, len(all))
	for _, r := range all {
		if r.Enabled {
			repos = append(repos, r)
		}
	}
	report, err := e.run(ctx, repos, opts)
	if err != nil {
		return report, fmt.Errorf("synchronisation : %w", err)
	}
	return report, nil
}

// SyncRepo synchronise le seul dépôt repoID (même suivi arrêté : c'est une demande explicite), avec
// le même verrou et le même garde-fou de quota que SyncAll, et renvoie un rapport à une entrée.
// Erreur enveloppant store.ErrNotFound si le dépôt n'existe pas.
//
// Stratégie pour un dépôt (documentation/03-architecture.md, « Synchronisation incrémentale ») :
//   - RepositoryByID(provider_id) : suit renommage et transfert, donne les métadonnées à jour ;
//   - Branches, filtrées par SelectBranches (branch_patterns du dépôt, branches.exclude) ; branche de
//     référence (ReferenceBranch) traitée en premier ; branches disparues → tracked = false ;
//   - aucun commit en base (premier import) : ListCommits de la branche de référence sans Stop
//     (borné par provider.MaxCommits) ;
//   - branche de référence changée (autre que celle de la synchro précédente : branche par défaut
//     renommée ou changée, ancienne référence supprimée ou exclue) : ListCommits sans Stop aussi, pour
//     la rattacher à tout son historique (commits déjà en base non réenregistrés) ;
//   - branche connue dont la tête a changé : CompareCommits(ancienne tête, nouvelle tête) ;
//   - nouvelle branche (ou autres branches du premier import) : CompareCommits(tête de la branche de
//     référence, tête de la branche) ;
//   - ErrUnknownBase : nouvel essai depuis la tête de la branche de référence (si la branche n'est
//     pas elle-même la référence), sinon ListCommits de la branche sans Stop ;
//   - tête inchangée : aucune requête de commits ; dépôt vide : succès sans commit ;
//   - statistiques (CommitStats) seulement si le dépôt avait déjà des commits avant cette synchro,
//     pour les commits absents de la base (store.ExistingCommitSHAs), au plus MaxStatsPerRepo, et
//     jamais quand le quota restant est sous QuotaReservePercent ; sinon additions / deletions à 0 ;
//   - pull requests (depuis la plus récente date de modification en base), releases et tags (V2,
//     voir fetchExtras) : un échec devient un avertissement (RepoReport.Warnings), le dépôt réussit ;
//   - écriture : store.ApplyRepoSync (une transaction).
func (e *Engine) SyncRepo(ctx context.Context, repoID int64, opts SyncOptions) (Report, error) {
	repo, err := e.store.GetRepository(ctx, repoID)
	if err != nil {
		return e.emptyReport(opts), fmt.Errorf("synchronisation du dépôt %d : %w", repoID, err)
	}
	report, err := e.run(ctx, []model.Repository{repo}, opts)
	if err != nil {
		return report, fmt.Errorf("synchronisation du dépôt %s : %w", repo.FullName(), err)
	}
	return report, nil
}

// RateLimit lit le token, construit un Provider et renvoie l'état du quota (Provider.RateLimit,
// gratuit). Sert à « watchoo status ». Sans gh (clones Git locaux), erreur enveloppant
// provider.ErrUnsupported : il n'y a pas de quota.
func (e *Engine) RateLimit(ctx context.Context) (provider.RateLimit, error) {
	prov, _, _, err := e.openProvider(ctx)
	if err != nil {
		return provider.RateLimit{}, fmt.Errorf("lecture du quota : %w", err)
	}
	rl, err := prov.RateLimit(ctx)
	if err != nil {
		return provider.RateLimit{}, fmt.Errorf("lecture du quota : %w", err)
	}
	return rl, nil
}

// Source renvoie la source que prendrait une synchro lancée maintenant (voir le commentaire du
// package), sans rien construire ni contacter : seul gh est interrogé (auth.Client.Token, qui ne
// contacte pas GitHub). Erreur si gh est installé mais en panne (délai, réponse illisible).
func (e *Engine) Source(ctx context.Context) (Source, error) {
	_, src, err := e.chooseSource(ctx)
	return src, err
}

// chooseSource lit le token de gh : token et SourceAPI si gh est utilisable, SourceGit (avec la raison)
// s'il est absent ou non connecté (auth.Absent), erreur sinon.
func (e *Engine) chooseSource(ctx context.Context) (string, Source, error) {
	token, err := e.auth.Token(ctx)
	switch {
	case err == nil:
		return token, Source{Kind: SourceAPI}, nil
	case ctx.Err() != nil:
		return "", Source{}, ctx.Err()
	case auth.Absent(err):
		return "", Source{Kind: SourceGit, Reason: err.Error()}, nil
	default:
		return "", Source{}, fmt.Errorf("lecture du token GitHub : %w", err)
	}
}

// openProvider choisit la source (chooseSource) et construit le Provider : API GitHub avec le token
// lu, ou clones Git locaux sans token. Le token n'est renvoyé que sous la forme d'une fonction de
// masquage (redact), qui le retire d'un message avant qu'il ne soit enregistré ou affiché : défense
// supplémentaire, le Provider ne le cite jamais. Le token n'est gardé que par cette fermeture, le
// temps de la synchro.
func (e *Engine) openProvider(ctx context.Context) (provider.Provider, func(string) string, Source, error) {
	token, src, err := e.chooseSource(ctx)
	if err != nil {
		return nil, nil, Source{}, err
	}
	if src.Kind == SourceGit {
		prov, err := e.newGit()
		if err != nil {
			return nil, nil, Source{}, fmt.Errorf("dépôts Git locaux (gh absent ou non connecté) : %w", err)
		}
		e.logger.Debug("synchro par clones Git locaux : gh absent ou non connecté")
		return prov, func(msg string) string { return msg }, src, nil
	}
	prov, err := e.newProvider(token)
	if err != nil {
		return nil, nil, Source{}, fmt.Errorf("connexion à GitHub : %w", err)
	}
	redact := func(msg string) string {
		if token == "" {
			return msg
		}
		return strings.ReplaceAll(msg, token, "[token masqué]")
	}
	return prov, redact, src, nil
}

// forEach lance fn(i) pour i de 0 à n-1, au plus limit à la fois (errgroup.SetLimit, limit ramené
// entre 1 et workspace.MaxSyncParallelism). fn ne renvoie pas d'erreur : chaque dépôt consigne la
// sienne dans son rapport, pour qu'un échec n'arrête pas les autres.
func forEach(n, limit int, fn func(i int)) {
	limit = max(1, min(limit, workspace.MaxSyncParallelism))
	var g errgroup.Group
	g.SetLimit(limit)
	for i := range n {
		g.Go(func() error {
			fn(i)
			return nil
		})
	}
	_ = g.Wait() // fn ne renvoie jamais d'erreur
}
