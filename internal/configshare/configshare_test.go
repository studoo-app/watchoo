package configshare_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// doc construit un document v1 avec les morceaux JSON donnés (settings et repositories peuvent être
// vides : clé absente).
func doc(settings, repositories string) string {
	parts := []string{`"format": "watchoo-config"`, `"version": 1`}
	if settings != "" {
		parts = append(parts, `"settings": `+settings)
	}
	if repositories != "" {
		parts = append(parts, `"repositories": `+repositories)
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func mustParse(t *testing.T, s string) *configshare.File {
	t.Helper()
	f, err := configshare.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Parse : %v\n%s", err, s)
	}
	return f
}

func TestExport(t *testing.T) {
	isolate(t)
	t.Setenv("WATCHOO_TIMEZONE", "UTC") // valeur EN VIGUEUR exportée, variable comprise
	e := newEnv(t, "licence-projet")
	e.ws.Config.Timezone = "UTC"
	e.track(1, "zoe", "projet", "Zoé Leroy")
	e.track(2, "Alice", "projet", "Alice Martin", "main", "release/*")
	stopped := e.track(3, "carol", "projet", "Carol")
	e.stop(stopped.ID)

	d, err := configshare.Export(t.Context(), e.ws, e.st, configshare.ExportOptions{Now: testNow.Add(500 * time.Millisecond), WatchooVersion: "v0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := d.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-27T10:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {
    "name": "licence-projet"
  },
  "settings": {
    "sync": {
      "auto": true,
      "interval": "30m",
      "parallelism": 4
    },
    "activity": {
      "flemme_after_days": 5,
      "dead_after_days": 15
    },
    "branches": {
      "exclude": []
    },
    "timezone": "UTC"
  },
  "repositories": [
    {
      "provider": "github",
      "repo": "Alice/projet",
      "label": "Alice Martin",
      "branches": [
        "main",
        "release/*"
      ]
    },
    {
      "provider": "github",
      "repo": "zoe/projet",
      "label": "Zoé Leroy",
      "branches": []
    }
  ]
}
`
	if string(data) != want {
		t.Errorf("export :\n%s\nattendu :\n%s", data, want)
	}
	for _, forbidden := range []string{"github.", "gh_path", "web", "addr", authtest.FakeToken, "@", e.ws.Root, "carol"} {
		if forbidden == "github." || forbidden == "web" {
			var m map[string]any
			_ = json.Unmarshal(data, &m)
			s := m["settings"].(map[string]any)
			if _, ok := s[strings.TrimSuffix(forbidden, ".")]; ok {
				t.Errorf("clé %s exportée", forbidden)
			}
			continue
		}
		if strings.Contains(string(data), forbidden) {
			t.Errorf("l'export contient %q", forbidden)
		}
	}
	// Le document exporté est accepté tel quel.
	if _, err := configshare.ParseBytes(data); err != nil {
		t.Errorf("export relu : %v", err)
	}
}

func TestExportEmptyWorkspace(t *testing.T) {
	isolate(t)
	e := newEnv(t, "vide")
	d, err := configshare.Export(t.Context(), e.ws, e.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()
	if !bytes.Contains(data, []byte(`"repositories": []`)) {
		t.Errorf("liste vide attendue :\n%s", data)
	}
}

// Aller-retour : export d'un espace, import dans un autre (dépôts accessibles), même configuration.
func TestRoundTrip(t *testing.T) {
	isolate(t)
	a := newEnv(t, "licence-a")
	a.ws.Config.Sync.Interval = time.Hour
	a.ws.Config.Activity.DeadAfterDays = 21
	a.ws.Config.Branches.Exclude = []string{"dependabot/*"}
	a.ws.Config.Timezone = "Europe/Paris"
	a.track(1, "alice", "projet", "Alice Martin", "main", "dev")
	a.track(2, "bob", "projet", "")
	d, err := configshare.Export(t.Context(), a.ws, a.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()

	b := newEnv(t, "licence-b")
	b.forge(1, "alice", "projet")
	b.forge(2, "bob", "projet")
	f, err := configshare.ParseBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.Source.WorkspaceName != "licence-a" || !f.Source.ExportedAt.Equal(testNow) {
		t.Errorf("source = %+v", f.Source)
	}
	res, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Incomplete() || res.Summary.Added != 2 || !res.Settings.Written {
		t.Errorf("bilan = %+v", res)
	}
	repos := b.repos()
	if r := repos["alice/projet"]; r.Label != "Alice Martin" || !slices.Equal(r.BranchPatterns, []string{"main", "dev"}) || !r.Enabled {
		t.Errorf("alice = %+v", r)
	}
	if r := repos["bob/projet"]; r.Label != "" || len(r.BranchPatterns) != 0 {
		t.Errorf("bob = %+v", r)
	}
	cfg := b.reload()
	if cfg.Name != "licence-b" || cfg.Sync.Interval != time.Hour || cfg.Activity.DeadAfterDays != 21 ||
		cfg.Timezone != "Europe/Paris" || !slices.Equal(cfg.Branches.Exclude, []string{"dependabot/*"}) {
		t.Errorf("réglages de b = %+v", cfg)
	}
	// Réexport de b : mêmes réglages et dépôts que a.
	b.ws.Config = cfg
	d2, _ := configshare.Export(t.Context(), b.ws, b.st, configshare.ExportOptions{Now: testNow})
	if !reflect.DeepEqual(d2.Settings, d.Settings) || !reflect.DeepEqual(d2.Repositories, d.Repositories) {
		t.Errorf("réexport différent :\n%+v\n%+v", d2, d)
	}
}

func TestParseAccepts(t *testing.T) {
	// Seulement des dépôts : branches absentes (garder), [] (toutes), doublons de motifs retirés, URL.
	f := mustParse(t, doc("", `[
		{"repo": "alice/projet"},
		{"provider": "github", "repo": "https://github.com/bob/projet", "label": "Bob", "branches": []},
		{"repo": "carol/projet", "branches": [" main ", "main", "release/*"]},
		{"repo": "dave/projet", "branches": null}]`))
	if !f.Settings.IsZero() || len(f.Repositories) != 4 {
		t.Fatalf("fichier = %+v", f)
	}
	r := f.Repositories
	if r[0].Branches != nil || r[0].Index != 1 || r[0].Ref.String() != "alice/projet" {
		t.Errorf("alice = %+v", r[0])
	}
	if r[1].Branches == nil || len(r[1].Branches) != 0 || r[1].Ref.String() != "bob/projet" || r[1].Label != "Bob" {
		t.Errorf("bob = %+v", r[1])
	}
	if !slices.Equal(r[2].Branches, []string{"main", "release/*"}) {
		t.Errorf("carol = %+v", r[2])
	}
	if r[3].Branches != nil {
		t.Errorf("dave = %+v", r[3])
	}

	// Seulement un fuseau ; aucun dépôt.
	f = mustParse(t, doc(`{"timezone": "Europe/Paris"}`, ""))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeyTimezone}) || len(f.Repositories) != 0 {
		t.Errorf("clés = %v, dépôts = %d", keys, len(f.Repositories))
	}
	// Valeurs null : non fournies.
	f = mustParse(t, doc(`{"sync": {"interval": null, "auto": false}, "activity": null}`, `[]`))
	if keys := f.Settings.Keys(); !slices.Equal(keys, []string{workspace.KeySyncAuto}) {
		t.Errorf("clés = %v", keys)
	}
	// Durée « 0 » : synchro manuelle seule.
	f = mustParse(t, doc(`{"sync": {"interval": "0"}}`, ""))
	if *f.Settings.SyncInterval != 0 {
		t.Error("interval 0 attendu")
	}
}

func TestParseRejects(t *testing.T) {
	big := `{"format": "watchoo-config", "version": 1, "workspace": {"name": "` + strings.Repeat("x", configshare.MaxFileSize) + `"}}`
	var many strings.Builder
	many.WriteString("[")
	for i := range configshare.MaxRepositories + 1 {
		if i > 0 {
			many.WriteString(",")
		}
		fmt.Fprintf(&many, `{"repo": "o/r%d"}`, i)
	}
	many.WriteString("]")
	var errorsList strings.Builder
	errorsList.WriteString("[")
	for i := range 25 {
		if i > 0 {
			errorsList.WriteString(",")
		}
		errorsList.WriteString(`{"repo": "invalide"}`)
	}
	errorsList.WriteString("]")

	for _, tt := range []struct {
		name, input string
		target      error
		parts       []string
	}{
		{"vide", "", configshare.ErrInvalid, []string{"JSON"}},
		{"JSON invalide", "{", configshare.ErrInvalid, []string{"JSON"}},
		{"pas un objet", "[]", configshare.ErrInvalid, []string{"objet JSON attendu"}},
		{"texte après", doc("", "") + " {}", configshare.ErrInvalid, []string{"texte après"}},
		{"trop gros", big, configshare.ErrTooLarge, nil},
		{"format absent", `{"version": 1}`, configshare.ErrUnsupportedFormat, []string{"watchoo-config"}},
		{"autre format", `{"format": "autre", "version": 1}`, configshare.ErrUnsupportedFormat, nil},
		{"format non texte", `{"format": 1, "version": 1}`, configshare.ErrUnsupportedFormat, nil},
		{"version absente", `{"format": "watchoo-config"}`, configshare.ErrInvalid, []string{"version absente"}},
		{"version texte", `{"format": "watchoo-config", "version": "1"}`, configshare.ErrInvalid, []string{"entier"}},
		{"version future", `{"format": "watchoo-config", "version": 2}`, configshare.ErrNewerVersion, []string{"plus récente"}},
		{"version nulle", `{"format": "watchoo-config", "version": 0}`, configshare.ErrInvalid, []string{"version 0"}},
		{"clé inconnue", `{"format": "watchoo-config", "version": 1, "token": "x"}`, configshare.ErrInvalid, []string{`"token"`}},
		{"github dans settings", doc(`{"github": {"gh_path": "/tmp/evil"}}`, ""), configshare.ErrInvalid, []string{`"github"`}},
		{"web dans settings", doc(`{"web": {"addr": "0.0.0.0:80"}}`, ""), configshare.ErrInvalid, []string{`"web"`}},
		{"name dans settings", doc(`{"name": "x"}`, ""), configshare.ErrInvalid, []string{`"name"`}},
		{"clé inconnue sync", doc(`{"sync": {"intervall": "1h"}}`, ""), configshare.ErrInvalid, []string{`"intervall"`}},
		{"clé inconnue dépôt", doc("", `[{"repo": "a/b", "enabled": false}]`), configshare.ErrInvalid, []string{`"enabled"`}},
		{"type", doc(`{"sync": {"parallelism": "4"}}`, ""), configshare.ErrInvalid, []string{"parallelism", "nombre entier"}},
		{"trop de dépôts", doc("", many.String()), configshare.ErrInvalid, []string{"1001 dépôts"}},
		{"forge", doc("", `[{"provider": "gitlab", "repo": "a/b"}]`), configshare.ErrInvalid, []string{"gitlab"}},
		{"repo vide", doc("", `[{"label": "x"}]`), configshare.ErrInvalid, []string{"dépôt n° 1", "dépôt vide"}},
		{"repo invalide", doc("", `[{"repo": "a/b/c"}]`), configshare.ErrInvalid, []string{"dépôt n° 1"}},
		{"autre hôte", doc("", `[{"repo": "https://gitlab.com/a/b"}]`), configshare.ErrInvalid, []string{"github.com"}},
		{"doublon", doc("", `[{"repo": "Alice/Projet"}, {"repo": "alice/projet"}]`), configshare.ErrInvalid,
			[]string{"dépôt n° 2", "deux fois", "n° 1"}},
		{"label contrôle", doc("", `[{"repo": "a/b", "label": "x\u001b[2J"}]`), configshare.ErrInvalid, []string{"contrôle"}},
		{"label long", doc("", `[{"repo": "a/b", "label": "`+strings.Repeat("é", 201)+`"}]`), configshare.ErrInvalid, []string{"trop long"}},
		{"motif vide", doc("", `[{"repo": "a/b", "branches": [" "]}]`), configshare.ErrInvalid, []string{"vide"}},
		{"motif virgule", doc("", `[{"repo": "a/b", "branches": ["main,dev"]}]`), configshare.ErrInvalid, []string{"virgule"}},
		{"motif invalide", doc("", `[{"repo": "a/b", "branches": ["[a"]}]`), configshare.ErrInvalid, []string{`"[a"`}},
		{"motif contrôle", doc("", `[{"repo": "a/b", "branches": ["a\nb"]}]`), configshare.ErrInvalid, []string{"branches"}},
		{"durée", doc(`{"sync": {"interval": "30 minutes"}}`, ""), configshare.ErrInvalid, []string{"interval", "illisible"}},
		{"exclude invalide", doc(`{"branches": {"exclude": ["[a"]}}`, ""), configshare.ErrInvalid, []string{"exclude"}},
		{"exclude vide", doc(`{"branches": {"exclude": [""]}}`, ""), configshare.ErrInvalid, []string{"vide"}},
		{"exclude contrôle", doc(`{"branches": {"exclude": ["a\nb: c"]}}`, ""), configshare.ErrInvalid, []string{"contrôle"}},
		{"fuseau injection", doc(`{"timezone": "Europe/Paris\"\ngithub:\n  gh_path: /tmp/evil"}`, ""), configshare.ErrInvalid,
			[]string{"timezone", "contrôle"}},
		{"nom espace contrôle", `{"format": "watchoo-config", "version": 1, "workspace": {"name": "a\u0007"}}`,
			configshare.ErrInvalid, []string{"workspace.name"}},
		{"date", `{"format": "watchoo-config", "version": 1, "exported_at": "hier"}`, configshare.ErrInvalid, []string{"exported_at"}},
		{"plusieurs erreurs", doc("", errorsList.String()), configshare.ErrInvalid, []string{"dépôt n° 1", "dépôt n° 20", "et 5 autres"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := configshare.Parse(strings.NewReader(tt.input))
			if !errors.Is(err, tt.target) {
				t.Fatalf("erreur = %v, attendu %v", err, tt.target)
			}
			for _, p := range tt.parts {
				if !strings.Contains(err.Error(), p) {
					t.Errorf("message sans %q :\n%v", p, err)
				}
			}
		})
	}
}

// failingReader renvoie toujours une erreur.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("lecture impossible") }

func TestParseReadError(t *testing.T) {
	if _, err := configshare.Parse(failingReader{}); err == nil || !strings.Contains(err.Error(), "lecture impossible") {
		t.Errorf("erreur = %v", err)
	}
}

// Fusion : ajout, mise à jour (label, branches), inchangé, dépôt arrêté repris ; l'aperçu ne modifie
// rien (base et config.yaml octet pour octet).
func TestPlanAndApplyMerge(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.track(1, "alice", "projet", "Alice")                 // inchangé
	e.track(2, "bob", "projet", "Bob", "main")             // label et branches mis à jour
	carol := e.track(3, "carol", "projet", "Carol", "dev") // arrêté : repris
	e.stop(carol.ID)
	e.track(4, "dave", "projet", "Dave", "main") // absent du fichier : jamais retiré
	e.forge(5, "eva", "projet")                  // à ajouter
	e.forge(6, "fred", "projet")

	f := mustParse(t, doc(`{"sync": {"interval": "30m"}}`, `[
		{"repo": "alice/projet", "label": "", "branches": null},
		{"repo": "Bob/Projet", "label": "Bob Durand", "branches": []},
		{"repo": "carol/projet"},
		{"repo": "eva/projet", "label": "Eva", "branches": ["main"]},
		{"repo": "fred/projet", "label": "Fred"},
		{"repo": "ghost/projet"}]`))

	configBefore := readFile(t, e.ws.ConfigPath())
	dbBefore := readFile(t, e.ws.DBPath())
	reposBefore := e.repos()

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantActions := []configshare.Action{configshare.ActionUnchanged, configshare.ActionUpdate, configshare.ActionUpdate,
		configshare.ActionAdd, configshare.ActionAdd, configshare.ActionAdd}
	for i, c := range p.Repositories {
		if c.Action != wantActions[i] || c.Index != i+1 || c.Outcome != "" || c.Repository != nil {
			t.Errorf("dépôt %d = %+v, action %s attendue", i+1, c, wantActions[i])
		}
	}
	bob := p.Repositories[1]
	if bob.Repo != "Bob/Projet" || bob.Before == nil || bob.Before.Label != "Bob" || !slices.Equal(bob.Before.Branches, []string{"main"}) ||
		bob.After.Label != "Bob Durand" || bob.After.Branches == nil || len(bob.After.Branches) != 0 || bob.Reenable {
		t.Errorf("bob = %+v (avant %+v)", bob, bob.Before)
	}
	if c := p.Repositories[2]; !c.Reenable || c.Before.Enabled || !c.After.Enabled || c.After.Label != "Carol" {
		t.Errorf("carol = %+v", c)
	}
	if c := p.Repositories[3]; c.Before != nil || c.After.Label != "Eva" || !slices.Equal(c.After.Branches, []string{"main"}) {
		t.Errorf("eva = %+v", c)
	}
	if want := (configshare.Summary{Added: 3, AlreadyTracked: 1, Updated: 2, Reenabled: 1}); p.Summary != want {
		t.Errorf("résumé = %+v, attendu %+v", p.Summary, want)
	}
	if p.Settings.Rewrite || len(p.Settings.Changes) != 0 || len(p.Warnings) != 0 || p.AccessChecked {
		t.Errorf("réglages identiques : %+v, avertissements %v", p.Settings, p.Warnings)
	}

	// Aperçu avec vérification d'accès : ghost inaccessible ; rien n'est écrit non plus.
	p, err = configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{CheckAccess: e.eng})
	if err != nil {
		t.Fatal(err)
	}
	if !p.AccessChecked || p.Repositories[5].Action != configshare.ActionInaccessible || p.Repositories[5].Error == "" {
		t.Errorf("ghost = %+v", p.Repositories[5])
	}
	if want := (configshare.Summary{Added: 2, AlreadyTracked: 1, Updated: 2, Reenabled: 1, Inaccessible: 1}); p.Summary != want {
		t.Errorf("résumé vérifié = %+v, attendu %+v", p.Summary, want)
	}
	if got := readFile(t, e.ws.ConfigPath()); got != configBefore {
		t.Error("l'aperçu a modifié config.yaml")
	}
	if got := readFile(t, e.ws.DBPath()); got != dbBefore {
		t.Error("l'aperçu a modifié la base")
	}
	if !reflect.DeepEqual(e.repos(), reposBefore) {
		t.Error("l'aperçu a modifié les dépôts")
	}

	// Application.
	res, err := configshare.Apply(t.Context(), e.ws, e.st, e.eng, f, configshare.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Incomplete() || res.Settings.Written {
		t.Errorf("bilan = %+v", res)
	}
	if want := (configshare.Summary{Added: 2, AlreadyTracked: 1, Updated: 2, Reenabled: 1, Inaccessible: 1}); res.Summary != want {
		t.Errorf("bilan = %+v, attendu %+v", res.Summary, want)
	}
	outcomes := []wsync.AddOutcome{wsync.OutcomeAlreadyTracked, wsync.OutcomeUpdated, wsync.OutcomeUpdated,
		wsync.OutcomeAdded, wsync.OutcomeAdded, wsync.OutcomeInaccessible}
	for i, c := range res.Preview.Repositories {
		if c.Outcome != outcomes[i] {
			t.Errorf("dépôt %d : résultat %s, attendu %s", i+1, c.Outcome, outcomes[i])
		}
		if (c.Repository == nil) != (outcomes[i] == wsync.OutcomeInaccessible) {
			t.Errorf("dépôt %d : Repository = %v", i+1, c.Repository)
		}
	}
	repos := e.repos()
	if r := repos["bob/projet"]; r.Label != "Bob Durand" || len(r.BranchPatterns) != 0 {
		t.Errorf("bob = %+v", r)
	}
	if r := repos["carol/projet"]; !r.Enabled || r.Label != "Carol" || !slices.Equal(r.BranchPatterns, []string{"dev"}) {
		t.Errorf("carol = %+v", r)
	}
	if r := repos["alice/projet"]; r.Label != "Alice" {
		t.Errorf("alice = %+v", r)
	}
	if r, ok := repos["dave/projet"]; !ok || !r.Enabled {
		t.Error("dave a été retiré ou arrêté")
	}
	if r := repos["eva/projet"]; r.Label != "Eva" || !slices.Equal(r.BranchPatterns, []string{"main"}) {
		t.Errorf("eva = %+v", r)
	}
	if r := repos["fred/projet"]; r.Label != "Fred" || len(r.BranchPatterns) != 0 {
		t.Errorf("fred = %+v", r)
	}
	if _, ok := repos["ghost/projet"]; ok || len(repos) != 6 {
		t.Errorf("dépôts = %v", repos)
	}
	// Réglages identiques : config.yaml non réécrit, pas de .bak.
	if got := readFile(t, e.ws.ConfigPath()); got != configBefore {
		t.Error("config.yaml réécrit alors qu'aucun réglage ne change")
	}
	if _, err := os.Stat(filepath.Join(e.ws.Dir, workspace.BackupFileName)); !os.IsNotExist(err) {
		t.Error(".bak créé sans réécriture")
	}
}

// Réglages : seules les valeurs différentes sont des modifications ; variable WATCHOO_* signalée ;
// commentaires perdus ; config.yaml réécrit avec .bak, clés machine gardées.
func TestPlanAndApplySettings(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	custom := "# commentaire perso\nname: \"Licence\"\ngithub:\n  gh_path: \"/opt/gh\"\nweb:\n  addr: \"127.0.0.1:9000\"\n"
	if err := os.WriteFile(e.ws.ConfigPath(), []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "1h")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"sync": {"auto": true, "interval": "45m", "parallelism": 2},
		"activity": {"dead_after_days": 20}, "branches": {"exclude": ["renovate/*"]}, "timezone": "Europe/Paris"}`, ""))

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	wantChanges := []configshare.SettingChange{
		{Key: "sync.interval", Before: "30m", After: "45m", EnvVar: "WATCHOO_SYNC_INTERVAL"}, // valeur du fichier, pas de la variable
		{Key: "sync.parallelism", Before: "4", After: "2"},
		{Key: "activity.dead_after_days", Before: "15", After: "20"},
		{Key: "branches.exclude", Before: "[]", After: `["renovate/*"]`},
		{Key: "timezone", Before: `""`, After: `"Europe/Paris"`},
	}
	if !reflect.DeepEqual(p.Settings.Changes, wantChanges) {
		t.Errorf("modifications = %+v\nattendu %+v", p.Settings.Changes, wantChanges)
	}
	if !p.Settings.Rewrite || len(p.Settings.Provided) != 6 {
		t.Errorf("réglages = %+v", p.Settings)
	}
	var codes []string
	for _, w := range p.Warnings {
		codes = append(codes, w.Code)
	}
	if !slices.Equal(codes, []string{configshare.WarnEnvOverride, configshare.WarnCommentsLost}) ||
		p.Warnings[0].Key != "sync.interval" || !strings.Contains(p.Warnings[0].Message, "WATCHOO_SYNC_INTERVAL") ||
		!strings.Contains(p.Warnings[1].Message, workspace.BackupFileName) {
		t.Errorf("avertissements = %+v", p.Warnings)
	}
	if got := readFile(t, e.ws.ConfigPath()); got != custom {
		t.Error("l'aperçu a modifié config.yaml")
	}

	res, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Settings.Written || res.Settings.Path != e.ws.ConfigPath() ||
		res.Settings.BackupPath != filepath.Join(e.ws.Dir, workspace.BackupFileName) || res.Incomplete() {
		t.Errorf("bilan = %+v", res.Settings)
	}
	if got := readFile(t, res.Settings.BackupPath); got != custom {
		t.Errorf(".bak = %q", got)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "")
	cfg := e.reload()
	if cfg.Name != "Licence" || cfg.GitHub.GHPath != "/opt/gh" || cfg.Web.Addr != "127.0.0.1:9000" {
		t.Errorf("clés machine perdues : %+v", cfg)
	}
	if cfg.Sync.Interval != 45*time.Minute || cfg.Sync.Parallelism != 2 || cfg.Activity.DeadAfterDays != 20 ||
		cfg.Timezone != "Europe/Paris" || !slices.Equal(cfg.Branches.Exclude, []string{"renovate/*"}) {
		t.Errorf("réglages importés = %+v", cfg)
	}
	if info, err := os.Stat(e.ws.ConfigPath()); err == nil && info.Mode().Perm() != 0o600 && os.PathSeparator == '/' {
		t.Errorf("droits = %v", info.Mode().Perm())
	}
}

