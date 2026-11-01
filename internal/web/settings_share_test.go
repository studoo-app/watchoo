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

// isolateShare isolates the test from the developer's environment (HOME, XDG_CONFIG_HOME,
// WATCHOO_*): the settings of the workspace are read again from the disk at each sharing request.
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

// withShareWorkspace creates a "licence-projet" workspace (workspace.Init) and gives it to the server
// of e (Options.Workspace), language source included. The database remains the one of the test
// environment.
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
	e.server.lang = newLangSource(res.Workspace, res.Workspace.Config.Lang)
	return res.Workspace
}

// readFile reads path (test failure otherwise).
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// postMultipart sends a valid multipart form (same origin, cookie and CSRF token if csrf): fields as
// simple fields, file (if not nil) in the "file" field.
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

// shareDoc is a valid sharing document: adds promo/eve, renames Alice, changes the branches of Bob,
// resumes Dave (stopped by the test), keeps Carol; changes sync.interval.
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

// snapshot is the observable state of the workspace: config.yaml and repositories in the database.
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
	assertContains(t, w, "Share the configuration", `href="/settings/export"`,
		`action="/settings/import" enctype="multipart/form-data"`, `name="file"`)
	assertNotContains(t, w, "config.yaml changed since")
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
			t.Errorf("export with %q:\n%s", bad, body)
		}
	}
	var doc configshare.Document
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unreadable export: %v", err)
	}
	if doc.Format != configshare.Format || doc.Version != configshare.Version || doc.WatchooVersion != "v0.0.0-test" ||
		!doc.ExportedAt.Equal(testNow) || doc.Workspace.Name != testWorkspace {
		t.Errorf("document header: %+v", doc)
	}
	var names []string
	for _, r := range doc.Repositories {
		names = append(names, r.Repo)
	}
	if got := strings.Join(names, ","); got != "promo/alice,promo/bob,promo/carol" {
		t.Errorf("exported repositories = %s (Dave is stopped)", got)
	}
	if doc.Repositories[0].Label != "Alice Martin" || strings.Join(doc.Repositories[1].Branches, ",") != "main,release/*" {
		t.Errorf("repositories: %+v", doc.Repositories)
	}
	if doc.Settings == nil || doc.Settings.Sync == nil || *doc.Settings.Sync.Interval != "30m" ||
		doc.Settings.Lang == nil || *doc.Settings.Lang != "en" {
		t.Errorf("settings: %+v", doc.Settings)
	}
	// The exported document is read back without error.
	if _, err := configshare.ParseBytes(w.Body.Bytes()); err != nil {
		t.Errorf("ParseBytes(export): %v", err)
	}
	// GET and HEAD without side effect.
	head := e.do(e.request(http.MethodHead, "/settings/export", nil))
	assertStatus(t, head, http.StatusOK)
	if after := e.snapshot(ws); after != before {
		t.Error("the export changed the workspace")
	}
}

