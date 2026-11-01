package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/logo"
)

// wideTerminal is a simulated 80-column terminal, with Unicode and color.
var wideTerminal = TermInfo{IsTerminal: true, Width: 80, Unicode: true, Color: true}

// ansiSeq matches an ANSI color or attribute escape sequence.
var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// onTerminal makes the harness's standard output pass for the terminal described by info, and returns a
// counter of the calls to Terminal.
func onTerminal(h *harness, info TermInfo) *int {
	calls := new(int)
	h.Terminal = func() TermInfo {
		*calls++
		return info
	}
	return calls
}

// cardFor is the Watchoo header card expected on the terminal info.
func cardFor(info TermInfo, subtitle, tip string) string {
	return logo.Card(logo.CardOptions{Title: "Watchoo", Version: Version, Subtitle: subtitle, Tip: tip,
		Width: info.Width, Unicode: info.Unicode, Color: info.Color})
}

// statusFor is the status line expected on the terminal info.
func statusFor(info TermInfo, text string) string {
	return logo.Status(text, logo.CardOptions{Unicode: info.Unicode, Color: info.Color})
}

// helpHeader is the header expected at the top of the general help on the terminal info: the card, a blank
// line, the workspace status line, a blank line.
func helpHeader(info TermInfo, status, tip string) string {
	return cardFor(info, helpSubtitle(), tip) + "\n" + statusFor(info, status) + "\n"
}

// versionLine is the output of "watchoo version" off a terminal.
func versionLine() string {
	return "watchoo " + Version + " (" + runtime.Version() + ", " + runtime.GOOS + "/" + runtime.GOARCH + ")\n"
}

// versionCard is the output of "watchoo version" on the terminal info.
func versionCard(info TermInfo) string {
	return cardFor(info, runtime.Version()+" · "+runtime.GOOS+"/"+runtime.GOARCH, "")
}

// assertNoCard checks that out contains no card, no status line and no escape sequence.
func assertNoCard(t *testing.T, what, out string) {
	t.Helper()
	for _, part := range []string{"╭", "█", "●", "+--", `|\    /|`, "Tip", "\x1b"} {
		if strings.Contains(out, part) {
			t.Errorf("%s: unexpected card (%q):\n%s", what, part, out)
			return
		}
	}
}

// generalHelp lists the ways to display the general help.
var generalHelp = [][]string{nil, {"help"}, {"--help"}, {"-h"}}

// On a terminal, the general help starts with the card (tip for "watchoo init" if there is no
// workspace) and the status line, then the usual help, unchanged.
func TestCardInGeneralHelp(t *testing.T) {
	plain := newHarness(t)
	h := newHarness(t)
	onTerminal(h, wideTerminal)
	header := helpHeader(wideTerminal, "No workspace here", tipInit())
	for _, part := range []string{"Watchoo", "dev", "Who worked, on what, and when?", "watchoo init", "╭", "╯"} {
		if !strings.Contains(ansiSeq.ReplaceAllString(header, ""), part) {
			t.Fatalf("header without %q:\n%s", part, header)
		}
	}
	for _, args := range generalHelp {
		want := plain.run(args...).mustSucceed(t).stdout
		r := h.run(args...).mustSucceed(t)
		if r.stdout != header+want {
			t.Errorf("watchoo %q on a terminal:\n%s\nwant the card, the status line, then:\n%s", args, r.stdout, want)
		}
		if r.stderr != "" {
			t.Errorf("watchoo %q: unexpected stderr:\n%s", args, r.stderr)
		}
	}
	// The help does not run gh, not even to look for the workspace.
	if len(h.configs) != 0 || h.fake().StatusCalls() != 0 {
		t.Error("the help built a gh access")
	}
}

