package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// configPath renvoie le chemin de .watchoo/config.yaml de l'espace du harness.
func configPath(h *harness) string {
	return filepath.Join(h.cwd, workspace.DirName, workspace.ConfigFileName)
}

// listRepos renvoie les dépôts de la base de l'espace du harness.
func listRepos(t *testing.T, h *harness) []model.Repository {
	t.Helper()
	repos, err := openTestStore(t, h).ListRepositories(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return repos
}

// sharedDoc est un document de partage factice : alice (nouveau label), bob (branches), carol
// (inaccessible), dave (sans label ni branches).
const sharedDoc = `{
  "format": "watchoo-config",
  "version": 1,
  "exported_at": "2026-10-20T08:00:00Z",
  "watchoo_version": "v0.1.0",
  "workspace": {"name": "licence-projet"},
  "repositories": [
    {"provider": "github", "repo": "alice/projet", "label": "Alice M.", "branches": []},
    {"provider": "github", "repo": "bob/projet", "label": "Bob Durand", "branches": ["main", "dev"]},
    {"provider": "github", "repo": "carol/projet", "label": "Carol Leroy", "branches": []},
    {"repo": "dave/projet", "label": ""}
  ]
}
`

func TestConfigExport(t *testing.T) {
	h := newSyncedWorkspace(t)
	h.run("repo", "add", "bob/projet", "--branch", "main,dev").mustSucceed(t)
	h.run("repo", "remove", "dave/projet", "--keep-history", "--yes").mustSucceed(t)

	r := h.run("config", "export").mustSucceed(t)
	assertSingleJSONObject(t, "config export", r.stdout)
	var doc configshare.Document
	decodeStrict(t, r.stdout, &doc)
	if doc.Format != configshare.Format || doc.Version != 1 || !doc.ExportedAt.Equal(testNow) ||
		doc.Workspace.Name != "licence-projet" || doc.WatchooVersion != appVersion() {
		t.Errorf("en-tête du document = %+v", doc)
	}
	// dave est arrêté : pas exporté ; tri par owner/name.
	want := []configshare.Repository{
		{Provider: "github", Repo: "alice/projet", Label: "Alice Martin", Branches: []string{}},
		{Provider: "github", Repo: "bob/projet", Label: "", Branches: []string{"main", "dev"}},
	}
	if !reflect.DeepEqual(doc.Repositories, want) {
		t.Errorf("dépôts exportés = %+v, attendu %+v", doc.Repositories, want)
	}
	if doc.Settings == nil || doc.Settings.Timezone == nil || *doc.Settings.Timezone != "UTC" ||
		doc.Settings.Sync == nil || *doc.Settings.Sync.Interval != "30m" {
		t.Errorf("réglages exportés = %+v", doc.Settings)
	}
	assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["config export"]...)
	assertJSONKeys(t, r.stdout, "settings", "sync", "activity", "branches", "timezone")
	// Ni e-mail, ni token, ni réglage machine, ni chemin local, ni carte d'en-tête.
	assertNotContains(t, "config export", r.stdout, "@", authtest.FakeToken, "gh_path", `"github":`, `"web"`,
		h.cwd, "\x1b")

	// --json : même document.
	if rj := h.run("config", "export", "--json").mustSucceed(t); rj.stdout != r.stdout {
		t.Errorf("export --json différent :\n%s\nattendu :\n%s", rj.stdout, r.stdout)
	}

	// Sur un terminal : pas de carte non plus.
	h.Terminal = func() TermInfo { return wideTerminal }
	if rt := h.run("config", "export").mustSucceed(t); rt.stdout != r.stdout {
		t.Errorf("export sur un terminal :\n%s", rt.stdout)
	}
	h.Terminal = nil

	// -o : fichier en 0600, ligne de confirmation.
	r = h.run("config", "export", "-o", "licence.json").mustSucceed(t)
	if r.stdout != "Configuration exported to licence.json: 2 repositories, 7 settings.\n" {
		t.Errorf("export -o = %q", r.stdout)
	}
	out := filepath.Join(h.cwd, "licence.json")
	assertMode(t, out, 0o600)
	var fromFile configshare.Document
	decodeStrict(t, readFile(t, out), &fromFile)
	if !reflect.DeepEqual(fromFile.Repositories, want) {
		t.Errorf("fichier exporté = %+v", fromFile.Repositories)
	}

	// Fichier existant : refus sans --force, rien n'est touché.
	// (Le préfixe « erreur : » vient de root.go : hors du périmètre de ce groupe.)
	writeFile(t, out, "ancien contenu\n")
	h.run("config", "export", "-o", "licence.json").mustFail(t, "licence.json already exists", "--force")
	if got := readFile(t, out); got != "ancien contenu\n" {
		t.Errorf("fichier écrasé sans --force : %q", got)
	}
	if err := os.Chmod(out, 0o644); err != nil {
		t.Fatal(err)
	}
	// --force --json : fichier remplacé (0600), rien sur la sortie standard.
	r = h.run("config", "export", "-o", "licence.json", "--force", "--json").mustSucceed(t)
	if r.stdout != "" {
		t.Errorf("export -o --json a écrit sur la sortie standard : %q", r.stdout)
	}
	assertMode(t, out, 0o600)
	decodeStrict(t, readFile(t, out), &fromFile)

	// Un répertoire n'est jamais remplacé.
	newDir(t, h.cwd, "rep")
	h.run("config", "export", "-o", "rep", "--force").mustFail(t, "is a directory")

	// Arguments.
	h.run("config", "export", "en-trop").mustFail(t, "takes no argument")
}