func TestSettingsShareUnavailable(t *testing.T) {
	e := newTestEnv(t) // without Options.Workspace
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
		"nothing is applied yet",
		"File: licence.json",            // name without path
		"promo &lt;b&gt;2026&lt;/b&gt;", // texts of the document escaped
		"Ève &lt;i&gt;Leroy&lt;/i&gt;",  //
		"1 to add", "3 to update", "including 1 to resume", "1 unchanged",
		"promo/eve", "to add", "to resume", "to update",
		"Alice Martin</span> → Alice M.",           // label before → after
		"all branches</span> → main, dev",          // branches of Bob
		"sync.interval", "30m</span> → <strong>1h", // changed setting
		"personal comments", // comments_lost warning
		`action="/settings/import/apply" enctype="multipart/form-data"`,
		`name="document" value="{`)
	assertNotContains(t, w, "<b>2026</b>", "<i>Leroy</i>", "../")
	// Preview without writing: database and config.yaml identical, no .bak.
	if after := e.snapshot(ws); after != before {
		t.Error("the preview changed the workspace")
	}
	if _, err := os.Stat(filepath.Join(ws.Dir, workspace.BackupFileName)); !os.IsNotExist(err) {
		t.Errorf("config.yaml.bak created by the preview: %v", err)
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
	assertContains(t, w, "Nothing to apply", "identical to those of this workspace", "Unchanged repositories (1)")
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
	assertContains(t, w, "overridden by the "+workspace.EnvVarName(workspace.KeySyncInterval)+" variable", "30m</span> → <strong>1h",
		"the WATCHOO_SYNC_INTERVAL variable is set") // file value, not the variable's
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
		{"json", `{"format":`, "Invalid configuration file", http.StatusBadRequest},
		{"format", `{"version":1}`, "not a shared Watchoo configuration", http.StatusBadRequest},
		{"version", `{"format":"watchoo-config","version":2}`, "newer version", http.StatusBadRequest},
		{"unknown key", `{"format":"watchoo-config","version":1,"settings":{"github":{"gh_path":"/tmp/x"}}}`, "github",
			http.StatusBadRequest},
		{"label", `{"format":"watchoo-config","version":1,"repositories":[{"repo":"promo/x","label":"a\u0007b"}]}`, "Invalid configuration file",
			http.StatusBadRequest},
		{"duplicate", `{"format":"watchoo-config","version":1,"repositories":[{"repo":"promo/X"},{"repo":"PROMO/x"}]}`, "Invalid configuration file",
			http.StatusBadRequest},
		{"settings", `{"format":"watchoo-config","version":1,"settings":{"activity":{"dead_after_days":1}}}`, "settings refused",
			http.StatusBadRequest},
		{"language", `{"format":"watchoo-config","version":1,"settings":{"lang":"de"}}`, "settings.lang",
			http.StatusBadRequest},
		{"provider", `{"format":"watchoo-config","version":1,"repositories":[{"provider":"gitlab","repo":"promo/x"}]}`, "Invalid configuration file",
			http.StatusBadRequest},
		{"too large", `{"format":"watchoo-config","version":1,"workspace":{"name":"` + strings.Repeat("a", configshare.MaxFileSize) + `"}}`,
			"too large", http.StatusRequestEntityTooLarge},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := e.postMultipart("/settings/import", nil, []byte(c.doc), true)
			assertStatus(t, w, c.status)
			assertContains(t, w, "File refused", c.want, "othing was imported", `name="file"`)
			assertNotContains(t, w, `name="document"`, "/settings/import/apply")
		})
	}
	// No file.
	w := e.postMultipart("/settings/import", nil, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "No file received")
	// Classic form (without multipart): no file either.
	w = e.post("/settings/import", url.Values{}, false)
	assertStatus(t, w, http.StatusBadRequest)
	if after := e.snapshot(ws); after != before {
		t.Error("a refused file changed the workspace")
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

	// Settings: config.yaml rewritten (0600), old file in .bak (0600), values read back.
	info, err := os.Stat(ws.ConfigPath())
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.yaml: %v, permissions %v", err, info.Mode().Perm())
	}
	bak := filepath.Join(ws.Dir, workspace.BackupFileName)
	if info, err := os.Stat(bak); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("config.yaml.bak: %v", err)
	}
	if !bytes.Equal(readFile(t, bak), oldConfig) {
		t.Error("config.yaml.bak different from the old config.yaml")
	}
	reloaded := &workspace.Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := workspace.Load(reloaded); err != nil {
		t.Fatal(err)
	}
	if reloaded.Config.Sync.Interval.String() != "1h0m0s" {
		t.Errorf("sync.interval read back = %v", reloaded.Config.Sync.Interval)
	}
	// The configuration of the process does not change on the fly.
	if e.server.cfg.Sync.Interval.String() != "30m0s" || e.server.ws.Config.Sync.Interval.String() != "30m0s" {
		t.Error("the configuration of the process changed")
	}

	// Repositories: addition, updates, resumption; nothing is removed.
	if n := e.repoCount(); n != 5 {
		t.Errorf("%d repositories, want 5", n)
	}
	if r := e.mustRepo(ids.Alice); r.Label != "Alice M." {
		t.Errorf("label of Alice = %q", r.Label)
	}
	if r := e.mustRepo(ids.Bob); strings.Join(r.BranchPatterns, ",") != "main,dev" {
		t.Errorf("branches of Bob = %v", r.BranchPatterns)
	}
	if r := e.mustRepo(ids.Dave); !r.Enabled {
		t.Error("the tracking of Dave did not resume")
	}

	// Result shown on /settings.
	page := e.get(w.Header().Get("Location"))
	assertStatus(t, page, http.StatusOK)
	assertContains(t, page, "Configuration imported: 1 repository added, 3 repositories updated (including 1 resumed), 1 repository already tracked.",
		"config.yaml is rewritten", "next start")
	assertNotContains(t, page, "alert-error")
	// The "config.yaml file" line shows the value read back from the disk (1h), not the one of the
	// start (30m), and reports that it only applies at the next start.
	assertContains(t, page, "config.yaml file: 1 h.",
		"config.yaml changed since &#34;watchoo serve&#34; started: this value will apply at the next start.")

	// A second import of the same file changes nothing any more (settings read back from the disk).
	w = e.postMultipart("/settings/import", nil, []byte(shareDoc), true)
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "Nothing to apply")
}

