package store

// Journal des synchronisations (table sync_runs) et verrou de synchro par dépôt.

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// BeginSyncRun prend le verrou de synchro du dépôt repoID, dans UNE transaction (BEGIN IMMEDIATE ou
// équivalent, pour que deux processus ne puissent pas passer en même temps ; busy_timeout du DSN) :
//  1. tout sync_run « running » de ce dépôt commencé avant now - staleAfter est clos : status
//     « error », error = InterruptedMessage, finished_at = now ;
//  2. s'il reste un sync_run « running » de ce dépôt (commencé il y a moins de staleAfter), rien
//     n'est inséré et l'erreur enveloppe ErrLocked ;
//  3. sinon un sync_run {started_at: now, status: running, new_commits: 0} est inséré et renvoyé.
//
// Erreur enveloppant ErrNotFound si le dépôt n'existe pas.
//
// La clôture des sync_runs périmés (étape 1) est conservée même quand le dépôt est verrouillé par un
// autre sync_run récent.
func (s *Store) BeginSyncRun(ctx context.Context, repoID int64, now time.Time, staleAfter time.Duration) (model.SyncRun, error) {
	var run model.SyncRun
	var locked bool
	err := s.writeTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetRepository(ctx, repoID); err != nil {
			return repositoryLookupError(i18n.T("store.repo.id", repoID), err)
		}
		if err := q.CloseStaleSyncRuns(ctx, db.CloseStaleSyncRunsParams{
			Message:     InterruptedMessage(),
			FinishedAt:  nullTime(now),
			RepoID:      repoID,
			StaleBefore: FormatTime(now.Add(-staleAfter)),
		}); err != nil {
			return wrapErr(i18n.T("store.runs.close_interrupted"), err)
		}
		running, err := q.CountRunningSyncRuns(ctx, repoID)
		if err != nil {
			return wrapErr(i18n.T("store.runs.read_running"), err)
		}
		if running > 0 {
			// Pas d'erreur ici : la clôture des synchros périmées doit être validée.
			locked = true
			return nil
		}
		row, err := q.CreateSyncRun(ctx, db.CreateSyncRunParams{RepoID: repoID, StartedAt: FormatTime(now)})
		if err != nil {
			return wrapErr(i18n.T("store.runs.record"), err)
		}
		run, err = syncRunFromDB(row)
		return err
	})
	if err != nil {
		return model.SyncRun{}, wrapErr(i18n.T("store.runs.begin", repoID), err)
	}
	if locked {
		return model.SyncRun{}, wrapErr(i18n.T("store.repo.id", repoID), ErrLocked)
	}
	return run, nil
}

// FinishSyncRun clôt le sync_run runID (encore « running ») avec status (SyncSuccess ou SyncError),
// finished_at, new_commits et message d'erreur (vide pour un succès ; en français, sans token ni
// donnée personnelle). Il ne modifie pas le dépôt (last_synced_at reste celui de la dernière synchro
// réussie). Sert aux échecs : une synchro réussie est close par ApplyRepoSync. Erreur enveloppant
// ErrNotFound si runID n'existe pas ou n'est plus « running ».
func (s *Store) FinishSyncRun(ctx context.Context, runID int64, finishedAt time.Time, status model.SyncStatus, newCommits int, errMsg string) error {
	if status != model.SyncSuccess && status != model.SyncError {
		return errors.New(i18n.T("store.runs.bad_status", runID, status))
	}
	n, err := s.queries.FinishSyncRun(ctx, db.FinishSyncRunParams{
		Status:     string(status),
		FinishedAt: nullTime(finishedAt),
		NewCommits: int64(newCommits),
		Message:    errMsg,
		ID:         runID,
	})
	if err != nil {
		return wrapErr(i18n.T("store.runs.finish", runID), err)
	}
	if n == 0 {
		return wrapErr(i18n.T("store.runs.running", runID), ErrNotFound)
	}
	return nil
}

// LastSyncRun renvoie le dernier sync_run du dépôt repoID (started_at le plus récent, puis id le plus
// grand), ou une erreur enveloppant ErrNotFound s'il n'y en a aucun.
func (s *Store) LastSyncRun(ctx context.Context, repoID int64) (model.SyncRun, error) {
	row, err := s.queries.LastSyncRun(ctx, repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SyncRun{}, wrapErr(i18n.T("store.runs.repo", repoID), ErrNotFound)
		}
		return model.SyncRun{}, wrapErr(i18n.T("store.runs.last", repoID), err)
	}
	return syncRunFromDB(row)
}

// syncRunFromDB convertit une ligne sqlc en model.SyncRun.
func syncRunFromDB(row db.SyncRun) (model.SyncRun, error) {
	startedAt, err := ParseTime(row.StartedAt)
	if err != nil {
		return model.SyncRun{}, wrapErr(i18n.T("store.runs.started_at", row.ID), err)
	}
	finishedAt, err := parseNullTime(row.FinishedAt)
	if err != nil {
		return model.SyncRun{}, wrapErr(i18n.T("store.runs.finished_at", row.ID), err)
	}
	return model.SyncRun{
		ID:         row.ID,
		RepoID:     row.RepoID,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		NewCommits: int(row.NewCommits),
		Status:     model.SyncStatus(row.Status),
		Error:      row.Error,
	}, nil
}
