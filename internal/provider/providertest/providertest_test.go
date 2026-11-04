package providertest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// t0 is the origin of the tests' dummy dates.
var t0 = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

var ctx = context.Background()

// graph is a history with a merged branch, in the repository 101 alice/projet:
//
//	r1 ── m1 ── m2 ─────── M   (main)
//	  └── f1 ── f2 ──────┘     (feature: f2)
//
// commit dates: r1 t0, m1 t0+1h, f1 t0+2h, m2 t0+3h, f2 t0+4h, M t0+5h.
type graph struct {
	r1, m1, f1, m2, f2, merge string
}

// alice and bob are dummy people.
var (
	alice = provider.Person{Name: "Alice Test", Email: "alice@example.test", Login: "alice", ID: 1, AvatarURL: "https://avatars.example.test/u/1"}
	bob   = provider.Person{Name: "Bob Test", Email: "bob@example.test"}
)

// newGraph returns a Fake with the repository 101 alice/projet and the graph history.
func newGraph(t *testing.T) (*Fake, graph) {
	t.Helper()
	f := New()
	f.AddRepository(provider.Repository{ID: 101, Owner: "alice", Name: "projet", Private: true})
	g := graph{r1: SHA("r1"), m1: SHA("m1"), f1: SHA("f1"), m2: SHA("m2"), f2: SHA("f2"), merge: SHA("merge")}
	commit := func(sha, msg string, h int, author provider.Person, parents ...string) provider.Commit {
		at := t0.Add(time.Duration(h) * time.Hour)
		return provider.Commit{SHA: sha, Message: msg, URL: "https://github.com/alice/projet/commit/" + sha,
			Author: author, Committer: author, AuthoredAt: at, CommittedAt: at, Parents: parents}
	}
	f.AddCommits(101,
		commit(g.r1, "racine", 0, alice),
		commit(g.m1, "m1", 1, alice, g.r1),
		commit(g.f1, "f1", 2, bob, g.r1),
		commit(g.m2, "m2", 3, alice, g.m1),
		commit(g.f2, "f2", 4, bob, g.f1),
		commit(g.merge, "fusion", 5, alice, g.m2, g.f2),
	)
	f.SetBranch(101, "main", g.merge)
	f.SetBranch(101, "feature", g.f2)
	return f, g
}

// shas returns the SHAs of a list of commits.
func shas(cs []provider.Commit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.SHA
	}
	return out
}

// expectPanic checks that fn panics.
func expectPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s: no panic", what)
		}
	}()
	fn()
}

// wantAPIError checks that err is a *provider.APIError with code status wrapping target.
func wantAPIError(t *testing.T, what string, err error, status int, target error) {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, target) || apiErr.Status != status || apiErr.Method != "GET" ||
		apiErr.Op == "" || !strings.HasPrefix(apiErr.Path, "/") {
		t.Errorf("%s: error %#v (%v), want *APIError HTTP %d wrapping %v", what, err, err, status, target)
	}
}

// asAPI returns the *provider.APIError held in err (the test stops if there is none).
func asAPI(t *testing.T, err error) *provider.APIError {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error %v: want *provider.APIError", err)
	}
	return apiErr
}

// clock is an adjustable clock.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func TestNameAndSHA(t *testing.T) {
	if n := New().Name(); n != model.ProviderGitHub {
		t.Errorf("Name() = %q, want %q", n, model.ProviderGitHub)
	}
	a := SHA("a")
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(a) || a != SHA("a") || a == SHA("b") {
		t.Errorf("SHA(\"a\") = %q: want 40 lowercase hex, deterministic and specific to the seed", a)
	}
	if err := provider.ValidateSHA(a); err != nil {
		t.Errorf("SHA invalid for provider: %v", err)
	}
}

func TestRepository(t *testing.T) {
	f := New()
	pushed := time.Date(2026, 9, 30, 17, 45, 12, 999, time.FixedZone("CEST", 2*3600))
	f.AddRepository(provider.Repository{ID: 7, Owner: "Ecole-Promo", Name: "projet.final", URL: "https://github.com/Ecole-Promo/projet.final",
		Stars: 2, OpenIssues: 1, Archived: true, Private: true, PushedAt: pushed})
	want := provider.Repository{ID: 7, Owner: "Ecole-Promo", Name: "projet.final", DefaultBranch: "main",
		URL: "https://github.com/Ecole-Promo/projet.final", Stars: 2, OpenIssues: 1, Archived: true, Private: true,
		PushedAt: time.Date(2026, 9, 30, 15, 45, 12, 0, time.UTC)}

	for _, args := range [][2]string{{"Ecole-Promo", "projet.final"}, {"ecole-promo", "PROJET.FINAL"}} {
		got, err := f.Repository(ctx, args[0], args[1])
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("Repository(%q, %q) = %+v, %v\nwant %+v", args[0], args[1], got, err, want)
		}
	}
	if got, err := f.RepositoryByID(ctx, 7); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("RepositoryByID(7) = %+v, %v", got, err)
	}

	_, err := f.Repository(ctx, "alice", "inconnu")
	wantAPIError(t, "unknown repository", err, 404, provider.ErrNotFound)
	if apiErr := asAPI(t, err); apiErr.Op != "reading repository alice/inconnu" || apiErr.Path != "/repos/alice/inconnu" {
		t.Errorf("APIError = %+v", apiErr)
	}
	_, err = f.RepositoryByID(ctx, 99)
	wantAPIError(t, "unknown identifier", err, 404, provider.ErrNotFound)
	if apiErr := asAPI(t, err); apiErr.Op != "reading repository #99" || apiErr.Path != "/repositories/99" {
		t.Errorf("APIError = %+v", apiErr)
	}

	var apiErr *provider.APIError
	for _, args := range [][2]string{{"", "x"}, {"alice", ".."}, {"-alice", "x"}, {"alice", "a/b"}, {"a_b", "x"}} {
		_, err := f.Repository(ctx, args[0], args[1])
		if !errors.Is(err, provider.ErrInvalidRef) || errors.As(err, &apiErr) {
			t.Errorf("Repository(%q, %q): error %v, want ErrInvalidRef without APIError", args[0], args[1], err)
		}
	}
	if _, err := f.RepositoryByID(ctx, 0); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("RepositoryByID(0): %v, want ErrInvalidRef", err)
	}
}

