package web

// Pull requests, releases and tags on a repository page ("repo-pulls" fragment, V2) and number of
// open PRs on the dashboard.

import (
	"html"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// xssTitle is a booby-trapped PR title: it must be escaped.
const xssTitle = `<img src=x onerror="alert(1)">PR piégée`

// seedPulls saves PRs, a release and two tags for repository repoID (promo/name).
func (e *testEnv) seedPulls(repoID int64, name, tagSHA string) {
	e.t.Helper()
	ctx := e.t.Context()
	run, err := e.store.BeginSyncRun(ctx, repoID, e.now.Add(-30*time.Minute), wsync.LockStaleAfter)
	if err != nil {
		e.t.Fatal(err)
	}
	at := e.now.Add(-2 * time.Hour)
	pull := func(n int, state string, merged, draft bool, title string) model.PullRequest {
		p := model.PullRequest{Number: n, Title: title, State: state, Merged: merged, Draft: draft,
			AuthorLogin: "alice-gh", AuthorProviderID: 10, URL: "https://github.com/promo/" + name + "/pull/" + string(rune('0'+n)),
			BaseBranch: "main", HeadBranch: "feature-x", CreatedAt: at, UpdatedAt: at.Add(time.Duration(n) * time.Minute)}
		if state == model.PullRequestClosed {
			p.ClosedAt = p.UpdatedAt
		}
		if merged {
			p.MergedAt = p.UpdatedAt
		}
		return p
	}
	if _, err := e.store.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repoID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: name, DefaultBranch: "main",
			URL: "https://github.com/promo/" + name},
		Branches: []store.BranchState{{Name: "main", HeadSHA: tagSHA}},
		PullRequests: []model.PullRequest{
			pull(1, model.PullRequestOpen, false, false, xssTitle),
			pull(2, model.PullRequestOpen, false, true, "Brouillon de PR"),
			pull(3, model.PullRequestClosed, true, false, "PR mergée"),
			pull(4, model.PullRequestClosed, false, false, "PR abandonnée"),
		},
		ReplaceReleases: true,
		Releases: []model.Release{{ProviderID: 7, TagName: "v1.0", Name: "Première version", Prerelease: true,
			URL: "https://github.com/promo/" + name + "/releases/tag/v1.0", CreatedAt: at, PublishedAt: at}},
		ReplaceTags: true,
		Tags:        []model.Tag{{Name: "v1.0", SHA: tagSHA}, {Name: "v0.9", SHA: strings.Repeat("e", 40)}},
		SyncedAt:    e.now.Add(-29 * time.Minute),
	}); err != nil {
		e.t.Fatalf("ApplyRepoSync: %v", err)
	}
}

func TestRepoPulls(t *testing.T) {
	e := newTestEnv(t)
	ids := e.seed()
	aliceHead := e.commitWrite(3, store.Identity{}, 0, "", "main").SHA
	e.seedPulls(ids.Alice, "alice", aliceHead)
	path := repoPath(ids.Alice)

	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		`id="repo-pulls"`, `hx-trigger="sync-finished from:body"`,
		"2 open · 1 merged · 1 closed without merge",
		html.EscapeString(xssTitle), `href="https://github.com/promo/alice/pull/1"`,
		"Brouillon de PR", ">draft<", "Latest merged", "PR mergée",
		"<code>feature-x</code> → <code>main</code>",
		"Releases and tags", `href="https://github.com/promo/alice/releases/tag/v1.0"`, "Première version", "pre-release",
		">v1.0<", ">v0.9<", `title="View the tag&#39;s commit on GitHub"`, shortSHA(aliceHead),
	)
	assertNotContains(t, w, xssTitle, "PR abandonnée")
	if strings.Index(w.Body.String(), "Brouillon de PR") > strings.Index(w.Body.String(), html.EscapeString(xssTitle)) {
		t.Error("open PRs: the most recently updated first")
	}

	// Fragment alone (refresh after a sync).
	frag := e.htmxWith(path, map[string]string{"HX-Target": repoPullsID})
	assertStatus(t, frag, http.StatusOK)
	body := frag.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="repo-pulls"`) || strings.Contains(body, `id="repo-commits"`) ||
		strings.Contains(body, "<html") {
		t.Errorf("repo-pulls fragment:\n%s", body)
	}

	// Repository without PR or release: empty states.
	w = e.get(repoPath(ids.Bob))
	assertContains(t, w, "No pull request in the database for this repository.", "No release and no tag in the database for this repository.",
		"0 open · 0 merged · 0 closed without merge")

	// Dashboard: number of open PRs on the card of the repository.
	w = e.get("/")
	assertContains(t, w, "2 open PRs")
	assertNotContains(t, w, "0 open PR")
}

// Repository tracked only by local Git clone (forge identifier unknown, V2): stars, issues, pull
// requests and releases are hidden; the tags remain.
func TestRepoWithoutForgeDetails(t *testing.T) {
	e := newTestEnv(t)
	repo, err := e.store.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub,
		Owner: "promo", Name: "local", URL: "https://github.com/promo/local", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	id := repo.ID
	w := e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Stars and issues not available", "tracked by local Git clone",
		"Releases not available through a local Git clone")
	assertNotContains(t, w, "0 star", "No pull request in the database", "0 open · 0 merged")
}