// The help's status line describes the workspace found as the other commands would.
func TestHelpWorkspaceStatus(t *testing.T) {
	isolateEnv(t)
	home := os.Getenv("HOME")
	root := newDir(t, home, "supervision", "licence-projet")
	(&harness{t: t, cwd: root, Client: authtest.NewLoggedIn("")}).run("init").mustSucceed(t)
	other := newDir(t, t.TempDir(), "master-projet")
	(&harness{t: t, cwd: other, Client: authtest.NewLoggedIn("")}).run("init").mustSucceed(t)
	empty := t.TempDir()
	requireNoWorkspaceAbove(t, empty)

	inHome := "Workspace: licence-projet (" + filepath.Join("~", "supervision", "licence-projet") + ")"
	inOther := "Workspace: master-projet (" + other + ")"
	for _, tt := range []struct {
		name string
		cwd  string
		args []string
		env  string // WATCHOO_DIR
		want string
		tip  string
	}{
		{"workspace of the current directory, under the home directory", root, nil, "", inHome, tipWorkspace()},
		{"from a subdirectory", newDir(t, root, "src", "module"), []string{"help"}, "", inHome, tipWorkspace()},
		{"outside a workspace", empty, []string{"--help"}, "", "No workspace here", tipInit()},
		{"-C option", empty, []string{"-C", other, "--help"}, "", inOther, tipWorkspace()},
		{"relative -C option", root, []string{"-C", "src", "help"}, "", inHome, tipWorkspace()},
		{"WATCHOO_DIR variable", empty, nil, other, inOther, tipWorkspace()},
		{"-C option without a workspace", root, []string{"-C", empty}, "", "No workspace here", tipInit()},
		{"nonexistent -C directory", root, []string{"-C", "absent", "-h"}, "",
			"Workspace: option -C: directory " + filepath.Join(root, "absent") + " does not exist", tipHelp()},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.env != "" {
				t.Setenv("WATCHOO_DIR", tt.env)
			}
			h := &harness{t: t, cwd: tt.cwd, Client: authtest.NewLoggedIn("")}
			onTerminal(h, wideTerminal)
			r := h.run(tt.args...).mustSucceed(t)
			if want := helpHeader(wideTerminal, tt.want, tt.tip); !strings.HasPrefix(r.stdout, want) {
				t.Errorf("help:\n%s\nwant the header:\n%s", r.stdout, want)
			}
			if len(h.configs) != 0 {
				t.Error("the help built a gh access")
			}
		})
	}
}

// Invalid settings, a workspace name with control characters or an inaccessible current directory
// never make the help fail.
func TestHelpWorkspaceStatusErrors(t *testing.T) {
	h := newHarness(t)
	h.run("init").mustSucceed(t)
	config := filepath.Join(h.cwd, ".watchoo", "config.yaml")
	onTerminal(h, wideTerminal)
	plainHelp := newHarness(t).run("--help").mustSucceed(t).stdout

	// statusOf returns the help's status line, without color, and checks that the usual help follows.
	statusOf := func(r result) string {
		t.Helper()
		_, rest, ok := strings.Cut(ansiSeq.ReplaceAllString(r.stdout, ""), "╯\n\n") // after the card
		if !ok {
			t.Fatalf("help without a card:\n%s", r.stdout)
		}
		line, help, _ := strings.Cut(rest, "\n\n")
		if help != plainHelp {
			t.Errorf("usual help missing or changed:\n%s", r.stdout)
		}
		return line
	}

	writeFile(t, config, "sync:\n  intervall: 1h\n")
	r := h.run("--help").mustSucceed(t)
	if got := statusOf(r); !strings.HasPrefix(got, "● Workspace: ") || !strings.Contains(got, "intervall") {
		t.Errorf(`invalid settings: status line %q`, got)
	}
	if r.stderr != "" {
		t.Errorf("unexpected stderr:\n%s", r.stderr)
	}

	writeFile(t, config, "sync:\n  interval: 5m\n")
	if got := statusOf(h.run("help").mustSucceed(t)); !strings.HasPrefix(got, "● Workspace: invalid settings: ") {
		t.Errorf("value out of bounds: status line %q", got)
	}

	// Name with an escape sequence and a line break (YAML "\e", "\n"): neutralized.
	writeFile(t, config, `name: "promo\e[2J\nlicence"`+"\n")
	r = h.run().mustSucceed(t)
	if got := statusOf(r); got != "● Workspace: promo?[2J licence ("+h.cwd+")" {
		t.Errorf("name with control characters: status line %q", got)
	}
	if rest := ansiSeq.ReplaceAllString(r.stdout, ""); strings.Contains(rest, "\x1b") {
		t.Errorf("escape sequence from the workspace name:\n%q", r.stdout)
	}

	// Inaccessible current directory: reported, except with an absolute -C.
	deps := Deps{Getwd: func() (string, error) { return "", errors.New("directory deleted") }}
	status, tip := helpStatus(deps, &options{})
	if status != "Workspace: current directory unavailable: directory deleted" || tip != tipHelp() {
		t.Errorf("inaccessible current directory: %q, %q", status, tip)
	}
	writeFile(t, config, "name: perso\n")
	if status, _ := helpStatus(deps, &options{workspaceDir: h.cwd}); status != "Workspace: perso ("+h.cwd+")" {
		t.Errorf("inaccessible current directory, absolute -C: %q", status)
	}
}

