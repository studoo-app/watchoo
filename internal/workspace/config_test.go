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
	_ "time/tzdata" // fuseaux IANA disponibles même sans base système (CI minimale)

	"github.com/spf13/viper"
)

var update = flag.Bool("update", false, "réécrit config.example.yaml à partir de ConfigTemplate")

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("licence-projet")
	want := Config{
		Name:     "licence-projet",
		Sync:     SyncConfig{Auto: true, Interval: 30 * time.Minute, Parallelism: 4},
		Activity: ActivityConfig{FlemmeAfterDays: 5, DeadAfterDays: 15},
		Branches: BranchesConfig{Exclude: []string{}},
		Web:      WebConfig{Addr: "127.0.0.1:8080"},
		Timezone: "",
		GitHub:   GitHubConfig{User: "", GHPath: "gh"},
	}
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("DefaultConfig = %+v\nattendu %+v", cfg, want)
	}
	if cfg.Branches.Exclude == nil {
		t.Error("Branches.Exclude doit être une slice vide non nil")
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("la configuration par défaut doit être valide : %v", err)
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
		"web.addr", "timezone", "github.user", "github.gh_path",
	}
	if !slices.Equal(keys, wantKeys) {
		t.Errorf("clés = %v\nattendu %v", keys, wantKeys)
	}
	wantEnvs := []string{
		"WATCHOO_NAME", "WATCHOO_SYNC_AUTO", "WATCHOO_SYNC_INTERVAL", "WATCHOO_SYNC_PARALLELISM",
		"WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS", "WATCHOO_ACTIVITY_DEAD_AFTER_DAYS", "WATCHOO_BRANCHES_EXCLUDE",
		"WATCHOO_WEB_ADDR", "WATCHOO_TIMEZONE", "WATCHOO_GITHUB_USER", "WATCHOO_GITHUB_GH_PATH",
	}
	if !slices.Equal(envs, wantEnvs) {
		t.Errorf("variables = %v\nattendu %v", envs, wantEnvs)
	}
}

