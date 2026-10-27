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

// t0 est l'origine des dates factices des tests.
var t0 = time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC)

var ctx = context.Background()

// graph est un historique avec une branche fusionnée, dans le dépôt 101 alice/projet :
//
//	r1 ── m1 ── m2 ─────── M   (main)
//	  └── f1 ── f2 ──────┘     (feature : f2)
//
// dates de commit : r1 t0, m1 t0+1h, f1 t0+2h, m2 t0+3h, f2 t0+4h, M t0+5h.
type graph struct {
	r1, m1, f1, m2, f2, merge string
}

// alice et bob sont des personnes factices.
var (
	alice = provider.Person{Name: "Alice Test", Email: "alice@example.test", Login: "alice", ID: 1, AvatarURL: "https://avatars.example.test/u/1"}
	bob   = provider.Person{Name: "Bob Test", Email: "bob@example.test"}
)

// newGraph renvoie un Fake avec le dépôt 101 alice/projet et l'historique graph.
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

// shas renvoie les SHA d'une liste de commits.
func shas(cs []provider.Commit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.SHA
	}
	return out
}

// expectPanic vérifie que fn panique.
func expectPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s : pas de panique", what)
		}
	}()
	fn()
}

// wantAPIError vérifie que err est un *provider.APIError de code status qui enveloppe target.
func wantAPIError(t *testing.T, what string, err error, status int, target error) {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) || !errors.Is(err, target) || apiErr.Status != status || apiErr.Method != "GET" ||
		apiErr.Op == "" || !strings.HasPrefix(apiErr.Path, "/") {
		t.Errorf("%s : erreur %#v (%v), veut *APIError HTTP %d enveloppant %v", what, err, err, status, target)
	}
}

// asAPI renvoie le *provider.APIError contenu dans err (le test s'arrête s'il n'y en a pas).
func asAPI(t *testing.T, err error) *provider.APIError {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("erreur %v : *provider.APIError attendu", err)
	}
	return apiErr
}

// clock est une horloge réglable.
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
		t.Errorf("Name() = %q, veut %q", n, model.ProviderGitHub)
	}
	a := SHA("a")
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(a) || a != SHA("a") || a == SHA("b") {
		t.Errorf("SHA(\"a\") = %q : veut 40 hex minuscules, déterministe et propre à la graine", a)
	}
	if err := provider.ValidateSHA(a); err != nil {
		t.Errorf("SHA invalide pour provider : %v", err)
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
			t.Errorf("Repository(%q, %q) = %+v, %v\nveut %+v", args[0], args[1], got, err, want)
		}
	}
	if got, err := f.RepositoryByID(ctx, 7); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("RepositoryByID(7) = %+v, %v", got, err)
	}

	_, err := f.Repository(ctx, "alice", "inconnu")
	wantAPIError(t, "dépôt inconnu", err, 404, provider.ErrNotFound)
	if apiErr := asAPI(t, err); apiErr.Op != "lecture du dépôt alice/inconnu" || apiErr.Path != "/repos/alice/inconnu" {
		t.Errorf("APIError = %+v", apiErr)
	}
	_, err = f.RepositoryByID(ctx, 99)
	wantAPIError(t, "identifiant inconnu", err, 404, provider.ErrNotFound)
	if apiErr := asAPI(t, err); apiErr.Op != "lecture du dépôt n° 99" || apiErr.Path != "/repositories/99" {
		t.Errorf("APIError = %+v", apiErr)
	}

	var apiErr *provider.APIError
	for _, args := range [][2]string{{"", "x"}, {"alice", ".."}, {"-alice", "x"}, {"alice", "a/b"}, {"a_b", "x"}} {
		_, err := f.Repository(ctx, args[0], args[1])
		if !errors.Is(err, provider.ErrInvalidRef) || errors.As(err, &apiErr) {
			t.Errorf("Repository(%q, %q) : erreur %v, veut ErrInvalidRef sans APIError", args[0], args[1], err)
		}
	}
	if _, err := f.RepositoryByID(ctx, 0); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("RepositoryByID(0) : %v, veut ErrInvalidRef", err)
	}
}

