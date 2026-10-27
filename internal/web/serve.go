package web

// Cycle de vie du serveur HTTP de « watchoo serve » : délais, démarrage du coordinateur de synchro,
// arrêt propre sur annulation du contexte (SIGINT / SIGTERM).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Délais du serveur HTTP.
const (
	readHeaderTimeout      = 5 * time.Second
	readTimeout            = 30 * time.Second
	writeTimeout           = 60 * time.Second
	idleTimeout            = 120 * time.Second
	maxHeaderBytes         = 64 << 10
	DefaultShutdownTimeout = 10 * time.Second
)

// Lifecycle démarre et arrête les synchros du processus ; *wsync.Coordinator le satisfait.
type Lifecycle interface {
	Start(ctx context.Context)
	Close(timeout time.Duration) error
}

// ServeOptions règle Serve.
type ServeOptions struct {
	Listener net.Listener // déjà ouvert (l'appelant a vérifié l'adresse et affiché l'URL)
	Handler  http.Handler // New
	Syncs    Lifecycle    // coordinateur ; nil = aucun
	Logger   *slog.Logger // erreurs du serveur ; nil = slog.Default()
	// ShutdownTimeout borne l'arrêt : fin des requêtes en cours, puis de la synchro en cours ;
	// 0 = DefaultShutdownTimeout pour chacune.
	ShutdownTimeout time.Duration
}

// Serve sert l'interface sur o.Listener et démarre les synchros (o.Syncs.Start) jusqu'à
// l'annulation de ctx, puis arrête proprement : plus de nouvelle connexion, requêtes en cours
// terminées (http.Server.Shutdown, au plus ShutdownTimeout), synchro en cours annulée et attendue
// (Syncs.Close, au plus ShutdownTimeout). L'appelant ferme la base APRÈS le retour de Serve. Renvoie
// nil après un arrêt demandé, ou l'erreur du serveur (ex. écoute impossible) et celles de l'arrêt.
func Serve(ctx context.Context, o ServeOptions) error {
	if o.Listener == nil || o.Handler == nil {
		return errors.New("interface web : écoute ou handler non renseigné")
	}
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	timeout := o.ShutdownTimeout
	if timeout <= 0 {
		timeout = DefaultShutdownTimeout
	}
	srv := &http.Server{
		Handler:           o.Handler,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
	}
	if o.Syncs != nil {
		o.Syncs.Start(ctx)
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Serve(o.Listener) }()

	var errs []error
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			errs = append(errs, fmt.Errorf("serveur web : %w", err))
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		errs = append(errs, fmt.Errorf("arrêt du serveur web : %w", err))
		_ = srv.Close()
	}
	if o.Syncs != nil {
		if err := o.Syncs.Close(timeout); err != nil {
			errs = append(errs, fmt.Errorf("arrêt des synchros : %w", err))
		}
	}
	return errors.Join(errs...)
}
