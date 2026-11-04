package workspace

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // IANA time zones available even without a system database (minimal CI)
	"unicode/utf8"

	"github.com/spf13/viper"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/logo"
)

var update = flag.Bool("update", false, "rewrites config.example.yaml from ConfigTemplate")

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("licence-projet")
	want := Config{
		Name:     "licence-projet",
		Sync:     SyncConfig{Auto: true, Interval: 30 * time.Minute, Parallelism: 4},
		Activity: ActivityConfig{FlemmeAfterDays: 5, DeadAfterDays: 15},
		Branches: BranchesConfig{Exclude: []string{}},
		Web:      WebConfig{Addr: "127.0.0.1:8080", Auth: WebAuthConfig{Username: ""}, TrustedProxy: false, AllowedHosts: []string{}},
		Timezone: "",
		Lang:     "en",
		GitHub:   GitHubConfig{User: "", GHPath: "gh"},
		Git:      GitConfig{Protocol: "ssh"},
		Diagnostic: DiagnosticConfig{Enabled: false, Exclude: diagnostic.DefaultExclude,
			RetentionDays: 365, SpeedSignal: false},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("DefaultConfig = %+v\nwant %+v", cfg, want)
	}
	cfg.Diagnostic.Exclude[0] = "changed"
	if diagnostic.DefaultExclude[0] == "changed" {
		t.Error("DefaultConfig shares diagnostic.DefaultExclude instead of a copy")
	}
	cfg = DefaultConfig("licence-projet")
	if cfg.Branches.Exclude == nil {
		t.Error("Branches.Exclude must be an empty non-nil slice")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("the default configuration must be valid: %v", err)
	}
}

func TestSettingKeys(t *testing.T) {
	var cfg Config
	var keys, envs []string
	for _, f := range settingFields(&cfg) {
		keys = append(keys, f.key)
		envs = append(envs, envVarName(f.key))
	}
	wantKeys := []string{
		"name", "sync.auto", "sync.interval", "sync.parallelism",
		"activity.flemme_after_days", "activity.dead_after_days", "branches.exclude",
		"web.addr", "web.auth.username", "web.trusted_proxy", "web.allowed_hosts", "timezone", "lang", "github.user", "github.gh_path",
		"git.protocol", "diagnostic.enabled", "diagnostic.exclude", "diagnostic.retention_days",
		"diagnostic.speed_signal",
	}
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("keys = %v\nwant %v", keys, wantKeys)
	}
	wantEnvs := []string{
		"WATCHOO_NAME", "WATCHOO_SYNC_AUTO", "WATCHOO_SYNC_INTERVAL", "WATCHOO_SYNC_PARALLELISM",
		"WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS", "WATCHOO_ACTIVITY_DEAD_AFTER_DAYS", "WATCHOO_BRANCHES_EXCLUDE",
		"WATCHOO_WEB_ADDR", "WATCHOO_WEB_AUTH_USERNAME", "WATCHOO_WEB_TRUSTED_PROXY", "WATCHOO_WEB_ALLOWED_HOSTS", "WATCHOO_TIMEZONE", "WATCHOO_LANG", "WATCHOO_GITHUB_USER", "WATCHOO_GITHUB_GH_PATH",
		"WATCHOO_GIT_PROTOCOL", "WATCHOO_DIAGNOSTIC_ENABLED", "WATCHOO_DIAGNOSTIC_EXCLUDE",
		"WATCHOO_DIAGNOSTIC_RETENTION_DAYS", "WATCHOO_DIAGNOSTIC_SPEED_SIGNAL",
	}
	if !slices.Equal(envs, wantEnvs) {
		t.Errorf("variables = %v\nwant %v", envs, wantEnvs)
	}
}

