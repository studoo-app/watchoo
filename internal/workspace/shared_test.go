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

// customConfig est un config.yaml d'espace avec des clés machine, un réglage commun et un commentaire
// personnel.
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
`

// newCustomWorkspace crée un espace dont config.yaml vaut customConfig.
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
		t.Errorf("branches.exclude = %#v, liste vide attendue", got)
	}
	if v[KeySyncInterval] != DefaultSyncInterval || v[KeySyncAuto] != true || v[KeyTimezone] != "" {
		t.Errorf("valeurs = %v", v)
	}
	if !(SharedSettings{}).IsZero() || s.IsZero() {
		t.Error("IsZero incorrect")
	}
	// Les listes sont copiées : modifier la source ne change pas les réglages.
	cfg.Branches.Exclude = []string{"a"}
	s = SharedSettingsFrom(cfg)
	cfg.Branches.Exclude[0] = "b"
	if (*s.BranchesExclude)[0] != "a" {
		t.Error("branches.exclude non copiée")
	}
}

func TestSharedSettingsApplyTo(t *testing.T) {
	cfg := DefaultConfig("x")
	cfg.GitHub.GHPath = "/usr/bin/gh"
	s := SharedSettings{SyncInterval: ptr(time.Hour), Timezone: ptr("Europe/Paris"), BranchesExclude: &[]string{}}
	got := s.ApplyTo(cfg)
	if got.Sync.Interval != time.Hour || got.Timezone != "Europe/Paris" || got.Branches.Exclude == nil ||
		len(got.Branches.Exclude) != 0 {
		t.Errorf("ApplyTo = %+v", got)
	}
	if got.Sync.Parallelism != cfg.Sync.Parallelism || got.GitHub.GHPath != "/usr/bin/gh" || got.Name != "x" {
		t.Errorf("valeurs non fournies modifiées : %+v", got)
	}
	if cfg.Sync.Interval != DefaultSyncInterval {
		t.Error("ApplyTo a modifié la configuration d'origine")
	}
	if !slices.Equal(s.Keys(), []string{KeySyncInterval, KeyBranchesExclude, KeyTimezone}) {
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
		t.Errorf("EnvOverrides = %v, attendu %v", got, want)
	}
	if EnvVarName(KeyFlemmeAfterDays) != "WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS" {
		t.Error(EnvVarName(KeyFlemmeAfterDays))
	}
}

// FileConfig ignore les variables WATCHOO_* ; WithEnv les réapplique comme Load.
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
		t.Error("WithEnv a modifié son argument")
	}
	t.Setenv("WATCHOO_SYNC_PARALLELISM", "beaucoup")
	if _, err := WithEnv(fileCfg); err == nil || !strings.Contains(err.Error(), "WATCHOO_SYNC_PARALLELISM") {
		t.Errorf("WithEnv avec une variable illisible = %v", err)
	}
	if _, err := FileConfig(nil); err == nil {
		t.Error("FileConfig(nil) accepté")
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
			t.Errorf("FormatSettingValue(%#v) = %s, attendu %s", tt.v, got, tt.want)
		}
	}
}

// Les clés machine (name, web.*, github.*, git.*) et les réglages communs non importés sont gardés, l'ancien
// fichier est copié dans .bak, les droits sont 0600, et le fichier relu donne les valeurs attendues.
func TestWriteSharedSettingsKeepsMachineKeys(t *testing.T) {
	ws := newCustomWorkspace(t)
	before := readFile(t, ws.ConfigPath())
	s := SharedSettings{
		SyncInterval:    ptr(time.Hour),
		DeadAfterDays:   ptr(21),
		BranchesExclude: &[]string{"dependabot/*", "renovate/*"},
		Timezone:        ptr("Europe/Paris"),
	}
	res, err := WriteSharedSettings(ws, s)
	if err != nil {
		t.Fatal(err)
	}
	if res.Path != ws.ConfigPath() || res.BackupPath != filepath.Join(ws.Dir, BackupFileName) {
		t.Errorf("résultat = %+v", res)
	}
	if got := readFile(t, res.BackupPath); got != before {
		t.Errorf("copie .bak différente de l'ancien fichier :\n%s", got)
	}
	assertMode(t, ws.ConfigPath(), FileMode)
	assertMode(t, res.BackupPath, FileMode)

	content := readFile(t, ws.ConfigPath())
	if strings.Contains(content, "Mon commentaire personnel") {
		t.Error("le commentaire personnel devait disparaître (il reste dans .bak)")
	}
	if !strings.HasPrefix(content, configHeader(i18n.For(i18n.English))) {
		t.Error("le fichier réécrit ne commence pas par la carte du modèle")
	}

	reloaded := &Workspace{Root: ws.Root, Dir: ws.Dir}
	if err := Load(reloaded); err != nil {
		t.Fatal(err)
	}
	c := reloaded.Config
	if c.Name != "Licence : projet # 2026" || c.Web.Addr != "127.0.0.1:9090" ||
		!slices.Equal(c.Web.AllowedHosts, []string{"watchoo.example.org"}) || c.Web.Auth.Username != "prof" ||
		c.GitHub.User != "prof-gh" || c.GitHub.GHPath != "/opt/gh/bin/gh" || c.Git.Protocol != "https" {
		t.Errorf("clés machine perdues : %+v", c)
	}
	if c.Sync.Parallelism != 2 {
		t.Errorf("sync.parallelism non importé perdu : %d", c.Sync.Parallelism)
	}
	if c.Sync.Interval != time.Hour || c.Activity.DeadAfterDays != 21 || c.Timezone != "Europe/Paris" ||
		!slices.Equal(c.Branches.Exclude, []string{"dependabot/*", "renovate/*"}) {
		t.Errorf("réglages importés absents : %+v", c)
	}
	if !reflect.DeepEqual(res.Config, c) {
		t.Errorf("WriteResult.Config = %+v, relu %+v", res.Config, c)
	}
	// Les réglages non fournis et absents du fichier restent en commentaire.
	for _, line := range []string{"#   auto: true", "#   flemme_after_days: 5"} {
		if !strings.Contains(content, line) {
			t.Errorf("ligne commentée %q absente :\n%s", line, content)
		}
	}
	for _, line := range []string{"sync:\n", "  interval: 1h ", "  parallelism: 2 ", "  dead_after_days: 21 ",
		`  exclude: ["dependabot/*", "renovate/*"]`, `timezone: "Europe/Paris"`, "  auth:\n", `    username: "prof"`,
		`  gh_path: "/opt/gh/bin/gh"`, "git:\n", `  protocol: "https"`} {
		if !strings.Contains(content, line) {
			t.Errorf("ligne active %q absente :\n%s", line, content)
		}
	}
	// ws.Config n'est pas modifié (un serve en cours garde sa configuration).
	if ws.Config.Sync.Interval != DefaultSyncInterval {
		t.Error("ws.Config modifié")
	}
}

// Deux écritures des mêmes réglages donnent le même fichier.
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
		t.Errorf("deuxième écriture différente :\n%s\n---\n%s", first, second)
	}
	if bak := readFile(t, filepath.Join(ws.Dir, BackupFileName)); bak != first {
		t.Error("la copie .bak doit être le fichier précédent")
	}
}

// Un motif contenant guillemets, sauts de ligne, « # » et « : » reste une valeur : il ne crée jamais
// d'autre clé (github.gh_path en particulier).
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
		t.Errorf("branches.exclude = %q, attendu %q", reloaded.Config.Branches.Exclude, evil)
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
			"web.auth.username", "github.user", "github.gh_path", "git.protocol"}, key) {
			t.Errorf("clé inattendue %s dans le fichier réécrit", key)
		}
	}
}

// Une valeur refusée (fuseau inconnu, injection comprise, ou réglage incohérent) n'écrit rien : ni
// config.yaml, ni .bak, ni fichier temporaire.
func TestWriteSharedSettingsInvalidChangesNothing(t *testing.T) {
	for name, s := range map[string]SharedSettings{
		"fuseau":       {Timezone: ptr("Europe/Paris\"\ngithub:\n  gh_path: /tmp/evil")},
		"parallélisme": {SyncParallelism: ptr(9)},
		"seuils":       {FlemmeAfterDays: ptr(20)},
		"utf8":         {Timezone: ptr("\xff")},
		"motif":        {BranchesExclude: &[]string{"[a"}},
	} {
		t.Run(name, func(t *testing.T) {
			ws := newCustomWorkspace(t)
			before := readFile(t, ws.ConfigPath())
			if _, err := WriteSharedSettings(ws, s); err == nil {
				t.Fatal("erreur attendue")
			}
			if got := readFile(t, ws.ConfigPath()); got != before {
				t.Errorf("config.yaml modifié :\n%s", got)
			}
			assertOnlyFiles(t, ws.Dir, ConfigFileName, GitignoreFileName)
		})
	}
}

// Sans config.yaml, le fichier est créé (sans .bak) depuis le modèle, sans ligne name active.
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
		t.Errorf("BackupPath = %q, vide attendu", res.BackupPath)
	}
	content := readFile(t, ws.ConfigPath())
	if !strings.Contains(content, `# name: "master"`) || !strings.Contains(content, "  flemme_after_days: 3 ") {
		t.Errorf("contenu :\n%s", content)
	}
	assertMode(t, ws.ConfigPath(), FileMode)
	if res.Config.Name != "master" || res.Config.Activity.FlemmeAfterDays != 3 {
		t.Errorf("Config = %+v", res.Config)
	}
}

