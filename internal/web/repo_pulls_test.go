package web

// Pull requests, releases et tags sur la page d'un dépôt (fragment « repo-pulls », V2) et nombre de
// PR ouvertes sur le tableau de bord.

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

// xssTitle est un titre de PR piégé : il doit être échappé.
const xssTitle = `<img src=x onerror="alert(1)">PR piégée`

// seedPulls enregistre des PR, une release et deux tags pour le dépôt repoID (promo/name).
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
			AuthorLogin: "alice-gh", URL: "https://github.com/promo/" + name + "/pull/" + string(rune('0'+n)),
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
		e.t.Fatalf("ApplyRepoSync : %v", err)
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
		"2 ouvertes · 1 mergée · 1 fermée sans merge",
		html.EscapeString(xssTitle), `href="https://github.com/promo/alice/pull/1"`,
		"Brouillon de PR", ">brouillon<", "Dernières mergées", "PR mergée",
		"<code>feature-x</code> → <code>main</code>",
		"Releases et tags", `href="https://github.com/promo/alice/releases/tag/v1.0"`, "Première version", "pré-release",
		">v1.0<", ">v0.9<", `title="Voir le commit du tag sur GitHub"`, shortSHA(aliceHead),
	)
	assertNotContains(t, w, xssTitle, "PR abandonnée")
	if strings.Index(w.Body.String(), "Brouillon de PR") > strings.Index(w.Body.String(), html.EscapeString(xssTitle)) {
		t.Error("PR ouvertes : la plus récemment modifiée d'abord")
	}

	// Fragment seul (rafraîchissement après une synchro).
	frag := e.htmxWith(path, map[string]string{"HX-Target": repoPullsID})
	assertStatus(t, frag, http.StatusOK)
	body := frag.Body.String()
	if !strings.HasPrefix(strings.TrimSpace(body), `<div id="repo-pulls"`) || strings.Contains(body, `id="repo-commits"`) ||
		strings.Contains(body, "<html") {
		t.Errorf("fragment repo-pulls :\n%s", body)
	}

	// Dépôt sans PR ni release : états vides.
	w = e.get(repoPath(ids.Bob))
	assertContains(t, w, "Aucune pull request en base pour ce dépôt.", "Aucune release ni aucun tag en base pour ce dépôt.",
		"0 ouverte · 0 mergée · 0 fermée sans merge")

	// Tableau de bord : nombre de PR ouvertes sur la carte du dépôt.
	w = e.get("/")
	assertContains(t, w, "2 PR ouvertes")
	assertNotContains(t, w, "0 PR ouverte")
}
