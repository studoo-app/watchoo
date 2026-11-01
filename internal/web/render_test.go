package web

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestTemplatesParseAndDefineFragments(t *testing.T) {
	e := newTestEnv(t)
	want := map[string][]string{
		pageDashboard:    {"content", "dashboard-repos", "dashboard-feed"},
		pageRepo:         {"content", "repo-commits", "scripts"},
		pageContributors: {"content", "contributors-table"},
		pageContributor: {"content", "scripts", "contributor-overview", "contributor-repos", "contributor-pulls",
			"contributor-commits"},
		pageSettings: {"content", "settings-quota"},
		pageErrors:   {"content", "error-alert"},
	}
	common := []string{"layout", "state-badge", "rel-time", "csrf-field", "avatar", "flash", "sync-status",
		"sync-all-button", "sync-repo-button", "profile-link", "contributor-link", "commit-item"}
	for _, lang := range langs {
		for page, names := range want {
			set, ok := e.server.pages[lang][page]
			if !ok {
				t.Errorf("page %s (%s) not parsed", page, lang)
				continue
			}
			for _, name := range append(names, common...) {
				if set.Lookup(name) == nil {
					t.Errorf("page %s (%s): block %q missing", page, lang, name)
				}
			}
		}
	}
}

func TestLayoutBasics(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/unknown-page")
	assertContains(t, w,
		`<html lang="en">`,
		`<title>Page not found · licence-projet · Watchoo</title>`,
		`href="/static/app.css"`,
		`src="/static/vendor/htmx-2.0.11.min.js"`,
		`src="/static/theme.js"`,
		`id="theme-toggle"`,
		`href="/static/watchoo-icone.svg"`,
		`src="/static/watchoo-logo-final-nom.svg"`, `src="/static/watchoo-logo-final-nom-dark.svg"`,
		`rel="manifest"`,
		`"allowEval":false`,
		`"includeIndicatorStyles":false`,
		`"historyCacheSize":0`,
		`href="/contributors"`, `href="/settings"`,
		`id="sync-status"`,
		"v0.0.0-test",
		`<script type="application/json" id="watchoo-texts">`, `"locale":"en-US"`,
	)
	// No inline script or style (CSP).
	body := w.Body.String()
	for _, bad := range []string{"<script>", "<style", " style=", " onclick=", " onload=", "javascript:"} {
		if strings.Contains(body, bad) {
			t.Errorf("layout with %q", bad)
		}
	}
	if got := w.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
}

