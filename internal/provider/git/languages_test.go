package git_test

// Languages (1.1.0) against real repositories: default branch only, empty repository, invalid
// reference, cancellation.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

func TestLanguages(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	m0 := r.Commit("main", "main.go", strings.Repeat("g", 300), "go", alice, t0)
	r.Commit("main", "index.html", strings.Repeat("h", 100), "html", alice, t0)
	r.Commit("main", "README.md", strings.Repeat("r", 500), "doc", alice, t0)
	r.Commit("main", "node_modules/lib.js", strings.Repeat("n", 900), "deps", alice, t0)
	r.Branch("dev", m0)
	r.Commit("dev", "autre.py", strings.Repeat("p", 800), "branche", alice, t0) // not on the default branch

	p, _, _ := newProvider(t, srv)
	got, err := p.Languages(t.Context(), "alice", "projet")
	if err != nil {
		t.Fatal(err)
	}
	want := []provider.Language{{Name: "Go", Bytes: 300}, {Name: "HTML", Bytes: 100}}
	if !slices.Equal(got, want) {
		t.Errorf("Languages = %+v, want %+v", got, want)
	}
}

func TestLanguagesEmptyRepository(t *testing.T) {
	srv := gittest.NewServer(t)
	srv.Create("alice", "vide", "main")
	p, _, _ := newProvider(t, srv)
	got, err := p.Languages(t.Context(), "alice", "vide")
	if err != nil || got == nil || len(got) != 0 {
		t.Errorf("empty repository: %#v, %v; want an empty non-nil list", got, err)
	}
}

func TestLanguagesErrors(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	r.Commit("main", "main.go", "package main\n", "go", alice, t0)
	p, _, _ := newProvider(t, srv)
	if _, err := p.Languages(t.Context(), "alice", "../x"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid name: %v", err)
	}
	if _, err := p.Languages(t.Context(), "alice", "absent"); !errors.Is(err, provider.ErrAccessDenied) {
		t.Errorf("missing repository: %v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.Languages(ctx, "alice", "projet"); err == nil {
		t.Error("canceled context: no error")
	}
}