// Une variable WATCHOO_* égale à la valeur importée, alors que config.yaml en a une autre : le
// réglage est comparé à la valeur du fichier, donc écrit, et la variable est signalée (constat : la
// comparaison à la valeur en vigueur ne voyait aucun changement et ne signalait rien).
func TestPlanEnvOverrideEqualToImported(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	if err := os.WriteFile(e.ws.ConfigPath(), []byte("sync:\n  interval: 1h\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "2h")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"sync": {"interval": "2h"}}`, ""))
	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	want := []configshare.SettingChange{{Key: "sync.interval", Before: "1h", After: "2h", EnvVar: "WATCHOO_SYNC_INTERVAL"}}
	if !reflect.DeepEqual(p.Settings.Changes, want) || !p.Settings.Rewrite {
		t.Errorf("réglages = %+v", p.Settings)
	}
	if len(p.Warnings) == 0 || p.Warnings[0].Code != configshare.WarnEnvOverride || p.Warnings[0].Key != "sync.interval" {
		t.Errorf("avertissements = %+v", p.Warnings)
	}
	res, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil || !res.Settings.Written {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "")
	if cfg := e.reload(); cfg.Sync.Interval != 2*time.Hour {
		t.Errorf("sync.interval sans la variable = %v, attendu 2h", cfg.Sync.Interval)
	}
}

// Un réglage masqué déjà écrit dans config.yaml : réimporter le même fichier ne réécrit rien (le
// .bak garde l'ancien fichier d'origine), mais la variable est toujours signalée.
func TestApplyEnvOverrideIdempotent(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	original := "# commentaire perso\nsync:\n  interval: 1h\n"
	if err := os.WriteFile(e.ws.ConfigPath(), []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WATCHOO_SYNC_INTERVAL", "1h")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, doc(`{"sync": {"interval": "45m"}}`, ""))
	res, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil || !res.Settings.Written {
		t.Fatalf("premier import = %+v, %v", res, err)
	}
	written := readFile(t, e.ws.ConfigPath())
	bak := filepath.Join(e.ws.Dir, workspace.BackupFileName)

	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if p.Settings.Rewrite || len(p.Settings.Changes) != 0 {
		t.Errorf("second aperçu : réglages = %+v", p.Settings)
	}
	if len(p.Warnings) != 1 || p.Warnings[0].Code != configshare.WarnEnvOverride ||
		!strings.Contains(p.Warnings[0].Message, "config.yaml contient déjà la valeur importée") {
		t.Errorf("second aperçu : avertissements = %+v", p.Warnings)
	}
	res, err = configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{})
	if err != nil || res.Settings.Written {
		t.Fatalf("second import = %+v, %v", res, err)
	}
	if readFile(t, e.ws.ConfigPath()) != written {
		t.Error("config.yaml réécrit par le second import")
	}
	if readFile(t, bak) != original {
		t.Error("config.yaml.bak ne garde plus le fichier d'origine")
	}
}

// Une variable WATCHOO_* qui rend invalides les réglages importés (seuils) : l'aperçu refuse comme
// l'écriture, avec ErrInvalid ; rien n'est écrit.
func TestPlanRejectsSettingsInvalidWithEnv(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	t.Setenv("WATCHOO_ACTIVITY_DEAD_AFTER_DAYS", "10")
	if err := workspace.Load(e.ws); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, e.ws.ConfigPath())
	f := mustParse(t, doc(`{"activity": {"flemme_after_days": 12, "dead_after_days": 20}}`, ""))
	_, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{})
	if !errors.Is(err, configshare.ErrInvalid) || !strings.Contains(err.Error(), "WATCHOO_") {
		t.Errorf("Plan = %v, ErrInvalid attendue", err)
	}
	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); !errors.Is(err, configshare.ErrInvalid) {
		t.Errorf("Apply = %v, ErrInvalid attendue", err)
	}
	if readFile(t, e.ws.ConfigPath()) != before {
		t.Error("config.yaml modifié")
	}
	if _, err := os.Stat(filepath.Join(e.ws.Dir, workspace.BackupFileName)); err == nil {
		t.Error("config.yaml.bak créé")
	}
}

