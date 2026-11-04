package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

func ptr[T any](v T) *T { return &v }

// customConfig is a workspace config.yaml with machine keys, a common setting and a personal
// comment.
const customConfig = `# Mon commentaire personnel
name: "Licence : projet # 2026"
sync:
  parallelism: 2
web:
  addr: "127.0.0.1:9090"
  allowed_hosts: ["watchoo.example.org"]
  auth:
    username: "prof"
github:
  user: "prof-gh"
  gh_path: "/opt/gh/bin/gh"
git:
  protocol: "https"
diagnostic:
  enabled: true
  retention_days: 90
`

// newCustomWorkspace creates a workspace whose config.yaml is customConfig.
func newCustomWorkspace(t *testing.T) *Workspace {
	t.Helper()
	isolateEnv(t)
	ws := mustInit(t, newDir(t, t.TempDir(), "licence"))
	writeFile(t, ws.ConfigPath(), customConfig)
	if err := Load(ws); err != nil {
		t.Fatal(err)
	}
	return ws
}

func TestSharedSettingsFromAndValues(t *testing.T) {
	cfg := DefaultConfig("x")
	cfg.Branches.Exclude = nil
	s := SharedSettingsFrom(cfg)
	if !slices.Equal(s.Keys(), SharedKeys) {
		t.Errorf("Keys = %v", s.Keys())
	}
	v := s.Values()
	if got := v[KeyBranchesExclude]; !reflect.DeepEqual(got, []string{}) {
		t.Errorf("branches.exclude = %#v, empty list expected", got)
	}
	if v[KeySyncInterval] != DefaultSyncInterval || v[KeySyncAuto] != true || v[KeyTimezone] != "" || v[KeyLang] != "en" {
		t.Errorf("values = %v", v)
	}
	if !(SharedSettings{}).IsZero() || s.IsZero() {
		t.Error("IsZero incorrect")
	}
	// The lists are copied: changing the source does not change the settings.
	cfg.Branches.Exclude = []string{"a"}
	s = SharedSettingsFrom(cfg)
	cfg.Branches.Exclude[0] = "b"
	if (*s.BranchesExclude)[0] != "a" {
		t.Error("branches.exclude not copied")
	}
}

func TestSharedSettingsApplyTo(t *testing.T) {
	cfg := DefaultConfig("x")
	cfg.GitHub.GHPath = "/usr/bin/gh"
	s := SharedSettings{SyncInterval: ptr(time.Hour), Timezone: ptr("Europe/Paris"), BranchesExclude: &[]string{}, Lang: ptr("fr")}
	got := s.ApplyTo(cfg)
	if got.Sync.Interval != time.Hour || got.Timezone != "Europe/Paris" || got.Lang != "fr" || got.Branches.Exclude == nil ||
		len(got.Branches.Exclude) != 0 {
		t.Errorf("ApplyTo = %+v", got)
	}
	if got.Sync.Parallelism != cfg.Sync.Parallelism || got.GitHub.GHPath != "/usr/bin/gh" || got.Name != "x" {
		t.Errorf("values not provided changed: %+v", got)
	}
	if cfg.Sync.Interval != DefaultSyncInterval {
		t.Error("ApplyTo changed the original configuration")
	}
	if !slices.Equal(s.Keys(), []string{KeySyncInterval, KeyBranchesExclude, KeyTimezone, KeyLang}) {
		t.Errorf("Keys = %v", s.Keys())
	}
}

