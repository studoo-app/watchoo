package cli

// « watchoo conventions » (phase 8.4) on local clones: rewrites of the default branch (BR4), tag history
// (SV3), due dates of the milestones (SV2), pre-releases (SV4), version bumps (SV5), precedence (SV7) and
// freshness of the CHANGELOG (CL10), in text (English and French), --json and --milestone. Never a commit
// message, never an attribution to a person.

import (
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
)

func TestConventionsHistoryLocalClone(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create("alice", "projet", "main")
	who := gittest.Author{Name: "Alice Martin", Email: "alice@example.test"}
	c1 := r.Commit("main", "a.txt", "1\n", "feat: initialise le projet", who, testNow.Add(-100*time.Hour))
	c2 := r.Commit("main", "a.txt", "2\n", "fix: corrige le menu", who, testNow.Add(-90*time.Hour))
	r.Commit("main", "CHANGELOG.md", "# Changelog\n", "docs: ajoute le journal", who, testNow.Add(-80*time.Hour))
	c4 := r.Commit("main", "a.txt", "4\n", "feat: ajoute la recherche", who, testNow.Add(-60*time.Hour))
	r.Tag("v0.1.0", c1, "")

	h := newWorkspace(t)
	h.Client = authtest.NewLoggedOut()
	h.GitURL = srv.URL
	t.Setenv("WATCHOO_CONVENTIONS_ENABLED", "true")
	t.Setenv("WATCHOO_TIMEZONE", "UTC")
	t.Setenv("WATCHOO_CONVENTIONS_TAGS_MILESTONES", "v0.1.1@2026-10-25,v0.1.0")
	h.run("repo", "add", "alice/projet").mustSucceed(t)
	// First sync on October 25 at noon, before the due date of v0.1.1 (end: October 26 at midnight).
	first := testNow.Add(-48 * time.Hour)
	h.Now = func() time.Time { return first }
	h.run("sync").mustSucceed(t)

	// v0.1.1 set after the due date; a free-form pre-release on a new commit.
	c5 := r.Commit("main", "a.txt", "5\n", "fix: corrige la recherche", who, testNow.Add(-50*time.Hour))
	r.Tag("v0.1.1", c4, "")
	r.Tag("v0.2.0-rc1", c5, "")
	h.Now = func() time.Time { return testNow }
	h.run("sync").mustSucceed(t)

	// History rewritten: main back on c4 (c5 stays reachable from its tag), v0.1.0 moved to c2.
	r.Reset("main", c4)
	r.DeleteTag("v0.1.0")
	r.Tag("v0.1.0", c2, "")
	later := testNow.Add(time.Hour)
	h.Now = func() time.Time { return later }
	h.run("sync").mustSucceed(t)

	out := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (history)", out,
		"1 rewrite(s) of the default branch observed, the last on 10/27/2026",
		"main on 10/27/2026: "+c5[:7]+" → "+c4[:7],
		"tag history: 2 without rewrite observed since 10/25/2026, 1 rewritten, 0 deleted",
		"v0.1.0: rewritten (other commit) (first seen on 10/25/2026)",
		"due dates: v0.1.1 late (first seen on 10/27/2026)",
		"pre-releases: 0/1 in a conforming form", "v0.2.0-rc1 (free form (expected -<form>.N))",
		"precedence: 1 pair(s) of final versions checked: consistent with the history",
		"version bumps: 0 consistent, 1 under-versioned, 0 undetermined, 0 unknown",
		"v0.1.0→v0.1.1 under-versioned (PATCH, MINOR expected)",
		"1 feat and 0 fix since the last commit that touched the CHANGELOG (")
	assertNotContains(t, "conventions (history)", out, "initialise le projet", "corrige le menu", "ajoute le journal",
		"ajoute la recherche", "corrige la recherche", "Alice Martin pushed", "never rewritten")

	js := h.run("conventions", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, js, "repositories.0.rewrites.events.0", "branch", "old_sha", "new_sha", "observed_at")
	assertJSONKeys(t, js, "repositories.0.tag_history.items.0", "name", "status", "sha", "first_seen_at", "rewrites", "deletions")
	assertJSONKeys(t, js, "repositories.0.tags.milestones.0.deadline", "status", "due_end", "first_seen_at", "commit_at")
	assertJSONKeys(t, js, "repositories.0.prereleases.notes.0", "code", "tag", "other")
	assertJSONKeys(t, js, "repositories.0.bumps.items.0", "from", "to", "status", "reason", "actual", "expected",
		"evaluated", "conform", "breaking", "features", "missing")
	assertNotContains(t, "conventions --json (history)", js, "initialise le projet", "ajoute la recherche")
	var rep conventionsReport
	decodeStrict(t, js, &rep)
	a := rep.Repositories[0]
	if a.Rewrites.Count != 1 || a.Rewrites.Events[0].OldSHA != c5 || a.Rewrites.Events[0].NewSHA != c4 ||
		!a.TagHistory.Known || a.TagHistory.Counts["rewritten"] != 1 ||
		a.Tags.Milestones[1].Deadline != nil || a.Tags.Milestones[0].Deadline == nil || a.Tags.Milestones[0].Deadline.Status != "late" ||
		a.Prereleases.Checked != 1 || a.Prereleases.Notes[0].Code != "free_form" || !a.Precedence.Conform ||
		a.Bumps.Counts["under"] != 1 || *a.Bumps.Items[0].Expected != "minor" ||
		!a.ChangelogFreshness.Known || a.ChangelogFreshness.Feat != 1 || a.ChangelogFreshness.Fix != 0 || a.ChangelogFreshness.Grayed {
		t.Errorf("conventions --json (history): %+v", a)
	}

	// Milestone review: SV2 late, SV3 without rewrite observed.
	t.Setenv("WATCHOO_CONVENTIONS_SINCE", "2020-01-01")
	ms := h.run("conventions", "--milestone", "v0.1.1").mustSucceed(t).stdout
	assertContains(t, "--milestone (history)", ms, "SV2  not conform — v0.1.1 present, late (first seen on 10/27/2026)",
		"SV3  conform — no rewrite observed since 10/27/2026")
	ms = h.run("conventions", "--milestone", "v0.1.0").mustSucceed(t).stdout
	assertContains(t, "--milestone v0.1.0", ms, "SV3  not conform — rewritten (other commit)")
	review := h.run("conventions", "--milestone", "v0.1.1", "--json").mustSucceed(t).stdout
	assertJSONKeys(t, review, "milestone.repositories.0.deadline", "status", "due_end", "first_seen_at", "commit_at")
	assertJSONKeys(t, review, "milestone.repositories.0.tag_history", "status", "first_seen_at", "rewrites", "deletions")

	// With --contributor: the repository history is not shown.
	contrib := h.run("conventions", "--contributor", "Alice Martin").mustSucceed(t).stdout
	assertNotContains(t, "--contributor (history)", contrib, "rewrite(s) of the default branch", "tag history")

	defer i18n.SetProcessForTest(i18n.French)()
	fr := h.run("conventions").mustSucceed(t).stdout
	assertContains(t, "conventions (history, fr)", fr,
		"1 réécriture(s) de la branche par défaut observée(s), la dernière le 27/10/2026",
		"historique des tags : 2 sans réécriture observée depuis le 25/10/2026",
		"échéances : v0.1.1 en retard (vu pour la première fois le 27/10/2026)",
		"v0.1.0→v0.1.1 sous-versionné (PATCH, MINOR attendu)")
}
