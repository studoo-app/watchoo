package github

// Tests of pulls.go (pull requests, releases, tags, V2) against the fake githubtest server: conversion,
// pagination and caps, early stop UpdatedSince, ETag and 304, errors. The dummy titles and logins
// are confidential (scSecret…): newScEnv checks that they appear in no log.

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

// scPulls adds n pull requests to the repository id, numbered from 1 to n, the n-th updated most
// recently (one minute apart), confidential title and author.
func (e *scEnv) scPulls(id int64, n int) {
	e.t.Helper()
	pulls := make([]githubtest.PullRequest, 0, n)
	for i := 1; i <= n; i++ {
		at := e.t0.Add(time.Duration(i) * time.Minute)
		pulls = append(pulls, githubtest.PullRequest{
			Number: i, Title: fmt.Sprintf("%s (PR %d)", scSecretMessage, i), Author: scSecretLogin,
			CreatedAt: at, UpdatedAt: at,
		})
	}
	e.srv.SetPullRequests(id, pulls...)
}

func TestPullRequestsConversion(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2001, "projet-pr"
	path := e.repo(id, name) + "/pulls"
	t0 := e.t0
	e.srv.SetPullRequests(id,
		githubtest.PullRequest{Number: 1, Title: "Ouverte", Author: scSecretLogin, Head: "feature/login",
			CreatedAt: t0, UpdatedAt: t0.Add(time.Hour)},
		githubtest.PullRequest{Number: 2, Title: "Mergée", Author: scSecretLogin, Closed: true, Base: "dev",
			CreatedAt: t0, UpdatedAt: t0.Add(3 * time.Hour), MergedAt: t0.Add(3 * time.Hour)},
		githubtest.PullRequest{Number: 3, Title: "Brouillon", Draft: true, CreatedAt: t0, UpdatedAt: t0.Add(2 * time.Hour)},
		githubtest.PullRequest{Number: 4, Title: "Fermée", Closed: true, CreatedAt: t0, UpdatedAt: t0.Add(4 * time.Hour),
			ClosedAt: t0.Add(4 * time.Hour)},
	)
	p := e.provider(testToken)
	got, err := p.PullRequests(t.Context(), scOwner, name, provider.PullRequestsOptions{})
	if err != nil {
		t.Fatalf("PullRequests: %v", err)
	}
	numbers := make([]int, len(got))
	for i, pr := range got {
		numbers[i] = pr.Number
	}
	if fmt.Sprint(numbers) != "[4 2 3 1]" {
		t.Errorf("order %v, want [4 2 3 1] (most recent update first)", numbers)
	}
	byNum := map[int]provider.PullRequest{}
	for _, pr := range got {
		byNum[pr.Number] = pr
	}
	open := byNum[1]
	if open.State != provider.PullRequestOpen || open.Merged || open.Draft || open.Title != "Ouverte" ||
		open.Author.Login != scSecretLogin || open.Author.ID == 0 || open.HeadBranch != "feature/login" ||
		open.BaseBranch != "main" || !open.CreatedAt.Equal(t0) || !open.UpdatedAt.Equal(t0.Add(time.Hour)) ||
		!open.ClosedAt.IsZero() || !open.MergedAt.IsZero() ||
		open.URL != "https://github.com/"+scOwner+"/"+name+"/pull/1" || open.CreatedAt.Location() != time.UTC {
		t.Errorf("open PR = %+v", open)
	}
	if m := byNum[2]; m.State != provider.PullRequestClosed || !m.Merged || !m.MergedAt.Equal(t0.Add(3*time.Hour)) ||
		m.BaseBranch != "dev" {
		t.Errorf("merged PR = %+v", m)
	}
	if d := byNum[3]; !d.Draft || d.Author != (provider.Account{}) {
		t.Errorf("draft PR without author = %+v", d)
	}
	if c := byNum[4]; c.State != provider.PullRequestClosed || c.Merged || c.ClosedAt.IsZero() {
		t.Errorf("closed PR = %+v", c)
	}
	reqs := e.srv.Requests()
	if len(reqs) != 1 || reqs[0].Path != path {
		t.Fatalf("requests %+v", reqs)
	}
	q := reqs[0].Query
	if q.Get("state") != "all" || q.Get("sort") != "updated" || q.Get("direction") != "desc" || q.Get("per_page") != "100" {
		t.Errorf("parameters %q, want state=all, sort=updated, direction=desc, per_page=100", reqs[0].RawQuery)
	}
}

