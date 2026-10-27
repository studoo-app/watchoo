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

// Pagination de l'API GitHub.
const (
	defaultPerPage = 30
	maxPerPage     = 100
)

// Liens de documentation repris dans les corps d'erreur, comme le fait GitHub.
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

// reply est une réponse calculée sous le verrou du serveur, écrite ensuite.
type reply struct {
	status int
	header http.Header
	body   []byte
	delay  time.Duration
}

// serveHTTP est le gestionnaire du faux serveur (voir la documentation du package pour l'ordre des
// étapes).
func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.tb.Errorf("githubtest : requête %s %s reçue : Watchoo ne doit envoyer que des GET et des HEAD", r.Method, r.URL.Path)
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
		// Client parti ou serveur arrêté : la connexion est coupée sans réponse (et sans log).
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

// wait attend d pendant le traitement d'une requête ; false si le client a abandonné la requête ou si
// le serveur s'arrête entre-temps.
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

// respond journalise r et calcule sa réponse. Appelée avec s.mu verrouillé.
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

// validAuthorization indique si v vaut « Bearer <token> » ou « token <token> » (schéma sans tenir
// compte de la casse).
func (s *Server) validAuthorization(v string) bool {
	scheme, cred, ok := strings.Cut(v, " ")
	if !ok || !(strings.EqualFold(scheme, "Bearer") || strings.EqualFold(scheme, "token")) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cred), []byte(s.token)) == 1
}

// reply calcule la réponse à une requête GET ou HEAD (étapes 3 à 8). Appelée avec s.mu verrouillé.
func (s *Server) reply(r *http.Request, validToken bool) reply {
	now := s.clock()
	if !validToken {
		return s.jsonReply(now, http.StatusUnauthorized, errorJSON(messageBadCreds, docsREST, http.StatusUnauthorized))
	}
	s.renewQuota(now)
	path := r.URL.Path
	free := path == "/rate_limit"

	// 4. Réponse injectée.
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

	// 5. Limite secondaire.
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

	// 6. Quota épuisé.
	if s.quota.remaining <= 0 && now.Before(s.quota.reset) && !free {
		rep := s.jsonReply(now, s.quota.status, errorJSON(messagePrimary, docsPrimaryLimit, s.quota.status))
		s.setQuotaHeaders(rep.header)
		return rep
	}

	// 7. Routage, puis 8. ETag.
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

// renewQuota ouvre une nouvelle fenêtre de quota si la précédente est échue. Appelée avec s.mu
// verrouillé.
func (s *Server) renewQuota(now time.Time) {
	if now.Before(s.quota.reset) {
		return
	}
	s.quota.remaining, s.quota.used = s.quota.limit, 0
	s.quota.reset = now.Add(quotaWindow).Truncate(time.Second)
}

// consumeQuota décompte une requête. Appelée avec s.mu verrouillé.
func (s *Server) consumeQuota() {
	if s.quota.remaining > 0 {
		s.quota.remaining--
		s.quota.used++
	}
}

// setQuotaHeaders pose les en-têtes de quota de la ressource core. Appelée avec s.mu verrouillé.
func (s *Server) setQuotaHeaders(h http.Header) {
	h.Set("X-RateLimit-Limit", strconv.Itoa(s.quota.limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(s.quota.remaining))
	h.Set("X-RateLimit-Used", strconv.Itoa(s.quota.used))
	h.Set("X-RateLimit-Reset", strconv.FormatInt(s.quota.reset.Unix(), 10))
	h.Set("X-RateLimit-Resource", "core")
}

// newReply prépare une réponse avec les en-têtes communs à toutes les réponses de GitHub.
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

// jsonReply prépare une réponse dont le corps est v encodé en JSON.
func (s *Server) jsonReply(now time.Time, status int, v any) reply {
	return s.newReply(now, status, s.marshal(v))
}

// applyETag pose l'ETag faible d'une réponse 200, ou la transforme en 304 si l'un des If-None-Match
// reçus correspond.
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

// etagMatches indique si l'un des en-têtes If-None-Match (listes séparées par des virgules, ETag
// faibles ou forts, ou « * ») désigne tag (ETag fort, entre guillemets).
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

// ----- Routage -----

// route sert les routes de l'API simulée. Appelée avec s.mu verrouillé.
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

// notFound renvoie la réponse 404 de GitHub.
func (s *Server) notFound(now time.Time) reply {
	return s.jsonReply(now, http.StatusNotFound, errorJSON(messageNotFound, docsREST, http.StatusNotFound))
}

// denied renvoie la réponse d'un dépôt inaccessible (404 ou 403 SSO) et true, ou false si l'accès est
// accordé.
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

// redirect renvoie la redirection 301 d'un ancien nom vers /repositories/{id}/…, paramètres compris.
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

// repositoriesURL renvoie l'URL absolue URL()+"repositories/{id}" suivie du reste du chemin (segments
// déjà échappés).
func (s *Server) repositoriesURL(repo *repoState, escRest []string) string {
	u := s.URL() + "repositories/" + strconv.FormatInt(repo.ID, 10)
	if len(escRest) > 0 {
		u += "/" + strings.Join(escRest, "/")
	}
	return u
}

// repoRoute sert les routes d'un dépôt trouvé par son nom actuel ou son identifiant.
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
	case len(rest) == 2 && rest[0] == "compare" && rest[1] != "":
		if empty {
			return s.emptyRepo(now)
		}
		return s.compareReply(repo, rest[1], q, linkBase, now)
	}
	return s.notFound(now)
}

