package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// setupDatabase crée la base SQLite de l'espace et lui applique les migrations. C'est une variable
// pour que les tests puissent simuler un échec (suppression de l'espace à moitié créé).
var setupDatabase = createDatabase

// initReport est le résumé de « watchoo init » (sortie texte ou JSON). Il ne contient jamais le token.
type initReport struct {
	Name       string `json:"name"`        // nom de l'espace (réglage name)
	Root       string `json:"root"`        // répertoire de l'espace
	Dir        string `json:"dir"`         // répertoire .watchoo/
	ConfigFile string `json:"config_file"` // .watchoo/config.yaml
	Database   string `json:"database"`    // .watchoo/watchoo.db
	// ParentWorkspace est la racine d'un espace parent déjà existant (espace imbriqué) ; vide sinon.
	ParentWorkspace string       `json:"parent_workspace"`
	GitHub          githubReport `json:"github"` // compte gh ; champs vides sans gh (Source « git »)
	// Source : « api » (gh installé et connecté) ou « git » (clones Git locaux, sans gh).
	Source   wsync.SourceKind `json:"source"`
	Warnings []string         `json:"warnings"` // avertissements, aussi affichés sur la sortie d'erreur
}

// githubReport décrit le compte gh vérifié par init, sans le token.
type githubReport struct {
	Host        string   `json:"host"`
	Login       string   `json:"login"`
	TokenSource string   `json:"token_source"` // origine du token selon gh (keyring, GH_TOKEN…)
	Scopes      []string `json:"scopes"`       // droits du token
}

