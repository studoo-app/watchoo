package web

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// isolateShare isole le test de l'environnement du développeur (HOME, XDG_CONFIG_HOME, WATCHOO_*) :
// les réglages de l'espace sont relus sur le disque à chaque requête de partage.
func isolateShare(t *testing.T) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("HOME", filepath.Join(base, "home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "xdg"))
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, workspace.EnvPrefix+"_") {
			t.Setenv(name, "")
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// withShareWorkspace crée un espace « licence-projet » (workspace.Init) et le donne au serveur de e
// (Options.Workspace). La base reste celle de l'environnement de test.
func (e *testEnv) withShareWorkspace() *workspace.Workspace {
	e.t.Helper()
	root := filepath.Join(e.t.TempDir(), testWorkspace)
	if err := os.Mkdir(root, 0o755); err != nil {
		e.t.Fatal(err)
	}
	res, err := workspace.Init(root)
	if err != nil {
		e.t.Fatal(err)
	}
	if err := workspace.Load(res.Workspace); err != nil {
		e.t.Fatal(err)
	}
	e.server.ws = res.Workspace
	return res.Workspace
}

// readFile lit path (échec du test sinon).
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// postMultipart envoie un formulaire multipart valide (même origine, cookie et jeton CSRF si csrf) :
// fields en champs simples, file (s'il n'est pas nil) dans le champ « file ».
func (e *testEnv) postMultipart(target string, fields map[string]string, file []byte, csrf bool) *httptest.ResponseRecorder {
	e.t.Helper()
	ck := e.csrfCookie()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if csrf {
		_ = mw.WriteField(CSRFField, ck.Value)
	}
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if file != nil {
		fw, err := mw.CreateFormFile(fieldImportFile, "../../licence.json")
		if err != nil {
			e.t.Fatal(err)
		}
		_, _ = fw.Write(file)
	}
	if err := mw.Close(); err != nil {
		e.t.Fatal(err)
	}
	r := e.request(http.MethodPost, target, &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set("Origin", testOrigin)
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.AddCookie(ck)
	return e.do(r)
}

// shareDoc est un document de partage valide : ajoute promo/eve, renomme Alice, change les branches de
// Bob, reprend Dave (arrêté par le test), garde Carol ; change sync.interval.
const shareDoc = `{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-20T08:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {"name": "promo <b>2026</b>"},
  "settings": {"sync": {"interval": "1h"}},
  "repositories": [
    {"provider": "github", "repo": "promo/eve", "label": "Ève <i>Leroy</i>", "branches": []},
    {"repo": "promo/alice", "label": "Alice M.", "branches": null},
    {"repo": "promo/bob", "label": "", "branches": ["main", "dev"]},
    {"repo": "promo/carol", "label": "", "branches": []},
    {"repo": "promo/dave"}
  ]
}`

// snapshot est l'état observable de l'espace : config.yaml et dépôts en base.
type snapshot struct {
	config string
	repos  string
}

func (e *testEnv) snapshot(ws *workspace.Workspace) snapshot {
	e.t.Helper()
	repos, err := e.store.ListRepositories(e.t.Context())
	if err != nil {
		e.t.Fatal(err)
	}
	b, err := json.Marshal(repos)
	if err != nil {
		e.t.Fatal(err)
	}
	return snapshot{config: string(readFile(e.t, ws.ConfigPath())), repos: string(b)}
}

func TestSettingsPageShareSection(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.withShareWorkspace()
	w := e.get("/settings")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Partager la configuration", `href="/settings/export"`,
		`action="/settings/import" enctype="multipart/form-data"`, `name="file"`)
	assertNotContains(t, w, "config.yaml a changé depuis le lancement")
}

func TestSettingsExport(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ids := e.seed()
	ws := e.withShareWorkspace()
	if err := e.store.SetRepositoryEnabled(t.Context(), ids.Dave, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.store.UpdateRepositorySettings(t.Context(), ids.Bob, "", []string{"main", "release/*"}); err != nil {
		t.Fatal(err)
	}
	before := e.snapshot(ws)

	w := e.get("/settings/export")
	assertStatus(t, w, http.StatusOK)
	h := w.Header()
	if got := h.Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := h.Get("Content-Disposition"); got != `attachment; filename="watchoo-config-licence-projet.json"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got := h.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q", got)
	}
	body := w.Body.String()
	for _, bad := range []string{"gh_path", `"github": `, `"web": `, ws.Root, "@example"} {
		if strings.Contains(body, bad) {
			t.Errorf("export avec %q :\n%s", bad, body)
		}
	}
	var doc configshare.Document
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("export illisible : %v", err)
	}
	if doc.Format != configshare.Format || doc.Version != configshare.Version || doc.WatchooVersion != "v0.0.0-test" ||
		!doc.ExportedAt.Equal(testNow) || doc.Workspace.Name != testWorkspace {
		t.Errorf("en-tête du document : %+v", doc)
	}
	var names []string
	for _, r := range doc.Repositories {
		names = append(names, r.Repo)
	}
	if got := strings.Join(names, ","); got != "promo/alice,promo/bob,promo/carol" {
		t.Errorf("dépôts exportés = %s (Dave est arrêté)", got)
	}
	if doc.Repositories[0].Label != "Alice Martin" || strings.Join(doc.Repositories[1].Branches, ",") != "main,release/*" {
		t.Errorf("dépôts : %+v", doc.Repositories)
	}
	if doc.Settings == nil || doc.Settings.Sync == nil || *doc.Settings.Sync.Interval != "30m" {
		t.Errorf("réglages : %+v", doc.Settings)
	}
	// Le document exporté se relit sans erreur.
	if _, err := configshare.ParseBytes(w.Body.Bytes()); err != nil {
		t.Errorf("ParseBytes(export) : %v", err)
	}
	// GET et HEAD sans effet de bord.
	head := e.do(e.request(http.MethodHead, "/settings/export", nil))
	assertStatus(t, head, http.StatusOK)
	if after := e.snapshot(ws); after != before {
		t.Error("l'export a modifié l'espace")
	}
}

func TestSettingsShareUnavailable(t *testing.T) {
	e := newTestEnv(t) // sans Options.Workspace
	assertStatus(t, e.get("/settings/export"), http.StatusServiceUnavailable)
	assertStatus(t, e.postMultipart("/settings/import", nil, []byte(shareDoc), true), http.StatusServiceUnavailable)
	assertStatus(t, e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: shareDoc}, nil, true),
		http.StatusServiceUnavailable)
}

func TestSettingsImportPreview(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ids := e.seed()
	ws := e.withShareWorkspace()
	if err := e.store.SetRepositoryEnabled(t.Context(), ids.Dave, false); err != nil {
		t.Fatal(err)
	}
	before := e.snapshot(ws)

	w := e.postMultipart("/settings/import", nil, []byte(shareDoc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w,
		"rien n'est encore appliqué",
		"Fichier : licence.json",        // nom sans chemin
		"promo &lt;b&gt;2026&lt;/b&gt;", // textes du document échappés
		"Ève &lt;i&gt;Leroy&lt;/i&gt;",  //
		"1 à ajouter", "3 à mettre à jour", "dont 1 à reprendre", "1 inchangés",
		"promo/eve", "à ajouter", "à reprendre", "à mettre à jour",
		"Alice Martin</span> → Alice M.",         // label avant → après
		"toutes les branches</span> → main, dev", // branches de Bob
		"sync.interval", "30m</span> → <strong>1h", // réglage modifié
		"commentaires personnels", // avertissement comments_lost
		`action="/settings/import/apply" enctype="multipart/form-data"`,
		`name="document" value="{`)
	assertNotContains(t, w, "<b>2026</b>", "<i>Leroy</i>", "../")
	// Aperçu sans écriture : base et config.yaml identiques, pas de .bak.
	if after := e.snapshot(ws); after != before {
		t.Error("l'aperçu a modifié l'espace")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, workspace.BackupFileName)); !os.IsNotExist(err) {
		t.Errorf("config.yaml.bak créé par l'aperçu : %v", err)
	}
}

func TestSettingsImportPreviewNothingToDo(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	e.withShareWorkspace()
	doc := `{"format":"watchoo-config","version":1,"settings":{"sync":{"interval":"30m"}},
	  "repositories":[{"repo":"promo/carol"}]}`
	w := e.postMultipart("/settings/import", nil, []byte(doc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Rien à appliquer", "identiques à ceux de cet espace", "Dépôts inchangés (1)")
	assertNotContains(t, w, "/settings/import/apply")
}

func TestSettingsImportEnvOverride(t *testing.T) {
	isolateShare(t)
	t.Setenv(workspace.EnvVarName(workspace.KeySyncInterval), "2h")
	e := newTestEnv(t)
	e.withShareWorkspace()
	doc := `{"format":"watchoo-config","version":1,"settings":{"sync":{"interval":"1h"}}}`
	w := e.postMultipart("/settings/import", nil, []byte(doc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "masqué par la variable "+workspace.EnvVarName(workspace.KeySyncInterval), "30m</span> → <strong>1h") // valeur du fichier, pas celle de la variable
}

func TestSettingsImportRefused(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	ws := e.withShareWorkspace()
	before := e.snapshot(ws)
	cases := []struct {
		name, doc, want string
		status          int
	}{
		{"json", `{"format":`, "invalide", http.StatusBadRequest},
		{"format", `{"version":1}`, "pas une configuration Watchoo", http.StatusBadRequest},
		{"version", `{"format":"watchoo-config","version":2}`, "version plus récente", http.StatusBadRequest},
		{"clé inconnue", `{"format":"watchoo-config","version":1,"settings":{"github":{"gh_path":"/tmp/x"}}}`, "github",
			http.StatusBadRequest},
		{"label", `{"format":"watchoo-config","version":1,"repositories":[{"repo":"promo/x","label":"a\u0007b"}]}`, "invalide",
			http.StatusBadRequest},
		{"doublon", `{"format":"watchoo-config","version":1,"repositories":[{"repo":"promo/X"},{"repo":"PROMO/x"}]}`, "invalide",
			http.StatusBadRequest},
		{"réglages", `{"format":"watchoo-config","version":1,"settings":{"activity":{"dead_after_days":1}}}`, "réglages refusés",
			http.StatusBadRequest},
		{"provider", `{"format":"watchoo-config","version":1,"repositories":[{"provider":"gitlab","repo":"promo/x"}]}`, "invalide",
			http.StatusBadRequest},
		{"trop gros", `{"format":"watchoo-config","version":1,"workspace":{"name":"` + strings.Repeat("a", configshare.MaxFileSize) + `"}}`,
			"trop gros", http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := e.postMultipart("/settings/import", nil, []byte(c.doc), true)
			assertStatus(t, w, c.status)
			assertContains(t, w, "Fichier refusé", c.want, "n&#39;a été importé", `name="file"`)
			assertNotContains(t, w, `name="document"`, "/settings/import/apply")
		})
	}
	// Aucun fichier.
	w := e.postMultipart("/settings/import", nil, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Aucun fichier reçu")
	// Formulaire classique (sans multipart) : aucun fichier non plus.
	w = e.post("/settings/import", url.Values{}, false)
	assertStatus(t, w, http.StatusBadRequest)
	if after := e.snapshot(ws); after != before {
		t.Error("un fichier refusé a modifié l'espace")
	}
}

func TestSettingsImportApply(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ids := e.seed()
	ws := e.withShareWorkspace()
	if err := e.store.SetRepositoryEnabled(t.Context(), ids.Dave, false); err != nil {
		t.Fatal(err)
	}
	e.fake.AddRepository(provider.Repository{ID: 5, Owner: "promo", Name: "eve", DefaultBranch: "main",
		URL: "https://github.com/promo/eve", Private: true})
	oldConfig := readFile(t, ws.ConfigPath())

	w := e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: shareDoc}, nil, true)
	assertRedirect(t, w, w.Code, "/settings?added=1&already=1&msg=imported&reenabled=1&updated=3&written=1")

	// Réglages : config.yaml réécrit (0600), ancien fichier en .bak (0600), valeurs relues.
	info, err := os.Stat(ws.ConfigPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.yaml : %v, droits %v", err, info.Mode().Perm())
	}
	bak := filepath.Join(ws.Dir, workspace.BackupFileName)
	if info, err := os.Stat(bak); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.yaml.bak : %v", err)
	}
	if !bytes.Equal(readFile(t, bak), oldConfig) {
		t.Error("config.yaml.bak différent de l'ancien config.yaml")
	}
	reloaded := &workspace.Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := workspace.Load(reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Config.Sync.Interval.String() != "1h0m0s" {
		t.Errorf("sync.interval relu = %v", reloaded.Config.Sync.Interval)
	}
	// La configuration du processus ne change pas à chaud.
	if e.server.cfg.Sync.Interval.String() != "30m0s" || e.server.ws.Config.Sync.Interval.String() != "30m0s" {
		t.Error("la configuration du processus a changé")
	}

	// Dépôts : ajout, mises à jour, reprise ; rien n'est retiré.
	if n := e.repoCount(); n != 5 {
		t.Errorf("%d dépôts, attendu 5", n)
	}
	if r := e.mustRepo(ids.Alice); r.Label != "Alice M." {
		t.Errorf("label d'Alice = %q", r.Label)
	}
	if r := e.mustRepo(ids.Bob); strings.Join(r.BranchPatterns, ",") != "main,dev" {
		t.Errorf("branches de Bob = %v", r.BranchPatterns)
	}
	if r := e.mustRepo(ids.Dave); !r.Enabled {
		t.Error("le suivi de Dave n'a pas repris")
	}

	// Bilan affiché sur /settings.
	page := e.get(w.Header().Get("Location"))
	assertStatus(t, page, http.StatusOK)
	assertContains(t, page, "Configuration importée : 1 dépôt ajouté, 3 dépôts mis à jour (dont 1 repris), 1 dépôt déjà suivi.",
		"config.yaml est réécrit", "prochain lancement")
	assertNotContains(t, page, "alert-error")
	// La ligne « Fichier config.yaml » montre la valeur relue sur le disque (1h), pas celle du
	// lancement (30m), et signale qu'elle ne vaut qu'au prochain lancement.
	assertContains(t, page, "Fichier config.yaml : 1 h.",
		"config.yaml a changé depuis le lancement de « watchoo serve » : cette valeur vaudra au prochain lancement.")

	// Un second import du même fichier ne change plus rien (réglages relus sur le disque).
	w = e.postMultipart("/settings/import", nil, []byte(shareDoc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Rien à appliquer")
}

func TestSettingsImportApplyInaccessible(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ids := e.seed()
	ws := e.withShareWorkspace()
	oldConfig := readFile(t, ws.ConfigPath())
	doc := `{"format":"watchoo-config","version":1,"repositories":[
	  {"repo":"promo/inconnu","label":"Zoé"},{"repo":"promo/alice","label":"Alice B."}]}`
	w := e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: doc}, nil, true)
	assertRedirect(t, w, w.Code, "/settings?inaccessible=1&msg=imported&updated=1")
	if r := e.mustRepo(ids.Alice); r.Label != "Alice B." {
		t.Errorf("label d'Alice = %q (un dépôt inaccessible ne bloque pas les autres)", r.Label)
	}
	if !bytes.Equal(readFile(t, ws.ConfigPath()), oldConfig) {
		t.Error("config.yaml réécrit sans réglage dans le document")
	}
	page := e.get(w.Header().Get("Location"))
	assertContains(t, page, "alert-error", "1 dépôt inaccessible", "importez de nouveau le fichier")
	if !strings.Contains(e.logs.String(), "promo/inconnu") {
		t.Error("dépôt inaccessible absent du journal")
	}
}

func TestSettingsImportApplyRevalidates(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	ws := e.withShareWorkspace()
	before := e.snapshot(ws)

	// Document modifié après l'aperçu, devenu invalide : refusé, rien n'est modifié.
	tampered := strings.Replace(shareDoc, `"interval": "1h"`, `"interval": "1h", "gh_path": "/tmp/evil"`, 1)
	w := e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: tampered}, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Fichier refusé", "gh_path")
	tampered = strings.Replace(shareDoc, `"interval": "1h"`, `"interval": "1m"`, 1)
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: tampered}, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "réglages refusés")
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: "  "}, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "Aucun document à appliquer")
	if after := e.snapshot(ws); after != before {
		t.Error("un document refusé a modifié l'espace")
	}

	// Document modifié mais valide : traité comme un nouveau (ce qu'il contient, pas l'aperçu).
	e.fake.AddRepository(provider.Repository{ID: 6, Owner: "promo", Name: "frank", DefaultBranch: "main",
		URL: "https://github.com/promo/frank", Private: true})
	other := `{"format":"watchoo-config","version":1,"repositories":[{"repo":"promo/frank","label":"Frank"}]}`
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: other}, nil, true)
	assertRedirect(t, w, w.Code, "/settings?added=1&msg=imported")
	if n := e.repoCount(); n != 5 {
		t.Errorf("%d dépôts, attendu 5", n)
	}
	if after := readFile(t, ws.ConfigPath()); string(after) != before.config {
		t.Error("config.yaml réécrit sans réglage dans le document")
	}
}

