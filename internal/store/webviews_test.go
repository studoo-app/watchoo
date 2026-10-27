package store

// Tests des vues de la phase 4 (interface web) : historique paginé, dates de commit, commits par
// contributeur et par dépôt, fin de la dernière synchro.

import (
	"slices"
	"testing"
	"time"
)

func TestCommitPage(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, _ := viewFixture(t, s)

	tests := []struct {
		name       string
		filter     CommitFilter
		page, per  int
		want       []string
		total      int
		gotPage    int
		pages      int
		prev, next bool
	}{
		{"première page", CommitFilter{}, 1, 2, []string{sha(6), sha(5)}, 6, 1, 3, false, true},
		{"page du milieu", CommitFilter{}, 2, 2, []string{sha(4), sha(3)}, 6, 2, 3, true, true},
		{"dernière page", CommitFilter{}, 3, 2, []string{sha(2), sha(1)}, 6, 3, 3, true, false},
		{"page trop grande ramenée à la dernière", CommitFilter{}, 99, 4, []string{sha(2), sha(1)}, 6, 2, 2, true, false},
		{"page nulle ramenée à la première", CommitFilter{}, 0, 4, []string{sha(6), sha(5), sha(4), sha(3)}, 6, 1, 2, false, true},
		{"filtre branche", CommitFilter{RepoID: aliceRepo, Branch: "dev"}, 1, 50, []string{sha(4), sha(3)}, 2, 1, 1, false, false},
		{"filtre auteur", CommitFilter{Author: "ALICE-GH"}, 1, 2, []string{sha(6), sha(2)}, 3, 1, 2, false, true},
		{"aucun commit", CommitFilter{Author: "zoé"}, 1, 50, []string{}, 0, 1, 1, false, false},
		{"Limit et Offset ignorés", CommitFilter{Limit: 1, Offset: 5}, 1, 3, []string{sha(6), sha(5), sha(4)}, 6, 1, 2, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := s.CommitPage(ctx, tt.filter, tt.page, tt.per)
			if err != nil {
				t.Fatal(err)
			}
			if p.Commits == nil {
				t.Fatal("Commits nil")
			}
			if !slices.Equal(shasOf(p.Commits), tt.want) {
				t.Errorf("SHA %v, attendu %v", shasOf(p.Commits), tt.want)
			}
			if p.Total != tt.total || p.Page != tt.gotPage || p.Pages != tt.pages {
				t.Errorf("Total %d Page %d Pages %d, attendu %d %d %d", p.Total, p.Page, p.Pages,
					tt.total, tt.gotPage, tt.pages)
			}
			if p.HasPrev() != tt.prev || p.HasNext() != tt.next {
				t.Errorf("HasPrev %t HasNext %t, attendu %t %t", p.HasPrev(), p.HasNext(), tt.prev, tt.next)
			}
		})
	}

	p, err := s.CommitPage(ctx, CommitFilter{}, 1, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if p.PerPage != MaxPerPage {
		t.Errorf("PerPage = %d, attendu %d", p.PerPage, MaxPerPage)
	}
}

func TestListRecentCommitsOffset(t *testing.T) {
	s := newTestStore(t)
	viewFixture(t, s)
	got, err := s.ListRecentCommits(t.Context(), CommitFilter{Limit: 2, Offset: 3})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{sha(3), sha(2)}; !slices.Equal(shasOf(got), want) {
		t.Errorf("SHA %v, attendu %v", shasOf(got), want)
	}
}

