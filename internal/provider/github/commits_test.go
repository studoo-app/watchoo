package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// SHAs of the dummy commits of testdata/ (commits_page1.json then commits_page2.json: c1 to c6, from
// newest to oldest; compare_page*.json: m1 to m5 since apiBaseSHA, from oldest to newest).
const (
	apiC1      = "0a934d3dea406ccab14c373d35d84c7b619d0e01" // merge of feature/login, committer web-flow
	apiC2      = "eca00f571cb2fc3b2a03707ea2929210e78596ac" // author without a GitHub account (author: null)
	apiC3      = "aff84faa7810a6905be3df348543504a3a261e84" // author and committer with an account
	apiC4      = "6ea33ad4236584499f2ebc29eb5cc14053030a70"
	apiC5      = "42747f4612f42fc6f3a3b6da1b21400f5fe7064a"
	apiC6      = "5f5ed3e6686167f5006df913235f1dc5fb885118" // root commit
	apiFeatSHA = "e7de2cd0585ecc3729db8117ba1e93e306e4866b" // head of feature/login
	apiBaseSHA = "747ca7ddcfcb37cc02e8b111fca5d2a0e70a30b3"
	apiM1      = "cbca9fded95d2f646db00358ef41ec585968d204"
	apiM2      = "ca99255ce8a56c4da42e560682f23b5f8450d66d"
	apiM3      = "15365ecdab7855913dbc916e7910cc9e7c135b40"
	apiM4      = "50c58cd04f5c4c02afb376a3895a3ed20d246fff"
	apiM5      = "df4cb7bcb3854c12971d7f4aca5a65ed91ee971e" // merge (2 parents)
)

// apiCommitsPath and apiComparePath are the API paths of the dummy commits and comparison.
const (
	apiCommitsPath = apiRepoPath + "/commits"
	apiComparePath = apiRepoPath + "/compare/" + apiBaseSHA + "..." + apiM5
)

// apiPersonal lists the dummy personal data of testdata/: it must appear neither in the logs nor in
// the error messages.
var apiPersonal = []string{
	"Alice Martin", "alice.martin@example.com", "alice-martin-dev", "Bob Durand", "bob.durand@example.org",
	"Carla Petit", "carla-petit", "Corrige le calcul de la moyenne", "Ajoute la page de connexion",
	"Merge pull request", "feature/login",
}

// apiCommitsServer serves the two pages of commits of main.
func apiCommitsServer(t *testing.T) *apiServer {
	t.Helper()
	s := apiNewServer(t)
	s.handle(apiCommitsPath, apiPaged(t, s, "/repositories/424242/commits", "[]",
		"commits_page1.json", "commits_page2.json"))
	return s
}

// apiSHAs returns the SHAs of commits.
func apiSHAs(commits []provider.Commit) []string {
	shas := make([]string, len(commits))
	for i, c := range commits {
		shas[i] = c.SHA
	}
	return shas
}

func TestListCommits(t *testing.T) {
	s := apiCommitsServer(t)
	p := apiProvider(t, s, Options{})

	commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if got, want := apiSHAs(commits), []string{apiC1, apiC2, apiC3, apiC4, apiC5, apiC6}; !slices.Equal(got, want) {
		t.Fatalf("SHA = %v, want %v", got, want)
	}

	// Merge commit made on GitHub: two parents, committer "web-flow".
	merge := commits[0]
	wantMerge := provider.Commit{
		SHA:     apiC1,
		Message: "Merge pull request #3 from promo-2026/feature/login\n\nAjoute la page de connexion",
		URL:     "https://github.com/promo-2026/projet-alice/commit/" + apiC1,
		Author: provider.Person{
			Name: "Alice Martin", Email: "alice.martin@example.com", Login: "alice-martin-dev", ID: 51000001,
			AvatarURL:  "https://avatars.githubusercontent.com/u/51000001?v=4",
			ProfileURL: "https://github.com/alice-martin-dev",
		},
		Committer: provider.Person{
			Name: "GitHub", Email: "noreply@github.com", Login: "web-flow", ID: 19864447,
			AvatarURL:  "https://avatars.githubusercontent.com/u/19864447?v=4",
			ProfileURL: "https://github.com/web-flow",
		},
		AuthoredAt:  time.Date(2026, 10, 20, 14, 3, 12, 0, time.UTC),
		CommittedAt: time.Date(2026, 10, 20, 14, 3, 12, 0, time.UTC),
		Parents:     []string{apiC2, apiFeatSHA},
	}
	if fmt.Sprintf("%#v", merge) != fmt.Sprintf("%#v", wantMerge) {
		t.Errorf("merge commit =\n%#v\nwant\n%#v", merge, wantMerge)
	}

	// Author whose e-mail is linked to no GitHub account: author and committer are null.
	noAccount := commits[1]
	if noAccount.Author != (provider.Person{Name: "Bob Durand", Email: "bob.durand@example.org"}) {
		t.Errorf("author without account = %+v", noAccount.Author)
	}
	if noAccount.Committer != (provider.Person{Name: "Bob Durand", Email: "bob.durand@example.org"}) {
		t.Errorf("committer without account = %+v", noAccount.Committer)
	}

	// Distinct authoring and commit dates; GitHub noreply e-mail.
	c3 := commits[2]
	if !c3.AuthoredAt.Equal(time.Date(2026, 10, 18, 10, 15, 0, 0, time.UTC)) ||
		!c3.CommittedAt.Equal(time.Date(2026, 10, 18, 10, 20, 30, 0, time.UTC)) {
		t.Errorf("dates = %v / %v", c3.AuthoredAt, c3.CommittedAt)
	}
	if c3.Author.Login != "carla-petit" || c3.Author.ID != 51000003 ||
		c3.Author.Email != "51000003+carla-petit@users.noreply.github.com" {
		t.Errorf("author = %+v", c3.Author)
	}
	if !slices.Equal(c3.Parents, []string{apiC4}) {
		t.Errorf("parents = %v", c3.Parents)
	}

	// Root commit: Parents empty but not nil.
	root := commits[5]
	if root.Parents == nil || len(root.Parents) != 0 {
		t.Errorf("root commit: Parents = %#v, want an empty non-nil slice", root.Parents)
	}
	for _, c := range commits {
		if c.AuthoredAt.Location() != time.UTC || c.CommittedAt.Location() != time.UTC {
			t.Errorf("commit %s: dates not in UTC", provider.ShortSHA(c.SHA))
		}
	}

	reqs := s.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	for i, r := range reqs {
		if r.Query.Get("per_page") != "100" || r.Query.Get("page") != fmt.Sprint(i+1) {
			t.Errorf("request %d: parameters %q", i+1, r.RawQuery)
		}
		if r.Query.Has("sha") || r.Query.Has("since") {
			t.Errorf("request %d: sha and since must be omitted: %q", i+1, r.RawQuery)
		}
	}
}

