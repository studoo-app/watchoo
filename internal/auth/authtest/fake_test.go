package authtest

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/studoo-app/watchoo/internal/auth"
)

func TestZeroValueIsLoggedOut(t *testing.T) {
	var f Fake
	_, err := f.Status(context.Background())
	if !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("Status : erreur = %v, veut ErrNotLoggedIn", err)
	}
	_, err = f.Token(context.Background())
	if !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("Token : erreur = %v, veut ErrNotLoggedIn", err)
	}
}

func TestNewLoggedOut(t *testing.T) {
	f := NewLoggedOut()
	st, err := f.Status(context.Background())
	if !errors.Is(err, auth.ErrNotLoggedIn) || !reflect.DeepEqual(st, auth.Status{}) {
		t.Errorf("Status = %+v, %v ; veut un Status vide et ErrNotLoggedIn", st, err)
	}
	if _, err := f.Token(context.Background()); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("Token : erreur = %v, veut ErrNotLoggedIn", err)
	}
}

func TestNewLoggedIn(t *testing.T) {
	f := NewLoggedIn("")
	st, err := f.Status(context.Background())
	if err != nil {
		t.Fatalf("Status : %v", err)
	}
	want := auth.Status{
		Host:        auth.DefaultHost,
		Login:       FakeLogin,
		TokenSource: "keyring",
		Active:      true,
		State:       "success",
		Scopes:      []string{"gist", "read:org", "repo"},
	}
	if !reflect.DeepEqual(st, want) {
		t.Errorf("Status = %+v, veut %+v", st, want)
	}
	token, err := f.Token(context.Background())
	if err != nil || token != FakeToken {
		t.Errorf("Token = %q, %v ; veut FakeToken", token, err)
	}

	st, err = NewLoggedIn("alice").Status(context.Background())
	if err != nil || st.Login != "alice" {
		t.Errorf("NewLoggedIn(\"alice\") : Status = %+v, %v", st, err)
	}
}

func TestSetStatus(t *testing.T) {
	f := NewLoggedOut()
	custom := auth.Status{Host: "github.com", Login: "bob", Active: true, State: "success", Scopes: []string{"repo"}}
	f.SetStatus(custom, nil)
	st, err := f.Status(context.Background())
	if err != nil || !reflect.DeepEqual(st, custom) {
		t.Errorf("Status = %+v, %v ; veut %+v", st, err, custom)
	}

	// Une erreur renvoie aussi le Status partiel (ex. login d'un token invalide).
	boom := errors.New("token révoqué")
	partial := auth.Status{Login: "bob", State: "error"}
	f.SetStatus(partial, boom)
	st, err = f.Status(context.Background())
	if !errors.Is(err, boom) || st.Login != "bob" || st.State != "error" {
		t.Errorf("Status = %+v, %v ; veut le Status partiel et l'erreur", st, err)
	}
}

func TestStatusScopesAreCopies(t *testing.T) {
	scopes := []string{"repo", "gist"}
	f := NewLoggedOut()
	f.SetStatus(auth.Status{Login: "bob", State: "success", Scopes: scopes}, nil)
	scopes[0] = "modifié par le test"

	st, err := f.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Scopes[0] != "repo" {
		t.Errorf("SetStatus partage la liste de l'appelant : %q", st.Scopes)
	}
	st.Scopes[0] = "modifié par l'appelant"
	st, _ = f.Status(context.Background())
	if st.Scopes[0] != "repo" {
		t.Errorf("Status partage la liste interne : %q", st.Scopes)
	}

	f.SetStatus(auth.Status{Scopes: []string{"repo"}}, errors.New("échec"))
	st, _ = f.Status(context.Background())
	st.Scopes[0] = "modifié"
	st, _ = f.Status(context.Background())
	if st.Scopes[0] != "repo" {
		t.Errorf("Status en erreur partage la liste interne : %q", st.Scopes)
	}
}

func TestSetTokensRotation(t *testing.T) {
	f := NewLoggedIn("")
	f.SetTokens("t1", "t2", "t3")
	var got []string
	for range 5 {
		token, err := f.Token(context.Background())
		if err != nil {
			t.Fatalf("Token : %v", err)
		}
		got = append(got, token)
	}
	want := []string{"t1", "t2", "t3", "t3", "t3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %q, veut %q (le dernier est répété)", got, want)
	}

	f.SetTokens()
	if _, err := f.Token(context.Background()); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("SetTokens() sans argument : erreur = %v, veut ErrNotLoggedIn", err)
	}
}

func TestSetTokensCopiesArguments(t *testing.T) {
	tokens := []string{"t1", "t2"}
	f := NewLoggedIn("")
	f.SetTokens(tokens...)
	tokens[0] = "modifié"
	if token, _ := f.Token(context.Background()); token != "t1" {
		t.Errorf("SetTokens partage la liste de l'appelant : %q", token)
	}
}

func TestSetTokenErr(t *testing.T) {
	f := NewLoggedIn("")
	f.SetTokenErr(auth.ErrTimeout)
	if _, err := f.Token(context.Background()); !errors.Is(err, auth.ErrTimeout) {
		t.Errorf("erreur = %v, veut ErrTimeout", err)
	}
	f.SetTokenErr(nil)
	if token, err := f.Token(context.Background()); err != nil || token != FakeToken {
		t.Errorf("après SetTokenErr(nil) : %q, %v ; veut FakeToken", token, err)
	}

	// SetTokens efface une erreur en cours.
	f.SetTokenErr(auth.ErrNotLoggedIn)
	f.SetTokens("nouveau")
	if token, err := f.Token(context.Background()); err != nil || token != "nouveau" {
		t.Errorf("après SetTokens : %q, %v ; veut \"nouveau\"", token, err)
	}
}

func TestCanceledContext(t *testing.T) {
	f := NewLoggedIn("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Status(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Status : erreur = %v, veut context.Canceled", err)
	}
	if _, err := f.Token(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Token : erreur = %v, veut context.Canceled", err)
	}
	// Les appels annulés sont comptés.
	if f.StatusCalls() != 1 || f.TokenCalls() != 1 {
		t.Errorf("compteurs = %d, %d ; veut 1, 1", f.StatusCalls(), f.TokenCalls())
	}
}

func TestCallCounters(t *testing.T) {
	f := NewLoggedIn("")
	for range 3 {
		_, _ = f.Status(context.Background())
	}
	for range 2 {
		_, _ = f.Token(context.Background())
	}
	if f.StatusCalls() != 3 || f.TokenCalls() != 2 {
		t.Errorf("compteurs = %d, %d ; veut 3, 2", f.StatusCalls(), f.TokenCalls())
	}
}

// TestConcurrentUse est surtout utile avec -race.
func TestConcurrentUse(t *testing.T) {
	f := NewLoggedIn("")
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			switch i % 4 {
			case 0:
				_, _ = f.Status(context.Background())
			case 1:
				_, _ = f.Token(context.Background())
			case 2:
				f.SetTokens(FakeToken, FakeToken+"_2")
			default:
				f.SetStatus(auth.Status{Login: FakeLogin, State: "success"}, nil)
			}
		})
	}
	wg.Wait()
	if f.StatusCalls() != 5 || f.TokenCalls() != 5 {
		t.Errorf("compteurs = %d, %d ; veut 5, 5", f.StatusCalls(), f.TokenCalls())
	}
}
