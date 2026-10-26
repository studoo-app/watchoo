// Package cli définit les commandes cobra de Watchoo. Il ne contient aucune logique métier : il
// analyse les arguments, appelle les packages du cœur (workspace, store, auth…) et formate la sortie.
// Les messages affichés à l'utilisateur sont en français.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/auth"
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
}

// DefaultDeps renvoie les dépendances de production : os.Stdout, os.Stderr, auth.GH construit depuis
// la configuration, os.Getwd, DetectTerminal(os.Stdout).
func DefaultDeps() Deps {
	return Deps{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		NewAuthClient: func(cfg workspace.Config) auth.Client {
			return auth.GH{Path: cfg.GitHub.GHPath, User: cfg.GitHub.User}
		},
		Getwd:    os.Getwd,
		Terminal: func() TermInfo { return DetectTerminal(os.Stdout) },
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
	return d
}

// options regroupe les options globales (persistantes), communes à toutes les sous-commandes, et
// l'option --version de la commande racine.
type options struct {
	workspaceDir string // -C / --workspace : répertoire de l'espace à utiliser
	json         bool   // --json : sortie JSON au lieu du texte
	version      bool   // --version (watchoo seul) : même effet que « watchoo version »
}

// NewRootCmd construit la commande racine « watchoo » et ses sous-commandes (init, version).
//
// Options globales (persistantes) : --json (sortie JSON) et -C / --workspace <répertoire> (espace à
// utiliser au lieu de celui trouvé depuis le répertoire courant). La commande racine a aussi l'option
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
		fmt.Fprintf(deps.Stderr, "erreur : %v\n", err)
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
