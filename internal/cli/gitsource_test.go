package cli

// Dépôts Git locaux (V2), de bout en bout par la CLI : sans gh, repo add, sync, status, log et
// repo remove (avec ou sans --keep-history) passent par de vrais clones Git (gittest, file://), avec
// copie de travail dans repos/ à la racine de l'espace.

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
	// Homonyme d'un autre étudiant, qui a commité un .watchoo/ (piège pour la recherche de l'espace).
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
		t.Errorf("repo add sans gh = %+v", added)
	}
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("repo add a cloné le dépôt (seule une vérification d'accès est attendue) : %v", err)
	}

	out := h.run("sync").mustSucceed(t)
	assertContains(t, "sync sans gh", out.stdout, "alice/projet", "Source : clones Git locaux")
	assertNotContains(t, "sync sans gh", out.stdout, "Quota GitHub")
	if info, err := os.Stat(clone); err != nil || !info.IsDir() {
		t.Fatalf("clone absent après la synchro : %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(clone, "README.md")); err != nil || string(got) != "a\nb\n" {
		t.Errorf("copie de travail : %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		if info, err := os.Stat(reposDir); err != nil || info.Mode().Perm() != 0o700 {
			t.Errorf("repos/ : %v, %v ; 0700 attendu", info, err)
		}
	}

	// Homonyme : rangé dans repos/<owner>-<name>. Lancée depuis son clone, une commande ignore le
	// .watchoo/ commité par l'étudiant et travaille sur l'espace qui contient les clones.
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	if _, err := os.Stat(filepath.Join(bobClone, ".watchoo", "config.yaml")); err != nil {
		t.Fatalf("clone de l'homonyme absent : %v", err)
	}
	h.cwd = bobClone
	assertContains(t, "status depuis un clone", h.run("status").mustSucceed(t).stdout, "licence-projet")
	h.cwd = h.root

	var sync syncReport
	decodeStrict(t, h.run("sync", "--json").mustSucceed(t).stdout, &sync)
	if sync.Source != wsync.SourceGit || sync.RateLimit != nil || sync.Summary.Success != 2 {
		t.Errorf("sync --json sans gh = %+v", sync)
	}

	logOut := h.run("log").mustSucceed(t)
	assertContains(t, "log", logOut.stdout, "Ajoute les tests", "Premier jet")

	st := h.run("status").mustSucceed(t)
	assertContains(t, "status sans gh", st.stdout, "Source : clones Git locaux", "gh auth login")
	var status map[string]any
	if err := json.Unmarshal([]byte(h.run("status", "--json").mustSucceed(t).stdout), &status); err != nil {
		t.Fatal(err)
	}
	if src, _ := status["source"].(map[string]any); src["kind"] != "git" {
		t.Errorf("status --json : source %v", status["source"])
	}

	// --keep-history : suivi arrêté, historique ET clone gardés.
	h.run("repo", "remove", "alice/projet", "--keep-history", "--yes").mustSucceed(t)
	if _, err := os.Stat(clone); err != nil {
		t.Errorf("clone supprimé malgré --keep-history : %v", err)
	}
	h.run("repo", "add", "alice/projet").mustSucceed(t) // reprise du suivi

	// Suppression : base et clone ; l'homonyme reprend repos/projet (clone déplacé).
	h.run("repo", "remove", "alice/projet", "--yes").mustSucceed(t)
	if _, err := os.Stat(filepath.Join(clone, "README.md")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clone toujours là après repo remove : %v", err)
	}
	if _, err := os.Stat(filepath.Join(clone, ".watchoo", "config.yaml")); err != nil {
		t.Errorf("clone de l'homonyme non déplacé vers repos/projet : %v", err)
	}
	if _, err := os.Stat(bobClone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("ancien emplacement de l'homonyme gardé : %v", err)
	}
	h.run("sync").mustSucceed(t)
	h.run("repo", "remove", "bob/projet", "--yes").mustSucceed(t)
	if _, err := os.Stat(clone); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("clone toujours là après repo remove : %v", err)
	}
	if _, err := os.Stat(reposDir); err != nil {
		t.Errorf("répertoire des clones supprimé : %v", err)
	}
}