// TestListCommitsQuery checks the sha (branch with "/", special characters) and since (converted to
// UTC) parameters.
func TestListCommitsQuery(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiCommitsPath, apiStatic(http.StatusOK, []byte("[]")))
	p := apiProvider(t, s, Options{})
	paris := time.FixedZone("UTC+2", 2*60*60)

	tests := []struct {
		opts      provider.ListCommitsOptions
		wantRaw   string // fragment expected in the raw query
		wantSHA   string
		wantSince string
	}{
		{provider.ListCommitsOptions{Branch: "feature/login"}, "sha=feature%2Flogin", "feature/login", ""},
		{provider.ListCommitsOptions{Branch: "équipe/été #1&x=y+z"}, "sha=%C3%A9quipe%2F%C3%A9t%C3%A9+%231%26x%3Dy%2Bz",
			"équipe/été #1&x=y+z", ""},
		{provider.ListCommitsOptions{Branch: apiC3}, "sha=" + apiC3, apiC3, ""},
		{
			provider.ListCommitsOptions{Branch: "main", Since: time.Date(2026, 10, 1, 0, 0, 0, 500, paris)},
			"since=2026-09-30T22%3A00%3A00Z", "main", "2026-09-30T22:00:00Z",
		},
	}
	for _, tt := range tests {
		commits, err := p.ListCommits(context.Background(), apiOwner, apiName, tt.opts)
		if err != nil || commits == nil || len(commits) != 0 {
			t.Errorf("ListCommits(%+v) = %#v, %v", tt.opts, commits, err)
			continue
		}
		reqs := s.requests()
		r := reqs[len(reqs)-1]
		if !strings.Contains(r.RawQuery, tt.wantRaw) {
			t.Errorf("query %q: want %q", r.RawQuery, tt.wantRaw)
		}
		if r.Query.Get("sha") != tt.wantSHA || r.Query.Get("since") != tt.wantSince {
			t.Errorf("sha = %q, since = %q; want %q, %q", r.Query.Get("sha"), r.Query.Get("since"), tt.wantSHA, tt.wantSince)
		}
		if r.Query.Has("x") || r.Query.Get("per_page") != "100" || r.Query.Get("page") != "1" {
			t.Errorf("unexpected parameters: %q", r.RawQuery)
		}
	}
}

// TestListCommitsStop checks the stop at the first known SHA: commit excluded, no next page.
func TestListCommitsStop(t *testing.T) {
	tests := []struct {
		known    string
		want     []string
		requests int
	}{
		{apiC5, []string{apiC1, apiC2, apiC3, apiC4}, 2}, // known SHA on page 2
		{apiC2, []string{apiC1}, 1},                      // known SHA on page 1: page 2 never requested
		{apiC1, []string{}, 1},                           // nothing new
		{apiC4, []string{apiC1, apiC2, apiC3}, 2},        // first commit of page 2
	}
	for _, tt := range tests {
		t.Run(provider.ShortSHA(tt.known), func(t *testing.T) {
			s := apiCommitsServer(t)
			p := apiProvider(t, s, Options{})
			var seen []string
			commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{
				Branch: "main",
				Stop: func(sha string) bool {
					seen = append(seen, sha)
					return sha == tt.known
				},
			})
			if err != nil {
				t.Fatalf("ListCommits: %v", err)
			}
			if commits == nil || !slices.Equal(apiSHAs(commits), tt.want) {
				t.Errorf("SHA = %v, want %v", apiSHAs(commits), tt.want)
			}
			if wantSeen := append(slices.Clone(tt.want), tt.known); !slices.Equal(seen, wantSeen) {
				t.Errorf("Stop called on %v, want %v (in order, then nothing more)", seen, wantSeen)
			}
			if n := s.count(apiCommitsPath); n != tt.requests {
				t.Errorf("%d requests, want %d", n, tt.requests)
			}
		})
	}
}

