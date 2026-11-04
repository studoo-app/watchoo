package web

// Language of the pages: the choice of the browser (EN / FR selector, cookie), else the lang setting
// of the workspace (English by default, French), read at each request, and the completeness of the texts of the templates in the i18n catalog.

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestFrenchPages renders the main pages of a workspace with lang: fr, the process language staying
// English: the pages follow the workspace, not the process.
func TestFrenchPages(t *testing.T) {
	e := newTestEnv(t, withLang("fr"))
	ids := e.seed()

	w := e.get("/")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<html lang="fr">`,
		"<title>Tableau de bord · "+testWorkspace+" · Watchoo</title>",
		`aria-current="page">Tableau de bord`, "Derniers commits", "Flemme · 9 j", "Dead · 40 j",
		"Jamais synchronisé", "il y a", `title="26/10/2026 à 10:00"`, "sur 4 dépôts",
		"Aucun commit connu : pas encore synchronisé.", "Synchroniser maintenant",
		`"locale":"fr-FR"`, `"week":"Semaine du {start} au {end}"`)
	assertNotContains(t, w, "Latest commits", "Slacking", "ago<", `lang="en"`)

	w = e.get(repoPath(ids.Alice))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<html lang="fr">`, ">Actif<", "Activité sur 90 jours", "Historique",
		"Synchroniser ce dépôt", "Seuils : Flemme après 5 jours (de l&#39;espace)", `title="Voir le commit sur GitHub"`,
		"Activité par contributeur sur 12 semaines", "depuis le 10/08/2026")

	w = e.get(repoPath(ids.Dave))
	assertContains(t, w, "Jamais synchronisé", "0 commit en base", ">jamais<")

	w = e.get("/page-inconnue")
	assertStatus(t, w, http.StatusNotFound)
	assertContains(t, w, `<html lang="fr">`, "<title>Page introuvable · "+testWorkspace+" · Watchoo</title>",
		"Cette page n&#39;existe pas", "Retour au tableau de bord")
	w = e.htmx("/page-inconnue")
	assertContains(t, w, "<strong>Page introuvable</strong> : Cette page")

	w = e.get("/settings?msg=saved&repo=1")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<html lang="fr">`, `aria-current="page">Réglages`, "Ajouter un dépôt",
		"Intervalle en vigueur : <strong>manuelle seule</strong>", "Partager la configuration",
		"toutes les branches", "Le dépôt « Alice Martin » est modifié", "Flemme · 9 j")

	w = e.get("/contributors")
	// The series grouped in "Other contributors" only exists with more than 8 contributors: its
	// French name is checked with the chart functions (charts_test.go).
	assertContains(t, w, `<html lang="fr">`, "Contributeurs", "5 commits de 3 contributeurs depuis le 10/08/2026",
		"Dernière activité")

	// Startup errors and other process messages stay in the process language (English here).
	if _, err := newServer(Options{}); err == nil || !strings.Contains(err.Error(), "database not provided") {
		t.Errorf("newServer without store = %v, want an English error (process language)", err)
	}
}

// TestFrenchFlashAndErrors checks a few messages built by the handlers in French.
func TestFrenchFlashAndErrors(t *testing.T) {
	e := newTestEnv(t, withLang("fr"))
	ids := e.seed()
	w := e.post("/sync", nil, true)
	assertContains(t, w, "Synchro lancée.")
	e.waitSyncIdle()
	w = e.post("/settings/interval", map[string][]string{"interval": {"abc"}}, false)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "intervalle &#34;abc&#34; illisible")
	w = e.post(repoPath(ids.Bob)+"/thresholds", map[string][]string{"flemme": {"-3"}}, false)
	assertContains(t, w, "Seuils refusés : Flemme : ")
	assertContains(t, e.get("/settings?msg=imported&added=2&errors=1"), "Configuration importée : 2 dépôts ajoutés.",
		"1 dépôt en erreur")
	assertStatus(t, e.get("/sync"), http.StatusMethodNotAllowed)
	assertContains(t, e.get("/sync"), "Action impossible")
}

// TestLangFollowsConfigFile checks that the lang setting is read again from config.yaml when it
// changes: no restart of "watchoo serve" is needed.
func TestLangFollowsConfigFile(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ws := e.withShareWorkspace()
	assertContains(t, e.get("/"), `<html lang="en">`, "Dashboard")

	content := string(readFile(t, ws.ConfigPath())) + "lang: fr\n"
	if err := os.WriteFile(ws.ConfigPath(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	assertContains(t, e.get("/"), `<html lang="fr">`, "Tableau de bord")

	// An invalid file keeps the last known language.
	if err := os.WriteFile(ws.ConfigPath(), []byte(content+"lang: de\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertContains(t, e.get("/"), `<html lang="fr">`)

	// WATCHOO_LANG overrides the file, as for the CLI.
	if err := os.WriteFile(ws.ConfigPath(), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_LANG", "en")
	e.server.lang.invalidate()
	assertContains(t, e.get("/"), `<html lang="en">`)
}

// TestImportChangesLang checks that a configuration import that changes lang changes the language
// of the next pages, without restarting the server.
func TestImportChangesLang(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	ws := e.withShareWorkspace()
	doc := `{"format":"watchoo-config","version":1,"settings":{"lang":"fr"}}`

	w := e.postMultipart("/settings/import", nil, []byte(doc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "lang", `&#34;en&#34;</span> → <strong>&#34;fr&#34;`, "Apply these changes")

	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: doc}, nil, true)
	assertRedirect(t, w, w.Code, "/settings?msg=imported&written=1")
	page := e.get(w.Header().Get("Location"))
	assertContains(t, page, `<html lang="fr">`, "Configuration importée.", "sauf la langue des pages")
	assertContains(t, e.get("/"), `<html lang="fr">`, "Tableau de bord")
	if !strings.Contains(string(readFile(t, ws.ConfigPath())), `lang: "fr"`) {
		t.Errorf("config.yaml without the imported language:\n%s", readFile(t, ws.ConfigPath()))
	}

	// And back to English.
	doc = `{"format":"watchoo-config","version":1,"settings":{"lang":"en"}}`
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: doc}, nil, true)
	assertStatus(t, w, http.StatusSeeOther)
	assertContains(t, e.get("/"), `<html lang="en">`, "Dashboard")
}

