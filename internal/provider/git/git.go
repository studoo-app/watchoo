// Package git est le provider des dépôts Git locaux (V2) : il lit les dépôts GitHub suivis depuis des
// clones avec copie de travail rangés dans repos/, à la racine de l'espace (repos/<name>, voir Layout
// pour les homonymes), mis à jour à chaque synchro par « git fetch » puis remis sur la branche par
// défaut, sans API ni token (documentation/fr/03-architecture.md, « Dépôts Git locaux »).
//
// Le moteur de synchro l'utilise comme source quand gh n'est pas installé ou pas connecté, et pour les
// statistiques de lignes dans tous les cas (voir « Statistiques avec ou sans gh »). Il implémente
// provider.Provider avec les mêmes conventions (validation avant tout appel, erreurs typées, dates en
// UTC, listes vides non nil, usage concurrent). Name renvoie « github » : c'est la forge des dépôts,
// git n'est que le moyen d'y accéder.
//
// # Exécution de git
//
// Le binaire git est lancé par os/exec, avec des tableaux d'arguments (jamais de shell), « -- » avant
// les URL et les références quand git l'accepte, et seulement des commandes de lecture du dépôt
// distant (ls-remote, fetch) ou du clone (for-each-ref, log, rev-parse, diff…), plus la création du
// clone (init) et la remise de sa copie de travail sur la branche par défaut (symbolic-ref, checkout,
// reset). Jamais de push, jamais de commande dans un dépôt de l'utilisateur : seuls les clones signés
// par Watchoo dans Options.Dir sont touchés, désignés par --git-dir (et --work-tree), sans recherche
// du dépôt depuis le répertoire courant.
//
// # Dépôts d'étudiants : rien ne s'exécute
//
// Les dépôts suivis ne sont pas de confiance. Aucun hook ne s'exécute (core.hooksPath vers le
// périphérique nul ; les hooks d'un dépôt distant ne sont de toute façon jamais copiés), aucun
// sous-module n'est lu (submodule.recurse et fetch.recurseSubmodules à false, --no-recurse-submodules),
// aucun fichier LFS n'est téléchargé (GIT_LFS_SKIP_SMUDGE=1 : les pointeurs restent tels quels),
// core.fsmonitor est coupé. Les variables d'environnement qui désignent un autre dépôt (GIT_DIR,
// GIT_WORK_TREE, GIT_INDEX_FILE…) sont retirées. core.symlinks garde sa valeur : git refuse déjà tout
// chemin qui sortirait de la copie de travail.
//
// # Identifiants
//
// Watchoo ne lit, ne demande ni ne stocke aucun identifiant ; aucune URL ne contient de token. git
// s'appuie sur ce que l'utilisateur a configuré (clé SSH, agent SSH, gestionnaire d'identifiants). Il
// est lancé sans entrée standard et sans invite : GIT_TERMINAL_PROMPT=0, GCM_INTERACTIVE=never et,
// si l'utilisateur n'a choisi aucune commande SSH (GIT_SSH_COMMAND, GIT_SSH, core.sshCommand),
// GIT_SSH_COMMAND="ssh -o BatchMode=yes" : ssh échoue au lieu de demander une phrase de passe ou de
// faire confiance à un hôte inconnu. Une commande SSH choisie par l'utilisateur est gardée telle
// quelle. Un échec d'accès donne provider.ErrAccessDenied (« accès refusé »).
//
// # Ce que la source ne fournit pas
//
// Ni identifiant numérique de dépôt (RepositoryByID : ErrUnsupported), ni login GitHub des auteurs
// (nom et e-mail Git seulement), ni étoiles, issues, état archivé (Repository.Partial), ni pull
// requests, ni releases (ErrUnsupported), ni quota (RateLimit : ErrUnsupported ; LastRateLimit :
// false). Les tags et les statistiques des commits (« --numstat ») sont disponibles.
//
// # Statistiques avec ou sans gh
//
// Le moteur ouvre aussi ce Provider quand gh est utilisé (« toujours cloner ») : Prepare crée ou met à
// jour le clone avec la branche par défaut donnée par l'API, puis CommitStatsBatch calcule les
// statistiques de lignes d'un paquet de commits. Sans gh, il sert de source complète.
//
// # Logs
//
// Niveau Debug seulement : commande git (sans URL ni argument), dépôt, durée, code de retour. Jamais
// la sortie de git, un message de commit, un nom ni un e-mail.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	gosync "sync"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// i18nError est une erreur sentinelle dont le message vient du catalogue i18n : le texte est traduit
// au moment de l'affichage (Error), dans la langue du processus. La valeur reste comparable avec ==
// (une structure d'un champ) : errors.Is retrouve l'erreur sentinelle à travers les emballages.
type i18nError struct{ key string }