func TestValidateAcceptsBoundaries(t *testing.T) {
	cases := map[string]func(*Config){
		"manual sync only":   func(c *Config) { c.Sync.Interval = 0 },
		"minimum interval":   func(c *Config) { c.Sync.Interval = 15 * time.Minute },
		"long interval":      func(c *Config) { c.Sync.Interval = 24 * time.Hour },
		"parallelism 1":      func(c *Config) { c.Sync.Parallelism = 1 },
		"flemme 1, dead 2":   func(c *Config) { c.Activity = ActivityConfig{FlemmeAfterDays: 1, DeadAfterDays: 2} },
		"exclusion patterns": func(c *Config) { c.Branches.Exclude = []string{"dependabot/*", "renovate/*", "wip-?"} },
		"public names of the proxy": func(c *Config) {
			c.Web.AllowedHosts = []string{"watchoo.example.org", "Watchoo-2.example.org", "192.168.1.5", "::1", "[fe80::1]"}
		},
		"all interfaces":            func(c *Config) { c.Web.Addr = ":8080" },
		"IPv6":                      func(c *Config) { c.Web.Addr = "[::1]:8080" },
		"port chosen by the system": func(c *Config) { c.Web.Addr = "localhost:0" },
		"IANA time zone":            func(c *Config) { c.Timezone = "Europe/Paris" },
		"UTC time zone":             func(c *Config) { c.Timezone = "UTC" },
		"french":                    func(c *Config) { c.Lang = "fr" },
		"gh account":                func(c *Config) { c.GitHub.User = "prof-dupont" },
		"gh path":                   func(c *Config) { c.GitHub.GHPath = "/opt/homebrew/bin/gh" },
		"diagnostic enabled":        func(c *Config) { c.Diagnostic.Enabled = true },
		"no diagnostic exclusion":   func(c *Config) { c.Diagnostic.Exclude = []string{} },
		"diagnostic patterns": func(c *Config) {
			c.Diagnostic.Exclude = []string{"vendor/", "*.pb.go", "web/static/*.js", "a/b/"}
		},
		"retention 1 day":    func(c *Config) { c.Diagnostic.RetentionDays = 1 },
		"retention 10 years": func(c *Config) { c.Diagnostic.RetentionDays = 3650 },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig("espace")
			mutate(&cfg)
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate: %v", err)
			}
		})
	}
}

func TestValidateRejectsInvalidValues(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Config)
		want   []string
	}{
		{"empty name", func(c *Config) { c.Name = "  " }, []string{"name:", "empty"}},
		{"interval too short", func(c *Config) { c.Sync.Interval = 10 * time.Minute },
			[]string{"sync.interval: 10m0s is below the minimum of 15 min (0 for manual sync only)"}},
		{"negative interval", func(c *Config) { c.Sync.Interval = -time.Minute },
			[]string{"sync.interval", "-1m0s", "negative"}},
		{"zero parallelism", func(c *Config) { c.Sync.Parallelism = 0 },
			[]string{"sync.parallelism: 0", "between 1 and 4"}},
		{"parallelism too large", func(c *Config) { c.Sync.Parallelism = 5 },
			[]string{"sync.parallelism: 5", "between 1 and 4"}},
		{"zero flemme", func(c *Config) { c.Activity.FlemmeAfterDays = 0 },
			[]string{"activity.flemme_after_days: 0", "at least 1"}},
		{"dead equal to flemme", func(c *Config) { c.Activity = ActivityConfig{FlemmeAfterDays: 7, DeadAfterDays: 7} },
			[]string{"activity.dead_after_days: 7", "strictly greater", "(7)"}},
		{"dead below flemme", func(c *Config) { c.Activity = ActivityConfig{FlemmeAfterDays: 10, DeadAfterDays: 3} },
			[]string{"activity.dead_after_days: 3"}},
		{"empty pattern", func(c *Config) { c.Branches.Exclude = []string{"main", " "} },
			[]string{"branches.exclude", "#2", "empty"}},
		{"invalid pattern", func(c *Config) { c.Branches.Exclude = []string{"release/[a"} },
			[]string{"branches.exclude", `"release/[a"`, "invalid"}},
		{"empty diagnostic pattern", func(c *Config) { c.Diagnostic.Exclude = []string{"vendor/", ""} },
			[]string{"diagnostic.exclude", "#2", "empty"}},
		{"invalid diagnostic pattern", func(c *Config) { c.Diagnostic.Exclude = []string{"gen/[a"} },
			[]string{"diagnostic.exclude", `"gen/[a"`, "invalid"}},
		{"zero retention", func(c *Config) { c.Diagnostic.RetentionDays = 0 },
			[]string{"diagnostic.retention_days: 0", "1 to 3650"}},
		{"retention too long", func(c *Config) { c.Diagnostic.RetentionDays = 3651 },
			[]string{"diagnostic.retention_days: 3651"}},
		{"public name with port", func(c *Config) { c.Web.AllowedHosts = []string{"watchoo.example.org:443"} },
			[]string{"web.allowed_hosts", "#1", "invalid"}},
		{"wildcard public name", func(c *Config) { c.Web.AllowedHosts = []string{"ok.example.org", "*"} },
			[]string{"web.allowed_hosts", "#2", `"*"`}},
		{"public name with scheme", func(c *Config) { c.Web.AllowedHosts = []string{"https://watchoo.example.org"} },
			[]string{"web.allowed_hosts", "invalid"}},
		{"empty public name", func(c *Config) { c.Web.AllowedHosts = []string{""} },
			[]string{"web.allowed_hosts", "invalid"}},
		{"address without port", func(c *Config) { c.Web.Addr = "127.0.0.1" },
			[]string{"web.addr", `"127.0.0.1"`, "host:port"}},
		{"empty address", func(c *Config) { c.Web.Addr = "" }, []string{"web.addr", "host:port"}},
		{"empty port", func(c *Config) { c.Web.Addr = "127.0.0.1:" }, []string{"web.addr", "missing port"}},
		{"non-numeric port", func(c *Config) { c.Web.Addr = "127.0.0.1:80a" }, []string{"web.addr", `port "80a"`}},
		{"port too large", func(c *Config) { c.Web.Addr = "127.0.0.1:70000" }, []string{"web.addr", "65535"}},
		{"web login with a colon", func(c *Config) { c.Web.Auth.Username = "prof:x" },
			[]string{"web.auth.username", `"prof:x"`, "invalid"}},
		{"web login with a control character", func(c *Config) { c.Web.Auth.Username = "prof\n" },
			[]string{"web.auth.username", "invalid"}},
		{"web login surrounded by spaces", func(c *Config) { c.Web.Auth.Username = " prof" },
			[]string{"web.auth.username", "invalid"}},
		{"unknown time zone", func(c *Config) { c.Timezone = "Mars/Olympus" },
			[]string{"timezone", `"Mars/Olympus"`, "IANA"}},
		{"unknown language", func(c *Config) { c.Lang = "de" },
			[]string{"lang", `"de"`, `"en"`, `"fr"`}},
		{"empty language", func(c *Config) { c.Lang = "" }, []string{"lang", `""`}},
		{"empty gh path", func(c *Config) { c.GitHub.GHPath = "" }, []string{"github.gh_path", "empty"}},
		{"relative gh path", func(c *Config) { c.GitHub.GHPath = ".watchoo/gh" },
			[]string{"github.gh_path", `".watchoo/gh"`, "relative", "absolute"}},
		{"relative gh path with ./", func(c *Config) { c.GitHub.GHPath = "./gh" },
			[]string{"github.gh_path", "relative"}},
		{"unknown git protocol", func(c *Config) { c.Git.Protocol = "ftp" },
			[]string{"git.protocol", `"ftp"`, "ssh", "https"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig("espace")
			tc.mutate(&cfg)
			assertErrorContains(t, cfg.Validate(), tc.want...)
		})
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	cfg := DefaultConfig("espace")
	cfg.Sync.Interval = time.Minute
	cfg.Sync.Parallelism = 8
	cfg.Timezone = "Nulle/Part"
	err := cfg.Validate()
	assertErrorContains(t, err, "sync.interval", "sync.parallelism", "timezone")
	if n := len(strings.Split(err.Error(), "\n")); n != 3 {
		t.Errorf("%d messages, want 3 (one per problem):\n%v", n, err)
	}
}

