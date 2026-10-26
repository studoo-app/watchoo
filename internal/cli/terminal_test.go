package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// clearTerminalEnv retire les variables d'environnement qui décrivent le terminal (restaurées à la fin
// du test).
func clearTerminalEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"COLUMNS", "LC_ALL", "LC_CTYPE", "LANG", "WT_SESSION", "NO_COLOR", "TERM"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// setTerminalEnv vide les variables qui décrivent le terminal, puis définit celles de env.
func setTerminalEnv(t *testing.T, env map[string]string) {
	t.Helper()
	clearTerminalEnv(t)
	for name, value := range env {
		t.Setenv(name, value)
	}
}

func TestTerminalWidth(t *testing.T) {
	for _, tt := range []struct {
		columns string // "" : COLUMNS absente
		system  int
		want    int
	}{
		{"120", 0, 120},
		{"120", 90, 120}, // COLUMNS passe avant le système
		{" 100 ", 0, 100},
		{"40", 200, 40},
		{"", 90, 90},
		{"", 0, 0}, // inconnue
		{"", -1, 0},
		{"0", 90, 90},
		{"-5", 90, 90},
		{"abc", 90, 90},
		{"80x", 0, 0},
	} {
		env := map[string]string{}
		if tt.columns != "" {
			env["COLUMNS"] = tt.columns
		}
		setTerminalEnv(t, env)
		systemCalls := 0
		got := terminalWidth(os.LookupEnv, func() int {
			systemCalls++
			return tt.system
		})
		if got != tt.want {
			t.Errorf("COLUMNS=%q, largeur système %d : largeur %d, attendu %d", tt.columns, tt.system, got, tt.want)
		}
		if tt.columns == "120" && systemCalls != 0 {
			t.Errorf("COLUMNS=%q : largeur système demandée alors que COLUMNS suffit", tt.columns)
		}
	}
}

func TestUnicodeTerminal(t *testing.T) {
	for _, tt := range []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{}, false},
		{map[string]string{"LANG": "fr_FR.UTF-8"}, true},
		{map[string]string{"LANG": "en_US.utf8"}, true},
		{map[string]string{"LANG": "fr_FR.Utf-8"}, true}, // casse ignorée
		{map[string]string{"LANG": "fr_FR.UTF8"}, true},
		{map[string]string{"LC_CTYPE": "UTF-8"}, true}, // valeur habituelle sous macOS
		{map[string]string{"LANG": "C"}, false},
		{map[string]string{"LANG": "POSIX"}, false},
		{map[string]string{"LANG": "fr_FR.ISO-8859-1"}, false},
		{map[string]string{"LANG": "fr_FR"}, false},
		// La première variable non vide décide, dans l'ordre LC_ALL, LC_CTYPE, LANG.
		{map[string]string{"LC_ALL": "C", "LC_CTYPE": "fr_FR.UTF-8", "LANG": "fr_FR.UTF-8"}, false},
		{map[string]string{"LC_ALL": "C", "LANG": "fr_FR.UTF-8"}, false},
		{map[string]string{"LC_ALL": "fr_FR.UTF-8", "LANG": "C"}, true},
		{map[string]string{"LC_CTYPE": "C", "LANG": "fr_FR.UTF-8"}, false},
		{map[string]string{"LC_CTYPE": "fr_FR.UTF-8", "LANG": "C"}, true},
		{map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "fr_FR.UTF-8"}, true},
		// Windows Terminal : Unicode, quelle que soit la locale.
		{map[string]string{"WT_SESSION": "c2a3f0e4-7d1b-4b6e-9f0a-1c2d3e4f5a6b"}, true},
		{map[string]string{"WT_SESSION": "c2a3f0e4", "LANG": "C"}, true},
		{map[string]string{"WT_SESSION": ""}, true}, // définie, même vide
	} {
		setTerminalEnv(t, tt.env)
		if got := unicodeTerminal(os.LookupEnv); got != tt.want {
			t.Errorf("environnement %v : Unicode = %t, attendu %t", tt.env, got, tt.want)
		}
	}
}

func TestColorAllowed(t *testing.T) {
	for _, tt := range []struct {
		env  map[string]string
		want bool
	}{
		{map[string]string{}, true},
		{map[string]string{"TERM": "xterm-256color"}, true},
		{map[string]string{"NO_COLOR": ""}, true}, // vide : ignorée (https://no-color.org/)
		{map[string]string{"NO_COLOR": "1"}, false},
		{map[string]string{"NO_COLOR": "0"}, false}, // toute valeur non vide
		{map[string]string{"TERM": "dumb"}, false},
		{map[string]string{"TERM": "dumb", "NO_COLOR": ""}, false},
	} {
		setTerminalEnv(t, tt.env)
		if got := colorAllowed(os.LookupEnv); got != tt.want {
			t.Errorf("environnement %v : couleur = %t, attendu %t", tt.env, got, tt.want)
		}
	}
}

func TestDescribeTerminal(t *testing.T) {
	setTerminalEnv(t, map[string]string{"LANG": "fr_FR.UTF-8", "TERM": "xterm-256color"})
	got := describeTerminal(os.LookupEnv, func() int { return 132 })
	if want := (TermInfo{IsTerminal: true, Width: 132, Unicode: true, Color: true}); got != want {
		t.Errorf("describeTerminal = %+v, attendu %+v", got, want)
	}

	setTerminalEnv(t, map[string]string{"LC_ALL": "C", "NO_COLOR": "1", "COLUMNS": "40"})
	got = describeTerminal(os.LookupEnv, func() int { return 132 })
	if want := (TermInfo{IsTerminal: true, Width: 40}); got != want {
		t.Errorf("describeTerminal = %+v, attendu %+v", got, want)
	}
}

// Un tampon, un fichier ordinaire ou un tube ne sont jamais des terminaux, quel que soit
// l'environnement.
func TestDetectTerminalNotATerminal(t *testing.T) {
	setTerminalEnv(t, map[string]string{"LANG": "fr_FR.UTF-8", "COLUMNS": "120", "TERM": "xterm-256color"})

	file, err := os.Create(filepath.Join(t.TempDir(), "sortie.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	for name, out := range map[string]io.Writer{"tampon": &bytes.Buffer{}, "fichier": file, "tube": w} {
		if got := DetectTerminal(out); got != (TermInfo{}) {
			t.Errorf("DetectTerminal(%s) = %+v, attendu aucun terminal", name, got)
		}
	}
	if got := DetectTerminal(nil); got != (TermInfo{}) {
		t.Errorf("DetectTerminal(nil) = %+v, attendu aucun terminal", got)
	}
}

// Sans Terminal, la sortie décrite est le Stdout retenu : un tampon n'est jamais un terminal.
func TestDefaultTerminalFollowsStdout(t *testing.T) {
	if DefaultDeps().Terminal == nil {
		t.Fatal("DefaultDeps sans Terminal")
	}
	d := (Deps{Stdout: &bytes.Buffer{}}).withDefaults()
	if d.Terminal == nil {
		t.Fatal("withDefaults sans Terminal")
	}
	if got := d.Terminal(); got.IsTerminal {
		t.Errorf("Terminal() d'un tampon = %+v, attendu aucun terminal", got)
	}
}