// On a terminal, "watchoo version" and "watchoo --version" print the card and nothing else.
func TestCardVersion(t *testing.T) {
	original := Version
	t.Cleanup(func() { Version = original })

	h := newHarness(t)
	onTerminal(h, wideTerminal)
	for _, tt := range []struct{ version, shown string }{{"dev", "Watchoo dev"}, {"0.1.0", "Watchoo v0.1.0"},
		{"v1.2.3", "Watchoo v1.2.3"}} {
		Version = tt.version
		want := versionCard(wideTerminal)
		plain := ansiSeq.ReplaceAllString(want, "")
		if !strings.Contains(plain, tt.shown) || !strings.Contains(plain, runtime.Version()+" · "+runtime.GOOS) ||
			strings.Contains(plain, "Tip:") {
			t.Fatalf("unexpected version card:\n%s", plain)
		}
		for _, args := range [][]string{{"version"}, {"--version"}, {"-C", "ailleurs", "--version"}} {
			r := h.run(args...).mustSucceed(t)
			if r.stdout != want || r.stderr != "" {
				t.Errorf("watchoo %q on a terminal = %q (stderr %q), want the card:\n%s", args, r.stdout,
					r.stderr, want)
			}
		}
	}
	if len(h.configs) != 0 {
		t.Error("version built a gh access")
	}
}

// On a terminal, the report of "watchoo init" is the card, then status lines.
func TestCardInit(t *testing.T) {
	h := newHarness(t)
	root := h.cwd
	onTerminal(h, wideTerminal)
	r := h.run("init").mustSucceed(t)
	want := cardFor(wideTerminal, `Workspace "licence-projet" created`, tipInitDone()) + "\n" +
		statusFor(wideTerminal, "Workspace: licence-projet ("+root+")") +
		statusFor(wideTerminal, "Settings: "+filepath.Join(root, ".watchoo", "config.yaml")) +
		statusFor(wideTerminal, "Database: "+filepath.Join(root, ".watchoo", "watchoo.db")) +
		statusFor(wideTerminal, "gh account: watchoo-test on github.com (scopes: gist, read:org, repo)")
	if r.stdout != want {
		t.Errorf("init on a terminal:\n%s\nwant:\n%s", r.stdout, want)
	}
	if !strings.Contains(ansiSeq.ReplaceAllString(r.stdout, ""), tipInitDone()) {
		t.Errorf("tip missing:\n%s", r.stdout)
	}
	if r.stderr != "" {
		t.Errorf("unexpected stderr:\n%s", r.stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".watchoo", "watchoo.db")); err != nil {
		t.Errorf("database missing: %v", err)
	}

	// Under the home directory: "~" in the paths; no scopes reported by gh.
	h2 := newHarness(t)
	h2.cwd = newDir(t, os.Getenv("HOME"), "perso")
	h2.fake().SetStatus(auth.Status{Host: "github.com", Login: "ci-bot", TokenSource: "GH_TOKEN", Active: true,
		State: "success"}, nil)
	info := TermInfo{IsTerminal: true, Width: 80, Unicode: true}
	onTerminal(h2, info)
	r = h2.run("init").mustSucceed(t)
	_, report, _ := strings.Cut(r.stdout, "╯\n\n")
	wantReport := "● Workspace: perso (" + filepath.Join("~", "perso") + ")\n" +
		"● Settings: " + filepath.Join("~", "perso", ".watchoo", "config.yaml") + "\n" +
		"● Database: " + filepath.Join("~", "perso", ".watchoo", "watchoo.db") + "\n" +
		"● gh account: ci-bot on github.com (scopes not reported by gh)\n"
	if report != wantReport {
		t.Errorf("report:\n%s\nwant:\n%s", report, wantReport)
	}
}

