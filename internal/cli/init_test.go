package cli

import (
	"bufio"
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

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func TestInitCreatesWorkspace(t *testing.T) {
	h := newHarness(t)
	root := h.cwd
	r := h.run("init").mustSucceed(t)

	// Résumé en français : nom, chemins et compte gh.
	for _, want := range []string{
		"Espace de supervision « licence-projet » créé.",
		"répertoire  " + root + "\n",
		"réglages    " + filepath.Join(root, ".watchoo", "config.yaml") + "\n",
		"base        " + filepath.Join(root, ".watchoo", "watchoo.db") + "\n",
		"compte gh   watchoo-test sur github.com (droits : gist, read:org, repo)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("résumé sans %q :\n%s", want, r.stdout)
		}
	}
	if r.stderr != "" {
		t.Errorf("sortie d'erreur inattendue :\n%s", r.stderr)
	}

	// gh : vérifié une fois avec Status, jamais Token ; réglages par défaut transmis.
	if got := h.fake().StatusCalls(); got != 1 {
		t.Errorf("Status appelé %d fois, attendu 1", got)
	}
	if got := h.fake().TokenCalls(); got != 0 {
		t.Errorf("Token appelé %d fois : init ne doit jamais demander le token", got)
	}
	if len(h.configs) != 1 || !reflect.DeepEqual(h.configs[0], workspace.DefaultConfig("licence-projet")) {
		t.Errorf("Config passée à NewAuthClient = %+v, attendu les valeurs par défaut", h.configs)
	}

	// Contenu et droits de .watchoo/.
	dir := filepath.Join(root, ".watchoo")
	assertMode(t, dir, 0o700)
	for _, name := range []string{"config.yaml", ".gitignore", "watchoo.db"} {
		assertMode(t, filepath.Join(dir, name), 0o600)
	}
	for _, name := range []string{"watchoo.db-wal", "watchoo.db-shm"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			assertMode(t, filepath.Join(dir, name), 0o600)
		}
	}
	if got := readFile(t, filepath.Join(dir, ".gitignore")); got != "*\n" {
		t.Errorf(".gitignore = %q, attendu %q", got, "*\n")
	}
	if got := readFile(t, filepath.Join(dir, "config.yaml")); got != workspace.ConfigTemplate("licence-projet") {
		t.Errorf("config.yaml différent de ConfigTemplate(\"licence-projet\") :\n%s", got)
	}

	// Base migrée : les 10 tables du modèle, plus la table de version de goose.
	want := slices.Sorted(slices.Values(append([]string{"goose_db_version"}, migratedTables...)))
	if got := tables(t, filepath.Join(dir, "watchoo.db")); !reflect.DeepEqual(got, want) {
		t.Errorf("tables = %v, attendu %v", got, want)
	}
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var version int64
	if err := st.DB().QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 6 {
		t.Errorf("version du schéma = %d, attendu 6 (00006_commits_stats_known)", version)
	}
	if repos, err := st.ListRepositories(ctx); err != nil || len(repos) != 0 {
		t.Errorf("ListRepositories = %v, %v ; attendu une liste vide", repos, err)
	}

	// Les autres commandes retrouvent l'espace, depuis un sous-répertoire aussi.
	ws, err := workspace.ResolveFrom("", newDir(t, root, "src", "module"))
	if err != nil {
		t.Fatalf("ResolveFrom après init : %v", err)
	}
	if ws.Root != root || ws.Config.Name != "licence-projet" {
		t.Errorf("espace retrouvé = %s (nom %q), attendu %s (nom licence-projet)", ws.Root, ws.Config.Name, root)
	}
}

