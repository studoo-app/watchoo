package selfupdate

// Vérification périodique de « watchoo serve » : au démarrage puis toutes les 30 min.

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// DefaultMonitorInterval est l'intervalle entre deux vérifications de « watchoo serve ».
const DefaultMonitorInterval = 30 * time.Minute

// defaultCheckTimeout borne chaque vérification du Monitor.
const defaultCheckTimeout = 30 * time.Second

// Checker lit la dernière version et la compare à current ; *Client le satisfait.
type Checker interface {
	Check(ctx context.Context, current string) (Status, error)
}

var _ Checker = (*Client)(nil)

// MonitorOptions règle NewMonitor.
type MonitorOptions struct {
	Checker  Checker       // lecture de la dernière version (obligatoire)
	Current  string        // version installée
	Interval time.Duration // entre deux vérifications ; 0 = DefaultMonitorInterval
	Timeout  time.Duration // durée maximale d'une vérification ; 0 = 30 s
	Logger   *slog.Logger  // nil = slog.Default()
	// OnUpdate est appelée (dans la goroutine de Run) la première fois qu'une version plus récente est
	// connue, puis à chaque version plus récente encore ; nil = rien.
	OnUpdate func(Status)
}

// Monitor vérifie périodiquement la dernière version et garde en mémoire la plus récente connue, pour
// l'interface web (Available). Sûr pour un usage concurrent.
type Monitor struct {
	opts MonitorOptions

	mu        sync.Mutex
	status    Status
	available bool
}

// NewMonitor renvoie un Monitor ; rien n'est vérifié avant Run ou CheckNow.
func NewMonitor(opts MonitorOptions) *Monitor {
	if opts.Interval <= 0 {
		opts.Interval = DefaultMonitorInterval
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultCheckTimeout
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Monitor{opts: opts}
}

// Run vérifie tout de suite, puis à chaque intervalle, jusqu'à l'annulation de ctx.
func (m *Monitor) Run(ctx context.Context) {
	m.CheckNow(ctx)
	ticker := time.NewTicker(m.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.CheckNow(ctx)
		}
	}
}

// CheckNow fait une vérification. Un échec est journalisé (debug) et laisse l'état inchangé.
func (m *Monitor) CheckNow(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, m.opts.Timeout)
	defer cancel()
	st, err := m.opts.Checker.Check(ctx, m.opts.Current)
	if err != nil {
		m.opts.Logger.Debug("vérification de la dernière version de Watchoo impossible", "erreur", err)
		return
	}
	m.opts.Logger.Debug("dernière version de Watchoo", "installée", st.Current, "publiée", st.Latest)
	m.mu.Lock()
	isNew := st.UpdateAvailable && (!m.available || st.Latest != m.status.Latest)
	if st.UpdateAvailable {
		m.status, m.available = st, true
	}
	m.mu.Unlock()
	if isNew && m.opts.OnUpdate != nil {
		m.opts.OnUpdate(st)
	}
}

// Available renvoie la nouvelle version connue, et true s'il y en a une.
func (m *Monitor) Available() (Status, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, m.available
}