// Les réglages sont validés après fusion avec ceux de l'espace : refus, rien n'est écrit (ni réglages
// ni dépôts).
func TestPlanRejectsInvalidMergedSettings(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.forge(1, "alice", "projet")
	before := readFile(t, e.ws.ConfigPath())
	for name, settings := range map[string]string{
		"seuils":       `{"activity": {"flemme_after_days": 20}}`,
		"parallélisme": `{"sync": {"parallelism": 9}}`,
		"intervalle":   `{"sync": {"interval": "5m"}}`,
		"fuseau":       `{"timezone": "Mars/Olympus"}`,
	} {
		f := mustParse(t, doc(settings, `[{"repo": "alice/projet"}]`))
		if _, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{}); !errors.Is(err, configshare.ErrInvalid) {
			t.Errorf("%s : Plan = %v, ErrInvalid attendue", name, err)
		}
		if _, err := configshare.Apply(t.Context(), e.ws, e.st, e.eng, f, configshare.ApplyOptions{}); !errors.Is(err, configshare.ErrInvalid) {
			t.Errorf("%s : Apply = %v, ErrInvalid attendue", name, err)
		}
	}
	if got := readFile(t, e.ws.ConfigPath()); got != before {
		t.Error("config.yaml modifié")
	}
	if n := len(e.repos()); n != 0 {
		t.Errorf("%d dépôts ajoutés", n)
	}
	if n := e.fake.Calls("Repository"); n != 0 {
		t.Errorf("%d appels à la forge", n)
	}
}