func TestPullRequestsPaginationAndUpdatedSince(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2002, "projet-pr-pages"
	path := e.repo(id, name) + "/pulls"
	e.scPulls(id, 250)
	p := e.provider(testToken)
	ctx := t.Context()

	all, err := p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{})
	if err != nil || len(all) != 250 || all[0].Number != 250 || all[249].Number != 1 {
		t.Fatalf("PullRequests: %d PR, %v", len(all), err)
	}
	scCheckPages(t, e.srv.Requests(), path, 3)

	// Early stop: only the PRs updated since the 230th (included), a single page.
	e.srv.ResetRequests()
	since := e.t0.Add(230 * time.Minute)
	recent, err := p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{UpdatedSince: since.Add(300 * time.Millisecond)})
	if err != nil {
		t.Fatalf("PullRequests UpdatedSince: %v", err)
	}
	if len(recent) != 21 || recent[20].Number != 230 {
		t.Errorf("UpdatedSince: %d PR (last #%d), want 21 up to #230 (same second included)", len(recent), recent[len(recent)-1].Number)
	}
	scCheckPages(t, e.srv.Requests(), path, 1)
	// Nothing updated since: the first page is served as a 304 (free), no other.
	e.srv.ResetRequests()
	none, err := p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{UpdatedSince: e.t0.Add(time.Hour * 24)})
	if err != nil || none == nil || len(none) != 0 {
		t.Errorf("UpdatedSince in the future: %v, %v; want an empty non-nil list", none, err)
	}
	scAll304(t, e.srv.Requests(), 1)

	// Limit.
	e.srv.ResetRequests()
	limited, err := p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{Limit: 120})
	if err != nil || len(limited) != 120 {
		t.Errorf("Limit 120: %d PR, %v", len(limited), err)
	}
	scCheckPages(t, e.srv.Requests(), path, 2)
}

func TestPullRequestsCap(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2003, "projet-pr-plafond"
	path := e.repo(id, name) + "/pulls"
	e.scPulls(id, provider.MaxPullRequests+50)
	got, err := e.provider(testToken).PullRequests(t.Context(), scOwner, name, provider.PullRequestsOptions{Limit: 5000})
	if err != nil || len(got) != provider.MaxPullRequests {
		t.Fatalf("PullRequests: %d PR, %v; want %d", len(got), err, provider.MaxPullRequests)
	}
	scCheckPages(t, e.srv.Requests(), path, provider.MaxPullRequests/PerPage)
}

func TestReleasesAndTags(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2004, "projet-releases"
	base := e.repo(id, name)
	sha := e.commit(id, "v1", e.t0)
	e.srv.SetBranch(id, "main", sha)
	t0 := e.t0
	e.srv.SetReleases(id,
		githubtest.Release{ID: 11, TagName: "v1.0", Name: "Première", Author: scSecretLogin, CreatedAt: t0, PublishedAt: t0.Add(time.Minute)},
		githubtest.Release{ID: 12, TagName: "v1.1-rc", Prerelease: true, CreatedAt: t0.Add(time.Hour), PublishedAt: t0.Add(time.Hour)},
		githubtest.Release{ID: 13, TagName: "v2.0", Draft: true, CreatedAt: t0.Add(2 * time.Hour)},
	)
	e.srv.SetTags(id, githubtest.Tag{Name: "v1.0", SHA: sha}, githubtest.Tag{Name: "v1.1-rc", SHA: sha})
	p := e.provider(testToken)
	ctx := t.Context()

	releases, err := p.Releases(ctx, scOwner, name)
	if err != nil || len(releases) != 3 {
		t.Fatalf("Releases: %+v, %v", releases, err)
	}
	if r := releases[0]; r.ID != 13 || !r.Draft || !r.PublishedAt.IsZero() || r.Name != "" {
		t.Errorf("draft = %+v", r)
	}
	if r := releases[1]; r.ID != 12 || !r.Prerelease || r.TagName != "v1.1-rc" {
		t.Errorf("pre-release = %+v", r)
	}
	if r := releases[2]; r.ID != 11 || r.Name != "Première" || r.Author.Login != scSecretLogin ||
		!r.CreatedAt.Equal(t0) || !r.PublishedAt.Equal(t0.Add(time.Minute)) ||
		r.URL != "https://github.com/"+scOwner+"/"+name+"/releases/tag/v1.0" {
		t.Errorf("release = %+v", r)
	}
	tags, err := p.Tags(ctx, scOwner, name)
	if err != nil || len(tags) != 2 || tags[0] != (provider.Tag{Name: "v1.0", SHA: sha}) || tags[1].Name != "v1.1-rc" {
		t.Errorf("Tags: %+v, %v", tags, err)
	}

	// Second read: 304s, free, same data.
	remaining := e.srv.Quota().Remaining
	e.srv.ResetRequests()
	again, err := p.Releases(ctx, scOwner, name)
	if err != nil || len(again) != 3 {
		t.Errorf("Releases (cache): %v, %v", again, err)
	}
	if _, err := p.Tags(ctx, scOwner, name); err != nil {
		t.Errorf("Tags (cache): %v", err)
	}
	scAll304(t, e.srv.Requests(), 2)
	if got := e.srv.Quota().Remaining; got != remaining {
		t.Errorf("quota %d after 304s, want %d", got, remaining)
	}
	scCheckPages(t, e.srv.Requests(), base+"/releases", 1)
}

