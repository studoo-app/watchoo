package cli

// Dummy data of the phase 3 tests: an initialized workspace, a fake provider (providertest) with a
// fixed clock and a few fictional student repositories.

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// testNow is the tests' fixed time (clock of the CLI, of the engine and of the fake provider).
var testNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// GitHub IDs of the dummy repositories.
const (
	aliceRepoID = 101 // alice/projet: active, branches main and dev
	bobRepoID   = 102 // bob/projet: Flemme (last commit 9 days ago)
	daveRepoID  = 104 // dave/projet: Dead (last commit 42 days ago)
)

// Dummy people.
var (
	alicePerson = provider.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh", ID: 2001,
		AvatarURL: "https://avatars.example.test/u/2001"}
	bobPerson  = provider.Person{Name: "Bob Durand", Email: "Bob@Example.test"} // without a GitHub account
	davePerson = provider.Person{Name: "Dave Petit", Email: "dave@example.test", Login: "dave-gh", ID: 2004}
)

// newWorkspace returns a harness whose current directory is an initialized workspace (UTC time zone),
// with the testNow clock and an empty fake provider on the same clock.
func newWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	t.Setenv("WATCHOO_TIMEZONE", "UTC")
	h.run("init").mustSucceed(t)
	h.Now = func() time.Time { return testNow }
	f := providertest.New()
	f.SetClock(h.Now)
	h.Provider = f
	return h
}

// fakeCommit describes a dummy commit.
type fakeCommit struct {
	seed   string // seed of the SHA (providertest.SHA)
	author provider.Person
	ago    time.Duration // age relative to testNow
	msg    string
}

// addCommits adds to the repository repoID a linear series of commits on top of parent (empty = root),
// puts branch on the last one and returns its SHA.
func addCommits(f *providertest.Fake, repoID int64, branch, parent string, commits ...fakeCommit) string {
	for _, c := range commits {
		sha := providertest.SHA(c.seed)
		at := testNow.Add(-c.ago)
		pc := provider.Commit{
			SHA: sha, Message: c.msg, URL: "https://github.example.test/commit/" + sha,
			Author: c.author, Committer: c.author, AuthoredAt: at, CommittedAt: at,
		}
		if parent != "" {
			pc.Parents = []string{parent}
		}
		f.AddCommits(repoID, pc)
		parent = sha
	}
	f.SetBranch(repoID, branch, parent)
	return parent
}

// seedProvider fills the fake provider: alice/projet (main: 2 commits by Alice, dev: 1 commit by Bob
// 1 hour ago; PR #1 open and #2 merged, release v1.0, tags v1.0 and v0.1), bob/projet (1 commit by Bob
// 9 days ago), dave/projet (1 commit by Dave 42 days ago). carol/projet does not exist (inaccessible
// repository).
func seedProvider(f *providertest.Fake) {
	f.AddRepository(provider.Repository{ID: aliceRepoID, Owner: "alice", Name: "projet", Private: true,
		URL: "https://github.example.test/alice/projet"})
	head := addCommits(f, aliceRepoID, "main", "",
		fakeCommit{"a1", alicePerson, 50 * time.Hour, "Premier jet du projet"},
		fakeCommit{"a2", alicePerson, 2 * time.Hour, "Ajoute les tests\n\nDétail du commit"})
	addCommits(f, aliceRepoID, "dev", head, fakeCommit{"a3", bobPerson, time.Hour, "Essai sur dev"})
	// Pull requests, release and tags of alice/projet (V2).
	f.SetPullRequests(aliceRepoID,
		provider.PullRequest{Number: 1, Title: "Ajoute la page de connexion", State: provider.PullRequestOpen,
			Author: provider.Account{Login: "alice-gh", ID: 2001}, URL: "https://github.example.test/alice/projet/pull/1",
			BaseBranch: "main", HeadBranch: "login", CreatedAt: testNow.Add(-5 * time.Hour), UpdatedAt: testNow.Add(-3 * time.Hour)},
		provider.PullRequest{Number: 2, Title: "Corrige le build", State: provider.PullRequestClosed, Merged: true,
			Author: provider.Account{Login: "alice-gh", ID: 2001}, URL: "https://github.example.test/alice/projet/pull/2",
			BaseBranch: "main", HeadBranch: "fix", CreatedAt: testNow.Add(-30 * time.Hour),
			UpdatedAt: testNow.Add(-26 * time.Hour), ClosedAt: testNow.Add(-26 * time.Hour), MergedAt: testNow.Add(-26 * time.Hour)},
	)
	f.SetReleases(aliceRepoID, provider.Release{ID: 501, TagName: "v1.0", Name: "Première version",
		Author: provider.Account{Login: "alice-gh"}, URL: "https://github.example.test/alice/projet/releases/tag/v1.0",
		CreatedAt: testNow.Add(-2 * time.Hour), PublishedAt: testNow.Add(-2 * time.Hour)})
	f.SetTags(aliceRepoID, provider.Tag{Name: "v1.0", SHA: head}, provider.Tag{Name: "v0.1", SHA: providertest.SHA("inconnu")})

	f.AddRepository(provider.Repository{ID: bobRepoID, Owner: "bob", Name: "projet",
		URL: "https://github.example.test/bob/projet"})
	addCommits(f, bobRepoID, "main", "", fakeCommit{"b1", bobPerson, 9 * 24 * time.Hour, "Initialisation"})

	f.AddRepository(provider.Repository{ID: daveRepoID, Owner: "dave", Name: "projet",
		URL: "https://github.example.test/dave/projet"})
	addCommits(f, daveRepoID, "main", "", fakeCommit{"d1", davePerson, 42 * 24 * time.Hour, "Rendu final"})
}