func (e i18nError) Error() string { return i18n.T(e.key) }

// errWrapped compose un texte traduit et une erreur enveloppée, avec la ponctuation de la langue
// (« texte : err » en français, « texte: err » en anglais).
func errWrapped(text string, err error) error {
	return fmt.Errorf("%s%s%w", text, i18n.T("provider.err.sep"), err)
}

// Valeurs par défaut et réglages.
const (
	// DefaultPath est l'exécutable git, cherché dans le PATH.
	DefaultPath = "git"
	// ProtocolSSH et ProtocolHTTPS sont les protocoles d'accès à GitHub (réglage git.protocol).
	ProtocolSSH   = "ssh"
	ProtocolHTTPS = "https"
	// DefaultNetworkTimeout borne une commande qui contacte le dépôt distant (ls-remote, clone, fetch).
	DefaultNetworkTimeout = 10 * time.Minute
	// DefaultLocalTimeout borne une commande de lecture du clone.
	DefaultLocalTimeout = 2 * time.Minute
	// WebBaseURL est la racine des pages web des dépôts (URL d'un dépôt, d'un commit).
	WebBaseURL = "https://github.com"
	// batchSSH est la commande SSH posée quand l'utilisateur n'en a choisi aucune.
	batchSSH = "ssh -o BatchMode=yes"
	// maxStderr borne la sortie d'erreur gardée pour classer un échec (jamais recopiée).
	maxStderr = 64 << 10
	// waitDelay est le temps laissé aux tubes pour se fermer après la fin de git.
	waitDelay = 2 * time.Second
)

// ErrGitNotFound : l'exécutable git est introuvable.
var ErrGitNotFound = i18nError{"provider.git.err.not_found"}

// Options règle New.
type Options struct {
	// Dir est le répertoire des clones (repos/ à la racine de l'espace) ; obligatoire. Il est créé en
	// 0700 au premier clone.
	Dir string
	// Repos est la liste des dépôts de l'espace (suivis ou arrêtés), dans l'ordre de leur ajout
	// (identifiant croissant) : elle fixe l'emplacement des clones des homonymes (Layout). Un dépôt
	// absent de la liste est rangé comme s'il venait à la suite.
	Repos []Ref
	// Protocol est ProtocolSSH (défaut, « git@github.com:owner/name.git ») ou ProtocolHTTPS
	// (« https://github.com/owner/name.git »).
	Protocol string
	// BaseURL, pour les tests, remplace l'adresse du dépôt distant par BaseURL/owner/name.git (ex.
	// « file:///tmp/remotes ») ; vide = GitHub selon Protocol.
	BaseURL string
	// GitPath est l'exécutable git ; vide = DefaultPath.
	GitPath string
	// NetworkTimeout et LocalTimeout bornent chaque commande ; 0 = valeurs par défaut.
	NetworkTimeout time.Duration
	LocalTimeout   time.Duration
	// Logger reçoit les logs de diagnostic (Debug) ; nil = slog.Default().
	Logger *slog.Logger
}

