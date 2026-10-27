package cli

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/tui"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// isolateEnv isole un test de l'environnement du développeur : HOME (et USERPROFILE sous Windows) et
// XDG_CONFIG_HOME pointent vers des répertoires temporaires vides, et les variables WATCHOO_*
// (WATCHOO_DIR compris), GH_TOKEN et GITHUB_TOKEN sont retirées. Elle renvoie le chemin du fichier de
// réglages utilisateur, qui n'existe pas encore. Les tests qui l'appellent ne peuvent pas être parallèles
// (t.Setenv).
func isolateEnv(t *testing.T) string {
	t.Helper()
	base := t.TempDir()
	home := filepath.Join(base, "home")
	xdg := filepath.Join(base, "xdg")
	for _, dir := range []string{home, xdg} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home) // répertoire personnel selon os.UserHomeDir (« ~ » des chemins affichés)
	}
	t.Setenv("XDG_CONFIG_HOME", xdg)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, workspace.EnvPrefix+"_") || name == "GH_TOKEN" || name == "GITHUB_TOKEN" {
			t.Setenv(name, "") // restaurée à la fin du test
			if err := os.Unsetenv(name); err != nil {
				t.Fatal(err)
			}
		}
	}
	return filepath.Join(xdg, workspace.UserConfigDirName, workspace.ConfigFileName)
}

// harness exécute la CLI avec des dépendances de test : sorties capturées, répertoire courant
// simulé, accès à gh fourni par Client (un authtest.Fake par défaut), sortie standard décrite par
// Terminal.
type harness struct {
	t      *testing.T
	cwd    string      // répertoire courant renvoyé par Getwd
	Client auth.Client // accès à gh renvoyé par NewAuthClient ; nil = auth.GH réel (DefaultDeps)
	// Terminal décrit la sortie standard ; nil = valeur par défaut (DetectTerminal du tampon qui
	// capture la sortie : jamais un terminal).
	Terminal func() TermInfo
	// configs reçoit la Config passée à chaque appel de NewAuthClient.
	configs []workspace.Config

	// Dépendances de la phase 3.

	// Provider est le faux provider renvoyé par NewProvider ; nil = NewProvider renvoie une erreur
	// (aucun test ne joint GitHub).
	Provider *providertest.Fake
	// NewProvider, s'il est renseigné, remplace Provider : il construit le provider à partir du token
	// (le vrai provider GitHub branché sur githubtest, pour le test de bout en bout).
	NewProvider func(token string) (provider.Provider, error)
	// tokens reçoit le token passé à chaque appel de NewProvider.
	tokens []string
	// Now est l'horloge ; nil = time.Now.
	Now func() time.Time
	// Stdin est le contenu de l'entrée standard ; StdinTerminal dit s'il s'agit d'un terminal.
	Stdin         string
	StdinTerminal bool
	// RunTUI remplace tui.Run ; nil = un faux qui échoue (le TUI réel n'est jamais lancé).
	RunTUI func(ctx context.Context, deps tui.Deps, term tui.IO) error
	// Listen remplace net.Listen pour « watchoo serve » ; nil = un faux qui échoue (aucun test
	// n'écoute sans le demander).
	Listen func(network, address string) (net.Listener, error)
	// GitURL est la racine du faux serveur Git (gittest) des clones locaux, utilisés sans gh ; vide =
	// NewGitProvider renvoie une erreur (aucun test ne joint GitHub par git).
	GitURL string
	// GitMissing simule un git absent pour « watchoo init » sans gh.
	GitMissing bool
}

// newHarness isole l'environnement et renvoie un harness dont le répertoire courant est un
// répertoire temporaire nommé « licence-projet », avec un faux gh connecté. Le test est sauté si un
// espace Watchoo existe au-dessus du répertoire temporaire (voir requireNoWorkspaceAbove).
func newHarness(t *testing.T) *harness {
	t.Helper()
	isolateEnv(t)
	base := t.TempDir()
	requireNoWorkspaceAbove(t, base)
	return &harness{
		t:      t,
		cwd:    newDir(t, base, "licence-projet"),
		Client: authtest.NewLoggedIn(""),
	}
}

// requireNoWorkspaceAbove saute le test si un espace Watchoo existe dans dir ou au-dessus (TMPDIR
// placé sous un répertoire qui contient un .watchoo/, par exemple) : init y verrait un espace parent
// et afficherait un avertissement que les tests n'attendent pas.
func requireNoWorkspaceAbove(t *testing.T, dir string) {
	t.Helper()
	if ws, err := workspace.Find(dir); err == nil {
		t.Skipf("un espace Watchoo existe au-dessus du répertoire temporaire %s : %s", dir, ws.Root)
	}
}

// result est le résultat d'une exécution de la CLI.
type result struct {
	code   int
	stdout string
	stderr string
}

// run exécute « watchoo args… » avec un contexte de fond.
func (h *harness) run(args ...string) result {
	h.t.Helper()
	return h.runContext(context.Background(), args...)
}

