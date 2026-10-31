package cli

import (
	"testing"
)

// Toutes les commandes de la phase 3 ont besoin d'un espace : hors d'un espace, un message clair dit
// de lancer « watchoo init » ou d'utiliser -C, sans rien créer ni appeler gh.
func TestPhase3CommandsOutsideWorkspace(t *testing.T) {
	h := newHarness(t)
	h.Terminal = func() TermInfo { return TermInfo{IsTerminal: true, Width: 100, Unicode: true} }
	h.StdinTerminal = true
	file := h.cwd + "/liste.txt"
	writeFile(t, file, "alice/projet\n")
	for _, args := range [][]string{
		{"repo", "list"},
		{"repo", "add", "alice/projet"},
		{"repo", "remove", "alice/projet", "--yes"},
		{"repo", "import", "liste.txt"},
		{"sync"},
		{"sync", "alice/projet"},
		{"log"},
		{"contributors"},
		{"status"},
		{"tui"},
		{"repo", "list", "--json"},
	} {
		h.run(args...).mustFail(t, "no Watchoo workspace here", "-C <directory>")
	}
	assertNoWorkspace(t, h.cwd)
	if len(h.configs) != 0 || len(h.tokens) != 0 {
		t.Errorf("gh ou le provider ont été appelés hors d'un espace : %d, %d", len(h.configs), len(h.tokens))
	}
}

// Option -C : les commandes travaillent sur l'espace désigné.
func TestPhase3WorkspaceFlag(t *testing.T) {
	h := newSyncedWorkspace(t)
	root := h.cwd
	h.cwd = newDir(t, t.TempDir(), "ailleurs")
	h.run("repo", "list").mustFail(t, "no Watchoo workspace here")
	r := h.run("-C", root, "repo", "list").mustSucceed(t)
	assertContains(t, "repo list -C", r.stdout, "Alice Martin", "bob/projet")
}

// Arguments en trop ou manquants : erreur qui rappelle l'usage (validateurs d'arguments de repo.go,
// sync.go et repo_import.go).
func TestPhase3Arguments(t *testing.T) {
	h := newWorkspace(t)
	h.run("repo", "add").mustFail(t, `"watchoo repo add" expects 1 argument (got: 0)`, "watchoo repo add owner/repo")
	h.run("repo", "remove", "a/b", "c/d").mustFail(t, "expects 1 argument (got: 2)")
	h.run("repo", "import").mustFail(t, `"watchoo repo import" expects 1 argument`)
	h.run("sync", "a/b", "c/d").mustFail(t, `"watchoo sync" expects at most 1 argument (got: a/b c/d)`)
	h.run("log", "extra").mustFail(t, `"watchoo log" takes no argument`)
	h.run("repo", "inconnue").mustFail(t, `unknown command "inconnue"`)
	r := h.run("repo").mustSucceed(t)
	assertContains(t, "aide de repo", r.stdout, "add", "import", "list", "remove")
}
