package cli

// « watchoo contributors --weeks N » : commits par semaine de chaque contributeur, pour l'espace ou un
// dépôt, en texte et en JSON (clé additive weekly).

import (
	"slices"
	"testing"
)

func TestContributorsWeeks(t *testing.T) {
	h := newSyncedWorkspace(t)

	// Dépôt d'Alice : a1 (Alice, dimanche 25/10), a2 (Alice, 27/10) sur main, a3 (Bob, 27/10) sur dev
	// seulement ; le commit de Bob dans son propre dépôt est exclu.
	r := h.run("contributors", "--repo", "alice/projet", "--weeks", "3").mustSucceed(t)
	assertContains(t, "contributors --repo --weeks", r.stdout, "CONTRIBUTOR", "12/10", "19/10", "26/10", "TOTAL")
	if row := tableRow(t, r.stdout, "Alice Martin"); !slices.Equal(row, []string{"Alice Martin", "0", "1", "1", "2"}) {
		t.Errorf("ligne d'Alice Martin = %q", row)
	}
	if row := tableRow(t, r.stdout, "Bob Durand"); !slices.Equal(row, []string{"Bob Durand", "0", "0", "1", "1"}) {
		t.Errorf("ligne de Bob Durand = %q", row)
	}
	assertContains(t, "contributors --repo --weeks", r.stdout, "Commits per week (Monday to Sunday, columns dated by Monday, "+
		"display time zone); the last week is in progress.\n")

	// Tout l'espace : b1 (Bob, 18/10) compte dans la semaine du 12/10 ; Dave (42 jours) sort de la
	// période et du classement.
	r = h.run("contributors", "--weeks", "3", "--json").mustSucceed(t)
	var rep contributorsReport
	decodeStrict(t, r.stdout, &rep)
	assertJSONKeys(t, r.stdout, "contributors.0", "id", "name", "login", "display_name", "avatar_url", "commits",
		"additions", "deletions", "repositories", "last_commit_at", "weekly")
	assertJSONKeys(t, r.stdout, "contributors.0.weekly.0", "week", "count")
	if len(rep.Contributors) != 2 {
		t.Fatalf("contributors --weeks 3 = %+v ; attendu Bob et Alice", rep)
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
			t.Errorf("%s : semaines %v", c.DisplayName, weeks)
		}
		got[c.DisplayName] = counts
		sum := 0
		for _, n := range counts {
			sum += n
		}
		if sum != c.Commits {
			t.Errorf("%s : %d commits, %d par semaine", c.DisplayName, c.Commits, sum)
		}
	}
	if !slices.Equal(got["Bob Durand"], []int{1, 0, 1}) || !slices.Equal(got["Alice Martin"], []int{0, 1, 1}) {
		t.Errorf("par semaine : %v", got)
	}

	// Sans --weeks : weekly vaut [] (jamais null).
	r = h.run("contributors", "--json").mustSucceed(t)
	assertContains(t, "contributors --json", r.stdout, `"weekly": []`)
	assertNotContains(t, "contributors --json", r.stdout, `"weekly": null`)

	// Aucun commit sur la période (dépôt de Dave, 42 jours) : message clair.
	r = h.run("contributors", "--repo", "dave/projet", "--weeks", "2").mustSucceed(t)
	assertContains(t, "contributors sans commit récent", r.stdout, "No contributor over the last 2 weeks")

	h.run("contributors", "--weeks", "53").mustFail(t, "--weeks 53 is invalid", "between 1 and 52")
	h.run("contributors", "--weeks", "-1").mustFail(t, "--weeks -1 is invalid")
	h.run("contributors", "--weeks", "4", "--since", "7d").mustFail(t, "--since and --weeks cannot be used together")
}
