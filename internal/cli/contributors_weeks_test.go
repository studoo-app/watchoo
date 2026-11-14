package cli

// "watchoo contributors --weeks N": commits per week of each contributor, for the workspace or a
// repository, in text and in JSON (additive weekly key).

import (
	"slices"
	"testing"
)

func TestContributorsWeeks(t *testing.T) {
	h := newSyncedWorkspace(t)

	// Alice's repository: a1 (Alice, Sunday 25/10), a2 (Alice, 27/10) on main, a3 (Bob, 27/10) on dev
	// only; Bob's commit in his own repository is excluded.
	r := h.run("contributors", "--repo", "alice/projet", "--weeks", "3").mustSucceed(t)
	assertContains(t, "contributors --repo --weeks", r.stdout, "CONTRIBUTOR", "10/12", "10/19", "10/26", "TOTAL")
	if row := tableRow(t, r.stdout, "Alice Martin"); !slices.Equal(row, []string{"Alice Martin", "0", "1", "1", "2"}) {
		t.Errorf("Alice Martin row = %q", row)
	}
	if row := tableRow(t, r.stdout, "Bob Durand"); !slices.Equal(row, []string{"Bob Durand", "0", "0", "1", "1"}) {
		t.Errorf("Bob Durand row = %q", row)
	}
	assertContains(t, "contributors --repo --weeks", r.stdout, "Commits per week (Monday to Sunday, columns dated by Monday, "+
		"display time zone); the last week is in progress.\n")

	// The whole workspace: b1 (Bob, 18/10) counts in the week of 12/10; Dave (42 days) falls out of the
	// period and of the ranking.
	r = h.run("contributors", "--weeks", "3", "--json").mustSucceed(t)
	var rep contributorsReport
	decodeStrict(t, r.stdout, &rep)
	assertJSONKeys(t, r.stdout, "contributors.0", "id", "name", "login", "label", "display_name", "accounts", "avatar_url", "commits",
		"additions", "deletions", "repositories", "share", "last_commit_at", "ai_signed_commits", "ai_scanned_commits", "ai_tools",
		"languages_scanned_commits", "languages", "weekly")
	assertJSONKeys(t, r.stdout, "contributors.0.weekly.0", "week", "count")
	if len(rep.Contributors) != 2 {
		t.Fatalf("contributors --weeks 3 = %+v; want Bob and Alice", rep)
	}
	got := map[string][]int{}
	for _, c := range rep.Contributors {
		var weeks []string
		var counts []int
		for _, w := range c.Weekly {
			weeks = append(weeks, w.Week)
			counts = append(counts, w.Count)
		}
		if !slices.Equal(weeks, []string{"2026-10-12", "2026-10-19", "2026-10-26"}) {
			t.Errorf("%s: weeks %v", c.DisplayName, weeks)
		}
		got[c.DisplayName] = counts
		sum := 0
		for _, n := range counts {
			sum += n
		}
		if sum != c.Commits {
			t.Errorf("%s: %d commits, %d per week", c.DisplayName, c.Commits, sum)
		}
	}
	if !slices.Equal(got["Bob Durand"], []int{1, 0, 1}) || !slices.Equal(got["Alice Martin"], []int{0, 1, 1}) {
		t.Errorf("per week: %v", got)
	}

	// Without --weeks: weekly is [] (never null).
	r = h.run("contributors", "--json").mustSucceed(t)
	assertContains(t, "contributors --json", r.stdout, `"weekly": []`)
	assertNotContains(t, "contributors --json", r.stdout, `"weekly": null`)

	// No commit over the period (Dave's repository, 42 days): clear message.
	r = h.run("contributors", "--repo", "dave/projet", "--weeks", "2").mustSucceed(t)
	assertContains(t, "contributors without a recent commit", r.stdout, "No contributor over the last 2 weeks")

	h.run("contributors", "--weeks", "53").mustFail(t, "--weeks 53 is invalid", "between 1 and 52")
	h.run("contributors", "--weeks", "-1").mustFail(t, "--weeks -1 is invalid")
	h.run("contributors", "--weeks", "4", "--since", "7d").mustFail(t, "--since and --weeks cannot be used together")
}
