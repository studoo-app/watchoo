package cli

// "watchoo languages" and the share of the contributors (1.1.0), end to end without gh (real clones).

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

func TestLanguagesAndShare(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	alice := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	bob := gittest.Author{Name: "Bob Durand", Email: "bob@example.test"}
	at := testNow.Add(-2 * time.Hour)
	r.Commit("main", "main.go", strings.Repeat("g", 3000), "Go", alice, at)
	r.Commit("main", "index.html", strings.Repeat("h", 1000), "HTML", alice, at)
	r.Commit("main", "style.css", strings.Repeat("c", 1000), "CSS", alice, at)
	r.Commit("main", "README.md", "doc\n", "Doc", bob, at)

	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	h.run("repo", "add", "alice/projet").mustSucceed(t)

	// Before the first sync: nothing, with a hint.
	assertContains(t, "languages before a sync", h.run("languages").mustSucceed(t).stdout, "No language in the database")

	h.run("sync").mustSucceed(t)
	out := h.run("languages", "--repo", "alice/projet").mustSucceed(t).stdout
	assertContains(t, "languages", out, "LANGUAGE", "Go", "60.0%", "HTML", "20.0%", "CSS", "2 KB")
	assertNotContains(t, "languages", out, "Markdown")

	jsonOut := h.run("languages", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, jsonOut, "", "languages")
	assertJSONKeys(t, jsonOut, "languages.0", "repository", "name", "bytes", "percent")
	assertContains(t, "languages --json", jsonOut, `"percent": 60`, `"bytes": 3000`)

	contributors := h.run("contributors").mustSucceed(t).stdout
	assertContains(t, "contributors", contributors, "SHARE", "75.0%", "25.0%")
	assertContains(t, "contributors --json", h.run("contributors", "--json").mustSucceed(t).stdout,
		`"share": 75`, `"share": 25`)

	restore := i18n.SetProcessForTest(i18n.French)
	defer restore()
	assertContains(t, "languages in French", h.run("languages").mustSucceed(t).stdout, "LANGAGE", "60,0 %", "2 Ko")
}

func TestByteSize(t *testing.T) {
	for n, want := range map[int64]string{1: "1 B", 1023: "1023 B", 1024: "1 KB", 5 << 20: "5 MB"} {
		if got := byteSize(n); got != want {
			t.Errorf("byteSize(%d) = %q, want %q", n, got, want)
		}
	}
}

// Languages of the contributors (1.2.0): LANGUAGE column of "contributors" (main language, "-" without
// any line in a recognized language) and "languages" in JSON, read from the clone at the first sync.
func TestContributorsLanguages(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	alice := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	bob := gittest.Author{Name: "Bob Durand", Email: "bob@example.test"}
	at := testNow.Add(-2 * time.Hour)
	r.Commit("main", "main.go", "package main\n\nfunc main() {}\n", "Go", alice, at)
	r.Commit("main", "index.php", "<?php\n", "PHP", alice, at)
	r.Commit("main", "README.md", "doc\n", "Doc", bob, at)

	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)

	out := h.run("contributors").mustSucceed(t).stdout
	assertContains(t, "contributors", out, "LANGUAGE")
	if row := tableRow(t, out, "Alice Martin"); !slices.Contains(row, "Go") {
		t.Errorf("Alice Martin row = %q", row)
	}
	if row := tableRow(t, out, "Bob Durand"); slices.Contains(row, "Go") || len(row) != 10 {
		t.Errorf("Bob Durand row = %q", row)
	}
	assertContains(t, "contributors --json", h.run("contributors", "--json").mustSucceed(t).stdout,
		`"languages_scanned_commits": 2`, `"name": "Go"`, `"lines": 3`, `"percent": 75`, `"name": "PHP"`,
		`"languages": []`)

	restore := i18n.SetProcessForTest(i18n.French)
	defer restore()
	assertContains(t, "contributors in French", h.run("contributors").mustSucceed(t).stdout, "LANGAGE")
}