// templateKey finds the catalog keys used by the templates: {{t "key" …}}, {{tn "key" …}} and the
// same calls in parentheses.
var templateKey = regexp.MustCompile(`\((t|tn) "([a-z0-9_.]+)"|\{\{-?\s*(t|tn) "([a-z0-9_.]+)"`)

// TestTemplateKeysInCatalog checks that every key used by a template exists in the catalog, in both
// languages (a missing key would show the key itself).
func TestTemplateKeysInCatalog(t *testing.T) {
	keys := map[string]bool{}
	err := fs.WalkDir(templateFS, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := fs.ReadFile(templateFS, path)
		if err != nil {
			return err
		}
		for _, m := range templateKey.FindAllStringSubmatch(string(data), -1) {
			fn, key := m[1], m[2]
			if fn == "" {
				fn, key = m[3], m[4]
			}
			if fn == "tn" {
				keys[key+".one"], keys[key+".other"] = true, true
			} else {
				keys[key] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) < 100 {
		t.Fatalf("only %d keys found in the templates: the pattern no longer matches", len(keys))
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	for key := range keys {
		if got := en.T(key); got == key {
			t.Errorf("key %q missing from the catalog", key)
		} else if fr.T(key) == got && !sameInBothLanguages(got) {
			t.Errorf("key %q: French text missing or identical to the English one (%q)", key, got)
		}
	}
}

// sameInBothLanguages lists the texts identical in English and French on purpose.
func sameInBothLanguages(s string) bool {
	switch s {
	case "Commits", "Branches", "Pull requests", "Releases", "Tags", "Total", "Dead", "Action", "inaccessible",
		"%d commit", "%d commits", "%d min", "%d h", "{n} commit", "{n} commits", "Signal", "Commit", "Pull request":
		return true
	}
	return false
}

// langCookieOf returns the language cookie set by w, or nil.
func langCookieOf(w *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if strings.HasPrefix(c.Name, "watchoo_lang") {
			return c
		}
	}
	return nil
}

// TestLangSelector checks the EN / FR selector of the header: English by default, the choice is kept
// in a cookie of the browser (config.yaml untouched) and wins over the workspace setting.
func TestLangSelector(t *testing.T) {
	e := newTestEnv(t)
	e.seed()

	w := e.get("/contributors")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, `<html lang="en">`, `action="/lang"`, `name="return" value="/contributors"`,
		`value="en" class="btn btn-ghost btn-xs join-item btn-active"`, `>FR</button>`)

	w = e.post("/lang", url.Values{"lang": {"fr"}, "return": {"/contributors"}}, false)
	assertStatus(t, w, http.StatusSeeOther)
	if loc := w.Header().Get("Location"); loc != "/contributors" {
		t.Errorf("Location = %q, want /contributors", loc)
	}
	ck := langCookieOf(w)
	if ck == nil || ck.Value != "fr" || !ck.HttpOnly || ck.Path != "/" || ck.MaxAge <= 0 {
		t.Fatalf("language cookie = %+v", ck)
	}

	r := e.request(http.MethodGet, "/contributors", nil)
	r.AddCookie(&http.Cookie{Name: ck.Name, Value: ck.Value})
	w = e.do(r)
	assertContains(t, w, `<html lang="fr">`, "Contributeurs",
		`value="fr" class="btn btn-ghost btn-xs join-item btn-active"`)

	// An invalid cookie falls back to the workspace language.
	r = e.request(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: ck.Name, Value: "de"})
	assertContains(t, e.do(r), `<html lang="en">`)

	// Unknown language: 400, no cookie.
	w = e.post("/lang", url.Values{"lang": {"de"}}, false)
	assertStatus(t, w, http.StatusBadRequest)
	if langCookieOf(w) != nil {
		t.Error("cookie set for an unknown language")
	}

	// HTMX: HX-Redirect.
	w = e.post("/lang", url.Values{"lang": {"en"}, "return": {"/settings"}}, true)
	if got := w.Header().Get("HX-Redirect"); got != "/settings" {
		t.Errorf("HX-Redirect = %q", got)
	}
}

