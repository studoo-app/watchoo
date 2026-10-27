package cli

// Tests de la mise à jour (0.6.0) : avis de nouvelle version et « watchoo update ». Les releases sont
// celles du faux serveur selfupdatetest : aucun accès au réseau.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/selfupdate"
	"github.com/studoo-app/watchoo/internal/selfupdate/selfupdatetest"
)

const wantNotice = "Nouvelle version de Watchoo disponible : v0.5.0 → v0.6.0. Mettez-la à jour avec « watchoo update »."

// newUpdateHarness renvoie un harness en version current (comme injectée par -ldflags) dont les
// releases sont celles d'un faux serveur, de dernière version latest, pour la plateforme courante.
func newUpdateHarness(t *testing.T, current, latest string) (*harness, *selfupdatetest.Server) {
	t.Helper()
	h := newHarness(t)
	setBuildVars(t, current, "", "")
	srv := selfupdatetest.New(t, latest, runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL, UserAgent: "watchoo/" + current})
	return h, srv
}

// fakeExecutable crée un faux binaire installé et le déclare au harness.
func fakeExecutable(t *testing.T, h *harness) string {
	t.Helper()
	name := "watchoo"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	exe := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(exe, []byte("ancien"), 0o755); err != nil {
		t.Fatal(err)
	}
	h.Executable = exe
	return exe
}

func TestUpdateNoticePrintedOnStderr(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	h.StderrTerminal = true
	r := h.run("version").mustSucceed(t)
	if r.stderr != wantNotice+"\n" {
		t.Errorf("stderr = %q, attendu l'avis", r.stderr)
	}
	assertNotContains(t, "stdout", r.stdout, "Nouvelle version")
	// Aussi après une commande en erreur (hors espace), après le message d'erreur.
	r = h.run("status").mustCode(t, 1)
	if !strings.HasPrefix(r.stderr, "erreur : ") || !strings.HasSuffix(r.stderr, wantNotice+"\n") {
		t.Errorf("stderr = %q", r.stderr)
	}
	// Une vérification par commande (pas de cache).
	if n := srv.Requests(); n != 2 {
		t.Errorf("%d requêtes, attendu 2", n)
	}
	// Dans l'image Docker : changer le tag.
	h.InDocker = true
	r = h.run("version").mustSucceed(t)
	assertContains(t, "avis Docker", r.stderr, "v0.5.0 → v0.6.0", "Changez le tag de l'image Docker")
}

func TestUpdateNoticeSkipped(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		latest   string
		terminal bool
		env      string
		args     []string
	}{
		{name: "json", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"version", "--json"}},
		{name: "stderr hors terminal", current: "v0.5.0", latest: "v0.6.0", args: []string{"version"}},
		{name: "variable", current: "v0.5.0", latest: "v0.6.0", terminal: true, env: "1", args: []string{"version"}},
		{name: "version dev", current: "dev", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "pseudo-version", current: "v0.0.0-20261027100000-9412b2f170d8", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "à jour", current: "v0.6.0", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "plus récente", current: "v0.7.0", latest: "v0.6.0", terminal: true, args: []string{"version"}},
		{name: "aide", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{}},
		{name: "help", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"help", "sync"}},
		{name: "update --check", current: "v0.5.0", latest: "v0.6.0", terminal: true, args: []string{"update", "--check"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, srv := newUpdateHarness(t, tt.current, tt.latest)
			h.StderrTerminal = tt.terminal
			if tt.env != "" {
				t.Setenv(selfupdate.EnvNoCheck, tt.env)
			}
			r := h.run(tt.args...).mustSucceed(t)
			assertNotContains(t, "stderr", r.stderr, "Nouvelle version de Watchoo")
			wantRequests := int64(0)
			switch tt.name {
			case "à jour", "plus récente":
				wantRequests = 1 // vérifiée, mais rien à annoncer
			case "update --check":
				wantRequests = 1 // la commande elle-même, pas l'avis
			}
			if n := srv.Requests(); n != wantRequests {
				t.Errorf("%d requêtes, attendu %d", n, wantRequests)
			}
		})
	}
	// « 0 » et « false » ne coupent pas la vérification.
	for _, v := range []string{"0", "false"} {
		h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
		h.StderrTerminal = true
		t.Setenv(selfupdate.EnvNoCheck, v)
		assertContains(t, "WATCHOO_NO_UPDATE_CHECK="+v, h.run("version").mustSucceed(t).stderr, wantNotice)
	}
}

