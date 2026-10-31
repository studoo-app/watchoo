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

	"github.com/studoo-app/watchoo/internal/i18n"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/tui"
)

// newTUICmd construit « watchoo tui ».
func newTUICmd(deps Deps, opts *options) *cobra.Command {
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "tui",
		Short: i18n.T("cli.sync.tui_short"),
		Long:  i18n.T("cli.sync.tui_long"),
		Example: `  watchoo tui
  watchoo tui --interval 1h
  watchoo tui --interval 0`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(cmd.Context(), deps, opts, interval, cmd.Flags().Changed("interval"))
		},
	}
	cmd.Flags().DurationVar(&interval, "interval", 0, i18n.T("cli.sync.flag_interval"))
	return cmd
}

// runTUI vérifie le terminal et l'intervalle, ouvre l'espace et lance le mode interactif.
func runTUI(ctx context.Context, deps Deps, opts *options, interval time.Duration, overridden bool) (err error) {
	if opts.json {
		return errors.New(i18n.T("cli.sync.tui_no_json"))
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