func TestValidateAcceptsBoundaries(t *testing.T) {
	cases := map[string]func(*Config){
		"synchro manuelle seule":     func(c *Config) { c.Sync.Interval = 0 },
		"intervalle minimum":         func(c *Config) { c.Sync.Interval = 15 * time.Minute },
		"intervalle long":            func(c *Config) { c.Sync.Interval = 24 * time.Hour },
		"parallélisme 1":             func(c *Config) { c.Sync.Parallelism = 1 },
		"flemme 1, dead 2":           func(c *Config) { c.Activity = ActivityConfig{FlemmeAfterDays: 1, DeadAfterDays: 2} },
		"motifs d'exclusion":         func(c *Config) { c.Branches.Exclude = []string{"dependabot/*", "renovate/*", "wip-?"} },
		"toutes les interfaces":      func(c *Config) { c.Web.Addr = ":8080" },
		"IPv6":                       func(c *Config) { c.Web.Addr = "[::1]:8080" },
		"port choisi par le système": func(c *Config) { c.Web.Addr = "localhost:0" },
		"fuseau IANA":                func(c *Config) { c.Timezone = "Europe/Paris" },
		"fuseau UTC":                 func(c *Config) { c.Timezone = "UTC" },
		"compte gh":                  func(c *Config) { c.GitHub.User = "prof-dupont" },
		"chemin de gh":               func(c *Config) { c.GitHub.GHPath = "/opt/homebrew/bin/gh" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig("espace")
			mutate(&cfg)
			if err := cfg.Validate(); err != nil {
				t.Errorf("Validate : %v", err)
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
		{"nom vide", func(c *Config) { c.Name = "  " }, []string{"name :", "vide"}},
		{"intervalle trop court", func(c *Config) { c.Sync.Interval = 10 * time.Minute },
			[]string{"sync.interval : 10m0s est inférieur au minimum de 15 min (0 pour la synchro manuelle seule)"}},
		{"intervalle négatif", func(c *Config) { c.Sync.Interval = -time.Minute },
			[]string{"sync.interval", "-1m0s", "négatif"}},
		{"parallélisme nul", func(c *Config) { c.Sync.Parallelism = 0 },
			[]string{"sync.parallelism : 0", "entre 1 et 4"}},
		{"parallélisme trop grand", func(c *Config) { c.Sync.Parallelism = 5 },
			[]string{"sync.parallelism : 5", "entre 1 et 4"}},
		{"flemme nulle", func(c *Config) { c.Activity.FlemmeAfterDays = 0 },
			[]string{"activity.flemme_after_days : 0", "au moins 1"}},
		{"dead égal à flemme", func(c *Config) { c.Activity = ActivityConfig{FlemmeAfterDays: 7, DeadAfterDays: 7} },
			[]string{"activity.dead_after_days : 7", "strictement supérieur", "(7)"}},
		{"dead inférieur à flemme", func(c *Config) { c.Activity = ActivityConfig{FlemmeAfterDays: 10, DeadAfterDays: 3} },
			[]string{"activity.dead_after_days : 3"}},
		{"motif vide", func(c *Config) { c.Branches.Exclude = []string{"main", " "} },
			[]string{"branches.exclude", "n° 2", "vide"}},
		{"motif invalide", func(c *Config) { c.Branches.Exclude = []string{"release/[a"} },
			[]string{"branches.exclude", `"release/[a"`, "invalide"}},
		{"adresse sans port", func(c *Config) { c.Web.Addr = "127.0.0.1" },
			[]string{"web.addr", `"127.0.0.1"`, "hôte:port"}},
		{"adresse vide", func(c *Config) { c.Web.Addr = "" }, []string{"web.addr", "hôte:port"}},
		{"port vide", func(c *Config) { c.Web.Addr = "127.0.0.1:" }, []string{"web.addr", "port manquant"}},
		{"port non numérique", func(c *Config) { c.Web.Addr = "127.0.0.1:80a" }, []string{"web.addr", `port "80a"`}},
		{"port trop grand", func(c *Config) { c.Web.Addr = "127.0.0.1:70000" }, []string{"web.addr", "65535"}},
		{"fuseau inconnu", func(c *Config) { c.Timezone = "Mars/Olympus" },
			[]string{"timezone", `"Mars/Olympus"`, "IANA"}},
		{"chemin de gh vide", func(c *Config) { c.GitHub.GHPath = "" }, []string{"github.gh_path", "vide"}},
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
		t.Errorf("%d messages, attendu 3 (un par problème) :\n%v", n, err)
	}
}

func TestLocation(t *testing.T) {
	cfg := DefaultConfig("espace")
	if loc, err := cfg.Location(); err != nil || loc != time.Local {
		t.Errorf("Location() avec timezone vide = %v, %v ; attendu time.Local", loc, err)
	}
	cfg.Timezone = "Europe/Paris"
	if loc, err := cfg.Location(); err != nil || loc.String() != "Europe/Paris" {
		t.Errorf("Location() = %v, %v ; attendu Europe/Paris", loc, err)
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
		t.Errorf("avec XDG_CONFIG_HOME : %q, %v", got, err)
	}

	want := filepath.Join(home, ".config", "watchoo", "config.yaml")
	for _, xdg := range []string{"", "relatif/xdg"} {
		t.Setenv("XDG_CONFIG_HOME", xdg)
		if got, err := UserConfigPath(); err != nil || got != want {
			t.Errorf("XDG_CONFIG_HOME=%q : %q, %v ; attendu %q (jamais ~/Library/Application Support)",
				xdg, got, err, want)
		}
	}
}

// newLoadWorkspace crée un espace « licence-projet » (isolé de l'environnement) et renvoie l'espace
// et le chemin du fichier de réglages utilisateur.
func newLoadWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	userConfig := isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	return mustInit(t, root), userConfig
}

func TestLoadDefaults(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v, attendu les valeurs par défaut", ws.Config)
	}
}

func TestLoadBeforeInit(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "pas-encore")
	ws := &Workspace{Root: root, Dir: filepath.Join(root, DirName)}
	t.Setenv("WATCHOO_GITHUB_GH_PATH", "/opt/gh")

	if err := Load(ws); err != nil {
		t.Fatalf("Load sans .watchoo : %v", err)
	}
	want := DefaultConfig("pas-encore")
	want.GitHub.GHPath = "/opt/gh"
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v, attendu %+v", ws.Config, want)
	}
}

