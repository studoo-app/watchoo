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

	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
	"github.com/studoo-app/watchoo/internal/web"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Textes de la carte de « watchoo serve ».
const (
	serveSubtitle = "Interface web de l'espace « %s »"
	serveTip      = "Astuce : Ctrl+C arrête le serveur (la synchro en cours est annulée proprement)."
)

// newServeCmd construit « watchoo serve ».
func newServeCmd(deps Deps, opts *options) *cobra.Command {
	var addr string
	var interval time.Duration
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Lance l'interface web locale et la synchro périodique",
		Long: `Lance l'interface web de l'espace (tableau de bord, détail des dépôts, contributeurs,
réglages) et synchronise les dépôts en arrière-plan tant qu'elle tourne.

L'interface écoute sur web.addr (127.0.0.1:8080 par défaut) ; --addr remplace ce
réglage. Ailleurs que sur la boucle locale (127.0.0.1, ::1, localhost), elle refuse de
démarrer sans authentification : réglez web.auth.username et donnez le mot de passe dans
la variable WATCHOO_WEB_PASSWORD (jamais dans un fichier), ou déclarez un reverse proxy
authentifiant avec web.trusted_proxy: true et ses noms publics dans web.allowed_hosts.

La synchro automatique suit sync.interval (30 min par défaut) si sync.auto est vrai ;
--interval remplace ce réglage le temps du processus : 0 pour la synchro manuelle seule,
sinon 15 min au minimum. Au démarrage, une synchro n'est lancée que si la dernière date
de plus d'un intervalle. Ctrl+C arrête le serveur proprement.`,
		Example: `  watchoo serve
  watchoo serve --addr 127.0.0.1:9000 --interval 1h
  WATCHOO_WEB_PASSWORD=… watchoo serve --addr 0.0.0.0:8080   # avec web.auth.username`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(cmd.Context(), deps, opts, addr, cmd.Flags().Changed("addr"),
				interval, cmd.Flags().Changed("interval"))
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "", "`adresse` d'écoute hôte:port (ex. 127.0.0.1:9000) ; défaut : web.addr")
	cmd.Flags().DurationVar(&interval, "interval", 0,
		"`intervalle` de synchro automatique (ex. 30m, 1h ; 0 = manuelle seule) ; défaut : sync.interval")
	return cmd
}

// runServe vérifie les réglages, ouvre l'espace, écoute, affiche l'URL et sert jusqu'à l'annulation
// de ctx.
func runServe(ctx context.Context, deps Deps, opts *options, addr string, addrSet bool,
	interval time.Duration, intervalSet bool) (err error) {
	if opts.json {
		return errors.New("« watchoo serve » n'a pas de sortie JSON : utilisez les autres commandes avec --json")
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
			return fmt.Errorf("--addr : %w", err)
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
			return fmt.Errorf("l'adresse %s est déjà utilisée (un autre « watchoo serve » ?) : "+
				"choisissez un autre port avec web.addr ou --addr", cfg.Web.Addr)
		}
		return fmt.Errorf("écoute sur %s impossible : %w", cfg.Web.Addr, err)
	}
	handler, err := web.New(web.Options{
		Store:        s.store,
		Sync:         coord,
		Repos:        engine,
		Config:       cfg,
		Location:     s.loc,
		Now:          deps.Now,
		Logger:       s.logger,
		Version:      appVersion(),
		ListenAddr:   ln.Addr().String(),
		Auth:         auth,
		TrustedProxy: cfg.Web.TrustedProxy,
		AllowedHosts: cfg.Web.AllowedHosts,
	})
	if err != nil {
		_ = ln.Close()
		return err
	}

	if warning != "" {
		fmt.Fprintf(deps.Stderr, "attention : %s\n", warning)
	}
	if err := writeServeBanner(deps, opts, cfg.Name, serveURL(ln.Addr()), auto); err != nil {
		_ = ln.Close()
		return err
	}
	if err := web.Serve(ctx, web.ServeOptions{Listener: ln, Handler: handler, Syncs: coord, Logger: s.logger}); err != nil {
		return err
	}
	fmt.Fprintln(deps.Stdout, "Interface web arrêtée.")
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
		"Interface web : " + url,
		"Synchro automatique : " + intervalText(interval),
	}
	var out string
	if term, ok := cardTerminal(deps, opts); ok {
		if card := renderCard(term, fmt.Sprintf(serveSubtitle, textclean.Clean(name)), serveTip); card != "" {
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
		out += "Ctrl+C pour arrêter.\n"
	}
	if _, err := io.WriteString(deps.Stdout, out); err != nil {
		return fmt.Errorf("affichage de l'adresse de l'interface : %w", err)
	}
	return nil
}

// intervalText décrit l'intervalle de synchro automatique.
func intervalText(d time.Duration) string {
	if d <= 0 {
		return "désactivée (synchro manuelle depuis l'interface)"
	}
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("toutes les %d h", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("toutes les %d min", d/time.Minute)
	default:
		return "toutes les " + d.String()
	}
}
