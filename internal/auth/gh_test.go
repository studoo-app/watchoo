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
		t.Fatalf("Status : %v", err)
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
		t.Errorf("Status = %+v, veut %+v", st, want)
	}

	calls := readCalls(t, env)
	if len(calls) != 1 {
		t.Fatalf("%d appels à gh, veut 1", len(calls))
	}
	wantArgs := []string{"auth", "status", "--json", "hosts", "--hostname", "github.com"}
	if !reflect.DeepEqual(calls[0].Args, wantArgs) {
		t.Errorf("arguments = %q, veut %q", calls[0].Args, wantArgs)
	}
}

func TestTokenLoggedIn(t *testing.T) {
	env := useScenario(t, "logged-in")

	token, err := fakeGH().Token(context.Background())
	if err != nil {
		t.Fatalf("Token : %v", err)
	}
	if token != fakeToken {
		t.Errorf("Token a renvoyé une valeur inattendue (longueur %d)", len(token))
	}

	calls := readCalls(t, env)
	if len(calls) != 1 {
		t.Fatalf("%d appels à gh, veut 1", len(calls))
	}
	wantArgs := []string{"auth", "token", "--hostname", "github.com"}
	if !reflect.DeepEqual(calls[0].Args, wantArgs) {
		t.Errorf("arguments = %q, veut %q", calls[0].Args, wantArgs)
	}
}

func TestEnvironmentPassedToGH(t *testing.T) {
	env := useScenario(t, "logged-in")
	// Des valeurs contraires dans l'environnement hérité doivent être remplacées.
	t.Setenv("GH_PROMPT_DISABLED", "0")
	t.Setenv("GH_NO_UPDATE_NOTIFIER", "")

	g := fakeGH()
	if _, err := g.Status(context.Background()); err != nil {
		t.Fatalf("Status : %v", err)
	}
	if _, err := g.Token(context.Background()); err != nil {
		t.Fatalf("Token : %v", err)
	}
	calls := readCalls(t, env)
	if len(calls) != 2 {
		t.Fatalf("%d appels à gh, veut 2", len(calls))
	}
	for _, c := range calls {
		for _, key := range []string{"GH_PROMPT_DISABLED", "GH_NO_UPDATE_NOTIFIER"} {
			if got := c.Env[key]; got != "1" {
				t.Errorf("appel %q : %s = %q, veut \"1\"", c.Args, key, got)
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
		t.Fatalf("Status : %v", err)
	}
	if st.Host != "ghe.example.test" {
		t.Errorf("Host = %q, veut ghe.example.test", st.Host)
	}
	if _, err := g.Token(context.Background()); err != nil {
		t.Fatalf("Token : %v", err)
	}
	for _, c := range readCalls(t, env) {
		if !containsSeq(c.Args, "--hostname", "ghe.example.test") {
			t.Errorf("arguments = %q, veut --hostname ghe.example.test", c.Args)
		}
	}
}

func TestNotLoggedIn(t *testing.T) {
	useScenario(t, "not-logged-in")
	g := fakeGH()

	st, err := g.Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if st.Login != "" {
		t.Errorf("Status.Login = %q, veut vide", st.Login)
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("le message %q devrait indiquer « gh auth login »", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
}

func TestExitNotLoggedIn(t *testing.T) {
	useScenario(t, "exit-not-logged-in")

	_, err := fakeGH().Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if errors.Is(err, ErrUnexpectedOutput) {
		t.Errorf("l'erreur %q ne devrait pas être une réponse inattendue", err)
	}
	if !strings.Contains(err.Error(), "You are not logged into any GitHub hosts") {
		t.Errorf("le message %q devrait reprendre la sortie d'erreur de gh", err)
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
				t.Fatalf("Status : %v", err)
			}
			if st.Login != tt.wantLogin || st.Active != tt.wantActive || !reflect.DeepEqual(st.Scopes, tt.wantScopes) {
				t.Errorf("Status = %+v, veut login %q, active %v, scopes %q", st, tt.wantLogin, tt.wantActive, tt.wantScopes)
			}

			token, err := g.Token(context.Background())
			if err != nil {
				t.Fatalf("Token : %v", err)
			}
			if token != tt.wantToken {
				t.Errorf("Token n'a pas renvoyé le token du compte %q", tt.wantLogin)
			}

			calls := readCalls(t, env)
			if len(calls) != 2 {
				t.Fatalf("%d appels à gh, veut 2", len(calls))
			}
			hasUser := containsSeq(calls[1].Args, "--user", tt.user)
			if tt.user != "" && !hasUser {
				t.Errorf("arguments de auth token = %q, veut --user %s", calls[1].Args, tt.user)
			}
			if tt.user == "" && slices.Contains(calls[1].Args, "--user") {
				t.Errorf("arguments de auth token = %q, sans --user attendu", calls[1].Args)
			}
		})
	}
}

// gh distingue les majuscules dans « gh auth token --user » : un github.user écrit avec une autre
// casse doit être refusé dès Status, sinon init réussirait et chaque Token échouerait ensuite.
func TestMultiAccountUserWrongCase(t *testing.T) {
	useScenario(t, "multi-account")
	g := fakeGH()
	g.User = strings.ToUpper(otherLogin)

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrUnusableUser)
	wantErrIs(t, err, ErrNotLoggedIn)
	if !strings.Contains(err.Error(), `"`+otherLogin+`"`) || !strings.Contains(err.Error(), "majuscules") {
		t.Errorf("le message %q devrait donner le login exact connu de gh", err)
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
		t.Errorf("le message %q devrait citer le compte et le réglage github.user", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if !strings.Contains(err.Error(), `"inconnu"`) {
		t.Errorf("le message %q devrait citer le compte", err)
	}
}

func TestEnvToken(t *testing.T) {
	useScenario(t, "env-token")

	// Sans github.user : compte actif, c'est-à-dire le token de GH_TOKEN.
	g := fakeGH()
	st, err := g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status : %v", err)
	}
	if st.TokenSource != "GH_TOKEN" || !st.Active {
		t.Errorf("Status = %+v, veut le compte actif issu de GH_TOKEN", st)
	}
	if token, err := g.Token(context.Background()); err != nil || token != envToken {
		t.Errorf("Token sans --user : erreur %v ou token inattendu", err)
	}

	// Avec github.user : gh auth token --user lit le compte enregistré, Status doit le décrire.
	g.User = fakeLogin
	st, err = g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status : %v", err)
	}
	if st.TokenSource != "keyring" {
		t.Errorf("Status = %+v, veut le compte enregistré (keyring)", st)
	}
	if token, err := g.Token(context.Background()); err != nil || token != fakeToken {
		t.Errorf("Token avec --user : erreur %v ou token inattendu", err)
	}
}

