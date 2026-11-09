package githubtest

import (
	"cmp"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Pagination of the GitHub API.
const (
	defaultPerPage = 30
	maxPerPage     = 100
)

// Documentation links quoted in the error bodies, as GitHub does.
const (
	docsREST          = "https://docs.github.com/rest"
	docsRedirects     = "https://docs.github.com/v3/#http-redirects"
	docsPrimaryLimit  = "https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#about-the-primary-rate-limit"
	docsSecondary     = "https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#about-secondary-rate-limits"
	docsSAML          = "https://docs.github.com/articles/authenticating-to-a-github-organization-with-saml-single-sign-on/"
	docsListCommits   = "https://docs.github.com/rest/commits/commits#list-commits"
	docsGetCommit     = "https://docs.github.com/rest/commits/commits#get-a-commit"
	contentTypeJSON   = "application/json; charset=utf-8"
	messageSecondary  = "You have exceeded a secondary rate limit. Please wait a few minutes before you try again."
	messagePrimary    = "API rate limit exceeded for user ID 1."
	messageSAML       = "Resource protected by organization SAML enforcement. You must grant your OAuth token access to this organization."
	messageEmptyRepo  = "Git Repository is empty."
	messageNotFound   = "Not Found"
	messageBadCreds   = "Bad credentials"
	messageMoved      = "Moved Permanently"
	messageNoMethod   = "Method Not Allowed"
	messageValidation = "Validation Failed"
)

// reply is a response computed under the server's lock, written afterwards.
type reply struct {
	status int
	header http.Header
	body   []byte
	delay  time.Duration
}

// serveHTTP is the handler of the fake server (see the package documentation for the order of the
// steps).
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.tb.Errorf("githubtest: request %s %s received: Watchoo must only send GET and HEAD", r.Method, r.URL.Path)
		w.Header().Set("Allow", "GET, HEAD")
		w.Header().Set("Content-Type", contentTypeJSON)
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write(s.marshal(errorJSON(messageNoMethod, docsREST, http.StatusMethodNotAllowed)))
		return
	}

	s.mu.Lock()
	rep := s.respond(r)
	s.mu.Unlock()

	if rep.delay > 0 && !s.wait(r.Context(), rep.delay) {
		// Client gone or server stopped: the connection is cut without response (and without log).
		panic(http.ErrAbortHandler)
	}
	h := w.Header()
	for k, v := range rep.header {
		h[k] = v
	}
	w.WriteHeader(rep.status)
	if len(rep.body) > 0 {
		_, _ = w.Write(rep.body)
	}
}

// wait waits d while handling a request; false if the client abandoned the request or if the
// server stops in the meantime.
func (s *Server) wait(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-s.done:
		return false
	}
}

// respond logs r and computes its response. Called with s.mu locked.
func (s *Server) respond(r *http.Request) reply {
	auth := r.Header.Values("Authorization")
	entry := &Request{
		Method:           r.Method,
		Path:             r.URL.Path,
		RawQuery:         r.URL.RawQuery,
		Query:            r.URL.Query(),
		IfNoneMatch:      strings.Join(r.Header.Values("If-None-Match"), ", "),
		UserAgent:        r.UserAgent(),
		HasAuthorization: len(auth) > 0,
		ValidToken:       len(auth) == 1 && s.validAuthorization(auth[0]),
	}
	s.requests = append(s.requests, entry)
	rep := s.reply(r, entry.ValidToken)
	entry.Status = rep.status
	return rep
}

// validAuthorization reports whether v is "Bearer <token>" or "token <token>" (case-insensitive
// scheme).
func (s *Server) validAuthorization(v string) bool {
	scheme, cred, ok := strings.Cut(v, " ")
	if !ok || !(strings.EqualFold(scheme, "Bearer") || strings.EqualFold(scheme, "token")) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cred), []byte(s.token)) == 1
}

