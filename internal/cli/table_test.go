package cli

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/workspace"
)

func TestRenderTable(t *testing.T) {
	headers := []string{"Dépôt", "État"}
	rows := [][]string{{"alice/projet", "-"}, {"Élodie", "⚠ 42 j"}, {"x", ""}}
	want := "DÉPÔT         ÉTAT\n" +
		"alice/projet  -\n" +
		"Élodie        ⚠ 42 j\n" +
		"x\n"
	if got := renderTable(textStyle{}, headers, rows, nil); got != want {
		t.Errorf("renderTable =\n%q\nattendu :\n%q", got, want)
	}
	// Accent décomposé (NFD) : une seule colonne.
	if got := renderTable(textStyle{}, []string{"a", "b"}, [][]string{{"éte", "1"}, {"abcd", "2"}}, nil); got !=
		"A     B\néte   1\nabcd  2\n" {
		t.Errorf("renderTable NFD =\n%q", got)
	}

	// Couleurs : en-têtes en gras, lignes grises en gris ; sans TermInfo.Color, aucune séquence.
	color := textStyle{term: TermInfo{IsTerminal: true, Color: true, Unicode: true}}
	got := renderTable(color, headers, rows, func(row int) bool { return row == 1 })
	lines := strings.Split(got, "\n")
	if !strings.Contains(lines[0], "\x1b[1m") || strings.Contains(lines[1], "\x1b[") || !strings.Contains(lines[2], "\x1b[") {
		t.Errorf("renderTable en couleur :\n%q", got)
	}
	if plain := ansiSeq.ReplaceAllString(got, ""); plain != want {
		t.Errorf("renderTable en couleur, sans séquences =\n%q\nattendu :\n%q", plain, want)
	}
	noColor := textStyle{term: TermInfo{IsTerminal: true, Unicode: true}}
	if got := renderTable(noColor, headers, rows, func(int) bool { return true }); got != want {
		t.Errorf("renderTable sans couleur =\n%q", got)
	}
}

func TestTextStyle(t *testing.T) {
	redirected := textStyle{}
	ascii := textStyle{term: TermInfo{IsTerminal: true}}
	if !redirected.unicode() || ascii.unicode() || redirected.color() {
		t.Error("unicode/color : une sortie redirigée est en UTF-8 sans couleur ; un terminal ASCII sans Unicode")
	}
	if got := redirected.truncate("Un message bien trop long\nsuite", 10); got != "Un message…" && got != "Un messag…" {
		t.Errorf("truncate = %q", got)
	}
	if got := redirected.truncate("court\nsuite", 10); got != "court" {
		t.Errorf("truncate court = %q", got)
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
			input += "reste" // ne doit pas être lu
		}
		in := strings.NewReader(input)
		deps := Deps{Stderr: &stderr, Stdin: in, StdinIsTerminal: func() bool { return true }}
		got, err := confirm(deps, "Continuer ?")
		if err != nil || got != tt.want {
			t.Errorf("confirm(%q) = %v, %v ; attendu %v", tt.input, got, err, tt.want)
		}
		if !strings.HasPrefix(stderr.String(), "Continuer ? [o/N] ") {
			t.Errorf("question = %q", stderr.String())
		}
		if strings.HasSuffix(tt.input, "\n") {
			if rest, _ := io.ReadAll(in); string(rest) != "reste" {
				t.Errorf("confirm(%q) a lu au-delà de la ligne : reste %q", tt.input, rest)
			}
		}
	}

	// Hors terminal : rien n'est lu ni demandé, --yes est exigé.
	var stderr bytes.Buffer
	in := strings.NewReader("o\n")
	_, err := confirm(Deps{Stderr: &stderr, Stdin: in, StdinIsTerminal: func() bool { return false }}, "Continuer ?")
	if !errors.Is(err, errNeedYes) || stderr.Len() != 0 || in.Len() != 2 {
		t.Errorf("confirm hors terminal = %v (stderr %q, %d octets restants)", err, stderr.String(), in.Len())
	}

	// Erreur de lecture.
	_, err = confirm(Deps{Stderr: io.Discard, Stdin: iotest.ErrReader(errors.New("panne")),
		StdinIsTerminal: func() bool { return true }}, "?")
	if err == nil || !strings.Contains(err.Error(), "lecture de la réponse : panne") {
		t.Errorf("confirm sur une entrée en erreur = %v", err)
	}
}

func TestNewLogger(t *testing.T) {
	for _, tt := range []struct {
		level   string
		verbose bool
		enabled slog.Level // plus bas niveau écrit
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
		hasWarning := strings.Contains(buf.String(), "attention : WATCHOO_LOG_LEVEL")
		if hasWarning != tt.warning {
			t.Errorf("newLogger(%q) : avertissement %v, attendu %v (%q)", tt.level, hasWarning, tt.warning, buf.String())
		}
		ctx := t.Context()
		if !logger.Enabled(ctx, tt.enabled) || (tt.enabled > slog.LevelDebug && logger.Enabled(ctx, tt.enabled-4)) {
			t.Errorf("newLogger(%q, %v) : niveau minimal attendu %v", tt.level, tt.verbose, tt.enabled)
		}
	}
}

// --verbose : les logs du moteur (niveau debug) arrivent sur la sortie d'erreur, sans le token.
func TestVerboseLogs(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	// Sans Deps.Logger, la CLI construit son propre logger sur la sortie d'erreur.
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
		t.Fatalf("sync --verbose : code %d\n%s", code, stderr.String())
	}
	assertContains(t, "logs --verbose", stderr.String(), "level=", "alice/projet")
	assertNotContains(t, "logs --verbose", stderr.String(), "gho_", "Alice Martin", "alice@example.test",
		"Premier jet")
	stderr.Reset()
	if code := Run(t.Context(), deps, []string{"sync"}); code != 0 || stderr.Len() != 0 {
		t.Errorf("sync sans --verbose : code %d, logs %q", code, stderr.String())
	}
}