func TestSettingsImportApplyInaccessible(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	ids := e.seed()
	ws := e.withShareWorkspace()
	oldConfig := readFile(t, ws.ConfigPath())
	doc := `{"format":"watchoo-config","version":1,"repositories":[
	  {"repo":"promo/unknown","label":"Zoé"},{"repo":"promo/alice","label":"Alice B."}]}`
	w := e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: doc}, nil, true)
	assertRedirect(t, w, w.Code, "/settings?inaccessible=1&msg=imported&updated=1")
	if r := e.mustRepo(ids.Alice); r.Label != "Alice B." {
		t.Errorf("label of Alice = %q (an inaccessible repository does not block the others)", r.Label)
	}
	if !bytes.Equal(readFile(t, ws.ConfigPath()), oldConfig) {
		t.Error("config.yaml rewritten without a setting in the document")
	}
	page := e.get(w.Header().Get("Location"))
	assertContains(t, page, "alert-error", "1 repository inaccessible", "import the file again")
	if !strings.Contains(e.logs.String(), "promo/unknown") {
		t.Error("inaccessible repository missing from the log")
	}
}

func TestSettingsImportApplyRevalidates(t *testing.T) {
	isolateShare(t)
	e := newTestEnv(t)
	e.seed()
	ws := e.withShareWorkspace()
	before := e.snapshot(ws)

	// Document changed after the preview, now invalid: refused, nothing is changed.
	tampered := strings.Replace(shareDoc, `"interval": "1h"`, `"interval": "1h", "gh_path": "/tmp/evil"`, 1)
	w := e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: tampered}, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "File refused", "gh_path")
	tampered = strings.Replace(shareDoc, `"interval": "1h"`, `"interval": "1m"`, 1)
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: tampered}, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "settings refused")
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: "  "}, nil, true)
	assertStatus(t, w, http.StatusBadRequest)
	assertContains(t, w, "No document to apply")
	if after := e.snapshot(ws); after != before {
		t.Error("a refused document changed the workspace")
	}

	// Document changed but valid: processed as a new one (what it contains, not the preview).
	e.fake.AddRepository(provider.Repository{ID: 6, Owner: "promo", Name: "frank", DefaultBranch: "main",
		URL: "https://github.com/promo/frank", Private: true})
	other := `{"format":"watchoo-config","version":1,"repositories":[{"repo":"promo/frank","label":"Frank"}]}`
	w = e.postMultipart("/settings/import/apply", map[string]string{fieldImportDocument: other}, nil, true)
	assertRedirect(t, w, w.Code, "/settings?added=1&msg=imported")
	if n := e.repoCount(); n != 5 {
		t.Errorf("%d repositories, want 5", n)
	}
	if after := readFile(t, ws.ConfigPath()); string(after) != before.config {
		t.Error("config.yaml rewritten without a setting in the document")
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
		t.Error("a request without CSRF token changed the workspace")
	}
	// GET on the import routes: refused (no action in GET).
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
		t.Error("a refused import (already running) changed the workspace")
	}
}

func TestSettingsImportFlash(t *testing.T) {
	e := newTestEnv(t)
	cases := []struct{ query, want, absent string }{
		{"msg=imported", "Configuration imported: nothing changed.", "alert-error"},
		{"msg=imported&written=1", "Configuration imported. config.yaml is rewritten", "alert-error"},
		{"msg=imported&added=2&errors=1", "2 repositories added.", ""},
		// Forged values: bounded, never copied.
		{"msg=imported&added=99999999&updated=%3Cscript%3E&reenabled=-3", "1000 repositories added.", "&lt;script"},
	}
	for _, c := range cases {
		w := e.get("/settings?" + c.query)
		assertStatus(t, w, http.StatusOK)
		assertContains(t, w, c.want)
		if c.absent != "" {
			assertNotContains(t, w, c.absent)
		}
	}
	assertContains(t, e.get("/settings?msg=imported&errors=1"), "alert-error", "1 repository in error")
}