// Export d'un espace, import dans un autre espace (celui d'une collègue) : aperçu, application,
// relance sans changement. WATCHOO_TIMEZONE=UTC (fixtures) est exporté comme valeur en vigueur ;
// dans l'espace importateur, la variable masque timezone : la valeur est écrite dans config.yaml
// (comparée à celle du fichier, ""), signalée, puis la relance ne réécrit plus rien.
func TestConfigImportRoundTrip(t *testing.T) {
	a := newSyncedWorkspace(t)
	a.run("repo", "add", "bob/projet", "--branch", "main,dev").mustSucceed(t)
	exported := a.run("config", "export").mustSucceed(t).stdout

	b := newWorkspace(t)
	seedProvider(b.Provider)
	writeFile(t, filepath.Join(b.cwd, "licence.json"), exported)
	configBefore := readFile(t, configPath(b))

	// --dry-run : aperçu avec vérification d'accès ; rien n'est écrit.
	// (Les avertissements « ! … » viennent de configshare : hors du périmètre de ce groupe.)
	r := b.run("config", "import", "licence.json", "--dry-run").mustSucceed(t)
	assertContains(t, "aperçu", r.stdout,
		"IMPORT  licence.json  (workspace \"licence-projet\", exported on 10/27/2026 with Watchoo "+appVersion()+")\n",
		"Settings\n  timezone                   \"\" → \"UTC\"   (overridden by WATCHOO_TIMEZONE)\n",
		"Repositories\n  to add             3\n",
		"  inaccessible       0\n",
		"! la variable WATCHOO_TIMEZONE est définie : elle masque timezone (la valeur importée sera écrite",
		"commentaires personnels",
		"Preview only (--dry-run): nothing was modified.\n")
	if n := len(listRepos(t, b)); n != 0 {
		t.Errorf("--dry-run a enregistré %d dépôts", n)
	}
	if got := readFile(t, configPath(b)); got != configBefore {
		t.Error("--dry-run a modifié config.yaml")
	}
	if n := b.Provider.Calls(providertest.MethodRepository); n != 3 {
		t.Errorf("accès vérifié %d fois, attendu 3", n)
	}

	// Application avec --yes : aperçu puis bilan.
	r = b.run("config", "import", "licence.json", "--yes").mustSucceed(t)
	assertContains(t, "import", r.stdout, "  to add             3\n",
		"(repository access will be checked against GitHub when applying)\n",
		"SUMMARY\n  settings written in ",
		"  added              3\n", "Run \"watchoo sync\"")
	written := readFile(t, configPath(b))
	assertContains(t, "config.yaml réécrit", written, `timezone: "UTC"`)
	bak := filepath.Join(b.cwd, workspace.DirName, workspace.BackupFileName)
	if readFile(t, bak) != configBefore {
		t.Error("config.yaml.bak n'est pas l'ancien fichier")
	}
	alice := storedRepo(t, b, "alice", "projet")
	bob := storedRepo(t, b, "bob", "projet")
	if alice.Label != "Alice Martin" || len(alice.BranchPatterns) != 0 ||
		!reflect.DeepEqual(bob.BranchPatterns, []string{"main", "dev"}) || !bob.Enabled {
		t.Errorf("dépôts importés : alice %+v, bob %+v", alice, bob)
	}

	// Relance : rien à changer, pas de question (même hors terminal, sans --yes).
	r = b.run("config", "import", "licence.json").mustSucceed(t)
	assertContains(t, "relance", r.stdout, "  unchanged          3\n", "Nothing to change: the workspace is already up to date.\n",
		"  already tracked    3\n", "unchanged (7 settings, identical to those of the workspace)\n",
		"! la variable WATCHOO_TIMEZONE est définie : elle masque timezone (config.yaml contient déjà la valeur importée")
	assertNotContains(t, "relance", r.stderr, "Apply")
	if readFile(t, configPath(b)) != written || readFile(t, bak) != configBefore {
		t.Error("relance : config.yaml ou config.yaml.bak réécrit alors qu'aucun réglage ne change")
	}

	// Le second espace réexporte les mêmes dépôts.
	var docA, docB configshare.Document
	decodeStrict(t, exported, &docA)
	decodeStrict(t, b.run("config", "export").mustSucceed(t).stdout, &docB)
	if !reflect.DeepEqual(docA.Repositories, docB.Repositories) || !reflect.DeepEqual(docA.Settings, docB.Settings) {
		t.Errorf("aller-retour : %+v / %+v", docA, docB)
	}
}