func TestUpdateRepository(t *testing.T) {
	f, g := newGraph(t)
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet-alice", DefaultBranch: "main", Stars: 5})

	for _, args := range [][2]string{{"alice", "projet"}, {"ECOLE", "Projet-Alice"}} {
		got, err := f.Repository(ctx, args[0], args[1])
		if err != nil || got.Owner != "ecole" || got.Name != "projet-alice" || got.Stars != 5 || got.ID != 101 {
			t.Errorf("Repository(%q, %q) after renaming = %+v, %v", args[0], args[1], got, err)
		}
	}
	// The other methods follow the old name too.
	if cs, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{}); err != nil || len(cs) != 6 {
		t.Errorf("ListCommits by the old name: %d commits, %v", len(cs), err)
	}
	if bs, err := f.Branches(ctx, "alice", "projet"); err != nil || len(bs) != 2 {
		t.Errorf("Branches by the old name: %v, %v", bs, err)
	}
	if cs, err := f.CompareCommits(ctx, "alice", "projet", g.m2, g.merge); err != nil || len(cs) != 3 {
		t.Errorf("CompareCommits by the old name: %v, %v", shas(cs), err)
	}

	// Second renaming: both old names stay valid.
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet-final"})
	for _, args := range [][2]string{{"alice", "projet"}, {"ecole", "projet-alice"}, {"ecole", "projet-final"}} {
		if got, err := f.Repository(ctx, args[0], args[1]); err != nil || got.Name != "projet-final" || got.DefaultBranch != "main" {
			t.Errorf("Repository(%q, %q) = %+v, %v", args[0], args[1], got, err)
		}
	}

	// A new repository takes an old name again.
	f.AddRepository(provider.Repository{ID: 202, Owner: "alice", Name: "projet"})
	if got, err := f.Repository(ctx, "alice", "projet"); err != nil || got.ID != 202 {
		t.Errorf("old name taken again: %+v, %v, want repository 202", got, err)
	}
	if got, err := f.Repository(ctx, "ecole", "projet-alice"); err != nil || got.ID != 101 {
		t.Errorf("other old name: %+v, %v, want repository 101", got, err)
	}

	expectPanic(t, "renaming to a taken current name", func() {
		f.UpdateRepository(provider.Repository{ID: 101, Owner: "alice", Name: "projet"})
	})
	expectPanic(t, "unknown repository", func() { f.UpdateRepository(provider.Repository{ID: 9, Owner: "a", Name: "b"}) })
}

func TestRemoveRepository(t *testing.T) {
	f, g := newGraph(t)
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet"})
	f.RemoveRepository(101)
	checks := map[string]error{}
	_, checks["Repository (current name)"] = f.Repository(ctx, "ecole", "projet")
	_, checks["Repository (old name)"] = f.Repository(ctx, "alice", "projet")
	_, checks["RepositoryByID"] = f.RepositoryByID(ctx, 101)
	_, checks["Branches"] = f.Branches(ctx, "ecole", "projet")
	_, checks["ListCommits"] = f.ListCommits(ctx, "ecole", "projet", provider.ListCommitsOptions{})
	_, checks["CompareCommits"] = f.CompareCommits(ctx, "ecole", "projet", g.m1, g.merge)
	_, checks["CommitStats"] = f.CommitStats(ctx, "ecole", "projet", g.merge)
	for what, err := range checks {
		wantAPIError(t, what, err, 404, provider.ErrNotFound)
	}
	// The names are released.
	f.AddRepository(provider.Repository{ID: 303, Owner: "alice", Name: "projet"})
	expectPanic(t, "unknown RemoveRepository", func() { f.RemoveRepository(101) })
}

