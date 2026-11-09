package sync_test

// Conventions, phase 8.4: the sync records the observations of the tags (tag_observations: SV2 deadline,
// SV3) and the rewrites of the default branch (branch_rewrites: BR4), only when the conventions are
// enabled; a tag list that failed, is truncated or incomplete closes nothing.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// observations returns the tag observations of the repository.
func observations(t *testing.T, e *env, repoID int64) []model.TagObservation {
	t.Helper()
	obs, err := e.st.TagObservations(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	return obs
}

// describeObs writes the observations as "name sha first last end reason" lines, for comparisons.
func describeObs(obs []model.TagObservation) string {
	var b strings.Builder
	for _, o := range obs {
		end := "-"
		if o.EndedAt != nil {
			end = o.EndedAt.Format(time.RFC3339)
		}
		fmt.Fprintf(&b, "%s %s %s %s %s %s\n", o.Name, provider.ShortSHA(o.SHA), o.FirstSeenAt.Format(time.RFC3339),
			o.LastSeenAt.Format(time.RFC3339), end, o.EndReason)
	}
	return b.String()
}

// rewrites returns the recorded rewrites of the default branch of the repository.
func rewrites(t *testing.T, e *env, repoID int64) []model.BranchRewrite {
	t.Helper()
	rows, err := e.st.BranchRewrites(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// tick moves the clock of the environment forward by one hour and returns the new time (UTC).
func (e *env) tick() time.Time {
	now := e.clock.Now().Add(time.Hour)
	e.clock.Set(now)
	return now.UTC()
}

func TestTagObservationsAPI(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "projet")
	c1 := e.commit(1, "c1", epoch)
	c2 := e.commit(1, "c2", epoch.Add(time.Hour), c1)
	e.fake.SetBranch(1, "main", c2)
	e.fake.SetTags(1, provider.Tag{Name: "v1.0.0", SHA: c1}, provider.Tag{Name: "v1.1.0", SHA: c2})
	t1 := e.clock.Now().UTC()
	e.syncOK(repo.ID)
	want := fmt.Sprintf("v1.0.0 %s %s %s - \nv1.1.0 %s %s %s - \n", provider.ShortSHA(c1), t1.Format(time.RFC3339),
		t1.Format(time.RFC3339), provider.ShortSHA(c2), t1.Format(time.RFC3339), t1.Format(time.RFC3339))
	if got := describeObs(observations(t, e, repo.ID)); got != want {
		t.Fatalf("after the first sync:\n%s\nwant:\n%s", got, want)
	}

	// Seen again: last_seen_at moves forward.
	t2 := e.tick()
	e.syncOK(repo.ID)
	for _, o := range observations(t, e, repo.ID) {
		if !o.FirstSeenAt.Equal(t1) || !o.LastSeenAt.Equal(t2) || !o.Open() {
			t.Errorf("seen again: %+v", o)
		}
	}

	// v1.0.0 moved to another commit: the old period is closed (rewritten), a new one opened.
	e.fake.SetTags(1, provider.Tag{Name: "v1.0.0", SHA: c2}, provider.Tag{Name: "v1.1.0", SHA: c2})
	t3 := e.tick()
	e.syncOK(repo.ID)
	obs := observations(t, e, repo.ID)
	var closed, reopened bool
	for _, o := range obs {
		switch {
		case o.Name == "v1.0.0" && o.SHA == c1:
			closed = o.EndedAt != nil && o.EndedAt.Equal(t3) && o.EndReason == model.TagEndRewritten && o.LastSeenAt.Equal(t2)
		case o.Name == "v1.0.0" && o.SHA == c2:
			reopened = o.Open() && o.FirstSeenAt.Equal(t3)
		}
	}
	if len(obs) != 3 || !closed || !reopened {
		t.Fatalf("after the rewrite:\n%s", describeObs(obs))
	}

	// A failed tag list closes nothing and changes nothing.
	e.fake.SetTags(1)
	before := describeObs(obs)
	e.fake.FailNext(providertest.MethodTags, errors.New("panne"))
	e.tick()
	e.syncOK(repo.ID)
	if got := describeObs(observations(t, e, repo.ID)); got != before {
		t.Errorf("after a failed tag list:\n%s\nwant:\n%s", got, before)
	}

	// The tags disappear: the open periods are closed (deleted).
	t5 := e.tick()
	e.syncOK(repo.ID)
	for _, o := range observations(t, e, repo.ID) {
		if o.Open() || (o.SHA == c2 && (!o.EndedAt.Equal(t5) || o.EndReason != model.TagEndDeleted)) {
			t.Errorf("after the deletion: %+v", o)
		}
	}
	if len(rewrites(t, e, repo.ID)) != 0 {
		t.Error("rewrite recorded without a rewrite of the default branch")
	}
	assertNoTagLeak(t, e)
}

// assertNoTagLeak checks that no tag name reached the logs.
func assertNoTagLeak(t *testing.T, e *env) {
	t.Helper()
	for _, leak := range []string{"v1.0.0", "v1.1.0", "v2.0.0"} {
		if strings.Contains(e.logs.String(), leak) {
			t.Errorf("logs contain %q:\n%s", leak, e.logs.String())
		}
	}
}

func TestTagObservationsDisabled(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	c1 := e.commit(1, "c1", epoch)
	e.fake.SetBranch(1, "main", c1)
	e.fake.SetTags(1, provider.Tag{Name: "v1.0.0", SHA: c1})
	e.syncOK(repo.ID)
	if obs := observations(t, e, repo.ID); len(obs) != 0 {
		t.Errorf("observations with the conventions disabled:\n%s", describeObs(obs))
	}
}

// incompleteTags adds a tag without a name to the tag list (skipped by the sync).
type incompleteTags struct {
	provider.Provider
}

func (p incompleteTags) Tags(ctx context.Context, owner, name string) ([]provider.Tag, error) {
	tags, err := p.Provider.Tags(ctx, owner, name)
	return append(tags, provider.Tag{SHA: providertest.SHA("orphelin")}), err
}

// An incomplete (a tag skipped) or truncated (provider.MaxTags) tag list updates no observation: a tag
// missing from it would be closed wrongly.
func TestTagObservationsIncompleteList(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts []envOption
		tags func(sha string) []provider.Tag
	}{
		{"skipped tag", []envOption{withWrap(func(p provider.Provider) provider.Provider { return incompleteTags{p} })},
			func(string) []provider.Tag { return nil }},
		{"truncated", nil, func(sha string) []provider.Tag {
			tags := make([]provider.Tag, provider.MaxTags)
			for i := range tags {
				tags[i] = provider.Tag{Name: fmt.Sprintf("t%04d", i), SHA: sha}
			}
			return tags
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t, append([]envOption{withConventions()}, tc.opts...)...)
			repo := e.addRepo(1, "projet")
			c1 := e.commit(1, "c1", epoch)
			e.fake.SetBranch(1, "main", c1)
			e.fake.SetTags(1, tc.tags(c1)...)
			e.syncOK(repo.ID)
			if obs := observations(t, e, repo.ID); len(obs) != 0 {
				t.Errorf("observations from an incomplete list: %d", len(obs))
			}
		})
	}
}