// newSyncedWorkspace returns a workspace where the three dummy repositories are tracked (alice/projet
// with the label "Alice Martin") and synced.
func newSyncedWorkspace(t *testing.T) *harness {
	t.Helper()
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--label", "Alice Martin").mustSucceed(t)
	h.run("repo", "add", "bob/projet").mustSucceed(t)
	h.run("repo", "add", "https://github.com/dave/projet").mustSucceed(t)
	h.run("sync").mustSucceed(t)
	return h
}

// decodeStrict decodes stdout into v, rejecting unknown fields and any text after the object.
func decodeStrict(t *testing.T, stdout string, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if dec.More() {
		t.Fatalf("text after the JSON object:\n%s", stdout)
	}
}

// assertJSONKeys checks that the JSON object designated by path (object keys, or "0" for the first
// element of an array) in stdout has exactly the keys want: the format of the --json outputs is a
// contract for scripts.
func assertJSONKeys(t *testing.T, stdout string, path string, want ...string) {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(stdout), &v); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, stdout)
	}
	if path != "" {
		for _, p := range strings.Split(path, ".") {
			switch node := v.(type) {
			case map[string]any:
				v = node[p]
			case []any:
				if p != "0" || len(node) == 0 {
					t.Fatalf("path %q: empty array or unsupported index %q", path, p)
				}
				v = node[0]
			default:
				t.Fatalf("path %q: %q in a %T value", path, p, v)
			}
		}
	}
	obj, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("path %q: want an object, got %T (%v)", path, v, v)
	}
	var got []string
	for k := range obj {
		got = append(got, k)
	}
	slices.Sort(got)
	want = slices.Clone(want)
	slices.Sort(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("JSON keys of %q = %v, want %v", path, got, want)
	}
}

// repositoryKeys are the keys of repositoryJSON.
var repositoryKeys = []string{"id", "full_name", "owner", "name", "label", "display_name", "url",
	"default_branch", "branch_patterns", "flemme_after_days", "dead_after_days", "enabled", "archived", "stars", "open_issues", "last_synced_at",
	"last_commit_at"}

// assertContains checks that s contains each of the excerpts.
func assertContains(t *testing.T, what, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(s, p) {
			t.Errorf("%s without %q:\n%s", what, p, s)
		}
	}
}

// assertNotContains checks that s contains none of the excerpts.
func assertNotContains(t *testing.T, what, s string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(s, p) {
			t.Errorf("%s with %q:\n%s", what, p, s)
		}
	}
}

// tableRow returns the line of out that starts with prefix, its columns separated by multiple spaces;
// the test fails if there is none.
func tableRow(t *testing.T, out, prefix string) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			var cols []string
			for _, c := range strings.Split(line, "  ") {
				if c = strings.TrimSpace(c); c != "" {
					cols = append(cols, c)
				}
			}
			return cols
		}
	}
	t.Fatalf("no line starting with %q:\n%s", prefix, out)
	return nil
}

// mustCode checks the exit code.
func (r result) mustCode(t *testing.T, code int) result {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit code %d, want %d\nstdout:\n%s\nstderr:\n%s", r.code, code, r.stdout, r.stderr)
	}
	return r
}
