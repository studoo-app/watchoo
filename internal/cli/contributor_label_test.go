package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// TestContributorLabelInCLI checks that a name given in the web interface (1.8.0) is displayed by
// "contributors" and "log", carried by the label and display_name keys, and accepted by --author.
func TestContributorLabelInCLI(t *testing.T) {
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
	for _, c := range ranking {
		if c.ProviderLogin == "alice-gh" {
			if _, err := st.SetContributorLabel(ctx, c.ID, "Alice M. (groupe 2)"); err != nil {
				t.Fatal(err)
			}
		}
	}
	_ = st.Close()

	r := h.run("contributors").mustSucceed(t)
	assertContains(t, "contributors", r.stdout, "Alice M. (groupe 2)", "alice-gh")

	r = h.run("contributors", "--json").mustSucceed(t)
	var rep contributorsReport
	decodeStrict(t, r.stdout, &rep)
	found := false
	for _, c := range rep.Contributors {
		switch c.Login {
		case "alice-gh":
			found = true
			if c.Label != "Alice M. (groupe 2)" || c.DisplayName != "Alice M. (groupe 2)" || c.Name != "Alice Martin" {
				t.Errorf("alice: %+v", c)
			}
		default:
			if c.Label != "" || c.DisplayName == "" {
				t.Errorf("%s: %+v", c.Login, c)
			}
		}
	}
	if !found {
		t.Fatal("alice-gh missing")
	}

	r = h.run("log", "--author", "alice m. (groupe 2)", "--json").mustSucceed(t)
	assertContains(t, "log --author label", r.stdout, `"display_name": "Alice M. (groupe 2)"`, `"name": "Alice Martin"`)
}
