package sync_test

// Conventions, phase 8.3: the file criteria (CHANGELOG, README, documentation, tooling) are measured by the
// sync on the updated clone, through its Git objects, and cached in repository_conventions.

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/provider/git/gittest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// withConventions enables the conventions with the milestones ms.
func withConventions(ms ...string) envOption {
	return func(e *env) {
		e.cfg.Conventions.Enabled = true
		e.cfg.Conventions.Tags.Milestones = ms
	}
}

const filesChangelog = `# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [1.0.0] - 2026-09-01

### Added

- Première version.
`

// filesRepo creates a repository with a CHANGELOG, a README and a documentation tree, a milestone tag
// v1.0.0 and a tag with a hostile name, and returns the environment after a first clone-only sync.
func filesRepo(t *testing.T, opts ...envOption) (*env, *gittest.Repo, model.Repository) {
	t.Helper()
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	r.Commit("main", "CHANGELOG.md", filesChangelog, "docs: ajoute le changelog", secretAuthor, epoch)
	r.Commit("main", "documentation/INDEX.md", "# Index\n", "docs: ajoute l'index", secretAuthor, epoch.Add(time.Hour))
	c3 := r.Commit("main", "README.md", "# Projet secret\n", "docs: ajoute le README", secretAuthor, epoch.Add(2*time.Hour))
	r.Tag("v1.0.0", c3, "version 1")
	r.Tag("x;$(touch-pwned)", c3, "")

	e := newEnv(t, append([]envOption{withGit(srv.URL)}, opts...)...)
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}},
		wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	return e, r, res[0].Repo
}

// cachedFiles returns the cached rows of the repository, by "kind/tag".
func cachedFiles(t *testing.T, e *env, repoID int64) map[string]model.RepositoryConventions {
	t.Helper()
	rows, err := e.st.RepositoryConventions(t.Context(), repoID)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]model.RepositoryConventions{}
	for _, r := range rows {
		out[r.RevKind+"/"+r.TagName] = r
	}
	return out
}

// assertNoFileLeak checks that neither a path, a file content, a tag name, a commit subject, an author nor
// an image name (phase 8.7 leads) reached the logs.
func assertNoFileLeak(t *testing.T, e *env) {
	t.Helper()
	for _, leak := range []string{"CHANGELOG", "README", "INDEX", "documentation", "secret", "v1.0.0", "pwned", "Première",
		"workflows", "commitlint", ".env", "écran", "Alice", ".png"} {
		if strings.Contains(e.logs.String(), leak) {
			t.Errorf("logs contain %q:\n%s", leak, e.logs.String())
		}
	}
}

