package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func TestVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	h := newHarness(t)
	r := h.run("version").mustSucceed(t)
	want := "watchoo dev (" + runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH + ")\n"
	if r.stdout != want || r.stderr != "" {
		t.Errorf("version = %q (stderr %q), attendu %q", r.stdout, r.stderr, want)
	}

	Version = "v1.2.3" // valeur injectée par -ldflags -X
	r = h.run("version", "--json").mustSucceed(t)
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	dec.DisallowUnknownFields()
	var got versionReport
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("sortie JSON invalide : %v\n%s", err, r.stdout)
	}
	if (got != versionReport{Version: "v1.2.3", Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}) {
		t.Errorf("version JSON = %+v", got)
	}

	// version n'a besoin ni d'un espace ni de gh.
	if len(h.configs) != 0 {
		t.Errorf("version a construit un accès à gh")
	}
	h.run("version", "extra").mustFail(t, "« watchoo version » ne prend pas d'argument")
}

func TestHelpIsInFrench(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{nil, {"--help"}, {"help"}, {"-h"}} {
		r := h.run(args...).mustSucceed(t)
		for _, want := range []string{"Utilisation :", "Commandes :", "init", "version", "--json",
			"-C, --workspace chemin", "affiche l'aide de la commande"} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("watchoo %q : aide sans %q :\n%s", args, want, r.stdout)
			}
		}
		for _, english := range []string{"Usage:", "Available Commands", "Flags:", "help for", "[flags]", "completion"} {
			if strings.Contains(r.stdout, english) {
				t.Errorf("watchoo %q : aide avec %q (anglais) :\n%s", args, english, r.stdout)
			}
		}
	}

	r := h.run("help", "init").mustSucceed(t)
	for _, want := range []string{"Fait du répertoire courant", "Exemples :", "Options globales :", "watchoo init [options]"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("aide de init sans %q :\n%s", want, r.stdout)
		}
	}
	if len(h.configs) != 0 {
		t.Error("l'aide a construit un accès à gh")
	}
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t)
	h.run("inti").mustFail(t, "commande inconnue « inti »", "vouliez-vous dire « init » ?")
	h.run("help", "inconnue").mustFail(t, "aide : commande inconnue « inconnue »")
	h.run("init", "mon-espace").mustFail(t, "« watchoo init » ne prend pas d'argument (reçu : mon-espace)")

	// Erreurs d'options : message entièrement en français (pflag les écrit en anglais), qui reprend
	// seulement ce que l'utilisateur a tapé, suivi de la commande d'aide.
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--inconnue"}, "erreur : option inconnue : --inconnue (aide : « watchoo --help »)\n"},
		{[]string{"init", "--inconnue"}, "erreur : option inconnue : --inconnue (aide : « watchoo init --help »)\n"},
		{[]string{"init", "--inconnue=1"}, "erreur : option inconnue : --inconnue (aide : « watchoo init --help »)\n"},
		{[]string{"init", "-Z"}, "erreur : option courte inconnue : -Z (aide : « watchoo init --help »)\n"},
		{[]string{"init", "-hZ"}, "erreur : option courte inconnue : -Z (aide : « watchoo init --help »)\n"},
		{[]string{"init", "-Zx"}, "erreur : option courte inconnue : -Z (aide : « watchoo init --help »)\n"},
		{[]string{"init", "-C"}, "erreur : l'option -C attend une valeur (aide : « watchoo init --help »)\n"},
		{[]string{"init", "--workspace"}, "erreur : l'option --workspace attend une valeur (aide : « watchoo init --help »)\n"},
		{[]string{"init", "--json=x"}, "erreur : valeur invalide \"x\" pour --json : true ou false attendu (aide : « watchoo init --help »)\n"},
		{[]string{"--json=oui", "version"}, "erreur : valeur invalide \"oui\" pour --json : true ou false attendu (aide : « watchoo version --help »)\n"},
		{[]string{"init", "---x"}, "erreur : option mal écrite : ---x (aide : « watchoo init --help »)\n"},
	} {
		r := h.run(tt.args...).mustFail(t)
		if r.stderr != tt.want {
			t.Errorf("watchoo %q : sortie d'erreur\n%q\nattendu\n%q", tt.args, r.stderr, tt.want)
		}
		for _, english := range []string{" in -", " for ", "flag", "invalid argument", "syntax", "strconv", "unknown", "needs"} {
			if strings.Contains(r.stderr, english) {
				t.Errorf("watchoo %q : message avec %q (anglais) : %s", tt.args, english, r.stderr)
			}
		}
	}

	assertNoWorkspace(t, h.cwd)
	if got := h.fake().StatusCalls(); got != 0 {
		t.Errorf("Status appelé %d fois sur une erreur d'usage", got)
	}
}

// La commande completion de cobra (aide en anglais, non documentée) est désactivée.
func TestCompletionDisabled(t *testing.T) {
	h := newHarness(t)
	h.run("completion").mustFail(t, "commande inconnue « completion »")
	h.run("completion", "bash").mustFail(t, "commande inconnue « completion »")
	h.run("help", "completion").mustFail(t, "aide : commande inconnue « completion »")
	r := h.run("completion", "--help").mustSucceed(t)
	if strings.Contains(r.stdout, "autocompletion") || !strings.Contains(r.stdout, "Utilisation :") {
		t.Errorf("completion --help devrait afficher l'aide de watchoo :\n%s", r.stdout)
	}
}