func TestLocation(t *testing.T) {
	cfg := DefaultConfig("espace")
	if loc, err := cfg.Location(); err != nil || loc != time.Local {
		t.Errorf("Location() with empty timezone = %v, %v; want time.Local", loc, err)
	}
	cfg.Timezone = "Europe/Paris"
	if loc, err := cfg.Location(); err != nil || loc.String() != "Europe/Paris" {
		t.Errorf("Location() = %v, %v; want Europe/Paris", loc, err)
	}
	cfg.Timezone = "Pas/Un/Fuseau"
	_, err := cfg.Location()
	assertErrorContains(t, err, "timezone", "Pas/Un/Fuseau")
}

func TestUserConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	t.Setenv("XDG_CONFIG_HOME", "/srv/xdg")
	if got, err := UserConfigPath(); err != nil || got != filepath.Join("/srv/xdg", "watchoo", "config.yaml") {
		t.Errorf("with XDG_CONFIG_HOME: %q, %v", got, err)
	}

	want := filepath.Join(home, ".config", "watchoo", "config.yaml")
	for _, xdg := range []string{"", "relative/xdg"} {
		t.Setenv("XDG_CONFIG_HOME", xdg)
		if got, err := UserConfigPath(); err != nil || got != want {
			t.Errorf("XDG_CONFIG_HOME=%q: %q, %v; want %q (never ~/Library/Application Support)",
				xdg, got, err, want)
		}
	}
}

// newLoadWorkspace creates a "licence-projet" workspace (isolated from the environment) and returns
// the workspace and the path of the user settings file.
func newLoadWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	userConfig := isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	return mustInit(t, root), userConfig
}

func TestLoadDefaults(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v, want the default values", ws.Config)
	}
}

func TestLoadBeforeInit(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "pas-encore")
	ws := &Workspace{Root: root, Dir: filepath.Join(root, DirName)}
	t.Setenv("WATCHOO_GITHUB_GH_PATH", "/opt/gh")

	if err := Load(ws); err != nil {
		t.Fatalf("Load without .watchoo: %v", err)
	}
	want := DefaultConfig("pas-encore")
	want.GitHub.GHPath = "/opt/gh"
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v, want %+v", ws.Config, want)
	}
}

