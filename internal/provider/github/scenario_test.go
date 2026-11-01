package github

// End-to-end scenarios of phase 2: the REAL Provider (transport, ETag cache, quota, error conversion,
// API calls) against the programmable fake server githubtest. They cover the "Simulated GitHub API"
// section of documentation/en/09-testing.md. No real network call: githubtest listens on 127.0.0.1
// and fails the test on any request other than GET or HEAD.
//
// Every environment (newScEnv) captures the Provider's logs at Debug level and checks at the end of
// the test that they contain neither token, nor Authorization header, nor commit message, author
// name, e-mail or login, and that they are all at Debug level. The scenarios' errors go through scNoLeak.
//
// Helpers prefixed with sc; they reuse trClock, trEpoch, trLogs and trLogger (transport_test.go).

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

// Dummy data of the scenarios.
const (
	scOwner      = "ecole-demo"            // organization owning the repositories (it may appear in the logs)
	scOtherToken = "gho_autre_compte_fake" // token of another gh account, refused by the fake server
	scBadToken   = "gho_token_revoque_fake"
	scUserAgent  = "watchoo/v0.0.0-test"

	// Identity and message of the commits added by scEnv.commit: they must appear neither in a log nor
	// in an error.
	scSecretName    = "Eleve Confidentiel"
	scSecretEmail   = "eleve.confidentiel@example.test"
	scSecretLogin   = "eleve-confidentiel"
	scSecretMessage = "Message de commit confidentiel"
)

// scForbidden is the list of strings forbidden in the logs and in the error messages.
var scForbidden = []string{
	testToken, scOtherToken, scBadToken, "gho_", "Bearer", "authorization_request",
	scSecretName, scSecretEmail, scSecretLogin, scSecretMessage,
	// Dummy identity of githubtest.AppendCommits.
	"Étudiant Test", "etudiant@example.test", "etudiant-test",
}

// scAppendMessage recognizes the "commit N" messages of githubtest.AppendCommits (a short SHA quoted
// after "commit" has 7 characters: it does not match).
var scAppendMessage = regexp.MustCompile(`\bcommit [0-9]{1,4}\b`)

// scEnv is a scenario environment: fake GitHub server, clock shared by the server and the Providers,
// captured logs, ETag cache and quota state shared between the built Providers.
type scEnv struct {
	t     *testing.T
	srv   *githubtest.Server
	clock *trClock
	logs  *trLogs
	cache *Cache
	quota *Quota
	t0    time.Time // start date of the dummy commits (in the clock's past)
}

// newScEnv prepares an environment and schedules the check of the logs at the end of the test.
func newScEnv(t *testing.T) *scEnv {
	t.Helper()
	clock := newTrClock(trEpoch)
	e := &scEnv{
		t:     t,
		srv:   githubtest.NewServer(t, githubtest.Options{Token: testToken, Now: clock.now}),
		clock: clock,
		logs:  &trLogs{},
		cache: NewCache(0, 0),
		quota: NewQuota(),
		t0:    trEpoch.Add(-30 * 24 * time.Hour),
	}
	t.Cleanup(e.checkNoLeak)
	return e
}

// provider builds a Provider authenticated by token, plugged into the fake server, with the cache and
// the quota state of the environment: like the sync engine, which builds one per sync.
func (e *scEnv) provider(token string) *Provider {
	e.t.Helper()
	p, err := New(token, Options{
		BaseURL:   e.srv.URL(),
		UserAgent: scUserAgent,
		Cache:     e.cache,
		Quota:     e.quota,
		Logger:    trLogger(e.logs, slog.LevelDebug),
	})
	if err != nil {
		e.t.Fatalf("New: %v", err)
	}
	p.tr.now = e.clock.now
	return p
}

// repo adds a private repository of scOwner and returns its API path ("/repos/ecole-demo/…").
func (e *scEnv) repo(id int64, name string) string {
	e.t.Helper()
	e.srv.AddRepo(githubtest.Repo{ID: id, Owner: scOwner, Name: name, Private: true})
	return "/repos/" + scOwner + "/" + name
}

// commit adds to the repository repoID a commit by the confidential dummy author, dated at, and
// returns its SHA. label makes the message unique.
func (e *scEnv) commit(repoID int64, label string, at time.Time, parents ...string) string {
	e.t.Helper()
	return e.srv.AddCommit(repoID, githubtest.Commit{
		Message:     scSecretMessage + " (" + label + ")",
		Author:      githubtest.Person{Name: scSecretName, Email: scSecretEmail, Login: scSecretLogin},
		CommittedAt: at,
		Parents:     parents,
		Additions:   10,
		Deletions:   2,
	})
}

// countLogs returns the number of logs with message msg.
func (e *scEnv) countLogs(msg string) int {
	e.t.Helper()
	n := 0
	for _, rec := range e.logs.records(e.t) {
		if rec["msg"] == msg {
			n++
		}
	}
	return n
}

// checkNoLeak checks, at the end of the test, the captured logs and the fake server's log.
func (e *scEnv) checkNoLeak() {
	logs := e.logs.String()
	for _, s := range scForbidden {
		if strings.Contains(logs, s) {
			e.t.Errorf("the logs contain %q", s)
		}
	}
	if m := scAppendMessage.FindString(logs); m != "" {
		e.t.Errorf("the logs contain a commit message (%q)", m)
	}
	for _, rec := range e.logs.records(e.t) {
		if rec["level"] != slog.LevelDebug.String() {
			e.t.Errorf("log at level %v (%v): only the Debug level is allowed", rec["level"], rec["msg"])
		}
	}
	for _, r := range e.srv.Requests() {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			e.t.Errorf("request %s %s received by the fake server", r.Method, r.Path)
		}
	}
}

// scNoLeak checks that the message of err quotes neither token, nor personal data, nor request
// parameter.
func scNoLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	for _, s := range append(append([]string{}, scForbidden...), "per_page", "page=", "sha=") {
		if strings.Contains(msg, s) {
			t.Errorf("the error message %q contains %q", msg, s)
		}
	}
	if m := scAppendMessage.FindString(msg); m != "" {
		t.Errorf("the error message %q quotes a commit message", msg)
	}
}

// scAPIError checks that err is a *provider.APIError with code status (and path path, if it is not
// empty) and returns it.
func scAPIError(t *testing.T, err error, status int, path string) *provider.APIError {
	t.Helper()
	apiErr, ok := errors.AsType[*provider.APIError](err)
	if !ok {
		t.Fatalf("error %v (%T): want *provider.APIError", err, err)
	}
	if apiErr.Status != status || (path != "" && apiErr.Path != path) {
		t.Errorf("APIError {Path %q, Status %d}, want {Path %q, Status %d}: %v", apiErr.Path, apiErr.Status, path, status, err)
	}
	scNoLeak(t, err)
	return apiErr
}

// scRateLimit checks that err is a compliant *provider.RateLimitError.
func scRateLimit(t *testing.T, err error, secondary bool, reset time.Time, retryAfter time.Duration) {
	t.Helper()
	if !errors.Is(err, provider.ErrRateLimited) {
		t.Fatalf("error %v: want ErrRateLimited", err)
	}
	rle, ok := errors.AsType[*provider.RateLimitError](err)
	if !ok {
		t.Fatalf("error %v: want *provider.RateLimitError", err)
	}
	if rle.Secondary != secondary || !rle.Reset.Equal(reset) || rle.RetryAfter != retryAfter {
		t.Errorf("RateLimitError = %+v, want {Secondary %v, Reset %v, RetryAfter %v}", rle, secondary, reset, retryAfter)
	}
	scNoLeak(t, err)
}

