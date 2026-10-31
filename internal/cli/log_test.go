package cli

import (
	"slices"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
)

// commitMessages renvoie les messages des commits de la sortie JSON de « watchoo log ».
func commitMessages(t *testing.T, stdout string) []string {
	t.Helper()
	var rep logReport
	decodeStrict(t, stdout, &rep)
	msgs := make([]string, len(rep.Commits))
	for i, c := range rep.Commits {
		msgs[i] = firstLine(c.Message)
	}
	return msgs
}

func TestLog(t *testing.T) {
	h := newSyncedWorkspace(t)

	r := h.run("log").mustSucceed(t)
	lines := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
	if len(lines) != 6 || !strings.HasPrefix(lines[0], "DATE") {
		t.Fatalf("log =\n%s", r.stdout)
	}
	if row := tableRow(t, r.stdout, "2 h ago"); !slices.Equal(row[1:], []string{"Alice Martin", "Alice Martin",
		"b9f85da", "-", "Ajoute les tests"}) { // lignes inconnues : premier import par l'API, sans clone
		t.Errorf("ligne du commit a2 = %q", row)
	}
	assertNotContains(t, "log", r.stdout, "Détail du commit") // première ligne du message seulement

	for _, tt := range []struct {
		args []string
		want []string
	}{
		{[]string{"-n", "2"}, []string{"Essai sur dev", "Ajoute les tests"}},
		{[]string{"--repo", "bob/projet"}, []string{"Initialisation"}},
		{[]string{"--author", "ALICE-GH"}, []string{"Ajoute les tests", "Premier jet du projet"}},
		{[]string{"--author", "bob@example.test"}, []string{"Essai sur dev", "Initialisation"}},
		{[]string{"--author", "Bob Durand", "--repo", "alice/projet"}, []string{"Essai sur dev"}},
		{[]string{"--branch", "dev"}, []string{"Essai sur dev"}},
		{[]string{"--branch", "main", "--repo", "alice/projet"}, []string{"Ajoute les tests", "Premier jet du projet"}},
		{[]string{"--since", "3h"}, []string{"Essai sur dev", "Ajoute les tests"}},
		{[]string{"--since", "10d"}, []string{"Essai sur dev", "Ajoute les tests", "Premier jet du projet", "Initialisation"}},
		{[]string{"-n", "0"}, []string{"Essai sur dev", "Ajoute les tests", "Premier jet du projet", "Initialisation", "Rendu final"}},
		{[]string{"--author", "personne"}, []string{}},
	} {
		r := h.run(append([]string{"log", "--json"}, tt.args...)...).mustSucceed(t)
		if got := commitMessages(t, r.stdout); !slices.Equal(got, tt.want) {
			t.Errorf("log %q = %q, attendu %q", tt.args, got, tt.want)
		}
	}

	r = h.run("log", "--json", "-n", "1").mustSucceed(t)
	assertJSONKeys(t, r.stdout, "", "commits")
	assertJSONKeys(t, r.stdout, "commits.0", "repository", "sha", "short_sha", "message", "url", "author",
		"authored_at", "committed_at", "additions", "deletions", "stats_known")
	assertJSONKeys(t, r.stdout, "commits.0.repository", "id", "full_name", "display_name")

	r = h.run("log", "--author", "personne", "--since", "7d").mustSucceed(t)
	if r.stdout != "No commit matches the filters (author personne, since 7d).\n" {
		t.Errorf("log sans résultat = %q", r.stdout)
	}
	if h.fake().TokenCalls() != 4 { // 3 repo add + 1 sync : log ne demande jamais le token
		t.Errorf("Token appelé %d fois", h.fake().TokenCalls())
	}
}