func TestBranches(t *testing.T) {
	f, g := newGraph(t)
	f.SetBranch(101, "dev", g.m1)
	f.SetBranch(101, "release/1.0", g.r1)
	got, err := f.Branches(ctx, "alice", "projet")
	want := []provider.Branch{
		{Name: "dev", HeadSHA: g.m1}, {Name: "feature", HeadSHA: g.f2}, {Name: "main", HeadSHA: g.merge}, {Name: "release/1.0", HeadSHA: g.r1},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Branches = %v, %v\nwant %v", got, err, want)
	}
	f.DeleteBranch(101, "dev")
	if got, _ := f.Branches(ctx, "alice", "projet"); len(got) != 3 {
		t.Errorf("after DeleteBranch: %v", got)
	}

	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	if got, err := f.Branches(ctx, "alice", "vide"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("empty repository: %#v, %v, want an empty non-nil list", got, err)
	}

	// Cap provider.MaxBranches.
	f.AddRepository(provider.Repository{ID: 3, Owner: "alice", Name: "branches"})
	f.AppendCommits(3, "main", 1, t0, 0)
	head, _ := f.Branches(ctx, "alice", "branches")
	for i := range provider.MaxBranches + 5 {
		f.SetBranch(3, fmt.Sprintf("b%05d", i), head[0].HeadSHA)
	}
	got, err = f.Branches(ctx, "alice", "branches")
	if err != nil || len(got) != provider.MaxBranches || got[0].Name != "b00000" {
		t.Errorf("cap: %d branches (first %v), %v; want %d", len(got), got[0], err, provider.MaxBranches)
	}
}

func TestListCommits(t *testing.T) {
	f, g := newGraph(t)
	cases := []struct {
		name string
		opts provider.ListCommitsOptions
		want []string
	}{
		{"default branch", provider.ListCommitsOptions{}, []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}},
		{"main", provider.ListCommitsOptions{Branch: "main"}, []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}},
		{"feature", provider.ListCommitsOptions{Branch: "feature"}, []string{g.f2, g.f1, g.r1}},
		{"SHA", provider.ListCommitsOptions{Branch: g.m2}, []string{g.m2, g.m1, g.r1}},
		{"Since", provider.ListCommitsOptions{Since: t0.Add(2 * time.Hour)}, []string{g.merge, g.f2, g.m2, g.f1}},
		{"Since other time zone", provider.ListCommitsOptions{Since: t0.Add(2 * time.Hour).In(time.FixedZone("X", -5*3600))}, []string{g.merge, g.f2, g.m2, g.f1}},
		// Since truncated to the second, like the since sent to GitHub: f1 (exactly t0 + 2 h) stays.
		{"Since with a fraction of a second", provider.ListCommitsOptions{Since: t0.Add(2*time.Hour + 500*time.Millisecond)}, []string{g.merge, g.f2, g.m2, g.f1}},
		{"Since right after the second", provider.ListCommitsOptions{Since: t0.Add(2*time.Hour + time.Second)}, []string{g.merge, g.f2, g.m2}},
		{"future Since", provider.ListCommitsOptions{Since: t0.Add(24 * time.Hour)}, []string{}},
	}
	for _, c := range cases {
		got, err := f.ListCommits(ctx, "alice", "projet", c.opts)
		if err != nil || got == nil || !reflect.DeepEqual(shas(got), c.want) {
			t.Errorf("%s: %v, %v\nwant %v", c.name, shas(got), err, c.want)
		}
	}

	got, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	m := got[0]
	if m.Message != "fusion" || m.Author != alice || m.URL != "https://github.com/alice/projet/commit/"+g.merge ||
		!m.CommittedAt.Equal(t0.Add(5*time.Hour)) || !reflect.DeepEqual(m.Parents, []string{g.m2, g.f2}) {
		t.Errorf("merge commit: %+v", m)
	}
	if root := got[5]; root.Parents == nil || len(root.Parents) != 0 {
		t.Errorf("root commit: Parents %#v, want an empty non-nil slice", root.Parents)
	}
	// Independent copies.
	got[0].Parents[0] = "modifié"
	got[0].Message = "modifié"
	again, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	if again[0].Parents[0] != g.m2 || again[0].Message != "fusion" {
		t.Error("modifying a result modified the Fake")
	}

	for _, branch := range []string{"inconnue", SHA("absent")} {
		_, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: branch})
		wantAPIError(t, "branch "+branch, err, 404, provider.ErrNotFound)
	}
	if _, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "a\nb"}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid branch: %v, want ErrInvalidRef", err)
	}

	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	for _, branch := range []string{"", "main", "inconnue"} {
		got, err := f.ListCommits(ctx, "alice", "vide", provider.ListCommitsOptions{Branch: branch})
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("empty repository, branch %q: %#v, %v; want an empty non-nil list without error", branch, got, err)
		}
	}

	// At equal dates, the last added first.
	a, b, ab := SHA("a"), SHA("b"), SHA("ab")
	at := t0.Add(6 * time.Hour)
	f.AddCommits(101,
		provider.Commit{SHA: a, CommittedAt: at, Parents: []string{g.merge}},
		provider.Commit{SHA: b, CommittedAt: at, Parents: []string{g.merge}},
		provider.Commit{SHA: ab, CommittedAt: at.Add(time.Hour), Parents: []string{a, b}},
	)
	f.SetBranch(101, "main", ab)
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Limit: 3})
	if want := []string{ab, b, a}; !reflect.DeepEqual(shas(got), want) {
		t.Errorf("equal dates: %v, want %v", shas(got), want)
	}
}

