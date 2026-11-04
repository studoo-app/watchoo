package web

// "Languages" card of a contributor's page and main language of their repositories (1.2.0).

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
)

// seedLanguages adds commits of Eve in ten languages (the 8 first shown, the last two grouped) and one
// commit not analyzed yet, and returns the path of her page.
func (e *testEnv) seedLanguages() string {
	e.t.Helper()
	ids := e.seed()
	eve := store.Identity{Name: "Eve", Email: "eve@example.test"}
	var commits []store.CommitWrite
	for i := range 10 {
		c := e.commitWrite(20+i, eve, time.Duration(10-i)*time.Hour, "travail", "main")
		c.Languages = map[string]int{fmt.Sprintf("Lang%02d", i): 100 - i*10}
		commits = append(commits, c)
	}
	pending := e.commitWrite(40, eve, 0, "pas encore", "main")
	pending.Languages = nil
	commits = append(commits, pending)
	e.applySync(ids.Alice, "alice", commits...)
	return contributorPath(e.contributorIDs()["Eve"])
}

func TestContributorLanguages(t *testing.T) {
	e := newTestEnv(t)
	path := e.seedLanguages()

	w := e.get(path)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `id="contributor-languages"`, "Languages", "Lang00", "100 lines", "18.2%", "Lang07",
		"Other languages", "1 commit not analyzed yet.", "Main language", "<td class=\"whitespace-nowrap\">Lang00</td>",
		"lines added measure neither quality nor skill")
	assertNotContains(t, w, "Lang08", "No known language")

	// Fragment refreshed after a sync.
	f := e.htmxWith(path, map[string]string{"HX-Target": contributorLanguagesID})
	assertStatus(t, f, http.StatusOK)
	assertContains(t, f, `id="contributor-languages"`, "Lang00")
	assertNotContains(t, f, `id="contributor-repos"`)

	// Contributor whose commits are not analyzed yet: "No known language", "—" in their repositories.
	alice := e.get(contributorPath(e.contributorIDs()["Alice Martin"]))
	assertContains(t, alice, "No known language: they are read from the local clone at the next sync.")
	assertNotContains(t, alice, "not analyzed yet.")

	// French.
	fr := newTestEnv(t, withLang("fr"))
	assertContains(t, fr.get(fr.seedLanguages()), "Langages", "100 lignes", "18,2 %", "Autres langages", "1 commit pas encore analysé.",
		"Langage principal")
}
