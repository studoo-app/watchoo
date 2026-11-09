package sync_test

import (
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func strp(s string) *string { return &s }

// repo add --conventions-since: creation, change, back to the workspace date (""), not given, dry run.
func TestAddConventionsSince(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	add := func(since *string, dryRun bool) wsync.AddResult {
		t.Helper()
		r := req(0, testOwner, "alpha", "")
		r.ConventionsSince = since
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
		return repo.ConventionsSince
	}

	if r := add(strp(" 2026-09-01 "), false); r.Outcome != wsync.OutcomeAdded || r.Repo.ConventionsSince != "2026-09-01" {
		t.Errorf("creation: %s %q", r.Outcome, r.Repo.ConventionsSince)
	}
	steps := []struct {
		name    string
		since   *string
		dryRun  bool
		outcome wsync.AddOutcome
		repo    string // date of the result
		stored  string // date in the database afterwards
	}{
		{"not given", nil, false, wsync.OutcomeAlreadyTracked, "2026-09-01", "2026-09-01"},
		{"identical", strp("2026-09-01"), false, wsync.OutcomeAlreadyTracked, "2026-09-01", "2026-09-01"},
		{"simulation", strp("2027-01-04"), true, wsync.OutcomeUpdated, "2027-01-04", "2026-09-01"},
		{"changed", strp("2027-01-04"), false, wsync.OutcomeUpdated, "2027-01-04", "2027-01-04"},
		{"back to the workspace date", strp(""), false, wsync.OutcomeUpdated, "", ""},
		{"empty without own date", strp(""), false, wsync.OutcomeAlreadyTracked, "", ""},
	}
	for _, s := range steps {
		r := add(s.since, s.dryRun)
		if r.Outcome != s.outcome || r.Repo.ConventionsSince != s.repo {
			t.Errorf("%s: %s %q, want %s %q", s.name, r.Outcome, r.Repo.ConventionsSince, s.outcome, s.repo)
		}
		if got := stored(); got != s.stored {
			t.Errorf("%s: in the database %q, want %q", s.name, got, s.stored)
		}
	}
}

// Invalid date: error before any call to the forge.
func TestAddConventionsSinceInvalid(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	r := req(0, testOwner, "alpha", "")
	r.ConventionsSince = strp("01/09/2026")
	if _, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{r}, wsync.AddOptions{}); err == nil ||
		!strings.Contains(err.Error(), "YYYY-MM-DD") {
		t.Errorf("error = %v", err)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository called %d times", n)
	}
}