// Provider est le provider des clones Git locaux. Construire avec New ; sûr pour un usage concurrent.
// Un Provider vit le temps d'une synchro : chaque clone y est créé ou mis à jour (fetch et copie de
// travail) au plus une fois, au premier besoin.
type Provider struct {
	dir            string
	protocol       string
	baseURL        string
	gitPath        string
	networkTimeout time.Duration
	localTimeout   time.Duration
	logger         *slog.Logger
	layout         *Layout

	envOnce gosync.Once
	env     []string

	mu       gosync.Mutex
	repos    map[string]*repoState // clé : owner/name en minuscules
	defaults map[string]string     // branche par défaut lue par Repository ; même clé
}

// repoState sérialise la mise à jour du clone d'un dépôt.
type repoState struct {
	mu      gosync.Mutex
	fetched bool
}

// Vérifie à la compilation que *Provider implémente provider.Provider.
var _ provider.Provider = (*Provider)(nil)

// New vérifie les options et construit un Provider (aucune commande lancée).
func New(opts Options) (*Provider, error) {
	if opts.Dir == "" {
		return nil, errors.New(i18n.T("provider.git.err.dir_missing"))
	}
	protocol := opts.Protocol
	if protocol == "" {
		protocol = ProtocolSSH
	}
	if protocol != ProtocolSSH && protocol != ProtocolHTTPS {
		return nil, errors.New(i18n.T("provider.git.err.protocol_unknown", protocol, ProtocolSSH, ProtocolHTTPS))
	}
	gitPath := opts.GitPath
	if gitPath == "" {
		gitPath = DefaultPath
	}
	p := &Provider{
		dir:            opts.Dir,
		protocol:       protocol,
		baseURL:        strings.TrimSuffix(opts.BaseURL, "/"),
		gitPath:        gitPath,
		networkTimeout: opts.NetworkTimeout,
		localTimeout:   opts.LocalTimeout,
		logger:         opts.Logger,
		layout:         NewLayout(opts.Dir, opts.Repos),
		repos:          make(map[string]*repoState),
		defaults:       make(map[string]string),
	}
	if p.networkTimeout <= 0 {
		p.networkTimeout = DefaultNetworkTimeout
	}
	if p.localTimeout <= 0 {
		p.localTimeout = DefaultLocalTimeout
	}
	if p.logger == nil {
		p.logger = slog.Default()
	}
	return p, nil
}

// Name renvoie la forge des dépôts suivis : « github ».
func (p *Provider) Name() string { return model.ProviderGitHub }

// String décrit le Provider sans rien en révéler.
func (p *Provider) String() string { return "provider Git local" }

// Check vérifie que l'exécutable git (gitPath, vide = DefaultPath) peut être lancé (« git --version ») :
// erreur enveloppant ErrGitNotFound sinon. Sert à « watchoo init » sans gh.
func Check(ctx context.Context, gitPath string) error {
	if gitPath == "" {
		gitPath = DefaultPath
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gitPath, "--version")
	cmd.Stdin = nil
	cmd.WaitDelay = waitDelay
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil && !errors.Is(ctxErr, context.DeadlineExceeded) {
			return ctxErr
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			return ErrGitNotFound
		}
		return errWrapped(i18n.T("provider.git.err.launch"), ErrGitNotFound)
	}
	return nil
}

// remoteURL renvoie l'adresse du dépôt distant owner/name (déjà validés). Jamais de token.
func (p *Provider) remoteURL(owner, name string) string {
	switch {
	case p.baseURL != "":
		// Comme GitHub, le faux serveur des tests (gittest) ignore la casse : ses dépôts sont rangés en
		// minuscules, quel que soit le système de fichiers.
		return p.baseURL + "/" + strings.ToLower(owner) + "/" + strings.ToLower(name) + ".git"
	case p.protocol == ProtocolHTTPS:
		return WebBaseURL + "/" + owner + "/" + name + ".git"
	default:
		return "git@github.com:" + owner + "/" + name + ".git"
	}
}