// scSHAs returns the SHAs of commits, in order.
func scSHAs(commits []provider.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.SHA)
	}
	return out
}

// scWantSHAs compares the got and wanted SHAs.
func scWantSHAs(t *testing.T, what string, got []provider.Commit, want ...string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s: nil list (want an empty non-nil slice)", what)
	}
	if g := scSHAs(got); !reflect.DeepEqual(g, append([]string{}, want...)) {
		t.Errorf("%s: %d commits %v\nwant %d commits %v", what, len(g), shortSHAs(g), len(want), shortSHAs(want))
	}
}

// shortSHAs shortens SHAs for the failure messages.
func shortSHAs(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[i] = provider.ShortSHA(s)
	}
	return out
}

// scReversed returns a copy of shas in reverse order.
func scReversed(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[len(shas)-1-i] = s
	}
	return out
}

// scCheckPages checks that the requests received for path are exactly the pages 1 to n, with
// per_page=100.
func scCheckPages(t *testing.T, reqs []githubtest.Request, path string, n int) {
	t.Helper()
	var pages []string
	for _, r := range reqs {
		if r.Path != path {
			continue
		}
		if pp := r.Query.Get("per_page"); pp != strconv.Itoa(PerPage) {
			t.Errorf("%s: per_page=%q, want %d", path, pp, PerPage)
		}
		pages = append(pages, r.Query.Get("page"))
	}
	want := make([]string, n)
	for i := range n {
		want[i] = strconv.Itoa(i + 1)
	}
	if !reflect.DeepEqual(pages, want) {
		t.Errorf("%s: requested pages %v, want %v", path, pages, want)
	}
}

// scAll304 checks that the received requests are n conditional requests served as 304s.
func scAll304(t *testing.T, reqs []githubtest.Request, n int) {
	t.Helper()
	if len(reqs) != n {
		t.Errorf("%d requests received, want %d", len(reqs), n)
	}
	for _, r := range reqs {
		if r.Status != http.StatusNotModified || r.IfNoneMatch == "" {
			t.Errorf("%s ?%s: code %d, If-None-Match %q; want 304 with If-None-Match", r.Path, r.RawQuery, r.Status, r.IfNoneMatch)
		}
	}
}

