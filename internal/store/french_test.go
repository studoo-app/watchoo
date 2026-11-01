package store

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// The messages follow the process language: French typography (space before ":"), French message
// recorded in sync_runs.error for an interrupted sync. Not parallel: the process language is shared.
func TestMessagesFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	s := newTestStore(t)
	ctx := context.Background()
	if _, err := s.GetRepository(ctx, 42); !errors.Is(err, ErrNotFound) || err.Error() != "dépôt 42 : élément introuvable" {
		t.Errorf("GetRepository = %v", err)
	}
	if _, err := ParseTime(""); err == nil || err.Error() != "date vide : format attendu "+TimeLayout {
		t.Errorf("ParseTime = %v", err)
	}
	if got := InterruptedMessage(); got != "synchro interrompue" {
		t.Errorf("InterruptedMessage = %q", got)
	}
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	now := time.Date(2026, 10, 26, 14, 0, 0, 0, time.UTC)
	if _, err := s.BeginSyncRun(ctx, repoID, now, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := s.BeginSyncRun(ctx, repoID, now, 30*time.Minute); !errors.Is(err, ErrLocked) ||
		err.Error() != "dépôt "+strconv.FormatInt(repoID, 10)+" : déjà en cours de synchronisation" {
		t.Errorf("BeginSyncRun = %v", err)
	}
}