// webURL renvoie la page web du dépôt owner/name sur GitHub.
func webURL(owner, name string) string {
	return WebBaseURL + "/" + owner + "/" + name
}

// result est le résultat d'une commande git allée à son terme.
type result struct {
	stdout   []byte
	stderr   string
	exitCode int
}

// gitEnv renvoie l'environnement de git : celui hérité, plus de quoi ne jamais attendre de saisie (voir
// la documentation du package). Calculé une fois par Provider.
func (p *Provider) gitEnv(ctx context.Context) []string {
	p.envOnce.Do(func() {
		env := make([]string, 0, len(os.Environ())+8)
		for _, kv := range os.Environ() {
			if !isRepoEnv(kv) {
				env = append(env, kv)
			}
		}
		env = append(env,
			"GIT_TERMINAL_PROMPT=0", // pas d'invite pour un identifiant HTTPS
			"GIT_LFS_SKIP_SMUDGE=1", // fichiers LFS : pointeurs gardés, rien de téléchargé
			"GCM_INTERACTIVE=never", // Git Credential Manager : pas de fenêtre
			"LC_ALL=C",              // messages de git en anglais : classement des échecs
			"LANGUAGE=C",
		)
		if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" && !p.userSSHCommand(ctx) {
			env = append(env, "GIT_SSH_COMMAND="+batchSSH)
		}
		p.env = env
	})
	return p.env
}

// repoEnv sont les variables d'environnement qui feraient travailler git sur un autre dépôt que le
// clone désigné : retirées de l'environnement de git.
var repoEnv = []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE", "GIT_CEILING_DIRECTORIES",
	"GIT_DISCOVERY_ACROSS_FILESYSTEM"}

// isRepoEnv indique si kv (« NOM=valeur ») est l'une des variables de repoEnv.
func isRepoEnv(kv string) bool {
	name, _, _ := strings.Cut(kv, "=")
	for _, r := range repoEnv {
		if strings.EqualFold(name, r) {
			return true
		}
	}
	return false
}

// userSSHCommand indique si la configuration Git de l'utilisateur choisit une commande SSH
// (core.sshCommand) : elle est alors gardée.
func (p *Provider) userSSHCommand(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, p.localTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, p.gitPath, "config", "--get", "core.sshCommand")
	cmd.Stdin = nil
	cmd.WaitDelay = waitDelay
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) != ""
}

// run lance « git <args> » (gitDir non vide : « git --git-dir=<gitDir> <args> » ; les commandes qui
// touchent la copie de travail commencent args par « --work-tree=<clone> »), borné par timeout.
// name (« fetch »…) sert aux logs. Un code de retour non nul n'est pas une erreur de run : il est dans
// result. Erreur si git n'a pas pu être lancé (ErrGitNotFound), si le délai a expiré
// (provider.ErrUnavailable) ou si ctx est terminé (ctx.Err()).
func (p *Provider) run(ctx context.Context, timeout time.Duration, gitDir, name string, args ...string) (result, error) {
	return p.runInput(ctx, timeout, gitDir, name, nil, args...)
}