func TestUpdateRepository(t *testing.T) {
	f, g := newGraph(t)
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet-alice", DefaultBranch: "main", Stars: 5})

	for _, args := range [][2]string{{"alice", "projet"}, {"ECOLE", "Projet-Alice"}} {
		got, err := f.Repository(ctx, args[0], args[1])
		if err != nil || got.Owner != "ecole" || got.Name != "projet-alice" || got.Stars != 5 || got.ID != 101 {
			t.Errorf("Repository(%q, %q) après renommage = %+v, %v", args[0], args[1], got, err)
		}
	}
	// Les autres méthodes suivent aussi l'ancien nom.
	if cs, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{}); err != nil || len(cs) != 6 {
		t.Errorf("ListCommits par l'ancien nom : %d commits, %v", len(cs), err)
	}
	if bs, err := f.Branches(ctx, "alice", "projet"); err != nil || len(bs) != 2 {
		t.Errorf("Branches par l'ancien nom : %v, %v", bs, err)
	}
	if cs, err := f.CompareCommits(ctx, "alice", "projet", g.m2, g.merge); err != nil || len(cs) != 3 {
		t.Errorf("CompareCommits par l'ancien nom : %v, %v", shas(cs), err)
	}

	// Deuxième renommage : les deux anciens noms restent valides.
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet-final"})
	for _, args := range [][2]string{{"alice", "projet"}, {"ecole", "projet-alice"}, {"ecole", "projet-final"}} {
		if got, err := f.Repository(ctx, args[0], args[1]); err != nil || got.Name != "projet-final" || got.DefaultBranch != "main" {
			t.Errorf("Repository(%q, %q) = %+v, %v", args[0], args[1], got, err)
		}
	}

	// Un nouveau dépôt reprend un ancien nom.
	f.AddRepository(provider.Repository{ID: 202, Owner: "alice", Name: "projet"})
	if got, err := f.Repository(ctx, "alice", "projet"); err != nil || got.ID != 202 {
		t.Errorf("ancien nom repris : %+v, %v, veut le dépôt 202", got, err)
	}
	if got, err := f.Repository(ctx, "ecole", "projet-alice"); err != nil || got.ID != 101 {
		t.Errorf("autre ancien nom : %+v, %v, veut le dépôt 101", got, err)
	}

	expectPanic(t, "renommage vers un nom actuel pris", func() {
		f.UpdateRepository(provider.Repository{ID: 101, Owner: "alice", Name: "projet"})
	})
	expectPanic(t, "dépôt inconnu", func() { f.UpdateRepository(provider.Repository{ID: 9, Owner: "a", Name: "b"}) })
}

func TestRemoveRepository(t *testing.T) {
	f, g := newGraph(t)
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet"})
	f.RemoveRepository(101)
	checks := map[string]error{}
	_, checks["Repository (nom actuel)"] = f.Repository(ctx, "ecole", "projet")
	_, checks["Repository (ancien nom)"] = f.Repository(ctx, "alice", "projet")
	_, checks["RepositoryByID"] = f.RepositoryByID(ctx, 101)
	_, checks["Branches"] = f.Branches(ctx, "ecole", "projet")
	_, checks["ListCommits"] = f.ListCommits(ctx, "ecole", "projet", provider.ListCommitsOptions{})
	_, checks["CompareCommits"] = f.CompareCommits(ctx, "ecole", "projet", g.m1, g.merge)
	_, checks["CommitStats"] = f.CommitStats(ctx, "ecole", "projet", g.merge)
	for what, err := range checks {
		wantAPIError(t, what, err, 404, provider.ErrNotFound)
	}
	// Les noms sont libérés.
	f.AddRepository(provider.Repository{ID: 303, Owner: "alice", Name: "projet"})
	expectPanic(t, "RemoveRepository inconnu", func() { f.RemoveRepository(101) })
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
		t.Errorf("Branches = %v, %v\nveut %v", got, err, want)
	}
	f.DeleteBranch(101, "dev")
	if got, _ := f.Branches(ctx, "alice", "projet"); len(got) != 3 {
		t.Errorf("après DeleteBranch : %v", got)
	}

	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	if got, err := f.Branches(ctx, "alice", "vide"); err != nil || got == nil || len(got) != 0 {
		t.Errorf("dépôt vide : %#v, %v, veut une liste vide non nil", got, err)
	}

	// Plafond provider.MaxBranches.
	f.AddRepository(provider.Repository{ID: 3, Owner: "alice", Name: "branches"})
	f.AppendCommits(3, "main", 1, t0, 0)
	head, _ := f.Branches(ctx, "alice", "branches")
	for i := range provider.MaxBranches + 5 {
		f.SetBranch(3, fmt.Sprintf("b%05d", i), head[0].HeadSHA)
	}
	got, err = f.Branches(ctx, "alice", "branches")
	if err != nil || len(got) != provider.MaxBranches || got[0].Name != "b00000" {
		t.Errorf("plafond : %d branches (première %v), %v ; veut %d", len(got), got[0], err, provider.MaxBranches)
	}
}