// Docker ou CI : GH_TOKEN défini, aucun compte enregistré. Sans github.user, Status et Token
// utilisent le token de GH_TOKEN ; avec github.user, « gh auth token --user » ignore GH_TOKEN et
// échoue, donc Status doit déjà refuser (avec un message qui dit de vider github.user).
func TestEnvTokenOnly(t *testing.T) {
	env := useScenario(t, "env-token-only")

	g := fakeGH()
	st, err := g.Status(context.Background())
	if err != nil {
		t.Fatalf("Status : %v", err)
	}
	if st.Login != fakeLogin || st.TokenSource != "GH_TOKEN" || !st.Active {
		t.Errorf("Status = %+v, veut le compte actif issu de GH_TOKEN", st)
	}
	if token, err := g.Token(context.Background()); err != nil || token != envToken {
		t.Errorf("Token sans --user : erreur %v ou token inattendu", err)
	}

	g.User = fakeLogin
	_, err = g.Status(context.Background())
	wantErrIs(t, err, ErrUnusableUser)
	for _, part := range []string{"github.user", "GH_TOKEN", "videz-le"} {
		if !strings.Contains(err.Error(), part) {
			t.Errorf("le message %q devrait contenir %q", err, part)
		}
	}
	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)

	calls := readCalls(t, env)
	if len(calls) != 4 || !containsSeq(calls[3].Args, "--user", fakeLogin) {
		t.Errorf("appels = %q, veut un dernier « auth token --user %s »", calls, fakeLogin)
	}
}

// TestStatusAndTokenAgree vérifie le contrat de Client sur le choix du compte : quand Status réussit,
// Token renvoie le token de ce compte ; quand Status ne trouve pas de compte utilisable, Token échoue
// aussi. Sinon « watchoo init » validerait un compte dont la synchro ne pourrait jamais lire le token.
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
					t.Fatalf("Status a réussi (compte %q) mais Token échoue : %v", st.Login, tokenErr)
				case statusErr == nil && st.TokenSource != "GH_TOKEN" && token != tokens[st.Login]:
					t.Errorf("Token n'a pas renvoyé le token du compte %q décrit par Status", st.Login)
				case statusErr == nil && st.TokenSource == "GH_TOKEN" && token != envToken:
					t.Errorf("Token n'a pas renvoyé le token de GH_TOKEN décrit par Status")
				case statusErr != nil && !errors.Is(statusErr, ErrNotLoggedIn):
					t.Fatalf("Status : erreur inattendue %v", statusErr)
				case statusErr != nil && tokenErr == nil:
					t.Errorf("Status échoue (%v) mais Token réussit", statusErr)
				}
			})
		}
	}
}