func TestConventionFilesCache(t *testing.T) {
	e, r, repo := filesRepo(t, withConventions("v1.0.0", "v2.0.0"))
	ctx := t.Context()
	rows := cachedFiles(t, e, repo.ID)
	head, tag := rows["head/"], rows["tag/v1.0.0"]
	if len(rows) != 2 || head.RevSHA != r.Head("main") || tag.RevSHA != r.Head("main") ||
		head.RulesVersion != conventions.FileRulesVersion || head.ConfigHash == "" || tag.ConfigHash != head.ConfigHash {
		t.Fatalf("rows after the first sync: %+v", rows)
	}
	res, err := conventions.DecodeFileResults(head.Results)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := res.Find(conventions.CriterionCL1); !ok || c.Status != conventions.CriterionConform {
		t.Errorf("CL1 at the head: %+v", c)
	}
	if _, ok := res.Find(conventions.CriterionCL9); ok {
		t.Error("CL9 measured at the head")
	}
	tagRes, err := conventions.DecodeFileResults(tag.Results)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := tagRes.Find(conventions.CriterionCL9); !ok || c.Status != conventions.CriterionConform {
		t.Errorf("CL9 on the milestone tag: %+v", c)
	}
	if strings.Contains(head.Results, "README") || strings.Contains(head.Results, "secret") {
		t.Errorf("results contain a path or a content: %s", head.Results)
	}

	// Nothing changed: nothing recomputed.
	e.clock.Set(e.clock.Now().Add(time.Hour))
	e.syncAll()
	if again := cachedFiles(t, e, repo.ID); !again["head/"].ComputedAt.Equal(head.ComputedAt) ||
		!again["tag/v1.0.0"].ComputedAt.Equal(tag.ComputedAt) {
		t.Errorf("recomputed without change: %+v", again)
	}

	// A new commit: the head only is recomputed.
	r.Commit("main", "documentation/fr/01-vision.md", "# Vision\n", "docs: ajoute la vision", secretAuthor, epoch.Add(3*time.Hour))
	e.clock.Set(e.clock.Now().Add(time.Hour))
	e.syncAll()
	moved := cachedFiles(t, e, repo.ID)
	if moved["head/"].RevSHA != r.Head("main") || !moved["head/"].ComputedAt.After(head.ComputedAt) ||
		!moved["tag/v1.0.0"].ComputedAt.Equal(tag.ComputedAt) {
		t.Errorf("after a commit: %+v", moved)
	}

	// A changed setting: every row is recomputed with the new hash.
	e.cfg.Conventions.Readme.MinWords = 5
	e.eng = e.newEngine(e.st)
	e.clock.Set(e.clock.Now().Add(time.Hour))
	e.syncAll()
	changed := cachedFiles(t, e, repo.ID)
	if changed["head/"].ConfigHash == head.ConfigHash || changed["tag/v1.0.0"].ConfigHash == tag.ConfigHash ||
		!changed["tag/v1.0.0"].ComputedAt.After(tag.ComputedAt) {
		t.Errorf("after a setting change: %+v", changed)
	}

	// Milestone removed: its row is deleted, the head stays.
	e.cfg.Conventions.Tags.Milestones = []string{"v2.0.0"}
	e.eng = e.newEngine(e.st)
	e.syncAll()
	if gone := cachedFiles(t, e, repo.ID); len(gone) != 1 || gone["head/"].RevSHA == "" {
		t.Errorf("after removing the milestone: %+v", gone)
	}
	if n, err := e.st.DeleteRepositoryConventionTags(ctx, repo.ID, nil); err != nil || n != 0 {
		t.Errorf("tag rows left: %d, %v", n, err)
	}

	// Milestone configured again: its row comes back. Tag deleted from the repository while the milestone
	// stays configured: its row is deleted.
	e.cfg.Conventions.Tags.Milestones = []string{"v1.0.0"}
	e.eng = e.newEngine(e.st)
	e.syncAll()
	if back := cachedFiles(t, e, repo.ID); len(back) != 2 || back["tag/v1.0.0"].RevSHA == "" {
		t.Fatalf("after configuring the milestone again: %+v", back)
	}
	r.DeleteTag("v1.0.0")
	e.syncAll()
	if gone := cachedFiles(t, e, repo.ID); len(gone) != 1 || gone["head/"].RevSHA == "" {
		t.Errorf("after deleting the tag: %+v", gone)
	}

	// Default branch unknown on the forge (HEAD on a missing branch): the head row is deleted, its old
	// result is no longer shown as computed.
	r.SetDefaultBranch("absente")
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync without default branch: %+v", rep.Repos)
	}
	if rows := cachedFiles(t, e, repo.ID); len(rows) != 0 {
		t.Errorf("after losing the default branch: %+v", rows)
	}
	assertNoFileLeak(t, e)
}

// Phase 8.7: the tree leads are measured with the file criteria at the head only, and cached with them;
// CL12 compares the CHANGELOG bullets with the commit subjects read from the database, in memory only.
func TestConventionFilesLeads(t *testing.T) {
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	var log strings.Builder
	log.WriteString("# Changelog\n\n## [Unreleased]\n\n### Added\n\n")
	for i := range 10 {
		subject := fmt.Sprintf("feat: ajoute l'écran secret %d", i)
		r.Commit("main", fmt.Sprintf("src/ecran%d.txt", i), "x", subject, secretAuthor, epoch.Add(time.Duration(i)*time.Minute))
		log.WriteString("- " + subject + "\n")
	}
	r.Commit("main", "CHANGELOG.md", log.String(), "docs: changelog", secretAuthor, epoch.Add(time.Hour))
	r.Commit("main", ".github/workflows/ci.yml", "steps:\n  - uses: wagoid/commitlint-github-action@v6\n",
		"ci: commitlint", secretAuthor, epoch.Add(2*time.Hour))
	r.Commit("main", "documentation/a.md", "Auteur : Alice\n\n![x](Media/absent-secret.png)\n", "docs: a",
		secretAuthor, epoch.Add(3*time.Hour))
	c := r.Commit("main", "README.md", "# P\n\n## Installation\n\nCopier .env puis lancer.\n", "docs: readme",
		secretAuthor, epoch.Add(4*time.Hour))
	r.Tag("v1.0.0", c, "")

	e := newEnv(t, withGit(srv.URL), withConventions("v1.0.0"))
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}},
		wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	rows := cachedFiles(t, e, res[0].Repo.ID)
	head, err := conventions.DecodeFileResults(rows["head/"].Results)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		conventions.CriterionCL11: conventions.LeadReview, // [Unreleased] without a comparison link
		conventions.CriterionCL12: conventions.LeadReview, // 100 % of the bullets are commit subjects
		conventions.CriterionRD5:  conventions.LeadReview, // no command block, .env announced without a model
		conventions.CriterionDOC6: conventions.LeadReview,
		conventions.CriterionDOC7: conventions.LeadReview, // broken image
		conventions.CriterionTL4:  conventions.LeadNone,   // commitlint in a workflow
	}
	if len(head.Leads) != len(want) {
		t.Fatalf("leads at the head: %+v", head.Leads)
	}
	for id, status := range want {
		if l, ok := conventions.FindLead(head.Leads, id); !ok || l.Status != status {
			t.Errorf("%s: %+v, want %s", id, l, status)
		}
	}
	if l, _ := conventions.FindLead(head.Leads, conventions.CriterionCL12); l.Counts[conventions.CountSubjectPercent] != 100 ||
		l.Counts[conventions.CountBullets] != 10 {
		t.Errorf("CL12 counts: %v", l.Counts)
	}
	tag, err := conventions.DecodeFileResults(rows["tag/v1.0.0"].Results)
	if err != nil || tag.Leads != nil {
		t.Errorf("leads on the milestone tag: %+v, %v", tag.Leads, err)
	}
	for _, leak := range []string{"écran", "secret", "Alice", "png", "commitlint-github-action", "Media"} {
		if strings.Contains(rows["head/"].Results, leak) {
			t.Errorf("cached results contain %q: %s", leak, rows["head/"].Results)
		}
	}
	assertNoFileLeak(t, e)
}