func TestListCommits(t *testing.T) {
	f, g := newGraph(t)
	cases := []struct {
		name string
		opts provider.ListCommitsOptions
		want []string
	}{
		{"branche par défaut", provider.ListCommitsOptions{}, []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}},
		{"main", provider.ListCommitsOptions{Branch: "main"}, []string{g.merge, g.f2, g.m2, g.f1, g.m1, g.r1}},
		{"feature", provider.ListCommitsOptions{Branch: "feature"}, []string{g.f2, g.f1, g.r1}},
		{"SHA", provider.ListCommitsOptions{Branch: g.m2}, []string{g.m2, g.m1, g.r1}},
		{"Since", provider.ListCommitsOptions{Since: t0.Add(2 * time.Hour)}, []string{g.merge, g.f2, g.m2, g.f1}},
		{"Since autre fuseau", provider.ListCommitsOptions{Since: t0.Add(2 * time.Hour).In(time.FixedZone("X", -5*3600))}, []string{g.merge, g.f2, g.m2, g.f1}},
		// Since tronqué à la seconde, comme le since envoyé à GitHub : f1 (t0 + 2 h pile) reste.
		{"Since à fraction de seconde", provider.ListCommitsOptions{Since: t0.Add(2*time.Hour + 500*time.Millisecond)}, []string{g.merge, g.f2, g.m2, g.f1}},
		{"Since juste après la seconde", provider.ListCommitsOptions{Since: t0.Add(2*time.Hour + time.Second)}, []string{g.merge, g.f2, g.m2}},
		{"Since futur", provider.ListCommitsOptions{Since: t0.Add(24 * time.Hour)}, []string{}},
	}
	for _, c := range cases {
		got, err := f.ListCommits(ctx, "alice", "projet", c.opts)
		if err != nil || got == nil || !reflect.DeepEqual(shas(got), c.want) {
			t.Errorf("%s : %v, %v\nveut %v", c.name, shas(got), err, c.want)
		}
	}

	got, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	m := got[0]
	if m.Message != "fusion" || m.Author != alice || m.URL != "https://github.com/alice/projet/commit/"+g.merge ||
		!m.CommittedAt.Equal(t0.Add(5*time.Hour)) || !reflect.DeepEqual(m.Parents, []string{g.m2, g.f2}) {
		t.Errorf("commit de fusion : %+v", m)
	}
	if root := got[5]; root.Parents == nil || len(root.Parents) != 0 {
		t.Errorf("commit racine : Parents %#v, veut une slice vide non nil", root.Parents)
	}
	// Copies indépendantes.
	got[0].Parents[0] = "modifié"
	got[0].Message = "modifié"
	again, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	if again[0].Parents[0] != g.m2 || again[0].Message != "fusion" {
		t.Error("modifier un résultat a modifié le Fake")
	}

	for _, branch := range []string{"inconnue", SHA("absent")} {
		_, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: branch})
		wantAPIError(t, "branche "+branch, err, 404, provider.ErrNotFound)
	}
	if _, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Branch: "a\nb"}); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("branche invalide : %v, veut ErrInvalidRef", err)
	}

	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	for _, branch := range []string{"", "main", "inconnue"} {
		got, err := f.ListCommits(ctx, "alice", "vide", provider.ListCommitsOptions{Branch: branch})
		if err != nil || got == nil || len(got) != 0 {
			t.Errorf("dépôt vide, branche %q : %#v, %v ; veut une liste vide non nil sans erreur", branch, got, err)
		}
	}

	// À date égale, le dernier ajouté d'abord.
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
		t.Errorf("dates égales : %v, veut %v", shas(got), want)
	}
}