func TestLoadUserConfig(t *testing.T) {
	userConfig := isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	writeFile(t, userConfig, "github:\n  user: compte-utilisateur\ntimezone: Europe/Paris\n")
	t.Setenv("WATCHOO_GITHUB_GH_PATH", "/opt/gh")

	cfg, err := LoadUserConfig(root)
	if err != nil {
		t.Fatalf("LoadUserConfig : %v", err)
	}
	want := DefaultConfig("licence-projet")
	want.GitHub.User = "compte-utilisateur" // utilisateur > défaut
	want.Timezone = "Europe/Paris"
	want.GitHub.GHPath = "/opt/gh" // environnement > utilisateur
	if !reflect.DeepEqual(cfg, want) {
		t.Errorf("Config = %+v\nattendu  %+v", cfg, want)
	}
}

func TestLoadUserConfigIgnoresWorkspaceFile(t *testing.T) {
	isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")
	ws := mustInit(t, root)
	// Un config.yaml d'espace invalide ne doit pas être lu : init refusera l'espace existant ensuite.
	writeFile(t, ws.ConfigPath(), "cle_inconnue: 1\nsync:\n  interval: 1m\n")

	cfg, err := LoadUserConfig(root)
	if err != nil {
		t.Fatalf("LoadUserConfig : %v", err)
	}
	if !reflect.DeepEqual(cfg, DefaultConfig("licence-projet")) {
		t.Errorf("Config = %+v, attendu les valeurs par défaut", cfg)
	}
}

func TestLoadUserConfigErrors(t *testing.T) {
	userConfig := isolateEnv(t)
	root := newDir(t, t.TempDir(), "licence-projet")

	_, err := LoadUserConfig("")
	assertErrorContains(t, err, "répertoire non renseigné")

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
		t.Fatalf("Load : %v", err)
	}
	want := DefaultConfig("licence-projet")
	want.Name = "Licence 2026"                                            // espace > défaut
	want.Timezone = "Europe/Paris"                                        // utilisateur > défaut
	want.Sync.Interval = 3 * time.Hour                                    // environnement > espace > utilisateur
	want.Sync.Parallelism = 2                                             // utilisateur, non remplacé par l'espace
	want.Activity = ActivityConfig{FlemmeAfterDays: 3, DeadAfterDays: 20} // fusion clé par clé
	want.Branches.Exclude = []string{"renovate/*", "wip"}                 // la liste de l'espace remplace celle de l'utilisateur
	want.GitHub.User = "compte-env"                                       // environnement > utilisateur
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v\nattendu  %+v", ws.Config, want)
	}
}

func TestLoadUserConfigFromHome(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	home := os.Getenv("HOME")
	writeFile(t, filepath.Join(home, ".config", "watchoo", "config.yaml"), "timezone: UTC\n")
	// Ce fichier ne doit jamais être lu (os.UserConfigDir sous macOS).
	writeFile(t, filepath.Join(home, "Library", "Application Support", "watchoo", "config.yaml"),
		"timezone: Asia/Tokyo\n")

	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if ws.Config.Timezone != "UTC" {
		t.Errorf("Timezone = %q, attendu UTC (lu dans ~/.config/watchoo/config.yaml)", ws.Config.Timezone)
	}
}

func TestLoadWithoutHome(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	if err := Load(ws); err != nil {
		t.Fatalf("Load sans répertoire personnel : %v", err)
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
		"WATCHOO_TIMEZONE":                   "America/Montreal",
		"WATCHOO_GITHUB_USER":                "prof",
		"WATCHOO_GITHUB_GH_PATH":             "/usr/local/bin/gh",
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	want := Config{
		Name:     "Depuis l'environnement",
		Sync:     SyncConfig{Auto: false, Interval: 0, Parallelism: 1},
		Activity: ActivityConfig{FlemmeAfterDays: 2, DeadAfterDays: 9},
		Branches: BranchesConfig{Exclude: []string{"dependabot/*", "renovate/*"}},
		Web:      WebConfig{Addr: "0.0.0.0:9090"},
		Timezone: "America/Montreal",
		GitHub:   GitHubConfig{User: "prof", GHPath: "/usr/local/bin/gh"},
	}
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v\nattendu  %+v", ws.Config, want)
	}
}