// Fusion : ajout, mise à jour, reprise d'un dépôt arrêté, inchangé, inaccessible ; rien n'est
// retiré ; code non nul.
func TestConfigImportMerge(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--label", "Alice Martin").mustSucceed(t)
	h.run("repo", "add", "bob/projet", "--label", "Bob Durand").mustSucceed(t)
	h.run("repo", "remove", "bob/projet", "--keep-history", "--yes").mustSucceed(t)
	h.run("repo", "add", "dave/projet").mustSucceed(t)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), sharedDoc)

	// Hors terminal, sans --yes : refus avant tout affichage, rien n'est changé.
	r := h.run("config", "import", "partage.json").mustFail(t, "--yes")
	if got := storedRepo(t, h, "alice", "projet").Label; got != "Alice Martin" {
		t.Errorf("label changé sans confirmation : %q", got)
	}

	// Aperçu sans vérification d'accès (non --dry-run, au terminal, réponse « non »).
	h.StdinTerminal, h.Stdin = true, "n\n"
	r = h.run("config", "import", "partage.json")
	h.StdinTerminal, h.Stdin = false, ""
	if r.code != 1 {
		t.Fatalf("import refusé au terminal : code %d\n%s", r.code, r.stderr)
	}
	assertContains(t, "aperçu", r.stdout,
		"  to add             1\n",
		"  to update          1   alice/projet (label: \"Alice Martin\" → \"Alice M.\")\n",
		"  to resume          1   bob/projet (branches: all → main, dev)\n",
		"  unchanged          1\n",
		"Settings\n  none in the file\n")
	assertContains(t, "refus", r.stderr, "nothing was modified")
	if got := storedRepo(t, h, "alice", "projet").Label; got != "Alice Martin" {
		t.Errorf("label changé sans confirmation : %q", got)
	}

	// --dry-run --json : vérification d'accès, carol inaccessible, code non nul, rien d'écrit.
	r = h.run("config", "import", "partage.json", "--dry-run", "--json").mustCode(t, 1)
	var rep configImportReport
	decodeStrict(t, r.stdout, &rep)
	wantSum := configSummaryJSON{Added: 0, AlreadyTracked: 1, Updated: 2, Reenabled: 1, Inaccessible: 1}
	if !rep.DryRun || rep.Applied || rep.Summary != wantSum || len(rep.Repositories) != 4 ||
		rep.Repositories[2].Action != "inaccessible" || rep.Repositories[2].Error == "" ||
		rep.Repositories[1].Action != "update" || !rep.Repositories[1].Reenable ||
		rep.Repositories[1].Before == nil || rep.Repositories[1].Before.Enabled ||
		rep.Repositories[0].Outcome != "" {
		t.Errorf("aperçu --json = %+v", rep)
	}
	assertContains(t, "--dry-run --json", r.stderr, "import incomplete: 1 inaccessible repository (carol/projet)")
	if storedRepo(t, h, "bob", "projet").Enabled {
		t.Error("--dry-run a repris le suivi de bob")
	}

	// Application en JSON : bilan sur la sortie standard, code non nul (carol).
	r = h.run("config", "import", "partage.json", "--yes", "--json").mustCode(t, 1)
	decodeStrict(t, r.stdout, &rep)
	if rep.DryRun || !rep.Applied || rep.Summary != wantSum || rep.Repositories[0].Outcome != "updated" ||
		rep.Repositories[2].Outcome != "inaccessible" || rep.Settings.Written {
		t.Errorf("bilan --json = %+v", rep)
	}
	alice, bob, dave := storedRepo(t, h, "alice", "projet"), storedRepo(t, h, "bob", "projet"),
		storedRepo(t, h, "dave", "projet")
	if alice.Label != "Alice M." || !bob.Enabled || !reflect.DeepEqual(bob.BranchPatterns, []string{"main", "dev"}) ||
		dave.Label != "" || !dave.Enabled {
		t.Errorf("après import : alice %+v, bob %+v, dave %+v", alice, bob, dave)
	}
	if n := len(listRepos(t, h)); n != 3 {
		t.Errorf("%d dépôts, attendu 3 (rien n'est retiré, carol non ajouté)", n)
	}

	// En texte : bilan avec le détail de l'échec.
	r = h.run("config", "import", "partage.json", "--yes").mustCode(t, 1)
	assertContains(t, "bilan texte", r.stdout, "SUMMARY\n", "  inaccessible       1   carol/projet\n",
		"  repository #3  carol/projet: ")
	assertContains(t, "erreur", r.stderr, "error: import incomplete: 1 inaccessible repository (carol/projet)")
}