func TestReleasesAndTagsCap(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2005, "projet-tags"
	base := e.repo(id, name)
	tags := make([]githubtest.Tag, 0, provider.MaxTags+30)
	releases := make([]githubtest.Release, 0, provider.MaxReleases+30)
	for i := range provider.MaxTags + 30 {
		tags = append(tags, githubtest.Tag{Name: fmt.Sprintf("t%d", i), SHA: githubtest.FakeSHA(fmt.Sprint(i))})
		releases = append(releases, githubtest.Release{ID: int64(i + 1), TagName: fmt.Sprintf("t%d", i),
			CreatedAt: e.t0.Add(time.Duration(i) * time.Minute)})
	}
	e.srv.SetTags(id, tags...)
	e.srv.SetReleases(id, releases...)
	p := e.provider(testToken)
	gotTags, err := p.Tags(t.Context(), scOwner, name)
	if err != nil || len(gotTags) != provider.MaxTags || gotTags[0].Name != "t0" {
		t.Errorf("Tags: %d, %v", len(gotTags), err)
	}
	scCheckPages(t, e.srv.Requests(), base+"/tags", provider.MaxTags/PerPage)
	gotReleases, err := p.Releases(t.Context(), scOwner, name)
	if err != nil || len(gotReleases) != provider.MaxReleases || gotReleases[0].ID != int64(provider.MaxReleases+30) {
		t.Errorf("Releases: %d, %v", len(gotReleases), err)
	}
	scCheckPages(t, e.srv.Requests(), base+"/releases", provider.MaxReleases/PerPage)
}

func TestPullsReleasesTagsEmptyAndErrors(t *testing.T) {
	e := newScEnv(t)
	const id, name = 2006, "projet-vide"
	base := e.repo(id, name)
	p := e.provider(testToken)
	ctx := t.Context()

	// Empty repository: empty non-nil lists.
	pulls, err := p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{})
	if err != nil || pulls == nil || len(pulls) != 0 {
		t.Errorf("PullRequests of an empty repository: %v, %v", pulls, err)
	}
	releases, err := p.Releases(ctx, scOwner, name)
	if err != nil || releases == nil || len(releases) != 0 {
		t.Errorf("Releases of an empty repository: %v, %v", releases, err)
	}
	tags, err := p.Tags(ctx, scOwner, name)
	if err != nil || tags == nil || len(tags) != 0 {
		t.Errorf("Tags of an empty repository: %v, %v", tags, err)
	}

	// 409 (empty repository, if GitHub returned it): empty list too.
	e.srv.Inject(base+"/tags", githubtest.Response{Status: http.StatusConflict, Body: `{"message":"Git Repository is empty."}`, Times: 1})
	if tags, err := p.Tags(ctx, scOwner, name); err != nil || len(tags) != 0 {
		t.Errorf("Tags on 409: %v, %v", tags, err)
	}

	// Unusable response: ErrUnexpected.
	e.srv.Inject(base+"/pulls", githubtest.Response{Body: `[{"number":1,"state":"bizarre","updated_at":"2026-10-01T00:00:00Z"}]`, Times: 1})
	if _, err := p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{}); !errors.Is(err, provider.ErrUnexpected) {
		t.Errorf("PR with unknown state: %v, want ErrUnexpected", err)
	}
	e.srv.Inject(base+"/releases", githubtest.Response{Body: `[{"id":0,"tag_name":"v1"}]`, Times: 1})
	if _, err := p.Releases(ctx, scOwner, name); !errors.Is(err, provider.ErrUnexpected) {
		t.Errorf("release without identifier: %v, want ErrUnexpected", err)
	}
	e.srv.Inject(base+"/tags", githubtest.Response{Body: `[{"name":"v1","commit":{"sha":"abc"}}]`, Times: 1})
	if _, err := p.Tags(ctx, scOwner, name); !errors.Is(err, provider.ErrUnexpected) {
		t.Errorf("tag without full SHA: %v, want ErrUnexpected", err)
	}
	e.srv.Inject(base+"/releases", githubtest.Response{Status: http.StatusInternalServerError, Body: `{}`, Times: 1})
	_, err = p.Releases(ctx, scOwner, name)
	if !errors.Is(err, provider.ErrUnavailable) {
		t.Errorf("releases on 500: %v, want ErrUnavailable", err)
	}
	scAPIError(t, err, http.StatusInternalServerError, base+"/releases")

	// Inaccessible repository: ErrNotFound.
	e.srv.SetRepoAccess(id, githubtest.AccessHidden)
	_, err = p.PullRequests(ctx, scOwner, name, provider.PullRequestsOptions{})
	if !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("PR of a hidden repository: %v, want ErrNotFound", err)
	}
	scAPIError(t, err, http.StatusNotFound, base+"/pulls")

	// Invalid arguments: no request.
	e.srv.ResetRequests()
	if _, err := p.PullRequests(ctx, "", name, provider.PullRequestsOptions{}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("empty owner: %v", err)
	}
	if _, err := p.Releases(ctx, scOwner, "a/b"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid name: %v", err)
	}
	if _, err := p.Tags(ctx, scOwner, ""); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("empty name: %v", err)
	}
	if n := len(e.srv.Requests()); n != 0 {
		t.Errorf("%d requests for invalid arguments, want 0", n)
	}
}