// TestScenarioPagination: branches and commits over several pages (per_page=100), conversion of the
// commits, Limit and Since.
func TestScenarioPagination(t *testing.T) {
	e := newScEnv(t)
	const id = 1001
	base := e.repo(id, "projet-pagination")
	shas := e.srv.AppendCommits(id, "main", 250, e.t0, time.Minute)
	for i := range 149 {
		e.srv.SetBranch(id, fmt.Sprintf("feature/%03d", i), shas[i])
	}
	p := e.provider(testToken)
	ctx := t.Context()
	if p.Name() != "github" {
		t.Errorf("Name() = %q", p.Name())
	}

	// 150 branches: 2 pages, GitHub's alphabetical order.
	branches, err := p.Branches(ctx, scOwner, "projet-pagination")
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	if len(branches) != 150 || branches[0] != (provider.Branch{Name: "feature/000", HeadSHA: shas[0]}) ||
		branches[149] != (provider.Branch{Name: "main", HeadSHA: shas[249]}) {
		t.Errorf("Branches: %d branches, first %+v, last %+v", len(branches), branches[0], branches[len(branches)-1])
	}
	scCheckPages(t, e.srv.Requests(), base+"/branches", 2)
	for _, r := range e.srv.Requests() {
		if !r.ValidToken || r.UserAgent != scUserAgent {
			t.Errorf("request %s: valid token %v, User-Agent %q", r.Path, r.ValidToken, r.UserAgent)
		}
	}

	// 250 commits: 3 pages, from newest to oldest.
	e.srv.ResetRequests()
	commits, err := p.ListCommits(ctx, scOwner, "projet-pagination", provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	scWantSHAs(t, "ListCommits", commits, scReversed(shas)...)
	scCheckPages(t, e.srv.Requests(), base+"/commits", 3)
	for _, r := range e.srv.Requests() {
		if r.Query.Get("sha") != "main" {
			t.Errorf("sha parameter = %q, want main", r.Query.Get("sha"))
		}
	}

	// Conversion of a commit (Git identity and linked GitHub account, UTC dates, parents).
	if len(commits) == 250 {
		c := commits[0]
		at := e.t0.Add(249 * time.Minute)
		if c.Author.Name != "Étudiant Test" || c.Author.Email != "etudiant@example.test" ||
			c.Author.Login != "etudiant-test" || c.Author.ID == 0 || c.Author.AvatarURL == "" || c.Committer != c.Author {
			t.Errorf("Author = %+v, Committer = %+v", c.Author, c.Committer)
		}
		if !c.AuthoredAt.Equal(at) || !c.CommittedAt.Equal(at) || c.AuthoredAt.Location() != time.UTC {
			t.Errorf("dates %v / %v, want %v in UTC", c.AuthoredAt, c.CommittedAt, at)
		}
		if c.Message != "commit 250" || !strings.Contains(c.URL, c.SHA) {
			t.Errorf("Message %q, URL %q", c.Message, c.URL)
		}
		if !reflect.DeepEqual(c.Parents, []string{shas[248]}) {
			t.Errorf("Parents = %v", c.Parents)
		}
		if root := commits[249]; root.Parents == nil || len(root.Parents) != 0 {
			t.Errorf("root commit: Parents = %#v, want an empty non-nil slice", root.Parents)
		}
	}

	// Limit: only the needed pages are requested.
	e.srv.ResetRequests()
	limited, err := p.ListCommits(ctx, scOwner, "projet-pagination", provider.ListCommitsOptions{Branch: "main", Limit: 150})
	if err != nil {
		t.Fatalf("ListCommits (Limit): %v", err)
	}
	scWantSHAs(t, "ListCommits (Limit 150)", limited, scReversed(shas)[:150]...)
	scCheckPages(t, e.srv.Requests(), base+"/commits", 2)

	// Since (in another time zone): converted to UTC, filters on the commit date, a single page.
	e.srv.ResetRequests()
	since := e.t0.Add(200 * time.Minute)
	recent, err := p.ListCommits(ctx, scOwner, "projet-pagination", provider.ListCommitsOptions{
		Branch: "main", Since: since.In(time.FixedZone("UTC+2", 2*3600)),
	})
	if err != nil {
		t.Fatalf("ListCommits (Since): %v", err)
	}
	scWantSHAs(t, "ListCommits (Since)", recent, scReversed(shas[200:])...)
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Query.Get("since") != since.UTC().Format(time.RFC3339) {
		t.Errorf("requests %+v, want one request with since=%s", reqs, since.UTC().Format(time.RFC3339))
	}
}

// TestScenarioIncrementalSync follows the "Incremental sync" strategy
// (documentation/en/03-architecture.md): first import of the default branch by ListCommits, other
// branch by CompareCommits from the head of the default branch, then incremental sync by
// CompareCommits between the old and the new head, including a merge that brings commits older
// than the old head (and that stopping at the first known SHA would miss).
func TestScenarioIncrementalSync(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1002, "projet-incremental"
	base := e.repo(id, name)
	h := func(hours float64) time.Time { return e.t0.Add(time.Duration(hours * float64(time.Hour))) }
	m1 := e.commit(id, "m1", h(1))
	m2 := e.commit(id, "m2", h(2), m1)
	m3 := e.commit(id, "m3", h(3), m2)
	e.srv.SetBranch(id, "main", m3)
	d1 := e.commit(id, "d1", h(4), m2)
	d2 := e.commit(id, "d2", h(5), d1)
	e.srv.SetBranch(id, "dev", d2)
	p := e.provider(testToken)
	ctx := t.Context()

	// The sync engine's database, simulated: the SHAs already recorded.
	known := map[string]bool{}
	stop := func(sha string) bool { return known[sha] }
	remember := func(commits []provider.Commit) {
		for _, c := range commits {
			known[c.SHA] = true
		}
	}

	// First import: main, the default branch, through the list of commits (nothing in the database).
	first, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits main: %v", err)
	}
	scWantSHAs(t, "first import of main", first, m3, m2, m1)
	remember(first)

	// Other branch: compare from the head of main; m2, shared with main, is not returned.
	e.srv.ResetRequests()
	dev, err := p.CompareCommits(ctx, scOwner, name, m3, d2)
	if err != nil {
		t.Fatalf("CompareCommits main...dev: %v", err)
	}
	scWantSHAs(t, "first import of dev", dev, d2, d1)
	if n := e.srv.Count(base + "/compare/" + m3 + "..." + d2); n != 1 || len(e.srv.Requests()) != 1 {
		t.Errorf("%d requests (%d for the comparison), want a single comparison", len(e.srv.Requests()), n)
	}
	remember(dev)

	// New activity on main: a branch started from m1, with commits OLDER than the old head m3, merged
	// into main, then a commit.
	f1 := e.commit(id, "f1", h(1.5), m1)
	f2 := e.commit(id, "f2", h(2.5), f1)
	merge := e.commit(id, "merge", h(6), m3, f2)
	m4 := e.commit(id, "m4", h(7), merge)
	e.srv.SetBranch(id, "main", m4)

	// The trap: in chronological order, f2 and f1 come after m3 (already known); stopping at the first
	// known SHA misses them.
	naive, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main", Stop: stop})
	if err != nil {
		t.Fatalf("ListCommits main (2): %v", err)
	}
	scWantSHAs(t, "ListCommits stopped at the first known SHA", naive, m4, merge)

	// The exact method: compare between the old and the new head, from newest to oldest.
	got, err := p.CompareCommits(ctx, scOwner, name, m3, m4)
	if err != nil {
		t.Fatalf("CompareCommits: %v", err)
	}
	scWantSHAs(t, "CompareCommits m3...m4", got, m4, merge, f2, f1)
	if len(got) == 4 && !reflect.DeepEqual(got[1].Parents, []string{m3, f2}) {
		t.Errorf("merge parents = %v", shortSHAs(got[1].Parents))
	}
	if len(got) == 4 && (got[0].Author.Login != scSecretLogin || got[0].Message == "") {
		t.Errorf("converted commit: %+v", got[0].Author)
	}

	// base == head: empty list, without request; head behind base: empty list.
	e.srv.ResetRequests()
	same, err := p.CompareCommits(ctx, scOwner, name, m4, m4)
	if err != nil || same == nil || len(same) != 0 || len(e.srv.Requests()) != 0 {
		t.Errorf("CompareCommits(m4, m4): %v, %v, %d requests; want an empty list without request", same, err, len(e.srv.Requests()))
	}
	behind, err := p.CompareCommits(ctx, scOwner, name, m4, m3)
	if err != nil || behind == nil || len(behind) != 0 {
		t.Errorf("CompareCommits(m4, m3): %v, %v; want an empty list", behind, err)
	}

	// Paginated compare: 130 new commits, 2 pages, result from newest to oldest.
	news := e.srv.AppendCommits(id, "main", 130, h(8), time.Minute)
	e.srv.ResetRequests()
	got, err = p.CompareCommits(ctx, scOwner, name, m4, news[129])
	if err != nil {
		t.Fatalf("paginated CompareCommits: %v", err)
	}
	scWantSHAs(t, "paginated CompareCommits", got, scReversed(news)...)
	scCheckPages(t, e.srv.Requests(), base+"/compare/"+m4+"..."+news[129], 2)

	// Statistics of a new commit.
	stats, err := p.CommitStats(ctx, scOwner, name, merge)
	if err != nil || stats != (provider.CommitStats{Additions: 10, Deletions: 2}) {
		t.Errorf("CommitStats: %+v, %v", stats, err)
	}
}

// TestScenarioNewBranchWithMerge: common case among students, non-regression of the "Incremental
// sync" strategy (documentation/en/03-architecture.md). main is in the database (M0 then M1); a
// student pushes a new branch started from M0, with F1 (dated between M0 and M1), then a merge of
// main (X). Stopping at the first known SHA loses F1 without error; the comparison from the head of
// the default branch gives it, and the next sync compares from the new head.
func TestScenarioNewBranchWithMerge(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1012, "projet-branche-fusion"
	e.repo(id, name)
	h := func(hours float64) time.Time { return e.t0.Add(time.Duration(hours * float64(time.Hour))) }
	m0 := e.commit(id, "m0", h(0))
	m1 := e.commit(id, "m1", h(2), m0)
	e.srv.SetBranch(id, "main", m1)
	p := e.provider(testToken)
	ctx := t.Context()

	// First import of main.
	known := map[string]bool{}
	first, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits main: %v", err)
	}
	scWantSHAs(t, "first import of main", first, m1, m0)
	for _, c := range first {
		known[c.SHA] = true
	}

	// New branch: F1 from M0, then merge of main.
	f1 := e.commit(id, "f1", h(1), m0)
	x := e.commit(id, "merge de main", h(3), f1, m1)
	e.srv.SetBranch(id, "feature", x)

	// The trap: the chronological list gives X, M1, F1, M0; stopping at the first known SHA (M1) loses F1.
	all, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "feature"})
	if err != nil {
		t.Fatalf("ListCommits feature: %v", err)
	}
	scWantSHAs(t, "chronological list of feature", all, x, m1, f1, m0)
	naive, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{
		Branch: "feature", Stop: func(sha string) bool { return known[sha] },
	})
	if err != nil {
		t.Fatalf("ListCommits feature (Stop): %v", err)
	}
	scWantSHAs(t, "ListCommits stopped at the first known SHA (trap)", naive, x)

	// The documented method: compare from the head of the default branch.
	branches, err := p.Branches(ctx, scOwner, name)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	heads := map[string]string{}
	for _, b := range branches {
		heads[b.Name] = b.HeadSHA
	}
	got, err := p.CompareCommits(ctx, scOwner, name, heads["main"], heads["feature"])
	if err != nil {
		t.Fatalf("CompareCommits main...feature: %v", err)
	}
	scWantSHAs(t, "CompareCommits from the head of main", got, x, f1)
	for _, c := range got {
		known[c.SHA] = true
	}

	// Next sync: compare from the old head of feature.
	f2 := e.commit(id, "f2", h(4), x)
	e.srv.SetBranch(id, "feature", f2)
	got, err = p.CompareCommits(ctx, scOwner, name, x, f2)
	if err != nil {
		t.Fatalf("CompareCommits X...F2: %v", err)
	}
	scWantSHAs(t, "next sync of feature", got, f2)
	for _, c := range got {
		known[c.SHA] = true
	}
	for _, sha := range []string{m0, m1, f1, x, f2} {
		if !known[sha] {
			t.Errorf("commit %s never reported", provider.ShortSHA(sha))
		}
	}
}