func TestLoadEmptyEnvironmentVariableIgnored(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "timezone: Europe/Paris\n")
	t.Setenv("WATCHOO_TIMEZONE", "")
	t.Setenv(EnvDir, ws.Root) // n'est pas un réglage
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if ws.Config.Timezone != "Europe/Paris" {
		t.Errorf("Timezone = %q, une variable vide doit être ignorée", ws.Config.Timezone)
	}
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
		t.Fatalf("Load : %v", err)
	}
	want := DefaultConfig("licence-projet")
	want.Name = "2026"
	want.Sync = SyncConfig{Auto: false, Interval: 0, Parallelism: 3}
	want.Branches.Exclude = []string{"2024", "dependabot/*"}
	if !reflect.DeepEqual(ws.Config, want) {
		t.Errorf("Config = %+v\nattendu  %+v", ws.Config, want)
	}
}

func TestLoadEmptyValuesIgnored(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "timezone: Europe/Paris\nsync:\n  auto: false\nbranches:\n  exclude: [a]\n")
	// Sections vides et clés sans valeur : rien n'est remplacé.
	writeFile(t, ws.ConfigPath(), "timezone:\nsync:\n  auto:\nbranches:\n  exclude:\nweb:\nactivity: {}\n")
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if ws.Config.Timezone != "Europe/Paris" || ws.Config.Sync.Auto ||
		!slices.Equal(ws.Config.Branches.Exclude, []string{"a"}) {
		t.Errorf("Config = %+v : les valeurs de l'utilisateur doivent être conservées", ws.Config)
	}
}

func TestLoadRejectsUnknownKeys(t *testing.T) {
	cases := []struct {
		name, content string
		want          []string
	}{
		{"faute de frappe", "sync:\n  intervall: 1h\n",
			[]string{`clé inconnue "sync.intervall"`, "vouliez-vous dire « sync.interval »"}},
		{"clé hors de sa section", "interval: 1h\n",
			[]string{`clé inconnue "interval"`, "« sync.interval »"}},
		{"clé inconnue", "couleur: bleu\n", []string{`clé inconnue "couleur"`}},
		{"clé inconnue sans valeur", "couleur:\n", []string{`clé inconnue "couleur"`}},
		{"section inconnue", "token:\n  value: x\n", []string{`clé inconnue "token.value"`}},
		{"section avec une valeur", "sync: 5\n", []string{`"sync" est une section`, "sync.auto"}},
		{"réglage avec des sous-clés", "web:\n  addr:\n    host: x\n",
			[]string{`"web.addr" n'accepte pas de sous-clés`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws, _ := newLoadWorkspace(t)
			writeFile(t, ws.ConfigPath(), tc.content)
			before := ws.Config
			err := Load(ws)
			assertErrorContains(t, err, append([]string{ws.ConfigPath()}, tc.want...)...)
			if !reflect.DeepEqual(ws.Config, before) {
				t.Errorf("ws.Config modifié malgré l'erreur")
			}
		})
	}
}