// A workspace whose name has decomposed accents ("e" followed by U+0301), like a directory created by the
// macOS Finder, does not shift the right border of the "watchoo init" card; without Unicode, its accents
// are removed, not replaced by "?", in the card and the status lines of init and of the help.
func TestCardDecomposedWorkspaceName(t *testing.T) {
	const name = "promo-re\u0301seaux"
	for _, unicodeOK := range []bool{true, false} {
		h := newHarness(t)
		h.cwd = newDir(t, filepath.Dir(h.cwd), name)
		onTerminal(h, TermInfo{IsTerminal: true, Width: 80, Unicode: unicodeOK, Color: true})
		initOut := ansiSeq.ReplaceAllString(h.run("init").mustSucceed(t).stdout, "")
		helpOut := ansiSeq.ReplaceAllString(h.run("--help").mustSucceed(t).stdout, "")

		card, report, ok := strings.Cut(initOut, "\n\n")
		if !ok {
			t.Fatalf("Unicode %t: init without a card:\n%s", unicodeOK, initOut)
		}
		lines := strings.Split(card, "\n")
		for i, line := range lines {
			if got, want := columns(line), columns(lines[0]); got != want {
				t.Errorf("Unicode %t: card line %d is %d columns wide, want %d like the first one:\n%s",
					unicodeOK, i+1, got, want, card)
			}
		}
		shown, status := `"`+name+`"`, "● Workspace: "+name+" ("+h.cwd+")"
		if !unicodeOK {
			// The whole status line is reduced to ASCII, path included.
			shown = `"promo-reseaux"`
			status = "* Workspace: promo-reseaux (" + strings.ReplaceAll(h.cwd, name, "promo-reseaux") + ")"
		}
		if !strings.Contains(card, shown) {
			t.Errorf("Unicode %t: %s missing from the card:\n%s", unicodeOK, shown, card)
		}
		if !strings.HasPrefix(report, status+"\n") {
			t.Errorf("Unicode %t: report\n%s\nwant %q at the top", unicodeOK, report, status)
		}
		header, _, ok := strings.Cut(helpOut, "\n\n"+status+"\n\n")
		if !ok {
			t.Errorf("Unicode %t: help without the status line %q:\n%s", unicodeOK, status, helpOut)
		}
		if !unicodeOK && (!isASCII(initOut) || !isASCII(header) || strings.Contains(initOut, "re?s")) {
			t.Errorf("without Unicode: accents badly removed:\n%s\n%s", initOut, header)
		}
	}
}

// columns returns the displayed width of a line without ANSI sequences: its number of characters, without
// the combining marks (Mn, Me), which the terminal draws in the column of the previous character.
func columns(line string) int {
	n := 0
	for _, r := range line {
		if !unicode.In(r, unicode.Mn, unicode.Me) {
			n++
		}
	}
	return n
}

// Off a terminal (redirected output, pipe), no output has a card: they stay identical.
func TestNoCardOffTerminal(t *testing.T) {
	h := newHarness(t) // default Terminal: DetectTerminal of the output buffer
	plainHelp := h.run("--help").mustSucceed(t).stdout
	calls := 0
	for _, info := range []func() TermInfo{nil, func() TermInfo {
		calls++
		return TermInfo{Width: 80, Unicode: true, Color: true} // not a terminal
	}} {
		h.Terminal = info
		for _, args := range generalHelp {
			if r := h.run(args...).mustSucceed(t); r.stdout != plainHelp {
				t.Errorf("watchoo %q off a terminal:\n%s", args, r.stdout)
			}
		}
		for _, args := range [][]string{{"version"}, {"--version"}} {
			if r := h.run(args...).mustSucceed(t); r.stdout != versionLine() || r.stderr != "" {
				t.Errorf("watchoo %q off a terminal = %q (stderr %q), want %q", args, r.stdout, r.stderr,
					versionLine())
			}
		}
	}
	if calls == 0 {
		t.Error("Terminal was not consulted")
	}
	assertNoCard(t, "help", plainHelp)

	h2 := newHarness(t)
	h2.Terminal = func() TermInfo { return TermInfo{} }
	r := h2.run("init").mustSucceed(t)
	if !strings.HasPrefix(r.stdout, `Workspace "licence-projet" created.`+"\n  directory   "+h2.cwd+"\n") {
		t.Errorf("init off a terminal: unexpected output:\n%s", r.stdout)
	}
	assertNoCard(t, "init", r.stdout)
}

// With --json, never a card, even on a terminal: the output stays pure JSON. The terminal is not even
// consulted.
func TestNoCardWithJSON(t *testing.T) {
	h := newHarness(t)
	plainHelp := newHarness(t).run("--help").mustSucceed(t).stdout
	calls := onTerminal(h, wideTerminal)
	for _, args := range [][]string{{"version", "--json"}, {"--json", "version"}, {"--version", "--json"},
		{"--json", "--version"}} {
		r := h.run(args...).mustSucceed(t)
		var got versionReport
		if err := json.Unmarshal([]byte(r.stdout), &got); err != nil || got.Version != Version {
			t.Errorf("watchoo %q: invalid JSON output (%v):\n%s", args, err, r.stdout)
		}
	}
	for _, args := range [][]string{{"--json"}, {"--help", "--json"}, {"help", "--json"}} {
		if r := h.run(args...).mustSucceed(t); r.stdout != plainHelp {
			t.Errorf("watchoo %q: want the help without a header:\n%s", args, r.stdout)
		}
	}
	r := h.run("init", "--json").mustSucceed(t)
	if report := decodeInitReport(t, r.stdout); report.Name != "licence-projet" {
		t.Errorf("init --json: unexpected report: %+v", report)
	}
	if *calls != 0 {
		t.Errorf("Terminal consulted %d times with --json", *calls)
	}
}

