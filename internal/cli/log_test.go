package cli

import (
	"slices"
	"strings"
	"testing"
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
	if row := tableRow(t, r.stdout, "il y a 2 h"); !slices.Equal(row[1:], []string{"Alice Martin", "Alice Martin",
		"b9f85da", "Ajoute les tests"}) {
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
		"authored_at", "committed_at", "additions", "deletions")
	assertJSONKeys(t, r.stdout, "commits.0.repository", "id", "full_name", "display_name")

	r = h.run("log", "--author", "personne", "--since", "7d").mustSucceed(t)
	if r.stdout != "Aucun commit ne correspond aux filtres (auteur personne, depuis 7d).\n" {
		t.Errorf("log sans résultat = %q", r.stdout)
	}
	if h.fake().TokenCalls() != 4 { // 3 repo add + 1 sync : log ne demande jamais le token
		t.Errorf("Token appelé %d fois", h.fake().TokenCalls())
	}
}

func TestLogErrors(t *testing.T) {
	h := newWorkspace(t)
	r := h.run("log").mustSucceed(t)
	assertContains(t, "log base vide", r.stdout, "la base est vide (lancez « watchoo sync »)")
	h.run("log", "--since", "7j").mustFail(t, `--since "7j" invalide`, "7d, 12h ou 30m")
	h.run("log", "--since", "-3d").mustFail(t, "--since")
	h.run("log", "-n", "-1").mustFail(t, "-n -1 invalide")
	h.run("log", "-n", "deux").mustFail(t, `valeur invalide "deux" pour --limit : nombre entier attendu`)
	h.run("log", "--repo", "alice/projet").mustFail(t, "le dépôt alice/projet n'est pas suivi")
}

func TestContributors(t *testing.T) {
	h := newSyncedWorkspace(t)

	r := h.run("contributors").mustSucceed(t)
	want := "CONTRIBUTEUR  LOGIN     COMMITS  AJOUTS  SUPPRESSIONS  DÉPÔTS  DERNIER COMMIT\n" +
		"Bob Durand    -         2        +0      -0            2       il y a 1 h\n" +
		"Alice Martin  alice-gh  2        +0      -0            1       il y a 2 h\n" +
		"Dave Petit    dave-gh   1        +0      -0            1       il y a 42 j\n"
	if r.stdout != want {
		t.Errorf("contributors =\n%s\nattendu :\n%s", r.stdout, want)
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
		"additions", "deletions", "repositories", "last_commit_at")

	r = h.run("contributors", "--since", "30d", "--json").mustSucceed(t)
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Contributors) != 2 { // Dave (42 jours) sort de la période
		t.Errorf("contributors --since 30d = %+v", rep)
	}
	r = h.run("contributors", "--since", "30m").mustSucceed(t)
	assertContains(t, "contributors sans résultat", r.stdout, "Aucun contributeur depuis 30m")

	h.run("contributors", "--since", "un mois").mustFail(t, `--since "un mois" invalide`)
	h.run("contributors", "--repo", "carol/projet").mustFail(t, "le dépôt carol/projet n'est pas suivi")
}