func TestListCommitsStopAndLimit(t *testing.T) {
	f, g := newGraph(t)
	var seen []string
	stop := func(sha string) bool {
		seen = append(seen, sha)
		_ = f.Calls(MethodListCommits) // Stop is called outside the lock: no deadlock
		return sha == g.m2
	}
	got, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: stop})
	if err != nil || !reflect.DeepEqual(shas(got), []string{g.merge, g.f2}) {
		t.Errorf("Stop: %v, %v; want [merge f2]", shas(got), err)
	}
	if !reflect.DeepEqual(seen, []string{g.merge, g.f2, g.m2}) {
		t.Errorf("Stop called on %v, want [merge f2 m2]", seen)
	}

	seen = nil
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: stop, Limit: 1})
	if !reflect.DeepEqual(shas(got), []string{g.merge}) || len(seen) != 1 {
		t.Errorf("Limit 1: %v, Stop called %d times; want [merge] and 1 call", shas(got), len(seen))
	}

	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: func(sha string) bool { return sha == g.merge }})
	if got == nil || len(got) != 0 {
		t.Errorf("Stop on the first commit: %#v, want an empty non-nil list", got)
	}
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Since: t0.Add(4 * time.Hour), Stop: func(string) bool { return false }})
	if !reflect.DeepEqual(shas(got), []string{g.merge, g.f2}) {
		t.Errorf("Since + Stop: %v", shas(got))
	}
	for _, limit := range []int{-1, 0, provider.MaxCommits + 1} {
		if got, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Limit: limit}); len(got) != 6 {
			t.Errorf("Limit %d: %d commits, want 6", limit, len(got))
		}
	}

	// Cap provider.MaxCommits.
	f.AddRepository(provider.Repository{ID: 9, Owner: "alice", Name: "gros"})
	created := f.AppendCommits(9, "main", provider.MaxCommits+2, t0, time.Second)
	got, err = f.ListCommits(ctx, "alice", "gros", provider.ListCommitsOptions{Limit: provider.MaxCommits + 50})
	if err != nil || len(got) != provider.MaxCommits || got[0].SHA != created[len(created)-1].SHA {
		t.Errorf("cap: %d commits, %v; want %d starting with the most recent", len(got), err, provider.MaxCommits)
	}
}

func TestCompareCommits(t *testing.T) {
	f, g := newGraph(t)
	cases := []struct {
		base, head string
		want       []string
	}{
		{g.m2, g.merge, []string{g.merge, g.f2, g.f1}},
		{g.m1, g.merge, []string{g.merge, g.f2, g.m2, g.f1}},
		{g.r1, g.f2, []string{g.f2, g.f1}},
		{g.f2, g.m2, []string{g.m2, g.m1}},
		{g.merge, g.m2, []string{}}, // head contained in base
		{g.merge, g.merge, []string{}},
	}
	for _, c := range cases {
		got, err := f.CompareCommits(ctx, "alice", "projet", c.base, c.head)
		if err != nil || got == nil || !reflect.DeepEqual(shas(got), c.want) {
			t.Errorf("%s...%s: %v, %v\nwant %v", provider.ShortSHA(c.base), provider.ShortSHA(c.head), shas(got), err, c.want)
		}
	}

	// unknown base: ErrUnknownBase and above all not ErrNotFound.
	_, err := f.CompareCommits(ctx, "alice", "projet", SHA("absent"), g.merge)
	wantAPIError(t, "unknown base", err, 404, provider.ErrUnknownBase)
	if errors.Is(err, provider.ErrNotFound) {
		t.Error("unknown base: the error must not wrap ErrNotFound")
	}
	if op := asAPI(t, err).Op; op != fmt.Sprintf("comparing %s...%s in alice/projet", provider.ShortSHA(SHA("absent")), provider.ShortSHA(g.merge)) {
		t.Errorf("Op = %q", op)
	}

	// unknown head: like the GitHub Provider, which cannot tell this case apart from an unknown base
	// (GitHub answers 404 in both cases, on an accessible repository), ErrUnknownBase.
	_, err = f.CompareCommits(ctx, "alice", "projet", g.merge, SHA("absent"))
	wantAPIError(t, "unknown head", err, 404, provider.ErrUnknownBase)
	if errors.Is(err, provider.ErrNotFound) {
		t.Error("unknown head: the error must not wrap ErrNotFound")
	}

	// Force-push then cleanup: the old head disappears.
	f.SetBranch(101, "feature", g.f1)
	if got, err := f.CompareCommits(ctx, "alice", "projet", g.f2, g.merge); err != nil || len(got) != 3 {
		t.Errorf("before cleanup: %v, %v", shas(got), err)
	}
	f.RemoveCommit(101, g.f2)
	_, err = f.CompareCommits(ctx, "alice", "projet", g.f2, g.merge)
	wantAPIError(t, "removed base", err, 404, provider.ErrUnknownBase)

	// Histories without common ancestor.
	orphan := SHA("orphelin")
	f.AddCommits(101, provider.Commit{SHA: orphan, CommittedAt: t0.Add(9 * time.Hour)})
	f.SetBranch(101, "orpheline", orphan)
	_, err = f.CompareCommits(ctx, "alice", "projet", g.m1, orphan)
	wantAPIError(t, "without common ancestor", err, 404, provider.ErrUnknownBase)

	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	_, err = f.CompareCommits(ctx, "alice", "vide", g.m1, g.merge)
	wantAPIError(t, "empty repository", err, 409, provider.ErrEmptyRepository)

	for _, args := range [][2]string{{"abc", g.merge}, {g.merge, strings.ToUpper(g.m1)}} {
		if _, err := f.CompareCommits(ctx, "alice", "projet", args[0], args[1]); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("invalid SHA %v: %v, want ErrInvalidRef", args, err)
		}
	}

	// base == head: no check after the validation (the GitHub Provider sends nothing).
	f.SetRateLimit(provider.RateLimit{Limit: 10, Remaining: 0, Reset: time.Now().Add(time.Hour)})
	f.SetError(MethodCompareCommits, errors.New("injected"))
	f.SetRepoError("alice", "projet", errors.New("repository"))
	if got, err := f.CompareCommits(ctx, "alice", "projet", g.merge, g.merge); err != nil || got == nil || len(got) != 0 {
		t.Errorf("base == head: %#v, %v; want an empty non-nil list without error", got, err)
	}
}

