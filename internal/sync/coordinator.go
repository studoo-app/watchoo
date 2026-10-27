package sync

// Coordinateur des synchros d'un processus qui tourne longtemps (« watchoo serve ») : synchro
// périodique, synchros manuelles (tous les dépôts ou un seul), une seule à la fois, progression
// consultable à tout moment et arrêt propre. Il ne fait que planifier : chaque synchro est celle du
// moteur (Engine.SyncAll / SyncRepo), avec son verrou en base et son garde-fou de quota.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	gosync "sync"
	"time"

	"github.com/studoo-app/watchoo/internal/workspace"
)

// Erreurs de Coordinator.SyncAll et Coordinator.SyncRepo, à tester avec errors.Is.
var (
	// ErrSyncRunning : une synchro (manuelle ou automatique) est déjà en cours dans ce processus.
	ErrSyncRunning = errors.New("synchro déjà en cours")
	// ErrCoordinatorClosed : le processus s'arrête (Close), aucune nouvelle synchro ne démarre.
	ErrCoordinatorClosed = errors.New("arrêt en cours : aucune nouvelle synchro")
	// ErrCoordinatorNotStarted : Start n'a pas encore été appelé.
	ErrCoordinatorNotStarted = errors.New("coordinateur de synchro non démarré")
)

// Syncer lance une synchro ; *Engine le satisfait.
type Syncer interface {
	SyncAll(ctx context.Context, opts SyncOptions) (Report, error)
	SyncRepo(ctx context.Context, repoID int64, opts SyncOptions) (Report, error)
}

var _ Syncer = (*Engine)(nil)

// CoordinatorOptions règle NewCoordinator.
type CoordinatorOptions struct {
	Syncer Syncer // moteur (obligatoire)
	// Interval est l'intervalle de la synchro automatique, déjà validé par AutoInterval ; 0 = synchro
	// manuelle seule.
	Interval time.Duration
	// LastSync renvoie la fin de la dernière synchro connue (store.LastSyncFinishedAt) : au démarrage,
	// la première synchro automatique n'est lancée tout de suite que si elle date de plus d'un
	// intervalle (ou s'il n'y en a jamais eu) ; sinon elle a lieu un intervalle après la dernière.
	// nil = aucune synchro connue. Une erreur est journalisée et traitée comme « aucune ».
	LastSync func(ctx context.Context) (time.Time, error)
	Now      func() time.Time // horloge ; nil = time.Now
	// After remplace time.After pour la planification (tests) ; nil = time.After.
	After  func(d time.Duration) <-chan time.Time
	Logger *slog.Logger // nil = slog.Default() ; jamais de nom, d'e-mail ni de message de commit
}

// SyncState est l'état des synchros du coordinateur, à un instant donné (copie : le modifier n'a
// aucun effet). Il ne contient ni token ni e-mail ; Current et LastReport contiennent des noms
// affichés de dépôts (label ou owner/name).
type SyncState struct {
	Running   bool      // une synchro est en cours
	Auto      bool      // la synchro en cours (ou la dernière) est automatique
	RepoID    int64     // dépôt de la synchro en cours (ou de la dernière) ; 0 = tous les dépôts
	StartedAt time.Time // début de la synchro en cours (ou de la dernière), UTC ; zéro si aucune
	Done      int       // dépôts terminés dans la synchro en cours
	Total     int       // dépôts à traiter dans la synchro en cours (0 tant qu'il est inconnu)
	Current   string    // nom affiché du dernier dépôt commencé ; vide si aucun
	// Generation augmente à chaque synchro lancée : deux états de même Generation décrivent la même
	// synchro.
	Generation uint64

	LastFinishedAt time.Time // fin de la dernière synchro terminée dans ce processus, UTC ; zéro si aucune
	HasReport      bool      // LastReport est renseigné
	LastReport     Report    // rapport de la dernière synchro terminée dans ce processus
	LastError      string    // erreur globale de la dernière synchro (token, base…), en français ; vide sinon

	Interval time.Duration // intervalle de la synchro automatique en vigueur ; 0 = manuelle seule
	NextAuto time.Time     // prochaine synchro automatique prévue, UTC ; zéro si manuelle seule ou non démarré
}

// Coordinator planifie les synchros d'un processus : synchro automatique toutes les Interval,
// synchros manuelles à la demande, jamais deux en même temps (une synchro automatique qui tombe
// pendant une synchro est sautée ; une synchro manuelle ne décale pas la prochaine synchro
// automatique). Construire avec NewCoordinator, démarrer avec Start, arrêter avec Close. Sûr pour un
// usage concurrent.
type Coordinator struct {
	syncer   Syncer
	lastSync func(ctx context.Context) (time.Time, error)
	now      func() time.Time
	after    func(d time.Duration) <-chan time.Time
	logger   *slog.Logger

	mu         gosync.Mutex
	state      SyncState
	ctx        context.Context // contexte des synchros (Start) ; nil avant Start
	cancel     context.CancelFunc
	closed     bool
	reschedule chan struct{} // réveille la boucle de planification (SetInterval)
	syncs      gosync.WaitGroup
	loopDone   chan struct{}
}

