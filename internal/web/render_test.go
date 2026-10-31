package web

import (
	"bytes"
	"html/template"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
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
	for page, names := range want {
		set, ok := e.server.pages[page]
		if !ok {
			t.Errorf("page %s non analysée", page)
			continue
		}
		for _, name := range append(names, common...) {
			if set.Lookup(name) == nil {
				t.Errorf("page %s : bloc %q absent", page, name)
			}
		}
	}
}

func TestLayoutBasics(t *testing.T) {
	e := newTestEnv(t)
	w := e.get("/page-inconnue")
	assertContains(t, w,
		`<html lang="fr">`,
		`<title>Page introuvable · licence-projet · Watchoo</title>`,
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
	)
	// Aucun script ni style en ligne (CSP).
	body := w.Body.String()
	for _, bad := range []string{"<script>", "<style", " style=", " onclick=", " onload=", "javascript:"} {
		if strings.Contains(body, bad) {
			t.Errorf("layout avec %q", bad)
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
		t.Error("badgeClass : mauvaises classes")
	}
	for res, want := range map[activity.Result]string{
		{State: activity.StateActive, Days: 1}:  "Active",
		{State: activity.StateFlemme, Days: 9}:  "Slacking · 9 j",
		{State: activity.StateDead, Days: 42}:   "Dead · 42 j",
		{State: activity.StateError, Days: -1}:  "Error",
		{State: activity.StateStopped, Days: 3}: "Tracking stopped",
	} {
		if got := stateText(res); got != want {
			t.Errorf("stateText(%v) = %q, attendu %q", res, got, want)
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
		t.Error("initial : mauvais résultat")
	}
	if avatarURL("https://avatars.githubusercontent.com/u/1?v=4") == "" || avatarURL("https://evil.example.test/a.png") != "" ||
		avatarURL("javascript:alert(1)") != "" || avatarURL(`https://avatars.githubusercontent.com/u/1"onerror=x`) != "" {
		t.Error("avatarURL : filtrage incorrect")
	}
	if pluralize(1, "commit", "commits") != "1 commit" || pluralize(3, "commit", "commits") != "3 commits" ||
		pluralize(0, "commit", "commits") != "0 commit" {
		t.Error("plural : mauvais accord")
	}
	for d, want := range map[time.Duration]string{0: "manuelle seule", 30 * time.Minute: "30 min", time.Hour: "1 h",
		90 * time.Minute: "1 h 30"} {
		if got := formatInterval(d); got != want {
			t.Errorf("interval(%s) = %q, attendu %q", d, got, want)
		}
	}
	if percent(1, 3) != 33 || percent(5, 0) != 0 || percent(7, 5) != 100 {
		t.Error("percent : mauvais calcul")
	}
	if m, err := dict("A", 1, "B", "x"); err != nil || m["A"] != 1 || m["B"] != "x" {
		t.Errorf("dict = %v, %v", m, err)
	}
	if _, err := dict("A"); err == nil {
		t.Error("dict impair accepté")
	}
	if _, err := dict(1, 2); err == nil {
		t.Error("dict avec une clé non chaîne accepté")
	}
	if repoPath(12) != "/repos/12" {
		t.Error("repoPath")
	}
	if got, err := withQuery("/repos/3/commits", "branch", "a&b #c", "author", "", "page", 2); err != nil ||
		got != "/repos/3/commits?branch=a%26b+%23c&page=2" {
		t.Errorf("withQuery = %q, %v", got, err)
	}
	if got, _ := withQuery("/repos/3", "page", 0); got != "/repos/3" {
		t.Errorf("withQuery sans paramètre = %q", got)
	}
	if _, err := withQuery("/x", "k"); err == nil {
		t.Error("withQuery impair accepté")
	}
	if _, err := withQuery("/x", "k", 1.5); err == nil {
		t.Error("withQuery avec un flottant accepté")
	}
}

func TestTimeFuncsUseDisplayZone(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("fuseau Europe/Paris indisponible")
	}
	e := newTestEnv(t)
	e.server.loc = paris
	f := e.server.funcs()
	at := time.Date(2026, 10, 27, 13, 5, 0, 0, time.UTC)
	if got := f["localTime"].(func(time.Time) string)(at); got != "27/10/2026 à 14:05" {
		t.Errorf("localTime = %q", got)
	}
	if got := f["localClock"].(func(time.Time) string)(at); got != "14:05" {
		t.Errorf("localClock = %q", got)
	}
	if got := f["relTime"].(func(time.Time) string)(testNow.Add(-2 * time.Hour)); got != "2 h ago" {
		t.Errorf("relTime = %q", got)
	}
	if got := f["localTime"].(func(time.Time) string)(time.Time{}); got != "" {
		t.Errorf("localTime(zéro) = %q", got)
	}
	if isoTime(at) != "2026-10-27T13:05:00Z" || isoTime(time.Time{}) != "" {
		t.Error("isoTime")
	}
}

// TestJSONScriptEscaping fixe le contrat des données passées au JavaScript : dans un
// <script type="application/json">, html/template encode la valeur en JSON et neutralise toute
// fermeture de balise.
func TestJSONScriptEscaping(t *testing.T) {
	tmpl := template.Must(template.New("").Parse(`<script type="application/json" id="d">{{.}}</script>`))
	var buf bytes.Buffer
	data := []activity.DayCount{{Date: "</script><script>alert(1)</script>", Count: 2}}
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Count(out, "</script>") != 1 || !strings.Contains(out, `"count":2`) {
		t.Errorf("JSON mal échappé : %s", out)
	}
}

func TestPartials(t *testing.T) {
	e := newTestEnv(t)
	set := e.server.pages[pageErrors]
	run := func(name string, data any) string {
		t.Helper()
		var buf bytes.Buffer
		if err := set.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("%s : %v", name, err)
		}
		return buf.String()
	}
	if out := run("state-badge", activity.Result{State: activity.StateFlemme, Days: 9}); !strings.Contains(out, "badge-warning") ||
		!strings.Contains(out, "Slacking · 9 j") {
		t.Errorf("state-badge : %s", out)
	}
	if out := run("rel-time", testNow.Add(-5*time.Minute)); !strings.Contains(out, "5 min ago") ||
		!strings.Contains(out, `datetime="2026-10-27T09:55:00Z"`) || !strings.Contains(out, `title="27/10/2026 à 09:55"`) {
		t.Errorf("rel-time : %s", out)
	}
	if out := run("rel-time", time.Time{}); !strings.Contains(out, "jamais") {
		t.Errorf("rel-time zéro : %s", out)
	}
	out := run("avatar", map[string]any{"Name": "alice", "URL": "https://avatars.githubusercontent.com/u/10?v=4"})
	for _, want := range []string{`referrerpolicy="no-referrer"`, `loading="lazy"`, `src="https://avatars.githubusercontent.com/u/10?v=4"`} {
		if !strings.Contains(out, want) {
			t.Errorf("avatar sans %q : %s", want, out)
		}
	}
	if out := run("avatar", map[string]any{"Name": "bob", "URL": "https://evil.example.test/p.png"}); strings.Contains(out, "<img") ||
		!strings.Contains(out, ">B<") {
		t.Errorf("avatar de repli : %s", out)
	}
	page := e.server.page(e.request(http.MethodGet, "/", nil), "", "", nil)
	page.CSRFToken = "jeton"
	if out := run("sync-repo-button", map[string]any{"Page": page, "RepoID": int64(7)}); !strings.Contains(out, `hx-post="/repos/7/sync"`) ||
		!strings.Contains(out, `value="jeton"`) {
		t.Errorf("sync-repo-button : %s", out)
	}
	if out := run("sync-all-button", page); !strings.Contains(out, `action="/sync"`) || !strings.Contains(out, `name="csrf_token"`) {
		t.Errorf("sync-all-button : %s", out)
	}
	if out := run("flash", map[string]any{"Message": "<b>x</b>", "Error": true}); !strings.Contains(out, "alert-error") ||
		strings.Contains(out, "<b>") {
		t.Errorf("flash : %s", out)
	}
}