// BR4 in API mode without a clone: the old head unknown to the forge (ErrUnknownBase) is a rewrite; a
// comparison that succeeds (old commit still on the forge) shows nothing (documented limit).
func TestBranchRewriteAPI(t *testing.T) {
	e := newEnv(t, withConventions())
	repo := e.addRepo(1, "projet")
	c1 := e.commit(1, "c1", epoch)
	c2 := e.commit(1, "c2", epoch.Add(time.Hour), c1)
	e.fake.SetBranch(1, "main", c2)
	e.syncOK(repo.ID)

	// Normal progress: no rewrite.
	c3 := e.commit(1, "c3", epoch.Add(2*time.Hour), c2)
	e.fake.SetBranch(1, "main", c3)
	e.tick()
	e.syncOK(repo.ID)
	if rw := rewrites(t, e, repo.ID); len(rw) != 0 {
		t.Fatalf("rewrite on a fast-forward: %+v", rw)
	}

	// Force-push, old commit still on the forge: the comparison succeeds, nothing is seen.
	d3 := e.commit(1, "d3", epoch.Add(3*time.Hour), c2)
	e.fake.SetBranch(1, "main", d3)
	e.tick()
	e.syncOK(repo.ID)
	if rw := rewrites(t, e, repo.ID); len(rw) != 0 {
		t.Fatalf("rewrite seen through a successful comparison: %+v", rw)
	}

	// Force-push, old commit removed from the forge: ErrUnknownBase, one rewrite.
	d4 := e.commit(1, "d4", epoch.Add(4*time.Hour), c2)
	e.fake.SetBranch(1, "main", d4)
	e.fake.RemoveCommit(1, d3)
	at := e.tick()
	e.syncOK(repo.ID)
	rw := rewrites(t, e, repo.ID)
	if len(rw) != 1 || rw[0].Branch != "main" || rw[0].OldSHA != d3 || rw[0].NewSHA != d4 || !rw[0].ObservedAt.Equal(at) {
		t.Fatalf("rewrites = %+v", rw)
	}
	assertNoSHALeak(t, e, d3, d4)
}