// decodeInitReport décode la sortie JSON de init, en refusant les champs inconnus et tout ce qui
// suivrait l'objet.
func decodeInitReport(t *testing.T, stdout string) initReport {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	var report initReport
	if err := dec.Decode(&report); err != nil {
		t.Fatalf("sortie JSON invalide : %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("texte après l'objet JSON :\n%s", stdout)
	}
	return report
}

func TestInitJSON(t *testing.T) {
	h := newHarness(t)
	root := h.cwd
	r := h.run("init", "--json").mustSucceed(t)
	if r.stderr != "" {
		t.Errorf("sortie d'erreur inattendue :\n%s", r.stderr)
	}

	got := decodeInitReport(t, r.stdout)
	want := initReport{
		Name:            "licence-projet",
		Root:            root,
		Dir:             filepath.Join(root, ".watchoo"),
		ConfigFile:      filepath.Join(root, ".watchoo", "config.yaml"),
		Database:        filepath.Join(root, ".watchoo", "watchoo.db"),
		ParentWorkspace: "",
		GitHub: githubReport{
			Host:        "github.com",
			Login:       authtest.FakeLogin,
			TokenSource: "keyring",
			Scopes:      []string{"gist", "read:org", "repo"},
		},
		Source:   "api",
		Warnings: []string{},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rapport JSON = %+v\nattendu        %+v", got, want)
	}
	// Listes toujours présentes ([] et non null), pour les scripts.
	for _, key := range []string{`"warnings": []`, `"parent_workspace": ""`} {
		if !strings.Contains(r.stdout, key) {
			t.Errorf("sortie JSON sans %s :\n%s", key, r.stdout)
		}
	}

	// --json placé avant la sous-commande : même effet (option globale).
	h2 := newHarness(t)
	r2 := h2.run("--json", "init").mustSucceed(t)
	if decodeInitReport(t, r2.stdout).Root != h2.cwd {
		t.Errorf("--json avant init : sortie inattendue :\n%s", r2.stdout)
	}
}

func TestInitJSONWithoutScopes(t *testing.T) {
	h := newHarness(t)
	h.fake().SetStatus(auth.Status{Host: "github.com", Login: "ci-bot", TokenSource: "GH_TOKEN",
		Active: true, State: "success"}, nil)

	report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
	if report.GitHub.Scopes == nil || len(report.GitHub.Scopes) != 0 {
		t.Errorf("scopes = %#v, attendu une liste vide non nulle", report.GitHub.Scopes)
	}
	if report.GitHub.TokenSource != "GH_TOKEN" || report.GitHub.Login != "ci-bot" {
		t.Errorf("compte gh = %+v", report.GitHub)
	}

	h2 := newHarness(t)
	h2.Client = h.Client
	r := h2.run("init").mustSucceed(t)
	if !strings.Contains(r.stdout, "ci-bot sur github.com (droits non indiqués par gh)") {
		t.Errorf("résumé sans droits : %s", r.stdout)
	}
}

func TestInitWorkspaceFlag(t *testing.T) {
	t.Run("chemin relatif au répertoire courant", func(t *testing.T) {
		h := newHarness(t)
		target := newDir(t, h.cwd, "promo", "master-projet")
		r := h.run("init", "-C", filepath.Join("promo", "master-projet")).mustSucceed(t)
		if !strings.Contains(r.stdout, "« master-projet »") || !strings.Contains(r.stdout, target) {
			t.Errorf("résumé inattendu :\n%s", r.stdout)
		}
		if _, err := os.Stat(filepath.Join(target, ".watchoo", "watchoo.db")); err != nil {
			t.Errorf("base absente dans le répertoire de -C : %v", err)
		}
		assertNoWorkspace(t, h.cwd)
	})

	t.Run("chemin absolu, répertoire courant inaccessible", func(t *testing.T) {
		isolateEnv(t)
		target := newDir(t, t.TempDir(), "perso")
		var stdout, stderr bytes.Buffer
		deps := Deps{
			Stdout:        &stdout,
			Stderr:        &stderr,
			NewAuthClient: func(workspace.Config) auth.Client { return authtest.NewLoggedIn("") },
			Getwd:         func() (string, error) { return "", errors.New("répertoire supprimé") },
		}
		if code := Run(context.Background(), deps, []string{"--workspace", target, "init"}); code != 0 {
			t.Fatalf("code %d, stderr :\n%s", code, stderr.String())
		}
		if _, err := os.Stat(filepath.Join(target, ".watchoo")); err != nil {
			t.Errorf(".watchoo absent : %v", err)
		}
	})

	t.Run("répertoire inexistant", func(t *testing.T) {
		h := newHarness(t)
		h.run("init", "-C", "absent").mustFail(t, filepath.Join(h.cwd, "absent"), "n'existe pas")
		if _, err := os.Stat(filepath.Join(h.cwd, "absent")); err == nil {
			t.Error("init ne doit pas créer le répertoire de -C")
		}
	})

	t.Run("sans -C, le répertoire courant", func(t *testing.T) {
		h := newHarness(t)
		h.run("init").mustSucceed(t)
		if _, err := os.Stat(filepath.Join(h.cwd, ".watchoo")); err != nil {
			t.Errorf(".watchoo absent du répertoire courant : %v", err)
		}
	})
}

func TestInitRefusesExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	h.run("init").mustSucceed(t)
	dir := filepath.Join(h.cwd, ".watchoo")

	// Données de l'espace existant : une modification de config.yaml et un dépôt en base.
	config := readFile(t, filepath.Join(dir, "config.yaml")) + "# réglage ajouté à la main\n"
	writeFile(t, filepath.Join(dir, "config.yaml"), config)
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(dir, "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 42,
		Owner: "alice", Name: "projet-licence", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{{"init"}, {"init", "--json"}, {"-C", ".", "init"}} {
		h.run(args...).mustFail(t, h.cwd, "déjà un espace Watchoo", "rien n'a été modifié")
	}

	if got := readFile(t, filepath.Join(dir, "config.yaml")); got != config {
		t.Errorf("config.yaml modifié par le second init :\n%s", got)
	}
	st, err = store.Open(ctx, filepath.Join(dir, "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.GetRepositoryByName(ctx, model.ProviderGitHub, "alice", "projet-licence"); err != nil {
		t.Errorf("dépôt perdu après le second init : %v", err)
	}
	if got := h.fake().TokenCalls(); got != 0 {
		t.Errorf("Token appelé %d fois", got)
	}
}

func TestInitIgnoresConfigOfExistingWorkspace(t *testing.T) {
	h := newHarness(t)
	h.run("init").mustSucceed(t)
	// Un config.yaml invalide dans l'espace existant : le refus doit rester clair.
	writeFile(t, filepath.Join(h.cwd, ".watchoo", "config.yaml"), "cle_inconnue: 1\n")
	r := h.run("init").mustFail(t, "déjà un espace Watchoo")
	if strings.Contains(r.stderr, "cle_inconnue") {
		t.Errorf("init a lu le config.yaml de l'espace existant :\n%s", r.stderr)
	}
}

func TestInitRefusesWatchooFile(t *testing.T) {
	h := newHarness(t)
	writeFile(t, filepath.Join(h.cwd, ".watchoo"), "pas un répertoire")
	h.run("init").mustFail(t, "n'est pas un répertoire")
	if got := readFile(t, filepath.Join(h.cwd, ".watchoo")); got != "pas un répertoire" {
		t.Errorf("fichier .watchoo modifié : %q", got)
	}
}

func TestInitNestedWorkspace(t *testing.T) {
	h := newHarness(t)
	parent := h.cwd
	h.run("init").mustSucceed(t)

	// Texte : avertissement sur la sortie d'erreur, résumé sur la sortie standard.
	h.cwd = newDir(t, parent, "groupe-a")
	r := h.run("init").mustSucceed(t)
	if !strings.HasPrefix(r.stderr, "attention : ") || !strings.Contains(r.stderr, "dans l'espace "+parent) ||
		!strings.Contains(r.stderr, "indépendant") {
		t.Errorf("avertissement d'espace parent absent ou incomplet :\n%s", r.stderr)
	}
	if !strings.Contains(r.stdout, "« groupe-a » créé") {
		t.Errorf("résumé inattendu :\n%s", r.stdout)
	}

	// JSON : même avertissement, plus parent_workspace.
	h.cwd = newDir(t, parent, "groupe-b", "sous-groupe")
	r = h.run("init", "--json").mustSucceed(t)
	report := decodeInitReport(t, r.stdout)
	if report.ParentWorkspace != parent {
		t.Errorf("parent_workspace = %q, attendu %q", report.ParentWorkspace, parent)
	}
	if len(report.Warnings) != 1 || !strings.Contains(report.Warnings[0], parent) {
		t.Errorf("warnings = %q", report.Warnings)
	}
	if !strings.Contains(r.stderr, "attention : "+report.Warnings[0]) {
		t.Errorf("avertissement absent de la sortie d'erreur en mode --json :\n%s", r.stderr)
	}

	// Les espaces sont indépendants : chacun est retrouvé depuis son propre répertoire.
	for _, root := range []string{parent, filepath.Join(parent, "groupe-a"), h.cwd} {
		ws, err := workspace.ResolveFrom("", root)
		if err != nil || ws.Root != root {
			t.Errorf("ResolveFrom(%s) = %v, %v", root, ws, err)
		}
	}
}

// Sans gh (absent ou non connecté), init crée l'espace et prévient que les dépôts seront suivis par
// clones Git locaux (V2) ; sans git non plus, il s'arrête.
func TestInitWithoutGH(t *testing.T) {
	t.Run("vrai auth.GH, chemin inexistant", func(t *testing.T) {
		h := newHarness(t)
		h.Client = nil // auth.GH réel, construit depuis la configuration
		missing := filepath.Join(t.TempDir(), "gh-absent")
		t.Setenv("WATCHOO_GITHUB_GH_PATH", missing)
		r := h.run("init").mustSucceed(t)
		assertContains(t, "init sans gh", r.stdout, "source      clones Git locaux")
		assertContains(t, "init sans gh (avertissement)", r.stderr, "gh introuvable", missing,
			"installez la CLI GitHub gh", "clones Git locaux", "gh auth login")
		if _, err := os.Stat(filepath.Join(h.cwd, ".watchoo", "watchoo.db")); err != nil {
			t.Errorf("espace non créé : %v", err)
		}
	})

	cases := map[string]func() *authtest.Fake{
		"gh absent": func() *authtest.Fake {
			f := authtest.NewLoggedIn("")
			f.SetStatus(auth.Status{}, fmt.Errorf("lancement de \"gh\" : %w", auth.ErrGHNotFound))
			return f
		},
		"non connecté": authtest.NewLoggedOut,
		"message sans conseil": func() *authtest.Fake {
			f := authtest.NewLoggedIn("")
			f.SetStatus(auth.Status{}, &wrappedError{msg: "compte absent", err: auth.ErrNotLoggedIn})
			return f
		},
	}
	for name, newFake := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.Client = newFake()
			r := h.run("init", "--json").mustSucceed(t)
			got := decodeInitReport(t, r.stdout)
			if got.Source != "git" || got.GitHub.Login != "" || len(got.Warnings) != 1 ||
				!strings.Contains(got.Warnings[0], "clones Git locaux") || !strings.Contains(got.Warnings[0], "gh auth login") {
				t.Errorf("rapport JSON sans gh = %+v", got)
			}
			if n := strings.Count(r.stderr, "gh auth login"); n != 1 {
				t.Errorf("conseil « gh auth login » répété %d fois :\n%s", n, r.stderr)
			}
			if got := h.fake().TokenCalls(); got != 0 {
				t.Errorf("Token appelé %d fois", got)
			}

			// Sans git non plus : arrêt, rien de créé.
			h2 := newHarness(t)
			h2.Client = newFake()
			h2.GitMissing = true
			r = h2.run("init").mustFail(t, "clones Git locaux", "git est introuvable")
			if strings.Count(r.stderr, "\n") != 1 {
				t.Errorf("message sur plusieurs lignes :\n%s", r.stderr)
			}
			assertNoWorkspace(t, h2.cwd)
		})
	}
}

