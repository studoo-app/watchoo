package cli

// « watchoo diagnostic » (phase 7.2): leads of level "declared" for the trainer, text and --json,
// filters, disabled diagnosis.

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// diagnosticWorkspace returns a synced workspace with the diagnosis enabled: a PR of alice/projet
// opened from a branch named by an agent, with a signature in its description, and a commit of
// alice/projet whose signals declare an AI co-author (written in the database: the CLI tests have no
// local clone).
func diagnosticWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newWorkspace(t)
	t.Setenv("WATCHOO_DIAGNOSTIC_ENABLED", "true")
	f := h.Provider
	seedProvider(f)
	f.SetPullRequests(aliceRepoID, provider.PullRequest{Number: 3, Title: "Refonte du menu", State: provider.PullRequestOpen,
		Author: provider.Account{Login: "alice-gh", ID: 2001}, URL: "https://github.example.test/alice/projet/pull/3",
		BaseBranch: "main", HeadBranch: "codex/menu", Body: "Résumé\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)",
		CreatedAt: testNow.Add(-4 * time.Hour), UpdatedAt: testNow.Add(-4 * time.Hour)})
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)

	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(h.root, workspace.DirName, workspace.DBFileName))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	repos, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		if r.FullName() != "alice/projet" {
			continue
		}
		if _, err := st.WriteCommitSignals(ctx, r.ID, []store.CommitSignalWrite{
			{SHA: providertest.SHA("a2"), FilesKept: 2, AdditionsKept: 40, Tools: []string{"D1:claude"}},
			{SHA: providertest.SHA("a1"), FilesKept: 1, AdditionsKept: 3}, // no declarative signal
		}, testNow); err != nil {
			t.Fatal(err)
		}
		if err := st.SetAgentConfigSince(ctx, r.ID, testNow.Add(-48*time.Hour), testNow); err != nil {
			t.Fatal(err)
		}
	}
	return h
}

func TestDiagnosticText(t *testing.T) {
	h := diagnosticWorkspace(t)
	out := h.run("diagnostic").mustSucceed(t).stdout
	assertContains(t, "diagnostic", out,
		"Reflection leads for the trainer, not verdicts",
		"PR #3", "D2 claude, D2 unnamed, D5 codex/menu",
		"commit "+provider.ShortSHA(providertest.SHA("a2")), "D1 claude", "declared",
		"Context (D4):", "alice/projet: agent configured since 10/25/2026")
	assertNotContains(t, "diagnostic", out, provider.ShortSHA(providertest.SHA("a1")), "Ajoute les tests")
	// PR (4 h ago) before the commit (2 h ago)? No: newest first, the commit comes first.
	if strings.Index(out, "commit ") > strings.Index(out, "PR #3") {
		t.Errorf("leads not sorted from newest to oldest:\n%s", out)
	}

	// Filters: another repository, another author, a short period.
	assertContains(t, "--repo bob", h.run("diagnostic", "--repo", "bob/projet").mustSucceed(t).stdout, "No reflection lead.")
	assertContains(t, "--author bob", h.run("diagnostic", "--author", "bob-gh").mustSucceed(t).stdout, "No reflection lead.")
	since := h.run("diagnostic", "--since", "3h").mustSucceed(t).stdout
	assertContains(t, "--since 3h", since, "D1 claude")
	assertNotContains(t, "--since 3h", since, "PR #3")

	// French.
	defer i18n.SetProcessForTest(i18n.French)()
	fr := h.run("diagnostic").mustSucceed(t).stdout
	assertContains(t, "diagnostic (fr)", fr, "Pistes de réflexion pour le formateur", "déclaré",
		"alice/projet : agent configuré depuis le 25/10/2026")
}

func TestDiagnosticJSON(t *testing.T) {
	h := diagnosticWorkspace(t)
	out := h.run("diagnostic", "--json").mustSucceed(t).stdout
	assertSingleJSONObject(t, "diagnostic --json", out)
	assertJSONKeys(t, out, "", jsonTopLevelKeys["diagnostic"]...)
	assertJSONKeys(t, out, "leads.0", "kind", "level", "repository", "sha", "short_sha", "number", "title", "url",
		"author", "date", "signals")
	assertJSONKeys(t, out, "leads.0.signals.0", "id", "value")
	assertJSONKeys(t, out, "contexts.0", "repository", "agent_config_since")
	assertJSONKeys(t, out, "filters", "repository", "author", "since")
	var rep struct {
		Enabled bool `json:"enabled"`
		Leads   []struct {
			Kind    string `json:"kind"`
			Level   string `json:"level"`
			Number  int    `json:"number"`
			Signals []struct{ ID, Value string }
		} `json:"leads"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatal(err)
	}
	if !rep.Enabled || len(rep.Leads) != 2 || rep.Leads[0].Kind != "commit" || rep.Leads[1].Kind != "pull_request" ||
		rep.Leads[1].Number != 3 || rep.Leads[1].Level != "declared" || len(rep.Leads[1].Signals) != 3 ||
		rep.Leads[1].Signals[2].ID != "D5" || rep.Leads[1].Signals[2].Value != "codex/menu" {
		t.Errorf("diagnostic --json: %+v\n%s", rep, out)
	}
	// The PR description is never written.
	assertNotContains(t, "diagnostic --json", out, "Résumé", "Generated with")
}

func TestDiagnosticDisabled(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	out := h.run("diagnostic").mustSucceed(t).stdout
	assertContains(t, "diagnostic (disabled)", out, "The diagnosis is disabled in this workspace", "Inform the learners")
	js := h.run("diagnostic", "--json").mustSucceed(t).stdout
	assertContains(t, "diagnostic --json (disabled)", js, `"enabled": false`, `"leads": []`, `"contexts": []`)
	// Invalid option: refused before the workspace is opened.
	if r := h.run("diagnostic", "--since", "demain"); r.code == 0 {
		t.Errorf("--since demain accepted: %+v", r)
	}
}