func TestListCommitsStopAndLimit(t *testing.T) {
	f, g := newGraph(t)
	var seen []string
	stop := func(sha string) bool {
		seen = append(seen, sha)
		_ = f.Calls(MethodListCommits) // Stop est appelé hors du verrou : pas d'interblocage
		return sha == g.m2
	}
	got, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: stop})
	if err != nil || !reflect.DeepEqual(shas(got), []string{g.merge, g.f2}) {
		t.Errorf("Stop : %v, %v ; veut [merge f2]", shas(got), err)
	}
	if !reflect.DeepEqual(seen, []string{g.merge, g.f2, g.m2}) {
		t.Errorf("Stop appelé sur %v, veut [merge f2 m2]", seen)
	}

	seen = nil
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: stop, Limit: 1})
	if !reflect.DeepEqual(shas(got), []string{g.merge}) || len(seen) != 1 {
		t.Errorf("Limit 1 : %v, Stop appelé %d fois ; veut [merge] et 1 appel", shas(got), len(seen))
	}

	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Stop: func(sha string) bool { return sha == g.merge }})
	if got == nil || len(got) != 0 {
		t.Errorf("Stop sur le premier commit : %#v, veut une liste vide non nil", got)
	}
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Since: t0.Add(4 * time.Hour), Stop: func(string) bool { return false }})
	if !reflect.DeepEqual(shas(got), []string{g.merge, g.f2}) {
		t.Errorf("Since + Stop : %v", shas(got))
	}
	for _, limit := range []int{-1, 0, provider.MaxCommits + 1} {
		if got, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Limit: limit}); len(got) != 6 {
			t.Errorf("Limit %d : %d commits, veut 6", limit, len(got))
		}
	}

	// Plafond provider.MaxCommits.
	f.AddRepository(provider.Repository{ID: 9, Owner: "alice", Name: "gros"})
	created := f.AppendCommits(9, "main", provider.MaxCommits+2, t0, time.Second)
	got, err = f.ListCommits(ctx, "alice", "gros", provider.ListCommitsOptions{Limit: provider.MaxCommits + 50})
	if err != nil || len(got) != provider.MaxCommits || got[0].SHA != created[len(created)-1].SHA {
		t.Errorf("plafond : %d commits, %v ; veut %d en commençant par le plus récent", len(got), err, provider.MaxCommits)
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
		{g.merge, g.m2, []string{}}, // head contenu dans base
		{g.merge, g.merge, []string{}},
	}
	for _, c := range cases {
		got, err := f.CompareCommits(ctx, "alice", "projet", c.base, c.head)
		if err != nil || got == nil || !reflect.DeepEqual(shas(got), c.want) {
			t.Errorf("%s...%s : %v, %v\nveut %v", provider.ShortSHA(c.base), provider.ShortSHA(c.head), shas(got), err, c.want)
		}
	}

	// base inconnue : ErrUnknownBase et surtout pas ErrNotFound.
	_, err := f.CompareCommits(ctx, "alice", "projet", SHA("absent"), g.merge)
	wantAPIError(t, "base inconnue", err, 404, provider.ErrUnknownBase)
	if errors.Is(err, provider.ErrNotFound) {
		t.Error("base inconnue : l'erreur ne doit pas envelopper ErrNotFound")
	}
	if op := asAPI(t, err).Op; op != fmt.Sprintf("comparaison %s...%s dans alice/projet", provider.ShortSHA(SHA("absent")), provider.ShortSHA(g.merge)) {
		t.Errorf("Op = %q", op)
	}

	// head inconnu : comme le Provider GitHub, qui ne peut pas distinguer ce cas d'une base inconnue
	// (GitHub répond 404 dans les deux cas, sur un dépôt accessible), ErrUnknownBase.
	_, err = f.CompareCommits(ctx, "alice", "projet", g.merge, SHA("absent"))
	wantAPIError(t, "head inconnu", err, 404, provider.ErrUnknownBase)
	if errors.Is(err, provider.ErrNotFound) {
		t.Error("head inconnu : l'erreur ne doit pas envelopper ErrNotFound")
	}

	// Force-push puis nettoyage : l'ancienne tête disparaît.
	f.SetBranch(101, "feature", g.f1)
	if got, err := f.CompareCommits(ctx, "alice", "projet", g.f2, g.merge); err != nil || len(got) != 3 {
		t.Errorf("avant nettoyage : %v, %v", shas(got), err)
	}
	f.RemoveCommit(101, g.f2)
	_, err = f.CompareCommits(ctx, "alice", "projet", g.f2, g.merge)
	wantAPIError(t, "base retirée", err, 404, provider.ErrUnknownBase)

	// Historiques sans ancêtre commun.
	orphan := SHA("orphelin")
	f.AddCommits(101, provider.Commit{SHA: orphan, CommittedAt: t0.Add(9 * time.Hour)})
	f.SetBranch(101, "orpheline", orphan)
	_, err = f.CompareCommits(ctx, "alice", "projet", g.m1, orphan)
	wantAPIError(t, "sans ancêtre commun", err, 404, provider.ErrUnknownBase)

	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	_, err = f.CompareCommits(ctx, "alice", "vide", g.m1, g.merge)
	wantAPIError(t, "dépôt vide", err, 409, provider.ErrEmptyRepository)

	for _, args := range [][2]string{{"abc", g.merge}, {g.merge, strings.ToUpper(g.m1)}} {
		if _, err := f.CompareCommits(ctx, "alice", "projet", args[0], args[1]); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("SHA invalide %v : %v, veut ErrInvalidRef", args, err)
		}
	}

	// base == head : aucun contrôle après la validation (le Provider GitHub n'envoie rien).
	f.SetRateLimit(provider.RateLimit{Limit: 10, Remaining: 0, Reset: time.Now().Add(time.Hour)})
	f.SetError(MethodCompareCommits, errors.New("injectée"))
	f.SetRepoError("alice", "projet", errors.New("dépôt"))
	if got, err := f.CompareCommits(ctx, "alice", "projet", g.merge, g.merge); err != nil || got == nil || len(got) != 0 {
		t.Errorf("base == head : %#v, %v ; veut une liste vide non nil sans erreur", got, err)
	}
}

