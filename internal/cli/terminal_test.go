package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// clearTerminalEnv removes the environment variables that describe the terminal (restored at the end of
// the test).
func clearTerminalEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"COLUMNS", "LC_ALL", "LC_CTYPE", "LANG", "WT_SESSION", "NO_COLOR", "TERM"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

// setTerminalEnv clears the variables that describe the terminal, then sets those of env.
func setTerminalEnv(t *testing.T, env map[string]string) {
	t.Helper()
	clearTerminalEnv(t)
	for name, value := range env {
		t.Setenv(name, value)
	}
}

func TestTerminalWidth(t *testing.T) {
	for _, tt := range []struct {
		columns string // "": COLUMNS unset
		system  int
		want    int
	}{
		{"120", 0, 120},
		{"120", 90, 120}, // COLUMNS takes precedence over the system
		{" 100 ", 0, 100},
		{"40", 200, 40},
		{"", 90, 90},
		{"", 0, 0}, // unknown
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
			t.Errorf("COLUMNS=%q, system width %d: width %d, want %d", tt.columns, tt.system, got, tt.want)
		}
		if tt.columns == "120" && systemCalls != 0 {
			t.Errorf("COLUMNS=%q: system width asked although COLUMNS is enough", tt.columns)
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
		{map[string]string{"LANG": "fr_FR.Utf-8"}, true}, // case ignored
		{map[string]string{"LANG": "fr_FR.UTF8"}, true},
		{map[string]string{"LC_CTYPE": "UTF-8"}, true}, // usual value on macOS
		{map[string]string{"LANG": "C"}, false},
		{map[string]string{"LANG": "POSIX"}, false},
		{map[string]string{"LANG": "fr_FR.ISO-8859-1"}, false},
		{map[string]string{"LANG": "fr_FR"}, false},
		// The first non-empty variable decides, in the order LC_ALL, LC_CTYPE, LANG.
		{map[string]string{"LC_ALL": "C", "LC_CTYPE": "fr_FR.UTF-8", "LANG": "fr_FR.UTF-8"}, false},
		{map[string]string{"LC_ALL": "C", "LANG": "fr_FR.UTF-8"}, false},
		{map[string]string{"LC_ALL": "fr_FR.UTF-8", "LANG": "C"}, true},
		{map[string]string{"LC_CTYPE": "C", "LANG": "fr_FR.UTF-8"}, false},
		{map[string]string{"LC_CTYPE": "fr_FR.UTF-8", "LANG": "C"}, true},
		{map[string]string{"LC_ALL": "", "LC_CTYPE": "", "LANG": "fr_FR.UTF-8"}, true},
		// Windows Terminal: Unicode, whatever the locale.
		{map[string]string{"WT_SESSION": "c2a3f0e4-7d1b-4b6e-9f0a-1c2d3e4f5a6b"}, true},
		{map[string]string{"WT_SESSION": "c2a3f0e4", "LANG": "C"}, true},
		{map[string]string{"WT_SESSION": ""}, true}, // set, even empty
	} {
		setTerminalEnv(t, tt.env)
		if got := unicodeTerminal(os.LookupEnv); got != tt.want {
			t.Errorf("environment %v: Unicode = %t, want %t", tt.env, got, tt.want)
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
		{map[string]string{"NO_COLOR": ""}, true}, // empty: ignored (https://no-color.org/)
		{map[string]string{"NO_COLOR": "1"}, false},
		{map[string]string{"NO_COLOR": "0"}, false}, // any non-empty value
		{map[string]string{"TERM": "dumb"}, false},
		{map[string]string{"TERM": "dumb", "NO_COLOR": ""}, false},
	} {
		setTerminalEnv(t, tt.env)
		if got := colorAllowed(os.LookupEnv); got != tt.want {
			t.Errorf("environment %v: color = %t, want %t", tt.env, got, tt.want)
		}
	}
}

func TestDescribeTerminal(t *testing.T) {
	setTerminalEnv(t, map[string]string{"LANG": "fr_FR.UTF-8", "TERM": "xterm-256color"})
	got := describeTerminal(os.LookupEnv, func() int { return 132 })
	if want := (TermInfo{IsTerminal: true, Width: 132, Unicode: true, Color: true}); got != want {
		t.Errorf("describeTerminal = %+v, want %+v", got, want)
	}

	setTerminalEnv(t, map[string]string{"LC_ALL": "C", "NO_COLOR": "1", "COLUMNS": "40"})
	got = describeTerminal(os.LookupEnv, func() int { return 132 })
	if want := (TermInfo{IsTerminal: true, Width: 40}); got != want {
		t.Errorf("describeTerminal = %+v, want %+v", got, want)
	}
}

// A buffer, a regular file or a pipe are never terminals, whatever the environment.
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

	for name, out := range map[string]io.Writer{"buffer": &bytes.Buffer{}, "file": file, "pipe": w} {
		if got := DetectTerminal(out); got != (TermInfo{}) {
			t.Errorf("DetectTerminal(%s) = %+v, want no terminal", name, got)
		}
	}
	if got := DetectTerminal(nil); got != (TermInfo{}) {
		t.Errorf("DetectTerminal(nil) = %+v, want no terminal", got)
	}
}

// Without Terminal, the described output is the chosen Stdout: a buffer is never a terminal.
func TestDefaultTerminalFollowsStdout(t *testing.T) {
	if DefaultDeps().Terminal == nil {
		t.Fatal("DefaultDeps without Terminal")
	}
	d := (Deps{Stdout: &bytes.Buffer{}}).withDefaults()
	if d.Terminal == nil {
		t.Fatal("withDefaults without Terminal")
	}
	if got := d.Terminal(); got.IsTerminal {
		t.Errorf("Terminal() of a buffer = %+v, want no terminal", got)
	}
}
