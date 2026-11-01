// Package providertest provides a fake in-memory provider.Provider, for the tests of the sync
// engine (phase 3), the CLI, the web and the TUI. No network request, no server.
//
// The Fake reproduces the CONTRACT of provider.Provider (see internal/provider): validation of the
// arguments with provider.Validate* (ErrInvalidRef), owner/name compared case-insensitively,
// tracking of renamings, order of the commits, Since, Stop, Limit, MaxCommits, MaxBranches, empty
// repository, ErrUnknownBase, empty non-nil slices, independent copies (modifying a result does not
// modify the Fake). It only depends on internal/provider, internal/model and internal/i18n.
//
// Errors: like the GitHub Provider, the errors produced by the Fake itself (unknown repository, empty
// repository, exhausted quota…) are *provider.APIError {Op, Method: "GET", Path, Status} wrapping
// the sentinel error (Op and Path reuse the labels of the GitHub Provider, from the i18n catalog, for
// example "reading repository alice/projet" and "/repos/alice/projet"). Validation errors, ctx.Err()
// and injected errors (SetError, FailNext, SetRepoError) are returned as is.
//
// Dates: as on GitHub, they are in UTC and to the second; the dates received by AddRepository,
// UpdateRepository, AddCommits and AppendCommits are converted to UTC and truncated to the second.
//
// Safe for concurrent use. The setup methods (AddRepository, AddCommits…) panic on an inconsistent
// setup (unknown repository, unknown parent…): it is an error of the test.
//
// Order of the checks in every method of provider.Provider:
//  1. counting the call (Calls, CallLog);
//  2. ctx.Err() (context done): returned as is;
//  3. validation of the arguments: error wrapping provider.ErrInvalidRef;
//  4. injected error: first the FailNext queue for this method, then SetError;
//  5. error specific to the repository (SetRepoError), for the methods on a repository;
//  6. simulated quota (SetRateLimit): see SetRateLimit;
//  7. in-memory logic.
//
// Only exception, as for the GitHub Provider which then sends no request: CompareCommits with
// base == head returns an empty list right after step 3.
package providertest

import (
	"cmp"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// Default values of the simulated quota (like GitHub: 5,000 requests per hour).
const (
	defaultRateLimit = 5000
	rateWindow       = time.Hour
)

// HTTP codes of the errors produced by the Fake (those GitHub would return).
const (
	statusForbidden = 403
	statusNotFound  = 404
	statusConflict  = 409
)

// Method names a method of provider.Provider, for error injection and call counting.
type Method string

// Methods of provider.Provider (Name and LastRateLimit are neither counted nor injectable).
const (
	MethodRepository     Method = "Repository"
	MethodRepositoryByID Method = "RepositoryByID"
	MethodBranches       Method = "Branches"
	MethodListCommits    Method = "ListCommits"
	MethodCompareCommits Method = "CompareCommits"
	MethodCommitStats    Method = "CommitStats"
	MethodPullRequests   Method = "PullRequests"
	MethodReleases       Method = "Releases"
	MethodTags           Method = "Tags"
	MethodRateLimit      Method = "RateLimit"
)

// valid reports whether m is one of the methods above.
func (m Method) valid() bool {
	switch m {
	case MethodRepository, MethodRepositoryByID, MethodBranches, MethodListCommits, MethodCompareCommits,
		MethodCommitStats, MethodPullRequests, MethodReleases, MethodTags, MethodRateLimit:
		return true
	}
	return false
}

// Call is a call received by the Fake (CallLog). Only the fields useful to the method are filled.
type Call struct {
	Method Method
	Owner  string // owner as passed (all the methods on a named repository)
	Name   string // name as passed
	ID     int64  // RepositoryByID
	// Opts holds the options of ListCommits, except Stop (always nil here); StopSet reports whether a
	// Stop was given.
	Opts    provider.ListCommitsOptions
	StopSet bool
	Base    string // CompareCommits
	Head    string // CompareCommits
	SHA     string // CommitStats
	// PullOpts holds the options of PullRequests.
	PullOpts provider.PullRequestsOptions
}

// Fake is an in-memory provider.Provider. Build it with New.
type Fake struct {
	mu       sync.Mutex
	repos    map[int64]*fakeRepo
	names    map[string]int64 // nameKey (current or old name) → repository identifier
	errs     map[Method]error
	next     map[Method][]error
	repoErrs map[string]error // nameKey → error (SetRepoError)
	calls    map[Method]int
	log      []Call
	rate     provider.RateLimit // simulated quota (if rateOn)
	rateOn   bool
	now      func() time.Time
}

// fakeRepo is an in-memory repository.
type fakeRepo struct {
	meta     provider.Repository
	keys     []string // nameKey of the current name and of the old names still redirected
	commits  map[string]*fakeCommit
	seq      int               // number of commits added since creation (order of addition)
	branches map[string]string // name → head SHA
	stats    map[string]provider.CommitStats
	pulls    map[int]provider.PullRequest // number → pull request (SetPullRequests)
	releases []provider.Release           // SetReleases
	tags     []provider.Tag               // SetTags, in the given order
}

// fakeCommit is an in-memory commit and its rank of addition. It is never modified after
// AddCommits: it can therefore be read outside the lock once obtained under the lock.
type fakeCommit struct {
	c   provider.Commit
	seq int
}

// Checks at compile time that *Fake implements provider.Provider.
var _ provider.Provider = (*Fake)(nil)

// New returns an empty Fake: no repository, no simulated quota, clock time.Now.
func New() *Fake {
	return &Fake{
		repos:    make(map[int64]*fakeRepo),
		names:    make(map[string]int64),
		errs:     make(map[Method]error),
		next:     make(map[Method][]error),
		repoErrs: make(map[string]error),
		calls:    make(map[Method]int),
		now:      time.Now,
	}
}

// nameKey returns the key of a repository: "owner/name" in lowercase (GitHub ignores case).
func nameKey(owner, name string) string {
	return strings.ToLower(owner + "/" + name)
}

// utcSecond converts t to UTC to the second; the zero value stays zero.
func utcSecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC().Truncate(time.Second)
}