func TestCompareCommitsMaxCommits(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 1, Owner: "alice", Name: "gros"})
	created := f.AppendCommits(1, "main", provider.MaxCommits+2, t0, time.Second)
	got, err := f.CompareCommits(ctx, "alice", "gros", created[0].SHA, created[len(created)-1].SHA)
	// MaxCommits+1 commits en avance : les MaxCommits plus anciens sont gardés (comme le Provider
	// GitHub, qui lit les pages du plus ancien au plus récent), du plus récent au plus ancien.
	if err != nil || len(got) != provider.MaxCommits || got[0].SHA != created[provider.MaxCommits].SHA ||
		got[len(got)-1].SHA != created[1].SHA {
		t.Errorf("plafond : %d commits, %v", len(got), err)
	}
}

func TestCommitStats(t *testing.T) {
	f, g := newGraph(t)
	f.SetStats(101, g.merge, provider.CommitStats{Additions: 12, Deletions: 3})
	if s, err := f.CommitStats(ctx, "alice", "projet", g.merge); err != nil || s != (provider.CommitStats{Additions: 12, Deletions: 3}) {
		t.Errorf("CommitStats(merge) = %+v, %v", s, err)
	}
	if s, err := f.CommitStats(ctx, "alice", "projet", g.r1); err != nil || s != (provider.CommitStats{}) {
		t.Errorf("CommitStats(r1) = %+v, %v ; veut zéro", s, err)
	}
	_, err := f.CommitStats(ctx, "alice", "projet", SHA("absent"))
	wantAPIError(t, "commit inconnu", err, 404, provider.ErrNotFound)
	if op := asAPI(t, err).Op; op != "statistiques du commit "+provider.ShortSHA(SHA("absent"))+" de alice/projet" {
		t.Errorf("Op = %q", op)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", "main"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("SHA invalide : %v", err)
	}
	f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "vide"})
	_, err = f.CommitStats(ctx, "alice", "vide", g.r1)
	wantAPIError(t, "dépôt vide", err, 409, provider.ErrEmptyRepository)

	// Un commit retiré perd ses statistiques.
	f.SetBranch(101, "feature", g.f1)
	f.SetStats(101, g.f2, provider.CommitStats{Additions: 1})
	f.RemoveCommit(101, g.f2)
	_, err = f.CommitStats(ctx, "alice", "projet", g.f2)
	wantAPIError(t, "commit retiré", err, 404, provider.ErrNotFound)
}

