package cli

// Test de bout en bout du partage de la configuration : un espace A exporte (CLI), un espace B
// importe (CLI : aperçu --dry-run sans écriture, puis --yes), un espace C importe le même fichier par
// l'interface web (« watchoo serve » réel, aperçu puis « Appliquer »). Vrai accès à gh (faux binaire),
// vrai provider GitHub branché sur githubtest, vrai moteur, bases SQLite temporaires. Aucun réseau
// hors de 127.0.0.1 ; données factices.

import (
	"bytes"
	"encoding/json"
	"html"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

func TestEndToEndConfigShare(t *testing.T) {
	clk := &e2eClock{t: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	srv := githubtest.NewServer(t, githubtest.Options{Now: clk.Now})
	t.Cleanup(srv.Close)

	hA := newHarness(t)
	useFakeGH(t, hA, "logged-in")
	hA.Now = clk.Now
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	hA.NewProvider = func(token string) (provider.Provider, error) {
		return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
			UserAgent: "watchoo/test", Logger: slog.Default()})
	}
	alice := githubtest.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh"}
	for i, owner := range []string{"alice", "bob"} {
		id := int64(i + 1)
		srv.AddRepo(githubtest.Repo{ID: id, Owner: owner, Name: "projet", Private: true})
		srv.SetBranch(id, "main", srv.AddCommit(id, githubtest.Commit{Message: "e2e-share-" + owner,
			Author: alice, AuthoredAt: clk.Now().Add(-time.Hour)}))
	}
	srv.AddRepo(githubtest.Repo{ID: 3, Owner: "carol", Name: "cache", Private: true})
	srv.SetRepoAccess(3, githubtest.AccessHidden)

	// Espace A : deux dépôts, un réglage commun modifié, puis export.
	hA.run("init").mustSucceed(t)
	hA.run("repo", "add", "alice/projet", "--label", "Alice Martin", "--branch", "main,release/*").mustSucceed(t)
	hA.run("repo", "add", "bob/projet").mustSucceed(t)
	cfgA := filepath.Join(hA.cwd, ".watchoo", "config.yaml")
	writeFile(t, cfgA, readFile(t, cfgA)+"\nsync:\n  interval: 1h\nactivity:\n  flemme_after_days: 3\n")
	share := filepath.Join(t.TempDir(), "licence.json")
	hA.run("config", "export", "-o", share).mustSucceed(t)
	if info, err := os.Stat(share); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("fichier exporté : %v, %v", info, err)
	}
	exported := readFile(t, share)
	for _, bad := range []string{"alice@example.test", "gh_path", "\"web\"", "\"github\":", hA.cwd, githubtest.Token} {
		if strings.Contains(exported, bad) {
			t.Errorf("l'export contient %q :\n%s", bad, exported)
		}
	}
	// La personne qui partage ajoute à la main un dépôt qui sera inaccessible chez les autres.
	var doc map[string]any
	if err := json.Unmarshal([]byte(exported), &doc); err != nil {
		t.Fatal(err)
	}
	doc["repositories"] = append(doc["repositories"].([]any),
		map[string]any{"provider": "github", "repo": "carol/cache", "label": "", "branches": []any{}})
	edited, _ := json.MarshalIndent(doc, "", "  ")
	writeFile(t, share, string(edited))

	// Espace B : aperçu --dry-run (code 1 : un dépôt inaccessible), rien n'est écrit.
	hB := *hA
	hB.cwd = newDir(t, t.TempDir(), "promo-b")
	hB.run("init").mustSucceed(t)
	cfgB := filepath.Join(hB.cwd, ".watchoo", "config.yaml")
	dbB := filepath.Join(hB.cwd, ".watchoo", "watchoo.db")
	cfgBefore, dbBefore := readFile(t, cfgB), readFile(t, dbB)
	dry := hB.run("config", "import", share, "--dry-run", "--json")
	if dry.code != 1 || !strings.Contains(dry.stdout, `"dry_run": true`) {
		t.Fatalf("import --dry-run : code %d\n%s\n%s", dry.code, dry.stdout, dry.stderr)
	}
	var plan struct {
		Summary struct{ Added, Inaccessible int } `json:"summary"`
	}
	decodeLoose(t, dry.stdout, &plan)
	if plan.Summary.Added != 2 || plan.Summary.Inaccessible != 1 {
		t.Errorf("aperçu = %+v\n%s", plan.Summary, dry.stdout)
	}
	if readFile(t, cfgB) != cfgBefore || readFile(t, dbB) != dbBefore {
		t.Error("--dry-run a modifié config.yaml ou la base")
	}
	if _, err := os.Stat(cfgB + ".bak"); !os.IsNotExist(err) {
		t.Errorf("--dry-run a créé une sauvegarde : %v", err)
	}
	hB.run("config", "import", share).mustFail(t, "--yes") // hors terminal

	// Import --yes : réglages réécrits, dépôts ajoutés, code 1 pour le dépôt inaccessible.
	imp := hB.run("config", "import", share, "--yes")
	if imp.code != 1 || !strings.Contains(imp.stdout, "carol/cache") {
		t.Fatalf("import --yes : code %d\n%s\n%s", imp.code, imp.stdout, imp.stderr)
	}
	e2eAssertShared(t, &hB, cfgB, cfgBefore)
	if rep := e2eSync(t, &hB); rep.Summary.Success != 2 {
		t.Errorf("synchro après import = %+v", rep.Summary)
	}
	// Réimporter le même fichier ne change rien (hors dépôt inaccessible).
	again := hB.run("config", "import", share, "--yes", "--json")
	var againRep struct {
		Settings struct {
			Written bool `json:"written"`
		} `json:"settings"`
		Summary struct {
			AlreadyTracked int `json:"already_tracked"`
			Updated        int `json:"updated"`
		} `json:"summary"`
	}
	decodeLoose(t, again.stdout, &againRep)
	if againRep.Settings.Written || againRep.Summary.AlreadyTracked != 2 || againRep.Summary.Updated != 0 {
		t.Errorf("second import = %+v", againRep)
	}

	// Espace C : import par l'interface web d'un « watchoo serve » réel.
	hC := *hA
	hC.cwd = newDir(t, t.TempDir(), "promo-c")
	hC.run("init").mustSucceed(t)
	cfgC := filepath.Join(hC.cwd, ".watchoo", "config.yaml")
	cfgCBefore := readFile(t, cfgC)
	run := startServe(t, &hC)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	base := "http://" + run.addr
	page := e2eHTTP(t, client, http.MethodGet, base+"/settings", "", nil, http.StatusOK)
	token := e2eField(t, page, "csrf_token")
	preview := e2eMultipart(t, client, base+"/settings/import", map[string]string{"csrf_token": token},
		"file", "licence.json", edited, http.StatusOK)
	for _, want := range []string{"alice/projet", "bob/projet", "carol/cache", "Alice Martin"} {
		if !strings.Contains(preview, want) {
			t.Errorf("aperçu web sans %q", want)
		}
	}
	if readFile(t, cfgC) != cfgCBefore {
		t.Error("l'aperçu web a modifié config.yaml")
	}
	document := e2eField(t, preview, "document")
	e2eMultipart(t, client, base+"/settings/import/apply", map[string]string{"csrf_token": token,
		"document": document}, "", "", nil, http.StatusSeeOther)
	if r := run.stop(t); r.code != 0 {
		t.Fatalf("serve : code %d\n%s", r.code, r.stderr)
	}
	e2eAssertShared(t, &hC, cfgC, cfgCBefore)

	logs := testLogs.String()
	for _, bad := range []string{"alice@example.test", "e2e-share-alice", "e2e-share-bob", githubtest.Token} {
		if strings.Contains(logs, bad) {
			t.Errorf("donnée sensible %q dans les logs", bad)
		}
	}
}