// emptyRepo renvoie la réponse 409 d'un dépôt sans commit.
func (s *Server) emptyRepo(now time.Time) reply {
	return s.jsonReply(now, http.StatusConflict, errorJSON(messageEmptyRepo, docsListCommits, http.StatusConflict))
}

// branchesReply sert GET …/branches.
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

// commitsReply sert GET …/commits.
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

// commitReply sert GET …/commits/{ref}.
func (s *Server) commitReply(repo *repoState, ref string, now time.Time) reply {
	sha, ok := resolveRef(repo, ref)
	if !ok {
		return s.jsonReply(now, http.StatusUnprocessableEntity, errorJSON("No commit found for SHA: "+ref, docsGetCommit, http.StatusUnprocessableEntity))
	}
	return s.jsonReply(now, http.StatusOK, s.commitJSON(repo, repo.commits[sha], true))
}

// compareReply sert GET …/compare/{base}...{head}.
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
	mergeBase := common[len(common)-1] // le plus récent des ancêtres communs
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

// parseQueryTime lit le paramètre de date name (RFC 3339) ; zéro s'il est absent, false s'il est
// illisible.
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

// ----- Graphe des commits -----

// resolveRef résout ref en SHA : nom de branche d'abord, puis SHA complet d'un commit présent.
func resolveRef(repo *repoState, ref string) (string, bool) {
	if sha, ok := repo.branches[ref]; ok {
		return sha, true
	}
	if _, ok := repo.commits[ref]; ok {
		return ref, true
	}
	return "", false
}

// reachable renvoie l'ensemble des SHA atteignables depuis head par les parents (head compris), en
// ignorant les parents retirés (RemoveCommit).
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

// compareCommits ordonne deux commits du plus ancien au plus récent : date de commit, puis ordre
// d'ajout.
func compareCommits(a, b *commitState) int {
	if c := a.CommittedAt.Compare(b.CommittedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.seq, b.seq)
}

// sortedCommits renvoie les commits de set triés du plus ancien au plus récent, ou l'inverse si
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

// page est une page d'une liste : éléments [start, end), numéro et dernière page.
type page struct {
	start, end int
	number     int
	last       int
}

// paginate découpe une liste de total éléments selon les paramètres per_page et page.
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

// setLink pose l'en-tête Link de la page p (format GitHub : prev, next, last, first), sauf pour une
// liste tenant sur la première page.
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