// Token refusé par GitHub (gh connecté à un compte dont le token est révoqué) : gh garde ce token et
// les synchros passeraient par l'API ; init s'arrête et conseille de se reconnecter.
func TestInitTokenRefused(t *testing.T) {
	h := newHarness(t)
	f := authtest.NewLoggedIn("alice")
	f.SetStatus(auth.Status{Host: "github.com", Login: "alice", State: "error"},
		fmt.Errorf("le token gh du compte \"alice\" n'est pas utilisable : %w", auth.ErrNotLoggedIn))
	h.Client = f
	r := h.run("init").mustFail(t, "lancez « gh auth login »")
	if n := strings.Count(r.stderr, "gh auth login"); n != 1 {
		t.Errorf("conseil « gh auth login » répété %d fois :\n%s", n, r.stderr)
	}
	assertNoWorkspace(t, h.cwd)
	if got := h.fake().TokenCalls(); got != 0 {
		t.Errorf("Token appelé %d fois", got)
	}
}

// wrappedError est une erreur dont le message ne reprend pas celui de l'erreur enveloppée.
type wrappedError struct {
	msg string
	err error
}

func (e *wrappedError) Error() string { return e.msg }
func (e *wrappedError) Unwrap() error { return e.err }

func TestInitOtherGHErrors(t *testing.T) {
	for name, err := range map[string]error{
		"délai":   fmt.Errorf("« gh auth status » : %w (10s)", auth.ErrTimeout),
		"réponse": fmt.Errorf("lecture de la réponse de « gh auth status » : %w", auth.ErrUnexpectedOutput),
		"GitHub injoignable": fmt.Errorf("le token gh du compte \"alice\" n'a pas pu être vérifié : %w",
			auth.ErrCheckFailed),
		"github.user inutilisable": fmt.Errorf("github.user vaut \"Alice\", mais gh connaît \"alice\" : %w",
			auth.ErrUnusableUser),
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.fake().SetStatus(auth.Status{}, err)
			r := h.run("init").mustFail(t, "vérification de gh : "+err.Error())
			// Se reconnecter ne réglerait rien : pas de conseil « gh auth login ».
			if strings.Contains(r.stderr, "gh auth login") {
				t.Errorf("conseil « gh auth login » inapproprié :\n%s", r.stderr)
			}
			assertNoWorkspace(t, h.cwd)
		})
	}
}