func TestLoadUserConfig(t *testing.T) {
	userConfig := isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	writeFile(t, userConfig, "github:\n  user: compte-utilisateur\ntimezone: Europe/Paris\n")
	t.Setenv("WATCHOO_GITHUB_GH_PATH", "/opt/gh")

	cfg, err := LoadUserConfig(root)
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	want := DefaultConfig("licence-projet")
	want.GitHub.User = "compte-utilisateur" // user > default
	want.Timezone = "Europe/Paris"
	want.GitHub.GHPath = "/opt/gh" // environment > user
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Config = %+v\nwant  %+v", cfg, want)
	}
}

func TestLoadUserConfigIgnoresWorkspaceFile(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	ws := mustInit(t, root)
	// An invalid workspace config.yaml must not be read: init will refuse the existing workspace next.
	writeFile(t, ws.ConfigPath(), "cle_inconnue: 1\nsync:\n  interval: 1m\n")

	cfg, err := LoadUserConfig(root)
	if err != nil {
		t.Fatalf("LoadUserConfig: %v", err)
	}
	if !reflect.DeepEqual(cfg, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v, want the default values", cfg)
	}
}

func TestLoadUserConfigErrors(t *testing.T) {
	userConfig := isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")

	_, err := LoadUserConfig("")
	assertErrorContains(t, err, "directory not provided")

	writeFile(t, userConfig, "sync:\n  interval: 1m\n")
	_, err = LoadUserConfig(root)
	assertErrorContains(t, err, "sync.interval", "15 min")

	writeFile(t, userConfig, "")
	t.Setenv("WATCHOO_SYNC_PARALLELISM", "9")
	_, err = LoadUserConfig(root)
	assertErrorContains(t, err, "sync.parallelism")
}

func TestLoadPrecedence(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, `
timezone: Europe/Paris
sync:
  interval: 1h
  parallelism: 2
activity:
  flemme_after_days: 3
  dead_after_days: 30
branches:
  exclude: ["dependabot/*"]
github:
  user: compte-utilisateur
`)
	writeFile(t, ws.ConfigPath(), `
name: "Licence 2026"
sync:
  interval: 2h
activity:
  dead_after_days: 20
branches:
  exclude: ["renovate/*", "wip"]
`)
	t.Setenv("WATCHOO_SYNC_INTERVAL", "3h")
	t.Setenv("WATCHOO_GITHUB_USER", "compte-env")

	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := DefaultConfig("licence-projet")
	want.Name = "Licence 2026"                                            // workspace > default
	want.Timezone = "Europe/Paris"                                        // user > default
	want.Sync.Interval = 3 * time.Hour                                    // environment > workspace > user
	want.Sync.Parallelism = 2                                             // user, not replaced by the workspace
	want.Activity = ActivityConfig{FlemmeAfterDays: 3, DeadAfterDays: 20} // merge key by key
	want.Branches.Exclude = []string{"renovate/*", "wip"}                 // the list of the workspace replaces the user's
	want.GitHub.User = "compte-env"                                       // environment > user
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v\nwant  %+v", ws.Config, want)
	}
}

func TestLoadUserConfigFromHome(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	home := os.Getenv("HOME")
	writeFile(t, filepath.Join(home, ".config", "watchoo", "config.yaml"), "timezone: UTC\n")
	// This file must never be read (os.UserConfigDir on macOS).
	writeFile(t, filepath.Join(home, "Library", "Application Support", "watchoo", "config.yaml"),
		"timezone: Asia/Tokyo\n")

	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want UTC (read in ~/.config/watchoo/config.yaml)", ws.Config.Timezone)
	}
}

func TestLoadWithoutHome(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if err := Load(ws); err != nil {
		t.Fatalf("Load without home directory: %v", err)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v", ws.Config)
	}
}

