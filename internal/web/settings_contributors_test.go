package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestSettingsContributorsPage(t *testing.T) {
	e := newTestEnv(t)
	assertContains(t, e.get("/settings/contributors"), "No contributor yet")

	e.seed()
	ids := e.contributorIDs()
	alice := strconv.FormatInt(ids["Alice Martin"], 10)
	w := e.get("/settings/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"<title>Settings · Contributors · "+testWorkspace+" · Watchoo</title>",
		`href="/settings/contributors" class="whitespace-nowrap menu-active" aria-current="page">Contributors`,
		"Names of the contributors", "never exported by the configuration sharing",
		`id="contributor-`+alice+`"`, `action="/settings/contributors/`+alice+`"`, `name="label"`, `maxlength="100"`,
		"Alice Martin", "@alice-gh", "Bob Durand", "Carol", `name="csrf_token"`, "Name for Alice Martin",
	)
	// Search: on the label, the name or the login, without case.
	w = e.get("/settings/contributors?q=CAROL-G")
	assertContains(t, w, "1 of 3 contributors", "Carol", `value="CAROL-G"`, "Show all")
	// Bob's row is hidden (his name stays in the "attach to" list of Carol's row).
	assertNotContains(t, w, `id="contributor-`+strconv.FormatInt(ids["Bob Durand"], 10)+`"`)
	assertContains(t, e.get("/settings/contributors?q=zzz"), "No contributor matches.")
}

func TestSettingsContributorLabel(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	ids := e.contributorIDs()
	bob := ids["Bob Durand"]
	path := "/settings/contributors/" + strconv.FormatInt(bob, 10)
	n := strconv.FormatInt(bob, 10)

	w := e.post(path, url.Values{"label": {"  Robert <b>Durand</b> "}}, false)
	assertRedirect(t, w, w.Code, "/settings/contributors?msg=label_saved&contributor="+n+"#contributor-"+n)
	labels, err := e.store.ContributorLabels(t.Context())
	if err != nil || labels[bob] != "Robert <b>Durand</b>" {
		t.Fatalf("labels = %v, %v", labels, err)
	}
	w = e.get("/settings/contributors?msg=label_saved&contributor=" + n)
	assertContains(t, w, "The name of the contributor is saved: Robert &lt;b&gt;Durand&lt;/b&gt;.",
		"Name in the commits: Bob Durand", `value="Robert &lt;b&gt;Durand&lt;/b&gt;"`)
	assertNotContains(t, w, "<b>Durand")
	// Unknown or forged message parameters: no message.
	assertNotContains(t, e.get("/settings/contributors?msg=label_saved&contributor=abc"), "is saved")
	assertNotContains(t, e.get("/settings/contributors?msg=label_saved&contributor=999"), "is saved")

	// The label is displayed everywhere instead of the pseudonym, which stays visible.
	assertContains(t, e.get("/contributors"), "Robert &lt;b&gt;Durand&lt;/b&gt;", "Bob Durand")
	w = e.get(contributorPath(bob))
	assertContains(t, w, `<h1 id="contributor-title" class="text-2xl font-bold break-words">Robert &lt;b&gt;Durand&lt;/b&gt;</h1>`,
		"Name in the commits: Bob Durand", `href="/settings/contributors#contributor-`+n+`"`, "Rename")
	assertContains(t, e.get("/"), "Robert &lt;b&gt;Durand&lt;/b&gt;")

	// Refused: too long, control characters. The page is displayed again with the error on the row.
	w = e.post(path, url.Values{"label": {strings.Repeat("é", model.MaxContributorLabelRunes+1)}}, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Name refused: see the form of the contributor.", "Name too long: 100 characters at most.",
		`aria-invalid="true"`)
	w = e.post(path, url.Values{"label": {"Bob\x1b[31m"}}, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "control characters are not allowed")
	assertNotContains(t, w, "\x1b")
	if labels, _ := e.store.ContributorLabels(t.Context()); labels[bob] != "Robert <b>Durand</b>" {
		t.Errorf("label changed by a refused input: %q", labels[bob])
	}

	// Unknown contributor: 404, nothing written.
	assertStatus(t, e.post("/settings/contributors/999", url.Values{"label": {"X"}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/settings/contributors/999", url.Values{"label": {strings.Repeat("x", 200)}}, false), http.StatusNotFound)

	// Empty: back to the pseudonym.
	w = e.post(path, url.Values{"label": {" "}}, false)
	assertRedirect(t, w, w.Code, "/settings/contributors?msg=label_removed&contributor="+n+"#contributor-"+n)
	assertContains(t, e.get("/settings/contributors?msg=label_removed&contributor="+n),
		"The name is removed: the contributor is shown again as Bob Durand.")
	if labels, _ := e.store.ContributorLabels(t.Context()); len(labels) != 0 {
		t.Errorf("labels after removal: %v", labels)
	}

	// HTMX: HX-Redirect.
	w = e.post(path, url.Values{"label": {"Bobby"}}, true)
	if got := w.Header().Get("HX-Redirect"); got != "/settings/contributors?msg=label_saved&contributor="+n+"#contributor-"+n {
		t.Errorf("HX-Redirect = %q", got)
	}
}

func TestSettingsContributorLabelRequiresCSRF(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	id := e.contributorIDs()["Bob Durand"]
	r := e.request(http.MethodPost, "/settings/contributors/"+strconv.FormatInt(id, 10), strings.NewReader("label=X"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assertStatus(t, e.do(r), http.StatusForbidden)
	if labels, _ := e.store.ContributorLabels(t.Context()); len(labels) != 0 {
		t.Errorf("label written without CSRF: %v", labels)
	}
	// No action in GET.
	assertStatus(t, e.get("/settings/contributors/"+strconv.FormatInt(id, 10)), http.StatusMethodNotAllowed)
}

func TestMatchesQuery(t *testing.T) {
	c := model.Contributor{Label: "Élodie Martin", Name: "elo", ProviderLogin: "elo-gh"}
	for q, want := range map[string]bool{"": true, "martin": true, "ELO": true, "-GH": true, "zz": false} {
		if got := matchesQuery(c, q); got != want {
			t.Errorf("matchesQuery(%q) = %v, want %v", q, got, want)
		}
	}
}
