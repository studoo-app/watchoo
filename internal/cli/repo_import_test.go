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

// promoCSV est un fichier d'import factice : trois dépôts accessibles (dont un par URL), un
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

	// --dry-run : bilan complet, rien d'enregistré, code non nul (carol inaccessible).
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
		t.Errorf("import --dry-run =\n%q\nattendu :\n%q", r.stdout, want)
	}
	assertContains(t, "erreur de l'import", r.stderr, "error: import incomplete: 1 inaccessible repository (carol/projet)")
	if n := countRepos(); n != 0 {
		t.Errorf("--dry-run a enregistré %d dépôts", n)
	}
	if n := h.Provider.Calls(providertest.MethodRepository); n != 4 {
		t.Errorf("Repository appelé %d fois, attendu une fois par dépôt (4)", n)
	}

	// Import réel.
	r = h.run("repo", "import", "promo.csv").mustCode(t, 1)
	assertContains(t, "import", r.stdout, "IMPORT  promo.csv\n", "  added              3\n",
		"  inaccessible       1   carol/projet\n", "Run \"watchoo sync\"")
	if n := countRepos(); n != 3 {
		t.Errorf("%d dépôts enregistrés, attendu 3", n)
	}
	if bob := storedRepo(t, h, "bob", "projet"); bob.Label != "Bob Durand" ||
		!reflect.DeepEqual(bob.BranchPatterns, []string{"main", "dev"}) {
		t.Errorf("bob/projet = %+v", bob)
	}

	// Relance idempotente, en JSON : déjà suivis ; une cellule vide garde le label existant.
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

	// Un nouveau label met à jour.
	writeFile(t, filepath.Join(h.cwd, "promo3.csv"), "repo,label\nalice/projet,Alice M.\n")
	r = h.run("repo", "import", "promo3.csv").mustSucceed(t)
	assertContains(t, "import mise à jour", r.stdout, "  updated            1\n")
	if got := storedRepo(t, h, "alice", "projet").Label; got != "Alice M." {
		t.Errorf("label = %q, attendu « Alice M. »", got)
	}

	// En JSON avec un dépôt inaccessible : bilan sur la sortie standard, code non nul.
	r = h.run("repo", "import", "promo.csv", "--json").mustCode(t, 1)
	decodeStrict(t, r.stdout, &rep)
	if rep.Summary.Inaccessible != 1 || rep.Entries[2].Outcome != wsync.OutcomeInaccessible ||
		rep.Entries[2].Repository != nil || rep.Entries[2].Error == "" {
		t.Errorf("import --json inaccessible = %+v", rep)
	}

	// Entrée standard.
	h.Stdin = "dave/projet\n"
	r = h.run("repo", "import", "-").mustSucceed(t)
	assertContains(t, "import -", r.stdout, "IMPORT  -\n", "  already tracked    1\n")
}

func TestRepoImportInvalidFile(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "faux.csv"), "repo,label\nalice/projet,Alice\npas-un-depot,X\nalice/projet,Encore\n")
	r := h.run("repo", "import", "faux.csv").mustFail(t, "faux.csv: 2 lignes en erreur", "ligne 3", "ligne 4",
		"nothing was imported")
	if strings.Count(r.stderr, "error: ") != 1 {
		t.Errorf("message d'erreur :\n%s", r.stderr)
	}
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 || len(h.tokens) != 0 {
		t.Errorf("accès vérifié malgré un fichier invalide (%d appels)", n)
	}
	writeFile(t, filepath.Join(h.cwd, "vide.csv"), "# rien\n\n")
	h.run("repo", "import", "vide.csv").mustFail(t, "aucun dépôt dans le fichier")
	h.run("repo", "import", "absent.csv").mustFail(t, "import file absent.csv not found")
	if repos, err := openTestStore(t, h).ListRepositories(context.Background()); err != nil || len(repos) != 0 {
		t.Errorf("dépôts enregistrés : %v, %v", repos, err)
	}
}