// Une erreur globale de l'ajout (gh non connecté) est renvoyée avec le résultat ; les réglages sont
// déjà écrits, les dépôts non traités sont en erreur.
func TestApplyGlobalError(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.forge(1, "alice", "projet")
	e.auth.SetTokenErr(errors.New("gh non connecté"))
	f := mustParse(t, doc(`{"timezone": "UTC"}`, `[{"repo": "alice/projet"}]`))
	res, err := configshare.Apply(t.Context(), e.ws, e.st, e.eng, f, configshare.ApplyOptions{})
	if err == nil || res == nil {
		t.Fatalf("Apply = %+v, %v", res, err)
	}
	if !res.Settings.Written || res.Summary.Errors != 1 || !res.Incomplete() ||
		res.Preview.Repositories[0].Outcome != wsync.OutcomeError {
		t.Errorf("bilan = %+v", res)
	}
	if strings.Contains(err.Error(), authtest.FakeToken) {
		t.Error("token dans l'erreur")
	}
}

func TestApplyWithoutAdder(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	f := mustParse(t, doc("", `[{"repo": "alice/projet"}]`))
	if _, err := configshare.Apply(t.Context(), e.ws, e.st, nil, f, configshare.ApplyOptions{}); err == nil {
		t.Error("erreur attendue sans Adder")
	}
}