// runInput est run avec stdin pour entrée standard (nil = aucune) : listes de SHA des commandes
// « --stdin » (rev-list, diff-tree).
func (p *Provider) runInput(ctx context.Context, timeout time.Duration, gitDir, name string, stdin []byte,
	args ...string) (result, error) {
	full := make([]string, 0, len(args)+8)
	// Lecture seule et sortie stable, quelle que soit la configuration de l'utilisateur.
	full = append(full, "-c", "log.showSignature=false", "-c", "color.ui=false", "-c", "core.quotePath=false",
		// Entretien automatique du clone (après un fetch) au premier plan : un processus détaché
		// garderait les sorties ouvertes après la fin de git.
		"-c", "gc.autoDetach=false", "-c", "maintenance.autoDetach=false",
		// Dépôts d'étudiants : ni hook, ni sous-module, ni fsmonitor (voir la documentation du package).
		"-c", "core.hooksPath="+os.DevNull, "-c", "submodule.recurse=false",
		"-c", "fetch.recurseSubmodules=false", "-c", "core.fsmonitor=false",
		"-c", "advice.detachedHead=false")
	if gitDir != "" {
		full = append(full, "--git-dir="+gitDir)
	}
	full = append(full, args...)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout bytes.Buffer
	stderr := &cappedBuffer{max: maxStderr}
	cmd := exec.CommandContext(runCtx, p.gitPath, full...)
	cmd.Env = p.gitEnv(ctx)
	// Répertoire neutre : lancé depuis un dépôt Git de l'utilisateur (un clone d'étudiant, un worktree),
	// git lirait sa configuration, voire échouerait s'il est illisible.
	cmd.Dir = os.TempDir()
	cmd.Stdin = nil
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = &stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = waitDelay

	start := time.Now()
	err := cmd.Run()
	res := result{stdout: stdout.Bytes(), stderr: stderr.String()}
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case ctx.Err() != nil:
			return result{}, ctx.Err()
		case runCtx.Err() != nil:
			return result{}, errWrapped(i18n.T("provider.git.err.timeout", name, timeout), provider.ErrUnavailable)
		case errors.As(err, &exitErr):
			res.exitCode = exitErr.ExitCode()
		case errors.Is(err, exec.ErrNotFound) || errors.Is(err, fs.ErrNotExist):
			return result{}, ErrGitNotFound
		default:
			return result{}, errWrapped(i18n.T("provider.git.err.launch_cmd", name), ErrGitNotFound)
		}
	}
	p.logger.Debug("commande git", "commande", name, "code", res.exitCode,
		"duree", time.Since(start).Round(time.Millisecond))
	return res, nil
}

// cappedBuffer garde au plus max octets.
type cappedBuffer struct {
	buf bytes.Buffer
	max int
}

func (c *cappedBuffer) Write(b []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(b[:min(len(b), room)])
	}
	return len(b), nil
}

func (c *cappedBuffer) String() string { return c.buf.String() }

// Motifs (en minuscules) de la sortie d'erreur de git, lancé avec LC_ALL=C.
var (
	unavailablePatterns = []string{
		"could not resolve host", "could not resolve hostname", "connection refused", "connection timed out",
		"operation timed out", "network is unreachable", "no route to host", "failed to connect",
		"connection reset", "returned error: 5", "temporary failure in name resolution",
	}
	accessPatterns = []string{
		"permission denied", "repository not found", "could not read username", "could not read password",
		"authentication failed", "terminal prompts disabled", "host key verification failed",
		"does not appear to be a git repository", "could not read from remote repository", "access denied",
		"returned error: 4", "not found", "no such file or directory",
	}
)

// remoteError traduit l'échec d'une commande qui contacte le dépôt distant : provider.ErrUnavailable
// (réseau), provider.ErrAccessDenied (accès refusé, dépôt introuvable) ou provider.ErrUnexpected. La
// sortie de git n'est jamais recopiée.
func remoteError(op string, res result) error {
	stderr := strings.ToLower(res.stderr)
	cause := provider.ErrUnexpected
	switch {
	case containsAny(stderr, unavailablePatterns):
		cause = provider.ErrUnavailable
	case containsAny(stderr, accessPatterns):
		cause = provider.ErrAccessDenied
	}
	return &provider.APIError{Op: i18n.T("provider.git.op.git_suffix", op, res.exitCode), Err: cause}
}

// localError traduit l'échec d'une commande de lecture du clone.
func localError(op string, res result) error {
	return &provider.APIError{Op: i18n.T("provider.git.op.git_suffix", op, res.exitCode), Err: provider.ErrUnexpected}
}

// containsAny indique si s contient l'un des motifs.
func containsAny(s string, patterns []string) bool {
	for _, pat := range patterns {
		if strings.Contains(s, pat) {
			return true
		}
	}
	return false
}

