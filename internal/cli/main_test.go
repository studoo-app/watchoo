package cli

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// fakeGHPath is the path of the fake gh (internal/auth/testdata/fakegh) built by TestMain, for the
// integration tests that go through the real auth.GH.
var fakeGHPath string

// testLogs receives all the slog logs emitted during the tests (Debug level included), to check that
// they never contain a token.
var testLogs syncBuffer

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests builds the fake gh, forces the language of the messages (English: the tests' expected texts
// are written in this language, see documentation/en/09-testing.md, "Languages in tests"), redirects
// the logs, then runs the tests.
func runTests(m *testing.M) int {
	i18n.SetProcessForTest(i18n.English)
	dir, err := os.MkdirTemp("", "watchoo-cli-fakegh-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "creating the fake gh directory: %v\n", err)
		return 1
	}
	defer os.RemoveAll(dir)

	name := "gh"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	fakeGHPath = filepath.Join(dir, name)
	goBin, err := exec.LookPath("go")
	if err != nil {
		fmt.Fprintf(os.Stderr, "go command not found to build the fake gh: %v\n", err)
		return 1
	}
	cmd := exec.Command(goBin, "build", "-o", fakeGHPath, "../auth/testdata/fakegh")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building the fake gh: %v\n%s", err, out)
		return 1
	}

	// The build information of the test binary must not change the displayed version: the version
	// tests replace it themselves.
	readBuildInfo = func() (*debug.BuildInfo, bool) { return nil, false }

	slog.SetDefault(slog.New(slog.NewTextHandler(&testLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return m.Run()
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