func TestFuncs(t *testing.T) {
	if badgeClass(activity.StateActive) != "badge-success" || badgeClass(activity.StateFlemme) != "badge-warning" ||
		badgeClass(activity.StateDead) != "badge-error" || badgeClass(activity.StateNeverSynced) != "badge-neutral" ||
		badgeClass(activity.StateArchived) != "badge-neutral" {
		t.Error("badgeClass: wrong classes")
	}
	fr := i18n.For(i18n.French)
	for res, want := range map[activity.Result][2]string{
		{State: activity.StateActive, Days: 1}:  {"Active", "Actif"},
		{State: activity.StateFlemme, Days: 9}:  {"Slacking · 9 d", "Flemme · 9 j"},
		{State: activity.StateDead, Days: 42}:   {"Dead · 42 d", "Dead · 42 j"},
		{State: activity.StateError, Days: -1}:  {"Error", "En erreur"},
		{State: activity.StateStopped, Days: 3}: {"Tracking stopped", "Suivi arrêté"},
	} {
		if got := stateText(en, res); got != want[0] {
			t.Errorf("stateText(en, %v) = %q, want %q", res, got, want[0])
		}
		if got := stateText(fr, res); got != want[1] {
			t.Errorf("stateText(fr, %v) = %q, want %q", res, got, want[1])
		}
	}
	if got := firstLine("titre\x1b[31m rouge\ncorps"); got != "titre rouge" {
		t.Errorf("firstLine = %q", got)
	}
	if got := truncate(5, "abcdefgh"); got != "abcde…" {
		t.Errorf("truncate = %q", got)
	}
	if got := shortSHA("0123456789abcdef"); got != "0123456" {
		t.Errorf("shortSHA = %q", got)
	}
	if initial("  élodie") != "É" || initial("") != "?" || initial("\x1b[1m") != "?" {
		t.Error("initial: wrong result")
	}
	if avatarURL("https://avatars.githubusercontent.com/u/1?v=4") == "" || avatarURL("https://evil.example.test/a.png") != "" ||
		avatarURL("javascript:alert(1)") != "" || avatarURL(`https://avatars.githubusercontent.com/u/1"onerror=x`) != "" {
		t.Error("avatarURL: wrong filtering")
	}
	for _, c := range []struct {
		tr   *i18n.Translator
		n    int
		want string
	}{
		{en, 1, "1 commit"}, {en, 3, "3 commits"}, {en, 0, "0 commits"},
		{fr, 1, "1 commit"}, {fr, 3, "3 commits"}, {fr, 0, "0 commit"},
	} {
		if got := pluralText(c.tr, "web.n.commit", c.n); got != c.want {
			t.Errorf("pluralText(%s, %d) = %q, want %q", c.tr.Lang(), c.n, got, c.want)
		}
	}
	for d, want := range map[time.Duration][2]string{0: {"manual only", "manuelle seule"},
		30 * time.Minute: {"30 min", "30 min"}, time.Hour: {"1 h", "1 h"}, 90 * time.Minute: {"1 h 30 min", "1 h 30"}} {
		if got := formatInterval(en, d); got != want[0] {
			t.Errorf("interval(en, %s) = %q, want %q", d, got, want[0])
		}
		if got := formatInterval(fr, d); got != want[1] {
			t.Errorf("interval(fr, %s) = %q, want %q", d, got, want[1])
		}
	}
	if percent(1, 3) != 33 || percent(5, 0) != 0 || percent(7, 5) != 100 {
		t.Error("percent: wrong computation")
	}
	if m, err := dict("A", 1, "B", "x"); err != nil || m["A"] != 1 || m["B"] != "x" {
		t.Errorf("dict = %v, %v", m, err)
	}
	if _, err := dict("A"); err == nil {
		t.Error("dict with an odd number of arguments accepted")
	}
	if _, err := dict(1, 2); err == nil {
		t.Error("dict with a non-string key accepted")
	}
	if repoPath(12) != "/repos/12" {
		t.Error("repoPath")
	}
	if got, err := withQuery("/repos/3/commits", "branch", "a&b #c", "author", "", "page", 2); err != nil ||
		got != "/repos/3/commits?branch=a%26b+%23c&page=2" {
		t.Errorf("withQuery = %q, %v", got, err)
	}
	if got, _ := withQuery("/repos/3", "page", 0); got != "/repos/3" {
		t.Errorf("withQuery without parameter = %q", got)
	}
	if _, err := withQuery("/x", "k"); err == nil {
		t.Error("withQuery with an odd number of arguments accepted")
	}
	if _, err := withQuery("/x", "k", 1.5); err == nil {
		t.Error("withQuery with a float accepted")
	}
}

func TestTimeFuncsUseDisplayZone(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("Europe/Paris time zone unavailable")
	}
	e := newTestEnv(t)
	e.server.loc = paris
	f := e.server.funcs(en)
	at := time.Date(2026, 10, 27, 13, 5, 0, 0, time.UTC)
	if got := f["localTime"].(func(time.Time) string)(at); got != "Oct 27, 2026 14:05" {
		t.Errorf("localTime = %q", got)
	}
	ff := e.server.funcs(i18n.For(i18n.French))
	if got := ff["localTime"].(func(time.Time) string)(at); got != "27/10/2026 à 14:05" {
		t.Errorf("French localTime = %q", got)
	}
	if got := ff["relTime"].(func(time.Time) string)(testNow.Add(-2 * time.Hour)); got != "il y a 2 h" {
		t.Errorf("French relTime = %q", got)
	}
	if got := f["localClock"].(func(time.Time) string)(at); got != "14:05" {
		t.Errorf("localClock = %q", got)
	}
	if got := f["relTime"].(func(time.Time) string)(testNow.Add(-2 * time.Hour)); got != "2 h ago" {
		t.Errorf("relTime = %q", got)
	}
	if got := f["localTime"].(func(time.Time) string)(time.Time{}); got != "" {
		t.Errorf("localTime(zero) = %q", got)
	}
	if isoTime(at) != "2026-10-27T13:05:00Z" || isoTime(time.Time{}) != "" {
		t.Error("isoTime")
	}
}