func TestInitInterrupted(t *testing.T) {
	h := newHarness(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.runContext(ctx, "init").mustFail(t, "commande interrompue")
	assertNoWorkspace(t, h.cwd)
}

func TestInitUsesUserAndEnvironmentSettings(t *testing.T) {
	h := newHarness(t)
	userConfig := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "watchoo", "config.yaml")
	writeFile(t, userConfig, "github:\n  gh_path: /opt/gh/bin/gh\n  user: compte-utilisateur\n")
	t.Setenv("WATCHOO_GITHUB_USER", "compte-env")

	h.run("init").mustSucceed(t)
	if len(h.configs) != 1 {
		t.Fatalf("NewAuthClient appelé %d fois, attendu 1", len(h.configs))
	}
	if got := h.configs[0].GitHub; got.GHPath != "/opt/gh/bin/gh" || got.User != "compte-env" {
		t.Errorf("réglages gh transmis = %+v, attendu gh_path du fichier utilisateur et user de l'environnement", got)
	}
}

func TestInitInvalidSettings(t *testing.T) {
	h := newHarness(t)
	userConfig := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "watchoo", "config.yaml")
	// Clé inconnue dans le fichier utilisateur : l'erreur cite le fichier.
	writeFile(t, userConfig, "sync:\n  intervall: 1h\n")
	h.run("init").mustFail(t, userConfig, "sync.intervall")
	// Valeur hors limites : l'erreur cite la clé.
	writeFile(t, userConfig, "sync:\n  interval: 5m\n")
	h.run("init").mustFail(t, "réglages invalides", "sync.interval")
	assertNoWorkspace(t, h.cwd)
	if got := h.fake().StatusCalls(); got != 0 {
		t.Errorf("Status appelé %d fois malgré des réglages invalides", got)
	}
}