// GitHub injoignable (réseau, proxy, panne) : gh répond « error » avec une erreur réseau, même avec un
// token valide. Ce n'est pas un problème de connexion à gh : pas de conseil « gh auth login ».
func TestOffline(t *testing.T) {
	useScenario(t, "offline")
	g := fakeGH()

	st, err := g.Status(context.Background())
	wantErrIs(t, err, ErrCheckFailed)
	if errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("l'erreur %q ne devrait pas dire que gh n'est pas connecté", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "gh auth login") {
		t.Errorf("le message %q ne devrait pas conseiller « gh auth login »", msg)
	}
	if !strings.Contains(msg, "proxyconnect tcp") || !strings.Contains(msg, fakeLogin) {
		t.Errorf("le message %q devrait citer le compte et l'erreur de gh", msg)
	}
	if strings.Contains(msg, fakeToken) || !strings.Contains(msg, redacted) {
		t.Errorf("le message %q devrait masquer le token présent dans l'erreur de gh", msg)
	}
	if st.Login != fakeLogin || st.State != "error" {
		t.Errorf("Status partiel = %+v, veut login %q et état error", st, fakeLogin)
	}

	// gh auth token ne contacte pas GitHub : il fonctionne hors ligne.
	if token, err := g.Token(context.Background()); err != nil || token != fakeToken {
		t.Errorf("Token hors ligne : erreur %v ou token inattendu", err)
	}
}

func TestInvalidToken(t *testing.T) {
	useScenario(t, "invalid-token")

	st, err := fakeGH().Status(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if errors.Is(err, ErrCheckFailed) {
		t.Errorf("un token refusé (HTTP 401) n'est pas une vérification impossible : %v", err)
	}
	// Le Status partiel est renvoyé pour afficher le compte concerné.
	if st.Login != fakeLogin || st.State != "error" {
		t.Errorf("Status partiel = %+v, veut login %q et état error", st, fakeLogin)
	}
	if !strings.Contains(err.Error(), fakeLogin) || !strings.Contains(err.Error(), "error") {
		t.Errorf("le message %q devrait citer le compte et son état", err)
	}
}

func TestTokenRotation(t *testing.T) {
	useScenario(t, "token-rotation")
	g := fakeGH()

	first, err := g.Token(context.Background())
	if err != nil {
		t.Fatalf("premier Token : %v", err)
	}
	second, err := g.Token(context.Background())
	if err != nil {
		t.Fatalf("second Token : %v", err)
	}
	if first == second {
		t.Error("le token a changé dans gh, mais Token a renvoyé deux fois le même")
	}
	if first != fakeToken+"_1" || second != fakeToken+"_2" {
		t.Error("tokens inattendus : chaque appel doit redemander le token à gh")
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
		t.Errorf("le message %q ne doit pas reprendre la sortie standard de gh", err)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)
	if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), "warning") {
		t.Errorf("le message %q ne doit pas reprendre la sortie standard de gh", err)
	}
}

func TestGHError(t *testing.T) {
	useScenario(t, "error")
	g := fakeGH()

	_, err := g.Status(context.Background())
	wantErrIs(t, err, ErrUnexpectedOutput)
	msg := err.Error()
	if !strings.Contains(msg, "code 2") || !strings.Contains(msg, "erreur simulée de gh") {
		t.Errorf("le message %q devrait donner le code de retour et la première ligne de stderr", msg)
	}
	if strings.Contains(msg, "second line") {
		t.Errorf("le message %q ne devrait reprendre que la première ligne de stderr", msg)
	}
	if strings.Contains(msg, fakeToken) || !strings.Contains(msg, redacted) {
		t.Errorf("le message %q devrait masquer le token présent dans stderr", msg)
	}

	_, err = g.Token(context.Background())
	wantErrIs(t, err, ErrNotLoggedIn)
	if strings.Contains(err.Error(), "erreur simulée") {
		t.Errorf("le message %q ne doit pas reprendre la sortie d'erreur de « gh auth token »", err)
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
				t.Errorf("l'erreur %q ne devrait pas être celle du contexte de l'appelant", err)
			}
			if !strings.Contains(err.Error(), "500ms") {
				t.Errorf("le message %q devrait donner le délai", err)
			}
			if elapsed > g.Timeout+waitDelay+3*time.Second {
				t.Errorf("%s a duré %s, le délai de %s n'a pas été appliqué", method, elapsed, g.Timeout)
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
		t.Errorf("une annulation par l'appelant n'est pas un dépassement du délai : %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Status a duré %s après l'annulation", elapsed)
	}

	// Contexte déjà annulé : gh n'est même pas lancé utilement.
	_, err = g.Token(ctx)
	wantErrIs(t, err, context.Canceled)
}