// reply computes the response to a GET or HEAD request (steps 3 to 8). Called with s.mu locked.
func (s *Server) reply(r *http.Request, validToken bool) reply {
	now := s.clock()
	if !validToken {
		return s.jsonReply(now, http.StatusUnauthorized, errorJSON(messageBadCreds, docsREST, http.StatusUnauthorized))
	}
	s.renewQuota(now)
	path := r.URL.Path
	free := path == "/rate_limit"

	// 4. Injected response.
	if inj, ok := s.injections[path]; ok {
		if inj.left > 0 {
			inj.left--
			if inj.left == 0 {
				delete(s.injections, path)
			}
		}
		rep := s.newReply(now, inj.resp.Status, []byte(inj.resp.Body))
		rep.delay = inj.resp.Delay
		if rep.status != http.StatusNotModified && !free {
			s.consumeQuota()
		}
		s.setQuotaHeaders(rep.header)
		for k, v := range inj.resp.Header {
			rep.header[http.CanonicalHeaderKey(k)] = append([]string(nil), v...)
		}
		return rep
	}

	// 5. Secondary limit.
	if sec := s.secondary; sec != nil && !free {
		sec.left--
		if sec.left <= 0 {
			s.secondary = nil
		}
		rep := s.newReply(now, sec.status, s.marshal(map[string]string{
			"message":           messageSecondary,
			"documentation_url": docsSecondary,
		}))
		if sec.retryAfter > 0 {
			secs := int64(math.Round(sec.retryAfter.Seconds()))
			rep.header.Set("Retry-After", strconv.FormatInt(max(secs, 1), 10))
		}
		s.setQuotaHeaders(rep.header)
		return rep
	}

	// 6. Exhausted quota.
	if s.quota.remaining <= 0 && now.Before(s.quota.reset) && !free {
		rep := s.jsonReply(now, s.quota.status, errorJSON(messagePrimary, docsPrimaryLimit, s.quota.status))
		s.setQuotaHeaders(rep.header)
		return rep
	}

	// 7. Routing, then 8. ETag.
	rep := s.route(r, now)
	if rep.status == http.StatusOK {
		applyETag(&rep, r.Header.Values("If-None-Match"))
	}
	if rep.status != http.StatusNotModified && !free {
		s.consumeQuota()
	}
	s.setQuotaHeaders(rep.header)
	return rep
}

// renewQuota opens a new quota window if the previous one has expired. Called with s.mu
// locked.
func (s *Server) renewQuota(now time.Time) {
	if now.Before(s.quota.reset) {
		return
	}
	s.quota.remaining, s.quota.used = s.quota.limit, 0
	s.quota.reset = now.Add(quotaWindow).Truncate(time.Second)
}

// consumeQuota counts one request. Called with s.mu locked.
func (s *Server) consumeQuota() {
	if s.quota.remaining > 0 {
		s.quota.remaining--
		s.quota.used++
	}
}

// setQuotaHeaders sets the quota headers of the core resource. Called with s.mu locked.
func (s *Server) setQuotaHeaders(h http.Header) {
	h.Set("X-RateLimit-Limit", strconv.Itoa(s.quota.limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(s.quota.remaining))
	h.Set("X-RateLimit-Used", strconv.Itoa(s.quota.used))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(s.quota.reset.Unix(), 10))
	h.Set("X-RateLimit-Resource", "core")
}

// newReply prepares a response with the headers common to all GitHub responses.
func (s *Server) newReply(now time.Time, status int, body []byte) reply {
	h := make(http.Header)
	h.Set("Date", now.Format(http.TimeFormat))
	h.Set("Vary", "Accept, Authorization, Cookie, X-GitHub-OTP")
	h.Set("X-GitHub-Media-Type", "github.v3; format=json")
	h.Set("X-GitHub-Api-Version-Selected", "2022-11-28")
	if len(body) > 0 {
		h.Set("Content-Type", contentTypeJSON)
	}
	return reply{status: status, header: h, body: body}
}

// jsonReply prepares a response whose body is v encoded as JSON.
func (s *Server) jsonReply(now time.Time, status int, v any) reply {
	return s.newReply(now, status, s.marshal(v))
}

// applyETag sets the weak ETag of a 200 response, or turns it into a 304 if one of the received
// If-None-Match matches.
func applyETag(rep *reply, ifNoneMatch []string) {
	sum := sha256.Sum256(rep.body)
	tag := `"` + hex.EncodeToString(sum[:]) + `"`
	if etagMatches(ifNoneMatch, tag) {
		rep.status = http.StatusNotModified
		rep.body = nil
		rep.header.Del("Content-Type")
		rep.header.Del("Link")
		rep.header.Set("ETag", tag)
		return
	}
	rep.header.Set("ETag", "W/"+tag)
}