func TestCommitTimes(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)

	got, err := s.CommitTimes(ctx, aliceRepo, t0.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b time.Time) int { return a.Compare(b) })
	want := []time.Time{t0.Add(3 * time.Hour), t0.Add(4 * time.Hour), t0.Add(5 * time.Hour)}
	if !slices.EqualFunc(got, want, time.Time.Equal) {
		t.Errorf("CommitTimes = %v, attendu %v", got, want)
	}
	for _, tm := range got {
		if tm.Location() != time.UTC {
			t.Errorf("date %v pas en UTC", tm)
		}
	}

	all, err := s.CommitTimes(ctx, 0, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 6 {
		t.Errorf("tous les dépôts : %d dates, attendu 6", len(all))
	}
	none, err := s.CommitTimes(ctx, bobRepo, t0.Add(100*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if none == nil || len(none) != 0 {
		t.Errorf("aucune date attendue (liste vide non nil), reçu %v", none)
	}
}

func TestContributorRepoBreakdown(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)

	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, c := range ranking {
		ids[c.Name] = c.ID
	}

	got, err := s.ContributorRepoBreakdown(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		contributor, repo int64
		commits           int
		last              time.Time
		display           string
	}
	var rows []row
	for _, g := range got {
		rows = append(rows, row{g.ContributorID, g.RepoID, g.Commits, g.LastCommitAt, g.RepoDisplayName()})
	}
	want := []row{
		{ids["Alice"], aliceRepo, 2, t0.Add(2 * time.Hour), "Alice Martin"},
		{ids["Alice"], bobRepo, 1, t0.Add(6 * time.Hour), "promo/bob"},
		{ids["Bob"], aliceRepo, 2, t0.Add(4 * time.Hour), "Alice Martin"},
	}
	slices.SortFunc(want, func(a, b row) int { return int(a.contributor - b.contributor) })
	if !slices.EqualFunc(rows, want, func(a, b row) bool {
		return a.contributor == b.contributor && a.repo == b.repo && a.commits == b.commits &&
			a.last.Equal(b.last) && a.display == b.display
	}) {
		t.Errorf("ContributorRepoBreakdown = %+v\nattendu %+v", rows, want)
	}
}

func TestLastSyncFinishedAt(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	got, err := s.LastSyncFinishedAt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsZero() {
		t.Errorf("base vide : %v, attendu zéro", got)
	}

	repo := mustCreateRepo(t, s, 99, "promo", "zoe")
	beginRun(t, s, repo, t0) // en cours : pas de fin
	if got, err = s.LastSyncFinishedAt(ctx); err != nil || !got.IsZero() {
		t.Errorf("synchro en cours seulement : %v, %v ; attendu zéro", got, err)
	}

	viewFixture(t, s)
	got, err = s.LastSyncFinishedAt(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if want := t0.Add(1000*time.Hour + time.Minute); !got.Equal(want) {
		t.Errorf("LastSyncFinishedAt = %v, attendu %v", got, want)
	}
}

func TestAuthorCommitTimes(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	aliceRepo, bobRepo := viewFixture(t, s)
	ranking, err := s.ContributorRanking(ctx, ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, c := range ranking {
		ids[c.DisplayName()] = c.ID
	}

	got, err := s.AuthorCommitTimes(ctx, 0, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	// Commits 2 à 6 ; le 5 n'a pas d'auteur connu (identité vide) : absent.
	byAuthor := map[int64][]time.Time{}
	for _, a := range got {
		if a.CommittedAt.Location() != time.UTC {
			t.Errorf("date %v pas en UTC", a.CommittedAt)
		}
		byAuthor[a.ContributorID] = append(byAuthor[a.ContributorID], a.CommittedAt)
	}
	for _, list := range byAuthor {
		slices.SortFunc(list, func(a, b time.Time) int { return a.Compare(b) })
	}
	at := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Hour) }
	want := map[int64][]time.Time{
		ids["Alice"]: {at(2), at(6)},
		ids["Bob"]:   {at(3), at(4)},
	}
	if len(byAuthor) != len(want) {
		t.Fatalf("auteurs %v, attendu %v (ids %v)", byAuthor, want, ids)
	}
	for id, w := range want {
		if !slices.EqualFunc(byAuthor[id], w, time.Time.Equal) {
			t.Errorf("auteur %d : %v, attendu %v", id, byAuthor[id], w)
		}
	}

	none, err := s.AuthorCommitTimes(ctx, 0, t0.Add(100*time.Hour))
	if err != nil || len(none) != 0 {
		t.Errorf("AuthorCommitTimes(futur) = %v, %v ; attendu vide", none, err)
	}

	// Filtre par dépôt : le commit 3, sur main et dev, n'est compté qu'une fois ; le commit 6 (autre
	// dépôt) est exclu.
	only, err := s.AuthorCommitTimes(ctx, aliceRepo, t0)
	if err != nil {
		t.Fatal(err)
	}
	perAuthor := map[int64]int{}
	for _, a := range only {
		perAuthor[a.ContributorID]++
	}
	if len(only) != 4 || perAuthor[ids["Alice"]] != 2 || perAuthor[ids["Bob"]] != 2 {
		t.Errorf("dépôt d'Alice : %v, attendu 2 commits d'Alice et 2 de Bob", only)
	}
	other, err := s.AuthorCommitTimes(ctx, bobRepo, t0)
	if err != nil || len(other) != 1 || other[0].ContributorID != ids["Alice"] || !other[0].CommittedAt.Equal(at(6)) {
		t.Errorf("dépôt de Bob : %v, %v ; attendu le seul commit 6 d'Alice", other, err)
	}
}