func TestCallerDeadline(t *testing.T) {
	useScenario(t, "hang")
	g := fakeGH() // délai de GH : 10 s

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_, err := g.Token(ctx)
	wantErrIs(t, err, context.DeadlineExceeded)
	if errors.Is(err, ErrTimeout) {
		t.Errorf("le délai venait de l'appelant, pas de GH.Timeout : %v", err)
	}
}

func TestGHNotFound(t *testing.T) {
	useScenario(t, "logged-in")
	paths := map[string]string{
		"chemin absent":  filepath.Join(t.TempDir(), "absent", "gh"),
		"absent du PATH": "watchoo-gh-introuvable",
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
		t.Skip("droits d'exécution Unix")
	}
	useScenario(t, "logged-in")
	path := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(path, []byte("pas un exécutable"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := GH{Path: path}.Status(context.Background())
	if err == nil {
		t.Fatal("Status a réussi avec un gh non exécutable")
	}
	if !strings.Contains(err.Error(), "lancement de") {
		t.Errorf("le message %q devrait indiquer que gh n'a pas pu être lancé", err)
	}
}

func TestDefaults(t *testing.T) {
	var g GH
	if g.path() != DefaultPath || g.host() != DefaultHost || g.timeout() != DefaultTimeout {
		t.Errorf("valeurs par défaut = %q, %q, %s", g.path(), g.host(), g.timeout())
	}
	g.Timeout = -time.Second
	if g.timeout() != DefaultTimeout {
		t.Errorf("délai négatif : %s, veut %s", g.timeout(), DefaultTimeout)
	}
	g = GH{Path: "/opt/gh", Host: "ghe.example.test", Timeout: time.Second}
	if g.path() != "/opt/gh" || g.host() != "ghe.example.test" || g.timeout() != time.Second {
		t.Errorf("valeurs réglées ignorées : %q, %q, %s", g.path(), g.host(), g.timeout())
	}
}

func TestDefaultPathUsesPATH(t *testing.T) {
	env := useScenario(t, "logged-in")
	// Seul le répertoire du faux gh est dans le PATH : GH{} doit le trouver sous le nom « gh ».
	t.Setenv("PATH", filepath.Dir(fakeGHPath))

	var c Client = &GH{}
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status : %v", err)
	}
	if st.Login != fakeLogin {
		t.Errorf("Login = %q, veut %q", st.Login, fakeLogin)
	}
	if len(readCalls(t, env)) != 1 {
		t.Error("le faux gh du PATH n'a pas été appelé")
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
				err = errors.New("token inattendu")
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

// callMethod appelle g.Status ou g.Token et renvoie seulement l'erreur.
func callMethod(ctx context.Context, g GH, method string) error {
	if method == "Status" {
		_, err := g.Status(ctx)
		return err
	}
	_, err := g.Token(ctx)
	return err
}

// assertProcessGone vérifie que le faux gh bloqué (scénario hang) a bien été tué.
func assertProcessGone(t *testing.T, env fakeEnv) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return // os.Process.Signal(0) n'existe pas sous Windows
	}
	data, err := os.ReadFile(env.pid)
	if err != nil {
		t.Logf("PID du faux gh indisponible (démarrage plus lent que le délai ?) : %v", err)
		return
	}
	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("PID illisible %q : %v", data, err)
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return
	}
	if err := p.Signal(syscall.Signal(0)); err == nil {
		_ = p.Kill()
		t.Errorf("le faux gh (PID %d) tourne encore après le délai", pid)
	}
}

// containsSeq indique si args contient a immédiatement suivi de b.
func containsSeq(args []string, a, b string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == a && args[i+1] == b {
			return true
		}
	}
	return false
}