func TestListCommitsLimit(t *testing.T) {
	tests := []struct {
		limit    int
		want     int
		requests int
	}{
		{1, 1, 1}, {3, 3, 1}, {4, 4, 2}, {6, 6, 2}, {50, 6, 2}, {0, 6, 2}, {-1, 6, 2},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.limit), func(t *testing.T) {
			s := apiCommitsServer(t)
			p := apiProvider(t, s, Options{})
			commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{Limit: tt.limit})
			if err != nil {
				t.Fatalf("ListCommits: %v", err)
			}
			if len(commits) != tt.want {
				t.Errorf("%d commits, want %d", len(commits), tt.want)
			}
			if n := s.count(apiCommitsPath); n != tt.requests {
				t.Errorf("%d requests, want %d", n, tt.requests)
			}
			for _, r := range s.requests() {
				if r.Query.Get("per_page") != "100" {
					t.Errorf("per_page must always be 100: %q", r.RawQuery)
				}
			}
		})
	}
}

// apiCommitPage returns a page of n minimal commits, numbered from first.
func apiCommitPage(first, n int) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"sha":"%040x","commit":{"author":{"name":"Étudiant factice","email":"e@example.com",`+
			`"date":"2026-10-01T00:00:00Z"},"committer":{"name":"Étudiant factice","email":"e@example.com",`+
			`"date":"2026-10-01T00:00:00Z"},"message":"m"},"author":null,"committer":null,"parents":[{"sha":"%040x"}]}`,
			first+i+1, first+i+2)
	}
	b.WriteByte(']')
	return b.Bytes()
}

// TestListCommitsCap: a server that always announces a full next page is read up to the cap
// provider.MaxCommits; a Limit above the cap is brought back to it.
func TestListCommitsCap(t *testing.T) {
	for _, limit := range []int{0, provider.MaxCommits + 500} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiCommitsPath, func(w http.ResponseWriter, r *http.Request) {
				page := apiPage(r)
				h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/commits", page, page+1)}}
				apiWrite(w, http.StatusOK, apiCommitPage((page-1)*PerPage, PerPage), h)
			})
			p := apiProvider(t, s, Options{})
			commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{Limit: limit})
			if err != nil {
				t.Fatalf("ListCommits: %v", err)
			}
			if len(commits) != provider.MaxCommits {
				t.Errorf("%d commits, want the cap %d", len(commits), provider.MaxCommits)
			}
			if n := s.count(apiCommitsPath); n != provider.MaxCommits/PerPage {
				t.Errorf("%d requests, want %d", n, provider.MaxCommits/PerPage)
			}
		})
	}
}

// TestListCommitsEndlessPages: a server that endlessly announces pages of a single commit is stopped
// after maxCommitPages pages.
func TestListCommitsEndlessPages(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiCommitsPath, func(w http.ResponseWriter, r *http.Request) {
		page := apiPage(r)
		h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/commits", page, page+1)}}
		apiWrite(w, http.StatusOK, apiCommitPage(page, 1), h)
	})
	p := apiProvider(t, s, Options{})
	commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if n := s.count(apiCommitsPath); n != maxCommitPages || len(commits) != maxCommitPages {
		t.Errorf("%d requests, %d commits; want %d", n, len(commits), maxCommitPages)
	}
}

// TestListCommitsETag: a second sync receives 304s (without Link or body, like GitHub); the shared
// ETag cache serves them like the original 200s, Link header included: the pagination and the
// conversion give the same result, and the quota state follows the headers of the 304s.
func TestListCommitsETag(t *testing.T) {
	s := apiNewServer(t)
	bodies := [][]byte{apiFixture(t, "commits_page1.json"), apiFixture(t, "commits_page2.json")}
	s.handle(apiCommitsPath, func(w http.ResponseWriter, r *http.Request) {
		page := apiPage(r)
		if page > len(bodies) {
			t.Errorf("page %d requested", page)
			apiWrite(w, http.StatusOK, []byte("[]"), nil)
			return
		}
		etag := fmt.Sprintf(`W/"commits-page-%d"`, page)
		w.Header().Set("ETag", etag)
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Resource", "core")
		w.Header().Set("X-RateLimit-Reset", "1792000000")
		if r.Header.Get("If-None-Match") == etag {
			w.Header().Set("X-RateLimit-Remaining", "4000")
			w.Header().Set("X-RateLimit-Used", "1000")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("X-RateLimit-Remaining", fmt.Sprint(4100-page))
		w.Header().Set("X-RateLimit-Used", fmt.Sprint(900+page))
		h := http.Header{}
		if l := apiLink(s.srv.URL, "/repositories/424242/commits", page, len(bodies)); l != "" {
			h.Set("Link", l)
		}
		apiWrite(w, http.StatusOK, bodies[page-1], h)
	})
	cache, quota := NewCache(0, 0), NewQuota()

	first := apiProvider(t, s, Options{Cache: cache, Quota: quota})
	want, err := first.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{Branch: "main"})
	if err != nil || len(want) != 6 {
		t.Fatalf("first sync: %d commits, %v", len(want), err)
	}

	// Next sync: new Provider (token asked again from gh), same cache and same quota.
	next := apiProvider(t, s, Options{Cache: cache, Quota: quota})
	got, err := next.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("next sync: %v", err)
	}
	if fmt.Sprintf("%#v", got) != fmt.Sprintf("%#v", want) {
		t.Errorf("the commits served from the cache differ:\n%v\nwant\n%v", apiSHAs(got), apiSHAs(want))
	}
	reqs := s.requests()
	if len(reqs) != 4 {
		t.Fatalf("%d requests, want 4 (2 pages, twice)", len(reqs))
	}
	for i, r := range reqs[2:] {
		if wantTag := fmt.Sprintf(`W/"commits-page-%d"`, i+1); r.IfNone != wantTag {
			t.Errorf("page %d: If-None-Match = %q, want %q", i+1, r.IfNone, wantTag)
		}
	}
	if rl, ok := next.LastRateLimit(); !ok || rl.Remaining != 4000 {
		t.Errorf("LastRateLimit = %+v, %v; want the state of the 304s (4000 remaining)", rl, ok)
	}
}

// TestListCommitsEmptyRepository: a student's freshly created repository answers 409; the list is
// empty, without error.
func TestListCommitsEmptyRepository(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiCommitsPath, apiStatic(http.StatusConflict, apiFixture(t, "empty_repository.json")))
	p := apiProvider(t, s, Options{})
	commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{Branch: "main"})
	if err != nil || commits == nil || len(commits) != 0 {
		t.Errorf("ListCommits = %#v, %v; want an empty non-nil list, without error", commits, err)
	}
}

// TestListCommitsUnknownBranch: an unknown branch answers 404 → ErrNotFound.
func TestListCommitsUnknownBranch(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiCommitsPath, apiStatic(http.StatusNotFound, apiFixture(t, "not_found.json")))
	p := apiProvider(t, s, Options{})
	_, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{Branch: "supprimee"})
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("ListCommits: %v, want ErrNotFound", err)
	}
	apiAPIError(t, err, "listing commits of promo-2026/projet-alice", http.StatusNotFound)
	apiNoLeak(t, err, "supprimee")
}

func TestListCommitsUnexpected(t *testing.T) {
	for label, body := range map[string]string{
		"without SHA":            `[{"commit":{"message":"m"},"parents":[]}]`,
		"invalid SHA":            `[{"sha":"XYZ","commit":{"message":"m"},"parents":[]}]`,
		"parent without SHA":     `[{"sha":"` + apiC1 + `","commit":{"message":"m"},"parents":[{"url":"x"}]}]`,
		"null element":           `[null]`,
		"null parent":            `[{"sha":"` + apiC1 + `","commit":{"message":"m"},"parents":[null]}]`,
		"object instead of list": `{"message":"ok"}`,
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiCommitsPath, apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})
			_, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{})
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Errorf("ListCommits: %v, want ErrUnexpected", err)
			}
		})
	}
}

// TestCommitConversionEdgeCases: dates with a time offset converted to UTC, commit without a commit
// block, empty accounts ({}), missing parents.
func TestCommitConversionEdgeCases(t *testing.T) {
	body := `[
	  {"sha":"` + apiC1 + `","commit":{"author":{"name":"A","email":"a@example.com","date":"2026-10-20T16:03:12+02:00"},
	   "committer":{"name":"C","email":"c@example.com","date":"2026-10-20T09:00:00-05:00"},"message":"m"},
	   "author":{},"committer":{"login":"bot","id":7},"html_url":"https://github.com/x/y/commit/` + apiC1 + `"},
	  {"sha":"` + apiC2 + `"}
	]`
	s := apiNewServer(t)
	s.handle(apiCommitsPath, apiStatic(http.StatusOK, []byte(body)))
	p := apiProvider(t, s, Options{})
	commits, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{})
	if err != nil || len(commits) != 2 {
		t.Fatalf("ListCommits = %v, %v", commits, err)
	}
	c := commits[0]
	if !c.AuthoredAt.Equal(time.Date(2026, 10, 20, 14, 3, 12, 0, time.UTC)) || c.AuthoredAt.Location() != time.UTC ||
		!c.CommittedAt.Equal(time.Date(2026, 10, 20, 14, 0, 0, 0, time.UTC)) || c.CommittedAt.Location() != time.UTC {
		t.Errorf("dates = %v / %v, want UTC", c.AuthoredAt, c.CommittedAt)
	}
	if c.Author != (provider.Person{Name: "A", Email: "a@example.com"}) || c.Committer.Login != "bot" || c.Committer.ID != 7 {
		t.Errorf("people = %+v / %+v", c.Author, c.Committer)
	}
	bare := commits[1]
	if bare.Message != "" || !bare.AuthoredAt.IsZero() || !bare.CommittedAt.IsZero() || bare.Parents == nil ||
		bare.Author != (provider.Person{}) {
		t.Errorf("minimal commit = %#v", bare)
	}
}

func TestCompareCommits(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiComparePath, apiPaged(t, s, "/repositories/424242/compare/"+apiBaseSHA+"..."+apiM5,
		`{"status":"ahead","total_commits":5,"commits":[]}`, "compare_page1.json", "compare_page2.json"))
	p := apiProvider(t, s, Options{})

	commits, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
	if err != nil {
		t.Fatalf("CompareCommits: %v", err)
	}
	// The API gives them from oldest to newest; the result is from newest to oldest.
	if got, want := apiSHAs(commits), []string{apiM5, apiM4, apiM3, apiM2, apiM1}; !slices.Equal(got, want) {
		t.Fatalf("SHA = %v, want %v", got, want)
	}
	if !slices.Equal(commits[0].Parents, []string{apiM4, apiM2}) {
		t.Errorf("merge: parents = %v", commits[0].Parents)
	}
	if !slices.Equal(commits[4].Parents, []string{apiBaseSHA}) {
		t.Errorf("first commit: parents = %v", commits[4].Parents)
	}
	noAccount := commits[2]
	if noAccount.Author.Login != "" || noAccount.Author.ID != 0 || noAccount.Author.Name != "Bob Durand" {
		t.Errorf("author without account = %+v", noAccount.Author)
	}
	if !commits[0].CommittedAt.Equal(time.Date(2026, 10, 6, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("CommittedAt = %v", commits[0].CommittedAt)
	}

	reqs := s.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2: %+v", len(reqs), reqs)
	}
	for i, r := range reqs {
		if r.Path != apiComparePath || r.Query.Get("per_page") != "100" || r.Query.Get("page") != fmt.Sprint(i+1) {
			t.Errorf("request %d: %s?%s", i+1, r.Path, r.RawQuery)
		}
	}
}

func TestCompareCommitsSameSHA(t *testing.T) {
	s := apiNewServer(t)
	p := apiProvider(t, s, Options{})
	commits, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiM5, apiM5)
	if err != nil || commits == nil || len(commits) != 0 {
		t.Errorf("CompareCommits(base == head) = %#v, %v; want an empty non-nil list", commits, err)
	}
	if n := len(s.requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

// TestCompareCommitsNothingNew: head behind base (or identical) → empty non-nil list.
func TestCompareCommitsNothingNew(t *testing.T) {
	for label, body := range map[string][]byte{
		"behind":    apiFixture(t, "compare_behind.json"),
		"identical": []byte(`{"status":"identical","ahead_by":0,"behind_by":0,"total_commits":0,"commits":[],"files":[]}`),
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiComparePath, apiStatic(http.StatusOK, body))
			p := apiProvider(t, s, Options{})
			commits, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
			if err != nil || commits == nil || len(commits) != 0 {
				t.Errorf("CompareCommits = %#v, %v; want an empty non-nil list", commits, err)
			}
			if n := len(s.requests()); n != 1 {
				t.Errorf("%d requests, want 1", n)
			}
		})
	}
}

// apiComparePage returns a comparison page of n minimal commits, numbered from first.
func apiComparePage(total, first, n int) []byte {
	return fmt.Appendf(nil, `{"status":"ahead","ahead_by":%d,"behind_by":0,"total_commits":%d,"commits":%s}`,
		total, total, apiCommitPage(first, n))
}

// TestCompareCommitsPagination: stop at total_commits, next page requested if the Link header is
// missing while total_commits is not reached, cap provider.MaxCommits.
func TestCompareCommitsPagination(t *testing.T) {
	t.Run("without Link header", func(t *testing.T) {
		s := apiNewServer(t)
		s.handle(apiComparePath, func(w http.ResponseWriter, r *http.Request) {
			switch apiPage(r) {
			case 1:
				apiWrite(w, http.StatusOK, apiComparePage(150, 0, 100), nil)
			case 2:
				apiWrite(w, http.StatusOK, apiComparePage(150, 100, 50), nil)
			default:
				t.Errorf("page %d requested", apiPage(r))
				apiWrite(w, http.StatusOK, apiComparePage(150, 0, 0), nil)
			}
		})
		p := apiProvider(t, s, Options{})
		commits, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
		if err != nil {
			t.Fatalf("CompareCommits: %v", err)
		}
		if len(commits) != 150 || commits[0].SHA != fmt.Sprintf("%040x", 150) || commits[149].SHA != fmt.Sprintf("%040x", 1) {
			t.Errorf("%d commits, want them from newest to oldest", len(commits))
		}
		if n := len(s.requests()); n != 2 {
			t.Errorf("%d requests, want 2", n)
		}
	})
	t.Run("total reached despite a Link header", func(t *testing.T) {
		s := apiNewServer(t)
		s.handle(apiComparePath, func(w http.ResponseWriter, r *http.Request) {
			page := apiPage(r)
			h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/compare/x...y", page, page+1)}}
			apiWrite(w, http.StatusOK, apiComparePage(3, (page-1)*3, 3), h)
		})
		p := apiProvider(t, s, Options{})
		commits, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
		if err != nil || len(commits) != 3 {
			t.Fatalf("CompareCommits = %d commits, %v; want 3", len(commits), err)
		}
		if n := len(s.requests()); n != 1 {
			t.Errorf("%d requests, want 1", n)
		}
	})
	t.Run("cap", func(t *testing.T) {
		s := apiNewServer(t)
		s.handle(apiComparePath, func(w http.ResponseWriter, r *http.Request) {
			page := apiPage(r)
			h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/compare/x...y", page, page+1)}}
			apiWrite(w, http.StatusOK, apiComparePage(20000, (page-1)*PerPage, PerPage), h)
		})
		p := apiProvider(t, s, Options{})
		commits, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
		if err != nil {
			t.Fatalf("CompareCommits: %v", err)
		}
		if len(commits) != provider.MaxCommits {
			t.Errorf("%d commits, want the cap %d", len(commits), provider.MaxCommits)
		}
		if n := len(s.requests()); n != provider.MaxCommits/PerPage {
			t.Errorf("%d requests, want %d", n, provider.MaxCommits/PerPage)
		}
	})
}

// TestCompareCommitsUnknownBase: the compare answers 404 while the repository is accessible: the base
// no longer exists (force-push) → ErrUnknownBase, and NOT ErrNotFound.
func TestCompareCommitsUnknownBase(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiComparePath, apiStatic(http.StatusNotFound, apiFixture(t, "not_found.json")))
	s.handle(apiRepoPath, apiStatic(http.StatusOK, apiFixture(t, "repo.json")))
	p := apiProvider(t, s, Options{})

	_, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
	if !errors.Is(err, provider.ErrUnknownBase) {
		t.Fatalf("CompareCommits: %v, want ErrUnknownBase", err)
	}
	if errors.Is(err, provider.ErrNotFound) {
		t.Errorf("the error must not satisfy ErrNotFound: %v", err)
	}
	apiErr := apiAPIError(t, err, "comparing 747ca7d...df4cb7b in promo-2026/projet-alice", http.StatusNotFound)
	if apiErr.Path != apiComparePath || apiErr.Method != http.MethodGet {
		t.Errorf("Method, Path = %q, %q", apiErr.Method, apiErr.Path)
	}
	apiNoLeak(t, err, "docs.github.com", "Not Found")
	if s.count(apiRepoPath) != 1 {
		t.Errorf("the repository must be read again once to settle the 404")
	}
}

// TestCompareCommitsRepoNotFound: the compare and the repository answer 404 (private repository
// without access) → ErrNotFound, and not ErrUnknownBase.
func TestCompareCommitsRepoNotFound(t *testing.T) {
	s := apiNewServer(t)
	notFound := apiFixture(t, "not_found.json")
	s.handle(apiComparePath, apiStatic(http.StatusNotFound, notFound))
	s.handle(apiRepoPath, apiStatic(http.StatusNotFound, notFound))
	p := apiProvider(t, s, Options{})

	_, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
	if !errors.Is(err, provider.ErrNotFound) || errors.Is(err, provider.ErrUnknownBase) {
		t.Fatalf("CompareCommits: %v, want ErrNotFound without ErrUnknownBase", err)
	}
}

func TestCompareCommitsEmptyRepository(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiComparePath, apiStatic(http.StatusConflict, apiFixture(t, "empty_repository.json")))
	p := apiProvider(t, s, Options{})
	_, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
	if !errors.Is(err, provider.ErrEmptyRepository) {
		t.Fatalf("CompareCommits: %v, want ErrEmptyRepository", err)
	}
	apiAPIError(t, err, "comparing 747ca7d...df4cb7b in promo-2026/projet-alice", http.StatusConflict)
	if s.count(apiRepoPath) != 0 {
		t.Error("a 409 must not cause the repository to be read again")
	}
}

func TestCompareCommitsUnexpected(t *testing.T) {
	for label, body := range map[string]string{
		"null":          `null`,
		"broken commit": `{"status":"ahead","total_commits":1,"commits":[{"sha":"court"}]}`,
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiComparePath, apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})
			_, err := p.CompareCommits(context.Background(), apiOwner, apiName, apiBaseSHA, apiM5)
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Errorf("CompareCommits: %v, want ErrUnexpected", err)
			}
		})
	}
}

// TestCommitStats checks the conversion and the request: a single page of files (per_page=1, since
// only the totals are used: without this parameter, GitHub returns up to 300 files with their patch,
// that is the code of the private repository), and a response never cached, even with an ETag
// (every commit is requested only once).
func TestCommitStats(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiCommitsPath+"/"+apiC3, func(w http.ResponseWriter, r *http.Request) {
		apiWrite(w, http.StatusOK, apiFixture(t, "commit_stats.json"), http.Header{"Etag": {`"stats-c3"`}})
	})
	cache := NewCache(0, 0)
	p := apiProvider(t, s, Options{Cache: cache})

	for range 2 {
		stats, err := p.CommitStats(context.Background(), apiOwner, apiName, apiC3)
		if err != nil {
			t.Fatalf("CommitStats: %v", err)
		}
		if stats != (provider.CommitStats{Additions: 30, Deletions: 12}) {
			t.Errorf("CommitStats = %+v, want 30 / 12", stats)
		}
	}
	reqs := s.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requests, want 2", len(reqs))
	}
	for i, r := range reqs {
		if r.RawQuery != "per_page=1" {
			t.Errorf("request %d: parameters %q, want \"per_page=1\" (a single page of files)", i+1, r.RawQuery)
		}
		if r.IfNone != "" {
			t.Errorf("request %d: If-None-Match %q, want none (response never cached)", i+1, r.IfNone)
		}
	}
	if n := cache.Len(); n != 0 {
		t.Errorf("cache: %d entry(ies), want 0 (a commit's statistics are not cached)", n)
	}
}

func TestCommitStatsUnexpected(t *testing.T) {
	for label, body := range map[string]string{
		"null":          `null`,
		"without stats": `{"sha":"` + apiC3 + `"}`,
		"stats null":    `{"sha":"` + apiC3 + `","stats":null}`,
		"partial stats": `{"sha":"` + apiC3 + `","stats":{"total":3,"additions":3}}`,
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiCommitsPath+"/"+apiC3, apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})
			_, err := p.CommitStats(context.Background(), apiOwner, apiName, apiC3)
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Fatalf("CommitStats: %v, want ErrUnexpected", err)
			}
			apiAPIError(t, err, "statistics of commit aff84fa in promo-2026/projet-alice", http.StatusOK)
		})
	}
	// Zero statistics (empty commit) are valid.
	s := apiNewServer(t)
	s.handle(apiCommitsPath+"/"+apiC3, apiStatic(http.StatusOK, []byte(`{"sha":"`+apiC3+`","stats":{"total":0,"additions":0,"deletions":0}}`)))
	p := apiProvider(t, s, Options{})
	if stats, err := p.CommitStats(context.Background(), apiOwner, apiName, apiC3); err != nil || stats != (provider.CommitStats{}) {
		t.Errorf("empty commit: %+v, %v", stats, err)
	}
}

// TestCommitStatsErrors: 409 (empty repository) returned as is; 422 "No commit found for SHA"
// (vanished commit) → ErrNotFound; any other 422 (diff too long to compute, validation refused)
// stays ErrUnexpected: the commit does exist.
func TestCommitStatsErrors(t *testing.T) {
	const gone = "d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0d0"
	s := apiNewServer(t)
	s.handle(apiCommitsPath+"/"+apiC3, apiStatic(http.StatusConflict, apiFixture(t, "empty_repository.json")))
	s.handle(apiCommitsPath+"/"+gone, apiStatic(http.StatusUnprocessableEntity, apiFixture(t, "commit_unknown.json")))
	for sha, body := range map[string]string{
		// Huge commit (committed node_modules): GitHub gives up computing the diff.
		apiC4: `{"message":"Server Error: Sorry, this diff is taking too long to generate.",` +
			`"errors":[{"resource":"Commit","field":"diff","code":"not_available"}],` +
			`"documentation_url":"https://docs.github.com/rest/commits/commits#get-a-commit","status":"422"}`,
		apiC5: `{"message":"Validation Failed","errors":[{"resource":"Commit","code":"invalid"}],"status":"422"}`,
		apiC6: `{"status":"422"}`,
	} {
		s.handle(apiCommitsPath+"/"+sha, apiStatic(http.StatusUnprocessableEntity, []byte(body)))
	}
	p := apiProvider(t, s, Options{})

	for _, sha := range []string{apiC4, apiC5, apiC6} {
		_, err := p.CommitStats(context.Background(), apiOwner, apiName, sha)
		if !errors.Is(err, provider.ErrUnexpected) || errors.Is(err, provider.ErrNotFound) {
			t.Errorf("CommitStats (422 %s): %v, want ErrUnexpected without ErrNotFound", provider.ShortSHA(sha), err)
			continue
		}
		apiAPIError(t, err, "statistics of commit "+provider.ShortSHA(sha)+" in promo-2026/projet-alice", http.StatusUnprocessableEntity)
		apiNoLeak(t, err, "taking too long", "Validation Failed", "docs.github.com")
	}

	_, err := p.CommitStats(context.Background(), apiOwner, apiName, apiC3)
	if !errors.Is(err, provider.ErrEmptyRepository) {
		t.Errorf("CommitStats (409): %v, want ErrEmptyRepository", err)
	}

	_, err = p.CommitStats(context.Background(), apiOwner, apiName, gone)
	if !errors.Is(err, provider.ErrNotFound) || errors.Is(err, provider.ErrUnexpected) {
		t.Fatalf("CommitStats (422): %v, want ErrNotFound", err)
	}
	apiErr := apiAPIError(t, err, "statistics of commit d0d0d0d in promo-2026/projet-alice", http.StatusUnprocessableEntity)
	if apiErr.Path != apiCommitsPath+"/"+gone {
		t.Errorf("Path = %q", apiErr.Path)
	}
	apiNoLeak(t, err, "No commit found", "docs.github.com")
}

// TestAPIConcurrent: a single Provider serves concurrent calls (4 repositories in parallel); run
// with -race.
func TestAPIConcurrent(t *testing.T) {
	s := apiCommitsServer(t)
	s.handle(apiRepoPath, apiStatic(http.StatusOK, apiFixture(t, "repo.json")))
	s.handle(apiRepoPath+"/branches", apiPaged(t, s, "/repositories/424242/branches", "[]",
		"branches_page1.json", "branches_page2.json"))
	s.handle(apiComparePath, apiPaged(t, s, "/repositories/424242/compare/"+apiBaseSHA+"..."+apiM5,
		`{"status":"ahead","total_commits":5,"commits":[]}`, "compare_page1.json", "compare_page2.json"))
	s.handle(apiCommitsPath+"/"+apiC3, apiStatic(http.StatusOK, apiFixture(t, "commit_stats.json")))
	s.handle("/rate_limit", apiStatic(http.StatusOK, apiFixture(t, "rate_limit.json")))
	p := apiProvider(t, s, Options{})

	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 4*6*5)
	for range 4 {
		wg.Go(func() {
			for range 5 {
				if _, err := p.Repository(ctx, apiOwner, apiName); err != nil {
					errs <- err
				}
				if b, err := p.Branches(ctx, apiOwner, apiName); err != nil || len(b) != 4 {
					errs <- fmt.Errorf("Branches: %d, %v", len(b), err)
				}
				if c, err := p.ListCommits(ctx, apiOwner, apiName, provider.ListCommitsOptions{}); err != nil || len(c) != 6 {
					errs <- fmt.Errorf("ListCommits: %d, %v", len(c), err)
				}
				if c, err := p.CompareCommits(ctx, apiOwner, apiName, apiBaseSHA, apiM5); err != nil || len(c) != 5 {
					errs <- fmt.Errorf("CompareCommits: %d, %v", len(c), err)
				}
				if _, err := p.CommitStats(ctx, apiOwner, apiName, apiC3); err != nil {
					errs <- err
				}
				if _, err := p.RateLimit(ctx); err != nil {
					errs <- err
				}
				p.LastRateLimit()
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

// TestAPILogsNoPersonalData: the logs (Debug level included) contain neither the token, nor commit
// message, name, e-mail, login or branch name; nor do the errors.
func TestAPILogsNoPersonalData(t *testing.T) {
	s := apiCommitsServer(t)
	s.handle(apiRepoPath, apiStatic(http.StatusOK, apiFixture(t, "repo.json")))
	s.handle(apiRepoPath+"/branches", apiPaged(t, s, "/repositories/424242/branches", "[]",
		"branches_page1.json", "branches_page2.json"))
	s.handle(apiComparePath, apiPaged(t, s, "/repositories/424242/compare/"+apiBaseSHA+"..."+apiM5,
		`{"status":"ahead","total_commits":5,"commits":[]}`, "compare_page1.json", "compare_page2.json"))
	s.handle(apiCommitsPath+"/"+apiC3, apiStatic(http.StatusOK, apiFixture(t, "commit_stats.json")))
	s.handle("/rate_limit", apiStatic(http.StatusOK, apiFixture(t, "rate_limit.json")))
	var logs apiLogBuffer
	p := apiProvider(t, s, Options{Logger: apiLogger(&logs)})
	ctx := context.Background()

	if _, err := p.Repository(ctx, apiOwner, apiName); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Branches(ctx, apiOwner, apiName); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ListCommits(ctx, apiOwner, apiName, provider.ListCommitsOptions{Branch: "feature/login", Stop: func(sha string) bool { return sha == apiC4 }}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CompareCommits(ctx, apiOwner, apiName, apiBaseSHA, apiM5); err != nil {
		t.Fatal(err)
	}
	if _, err := p.CommitStats(ctx, apiOwner, apiName, apiC3); err != nil {
		t.Fatal(err)
	}
	if _, err := p.RateLimit(ctx); err != nil {
		t.Fatal(err)
	}
	// Errors: unusable commit (the body holds a message and an e-mail).
	s.handle(apiCommitsPath, apiStatic(http.StatusOK, []byte(`[{"sha":"court","commit":{"message":"Corrige le calcul de la moyenne","author":{"email":"alice.martin@example.com"}}}]`)))
	_, err := p.ListCommits(ctx, apiOwner, apiName, provider.ListCommitsOptions{Branch: "feature/login"})
	if !errors.Is(err, provider.ErrUnexpected) {
		t.Errorf("ListCommits: %v, want ErrUnexpected", err)
	}
	apiNoLeak(t, err, apiPersonal...)

	out := logs.String()
	if !strings.Contains(out, "commits listés") || !strings.Contains(out, "commits comparés") {
		t.Errorf("want Debug logs, got:\n%s", out)
	}
	for _, s := range append([]string{testToken, "Bearer", "Authorization"}, apiPersonal...) {
		if strings.Contains(out, s) {
			t.Errorf("the logs quote %q:\n%s", s, out)
		}
	}
}
