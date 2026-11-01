package sync_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// req builds an addition request.
func req(line int, owner, name, label string, patterns ...string) wsync.AddRequest {
	r := wsync.AddRequest{Line: line, Ref: repoimport.Ref{Owner: owner, Name: name}, Label: label}
	if patterns != nil {
		r.BranchPatterns = patterns
	}
	return r
}

// forgeRepo adds a repository to the fake Provider only.
func forgeRepo(e *env, id int64, name string) {
	e.fake.AddRepository(provider.Repository{ID: id, Owner: testOwner, Name: name, DefaultBranch: "main",
		URL: "https://github.com/" + testOwner + "/" + name, Stars: 2, OpenIssues: 1, Private: true})
}

func TestAddRepositories(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	forgeRepo(e, 3, "sso")
	e.fake.SetRepoError(testOwner, "sso", &provider.SSORequiredError{Organization: testOwner})

	var progress [][2]int
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, "PROMO-DEMO", "alpha", "Alice et Bob", "main", "release/*"),
		req(2, testOwner, "beta", ""),
		req(3, testOwner, "inconnu", ""),
		req(4, testOwner, "sso", ""),
	}, wsync.AddOptions{Progress: func(done, total int) { progress = append(progress, [2]int{done, total}) }})
	if err != nil {
		t.Fatal(err)
	}
	want := []wsync.AddOutcome{wsync.OutcomeAdded, wsync.OutcomeAdded, wsync.OutcomeInaccessible, wsync.OutcomeInaccessible}
	for i, r := range res {
		if r.Outcome != want[i] {
			t.Errorf("result %d = %+v, want %s", i, r, want[i])
		}
		if r.Request.Line != i+1 {
			t.Errorf("result %d: line %d", i, r.Request.Line)
		}
	}
	alpha := res[0].Repo
	if alpha.ID == 0 || alpha.Owner != testOwner || alpha.Label != "Alice et Bob" || !alpha.Enabled ||
		!slices.Equal(alpha.BranchPatterns, []string{"main", "release/*"}) || alpha.ProviderID != 1 ||
		alpha.Stars != 2 || alpha.DefaultBranch != "main" || alpha.Provider != model.ProviderGitHub {
		t.Errorf("added repository = %+v", alpha)
	}
	if res[2].Error != "repository not found or account without access" || res[2].Repo.ID != 0 {
		t.Errorf("inaccessible = %+v", res[2])
	}
	if !strings.Contains(res[3].Error, "SSO") {
		t.Errorf("SSO = %+v", res[3])
	}
	if !slices.Equal(progress, [][2]int{{1, 4}, {2, 4}, {3, 4}, {4, 4}}) {
		t.Errorf("progression = %v", progress)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 4 {
		t.Errorf("Repository called %d times", n)
	}
	repos, _ := e.st.ListRepositories(t.Context())
	if len(repos) != 2 {
		t.Errorf("%d repositories in the database", len(repos))
	}

	// Idempotent: already tracked; label and patterns updated only if they are given.
	res, err = e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", ""),
		req(2, testOwner, "alpha", "Alice et Bob"),
		req(3, testOwner, "beta", "Bêta", "dev"),
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != wsync.OutcomeAlreadyTracked || res[1].Outcome != wsync.OutcomeAlreadyTracked ||
		res[2].Outcome != wsync.OutcomeUpdated {
		t.Errorf("results = %+v", res)
	}
	if res[0].Repo.Label != "Alice et Bob" || !slices.Equal(res[0].Repo.BranchPatterns, []string{"main", "release/*"}) {
		t.Errorf("settings overwritten: %+v", res[0].Repo)
	}
	beta, _ := e.st.GetRepository(t.Context(), res[2].Repo.ID)
	if beta.Label != "Bêta" || !slices.Equal(beta.BranchPatterns, []string{"dev"}) {
		t.Errorf("beta in the database = %+v", beta)
	}

	// Tracking stopped (repo remove --keep-history): resumed by a new addition.
	if err := e.st.SetRepositoryEnabled(t.Context(), alpha.ID, false); err != nil {
		t.Fatal(err)
	}
	res, _ = e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, testOwner, "alpha", "")}, wsync.AddOptions{})
	if res[0].Outcome != wsync.OutcomeUpdated || !res[0].Repo.Enabled {
		t.Errorf("resume = %+v", res[0])
	}
	if stored, _ := e.st.GetRepository(t.Context(), alpha.ID); !stored.Enabled {
		t.Error("tracking not resumed in the database")
	}
}

