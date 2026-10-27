package store

// Journal des synchronisations (table sync_runs) et verrou de synchro par dépôt.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

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
			return repositoryLookupError(fmt.Sprintf("dépôt %d", repoID), err)
		}
		if err := q.CloseStaleSyncRuns(ctx, db.CloseStaleSyncRunsParams{
			Message:     InterruptedMessage,
			FinishedAt:  nullTime(now),
			RepoID:      repoID,
			StaleBefore: FormatTime(now.Add(-staleAfter)),
		}); err != nil {
			return fmt.Errorf("clôture des synchros interrompues : %w", err)
		}
		running, err := q.CountRunningSyncRuns(ctx, repoID)
		if err != nil {
			return fmt.Errorf("lecture des synchros en cours : %w", err)
		}
		if running > 0 {
			// Pas d'erreur ici : la clôture des synchros périmées doit être validée.
			locked = true
			return nil
		}
		row, err := q.CreateSyncRun(ctx, db.CreateSyncRunParams{RepoID: repoID, StartedAt: FormatTime(now)})
		if err != nil {
			return fmt.Errorf("enregistrement de la synchro : %w", err)
		}
		run, err = syncRunFromDB(row)
		return err
	})
	if err != nil {
		return model.SyncRun{}, fmt.Errorf("début de synchro du dépôt %d : %w", repoID, err)
	}
	if locked {
		return model.SyncRun{}, fmt.Errorf("dépôt %d : %w", repoID, ErrLocked)
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
		return fmt.Errorf("fin de synchro %d : statut %q invalide", runID, status)
	}
	n, err := s.queries.FinishSyncRun(ctx, db.FinishSyncRunParams{
		Status:     string(status),
		FinishedAt: nullTime(finishedAt),
		NewCommits: int64(newCommits),
		Message:    errMsg,
		ID:         runID,
	})
	if err != nil {
		return fmt.Errorf("fin de synchro %d : %w", runID, err)
	}
	if n == 0 {
		return fmt.Errorf("synchro en cours %d : %w", runID, ErrNotFound)
	}
	return nil
}

// LastSyncRun renvoie le dernier sync_run du dépôt repoID (started_at le plus récent, puis id le plus
// grand), ou une erreur enveloppant ErrNotFound s'il n'y en a aucun.
func (s *Store) LastSyncRun(ctx context.Context, repoID int64) (model.SyncRun, error) {
	row, err := s.queries.LastSyncRun(ctx, repoID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.SyncRun{}, fmt.Errorf("synchro du dépôt %d : %w", repoID, ErrNotFound)
		}
		return model.SyncRun{}, fmt.Errorf("dernière synchro du dépôt %d : %w", repoID, err)
	}
	return syncRunFromDB(row)
}

// syncRunFromDB convertit une ligne sqlc en model.SyncRun.
func syncRunFromDB(row db.SyncRun) (model.SyncRun, error) {
	startedAt, err := ParseTime(row.StartedAt)
	if err != nil {
		return model.SyncRun{}, fmt.Errorf("synchro %d, started_at : %w", row.ID, err)
	}
	finishedAt, err := parseNullTime(row.FinishedAt)
	if err != nil {
		return model.SyncRun{}, fmt.Errorf("synchro %d, finished_at : %w", row.ID, err)
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