// Réglages : réécriture de config.yaml (clés machine gardées, .bak, 0600), avertissements.
func TestConfigImportSettings(t *testing.T) {
	h := newWorkspace(t)
	cfg := configPath(h)
	// Une clé propre à la machine, à garder.
	writeFile(t, cfg, readFile(t, cfg)+"github:\n  gh_path: /opt/gh/bin/gh\n")
	if err := os.Chmod(cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	original := readFile(t, cfg)
	doc := `{"format":"watchoo-config","version":1,"settings":{"sync":{"interval":"1h"},
"activity":{"dead_after_days":21},"timezone":"Europe/Paris"}}`
	writeFile(t, filepath.Join(h.cwd, "reglages.json"), doc)

	r := h.run("config", "import", "reglages.json", "--dry-run").mustSucceed(t)
	assertContains(t, "aperçu des réglages", r.stdout,
		"IMPORT  reglages.json\n",
		"  sync.interval              30m → 1h\n",
		"  activity.dead_after_days   15 → 21\n",
		`  timezone                   "" → "Europe/Paris"   (overridden by WATCHOO_TIMEZONE)`,
		"Repositories\n  none in the file\n",
		"! la variable WATCHOO_TIMEZONE est définie",
		"! config.yaml sera réécrit")
	if readFile(t, cfg) != original {
		t.Fatal("--dry-run a modifié config.yaml")
	}

	// Confirmation sur un terminal : « n » annule.
	h.StdinTerminal, h.Stdin = true, "n\n"
	h.run("config", "import", "reglages.json").mustCode(t, 1)
	if readFile(t, cfg) != original {
		t.Fatal("config.yaml modifié malgré le refus")
	}
	// « o » applique. (Le suffixe « [o/N] » vient de confirm.go : hors du périmètre de ce groupe.)
	h.Stdin = "o\n"
	r = h.run("config", "import", "reglages.json").mustSucceed(t)
	assertContains(t, "question", r.stderr, "Apply these changes? [y/N]")
	assertContains(t, "bilan", r.stdout, "SUMMARY\n  settings written in ", "config.yaml.bak",
		"next start")
	assertNotContains(t, "bilan sans dépôt", r.stdout, "added")
	written := readFile(t, cfg)
	assertContains(t, "config.yaml réécrit", written, "interval: 1h", "dead_after_days: 21",
		`timezone: "Europe/Paris"`, `gh_path: "/opt/gh/bin/gh"`)
	assertMode(t, cfg, 0o600)
	bak := filepath.Join(h.cwd, workspace.DirName, workspace.BackupFileName)
	if readFile(t, bak) != original {
		t.Error("config.yaml.bak n'est pas l'ancien fichier")
	}
	assertMode(t, bak, 0o600)

	// Les commandes suivantes voient les réglages (hors fuseau, masqué par la variable).
	var exp configshare.Document
	decodeStrict(t, h.run("config", "export").mustSucceed(t).stdout, &exp)
	if *exp.Settings.Sync.Interval != "1h" || *exp.Settings.Activity.DeadAfterDays != 21 || *exp.Settings.Timezone != "UTC" {
		t.Errorf("réglages après import : %+v", exp.Settings)
	}

	// Réglages refusés une fois fusionnés : rien n'est écrit.
	writeFile(t, filepath.Join(h.cwd, "faux.json"),
		`{"format":"watchoo-config","version":1,"settings":{"activity":{"flemme_after_days":30}}}`)
	h.run("config", "import", "faux.json", "--yes").mustFail(t, "faux.json: ", "nothing was imported")
	if readFile(t, cfg) != written {
		t.Error("config.yaml modifié par un import refusé")
	}
}

// Confirmation : --yes obligatoire hors terminal, avec --json et avec « - ».
func TestConfigImportConfirmation(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), sharedDoc)

	h.run("config", "import", "partage.json", "--json").mustFail(t, "--yes")
	h.Stdin = sharedDoc
	h.run("config", "import", "-").mustFail(t, "--yes")
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("accès vérifié %d fois avant la confirmation", n)
	}
	if n := len(listRepos(t, h)); n != 0 {
		t.Errorf("%d dépôts enregistrés sans confirmation", n)
	}

	// Terminal, réponse vide (= non).
	h.StdinTerminal, h.Stdin = true, "\n"
	r := h.run("config", "import", "partage.json").mustCode(t, 1)
	assertContains(t, "annulé", r.stderr, "import cancelled: nothing was modified")
	if n := len(listRepos(t, h)); n != 0 {
		t.Errorf("%d dépôts enregistrés après « non »", n)
	}

	// Entrée standard avec --yes.
	h.StdinTerminal, h.Stdin = false, sharedDoc
	r = h.run("config", "import", "-", "--yes").mustCode(t, 1) // carol inaccessible
	assertContains(t, "import -", r.stdout, "IMPORT  -  (workspace \"licence-projet\", exported on 10/20/2026",
		"  added              3\n")
}