func TestInitDatabaseFailureRemovesWorkspace(t *testing.T) {
	h := newHarness(t)
	original := setupDatabase
	t.Cleanup(func() { setupDatabase = original })
	var dbPath string
	setupDatabase = func(ctx context.Context, path string) error {
		dbPath = path
		if err := original(ctx, path); err != nil {
			return err
		}
		return errors.New("disque plein (simulé)")
	}

	h.run("init").mustFail(t, "création de la base de l'espace", "disque plein (simulé)")
	if dbPath != filepath.Join(h.cwd, ".watchoo", "watchoo.db") {
		t.Errorf("base créée à %q", dbPath)
	}
	assertNoWorkspace(t, h.cwd)

	// Une fois le problème réglé, init fonctionne à nouveau.
	setupDatabase = original
	h.run("init").mustSucceed(t)
}

func TestInitWarnsAboutWatchooDir(t *testing.T) {
	t.Run("autre espace", func(t *testing.T) {
		h := newHarness(t)
		other := newDir(t, t.TempDir(), "autre")
		t.Setenv("WATCHOO_DIR", other)
		r := h.run("init").mustSucceed(t)
		if !strings.Contains(r.stderr, "attention : la variable WATCHOO_DIR vaut \""+other+"\"") {
			t.Errorf("avertissement WATCHOO_DIR absent :\n%s", r.stderr)
		}
		// init travaille sur le répertoire courant, pas sur WATCHOO_DIR.
		assertNoWorkspace(t, other)
	})

	t.Run("autre espace existant", func(t *testing.T) {
		h := newHarness(t)
		other := newDir(t, t.TempDir(), "autre")
		h2 := &harness{t: t, cwd: other, Client: authtest.NewLoggedIn("")}
		h2.run("init").mustSucceed(t)
		t.Setenv("WATCHOO_DIR", filepath.Join(other, "sous-repertoire"))
		newDir(t, other, "sous-repertoire")
		r := h.run("init").mustSucceed(t)
		if !strings.Contains(r.stderr, "les autres commandes utiliseront l'espace "+other+" et non celui-ci") {
			t.Errorf("avertissement WATCHOO_DIR absent ou incomplet :\n%s", r.stderr)
		}
	})

	t.Run("même espace", func(t *testing.T) {
		h := newHarness(t)
		t.Setenv("WATCHOO_DIR", filepath.Join(h.cwd, "sous-repertoire-a-venir"))
		newDir(t, h.cwd, "sous-repertoire-a-venir")
		r := h.run("init").mustSucceed(t)
		if r.stderr != "" {
			t.Errorf("avertissement inattendu :\n%s", r.stderr)
		}
	})

	// Un WATCHOO_DIR inexistant ou qui désigne un fichier, dans le nouvel espace : Find remonterait
	// jusqu'à l'espace créé, mais les autres commandes refuseront ce chemin (ResolveFrom).
	for name, setup := range map[string]func(h *harness) string{
		"répertoire inexistant":     func(h *harness) string { return filepath.Join(h.cwd, "typo") },
		"chemin relatif inexistant": func(h *harness) string { return "typo" },
		"fichier": func(h *harness) string {
			path := filepath.Join(h.cwd, "notes.txt")
			writeFile(t, path, "pas un répertoire")
			return path
		},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			env := setup(h)
			t.Setenv("WATCHOO_DIR", env)
			r := h.run("init", "--json").mustSucceed(t)
			want := "la variable WATCHOO_DIR vaut " + fmt.Sprintf("%q", env) +
				" : tant qu'elle est définie, les autres commandes échoueront"
			if !strings.Contains(r.stderr, "attention : "+want) {
				t.Errorf("avertissement WATCHOO_DIR absent :\n%s", r.stderr)
			}
			if report := decodeInitReport(t, r.stdout); len(report.Warnings) != 1 ||
				!strings.HasPrefix(report.Warnings[0], want) {
				t.Errorf("warnings = %q", report.Warnings)
			}
			// L'avertissement dit vrai : la recherche des autres commandes échoue bien.
			if _, err := workspace.ResolveFrom("", h.cwd); err == nil {
				t.Error("ResolveFrom a réussi avec ce WATCHOO_DIR")
			}
		})
	}

	t.Run("avec -C", func(t *testing.T) {
		h := newHarness(t)
		t.Setenv("WATCHOO_DIR", t.TempDir())
		target := newDir(t, h.cwd, "cible")
		r := h.run("init", "-C", target).mustSucceed(t)
		if r.stderr != "" {
			t.Errorf("avertissement inattendu avec -C :\n%s", r.stderr)
		}
	})
}

