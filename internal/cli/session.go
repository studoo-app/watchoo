package cli

// Ouverture de l'espace et de la base, partagée par toutes les commandes sauf init, version et help.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// session est l'espace résolu et sa base ouverte, le temps d'une commande.
type session struct {
	ws     *workspace.Workspace
	store  *store.Store
	loc    *time.Location // fuseau d'affichage (Config.Location)
	deps   Deps
	opts   *options
	logger *slog.Logger // logs du moteur et du provider (deps.Logger, sinon newLogger)
}

// openSession résout l'espace comme helpStatus (deps.Getwd, -C, WATCHOO_DIR, puis
// workspace.ResolveFrom), ouvre .watchoo/watchoo.db (store.Open) et applique les migrations
// (store.Migrate). Hors d'un espace, l'erreur enveloppe workspace.ErrNotFound et son message dit de
// lancer « watchoo init » (ou d'utiliser -C). L'appelant ferme la session (Close).
func openSession(ctx context.Context, deps Deps, opts *options) (*session, error) {
	ws, err := resolveForHelp(deps, opts)
	if err != nil {
		if errors.Is(err, workspace.ErrNotFound) {
			return nil, &hintError{
				msg: err.Error() + " (ou indiquez un espace avec -C <répertoire>)",
				err: err,
			}
		}
		return nil, err
	}
	loc, err := ws.Config.Location()
	if err != nil {
		return nil, err
	}
	st, err := store.Open(ctx, ws.DBPath())
	if err != nil {
		return nil, fmt.Errorf("ouverture de la base de l'espace : %w", err)
	}
	if err := st.Migrate(ctx); err != nil {
		_ = st.Close() // l'erreur de migration est la plus utile
		return nil, fmt.Errorf("mise à jour de la base de l'espace : %w", err)
	}
	logger := deps.Logger
	if logger == nil {
		logger = newLogger(deps.Stderr, os.Getenv(EnvLogLevel), opts.verbose)
	}
	return &session{ws: ws, store: st, loc: loc, deps: deps, opts: opts, logger: logger}, nil
}

// Close ferme la base.
func (s *session) Close() error {
	return s.store.Close()
}

// closeSession ferme la session à la fin d'une commande ; une erreur de fermeture n'est renvoyée que si
// la commande a réussi (*err nil).
func closeSession(s *session, err *error) {
	if cerr := s.Close(); cerr != nil && *err == nil {
		*err = fmt.Errorf("fermeture de la base : %w", cerr)
	}
}

// engine construit le moteur de synchro de l'espace : store de la session, deps.NewAuthClient(config),
// deps.NewProvider (nil = provider GitHub par défaut), deps.NewGitProvider (nil = clones Git locaux
// dans .watchoo/repos, sans gh), UserAgent « watchoo/<version> » (appVersion),
// deps.Now, logger de la session (deps.Logger, sinon newLogger sur la sortie d'erreur).
func (s *session) engine() (*wsync.Engine, error) {
	return s.engineWithLogger(s.logger)
}

// engineWithLogger est engine avec un logger choisi (le TUI fait taire les logs : io.Discard).
func (s *session) engineWithLogger(logger *slog.Logger) (*wsync.Engine, error) {
	return wsync.New(wsync.Options{
		Store:          s.store,
		Auth:           s.deps.NewAuthClient(s.ws.Config),
		Config:         s.ws.Config,
		NewProvider:    s.deps.NewProvider,
		NewGitProvider: s.deps.NewGitProvider,
		ReposDir:       s.ws.ReposDir(),
		UserAgent:      "watchoo/" + appVersion(),
		Now:            s.deps.Now,
		Logger:         logger,
	})
}

// findRepository renvoie le dépôt suivi désigné par ref (« owner/repo » ou URL, repoimport.ParseRef).
// Dépôt non suivi : erreur claire qui cite ref et renvoie vers « watchoo repo list ».
func (s *session) findRepository(ctx context.Context, ref string) (model.Repository, error) {
	parsed, err := repoimport.ParseRef(ref)
	if err != nil {
		return model.Repository{}, err
	}
	repo, err := s.store.GetRepositoryByName(ctx, model.ProviderGitHub, parsed.Owner, parsed.Name)
	if errors.Is(err, store.ErrNotFound) {
		return model.Repository{}, &hintError{
			msg: fmt.Sprintf("le dépôt %s n'est pas suivi dans cet espace (dépôts suivis : « watchoo repo list »)",
				parsed),
			err: err,
		}
	}
	if err != nil {
		return model.Repository{}, err
	}
	return repo, nil
}

// now renvoie l'heure courante (deps.Now, sinon time.Now).
func (s *session) now() time.Time {
	if s.deps.Now != nil {
		return s.deps.Now()
	}
	return time.Now()
}