func TestLoadEnvironmentVariables(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	env := map[string]string{
		"WATCHOO_NAME":                       "Depuis l'environnement",
		"WATCHOO_SYNC_AUTO":                  "false",
		"WATCHOO_SYNC_INTERVAL":              "0",
		"WATCHOO_SYNC_PARALLELISM":           "1",
		"WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS": "2",
		"WATCHOO_ACTIVITY_DEAD_AFTER_DAYS":   "9",
		"WATCHOO_BRANCHES_EXCLUDE":           "dependabot/*, renovate/*",
		"WATCHOO_WEB_ADDR":                   "0.0.0.0:9090",
		"WATCHOO_WEB_ALLOWED_HOSTS":          "watchoo.example.org, autre.example.org",
		"WATCHOO_TIMEZONE":                   "America/Montreal",
		"WATCHOO_LANG":                       "fr",
		"WATCHOO_GITHUB_USER":                "prof",
		"WATCHOO_GITHUB_GH_PATH":             "/usr/local/bin/gh",
		"WATCHOO_GIT_PROTOCOL":               "https",
		"WATCHOO_DIAGNOSTIC_ENABLED":         "true",
		"WATCHOO_DIAGNOSTIC_EXCLUDE":         "vendor/, *.lock",
		"WATCHOO_DIAGNOSTIC_RETENTION_DAYS":  "30",
		"WATCHOO_DIAGNOSTIC_SPEED_SIGNAL":    "true",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := Config{
		Name:     "Depuis l'environnement",
		Sync:     SyncConfig{Auto: false, Interval: 0, Parallelism: 1},
		Activity: ActivityConfig{FlemmeAfterDays: 2, DeadAfterDays: 9},
		Branches: BranchesConfig{Exclude: []string{"dependabot/*", "renovate/*"}},
		Web:      WebConfig{Addr: "0.0.0.0:9090", AllowedHosts: []string{"watchoo.example.org", "autre.example.org"}},
		Timezone: "America/Montreal",
		Lang:     "fr",
		GitHub:   GitHubConfig{User: "prof", GHPath: "/usr/local/bin/gh"},
		Git:      GitConfig{Protocol: "https"},
		Diagnostic: DiagnosticConfig{Enabled: true, Exclude: []string{"vendor/", "*.lock"},
			RetentionDays: 30, SpeedSignal: true},
	}
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v\nwant  %+v", ws.Config, want)
	}
}

func TestLoadEmptyEnvironmentVariableIgnored(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "timezone: Europe/Paris\n")
	t.Setenv("WATCHOO_TIMEZONE", "")
	t.Setenv(EnvDir, ws.Root) // not a setting
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.Timezone != "Europe/Paris" {
		t.Errorf("Timezone = %q, an empty variable must be ignored", ws.Config.Timezone)
	}
}

// TestLoadLang checks the lang setting: read from config.yaml, normalized ("FR" → "fr"), and
// overridden by WATCHOO_LANG, the same variable that selects the CLI and TUI language (one variable
// forces the language everywhere). An unknown value is refused, from the file or the variable.
func TestLoadLang(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "lang: \" FR \"\n")
		if err := Load(ws); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if ws.Config.Lang != "fr" || ws.Config.Translator().Lang() != i18n.French {
			t.Errorf("Lang = %q, want fr", ws.Config.Lang)
		}
	})
	t.Run("WATCHOO_LANG overrides the file", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "lang: fr\n")
		t.Setenv("WATCHOO_LANG", "en")
		if err := Load(ws); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if ws.Config.Lang != "en" {
			t.Errorf("Lang = %q, want en (WATCHOO_LANG)", ws.Config.Lang)
		}
		if got := i18n.Detect(os.Getenv); got != i18n.English {
			t.Errorf("i18n.Detect = %q, want en: the same variable selects the CLI language", got)
		}
	})
	t.Run("unknown value in the file", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "lang: de\n")
		assertErrorContains(t, Load(ws), "lang", `"de"`)
	})
	t.Run("unknown value in WATCHOO_LANG", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		t.Setenv("WATCHOO_LANG", "es")
		assertErrorContains(t, Load(ws), "lang", `"es"`)
	})
}

func TestLoadFileValues(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	writeFile(t, ws.ConfigPath(), `
name: 2026
sync:
  auto: false
  interval: 0
  parallelism: "3"
branches:
  exclude: [2024, "dependabot/*"]
`)
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := DefaultConfig("licence-projet")
	want.Name = "2026"
	want.Sync = SyncConfig{Auto: false, Interval: 0, Parallelism: 3}
	want.Branches.Exclude = []string{"2024", "dependabot/*"}
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v\nwant  %+v", ws.Config, want)
	}
}