// Un échec d'écriture du fichier temporaire ne change rien.
func TestWriteSharedSettingsWriteFailure(t *testing.T) {
	ws := newCustomWorkspace(t)
	before := readFile(t, ws.ConfigPath())
	boom := errors.New("disque plein")
	orig := writeTemp
	writeTemp = func(string, string, []byte) (string, error) { return "", boom }
	t.Cleanup(func() { writeTemp = orig })
	if _, err := WriteSharedSettings(ws, SharedSettings{SyncAuto: ptr(false)}); !errors.Is(err, boom) {
		t.Fatalf("erreur = %v", err)
	}
	if got := readFile(t, ws.ConfigPath()); got != before {
		t.Error("config.yaml modifié")
	}
}

// L'espace doit appartenir à l'utilisateur, comme pour Load.
func TestWriteSharedSettingsUntrusted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("droits Unix")
	}
	ws := newCustomWorkspace(t)
	if err := os.Chmod(ws.ConfigPath(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteSharedSettings(ws, SharedSettings{SyncAuto: ptr(false)}); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("erreur = %v, ErrUntrusted attendue", err)
	}
}

func TestWriteSharedSettingsRequiresWorkspace(t *testing.T) {
	if _, err := WriteSharedSettings(nil, SharedSettings{}); err == nil {
		t.Error("erreur attendue")
	}
}

// Les réglages importés et les clés machine relus valent ceux attendus, y compris pour le modèle de Init
// réécrit sans aucun réglage commun : seul name est actif.
func TestRenderConfigOnlyName(t *testing.T) {
	if got := renderConfig(i18n.For(i18n.English), "x", map[string]any{"name": "x"}); got != ConfigTemplate(i18n.English, "x") {
		t.Error("renderConfig avec name seul doit donner ConfigTemplate")
	}
}

// assertOnlyFiles vérifie que dir ne contient que les fichiers names.
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
		t.Errorf("fichiers de %s = %v, attendu %v", dir, got, names)
	}
}
