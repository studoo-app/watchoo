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

	"github.com/studoo-app/watchoo/internal/conventions"
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
		Conventions: ConventionsConfig{Enabled: false, Since: "",
			Commit: ConventionsCommitConfig{
				Types:          []string{"feat", "fix", "docs", "style", "refactor", "perf", "test", "build", "ci", "chore", "revert"},
				Scopes:         []string{},
				MaxSubjectSoft: 50, MaxSubjectHard: 72,
				Forbidden: conventions.DefaultForbidden(),
				Language:  "any",
			},
			Branch: ConventionsBranchConfig{Pattern: "", Exclude: []string{"gh-pages", "dependabot/*", "renovate/*"}},
			PR: ConventionsPRConfig{BodySections: []string{"context", "changes", "testing", "screenshots"},
				RequireCloses: true},
			Tags: ConventionsTagsConfig{Milestones: []string{"v1.0.0", "v2.0.0", "v3.0.0"},
				PrereleaseForms: []string{"rc", "beta", "alpha"}},
			Changelog: ConventionsChangelogConfig{CategoriesFR: true},
			Readme: ConventionsReadmeConfig{MinWords: 150, Sections: []string{"presentation", "group", "stack",
				"install", "run", "env", "tree", "license"}},
			Docs:             ConventionsDocsConfig{Dir: "documentation", MediaDir: "Media", ArchivesScope: "repository"},
			ExcludedAccounts: []string{}},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("DefaultConfig = %+v\nwant %+v", cfg, want)
	}
	for _, f := range []string{"update", "maj", "wip", "test", "modif", "fix", "dépôt 1", ".", "initial commit"} {
		if !slices.Contains(cfg.Conventions.Commit.Forbidden, f) {
			t.Errorf("conventions.commit.forbidden lacks %q from the charter", f)
		}
	}
	cfg.Conventions.Commit.Types[0] = "changed"
	if DefaultConfig("x").Conventions.Commit.Types[0] != "feat" {
		t.Error("DefaultConfig shares its conventions.commit.types list")
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
		"diagnostic.speed_signal", "conventions.enabled", "conventions.since", "conventions.commit.types",
		"conventions.commit.scopes", "conventions.commit.max_subject_soft", "conventions.commit.max_subject_hard",
		"conventions.commit.forbidden", "conventions.commit.language", "conventions.branch.pattern",
		"conventions.branch.exclude", "conventions.pr.body_sections", "conventions.pr.require_closes",
		"conventions.tags.milestones", "conventions.tags.prerelease_forms",
		"conventions.changelog.categories_fr",
		"conventions.readme.sections", "conventions.readme.min_words", "conventions.docs.dir",
		"conventions.docs.media_dir", "conventions.docs.archives_scope", "conventions.excluded_accounts",
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
		"WATCHOO_CONVENTIONS_ENABLED", "WATCHOO_CONVENTIONS_SINCE", "WATCHOO_CONVENTIONS_COMMIT_TYPES",
		"WATCHOO_CONVENTIONS_COMMIT_SCOPES", "WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_SOFT",
		"WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_HARD", "WATCHOO_CONVENTIONS_COMMIT_FORBIDDEN",
		"WATCHOO_CONVENTIONS_COMMIT_LANGUAGE", "WATCHOO_CONVENTIONS_BRANCH_PATTERN",
		"WATCHOO_CONVENTIONS_BRANCH_EXCLUDE", "WATCHOO_CONVENTIONS_PR_BODY_SECTIONS",
		"WATCHOO_CONVENTIONS_PR_REQUIRE_CLOSES", "WATCHOO_CONVENTIONS_TAGS_MILESTONES",
		"WATCHOO_CONVENTIONS_TAGS_PRERELEASE_FORMS",
		"WATCHOO_CONVENTIONS_CHANGELOG_CATEGORIES_FR", "WATCHOO_CONVENTIONS_README_SECTIONS",
		"WATCHOO_CONVENTIONS_README_MIN_WORDS", "WATCHOO_CONVENTIONS_DOCS_DIR",
		"WATCHOO_CONVENTIONS_DOCS_MEDIA_DIR", "WATCHOO_CONVENTIONS_DOCS_ARCHIVES_SCOPE",
		"WATCHOO_CONVENTIONS_EXCLUDED_ACCOUNTS",
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
		"retention 1 day":     func(c *Config) { c.Diagnostic.RetentionDays = 1 },
		"retention 10 years":  func(c *Config) { c.Diagnostic.RetentionDays = 3650 },
		"conventions enabled": func(c *Config) { c.Conventions.Enabled = true },
		"conventions since":   func(c *Config) { c.Conventions.Since = "2026-09-01" },
		"leap day":            func(c *Config) { c.Conventions.Since = "2028-02-29" },
		"own types and scopes": func(c *Config) {
			c.Conventions.Commit.Types = []string{"feat", "fix", "wip"}
			c.Conventions.Commit.Scopes = []string{"api", "web-ui", "docs/fr", "v1.2", "a_b", "0"}
		},
		"subject lengths 1 and 2": func(c *Config) {
			c.Conventions.Commit.MaxSubjectSoft, c.Conventions.Commit.MaxSubjectHard = 1, 2
		},
		"no forbidden subject":  func(c *Config) { c.Conventions.Commit.Forbidden = []string{} },
		"french subjects":       func(c *Config) { c.Conventions.Commit.Language = "fr" },
		"english subjects":      func(c *Config) { c.Conventions.Commit.Language = "en" },
		"excluded accounts":     func(c *Config) { c.Conventions.ExcludedAccounts = []string{"prof-dupont", "Jean Prof"} },
		"no README section":     func(c *Config) { c.Conventions.Readme.Sections = []string{} },
		"some README sections":  func(c *Config) { c.Conventions.Readme.Sections = []string{"license", "install"} },
		"README min words 1":    func(c *Config) { c.Conventions.Readme.MinWords = 1 },
		"no PR body section":    func(c *Config) { c.Conventions.PR.BodySections = []string{} },
		"some PR body sections": func(c *Config) { c.Conventions.PR.BodySections = []string{"testing", "context"} },
		"closes not required":   func(c *Config) { c.Conventions.PR.RequireCloses = false },
		"French labels refused": func(c *Config) { c.Conventions.Changelog.CategoriesFR = false },
		"nested docs dir":       func(c *Config) { c.Conventions.Docs.Dir = "docs/projet" },
		"own media dir":         func(c *Config) { c.Conventions.Docs.MediaDir = "images" },
		"archives in the docs":  func(c *Config) { c.Conventions.Docs.ArchivesScope = "documentation" },
		"own pre-release forms": func(c *Config) { c.Conventions.Tags.PrereleaseForms = []string{"rc", "preview"} },
		"single pre-release form": func(c *Config) {
			c.Conventions.Tags.PrereleaseForms = []string{strings.Repeat("a", conventions.MaxPrereleaseFormLength)}
		},
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
		{"since in another form", func(c *Config) { c.Conventions.Since = "01/09/2026" },
			[]string{"conventions.since", `"01/09/2026"`, "YYYY-MM-DD"}},
		{"since with a time", func(c *Config) { c.Conventions.Since = "2026-09-01T00:00:00Z" },
			[]string{"conventions.since", "YYYY-MM-DD"}},
		{"impossible since", func(c *Config) { c.Conventions.Since = "2026-02-30" },
			[]string{"conventions.since", `"2026-02-30"`}},
		{"no type", func(c *Config) { c.Conventions.Commit.Types = []string{} },
			[]string{"conventions.commit.types", "at least one type"}},
		{"uppercase type", func(c *Config) { c.Conventions.Commit.Types = []string{"feat", "Fix"} },
			[]string{"conventions.commit.types", "#2", `"Fix"`, "lowercase"}},
		{"empty type", func(c *Config) { c.Conventions.Commit.Types = []string{""} },
			[]string{"conventions.commit.types", "#1"}},
		{"invalid scope", func(c *Config) { c.Conventions.Commit.Scopes = []string{"api", "API"} },
			[]string{"conventions.commit.scopes", "#2", `"API"`}},
		{"empty scope", func(c *Config) { c.Conventions.Commit.Scopes = []string{""} },
			[]string{"conventions.commit.scopes", "#1"}},
		{"zero soft length", func(c *Config) { c.Conventions.Commit.MaxSubjectSoft = 0 },
			[]string{"conventions.commit.max_subject_soft: 0", "at least 1"}},
		{"hard length equal to soft", func(c *Config) { c.Conventions.Commit.MaxSubjectHard = 50 },
			[]string{"conventions.commit.max_subject_hard: 50", "strictly greater", "(50)"}},
		{"empty forbidden subject", func(c *Config) { c.Conventions.Commit.Forbidden = []string{"wip", "  "} },
			[]string{"conventions.commit.forbidden", "#2", "empty"}},
		{"unknown subject language", func(c *Config) { c.Conventions.Commit.Language = "de" },
			[]string{"conventions.commit.language", `"de"`, "any"}},
		{"empty subject language", func(c *Config) { c.Conventions.Commit.Language = "" },
			[]string{"conventions.commit.language", `""`}},
		{"empty excluded account", func(c *Config) { c.Conventions.ExcludedAccounts = []string{""} },
			[]string{"conventions.excluded_accounts", "#1", "empty"}},
		{"invalid branch pattern", func(c *Config) { c.Conventions.Branch.Pattern = "^(feat/" },
			[]string{"conventions.branch.pattern", `"^(feat/"`, "RE2"}},
		{"Perl-only branch pattern", func(c *Config) { c.Conventions.Branch.Pattern = `^(?!main)` },
			[]string{"conventions.branch.pattern", "RE2"}},
		{"empty branch exclusion", func(c *Config) { c.Conventions.Branch.Exclude = []string{"gh-pages", " "} },
			[]string{"conventions.branch.exclude", "#2", "empty"}},
		{"invalid branch exclusion", func(c *Config) { c.Conventions.Branch.Exclude = []string{"release/["} },
			[]string{"conventions.branch.exclude", "#1", `"release/["`, "invalid"}},
		{"milestone without v", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0.0", "2.0.0"} },
			[]string{"conventions.tags.milestones", "#2", `"2.0.0"`, "vX.Y.Z"}},
		{"uppercase milestone", func(c *Config) { c.Conventions.Tags.Milestones = []string{"V1.0.0"} },
			[]string{"conventions.tags.milestones", "#1", `"V1.0.0"`}},
		{"incomplete milestone", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0"} },
			[]string{"conventions.tags.milestones", "#1", `"v1.0"`}},
		{"pre-release milestone", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0.0-rc.1"} },
			[]string{"conventions.tags.milestones", "#1", "final"}},
		{"milestone with build metadata", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0.0+build"} },
			[]string{"conventions.tags.milestones", "#1", `"v1.0.0+build"`}},
		{"empty milestone", func(c *Config) { c.Conventions.Tags.Milestones = []string{""} },
			[]string{"conventions.tags.milestones", "#1"}},
		{"impossible due date", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0.0@2026-02-30"} },
			[]string{"conventions.tags.milestones", "#1", "due date", "vX.Y.Z@YYYY-MM-DD"}},
		{"due date with a time", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0.0@2026-12-18T00:00"} },
			[]string{"conventions.tags.milestones", "#1", "due date"}},
		{"empty due date", func(c *Config) { c.Conventions.Tags.Milestones = []string{"v1.0.0@"} },
			[]string{"conventions.tags.milestones", "#1", "due date"}},
		{"repeated milestone", func(c *Config) {
			c.Conventions.Tags.Milestones = []string{"v1.0.0", "v2.0.0", "v1.0.0@2026-12-18"}
		}, []string{"conventions.tags.milestones", "#3", "v1.0.0"}},
		{"no pre-release form", func(c *Config) { c.Conventions.Tags.PrereleaseForms = []string{} },
			[]string{"conventions.tags.prerelease_forms", "at least one"}},
		{"uppercase pre-release form", func(c *Config) { c.Conventions.Tags.PrereleaseForms = []string{"rc", "RC"} },
			[]string{"conventions.tags.prerelease_forms", "#2", `"RC"`, "lowercase"}},
		{"pre-release form with a digit", func(c *Config) { c.Conventions.Tags.PrereleaseForms = []string{"rc1"} },
			[]string{"conventions.tags.prerelease_forms", "#1", `"rc1"`}},
		{"pre-release form too long", func(c *Config) {
			c.Conventions.Tags.PrereleaseForms = []string{strings.Repeat("a", conventions.MaxPrereleaseFormLength+1)}
		}, []string{"conventions.tags.prerelease_forms", "#1", "invalid"}},
		{"repeated pre-release form", func(c *Config) { c.Conventions.Tags.PrereleaseForms = []string{"rc", "beta", "rc"} },
			[]string{"conventions.tags.prerelease_forms", "#3", `"rc"`}},
		{"unknown PR body section", func(c *Config) { c.Conventions.PR.BodySections = []string{"context", "closes"} },
			[]string{"conventions.pr.body_sections", "#2", `"closes"`, "context, changes, testing, screenshots"}},
		{"PR body section with a capital", func(c *Config) { c.Conventions.PR.BodySections = []string{"Testing"} },
			[]string{"conventions.pr.body_sections", "#1", `"Testing"`}},
		{"repeated PR body section", func(c *Config) {
			c.Conventions.PR.BodySections = []string{"testing", "changes", "testing"}
		}, []string{"conventions.pr.body_sections", "#3", "testing"}},
		{"unknown README section", func(c *Config) { c.Conventions.Readme.Sections = []string{"install", "Install"} },
			[]string{"conventions.readme.sections", "#2", `"Install"`, "presentation, group"}},
		{"repeated README section", func(c *Config) { c.Conventions.Readme.Sections = []string{"run", "env", "run"} },
			[]string{"conventions.readme.sections", "#3", "run"}},
		{"README min words 0", func(c *Config) { c.Conventions.Readme.MinWords = 0 },
			[]string{"conventions.readme.min_words", "at least 1", "0"}},
		{"empty docs dir", func(c *Config) { c.Conventions.Docs.Dir = "" },
			[]string{"conventions.docs.dir", `""`}},
		{"absolute docs dir", func(c *Config) { c.Conventions.Docs.Dir = "/documentation" },
			[]string{"conventions.docs.dir", `"/documentation"`}},
		{"docs dir going up", func(c *Config) { c.Conventions.Docs.Dir = "docs/../.." },
			[]string{"conventions.docs.dir", `"docs/../.."`}},
		{"media dir with a slash", func(c *Config) { c.Conventions.Docs.MediaDir = "a/b" },
			[]string{"conventions.docs.media_dir", `"a/b"`}},
		{"empty media dir", func(c *Config) { c.Conventions.Docs.MediaDir = "" },
			[]string{"conventions.docs.media_dir", `""`}},
		{"unknown archives scope", func(c *Config) { c.Conventions.Docs.ArchivesScope = "docs" },
			[]string{"conventions.docs.archives_scope", `"docs"`, `"repository"`, `"documentation"`}},
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

		"WATCHOO_CONVENTIONS_ENABLED":                 "true",
		"WATCHOO_CONVENTIONS_SINCE":                   "2026-09-01",
		"WATCHOO_CONVENTIONS_COMMIT_TYPES":            "feat, fix",
		"WATCHOO_CONVENTIONS_COMMIT_SCOPES":           "api,web",
		"WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_SOFT": "60",
		"WATCHOO_CONVENTIONS_COMMIT_MAX_SUBJECT_HARD": "100",
		"WATCHOO_CONVENTIONS_COMMIT_FORBIDDEN":        "wip, maj",
		"WATCHOO_CONVENTIONS_COMMIT_LANGUAGE":         "fr",
		"WATCHOO_CONVENTIONS_BRANCH_PATTERN":          "^(feat|fix)/[a-z0-9-]{2,40}$",
		"WATCHOO_CONVENTIONS_BRANCH_EXCLUDE":          "gh-pages, release/*",
		"WATCHOO_CONVENTIONS_TAGS_MILESTONES":         "v1.0.0@2026-12-18, v2.0.0",
		"WATCHOO_CONVENTIONS_TAGS_PRERELEASE_FORMS":   "rc, preview",
		"WATCHOO_CONVENTIONS_PR_BODY_SECTIONS":        "testing, changes",
		"WATCHOO_CONVENTIONS_PR_REQUIRE_CLOSES":       "false",
		"WATCHOO_CONVENTIONS_CHANGELOG_CATEGORIES_FR": "false",
		"WATCHOO_CONVENTIONS_README_SECTIONS":         "install, run",
		"WATCHOO_CONVENTIONS_README_MIN_WORDS":        "80",
		"WATCHOO_CONVENTIONS_DOCS_DIR":                "docs/projet",
		"WATCHOO_CONVENTIONS_DOCS_MEDIA_DIR":          "images",
		"WATCHOO_CONVENTIONS_DOCS_ARCHIVES_SCOPE":     "documentation",
		"WATCHOO_CONVENTIONS_EXCLUDED_ACCOUNTS":       "prof-dupont",
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
		Conventions: ConventionsConfig{Enabled: true, Since: "2026-09-01",
			Commit: ConventionsCommitConfig{Types: []string{"feat", "fix"}, Scopes: []string{"api", "web"},
				MaxSubjectSoft: 60, MaxSubjectHard: 100, Forbidden: []string{"wip", "maj"}, Language: "fr"},
			Branch: ConventionsBranchConfig{Pattern: "^(feat|fix)/[a-z0-9-]{2,40}$",
				Exclude: []string{"gh-pages", "release/*"}},
			PR: ConventionsPRConfig{BodySections: []string{"testing", "changes"}, RequireCloses: false},
			Tags: ConventionsTagsConfig{Milestones: []string{"v1.0.0@2026-12-18", "v2.0.0"},
				PrereleaseForms: []string{"rc", "preview"}},
			Changelog:        ConventionsChangelogConfig{CategoriesFR: false},
			Readme:           ConventionsReadmeConfig{Sections: []string{"install", "run"}, MinWords: 80},
			Docs:             ConventionsDocsConfig{Dir: "docs/projet", MediaDir: "images", ArchivesScope: "documentation"},
			ExcludedAccounts: []string{"prof-dupont"}},
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

// TestLoadConventions checks the conventions.* block read from config.yaml, including an unquoted
// date (since: 2026-09-01), which YAML reads as a timestamp: it becomes the written date again.
func TestLoadConventions(t *testing.T) {
	t.Run("file with an unquoted date", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), `
conventions:
  enabled: true
  since: 2026-09-01
  commit:
    types: [feat, fix, docs]
    scopes: []
    max_subject_soft: 60
    max_subject_hard: 80
    forbidden: ["wip", "."]
    language: en
  branch:
    pattern: '^(feat|fix)/[a-z]+$'
    exclude: [gh-pages]
  pr:
    body_sections: [context, testing]
    require_closes: false
  tags:
    milestones: [v1.0.0@2026-12-18, "v2.0.0"]
    prerelease_forms: [rc, beta]
  changelog:
    categories_fr: false
  readme:
    sections: [presentation, install]
    min_words: 60
  docs:
    dir: docs
    media_dir: img
    archives_scope: documentation
  excluded_accounts: [prof-dupont]
`)
		if err := Load(ws); err != nil {
			t.Fatalf("Load: %v", err)
		}
		want := DefaultConfig("licence-projet")
		want.Conventions = ConventionsConfig{Enabled: true, Since: "2026-09-01",
			Commit: ConventionsCommitConfig{Types: []string{"feat", "fix", "docs"}, Scopes: []string{},
				MaxSubjectSoft: 60, MaxSubjectHard: 80, Forbidden: []string{"wip", "."}, Language: "en"},
			Branch: ConventionsBranchConfig{Pattern: "^(feat|fix)/[a-z]+$", Exclude: []string{"gh-pages"}},
			PR:     ConventionsPRConfig{BodySections: []string{"context", "testing"}, RequireCloses: false},
			Tags: ConventionsTagsConfig{Milestones: []string{"v1.0.0@2026-12-18", "v2.0.0"},
				PrereleaseForms: []string{"rc", "beta"}},
			Changelog:        ConventionsChangelogConfig{CategoriesFR: false},
			Readme:           ConventionsReadmeConfig{Sections: []string{"presentation", "install"}, MinWords: 60},
			Docs:             ConventionsDocsConfig{Dir: "docs", MediaDir: "img", ArchivesScope: "documentation"},
			ExcludedAccounts: []string{"prof-dupont"}}
		if !reflect.DeepEqual(ws.Config, want) {
			t.Errorf("Config = %+v\nwant  %+v", ws.Config.Conventions, want.Conventions)
		}
		rules := ws.Config.Conventions.Rules()
		if !slices.Equal(rules.Types, []string{"feat", "fix", "docs"}) || rules.MaxSubjectSoft != 60 ||
			rules.MaxSubjectHard != 80 || !slices.Equal(rules.Forbidden, []string{"wip", "."}) {
			t.Errorf("Rules = %+v", rules)
		}
		rules.Types[0] = "changed"
		if ws.Config.Conventions.Commit.Types[0] != "feat" {
			t.Error("Rules shares its lists with the configuration")
		}
		paris, _ := time.LoadLocation("Europe/Paris")
		bo := ws.Config.Conventions.BuildOptions(paris)
		if bo.Since != "2026-09-01" || bo.Location != paris || !slices.Equal(bo.Rules.Types, []string{"feat", "fix", "docs"}) ||
			!slices.Equal(bo.ExcludedAccounts, []string{"prof-dupont"}) || bo.Language != "en" ||
			bo.Branch.Pattern != "^(feat|fix)/[a-z]+$" || !slices.Equal(bo.Branch.Exclude, []string{"gh-pages"}) ||
			len(bo.Milestones) != 2 || bo.Milestones[0].Due != "2026-12-18" || bo.LocalOnly ||
			!slices.Equal(bo.PrereleaseForms, []string{"rc", "beta"}) ||
			!slices.Equal(bo.PRBody.Sections, []string{"context", "testing"}) || bo.PRBody.RequireCloses {
			t.Errorf("BuildOptions = %+v", bo)
		}
		bo.PRBody.Sections[0] = "changed"
		if ws.Config.Conventions.PR.BodySections[0] != "context" {
			t.Error("PRBodyRules shares its list with the configuration")
		}
		if d := DefaultConfig("x").Conventions.PRBodyRules(); !reflect.DeepEqual(d, conventions.DefaultPRBodyRules()) {
			t.Errorf("default PRBodyRules = %+v\nwant the charter %+v", d, conventions.DefaultPRBodyRules())
		}
		broken := ws.Config.Conventions
		broken.Tags.Milestones = []string{"1.0"}
		if got := broken.BuildOptions(paris).Milestones; got != nil {
			t.Errorf("unreadable milestones: %+v, want none", got)
		}
		bo.ExcludedAccounts[0], bo.PrereleaseForms[0] = "changed", "changed"
		if ws.Config.Conventions.ExcludedAccounts[0] != "prof-dupont" || ws.Config.Conventions.Tags.PrereleaseForms[0] != "rc" {
			t.Error("BuildOptions shares its lists with the configuration")
		}
		wantFiles := conventions.FileRules{CategoriesFR: false, ReadmeSections: []string{"presentation", "install"},
			ReadmeMinWords: 60, DocsDir: "docs", MediaDir: "img", ArchivesScope: "documentation"}
		if !reflect.DeepEqual(bo.Files, wantFiles) {
			t.Errorf("BuildOptions.Files = %+v\nwant %+v", bo.Files, wantFiles)
		}
		bo.Files.ReadmeSections[0] = "changed"
		if ws.Config.Conventions.Readme.Sections[0] != "presentation" {
			t.Error("FileRules shares its lists with the configuration")
		}
		if d := DefaultConfig("x").Conventions.FileRules(); !reflect.DeepEqual(d, conventions.DefaultFileRules()) {
			t.Errorf("default FileRules = %+v\nwant the charter %+v", d, conventions.DefaultFileRules())
		}
		br := ws.Config.Conventions.BranchRules()
		if br.Pattern != "^(feat|fix)/[a-z]+$" || !slices.Equal(br.Types, []string{"feat", "fix", "docs"}) ||
			!slices.Equal(br.Exclude, []string{"gh-pages"}) {
			t.Errorf("BranchRules = %+v", br)
		}
		br.Types[0], br.Exclude[0] = "changed", "changed"
		if ws.Config.Conventions.Commit.Types[0] != "feat" || ws.Config.Conventions.Branch.Exclude[0] != "gh-pages" {
			t.Error("BranchRules shares its lists with the configuration")
		}
		ms, err := ws.Config.Conventions.Milestones()
		if err != nil || len(ms) != 2 || ms[0].Name != "v1.0.0" || ms[0].Due != "2026-12-18" ||
			ms[1].Name != "v2.0.0" || ms[1].HasDue() {
			t.Errorf("Milestones = %+v, %v", ms, err)
		}
	})
	t.Run("quoted date", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "conventions:\n  since: \"2026-09-01\"\n")
		if err := Load(ws); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if ws.Config.Conventions.Since != "2026-09-01" {
			t.Errorf("Since = %q", ws.Config.Conventions.Since)
		}
	})
	t.Run("WATCHOO_CONVENTIONS_SINCE overrides the file", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "conventions:\n  since: 2026-09-01\n")
		t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2027-01-15")
		if err := Load(ws); err != nil {
			t.Fatalf("Load: %v", err)
		}
		if ws.Config.Conventions.Since != "2027-01-15" {
			t.Errorf("Since = %q, want 2027-01-15", ws.Config.Conventions.Since)
		}
	})
	t.Run("unquoted timestamp with a time is refused", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "conventions:\n  since: 2026-09-01T08:00:00Z\n")
		assertErrorContains(t, Load(ws), "conventions.since", `"2026-09-01T08:00:00Z"`, "YYYY-MM-DD")
	})
	t.Run("unquoted timestamp with an offset is refused", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "conventions:\n  since: 2026-09-01T02:00:00+02:00\n")
		assertErrorContains(t, Load(ws), "conventions.since", "YYYY-MM-DD")
	})
	t.Run("quoted timestamp at midnight is refused", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "conventions:\n  since: \"2026-09-01T00:00:00Z\"\n")
		assertErrorContains(t, Load(ws), "conventions.since", `"2026-09-01T00:00:00Z"`, "YYYY-MM-DD")
	})
	t.Run("unquoted timestamp at midnight UTC is read as its date", func(t *testing.T) {
		// The YAML decoder gives the same value for 2026-09-01 and 2026-09-01T00:00:00Z: documented tolerance.
		for _, raw := range []string{"2026-09-01T00:00:00Z", "2026-09-01 00:00:00"} {
			ws, _ := newLoadWorkspace(t)
			writeFile(t, ws.ConfigPath(), "conventions:\n  since: "+raw+"\n")
			if err := Load(ws); err != nil {
				t.Fatalf("Load(%s): %v", raw, err)
			}
			if ws.Config.Conventions.Since != "2026-09-01" {
				t.Errorf("Since(%s) = %q, want 2026-09-01", raw, ws.Config.Conventions.Since)
			}
		}
	})
	t.Run("invalid date in WATCHOO_CONVENTIONS_SINCE", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		t.Setenv("WATCHOO_CONVENTIONS_SINCE", "septembre")
		assertErrorContains(t, Load(ws), "conventions.since", `"septembre"`)
	})
	t.Run("unknown key in the block", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		writeFile(t, ws.ConfigPath(), "conventions:\n  commit:\n    type: [feat]\n")
		assertErrorContains(t, Load(ws), "conventions.commit.type")
	})
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

// A YAML timestamp (unquoted date) decodes to a string: the written date, or RFC 3339 with a time.
func TestDecodeSettingTimestamp(t *testing.T) {
	cases := map[time.Time]string{
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC):                  "2026-09-01",
		time.Date(2026, 9, 1, 8, 30, 0, 0, time.UTC):                 "2026-09-01T08:30:00Z",
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.FixedZone("x", 3600)): "2026-09-01T00:00:00+01:00",
	}
	for raw, want := range cases {
		var s string
		if err := decodeSetting(&s, raw); err != nil || s != want {
			t.Errorf("decodeSetting(%v) = %q, %v; want %q", raw, s, err, want)
		}
	}
	var list []string
	if err := decodeSetting(&list, []any{time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}); err != nil ||
		!slices.Equal(list, []string{"2026-09-01"}) {
		t.Errorf("list of dates = %q, %v", list, err)
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
