// Package authtest fournit une fausse implémentation en mémoire de auth.Client, pour les tests des
// autres packages (synchro, CLI, web, TUI). Aucun processus n'est lancé.
package authtest

import (
	"context"
	"sync"

	"github.com/studoo-app/watchoo/internal/auth"
)

// FakeToken est le faux token renvoyé par défaut. Les tests vérifient qu'il n'apparaît jamais dans les
// logs ni dans les sorties CLI, web ou TUI.
const FakeToken = "gho_fake_for_tests"

// FakeLogin est le login renvoyé par défaut par NewLoggedIn("").
const FakeLogin = "watchoo-test"

// Fake est un auth.Client en mémoire, sûr pour un usage concurrent. Sa valeur zéro n'est connectée à
// aucun compte : Status et Token renvoient une erreur enveloppant auth.ErrNotLoggedIn.
type Fake struct {
	mu          sync.Mutex
	status      auth.Status
	statusErr   error
	tokens      []string
	tokenErr    error
	statusCalls int
	tokenCalls  int
	loggedIn    bool
}

// Vérifie à la compilation que *Fake implémente auth.Client.
var _ auth.Client = (*Fake)(nil)

// NewLoggedIn renvoie un Fake connecté au compte login (FakeLogin si login est vide), sur github.com,
// avec les droits « gist, read:org, repo » et le token FakeToken.
func NewLoggedIn(login string) *Fake {
	if login == "" {
		login = FakeLogin
	}
	return &Fake{
		loggedIn: true,
		status: auth.Status{
			Host:        auth.DefaultHost,
			Login:       login,
			TokenSource: "keyring",
			Active:      true,
			State:       "success",
			Scopes:      []string{"gist", "read:org", "repo"},
		},
		tokens: []string{FakeToken},
	}
}

// NewLoggedOut renvoie un Fake non connecté : Status et Token renvoient une erreur enveloppant
// auth.ErrNotLoggedIn.
func NewLoggedOut() *Fake {
	return &Fake{}
}

// SetStatus fixe le résultat des prochains appels à Status (err non nil : Status renvoie s et err).
func (f *Fake) SetStatus(s auth.Status, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.status = copyStatus(s)
	f.statusErr = err
	f.loggedIn = err == nil
}

// SetTokens fixe les tokens renvoyés par les prochains appels à Token, dans l'ordre ; le dernier est
// ensuite renvoyé indéfiniment. Permet de simuler un token changé entre deux synchros. Sans argument,
// Token renvoie une erreur enveloppant auth.ErrNotLoggedIn.
func (f *Fake) SetTokens(tokens ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokens = append([]string(nil), tokens...)
	f.tokenErr = nil
}

// SetTokenErr fait échouer les prochains appels à Token avec err (nil pour revenir aux tokens).
func (f *Fake) SetTokenErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenErr = err
}

// Status implémente auth.Client.
func (f *Fake) Status(ctx context.Context) (auth.Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statusCalls++
	if err := ctx.Err(); err != nil {
		return auth.Status{}, err
	}
	if f.statusErr != nil {
		return copyStatus(f.status), f.statusErr
	}
	if !f.loggedIn {
		return auth.Status{}, auth.ErrNotLoggedIn
	}
	return copyStatus(f.status), nil
}

// copyStatus renvoie une copie de s dont la liste Scopes n'est pas partagée : ni l'appelant ni le
// test ne peuvent modifier l'état du Fake par ce biais.
func copyStatus(s auth.Status) auth.Status {
	s.Scopes = append([]string(nil), s.Scopes...)
	return s
}

// Token implémente auth.Client.
func (f *Fake) Token(ctx context.Context) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tokenCalls++
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f.tokenErr != nil {
		return "", f.tokenErr
	}
	if len(f.tokens) == 0 {
		return "", auth.ErrNotLoggedIn
	}
	t := f.tokens[0]
	if len(f.tokens) > 1 {
		f.tokens = f.tokens[1:]
	}
	return t, nil
}

// StatusCalls renvoie le nombre d'appels à Status.
func (f *Fake) StatusCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statusCalls
}

// TokenCalls renvoie le nombre d'appels à Token.
func (f *Fake) TokenCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tokenCalls
}