func TestLoadEmptyValuesIgnored(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "timezone: Europe/Paris\nsync:\n  auto: false\nbranches:\n  exclude: [a]\n")
	// Empty sections and keys without value: nothing is replaced.
	writeFile(t, ws.ConfigPath(), "timezone:\nsync:\n  auto:\nbranches:\n  exclude:\nweb:\nactivity: {}\n")
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.Timezone != "Europe/Paris" || ws.Config.Sync.Auto ||
		!slices.Equal(ws.Config.Branches.Exclude, []string{"a"}) {
		t.Errorf("Config = %+v: the user's values must be kept", ws.Config)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	cases := []struct {
		name, content string
		want          []string
	}{
		{"typo", "sync:\n  intervall: 1h\n",
			[]string{`unknown key "sync.intervall"`, `did you mean "sync.interval"?`}},
		{"key outside its section", "interval: 1h\n",
			[]string{`unknown key "interval"`, `"sync.interval"`}},
		{"unknown key", "couleur: bleu\n", []string{`unknown key "couleur"`}},
		{"unknown key without value", "couleur:\n", []string{`unknown key "couleur"`}},
		{"unknown section", "token:\n  value: x\n", []string{`unknown key "token.value"`}},
		{"section with a value", "sync: 5\n", []string{`"sync" is a section`, "sync.auto"}},
		{"setting with sub-keys", "web:\n  addr:\n    host: x\n",
			[]string{`"web.addr" takes no sub-keys`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws, _ := newLoadWorkspace(t)
			writeFile(t, ws.ConfigPath(), tc.content)
			before := ws.Config
			err := Load(ws)
			assertErrorContains(t, err, append([]string{ws.ConfigPath()}, tc.want...)...)
			if !reflect.DeepEqual(ws.Config, before) {
				t.Errorf("ws.Config changed despite the error")
			}
		})
	}
}

func TestLoadRejectsInvalidFileValues(t *testing.T) {
	cases := []struct {
		name, content string
		want          []string
	}{
		{"unreadable duration", "sync:\n  interval: bientôt\n",
			[]string{"sync.interval", `invalid duration "bientôt"`}},
		{"duration without unit", "sync:\n  interval: 30\n", []string{"sync.interval", "duration without a unit 30"}},
		{"interval too short", "sync:\n  interval: 5m\n",
			[]string{"invalid settings", "sync.interval: 5m0s is below the minimum of 15 min"}},
		{"boolean", "sync:\n  auto: oui\n", []string{"sync.auto", "expected true or false", `"oui"`}},
		{"integer", "sync:\n  parallelism: deux\n", []string{"sync.parallelism", "expected an integer"}},
		{"decimal integer", "activity:\n  flemme_after_days: 2.5\n",
			[]string{"activity.flemme_after_days", "expected an integer", "2.5"}},
		{"list expected", "branches:\n  exclude: {a: 1}\n", []string{"branches.exclude"}},
		{"list item", "branches:\n  exclude: [[a]]\n", []string{"branches.exclude", "item #1"}},
		{"string expected", "web:\n  addr: [a, b]\n", []string{"web.addr", "expected a string"}},
		{"parallelism out of bounds", "sync:\n  parallelism: 9\n", []string{"sync.parallelism: 9", "between 1 and 4"}},
		{"inconsistent thresholds", "activity:\n  flemme_after_days: 20\n",
			[]string{"activity.dead_after_days: 15", "strictly greater"}},
		{"unknown time zone", "timezone: Lune/Base\n", []string{"timezone", "Lune/Base"}},
		{"several errors", "sync:\n  auto: peut-être\n  parallelism: x\n",
			[]string{"sync.auto", "sync.parallelism"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws, _ := newLoadWorkspace(t)
			writeFile(t, ws.ConfigPath(), tc.content)
			assertErrorContains(t, Load(ws), tc.want...)
		})
	}
}

func TestLoadErrorsCiteTheFile(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "sync:\n  interval: n'importe\n")
	assertErrorContains(t, Load(ws), userConfig, "sync.interval")
}

func TestLoadInvalidYAML(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	writeFile(t, ws.ConfigPath(), "name: [non fermé\n")
	assertErrorContains(t, Load(ws), ws.ConfigPath(), "invalid YAML")

	writeFile(t, ws.ConfigPath(), "- une\n- liste\n")
	assertErrorContains(t, Load(ws), ws.ConfigPath(), "invalid YAML")

	writeFile(t, ws.ConfigPath(), "name: a\nname: b\n")
	assertErrorContains(t, Load(ws), ws.ConfigPath(), "invalid YAML")
}