// TestScenarioForcePush: rewritten history. As long as GitHub keeps the old head, compare gives the
// new commits; after its cleanup, ErrUnknownBase (and not ErrNotFound), then fallback on
// ListCommits without Stop.
func TestScenarioForcePush(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1003, "projet-force-push"
	base := e.repo(id, name)
	old := e.srv.AppendCommits(id, "main", 5, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()

	known := map[string]bool{}
	first, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	scWantSHAs(t, "first sync", first, scReversed(old)...)
	for _, c := range first {
		known[c.SHA] = true
	}

	// Force-push: main goes back to old[2], then two new commits.
	oldHead := old[4]
	e.srv.SetBranch(id, "main", old[2])
	rewritten := e.srv.AppendCommits(id, "main", 2, e.t0.Add(10*time.Hour), time.Hour)
	newHead := rewritten[1]

	got, err := p.CompareCommits(ctx, scOwner, name, oldHead, newHead)
	if err != nil {
		t.Fatalf("CompareCommits before cleanup: %v", err)
	}
	scWantSHAs(t, "diverged histories", got, rewritten[1], rewritten[0])

	// GitHub's cleanup: the old head no longer exists.
	e.srv.RemoveCommit(id, oldHead)
	e.srv.ResetRequests()
	_, err = p.CompareCommits(ctx, scOwner, name, oldHead, newHead)
	if !errors.Is(err, provider.ErrUnknownBase) || errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("CompareCommits after cleanup: %v; want ErrUnknownBase without ErrNotFound", err)
	}
	scAPIError(t, err, http.StatusNotFound, base+"/compare/"+oldHead+"..."+newHead)
	if !strings.Contains(err.Error(), "force-push") {
		t.Errorf("the message must mention a force-push: %v", err)
	}
	if n := e.srv.Count(base); n != 1 {
		t.Errorf("%d reads of the repository to settle the 404, want 1", n)
	}

	// The engine's fallback (main is the reference branch: no other base to try): ListCommits without
	// Stop, bounded by Limit, ignoring the SHAs already in the database.
	again, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListCommits after force-push: %v", err)
	}
	scWantSHAs(t, "fallback on ListCommits", again, rewritten[1], rewritten[0], old[2], old[1], old[0])
	var unknown []provider.Commit
	for _, c := range again {
		if !known[c.SHA] {
			unknown = append(unknown, c)
		}
	}
	scWantSHAs(t, "new commits after force-push", unknown, rewritten[1], rewritten[0])

	// Statistics of the vanished commit: GitHub answers 422, the Provider returns ErrNotFound.
	_, err = p.CommitStats(ctx, scOwner, name, oldHead)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("CommitStats of a vanished commit: %v; want ErrNotFound", err)
	}
	scAPIError(t, err, http.StatusUnprocessableEntity, base+"/commits/"+oldHead)
}

// scSnapshot is the result of a full read of a repository.
type scSnapshot struct {
	Repo     provider.Repository
	Branches []provider.Branch
	Commits  []provider.Commit
}

// scRead reads the repository name, its branches and the commits of main.
func scRead(t *testing.T, p *Provider, name string) scSnapshot {
	t.Helper()
	ctx := t.Context()
	repo, err := p.Repository(ctx, scOwner, name)
	if err != nil {
		t.Fatalf("Repository: %v", err)
	}
	branches, err := p.Branches(ctx, scOwner, name)
	if err != nil {
		t.Fatalf("Branches: %v", err)
	}
	commits, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	return scSnapshot{Repo: repo, Branches: branches, Commits: commits}
}