// fakeGHCall est un appel enregistré par le faux gh (FAKEGH_RECORD_FILE).
type fakeGHCall struct {
	Args []string `json:"args"`
}

// useFakeGH fait passer le harness par le vrai auth.GH et le faux binaire gh, avec le scénario
// donné ; elle renvoie le chemin du fichier où le faux gh enregistre ses appels.
func useFakeGH(t *testing.T, h *harness, scenario string) string {
	t.Helper()
	h.Client = nil
	dir := t.TempDir()
	record := filepath.Join(dir, "calls.jsonl")
	t.Setenv("WATCHOO_GITHUB_GH_PATH", fakeGHPath)
	t.Setenv("FAKEGH_SCENARIO", scenario)
	t.Setenv("FAKEGH_RECORD_FILE", record)
	t.Setenv("FAKEGH_STATE_FILE", filepath.Join(dir, "state"))
	return record
}

// readCalls renvoie les appels enregistrés par le faux gh.
func readCalls(t *testing.T, record string) []fakeGHCall {
	t.Helper()
	data, err := os.ReadFile(record)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls []fakeGHCall
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		var c fakeGHCall
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("appel enregistré illisible : %v", err)
		}
		calls = append(calls, c)
	}
	return calls
}

// assertOnlyStatusCalls vérifie que le faux gh n'a reçu que des « auth status », jamais « auth token ».
func assertOnlyStatusCalls(t *testing.T, record string) {
	t.Helper()
	calls := readCalls(t, record)
	if len(calls) == 0 {
		t.Fatal("le faux gh n'a pas été appelé")
	}
	for _, c := range calls {
		if len(c.Args) < 2 || c.Args[0] != "auth" || c.Args[1] != "status" {
			t.Errorf("appel inattendu du faux gh : %q (init ne doit lancer que « gh auth status »)", c.Args)
		}
	}
}

