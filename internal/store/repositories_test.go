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
		ID:              999, // ignoré
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
		t.Fatalf("CreateRepository : %v", err)
	}
	if created.ID == 0 || created.ID == in.ID {
		t.Errorf("ID = %d : l'ID fourni devrait être ignoré et un ID attribué", created.ID)
	}

	want := in
	want.ID = created.ID
	want.LastSyncedAt = time.Date(2026, 10, 26, 14, 3, 0, 0, time.UTC) // UTC, à la seconde
	if !reflect.DeepEqual(created, want) {
		t.Errorf("CreateRepository =\n%+v\nattendu\n%+v", created, want)
	}

	got, err := s.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository : %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetRepository =\n%+v\nattendu\n%+v", got, want)
	}

	// Stockage : dates au format fixe.
	var lastSynced string
	if err := s.DB().QueryRowContext(ctx, "SELECT last_synced_at FROM repositories WHERE id = ?",
		created.ID).Scan(&lastSynced); err != nil {
		t.Fatalf("lecture de last_synced_at : %v", err)
	}
	if lastSynced != "2026-10-26T14:03:00Z" {
		t.Errorf("last_synced_at stocké = %q, attendu %q", lastSynced, "2026-10-26T14:03:00Z")
	}
}

func TestCreateRepositoryNullableFields(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	created, err := s.CreateRepository(ctx, testRepo(1, "promo", "bob"))
	if err != nil {
		t.Fatalf("CreateRepository : %v", err)
	}
	// Dates zéro et seuils nil sont enregistrés à NULL.
	n := count(t, s, `SELECT count(*) FROM repositories WHERE id = ?
		AND last_synced_at IS NULL AND last_commit_at IS NULL
		AND flemme_after_days IS NULL AND dead_after_days IS NULL`, created.ID)
	if n != 1 {
		t.Error("dates zéro et seuils nil : NULL attendu en base")
	}

	got, err := s.GetRepository(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetRepository : %v", err)
	}
	if !got.LastSyncedAt.IsZero() || !got.LastCommitAt.IsZero() {
		t.Errorf("dates = %v / %v, attendu la valeur zéro", got.LastSyncedAt, got.LastCommitAt)
	}
	if got.FlemmeAfterDays != nil || got.DeadAfterDays != nil {
		t.Errorf("seuils = %v / %v, attendu nil", got.FlemmeAfterDays, got.DeadAfterDays)
	}
	if !got.Enabled || got.Archived {
		t.Errorf("Enabled = %v, Archived = %v, attendu true, false", got.Enabled, got.Archived)
	}
}

func TestCreateRepositoryAlreadyExistsMessage(t *testing.T) {
	s := newTestStore(t)
	mustCreateRepo(t, s, 1, "promo", "alice")
	_, err := s.CreateRepository(t.Context(), testRepo(1, "promo", "alice"))
	if !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("erreur %v, attendu ErrAlreadyExists", err)
	}
	if !strings.Contains(err.Error(), "promo/alice") {
		t.Errorf("erreur %q : le dépôt devrait être cité", err)
	}
}

func TestGetRepositoryByName(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "Promo-2026", "Projet-Alice")

	got, err := s.GetRepositoryByName(ctx, model.ProviderGitHub, "promo-2026", "PROJET-alice")
	if err != nil {
		t.Fatalf("GetRepositoryByName sans tenir compte de la casse : %v", err)
	}
	if got.ID != id || got.Owner != "Promo-2026" || got.Name != "Projet-Alice" {
		t.Errorf("GetRepositoryByName = %+v, attendu l'ID %d avec la casse d'origine", got, id)
	}

	for _, tt := range []struct{ provider, owner, name string }{
		{model.ProviderGitHub, "promo-2026", "inconnu"},
		{model.ProviderGitHub, "autre", "projet-alice"},
		{"gitlab", "promo-2026", "projet-alice"},
	} {
		_, err := s.GetRepositoryByName(ctx, tt.provider, tt.owner, tt.name)
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("GetRepositoryByName(%s, %s, %s) : erreur %v, attendu ErrNotFound",
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
		t.Fatalf("GetRepositoryByProviderID : %v", err)
	}
	if got.ID != id {
		t.Errorf("ID = %d, attendu %d", got.ID, id)
	}
	if _, err := s.GetRepositoryByProviderID(ctx, model.ProviderGitHub, 1); !errors.Is(err, ErrNotFound) {
		t.Errorf("identifiant inconnu : erreur %v, attendu ErrNotFound", err)
	}
	if _, err := s.GetRepositoryByProviderID(ctx, "gitlab", 4242); !errors.Is(err, ErrNotFound) {
		t.Errorf("autre forge : erreur %v, attendu ErrNotFound", err)
	}
}

func TestGetRepositoryNotFound(t *testing.T) {
	s := newTestStore(t)
	_, err := s.GetRepository(t.Context(), 42)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("erreur %v, attendu ErrNotFound", err)
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("erreur %q : l'identifiant devrait être cité", err)
	}
}

func TestListRepositories(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()

	empty, err := s.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories : %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Errorf("ListRepositories sur une base vide = %#v, attendu une liste vide non nil", empty)
	}

	// Tri par owner puis name, sans tenir compte de la casse.
	mustCreateRepo(t, s, 1, "promo-b", "zoe")
	mustCreateRepo(t, s, 2, "Promo-A", "bob")
	mustCreateRepo(t, s, 3, "promo-a", "Alice")
	mustCreateRepo(t, s, 4, "promo-b", "Adam")

	repos, err := s.ListRepositories(ctx)
	if err != nil {
		t.Fatalf("ListRepositories : %v", err)
	}
	var got []string
	for _, r := range repos {
		got = append(got, r.FullName())
	}
	want := []string{"promo-a/Alice", "Promo-A/bob", "promo-b/Adam", "promo-b/zoe"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ListRepositories = %v, attendu %v", got, want)
	}
}

func TestDeleteRepository(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "promo", "alice")

	if err := s.DeleteRepository(ctx, id); err != nil {
		t.Fatalf("DeleteRepository : %v", err)
	}
	if _, err := s.GetRepository(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("GetRepository après suppression : erreur %v, attendu ErrNotFound", err)
	}
	if err := s.DeleteRepository(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Errorf("seconde suppression : erreur %v, attendu ErrNotFound", err)
	}
}