// Fichiers refusés : rien n'est modifié, aucun accès vérifié.
func TestConfigImportInvalid(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	cfg := readFile(t, configPath(h))
	big := `{"format":"watchoo-config","version":1,"workspace":{"name":"` +
		strings.Repeat("x", configshare.MaxFileSize) + `"}}`
	for _, tt := range []struct {
		name, content string
		want          []string
	}{
		{"pas du JSON", "repo,label\nalice/projet,Alice\n", []string{"nothing was imported"}},
		{"format absent", `{"version":1}`, []string{"pas une configuration Watchoo"}},
		{"version plus récente", `{"format":"watchoo-config","version":2}`, []string{"version plus récente"}},
		{"clé machine", `{"format":"watchoo-config","version":1,"settings":{"github":{"gh_path":"/tmp/x"}}}`,
			[]string{"github"}},
		{"doublon", `{"format":"watchoo-config","version":1,"repositories":[{"repo":"alice/projet"},{"repo":"Alice/Projet"}]}`,
			[]string{"deux fois"}},
		{"trop gros", big, []string{"trop gros"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			writeFile(t, filepath.Join(h.cwd, "f.json"), tt.content)
			h.run("config", "import", "f.json", "--yes").mustFail(t, append([]string{"f.json: "}, tt.want...)...)
			h.run("config", "import", "f.json", "--dry-run", "--json").mustFail(t, tt.want...)
		})
	}
	h.run("config", "import", "absent.json", "--yes").mustFail(t, "configuration file absent.json not found")
	h.run("config", "import").mustFail(t)
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 || len(h.tokens) != 0 {
		t.Errorf("accès vérifié malgré des fichiers invalides (%d appels)", n)
	}
	if n := len(listRepos(t, h)); n != 0 || readFile(t, configPath(h)) != cfg {
		t.Errorf("espace modifié par des fichiers invalides (%d dépôts)", n)
	}
}

// Les textes venus du fichier sont nettoyés avant l'affichage.
func TestConfigImportCleansText(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	doc, err := json.Marshal(map[string]any{
		"format": "watchoo-config", "version": 1,
		"repositories": []map[string]any{{"repo": "alice/projet", "label": "Alice ‮M"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(h.cwd, "f.json"), string(doc))
	r := h.run("config", "import", "f.json", "--dry-run")
	assertNotContains(t, "aperçu", r.stdout+r.stderr, "‮", "\x1b")
}

func TestConfigHelp(t *testing.T) {
	h := newHarness(t)
	r := h.run("config").mustSucceed(t)
	assertContains(t, "aide de config", r.stdout, "export", "import", "Partage la configuration")
	r = h.run("config", "import", "--help").mustSucceed(t)
	assertContains(t, "aide de config import", r.stdout, "--dry-run", "--yes", "rien n'est")
	h.run("config", "inconnue").mustFail(t, `unknown command "inconnue"`)
}
