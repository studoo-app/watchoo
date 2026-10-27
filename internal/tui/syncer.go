package tui

// Synchros lancées depuis le TUI (« s », « S », synchro automatique). La synchro tourne dans une
// goroutine démarrée par une commande Bubble Tea ; ses événements de progression et son rapport
// arrivent par un canal, lu message par message par waitSync, et repassent par Update. Une seule
// synchro à la fois côté TUI.

import (
	"fmt"
	"strings"
	gosync "sync"
	"time"

	tea "charm.land/bubbletea/v2"

	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// quitSyncTimeout borne l'attente, en quittant, de la synchro en cours : le moteur s'arrête vite sur
// annulation et clôt ses sync_runs (« synchro annulée ») avant que la base ne soit fermée.
const quitSyncTimeout = 10 * time.Second

// syncTracker suit les goroutines de synchro lancées par le TUI, pour que Run les attende avant de
// rendre la main (sinon la base serait fermée sous elles et leurs sync_runs resteraient
// « running », ce qui verrouillerait les dépôts 30 min). Partagé par pointeur entre les copies du
// Model.
type syncTracker struct {
	mu     gosync.Mutex
	closed bool // Run est terminé : plus aucune synchro ne démarre
	wg     gosync.WaitGroup
}

// begin enregistre une synchro qui démarre ; false si le TUI est déjà fermé (ne pas la lancer).
func (t *syncTracker) begin() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return false
	}
	t.wg.Add(1)
	return true
}

// done signale la fin d'une synchro enregistrée par begin.
func (t *syncTracker) done() {
	t.wg.Done()
}

// closeAndWait empêche toute nouvelle synchro, puis attend la fin des synchros en cours, au plus
// timeout ; false si le délai est dépassé.
func (t *syncTracker) closeAndWait(timeout time.Duration) bool {
	t.mu.Lock()
	t.closed = true
	t.mu.Unlock()
	finished := make(chan struct{})
	go func() {
		t.wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
		return true
	case <-time.After(timeout):
		return false
	}
}

// syncProgressMsg est un événement de progression de la synchro gen.
type syncProgressMsg struct {
	gen int
	ev  wsync.Event
}

// syncDoneMsg est la fin de la synchro gen.
type syncDoneMsg struct {
	gen    int
	repoID int64 // 0 = tous les dépôts
	report wsync.Report
	err    error
}

// startSync lance la synchro de tous les dépôts (repoID 0) ou du dépôt repoID. Une demande pendant
// une synchro est ignorée et signalée.
func (m Model) startSync(repoID int64, auto bool) (Model, tea.Cmd) {
	if m.sync.running {
		if !auto {
			m.setStatus("une synchro est déjà en cours (" + m.sync.label + ") : patientez")
		}
		return m, nil
	}
	if m.deps.Syncer == nil {
		m.setError("synchro indisponible")
		return m, nil
	}
	label := "tous les dépôts"
	if repoID != 0 {
		label = fmt.Sprintf("dépôt n° %d", repoID)
		if r, ok := m.repoByID(repoID); ok {
			label = clean(r.DisplayName())
		}
	}
	ch := make(chan tea.Msg, 16)
	gen := m.sync.gen + 1
	m.sync = syncState{running: true, gen: gen, ch: ch, label: label, auto: auto, lastAt: m.sync.lastAt}
	if auto {
		m.setStatus("synchro automatique de " + label + "…")
	} else {
		m.setStatus("synchro de " + label + "…")
	}

	ctx, syncer, tracker := m.ctx, m.deps.Syncer, m.tracker
	start := func() tea.Msg {
		if ctx.Err() != nil || !tracker.begin() {
			close(ch)
			return nil // on quitte : la synchro ne démarre pas
		}
		go func() {
			defer tracker.done()
			defer close(ch)
			send := func(msg tea.Msg) {
				select {
				case ch <- msg:
				case <-ctx.Done():
				}
			}
			opts := wsync.SyncOptions{Progress: func(ev wsync.Event) { send(syncProgressMsg{gen: gen, ev: ev}) }}
			done := syncDoneMsg{gen: gen, repoID: repoID}
			if repoID == 0 {
				done.report, done.err = syncer.SyncAll(ctx, opts)
			} else {
				done.report, done.err = syncer.SyncRepo(ctx, repoID, opts)
			}
			send(done)
		}()
		return waitSync(ch)()
	}
	return m, start
}

