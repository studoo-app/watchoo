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
	"unicode/utf8"

	"github.com/spf13/cobra"
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
		t.Errorf("version = %q (stderr %q), want %q", r.stdout, r.stderr, want)
	}

	Version = "v1.2.3" // value injected by -ldflags -X
	r = h.run("version", "--json").mustSucceed(t)
	dec := json.NewDecoder(strings.NewReader(r.stdout))
	dec.DisallowUnknownFields()
	var got versionReport
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, r.stdout)
	}
	if (got != versionReport{Version: "v1.2.3", Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH}) {
		t.Errorf("version JSON = %+v", got)
	}

	// version needs neither a workspace nor gh.
	if len(h.configs) != 0 {
		t.Errorf("version built a gh access")
	}
	h.run("version", "extra").mustFail(t, `"watchoo version" takes no argument`)
}

// TestHelpIsTranslated goes through the general help: the texts are those of the process language
// (English, set by TestMain), with no trace of French nor of cobra's English texts.
func TestHelpIsTranslated(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{nil, {"--help"}, {"help"}, {"-h"}} {
		r := h.run(args...).mustSucceed(t)
		for _, want := range []string{"Usage:", "Commands:", "init", "version", "--json",
			"-C, --workspace path", "show help for a command"} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("watchoo %q: help without %q:\n%s", args, want, r.stdout)
			}
		}
		for _, french := range []string{"Utilisation :", "Commandes :", "Options globales :",
			"affiche l'aide", "[flags]", "completion"} {
			if strings.Contains(r.stdout, french) {
				t.Errorf("watchoo %q: help with %q (French):\n%s", args, french, r.stdout)
			}
		}
	}

	r := h.run("help", "init").mustSucceed(t)
	for _, want := range []string{"Examples:", "Global options:", "watchoo init [options]"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("init help without %q:\n%s", want, r.stdout)
		}
	}
	if len(h.configs) != 0 {
		t.Error("the help built a gh access")
	}
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t)
	h.run("inti").mustFail(t, `unknown command "inti"`, `did you mean "init"?`)
	h.run("help", "inconnue").mustFail(t, `help: unknown command "inconnue"`)
	h.run("init", "mon-espace").mustFail(t, `"watchoo init" takes no argument (got: mon-espace)`)

	// Option errors: message entirely in the process language (pflag writes them in English), which
	// only repeats what the user typed, followed by the help command.
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"--inconnue"}, "error: unknown option: --inconnue (help: \"watchoo --help\")\n"},
		{[]string{"init", "--inconnue"}, "error: unknown option: --inconnue (help: \"watchoo init --help\")\n"},
		{[]string{"init", "--inconnue=1"}, "error: unknown option: --inconnue (help: \"watchoo init --help\")\n"},
		{[]string{"init", "-Z"}, "error: unknown short option: -Z (help: \"watchoo init --help\")\n"},
		{[]string{"init", "-hZ"}, "error: unknown short option: -Z (help: \"watchoo init --help\")\n"},
		{[]string{"init", "-Zx"}, "error: unknown short option: -Z (help: \"watchoo init --help\")\n"},
		{[]string{"init", "-C"}, "error: option -C requires a value (help: \"watchoo init --help\")\n"},
		{[]string{"init", "--workspace"}, "error: option --workspace requires a value (help: \"watchoo init --help\")\n"},
		{[]string{"init", "--json=x"}, "error: invalid value \"x\" for --json: expected true or false (help: \"watchoo init --help\")\n"},
		{[]string{"--json=oui", "version"}, "error: invalid value \"oui\" for --json: expected true or false (help: \"watchoo version --help\")\n"},
		{[]string{"init", "---x"}, "error: malformed option: ---x (help: \"watchoo init --help\")\n"},
	} {
		r := h.run(tt.args...).mustFail(t)
		if r.stderr != tt.want {
			t.Errorf("watchoo %q: stderr\n%q\nwant\n%q", tt.args, r.stderr, tt.want)
		}
		for _, english := range []string{" in -", "flag", "invalid argument", "syntax", "strconv", "needs"} {
			if strings.Contains(r.stderr, english) {
				t.Errorf("watchoo %q: message with %q (pflag's English): %s", tt.args, english, r.stderr)
			}
		}
	}

	assertNoWorkspace(t, h.cwd)
	if got := h.fake().StatusCalls(); got != 0 {
		t.Errorf("Status called %d times on a usage error", got)
	}
}

// cobra's completion command (help in English, undocumented) is disabled.
func TestCompletionDisabled(t *testing.T) {
	h := newHarness(t)
	h.run("completion").mustFail(t, `unknown command "completion"`)
	h.run("completion", "bash").mustFail(t, `unknown command "completion"`)
	h.run("help", "completion").mustFail(t, `help: unknown command "completion"`)
	r := h.run("completion", "--help").mustSucceed(t)
	if strings.Contains(r.stdout, "autocompletion") || !strings.Contains(r.stdout, "Usage:") {
		t.Errorf("completion --help should print the watchoo help:\n%s", r.stdout)
	}
}

func TestTranslateFlagErrorUnknownType(t *testing.T) {
	err := errors.New("message without a pflag type")
	if got := translateFlagError(err); got != err.Error() {
		t.Errorf("translateFlagError = %q, want the original message", got)
	}
}