func TestLoadRejectsInvalidFileValues(t *testing.T) {
	cases := []struct {
		name, content string
		want          []string
	}{
		{"durée illisible", "sync:\n  interval: bientôt\n",
			[]string{"sync.interval", `durée invalide "bientôt"`}},
		{"durée sans unité", "sync:\n  interval: 30\n", []string{"sync.interval", "durée sans unité 30"}},
		{"intervalle trop court", "sync:\n  interval: 5m\n",
			[]string{"réglages invalides", "sync.interval : 5m0s est inférieur au minimum de 15 min"}},
		{"booléen", "sync:\n  auto: oui\n", []string{"sync.auto", "true ou false attendu", `"oui"`}},
		{"entier", "sync:\n  parallelism: deux\n", []string{"sync.parallelism", "nombre entier attendu"}},
		{"entier décimal", "activity:\n  flemme_after_days: 2.5\n",
			[]string{"activity.flemme_after_days", "nombre entier attendu", "2.5"}},
		{"liste attendue", "branches:\n  exclude: {a: 1}\n", []string{"branches.exclude"}},
		{"élément de liste", "branches:\n  exclude: [[a]]\n", []string{"branches.exclude", "élément n° 1"}},
		{"chaîne attendue", "web:\n  addr: [a, b]\n", []string{"web.addr", "chaîne de caractères attendue"}},
		{"parallélisme hors limites", "sync:\n  parallelism: 9\n", []string{"sync.parallelism : 9", "entre 1 et 4"}},
		{"seuils incohérents", "activity:\n  flemme_after_days: 20\n",
			[]string{"activity.dead_after_days : 15", "strictement supérieur"}},
		{"fuseau inconnu", "timezone: Lune/Base\n", []string{"timezone", "Lune/Base"}},
		{"plusieurs erreurs", "sync:\n  auto: peut-être\n  parallelism: x\n",
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
	assertErrorContains(t, Load(ws), ws.ConfigPath(), "YAML invalide")

	writeFile(t, ws.ConfigPath(), "- une\n- liste\n")
	assertErrorContains(t, Load(ws), ws.ConfigPath(), "YAML invalide")

	writeFile(t, ws.ConfigPath(), "name: a\nname: b\n")
	assertErrorContains(t, Load(ws), ws.ConfigPath(), "YAML invalide")
}

func TestLoadUnreadableFile(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	if err := os.Remove(ws.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	newDir(t, ws.ConfigPath()) // un répertoire à la place du fichier
	assertErrorContains(t, Load(ws), "lecture des réglages", ws.ConfigPath())
}

func TestLoadMissingWorkspaceFile(t *testing.T) {
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "sync:\n  parallelism: 2\n")
	if err := os.Remove(ws.ConfigPath()); err != nil {
		t.Fatal(err)
	}
	if err := Load(ws); err != nil {
		t.Fatalf("Load sans config.yaml : %v", err)
	}
	if ws.Config.Sync.Parallelism != 2 || ws.Config.Name != "licence-projet" {
		t.Errorf("Config = %+v", ws.Config)
	}
}

func TestLoadRejectsInvalidEnvironment(t *testing.T) {
	cases := map[string][]string{
		"WATCHOO_SYNC_INTERVAL":            {"variable WATCHOO_SYNC_INTERVAL (sync.interval)", "durée invalide"},
		"WATCHOO_SYNC_AUTO":                {"variable WATCHOO_SYNC_AUTO (sync.auto)", "true ou false"},
		"WATCHOO_SYNC_PARALLELISM":         {"variable WATCHOO_SYNC_PARALLELISM", "nombre entier"},
		"WATCHOO_ACTIVITY_DEAD_AFTER_DAYS": {"variable WATCHOO_ACTIVITY_DEAD_AFTER_DAYS", "nombre entier"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			ws, _ := newLoadWorkspace(t)
			t.Setenv(name, "pas-valable")
			assertErrorContains(t, Load(ws), want...)
		})
	}

	t.Run("durée sans unité", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		t.Setenv("WATCHOO_SYNC_INTERVAL", "10")
		assertErrorContains(t, Load(ws), "variable WATCHOO_SYNC_INTERVAL", `durée sans unité "10"`)
	})

	t.Run("valeur refusée par Validate", func(t *testing.T) {
		ws, _ := newLoadWorkspace(t)
		t.Setenv("WATCHOO_SYNC_INTERVAL", "1m")
		assertErrorContains(t, Load(ws), "sync.interval : 1m0s est inférieur au minimum")
	})
}

func TestLoadCaseInsensitiveKeys(t *testing.T) {
	ws, _ := newLoadWorkspace(t)
	writeFile(t, ws.ConfigPath(), "Sync:\n  Interval: 45m\n")
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if ws.Config.Sync.Interval != 45*time.Minute {
		t.Errorf("Interval = %v, attendu 45m", ws.Config.Sync.Interval)
	}
}

func TestLoadRequiresWorkspace(t *testing.T) {
	assertErrorContains(t, Load(nil), "espace non renseigné")
	assertErrorContains(t, Load(&Workspace{}), "espace non renseigné")
}