func TestInitWithFakeGHBinary(t *testing.T) {
	t.Run("connecté", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "logged-in")
		report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
		if report.GitHub.Login != authtest.FakeLogin || report.GitHub.TokenSource != "keyring" {
			t.Errorf("compte gh = %+v", report.GitHub)
		}
		assertOnlyStatusCalls(t, record)
	})

	t.Run("compte choisi par github.user", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "multi-account")
		t.Setenv("WATCHOO_GITHUB_USER", "watchoo-other")
		report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
		if report.GitHub.Login != "watchoo-other" || !reflect.DeepEqual(report.GitHub.Scopes, []string{"repo"}) {
			t.Errorf("compte gh = %+v, attendu watchoo-other", report.GitHub)
		}
		assertOnlyStatusCalls(t, record)
	})

	// Aucun compte : espace créé, dépôts suivis par clones Git locaux (V2).
	for _, scenario := range []string{"not-logged-in", "exit-not-logged-in"} {
		t.Run(scenario, func(t *testing.T) {
			h := newHarness(t)
			record := useFakeGH(t, h, scenario)
			r := h.run("init", "--json").mustSucceed(t)
			if report := decodeInitReport(t, r.stdout); report.Source != "git" {
				t.Errorf("source = %q, git attendue", report.Source)
			}
			assertContains(t, scenario, r.stderr, "clones Git locaux", "lancez « gh auth login »")
			assertOnlyStatusCalls(t, record)
		})
	}

	// Token refusé par GitHub : se reconnecter.
	t.Run("invalid-token", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "invalid-token")
		h.run("init").mustFail(t, "lancez « gh auth login »")
		assertNoWorkspace(t, h.cwd)
		assertOnlyStatusCalls(t, record)
	})

	t.Run("GH_TOKEN seul, sans github.user", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "env-token-only")
		report := decodeInitReport(t, h.run("init", "--json").mustSucceed(t).stdout)
		if report.GitHub.Login != authtest.FakeLogin || report.GitHub.TokenSource != "GH_TOKEN" {
			t.Errorf("compte gh = %+v, attendu celui de GH_TOKEN", report.GitHub)
		}
		assertOnlyStatusCalls(t, record)
	})

	// Docker / CI : gh ne connaît le compte que par GH_TOKEN, que « gh auth token --user » ignore.
	// init doit refuser github.user, sinon chaque synchro échouerait ensuite.
	t.Run("GH_TOKEN seul, avec github.user", func(t *testing.T) {
		h := newHarness(t)
		record := useFakeGH(t, h, "env-token-only")
		t.Setenv("WATCHOO_GITHUB_USER", authtest.FakeLogin)
		r := h.run("init").mustFail(t, "vérification de gh", `github.user vaut "watchoo-test"`, "GH_TOKEN", "videz-le")
		if strings.Contains(r.stderr, "gh auth login") {
			t.Errorf("conseil « gh auth login » inapproprié :\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
		assertOnlyStatusCalls(t, record)
	})

	// gh distingue les majuscules dans « gh auth token --user ».
	t.Run("github.user avec une autre casse", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "multi-account")
		t.Setenv("WATCHOO_GITHUB_USER", "Watchoo-Other")
		r := h.run("init").mustFail(t, `github.user vaut "Watchoo-Other"`, `sous le nom "watchoo-other"`, "majuscules")
		if strings.Contains(r.stderr, "gh auth login") {
			t.Errorf("conseil « gh auth login » inapproprié :\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
	})

	// GitHub injoignable (proxy, réseau coupé) : l'erreur le dit, sans conseiller « gh auth login ».
	t.Run("GitHub injoignable", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "offline")
		r := h.run("init").mustFail(t, "vérification de gh", "n'a pas pu être vérifié", "proxyconnect tcp",
			"vérifiez la connexion réseau")
		if strings.Contains(r.stderr, "gh auth login") {
			t.Errorf("conseil « gh auth login » inapproprié :\n%s", r.stderr)
		}
		if strings.Contains(r.stderr, authtest.FakeToken) {
			t.Errorf("token dans la sortie d'erreur :\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
	})

	t.Run("compte github.user absent", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "logged-in")
		t.Setenv("WATCHOO_GITHUB_USER", "inconnu")
		h.run("init").mustFail(t, "inconnu", "github.user", "lancez « gh auth login »")
		assertNoWorkspace(t, h.cwd)
	})

	t.Run("erreur de gh", func(t *testing.T) {
		h := newHarness(t)
		useFakeGH(t, h, "error")
		r := h.run("init").mustFail(t, "vérification de gh")
		if strings.Contains(r.stderr, authtest.FakeToken) {
			t.Errorf("token dans la sortie d'erreur :\n%s", r.stderr)
		}
		assertNoWorkspace(t, h.cwd)
	})
}
