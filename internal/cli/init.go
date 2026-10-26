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
	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Messages qui disent quoi faire quand gh n'est pas utilisable.
const (
	hintInstallGH = "installez la CLI GitHub gh : https://cli.github.com/"
	hintLoginGH   = "lancez « gh auth login »"
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
	GitHub          githubReport `json:"github"`
	Warnings        []string     `json:"warnings"` // avertissements, aussi affichés sur la sortie d'erreur
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
vérifie que la CLI GitHub gh est installée et connectée, puis crée .watchoo/ avec
config.yaml, la base SQLite au bon schéma et un .gitignore. Le répertoire .watchoo/
est lisible par vous seul (droits 0700, fichiers en 0600).

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
// seulement, jamais Token), création de .watchoo/, création et migration de la base, résumé.
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
	status, err := deps.NewAuthClient(cfg).Status(ctx)
	if err != nil {
		return ghError(err, cfg)
	}

	res, err := workspace.Init(root)
	if err != nil {
		if errors.Is(err, workspace.ErrAlreadyInitialized) {
			return fmt.Errorf("%w : rien n'a été modifié", err)
		}
		return err
	}
	ws := res.Workspace
	if err := finishInit(ctx, ws); err != nil {
		// Ne pas laisser un espace à moitié créé : Init vient de créer .watchoo/, il est à nous.
		if rmErr := os.RemoveAll(ws.Dir); rmErr != nil {
			err = errors.Join(err, fmt.Errorf("nettoyage de %s : %w", ws.Dir, rmErr))
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
		Warnings: initWarnings(deps, opts, ws.Root, res.ParentWorkspace),
	}
	for _, w := range report.Warnings {
		warn(deps.Stderr, w)
	}
	if opts.json {
		return writeJSON(deps.Stdout, report)
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
		return "", fmt.Errorf("répertoire courant inaccessible : %w", err)
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
		return fmt.Errorf("création de la base de l'espace : %w", err)
	}
	if err := workspace.Load(ws); err != nil {
		return fmt.Errorf("relecture des réglages de l'espace créé : %w", err)
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
			err = fmt.Errorf("fermeture de la base %s : %w", path, cerr)
		}
	}()
	return st.Migrate(ctx)
}

// ghError traduit une erreur de vérification de gh en message qui dit quoi faire. L'erreur d'origine
// reste accessible par errors.Is. Le conseil « gh auth login » est réservé à un gh non connecté ou à
// un token refusé : ni un réglage github.user inutilisable (ErrUnusableUser, dont le message dit quoi
// corriger) ni un GitHub injoignable (ErrCheckFailed) ne se règlent en se reconnectant.
func ghError(err error, cfg workspace.Config) error {
	switch {
	case errors.Is(err, auth.ErrGHNotFound):
		return &hintError{
			msg: fmt.Sprintf("gh introuvable (github.gh_path : %q) : %s", cfg.GitHub.GHPath, hintInstallGH),
			err: err,
		}
	case errors.Is(err, auth.ErrUnusableUser):
		return fmt.Errorf("vérification de gh : %w", err)
	case errors.Is(err, auth.ErrNotLoggedIn):
		msg := "vérification de gh : " + err.Error()
		if !strings.Contains(msg, hintLoginGH) {
			msg += " : " + hintLoginGH
		}
		return &hintError{msg: msg, err: err}
	default:
		return fmt.Errorf("vérification de gh : %w", err)
	}
}

// initWarnings renvoie les avertissements à afficher après un init réussi.
func initWarnings(deps Deps, opts *options, root, parent string) []string {
	warnings := []string{}
	if parent != "" {
		warnings = append(warnings, fmt.Sprintf(
			"%s est dans l'espace %s : le nouvel espace est imbriqué et indépendant du parent "+
				"(dépôts, réglages et données séparés)", root, parent))
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
			warnings = append(warnings, fmt.Sprintf(
				"la variable %s vaut %q : tant qu'elle est définie, les autres commandes échoueront (%v)",
				workspace.EnvDir, env, err))
		case found.Root != root:
			warnings = append(warnings, fmt.Sprintf(
				"la variable %s vaut %q : tant qu'elle est définie, les autres commandes utiliseront "+
					"l'espace %s et non celui-ci", workspace.EnvDir, env, found.Root))
		}
	}
	return warnings
}

// writeInitText affiche le résumé de init en texte.
func writeInitText(w io.Writer, r initReport) error {
	scopes := "droits non indiqués par gh"
	if len(r.GitHub.Scopes) > 0 {
		scopes = "droits : " + strings.Join(r.GitHub.Scopes, ", ")
	}
	_, err := fmt.Fprintf(w, `Espace de supervision « %s » créé.
  répertoire  %s
  réglages    %s
  base        %s
  compte gh   %s sur %s (%s)
`, r.Name, r.Root, r.ConfigFile, r.Database, r.GitHub.Login, r.GitHub.Host, scopes)
	return err
}
