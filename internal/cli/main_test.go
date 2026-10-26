package cli

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// fakeGHPath est le chemin du faux gh (internal/auth/testdata/fakegh) compilé par TestMain, pour les
// tests d'intégration qui passent par le vrai auth.GH.
var fakeGHPath string

// testLogs reçoit tous les logs slog émis pendant les tests (niveau Debug compris), pour vérifier
// qu'ils ne contiennent jamais de token.
var testLogs syncBuffer

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests compile le faux gh, redirige les logs, puis lance les tests.
func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "watchoo-cli-fakegh-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "création du répertoire du faux gh : %v\n", err)
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
		fmt.Fprintf(os.Stderr, "commande go introuvable pour compiler le faux gh : %v\n", err)
		return 1
	}
	cmd := exec.Command(goBin, "build", "-o", fakeGHPath, "../auth/testdata/fakegh")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "compilation du faux gh : %v\n%s", err, out)
		return 1
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(&testLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return m.Run()
}

// syncBuffer est un bytes.Buffer sûr pour un usage concurrent.
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