// etagMatches reports whether one of the If-None-Match headers (comma-separated lists, weak or
// strong ETags, or "*") designates tag (strong ETag, in quotes).
func etagMatches(headers []string, tag string) bool {
	for _, h := range headers {
		for part := range strings.SplitSeq(h, ",") {
			p := strings.TrimSpace(part)
			if p == "*" || strings.TrimPrefix(p, "W/") == tag {
				return true
			}
		}
	}
	return false
}

// ----- Routing -----

// route serves the routes of the simulated API. Called with s.mu locked.
func (s *Server) route(r *http.Request, now time.Time) reply {
	esc := strings.Split(strings.TrimPrefix(r.URL.EscapedPath(), "/"), "/")
	segs := make([]string, len(esc))
	for i, e := range esc {
		u, err := url.PathUnescape(e)
		if err != nil {
			return s.notFound(now)
		}
		segs[i] = u
	}
	switch {
	case len(segs) == 1 && segs[0] == "rate_limit":
		return s.jsonReply(now, http.StatusOK, s.rateLimitJSON())
	case len(segs) >= 3 && segs[0] == "repos":
		key := nameKey(segs[1], segs[2])
		if id, ok := s.names[key]; ok {
			return s.repoRoute(s.repos[id], segs[3:], esc[3:], r, now)
		}
		if id, ok := s.redirects[key]; ok {
			repo := s.repos[id]
			if rep, denied := s.denied(repo, now); denied {
				return rep
			}
			return s.redirect(repo, esc[3:], r, now)
		}
	case len(segs) >= 2 && segs[0] == "repositories":
		id, err := strconv.ParseInt(segs[1], 10, 64)
		if repo, ok := s.repos[id]; ok && err == nil {
			return s.repoRoute(repo, segs[2:], esc[2:], r, now)
		}
	}
	return s.notFound(now)
}

// notFound returns GitHub's 404 response.
func (s *Server) notFound(now time.Time) reply {
	return s.jsonReply(now, http.StatusNotFound, errorJSON(messageNotFound, docsREST, http.StatusNotFound))
}

// denied returns the response of an inaccessible repository (404 or 403 SSO) and true, or false if
// access is granted.
func (s *Server) denied(repo *repoState, now time.Time) (reply, bool) {
	switch repo.access {
	case AccessHidden:
		return s.notFound(now), true
	case AccessSSO:
		rep := s.jsonReply(now, http.StatusForbidden, errorJSON(messageSAML, docsSAML, http.StatusForbidden))
		rep.header.Set("X-GitHub-SSO", "required; url=https://github.com/orgs/"+url.PathEscape(repo.Owner)+"/sso?authorization_request=fake")
		return rep, true
	}
	return reply{}, false
}

// redirect returns the 301 redirection of an old name to /repositories/{id}/…, parameters included.
func (s *Server) redirect(repo *repoState, escRest []string, r *http.Request, now time.Time) reply {
	loc := s.repositoriesURL(repo, escRest)
	if r.URL.RawQuery != "" {
		loc += "?" + r.URL.RawQuery
	}
	rep := s.jsonReply(now, http.StatusMovedPermanently, map[string]string{
		"message":           messageMoved,
		"url":               loc,
		"documentation_url": docsRedirects,
	})
	rep.header.Set("Location", loc)
	return rep
}

// repositoriesURL returns the absolute URL URL()+"repositories/{id}" followed by the rest of the path
// (segments already escaped).
func (s *Server) repositoriesURL(repo *repoState, escRest []string) string {
	u := s.URL() + "repositories/" + strconv.FormatInt(repo.ID, 10)
	if len(escRest) > 0 {
		u += "/" + strings.Join(escRest, "/")
	}
	return u
}

