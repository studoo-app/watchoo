package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestStatusLoggedIn(t *testing.T) {
	env := useScenario(t, "logged-in")

	st, err := fakeGH().Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	want := Status{
		Host:        "github.com",
		Login:       fakeLogin,
		TokenSource: "keyring",
		Active:      true,
		State:       "success",
		Scopes:      []string{"gist", "read:org", "repo"},
	}
	if !reflect.DeepEqual(st, want) {
		t.Errorf("Status = %+v, want %+v", st, want)
	}

	calls := readCalls(t, env)
	if len(calls) != 1 {
		t.Fatalf("%d calls to gh, want 1", len(calls))
	}
	wantArgs := []string{"auth", "status", "--json", "hosts", "--hostname", "github.com"}
	if !reflect.DeepEqual(calls[0].Args, wantArgs) {
		t.Errorf("arguments = %q, want %q", calls[0].Args, wantArgs)
	}
}

func TestTokenLoggedIn(t *testing.T) {
	env := useScenario(t, "logged-in")

	token, err := fakeGH().Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if token != fakeToken {
		t.Errorf("Token returned an unexpected value (length %d)", len(token))
	}

	calls := readCalls(t, env)
	if len(calls) != 1 {
		t.Fatalf("%d calls to gh, want 1", len(calls))
	}
	wantArgs := []string{"auth", "token", "--hostname", "github.com"}
	if !reflect.DeepEqual(calls[0].Args, wantArgs) {
		t.Errorf("arguments = %q, want %q", calls[0].Args, wantArgs)
	}
}

func TestEnvironmentPassedToGH(t *testing.T) {
	env := useScenario(t, "logged-in")
	// Opposite values in the inherited environment must be replaced.
	t.Setenv("GH_PROMPT_DISABLED", "0")
	t.Setenv("GH_NO_UPDATE_NOTIFIER", "")

	g := fakeGH()
	if _, err := g.Status(context.Background()); err != nil {
		t.Fatalf("Status: %v", err)
	}
	if _, err := g.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	calls := readCalls(t, env)
	if len(calls) != 2 {
		t.Fatalf("%d calls to gh, want 2", len(calls))
	}
	for _, c := range calls {
		for _, key := range []string{"GH_PROMPT_DISABLED", "GH_NO_UPDATE_NOTIFIER"} {
			if got := c.Env[key]; got != "1" {
				t.Errorf("call %q: %s = %q, want \"1\"", c.Args, key, got)
			}
		}
	}
}

func TestCustomHost(t *testing.T) {
	env := useScenario(t, "logged-in")
	g := fakeGH()
	g.Host = "ghe.example.test"

	st, err := g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Host != "ghe.example.test" {
		t.Errorf("Host = %q, want ghe.example.test", st.Host)
	}
	if _, err := g.Token(context.Background()); err != nil {
		t.Fatalf("Token: %v", err)
	}
	for _, c := range readCalls(t, env) {
		if !containsSeq(c.Args, "--hostname", "ghe.example.test") {
			t.Errorf("arguments = %q, want --hostname ghe.example.test", c.Args)
		}
	}
}