// NewCoordinator vérifie les options et construit un Coordinator (rien ne démarre avant Start).
func NewCoordinator(opts CoordinatorOptions) (*Coordinator, error) {
	if opts.Syncer == nil {
		return nil, errors.New("coordinateur de synchro : moteur non renseigné")
	}
	if err := checkInterval(opts.Interval); err != nil {
		return nil, err
	}
	c := &Coordinator{
		syncer:     opts.Syncer,
		lastSync:   opts.LastSync,
		now:        opts.Now,
		after:      opts.After,
		logger:     opts.Logger,
		reschedule: make(chan struct{}, 1),
	}
	if c.now == nil {
		c.now = time.Now
	}
	if c.after == nil {
		c.after = time.After
	}
	if c.logger == nil {
		c.logger = slog.Default()
	}
	c.state.Interval = opts.Interval
	return c, nil
}

// checkInterval applique les bornes de sync.interval : 0 ou au moins workspace.MinSyncInterval.
func checkInterval(d time.Duration) error {
	minutes := int(workspace.MinSyncInterval / time.Minute)
	switch {
	case d < 0:
		return fmt.Errorf("intervalle %s négatif (0 pour la synchro manuelle seule, sinon au moins %d min)", d, minutes)
	case d > 0 && d < workspace.MinSyncInterval:
		return fmt.Errorf("intervalle %s inférieur au minimum de %d min (0 pour la synchro manuelle seule)", d, minutes)
	}
	return nil
}

// Start démarre la planification : les synchros tournent sous ctx (annulé par Close ou par
// l'appelant). La première synchro automatique a lieu tout de suite si la dernière synchro connue
// (LastSync) date de plus d'un intervalle, sinon un intervalle après elle. Un second appel est sans
// effet.
func (c *Coordinator) Start(ctx context.Context) {
	c.mu.Lock()
	if c.ctx != nil || c.closed {
		c.mu.Unlock()
		return
	}
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.loopDone = make(chan struct{})
	c.mu.Unlock()

	var last time.Time
	if c.lastSync != nil {
		t, err := c.lastSync(ctx)
		if err != nil {
			c.logger.Warn("date de la dernière synchro illisible : synchro automatique au démarrage", "erreur", err)
		} else {
			last = t
		}
	}
	c.mu.Lock()
	c.state.NextAuto = c.firstAuto(last)
	c.mu.Unlock()
	go c.loop()
}

// firstAuto renvoie la première synchro automatique (zéro si manuelle seule) : maintenant si last est
// zéro ou date de plus d'un intervalle, sinon last + intervalle. Appelé sous c.mu.
func (c *Coordinator) firstAuto(last time.Time) time.Time {
	interval := c.state.Interval
	if interval == 0 {
		return time.Time{}
	}
	now := c.now().UTC()
	if last.IsZero() || now.Sub(last) >= interval {
		return now
	}
	return last.Add(interval).UTC()
}

// loop attend la prochaine synchro automatique et la lance, jusqu'à l'annulation du contexte.
func (c *Coordinator) loop() {
	defer close(c.loopDone)
	for {
		c.mu.Lock()
		ctx, next := c.ctx, c.state.NextAuto
		c.mu.Unlock()
		var tick <-chan time.Time
		if !next.IsZero() {
			tick = c.after(max(next.Sub(c.now()), 0))
		}
		select {
		case <-ctx.Done():
			return
		case <-c.reschedule:
			continue
		case <-tick:
			c.mu.Lock()
			if !c.state.NextAuto.Equal(next) { // replanifié entre-temps (SetInterval)
				c.mu.Unlock()
				continue
			}
			if c.state.Interval > 0 {
				c.state.NextAuto = c.now().UTC().Add(c.state.Interval)
			} else {
				c.state.NextAuto = time.Time{}
			}
			c.mu.Unlock()
			if err := c.start(0, true); err != nil {
				c.logger.Info("synchro automatique sautée", "raison", err.Error())
			}
		}
	}
}

// SyncAll lance tout de suite, en arrière-plan, la synchro de tous les dépôts suivis. Erreur
// ErrSyncRunning si une synchro est en cours, ErrCoordinatorClosed pendant l'arrêt,
// ErrCoordinatorNotStarted avant Start. Ne décale pas la prochaine synchro automatique.
func (c *Coordinator) SyncAll() error {
	return c.start(0, false)
}

