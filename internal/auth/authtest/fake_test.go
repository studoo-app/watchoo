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
		t.Errorf("Status: error = %v, want ErrNotLoggedIn", err)
	}
	_, err = f.Token(context.Background())
	if !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("Token: error = %v, want ErrNotLoggedIn", err)
	}
}

func TestNewLoggedOut(t *testing.T) {
	f := NewLoggedOut()
	st, err := f.Status(context.Background())
	if !errors.Is(err, auth.ErrNotLoggedIn) || !reflect.DeepEqual(st, auth.Status{}) {
		t.Errorf("Status = %+v, %v; want an empty Status and ErrNotLoggedIn", st, err)
	}
	if _, err := f.Token(context.Background()); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("Token: error = %v, want ErrNotLoggedIn", err)
	}
}

func TestNewLoggedIn(t *testing.T) {
	f := NewLoggedIn("")
	st, err := f.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
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
		t.Errorf("Status = %+v, want %+v", st, want)
	}
	token, err := f.Token(context.Background())
	if err != nil || token != FakeToken {
		t.Errorf("Token = %q, %v; want FakeToken", token, err)
	}

	st, err = NewLoggedIn("alice").Status(context.Background())
	if err != nil || st.Login != "alice" {
		t.Errorf("NewLoggedIn(\"alice\"): Status = %+v, %v", st, err)
	}
}

func TestSetStatus(t *testing.T) {
	f := NewLoggedOut()
	custom := auth.Status{Host: "github.com", Login: "bob", Active: true, State: "success", Scopes: []string{"repo"}}
	f.SetStatus(custom, nil)
	st, err := f.Status(context.Background())
	if err != nil || !reflect.DeepEqual(st, custom) {
		t.Errorf("Status = %+v, %v; want %+v", st, err, custom)
	}

	// An error also returns the partial Status (e.g. login of an invalid token).
	boom := errors.New("revoked token")
	partial := auth.Status{Login: "bob", State: "error"}
	f.SetStatus(partial, boom)
	st, err = f.Status(context.Background())
	if !errors.Is(err, boom) || st.Login != "bob" || st.State != "error" {
		t.Errorf("Status = %+v, %v; want the partial Status and the error", st, err)
	}
}

func TestStatusScopesAreCopies(t *testing.T) {
	scopes := []string{"repo", "gist"}
	f := NewLoggedOut()
	f.SetStatus(auth.Status{Login: "bob", State: "success", Scopes: scopes}, nil)
	scopes[0] = "changed by the test"

	st, err := f.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Scopes[0] != "repo" {
		t.Errorf("SetStatus shares the caller's list: %q", st.Scopes)
	}
	st.Scopes[0] = "changed by the caller"
	st, _ = f.Status(context.Background())
	if st.Scopes[0] != "repo" {
		t.Errorf("Status shares the internal list: %q", st.Scopes)
	}

	f.SetStatus(auth.Status{Scopes: []string{"repo"}}, errors.New("failure"))
	st, _ = f.Status(context.Background())
	st.Scopes[0] = "changed"
	st, _ = f.Status(context.Background())
	if st.Scopes[0] != "repo" {
		t.Errorf("Status in error shares the internal list: %q", st.Scopes)
	}
}

func TestSetTokensRotation(t *testing.T) {
	f := NewLoggedIn("")
	f.SetTokens("t1", "t2", "t3")
	var got []string
	for range 5 {
		token, err := f.Token(context.Background())
		if err != nil {
			t.Fatalf("Token: %v", err)
		}
		got = append(got, token)
	}
	want := []string{"t1", "t2", "t3", "t3", "t3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokens = %q, want %q (the last one is repeated)", got, want)
	}

	f.SetTokens()
	if _, err := f.Token(context.Background()); !errors.Is(err, auth.ErrNotLoggedIn) {
		t.Errorf("SetTokens() without arguments: error = %v, want ErrNotLoggedIn", err)
	}
}

func TestSetTokensCopiesArguments(t *testing.T) {
	tokens := []string{"t1", "t2"}
	f := NewLoggedIn("")
	f.SetTokens(tokens...)
	tokens[0] = "changed"
	if token, _ := f.Token(context.Background()); token != "t1" {
		t.Errorf("SetTokens shares the caller's list: %q", token)
	}
}

func TestSetTokenErr(t *testing.T) {
	f := NewLoggedIn("")
	f.SetTokenErr(auth.ErrTimeout)
	if _, err := f.Token(context.Background()); !errors.Is(err, auth.ErrTimeout) {
		t.Errorf("error = %v, want ErrTimeout", err)
	}
	f.SetTokenErr(nil)
	if token, err := f.Token(context.Background()); err != nil || token != FakeToken {
		t.Errorf("after SetTokenErr(nil): %q, %v; want FakeToken", token, err)
	}

	// SetTokens clears a pending error.
	f.SetTokenErr(auth.ErrNotLoggedIn)
	f.SetTokens("new")
	if token, err := f.Token(context.Background()); err != nil || token != "new" {
		t.Errorf("after SetTokens: %q, %v; want \"new\"", token, err)
	}
}

func TestCanceledContext(t *testing.T) {
	f := NewLoggedIn("")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Status(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Status: error = %v, want context.Canceled", err)
	}
	if _, err := f.Token(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Token: error = %v, want context.Canceled", err)
	}
	// Canceled calls are counted.
	if f.StatusCalls() != 1 || f.TokenCalls() != 1 {
		t.Errorf("counters = %d, %d; want 1, 1", f.StatusCalls(), f.TokenCalls())
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
		t.Errorf("counters = %d, %d; want 3, 2", f.StatusCalls(), f.TokenCalls())
	}
}

// TestConcurrentUse is mostly useful with -race.
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
		t.Errorf("counters = %d, %d; want 5, 5", f.StatusCalls(), f.TokenCalls())
	}
}
