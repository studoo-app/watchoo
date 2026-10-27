// Package cli définit les commandes cobra de Watchoo. Il ne contient aucune logique métier : il
// analyse les arguments, appelle les packages du cœur (workspace, store, auth…) et formate la sortie.
// Les messages affichés à l'utilisateur sont en français.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/auth"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
	"github.com/studoo-app/watchoo/internal/tui"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Deps regroupe les dépendances injectées dans les commandes, pour les tests.
type Deps struct {
	Stdout io.Writer // sortie normale des commandes
	Stderr io.Writer // erreurs, avertissements et logs
	// NewAuthClient construit l'accès à gh à partir des réglages (github.gh_path, github.user).
	// Les tests y renvoient un authtest.Fake.
	NewAuthClient func(cfg workspace.Config) auth.Client
	// Getwd renvoie le répertoire courant (os.Getwd en production). La CLI l'utilise pour init et le
	// passe à workspace.ResolveFrom pour les autres commandes.
	Getwd func() (string, error)
	// Terminal décrit la sortie standard (terminal ou non, largeur, Unicode, couleur), pour décider d'y
	// afficher la carte d'en-tête (voir cardTerminal). Elle n'est appelée qu'en sortie texte, au moment
	// d'afficher la carte. Par défaut, DetectTerminal appliquée à Stdout : un Stdout qui n'est pas un
	// fichier (tampon des tests) n'est jamais un terminal.
	Terminal func() TermInfo

	// Dépendances de la phase 3.

	// NewProvider construit le Provider des synchros et vérifications d'accès à partir du token ;
	// nil = provider GitHub par défaut (voir wsync.Options.NewProvider). Les tests y renvoient un
	// providertest.Fake.
	NewProvider wsync.ProviderFactory
	// NewGitProvider construit le Provider des clones Git locaux, utilisé sans gh (V2) ; nil =
	// provider Git par défaut, clones dans repos/ à la racine de l'espace (voir wsync.Options.NewGitProvider). Les
	// tests y branchent un provider Git sur gittest.
	NewGitProvider wsync.GitProviderFactory
	// CheckGit vérifie que git est installé (« watchoo init » sans gh) ; nil = git.Check sur le git
	// du PATH. Erreur enveloppant git.ErrGitNotFound s'il manque.
	CheckGit func(ctx context.Context) error
	// Now est l'horloge (dates relatives, --since, synchros) ; nil = time.Now.
	Now func() time.Time
	// Stdin est l'entrée standard (confirmation de « repo remove », clavier du TUI) ; nil = os.Stdin.
	Stdin io.Reader
	// StdinIsTerminal indique si Stdin est un terminal ; nil = détection sur Stdin (isatty). Les
	// tests y répondent sans terminal réel.
	StdinIsTerminal func() bool
	// RunTUI lance le mode interactif ; nil = tui.Run. Les tests de « watchoo tui » y vérifient les
	// dépendances reçues sans ouvrir de terminal.
	RunTUI func(ctx context.Context, deps tui.Deps, term tui.IO) error
	// Listen ouvre l'écoute de « watchoo serve » ; nil = net.Listen. Les tests y écoutent sur un port
	// libre de 127.0.0.1 et récupèrent l'adresse.
	Listen func(network, address string) (net.Listener, error)
	// Logger reçoit les logs du moteur de synchro et du provider ; nil = logger texte sur Stderr, au
	// niveau de WATCHOO_LOG_LEVEL (warn par défaut) ou debug avec --verbose (voir newLogger). Les tests
	// y passent un logger qui capture tout, pour vérifier qu'aucun token n'y apparaît.
	Logger *slog.Logger
}