// Un serveur trop lent ne retarde pas la commande au-delà du délai, et l'avis est abandonné.
func TestUpdateNoticeDoesNotDelayCommand(t *testing.T) {
	original := updateNoticeTimeout
	t.Cleanup(func() { updateNoticeTimeout = original })
	updateNoticeTimeout = 100 * time.Millisecond

	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	h.StderrTerminal = true
	srv.Delay = 5 * time.Second
	start := time.Now()
	r := h.run("version").mustSucceed(t)
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("commande retardée de %v", d)
	}
	if r.stderr != "" {
		t.Errorf("stderr = %q, attendu vide", r.stderr)
	}
}

func TestUpdateCheckText(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	r := h.run("update", "--check").mustSucceed(t)
	assertContains(t, "update --check", r.stdout, "Version installée : v0.5.0", "Dernière version : v0.6.0",
		"Nouvelle version disponible : http://127.0.0.1", "/tag/v0.6.0", "« watchoo update »")

	h, _ = newUpdateHarness(t, "v0.6.0", "v0.6.0")
	assertContains(t, "à jour", h.run("update", "--check").mustSucceed(t).stdout, "Watchoo est à jour.")

	h, _ = newUpdateHarness(t, "dev", "v0.6.0")
	assertContains(t, "dev", h.run("update", "--check").mustSucceed(t).stdout, "Dernière version : v0.6.0",
		"comparaison impossible")

	// Hors espace et sans gh : pas besoin d'espace ; réseau en échec : code non nul.
	h = newHarness(t)
	h.run("update", "--check").mustFail(t, "impossible de lire la dernière version")
}

func TestUpdateCheckJSON(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	r := h.run("update", "--check", "--json").mustSucceed(t)
	assertSingleJSONObject(t, "update --check --json", r.stdout)
	assertJSONKeys(t, r.stdout, "", jsonTopLevelKeys["update"]...)
	var got updateReport
	if err := json.Unmarshal([]byte(r.stdout), &got); err != nil {
		t.Fatal(err)
	}
	want := updateReport{Current: "v0.5.0", Latest: "v0.6.0", UpdateAvailable: true, ReleaseURL: srv.URL + "/tag/v0.6.0"}
	if got != want {
		t.Errorf("update --check --json = %+v, attendu %+v", got, want)
	}
	// Sans --check, --json est refusé.
	h.run("update", "--json").mustFail(t, "--check")
}

func TestUpdateConfirmed(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	h.StdinTerminal, h.Stdin = true, "o\n"
	r := h.run("update").mustSucceed(t)
	assertContains(t, "update", r.stdout, "Mise à jour de Watchoo : v0.5.0 → v0.6.0", "/tag/v0.6.0",
		"/blob/v0.6.0/CHANGELOG.md", "Watchoo v0.6.0 installé dans "+exe, "watchoo serve", "migrations")
	assertContains(t, "question", r.stderr, "Installer Watchoo v0.6.0 ? [o/N]")
	if got, _ := os.ReadFile(exe); string(got) != selfupdatetest.DefaultBinary {
		t.Errorf("binaire = %q, attendu le nouveau", got)
	}
}

func TestUpdateRefusedByUser(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	h.StdinTerminal, h.Stdin = true, "n\n"
	r := h.run("update").mustCode(t, 1)
	assertContains(t, "annulation", r.stderr, "mise à jour annulée")
	if got, _ := os.ReadFile(exe); string(got) != "ancien" {
		t.Errorf("binaire modifié : %q", got)
	}
}

