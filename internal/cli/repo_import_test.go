package cli

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// promoCSV is a dummy import file: three accessible repositories (one of them by URL), one
// inaccessible.
const promoCSV = `repo,label,branches
alice/projet,Alice Martin,
bob/projet,Bob Durand,"main,dev"
# dépôt pas encore partagé
carol/projet,Carol Leroy,
https://github.com/dave/projet.git,,
`

func TestRepoImport(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "promo.csv"), promoCSV)
	st := openTestStore(t, h)
	countRepos := func() int {
		t.Helper()
		repos, err := st.ListRepositories(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return len(repos)
	}

	// --dry-run: complete summary, nothing saved, non-zero code (carol inaccessible).
	r := h.run("repo", "import", "promo.csv", "--dry-run").mustCode(t, 1)
	want := "IMPORT  promo.csv  (dry run: nothing was saved)\n" +
		"  to add             3\n" +
		"  already tracked    0\n" +
		"  updated            0\n" +
		"  inaccessible       1   carol/projet\n" +
		"  lines in error     0\n" +
		"\n" +
		"  line 5  carol/projet: repository not found or account without access\n"
	if r.stdout != want {
		t.Errorf("import --dry-run =\n%q\nwant:\n%q", r.stdout, want)
	}
	assertContains(t, "import error", r.stderr, "error: import incomplete: 1 inaccessible repository (carol/projet)")
	if n := countRepos(); n != 0 {
		t.Errorf("--dry-run saved %d repositories", n)
	}
	if n := h.Provider.Calls(providertest.MethodRepository); n != 4 {
		t.Errorf("Repository called %d times, want once per repository (4)", n)
	}

	// Real import.
	r = h.run("repo", "import", "promo.csv").mustCode(t, 1)
	assertContains(t, "import", r.stdout, "IMPORT  promo.csv\n", "  added              3\n",
		"  inaccessible       1   carol/projet\n", "Run \"watchoo sync\"")
	if n := countRepos(); n != 3 {
		t.Errorf("%d repositories saved, want 3", n)
	}
	if bob := storedRepo(t, h, "bob", "projet"); bob.Label != "Bob Durand" ||
		!reflect.DeepEqual(bob.BranchPatterns, []string{"main", "dev"}) {
		t.Errorf("bob/projet = %+v", bob)
	}

	// Idempotent rerun, in JSON: already tracked; an empty cell keeps the existing label.
	writeFile(t, filepath.Join(h.cwd, "promo2.txt"), "# liste simple\n\nalice/projet\nbob/projet\n")
	r = h.run("repo", "import", "promo2.txt", "--json").mustSucceed(t)
	var rep importReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Format != "list" || rep.DryRun || rep.Summary != (importSummary{AlreadyTracked: 2}) || len(rep.Entries) != 2 ||
		rep.Entries[0].Line != 3 || rep.Entries[0].Outcome != wsync.OutcomeAlreadyTracked ||
		rep.Entries[0].Repository == nil || rep.Entries[0].Repository.Label != "Alice Martin" {
		t.Errorf("import --json = %+v", rep)
	}
	assertJSONKeys(t, r.stdout, "", "file", "format", "dry_run", "summary", "entries")
	assertJSONKeys(t, r.stdout, "summary", "added", "already_tracked", "updated", "inaccessible", "errors")
	assertJSONKeys(t, r.stdout, "entries.0", "line", "repo", "outcome", "repository", "error")

	// A new label updates.
	writeFile(t, filepath.Join(h.cwd, "promo3.csv"), "repo,label\nalice/projet,Alice M.\n")
	r = h.run("repo", "import", "promo3.csv").mustSucceed(t)
	assertContains(t, "import update", r.stdout, "  updated            1\n")
	if got := storedRepo(t, h, "alice", "projet").Label; got != "Alice M." {
		t.Errorf("label = %q, want \"Alice M.\"", got)
	}

	// In JSON with an inaccessible repository: summary on the standard output, non-zero code.
	r = h.run("repo", "import", "promo.csv", "--json").mustCode(t, 1)
	decodeStrict(t, r.stdout, &rep)
	if rep.Summary.Inaccessible != 1 || rep.Entries[2].Outcome != wsync.OutcomeInaccessible ||
		rep.Entries[2].Repository != nil || rep.Entries[2].Error == "" {
		t.Errorf("import --json inaccessible = %+v", rep)
	}

	// Standard input.
	h.Stdin = "dave/projet\n"
	r = h.run("repo", "import", "-").mustSucceed(t)
	assertContains(t, "import -", r.stdout, "IMPORT  -\n", "  already tracked    1\n")
}

func TestRepoImportInvalidFile(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "faux.csv"), "repo,label\nalice/projet,Alice\npas-un-depot,X\nalice/projet,Encore\n")
	r := h.run("repo", "import", "faux.csv").mustFail(t, "faux.csv: 2 lines with errors", "line 3", "line 4",
		"nothing was imported")
	if strings.Count(r.stderr, "error: ") != 1 {
		t.Errorf("error message:\n%s", r.stderr)
	}
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 || len(h.tokens) != 0 {
		t.Errorf("access checked despite an invalid file (%d calls)", n)
	}
	writeFile(t, filepath.Join(h.cwd, "vide.csv"), "# rien\n\n")
	h.run("repo", "import", "vide.csv").mustFail(t, "no repository in the file")
	h.run("repo", "import", "absent.csv").mustFail(t, "import file absent.csv not found")
	if repos, err := openTestStore(t, h).ListRepositories(context.Background()); err != nil || len(repos) != 0 {
		t.Errorf("saved repositories: %v, %v", repos, err)
	}
}
