package auth

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// Dummy values of the fake gh (testdata/fakegh), copied here: the auth package cannot import
// authtest (which imports it). TestFakeValuesMatchAuthtest checks that they match.
const (
	fakeToken  = "gho_fake_for_tests"
	otherToken = fakeToken + "_other"
	envToken   = fakeToken + "_env"
	fakeLogin  = "watchoo-test"
	otherLogin = "watchoo-other"
)

// fakeGHPath is the path of the fake gh built by TestMain. The binary is named "gh" ("gh.exe" on
// Windows), so that it can also be found through the PATH.
var fakeGHPath string

// testLogs receives all the slog logs emitted during the tests (Debug level included), to check that
// they never contain a token.
var testLogs syncBuffer

// TestMain sets the process language to English for the whole package (the expected texts of the
// tests are written in English, whatever WATCHOO_LANG and the machine locale), then runs the tests.
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(runTests(m))
}

// runTests builds the fake gh, redirects the logs, then runs the tests.
func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "watchoo-fakegh-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating the directory of the fake gh: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fakeGHPath = filepath.Join(dir, name)
	if err := buildFakeGH(fakeGHPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(&testLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return m.Run()
}

// buildFakeGH builds testdata/fakegh into out. "go test" puts GOROOT/bin first in the PATH of the
// test binaries, so the go command is found.
func buildFakeGH(out string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("go command not found to build the fake gh: %w", err)
	}
	cmd := exec.Command(goBin, "build", "-o", out, "./testdata/fakegh")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("building the fake gh: %w\n%s", err, output)
	}
	return nil
}

// syncBuffer is a bytes.Buffer safe for concurrent use.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// fakeEnv describes the files of a test using the fake gh.
type fakeEnv struct {
	record string // FAKEGH_RECORD_FILE: one call per line
	pid    string // FAKEGH_PID_FILE: PID of the blocked fake gh (hang scenario)
}

// useScenario chooses the scenario of the fake gh for the test (environment variables, hence
// non-parallel tests) and prepares its files.
func useScenario(t *testing.T, scenario string) fakeEnv {
	t.Helper()
	dir := t.TempDir()
	env := fakeEnv{
		record: filepath.Join(dir, "calls.jsonl"),
		pid:    filepath.Join(dir, "pid"),
	}
	t.Setenv("FAKEGH_SCENARIO", scenario)
	t.Setenv("FAKEGH_RECORD_FILE", env.record)
	t.Setenv("FAKEGH_STATE_FILE", filepath.Join(dir, "state"))
	t.Setenv("FAKEGH_PID_FILE", env.pid)
	return env
}

// fakeGH returns a GH pointing to the fake gh, with a generous timeout for slow machines.
func fakeGH() GH {
	return GH{Path: fakeGHPath, Timeout: 10 * time.Second}
}

// call is a call recorded by the fake gh.
type call struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

// readCalls reads the calls recorded by the fake gh (none if the file does not exist).
func readCalls(t *testing.T, env fakeEnv) []call {
	t.Helper()
	f, err := os.Open(env.record)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("opening the recorded calls: %v", err)
	}
	defer f.Close()
	var calls []call
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c call
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("unreadable recorded call %q: %v", sc.Text(), err)
		}
		calls = append(calls, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("reading the recorded calls: %v", err)
	}
	return calls
}

// wantErrIs fails if err does not wrap target.
func wantErrIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("error = %v, want an error wrapping %q", err, target)
	}
}