func TestUpdateYesAndNonTerminal(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	// Entrée hors terminal sans --yes : refus, rien de modifié.
	h.run("update").mustCode(t, 1)
	if got, _ := os.ReadFile(exe); string(got) != "ancien" {
		t.Errorf("binaire modifié sans --yes : %q", got)
	}
	r := h.run("update", "-y").mustSucceed(t)
	assertNotContains(t, "question", r.stderr, "[o/N]")
	if got, _ := os.ReadFile(exe); string(got) != selfupdatetest.DefaultBinary {
		t.Errorf("binaire = %q, attendu le nouveau", got)
	}
}

func TestUpdateNonTerminalMessage(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	fakeExecutable(t, h)
	r := h.run("update").mustCode(t, 1)
	assertContains(t, "sans terminal", r.stderr, "--yes")
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	h, _ := newUpdateHarness(t, "v0.6.0", "v0.6.0")
	exe := fakeExecutable(t, h)
	r := h.run("update").mustSucceed(t)
	assertContains(t, "à jour", r.stdout, "Watchoo v0.6.0 est à jour")
	if got, _ := os.ReadFile(exe); string(got) != "ancien" {
		t.Errorf("binaire modifié : %q", got)
	}
}

func TestUpdateRefusals(t *testing.T) {
	h, srv := newUpdateHarness(t, "v0.5.0", "v0.6.0")
	fakeExecutable(t, h)
	h.InDocker = true
	h.run("update", "--yes").mustFail(t, "Docker", "docker compose pull")
	h.InDocker = false

	setBuildVars(t, "dev", "", "")
	h.run("update", "--yes").mustFail(t, "compilée depuis les sources", "go install", "make build")
	if n := srv.Requests(); n != 0 {
		t.Errorf("%d requêtes avant les refus, attendu 0", n)
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("droits POSIX non applicables (Windows ou root)")
	}
	setBuildVars(t, "v0.5.0", "", "")
	ro := filepath.Join(t.TempDir(), "ro")
	if err := os.Mkdir(ro, 0o755); err != nil {
		t.Fatal(err)
	}
	h.Executable = filepath.Join(ro, "watchoo")
	if err := os.WriteFile(h.Executable, []byte("ancien"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(ro, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(ro, 0o755) })
	h.run("update", "--yes").mustFail(t, "n'est pas accessible en écriture", "sudo", "WATCHOO_INSTALL_DIR")
}

// « watchoo serve » vérifie au démarrage : avis sur stderr et bandeau dans l'interface web.
func TestServeAnnouncesNewVersion(t *testing.T) {
	h := newSyncedWorkspace(t)
	setBuildVars(t, "v0.5.0", "", "")
	srv := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL})
	run := startServe(t, h, "--interval", "0")
	deadline := time.Now().Add(10 * time.Second)
	for {
		_, body := run.get(t, "/")
		if strings.Contains(body, `id="update-banner"`) {
			assertContains(t, "bandeau", body, "v0.5.0 → v0.6.0", "watchoo update", srv.URL+"/tag/v0.6.0")
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("bandeau absent :\n%s", body)
		}
		time.Sleep(10 * time.Millisecond)
	}
	r := run.stop(t).mustSucceed(t)
	if strings.Count(r.stderr, wantNotice) != 1 {
		t.Errorf("stderr = %q, attendu l'avis une fois", r.stderr)
	}
}

func TestServeUpdateCheckDisabled(t *testing.T) {
	h := newSyncedWorkspace(t)
	setBuildVars(t, "v0.5.0", "", "")
	t.Setenv(selfupdate.EnvNoCheck, "1")
	srv := selfupdatetest.New(t, "v0.6.0", runtime.GOOS, runtime.GOARCH)
	h.Updater = selfupdate.New(selfupdate.Options{BaseURL: srv.URL})
	run := startServe(t, h, "--interval", "0")
	_, body := run.get(t, "/")
	assertNotContains(t, "page", body, `id="update-banner"`)
	r := run.stop(t).mustSucceed(t)
	assertNotContains(t, "stderr", r.stderr, "Nouvelle version")
	if n := srv.Requests(); n != 0 {
		t.Errorf("%d requêtes, attendu 0", n)
	}
}