func TestSettingsImportRequiresCSRF(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	ws := e.withShareWorkspace()
	before := e.snapshot(ws)
	for _, path := range []string{"/settings/import", "/settings/import/apply"} {
		w := e.postMultipart(path, map[string]string{fieldImportDocument: shareDoc}, []byte(shareDoc), false)
		assertStatus(t, w, http.StatusForbidden)
	}
	if after := e.snapshot(ws); after != before {
		t.Error("une requête sans jeton CSRF a modifié l'espace")
	}
	// GET sur les routes d'import : refusé (aucune action en GET).
	assertStatus(t, e.get("/settings/import/apply"), http.StatusMethodNotAllowed)
}

func TestSettingsImportBusy(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	ws := e.withShareWorkspace()
	before := e.snapshot(ws)
	importMu.Lock()
	w := e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: shareDoc}, nil, true)
	importMu.Unlock()
	assertStatus(t, w, http.StatusConflict)
	if after := e.snapshot(ws); after != before {
		t.Error("un import refusé (déjà en cours) a modifié l'espace")
	}
}

func TestSettingsImportFlash(t *testing.T) {
	e := newTestEnv(t)
	cases := []struct{ query, want, absent string }{
		{"msg=imported", "Configuration importée : rien n&#39;a changé.", "alert-error"},
		{"msg=imported&written=1", "Configuration importée. config.yaml est réécrit", "alert-error"},
		{"msg=imported&added=2&errors=1", "2 dépôts ajoutés.", ""},
		// Valeurs forgées : bornées, jamais recopiées.
		{"msg=imported&added=99999999&updated=%3Cscript%3E&reenabled=-3", "1000 dépôts ajoutés.", "&lt;script"},
	}
	for _, c := range cases {
		w := e.get("/settings?" + c.query)
		assertStatus(t, w, http.StatusOK)
		assertContains(t, w, c.want)
		if c.absent != "" {
			assertNotContains(t, w, c.absent)
		}
	}
	assertContains(t, e.get("/settings?msg=imported&errors=1"), "alert-error", "1 dépôt en erreur")
}