// waitSync attend le prochain message de la synchro (nil quand le canal est fermé).
func waitSync(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// handleSyncProgress met à jour la progression et attend le message suivant.
func (m Model) handleSyncProgress(msg syncProgressMsg) (tea.Model, tea.Cmd) {
	if !m.sync.running || msg.gen != m.sync.gen {
		return m, nil
	}
	ev := msg.ev
	switch ev.Kind {
	case wsync.EventStarted:
		m.sync.total = ev.Total
	case wsync.EventRepoStarted:
		m.sync.current = clean(ev.Repo.DisplayName)
		m.sync.total = max(m.sync.total, ev.Total)
	case wsync.EventRepoFinished:
		m.sync.done = ev.Done
		m.sync.total = max(m.sync.total, ev.Total)
	}
	return m, waitSync(m.sync.ch)
}

// handleSyncDone affiche le bilan de la synchro et relit la base.
func (m Model) handleSyncDone(msg syncDoneMsg) (tea.Model, tea.Cmd) {
	if !m.sync.running || msg.gen != m.sync.gen {
		return m, nil
	}
	m.sync.running = false
	m.sync.ch = nil
	m.sync.lastAt = m.now()
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return m, nil // on quitte
		}
		m.setError("synchro impossible : " + clean(msg.err.Error()))
		return m, nil
	}
	text, isErr := m.summarize(msg.report)
	if isErr {
		m.setError(text)
	} else {
		m.setStatus(text)
	}
	m.loading = true
	cmds := []tea.Cmd{m.loadCmd()}
	cmds = append(cmds, m.reloadDetails()...)
	return m, tea.Batch(cmds...)
}

// summarize renvoie le bilan d'une synchro en une ligne, et s'il signale un problème. Les heures de
// reprise sont converties dans le fuseau d'affichage.
func (m Model) summarize(rep wsync.Report) (string, bool) {
	n := len(rep.Repos)
	if n == 0 {
		return "synchro terminée : aucun dépôt suivi", false
	}
	if n == 1 && rep.Repos[0].Status != wsync.StatusSuccess {
		r := rep.Repos[0]
		return clean(r.DisplayName) + " : " + m.repoIssue(r), true
	}
	parts := []string{fmt.Sprintf("synchro terminée : %s, %s", plural(n, "dépôt", "dépôts"),
		plural(rep.NewCommits(), "nouveau commit", "nouveaux commits"))}
	bad := false
	if e := rep.Count(wsync.StatusError); e > 0 {
		parts = append(parts, fmt.Sprintf("%d en erreur", e))
		bad = true
	}
	if l := rep.Count(wsync.StatusLocked); l > 0 {
		parts = append(parts, fmt.Sprintf("%d déjà en cours ailleurs", l))
	}
	if d := rep.Count(wsync.StatusDeferred); d > 0 {
		s := plural(d, "reporté", "reportés")
		for _, r := range rep.Repos {
			if r.Status == wsync.StatusDeferred && !r.ResumeAt.IsZero() {
				s += " (quota GitHub bas, reprise vers " + r.ResumeAt.In(m.deps.Location).Format("15:04") + ")"
				break
			}
		}
		parts = append(parts, s)
		bad = true
	}
	if w := rep.Warnings(); w > 0 {
		// Pull requests, releases ou tags non synchronisés : signalé, sans changer l'état des dépôts.
		parts = append(parts, plural(w, "avertissement", "avertissements")+" (pull requests, releases ou tags)")
	}
	return strings.Join(parts, ", "), bad
}

// repoIssue décrit le résultat non réussi d'un dépôt, heure de reprise en heure locale.
func (m Model) repoIssue(r wsync.RepoReport) string {
	switch r.Status {
	case wsync.StatusDeferred:
		if !r.ResumeAt.IsZero() {
			return "reporté : quota GitHub bas jusqu'à " + r.ResumeAt.In(m.deps.Location).Format("15:04")
		}
		return "reporté : quota GitHub bas"
	case wsync.StatusLocked:
		return "déjà en cours de synchronisation"
	default:
		if r.Error != "" {
			return clean(r.Error)
		}
		return "échec de la synchro"
	}
}

// plural renvoie « n mot » au singulier (0 ou 1) ou au pluriel.
func plural(n int, one, many string) string {
	if n <= 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