// runContext exécute « watchoo args… » avec ctx.
func (h *harness) runContext(ctx context.Context, args ...string) result {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	deps := Deps{
		Stdout: &stdout,
		Stderr: &stderr,
		NewAuthClient: func(cfg workspace.Config) auth.Client {
			h.configs = append(h.configs, cfg)
			if h.Client == nil {
				return DefaultDeps().NewAuthClient(cfg)
			}
			return h.Client
		},
		Getwd:    func() (string, error) { return h.cwd, nil },
		Terminal: h.Terminal,
		NewProvider: func(token string) (provider.Provider, error) {
			h.tokens = append(h.tokens, token)
			if h.NewProvider != nil {
				return h.NewProvider(token)
			}
			if h.Provider == nil {
				return nil, errors.New("aucun provider dans ce test")
			}
			return h.Provider, nil
		},
		NewGitProvider: func() (provider.Provider, error) {
			if h.GitURL == "" {
				return nil, errors.New("aucun dépôt Git dans ce test")
			}
			return gitprov.New(gitprov.Options{Dir: filepath.Join(h.cwd, workspace.DirName, workspace.ReposDirName),
				BaseURL: h.GitURL, Logger: slog.Default()})
		},
		CheckGit: func(context.Context) error {
			if h.GitMissing {
				return gitprov.ErrGitNotFound
			}
			return nil
		},
		Now:             h.Now,
		Stdin:           strings.NewReader(h.Stdin),
		StdinIsTerminal: func() bool { return h.StdinTerminal },
		RunTUI:          h.RunTUI,
		Listen:          h.Listen,
		Logger:          slog.Default(), // testLogs (TestMain)
	}
	if deps.Listen == nil {
		deps.Listen = func(string, string) (net.Listener, error) {
			return nil, errors.New("aucune écoute réseau dans ce test")
		}
	}
	if deps.RunTUI == nil {
		deps.RunTUI = func(context.Context, tui.Deps, tui.IO) error {
			return errors.New("le vrai TUI ne doit pas être lancé dans les tests")
		}
	}
	code := Run(ctx, deps, args)
	r := result{code: code, stdout: stdout.String(), stderr: stderr.String()}
	// Le token n'apparaît jamais : ni dans les sorties, ni dans les logs.
	for name, out := range map[string]string{"stdout": r.stdout, "stderr": r.stderr, "logs": testLogs.String()} {
		if strings.Contains(out, authtest.FakeToken) {
			h.t.Errorf("watchoo %q : token dans %s :\n%s", args, name, out)
		}
	}
	return r
}

// fake renvoie le authtest.Fake du harness.
func (h *harness) fake() *authtest.Fake {
	h.t.Helper()
	f, ok := h.Client.(*authtest.Fake)
	if !ok {
		h.t.Fatalf("Client n'est pas un *authtest.Fake : %T", h.Client)
	}
	return f
}

// mustSucceed vérifie que l'exécution a réussi.
func (r result) mustSucceed(t *testing.T) result {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("code de retour %d, attendu 0\nstdout :\n%s\nstderr :\n%s", r.code, r.stdout, r.stderr)
	}
	return r
}

// mustFail vérifie que l'exécution a échoué (code 1), sans rien sur la sortie standard, avec un
// message « erreur : … » qui contient chacun des extraits.
func (r result) mustFail(t *testing.T, parts ...string) result {
	t.Helper()
	if r.code != 1 {
		t.Fatalf("code de retour %d, attendu 1\nstdout :\n%s\nstderr :\n%s", r.code, r.stdout, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("sortie standard non vide après une erreur :\n%s", r.stdout)
	}
	if !strings.HasPrefix(r.stderr, "erreur : ") {
		t.Errorf("sortie d'erreur sans « erreur : » au début :\n%s", r.stderr)
	}
	for _, part := range parts {
		if !strings.Contains(r.stderr, part) {
			t.Errorf("sortie d'erreur sans %q :\n%s", part, r.stderr)
		}
	}
	return r
}

// newDir crée le répertoire parent/elems… (avec ses parents) et renvoie son chemin.
func newDir(t *testing.T, parent string, elems ...string) string {
	t.Helper()
	dir := filepath.Join(append([]string{parent}, elems...)...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// writeFile écrit content dans path, en créant les répertoires parents.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// readFile renvoie le contenu de path.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// assertNoWorkspace vérifie que root ne contient pas de .watchoo/.
func assertNoWorkspace(t *testing.T, root string) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(root, workspace.DirName)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s/%s ne devrait pas exister (Lstat : %v)", root, workspace.DirName, err)
	}
}

// assertMode vérifie les droits de path (ignoré sous Windows, qui n'a pas de droits Unix).
func assertMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("droits de %s = %#o, attendu %#o", path, got, want)
	}
}

// migratedTables est la liste des tables créées par les migrations (hors table de goose).
var migratedTables = []string{
	"branches", "commit_branches", "commits", "contributor_emails", "contributors", "pull_requests", "releases",
	"repositories", "sync_runs", "tags",
}

// tables renvoie les tables de la base path, triées, hors tables internes de SQLite. La base est
// ouverte directement, sans passer par store (ni pragmas ni migrations), pour vérifier le fichier tel
// qu'init l'a laissé. Pas de mode=ro : une base WAL sans fichier -shm ne s'ouvre pas toujours en
// lecture seule.
func tables(t *testing.T, path string) []string {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	rows, err := sqlDB.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatalf("lecture des tables de %s : %v", path, err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	return names
}
