package store

// Languages of a repository (1.1.0): replacement, keeping when the clone was not read, percentages,
// refusal of invalid data, cascade.

import (
	"strconv"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestApplyRepoSyncLanguages(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	other := mustCreateRepo(t, s, 2, "promo", "bob")

	list := func(id int64) []LanguageView {
		t.Helper()
		got, err := s.ListLanguages(ctx, id)
		if err != nil {
			t.Fatalf("ListLanguages: %v", err)
		}
		return got
	}
	if got := list(0); got == nil || len(got) != 0 {
		t.Fatalf("no language: %#v, want an empty non-nil list", got)
	}
	langs := []model.RepositoryLanguage{{Language: "HTML", Bytes: 251}, {Language: "Go", Bytes: 749}}
	if err := applyExtras(t, s, repoID, RepoSyncWrite{ReplaceLanguages: true, Languages: langs}); err != nil {
		t.Fatal(err)
	}
	now := t0.Add(1000 * time.Hour)
	if _, err := s.ApplyRepoSync(ctx, RepoSyncWrite{RepoID: other, RunID: beginRun(t, s, other, now),
		Metadata: meta("promo", "bob"), SyncedAt: now, ReplaceLanguages: true,
		Languages: []model.RepositoryLanguage{{Language: "Java", Bytes: 10}}}); err != nil {
		t.Fatal(err)
	}

	got := list(repoID)
	if len(got) != 2 || got[0].Language != "Go" || got[1].Language != "HTML" {
		t.Fatalf("languages = %+v, want Go then HTML", got)
	}
	if got[0].Permille() != 749 || got[1].Permille() != 251 || got[0].TotalBytes != 1000 {
		t.Errorf("shares: %d, %d (total %d)", got[0].Permille(), got[1].Permille(), got[0].TotalBytes)
	}
	if got[0].RepoFullName() != "promo/alice" {
		t.Errorf("repository: %q", got[0].RepoFullName())
	}
	if all := list(0); len(all) != 3 || all[2].Language != "Java" || all[2].Permille() != 1000 {
		t.Errorf("all the repositories: %+v", all)
	}

	// Clone not read: what is in the database stays. Read: replaced, even by nothing.
	if err := applyExtras(t, s, repoID, RepoSyncWrite{}); err != nil {
		t.Fatal(err)
	}
	if got := list(repoID); len(got) != 2 {
		t.Errorf("languages lost without a replacement: %+v", got)
	}
	if err := applyExtras(t, s, repoID, RepoSyncWrite{ReplaceLanguages: true}); err != nil {
		t.Fatal(err)
	}
	if got := list(repoID); len(got) != 0 {
		t.Errorf("languages not replaced: %+v", got)
	}

	// Invalid data: refused, nothing written.
	for i, bad := range []model.RepositoryLanguage{{Language: "", Bytes: 1}, {Language: "Go", Bytes: 0}} {
		id := mustCreateRepo(t, s, int64(10+i), "promo", "refus"+strconv.Itoa(i)) // a refusal leaves its run open
		if err := applyExtras(t, s, id, RepoSyncWrite{ReplaceLanguages: true,
			Languages: []model.RepositoryLanguage{bad}}); err == nil {
			t.Errorf("invalid language accepted: %+v", bad)
		}
		if got := list(id); len(got) != 0 {
			t.Errorf("refused language written: %+v", got)
		}
	}

	// Deleting a repository deletes its languages.
	if err := s.DeleteRepository(ctx, other); err != nil {
		t.Fatal(err)
	}
	if got := list(0); len(got) != 0 {
		t.Errorf("languages of a deleted repository: %+v", got)
	}
}