func TestCompareCommitsMaxCommits(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 1, Owner: "alice", Name: "gros"})
	created := f.AppendCommits(1, "main", provider.MaxCommits+2, t0, time.Second)
	got, err := f.CompareCommits(ctx, "alice", "gros", created[0].SHA, created[len(created)-1].SHA)
	// MaxCommits+1 commits ahead: the MaxCommits oldest are kept (like the GitHub Provider, which reads
	// the pages from oldest to newest), from newest to oldest.
	if err != nil || len(got) != provider.MaxCommits || got[0].SHA != created[provider.MaxCommits].SHA ||
		got[len(got)-1].SHA != created[1].SHA {
		t.Errorf("cap: %d commits, %v", len(got), err)
	}
}

func TestCommitStats(t *testing.T) {
	f, g := newGraph(t)
	f.SetStats(101, g.merge, provider.CommitStats{Additions: 12, Deletions: 3})
	if s, err := f.CommitStats(ctx, "alice", "projet", g.merge); err != nil || s.Additions != 12 || s.Deletions != 3 {
		t.Errorf("CommitStats(merge) = %+v, %v", s, err)
	}
	if s, err := f.CommitStats(ctx, "alice", "projet", g.r1); err != nil || s.Additions != 0 || s.Deletions != 0 || s.Files != nil {
		t.Errorf("CommitStats(r1) = %+v, %v; want zero", s, err)
	}
	_, err := f.CommitStats(ctx, "alice", "projet", SHA("absent"))
	wantAPIError(t, "unknown commit", err, 404, provider.ErrNotFound)
	if op := asAPI(t, err).Op; op != "statistics of commit "+provider.ShortSHA(SHA("absent"))+" in alice/projet" {
		t.Errorf("Op = %q", op)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", "main"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("invalid SHA: %v", err)
	}
	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	_, err = f.CommitStats(ctx, "alice", "vide", g.r1)
	wantAPIError(t, "empty repository", err, 409, provider.ErrEmptyRepository)

	// A removed commit loses its statistics.
	f.SetBranch(101, "feature", g.f1)
	f.SetStats(101, g.f2, provider.CommitStats{Additions: 1})
	f.RemoveCommit(101, g.f2)
	_, err = f.CommitStats(ctx, "alice", "projet", g.f2)
	wantAPIError(t, "removed commit", err, 404, provider.ErrNotFound)
}

func TestInjectedErrors(t *testing.T) {
	f, g := newGraph(t)
	errA, errB, errC := errors.New("A"), errors.New("B"), errors.New("C")
	f.SetError(MethodRepository, errC)
	f.FailNext(MethodRepository, errA)
	f.FailNext(MethodRepository, errB)
	for i, want := range []error{errA, errB, errC, errC} {
		if _, err := f.Repository(ctx, "alice", "projet"); err != want {
			t.Errorf("call %d: %v, want %v", i+1, err, want)
		}
	}
	f.SetError(MethodRepository, nil)
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Errorf("after SetError(nil): %v", err)
	}
	if n := f.Calls(MethodRepository); n != 5 {
		t.Errorf("Calls = %d, want 5 (failures included)", n)
	}

	// The other methods are not touched.
	f.SetError(MethodBranches, errA)
	if _, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{}); err != nil {
		t.Errorf("ListCommits touched by SetError(Branches): %v", err)
	}

	// Order: context, then validation, then injection.
	f.FailNext(MethodCommitStats, errA)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.CommitStats(canceled, "alice", "projet", "invalide"); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled context: %v, want context.Canceled", err)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", "invalide"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("validation before injection: %v", err)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", g.merge); err != errA {
		t.Errorf("injection after validation: %v, want A", err)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", g.merge); err != nil {
		t.Errorf("emptied queue: %v", err)
	}
	if n := f.Calls(MethodCommitStats); n != 4 {
		t.Errorf("Calls(CommitStats) = %d, want 4", n)
	}

	for _, m := range []Method{MethodRepositoryByID, MethodListCommits, MethodCompareCommits, MethodRateLimit} {
		f.FailNext(m, errB)
	}
	_, e1 := f.RepositoryByID(ctx, 101)
	_, e2 := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	_, e3 := f.CompareCommits(ctx, "alice", "projet", g.m1, g.merge)
	_, e4 := f.RateLimit(ctx)
	for i, err := range []error{e1, e2, e3, e4} {
		if err != errB {
			t.Errorf("FailNext method %d: %v", i, err)
		}
	}

	expectPanic(t, "SetError unknown method", func() { f.SetError("Inconnue", errA) })
	expectPanic(t, "FailNext unknown method", func() { f.FailNext("Name", errA) })
	expectPanic(t, "FailNext nil", func() { f.FailNext(MethodRepository, nil) })
}