// assertNoSHALeak checks that no full SHA reached the logs.
func assertNoSHALeak(t *testing.T, e *env, shas ...string) {
	t.Helper()
	for _, sha := range shas {
		if strings.Contains(e.logs.String(), sha) {
			t.Errorf("logs contain the full SHA %s", provider.ShortSHA(sha))
		}
	}
}

// BR4 is not recorded with the conventions disabled, nor on a change of default branch.
func TestBranchRewriteNotRecorded(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		e := newEnv(t)
		repo := e.addRepo(1, "projet")
		c1 := e.commit(1, "c1", epoch)
		c2 := e.commit(1, "c2", epoch.Add(time.Hour), c1)
		e.fake.SetBranch(1, "main", c2)
		e.syncOK(repo.ID)
		d2 := e.commit(1, "d2", epoch.Add(2*time.Hour), c1)
		e.fake.SetBranch(1, "main", d2)
		e.fake.RemoveCommit(1, c2)
		e.tick()
		e.syncOK(repo.ID)
		if rw := rewrites(t, e, repo.ID); len(rw) != 0 {
			t.Errorf("rewrite recorded with the conventions disabled: %+v", rw)
		}
	})
	t.Run("default branch changed", func(t *testing.T) {
		e := newEnv(t, withConventions())
		repo := e.addRepo(1, "projet")
		c1 := e.commit(1, "c1", epoch)
		c2 := e.commit(1, "c2", epoch.Add(time.Hour), c1)
		e.fake.SetBranch(1, "main", c2)
		e.syncOK(repo.ID)
		d2 := e.commit(1, "d2", epoch.Add(2*time.Hour), c1)
		e.fake.SetBranch(1, "trunk", d2)
		e.fake.DeleteBranch(1, "main")
		e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: testOwner, Name: "projet", DefaultBranch: "trunk",
			URL: "https://github.com/" + testOwner + "/projet", Private: true})
		e.tick()
		e.syncOK(repo.ID)
		if rw := rewrites(t, e, repo.ID); len(rw) != 0 {
			t.Errorf("rewrite recorded on a change of default branch: %+v", rw)
		}
	})
}

