package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func TestRenderTable(t *testing.T) {
	headers := []string{"Dépôt", "État"}
	rows := [][]string{{"alice/projet", "-"}, {"Élodie", "⚠ 42 d"}, {"x", ""}}
	want := "DÉPÔT         ÉTAT\n" +
		"alice/projet  -\n" +
		"Élodie        ⚠ 42 d\n" +
		"x\n"
	if got := renderTable(textStyle{}, headers, rows, nil); got != want {
		t.Errorf("renderTable =\n%q\nwant:\n%q", got, want)
	}
	// Decomposed accent (NFD): a single column.
	if got := renderTable(textStyle{}, []string{"a", "b"}, [][]string{{"éte", "1"}, {"abcd", "2"}}, nil); got !=
		"A     B\néte   1\nabcd  2\n" {
		t.Errorf("renderTable NFD =\n%q", got)
	}

	// Colors: headers in bold, gray lines in gray; without TermInfo.Color, no sequence.
	color := textStyle{term: TermInfo{IsTerminal: true, Color: true, Unicode: true}}
	got := renderTable(color, headers, rows, func(row int) bool { return row == 1 })
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[0], "\x1b[1m") || strings.Contains(lines[1], "\x1b[") || !strings.Contains(lines[2], "\x1b[") {
		t.Errorf("renderTable in color:\n%q", got)
	}
	if plain := ansiSeq.ReplaceAllString(got, ""); plain != want {
		t.Errorf("renderTable in color, without sequences =\n%q\nwant:\n%q", plain, want)
	}
	noColor := textStyle{term: TermInfo{IsTerminal: true, Unicode: true}}
	if got := renderTable(noColor, headers, rows, func(int) bool { return true }); got != want {
		t.Errorf("renderTable without color =\n%q", got)
	}
}

func TestTextStyle(t *testing.T) {
	redirected := textStyle{}
	ascii := textStyle{term: TermInfo{IsTerminal: true}}
	if !redirected.unicode() || ascii.unicode() || redirected.color() {
		t.Error("unicode/color: a redirected output is UTF-8 without color; an ASCII terminal without Unicode")
	}
	if got := redirected.truncate("Un message bien trop long\nsuite", 10); got != "Un message…" && got != "Un messag…" {
		t.Errorf("truncate = %q", got)
	}
	if got := redirected.truncate("court\nsuite", 10); got != "court" {
		t.Errorf("truncate short = %q", got)
	}
	if got := ascii.truncate("abcdefghijkl", 8); got != "abcde..." {
		t.Errorf("truncate ASCII = %q", got)
	}
	if got := redirected.truncate("abcdefghijkl", 8); got != "abcdefg…" {
		t.Errorf("truncate Unicode = %q", got)
	}
}

func TestConfirm(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  bool
	}{
		{"o\n", true}, {"OUI\n", true}, {" y \r\n", true}, {"yes", true},
		{"n\n", false}, {"\n", false}, {"", false}, {"non\n", false}, {"oui oui\n", false},
	} {
		var stderr bytes.Buffer
		input := tt.input
		if strings.HasSuffix(input, "\n") {
			input += "rest" // must not be read
		}
		in := strings.NewReader(input)
		deps := Deps{Stderr: &stderr, Stdin: in, StdinIsTerminal: func() bool { return true }}
		got, err := confirm(context.Background(), deps, "Continue?")
		if err != nil || got != tt.want {
			t.Errorf("confirm(%q) = %v, %v; want %v", tt.input, got, err, tt.want)
		}
		if !strings.HasPrefix(stderr.String(), "Continue? [y/N] ") {
			t.Errorf("question = %q", stderr.String())
		}
		if strings.HasSuffix(tt.input, "\n") {
			if rest, _ := io.ReadAll(in); string(rest) != "rest" {
				t.Errorf("confirm(%q) read beyond the line: rest %q", tt.input, rest)
			}
		}
	}

	// Off a terminal: nothing is read or asked, --yes is required.
	var stderr bytes.Buffer
	in := strings.NewReader("o\n")
	_, err := confirm(context.Background(), Deps{Stderr: &stderr, Stdin: in, StdinIsTerminal: func() bool { return false }}, "Continue?")
	if !errors.Is(err, errNeedYes) || stderr.Len() != 0 || in.Len() != 2 {
		t.Errorf("confirm off a terminal = %v (stderr %q, %d bytes left)", err, stderr.String(), in.Len())
	}

	// Read error.
	_, err = confirm(context.Background(), Deps{Stderr: io.Discard, Stdin: iotest.ErrReader(errors.New("broken")),
		StdinIsTerminal: func() bool { return true }}, "?")
	if err == nil || !strings.Contains(err.Error(), "reading the answer: broken") {
		t.Errorf("confirm on a failing input = %v", err)
	}
}