func TestInjectedErrors(t *testing.T) {
	f, g := newGraph(t)
	errA, errB, errC := errors.New("A"), errors.New("B"), errors.New("C")
	f.SetError(MethodRepository, errC)
	f.FailNext(MethodRepository, errA)
	f.FailNext(MethodRepository, errB)
	for i, want := range []error{errA, errB, errC, errC} {
		if _, err := f.Repository(ctx, "alice", "projet"); err != want {
			t.Errorf("appel %d : %v, veut %v", i+1, err, want)
		}
	}
	f.SetError(MethodRepository, nil)
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Errorf("après SetError(nil) : %v", err)
	}
	if n := f.Calls(MethodRepository); n != 5 {
		t.Errorf("Calls = %d, veut 5 (échecs compris)", n)
	}

	// Les autres méthodes ne sont pas touchées.
	f.SetError(MethodBranches, errA)
	if _, err := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{}); err != nil {
		t.Errorf("ListCommits touché par SetError(Branches) : %v", err)
	}

	// Ordre : contexte, puis validation, puis injection.
	f.FailNext(MethodCommitStats, errA)
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.CommitStats(canceled, "alice", "projet", "invalide"); !errors.Is(err, context.Canceled) {
		t.Errorf("contexte annulé : %v, veut context.Canceled", err)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", "invalide"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("validation avant injection : %v", err)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", g.merge); err != errA {
		t.Errorf("injection après validation : %v, veut A", err)
	}
	if _, err := f.CommitStats(ctx, "alice", "projet", g.merge); err != nil {
		t.Errorf("file vidée : %v", err)
	}
	if n := f.Calls(MethodCommitStats); n != 4 {
		t.Errorf("Calls(CommitStats) = %d, veut 4", n)
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
			t.Errorf("FailNext méthode %d : %v", i, err)
		}
	}

	expectPanic(t, "SetError méthode inconnue", func() { f.SetError("Inconnue", errA) })
	expectPanic(t, "FailNext méthode inconnue", func() { f.FailNext("Name", errA) })
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
			t.Errorf("%s : %v, veut l'erreur SSO posée", m, err)
		}
	}
	if _, err := f.Repository(ctx, "ecole", "autre"); err != nil {
		t.Errorf("autre dépôt touché : %v", err)
	}

	// Après renommage, l'erreur suit le dépôt (posée sur l'ancien nom).
	f.UpdateRepository(provider.Repository{ID: 101, Owner: "ecole", Name: "projet"})
	if _, err := f.Repository(ctx, "ecole", "projet"); err != error(sso) {
		t.Errorf("nom actuel après renommage : %v", err)
	}
	// Rétablir par le nom actuel lève aussi l'erreur posée sur l'ancien.
	f.SetRepoError("ecole", "projet", nil)
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Errorf("après SetRepoError(nil) : %v", err)
	}

	// Posée avant l'ajout du dépôt.
	f.SetRepoError("bob", "futur", provider.ErrForbidden)
	if _, err := f.Repository(ctx, "bob", "futur"); err != provider.ErrForbidden {
		t.Errorf("dépôt encore inconnu : %v, veut l'erreur posée", err)
	}
	f.AddRepository(provider.Repository{ID: 303, Owner: "bob", Name: "futur"})
	if _, err := f.RepositoryByID(ctx, 303); err != provider.ErrForbidden {
		t.Errorf("RepositoryByID d'un dépôt ajouté après SetRepoError : %v", err)
	}
	// L'erreur injectée par méthode passe avant celle du dépôt.
	f.FailNext(MethodRepositoryByID, context.DeadlineExceeded)
	if _, err := f.RepositoryByID(ctx, 303); err != context.DeadlineExceeded {
		t.Errorf("FailNext avant SetRepoError : %v", err)
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
	_, _ = f.Repository(ctx, "", "") // invalide : compté quand même
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
		t.Fatalf("CallLog : %d appels, veut %d", len(log), len(want))
	}
	for i := range want {
		if log[i].Opts.Stop != nil {
			t.Errorf("appel %d : Opts.Stop non nil", i)
		}
		log[i].Opts.Stop = nil
		if !reflect.DeepEqual(log[i], want[i]) {
			t.Errorf("appel %d : %+v\nveut %+v", i, log[i], want[i])
		}
	}
	if f.Calls(MethodListCommits) != 2 || f.Calls(MethodRepository) != 2 {
		t.Errorf("Calls : ListCommits %d, Repository %d", f.Calls(MethodListCommits), f.Calls(MethodRepository))
	}
	log[0].Owner = "modifié"
	if f.CallLog()[0].Owner != "Alice" {
		t.Error("CallLog ne renvoie pas une copie")
	}
	f.ResetCalls()
	if len(f.CallLog()) != 0 || f.Calls(MethodRepository) != 0 {
		t.Error("ResetCalls n'a pas tout vidé")
	}
}

