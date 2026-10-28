package cli

// « watchoo tui [--interval 30m] » : refus hors terminal (tui.ErrNotTerminal), intervalle validé par
// wsync.AutoInterval, logs redirigés hors du terminal, puis deps.RunTUI (tui.Run).

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"

	"github.com/spf13/cobra"

	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/tui"
)

// newTUICmd construit « watchoo tui ».
func newTUICmd(deps Deps, opts *options) *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "tui",
		Short: "Ouvre le mode interactif dans le terminal",
		Long: `Ouvre le mode interactif : dépôts (état, dernier commit, auteur), commits et
contributeurs se parcourent au clavier, avec filtre et détail d'un dépôt.

Touches principales : tab change de vue, ↑ ↓ déplacent la sélection, entrée ouvre le
détail, / filtre, s synchronise tout, S le dépôt sélectionné, a ajoute un dépôt à suivre
(vue Dépôts, comme « watchoo repo add » sans option), ? affiche l'aide, q quitte.

Tant qu'il est ouvert, le mode interactif synchronise automatiquement les dépôts toutes
les sync.interval (30 min par défaut) si sync.auto est vrai. --interval remplace ce
réglage le temps de la session : 0 pour la synchro manuelle seule, sinon 15 min au
minimum. Il doit être lancé dans un terminal.`,
		Example: `  watchoo tui
  watchoo tui --interval 1h
  watchoo tui --interval 0`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd.Context(), deps, opts, interval, cmd.Flags().Changed("interval"))
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 0,
		"`intervalle` de synchro automatique (ex. 30m, 1h ; 0 = manuelle seule) ; défaut : sync.interval")
	return cmd
}

// runTUI vérifie le terminal et l'intervalle, ouvre l'espace et lance le mode interactif.
func runTUI(ctx context.Context, deps Deps, opts *options, interval time.Duration, overridden bool) (err error) {
	if opts.json {
		return errors.New("« watchoo tui » n'a pas de sortie JSON : utilisez les autres commandes avec --json")
	}
	term := deps.Terminal()
	if !term.IsTerminal || !deps.StdinIsTerminal() {
		return tui.ErrNotTerminal
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	auto, err := wsync.AutoInterval(s.ws.Config.Sync, interval, overridden)
	if err != nil {
		return err
	}
	// Les logs écrits sur la sortie d'erreur casseraient l'affichage du TUI.
	engine, err := s.engineWithLogger(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		return err
	}
	return deps.RunTUI(ctx, tui.Deps{
		Data:     s.store,
		Syncer:   engine,
		Adder:    engine,
		Config:   s.ws.Config,
		Location: s.loc,
		Interval: auto,
		Now:      deps.Now,
		Version:  appVersion(),
		Unicode:  term.Unicode,
		Color:    term.Color,
		// Avis de nouvelle version dans l'en-tête (pas d'avis CLI après le TUI : noNoticeCommands).
		UpdateCheck: tuiUpdateCheck(deps),
	}, tui.IO{In: deps.Stdin, Out: deps.Stdout})
}