func TestNotLoggedIn(t *testing.T) {
	useScenario(t, "not-logged-in")
	g := fakeGH()

	st, err := g.Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if st.Login != "" {
		t.Errorf("Status.Login = %q, want empty", st.Login)
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("the message %q should mention \"gh auth login\"", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
}

func TestExitNotLoggedIn(t *testing.T) {
	useScenario(t, "exit-not-logged-in")

	_, err := fakeGH().Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if errors.Is(err, ErrUnexpectedOutput) {
		t.Errorf("the error %q should not be an unexpected answer", err)
	}
	if !strings.Contains(err.Error(), "You are not logged into any GitHub hosts") {
		t.Errorf("the message %q should quote the error output of gh", err)
	}
}

func TestMultiAccount(t *testing.T) {
	tests := []struct {
		user       string
		wantLogin  string
		wantActive bool
		wantScopes []string
		wantToken  string
	}{
		{"", fakeLogin, true, []string{"gist", "read:org", "repo"}, fakeToken},
		{fakeLogin, fakeLogin, true, []string{"gist", "read:org", "repo"}, fakeToken},
		{otherLogin, otherLogin, false, []string{"repo"}, otherToken},
	}
	for _, tt := range tests {
		t.Run("user="+tt.user, func(t *testing.T) {
			env := useScenario(t, "multi-account")
			g := fakeGH()
			g.User = tt.user

			st, err := g.Status(context.Background())
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if st.Login != tt.wantLogin || st.Active != tt.wantActive || !reflect.DeepEqual(st.Scopes, tt.wantScopes) {
				t.Errorf("Status = %+v, want login %q, active %v, scopes %q", st, tt.wantLogin, tt.wantActive, tt.wantScopes)
			}

			token, err := g.Token(context.Background())
			if err != nil {
				t.Fatalf("Token: %v", err)
			}
			if token != tt.wantToken {
				t.Errorf("Token did not return the token of account %q", tt.wantLogin)
			}

			calls := readCalls(t, env)
			if len(calls) != 2 {
				t.Fatalf("%d calls to gh, want 2", len(calls))
			}
			hasUser := containsSeq(calls[1].Args, "--user", tt.user)
			if tt.user != "" && !hasUser {
				t.Errorf("arguments of auth token = %q, want --user %s", calls[1].Args, tt.user)
			}
			if tt.user == "" && slices.Contains(calls[1].Args, "--user") {
				t.Errorf("arguments of auth token = %q, no --user expected", calls[1].Args)
			}
		})
	}
}

// gh is case-sensitive in "gh auth token --user": a github.user written with another case must be
// refused by Status already, otherwise init would succeed and every Token would fail afterwards.
func TestMultiAccountUserWrongCase(t *testing.T) {
	useScenario(t, "multi-account")
	g := fakeGH()
	g.User = strings.ToUpper(otherLogin)

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrUnusableUser)
	wantErrIs(t, err, ErrNotLoggedIn)
	if !strings.Contains(err.Error(), `"`+otherLogin+`"`) || !strings.Contains(err.Error(), "case-sensitive") {
		t.Errorf("the message %q should give the exact login known to gh", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
}

func TestMultiAccountUnknownUser(t *testing.T) {
	useScenario(t, "multi-account")
	g := fakeGH()
	g.User = "inconnu"

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if !strings.Contains(err.Error(), `"inconnu"`) || !strings.Contains(err.Error(), "github.user") {
		t.Errorf("the message %q should quote the account and the github.user setting", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if !strings.Contains(err.Error(), `"inconnu"`) {
		t.Errorf("the message %q should quote the account", err)
	}
}

func TestEnvToken(t *testing.T) {
	useScenario(t, "env-token")

	// Without github.user: the active account, that is the token of GH_TOKEN.
	g := fakeGH()
	st, err := g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.TokenSource != "GH_TOKEN" || !st.Active {
		t.Errorf("Status = %+v, want the active account from GH_TOKEN", st)
	}
	if token, err := g.Token(context.Background()); err != nil || token != envToken {
		t.Errorf("Token without --user: error %v or unexpected token", err)
	}

	// With github.user: gh auth token --user reads the stored account, Status must describe it.
	g.User = fakeLogin
	st, err = g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.TokenSource != "keyring" {
		t.Errorf("Status = %+v, want the stored account (keyring)", st)
	}
	if token, err := g.Token(context.Background()); err != nil || token != fakeToken {
		t.Errorf("Token with --user: error %v or unexpected token", err)
	}
}

// Docker or CI: GH_TOKEN set, no stored account. Without github.user, Status and Token use the token
// of GH_TOKEN; with github.user, "gh auth token --user" ignores GH_TOKEN and fails, so Status must
// refuse already (with a message that says to empty github.user).
func TestEnvTokenOnly(t *testing.T) {
	env := useScenario(t, "env-token-only")

	g := fakeGH()
	st, err := g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Login != fakeLogin || st.TokenSource != "GH_TOKEN" || !st.Active {
		t.Errorf("Status = %+v, want the active account from GH_TOKEN", st)
	}
	if token, err := g.Token(context.Background()); err != nil || token != envToken {
		t.Errorf("Token without --user: error %v or unexpected token", err)
	}

	g.User = fakeLogin
	_, err = g.Status(context.Background())
	wantErrIs(t, err, ErrUnusableUser)
	for _, part := range []string{"github.user", "GH_TOKEN", "empty it"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("the message %q should contain %q", err, part)
		}
	}
	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)

	calls := readCalls(t, env)
	if len(calls) != 4 || !containsSeq(calls[3].Args, "--user", fakeLogin) {
		t.Errorf("calls = %q, want a last \"auth token --user %s\"", calls, fakeLogin)
	}
}

// TestStatusAndTokenAgree checks the Client contract on the choice of the account: when Status
// succeeds, Token returns the token of this account; when Status finds no usable account, Token
// fails too. Otherwise "watchoo init" would accept an account whose token the sync could never read.
func TestStatusAndTokenAgree(t *testing.T) {
	tokens := map[string]string{fakeLogin: fakeToken, otherLogin: otherToken}
	scenarios := []string{"logged-in", "multi-account", "env-token", "env-token-only", "not-logged-in"}
	users := []string{"", fakeLogin, otherLogin, strings.ToUpper(fakeLogin), "Watchoo-Other", "inconnu"}
	for _, scenario := range scenarios {
		for _, user := range users {
			t.Run(scenario+"/user="+user, func(t *testing.T) {
				useScenario(t, scenario)
				g := fakeGH()
				g.User = user

				st, statusErr := g.Status(context.Background())
				token, tokenErr := g.Token(context.Background())
				switch {
				case statusErr == nil && tokenErr != nil:
					t.Fatalf("Status succeeded (account %q) but Token fails: %v", st.Login, tokenErr)
				case statusErr == nil && st.TokenSource != "GH_TOKEN" && token != tokens[st.Login]:
					t.Errorf("Token did not return the token of account %q described by Status", st.Login)
				case statusErr == nil && st.TokenSource == "GH_TOKEN" && token != envToken:
					t.Errorf("Token did not return the GH_TOKEN token described by Status")
				case statusErr != nil && !errors.Is(statusErr, ErrNotLoggedIn):
					t.Fatalf("Status: unexpected error %v", statusErr)
				case statusErr != nil && tokenErr == nil:
					t.Errorf("Status fails (%v) but Token succeeds", statusErr)
				}
			})
		}
	}
}

// GitHub unreachable (network, proxy, outage): gh answers "error" with a network error, even with a
// valid token. It is not a gh sign-in problem: no "gh auth login" advice.
func TestOffline(t *testing.T) {
	useScenario(t, "offline")
	g := fakeGH()

	st, err := g.Status(context.Background())
	wantErrIs(t, err, ErrCheckFailed)
	if errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("the error %q should not say that gh is not signed in", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "gh auth login") {
		t.Errorf("the message %q should not advise \"gh auth login\"", msg)
	}
	if !strings.Contains(msg, "proxyconnect tcp") || !strings.Contains(msg, fakeLogin) {
		t.Errorf("the message %q should quote the account and the error of gh", msg)
	}
	if strings.Contains(msg, fakeToken) || !strings.Contains(msg, redacted()) {
		t.Errorf("the message %q should mask the token found in the error of gh", msg)
	}
	if st.Login != fakeLogin || st.State != "error" {
		t.Errorf("partial Status = %+v, want login %q and state error", st, fakeLogin)
	}

	// gh auth token does not contact GitHub: it works offline.
	if token, err := g.Token(context.Background()); err != nil || token != fakeToken {
		t.Errorf("offline Token: error %v or unexpected token", err)
	}
}

func TestInvalidToken(t *testing.T) {
	useScenario(t, "invalid-token")

	st, err := fakeGH().Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if errors.Is(err, ErrCheckFailed) {
		t.Errorf("a refused token (HTTP 401) is not an impossible check: %v", err)
	}
	// The partial Status is returned to display the account concerned.
	if st.Login != fakeLogin || st.State != "error" {
		t.Errorf("partial Status = %+v, want login %q and state error", st, fakeLogin)
	}
	if !strings.Contains(err.Error(), fakeLogin) || !strings.Contains(err.Error(), "error") {
		t.Errorf("the message %q should quote the account and its state", err)
	}
}

func TestTokenRotation(t *testing.T) {
	useScenario(t, "token-rotation")
	g := fakeGH()

	first, err := g.Token(context.Background())
	if err != nil {
		t.Fatalf("first Token: %v", err)
	}
	second, err := g.Token(context.Background())
	if err != nil {
		t.Fatalf("second Token: %v", err)
	}
	if first == second {
		t.Error("the token changed in gh, but Token returned the same one twice")
	}
	if first != fakeToken+"_1" || second != fakeToken+"_2" {
		t.Error("unexpected tokens: every call must ask gh for the token again")
	}
}

func TestEmptyOutput(t *testing.T) {
	useScenario(t, "empty-output")
	g := fakeGH()

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)
}

func TestGarbageOutput(t *testing.T) {
	useScenario(t, "garbage")
	g := fakeGH()

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)
	if strings.Contains(err.Error(), "Logged in") {
		t.Errorf("the message %q must not quote the standard output of gh", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)
	if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), "warning") {
		t.Errorf("the message %q must not quote the standard output of gh", err)
	}
}