func TestRateLimit(t *testing.T) {
	c := &clock{now: t0.Add(500 * time.Millisecond)}
	f, _ := newGraph(t)
	f.SetClock(c.Now)

	if _, ok := f.LastRateLimit(); ok {
		t.Error("LastRateLimit sans SetRateLimit : true, veut false")
	}
	rl, err := f.RateLimit(ctx)
	if err != nil || rl.Limit != 5000 || rl.Remaining != 5000 || rl.Used != 0 || !rl.Reset.Equal(t0.Add(time.Hour)) || !rl.ObservedAt.Equal(c.Now()) {
		t.Errorf("RateLimit sans quota simulé = %+v, %v", rl, err)
	}
	if _, ok := f.LastRateLimit(); ok {
		t.Error("LastRateLimit après RateLimit sans SetRateLimit : true, veut false")
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
	if _, err := f.Branches(ctx, "alice", "inconnu"); !errors.Is(err, provider.ErrNotFound) { // une 404 consomme aussi
		t.Fatal(err)
	}
	if rl, _ := f.RateLimit(ctx); rl.Remaining != 0 || rl.Used != 10 {
		t.Errorf("après 2 appels : %+v", rl)
	}
	_, err = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	wantAPIError(t, "quota épuisé", err, 403, provider.ErrRateLimited)
	var rle *provider.RateLimitError
	if !errors.As(err, &rle) || !rle.Reset.Equal(reset) || rle.Secondary {
		t.Errorf("RateLimitError = %+v", rle)
	}
	// RateLimit reste permis et gratuit ; la validation passe avant le quota.
	if _, err := f.RateLimit(ctx); err != nil {
		t.Errorf("RateLimit quota épuisé : %v", err)
	}
	if _, err := f.Repository(ctx, "alice", "a/b"); !errors.Is(err, provider.ErrInvalidRef) {
		t.Errorf("validation avant le quota : %v", err)
	}

	// Remise à zéro : Reset avancé d'heure en heure jusqu'à dépasser maintenant.
	c.Advance(2*time.Hour + 10*time.Minute) // maintenant = t0 + 2 h 10 min 0,5 s
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Fatalf("après la remise à zéro : %v", err)
	}
	rl, _ = f.LastRateLimit()
	if rl.Remaining != 9 || rl.Used != 1 || !rl.Reset.Equal(t0.Add(150*time.Minute)) {
		t.Errorf("nouvelle fenêtre : %+v, veut Remaining 9, Used 1, Reset %s", rl, t0.Add(150*time.Minute))
	}

	// Valeurs par défaut de SetRateLimit.
	f.SetRateLimit(provider.RateLimit{Remaining: 4000})
	rl, _ = f.LastRateLimit()
	if rl.Limit != 5000 || rl.Used != 1000 || !rl.Reset.Equal(c.Now().Add(time.Hour).Truncate(time.Second)) {
		t.Errorf("SetRateLimit par défaut : %+v", rl)
	}
	expectPanic(t, "Remaining négatif", func() { f.SetRateLimit(provider.RateLimit{Remaining: -1}) })
	f.SetClock(nil)
	if rl, _ := f.LastRateLimit(); time.Since(rl.ObservedAt) > time.Minute {
		t.Errorf("SetClock(nil) : ObservedAt %s, veut l'heure réelle", rl.ObservedAt)
	}
}

func TestAppendCommits(t *testing.T) {
	f := New()
	f.AddRepository(provider.Repository{ID: 5, Owner: "alice", Name: "projet"})
	if got := f.AppendCommits(5, "main", 0, t0, time.Hour); got == nil || len(got) != 0 {
		t.Errorf("n = 0 : %#v", got)
	}
	if bs, _ := f.Branches(ctx, "alice", "projet"); len(bs) != 0 {
		t.Errorf("n = 0 a créé une branche : %v", bs)
	}

	start := time.Date(2026, 10, 1, 10, 0, 0, 700, time.FixedZone("CEST", 2*3600))
	first := f.AppendCommits(5, "main", 2, start, time.Hour)
	student := provider.Person{Name: "Étudiant Test", Email: "etudiant@example.test", Login: "etudiant-test", ID: 1001,
		AvatarURL: "https://avatars.example.test/u/1001", ProfileURL: "https://github.example.test/etudiant-test"}
	want0 := provider.Commit{SHA: SHA("5/main/1"), Message: "commit 1", URL: "https://github.com/alice/projet/commit/" + SHA("5/main/1"),
		Author: student, Committer: student, AuthoredAt: t0, CommittedAt: t0, Parents: []string{}}
	if !reflect.DeepEqual(first[0], want0) {
		t.Errorf("premier commit : %+v\nveut %+v", first[0], want0)
	}
	if first[1].SHA != SHA("5/main/2") || first[1].Message != "commit 2" || !reflect.DeepEqual(first[1].Parents, []string{first[0].SHA}) ||
		!first[1].CommittedAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("deuxième commit : %+v", first[1])
	}

	// Suite de la même branche, en évitant un SHA déjà présent : le commit ajouté à la main porte le
	// SHA que le prochain AppendCommits aurait choisi (rang 4).
	f.AddCommits(5, provider.Commit{SHA: SHA("5/main/4"), CommittedAt: t0, Parents: []string{first[1].SHA}})
	more := f.AppendCommits(5, "main", 1, t0.Add(5*time.Hour), 0)
	if more[0].SHA != SHA("5/main/5") || more[0].Message != "commit 5" || more[0].Parents[0] != first[1].SHA {
		t.Errorf("suite : %+v, veut le SHA de « 5/main/5 »", more[0])
	}
	got, _ := f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{})
	if want := []string{more[0].SHA, first[1].SHA, first[0].SHA}; !reflect.DeepEqual(shas(got), want) {
		t.Errorf("historique : %v, veut %v", shas(got), want)
	}

	// Modifier le résultat ne modifie pas le Fake.
	more[0].Parents[0] = "modifié"
	got, _ = f.ListCommits(ctx, "alice", "projet", provider.ListCommitsOptions{Limit: 1})
	if got[0].Parents[0] != first[1].SHA {
		t.Error("le résultat de AppendCommits partage sa mémoire avec le Fake")
	}
}