// repoRoute serves the routes of a repository found by its current name or its identifier.
func (s *Server) repoRoute(repo *repoState, rest, escRest []string, r *http.Request, now time.Time) reply {
	if rep, denied := s.denied(repo, now); denied {
		return rep
	}
	q := r.URL.Query()
	linkBase := s.repositoriesURL(repo, escRest)
	empty := len(repo.branches) == 0
	switch {
	case len(rest) == 0:
		return s.jsonReply(now, http.StatusOK, s.repoJSON(repo))
	case len(rest) == 1 && rest[0] == "branches":
		return s.branchesReply(repo, q, linkBase, now)
	case len(rest) == 1 && rest[0] == "commits":
		if empty {
			return s.emptyRepo(now)
		}
		return s.commitsReply(repo, q, linkBase, now)
	case len(rest) == 2 && rest[0] == "commits" && rest[1] != "":
		if empty {
			return s.emptyRepo(now)
		}
		return s.commitReply(repo, rest[1], now)
	case len(rest) == 1 && rest[0] == "pulls":
		return s.pullsReply(repo, q, linkBase, now)
	case len(rest) == 3 && rest[0] == "pulls" && rest[2] == "reviews":
		return s.reviewsReply(repo, rest[1], q, linkBase, now)
	case len(rest) == 1 && rest[0] == "releases":
		return s.releasesReply(repo, q, linkBase, now)
	case len(rest) == 1 && rest[0] == "tags":
		return s.tagsReply(repo, q, linkBase, now)
	case len(rest) == 2 && rest[0] == "compare" && rest[1] != "":
		if empty {
			return s.emptyRepo(now)
		}
		return s.compareReply(repo, rest[1], q, linkBase, now)
	}
	return s.notFound(now)
}

// emptyRepo returns the 409 response of a repository without commit.
func (s *Server) emptyRepo(now time.Time) reply {
	return s.jsonReply(now, http.StatusConflict, errorJSON(messageEmptyRepo, docsListCommits, http.StatusConflict))
}

// branchesReply serves GET …/branches.
func (s *Server) branchesReply(repo *repoState, q url.Values, linkBase string, now time.Time) reply {
	names := make([]string, 0, len(repo.branches))
	for name := range repo.branches {
		names = append(names, name)
	}
	slices.Sort(names)
	p := paginate(len(names), q)
	items := make([]branchJSON, 0, p.end-p.start)
	for _, name := range names[p.start:p.end] {
		items = append(items, s.branchJSON(repo, name))
	}
	rep := s.jsonReply(now, http.StatusOK, items)
	setLink(rep.header, linkBase, q, p)
	return rep
}

// commitsReply serves GET …/commits.
func (s *Server) commitsReply(repo *repoState, q url.Values, linkBase string, now time.Time) reply {
	ref := q.Get("sha")
	if ref == "" {
		ref = repo.DefaultBranch
	}
	head, ok := resolveRef(repo, ref)
	if !ok {
		return s.notFound(now)
	}
	since, okSince := parseQueryTime(q, "since")
	until, okUntil := parseQueryTime(q, "until")
	if !okSince || !okUntil {
		return s.jsonReply(now, http.StatusUnprocessableEntity, errorJSON(messageValidation, docsListCommits, http.StatusUnprocessableEntity))
	}
	all := sortedCommits(repo, reachable(repo, head), true)
	list := all[:0:0]
	for _, c := range all {
		if (!since.IsZero() && c.CommittedAt.Before(since)) || (!until.IsZero() && c.CommittedAt.After(until)) {
			continue
		}
		list = append(list, c)
	}
	p := paginate(len(list), q)
	items := make([]commitJSON, 0, p.end-p.start)
	for _, c := range list[p.start:p.end] {
		items = append(items, s.commitJSON(repo, c, false))
	}
	rep := s.jsonReply(now, http.StatusOK, items)
	setLink(rep.header, linkBase, q, p)
	return rep
}

// commitReply serves GET …/commits/{ref}.
func (s *Server) commitReply(repo *repoState, ref string, now time.Time) reply {
	sha, ok := resolveRef(repo, ref)
	if !ok {
		return s.jsonReply(now, http.StatusUnprocessableEntity, errorJSON("No commit found for SHA: "+ref, docsGetCommit, http.StatusUnprocessableEntity))
	}
	return s.jsonReply(now, http.StatusOK, s.commitJSON(repo, repo.commits[sha], true))
}

