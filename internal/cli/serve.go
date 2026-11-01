package cli

// « watchoo serve [--addr 127.0.0.1:8080] [--interval 30m] » : interface web locale et synchro
// périodique en arrière-plan (internal/web, wsync.Coordinator). Refus de démarrer hors boucle locale
// sans authentification (web.CheckExposure) ; arrêt propre sur Ctrl+C / SIGTERM (contexte annulé par
// Execute) : serveur arrêté, synchro en cours annulée et attendue, puis base fermée.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/i18n"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
	"github.com/studoo-app/watchoo/internal/web"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// newServeCmd construit « watchoo serve ».
func newServeCmd(deps Deps, opts *options) *cobra.Command {
	var addr string
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: i18n.T("cli.sync.serve_short"),
		Long:  i18n.T("cli.sync.serve_long"),
		Example: `  watchoo serve
  watchoo serve --addr 127.0.0.1:9000 --interval 1h
  WATCHOO_WEB_PASSWORD=… watchoo serve --addr 0.0.0.0:8080   # avec web.auth.username`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context(), deps, opts, addr, cmd.Flags().Changed("addr"),
				interval, cmd.Flags().Changed("interval"))
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", i18n.T("cli.sync.flag_addr"))
	cmd.Flags().DurationVar(&interval, "interval", 0, i18n.T("cli.sync.flag_interval"))
	return cmd
}

// runServe vérifie les réglages, ouvre l'espace, écoute, affiche l'URL et sert jusqu'à l'annulation
// de ctx.
func runServe(ctx context.Context, deps Deps, opts *options, addr string, addrSet bool,
	interval time.Duration, intervalSet bool) (err error) {
	if opts.json {
		return errors.New(i18n.T("cli.sync.serve_no_json"))
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)

	cfg := s.ws.Config
	if addrSet {
		cfg.Web.Addr = addr
		if err := cfg.Validate(); err != nil {
			return fmt.Errorf("--addr%s%w", i18n.T("cli.core.err.sep"), err)
		}
	}
	auto, err := wsync.AutoInterval(cfg.Sync, interval, intervalSet)
	if err != nil {
		return err
	}
	auth := web.Auth{Username: cfg.Web.Auth.Username, Password: os.Getenv(workspace.WebPasswordEnv)}
	warning, err := web.CheckExposure(cfg.Web.Addr, auth, cfg.Web.TrustedProxy, cfg.Web.AllowedHosts)
	if err != nil {
		return err
	}

	engine, err := s.engine()
	if err != nil {
		return err
	}
	coord, err := wsync.NewCoordinator(wsync.CoordinatorOptions{
		Syncer:   engine,
		Interval: auto,
		LastSync: s.store.LastSyncFinishedAt,
		Now:      deps.Now,
		Logger:   s.logger,
	})
	if err != nil {
		return err
	}

	ln, err := deps.Listen("tcp", cfg.Web.Addr)
	if err != nil {
		if errors.Is(err, syscall.EADDRINUSE) {
			return errors.New(i18n.T("cli.sync.serve_addr_in_use", cfg.Web.Addr))
		}
		return fmt.Errorf("%s%s%w", i18n.T("cli.sync.serve_listen_failed", cfg.Web.Addr),
			i18n.T("cli.core.err.sep"), err)
	}
	monitor := newUpdateMonitor(deps, s.logger)
	var updates web.UpdateSource // nil (et non un *Monitor nil) sans vérification
	if monitor != nil {
		updates = monitor
	}
	handler, err := web.New(web.Options{
		Store:        s.store,
		Sync:         coord,
		Repos:        engine,
		Config:       cfg,
		Workspace:    s.ws,
		Location:     s.loc,
		Now:          deps.Now,
		Logger:       s.logger,
		Version:      appVersion(),
		ListenAddr:   ln.Addr().String(),
		Auth:         auth,
		TrustedProxy: cfg.Web.TrustedProxy,
		AllowedHosts: cfg.Web.AllowedHosts,
		Updates:      updates,
		UpdateDocker: deps.InDocker(),
	})
	if err != nil {
		_ = ln.Close()
		return err
	}

	if warning != "" {
		warn(deps.Stderr, warning)
	}
	if err := writeServeBanner(deps, opts, cfg.Name, serveURL(ln.Addr()), auto); err != nil {
		_ = ln.Close()
		return err
	}
	if monitor != nil {
		monitorCtx, stopMonitor := context.WithCancel(ctx)
		monitorDone := make(chan struct{})
		go func() {
			defer close(monitorDone)
			monitor.Run(monitorCtx)
		}()
		defer func() {
			stopMonitor()
			<-monitorDone
		}()
	}
	if err := web.Serve(ctx, web.ServeOptions{Listener: ln, Handler: handler, Syncs: coord, Logger: s.logger}); err != nil {
		return err
	}
	fmt.Fprintln(deps.Stdout, i18n.T("cli.sync.serve_stopped"))
	return nil
}

// serveURL renvoie l'URL à ouvrir pour l'adresse écoutée : une écoute sur toutes les interfaces
// (0.0.0.0, ::) est montrée avec 127.0.0.1, joignable depuis la machine elle-même.
func serveURL(a net.Addr) string {
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return "http://" + a.String() + "/"
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsUnspecified() {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}

// writeServeBanner annonce le démarrage : sur un terminal, la carte d'en-tête puis les lignes
// d'état ; sinon (sortie redirigée) des lignes simples.
func writeServeBanner(deps Deps, opts *options, name, url string, interval time.Duration) error {
	lines := []string{
		i18n.T("cli.sync.serve_web_line", url),
		i18n.T("cli.sync.serve_sync_line", intervalText(interval)),
	}
	var out string
	if term, ok := cardTerminal(deps, opts); ok {
		if card := renderCard(term, i18n.T("cli.sync.serve_subtitle", textclean.Clean(name)),
			i18n.T("cli.sync.serve_tip")); card != "" {
			out = card + "\n"
			for _, l := range lines {
				out += statusLine(term, l)
			}
		}
	}
	if out == "" {
		for _, l := range lines {
			out += l + "\n"
		}
		out += i18n.T("cli.sync.serve_ctrl_c")
	}
	if _, err := io.WriteString(deps.Stdout, out); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.sync.serve_banner_err"),
			i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// intervalText décrit l'intervalle de synchro automatique.
func intervalText(d time.Duration) string {
	if d <= 0 {
		return i18n.T("cli.sync.interval_off")
	}
	switch {
	case d%time.Hour == 0:
		return i18n.T("cli.sync.interval_hours", d/time.Hour)
	case d%time.Minute == 0:
		return i18n.T("cli.sync.interval_minutes", d/time.Minute)
	default:
		return i18n.T("cli.sync.interval_every", d.String())
	}
}