func TestEnvOverrides(t *testing.T) {
	isolateEnv(t)
	t.Setenv("WATCHOO_SYNC_INTERVAL", "1h")
	t.Setenv("WATCHOO_TIMEZONE", "")
	got := EnvOverrides([]string{KeySyncInterval, KeyTimezone, KeySyncAuto})
	want := []EnvOverride{{Key: KeySyncInterval, Var: "WATCHOO_SYNC_INTERVAL"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("EnvOverrides = %v, want %v", got, want)
	}
	if EnvVarName(KeyFlemmeAfterDays) != "WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS" {
		t.Error(EnvVarName(KeyFlemmeAfterDays))
	}
}

// FileConfig ignores the WATCHOO_* variables; WithEnv applies them again like Load.
func TestFileConfigAndWithEnv(t *testing.T) {
	ws := newCustomWorkspace(t)
	t.Setenv("WATCHOO_SYNC_PARALLELISM", "3")
	t.Setenv("WATCHOO_BRANCHES_EXCLUDE", "renovate/*,dependabot/*")
	if err := Load(ws); err != nil {
		t.Fatal(err)
	}
	fileCfg, err := FileConfig(ws)
	if err != nil {
		t.Fatal(err)
	}
	if fileCfg.Sync.Parallelism != 2 || len(fileCfg.Branches.Exclude) != 0 || fileCfg.GitHub.GHPath != "/opt/gh/bin/gh" {
		t.Errorf("FileConfig = %+v", fileCfg)
	}
	withEnv, err := WithEnv(fileCfg)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(withEnv, ws.Config) {
		t.Errorf("WithEnv(FileConfig) = %+v\nLoad = %+v", withEnv, ws.Config)
	}
	if fileCfg.Sync.Parallelism != 2 {
		t.Error("WithEnv changed its argument")
	}
	t.Setenv("WATCHOO_SYNC_PARALLELISM", "beaucoup")
	if _, err := WithEnv(fileCfg); err == nil || !strings.Contains(err.Error(), "WATCHOO_SYNC_PARALLELISM") {
		t.Errorf("WithEnv with an unreadable variable = %v", err)
	}
	if _, err := FileConfig(nil); err == nil {
		t.Error("FileConfig(nil) accepted")
	}
}

func TestFormatSettingValue(t *testing.T) {
	for _, tt := range []struct {
		v    any
		want string
	}{
		{"Europe/Paris", `"Europe/Paris"`},
		{"a\"b\n#c: d", `"a\"b\n#c: d"`},
		{true, "true"},
		{4, "4"},
		{90 * time.Minute, "1h30m"},
		{time.Duration(0), "0s"},
		{[]string{}, "[]"},
		{[]string{"dependabot/*", `x"y`}, `["dependabot/*", "x\"y"]`},
	} {
		if got := FormatSettingValue(tt.v); got != tt.want {
			t.Errorf("FormatSettingValue(%#v) = %s, want %s", tt.v, got, tt.want)
		}
	}
}

// The machine keys (name, web.*, github.*, git.*) and the common settings not imported are kept, the
// old file is copied to .bak, the permissions are 0600, and the file read back gives the expected
// values.
func TestWriteSharedSettingsKeepsMachineKeys(t *testing.T) {
	ws := newCustomWorkspace(t)
	before := readFile(t, ws.ConfigPath())
	s := SharedSettings{
		SyncInterval:    ptr(time.Hour),
		DeadAfterDays:   ptr(21),
		BranchesExclude: &[]string{"dependabot/*", "renovate/*"},
		Timezone:        ptr("Europe/Paris"),
		Lang:            ptr("fr"),
	}
	res, err := WriteSharedSettings(ws, s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != ws.ConfigPath() || res.BackupPath != filepath.Join(ws.Dir, BackupFileName) {
		t.Errorf("result = %+v", res)
	}
	if got := readFile(t, res.BackupPath); got != before {
		t.Errorf(".bak copy different from the old file:\n%s", got)
	}
	assertMode(t, ws.ConfigPath(), FileMode)
	assertMode(t, res.BackupPath, FileMode)

	content := readFile(t, ws.ConfigPath())
	if strings.Contains(content, "Mon commentaire personnel") {
		t.Error("the personal comment should disappear (it stays in .bak)")
	}
	if !strings.HasPrefix(content, configHeader(i18n.For(i18n.English))) {
		t.Error("the rewritten file does not start with the card of the template")
	}

	reloaded := &Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := Load(reloaded); err != nil {
		t.Fatal(err)
	}
	c := reloaded.Config
	if c.Name != "Licence : projet # 2026" || c.Web.Addr != "127.0.0.1:9090" ||
		!slices.Equal(c.Web.AllowedHosts, []string{"watchoo.example.org"}) || c.Web.Auth.Username != "prof" ||
		c.GitHub.User != "prof-gh" || c.GitHub.GHPath != "/opt/gh/bin/gh" || c.Git.Protocol != "https" {
		t.Errorf("machine keys lost: %+v", c)
	}
	if !c.Diagnostic.Enabled || c.Diagnostic.RetentionDays != 90 {
		t.Errorf("diagnostic.* settings (never shared) lost: %+v", c.Diagnostic)
	}
	if c.Sync.Parallelism != 2 {
		t.Errorf("sync.parallelism not imported lost: %d", c.Sync.Parallelism)
	}
	if c.Sync.Interval != time.Hour || c.Activity.DeadAfterDays != 21 || c.Timezone != "Europe/Paris" || c.Lang != "fr" ||
		!slices.Equal(c.Branches.Exclude, []string{"dependabot/*", "renovate/*"}) {
		t.Errorf("imported settings missing: %+v", c)
	}
	if !reflect.DeepEqual(res.Config, c) {
		t.Errorf("WriteResult.Config = %+v, read back %+v", res.Config, c)
	}
	// The settings not provided and absent from the file stay in comments.
	for _, line := range []string{"#   auto: true", "#   flemme_after_days: 5"} {
		if !strings.Contains(content, line) {
			t.Errorf("commented line %q missing:\n%s", line, content)
		}
	}
	for _, line := range []string{"sync:\n", "  interval: 1h ", "  parallelism: 2 ", "  dead_after_days: 21 ",
		`  exclude: ["dependabot/*", "renovate/*"]`, `timezone: "Europe/Paris"`, `lang: "fr"`, "  auth:\n", `    username: "prof"`,
		`  gh_path: "/opt/gh/bin/gh"`, "git:\n", `  protocol: "https"`} {
		if !strings.Contains(content, line) {
			t.Errorf("active line %q missing:\n%s", line, content)
		}
	}
	// ws.Config is not changed (a running serve keeps its configuration).
	if ws.Config.Sync.Interval != DefaultSyncInterval {
		t.Error("ws.Config changed")
	}
}

// Two writes of the same settings give the same file.
func TestWriteSharedSettingsIdempotent(t *testing.T) {
	ws := newCustomWorkspace(t)
	s := SharedSettings{SyncAuto: ptr(false), Timezone: ptr("UTC")}
	if _, err := WriteSharedSettings(ws, s); err != nil {
		t.Fatal(err)
	}
	first := readFile(t, ws.ConfigPath())
	if _, err := WriteSharedSettings(ws, s); err != nil {
		t.Fatal(err)
	}
	if second := readFile(t, ws.ConfigPath()); second != first {
		t.Errorf("different second write:\n%s\n---\n%s", first, second)
	}
	if bak := readFile(t, filepath.Join(ws.Dir, BackupFileName)); bak != first {
		t.Error("the .bak copy must be the previous file")
	}
}

// A pattern containing quotes, line breaks, "#" and ":" remains a value: it never creates another
// key (github.gh_path in particular).
func TestWriteSharedSettingsNoYAMLInjection(t *testing.T) {
	ws := newCustomWorkspace(t)
	evil := []string{
		"x\"\ngithub:\n  gh_path: /tmp/evil\n#",
		"a: b # c",
		"'quote' \"double\" \\n",
		"}]\n- y",
		"\tétoile*  ",
	}
	if _, err := WriteSharedSettings(ws, SharedSettings{BranchesExclude: &evil}); err != nil {
		t.Fatal(err)
	}
	reloaded := &Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := Load(reloaded); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(reloaded.Config.Branches.Exclude, evil) {
		t.Errorf("branches.exclude = %q, want %q", reloaded.Config.Branches.Exclude, evil)
	}
	if reloaded.Config.GitHub.GHPath != "/opt/gh/bin/gh" {
		t.Errorf("github.gh_path = %q", reloaded.Config.GitHub.GHPath)
	}
	values, err := fileValues(ws.ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	for key := range values {
		if !slices.Contains([]string{"name", "sync.parallelism", "branches.exclude", "web.addr", "web.allowed_hosts",
			"web.auth.username", "github.user", "github.gh_path", "git.protocol", "diagnostic.enabled",
			"diagnostic.retention_days"}, key) {
			t.Errorf("unexpected key %s in the rewritten file", key)
		}
	}
}

// A refused value (unknown time zone, injection included, or inconsistent setting) writes nothing:
// neither config.yaml, nor .bak, nor temporary file.
func TestWriteSharedSettingsInvalidChangesNothing(t *testing.T) {
	for name, s := range map[string]SharedSettings{
		"time zone":   {Timezone: ptr("Europe/Paris\"\ngithub:\n  gh_path: /tmp/evil")},
		"parallelism": {SyncParallelism: ptr(9)},
		"thresholds":  {FlemmeAfterDays: ptr(20)},
		"utf8":        {Timezone: ptr("\xff")},
		"lang":        {Lang: ptr("de")},
		"pattern":     {BranchesExclude: &[]string{"[a"}},
	} {
		t.Run(name, func(t *testing.T) {
			ws := newCustomWorkspace(t)
			before := readFile(t, ws.ConfigPath())
			if _, err := WriteSharedSettings(ws, s); err == nil {
				t.Fatal("error expected")
			}
			if got := readFile(t, ws.ConfigPath()); got != before {
				t.Errorf("config.yaml changed:\n%s", got)
			}
			assertOnlyFiles(t, ws.Dir, ConfigFileName, GitignoreFileName)
		})
	}
}

// Without config.yaml, the file is created (without .bak) from the template, without an active name
// line.
func TestWriteSharedSettingsWithoutFile(t *testing.T) {
	isolateEnv(t)
	ws := mustInit(t, newDir(t, t.TempDir(), "master"))
	if err := os.Remove(ws.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	res, err := WriteSharedSettings(ws, SharedSettings{FlemmeAfterDays: ptr(3)})
	if err != nil {
		t.Fatal(err)
	}
	if res.BackupPath != "" {
		t.Errorf("BackupPath = %q, empty expected", res.BackupPath)
	}
	content := readFile(t, ws.ConfigPath())
	if !strings.Contains(content, `# name: "master"`) || !strings.Contains(content, "  flemme_after_days: 3 ") {
		t.Errorf("content:\n%s", content)
	}
	assertMode(t, ws.ConfigPath(), FileMode)
	if res.Config.Name != "master" || res.Config.Activity.FlemmeAfterDays != 3 {
		t.Errorf("Config = %+v", res.Config)
	}
}

// A failure to write the temporary file changes nothing.
func TestWriteSharedSettingsWriteFailure(t *testing.T) {
	ws := newCustomWorkspace(t)
	before := readFile(t, ws.ConfigPath())
	boom := errors.New("disk full")
	orig := writeTemp
	writeTemp = func(string, string, []byte) (string, error) { return "", boom }
	t.Cleanup(func() { writeTemp = orig })
	if _, err := WriteSharedSettings(ws, SharedSettings{SyncAuto: ptr(false)}); !errors.Is(err, boom) {
		t.Fatalf("error = %v", err)
	}
	if got := readFile(t, ws.ConfigPath()); got != before {
		t.Error("config.yaml changed")
	}
}

// The workspace must belong to the user, as for Load.
func TestWriteSharedSettingsUntrusted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	ws := newCustomWorkspace(t)
	if err := os.Chmod(ws.ConfigPath(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSharedSettings(ws, SharedSettings{SyncAuto: ptr(false)}); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("error = %v, want ErrUntrusted", err)
	}
}

func TestWriteSharedSettingsRequiresWorkspace(t *testing.T) {
	if _, err := WriteSharedSettings(nil, SharedSettings{}); err == nil {
		t.Error("error expected")
	}
}

// The imported settings and the machine keys read back are the expected ones, including for the Init
// template rewritten without any common setting: only name is active.
func TestRenderConfigOnlyName(t *testing.T) {
	if got := renderConfig(i18n.For(i18n.English), "x", map[string]any{"name": "x"}); got != ConfigTemplate(i18n.English, "x") {
		t.Error("renderConfig with name alone must give ConfigTemplate")
	}
}

// assertOnlyFiles checks that dir only contains the files names.
func assertOnlyFiles(t *testing.T, dir string, names ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	slices.Sort(got)
	slices.Sort(names)
	if !slices.Equal(got, names) {
		t.Errorf("files of %s = %v, want %v", dir, got, names)
	}
}