// compareReply serves GET …/compare/{base}...{head}.
func (s *Server) compareReply(repo *repoState, spec string, q url.Values, linkBase string, now time.Time) reply {
	baseRef, headRef, ok := strings.Cut(spec, "...")
	if !ok {
		return s.notFound(now)
	}
	base, okBase := resolveRef(repo, baseRef)
	head, okHead := resolveRef(repo, headRef)
	if !okBase || !okHead {
		return s.notFound(now)
	}
	fromBase, fromHead := reachable(repo, base), reachable(repo, head)
	var ahead, common []*commitState
	behind := 0
	for sha := range fromHead {
		if fromBase[sha] {
			common = append(common, repo.commits[sha])
		} else {
			ahead = append(ahead, repo.commits[sha])
		}
	}
	for sha := range fromBase {
		if !fromHead[sha] {
			behind++
		}
	}
	if len(common) == 0 {
		return s.notFound(now)
	}
	slices.SortFunc(common, compareCommits)
	mergeBase := common[len(common)-1] // the most recent of the common ancestors
	slices.SortFunc(ahead, compareCommits)

	status := "identical"
	switch {
	case len(ahead) > 0 && behind > 0:
		status = "diverged"
	case len(ahead) > 0:
		status = "ahead"
	case behind > 0:
		status = "behind"
	}
	p := paginate(len(ahead), q)
	commits := make([]commitJSON, 0, p.end-p.start)
	for _, c := range ahead[p.start:p.end] {
		commits = append(commits, s.commitJSON(repo, c, false))
	}
	rep := s.jsonReply(now, http.StatusOK, s.compareJSON(repo, spec, repo.commits[base], repo.commits[head],
		mergeBase, status, len(ahead), behind, commits))
	setLink(rep.header, linkBase, q, p)
	return rep
}

// parseQueryTime reads the date parameter name (RFC 3339); zero if it is missing, false if it is
// unreadable.
func parseQueryTime(q url.Values, name string) (time.Time, bool) {
	v := q.Get(name)
	if v == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// ----- Commit graph -----

// resolveRef resolves ref to a SHA: branch name first, then full SHA of a present commit.
func resolveRef(repo *repoState, ref string) (string, bool) {
	if sha, ok := repo.branches[ref]; ok {
		return sha, true
	}
	if _, ok := repo.commits[ref]; ok {
		return ref, true
	}
	return "", false
}

// reachable returns the set of SHAs reachable from head through the parents (head included),
// ignoring the removed parents (RemoveCommit).
func reachable(repo *repoState, head string) map[string]bool {
	seen := make(map[string]bool)
	stack := []string{head}
	for len(stack) > 0 {
		sha := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		c, ok := repo.commits[sha]
		if !ok || seen[sha] {
			continue
		}
		seen[sha] = true
		stack = append(stack, c.Parents...)
	}
	return seen
}

// compareCommits orders two commits from oldest to newest: commit date, then order of
// addition.
func compareCommits(a, b *commitState) int {
	if c := a.CommittedAt.Compare(b.CommittedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.seq, b.seq)
}

// sortedCommits returns the commits of set sorted from oldest to newest, or the reverse if
// newestFirst.
func sortedCommits(repo *repoState, set map[string]bool, newestFirst bool) []*commitState {
	out := make([]*commitState, 0, len(set))
	for sha := range set {
		out = append(out, repo.commits[sha])
	}
	slices.SortFunc(out, compareCommits)
	if newestFirst {
		slices.Reverse(out)
	}
	return out
}

// ----- Pagination -----

// page is a page of a list: elements [start, end), number and last page.
type page struct {
	start, end int
	number     int
	last       int
}

// paginate cuts a list of total elements according to the per_page and page parameters.
func paginate(total int, q url.Values) page {
	perPage := defaultPerPage
	if v, err := strconv.Atoi(q.Get("per_page")); err == nil && v > 0 {
		perPage = min(v, maxPerPage)
	}
	number := 1
	if v, err := strconv.Atoi(q.Get("page")); err == nil && v > 0 {
		number = v
	}
	last := max(1, (total+perPage-1)/perPage)
	p := page{start: total, end: total, number: number, last: last}
	if number <= last {
		p.start = (number - 1) * perPage
		p.end = min(p.start+perPage, total)
	}
	return p
}

// setLink sets the Link header of the page p (GitHub format: prev, next, last, first), except for a
// list fitting on the first page.
func setLink(h http.Header, base string, q url.Values, p page) {
	if p.last <= 1 && p.number <= 1 {
		return
	}
	var parts []string
	add := func(n int, rel string) {
		v := cloneValues(q)
		if v == nil {
			v = url.Values{}
		}
		v.Set("page", strconv.Itoa(n))
		parts = append(parts, fmt.Sprintf(`<%s?%s>; rel="%s"`, base, v.Encode(), rel))
	}
	if p.number > 1 {
		add(p.number-1, "prev")
	}
	if p.number < p.last {
		add(p.number+1, "next")
		add(p.last, "last")
	}
	if p.number > 1 {
		add(1, "first")
	}
	h.Set("Link", strings.Join(parts, ", "))
}