func TestGHError(t *testing.T) {
	useScenario(t, "error")
	g := fakeGH()

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)
	msg := err.Error()
	if !strings.Contains(msg, "code 2") || !strings.Contains(msg, "simulated gh error") {
		t.Errorf("the message %q should give the exit code and the first line of stderr", msg)
	}
	if strings.Contains(msg, "second line") {
		t.Errorf("the message %q should quote only the first line of stderr", msg)
	}
	if strings.Contains(msg, fakeToken) || !strings.Contains(msg, redacted()) {
		t.Errorf("the message %q should mask the token found in stderr", msg)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if strings.Contains(err.Error(), "simulated") {
		t.Errorf("the message %q must not quote the error output of \"gh auth token\"", err)
	}
}

func TestTimeoutKillsGH(t *testing.T) {
	for _, method := range []string{"Status", "Token"} {
		t.Run(method, func(t *testing.T) {
			env := useScenario(t, "hang")
			g := fakeGH()
			g.Timeout = 500 * time.Millisecond

			start := time.Now()
			err := callMethod(context.Background(), g, method)
			elapsed := time.Since(start)

			wantErrIs(t, err, ErrTimeout)
			if errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("the error %q should not be the one of the caller's context", err)
			}
			if !strings.Contains(err.Error(), "500ms") {
				t.Errorf("the message %q should give the timeout", err)
			}
			if elapsed > g.Timeout+waitDelay+3*time.Second {
				t.Errorf("%s took %s, the timeout of %s was not applied", method, elapsed, g.Timeout)
			}
			assertProcessGone(t, env)
		})
	}
}

