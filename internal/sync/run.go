package sync

// Déroulement d'une synchro de plusieurs dépôts : progression, parallélisme borné, verrou, garde-fou
// de quota, arrêt sur quota épuisé et rapport.

import (
	"context"
	"errors"
	"fmt"
	gosync "sync"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// Messages fixes des rapports (en français, sans donnée personnelle).
const (
	msgLocked    = "déjà en cours de synchronisation"
	msgCanceled  = "synchro annulée"
	msgDeadline  = "synchro interrompue : délai dépassé"
	reasonLow    = "quota bas"
	reasonSpent  = "quota épuisé"
	resumeLayout = "15:04"
)

// progress sérialise les appels à SyncOptions.Progress et tient le compte des dépôts terminés.
type progress struct {
	mu    gosync.Mutex
	fn    func(Event)
	done  int
	total int
}

// send envoie un événement (sans effet si aucune fonction n'est fournie). finished compte le dépôt
// comme terminé avant l'envoi.
func (p *progress) send(kind EventKind, repo RepoReport, finished bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if finished {
		p.done++
	}
	if p.fn == nil {
		return
	}
	p.fn(Event{Kind: kind, Repo: repo, Done: p.done, Total: p.total})
}

// halt mémorise l'arrêt des dépôts pas encore commencés (quota bas ou épuisé), partagé par les
// goroutines d'une synchro.
type halt struct {
	mu       gosync.Mutex
	halted   bool
	resumeAt time.Time
	reason   string
}

// set arrête les dépôts suivants jusqu'à resumeAt (la plus lointaine des échéances l'emporte ; un
// quota épuisé l'emporte sur un quota bas).
func (h *halt) set(resumeAt time.Time, reason string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.halted || resumeAt.After(h.resumeAt) {
		h.resumeAt = resumeAt.UTC()
	}
	if !h.halted || reason == reasonSpent {
		h.reason = reason
	}
	h.halted = true
}

// get renvoie l'arrêt en cours, s'il y en a un.
func (h *halt) get() (time.Time, string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.resumeAt, h.reason, h.halted
}

// QuotaLow indique si le quota observé rl est sous QuotaReservePercent de la limite et que sa
// fenêtre n'est pas encore terminée à now (après la remise à zéro, l'état observé est périmé) :
// garde-fou du moteur, que les couches d'affichage (« watchoo status ») signalent avec la même règle.
func QuotaLow(rl provider.RateLimit, now time.Time) bool {
	if rl.Limit <= 0 {
		return false
	}
	if !rl.Reset.IsZero() && !now.Before(rl.Reset) {
		return false
	}
	return rl.Remaining*100 < rl.Limit*QuotaReservePercent
}

// deferredMessage renvoie « reporté : <raison> jusqu'à HH:MM UTC » (sans heure si elle est inconnue).
// L'heure reste en UTC : les couches d'affichage la convertissent à partir de RepoReport.ResumeAt.
func deferredMessage(reason string, resumeAt time.Time) string {
	if resumeAt.IsZero() {
		return "reporté : " + reason
	}
	return fmt.Sprintf("reporté : %s jusqu'à %s UTC", reason, resumeAt.UTC().Format(resumeLayout))
}

// rateLimitResume renvoie le moment où réessayer après rle : Reset, sinon now + RetryAfter, sinon
// zéro.
func rateLimitResume(rle *provider.RateLimitError, now time.Time) time.Time {
	switch {
	case !rle.Reset.IsZero():
		return rle.Reset.UTC()
	case rle.RetryAfter > 0:
		return now.Add(rle.RetryAfter).UTC()
	default:
		return time.Time{}
	}
}

// errorMessage renvoie le message d'échec d'un dépôt, en français. Les erreurs du Provider et du
// store ne contiennent ni token ni donnée personnelle ; le moteur masque en plus le token (redact).
func errorMessage(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return msgCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return msgDeadline
	default:
		return err.Error()
	}
}

// baseReport renvoie le rapport initial d'un dépôt (avant synchro).
func baseReport(r model.Repository) RepoReport {
	return RepoReport{RepoID: r.ID, FullName: r.FullName(), DisplayName: r.DisplayName()}
}

// emptyReport renvoie le rapport d'une synchro qui n'a pu commencer (échec global avant le premier
// dépôt), après avoir envoyé les événements de début et de fin.
func (e *Engine) emptyReport(opts SyncOptions) Report {
	prog := &progress{fn: opts.Progress}
	now := e.now().UTC()
	prog.send(EventStarted, RepoReport{}, false)
	prog.send(EventFinished, RepoReport{}, false)
	return Report{StartedAt: now, FinishedAt: now, Repos: []RepoReport{}}
}