func TestLoadUnreadableFile(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	if err := os.Remove(ws.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	newDir(t, ws.ConfigPath()) // a directory instead of the file
	assertErrorContains(t, Load(ws), "reading the settings", ws.ConfigPath())
}

func TestLoadMissingWorkspaceFile(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "sync:\n  parallelism: 2\n")
	if err := os.Remove(ws.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if err := Load(ws); err != nil {
		t.Fatalf("Load without config.yaml: %v", err)
	}
	if ws.Config.Sync.Parallelism != 2 || ws.Config.Name != "licence-projet" {
		t.Errorf("Config = %+v", ws.Config)
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	cases := map[string][]string{
		"WATCHOO_SYNC_INTERVAL":            {"variable WATCHOO_SYNC_INTERVAL (sync.interval)", "invalid duration"},
		"WATCHOO_SYNC_AUTO":                {"variable WATCHOO_SYNC_AUTO (sync.auto)", "true or false"},
		"WATCHOO_SYNC_PARALLELISM":         {"variable WATCHOO_SYNC_PARALLELISM", "integer"},
		"WATCHOO_ACTIVITY_DEAD_AFTER_DAYS": {"variable WATCHOO_ACTIVITY_DEAD_AFTER_DAYS", "integer"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			ws, _ := newLoadWorkspace(t)
			t.Setenv(name, "pas-valable")
			assertErrorContains(t, Load(ws), want...)
		})
	}

	t.Run("duration without unit", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		t.Setenv("WATCHOO_SYNC_INTERVAL", "10")
		assertErrorContains(t, Load(ws), "variable WATCHOO_SYNC_INTERVAL", `duration without a unit "10"`)
	})

	t.Run("value refused by Validate", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		t.Setenv("WATCHOO_SYNC_INTERVAL", "1m")
		assertErrorContains(t, Load(ws), "sync.interval: 1m0s is below the minimum")
	})
}

func TestLoadCaseInsensitiveKeys(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	writeFile(t, ws.ConfigPath(), "Sync:\n  Interval: 45m\n")
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.Sync.Interval != 45*time.Minute {
		t.Errorf("Interval = %v, want 45m", ws.Config.Sync.Interval)
	}
}

func TestLoadRequiresWorkspace(t *testing.T) {
	assertErrorContains(t, Load(nil), "workspace not provided")
	assertErrorContains(t, Load(&Workspace{}), "workspace not provided")
}

// trickyNames are workspace names that need a correct YAML escaping.
var trickyNames = []string{
	"licence-projet",
	"Licence 2026 : groupe #3",
	`guillemets "doubles" et 'simples'`,
	`barre \ oblique`,
	"accents éàü — 日本語",
	"tab\tet\nretour",
	"  espaces autour  ",
	"yes", "null", "~", "true", "123", "1e3", "-", "[liste]", "{carte}", "*alias", "&ancre", "!tag",
	"%pourcent", "@arobase", "`accent grave`", "?question", "| bloc", "> plié",
}

func TestConfigTemplateRoundTrip(t *testing.T) {
	for _, name := range trickyNames {
		t.Run(name, func(t *testing.T) {
			ws, _ := newLoadWorkspace(t)
			writeFile(t, ws.ConfigPath(), ConfigTemplate(i18n.English, name))
			if err := Load(ws); err != nil {
				t.Fatalf("Load: %v\n%s", err, ConfigTemplate(i18n.English, name))
			}
			if !reflect.DeepEqual(ws.Config, DefaultConfig(name)) {
				t.Errorf("Config = %+v\nwant  %+v", ws.Config, DefaultConfig(name))
			}
		})
	}
}

func TestConfigTemplateKeepsUserSettings(t *testing.T) {
	// Only the name is active in the template: the user's settings keep applying.
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "name: ignoré\ntimezone: Europe/Paris\nsync:\n  interval: 1h\n")
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if ws.Config.Name != "licence-projet" || ws.Config.Timezone != "Europe/Paris" ||
		ws.Config.Sync.Interval != time.Hour {
		t.Errorf("Config = %+v", ws.Config)
	}
}

// settingLine recognizes a commented setting line of the template: "# key: value" or
// "#   key: value" (in a section).
var settingLine = regexp.MustCompile(`^# ( *)([a-z_]+):(.*)$`)

func TestConfigTemplateUncommentedEqualsDefaults(t *testing.T) {
	name := "licence-projet"
	var b strings.Builder
	for line := range strings.Lines(ConfigTemplate(i18n.English, name)) {
		b.WriteString(settingLine.ReplaceAllString(strings.TrimSuffix(line, "\n"), "$1$2:$3"))
		b.WriteByte('\n')
	}
	uncommented := b.String()

	// All the known keys are in the template.
	fv := viper.New()
	fv.SetConfigType("yaml")
	if err := fv.ReadConfig(bytes.NewReader([]byte(uncommented))); err != nil {
		t.Fatalf("unreadable uncommented template: %v\n%s", err, uncommented)
	}
	var cfg Config
	var want []string
	for _, f := range settingFields(&cfg) {
		want = append(want, f.key)
	}
	got := fv.AllKeys()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("keys of the uncommented template = %v\nwant %v", got, want)
	}

	// And their values are the default values.
	ws, _ := newLoadWorkspace(t)
	writeFile(t, ws.ConfigPath(), uncommented)
	if err := Load(ws); err != nil {
		t.Fatalf("Load: %v\n%s", err, uncommented)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig(name)) {
		t.Errorf("Config = %+v\nwant  %+v", ws.Config, DefaultConfig(name))
	}
}

