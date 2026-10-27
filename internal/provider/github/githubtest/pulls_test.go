package githubtest

import (
	"net/http"
	"testing"
	"time"
)

func TestPullRequestsRoute(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	s.SetPullRequests(1,
		PullRequest{Number: 1, Title: "a", Author: "alice", CreatedAt: t0, UpdatedAt: t0.Add(3 * time.Hour)},
		PullRequest{Number: 2, Title: "b", Closed: true, CreatedAt: t0.Add(time.Hour), MergedAt: t0.Add(2 * time.Hour), UpdatedAt: t0.Add(2 * time.Hour)},
		PullRequest{Number: 3, Title: "c", Draft: true, CreatedAt: t0.Add(2 * time.Hour)},
	)
	numbers := func(path string) []int {
		t.Helper()
		r := get(t, s, path)
		wantStatus(t, path, r, http.StatusOK)
		var list []struct {
			Number   int     `json:"number"`
			State    string  `json:"state"`
			MergedAt *string `json:"merged_at"`
			ClosedAt *string `json:"closed_at"`
			User     *struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		decode(t, r, &list)
		out := make([]int, len(list))
		for i, pr := range list {
			out[i] = pr.Number
			if pr.Number == 2 && (pr.State != "closed" || pr.MergedAt == nil || pr.ClosedAt == nil) {
				t.Errorf("PR 2 mergée : %+v", pr)
			}
			if pr.Number == 3 && pr.User != nil {
				t.Errorf("PR 3 sans auteur : user = %+v", pr.User)
			}
		}
		return out
	}
	for _, tt := range []struct {
		path string
		want []int
	}{
		{"/repos/alice/projet/pulls", []int{3, 1}},                                          // ouvertes, création décroissante
		{"/repos/alice/projet/pulls?state=all&sort=updated&direction=desc", []int{1, 3, 2}}, // modification décroissante, puis numéro
		{"/repos/alice/projet/pulls?state=closed", []int{2}},
		{"/repos/alice/projet/pulls?state=all&sort=updated", []int{2, 3, 1}}, // asc par défaut avec sort=updated
		{"/repos/alice/projet/pulls?state=all&per_page=1&page=2", []int{2}},
	} {
		if got := numbers(tt.path); len(got) != len(tt.want) || (len(got) > 0 && got[0] != tt.want[0]) ||
			(len(got) > 1 && got[len(got)-1] != tt.want[len(tt.want)-1]) {
			t.Errorf("%s : %v, veut %v", tt.path, got, tt.want)
		}
	}
	wantStatus(t, "state invalide", get(t, s, "/repos/alice/projet/pulls?state=merged"), http.StatusUnprocessableEntity)
	wantStatus(t, "dépôt inconnu", get(t, s, "/repos/alice/autre/pulls"), http.StatusNotFound)
}

func TestReleasesAndTagsRoutes(t *testing.T) {
	s := NewServer(t, Options{})
	s.AddRepo(Repo{ID: 1, Owner: "alice", Name: "projet"})
	sha := FakeSHA("x")
	s.SetReleases(1,
		Release{ID: 1, TagName: "v1", CreatedAt: t0, PublishedAt: t0},
		Release{ID: 2, TagName: "v2", Draft: true, Name: "Deux", Author: "alice", CreatedAt: t0.Add(time.Hour)},
	)
	s.SetTags(1, Tag{Name: "v2", SHA: sha}, Tag{Name: "v1", SHA: sha})

	r := get(t, s, "/repos/alice/projet/releases")
	wantStatus(t, "releases", r, http.StatusOK)
	var releases []struct {
		ID          int64   `json:"id"`
		Name        *string `json:"name"`
		PublishedAt *string `json:"published_at"`
		Draft       bool    `json:"draft"`
	}
	decode(t, r, &releases)
	if len(releases) != 2 || releases[0].ID != 2 || !releases[0].Draft || releases[0].PublishedAt != nil ||
		releases[1].Name != nil {
		t.Errorf("releases = %+v", releases)
	}

	r = get(t, s, "/repos/alice/projet/tags?per_page=1")
	wantStatus(t, "tags", r, http.StatusOK)
	var tags []struct {
		Name   string `json:"name"`
		Commit struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	decode(t, r, &tags)
	if len(tags) != 1 || tags[0].Name != "v2" || tags[0].Commit.SHA != sha {
		t.Errorf("tags = %+v", tags)
	}
	if link := r.header.Get("Link"); link == "" {
		t.Error("tags paginés sans en-tête Link")
	}

	rec := &recordTB{TB: t}
	bad := NewServer(rec, Options{})
	bad.AddRepo(Repo{ID: 1, Owner: "a", Name: "b"})
	expectFatal(t, rec, "PR sans numéro", func() { bad.SetPullRequests(1, PullRequest{CreatedAt: t0}) })
	expectFatal(t, rec, "release sans tag", func() { bad.SetReleases(1, Release{ID: 1, CreatedAt: t0}) })
	expectFatal(t, rec, "tag mal formé", func() { bad.SetTags(1, Tag{Name: "v1", SHA: "abc"}) })
}
