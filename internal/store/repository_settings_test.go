package store

import (
	"errors"
	"slices"
	"testing"
)

func TestUpdateRepositorySettings(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "promo", "alice")

	repo, err := s.UpdateRepositorySettings(ctx, id, "Alice Martin", []string{"main", " release/* ", ""})
	if err != nil {
		t.Fatal(err)
	}
	if repo.Label != "Alice Martin" || !slices.Equal(repo.BranchPatterns, []string{"main", "release/*"}) {
		t.Errorf("dépôt renvoyé : %+v", repo)
	}
	var raw string
	if err := s.DB().QueryRowContext(ctx, "SELECT branch_patterns FROM repositories WHERE id = ?", id).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if raw != "main,release/*" {
		t.Errorf("branch_patterns = %q", raw)
	}

	// Motifs vides : toutes les branches.
	repo, err = s.UpdateRepositorySettings(ctx, id, "", nil)
	if err != nil || repo.Label != "" || repo.BranchPatterns != nil {
		t.Errorf("remise à zéro : %+v, %v", repo, err)
	}
	if _, err := s.UpdateRepositorySettings(ctx, 999, "x", nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("dépôt inexistant : %v, attendu ErrNotFound", err)
	}
}

func TestSetRepositoryEnabled(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "promo", "alice")
	applySync(t, s, id, []BranchState{{Name: "main", HeadSHA: sha(1)}},
		cw(1, person("Max", "max@example.test", "", 0), "main"))

	if err := s.SetRepositoryEnabled(ctx, id, false); err != nil {
		t.Fatal(err)
	}
	repo, _ := s.GetRepository(ctx, id)
	if repo.Enabled {
		t.Error("suivi non arrêté")
	}
	if got := count(t, s, "SELECT count(*) FROM commits WHERE repo_id = ?", id); got != 1 {
		t.Error("historique perdu à l'arrêt du suivi")
	}
	if err := s.SetRepositoryEnabled(ctx, id, true); err != nil {
		t.Fatal(err)
	}
	if repo, _ := s.GetRepository(ctx, id); !repo.Enabled {
		t.Error("suivi non réactivé")
	}
	if err := s.SetRepositoryEnabled(ctx, 999, true); !errors.Is(err, ErrNotFound) {
		t.Errorf("dépôt inexistant : %v, attendu ErrNotFound", err)
	}
}

func TestSetRepositoryThresholds(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	id := mustCreateRepo(t, s, 1, "promo", "alice")
	flemme, dead := 3, 10

	repo, err := s.SetRepositoryThresholds(ctx, id, &flemme, &dead)
	if err != nil {
		t.Fatal(err)
	}
	if repo.FlemmeAfterDays == nil || *repo.FlemmeAfterDays != 3 || repo.DeadAfterDays == nil || *repo.DeadAfterDays != 10 {
		t.Errorf("seuils renvoyés : %v, %v", repo.FlemmeAfterDays, repo.DeadAfterDays)
	}
	if got, _ := s.GetRepository(ctx, id); got.FlemmeAfterDays == nil || *got.FlemmeAfterDays != 3 {
		t.Errorf("seuils relus : %+v", got)
	}

	// Un seul seuil propre, l'autre revient à la valeur globale (NULL).
	repo, err = s.SetRepositoryThresholds(ctx, id, nil, &dead)
	if err != nil || repo.FlemmeAfterDays != nil || repo.DeadAfterDays == nil {
		t.Errorf("seuil Flemme retiré : %+v, %v", repo, err)
	}
	var n int
	if err := s.DB().QueryRowContext(ctx,
		"SELECT count(*) FROM repositories WHERE id = ? AND flemme_after_days IS NULL", id).Scan(&n); err != nil || n != 1 {
		t.Errorf("flemme_after_days non NULL en base (%d, %v)", n, err)
	}

	// La contrainte CHECK refuse une valeur nulle.
	zero := 0
	if _, err := s.SetRepositoryThresholds(ctx, id, &zero, nil); err == nil {
		t.Error("seuil 0 accepté par la base")
	}
	if _, err := s.SetRepositoryThresholds(ctx, 999, nil, nil); !errors.Is(err, ErrNotFound) {
		t.Errorf("dépôt inexistant : %v, attendu ErrNotFound", err)
	}
}
