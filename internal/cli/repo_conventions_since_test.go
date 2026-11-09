package cli

import (
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// "repo add --conventions-since": date saved, displayed in the report and in "repo list", exposed in
// JSON (null = date of the workspace), "" goes back to the date of the workspace.
func TestRepoAddConventionsSince(t *testing.T) {
	h := newSyncedWorkspace(t)

	r := h.run("repo", "add", "bob/projet", "--conventions-since", "2026-09-01").mustSucceed(t)
	assertContains(t, "repo add --conventions-since", r.stdout,
		"Repository bob/projet already tracked, updated (all branches, conventions since 2026-09-01).")
	if got := storedRepo(t, h, "bob", "projet").ConventionsSince; got != "2026-09-01" {
		t.Errorf("saved date = %q", got)
	}
	r = h.run("repo", "add", "bob/projet", "--conventions-since", "2026-09-01").mustSucceed(t)
	assertContains(t, "same date", r.stdout, "nothing to change")

	r = h.run("repo", "list").mustSucceed(t)
	assertContains(t, "repo list", r.stdout, "Own conventions start date: bob/projet since 2026-09-01")
	r = h.run("repo", "list", "--json").mustSucceed(t)
	var list repoListReport
	decodeStrict(t, r.stdout, &list)
	for _, item := range list.Repositories {
		want := ""
		if item.FullName == "bob/projet" {
			want = "2026-09-01"
		}
		got := ""
		if item.ConventionsSince != nil {
			got = *item.ConventionsSince
		}
		if got != want {
			t.Errorf("%s: conventions_since %q, want %q", item.FullName, got, want)
		}
	}

	// French text.
	restore := i18n.SetProcessForTest(i18n.French)
	r = h.run("repo", "list").mustSucceed(t)
	restore()
	assertContains(t, "repo list (fr)", r.stdout, "Date de début des conventions propre : bob/projet depuis le 2026-09-01")

	// Back to the workspace date.
	r = h.run("repo", "add", "bob/projet", "--conventions-since", "", "--json").mustSucceed(t)
	var rep repoAddReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Outcome != "updated" || rep.Repository.ConventionsSince != nil {
		t.Errorf("repo add --conventions-since \"\" --json = %+v", rep)
	}
	assertContains(t, "repo add JSON", r.stdout, `"conventions_since": null`)
	if got := storedRepo(t, h, "bob", "projet").ConventionsSince; got != "" {
		t.Errorf("date after the reset = %q", got)
	}
}

func TestRepoAddConventionsSinceErrors(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--conventions-since", "01/09/2026").mustFail(t,
		`--conventions-since: "01/09/2026": date expected in the form YYYY-MM-DD`)
	h.run("repo", "add", "alice/projet", "--conventions-since", "2026-02-30").mustFail(t, "--conventions-since")
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository called %d times despite an invalid date", n)
	}
}

// "config import": the change of the per-repository conventions start date is described.
func TestRepoChangeTextConventionsSince(t *testing.T) {
	c := configshare.RepoChange{Repo: "bob/projet",
		Before: &configshare.RepoState{Branches: []string{}, Enabled: true},
		After:  configshare.RepoState{Branches: []string{}, Enabled: true, ConventionsSince: "2026-09-01"}}
	if got := repoChangeText(c); got != "bob/projet (conventions since: workspace → 2026-09-01)" {
		t.Errorf("repoChangeText = %q", got)
	}
	state := newConfigRepoState(c.After)
	if state.ConventionsSince == nil || *state.ConventionsSince != "2026-09-01" {
		t.Errorf("JSON state = %+v", state)
	}
	if newConfigRepoState(*c.Before).ConventionsSince != nil {
		t.Error("workspace date: conventions_since not null")
	}
}
