package cli

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// TestContributorGroupInCLI checks a contributor group (1.9.0) in "contributors" (one line per person,
// accounts key) and in the --author filter (an account selects the whole group).
func TestContributorGroupInCLI(t *testing.T) {
	h := newSyncedWorkspace(t)
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(h.root, workspace.DirName, workspace.DBFileName))
	if err != nil {
		t.Fatal(err)
	}
	ranking, err := st.ContributorRanking(ctx, store.ContributorFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var alice, bob int64
	var aliceCommits, bobCommits int
	for _, c := range ranking {
		switch c.DisplayName() {
		case "Alice Martin":
			alice, aliceCommits = c.ID, c.Commits
		case "Bob Durand":
			bob, bobCommits = c.ID, c.Commits
		}
	}
	if alice == 0 || bob == 0 {
		t.Fatalf("ranking: %+v", ranking)
	}
	if err := st.AttachContributor(ctx, bob, alice); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	r := h.run("contributors").mustSucceed(t)
	assertContains(t, "contributors", r.stdout, "alice-gh (+1)")
	for _, line := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(line, "Bob Durand") {
			t.Errorf("attached account still listed: %q", line)
		}
	}

	r = h.run("contributors", "--json").mustSucceed(t)
	var rep contributorsReport
	decodeStrict(t, r.stdout, &rep)
	found := false
	for _, c := range rep.Contributors {
		if c.ID == bob {
			t.Errorf("attached account listed: %+v", c)
		}
		if c.ID != alice {
			if len(c.Accounts) != 0 {
				t.Errorf("%d: accounts %+v", c.ID, c.Accounts)
			}
			continue
		}
		found = true
		if c.Commits != aliceCommits+bobCommits || len(c.Accounts) != 1 || c.Accounts[0].ID != bob ||
			c.Accounts[0].Name != "Bob Durand" || c.Accounts[0].Commits != bobCommits {
			t.Errorf("alice's group: %+v", c)
		}
	}
	if !found {
		t.Fatal("alice missing")
	}

	// An account of the group selects the commits of the whole group; display_name is the person's.
	r = h.run("log", "--author", "Bob Durand", "--json", "-n", "0").mustSucceed(t)
	assertContains(t, "log --author account", r.stdout, `"name": "Alice Martin"`, `"name": "Bob Durand"`,
		`"display_name": "Alice Martin"`)
}
