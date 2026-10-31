package cli

// Données factices des tests de la phase 3 : un espace initialisé, un faux provider (providertest) à
// horloge fixe et quelques dépôts d'étudiants fictifs.

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// testNow est l'heure fixe des tests (horloge de la CLI, du moteur et du faux provider).
var testNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// Identifiants GitHub des dépôts factices.
const (
	aliceRepoID = 101 // alice/projet : actif, branches main et dev
	bobRepoID   = 102 // bob/projet : Flemme (dernier commit 9 d agoours)
	daveRepoID  = 104 // dave/projet : Dead (dernier commit 42 d agoours)
)

// Personnes factices.
var (
	alicePerson = provider.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh", ID: 2001,
		AvatarURL: "https://avatars.example.test/u/2001"}
	bobPerson  = provider.Person{Name: "Bob Durand", Email: "Bob@Example.test"} // sans compte GitHub
	davePerson = provider.Person{Name: "Dave Petit", Email: "dave@example.test", Login: "dave-gh", ID: 2004}
)

// newWorkspace renvoie un harness dont le répertoire courant est un espace initialisé (fuseau UTC),
// avec l'horloge testNow et un faux provider vide à la même horloge.
func newWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	t.Setenv("WATCHOO_TIMEZONE", "UTC")
	h.run("init").mustSucceed(t)
	h.Now = func() time.Time { return testNow }
	f := providertest.New()
	f.SetClock(h.Now)
	h.Provider = f
	return h
}

// fakeCommit décrit un commit factice.
type fakeCommit struct {
	seed   string // graine du SHA (providertest.SHA)
	author provider.Person
	ago    time.Duration // ancienneté par rapport à testNow
	msg    string
}

// addCommits ajoute au dépôt repoID une suite linéaire de commits au-dessus de parent (vide = racine),
// place branch sur le dernier et renvoie son SHA.
func addCommits(f *providertest.Fake, repoID int64, branch, parent string, commits ...fakeCommit) string {
	for _, c := range commits {
		sha := providertest.SHA(c.seed)
		at := testNow.Add(-c.ago)
		pc := provider.Commit{
			SHA: sha, Message: c.msg, URL: "https://github.example.test/commit/" + sha,
			Author: c.author, Committer: c.author, AuthoredAt: at, CommittedAt: at,
		}
		if parent != "" {
			pc.Parents = []string{parent}
		}
		f.AddCommits(repoID, pc)
		parent = sha
	}
	f.SetBranch(repoID, branch, parent)
	return parent
}

// seedProvider remplit le faux provider : alice/projet (main : 2 commits d'Alice, dev : 1 commit de
// Bob 1 h ago ; PR n° 1 ouverte et n° 2 mergée, release v1.0, tags v1.0 et v0.1), bob/projet (1 commit de Bob 9 d agoours), dave/projet (1 commit de Dave il y a
// 42 jours). carol/projet n'existe pas (dépôt inaccessible).
func seedProvider(f *providertest.Fake) {
	f.AddRepository(provider.Repository{ID: aliceRepoID, Owner: "alice", Name: "projet", Private: true,
		URL: "https://github.example.test/alice/projet"})
	head := addCommits(f, aliceRepoID, "main", "",
		fakeCommit{"a1", alicePerson, 50 * time.Hour, "Premier jet du projet"},
		fakeCommit{"a2", alicePerson, 2 * time.Hour, "Ajoute les tests\n\nDétail du commit"})
	addCommits(f, aliceRepoID, "dev", head, fakeCommit{"a3", bobPerson, time.Hour, "Essai sur dev"})
	// Pull requests, release et tags d'alice/projet (V2).
	f.SetPullRequests(aliceRepoID,
		provider.PullRequest{Number: 1, Title: "Ajoute la page de connexion", State: provider.PullRequestOpen,
			Author: provider.Account{Login: "alice-gh", ID: 2001}, URL: "https://github.example.test/alice/projet/pull/1",
			BaseBranch: "main", HeadBranch: "login", CreatedAt: testNow.Add(-5 * time.Hour), UpdatedAt: testNow.Add(-3 * time.Hour)},
		provider.PullRequest{Number: 2, Title: "Corrige le build", State: provider.PullRequestClosed, Merged: true,
			Author: provider.Account{Login: "alice-gh", ID: 2001}, URL: "https://github.example.test/alice/projet/pull/2",
			BaseBranch: "main", HeadBranch: "fix", CreatedAt: testNow.Add(-30 * time.Hour),
			UpdatedAt: testNow.Add(-26 * time.Hour), ClosedAt: testNow.Add(-26 * time.Hour), MergedAt: testNow.Add(-26 * time.Hour)},
	)
	f.SetReleases(aliceRepoID, provider.Release{ID: 501, TagName: "v1.0", Name: "Première version",
		Author: provider.Account{Login: "alice-gh"}, URL: "https://github.example.test/alice/projet/releases/tag/v1.0",
		CreatedAt: testNow.Add(-2 * time.Hour), PublishedAt: testNow.Add(-2 * time.Hour)})
	f.SetTags(aliceRepoID, provider.Tag{Name: "v1.0", SHA: head}, provider.Tag{Name: "v0.1", SHA: providertest.SHA("inconnu")})

	f.AddRepository(provider.Repository{ID: bobRepoID, Owner: "bob", Name: "projet",
		URL: "https://github.example.test/bob/projet"})
	addCommits(f, bobRepoID, "main", "", fakeCommit{"b1", bobPerson, 9 * 24 * time.Hour, "Initialisation"})

	f.AddRepository(provider.Repository{ID: daveRepoID, Owner: "dave", Name: "projet",
		URL: "https://github.example.test/dave/projet"})
	addCommits(f, daveRepoID, "main", "", fakeCommit{"d1", davePerson, 42 * 24 * time.Hour, "Rendu final"})
}