// TestScenarioETag: second identical call served from the cache (304, quota not decremented), cache
// shared between two Providers of the same token, emptied by another token.
func TestScenarioETag(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1004, "projet-etag"
	e.repo(id, name)
	shas := e.srv.AppendCommits(id, "main", 250, e.t0, time.Minute)
	e.srv.SetBranch(id, "dev", shas[100])
	ctx := t.Context()

	// First read: 5 requests (repository, branches, 3 pages of commits), all counted.
	p1 := e.provider(testToken)
	first := scRead(t, p1, name)
	remaining := e.srv.Quota().Remaining
	if remaining != githubtest.DefaultRateLimit-5 {
		t.Errorf("remaining quota %d, want %d", remaining, githubtest.DefaultRateLimit-5)
	}
	if rl, ok := p1.LastRateLimit(); !ok || rl.Remaining != remaining || rl.Limit != githubtest.DefaultRateLimit {
		t.Errorf("LastRateLimit = %+v, %v", rl, ok)
	}
	if e.cache.Len() != 5 {
		t.Errorf("%d cache entries, want 5", e.cache.Len())
	}

	// Second identical read: 304 everywhere, same data (pagination included), quota intact.
	served := e.countLogs("304 servi depuis le cache")
	e.srv.ResetRequests()
	if second := scRead(t, p1, name); !reflect.DeepEqual(second, first) {
		t.Error("the data served from the cache differ from the first read")
	}
	scAll304(t, e.srv.Requests(), 5)
	if got := e.srv.Quota().Remaining; got != remaining {
		t.Errorf("remaining quota %d after 304s, want %d (unchanged)", got, remaining)
	}
	if rl, ok := p1.LastRateLimit(); !ok || rl.Remaining != remaining {
		t.Errorf("LastRateLimit after 304s = %+v, %v", rl, ok)
	}
	if n := e.countLogs("304 servi depuis le cache") - served; n != 5 {
		t.Errorf("%d \"304 servi depuis le cache\" logs, want 5", n)
	}

	// Next sync: new Provider, same token, same Cache and same Quota.
	p2 := e.provider(testToken)
	if rl, ok := p2.LastRateLimit(); !ok || rl.Remaining != remaining {
		t.Errorf("shared quota: LastRateLimit of the new Provider = %+v, %v", rl, ok)
	}
	e.srv.ResetRequests()
	if third := scRead(t, p2, name); !reflect.DeepEqual(third, first) {
		t.Error("shared cache: different data")
	}
	scAll304(t, e.srv.Requests(), 5)
	if got := e.srv.Quota().Remaining; got != remaining {
		t.Errorf("remaining quota %d, want %d (unchanged)", got, remaining)
	}

	// A change on GitHub invalidates the entry: 200 response, new data, one unit of quota.
	added := e.srv.AppendCommits(id, "main", 1, e.t0.Add(300*time.Minute), time.Minute)
	e.srv.ResetRequests()
	latest, err := p2.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main", Limit: 1})
	if err != nil {
		t.Fatalf("ListCommits after a push: %v", err)
	}
	scWantSHAs(t, "after a push", latest, added[0])
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Status != http.StatusOK || reqs[0].IfNoneMatch == "" {
		t.Errorf("requests %+v, want one conditional request answered 200", reqs)
	}
	if got := e.srv.Quota().Remaining; got != remaining-1 {
		t.Errorf("remaining quota %d, want %d", got, remaining-1)
	}

	// Other token (other gh account): the cache is emptied before sending, no response of the first
	// account is served to it.
	other := e.provider(scOtherToken)
	e.srv.ResetRequests()
	_, err = other.Repository(ctx, scOwner, name)
	if !errors.Is(err, provider.ErrUnauthorized) {
		t.Errorf("Repository with another token: %v; want ErrUnauthorized (token refused by the fake server)", err)
	}
	scAPIError(t, err, http.StatusUnauthorized, "/repos/"+scOwner+"/"+name)
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].IfNoneMatch != "" || reqs[0].ValidToken {
		t.Errorf("requests %+v, want one request without If-None-Match", reqs)
	}
	if n := e.cache.Len(); n != 0 {
		t.Errorf("%d cache entries after a token change, want 0", n)
	}
	if _, ok := other.LastRateLimit(); ok {
		t.Error("LastRateLimit of another token: want no state")
	}

	// Back to the first account: nothing left in the cache, full 200 response, quota counted.
	before := e.srv.Quota().Remaining
	e.srv.ResetRequests()
	repo, err := p1.Repository(ctx, scOwner, name)
	if err != nil || repo != first.Repo {
		t.Errorf("Repository: %+v, %v", repo, err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Status != http.StatusOK || reqs[0].IfNoneMatch != "" {
		t.Errorf("requests %+v, want one non-conditional request answered 200", reqs)
	}
	if got := e.srv.Quota().Remaining; got != before-1 {
		t.Errorf("remaining quota %d, want %d", got, before-1)
	}
}

// TestScenarioQuotaExhausted: exhausted quota (403 then 429) → RateLimitError with Reset; following
// requests held back without sending (even by another Provider sharing the Quota); /rate_limit
// allowed; resumption after the reset.
func TestScenarioQuotaExhausted(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1005, "projet-quota"
	base := e.repo(id, name)
	shas := e.srv.AppendCommits(id, "main", 3, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()
	if _, err := p.Repository(ctx, scOwner, name); err != nil {
		t.Fatalf("Repository: %v", err)
	}

	// Quota exhausted in the current window.
	q := e.srv.Quota()
	q.Remaining = 0
	e.srv.SetQuota(q)
	reset := q.Reset
	_, err := p.Branches(ctx, scOwner, name)
	scRateLimit(t, err, false, reset, 0)
	scAPIError(t, err, http.StatusForbidden, base+"/branches")
	if !strings.Contains(err.Error(), reset.Format("2006-01-02 15:04:05 UTC")) {
		t.Errorf("the message must give the reset: %v", err)
	}

	// Following requests held back without sending, including by the Provider of the next sync.
	e.srv.ResetRequests()
	_, err = p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{})
	scRateLimit(t, err, false, reset, 0)
	scAPIError(t, err, 0, base+"/commits")
	_, err = e.provider(testToken).Repository(ctx, scOwner, name)
	scRateLimit(t, err, false, reset, 0)
	if n := len(e.srv.Requests()); n != 0 {
		t.Errorf("%d requests sent while the quota is known as exhausted, want 0", n)
	}

	// GET /rate_limit stays allowed (and free).
	rl, err := p.RateLimit(ctx)
	if err != nil || rl.Remaining != 0 || !rl.Reset.Equal(reset) || rl.Limit != githubtest.DefaultRateLimit || !rl.ObservedAt.Equal(trEpoch) {
		t.Errorf("RateLimit: %+v, %v", rl, err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Path != "/rate_limit" {
		t.Errorf("requests %+v, want GET /rate_limit only", reqs)
	}
	if last, ok := p.LastRateLimit(); !ok || last.Remaining != 0 {
		t.Errorf("LastRateLimit = %+v, %v", last, ok)
	}

	// After the reset, the requests start again: the Provider never waits by itself.
	e.clock.set(reset.Add(time.Second))
	if _, err := p.Branches(ctx, scOwner, name); err != nil {
		t.Fatalf("Branches after the reset: %v", err)
	}
	if last, ok := p.LastRateLimit(); !ok || last.Remaining != githubtest.DefaultRateLimit-1 || !last.Reset.After(reset) {
		t.Errorf("LastRateLimit after the reset = %+v, %v", last, ok)
	}

	// 429 variant.
	q = e.srv.Quota()
	q.Remaining, q.Status = 0, http.StatusTooManyRequests
	e.srv.SetQuota(q)
	_, err = p.CommitStats(ctx, scOwner, name, shas[0])
	scRateLimit(t, err, false, q.Reset, 0)
	scAPIError(t, err, http.StatusTooManyRequests, base+"/commits/"+shas[0])
}

// TestScenarioSecondaryLimit: secondary limit with Retry-After (RetryAfter, Reset) then without
// (one minute of waiting, as GitHub recommends); requests held back during the wait.
func TestScenarioSecondaryLimit(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1006, "projet-secondaire"
	base := e.repo(id, name)
	e.srv.AppendCommits(id, "main", 3, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()

	e.srv.TriggerSecondaryLimit(githubtest.SecondaryLimit{RetryAfter: 90 * time.Second})
	_, err := p.Branches(ctx, scOwner, name)
	scRateLimit(t, err, true, trEpoch.Add(90*time.Second), 90*time.Second)
	scAPIError(t, err, http.StatusForbidden, base+"/branches")

	// During the wait: requests held back without sending; /rate_limit stays allowed.
	e.srv.ResetRequests()
	_, err = p.Repository(ctx, scOwner, name)
	scRateLimit(t, err, true, trEpoch.Add(90*time.Second), 90*time.Second)
	if _, err := p.RateLimit(ctx); err != nil {
		t.Errorf("RateLimit during a secondary limit: %v", err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Path != "/rate_limit" {
		t.Errorf("requests %+v, want GET /rate_limit only", reqs)
	}
	e.clock.add(91 * time.Second)
	if _, err := p.Branches(ctx, scOwner, name); err != nil {
		t.Fatalf("Branches after Retry-After: %v", err)
	}

	// 429 without Retry-After (message only): one minute of waiting.
	e.srv.TriggerSecondaryLimit(githubtest.SecondaryLimit{Status: http.StatusTooManyRequests})
	start := e.clock.now()
	_, err = p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{})
	scRateLimit(t, err, true, start.Add(time.Minute), 0)
	scAPIError(t, err, http.StatusTooManyRequests, base+"/commits")
	e.srv.ResetRequests()
	e.clock.add(30 * time.Second)
	_, err = p.Branches(ctx, scOwner, name)
	scRateLimit(t, err, true, start.Add(time.Minute), 30*time.Second)
	if n := len(e.srv.Requests()); n != 0 {
		t.Errorf("%d requests sent during the secondary limit, want 0", n)
	}
	e.clock.add(31 * time.Second)
	if _, err := p.Branches(ctx, scOwner, name); err != nil {
		t.Fatalf("Branches after one minute: %v", err)
	}
}

// scCall is a Provider call on a repository.
type scCall struct {
	name string
	call func() error
}

// scCalls returns a call of each Provider method on the repository owner/name (id), with dummy SHAs
// for CompareCommits and CommitStats.
func scCalls(t *testing.T, p *Provider, owner, name string, id int64) []scCall {
	ctx := t.Context()
	a, b := githubtest.FakeSHA("a"), githubtest.FakeSHA("b")
	return []scCall{
		{"Repository", func() error { _, err := p.Repository(ctx, owner, name); return err }},
		{"RepositoryByID", func() error { _, err := p.RepositoryByID(ctx, id); return err }},
		{"Branches", func() error { _, err := p.Branches(ctx, owner, name); return err }},
		{"ListCommits", func() error {
			_, err := p.ListCommits(ctx, owner, name, provider.ListCommitsOptions{Branch: "main"})
			return err
		}},
		{"CompareCommits", func() error { _, err := p.CompareCommits(ctx, owner, name, a, b); return err }},
		{"CommitStats", func() error { _, err := p.CommitStats(ctx, owner, name, a); return err }},
	}
}

// TestScenarioAccess: private repository without access (404), organization enforcing SSO (403),
// refused token (401), on all the methods.
func TestScenarioAccess(t *testing.T) {
	e := newScEnv(t)
	const hiddenID, ssoID = 1007, 1008
	e.repo(hiddenID, "projet-prive")
	e.srv.AppendCommits(hiddenID, "main", 3, e.t0, time.Hour)
	e.srv.SetRepoAccess(hiddenID, githubtest.AccessHidden)
	e.srv.AddRepo(githubtest.Repo{ID: ssoID, Owner: "ecole-sso", Name: "projet-sso", Private: true})
	e.srv.AppendCommits(ssoID, "main", 3, e.t0, time.Hour)
	e.srv.SetRepoAccess(ssoID, githubtest.AccessSSO)
	p := e.provider(testToken)

	// 404: never "deleted", never ErrUnknownBase (CompareCommits reads the repository again).
	for _, c := range scCalls(t, p, scOwner, "projet-prive", hiddenID) {
		err := c.call()
		if !errors.Is(err, provider.ErrNotFound) || errors.Is(err, provider.ErrUnknownBase) || errors.Is(err, provider.ErrForbidden) {
			t.Errorf("%s on a private repository without access: %v; want ErrNotFound", c.name, err)
			continue
		}
		scAPIError(t, err, http.StatusNotFound, "")
		if !strings.Contains(err.Error(), "repository not found or account without access") {
			t.Errorf("%s: message %q", c.name, err)
		}
	}

	// 403 SSO: *SSORequiredError, which satisfies ErrForbidden; the link is not quoted in the message.
	for _, c := range scCalls(t, p, "ecole-sso", "projet-sso", ssoID) {
		err := c.call()
		sso, ok := errors.AsType[*provider.SSORequiredError](err)
		if !ok || !errors.Is(err, provider.ErrForbidden) {
			t.Errorf("%s under SSO: %v; want *SSORequiredError", c.name, err)
			continue
		}
		if sso.Organization != "ecole-sso" || sso.URL != "https://github.com/orgs/ecole-sso/sso?authorization_request=fake" {
			t.Errorf("%s: SSORequiredError = %+v", c.name, sso)
		}
		scAPIError(t, err, http.StatusForbidden, "")
		if msg := err.Error(); !strings.Contains(msg, "ecole-sso") || !strings.Contains(msg, "SSO") {
			t.Errorf("%s: message %q", c.name, msg)
		}
	}

	// 401: refused (revoked) token, "gh auth login" advised, token never quoted.
	bad := e.provider(scBadToken)
	calls := append(scCalls(t, bad, scOwner, "projet-prive", hiddenID),
		scCall{"RateLimit", func() error { _, err := bad.RateLimit(t.Context()); return err }})
	for _, c := range calls {
		err := c.call()
		if !errors.Is(err, provider.ErrUnauthorized) {
			t.Errorf("%s with a refused token: %v; want ErrUnauthorized", c.name, err)
			continue
		}
		scAPIError(t, err, http.StatusUnauthorized, "")
		if !strings.Contains(err.Error(), "gh auth login") {
			t.Errorf("%s: the message must advise \"gh auth login\": %v", c.name, err)
		}
	}
}

// TestScenarioRenamedRepository: repository renamed then transferred; the old name follows the 301
// redirection to /repositories/{id} and the results carry the current owner/name.
func TestScenarioRenamedRepository(t *testing.T) {
	e := newScEnv(t)
	const id = 4242
	e.repo(id, "projet-ancien")
	shas := e.srv.AppendCommits(id, "main", 3, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()

	e.srv.RenameRepo(id, scOwner, "projet-renomme")
	repo, err := p.Repository(ctx, scOwner, "projet-ancien")
	if err != nil || repo.ID != id || repo.Owner != scOwner || repo.Name != "projet-renomme" {
		t.Fatalf("Repository after renaming: %+v, %v", repo, err)
	}
	reqs := e.srv.Requests()
	if len(reqs) != 2 || reqs[0].Status != http.StatusMovedPermanently || reqs[0].Path != "/repos/"+scOwner+"/projet-ancien" ||
		reqs[1].Status != http.StatusOK || reqs[1].Path != "/repositories/4242" || !reqs[1].ValidToken {
		t.Errorf("requests %+v, want 301 then 200 on /repositories/4242", reqs)
	}

	// Transfer to another organization: the original name still follows.
	e.srv.RenameRepo(id, "autre-ecole", "projet-transfere")
	repo, err = p.Repository(ctx, scOwner, "projet-ancien")
	if err != nil || repo.ID != id || repo.FullName() != "autre-ecole/projet-transfere" {
		t.Fatalf("Repository after transfer: %+v, %v", repo, err)
	}
	byID, err := p.RepositoryByID(ctx, id)
	if err != nil || byID != repo {
		t.Errorf("RepositoryByID: %+v, %v; want %+v", byID, err, repo)
	}

	// The other calls follow the redirection too.
	commits, err := p.ListCommits(ctx, scOwner, "projet-ancien", provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits by the old name: %v", err)
	}
	scWantSHAs(t, "ListCommits by the old name", commits, scReversed(shas)...)
	branches, err := p.Branches(ctx, scOwner, "projet-renomme")
	if err != nil || len(branches) != 1 || branches[0] != (provider.Branch{Name: "main", HeadSHA: shas[2]}) {
		t.Errorf("Branches by an intermediate name: %+v, %v", branches, err)
	}
	got, err := p.CompareCommits(ctx, scOwner, "projet-ancien", shas[0], shas[2])
	if err != nil {
		t.Fatalf("CompareCommits by the old name: %v", err)
	}
	scWantSHAs(t, "CompareCommits by the old name", got, shas[2], shas[1])
	if _, err := p.CommitStats(ctx, scOwner, "projet-ancien", shas[1]); err != nil {
		t.Errorf("CommitStats by the old name: %v", err)
	}
	// The new name answers directly.
	if repo, err := p.Repository(ctx, "autre-ecole", "projet-transfere"); err != nil || repo.ID != id {
		t.Errorf("Repository by the new name: %+v, %v", repo, err)
	}
}

// TestScenarioEmptyAndArchived: empty repository (freshly created by a student) and archived repository.
func TestScenarioEmptyAndArchived(t *testing.T) {
	e := newScEnv(t)
	base := e.repo(1009, "projet-vide")
	p := e.provider(testToken)
	ctx := t.Context()

	repo, err := p.Repository(ctx, scOwner, "projet-vide")
	if err != nil || repo.ID != 1009 || repo.DefaultBranch != "main" || !repo.Private || repo.Archived {
		t.Errorf("Repository of an empty repository: %+v, %v", repo, err)
	}
	branches, err := p.Branches(ctx, scOwner, "projet-vide")
	if err != nil || branches == nil || len(branches) != 0 {
		t.Errorf("Branches of an empty repository: %#v, %v; want an empty non-nil list", branches, err)
	}
	commits, err := p.ListCommits(ctx, scOwner, "projet-vide", provider.ListCommitsOptions{})
	if err != nil || commits == nil || len(commits) != 0 {
		t.Errorf("ListCommits of an empty repository: %#v, %v; want an empty non-nil list", commits, err)
	}
	a, b := githubtest.FakeSHA("a"), githubtest.FakeSHA("b")
	_, err = p.CompareCommits(ctx, scOwner, "projet-vide", a, b)
	if !errors.Is(err, provider.ErrEmptyRepository) {
		t.Errorf("CompareCommits of an empty repository: %v; want ErrEmptyRepository", err)
	} else {
		scAPIError(t, err, http.StatusConflict, base+"/compare/"+a+"..."+b)
	}
	_, err = p.CommitStats(ctx, scOwner, "projet-vide", a)
	if !errors.Is(err, provider.ErrEmptyRepository) || !strings.Contains(err.Error(), "empty repository") {
		t.Errorf("CommitStats of an empty repository: %v; want ErrEmptyRepository", err)
	} else {
		scAPIError(t, err, http.StatusConflict, base+"/commits/"+a)
	}

	// Archived repository, default branch "master": metadata and commits still readable.
	pushed := trEpoch.Add(-48 * time.Hour)
	e.srv.AddRepo(githubtest.Repo{
		ID: 1010, Owner: scOwner, Name: "projet-archive", Private: true, Archived: true,
		DefaultBranch: "master", Stars: 3, OpenIssues: 2, PushedAt: pushed,
	})
	shas := e.srv.AppendCommits(1010, "master", 2, e.t0, time.Hour)
	repo, err = p.Repository(ctx, scOwner, "projet-archive")
	want := provider.Repository{
		ID: 1010, Owner: scOwner, Name: "projet-archive", DefaultBranch: "master",
		URL: "https://github.com/" + scOwner + "/projet-archive", Stars: 3, OpenIssues: 2,
		Archived: true, Private: true, PushedAt: pushed,
	}
	if err != nil || repo != want || repo.PushedAt.Location() != time.UTC {
		t.Errorf("Repository of an archived repository: %+v, %v\nwant %+v", repo, err, want)
	}
	e.srv.ResetRequests()
	commits, err = p.ListCommits(ctx, scOwner, "projet-archive", provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits of an archived repository: %v", err)
	}
	scWantSHAs(t, "ListCommits (default branch)", commits, shas[1], shas[0])
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Query.Has("sha") {
		t.Errorf("requests %+v, want one request without sha parameter (default branch)", reqs)
	}
}

// TestScenarioReadOnly: the transport refuses POST, PATCH, PUT and DELETE before any sending, whether
// the request comes from go-github or from an http.Client plugged into it. The fake server would fail
// the test anyway if it received one.
func TestScenarioReadOnly(t *testing.T) {
	e := newScEnv(t)
	base := e.repo(1011, "projet-lecture")
	e.srv.AppendCommits(1011, "main", 1, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		req, err := p.client.NewRequest(ctx, method, "repos/"+scOwner+"/projet-lecture/issues", map[string]string{"title": "x"})
		if err != nil {
			t.Fatalf("NewRequest %s: %v", method, err)
		}
		_, err = p.client.Do(req, nil)
		if !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s through go-github: %v; want ErrWriteForbidden", method, err)
		}
		conv := p.convertError(ctx, "writing", nil, err)
		if !errors.Is(conv, provider.ErrWriteForbidden) {
			t.Errorf("converted %s: %v; want ErrWriteForbidden", method, conv)
		}
		if apiErr := scAPIError(t, conv, 0, base+"/issues"); apiErr.Method != method {
			t.Errorf("%s: APIError.Method = %q", method, apiErr.Method)
		}

		hreq, err := http.NewRequestWithContext(ctx, method, e.srv.URL()+"repos/"+scOwner+"/projet-lecture", strings.NewReader(`{"archived":true}`))
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := (&http.Client{Transport: p.tr}).Do(hreq)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s through http.Client: %v; want ErrWriteForbidden", method, err)
		}
		scNoLeak(t, err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 0 {
		t.Errorf("%d requests received by the fake server, want 0: %+v", len(reqs), reqs)
	}

	// A read goes through normally.
	if _, err := p.Repository(ctx, scOwner, "projet-lecture"); err != nil {
		t.Errorf("Repository: %v", err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Method != http.MethodGet {
		t.Errorf("requests %+v, want a GET", reqs)
	}
}

// TestScenarioNoLeak: a run that produces Debug logs of every kind (requests, 304, preventive
// refusal, errors) and errors of every kind; neither the token nor a personal data item appears
// (also checked by checkNoLeak at the end of the test). A logger at Info level receives nothing.
func TestScenarioNoLeak(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1012, "projet-fuites"
	e.repo(id, name)
	m1 := e.commit(id, "m1", e.t0)
	m2 := e.commit(id, "m2", e.t0.Add(time.Hour), m1)
	e.srv.SetBranch(id, "main", m2)
	e.srv.SetBranch(id, "feature/"+scSecretLogin, m1) // branch name never logged
	e.repo(1013, "projet-cache")
	e.srv.SetRepoAccess(1013, githubtest.AccessHidden)
	ctx := t.Context()

	run := func(p *Provider) []error {
		var errs []error
		keep := func(err error) {
			if err != nil {
				errs = append(errs, err)
			}
		}
		for range 2 { // the second time, from the cache
			_, err := p.Repository(ctx, scOwner, name)
			keep(err)
			_, err = p.Branches(ctx, scOwner, name)
			keep(err)
			_, err = p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "feature/" + scSecretLogin})
			keep(err)
			_, err = p.CompareCommits(ctx, scOwner, name, m1, m2)
			keep(err)
			_, err = p.CommitStats(ctx, scOwner, name, m2)
			keep(err)
		}
		_, err := p.Repository(ctx, scOwner, "projet-cache")
		keep(err)
		_, err = p.CompareCommits(ctx, scOwner, name, githubtest.FakeSHA("x"), m2)
		keep(err)
		e.srv.TriggerSecondaryLimit(githubtest.SecondaryLimit{RetryAfter: time.Minute})
		_, err = p.Branches(ctx, scOwner, name)
		keep(err)
		_, err = p.Branches(ctx, scOwner, name) // held back
		keep(err)
		e.clock.add(2 * time.Minute)
		_, err = e.provider(scBadToken).Repository(ctx, scOwner, name)
		keep(err)
		return errs
	}

	errs := run(e.provider(testToken))
	if len(errs) != 5 {
		t.Errorf("%d errors, want 5: %v", len(errs), errs)
	}
	for _, err := range errs {
		scNoLeak(t, err)
	}
	for _, msg := range []string{"requête à l'API GitHub", "304 servi depuis le cache", "requête retenue : limite secondaire de l'API en cours", "commits listés", "commits comparés"} {
		if e.countLogs(msg) == 0 {
			t.Errorf("no %q log: capturing Debug logs does not work", msg)
		}
	}
	if strings.Contains(e.logs.String(), "feature/") {
		t.Error("the logs quote a branch name")
	}

	// Info level: no log.
	var info trLogs
	p, err := New(testToken, Options{BaseURL: e.srv.URL(), Logger: trLogger(&info, slog.LevelInfo)})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{}); err != nil {
		t.Errorf("ListCommits: %v", err)
	}
	if _, err := p.Repository(ctx, scOwner, "projet-cache"); err == nil {
		t.Error("Repository of a hidden repository: want an error")
	}
	if s := info.String(); s != "" {
		t.Errorf("logs at Info level: %s", s)
	}
}

// scRepoSpec describes a repository of the concurrent scenario.
type scRepoSpec struct {
	id        int64
	name      string
	main, dev []string
}

// scSyncResult is what a simulated sync reads from a repository.
type scSyncResult struct {
	Repo     provider.Repository
	Branches []provider.Branch
	Commits  map[string][]string // branch → SHAs read (list for the default branch, compare otherwise)
	Compare  []string
	Stats    provider.CommitStats
}

// scSync reads a repository as the sync engine would at the first import ("Incremental sync",
// documentation/en/03-architecture.md): metadata, branches, commits of the default branch through
// ListCommits, those of every other branch through CompareCommits from the head of the default
// branch, then a comparison and statistics.
func scSync(t *testing.T, p *Provider, s scRepoSpec) (scSyncResult, error) {
	ctx := t.Context()
	var res scSyncResult
	var err error
	if res.Repo, err = p.Repository(ctx, scOwner, s.name); err != nil {
		return res, err
	}
	if res.Branches, err = p.Branches(ctx, scOwner, s.name); err != nil {
		return res, err
	}
	def := res.Repo.DefaultBranch
	heads := map[string]string{}
	for _, b := range res.Branches {
		heads[b.Name] = b.HeadSHA
	}
	res.Commits = map[string][]string{}
	list, err := p.ListCommits(ctx, scOwner, s.name, provider.ListCommitsOptions{Branch: def})
	if err != nil {
		return res, err
	}
	res.Commits[def] = scSHAs(list)
	for _, b := range res.Branches {
		if b.Name == def {
			continue
		}
		own, err := p.CompareCommits(ctx, scOwner, s.name, heads[def], b.HeadSHA)
		if err != nil {
			return res, err
		}
		res.Commits[b.Name] = scSHAs(own)
		_, _ = p.LastRateLimit()
	}
	cmp, err := p.CompareCommits(ctx, scOwner, s.name, s.main[99], s.main[149])
	if err != nil {
		return res, err
	}
	res.Compare = scSHAs(cmp)
	res.Stats, err = p.CommitStats(ctx, scOwner, s.name, s.dev[29])
	return res, err
}

// TestScenarioConcurrentRepositories: 4 repositories synced in parallel with a single Provider, then
// a second sync (new Provider, same Cache and same Quota) entirely served as 304s, under
// -race.
func TestScenarioConcurrentRepositories(t *testing.T) {
	e := newScEnv(t)
	specs := make([]scRepoSpec, 4)
	for i := range specs {
		s := scRepoSpec{id: int64(3001 + i), name: fmt.Sprintf("projet-groupe-%d", i+1)}
		e.repo(s.id, s.name)
		s.main = e.srv.AppendCommits(s.id, "main", 150, e.t0.Add(time.Duration(i)*time.Hour), time.Minute)
		e.srv.SetBranch(s.id, "dev", s.main[120])
		s.dev = e.srv.AppendCommits(s.id, "dev", 30, e.t0.Add(10*24*time.Hour), time.Minute)
		specs[i] = s
	}

	round := func(p *Provider) []scSyncResult {
		results := make([]scSyncResult, len(specs))
		errs := make([]error, len(specs))
		var wg sync.WaitGroup
		for i, s := range specs {
			wg.Go(func() { results[i], errs[i] = scSync(t, p, s) })
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("sync of %s: %v", specs[i].name, err)
			}
		}
		return results
	}

	first := round(e.provider(testToken))
	for i, r := range first {
		s := specs[i]
		if len(r.Commits["main"]) != 150 || len(r.Commits["dev"]) != 30 || len(r.Compare) != 50 ||
			r.Repo.ID != s.id || r.Stats != (provider.CommitStats{}) {
			t.Errorf("%s: main %d, dev %d, compare %d commits, repository %d, stats %+v",
				s.name, len(r.Commits["main"]), len(r.Commits["dev"]), len(r.Compare), r.Repo.ID, r.Stats)
		}
		if len(r.Commits["main"]) == 150 && r.Commits["main"][0] != s.main[149] {
			t.Errorf("%s: first commit of main %s, want %s", s.name, provider.ShortSHA(r.Commits["main"][0]), provider.ShortSHA(s.main[149]))
		}
		if len(r.Commits["dev"]) == 30 && r.Commits["dev"][0] != s.dev[29] {
			t.Errorf("%s: first commit of dev %s, want %s", s.name, provider.ShortSHA(r.Commits["dev"][0]), provider.ShortSHA(s.dev[29]))
		}
	}

	// Second sync: everything is served from the shared cache, without quota, except the statistics
	// of a commit, never cached (the sync engine does not ask for them again anyway).
	remaining := e.srv.Quota().Remaining
	e.srv.ResetRequests()
	second := round(e.provider(testToken))
	if !reflect.DeepEqual(second, first) {
		t.Error("the second sync, served from the cache, differs from the first")
	}
	var cached, stats []githubtest.Request
	for _, r := range e.srv.Requests() {
		if strings.Contains(r.Path, "/commits/") {
			stats = append(stats, r)
		} else {
			cached = append(cached, r)
		}
	}
	// Per repository: repository, branches, 2 pages of main, compare of dev, compare, then the stats.
	scAll304(t, cached, 4*6)
	if len(stats) != 4 {
		t.Errorf("%d statistics requests, want 4", len(stats))
	}
	for _, r := range stats {
		if r.Status != http.StatusOK || r.IfNoneMatch != "" || r.Query.Get("per_page") != "1" {
			t.Errorf("%s ?%s: code %d, If-None-Match %q; want 200 without If-None-Match, per_page=1",
				r.Path, r.RawQuery, r.Status, r.IfNoneMatch)
		}
	}
	if got := e.srv.Quota().Remaining; got != remaining-len(stats) {
		t.Errorf("remaining quota %d after the second sync, want %d", got, remaining-len(stats))
	}
}