// Renamed repository: requested by its old name, recorded under the current name; found afterwards by
// its forge identifier.
func TestAddRenomme(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "ancien")
	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: "nouvelle-orga", Name: "nouveau"})
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, testOwner, "ancien", "")}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != wsync.OutcomeAdded || res[0].Repo.FullName() != "nouvelle-orga/nouveau" {
		t.Errorf("result = %+v", res[0])
	}
	res, _ = e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, "nouvelle-orga", "nouveau", "")}, wsync.AddOptions{})
	if res[0].Outcome != wsync.OutcomeAlreadyTracked {
		t.Errorf("second addition = %+v", res[0])
	}
}

// DryRun: same results, no write, consistent duplicates within the batch.
func TestAddDryRun(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	existing, err := e.st.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub, ProviderID: 2,
		Owner: testOwner, Name: "beta", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", "A"),
		req(2, testOwner, "alpha", "A"),
		req(3, testOwner, "beta", "B"),
		req(4, testOwner, "beta", "B"),
	}, wsync.AddOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []wsync.AddOutcome{wsync.OutcomeAdded, wsync.OutcomeAlreadyTracked, wsync.OutcomeUpdated, wsync.OutcomeAlreadyTracked}
	for i, r := range res {
		if r.Outcome != want[i] {
			t.Errorf("result %d = %s, want %s", i, r.Outcome, want[i])
		}
	}
	if res[0].Repo.Label != "A" || res[0].Repo.ID != 0 || res[2].Repo.Label != "B" {
		t.Errorf("results = %+v", res)
	}
	repos, _ := e.st.ListRepositories(t.Context())
	if len(repos) != 1 || repos[0].Label != existing.Label {
		t.Errorf("DryRun wrote: %+v", repos)
	}
}

// Exhausted quota: this repository and the following ones in error, without a new request.
func TestAddQuotaEpuise(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	forgeRepo(e, 3, "gamma")
	e.fake.FailNext(providertest.MethodRepository, &provider.APIError{Op: "lecture", Status: 403,
		Err: &provider.RateLimitError{Reset: time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)}})
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", ""), req(2, testOwner, "beta", ""), req(3, testOwner, "gamma", ""),
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Outcome != wsync.OutcomeError || !strings.Contains(r.Error, "rate limit") {
			t.Errorf("result = %+v", r)
		}
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 1 {
		t.Errorf("Repository called %d times", n)
	}
	if _, err := e.st.GetRepositoryByProviderID(t.Context(), model.ProviderGitHub, 2); err == nil {
		t.Error("repository added despite the quota")
	}
}

// Other failure (network): OutcomeError for this repository only; token masked.
func TestAddAutreErreur(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	e.fake.FailNext(providertest.MethodRepository, &provider.APIError{Op: "lecture", Err: provider.ErrUnavailable})
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", ""), req(2, testOwner, "beta", ""),
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != wsync.OutcomeError || res[1].Outcome != wsync.OutcomeAdded {
		t.Errorf("results = %+v", res)
	}
	for _, r := range res {
		assertNoLeak(t, "result", r.Error)
	}
	if _, err := e.st.GetRepositoryByProviderID(t.Context(), model.ProviderGitHub, 1); err == nil {
		t.Error("repository in error added")
	} else if !strings.Contains(err.Error(), store.ErrNotFound.Error()) {
		t.Errorf("read: %v", err)
	}
}

// Canceled context: global error.
func TestAddAnnule(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.eng.AddRepositories(ctx, []wsync.AddRequest{req(1, testOwner, "alpha", "")}, wsync.AddOptions{}); err == nil {
		t.Error("canceled context: error expected")
	}
}

// An invalid label (control character) is refused before any call to the forge, as in
// the import.
func TestAddLabelInvalide(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	_, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, testOwner, "alpha", "a\x1b[2Jb")},
		wsync.AddOptions{})
	if err == nil || !strings.Contains(err.Error(), "control character not allowed") {
		t.Fatalf("error = %v", err)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository called %d times", n)
	}
	if repos, err := e.st.ListRepositories(t.Context()); err != nil || len(repos) != 0 {
		t.Errorf("repositories in the database = %v, %v", repos, err)
	}
}

// days returns a pointer to n (activity threshold of a request).
func days(n int) *int { return &n }

// thresholdsOf describes the thresholds of a repository ("3/10", "-/-" for the global values).
func thresholdsOf(r model.Repository) string {
	s := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	return s(r.FlemmeAfterDays) + "/" + s(r.DeadAfterDays)
}

