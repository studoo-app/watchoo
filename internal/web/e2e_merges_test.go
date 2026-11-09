package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

// End to end, phase 8.5: a repository tracked by local clone (gh signed out) with a merge made by GitHub
// and a direct commit on the default branch; the repository section lists the commit outside a pull
// request (short SHA and author of the commit), the contributor page gives it to its author and the
// milestone review judges PR3 on the interval of the milestone. Neither a commit message nor a SHA
// reaches the logs.
func TestWebEndToEndConventionsArrivals(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	alice := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	github := gittest.Author{Name: "GitHub", Email: "noreply@github.com"}
	at := func(h int) time.Time { return testNow.Add(-time.Duration(h) * time.Hour) }
	r.Commit("main", "a.txt", "1\n", "feat: initialise le projet", alice, at(60))
	r.Branch("feat/menu", "main")
	r.Commit("feat/menu", "b.txt", "1\n", "feat: ajoute le menu secret", alice, at(55))
	merge := r.Merge("main", "feat/menu", "Merge pull request #1 from alice/feat/menu", github, at(50))
	r.Tag("v1.0.0", "main", "")
	direct := r.Commit("main", "a.txt", "2\n", "fix: correction directe secrete", alice, at(40))

	e := newTestEnv(t, withConventions(), withGitURL(srv.URL), func(e *testEnv) {
		e.auth = authtest.NewLoggedOut()
		e.cfg.Conventions.Since = "2020-01-01"
	})
	w := e.post("/settings/repos", url.Values{"repo": {"alice/projet"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	id, err := strconv.ParseInt(loc.Query().Get("repo"), 10, 64)
	if err != nil {
		t.Fatalf("repository identifier %q", loc.Query().Get("repo"))
	}
	assertStatus(t, e.post("/sync", nil, true), http.StatusOK)
	if st := e.waitSyncIdle(); !st.HasReport {
		t.Fatalf("sync report = %+v", st.LastReport)
	}

	section := conventionsSection(t, e.get(repoPath(id)).Body.String())
	for _, want := range []string{`id="repo-conventions-merges"`,
		"Commits that reached the default branch outside a pull request: 1 of 2 (by pull request 1, undetermined 0).",
		"Clone only: read on the messages of GitHub", shortSHA(direct), "Alice Martin", "direct commit on the branch",
		"Allowed merge methods:", "unknown (no data from the GitHub API)"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	if strings.Contains(section, shortSHA(merge)) {
		t.Error("commit merged by a pull request listed outside")
	}

	w = e.get(contributorPath(e.contributorIDs()["Alice Martin"]))
	assertContains(t, w, `id="contributor-conventions-outside-pr"`,
		"Commits of the default branch they wrote that arrived outside a pull request: 1 of 1", shortSHA(direct))

	w = e.get("/conventions?milestone=v2.0.0")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "1 outside PR / 2", "no commit outside a pull request")
	w = e.get("/conventions?milestone=v1.0.0")
	assertContains(t, w, "PR3: 0 outside a pull request of 1 commits (0 undetermined)")

	for _, leak := range []string{"ajoute le menu secret", "correction directe secrete", direct, merge} {
		if strings.Contains(e.logs.String(), leak) {
			t.Errorf("%q in the logs", leak)
		}
	}
}