// CL4 takes the day in the time zone of the workspace: at 23:30 UTC on November 9, it is already
// November 10 in Paris, so a version dated November 10 is not in the future.
func TestConventionFilesTimeZone(t *testing.T) {
	if _, err := time.LoadLocation("Europe/Paris"); err != nil {
		t.Skip("time zone Europe/Paris unavailable:", err)
	}
	srv := gittest.NewServer(t)
	r := srv.Create(testOwner, "projet", "main")
	r.Commit("main", "CHANGELOG.md", strings.Replace(filesChangelog, "2026-09-01", "2026-11-10", 1),
		"docs: ajoute le changelog", secretAuthor, epoch)
	paris := func(e *env) {
		e.cfg.Timezone = "Europe/Paris"
		e.clock.Set(time.Date(2026, 11, 9, 23, 30, 0, 0, time.UTC))
	}
	e := newEnv(t, withGit(srv.URL), withConventions(), paris)
	e.auth.SetTokenErr(auth.ErrNotLoggedIn)
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{{Ref: repoimport.Ref{Owner: testOwner, Name: "projet"}}},
		wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Fatalf("sync: %+v", rep.Repos)
	}
	head := cachedFiles(t, e, res[0].Repo.ID)["head/"]
	got, err := conventions.DecodeFileResults(head.Results)
	if err != nil {
		t.Fatal(err)
	}
	if c, ok := got.Find(conventions.CriterionCL4); !ok || c.Status != conventions.CriterionConform {
		t.Errorf("CL4 at 23:30 UTC, November 10 in Paris: %+v", c)
	}
}

// Disabled conventions: nothing is read nor written.
func TestConventionFilesDisabled(t *testing.T) {
	e, _, repo := filesRepo(t)
	if rows := cachedFiles(t, e, repo.ID); len(rows) != 0 {
		t.Errorf("rows written while disabled: %+v", rows)
	}
	// ls-tree is also run by the languages: only one, never cat-file.
	if strings.Contains(e.logs.String(), "cat-file") || strings.Count(e.logs.String(), "commande=ls-tree") != 1 {
		t.Errorf("files read while disabled:\n%s", e.logs.String())
	}
}

// failFiles is a Git provider whose file reading fails.
type failFiles struct{ *gitprov.Provider }

func (failFiles) ReadBlobs(context.Context, string, string, string, []string, func(string, []byte, bool)) error {
	return &provider.APIError{Op: "files of " + testOwner + "/projet", Err: provider.ErrUnexpected}
}

// A failed reading does not fail the sync: nothing is written, the failure is logged in Debug without
// a path or tag name; a clone that cannot read files writes nothing either.
func TestConventionFilesReadFailure(t *testing.T) {
	failing := func(e *env) { e.gitWrap = func(p *gitprov.Provider) provider.Provider { return failFiles{p} } }
	e, _, repo := filesRepo(t, withConventions("v1.0.0"), failing)
	if rows := cachedFiles(t, e, repo.ID); len(rows) != 0 {
		t.Errorf("rows written despite the failure: %+v", rows)
	}
	if !strings.Contains(e.logs.String(), "critères de fichiers des conventions non calculés") ||
		!strings.Contains(e.logs.String(), "revisions=2") {
		t.Errorf("failure not logged:\n%s", e.logs.String())
	}
	assertNoFileLeak(t, e)

	e.gitWrap = func(p *gitprov.Provider) provider.Provider { return opaque{p} }
	e.eng = e.newEngine(e.st)
	if rep := e.syncAll(); rep.Repos[0].Status != wsync.StatusSuccess {
		t.Errorf("sync without file reading: %+v", rep.Repos)
	}
	if rows := cachedFiles(t, e, repo.ID); len(rows) != 0 {
		t.Errorf("rows written without file reading: %+v", rows)
	}
}
