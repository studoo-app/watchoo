package store

// Phase 8.1: squash commits recognised when reading the signal views (GitHub committer AND subject ending
// in « (#n) »), and the repository's own conventions start date carried by the views.

import (
	"fmt"
	"strings"
	"testing"
)

func TestCommitSignalViewsSquash(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	repoID := mustCreateRepo(t, s, 1, "promo", "alice")
	alice := person("Alice", "alice@example.test", "alice", 501)
	webFlow := person("GitHub", "web-flow@example.test", "web-flow", 900)
	noreply := person("GitHub", "NoReply@GitHub.com", "", 0) // clone only: the address, no account

	commit := func(n int, committer Identity, message string) CommitWrite {
		c := cw(n, alice, "main")
		c.Committer = committer
		c.Message = message
		return c
	}
	commits := []CommitWrite{
		commit(1, webFlow, "feat: secret subject one (#12)\n\nbody"),          // API: web-flow
		commit(2, noreply, "\n  \nfix: secret subject two (#3)\n\n* feat: a"), // clone: noreply@github.com
		commit(3, webFlow, "feat: secret subject three\n\nSee (#12)"),         // suffix in the body only
		commit(4, alice, "feat: secret subject four (#12)"),                   // human committer
		commit(5, webFlow, "feat: secret subject five"),                       // GitHub, no suffix
	}
	applySync(t, s, repoID, []BranchState{{Name: "main", HeadSHA: sha(5)}}, commits...)
	var writes []CommitSignalWrite
	for i := 1; i <= 5; i++ {
		writes = append(writes, CommitSignalWrite{SHA: sha(i), FilesKept: 1})
	}
	if _, err := s.WriteCommitSignals(ctx, repoID, writes, t0); err != nil {
		t.Fatal(err)
	}

	views, err := s.CommitSignalViews(ctx, repoID)
	if err != nil || len(views) != 5 {
		t.Fatalf("CommitSignalViews = %+v, %v", views, err)
	}
	want := map[string]bool{sha(1): true, sha(2): true, sha(3): false, sha(4): false, sha(5): false}
	for _, v := range views {
		if v.Squash != want[v.SHA] {
			t.Errorf("commit %s: Squash = %v, want %v", v.SHA, v.Squash, want[v.SHA])
		}
		if v.ConventionsSince != "" {
			t.Errorf("commit %s: ConventionsSince = %q, want the workspace date", v.SHA, v.ConventionsSince)
		}
		// The message read to recognise a squash is never kept in the view.
		if dump := fmt.Sprintf("%+v", v); strings.Contains(dump, "secret subject") {
			t.Errorf("message kept in the view: %s", dump)
		}
	}

	// The repository's own start date reaches the views, as a calendar date.
	if _, err := s.SetRepositoryConventionsSince(ctx, repoID, "2026-09-01"); err != nil {
		t.Fatal(err)
	}
	views, err = s.CommitSignalViews(ctx, 0)
	if err != nil || len(views) != 5 {
		t.Fatalf("CommitSignalViews = %+v, %v", views, err)
	}
	for _, v := range views {
		if v.ConventionsSince != "2026-09-01" {
			t.Errorf("commit %s: ConventionsSince = %q, want 2026-09-01", v.SHA, v.ConventionsSince)
		}
	}
}
