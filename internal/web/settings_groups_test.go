package web

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

// TestContributorGroupsWeb checks the contributor groups (1.9.0) on the web: attaching from the
// settings, one row per person in the ranking, the "Accounts" card, the redirect of an attached
// account's page, detaching and the refusals.
func TestContributorGroupsWeb(t *testing.T) {
	e := newTestEnv(t)
	e.seed()
	ids := e.contributorIDs()
	alice, bob, carol := ids["Alice Martin"], ids["Bob Durand"], ids["Carol"]
	n := func(id int64) string { return strconv.FormatInt(id, 10) }
	before := e.contributorIDs()
	if len(before) != 3 {
		t.Fatalf("contributors before grouping: %v", before)
	}

	w := e.get("/settings/contributors")
	assertContains(t, w, `action="/settings/contributors/`+n(bob)+`/attach"`, `name="primary"`,
		"Same person as… (attach to)", `<option value="`+n(alice)+`">Alice Martin</option>`)

	// Bob is attached to Alice.
	w = e.post("/settings/contributors/"+n(bob)+"/attach", url.Values{"primary": {n(alice)}}, false)
	assertRedirect(t, w, w.Code, "/settings/contributors?msg=attached&contributor="+n(alice)+"#contributor-"+n(alice))
	w = e.get("/settings/contributors?msg=attached&contributor=" + n(alice))
	assertContains(t, w, "An account is attached to Alice Martin.", `action="/settings/contributors/`+n(bob)+`/detach"`,
		">Accounts <span")
	assertNotContains(t, w, `id="contributor-`+n(bob)+`"`, `action="/settings/contributors/`+n(alice)+`/attach"`)
	// The search finds the group through the attached account.
	assertContains(t, e.get("/settings/contributors?q=bob"), `id="contributor-`+n(alice)+`"`)

	// One row per person: Bob's commits are counted with Alice.
	after := e.contributorIDs()
	if len(after) != 2 || after["Alice Martin"] != alice {
		t.Errorf("contributors after grouping: %v", after)
	}
	w = e.get("/contributors")
	assertContains(t, w, "&#43;1 account", `title="Bob Durand"`)
	assertNotContains(t, w, `href="/contributors/`+n(bob)+`"`)
	w = e.get(contributorPath(alice))
	assertContains(t, w, "Accounts", "Bob Durand", "Manage the accounts")
	// The page of the attached account leads to the person.
	w = e.get(contributorPath(bob) + "?page=2")
	if w.Code != http.StatusFound || w.Header().Get("Location") != contributorPath(alice)+"?page=2" {
		t.Errorf("attached account page: %d → %q", w.Code, w.Header().Get("Location"))
	}
	w = e.get(contributorPath(bob) + "/commits")
	if w.Code != http.StatusFound || w.Header().Get("Location") != contributorPath(alice)+"/commits" {
		t.Errorf("attached account commits: %d → %q", w.Code, w.Header().Get("Location"))
	}
	assertStatus(t, e.get("/contributors/999"), http.StatusNotFound)

	// Refusals: self, chain, principal with accounts, missing choice, unknown.
	for _, tc := range []struct {
		id      int64
		primary string
		want    string
	}{
		{carol, n(carol), "cannot be attached to itself"},
		{carol, n(bob), "is itself attached"},
		{alice, n(carol), "has attached accounts"},
		{bob, n(carol), "already attached"},
		{carol, "", "Choose the contributor"},
	} {
		w := e.post("/settings/contributors/"+n(tc.id)+"/attach", url.Values{"primary": {tc.primary}}, false)
		assertStatus(t, w, http.StatusBadRequest)
		assertContains(t, w, tc.want)
	}
	assertStatus(t, e.post("/settings/contributors/999/attach", url.Values{"primary": {n(alice)}}, false), http.StatusNotFound)
	assertStatus(t, e.post("/settings/contributors/"+n(carol)+"/attach", url.Values{"primary": {"999"}}, false), http.StatusNotFound)
	w = e.post("/settings/contributors/"+n(carol)+"/detach", nil, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "This account is not attached.")
	assertStatus(t, e.post("/settings/contributors/999/detach", nil, false), http.StatusNotFound)

	// CSRF: nothing changes.
	r := e.request(http.MethodPost, "/settings/contributors/"+n(bob)+"/detach", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	assertStatus(t, e.do(r), http.StatusForbidden)

	// Detaching restores the counts exactly.
	w = e.post("/settings/contributors/"+n(bob)+"/detach", nil, false)
	assertRedirect(t, w, w.Code, "/settings/contributors?msg=detached&contributor="+n(alice)+"#contributor-"+n(alice))
	assertContains(t, e.get("/settings/contributors?msg=detached&contributor="+n(alice)), "An account is detached from Alice Martin.")
	if got := e.contributorIDs(); len(got) != 3 || got["Bob Durand"] != bob {
		t.Errorf("contributors after detaching: %v", got)
	}
	assertStatus(t, e.get(contributorPath(bob)), http.StatusOK)
}
