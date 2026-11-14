package cli

// End-to-end test of configuration sharing: a workspace A exports (CLI), a workspace B imports (CLI:
// --dry-run preview without writing, then --yes), a workspace C imports the same file through the web
// interface (real "watchoo serve", preview then "Apply"). Real gh access (fake binary), real GitHub
// provider wired to githubtest, real engine, temporary SQLite databases. No network outside
// 127.0.0.1; dummy data.

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

	// Workspace A: two repositories, a shared setting changed, then export.
	hA.run("init").mustSucceed(t)
	hA.run("repo", "add", "alice/projet", "--label", "Alice Martin", "--branch", "main,release/*").mustSucceed(t)
	hA.run("repo", "add", "bob/projet").mustSucceed(t)
	cfgA := filepath.Join(hA.cwd, ".watchoo", "config.yaml")
	writeFile(t, cfgA, readFile(t, cfgA)+"\nsync:\n  interval: 1h\nactivity:\n  flemme_after_days: 3\n")
	share := filepath.Join(t.TempDir(), "licence.json")
	hA.run("config", "export", "-o", share).mustSucceed(t)
	if info, err := os.Stat(share); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("exported file: %v, %v", info, err)
	}
	exported := readFile(t, share)
	for _, bad := range []string{"alice@example.test", "gh_path", "\"web\"", "\"github\":", hA.cwd, githubtest.Token} {
		if strings.Contains(exported, bad) {
			t.Errorf("the export contains %q:\n%s", bad, exported)
		}
	}
	// The person sharing adds by hand a repository that will be inaccessible to the others.
	var doc map[string]any
	if err := json.Unmarshal([]byte(exported), &doc); err != nil {
		t.Fatal(err)
	}
	doc["repositories"] = append(doc["repositories"].([]any),
		map[string]any{"provider": "github", "repo": "carol/cache", "label": "", "branches": []any{}})
	edited, _ := json.MarshalIndent(doc, "", "  ")
	writeFile(t, share, string(edited))

	// Workspace B: --dry-run preview (code 1: an inaccessible repository), nothing is written.
	hB := *hA
	hB.cwd = newDir(t, t.TempDir(), "promo-b")
	hB.run("init").mustSucceed(t)
	cfgB := filepath.Join(hB.cwd, ".watchoo", "config.yaml")
	dbB := filepath.Join(hB.cwd, ".watchoo", "watchoo.db")
	cfgBefore, dbBefore := readFile(t, cfgB), readFile(t, dbB)
	dry := hB.run("config", "import", share, "--dry-run", "--json")
	if dry.code != 1 || !strings.Contains(dry.stdout, `"dry_run": true`) {
		t.Fatalf("import --dry-run: code %d\n%s\n%s", dry.code, dry.stdout, dry.stderr)
	}
	var plan struct {
		Summary struct{ Added, Inaccessible int } `json:"summary"`
	}
	decodeLoose(t, dry.stdout, &plan)
	if plan.Summary.Added != 2 || plan.Summary.Inaccessible != 1 {
		t.Errorf("preview = %+v\n%s", plan.Summary, dry.stdout)
	}
	if readFile(t, cfgB) != cfgBefore || readFile(t, dbB) != dbBefore {
		t.Error("--dry-run changed config.yaml or the database")
	}
	if _, err := os.Stat(cfgB + ".bak"); !os.IsNotExist(err) {
		t.Errorf("--dry-run created a backup: %v", err)
	}
	hB.run("config", "import", share).mustFail(t, "--yes") // off a terminal

	// Import --yes: settings rewritten, repositories added, code 1 for the inaccessible repository.
	imp := hB.run("config", "import", share, "--yes")
	if imp.code != 1 || !strings.Contains(imp.stdout, "carol/cache") {
		t.Fatalf("import --yes: code %d\n%s\n%s", imp.code, imp.stdout, imp.stderr)
	}
	e2eAssertShared(t, &hB, cfgB, cfgBefore)
	if rep := e2eSync(t, &hB); rep.Summary.Success != 2 {
		t.Errorf("sync after import = %+v", rep.Summary)
	}
	// Importing the same file again changes nothing (except the inaccessible repository).
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

	// Workspace C: import through the web interface of a real "watchoo serve".
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
	page := e2eHTTP(t, client, http.MethodGet, base+"/settings/share", "", nil, http.StatusOK)
	token := e2eField(t, page, "csrf_token")
	preview := e2eMultipart(t, client, base+"/settings/import", map[string]string{"csrf_token": token},
		"file", "licence.json", edited, http.StatusOK)
	for _, want := range []string{"alice/projet", "bob/projet", "carol/cache", "Alice Martin"} {
		if !strings.Contains(preview, want) {
			t.Errorf("web preview without %q", want)
		}
	}
	if readFile(t, cfgC) != cfgCBefore {
		t.Error("the web preview changed config.yaml")
	}
	document := e2eField(t, preview, "document")
	e2eMultipart(t, client, base+"/settings/import/apply", map[string]string{"csrf_token": token,
		"document": document}, "", "", nil, http.StatusSeeOther)
	if r := run.stop(t); r.code != 0 {
		t.Fatalf("serve: code %d\n%s", r.code, r.stderr)
	}
	e2eAssertShared(t, &hC, cfgC, cfgCBefore)

	logs := testLogs.String()
	for _, bad := range []string{"alice@example.test", "e2e-share-alice", "e2e-share-bob", githubtest.Token} {
		if strings.Contains(logs, bad) {
			t.Errorf("sensitive data %q in the logs", bad)
		}
	}
}

// e2eAssertShared checks a workspace after import: repositories, rewritten settings, backup in 0600.
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
		t.Errorf("repositories = %v, want %v", got, want)
	}
	cfg := readFile(t, cfgPath)
	for _, line := range []string{"interval: 1h", "flemme_after_days: 3"} {
		if !regexp.MustCompile(`\n  ` + regexp.QuoteMeta(line) + `\s`).MatchString(cfg) {
			t.Errorf("config.yaml without %q:\n%s", line, cfg)
		}
	}
	for _, p := range []string{cfgPath, cfgPath + ".bak"} {
		if info, err := os.Stat(p); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s: %v, %v", p, info, err)
		}
	}
	if readFile(t, cfgPath+".bak") != oldCfg {
		t.Error("config.yaml.bak differs from the old file")
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
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != status {
		t.Fatalf("%s %s: %d, want %d\n%s", method, url, resp.StatusCode, status, b)
	}
	if strings.Contains(string(b), githubtest.Token) {
		t.Errorf("token in the response to %s", url)
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

// e2eField returns the (decoded) value of the first form field name of the page.
func e2eField(t *testing.T, page, name string) string {
	t.Helper()
	m := regexp.MustCompile(`name="` + regexp.QuoteMeta(name) + `" value="([^"]*)"`).FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("field %s missing from the page", name)
	}
	return html.UnescapeString(m[1])
}
