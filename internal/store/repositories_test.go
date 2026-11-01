package store

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func intp(v int) *int { return &v }

func TestCreateRepositoryRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	paris := time.FixedZone("UTC+1", 3600)

	in := model.Repository{
		ID:              999, // ignored
		Provider:        model.ProviderGitHub,
		ProviderID:      123456789,
		Owner:           "promo-2026",
		Name:            "projet-alice",
		Label:           "Alice Martin",
		DefaultBranch:   "main",
		URL:             "https://github.com/promo-2026/projet-alice",
		Stars:           3,
		OpenIssues:      2,
		LastSyncedAt:    time.Date(2026, 10, 26, 15, 3, 0, 500_000_000, paris),
		LastCommitAt:    time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC),
		Archived:        true,
		FlemmeAfterDays: intp(3),
		DeadAfterDays:   intp(10),
		Enabled:         false,
	}
	created, err := s.CreateRepository(ctx, in)
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	if created.ID == 0 || created.ID == in.ID {
		t.Errorf("ID = %d: the given ID should be ignored and an ID assigned", created.ID)
	}

	want := in
	want.ID = created.ID
	want.LastSyncedAt = time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC) // UTC, to the second
	if !reflect.DeepEqual(created, want) {
		t.Errorf("CreateRepository =\n%+v\nwant\n%+v", created, want)
	}

	got, err := s.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetRepository =\n%+v\nwant\n%+v", got, want)
	}

	// Storage: dates in the fixed format.
	var lastSynced string
	if err := s.DB().QueryRowContext(ctx, "SELECT last_synced_at FROM repositories WHERE id = ?",
		created.ID).Scan(&lastSynced); err != nil {
		t.Fatalf("reading last_synced_at: %v", err)
	}
	if lastSynced != "2026-10-26T14:03:00Z" {
		t.Errorf("stored last_synced_at = %q, want %q", lastSynced, "2026-10-26T14:03:00Z")
	}
}

func TestCreateRepositoryNullableFields(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	created, err := s.CreateRepository(ctx, testRepo(1, "promo", "bob"))
	if err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}
	// Zero dates and nil thresholds are stored as NULL.
	n := count(t, s, `SELECT count(*) FROM repositories WHERE id = ?
		AND last_synced_at IS NULL AND last_commit_at IS NULL
		AND flemme_after_days IS NULL AND dead_after_days IS NULL`, created.ID)
	if n != 1 {
		t.Error("zero dates and nil thresholds: NULL expected in the database")
	}

	got, err := s.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository: %v", err)
	}
	if !got.LastSyncedAt.IsZero() || !got.LastCommitAt.IsZero() {
		t.Errorf("dates = %v / %v, want the zero value", got.LastSyncedAt, got.LastCommitAt)
	}
	if got.FlemmeAfterDays != nil || got.DeadAfterDays != nil {
		t.Errorf("thresholds = %v / %v, want nil", got.FlemmeAfterDays, got.DeadAfterDays)
	}
	if !got.Enabled || got.Archived {
		t.Errorf("Enabled = %v, Archived = %v, want true, false", got.Enabled, got.Archived)
	}
}

func TestCreateRepositoryAlreadyExistsMessage(t *testing.T) {
	s := newTestStore(t)
	mustCreateRepo(t, s, 1, "promo", "alice")
	_, err := s.CreateRepository(t.Context(), testRepo(1, "promo", "alice"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("error %v, want ErrAlreadyExists", err)
	}
	if !strings.Contains(err.Error(), "promo/alice") {
		t.Errorf("error %q: the repository should be quoted", err)
	}
}

func TestGetRepositoryByName(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "Promo-2026", "Projet-Alice")

	got, err := s.GetRepositoryByName(ctx, model.ProviderGitHub, "promo-2026", "PROJET-alice")
	if err != nil {
		t.Fatalf("case-insensitive GetRepositoryByName: %v", err)
	}
	if got.ID != id || got.Owner != "Promo-2026" || got.Name != "Projet-Alice" {
		t.Errorf("GetRepositoryByName = %+v, want ID %d with the original case", got, id)
	}

	for _, tt := range []struct{ provider, owner, name string }{
		{model.ProviderGitHub, "promo-2026", "inconnu"},
		{model.ProviderGitHub, "autre", "projet-alice"},
		{"gitlab", "promo-2026", "projet-alice"},
	} {
		_, err := s.GetRepositoryByName(ctx, tt.provider, tt.owner, tt.name)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("GetRepositoryByName(%s, %s, %s): error %v, want ErrNotFound",
				tt.provider, tt.owner, tt.name, err)
		}
	}
}

func TestGetRepositoryByProviderID(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 4242, "promo", "alice")

	got, err := s.GetRepositoryByProviderID(ctx, model.ProviderGitHub, 4242)
	if err != nil {
		t.Fatalf("GetRepositoryByProviderID: %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, want %d", got.ID, id)
	}
	if _, err := s.GetRepositoryByProviderID(ctx, model.ProviderGitHub, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("unknown identifier: error %v, want ErrNotFound", err)
	}
	if _, err := s.GetRepositoryByProviderID(ctx, "gitlab", 4242); !errors.Is(err, ErrNotFound) {
		t.Errorf("other forge: error %v, want ErrNotFound", err)
	}
}

func TestGetRepositoryNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRepository(t.Context(), 42)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error %v, want ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error %q: the identifier should be quoted", err)
	}
}

func TestListRepositories(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	empty, err := s.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListRepositories on an empty database = %#v, want a non-nil empty list", empty)
	}

	// Sorted by owner then name, case-insensitively.
	mustCreateRepo(t, s, 1, "promo-b", "zoe")
	mustCreateRepo(t, s, 2, "Promo-A", "bob")
	mustCreateRepo(t, s, 3, "promo-a", "Alice")
	mustCreateRepo(t, s, 4, "promo-b", "Adam")

	repos, err := s.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories: %v", err)
	}
	var got []string
	for _, r := range repos {
		got = append(got, r.FullName())
	}
	want := []string{"promo-a/Alice", "Promo-A/bob", "promo-b/Adam", "promo-b/zoe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListRepositories = %v, want %v", got, want)
	}
}

func TestDeleteRepository(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "promo", "alice")

	if err := s.DeleteRepository(ctx, id); err != nil {
		t.Fatalf("DeleteRepository: %v", err)
	}
	if _, err := s.GetRepository(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRepository after deletion: error %v, want ErrNotFound", err)
	}
	if err := s.DeleteRepository(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("second deletion: error %v, want ErrNotFound", err)
	}
}