func TestLogErrors(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("log").mustSucceed(t)
	assertContains(t, "log base vide", r.stdout, "the database is empty (run \"watchoo sync\")")
	h.run("log", "--since", "7j").mustFail(t, `--since "7j" is invalid`, "for example 7d, 12h or 30m")
	h.run("log", "--since", "-3d").mustFail(t, "--since")
	h.run("log", "-n", "-1").mustFail(t, "-n -1 is invalid")
	h.run("log", "-n", "deux").mustFail(t, `invalid value "deux" for --limit`, "expected an integer")
	h.run("log", "--repo", "alice/projet").mustFail(t, "repository alice/projet is not tracked")
}

func TestContributors(t *testing.T) {
	h := newSyncedWorkspace(t)

	r := h.run("contributors").mustSucceed(t)
	// Lignes inconnues (« - ») : premier import par l'API, sans clone Git local dans ce test.
	// Les dates relatives (« il y a … ») viennent d'internal/activity, couche traduite à part.
	assertContains(t, "contributors", r.stdout, "CONTRIBUTOR", "LOGIN", "COMMITS", "ADDITIONS", "DELETIONS",
		"REPOSITORIES", "LAST COMMIT")
	if row := tableRow(t, r.stdout, "Bob Durand"); !slices.Equal(row, []string{"Bob Durand", "-", "2", "-", "-", "2", "1 h ago"}) {
		t.Errorf("ligne de Bob Durand = %q", row)
	}
	if row := tableRow(t, r.stdout, "Alice Martin"); !slices.Equal(row, []string{"Alice Martin", "alice-gh", "2", "-", "-", "1", "2 h ago"}) {
		t.Errorf("ligne d'Alice Martin = %q", row)
	}
	if row := tableRow(t, r.stdout, "Dave Petit"); !slices.Equal(row, []string{"Dave Petit", "dave-gh", "1", "-", "-", "1", "42 d ago"}) {
		t.Errorf("ligne de Dave Petit = %q", row)
	}

	r = h.run("contributors", "--repo", "alice/projet", "--json").mustSucceed(t)
	var rep contributorsReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Contributors) != 2 || rep.Contributors[1].DisplayName != "Bob Durand" ||
		rep.Contributors[1].Commits != 1 || rep.Contributors[0].Login != "alice-gh" || rep.Contributors[0].Commits != 2 ||
		rep.Contributors[0].AvatarURL == "" || rep.Contributors[0].LastCommitAt == nil {
		t.Errorf("contributors --repo --json = %+v", rep)
	}
	assertJSONKeys(t, r.stdout, "", "contributors")
	assertJSONKeys(t, r.stdout, "contributors.0", "id", "name", "login", "display_name", "avatar_url", "commits",
		"additions", "deletions", "repositories", "last_commit_at", "weekly")

	r = h.run("contributors", "--since", "30d", "--json").mustSucceed(t)
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Contributors) != 2 { // Dave (42 jours) sort de la période
		t.Errorf("contributors --since 30d = %+v", rep)
	}
	r = h.run("contributors", "--since", "30m").mustSucceed(t)
	assertContains(t, "contributors sans résultat", r.stdout, "No contributor since 30m")

	h.run("contributors", "--since", "un mois").mustFail(t, `--since "un mois" is invalid`)
	h.run("contributors", "--repo", "carol/projet").mustFail(t, "repository carol/projet is not tracked")
}

// Lignes d'un commit dans « watchoo log » : « - » si inconnues, « +0/-0 » si nulles.
func TestCommitLines(t *testing.T) {
	for _, tc := range []struct {
		c    model.Commit
		want string
	}{
		{model.Commit{}, "-"},
		{model.Commit{StatsKnown: true}, "+0/-0"},
		{model.Commit{StatsKnown: true, Additions: 12, Deletions: 3}, "+12/-3"},
	} {
		if got := commitLines(tc.c); got != tc.want {
			t.Errorf("commitLines(%+v) = %q, attendu %q", tc.c, got, tc.want)
		}
	}
	if knownLines(false, "+", 0) != "-" || knownLines(true, "-", 4) != "-4" {
		t.Error("knownLines")
	}
}