// TestLangSelectorWinsOverWorkspace: with lang: fr in the workspace, choosing EN shows English.
func TestLangSelectorWinsOverWorkspace(t *testing.T) {
	e := newTestEnv(t, withLang("fr"))
	assertContains(t, e.get("/"), `<html lang="fr">`)
	r := e.request(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: langCookieName(e.listenAddr), Value: "en"})
	assertContains(t, e.do(r), `<html lang="en">`, "Dashboard")
}

// TestLangSelectorCSRF: the selector is a POST protected by the CSRF token.
func TestLangSelectorCSRF(t *testing.T) {
	e := newTestEnv(t)
	r := e.request(http.MethodPost, "/lang", strings.NewReader("lang=fr"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := e.do(r)
	assertStatus(t, w, http.StatusForbidden)
	if langCookieOf(w) != nil {
		t.Error("cookie set without a CSRF token")
	}
}

func TestLocalReturnPath(t *testing.T) {
	for raw, want := range map[string]string{
		"":                      "/",
		"/":                     "/",
		"/repos/3":              "/repos/3",
		"/repos/3?author=x#top": "/repos/3",
		"//evil.example/x":      "/",
		"/\\evil.example":       "/",
		"https://evil.example/": "/",
		"settings":              "/",
		"/a\nb":                 "/",
		"/contributors/%2e%2e":  "/contributors/%2e%2e",
	} {
		if got := localReturnPath(raw); got != want {
			t.Errorf("localReturnPath(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestLangCookieName(t *testing.T) {
	if got := langCookieName("127.0.0.1:8421"); got != "watchoo_lang_8421" {
		t.Errorf("got %q", got)
	}
	if got := langCookieName("bad"); got != "watchoo_lang" {
		t.Errorf("got %q", got)
	}
}