// trickyNames sont des noms d'espace qui exigent un échappement YAML correct.
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
			writeFile(t, ws.ConfigPath(), ConfigTemplate(name))
			if err := Load(ws); err != nil {
				t.Fatalf("Load : %v\n%s", err, ConfigTemplate(name))
			}
			if !reflect.DeepEqual(ws.Config, DefaultConfig(name)) {
				t.Errorf("Config = %+v\nattendu  %+v", ws.Config, DefaultConfig(name))
			}
		})
	}
}

func TestConfigTemplateKeepsUserSettings(t *testing.T) {
	// Seul le nom est actif dans le modèle : les réglages de l'utilisateur continuent de s'appliquer.
	ws, userConfig := newLoadWorkspace(t)
	writeFile(t, userConfig, "name: ignoré\ntimezone: Europe/Paris\nsync:\n  interval: 1h\n")
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v", err)
	}
	if ws.Config.Name != "licence-projet" || ws.Config.Timezone != "Europe/Paris" ||
		ws.Config.Sync.Interval != time.Hour {
		t.Errorf("Config = %+v", ws.Config)
	}
}

// settingLine reconnaît une ligne de réglage en commentaire du modèle : « # clé: valeur » ou
// « #   clé: valeur » (dans une section).
var settingLine = regexp.MustCompile(`^# ( *)([a-z_]+):(.*)$`)

func TestConfigTemplateUncommentedEqualsDefaults(t *testing.T) {
	name := "licence-projet"
	var b strings.Builder
	for line := range strings.Lines(ConfigTemplate(name)) {
		b.WriteString(settingLine.ReplaceAllString(strings.TrimSuffix(line, "\n"), "$1$2:$3"))
		b.WriteByte('\n')
	}
	uncommented := b.String()

	// Toutes les clés connues figurent dans le modèle.
	fv := viper.New()
	fv.SetConfigType("yaml")
	if err := fv.ReadConfig(bytes.NewReader([]byte(uncommented))); err != nil {
		t.Fatalf("modèle décommenté illisible : %v\n%s", err, uncommented)
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
		t.Errorf("clés du modèle décommenté = %v\nattendu %v", got, want)
	}

	// Et leurs valeurs sont bien les valeurs par défaut.
	ws, _ := newLoadWorkspace(t)
	writeFile(t, ws.ConfigPath(), uncommented)
	if err := Load(ws); err != nil {
		t.Fatalf("Load : %v\n%s", err, uncommented)
	}
	if !reflect.DeepEqual(ws.Config, DefaultConfig(name)) {
		t.Errorf("Config = %+v\nattendu  %+v", ws.Config, DefaultConfig(name))
	}
}

func TestConfigTemplateIsCommentedInFrench(t *testing.T) {
	tpl := ConfigTemplate("licence-projet")
	for _, part := range []string{
		"Réglages de l'espace de supervision Watchoo",
		"~/.config/watchoo/config.yaml",
		"WATCHOO_SYNC_INTERVAL",
		"gh auth token",
		`name: "licence-projet"`,
	} {
		if !strings.Contains(tpl, part) {
			t.Errorf("le modèle ne contient pas %q", part)
		}
	}
	for line := range strings.Lines(tpl) {
		line = strings.TrimSuffix(line, "\n")
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "name: ") {
			t.Errorf("seule la clé name doit être active, ligne trouvée : %q", line)
		}
	}
}

func TestConfigExampleFile(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	want := ConfigTemplate("licence-projet")
	if *update {
		if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lecture de config.example.yaml : %v (régénérer avec -update)", err)
	}
	if string(got) != want {
		t.Errorf("config.example.yaml n'est pas à jour : relancez « go test ./internal/workspace " +
			"-run TestConfigExampleFile -update »")
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
			t.Errorf("decodeSetting(%T, %#v) : erreur attendue", tc.ptr, tc.raw)
		}
	}
	if err := decodeSetting(new(float64), 1.0); err == nil {
		t.Error("un type de champ non géré doit être signalé")
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
			t.Errorf("shortDuration(%v) = %q, attendu %q", d, got, want)
		}
		if parsed, err := time.ParseDuration(shortDuration(d)); err != nil || parsed != d {
			t.Errorf("shortDuration(%v) ne se relit pas : %v, %v", d, parsed, err)
		}
	}
}