// Activity thresholds per repository: recorded on creation, updated if they change, kept if they are
// not given, reset to the global value with 0; DryRun computes without writing.
func TestAddThresholds(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	add := func(r wsync.AddRequest, dryRun bool) wsync.AddResult {
		t.Helper()
		res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{r}, wsync.AddOptions{DryRun: dryRun})
		if err != nil || len(res) != 1 {
			t.Fatalf("addition: %v, %v", res, err)
		}
		return res[0]
	}
	stored := func() string {
		t.Helper()
		repo, err := e.st.GetRepositoryByName(t.Context(), model.ProviderGitHub, testOwner, "alpha")
		if err != nil {
			t.Fatal(err)
		}
		return thresholdsOf(repo)
	}
	withT := func(flemme, dead *int) wsync.AddRequest {
		r := req(0, testOwner, "alpha", "")
		r.FlemmeAfterDays, r.DeadAfterDays = flemme, dead
		return r
	}

	if r := add(withT(days(3), days(10)), false); r.Outcome != wsync.OutcomeAdded || thresholdsOf(r.Repo) != "3/10" {
		t.Errorf("creation: %s %s", r.Outcome, thresholdsOf(r.Repo))
	}
	if got := stored(); got != "3/10" {
		t.Errorf("in the database after creation: %s", got)
	}
	steps := []struct {
		name    string
		req     wsync.AddRequest
		dryRun  bool
		outcome wsync.AddOutcome
		repo    string // thresholds of the result
		stored  string // thresholds in the database afterwards
	}{
		{"not given", withT(nil, nil), false, wsync.OutcomeAlreadyTracked, "3/10", "3/10"},
		{"identical", withT(days(3), days(10)), false, wsync.OutcomeAlreadyTracked, "3/10", "3/10"},
		{"simulation", withT(days(4), nil), true, wsync.OutcomeUpdated, "4/10", "3/10"},
		{"flemme changed", withT(days(4), nil), false, wsync.OutcomeUpdated, "4/10", "4/10"},
		{"dead reset to the global value", withT(nil, days(0)), false, wsync.OutcomeUpdated, "4/-", "4/-"},
		{"everything reset to the global value", withT(days(0), days(0)), false, wsync.OutcomeUpdated, "-/-", "-/-"},
		{"0 without own threshold", withT(days(0), nil), false, wsync.OutcomeAlreadyTracked, "-/-", "-/-"},
	}
	for _, s := range steps {
		r := add(s.req, s.dryRun)
		if r.Outcome != s.outcome || thresholdsOf(r.Repo) != s.repo {
			t.Errorf("%s: %s %s, want %s %s", s.name, r.Outcome, thresholdsOf(r.Repo), s.outcome, s.repo)
		}
		if got := stored(); got != s.stored {
			t.Errorf("%s: in the database %s, want %s", s.name, got, s.stored)
		}
	}

	// Label and thresholds changed together: both are written.
	r := withT(days(2), nil)
	r.Label = "Alpha"
	if res := add(r, false); res.Outcome != wsync.OutcomeUpdated || res.Repo.Label != "Alpha" || thresholdsOf(res.Repo) != "2/-" {
		t.Errorf("label and threshold: %+v", res)
	}
	if got := stored(); got != "2/-" {
		t.Errorf("in the database: %s", got)
	}
}

// Invalid thresholds: error before any call to the forge.
func TestAddThresholdsInvalides(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	for _, c := range []struct {
		flemme, dead *int
		want         string
	}{
		{days(10), days(5), "strictly greater"},
		{days(-2), nil, "flemme threshold"},
	} {
		r := req(0, testOwner, "alpha", "")
		r.FlemmeAfterDays, r.DeadAfterDays = c.flemme, c.dead
		_, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{r}, wsync.AddOptions{})
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("error = %v, want %q", err, c.want)
		}
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository called %d times", n)
	}
}

func TestSetThresholds(t *testing.T) {
	e := newEnv(t)
	repo, err := e.st.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub, ProviderID: 1,
		Owner: testOwner, Name: "alpha", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.eng.SetThresholds(t.Context(), repo.ID, days(4), days(12))
	if err != nil || thresholdsOf(got) != "4/12" {
		t.Fatalf("SetThresholds = %s, %v", thresholdsOf(got), err)
	}
	// 0 and nil: thresholds of the workspace.
	if got, err = e.eng.SetThresholds(t.Context(), repo.ID, days(0), nil); err != nil || thresholdsOf(got) != "-/-" {
		t.Errorf("reset = %s, %v", thresholdsOf(got), err)
	}
	// Invalid: nothing written.
	if _, err := e.eng.SetThresholds(t.Context(), repo.ID, days(8), days(8)); err == nil ||
		!strings.Contains(err.Error(), "strictly greater") {
		t.Errorf("inconsistent thresholds: %v", err)
	}
	if _, err := e.eng.SetThresholds(t.Context(), 999, days(3), nil); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("nonexistent repository: %v", err)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository called %d times", n)
	}
}
