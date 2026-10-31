package cli

// « watchoo prs » et « watchoo releases » (V2) : texte, filtres, JSON, options invalides, textes
// piégés neutralisés, et avertissements de « watchoo sync ».

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

func TestPRsText(t *testing.T) {
	h := newSyncedWorkspace(t)
	r := h.run("prs").mustSucceed(t)
	// La date relative (« 3 h ago ») vient d'internal/activity, couche traduite à part.
	row := tableRow(t, r.stdout, "3 h ago")
	if strings.Join(row, "|") != "3 h ago|Alice Martin|#1|open|alice-gh|Ajoute la page de connexion" {
		t.Errorf("ligne de la PR ouverte : %q\n%s", row, r.stdout)
	}
	assertContains(t, "prs", r.stdout, "UPDATED", "#2", "merged", "Corrige le build")
	if strings.Index(r.stdout, "#1") > strings.Index(r.stdout, "#2") {
		t.Errorf("ordre : la PR modifiée le plus récemment d'abord :\n%s", r.stdout)
	}

	r = h.run("prs", "--state", "merged").mustSucceed(t)
	assertNotContains(t, "prs --state merged", r.stdout, "#1")
	assertContains(t, "prs --state merged", r.stdout, "#2")
	r = h.run("prs", "--state", "closed").mustSucceed(t)
	assertContains(t, "prs --state closed", r.stdout, "No pull request for these filters (state closed).")
	r = h.run("prs", "--repo", "bob/projet").mustSucceed(t)
	assertContains(t, "prs --repo bob/projet", r.stdout, "No pull request for these filters (repository bob/projet).")
	r = h.run("prs", "-n", "1").mustSucceed(t)
	assertNotContains(t, "prs -n 1", r.stdout, "#2")

	r = h.run("prs", "--repo", "alice/projet", "--state", "open", "--json").mustSucceed(t)
	var rep prsReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.PullRequests) != 1 {
		t.Fatalf("prs --json : %+v", rep)
	}
	p := rep.PullRequests[0]
	if p.Number != 1 || p.State != "open" || p.Draft || p.Author.Login != "alice-gh" || p.HeadBranch != "login" ||
		p.Repository.DisplayName != "Alice Martin" || p.Repository.FullName != "alice/projet" || p.MergedAt != nil ||
		p.ClosedAt != nil || p.UpdatedAt == nil || !p.UpdatedAt.Equal(testNow.Add(-3*time.Hour)) {
		t.Errorf("prs --json = %+v", p)
	}

	h.run("prs", "--state", "fusionnee").mustCode(t, 1)
	h.run("prs", "-n", "-1").mustCode(t, 1)
	r = h.run("prs", "--repo", "carol/projet").mustCode(t, 1)
	assertContains(t, "dépôt inconnu", r.stderr, "carol/projet")
}

func TestReleasesText(t *testing.T) {
	h := newSyncedWorkspace(t)
	r := h.run("releases").mustSucceed(t)
	assertContains(t, "releases", r.stdout, "Releases\n", "Tags\n", "v1.0", "Première version", "published", "alice-gh",
		"COMMIT DATE")
	tag := tableRow(t, r.stdout, "Alice Martin  v0.1")
	if tag[len(tag)-1] != "-" {
		t.Errorf("tag dont le commit n'est pas en base : %q", tag)
	}
	if strings.Index(r.stdout, "v1.0  ") > strings.LastIndex(r.stdout, "v0.1") {
		t.Errorf("tags : le commit connu d'abord :\n%s", r.stdout)
	}

	r = h.run("releases", "--repo", "bob/projet").mustSucceed(t)
	assertContains(t, "releases bob", r.stdout, "No release for the repository bob/projet.", "No tag for the repository bob/projet.")

	r = h.run("releases", "--json", "-n", "1").mustSucceed(t)
	var rep releasesReport
	decodeStrict(t, r.stdout, &rep)
	if len(rep.Releases) != 1 || rep.Releases[0].ID != 501 || rep.Releases[0].PublishedAt == nil ||
		len(rep.Tags) != 1 || rep.Tags[0].Name != "v1.0" || rep.Tags[0].CommittedAt == nil || rep.Tags[0].CommitURL == "" ||
		rep.Tags[0].ShortSHA != provider.ShortSHA(providertest.SHA("a2")) {
		t.Errorf("releases --json = %+v", rep)
	}
	h.run("releases", "-n", "-2").mustCode(t, 1)
}

// Titres de PR et noms de release piégés : aucune séquence dans la sortie texte, texte d'origine en
// JSON.
func TestPRsReleasesNeutralizeEscapes(t *testing.T) {
	h := newWorkspace(t)
	h.Provider.AddRepository(provider.Repository{ID: 301, Owner: "eve", Name: "projet"})
	addCommits(h.Provider, 301, "main", "", fakeCommit{"e1", alicePerson, time.Hour, "ok"})
	h.Provider.SetPullRequests(301, provider.PullRequest{Number: 1, State: provider.PullRequestOpen,
		Title: "fix\x1b]0;PWNED\x07\x1b[2J ok", Author: provider.Account{Login: "eve\x1b[31m"},
		CreatedAt: testNow, UpdatedAt: testNow})
	h.Provider.SetReleases(301, provider.Release{ID: 1, TagName: "v1\x1b[2J", Name: "nom\x07‮", CreatedAt: testNow})
	h.run("repo", "add", "eve/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	for _, cmd := range []string{"prs", "releases"} {
		r := h.run(cmd).mustSucceed(t)
		if strings.ContainsAny(r.stdout, "\x1b\x07‮") {
			t.Errorf("%s : séquence de contrôle dans la sortie texte :\n%q", cmd, r.stdout)
		}
	}
	r := h.run("prs", "--json").mustSucceed(t)
	if !strings.Contains(r.stdout, `\u001b]0;PWNED`) {
		t.Errorf("prs --json : titre d'origine perdu :\n%s", r.stdout)
	}
}

// Un dépôt dont les pull requests n'ont pas pu être lues reste « ok » : avertissement dans DÉTAIL, en
// JSON et dans le bilan, code de retour nul.
func TestSyncWarnings(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.Provider.FailNext(providertest.MethodPullRequests, &provider.APIError{Op: "liste des pull requests",
		Status: 502, Err: provider.ErrUnavailable})
	r := h.run("sync").mustSucceed(t)
	row := tableRow(t, r.stdout, "alice/projet")
	if row[1] != "ok" || !strings.HasPrefix(row[len(row)-1], "pull requests non synchronisées") {
		t.Errorf("ligne du dépôt : %q", row)
	}
	assertContains(t, "sync", r.stdout, "1 warning")

	h.Provider.FailNext(providertest.MethodTags, &provider.APIError{Status: 404, Err: provider.ErrNotFound})
	r = h.run("sync", "--json").mustSucceed(t)
	assertContains(t, "sync --json", r.stdout, `"warnings": [`, "tags non synchronisés")
}