// newSyncedWorkspace renvoie un espace où les trois dépôts factices sont suivis (alice/projet avec le
// label « Alice Martin ») et synchronisés.
func newSyncedWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--label", "Alice Martin").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.run("repo", "add", "https://github.com/dave/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	return h
}

// decodeStrict décode stdout dans v en refusant les champs inconnus et tout texte après l'objet.
func decodeStrict(t *testing.T, stdout string, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("sortie JSON invalide : %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("texte après l'objet JSON :\n%s", stdout)
	}
}

// assertJSONKeys vérifie que l'objet JSON désigné par path (clés d'objets, ou « 0 » pour le premier
// élément d'un tableau) dans stdout a exactement les clés want : le format des sorties --json est un
// contrat pour les scripts.
func assertJSONKeys(t *testing.T, stdout string, path string, want ...string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("sortie JSON invalide : %v\n%s", err, stdout)
	}
	if path != "" {
		for _, p := range strings.Split(path, ".") {
			switch node := v.(type) {
			case map[string]any:
				v = node[p]
			case []any:
				if p != "0" || len(node) == 0 {
					t.Fatalf("chemin %q : tableau vide ou indice %q non géré", path, p)
				}
				v = node[0]
			default:
				t.Fatalf("chemin %q : %q dans une valeur %T", path, p, v)
			}
		}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("chemin %q : objet attendu, reçu %T (%v)", path, v, v)
	}
	var got []string
	for k := range obj {
		got = append(got, k)
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clés JSON de %q = %v, attendu %v", path, got, want)
	}
}

// repositoryKeys sont les clés de repositoryJSON.
var repositoryKeys = []string{"id", "full_name", "owner", "name", "label", "display_name", "url",
	"default_branch", "branch_patterns", "flemme_after_days", "dead_after_days", "enabled", "archived", "stars", "open_issues", "last_synced_at",
	"last_commit_at"}

// assertContains vérifie que s contient chacun des extraits.
func assertContains(t *testing.T, what, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			t.Errorf("%s sans %q :\n%s", what, p, s)
		}
	}
}

// assertNotContains vérifie que s ne contient aucun des extraits.
func assertNotContains(t *testing.T, what, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(s, p) {
			t.Errorf("%s avec %q :\n%s", what, p, s)
		}
	}
}

// tableRow renvoie la ligne de out qui commence par prefix, ses colonnes séparées par des espaces
// multiples ; échec du test si aucune.
func tableRow(t *testing.T, out, prefix string) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			var cols []string
			for _, c := range strings.Split(line, "  ") {
				if c = strings.TrimSpace(c); c != "" {
					cols = append(cols, c)
				}
			}
			return cols
		}
	}
	t.Fatalf("aucune ligne commençant par %q :\n%s", prefix, out)
	return nil
}

// mustCode vérifie le code de retour.
func (r result) mustCode(t *testing.T, code int) result {
	t.Helper()
	if r.code != code {
		t.Fatalf("code de retour %d, attendu %d\nstdout :\n%s\nstderr :\n%s", r.code, code, r.stdout, r.stderr)
	}
	return r
}