// The template in the process language (English, forced by TestMain) contains the expected
// landmarks, and only the name key is active in it.
func TestConfigTemplateIsCommentedInEnglish(t *testing.T) {
	tpl := ConfigTemplate(i18n.English, "licence-projet")
	for _, part := range []string{
		"Watchoo workspace settings",
		"~/.config/watchoo/config.yaml",
		"WATCHOO_SYNC_INTERVAL",
		"gh auth token",
		`name: "licence-projet"`,
	} {
		if !strings.Contains(tpl, part) {
			t.Errorf("the template does not contain %q", part)
		}
	}
	for line := range strings.Lines(tpl) {
		line = strings.TrimSuffix(line, "\n")
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "name: ") {
			t.Errorf("only the name key must be active, line found: %q", line)
		}
	}
}

// The file starts with the header card in comments (mini owl in blocks, "Watchoo" without version,
// subtitle, without tip or color, 80 columns at most), followed by a "#" line then by the settings
// header. The name of the workspace is not in the card.
func TestConfigTemplateStartsWithCard(t *testing.T) {
	want := strings.Join([]string{
		"# ╭────────────────────────────────╮",
		"# │  █▄    ▄█  Watchoo             │",
		"# │  █ ●██● █  Workspace settings  │",
		"# │  ▀██▙▟██▀                      │",
		"# ╰────────────────────────────────╯",
		"#",
		"# Watchoo workspace settings.",
		"",
	}, "\n")
	for _, name := range trickyNames {
		tpl := ConfigTemplate(i18n.English, name)
		if !strings.HasPrefix(tpl, want) {
			t.Errorf("ConfigTemplate(%q) does not start with the card:\n%s", name, tpl)
		}
	}
	tr := i18n.For(i18n.English)
	header := configHeader(tr)
	if header != strings.Join(strings.SplitAfter(want, "\n")[:5], "") {
		t.Errorf("configHeader =\n%s", header)
	}
	if header != commentLines(logo.Card(logo.CardOptions{Title: "Watchoo",
		Subtitle: tr.T("workspace.cfg.card_subtitle"), Width: 80, Unicode: true})) {
		t.Errorf("configHeader is not the logo.Card card in comments:\n%s", header)
	}
	if strings.Contains(header, "\x1b") {
		t.Errorf("escape sequence in the header:\n%q", header)
	}
	for line := range strings.Lines(header) {
		if n := utf8.RuneCountInString(strings.TrimPrefix(strings.TrimSuffix(line, "\n"), "# ")); n > 80 {
			t.Errorf("line of %d columns, more than 80: %q", n, line)
		}
	}
}

// commentLines prefixes each line of s with "#".
func commentLines(s string) string {
	var b strings.Builder
	for line := range strings.Lines(s) {
		b.WriteString("# " + line)
	}
	return b.String()
}

func TestConfigExampleFile(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	want := ConfigTemplate(i18n.English, "licence-projet")
	if *update {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading config.example.yaml: %v (regenerate with -update)", err)
	}
	if string(got) != want {
		t.Errorf("config.example.yaml is not up to date: run \"go test ./internal/workspace " +
			"-run TestConfigExampleFile -update\"")
	}
}

func TestDecodeSettingErrors(t *testing.T) {
	var s string
	var b bool
	var n int
	var d time.Duration
	var l []string
	cases := []struct {
		ptr, raw any
	}{
		{&s, true}, {&s, 1.5}, {&s, []any{"a"}}, {&s, map[string]any{}},
		{&b, 1}, {&b, "oui"},
		{&n, "1.5"}, {&n, true},
		{&d, 30}, {&d, "1 heure"}, {&d, 1.5},
		{&l, 3}, {&l, []any{true}},
	}
	for _, tc := range cases {
		if err := decodeSetting(tc.ptr, tc.raw); err == nil {
			t.Errorf("decodeSetting(%T, %#v): error expected", tc.ptr, tc.raw)
		}
	}
	if err := decodeSetting(new(float64), 1.0); err == nil {
		t.Error("an unsupported field type must be reported")
	}
}

func TestShortDuration(t *testing.T) {
	cases := map[time.Duration]string{
		30 * time.Minute:           "30m",
		15 * time.Minute:           "15m",
		time.Hour:                  "1h",
		90 * time.Minute:           "1h30m",
		45 * time.Second:           "45s",
		time.Hour + 30*time.Second: "1h0m30s",
		0:                          "0s",
	}
	for d, want := range cases {
		if got := shortDuration(d); got != want {
			t.Errorf("shortDuration(%v) = %q, want %q", d, got, want)
		}
		if parsed, err := time.ParseDuration(shortDuration(d)); err != nil || parsed != d {
			t.Errorf("shortDuration(%v) does not read back: %v, %v", d, parsed, err)
		}
	}
}