func TestPreparationPanics(t *testing.T) {
	f, g := newGraph(t)
	cases := map[string]func(){
		"AddRepository ID nul":          func() { f.AddRepository(provider.Repository{Owner: "a", Name: "b"}) },
		"AddRepository ID pris":         func() { f.AddRepository(provider.Repository{ID: 101, Owner: "a", Name: "b"}) },
		"AddRepository nom pris":        func() { f.AddRepository(provider.Repository{ID: 2, Owner: "ALICE", Name: "Projet"}) },
		"AddRepository nom invalide":    func() { f.AddRepository(provider.Repository{ID: 2, Owner: "alice", Name: "a/b"}) },
		"AddCommits dépôt inconnu":      func() { f.AddCommits(9, provider.Commit{SHA: SHA("x")}) },
		"AddCommits SHA invalide":       func() { f.AddCommits(101, provider.Commit{SHA: "abc"}) },
		"AddCommits SHA présent":        func() { f.AddCommits(101, provider.Commit{SHA: g.r1}) },
		"AddCommits parent inconnu":     func() { f.AddCommits(101, provider.Commit{SHA: SHA("x"), Parents: []string{SHA("y")}}) },
		"AppendCommits n négatif":       func() { f.AppendCommits(101, "main", -1, t0, 0) },
		"AppendCommits branche vide":    func() { f.AppendCommits(101, "", 1, t0, 0) },
		"AppendCommits dépôt inconnu":   func() { f.AppendCommits(9, "main", 1, t0, 0) },
		"SetBranch commit inconnu":      func() { f.SetBranch(101, "dev", SHA("inconnu")) },
		"SetBranch nom invalide":        func() { f.SetBranch(101, "a\x00b", g.r1) },
		"DeleteBranch inconnue":         func() { f.DeleteBranch(101, "inconnue") },
		"RemoveCommit tête":             func() { f.RemoveCommit(101, g.merge) },
		"RemoveCommit inconnu":          func() { f.RemoveCommit(101, SHA("inconnu")) },
		"SetStats commit inconnu":       func() { f.SetStats(101, SHA("inconnu"), provider.CommitStats{}) },
		"SetStats dépôt inconnu":        func() { f.SetStats(9, g.r1, provider.CommitStats{}) },
		"UpdateRepository nom invalide": func() { f.UpdateRepository(provider.Repository{ID: 101, Owner: "", Name: "x"}) },
	}
	for what, fn := range cases {
		expectPanic(t, what, fn)
	}
	// Un parent ajouté plus tôt dans le même appel est accepté.
	f.AddCommits(101,
		provider.Commit{SHA: SHA("p"), CommittedAt: t0, Parents: []string{g.r1}},
		provider.Commit{SHA: SHA("c"), CommittedAt: t0, Parents: []string{SHA("p")}},
	)
	// Le verrou a été rendu malgré les paniques.
	if _, err := f.Repository(ctx, "alice", "projet"); err != nil {
		t.Errorf("après les paniques : %v", err)
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
		t.Errorf("appels comptés : %d, veut 400", total)
	}
	if rl, _ := f.LastRateLimit(); rl.Used != 400-f.Calls(MethodRateLimit) {
		t.Errorf("quota consommé : %d, veut %d", rl.Used, 400-f.Calls(MethodRateLimit))
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
			t.Errorf("ProfileURL(%q) = %q, veut %q", login, got, want)
		}
	}
	if len(f.CallLog()) != 0 {
		t.Errorf("appels comptés : %v", f.CallLog())
	}
}