func TestRunNilArgsShowsHelp(t *testing.T) {
	isolateEnv(t)
	var stdout, stderr bytes.Buffer
	// Without arguments, cobra would read os.Args (those of the test binary): Run must avoid it.
	code := Run(context.Background(), Deps{Stdout: &stdout, Stderr: &stderr}, nil)
	if code != 0 || !strings.Contains(stdout.String(), "Usage:") {
		t.Errorf("Run(nil) = %d, stdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
}

func TestDefaultDeps(t *testing.T) {
	deps := DefaultDeps()
	if deps.Stdout == nil || deps.Stderr == nil || deps.Getwd == nil || deps.NewAuthClient == nil {
		t.Fatalf("incomplete DefaultDeps: %+v", deps)
	}
	cfg := workspace.DefaultConfig("x")
	cfg.GitHub = workspace.GitHubConfig{User: "alice", GHPath: "/opt/gh"}
	client := deps.NewAuthClient(cfg)
	gh, ok := client.(auth.GH)
	if !ok {
		t.Fatalf("NewAuthClient = %T, want auth.GH", client)
	}
	if gh != (auth.GH{Path: "/opt/gh", User: "alice"}) {
		t.Errorf("auth.GH = %+v", gh)
	}

	// The fields left unset take the production value.
	if d := (Deps{}).withDefaults(); d.Stdout == nil || d.Stderr == nil || d.Getwd == nil || d.NewAuthClient == nil {
		t.Errorf("incomplete withDefaults: %+v", d)
	}
}

// TestTokenNeverInOutput runs all the commands and all the error cases of phase 1, with the fake client
// and with the fake gh binary, and checks that the fake token appears neither on the standard output,
// nor on the error output, nor in the logs (Debug level included).
func TestTokenNeverInOutput(t *testing.T) {
	logsBefore := len(testLogs.String())
	var outputs []string
	check := func(r result) {
		outputs = append(outputs, r.stdout, r.stderr)
	}

	// Fake client, with its different states.
	for _, setup := range []func(h *harness){
		func(h *harness) {},
		func(h *harness) { h.Client = authtest.NewLoggedOut() },
		func(h *harness) {
			h.fake().SetStatus(auth.Status{}, fmt.Errorf("\"gh auth status\": %w", auth.ErrTimeout))
		},
		func(h *harness) { h.fake().SetTokens(authtest.FakeToken, authtest.FakeToken+"_2") },
	} {
		for _, args := range [][]string{{"init"}, {"init", "--json"}, {"version"}, {"version", "--json"}, {"--help"}} {
			h := newHarness(t)
			setup(h)
			check(h.run(args...))
			// Second init (refusal) and nested workspace.
			check(h.run(args...))
			h.cwd = newDir(t, h.cwd, "imbrique")
			check(h.run(args...))
		}
	}

	// Real auth.GH and fake gh binary: including the scenarios where gh writes the token on its output.
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

	// On a terminal: header card and status lines (gh account, paths).
	for _, scenario := range []string{"logged-in", "env-token", "token-rotation"} {
		h := newHarness(t)
		useFakeGH(t, h, scenario)
		onTerminal(h, wideTerminal)
		check(h.run("init"))
		check(h.run("--help"))
		check(h.run("version"))
	}

	// gh not found.
	h := newHarness(t)
	h.Client = nil
	t.Setenv("WATCHOO_GITHUB_GH_PATH", filepath.Join(t.TempDir(), "absent"))
	check(h.run("init"))

	for i, out := range outputs {
		if strings.Contains(out, authtest.FakeToken) {
			t.Errorf("output #%d contains the fake token:\n%s", i, out)
		}
	}
	if logs := testLogs.String()[logsBefore:]; strings.Contains(logs, authtest.FakeToken) {
		t.Errorf("the logs contain the fake token:\n%s", logs)
	}
	if len(outputs) == 0 || len(testLogs.String()) == logsBefore {
		t.Error("no output and no log checked")
	}
}

// TestHelpOptionsAligned goes through the help of all the commands: no trace of the French text of the
// default value, and, in each block of options, the descriptions start at the same column (counted in
// characters, accents included: "--repo dépôt" next to "--author auteur" in French).
func TestHelpOptionsAligned(t *testing.T) {
	h := newHarness(t)
	var visit func(c *cobra.Command)
	var paths [][]string
	visit = func(c *cobra.Command) {
		if !c.IsAvailableCommand() && c.HasParent() {
			return
		}
		parts := strings.Fields(c.CommandPath())[1:]
		paths = append(paths, parts)
		for _, sub := range c.Commands() {
			visit(sub)
		}
	}
	visit(NewRootCmd(Deps{}))
	if len(paths) < 10 {
		t.Fatalf("only %d commands found: %q", len(paths), paths)
	}
	for _, p := range paths {
		r := h.run(append(p, "--help")...).mustSucceed(t)
		for _, block := range optionBlocks(r.stdout) {
			col := -1
			for _, line := range block {
				name := strings.TrimLeft(line, " ")
				k := strings.Index(name, "   ")
				if k < 0 {
					t.Errorf("watchoo %q --help: option line without a description: %q", p, line)
					continue
				}
				desc := strings.TrimLeft(name[k:], " ")
				c := utf8.RuneCountInString(line) - utf8.RuneCountInString(desc)
				if col == -1 {
					col = c
				} else if c != col {
					t.Errorf("watchoo %q --help: description at column %d instead of %d: %q", p, c, col, line)
				}
			}
		}
	}

	r := h.run("log", "--help").mustSucceed(t)
	if !strings.Contains(r.stdout, "(default 20)") {
		t.Errorf("log help without the default value:\n%s", r.stdout)
	}
}

// optionBlocks returns the lines of the "Options" and "Global options" blocks of a help.
func optionBlocks(help string) [][]string {
	var blocks [][]string
	var cur []string
	in := false
	for _, line := range strings.Split(help, "\n") {
		switch {
		case line == "Options:" || line == "Global options:":
			in, cur = true, nil
		case in && strings.HasPrefix(line, "  "):
			cur = append(cur, line)
		case in:
			blocks, in = append(blocks, cur), false
		}
	}
	if in {
		blocks = append(blocks, cur)
	}
	return blocks
}
