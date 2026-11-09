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

// End to end, phase 8.3: a repository tracked by local clone (gh signed out) with a CHANGELOG, a README and
// a documentation; the sync computes the file criteria on the clone, the repository section shows the
// grids and, folded, the paths recomputed from the clone; the milestone review reads the tree of the tag.
// Neither a path nor a tag name reaches the logs.
func TestWebEndToEndConventionsFiles(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	who := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	at := func(h int) time.Time { return testNow.Add(-time.Duration(h) * time.Hour) }
	r.Commit("main", "CHANGELOG.md", "# Changelog\n\nThe format is based on [Keep a Changelog]"+
		"(https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning]"+
		"(https://semver.org/spec/v2.0.0.html).\n\n## [Unreleased]\n\n## [1.0.0] - 2026-10-01\n\n### Added\n\n"+
		"- Première version.\n", "docs: ajoute le changelog", who, at(60))
	r.Commit("main", "README.md", "# Projet\n\n## Installation\n\n`make`\n\n[doc](documentation/INDEX.md) [log](CHANGELOG.md)\n",
		"docs: ajoute le readme", who, at(59))
	r.Commit("main", "documentation/INDEX.md", "# Index\n\n| Document | Thème | Version | Dernière mise à jour | Responsable |\n"+
		"|---|---|---|---|---|\n| [Vision](01-cadrage/vision.md) | Cadrage | 1.0 | 2026-10-01 | Personne Secrète |\n",
		"docs: ajoute l'index", who, at(58))
	r.Commit("main", "documentation/01-cadrage/vision.md", "# Vision\n", "docs: ajoute la vision", who, at(57))
	r.Tag("v1.0.0", "main", "")
	r.Commit("main", "documentation/03-annexes/e2e-oubli.md", "# Oubli\n\n[mort](e2e-absent.md)\n", "docs: ajoute une annexe", who, at(50))

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

	w = e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	section := conventionsSection(t, w.Body.String())
	for _, want := range []string{"CHANGELOG · 8/8 criteria conform, 0 unknown", "Measured by the sync on the commit ",
		"Paths to look at", `id="repo-conventions-list-unreferenced"`, "Documents not referenced by the INDEX (DOC3): 1",
		"documentation/03-annexes/e2e-oubli.md", "Dead links (DOC4): 1", "e2e-oubli.md → e2e-absent.md"} {
		if !strings.Contains(section, want) {
			t.Errorf("conventions section without %q", want)
		}
	}
	// The cells of the INDEX (names of persons) are never shown.
	if strings.Contains(section, "Personne Secrète") {
		t.Error("cell of the INDEX in the conventions section")
	}

	w = e.get("/conventions?milestone=v1.0.0")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "CHANGELOG 8/8", "CHANGELOG section of the milestone")
	// Overview: no path.
	assertNotContains(t, w, "e2e-oubli.md", "Personne Secrète")

	for _, leak := range []string{"e2e-oubli.md", "e2e-absent.md", "CHANGELOG.md", "v1.0.0", "Personne Secrète"} {
		if strings.Contains(e.logs.String(), leak) {
			t.Errorf("%q in the logs", leak)
		}
	}
}