// DefaultDeps renvoie les dépendances de production : os.Stdout, os.Stderr, auth.GH construit depuis
// la configuration, os.Getwd, DetectTerminal(os.Stdout), time.Now, os.Stdin, tui.Run, net.Listen et
// git.Check. NewGitProvider reste nil (clones dans repos/ à la racine de l'espace).
// NewProvider et Logger restent nil (provider GitHub et logger de newLogger), StdinIsTerminal aussi
// (détection sur Stdin par withDefaults).
func DefaultDeps() Deps {
	return Deps{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		NewAuthClient: func(cfg workspace.Config) auth.Client {
			return auth.GH{Path: cfg.GitHub.GHPath, User: cfg.GitHub.User}
		},
		Getwd:    os.Getwd,
		Terminal: func() TermInfo { return DetectTerminal(os.Stdout) },
		Now:      time.Now,
		Stdin:    os.Stdin,
		RunTUI:   tui.Run,
		Listen:   net.Listen,
		CheckGit: func(ctx context.Context) error { return gitprov.Check(ctx, "") },
	}
}

// withDefaults complète les dépendances non renseignées avec celles de production. Sans Terminal, la
// sortie décrite est le Stdout retenu, pas forcément os.Stdout.
func (d Deps) withDefaults() Deps {
	def := DefaultDeps()
	if d.Stdout == nil {
		d.Stdout = def.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = def.Stderr
	}
	if d.NewAuthClient == nil {
		d.NewAuthClient = def.NewAuthClient
	}
	if d.Getwd == nil {
		d.Getwd = def.Getwd
	}
	if d.Terminal == nil {
		stdout := d.Stdout
		d.Terminal = func() TermInfo { return DetectTerminal(stdout) }
	}
	if d.Now == nil {
		d.Now = def.Now
	}
	if d.Stdin == nil {
		d.Stdin = def.Stdin
	}
	if d.StdinIsTerminal == nil {
		stdin := d.Stdin
		d.StdinIsTerminal = func() bool { return isTerminal(stdin) }
	}
	if d.RunTUI == nil {
		d.RunTUI = def.RunTUI
	}
	if d.Listen == nil {
		d.Listen = def.Listen
	}
	if d.CheckGit == nil {
		d.CheckGit = def.CheckGit
	}
	return d
}

// options regroupe les options globales (persistantes), communes à toutes les sous-commandes, et
// l'option --version de la commande racine.
type options struct {
	workspaceDir string // -C / --workspace : répertoire de l'espace à utiliser
	json         bool   // --json : sortie JSON au lieu du texte
	version      bool   // --version (watchoo seul) : même effet que « watchoo version »
	verbose      bool   // --verbose : logs détaillés (niveau debug) sur la sortie d'erreur
}