// Le quota épuisé pendant la vérification d'accès : dépôts en erreur, sans bloquer l'aperçu.
func TestPlanAccessQuota(t *testing.T) {
	isolate(t)
	e := newEnv(t, "licence")
	e.forge(1, "alice", "projet")
	e.fake.SetRepoError("alice", "projet", &provider.RateLimitError{Reset: testNow})
	f := mustParse(t, doc("", `[{"repo": "alice/projet"}]`))
	p, err := configshare.Plan(t.Context(), e.ws, e.st, f, configshare.PlanOptions{CheckAccess: e.eng})
	if err != nil {
		t.Fatal(err)
	}
	if p.Repositories[0].Action != configshare.ActionError || p.Summary.Errors != 1 {
		t.Errorf("aperçu = %+v", p.Repositories[0])
	}
}

func TestPlanRequiresArguments(t *testing.T) {
	if _, err := configshare.Plan(context.Background(), nil, nil, nil, configshare.PlanOptions{}); err == nil {
		t.Error("erreur attendue")
	}
	if _, err := configshare.Export(context.Background(), nil, nil, configshare.ExportOptions{}); err == nil {
		t.Error("erreur attendue")
	}
}

func TestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"licence-projet":           "watchoo-config-licence-projet.json",
		"Licence Projet 2026":      "watchoo-config-Licence-Projet-2026.json",
		"Élodie & co / \"x\"\r\n.": "watchoo-config-lodie-co-x.json",
		"../../etc/passwd":         "watchoo-config-etc-passwd.json",
		"":                         "watchoo-config.json",
		"ééé":                      "watchoo-config.json",
		strings.Repeat("a", 100):   "watchoo-config-" + strings.Repeat("a", 60) + ".json",
	} {
		if got := configshare.FileName(in); got != want {
			t.Errorf("FileName(%q) = %q, attendu %q", in, got, want)
		}
	}
}