func TestRepoErrors(t *testing.T) {
	f, g := newGraph(t)
	f.AddRepository(provider.Repository{ID: 202, Owner: "ecole", Name: "autre"})
	sso := &provider.SSORequiredError{Organization: "alice"}
	f.SetRepoError("ALICE", "Projet", sso)

	errs := map[string]error{}
	_, errs["Repository"] = f.Repository(ctx, "alice", "projet")
	_, errs["RepositoryByID"] = f.RepositoryByID(ctx, 101)
	_, errs["Branches"] = f.Branches(ctx, "alice", "projet")
	_, errs["ListCommits"] = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	_, errs["CompareCommits"] = f.CompareCommits(ctx, "alice", "projet", g.m1, g.merge)
	_, errs["CommitStats"] = f.CommitStats(ctx, "alice", "projet", g.merge)
	for m, err := range errs {
		if !errors.Is(err, provider.ErrForbidden) || err != error(sso) {
			t.Errorf("%s: %v, want the SSO error set", m, err)
		}
	}
	if _, err := f.Repository(ctx, "ecole", "autre"); err != nil {
		t.Errorf("other repository touched: %v", err)
	}

	// After renaming, the error follows the repository (set on the old name).
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet"})
	if _, err := f.Repository(ctx, "ecole", "projet"); err != error(sso) {
		t.Errorf("current name after renaming: %v", err)
	}
	// Restoring through the current name also lifts the error set on the old one.
	f.SetRepoError("ecole", "projet", nil)
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Errorf("after SetRepoError(nil): %v", err)
	}

	// Set before the repository is added.
	f.SetRepoError("bob", "futur", provider.ErrForbidden)
	if _, err := f.Repository(ctx, "bob", "futur"); err != provider.ErrForbidden {
		t.Errorf("repository still unknown: %v, want the error set", err)
	}
	f.AddRepository(provider.Repository{ID: 303, Owner: "bob", Name: "futur"})
	if _, err := f.RepositoryByID(ctx, 303); err != provider.ErrForbidden {
		t.Errorf("RepositoryByID of a repository added after SetRepoError: %v", err)
	}
	// The error injected by method comes before the repository's.
	f.FailNext(MethodRepositoryByID, context.DeadlineExceeded)
	if _, err := f.RepositoryByID(ctx, 303); err != context.DeadlineExceeded {
		t.Errorf("FailNext before SetRepoError: %v", err)
	}
}

func TestCallLog(t *testing.T) {
	f, g := newGraph(t)
	since := t0.Add(time.Hour)
	_, _ = f.Repository(ctx, "Alice", "Projet")
	_, _ = f.RepositoryByID(ctx, 101)
	_, _ = f.Branches(ctx, "alice", "projet")
	_, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "main", Since: since, Limit: 5, Stop: func(string) bool { return false }})
	_, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	_, _ = f.CompareCommits(ctx, "alice", "projet", g.m1, g.merge)
	_, _ = f.CommitStats(ctx, "alice", "projet", g.r1)
	_, _ = f.RateLimit(ctx)
	_, _ = f.Repository(ctx, "", "") // invalid: counted anyway
	f.LastRateLimit()
	_ = f.Name()

	want := []Call{
		{Method: MethodRepository, Owner: "Alice", Name: "Projet"},
		{Method: MethodRepositoryByID, ID: 101},
		{Method: MethodBranches, Owner: "alice", Name: "projet"},
		{Method: MethodListCommits, Owner: "alice", Name: "projet", Opts: provider.ListCommitsOptions{Branch: "main", Since: since, Limit: 5}, StopSet: true},
		{Method: MethodListCommits, Owner: "alice", Name: "projet"},
		{Method: MethodCompareCommits, Owner: "alice", Name: "projet", Base: g.m1, Head: g.merge},
		{Method: MethodCommitStats, Owner: "alice", Name: "projet", SHA: g.r1},
		{Method: MethodRateLimit},
		{Method: MethodRepository},
	}
	log := f.CallLog()
	if len(log) != len(want) {
		t.Fatalf("CallLog: %d calls, want %d", len(log), len(want))
	}
	for i := range want {
		if log[i].Opts.Stop != nil {
			t.Errorf("call %d: Opts.Stop not nil", i)
		}
		log[i].Opts.Stop = nil
		if !reflect.DeepEqual(log[i], want[i]) {
			t.Errorf("call %d: %+v\nwant %+v", i, log[i], want[i])
		}
	}
	if f.Calls(MethodListCommits) != 2 || f.Calls(MethodRepository) != 2 {
		t.Errorf("Calls: ListCommits %d, Repository %d", f.Calls(MethodListCommits), f.Calls(MethodRepository))
	}
	log[0].Owner = "modifié"
	if f.CallLog()[0].Owner != "Alice" {
		t.Error("CallLog does not return a copy")
	}
	f.ResetCalls()
	if len(f.CallLog()) != 0 || f.Calls(MethodRepository) != 0 {
		t.Error("ResetCalls did not empty everything")
	}
}