// NewRootCmd construit la commande racine « watchoo » et ses sous-commandes (init, version, repo,
// sync, log, contributors, status, tui, serve, config).
//
// Options globales (persistantes) : --json (sortie JSON), -C / --workspace <répertoire> (espace à
// utiliser au lieu de celui trouvé depuis le répertoire courant) et --verbose (logs détaillés sur
// la sortie d'erreur). La commande racine a aussi l'option
// --version, qui fait comme « watchoo version ». Sur un terminal, l'aide générale (« watchoo »,
// « watchoo help », « watchoo --help ») commence par la carte d'en-tête et la ligne d'état de l'espace
// (voir writeHelpHeader) ; l'aide des sous-commandes, non. Les erreurs ne sont pas affichées par cobra
// (SilenceErrors) ni accompagnées de l'usage (SilenceUsage) : c'est Run qui les affiche, une seule
// fois, sur deps.Stderr. Les dépendances non renseignées prennent leur valeur de production.
func NewRootCmd(deps Deps) *cobra.Command {
	deps = deps.withDefaults()
	opts := &options{}
	root := &cobra.Command{
		Use:   "watchoo",
		Short: "Supervise des dépôts GitHub : ce qui a bougé, quand, et qui l'a fait",
		Long: `Watchoo supervise des dépôts GitHub, surtout privés, et répond à trois questions :
qu'est-ce qui a bougé, quand, et qui l'a fait.

Chaque groupe de dépôts suivis vit dans son propre espace de supervision, créé par
« watchoo init ». Les autres commandes trouvent l'espace dans le répertoire courant
ou ses parents, comme Git, ou dans celui donné par -C ou la variable WATCHOO_DIR.

Prérequis : la CLI GitHub gh, installée et connectée (« gh auth login »).`,
		// Sans sous-commande : l'aide, ou la version avec --version. Avec un mot inconnu : une erreur
		// en français (le message de cobra serait en anglais).
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if opts.version {
				if len(args) > 0 {
					return fmt.Errorf("« %s --version » ne prend pas d'argument (reçu : %s)",
						cmd.CommandPath(), strings.Join(args, " "))
				}
				return runVersion(deps, opts)
			}
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return cmd.Help()
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.PersistentFlags().BoolVar(&opts.json, "json", false,
		"affiche le résultat au format JSON, pour les scripts")
	root.PersistentFlags().StringVarP(&opts.workspaceDir, "workspace", "C", "",
		"travaille sur l'espace situé à ce `chemin` au lieu de celui trouvé depuis le répertoire courant")
	// Sans lettre courte : « -v » serait confondu avec --version.
	root.PersistentFlags().BoolVar(&opts.verbose, "verbose", false,
		"affiche les logs détaillés (niveau debug) sur la sortie d'erreur")
	// Option locale, sans lettre courte : « watchoo init --version » est une option inconnue.
	root.Flags().BoolVar(&opts.version, "version", false, "affiche la version de Watchoo (comme « watchoo version »)")
	localize(root)
	// La carte en haut de l'aide générale seulement : la fonction d'aide de la racine est aussi celle
	// des sous-commandes, qui en héritent.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c == root {
			// cobra n'attend pas d'erreur de l'aide ; une sortie standard en échec fera aussi
			// échouer l'aide qui suit.
			_ = writeHelpHeader(deps, opts)
		}
		defaultHelp(c, args)
	})

	root.AddCommand(
		newInitCmd(deps, opts),
		newVersionCmd(deps, opts),
		newRepoCmd(deps, opts),
		newSyncCmd(deps, opts),
		newLogCmd(deps, opts),
		newContributorsCmd(deps, opts),
		newPRsCmd(deps, opts),
		newReleasesCmd(deps, opts),
		newStatusCmd(deps, opts),
		newTUICmd(deps, opts),
		newServeCmd(deps, opts),
		newConfigCmd(deps, opts),
	)
	return root
}

// Run exécute la CLI avec les arguments args (sans le nom du programme) et renvoie le code de sortie
// du processus : 0 en cas de succès, 1 en cas d'erreur (message « erreur : … » sur deps.Stderr).
func Run(ctx context.Context, deps Deps, args []string) int {
	deps = deps.withDefaults()
	cmd := NewRootCmd(deps)
	if args == nil {
		args = []string{} // sinon cobra lirait os.Args
	}
	cmd.SetArgs(args)
	if err := cmd.ExecuteContext(ctx); err != nil {
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			fmt.Fprintf(deps.Stderr, "erreur : commande interrompue (%v)\n", err)
			return 1
		}
		// Le message peut citer un texte de la forge ou de la base : nettoyé avant d'aller au terminal.
		fmt.Fprintf(deps.Stderr, "erreur : %s\n", textclean.CleanLines(err.Error()))
		return 1
	}
	return 0
}

// Execute lance la CLI avec les dépendances de production et les arguments du processus, et renvoie
// le code de sortie : 0 en cas de succès, 1 en cas d'erreur (message affiché sur la sortie d'erreur).
// Un Ctrl+C (ou SIGTERM) annule le contexte des commandes, qui peuvent ainsi nettoyer avant de rendre
// la main (init supprime alors l'espace à moitié créé).
func Execute() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return Run(ctx, DefaultDeps(), os.Args[1:])
}

// noArgs refuse tout argument positionnel, avec un message en français.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("« %s » ne prend pas d'argument (reçu : %s)", cmd.CommandPath(), strings.Join(args, " "))
	}
	return nil
}

// unknownCommandError signale une sous-commande inconnue, avec une suggestion si une commande proche
// existe.
func unknownCommandError(cmd *cobra.Command, name string) error {
	msg := fmt.Sprintf("commande inconnue « %s »", name)
	if suggestions := cmd.SuggestionsFor(name); len(suggestions) > 0 {
		msg += fmt.Sprintf(" ; vouliez-vous dire « %s » ?", strings.Join(suggestions, " », « "))
	}
	return fmt.Errorf("%s (liste des commandes : « %s --help »)", msg, cmd.CommandPath())
}
