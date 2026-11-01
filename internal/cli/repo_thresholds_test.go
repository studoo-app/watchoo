package cli

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// thresholdPair describes the own thresholds of a saved repository ("3/10", "-/-" = workspace
// thresholds).
func thresholdPair(t *testing.T, h *harness, owner, name string) string {
	t.Helper()
	repo := storedRepo(t, h, owner, name)
	s := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	return s(repo.FlemmeAfterDays) + "/" + s(repo.DeadAfterDays)
}

// "repo add --flemme / --dead": thresholds saved, displayed, exposed in JSON and applied by
// "repo list".
func TestRepoAddThresholds(t *testing.T) {
	h := newSyncedWorkspace(t) // bob/projet: last commit 9 days ago (Flemme with 5 / 15)

	r := h.run("repo", "add", "bob/projet", "--flemme", "10", "--dead", "20").mustSucceed(t)
	assertContains(t, "repo add --flemme", r.stdout,
		"Repository bob/projet already tracked, updated (all branches, thresholds: flemme 10 d, dead 20 d).")
	if got := thresholdPair(t, h, "bob", "projet"); got != "10/20" {
		t.Errorf("saved thresholds = %s", got)
	}
	r = h.run("repo", "add", "bob/projet", "--flemme", "10").mustSucceed(t)
	assertContains(t, "same thresholds", r.stdout, "nothing to change")

	// repo list: bob is Actif with a Flemme threshold of 10 days.
	r = h.run("repo", "list", "--json").mustSucceed(t)
	assertContains(t, "repo list --json", r.stdout, `"flemme_after_days": 10`, `"dead_after_days": 20`)
	var list repoListReport
	decodeStrict(t, r.stdout, &list)
	for _, item := range list.Repositories {
		if item.FullName != "bob/projet" {
			if item.FlemmeAfterDays != nil || item.DeadAfterDays != nil {
				t.Errorf("%s: own thresholds %v/%v, want null", item.FullName, item.FlemmeAfterDays, item.DeadAfterDays)
			}
			continue
		}
		if item.Activity.State != "active" || item.Activity.FlemmeAfterDays != 10 || item.Activity.DeadAfterDays != 20 {
			t.Errorf("bob activity = %+v", item.Activity)
		}
	}

	// --dead alone, smaller than the workspace Flemme threshold: Dead stays above Flemme (5).
	r = h.run("repo", "add", "bob/projet", "--flemme", "0", "--dead", "8", "--json").mustSucceed(t)
	var rep repoAddReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Outcome != "updated" || rep.Repository.FlemmeAfterDays != nil || rep.Repository.DeadAfterDays == nil ||
		*rep.Repository.DeadAfterDays != 8 {
		t.Errorf("repo add --flemme 0 --dead 8 --json = %+v", rep)
	}
	r = h.run("repo", "list").mustSucceed(t)
	// (The short date and state come from internal/activity: outside the scope of this group.)
	assertContains(t, "repo list Dead", r.stdout, "bob/projet    9 d ago      Bob Durand  ⚠ 9 d")

	// 0 and 0: back to the workspace thresholds.
	h.run("repo", "add", "bob/projet", "--flemme", "0", "--dead", "0").mustSucceed(t)
	if got := thresholdPair(t, h, "bob", "projet"); got != "-/-" {
		t.Errorf("thresholds after 0 / 0 = %s", got)
	}
}

func TestRepoAddThresholdsErrors(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--flemme", "10", "--dead", "5").mustFail(t,
		"--dead threshold (5 d): must be strictly greater than the flemme threshold (10 d)")
	h.run("repo", "add", "alice/projet", "--flemme", "-1").mustFail(t, "--flemme threshold: -1 days is invalid")
	h.run("repo", "add", "alice/projet", "--dead", "abc").mustFail(t, "--dead")
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository called %d times despite invalid thresholds", n)
	}
}

// flemme and dead columns of the import file: empty cell = not provided.
func TestRepoImportThresholds(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "promo.csv"), "repo,label,flemme,dead\nalice/projet,Alice,3,10\nbob/projet,,,7\n")
	h.run("repo", "import", "promo.csv").mustSucceed(t)
	if got := thresholdPair(t, h, "alice", "projet"); got != "3/10" {
		t.Errorf("alice: %s", got)
	}
	if got := thresholdPair(t, h, "bob", "projet"); got != "-/7" {
		t.Errorf("bob: %s", got)
	}

	// Reimport: empty cell = threshold kept, 0 = workspace threshold.
	writeFile(t, filepath.Join(h.cwd, "promo.csv"), "repo,flemme,dead\nalice/projet,,0\nbob/projet,,\n")
	r := h.run("repo", "import", "promo.csv", "--json").mustSucceed(t)
	assertContains(t, "reimport", r.stdout, `"updated": 1`, `"already_tracked": 1`)
	if got := thresholdPair(t, h, "alice", "projet"); got != "3/-" {
		t.Errorf("alice after reimport: %s", got)
	}

	// Inconsistent thresholds: line in error, nothing imported.
	writeFile(t, filepath.Join(h.cwd, "faux.csv"), "repo,flemme,dead\ndave/projet,9,4\n")
	h.run("repo", "import", "faux.csv").mustFail(t, "line 2", "strictly greater")
}

// "config import": own thresholds in the preview (text and JSON), then applied; "config export" writes
// them.
func TestConfigImportThresholds(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--dead", "20").mustSucceed(t)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), `{"format":"watchoo-config","version":1,
"repositories":[{"repo":"alice/projet","flemme_after_days":3,"dead_after_days":0}]}`)

	r := h.run("config", "import", "partage.json", "--dry-run").mustSucceed(t)
	assertContains(t, "preview", r.stdout, "alice/projet (flemme: workspace → 3 d; dead: 20 d → workspace)")
	r = h.run("config", "import", "partage.json", "--dry-run", "--json").mustSucceed(t)
	var rep configImportReport
	decodeStrict(t, r.stdout, &rep)
	c := rep.Repositories[0]
	if c.Action != "update" || c.Before == nil || c.Before.FlemmeAfterDays != nil || c.Before.DeadAfterDays == nil ||
		*c.Before.DeadAfterDays != 20 || c.After.FlemmeAfterDays == nil || *c.After.FlemmeAfterDays != 3 ||
		c.After.DeadAfterDays != nil {
		t.Errorf("preview --json = %+v (before %+v, after %+v)", c, c.Before, c.After)
	}
	if got := thresholdPair(t, h, "alice", "projet"); got != "-/20" {
		t.Errorf("--dry-run wrote: %s", got)
	}

	h.run("config", "import", "partage.json", "--yes").mustSucceed(t)
	if got := thresholdPair(t, h, "alice", "projet"); got != "3/-" {
		t.Errorf("after import: %s", got)
	}
	r = h.run("config", "export").mustSucceed(t)
	assertContains(t, "export", r.stdout, `"flemme_after_days": 3`)
}