// clock returns the Fake's time, in UTC. Called with f.mu locked.
func (f *Fake) clock() time.Time { return f.now().UTC() }

// ----- Setup -----

// AddRepository adds a repository (unique ID > 0; valid Owner and Name, unique case-insensitively;
// empty DefaultBranch = "main"). The repository has neither branch nor commit: it is empty. An old
// name of a renamed repository can be taken again: it then designates the new repository, as on GitHub.
func (f *Fake) AddRepository(r provider.Repository) {
	if err := provider.ValidateID(r.ID); err != nil {
		panic("providertest.AddRepository: " + err.Error())
	}
	if err := provider.ValidateRepo(r.Owner, r.Name); err != nil {
		panic("providertest.AddRepository: " + err.Error())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.repos[r.ID]; ok {
		panic(fmt.Sprintf("providertest.AddRepository: identifier %d already taken", r.ID))
	}
	key := nameKey(r.Owner, r.Name)
	f.claimName(key, r.ID, "providertest.AddRepository")
	repo := &fakeRepo{
		meta:     normalizeRepo(r),
		keys:     []string{key},
		commits:  make(map[string]*fakeCommit),
		branches: make(map[string]string),
		stats:    make(map[string]provider.CommitStats),
		pulls:    make(map[int]provider.PullRequest),
	}
	f.repos[r.ID] = repo
	f.names[key] = r.ID
}

// normalizeRepo applies the default values and the date conventions.
func normalizeRepo(r provider.Repository) provider.Repository {
	if r.DefaultBranch == "" {
		r.DefaultBranch = "main"
	}
	r.PushedAt = utcSecond(r.PushedAt)
	return r
}

// claimName reserves key for the repository id: panics if key is the CURRENT name of another
// repository; an old name of another repository is taken from it. Called with f.mu locked.
func (f *Fake) claimName(key string, id int64, op string) {
	other, ok := f.names[key]
	if !ok || other == id {
		return
	}
	o := f.repos[other]
	if nameKey(o.meta.Owner, o.meta.Name) == key {
		panic(fmt.Sprintf("%s: repository %s already present (identifier %d)", op, o.meta.FullName(), other))
	}
	o.keys = slices.DeleteFunc(o.keys, func(k string) bool { return k == key })
	delete(f.names, key)
}

// UpdateRepository replaces the metadata of the repository r.ID (which must exist; empty
// DefaultBranch = "main"). If Owner or Name change, it is a renaming or a transfer: Repository(old
// owner, old name) and the other methods called with the old name still find the repository (like
// GitHub's 301 redirection) and return the current values.
func (f *Fake) UpdateRepository(r provider.Repository) {
	if err := provider.ValidateRepo(r.Owner, r.Name); err != nil {
		panic("providertest.UpdateRepository: " + err.Error())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.UpdateRepository", r.ID)
	key := nameKey(r.Owner, r.Name)
	f.claimName(key, r.ID, "providertest.UpdateRepository")
	if !slices.Contains(repo.keys, key) {
		repo.keys = append(repo.keys, key)
	}
	f.names[key] = r.ID
	repo.meta = normalizeRepo(r)
}

// RemoveRepository removes the repository id: all the methods then answer ErrNotFound for it, as
// for a private repository that became inaccessible. Its names (current and old) are released.
func (f *Fake) RemoveRepository(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.RemoveRepository", id)
	for _, k := range repo.keys {
		delete(f.names, k)
	}
	delete(f.repos, id)
}

// repoLocked returns the repository id; panics if it is unknown. Called with f.mu locked.
func (f *Fake) repoLocked(op string, id int64) *fakeRepo {
	repo, ok := f.repos[id]
	if !ok {
		panic(fmt.Sprintf("%s: unknown repository %d", op, id))
	}
	return repo
}

// AddCommits adds commits to the repository repoID, in order (every parent must already exist in the
// repository or appear earlier in the call). The commits are copied. Adding a commit moves no branch
// (see SetBranch). Invalid (provider.ValidateSHA) or already present SHA: panic.
func (f *Fake) AddCommits(repoID int64, commits ...provider.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.AddCommits", repoID)
	for _, c := range commits {
		f.addCommitLocked(repo, c)
	}
}

// addCommitLocked adds a copy of c to repo. Called with f.mu locked.
func (f *Fake) addCommitLocked(repo *fakeRepo, c provider.Commit) {
	if err := provider.ValidateSHA(c.SHA); err != nil {
		panic("providertest.AddCommits: " + err.Error())
	}
	if _, ok := repo.commits[c.SHA]; ok {
		panic(fmt.Sprintf("providertest.AddCommits: commit %s already present in repository %d", c.SHA, repo.meta.ID))
	}
	for _, p := range c.Parents {
		if _, ok := repo.commits[p]; !ok {
			panic(fmt.Sprintf("providertest.AddCommits: parent %s of commit %s unknown in repository %d",
				provider.ShortSHA(p), provider.ShortSHA(c.SHA), repo.meta.ID))
		}
	}
	c.AuthoredAt, c.CommittedAt = utcSecond(c.AuthoredAt), utcSecond(c.CommittedAt)
	c.Parents = append([]string{}, c.Parents...)
	repo.seq++
	repo.commits[c.SHA] = &fakeCommit{c: c, seq: repo.seq}
}

// AppendCommits adds n linear commits on top of the head of branch (or a new history if the branch
// does not exist), with deterministic SHAs (SHA of "<repoID>/<branch>/<k>", k = rank of the commit in
// the repository, increased if needed) distinct from those already present, dated start, start+step…
// (AuthoredAt = CommittedAt, UTC), with messages "commit <k>", URLs
// "https://github.com/<owner>/<name>/commit/<sha>", by the dummy author {Name: "Étudiant Test",
// Email: "etudiant@example.test", Login: "etudiant-test", ID: 1001, AvatarURL:
// "https://avatars.example.test/u/1001", ProfileURL: "https://github.example.test/etudiant-test"}
// (same Committer), then puts branch on the last one.
// Returns the created commits in creation order (from oldest to newest; empty non-nil slice if n is
// 0, without touching the branch). Negative n or invalid branch: panic.
func (f *Fake) AppendCommits(repoID int64, branch string, n int, start time.Time, step time.Duration) []provider.Commit {
	if err := provider.ValidateBranch(branch); err != nil {
		panic("providertest.AppendCommits: " + err.Error())
	}
	if n < 0 {
		panic(fmt.Sprintf("providertest.AppendCommits: negative number of commits %d", n))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.AppendCommits", repoID)
	author := provider.Person{
		Name:       "Étudiant Test",
		Email:      "etudiant@example.test",
		Login:      "etudiant-test",
		ID:         1001,
		AvatarURL:  "https://avatars.example.test/u/1001",
		ProfileURL: "https://github.example.test/etudiant-test",
	}
	parent := repo.branches[branch]
	out := make([]provider.Commit, 0, n)
	for i := range n {
		k := repo.seq + 1
		sha := SHA(fmt.Sprintf("%d/%s/%d", repoID, branch, k))
		for repo.commits[sha] != nil {
			k++
			sha = SHA(fmt.Sprintf("%d/%s/%d", repoID, branch, k))
		}
		repo.seq = k - 1 // addCommitLocked brings it to k
		at := start.Add(time.Duration(i) * step)
		c := provider.Commit{
			SHA:         sha,
			Message:     fmt.Sprintf("commit %d", k),
			URL:         fmt.Sprintf("https://github.com/%s/%s/commit/%s", repo.meta.Owner, repo.meta.Name, sha),
			Author:      author,
			Committer:   author,
			AuthoredAt:  at,
			CommittedAt: at,
		}
		if parent != "" {
			c.Parents = []string{parent}
		}
		f.addCommitLocked(repo, c)
		out = append(out, copyCommit(repo.commits[sha].c))
		parent = sha
	}
	if n > 0 {
		repo.branches[branch] = parent
	}
	return out
}

// SetBranch creates the branch name of the repository repoID or moves it (including backwards:
// force-push) to headSHA, which must exist in the repository (otherwise panic; invalid name too).
func (f *Fake) SetBranch(repoID int64, name, headSHA string) {
	if err := provider.ValidateBranch(name); err != nil {
		panic("providertest.SetBranch: " + err.Error())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetBranch", repoID)
	if _, ok := repo.commits[headSHA]; !ok {
		panic(fmt.Sprintf("providertest.SetBranch: unknown commit %s in repository %d", headSHA, repoID))
	}
	repo.branches[name] = headSHA
}

// DeleteBranch deletes the branch name of the repository repoID (the commits stay). Unknown branch:
// panic.
func (f *Fake) DeleteBranch(repoID int64, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.DeleteBranch", repoID)
	if _, ok := repo.branches[name]; !ok {
		panic(fmt.Sprintf("providertest.DeleteBranch: unknown branch %q in repository %d", name, repoID))
	}
	delete(repo.branches, name)
}

// RemoveCommit removes the commit sha from the repository repoID (cleanup after a force-push):
// CompareCommits with this commit as base then returns ErrUnknownBase. The commit must no longer be
// the head of a branch (otherwise panic); the commits referencing it as a parent keep it in Parents
// (the history walk stops there).
func (f *Fake) RemoveCommit(repoID int64, sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.RemoveCommit", repoID)
	if _, ok := repo.commits[sha]; !ok {
		panic(fmt.Sprintf("providertest.RemoveCommit: unknown commit %s in repository %d", sha, repoID))
	}
	for name, head := range repo.branches {
		if head == sha {
			panic(fmt.Sprintf("providertest.RemoveCommit: commit %s is the head of branch %q", sha, name))
		}
	}
	delete(repo.commits, sha)
	delete(repo.stats, sha)
}

// SetStats sets the statistics returned by CommitStats for the commit sha of the repository repoID
// (zero by default for a known commit). Unknown commit: panic.
func (f *Fake) SetStats(repoID int64, sha string, s provider.CommitStats) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetStats", repoID)
	if _, ok := repo.commits[sha]; !ok {
		panic(fmt.Sprintf("providertest.SetStats: unknown commit %s in repository %d", sha, repoID))
	}
	repo.stats[sha] = s
}

// SetPullRequests adds or replaces (by number) pull requests of the repository repoID. The dates are
// converted to UTC and truncated to the second. Number <= 0, state other than provider.PullRequestOpen
// or provider.PullRequestClosed, zero UpdatedAt or unknown repository: panic.
func (f *Fake) SetPullRequests(repoID int64, pulls ...provider.PullRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetPullRequests", repoID)
	for _, pr := range pulls {
		switch {
		case pr.Number <= 0:
			panic(fmt.Sprintf("providertest.SetPullRequests: invalid number %d", pr.Number))
		case pr.State != provider.PullRequestOpen && pr.State != provider.PullRequestClosed:
			panic(fmt.Sprintf("providertest.SetPullRequests: invalid state %q", pr.State))
		case pr.UpdatedAt.IsZero():
			panic(fmt.Sprintf("providertest.SetPullRequests: PR #%d without UpdatedAt", pr.Number))
		}
		pr.CreatedAt, pr.UpdatedAt = utcSecond(pr.CreatedAt), utcSecond(pr.UpdatedAt)
		pr.ClosedAt, pr.MergedAt = utcSecond(pr.ClosedAt), utcSecond(pr.MergedAt)
		repo.pulls[pr.Number] = pr
	}
}

// SetReleases replaces the releases of the repository repoID (dates in UTC to the second). Identifier
// <= 0, empty tag or unknown repository: panic.
func (f *Fake) SetReleases(repoID int64, releases ...provider.Release) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetReleases", repoID)
	out := make([]provider.Release, 0, len(releases))
	for _, r := range releases {
		if r.ID <= 0 || r.TagName == "" {
			panic(fmt.Sprintf("providertest.SetReleases: invalid release %d (tag %q)", r.ID, r.TagName))
		}
		r.CreatedAt, r.PublishedAt = utcSecond(r.CreatedAt), utcSecond(r.PublishedAt)
		out = append(out, r)
	}
	repo.releases = out
}

// SetTags replaces the tags of the repository repoID, returned by Tags in that order. Empty name,
// invalid SHA (provider.ValidateSHA) or unknown repository: panic (the target commit need not exist).
func (f *Fake) SetTags(repoID int64, tags ...provider.Tag) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetTags", repoID)
	for _, t := range tags {
		if t.Name == "" {
			panic("providertest.SetTags: tag without name")
		}
		if err := provider.ValidateSHA(t.SHA); err != nil {
			panic("providertest.SetTags: " + err.Error())
		}
	}
	repo.tags = slices.Clone(tags)
}

// SHA returns a dummy, deterministic SHA-1 (40 lowercase hex) derived from seed.
func SHA(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// ----- Injected errors -----

// SetError makes all the following calls to m fail with err (nil = restore the normal behavior).
// Unknown method: panic.
func (f *Fake) SetError(m Method, err error) {
	if !m.valid() {
		panic(fmt.Sprintf("providertest.SetError: unknown method %q", m))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.errs, m)
		return
	}
	f.errs[m] = err
}

// FailNext makes the next call to m fail with err; several FailNext calls chain in order (queue),
// before SetError. Unknown method or nil err: panic.
func (f *Fake) FailNext(m Method, err error) {
	if !m.valid() {
		panic(fmt.Sprintf("providertest.FailNext: unknown method %q", m))
	}
	if err == nil {
		panic("providertest.FailNext: nil error")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next[m] = append(f.next[m], err)
}

// SetRepoError makes all the calls on the repository owner/name fail (case-insensitively, current or
// old name), whatever the method (RepositoryByID included, through the repository's ID if it is
// known), with err; nil = restore. Example: a *provider.SSORequiredError for a single repository of
// an organization. The error can be set before the repository is added.
func (f *Fake) SetRepoError(owner, name string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := nameKey(owner, name)
	if err != nil {
		f.repoErrs[key] = err
		return
	}
	delete(f.repoErrs, key)
	if id, ok := f.names[key]; ok {
		for _, k := range f.repos[id].keys {
			delete(f.repoErrs, k)
		}
	}
}

// ----- Calls -----

// Calls returns the number of calls received by m (including those that failed).
func (f *Fake) Calls(m Method) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[m]
}

// CallLog returns a copy of the call log, in order.
func (f *Fake) CallLog() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.log)
}