func TestRateLimit(t *testing.T) {
	c := &clock{now: t0.Add(500 * time.Millisecond)}
	f, _ := newGraph(t)
	f.SetClock(c.Now)

	if _, ok := f.LastRateLimit(); ok {
		t.Error("LastRateLimit without SetRateLimit: true, want false")
	}
	rl, err := f.RateLimit(ctx)
	if err != nil || rl.Limit != 5000 || rl.Remaining != 5000 || rl.Used != 0 || !rl.Reset.Equal(t0.Add(time.Hour)) || !rl.ObservedAt.Equal(c.Now()) {
		t.Errorf("RateLimit without simulated quota = %+v, %v", rl, err)
	}
	if _, ok := f.LastRateLimit(); ok {
		t.Error("LastRateLimit after RateLimit without SetRateLimit: true, want false")
	}

	reset := t0.Add(30 * time.Minute)
	f.SetRateLimit(provider.RateLimit{Limit: 10, Remaining: 2, Reset: reset.In(time.FixedZone("X", 3600))})
	rl, ok := f.LastRateLimit()
	if !ok || rl.Limit != 10 || rl.Remaining != 2 || rl.Used != 8 || !rl.Reset.Equal(reset) || rl.Reset.Location() != time.UTC || !rl.ObservedAt.Equal(c.Now()) {
		t.Errorf("LastRateLimit = %+v, %v", rl, ok)
	}

	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Branches(ctx, "alice", "inconnu"); !errors.Is(err, provider.ErrNotFound) { // a 404 consumes too
		t.Fatal(err)
	}
	if rl, _ := f.RateLimit(ctx); rl.Remaining != 0 || rl.Used != 10 {
		t.Errorf("after 2 calls: %+v", rl)
	}
	_, err = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	wantAPIError(t, "exhausted quota", err, 403, provider.ErrRateLimited)
	var rle *provider.RateLimitError
	if !errors.As(err, &rle) || !rle.Reset.Equal(reset) || rle.Secondary {
		t.Errorf("RateLimitError = %+v", rle)
	}
	// RateLimit stays allowed and free; the validation comes before the quota.
	if _, err := f.RateLimit(ctx); err != nil {
		t.Errorf("RateLimit with exhausted quota: %v", err)
	}
	if _, err := f.Repository(ctx, "alice", "a/b"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("validation before the quota: %v", err)
	}

	// Reset: Reset moved forward hour by hour until it passes now.
	c.Advance(2*time.Hour + 10*time.Minute) // now = t0 + 2 h 10 min 0.5 s
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Fatalf("after the reset: %v", err)
	}
	rl, _ = f.LastRateLimit()
	if rl.Remaining != 9 || rl.Used != 1 || !rl.Reset.Equal(t0.Add(150*time.Minute)) {
		t.Errorf("new window: %+v, want Remaining 9, Used 1, Reset %s", rl, t0.Add(150*time.Minute))
	}

	// Default values of SetRateLimit.
	f.SetRateLimit(provider.RateLimit{Remaining: 4000})
	rl, _ = f.LastRateLimit()
	if rl.Limit != 5000 || rl.Used != 1000 || !rl.Reset.Equal(c.Now().Add(time.Hour).Truncate(time.Second)) {
		t.Errorf("default SetRateLimit: %+v", rl)
	}
	expectPanic(t, "negative Remaining", func() { f.SetRateLimit(provider.RateLimit{Remaining: -1}) })
	f.SetClock(nil)
	if rl, _ := f.LastRateLimit(); time.Since(rl.ObservedAt) > time.Minute {
		t.Errorf("SetClock(nil): ObservedAt %s, want the real time", rl.ObservedAt)
	}
}

func TestAppendCommits(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 5, Owner: "alice", Name: "projet"})
	if got := f.AppendCommits(5, "main", 0, t0, time.Hour); got == nil || len(got) != 0 {
		t.Errorf("n = 0: %#v", got)
	}
	if bs, _ := f.Branches(ctx, "alice", "projet"); len(bs) != 0 {
		t.Errorf("n = 0 created a branch: %v", bs)
	}

	start := time.Date(2026, 10, 1, 10, 0, 0, 700, time.FixedZone("CEST", 2*3600))
	first := f.AppendCommits(5, "main", 2, start, time.Hour)
	student := provider.Person{Name: "Étudiant Test", Email: "etudiant@example.test", Login: "etudiant-test", ID: 1001,
		AvatarURL: "https://avatars.example.test/u/1001", ProfileURL: "https://github.example.test/etudiant-test"}
	want0 := provider.Commit{SHA: SHA("5/main/1"), Message: "commit 1", URL: "https://github.com/alice/projet/commit/" + SHA("5/main/1"),
		Author: student, Committer: student, AuthoredAt: t0, CommittedAt: t0, Parents: []string{}}
	if !reflect.DeepEqual(first[0], want0) {
		t.Errorf("first commit: %+v\nwant %+v", first[0], want0)
	}
	if first[1].SHA != SHA("5/main/2") || first[1].Message != "commit 2" || !reflect.DeepEqual(first[1].Parents, []string{first[0].SHA}) ||
		!first[1].CommittedAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("second commit: %+v", first[1])
	}

	// Continuation of the same branch, avoiding an already present SHA: the commit added by hand carries
	// the SHA the next AppendCommits would have chosen (rank 4).
	f.AddCommits(5, provider.Commit{SHA: SHA("5/main/4"), CommittedAt: t0, Parents: []string{first[1].SHA}})
	more := f.AppendCommits(5, "main", 1, t0.Add(5*time.Hour), 0)
	if more[0].SHA != SHA("5/main/5") || more[0].Message != "commit 5" || more[0].Parents[0] != first[1].SHA {
		t.Errorf("continuation: %+v, want the SHA of \"5/main/5\"", more[0])
	}
	got, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	if want := []string{more[0].SHA, first[1].SHA, first[0].SHA}; !reflect.DeepEqual(shas(got), want) {
		t.Errorf("history: %v, want %v", shas(got), want)
	}

	// Modifying the result does not modify the Fake.
	more[0].Parents[0] = "modifié"
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Limit: 1})
	if got[0].Parents[0] != first[1].SHA {
		t.Error("the result of AppendCommits shares its memory with the Fake")
	}
}