func TestCallerCancel(t *testing.T) {
	useScenario(t, "hang")
	g := fakeGH()

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(200*time.Millisecond, cancel)
	start := time.Now()
	_, err := g.Status(ctx)
	wantErrIs(t, err, context.Canceled)
	if errors.Is(err, ErrTimeout) {
		t.Errorf("a cancellation by the caller is not a timeout: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Status took %s after the cancellation", elapsed)
	}

	// Context already canceled: gh is not even usefully launched.
	_, err = g.Token(ctx)
	wantErrIs(t, err, context.Canceled)
}

func TestCallerDeadline(t *testing.T) {
	useScenario(t, "hang")
	g := fakeGH() // GH timeout: 10 s

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := g.Token(ctx)
	wantErrIs(t, err, context.DeadlineExceeded)
	if errors.Is(err, ErrTimeout) {
		t.Errorf("the deadline came from the caller, not from GH.Timeout: %v", err)
	}
}

func TestGHNotFound(t *testing.T) {
	useScenario(t, "logged-in")
	paths := map[string]string{
		"missing path":    filepath.Join(t.TempDir(), "absent", "gh"),
		"missing in PATH": "watchoo-gh-introuvable",
	}
	for name, path := range paths {
		t.Run(name, func(t *testing.T) {
			g := GH{Path: path}
			_, err := g.Status(context.Background())
			wantErrIs(t, err, ErrGHNotFound)
			_, err = g.Token(context.Background())
			wantErrIs(t, err, ErrGHNotFound)
		})
	}
}

func TestGHNotExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix execute permissions")
	}
	useScenario(t, "logged-in")
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte("not an executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := GH{Path: path}.Status(context.Background())
	if err == nil {
		t.Fatal("Status succeeded with a non-executable gh")
	}
	if !strings.Contains(err.Error(), "cannot launch") {
		t.Errorf("the message %q should say that gh could not be launched", err)
	}
}

func TestDefaults(t *testing.T) {
	var g GH
	if g.path() != DefaultPath || g.host() != DefaultHost || g.timeout() != DefaultTimeout {
		t.Errorf("default values = %q, %q, %s", g.path(), g.host(), g.timeout())
	}
	g.Timeout = -time.Second
	if g.timeout() != DefaultTimeout {
		t.Errorf("negative timeout: %s, want %s", g.timeout(), DefaultTimeout)
	}
	g = GH{Path: "/opt/gh", Host: "ghe.example.test", Timeout: time.Second}
	if g.path() != "/opt/gh" || g.host() != "ghe.example.test" || g.timeout() != time.Second {
		t.Errorf("set values ignored: %q, %q, %s", g.path(), g.host(), g.timeout())
	}
}

func TestDefaultPathUsesPATH(t *testing.T) {
	env := useScenario(t, "logged-in")
	// Only the directory of the fake gh is in the PATH: GH{} must find it under the name "gh".
	t.Setenv("PATH", filepath.Dir(fakeGHPath))

	var c Client = &GH{}
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.Login != fakeLogin {
		t.Errorf("Login = %q, want %q", st.Login, fakeLogin)
	}
	if len(readCalls(t, env)) != 1 {
		t.Error("the fake gh of the PATH was not called")
	}
}

func TestConcurrentCalls(t *testing.T) {
	useScenario(t, "logged-in")
	g := fakeGH()

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := range 8 {
		wg.Go(func() {
			if i%2 == 0 {
				_, err := g.Status(context.Background())
				errs <- err
				return
			}
			token, err := g.Token(context.Background())
			if err == nil && token != fakeToken {
				err = errors.New("unexpected token")
			}
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}

// callMethod calls g.Status or g.Token and returns only the error.
func callMethod(ctx context.Context, g GH, method string) error {
	if method == "Status" {
		_, err := g.Status(ctx)
		return err
	}
	_, err := g.Token(ctx)
	return err
}

// assertProcessGone checks that the blocked fake gh (hang scenario) was killed.
func assertProcessGone(t *testing.T, env fakeEnv) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // os.Process.Signal(0) does not exist on Windows
	}
	data, err := os.ReadFile(env.pid)
	if err != nil {
		t.Logf("PID of the fake gh unavailable (start slower than the timeout?): %v", err)
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("unreadable PID %q: %v", data, err)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if err := p.Signal(syscall.Signal(0)); err == nil {
		_ = p.Kill()
		t.Errorf("the fake gh (PID %d) is still running after the timeout", pid)
	}
}

// containsSeq reports whether args contains a immediately followed by b.
func containsSeq(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}