// newInitCmd construit « watchoo init ».
func newInitCmd(deps Deps, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Fait du répertoire courant un espace de supervision",
		Long: `Fait du répertoire courant (ou de celui donné par -C) un espace de supervision :
vérifie la CLI GitHub gh, puis crée .watchoo/ avec config.yaml, la base SQLite au bon
schéma et un .gitignore. Le répertoire .watchoo/ est lisible par vous seul (droits
0700, fichiers en 0600).

Sans gh installé et connecté, init prévient que les dépôts seront suivis par clones
Git locaux (git doit alors être installé, sinon init s'arrête) et rappelle
« gh auth login » pour passer par l'API GitHub.

init refuse un répertoire qui est déjà un espace, pour ne jamais écraser de données,
et prévient si l'espace créé est imbriqué dans un autre espace. Il ne demande ni ne
stocke aucun token : gh le garde.`,
		Example: `  mkdir licence-projet && cd licence-projet && watchoo init
  watchoo init -C ~/supervision/master-projet --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), deps, opts)
		},
	}
}

// runInit enchaîne les étapes de « watchoo init » : réglages utilisateur, vérification de gh (Status
// seulement, jamais Token ; sans gh utilisable, vérification de git), création de .watchoo/, création
// et migration de la base, résumé.
func runInit(ctx context.Context, deps Deps, opts *options) error {
	root, err := initRoot(deps, opts.workspaceDir)
	if err != nil {
		return err
	}

	// Réglages utilisateur et environnement, pour savoir quel gh lancer et avec quel compte. Le
	// config.yaml d'un éventuel espace existant n'est pas lu : Init le refusera plus bas.
	cfg, err := workspace.LoadUserConfig(root)
	if err != nil {
		return err
	}
	source := wsync.SourceAPI
	var warning string // gh absent ou non connecté
	status, err := deps.NewAuthClient(cfg).Status(ctx)
	switch {
	case err == nil:
	case noGH(err, status, cfg):
		// gh absent ou non connecté (V2) : suivi par clones Git locaux, qui demande git.
		if gitErr := deps.CheckGit(ctx); gitErr != nil {
			return &hintError{
				msg: i18n.T("cli.repo.init.need_git", ghError(err, cfg).Error(), gitErr.Error()),
				err: errors.Join(err, gitErr),
			}
		}
		source, status = wsync.SourceGit, auth.Status{}
		warning = noGHWarning(err, cfg)
	default:
		return ghError(err, cfg)
	}

	res, err := workspace.Init(root)
	if err != nil {
		if errors.Is(err, workspace.ErrAlreadyInitialized) {
			return fmt.Errorf("%w%s%s", err, i18n.T("cli.core.err.sep"),
				i18n.T("cli.repo.init.nothing_modified"))
		}
		return err
	}
	ws := res.Workspace
	if err := finishInit(ctx, ws); err != nil {
		// Ne pas laisser un espace à moitié créé : Init vient de créer .watchoo/, il est à nous.
		if rmErr := os.RemoveAll(ws.Dir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("%s%s%w", i18n.T("cli.repo.init.cleanup", ws.Dir),
				i18n.T("cli.core.err.sep"), rmErr))
		}
		return err
	}

	report := initReport{
		Name:            ws.Config.Name,
		Root:            ws.Root,
		Dir:             ws.Dir,
		ConfigFile:      ws.ConfigPath(),
		Database:        ws.DBPath(),
		ParentWorkspace: res.ParentWorkspace,
		GitHub: githubReport{
			Host:        status.Host,
			Login:       status.Login,
			TokenSource: status.TokenSource,
			Scopes:      append([]string{}, status.Scopes...),
		},
		Source:   source,
		Warnings: initWarnings(deps, opts, ws.Root, res.ParentWorkspace),
	}
	if warning != "" {
		report.Warnings = append([]string{warning}, report.Warnings...)
	}
	for _, w := range report.Warnings {
		warn(deps.Stderr, w)
	}
	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	if term, ok := cardTerminal(deps, opts); ok {
		subtitle := i18n.T("cli.repo.init.created_card", report.Name)
		if card := renderCard(term, subtitle, tipInitDone); card != "" {
			return writeInitCard(deps.Stdout, term, card, report)
		}
	}
	return writeInitText(deps.Stdout, report)
}

// initRoot renvoie le répertoire à initialiser, absolu et nettoyé : celui de -C (relatif au
// répertoire courant), sinon le répertoire courant. WATCHOO_DIR ne s'applique pas à init.
func initRoot(deps Deps, flagDir string) (string, error) {
	if flagDir != "" && filepath.IsAbs(flagDir) {
		return filepath.Clean(flagDir), nil
	}
	cwd, err := deps.Getwd()
	if err != nil {
		return "", fmt.Errorf("%s%s%w", i18n.T("cli.repo.cwd_unavailable"),
			i18n.T("cli.core.err.sep"), err)
	}
	if flagDir != "" {
		return filepath.Join(cwd, flagDir), nil
	}
	return filepath.Clean(cwd), nil
}

// finishInit termine la création de l'espace ws que Init vient de créer : base SQLite migrée, puis
// relecture des réglages comme le feront les autres commandes (nom effectif de l'espace).
func finishInit(ctx context.Context, ws *workspace.Workspace) error {
	if err := setupDatabase(ctx, ws.DBPath()); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.repo.init.db_create"),
			i18n.T("cli.core.err.sep"), err)
	}
	if err := workspace.Load(ws); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.repo.init.reread_settings"),
			i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// createDatabase crée le fichier de base en 0600 (avant que SQLite ne l'ouvre, pour que les fichiers
// -wal et -shm héritent de ces droits), l'ouvre, applique les migrations et le ferme.
func createDatabase(ctx context.Context, path string) (err error) {
	if err := store.CreateDBFile(path); err != nil {
		return err
	}
	st, err := store.Open(ctx, path)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := st.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("%s%s%w", i18n.T("cli.repo.init.db_close", path),
				i18n.T("cli.core.err.sep"), cerr)
		}
	}()
	return st.Migrate(ctx)
}

// noGH indique un gh absent ou connecté à aucun compte (auth.Absent), qui fait suivre les dépôts par
// clones Git locaux. N'en font pas partie pour init : un réglage github.user inutilisable
// (ErrUnusableUser) ou qui désigne un compte que gh ne connaît pas (github.user renseigné), fautes de
// réglage à corriger avant de créer l'espace, et un token refusé par GitHub (ErrNotLoggedIn avec le
// login du compte dans status) : gh garde ce token, les synchros passeraient par l'API et
// échoueraient ; il faut se reconnecter.
func noGH(err error, status auth.Status, cfg workspace.Config) bool {
	if errors.Is(err, auth.ErrGHNotFound) {
		return true
	}
	return auth.Absent(err) && !errors.Is(err, auth.ErrUnusableUser) && status.Login == "" && cfg.GitHub.User == ""
}

// noGHWarning renvoie l'avertissement de init sans gh : les dépôts seront suivis par clones Git
// locaux, et comment passer par l'API GitHub.
func noGHWarning(err error, cfg workspace.Config) string {
	local := i18n.T("cli.repo.init.no_gh_local")
	if errors.Is(err, auth.ErrGHNotFound) {
		return i18n.T("cli.repo.init.gh_not_found", cfg.GitHub.GHPath, local,
			i18n.T("cli.repo.hint.install_gh"))
	}
	return i18n.T("cli.repo.init.gh_not_logged", local, i18n.T("cli.repo.hint.login_gh"))
}

// ghError traduit une erreur de vérification de gh en message qui dit quoi faire. L'erreur d'origine
// reste accessible par errors.Is. Le conseil « gh auth login » est réservé à un gh non connecté ou à
// un token refusé : ni un réglage github.user inutilisable (ErrUnusableUser, dont le message dit quoi
// corriger) ni un GitHub injoignable (ErrCheckFailed) ne se règlent en se reconnectant.
func ghError(err error, cfg workspace.Config) error {
	prefix := i18n.T("cli.repo.init.checking_gh") + i18n.T("cli.core.err.sep")
	hintLogin := i18n.T("cli.repo.hint.login_gh")
	switch {
	case errors.Is(err, auth.ErrGHNotFound):
		return &hintError{
			msg: i18n.T("cli.repo.init.gh_not_found_short", cfg.GitHub.GHPath,
				i18n.T("cli.repo.hint.install_gh")),
			err: err,
		}
	case errors.Is(err, auth.ErrUnusableUser):
		return fmt.Errorf("%s%w", prefix, err)
	case errors.Is(err, auth.ErrNotLoggedIn):
		msg := prefix + err.Error()
		// Le conseil est déjà présent si l'erreur d'auth le donne elle-même (dans sa langue) : la
		// ligne de commande « gh auth login » ne dépend pas de la langue.
		if !strings.Contains(msg, "gh auth login") {
			msg += i18n.T("cli.core.err.sep") + hintLogin
		}
		return &hintError{msg: msg, err: err}
	default:
		return fmt.Errorf("%s%w", prefix, err)
	}
}

// initWarnings renvoie les avertissements à afficher après un init réussi.
func initWarnings(deps Deps, opts *options, root, parent string) []string {
	warnings := []string{}
	if parent != "" {
		warnings = append(warnings, i18n.T("cli.repo.init.nested", root, parent))
	}
	// WATCHOO_DIR passe avant le répertoire courant pour les autres commandes : si l'espace qu'elle
	// leur fait trouver n'est pas celui qui vient d'être créé, elles ne l'utiliseront pas. La
	// vérification est celle des autres commandes (ResolveFrom sans -C, depuis le répertoire
	// courant) : un WATCHOO_DIR inexistant ou qui désigne un fichier est donc signalé lui aussi.
	if env := os.Getenv(workspace.EnvDir); env != "" && opts.workspaceDir == "" {
		cwd, err := deps.Getwd()
		if err != nil {
			cwd = root // init vient de réussir dans le répertoire courant
		}
		switch found, err := workspace.ResolveFrom("", cwd); {
		case err != nil:
			warnings = append(warnings, i18n.T("cli.repo.init.env_err", workspace.EnvDir, env, err))
		case found.Root != root:
			warnings = append(warnings, i18n.T("cli.repo.init.env_other", workspace.EnvDir, env, found.Root))
		}
	}
	return warnings
}

// writeInitText affiche le résumé de init en texte, hors terminal (sur un terminal : writeInitCard).
func writeInitText(w io.Writer, r initReport) error {
	_, err := fmt.Fprint(w, i18n.T("cli.repo.init.summary", r.Name, r.Root, r.ConfigFile,
		r.Database, initAccountLine(r)))
	return err
}

// initAccountLine décrit l'accès aux dépôts dans le résumé de init : compte gh, ou clones Git locaux.
func initAccountLine(r initReport) string {
	if r.Source == wsync.SourceGit {
		return i18n.T("cli.repo.init.account_git")
	}
	return i18n.T("cli.repo.init.account_gh", r.GitHub.Login, r.GitHub.Host,
		scopesText(r.GitHub.Scopes))
}

// scopesText décrit les droits du token gh pour le résumé de init.
func scopesText(scopes []string) string {
	if len(scopes) == 0 {
		return i18n.T("cli.repo.init.scopes_unknown")
	}
	return i18n.T("cli.repo.init.scopes", strings.Join(scopes, ", "))
}