// The error output (errors, warnings) never has a card, and a failing command prints nothing on the
// standard output, even on a terminal.
func TestNoCardOnStderr(t *testing.T) {
	h := newHarness(t)
	onTerminal(h, wideTerminal)
	for _, args := range [][]string{{"version", "extra"}, {"--version", "extra"}, {"inti"}, {"--inconnue"},
		{"help", "inconnue"}} {
		r := h.run(args...).mustFail(t)
		assertNoCard(t, strings.Join(args, " ")+" (stderr)", r.stderr)
	}

	// Warning for a nested workspace: on the error output, without a card; the card stays at the top of
	// the report, on the standard output.
	parent := h.cwd
	h.run("init").mustSucceed(t)
	h.cwd = newDir(t, parent, "groupe-a")
	r := h.run("init").mustSucceed(t)
	if !strings.HasPrefix(r.stderr, "warning: ") {
		t.Errorf("want a warning on stderr:\n%s", r.stderr)
	}
	assertNoCard(t, "nested init (stderr)", r.stderr)
	if want := cardFor(wideTerminal, `Workspace "groupe-a" created`, tipInitDone()); !strings.HasPrefix(r.stdout, want) {
		t.Errorf("nested init: card missing from the report:\n%s", r.stdout)
	}

	// gh not signed in and git missing: error only.
	h2 := newHarness(t)
	h2.Client = authtest.NewLoggedOut()
	h2.GitMissing = true
	onTerminal(h2, wideTerminal)
	r = h2.run("init").mustFail(t, "gh auth login")
	assertNoCard(t, "init without gh signed in (stderr)", r.stderr)
}

// A subcommand's help has no card, even on a terminal.
func TestNoCardInSubcommandHelp(t *testing.T) {
	plain := newHarness(t)
	h := newHarness(t)
	onTerminal(h, wideTerminal)
	for _, args := range [][]string{{"help", "init"}, {"init", "--help"}, {"init", "-h"}, {"version", "--help"},
		{"help", "version"}, {"help", "help"}} {
		want := plain.run(args...).mustSucceed(t).stdout
		r := h.run(args...).mustSucceed(t)
		if r.stdout != want {
			t.Errorf("watchoo %q on a terminal:\n%s\nwant, as off a terminal:\n%s", args, r.stdout, want)
		}
		assertNoCard(t, strings.Join(args, " "), r.stdout)
	}
}