func TestPreparationPanics(t *testing.T) {
	f, g := newGraph(t)
	cases := map[string]func(){
		"AddRepository zero ID":            func() { f.AddRepository(provider.Repository{Owner: "a", Name: "b"}) },
		"AddRepository taken ID":           func() { f.AddRepository(provider.Repository{ID: 101, Owner: "a", Name: "b"}) },
		"AddRepository taken name":         func() { f.AddRepository(provider.Repository{ID: 2, Owner: "ALICE", Name: "Projet"}) },
		"AddRepository invalid name":       func() { f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "a/b"}) },
		"AddCommits unknown repository":    func() { f.AddCommits(9, provider.Commit{SHA: SHA("x")}) },
		"AddCommits invalid SHA":           func() { f.AddCommits(101, provider.Commit{SHA: "abc"}) },
		"AddCommits present SHA":           func() { f.AddCommits(101, provider.Commit{SHA: g.r1}) },
		"AddCommits unknown parent":        func() { f.AddCommits(101, provider.Commit{SHA: SHA("x"), Parents: []string{SHA("y")}}) },
		"AppendCommits negative n":         func() { f.AppendCommits(101, "main", -1, t0, 0) },
		"AppendCommits empty branch":       func() { f.AppendCommits(101, "", 1, t0, 0) },
		"AppendCommits unknown repository": func() { f.AppendCommits(9, "main", 1, t0, 0) },
		"SetBranch unknown commit":         func() { f.SetBranch(101, "dev", SHA("inconnu")) },
		"SetBranch invalid name":           func() { f.SetBranch(101, "a\x00b", g.r1) },
		"DeleteBranch unknown":             func() { f.DeleteBranch(101, "inconnue") },
		"RemoveCommit head":                func() { f.RemoveCommit(101, g.merge) },
		"RemoveCommit unknown":             func() { f.RemoveCommit(101, SHA("inconnu")) },
		"SetStats unknown commit":          func() { f.SetStats(101, SHA("inconnu"), provider.CommitStats{}) },
		"SetStats unknown repository":      func() { f.SetStats(9, g.r1, provider.CommitStats{}) },
		"UpdateRepository invalid name":    func() { f.UpdateRepository(provider.Repository{ID: 101, Owner: "", Name: "x"}) },
	}
	for what, fn := range cases {
		expectPanic(t, what, fn)
	}
	// A parent added earlier in the same call is accepted.
	f.AddCommits(101,
		provider.Commit{SHA: SHA("p"), CommittedAt: t0, Parents: []string{g.r1}},
		provider.Commit{SHA: SHA("c"), CommittedAt: t0, Parents: []string{SHA("p")}},
	)
	// The lock was released despite the panics.
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Errorf("after the panics: %v", err)
	}
}

func TestConcurrentUse(t *testing.T) {
	f, g := newGraph(t)
	f.SetRateLimit(provider.RateLimit{Limit: 1_000_000, Remaining: 1_000_000})
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Go(func() {
			for i := range 50 {
				var err error
				switch (w + i) % 6 {
				case 0:
					_, err = f.Repository(ctx, "alice", "projet")
				case 1:
					_, err = f.Branches(ctx, "alice", "projet")
				case 2:
					_, err = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: func(string) bool { return false }})
				case 3:
					_, err = f.CompareCommits(ctx, "alice", "projet", g.m1, g.merge)
				case 4:
					_, err = f.CommitStats(ctx, "alice", "projet", g.r1)
				case 5:
					_, err = f.RateLimit(ctx)
					f.LastRateLimit()
				}
				if err != nil {
					t.Error(err)
				}
			}
		})
	}
	for i := range 20 {
		f.AppendCommits(101, "dev", 1, t0.Add(time.Duration(i)*time.Minute), 0)
		f.SetError(MethodRateLimit, nil)
		_ = f.CallLog()
	}
	wg.Wait()
	total := 0
	for _, m := range []Method{MethodRepository, MethodBranches, MethodListCommits, MethodCompareCommits, MethodCommitStats, MethodRateLimit} {
		total += f.Calls(m)
	}
	if total != 400 {
		t.Errorf("counted calls: %d, want 400", total)
	}
	if rl, _ := f.LastRateLimit(); rl.Used != 400-f.Calls(MethodRateLimit) {
		t.Errorf("consumed quota: %d, want %d", rl.Used, 400-f.Calls(MethodRateLimit))
	}
}

func TestProfileURL(t *testing.T) {
	f := New()
	for login, want := range map[string]string{
		"etudiant-test":   "https://github.example.test/etudiant-test",
		"":                "",
		"dependabot[bot]": "",
		"a/b":             "",
	} {
		if got := f.ProfileURL(login); got != want {
			t.Errorf("ProfileURL(%q) = %q, want %q", login, got, want)
		}
	}
	if len(f.CallLog()) != 0 {
		t.Errorf("counted calls: %v", f.CallLog())
	}
}