func TestTranslateFlagErrorUnknownType(t *testing.T) {
	err := errors.New("message sans type pflag")
	if got := translateFlagError(err); got != err.Error() {
		t.Errorf("translateFlagError = %q, attendu le message d'origine", got)
	}
}

func TestRunNilArgsShowsHelp(t *testing.T) {
	isolateEnv(t)
	var stdout, stderr bytes.Buffer
	// Sans arguments, cobra lirait os.Args (ceux du binaire de test) : Run doit l'éviter.
	code := Run(context.Background(), Deps{Stdout: &stdout, Stderr: &stderr}, nil)
	if code != 0 || !strings.Contains(stdout.String(), "Utilisation :") {
		t.Errorf("Run(nil) = %d, stdout :\n%s\nstderr :\n%s", code, stdout.String(), stderr.String())
	}
}

func TestDefaultDeps(t *testing.T) {
	deps := DefaultDeps()
	if deps.Stdout == nil || deps.Stderr == nil || deps.Getwd == nil || deps.NewAuthClient == nil {
		t.Fatalf("DefaultDeps incomplet : %+v", deps)
	}
	cfg := workspace.DefaultConfig("x")
	cfg.GitHub = workspace.GitHubConfig{User: "alice", GHPath: "/opt/gh"}
	client := deps.NewAuthClient(cfg)
	gh, ok := client.(auth.GH)
	if !ok {
		t.Fatalf("NewAuthClient = %T, attendu auth.GH", client)
	}
	if gh != (auth.GH{Path: "/opt/gh", User: "alice"}) {
		t.Errorf("auth.GH = %+v", gh)
	}

	// Les champs non renseignés prennent la valeur de production.
	if d := (Deps{}).withDefaults(); d.Stdout == nil || d.Stderr == nil || d.Getwd == nil || d.NewAuthClient == nil {
		t.Errorf("withDefaults incomplet : %+v", d)
	}
}

// TestTokenNeverInOutput lance toutes les commandes et tous les cas d'erreur de la phase 1, avec le
// faux client et avec le faux binaire gh, et vérifie que le faux token n'apparaît ni sur la sortie
// standard, ni sur la sortie d'erreur, ni dans les logs (niveau Debug compris).
func TestTokenNeverInOutput(t *testing.T) {
	logsBefore := len(testLogs.String())
	var outputs []string
	check := func(r result) {
		outputs = append(outputs, r.stdout, r.stderr)
	}

	// Faux client, avec ses différents états.
	for _, setup := range []func(h *harness){
		func(h *harness) {},
		func(h *harness) { h.Client = authtest.NewLoggedOut() },
		func(h *harness) {
			h.fake().SetStatus(auth.Status{}, fmt.Errorf("« gh auth status » : %w", auth.ErrTimeout))
		},
		func(h *harness) { h.fake().SetTokens(authtest.FakeToken, authtest.FakeToken+"_2") },
	} {
		for _, args := range [][]string{{"init"}, {"init", "--json"}, {"version"}, {"version", "--json"}, {"--help"}} {
			h := newHarness(t)
			setup(h)
			check(h.run(args...))
			// Second init (refus) et espace imbriqué.
			check(h.run(args...))
			h.cwd = newDir(t, h.cwd, "imbrique")
			check(h.run(args...))
		}
	}

	// Vrai auth.GH et faux binaire gh : y compris les scénarios où gh écrit le token sur sa sortie.
	for _, scenario := range []string{"logged-in", "multi-account", "env-token", "env-token-only",
		"not-logged-in", "exit-not-logged-in", "invalid-token", "offline", "token-rotation", "empty-output",
		"garbage", "error"} {
		for _, user := range []string{"", authtest.FakeLogin, "watchoo-other", "Watchoo-Other"} {
			h := newHarness(t)
			useFakeGH(t, h, scenario)
			t.Setenv("WATCHOO_GITHUB_USER", user)
			check(h.run("init", "--json"))
			check(h.run("init"))
			h.cwd = newDir(t, h.cwd, "imbrique")
			check(h.run("init"))
		}
	}

	// gh introuvable.
	h := newHarness(t)
	h.Client = nil
	t.Setenv("WATCHOO_GITHUB_GH_PATH", filepath.Join(t.TempDir(), "absent"))
	check(h.run("init"))

	for i, out := range outputs {
		if strings.Contains(out, authtest.FakeToken) {
			t.Errorf("sortie n°%d contient le faux token :\n%s", i, out)
		}
	}
	if logs := testLogs.String()[logsBefore:]; strings.Contains(logs, authtest.FakeToken) {
		t.Errorf("les logs contiennent le faux token :\n%s", logs)
	}
	if len(outputs) == 0 || len(testLogs.String()) == logsBefore {
		t.Error("aucune sortie ni aucun log vérifié")
	}
}