// TestJSONScriptEscaping sets the contract of the data passed to JavaScript: in a
// <script type="application/json">, html/template encodes the value as JSON and neutralizes any
// closing tag.
func TestJSONScriptEscaping(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(`<script type="application/json" id="d">{{.}}</script>`))
	var buf bytes.Buffer
	data := []activity.DayCount{{Date: "</script><script>alert(1)</script>", Count: 2}}
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "</script>") != 1 || !strings.Contains(out, `"count":2`) {
		t.Errorf("JSON badly escaped: %s", out)
	}
}

func TestPartials(t *testing.T) {
	e := newTestEnv(t)
	set := e.server.pages[i18n.English][pageErrors]
	run := func(name string, data any) string {
		t.Helper()
		var buf bytes.Buffer
		if err := set.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return buf.String()
	}
	if out := run("state-badge", activity.Result{State: activity.StateFlemme, Days: 9}); !strings.Contains(out, "badge-warning") ||
		!strings.Contains(out, "Slacking · 9 d") || !strings.Contains(out, `title="State: Slacking"`) {
		t.Errorf("state-badge: %s", out)
	}
	if out := run("rel-time", testNow.Add(-5*time.Minute)); !strings.Contains(out, "5 min ago") ||
		!strings.Contains(out, `datetime="2026-10-27T09:55:00Z"`) || !strings.Contains(out, `title="Oct 27, 2026 09:55"`) {
		t.Errorf("rel-time: %s", out)
	}
	if out := run("rel-time", time.Time{}); !strings.Contains(out, "never") {
		t.Errorf("rel-time zero: %s", out)
	}
	frSet := e.server.pages[i18n.French][pageErrors]
	var frBuf bytes.Buffer
	if err := frSet.ExecuteTemplate(&frBuf, "state-badge", activity.Result{State: activity.StateFlemme, Days: 9}); err != nil ||
		!strings.Contains(frBuf.String(), "Flemme · 9 j") || !strings.Contains(frBuf.String(), `title="État : Flemme"`) {
		t.Errorf("French state-badge: %s, %v", frBuf.String(), err)
	}
	out := run("avatar", map[string]any{"Name": "alice", "URL": "https://avatars.githubusercontent.com/u/10?v=4"})
	for _, want := range []string{`referrerpolicy="no-referrer"`, `loading="lazy"`, `src="https://avatars.githubusercontent.com/u/10?v=4"`} {
		if !strings.Contains(out, want) {
			t.Errorf("avatar without %q: %s", want, out)
		}
	}
	if out := run("avatar", map[string]any{"Name": "bob", "URL": "https://evil.example.test/p.png"}); strings.Contains(out, "<img") ||
		!strings.Contains(out, ">B<") {
		t.Errorf("fallback avatar: %s", out)
	}
	page := e.server.page(e.request(http.MethodGet, "/", nil), "", "", nil)
	page.CSRFToken = "token"
	if out := run("sync-repo-button", map[string]any{"Page": page, "RepoID": int64(7)}); !strings.Contains(out, `hx-post="/repos/7/sync"`) ||
		!strings.Contains(out, `value="token"`) || !strings.Contains(out, "Sync this repository") {
		t.Errorf("sync-repo-button: %s", out)
	}
	if out := run("sync-all-button", page); !strings.Contains(out, `action="/sync"`) || !strings.Contains(out, `name="csrf_token"`) {
		t.Errorf("sync-all-button: %s", out)
	}
	if out := run("flash", map[string]any{"Message": "<b>x</b>", "Error": true}); !strings.Contains(out, "alert-error") ||
		strings.Contains(out, "<b>") {
		t.Errorf("flash: %s", out)
	}
}