// The card adapts to the terminal described by the environment: width (COLUMNS), locale (Unicode or pure
// ASCII), NO_COLOR and TERM=dumb (no color). The terminal is simulated, with an unknown system width.
func TestCardAdaptsToTerminal(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]string
		want TermInfo // IsTerminal false: no card (terminal too narrow)
	}{
		{"UTF-8 locale, unknown width", map[string]string{"LANG": "fr_FR.UTF-8", "TERM": "xterm-256color"},
			TermInfo{IsTerminal: true, Unicode: true, Color: true}},
		{"50 columns", map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "50"},
			TermInfo{IsTerminal: true, Width: 50, Unicode: true, Color: true}},
		{"40 columns", map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "40"},
			TermInfo{IsTerminal: true, Width: 40, Unicode: true, Color: true}},
		{"30 columns", map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "30"}, TermInfo{}},
		{"C locale", map[string]string{"LANG": "C", "LC_ALL": "C"}, TermInfo{IsTerminal: true, Color: true}},
		{"C locale, 50 columns", map[string]string{"LC_ALL": "C", "COLUMNS": "50"},
			TermInfo{IsTerminal: true, Width: 50, Color: true}},
		{"non-UTF-8 locale", map[string]string{"LANG": "fr_FR.ISO-8859-1"}, TermInfo{IsTerminal: true, Color: true}},
		{"Windows Terminal", map[string]string{"WT_SESSION": "c2a3f0e4"},
			TermInfo{IsTerminal: true, Unicode: true, Color: true}},
		{"NO_COLOR", map[string]string{"LANG": "fr_FR.UTF-8", "NO_COLOR": "1"},
			TermInfo{IsTerminal: true, Unicode: true}},
		{"TERM=dumb", map[string]string{"LANG": "fr_FR.UTF-8", "TERM": "dumb"}, TermInfo{IsTerminal: true, Unicode: true}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			plainHelp := newHarness(t).run("--help").mustSucceed(t).stdout
			setTerminalEnv(t, tt.env)
			h.Terminal = func() TermInfo { return describeTerminal(os.LookupEnv, func() int { return 0 }) }

			version := h.run("version").mustSucceed(t).stdout
			help := h.run("--help").mustSucceed(t).stdout
			initOut := h.run("init").mustSucceed(t).stdout
			if !tt.want.IsTerminal { // too narrow for the card: usual outputs
				if version != versionLine() || help != plainHelp ||
					!strings.HasPrefix(initOut, `Workspace "licence-projet" created.`+"\n  directory   "+h.cwd+"\n") {
					t.Errorf("terminal too narrow: outputs changed:\n%s\n%s\n%s", version, help, initOut)
				}
				assertNoCard(t, "init", initOut)
				return
			}
			info := tt.want
			if want := versionCard(info); version != want {
				t.Errorf("version:\n%s\nwant:\n%s", version, want)
			}
			header := helpHeader(info, "No workspace here", tipInit())
			if help != header+plainHelp {
				t.Errorf("help:\n%s\nwant the header:\n%s", help, header)
			}
			limit := tt.want.Width
			if limit == 0 {
				limit = 80
			}
			for _, out := range []string{version, header, initOut} {
				// The card lines fit in the terminal (not necessarily the status lines, which quote
				// paths).
				for line := range strings.Lines(ansiSeq.ReplaceAllString(out, "")) {
					line = strings.TrimSuffix(line, "\n")
					if n := utf8.RuneCountInString(line); n > limit && !strings.HasPrefix(line, "● ") &&
						!strings.HasPrefix(line, "* ") {
						t.Errorf("line of %d columns on a %d-column terminal: %q", n, limit, line)
					}
				}
				if !info.Unicode && !isASCII(out) {
					t.Errorf("output not pure ASCII without Unicode:\n%s", out)
				}
				if !info.Color && strings.Contains(out, "\x1b") {
					t.Errorf("escape sequence without color:\n%q", out)
				}
				if info.Color && !strings.Contains(out, "\x1b[") {
					t.Errorf("output without color:\n%s", out)
				}
			}
			if !info.Unicode && (!strings.Contains(version, "+--") || !strings.Contains(ansiSeq.ReplaceAllString(version, ""), "| o##o |")) {
				t.Errorf("version without the pure ASCII card:\n%s", version)
			}
		})
	}
}

// isASCII reports whether s contains only ASCII characters.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func TestVersionFlag(t *testing.T) {
	h := newHarness(t)
	version := h.run("version").mustSucceed(t)
	for _, args := range [][]string{{"--version"}, {"-C", "ailleurs", "--version"}} {
		if r := h.run(args...).mustSucceed(t); r != version {
			t.Errorf("watchoo %q = %+v, want the same as \"watchoo version\": %+v", args, r, version)
		}
	}
	versionJSON := h.run("version", "--json").mustSucceed(t)
	if r := h.run("--version", "--json").mustSucceed(t); r != versionJSON {
		t.Errorf("watchoo --version --json = %+v, want %+v", r, versionJSON)
	}

	h.run("--version", "extra").mustFail(t, `"watchoo --version" takes no argument (got: extra)`)
	// Option of the root command only, without a short letter.
	h.run("init", "--version").mustFail(t, `unknown option: --version (help: "watchoo init --help")`)
	h.run("-v").mustFail(t, "unknown short option: -v")
	assertNoWorkspace(t, h.cwd)
	if len(h.configs) != 0 {
		t.Error("--version built a gh access")
	}

	// Documented in the general help; absent from the subcommands' help.
	if r := h.run("--help").mustSucceed(t); !strings.Contains(r.stdout, "--version") ||
		!strings.Contains(r.stdout, "print the Watchoo version") {
		t.Errorf("general help without --version:\n%s", r.stdout)
	}
	if r := h.run("help", "init").mustSucceed(t); strings.Contains(r.stdout, "--version") {
		t.Errorf("init help with --version:\n%s", r.stdout)
	}
}