// With local Git clones (no gh): a tag set, moved by force, then deleted; the default branch rewritten by
// a force-push (the old head stays in the clone: ancestry read there); a fast-forward is not a rewrite.
func TestTagHistoryAndRewritesFromClone(t *testing.T) {
	e, r, repo := filesRepo(t, withConventions("v1.0.0", "v2.0.0"))
	first := e.clock.Now().UTC()
	byName := func() map[string][]model.TagObservation {
		out := map[string][]model.TagObservation{}
		for _, o := range observations(t, e, repo.ID) {
			out[o.Name] = append(out[o.Name], o)
		}
		return out
	}
	obs := byName()
	if len(obs["v1.0.0"]) != 1 || !obs["v1.0.0"][0].Open() || !obs["v1.0.0"][0].FirstSeenAt.Equal(first) ||
		obs["v1.0.0"][0].SHA != r.Head("main") {
		t.Fatalf("after the first sync: %+v", obs)
	}
	old := r.Head("main")

	// Fast-forward, and the milestone tag moved by force to the new commit.
	c4 := r.Commit("main", "a.txt", "a\n", "feat: ajoute a", secretAuthor, epoch.Add(3*time.Hour))
	r.DeleteTag("v1.0.0")
	r.Tag("v1.0.0", c4, "version 1 bis")
	moved := e.tick()
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	obs = byName()
	if len(obs["v1.0.0"]) != 2 {
		t.Fatalf("after the move: %s", describeObs(observations(t, e, repo.ID)))
	}
	for _, o := range obs["v1.0.0"] {
		switch o.SHA {
		case old:
			if o.EndedAt == nil || !o.EndedAt.Equal(moved) || o.EndReason != model.TagEndRewritten {
				t.Errorf("old period: %+v", o)
			}
		case c4:
			if !o.Open() || !o.FirstSeenAt.Equal(moved) {
				t.Errorf("new period: %+v", o)
			}
		default:
			t.Errorf("unexpected period: %+v", o)
		}
	}
	if rw := rewrites(t, e, repo.ID); len(rw) != 0 {
		t.Fatalf("rewrite on a fast-forward: %+v", rw)
	}

	// Default branch rewritten by a force-push: one rewrite, old and new heads.
	r.Reset("main", old)
	c5 := r.Commit("main", "b.txt", "b\n", "fix: corrige b", secretAuthor, epoch.Add(4*time.Hour))
	r.DeleteTag("v1.0.0")
	rewritten := e.tick()
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	rw := rewrites(t, e, repo.ID)
	if len(rw) != 1 || rw[0].Branch != "main" || rw[0].OldSHA != c4 || rw[0].NewSHA != c5 || !rw[0].ObservedAt.Equal(rewritten) {
		t.Fatalf("rewrites = %+v", rw)
	}
	for _, o := range byName()["v1.0.0"] {
		if o.SHA == c4 && (o.EndedAt == nil || o.EndReason != model.TagEndDeleted) {
			t.Errorf("deleted tag: %+v", o)
		}
	}

	// Nothing moved: nothing more.
	e.tick()
	e.syncAll()
	if rw := rewrites(t, e, repo.ID); len(rw) != 1 {
		t.Errorf("rewrites after a sync without change: %+v", rw)
	}
	assertNoFileLeak(t, e)
	assertNoSHALeak(t, e, old, c4, c5)
}

// In clone-only mode, an old head missing from the clone is "unknown", never a rewrite.
func TestBranchRewriteUnknownInClone(t *testing.T) {
	e, r, repo := filesRepo(t, withConventions())
	// The old head, rewritten away (and its tags deleted), is no longer in the recreated clone.
	r.DeleteTag("v1.0.0")
	r.DeleteTag("x;$(touch-pwned)")
	r.Reset("main", r.Head("main")+"~1")
	r.Commit("main", "b.txt", "b\n", "fix: corrige b", secretAuthor, epoch.Add(4*time.Hour))
	if err := os.RemoveAll(e.reposDir); err != nil {
		t.Fatal(err)
	}
	e.tick()
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	if rw := rewrites(t, e, repo.ID); len(rw) != 0 {
		t.Errorf("rewrite recorded with the old head missing from the clone: %+v", rw)
	}
}