// run synchronise repos (voir SyncAll) et renvoie le rapport, un RepoReport par dépôt dans l'ordre.
func (e *Engine) run(ctx context.Context, repos []model.Repository, opts SyncOptions) (Report, error) {
	prog := &progress{fn: opts.Progress, total: len(repos)}
	report := Report{StartedAt: e.now().UTC(), Repos: []RepoReport{}}
	prog.send(EventStarted, RepoReport{}, false)
	finish := func() {
		report.FinishedAt = e.now().UTC()
		prog.send(EventFinished, RepoReport{}, false)
	}

	prov, redact, err := e.openProvider(ctx)
	if err != nil {
		finish()
		return report, err
	}
	if _, ok := prov.LastRateLimit(); !ok {
		// Quota encore inconnu : un appel gratuit pour que le garde-fou fonctionne dès le premier
		// dépôt. Son échec n'empêche pas la synchro.
		if _, err := prov.RateLimit(ctx); err != nil {
			e.logger.Debug("lecture du quota impossible avant la synchro", "erreur", redact(errorMessage(err)))
		}
	}

	report.Repos = make([]RepoReport, len(repos))
	h := &halt{}
	forEach(len(repos), e.cfg.Sync.Parallelism, func(i int) {
		rep := e.syncOne(ctx, prov, redact, h, repos[i], prog)
		report.Repos[i] = rep // indice propre à chaque goroutine
		prog.send(EventRepoFinished, rep, true)
	})
	report.RateLimit, report.RateLimitKnown = prov.LastRateLimit()
	finish()
	e.logger.Info("synchro terminée",
		"depots", len(repos),
		"succes", report.Count(StatusSuccess),
		"erreurs", report.Count(StatusError),
		"verrouilles", report.Count(StatusLocked),
		"reportes", report.Count(StatusDeferred),
		"nouveaux_commits", report.NewCommits(),
		"duree", report.FinishedAt.Sub(report.StartedAt).Round(time.Millisecond))
	if err := ctx.Err(); err != nil {
		return report, err
	}
	return report, nil
}

// syncOne applique à un dépôt les contrôles de SyncAll (annulation, arrêt, garde-fou, verrou), puis
// le synchronise et enregistre un éventuel échec dans sync_runs.
func (e *Engine) syncOne(ctx context.Context, prov provider.Provider, redact func(string) string, h *halt,
	repo model.Repository, prog *progress) RepoReport {
	rep := baseReport(repo)
	if err := ctx.Err(); err != nil {
		rep.Status, rep.Error = StatusError, errorMessage(err)
		return rep
	}
	defer func() {
		if rep.Status == StatusDeferred {
			e.logger.Info("dépôt reporté", "repo", rep.FullName, "reprise", rep.ResumeAt)
		}
	}()
	if resumeAt, reason, ok := h.get(); ok {
		rep.Status, rep.ResumeAt, rep.Error = StatusDeferred, resumeAt, deferredMessage(reason, resumeAt)
		return rep
	}
	if rl, ok := prov.LastRateLimit(); ok && QuotaLow(rl, e.now()) {
		h.set(rl.Reset, reasonLow)
		resumeAt, reason, _ := h.get()
		rep.Status, rep.ResumeAt, rep.Error = StatusDeferred, resumeAt, deferredMessage(reason, resumeAt)
		return rep
	}

	run, err := e.store.BeginSyncRun(ctx, repo.ID, e.now().UTC(), LockStaleAfter)
	if errors.Is(err, store.ErrLocked) {
		rep.Status, rep.Error = StatusLocked, msgLocked
		e.logger.Info("dépôt ignoré : synchro déjà en cours", "repo", rep.FullName)
		return rep
	}
	if err != nil {
		rep.Status, rep.Error = StatusError, redact(errorMessage(err))
		e.logger.Warn("début de synchro impossible", "repo", rep.FullName, "erreur", rep.Error)
		return rep
	}

	prog.send(EventRepoStarted, rep, false)
	start := e.now()
	res, err := e.syncRepository(ctx, prov, h, repo, run)
	rep.Duration = e.now().Sub(start)
	if err != nil {
		var rle *provider.RateLimitError
		if errors.As(err, &rle) {
			h.set(rateLimitResume(rle, e.now()), reasonSpent)
		}
		rep.Status, rep.Error = StatusError, redact(errorMessage(err))
		// Contexte détaché : un échec dû à l'annulation doit quand même être enregistré.
		if ferr := e.store.FinishSyncRun(context.WithoutCancel(ctx), run.ID, e.now().UTC(), model.SyncError, 0,
			rep.Error); ferr != nil {
			e.logger.Error("enregistrement de l'échec de synchro impossible", "repo", rep.FullName,
				"erreur", redact(ferr.Error()))
		}
		e.logger.Warn("échec de synchro", "repo", rep.FullName, "erreur", rep.Error,
			"duree", rep.Duration.Round(time.Millisecond))
		return rep
	}
	rep.Status = StatusSuccess
	rep.FullName = res.fullName
	rep.DisplayName = res.displayName
	rep.NewCommits = res.result.NewCommits
	rep.TrackedBranches = res.result.TrackedBranches
	rep.StatsFetched = res.stats
	e.logger.Info("dépôt synchronisé", "repo", rep.FullName, "nouveaux_commits", rep.NewCommits,
		"branches", rep.TrackedBranches, "statistiques", rep.StatsFetched,
		"duree", rep.Duration.Round(time.Millisecond))
	return rep
}