// unsupported renvoie l'erreur d'une notion que la source ne connaît pas.
func unsupported(op string) error {
	return &provider.APIError{Op: i18n.T("provider.git.op.local_suffix", op), Err: provider.ErrUnsupported}
}

// Repository vérifie l'accès au dépôt distant (« git ls-remote --symref <url> HEAD », sans rien
// écrire) et renvoie ce que git en sait : owner et name tels que donnés, branche par défaut (HEAD du
// distant ; vide pour un dépôt vide), URL de la page GitHub, Partial vrai. Accès refusé :
// provider.ErrAccessDenied.
func (p *Provider) Repository(ctx context.Context, owner, name string) (provider.Repository, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.Repository{}, err
	}
	op := i18n.T("provider.op.repo", owner, name)
	res, err := p.run(ctx, p.networkTimeout, "", "ls-remote",
		"ls-remote", "--symref", "--", p.remoteURL(owner, name), "HEAD")
	if err != nil {
		return provider.Repository{}, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return provider.Repository{}, remoteError(op, res)
	}
	branch := parseSymref(res.stdout)
	p.mu.Lock()
	p.defaults[marker(owner, name)] = branch // pour la copie de travail (checkout)
	p.mu.Unlock()
	return provider.Repository{
		Owner:         owner,
		Name:          name,
		DefaultBranch: branch,
		URL:           webURL(owner, name),
		Partial:       true,
	}, nil
}

// parseSymref lit la branche visée par HEAD dans la sortie de « ls-remote --symref » :
// « ref: refs/heads/main<TAB>HEAD ».
func parseSymref(out []byte) string {
	for line := range strings.SplitSeq(string(out), "\n") {
		target, ok := strings.CutPrefix(line, "ref: ")
		if !ok {
			continue
		}
		ref, sym, ok := strings.Cut(target, "\t")
		if !ok || strings.TrimSpace(sym) != "HEAD" {
			continue
		}
		if branch, ok := strings.CutPrefix(ref, "refs/heads/"); ok {
			return branch
		}
	}
	return ""
}

// RepositoryByID : la source ne connaît pas les identifiants numériques (ErrUnsupported).
func (p *Provider) RepositoryByID(_ context.Context, id int64) (provider.Repository, error) {
	if err := provider.ValidateID(id); err != nil {
		return provider.Repository{}, err
	}
	return provider.Repository{}, unsupported(i18n.T("provider.git.op.repo_id", id))
}

// PullRequests : pas de pull requests dans un clone Git (ErrUnsupported).
func (p *Provider) PullRequests(_ context.Context, owner, name string, _ provider.PullRequestsOptions) ([]provider.PullRequest, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	return nil, unsupported(i18n.T("provider.git.op.pulls", owner, name))
}

// Releases : pas de releases dans un clone Git (ErrUnsupported).
func (p *Provider) Releases(_ context.Context, owner, name string) ([]provider.Release, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	return nil, unsupported(i18n.T("provider.git.op.releases", owner, name))
}

// RateLimit : pas de quota pour git (ErrUnsupported).
func (p *Provider) RateLimit(context.Context) (provider.RateLimit, error) {
	return provider.RateLimit{}, unsupported(i18n.T("provider.git.op.rate_limit"))
}

// LastRateLimit : aucun quota n'est jamais observé.
func (p *Provider) LastRateLimit() (provider.RateLimit, bool) {
	return provider.RateLimit{}, false
}

// ProfileURL renvoie la page du profil GitHub du compte login (« https://github.com/<login> »), sans
// commande ; vide si login n'a pas la forme d'un login (provider.ValidLogin). Les clones Git locaux ne
// connaissent aucun compte, mais la base peut en contenir (synchros précédentes par l'API).
func (p *Provider) ProfileURL(login string) string {
	if !provider.ValidLogin(login) {
		return ""
	}
	return WebBaseURL + "/" + login
}