// e2eAssertShared vérifie un espace après import : dépôts, réglages réécrits, sauvegarde en 0600.
func e2eAssertShared(t *testing.T, h *harness, cfgPath, oldCfg string) {
	t.Helper()
	var out struct {
		Repositories []struct {
			FullName       string   `json:"full_name"`
			Label          string   `json:"label"`
			BranchPatterns []string `json:"branch_patterns"`
		} `json:"repositories"`
	}
	decodeLoose(t, h.run("repo", "list", "--json").mustSucceed(t).stdout, &out)
	got := map[string]string{}
	for _, r := range out.Repositories {
		got[r.FullName] = r.Label + "|" + strings.Join(r.BranchPatterns, ",")
	}
	want := map[string]string{"alice/projet": "Alice Martin|main,release/*", "bob/projet": "|"}
	if len(got) != len(want) || got["alice/projet"] != want["alice/projet"] || got["bob/projet"] != want["bob/projet"] {
		t.Errorf("dépôts = %v, attendu %v", got, want)
	}
	cfg := readFile(t, cfgPath)
	for _, line := range []string{"interval: 1h", "flemme_after_days: 3"} {
		if !regexp.MustCompile(`\n  ` + regexp.QuoteMeta(line) + `\s`).MatchString(cfg) {
			t.Errorf("config.yaml sans %q :\n%s", line, cfg)
		}
	}
	for _, p := range []string{cfgPath, cfgPath + ".bak"} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s : %v, %v", p, info, err)
		}
	}
	if readFile(t, cfgPath+".bak") != oldCfg {
		t.Error("config.yaml.bak différent de l'ancien fichier")
	}
}

func e2eHTTP(t *testing.T, c *http.Client, method, url, ctype string, body []byte, status int) string {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatalf("%s %s : %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		t.Fatalf("%s %s : %d, attendu %d\n%s", method, url, resp.StatusCode, status, b)
	}
	if strings.Contains(string(b), githubtest.Token) {
		t.Errorf("token dans la réponse à %s", url)
	}
	return string(b)
}

func e2eMultipart(t *testing.T, c *http.Client, url string, fields map[string]string, fileField, fileName string,
	file []byte, status int) string {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	if fileField != "" {
		fw, err := mw.CreateFormFile(fileField, fileName)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fw.Write(file)
	}
	_ = mw.Close()
	return e2eHTTP(t, c, http.MethodPost, url, mw.FormDataContentType(), buf.Bytes(), status)
}

// e2eField renvoie la valeur (décodée) du premier champ de formulaire name de la page.
func e2eField(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("champ %s absent de la page", name)
	}
	return html.UnescapeString(m[1])
}
