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
)

// Valeurs factices du faux gh (testdata/fakegh), recopiées ici : le package auth ne peut pas
// importer authtest (qui l'importe). TestFakeValuesMatchAuthtest vérifie qu'elles concordent.
const (
	fakeToken  = "gho_fake_for_tests"
	otherToken = fakeToken + "_other"
	envToken   = fakeToken + "_env"
	fakeLogin  = "watchoo-test"
	otherLogin = "watchoo-other"
)

// fakeGHPath est le chemin du faux gh compilé par TestMain. Le binaire s'appelle « gh » (« gh.exe »
// sous Windows), pour pouvoir aussi être trouvé par le PATH.
var fakeGHPath string

// testLogs reçoit tous les logs slog émis pendant les tests (niveau Debug compris), pour vérifier
// qu'ils ne contiennent jamais de token.
var testLogs syncBuffer

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests compile le faux gh, redirige les logs, puis lance les tests.
func runTests(m *testing.M) int {
	dir, err := os.MkdirTemp("", "watchoo-fakegh-")
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
	if err := buildFakeGH(fakeGHPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(&testLogs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return m.Run()
}

// buildFakeGH compile testdata/fakegh vers out. « go test » place GOROOT/bin en tête du PATH des
// binaires de test, donc la commande go est trouvée.
func buildFakeGH(out string) error {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return fmt.Errorf("commande go introuvable pour compiler le faux gh : %w", err)
	}
	cmd := exec.Command(goBin, "build", "-o", out, "./testdata/fakegh")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("compilation du faux gh : %w\n%s", err, output)
	}
	return nil
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

// fakeEnv décrit les fichiers d'un test utilisant le faux gh.
type fakeEnv struct {
	record string // FAKEGH_RECORD_FILE : un appel par ligne
	pid    string // FAKEGH_PID_FILE : PID du faux gh bloqué (scénario hang)
}

// useScenario choisit le scénario du faux gh pour le test (variables d'environnement, donc tests non
// parallèles) et prépare ses fichiers.
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

// fakeGH renvoie un GH pointant vers le faux gh, avec un délai large pour les machines lentes.
func fakeGH() GH {
	return GH{Path: fakeGHPath, Timeout: 10 * time.Second}
}

// call est un appel enregistré par le faux gh.
type call struct {
	Args []string          `json:"args"`
	Env  map[string]string `json:"env"`
}

// readCalls lit les appels enregistrés par le faux gh (aucun si le fichier n'existe pas).
func readCalls(t *testing.T, env fakeEnv) []call {
	t.Helper()
	f, err := os.Open(env.record)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatalf("ouverture des appels enregistrés : %v", err)
	}
	defer f.Close()
	var calls []call
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var c call
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("appel enregistré illisible %q : %v", sc.Text(), err)
		}
		calls = append(calls, c)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("lecture des appels enregistrés : %v", err)
	}
	return calls
}

// wantErrIs échoue si err n'enveloppe pas target.
func wantErrIs(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("erreur = %v, veut une erreur enveloppant %q", err, target)
	}
}
