package cli

// Local Git repositories (V2), end to end through the CLI: without gh, repo add, sync, status, log and
// repo remove (with or without --keep-history) go through real Git clones (gittest, file://), with a
// working copy in repos/ at the workspace root.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestGitSourceEndToEnd(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	who := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	r.Commit("main", "README.md", "a\n", "Premier jet", who, testNow.Add(-50*time.Hour))
	r.Commit("main", "README.md", "a\nb\n", "Ajoute les tests", who, testNow.Add(-2*time.Hour))
	// Namesake of another student, who committed a .watchoo/ (a trap for the workspace lookup).
	rb := srv.Create("bob", "projet", "main")
	rb.Commit("main", ".watchoo/config.yaml", "name: piege\n", "Ajoute un espace", who, testNow.Add(-3*time.Hour))

	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	reposDir := filepath.Join(h.cwd, "repos")
	clone := filepath.Join(reposDir, "projet")
	bobClone := filepath.Join(reposDir, "bob-projet")

	r1 := h.run("repo", "add", "alice/projet", "--json").mustSucceed(t)
	var added repoAddReport
	decodeStrict(t, r1.stdout, &added)
	if added.Outcome != wsync.OutcomeAdded || added.Repository.URL != "https://github.com/alice/projet" {
		t.Errorf("repo add without gh = %+v", added)
	}
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("repo add cloned the repository (only an access check is expected): %v", err)
	}

	out := h.run("sync").mustSucceed(t)
	assertContains(t, "sync without gh", out.stdout, "alice/projet", "Source: local Git clones")
	assertNotContains(t, "sync without gh", out.stdout, "GitHub quota")
	if info, err := os.Stat(clone); err != nil || !info.IsDir() {
		t.Fatalf("clone missing after the sync: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(clone, "README.md")); err != nil || string(got) != "a\nb\n" {
		t.Errorf("working copy: %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(reposDir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("repos/: %v, %v; want 0700", info, err)
		}
	}

	// Namesake: stored in repos/<owner>-<name>. Run from its clone, a command ignores the .watchoo/
	// committed by the student and works on the workspace that contains the clones.
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	if _, err := os.Stat(filepath.Join(bobClone, ".watchoo", "config.yaml")); err != nil {
		t.Fatalf("namesake clone missing: %v", err)
	}
	h.cwd = bobClone
	assertContains(t, "status from a clone", h.run("status").mustSucceed(t).stdout, "licence-projet")
	h.cwd = h.root

	var sync syncReport
	decodeStrict(t, h.run("sync", "--json").mustSucceed(t).stdout, &sync)
	if sync.Source != wsync.SourceGit || sync.RateLimit != nil || sync.Summary.Success != 2 {
		t.Errorf("sync --json without gh = %+v", sync)
	}

	logOut := h.run("log").mustSucceed(t)
	assertContains(t, "log", logOut.stdout, "Ajoute les tests", "Premier jet")

	st := h.run("status").mustSucceed(t)
	assertContains(t, "status without gh", st.stdout, "Source: local Git clones", "gh auth login")
	var status map[string]any
	if err := json.Unmarshal([]byte(h.run("status", "--json").mustSucceed(t).stdout), &status); err != nil {
		t.Fatal(err)
	}
	if src, _ := status["source"].(map[string]any); src["kind"] != "git" {
		t.Errorf("status --json: source %v", status["source"])
	}

	// --keep-history: tracking stopped, history AND clone kept.
	h.run("repo", "remove", "alice/projet", "--keep-history", "--yes").mustSucceed(t)
	if _, err := os.Stat(clone); err != nil {
		t.Errorf("clone deleted despite --keep-history: %v", err)
	}
	h.run("repo", "add", "alice/projet").mustSucceed(t) // tracking resumed

	// Removal: database and clone; the namesake takes over repos/projet (clone moved).
	h.run("repo", "remove", "alice/projet", "--yes").mustSucceed(t)
	if _, err := os.Stat(filepath.Join(clone, "README.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clone still there after repo remove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(clone, ".watchoo", "config.yaml")); err != nil {
		t.Errorf("namesake clone not moved to repos/projet: %v", err)
	}
	if _, err := os.Stat(bobClone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("old location of the namesake kept: %v", err)
	}
	h.run("sync").mustSucceed(t)
	h.run("repo", "remove", "bob/projet", "--yes").mustSucceed(t)
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clone still there after repo remove: %v", err)
	}
	if _, err := os.Stat(reposDir); err != nil {
		t.Errorf("clones directory deleted: %v", err)
	}
}