// TestConfirmCanceled checks that a Ctrl+C (canceled context) during the question returns at once with
// "no", while the read of the input stays blocked (no answer typed).
func TestConfirmCanceled(t *testing.T) {
	pr, pw := io.Pipe() // input that never answers
	t.Cleanup(func() { _ = pw.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	var stderr bytes.Buffer
	type result struct {
		ok  bool
		err error
	}
	done := make(chan result, 1)
	go func() {
		ok, err := confirm(ctx, Deps{Stderr: &stderr, Stdin: pr, StdinIsTerminal: func() bool { return true }}, "Continue?")
		done <- result{ok, err}
	}()
	cancel()
	select {
	case r := <-done:
		if r.ok || r.err != nil {
			t.Errorf("canceled confirm = %v, %v; want false, nil", r.ok, r.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("confirm stays blocked despite the context cancellation")
	}
}

func TestNewLogger(t *testing.T) {
	for _, tt := range []struct {
		level   string
		verbose bool
		enabled slog.Level // lowest level written
		warning bool
	}{
		{"", false, slog.LevelWarn, false},
		{"debug", false, slog.LevelDebug, false},
		{" INFO ", false, slog.LevelInfo, false},
		{"warn", false, slog.LevelWarn, false},
		{"error", false, slog.LevelError, false},
		{"bavard", false, slog.LevelWarn, true},
		{"error", true, slog.LevelDebug, false},
	} {
		var buf bytes.Buffer
		logger := newLogger(&buf, tt.level, tt.verbose)
		hasWarning := strings.Contains(buf.String(), "warning: WATCHOO_LOG_LEVEL")
		if hasWarning != tt.warning {
			t.Errorf("newLogger(%q): warning %v, want %v (%q)", tt.level, hasWarning, tt.warning, buf.String())
		}
		ctx := t.Context()
		if !logger.Enabled(ctx, tt.enabled) || (tt.enabled > slog.LevelDebug && logger.Enabled(ctx, tt.enabled-4)) {
			t.Errorf("newLogger(%q, %v): want minimum level %v", tt.level, tt.verbose, tt.enabled)
		}
	}
}

// --verbose: the engine's logs (debug level) reach the error output, without the token.
func TestVerboseLogs(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	// Without Deps.Logger, the CLI builds its own logger on the error output.
	var stdout, stderr bytes.Buffer
	deps := Deps{
		Stdout:        &stdout,
		Stderr:        &stderr,
		NewAuthClient: func(workspace.Config) auth.Client { return h.Client },
		Getwd:         func() (string, error) { return h.cwd, nil },
		NewProvider:   func(string) (provider.Provider, error) { return h.Provider, nil },
		Now:           h.Now,
	}
	if code := Run(t.Context(), deps, []string{"sync", "--verbose"}); code != 0 {
		t.Fatalf("sync --verbose: code %d\n%s", code, stderr.String())
	}
	assertContains(t, "logs --verbose", stderr.String(), "level=", "alice/projet")
	assertNotContains(t, "logs --verbose", stderr.String(), "gho_", "Alice Martin", "alice@example.test",
		"Premier jet")
	stderr.Reset()
	if code := Run(t.Context(), deps, []string{"sync"}); code != 0 || stderr.Len() != 0 {
		t.Errorf("sync without --verbose: code %d, logs %q", code, stderr.String())
	}
}