// ResetCalls empties the log and the counters.
func (f *Fake) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = nil
	clear(f.calls)
}

// ----- Simulated quota -----

// SetRateLimit enables the simulated quota with the state rl (Reset in UTC, truncated to the second;
// zero = now + 1 h; zero or negative Limit = 5,000; Used = Limit - Remaining if Used is 0; negative
// Remaining: panic). From then on, every call to Repository, RepositoryByID, Branches,
// ListCommits, CompareCommits, CommitStats, PullRequests, Releases or Tags, after steps 1 to 5:
//   - if now >= Reset: new window (Remaining = Limit, Used = 0, Reset moved forward hour by hour
//     until it passes now);
//   - if Remaining == 0: error *provider.RateLimitError{Reset} (errors.Is ErrRateLimited), in a
//     *provider.APIError with code 403;
//   - otherwise: Remaining--, Used++, and the call goes on.
//
// LastRateLimit then returns the current state (ObservedAt = now) and true; RateLimit too, without
// consuming. Without SetRateLimit, LastRateLimit returns false (even after RateLimit) and RateLimit
// returns {Limit: 5000, Remaining: 5000, Reset: now + 1 h truncated to the second, ObservedAt:
// now}.
func (f *Fake) SetRateLimit(rl provider.RateLimit) {
	if rl.Remaining < 0 {
		panic(fmt.Sprintf("providertest.SetRateLimit: negative Remaining %d", rl.Remaining))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if rl.Limit <= 0 {
		rl.Limit = defaultRateLimit
	}
	if rl.Used == 0 {
		rl.Used = max(rl.Limit-rl.Remaining, 0)
	}
	if rl.Reset.IsZero() {
		rl.Reset = f.clock().Add(rateWindow)
	}
	rl.Reset = rl.Reset.UTC().Truncate(time.Second)
	rl.ObservedAt = time.Time{}
	f.rate, f.rateOn = rl, true
}

// SetClock replaces the Fake's clock (quota, ObservedAt); nil = time.Now. The clock is called under
// the Fake's lock.
func (f *Fake) SetClock(now func() time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if now == nil {
		now = time.Now
	}
	f.now = now
}

// renewRate opens a new quota window if the previous one has expired. Called with f.mu locked and
// the simulated quota active.
func (f *Fake) renewRate(now time.Time) {
	if now.Before(f.rate.Reset) {
		return
	}
	f.rate.Remaining, f.rate.Used = f.rate.Limit, 0
	windows := now.Sub(f.rate.Reset)/rateWindow + 1
	f.rate.Reset = f.rate.Reset.Add(windows * rateWindow)
}

// consume counts a call on the simulated quota (step 6). Called with f.mu locked.
func (f *Fake) consume(op, path string) error {
	if !f.rateOn {
		return nil
	}
	f.renewRate(f.clock())
	if f.rate.Remaining <= 0 {
		return apiError(op, path, statusForbidden, &provider.RateLimitError{Reset: f.rate.Reset})
	}
	f.rate.Remaining--
	f.rate.Used++
	return nil
}

// ----- Common steps -----

// begin counts the call (step 1) and returns ctx.Err() (step 2).
func (f *Fake) begin(ctx context.Context, c Call) error {
	f.mu.Lock()
	f.calls[c.Method]++
	f.log = append(f.log, c)
	f.mu.Unlock()
	return ctx.Err()
}

// injected returns the injected error for m (step 4). Called with f.mu locked.
func (f *Fake) injected(m Method) error {
	if q := f.next[m]; len(q) > 0 {
		err := q[0]
		if len(q) == 1 {
			delete(f.next, m)
		} else {
			f.next[m] = q[1:]
		}
		return err
	}
	return f.errs[m]
}

// repoErrByName returns the error set by SetRepoError for owner/name or for one of the names of the
// repository it designates (step 5). Called with f.mu locked.
func (f *Fake) repoErrByName(owner, name string) error {
	key := nameKey(owner, name)
	if err := f.repoErrs[key]; err != nil {
		return err
	}
	if id, ok := f.names[key]; ok {
		return f.repoErrByID(id)
	}
	return nil
}

// repoErrByID returns the error set by SetRepoError for one of the names of the repository id.
// Called with f.mu locked.
func (f *Fake) repoErrByID(id int64) error {
	repo, ok := f.repos[id]
	if !ok {
		return nil
	}
	for _, k := range repo.keys {
		if err := f.repoErrs[k]; err != nil {
			return err
		}
	}
	return nil
}

// prepare chains steps 4 to 6 for a call on owner/name. Called with f.mu locked.
func (f *Fake) prepare(m Method, owner, name, op, path string) error {
	if err := f.injected(m); err != nil {
		return err
	}
	if err := f.repoErrByName(owner, name); err != nil {
		return err
	}
	return f.consume(op, path)
}

// lookup returns the repository designated by owner/name (current or old name), or nil. Called with
// f.mu locked.
func (f *Fake) lookup(owner, name string) *fakeRepo {
	id, ok := f.names[nameKey(owner, name)]
	if !ok {
		return nil
	}
	return f.repos[id]
}

// apiError returns the error of a simulated GET request.
func apiError(op, path string, status int, err error) error {
	return &provider.APIError{Op: op, Method: "GET", Path: path, Status: status, Err: err}
}

// repoPath returns the API path of the repository owner/name, as passed.
func repoPath(owner, name string) string {
	return "/repos/" + owner + "/" + name
}

// copyCommit returns an independent copy of c (non-nil Parents).
func copyCommit(c provider.Commit) provider.Commit {
	c.Parents = append([]string{}, c.Parents...)
	return c
}

// ----- Commit graph -----

// resolve resolves ref to a SHA: branch name first, then full SHA of a present commit.
func (r *fakeRepo) resolve(ref string) (string, bool) {
	if sha, ok := r.branches[ref]; ok {
		return sha, true
	}
	if _, ok := r.commits[ref]; ok {
		return ref, true
	}
	return "", false
}

// reachable returns the set of SHAs reachable from head through the parents (head included),
// ignoring the removed commits (RemoveCommit).
func (r *fakeRepo) reachable(head string) map[string]bool {
	seen := make(map[string]bool)
	stack := []string{head}
	for len(stack) > 0 {
		sha := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c, ok := r.commits[sha]
		if !ok || seen[sha] {
			continue
		}
		seen[sha] = true
		stack = append(stack, c.c.Parents...)
	}
	return seen
}

// oldestFirst orders two commits from oldest to newest: CommittedAt, then order of addition.
func oldestFirst(a, b *fakeCommit) int {
	if c := a.c.CommittedAt.Compare(b.c.CommittedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.seq, b.seq)
}

// sorted returns the commits of set from oldest to newest (oldestFirst).
func (r *fakeRepo) sorted(set map[string]bool) []*fakeCommit {
	out := make([]*fakeCommit, 0, len(set))
	for sha := range set {
		out = append(out, r.commits[sha])
	}
	slices.SortFunc(out, oldestFirst)
	return out
}

// ----- provider.Provider -----

// Name implements provider.Provider: "github" (model.ProviderGitHub), since the Fake stands in for
// GitHub in the tests.
func (f *Fake) Name() string { return model.ProviderGitHub }

// Repository implements provider.Provider: repository found by its current name or an old name (the
// returned values are the current ones); unknown → error wrapping provider.ErrNotFound.
func (f *Fake) Repository(ctx context.Context, owner, name string) (provider.Repository, error) {
	if err := f.begin(ctx, Call{Method: MethodRepository, Owner: owner, Name: name}); err != nil {
		return provider.Repository{}, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.Repository{}, err
	}
	op, path := i18n.T("provider.op.repo", owner, name), repoPath(owner, name)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodRepository, owner, name, op, path); err != nil {
		return provider.Repository{}, err
	}
	repo := f.lookup(owner, name)
	if repo == nil {
		return provider.Repository{}, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	return repo.meta, nil
}

// RepositoryByID implements provider.Provider; unknown id → provider.ErrNotFound.
func (f *Fake) RepositoryByID(ctx context.Context, id int64) (provider.Repository, error) {
	if err := f.begin(ctx, Call{Method: MethodRepositoryByID, ID: id}); err != nil {
		return provider.Repository{}, err
	}
	if err := provider.ValidateID(id); err != nil {
		return provider.Repository{}, err
	}
	op, path := i18n.T("provider.github.op.repo_id", id), fmt.Sprintf("/repositories/%d", id)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.injected(MethodRepositoryByID); err != nil {
		return provider.Repository{}, err
	}
	if err := f.repoErrByID(id); err != nil {
		return provider.Repository{}, err
	}
	if err := f.consume(op, path); err != nil {
		return provider.Repository{}, err
	}
	repo, ok := f.repos[id]
	if !ok {
		return provider.Repository{}, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	return repo.meta, nil
}

// Branches implements provider.Provider: branches sorted by name, at most provider.MaxBranches;
// empty repository → empty non-nil list.
func (f *Fake) Branches(ctx context.Context, owner, name string) ([]provider.Branch, error) {
	if err := f.begin(ctx, Call{Method: MethodBranches, Owner: owner, Name: name}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := i18n.T("provider.github.op.branches", owner, name), repoPath(owner, name)+"/branches"
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodBranches, owner, name, op, path); err != nil {
		return nil, err
	}
	repo := f.lookup(owner, name)
	if repo == nil {
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	names := make([]string, 0, len(repo.branches))
	for n := range repo.branches {
		names = append(names, n)
	}
	slices.Sort(names)
	if len(names) > provider.MaxBranches {
		names = names[:provider.MaxBranches]
	}
	out := make([]provider.Branch, 0, len(names))
	for _, n := range names {
		out = append(out, provider.Branch{Name: n, HeadSHA: repo.branches[n]})
	}
	return out, nil
}

// ListCommits implements provider.Provider: commits reachable from opts.Branch (branch name or known
// full SHA; empty = DefaultBranch) through the parents, sorted by descending CommittedAt (at equal
// dates, the last added first), filtered by Since (CommittedAt >= Since truncated to the second,
// like the since sent to GitHub), cut at the first commit for which Stop returns true (excluded; Stop
// is called outside the Fake's lock, in order, and no more after the limit) and at the effective limit
// (Limit, at most provider.MaxCommits). Unknown branch → provider.ErrNotFound; empty repository (no
// branch) → empty non-nil list, without error.
func (f *Fake) ListCommits(ctx context.Context, owner, name string, opts provider.ListCommitsOptions) ([]provider.Commit, error) {
	logged := opts
	logged.Stop = nil
	if err := f.begin(ctx, Call{Method: MethodListCommits, Owner: owner, Name: name, Opts: logged, StopSet: opts.Stop != nil}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if opts.Branch != "" {
		if err := provider.ValidateBranch(opts.Branch); err != nil {
			return nil, err
		}
	}
	candidates, err := f.listCandidates(owner, name, opts)
	if err != nil {
		return nil, err
	}

	limit := opts.Limit
	if limit <= 0 || limit > provider.MaxCommits {
		limit = provider.MaxCommits
	}
	out := make([]provider.Commit, 0, min(limit, len(candidates)))
	for _, c := range candidates {
		if len(out) == limit || (opts.Stop != nil && opts.Stop(c.c.SHA)) {
			break
		}
		out = append(out, copyCommit(c.c))
	}
	return out, nil
}

// listCandidates applies steps 4 to 7 of ListCommits under the lock and returns the commits filtered
// by Since, from newest to oldest (before Stop and Limit).
func (f *Fake) listCandidates(owner, name string, opts provider.ListCommitsOptions) ([]*fakeCommit, error) {
	op, path := i18n.T("provider.github.op.commits", owner, name), repoPath(owner, name)+"/commits"
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodListCommits, owner, name, op, path); err != nil {
		return nil, err
	}
	repo := f.lookup(owner, name)
	if repo == nil {
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	if len(repo.branches) == 0 {
		return nil, nil // empty repository: empty list, without error
	}
	ref := opts.Branch
	if ref == "" {
		ref = repo.meta.DefaultBranch
	}
	head, ok := repo.resolve(ref)
	if !ok {
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	all := repo.sorted(repo.reachable(head))
	slices.Reverse(all)
	if opts.Since.IsZero() {
		return all, nil
	}
	// Like the GitHub Provider, which sends since in RFC 3339 format to the second: Since is truncated to
	// the second (the commit dates already are), and GitHub keeps the commits of that second.
	since := utcSecond(opts.Since)
	return slices.DeleteFunc(all, func(c *fakeCommit) bool { return c.c.CommittedAt.Before(since) }), nil
}

// CompareCommits implements provider.Provider: commits reachable from head but not from base, from
// newest to oldest (same order as ListCommits), at most provider.MaxCommits (like the GitHub
// Provider, which reads the pages from oldest to newest: beyond, the oldest are the ones kept);
// base == head (without any check after the validation, see the order of the checks) or head
// contained in base → empty non-nil list; empty repository (no branch) →
// provider.ErrEmptyRepository; unknown base or head, or no common ancestor between base and head
// (GitHub answers 404 in all three cases, and the GitHub Provider, which cannot tell them apart on an
// accessible repository, returns ErrUnknownBase) → provider.ErrUnknownBase (and not ErrNotFound);
// unknown repository → provider.ErrNotFound.
func (f *Fake) CompareCommits(ctx context.Context, owner, name, base, head string) ([]provider.Commit, error) {
	if err := f.begin(ctx, Call{Method: MethodCompareCommits, Owner: owner, Name: name, Base: base, Head: head}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if err := provider.ValidateSHA(base); err != nil {
		return nil, err
	}
	if err := provider.ValidateSHA(head); err != nil {
		return nil, err
	}
	if base == head {
		return []provider.Commit{}, nil
	}
	op := i18n.T("provider.github.op.compare", provider.ShortSHA(base), provider.ShortSHA(head), owner, name)
	path := repoPath(owner, name) + "/compare/" + base + "..." + head
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodCompareCommits, owner, name, op, path); err != nil {
		return nil, err
	}
	repo := f.lookup(owner, name)
	switch {
	case repo == nil:
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	case len(repo.branches) == 0:
		return nil, apiError(op, path, statusConflict, provider.ErrEmptyRepository)
	case repo.commits[base] == nil, repo.commits[head] == nil:
		return nil, apiError(op, path, statusNotFound, provider.ErrUnknownBase)
	}
	fromBase, fromHead := repo.reachable(base), repo.reachable(head)
	ahead := make(map[string]bool)
	common := false
	for sha := range fromHead {
		if fromBase[sha] {
			common = true
		} else {
			ahead[sha] = true
		}
	}
	if !common {
		return nil, apiError(op, path, statusNotFound, provider.ErrUnknownBase)
	}
	list := repo.sorted(ahead)
	if len(list) > provider.MaxCommits {
		list = list[:provider.MaxCommits]
	}
	out := make([]provider.Commit, len(list))
	for i, c := range list {
		out[len(list)-1-i] = copyCommit(c.c)
	}
	return out, nil
}

// CommitStats implements provider.Provider: statistics set by SetStats (zero by default); empty
// repository (no branch) → provider.ErrEmptyRepository; unknown commit → provider.ErrNotFound.
func (f *Fake) CommitStats(ctx context.Context, owner, name, sha string) (provider.CommitStats, error) {
	if err := f.begin(ctx, Call{Method: MethodCommitStats, Owner: owner, Name: name, SHA: sha}); err != nil {
		return provider.CommitStats{}, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.CommitStats{}, err
	}
	if err := provider.ValidateSHA(sha); err != nil {
		return provider.CommitStats{}, err
	}
	op := i18n.T("provider.github.op.commit_stats", provider.ShortSHA(sha), owner, name)
	path := repoPath(owner, name) + "/commits/" + sha
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodCommitStats, owner, name, op, path); err != nil {
		return provider.CommitStats{}, err
	}
	repo := f.lookup(owner, name)
	switch {
	case repo == nil:
		return provider.CommitStats{}, apiError(op, path, statusNotFound, provider.ErrNotFound)
	case len(repo.branches) == 0:
		return provider.CommitStats{}, apiError(op, path, statusConflict, provider.ErrEmptyRepository)
	case repo.commits[sha] == nil:
		return provider.CommitStats{}, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	return repo.stats[sha], nil
}

// PullRequests implements provider.Provider: pull requests set by SetPullRequests, sorted by
// descending UpdatedAt (at equal dates, descending number), cut at the first one updated strictly
// before opts.UpdatedSince truncated to the second (excluded) and at the effective limit (Limit, at
// most provider.MaxPullRequests); unknown repository → provider.ErrNotFound. An empty repository (no
// branch) returns its PRs like the others (GitHub answers 200).
func (f *Fake) PullRequests(ctx context.Context, owner, name string, opts provider.PullRequestsOptions) ([]provider.PullRequest, error) {
	if err := f.begin(ctx, Call{Method: MethodPullRequests, Owner: owner, Name: name, PullOpts: opts}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := i18n.T("provider.github.op.pulls", owner, name), repoPath(owner, name)+"/pulls"
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodPullRequests, owner, name, op, path); err != nil {
		return nil, err
	}
	repo := f.lookup(owner, name)
	if repo == nil {
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	all := make([]provider.PullRequest, 0, len(repo.pulls))
	for _, pr := range repo.pulls {
		all = append(all, pr)
	}
	slices.SortFunc(all, func(a, b provider.PullRequest) int {
		return cmp.Or(b.UpdatedAt.Compare(a.UpdatedAt), cmp.Compare(b.Number, a.Number))
	})
	limit := opts.Limit
	if limit <= 0 || limit > provider.MaxPullRequests {
		limit = provider.MaxPullRequests
	}
	since := utcSecond(opts.UpdatedSince)
	out := make([]provider.PullRequest, 0, min(limit, len(all)))
	for _, pr := range all {
		if len(out) == limit || (!since.IsZero() && pr.UpdatedAt.Before(since)) {
			break
		}
		out = append(out, pr)
	}
	return out, nil
}

// Releases implements provider.Provider: releases set by SetReleases, sorted by descending CreatedAt
// (at equal dates, descending identifier), at most provider.MaxReleases; unknown repository →
// provider.ErrNotFound.
func (f *Fake) Releases(ctx context.Context, owner, name string) ([]provider.Release, error) {
	if err := f.begin(ctx, Call{Method: MethodReleases, Owner: owner, Name: name}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := i18n.T("provider.github.op.releases", owner, name), repoPath(owner, name)+"/releases"
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodReleases, owner, name, op, path); err != nil {
		return nil, err
	}
	repo := f.lookup(owner, name)
	if repo == nil {
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	out := slices.Clone(repo.releases)
	if out == nil {
		out = []provider.Release{}
	}
	slices.SortStableFunc(out, func(a, b provider.Release) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
	if len(out) > provider.MaxReleases {
		out = out[:provider.MaxReleases]
	}
	return out, nil
}

// Tags implements provider.Provider: tags set by SetTags, in their order, at most
// provider.MaxTags; unknown repository → provider.ErrNotFound.
func (f *Fake) Tags(ctx context.Context, owner, name string) ([]provider.Tag, error) {
	if err := f.begin(ctx, Call{Method: MethodTags, Owner: owner, Name: name}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := i18n.T("provider.github.op.tags", owner, name), repoPath(owner, name)+"/tags"
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.prepare(MethodTags, owner, name, op, path); err != nil {
		return nil, err
	}
	repo := f.lookup(owner, name)
	if repo == nil {
		return nil, apiError(op, path, statusNotFound, provider.ErrNotFound)
	}
	out := slices.Clone(repo.tags)
	if out == nil {
		out = []provider.Tag{}
	}
	if len(out) > provider.MaxTags {
		out = out[:provider.MaxTags]
	}
	return out, nil
}

// RateLimit implements provider.Provider (see SetRateLimit); consumes no quota. Only steps 1, 2 and
// 4 apply.
func (f *Fake) RateLimit(ctx context.Context) (provider.RateLimit, error) {
	if err := f.begin(ctx, Call{Method: MethodRateLimit}); err != nil {
		return provider.RateLimit{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.injected(MethodRateLimit); err != nil {
		return provider.RateLimit{}, err
	}
	now := f.clock()
	if !f.rateOn {
		return provider.RateLimit{
			Limit:      defaultRateLimit,
			Remaining:  defaultRateLimit,
			Reset:      now.Add(rateWindow).Truncate(time.Second),
			ObservedAt: now,
		}, nil
	}
	return f.currentRate(now), nil
}

// LastRateLimit implements provider.Provider (see SetRateLimit); neither counted nor injectable.
func (f *Fake) LastRateLimit() (provider.RateLimit, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.rateOn {
		return provider.RateLimit{}, false
	}
	return f.currentRate(f.clock()), true
}

// currentRate returns the state of the simulated quota at now (window renewed if needed). Called with
// f.mu locked and the simulated quota active.
func (f *Fake) currentRate(now time.Time) provider.RateLimit {
	f.renewRate(now)
	rl := f.rate
	rl.ObservedAt = now
	return rl
}

// ProfileURL implements provider.Provider: "https://github.example.test/<login>" (same form as the
// ProfileURL of AppendCommits' dummy author), without counted call; empty if login does not have
// the form of a login (provider.ValidLogin).
func (f *Fake) ProfileURL(login string) string {
	if !provider.ValidLogin(login) {
		return ""
	}
	return "https://github.example.test/" + login
}