// SyncRepo lance tout de suite, en arrière-plan, la synchro du seul dépôt repoID (même suivi arrêté,
// comme Engine.SyncRepo). Mêmes erreurs que SyncAll ; un dépôt inconnu n'est signalé qu'à la fin, par
// SyncState.LastError.
func (c *Coordinator) SyncRepo(repoID int64) error {
	if repoID <= 0 {
		return fmt.Errorf("synchro du dépôt %d : identifiant invalide", repoID)
	}
	return c.start(repoID, false)
}

// start lance une synchro (repoID 0 = tous les dépôts) dans une goroutine suivie par c.syncs.
func (c *Coordinator) start(repoID int64, auto bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch {
	case c.closed:
		return ErrCoordinatorClosed
	case c.ctx == nil:
		return ErrCoordinatorNotStarted
	case c.state.Running:
		return ErrSyncRunning
	}
	c.state.Generation++
	gen := c.state.Generation
	c.state.Running, c.state.Auto, c.state.RepoID = true, auto, repoID
	c.state.StartedAt = c.now().UTC()
	c.state.Done, c.state.Total, c.state.Current = 0, 0, ""
	ctx := c.ctx
	c.syncs.Add(1)
	c.logger.Info("synchro lancée", "automatique", auto, "depot", repoID)
	go func() {
		defer c.syncs.Done()
		opts := SyncOptions{Progress: func(ev Event) { c.progress(gen, ev) }}
		var (
			report Report
			err    error
		)
		if repoID == 0 {
			report, err = c.syncer.SyncAll(ctx, opts)
		} else {
			report, err = c.syncer.SyncRepo(ctx, repoID, opts)
		}
		c.finish(gen, report, err)
	}()
	return nil
}

// progress applique un événement de la synchro gen à l'état.
func (c *Coordinator) progress(gen uint64, ev Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Generation != gen || !c.state.Running {
		return
	}
	c.state.Total = max(c.state.Total, ev.Total)
	switch ev.Kind {
	case EventRepoStarted:
		c.state.Current = ev.Repo.DisplayName
	case EventRepoFinished:
		c.state.Done = ev.Done
	}
}

// finish enregistre la fin de la synchro gen.
func (c *Coordinator) finish(gen uint64, report Report, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Generation != gen {
		return
	}
	c.state.Running = false
	c.state.LastFinishedAt = c.now().UTC()
	report.Repos = slices.Clone(report.Repos)
	c.state.LastReport, c.state.HasReport = report, true
	c.state.LastError = ""
	if err != nil {
		c.state.LastError = errorMessage(err)
		c.logger.Warn("synchro en échec", "erreur", c.state.LastError)
	}
}

// State renvoie une copie de l'état courant.
func (c *Coordinator) State() SyncState {
	c.mu.Lock()
	defer c.mu.Unlock()
	st := c.state
	st.LastReport.Repos = slices.Clone(st.LastReport.Repos)
	return st
}

// Interval renvoie l'intervalle de synchro automatique en vigueur (0 = manuelle seule).
func (c *Coordinator) Interval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state.Interval
}

// SetInterval change l'intervalle de la synchro automatique pour la durée du processus seulement (la
// configuration n'est pas réécrite) : 0 = manuelle seule, sinon au moins workspace.MinSyncInterval
// (erreur en français sinon). La prochaine synchro automatique est replanifiée à maintenant + d.
func (c *Coordinator) SetInterval(d time.Duration) error {
	if err := checkInterval(d); err != nil {
		return err
	}
	c.mu.Lock()
	c.state.Interval = d
	if d == 0 || c.ctx == nil {
		c.state.NextAuto = time.Time{}
	} else {
		c.state.NextAuto = c.now().UTC().Add(d)
	}
	c.mu.Unlock()
	select {
	case c.reschedule <- struct{}{}:
	default: // un réveil est déjà en attente
	}
	c.logger.Info("intervalle de synchro modifié", "intervalle", d)
	return nil
}

// Close arrête le coordinateur : plus aucune synchro ne démarre, la synchro en cours est annulée (le
// moteur clôt ses sync_runs « synchro annulée ») et attendue au plus timeout. Erreur si elle ne s'est
// pas arrêtée à temps : l'appelant ne doit fermer la base qu'après Close. Appels suivants sans effet.
func (c *Coordinator) Close(timeout time.Duration) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	cancel, loopDone := c.cancel, c.loopDone
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	finished := make(chan struct{})
	go func() {
		c.syncs.Wait()
		if loopDone != nil {
			<-loopDone
		}
		close(finished)
	}()
	select {
	case <-finished:
		return nil
	case <-time.After(timeout):
		return errors.New("la synchro en cours ne s'est pas arrêtée à temps")
	}
}
