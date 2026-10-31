package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Ce fichier porte les tests de repos.go et les helpers (préfixe api…) partagés avec commits_test.go
// et ratelimit_test.go. Les réponses viennent de testdata/ : JSON au format exact de l'API REST de
// GitHub, écrit à la main avec des données factices. Aucun appel réseau réel : httptest seulement.

// Données factices de testdata/.
const (
	apiOwner  = "promo-2026"
	apiName   = "projet-alice"
	apiRepoID = 424242
	// apiRepoPath est le chemin de l'API du dépôt factice.
	apiRepoPath = "/repos/" + apiOwner + "/" + apiName
)

// apiRequest est une requête reçue par le faux serveur.
type apiRequest struct {
	Method   string
	Path     string
	RawQuery string
	Query    url.Values
	HasAuth  bool   // Authorization = « Bearer <testToken> »
	IfNone   string // en-tête If-None-Match
}

// apiServer est un faux GitHub minimal : routes exactes (chemin, sans paramètres) vers des
// gestionnaires, journal des requêtes. Une méthode autre que GET ou HEAD, ou une route inconnue, fait
// échouer le test.
type apiServer struct {
	t      *testing.T
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string]http.HandlerFunc
	reqs   []apiRequest
}

// apiNewServer démarre un faux serveur, arrêté à la fin du test.
func apiNewServer(t *testing.T) *apiServer {
	t.Helper()
	s := &apiServer{t: t, routes: make(map[string]http.HandlerFunc)}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *apiServer) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.reqs = append(s.reqs, apiRequest{
		Method:   r.Method,
		Path:     r.URL.Path,
		RawQuery: r.URL.RawQuery,
		Query:    r.URL.Query(),
		HasAuth:  r.Header.Get("Authorization") == "Bearer "+testToken,
		IfNone:   r.Header.Get("If-None-Match"),
	})
	h := s.routes[r.URL.Path]
	s.mu.Unlock()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		s.t.Errorf("requête %s %s reçue : Watchoo n'accède à GitHub qu'en lecture", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if h == nil {
		s.t.Errorf("route inattendue : %s %s", r.Method, r.URL.Path)
		apiWrite(w, http.StatusNotFound, []byte(`{"message":"Not Found","status":"404"}`), nil)
		return
	}
	h(w, r)
}

// handle branche h sur le chemin path (sans paramètres de requête).
func (s *apiServer) handle(path string, h http.HandlerFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.routes[path] = h
}

// requests renvoie une copie du journal des requêtes.
func (s *apiServer) requests() []apiRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]apiRequest(nil), s.reqs...)
}

// count renvoie le nombre de requêtes reçues sur path.
func (s *apiServer) count(path string) int {
	n := 0
	for _, r := range s.requests() {
		if r.Path == path {
			n++
		}
	}
	return n
}

// apiProvider construit un Provider branché sur le faux serveur, avec le faux token des tests.
func apiProvider(t *testing.T, s *apiServer, opts Options) *Provider {
	t.Helper()
	opts.BaseURL = s.srv.URL
	p, err := New(testToken, opts)
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	return p
}

// apiFixture lit testdata/<name>.
func apiFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("lecture de la réponse factice : %v", err)
	}
	return b
}

// apiWrite écrit une réponse JSON, avec les en-têtes h en plus.
func apiWrite(w http.ResponseWriter, status int, body []byte, h http.Header) {
	for k, v := range h {
		w.Header()[k] = v
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// apiStatic renvoie un gestionnaire qui répond toujours status et body.
func apiStatic(status int, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { apiWrite(w, status, body, nil) }
}

// apiLink construit l'en-tête Link de GitHub pour la page page sur last, avec des URL absolues sous
// linkPath (GitHub les donne sous /repositories/{id}/…) ; "" s'il n'y a qu'une page.
func apiLink(base, linkPath string, page, last int) string {
	u := func(n int) string { return fmt.Sprintf("<%s%s?per_page=100&page=%d>", base, linkPath, n) }
	var links []string
	if page < last {
		links = append(links, u(page+1)+`; rel="next"`, u(last)+`; rel="last"`)
	}
	if page > 1 {
		links = append(links, u(page-1)+`; rel="prev"`, u(1)+`; rel="first"`)
	}
	return strings.Join(links, ", ")
}

// apiPage renvoie le paramètre page de la requête (1 s'il est absent ou invalide).
func apiPage(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || n < 1 {
		return 1
	}
	return n
}

// apiPaged renvoie un gestionnaire qui sert les réponses factices fixtures, une par page, avec
// l'en-tête Link ; au-delà de la dernière page, il sert empty. Les fichiers sont lus tout de suite
// (depuis la goroutine du test).
func apiPaged(t *testing.T, s *apiServer, linkPath string, empty string, fixtures ...string) http.HandlerFunc {
	t.Helper()
	bodies := make([][]byte, len(fixtures))
	for i, name := range fixtures {
		bodies[i] = apiFixture(t, name)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		page := apiPage(r)
		if page > len(bodies) {
			apiWrite(w, http.StatusOK, []byte(empty), nil)
			return
		}
		h := http.Header{}
		if l := apiLink(s.srv.URL, linkPath, page, len(bodies)); l != "" {
			h.Set("Link", l)
		}
		apiWrite(w, http.StatusOK, bodies[page-1], h)
	}
}

// apiAPIError vérifie que err est un *provider.APIError d'opération op et de code status, et le
// renvoie.
func apiAPIError(t *testing.T, err error, op string, status int) *provider.APIError {
	t.Helper()
	var apiErr *provider.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("erreur %v (%T) : *provider.APIError attendue", err, err)
	}
	if apiErr.Op != op {
		t.Errorf("Op = %q, veut %q", apiErr.Op, op)
	}
	if apiErr.Status != status {
		t.Errorf("Status = %d, veut %d", apiErr.Status, status)
	}
	return apiErr
}

// apiNoLeak vérifie que le message de err ne cite aucun des textes interdits (token, corps de la
// réponse, données personnelles).
func apiNoLeak(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	for _, s := range append([]string{testToken, "Bearer"}, forbidden...) {
		if strings.Contains(msg, s) {
			t.Errorf("le message d'erreur cite %q : %s", s, msg)
		}
	}
}

func TestRepository(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiRepoPath, apiStatic(http.StatusOK, apiFixture(t, "repo.json")))
	p := apiProvider(t, s, Options{})

	repo, err := p.Repository(context.Background(), apiOwner, apiName)
	if err != nil {
		t.Fatalf("Repository : %v", err)
	}
	want := provider.Repository{
		ID:            apiRepoID,
		Owner:         apiOwner,
		Name:          apiName,
		DefaultBranch: "main",
		URL:           "https://github.com/promo-2026/projet-alice",
		Stars:         2,
		OpenIssues:    3,
		Archived:      false,
		Private:       true,
		PushedAt:      time.Date(2026, 10, 20, 14, 5, 0, 0, time.UTC),
	}
	if repo != want {
		t.Errorf("Repository =\n%+v\nveut\n%+v", repo, want)
	}
	if repo.PushedAt.Location() != time.UTC {
		t.Errorf("PushedAt n'est pas en UTC : %v", repo.PushedAt.Location())
	}
	if repo.FullName() != "promo-2026/projet-alice" {
		t.Errorf("FullName = %q", repo.FullName())
	}
	reqs := s.requests()
	if len(reqs) != 1 || reqs[0].Method != http.MethodGet || reqs[0].Path != apiRepoPath || !reqs[0].HasAuth {
		t.Errorf("requêtes = %+v", reqs)
	}
}

// TestRepositoryRenamed simule un dépôt renommé et transféré : GitHub répond 301 vers
// /repositories/{id}, le client suit la redirection et renvoie les owner/nom actuels.
func TestRepositoryRenamed(t *testing.T) {
	s := apiNewServer(t)
	moved := apiFixture(t, "moved_permanently.json")
	s.handle("/repos/alice-martin-dev/projet-web", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", s.srv.URL+"/repositories/424243")
		apiWrite(w, http.StatusMovedPermanently, moved, nil)
	})
	s.handle("/repositories/424243", apiStatic(http.StatusOK, apiFixture(t, "repo_renamed.json")))
	p := apiProvider(t, s, Options{})

	repo, err := p.Repository(context.Background(), "alice-martin-dev", "projet-web")
	if err != nil {
		t.Fatalf("Repository : %v", err)
	}
	if repo.ID != 424243 || repo.Owner != "promo-2026" || repo.Name != "projet-web-alice" {
		t.Errorf("dépôt renommé : ID %d, %s ; veut 424243, promo-2026/projet-web-alice", repo.ID, repo.FullName())
	}
	if repo.DefaultBranch != "develop" || !repo.Archived || repo.Stars != 0 || repo.OpenIssues != 0 {
		t.Errorf("champs actuels mal lus : %+v", repo)
	}
	if !repo.PushedAt.Equal(time.Date(2026, 10, 2, 9, 30, 0, 0, time.UTC)) {
		t.Errorf("PushedAt = %v", repo.PushedAt)
	}
	reqs := s.requests()
	if len(reqs) != 2 || reqs[1].Path != "/repositories/424243" || reqs[1].Method != http.MethodGet || !reqs[1].HasAuth {
		t.Errorf("la redirection doit être suivie en GET, avec le token : %+v", reqs)
	}
}

func TestRepositoryByID(t *testing.T) {
	s := apiNewServer(t)
	s.handle("/repositories/424242", apiStatic(http.StatusOK, apiFixture(t, "repo.json")))
	p := apiProvider(t, s, Options{})

	repo, err := p.RepositoryByID(context.Background(), apiRepoID)
	if err != nil {
		t.Fatalf("RepositoryByID : %v", err)
	}
	if repo.ID != apiRepoID || repo.FullName() != "promo-2026/projet-alice" || !repo.Private {
		t.Errorf("RepositoryByID = %+v", repo)
	}

	for _, id := range []int64{0, -1} {
		if _, err := p.RepositoryByID(context.Background(), id); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("RepositoryByID(%d) : %v, veut ErrInvalidRef", id, err)
		}
	}
	if n := len(s.requests()); n != 1 {
		t.Errorf("%d requêtes, veut 1 (un identifiant invalide ne doit pas partir)", n)
	}
}

// TestRepositoryUnexpected vérifie qu'une réponse 200 inexploitable donne ErrUnexpected.
func TestRepositoryUnexpected(t *testing.T) {
	bodies := map[string]string{
		"null":          `null`,
		"sans id":       `{"name":"projet-alice","owner":{"login":"promo-2026"}}`,
		"id nul":        `{"id":0,"name":"projet-alice","owner":{"login":"promo-2026"}}`,
		"sans owner":    `{"id":1,"name":"projet-alice"}`,
		"owner vide":    `{"id":1,"name":"projet-alice","owner":{"login":""}}`,
		"sans nom":      `{"id":1,"owner":{"login":"promo-2026"}}`,
		"owner nul":     `{"id":1,"name":"projet-alice","owner":null}`,
		"pushed_at nul": `{"id":1,"name":"projet-alice","owner":{"login":"promo-2026"},"pushed_at":null}`,
	}
	for label, body := range bodies {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiRepoPath, apiStatic(http.StatusOK, []byte(body)))
			s.handle("/repositories/1", apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})

			repo, err := p.Repository(context.Background(), apiOwner, apiName)
			if label == "pushed_at nul" {
				// pushed_at absent : dépôt valide, date zéro.
				if err != nil || !repo.PushedAt.IsZero() || repo.ID != 1 {
					t.Errorf("Repository = %+v, %v ; veut PushedAt zéro sans erreur", repo, err)
				}
				return
			}
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Fatalf("Repository : %v, veut ErrUnexpected", err)
			}
			apiErr := apiAPIError(t, err, "reading repository promo-2026/projet-alice", http.StatusOK)
			if apiErr.Method != http.MethodGet || apiErr.Path != apiRepoPath {
				t.Errorf("Method, Path = %q, %q", apiErr.Method, apiErr.Path)
			}
			_, err = p.RepositoryByID(context.Background(), 1)
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Errorf("RepositoryByID : %v, veut ErrUnexpected", err)
			}
			apiAPIError(t, err, "reading repository #1", http.StatusOK)
		})
	}
}

// TestRepositoryErrors vérifie que les erreurs HTTP passent par convertError : 404 (dépôt privé sans
// accès), 401, et que le message ne reprend ni le corps ni le message de GitHub.
func TestRepositoryErrors(t *testing.T) {
	const canary = "canari-corps-de-reponse"
	tests := []struct {
		status int
		want   error
	}{
		{http.StatusNotFound, provider.ErrNotFound},
		{http.StatusUnauthorized, provider.ErrUnauthorized},
		{http.StatusInternalServerError, provider.ErrUnavailable},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			s := apiNewServer(t)
			body := fmt.Sprintf(`{"message":"%s","documentation_url":"https://docs.github.com/rest","status":"%d"}`, canary, tt.status)
			s.handle(apiRepoPath, apiStatic(tt.status, []byte(body)))
			s.handle("/repositories/424242", apiStatic(tt.status, []byte(body)))
			p := apiProvider(t, s, Options{})

			_, err := p.Repository(context.Background(), apiOwner, apiName)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Repository : %v, veut %v", err, tt.want)
			}
			apiAPIError(t, err, "reading repository promo-2026/projet-alice", tt.status)
			apiNoLeak(t, err, canary, "docs.github.com")

			_, err = p.RepositoryByID(context.Background(), apiRepoID)
			if !errors.Is(err, tt.want) {
				t.Errorf("RepositoryByID : %v, veut %v", err, tt.want)
			}
			apiAPIError(t, err, "reading repository #424242", tt.status)
			apiNoLeak(t, err, canary)
		})
	}
}

// TestForbiddenAndRateLimited vérifie que les refus 403 (SSO imposé, quota épuisé) remontent de
// chaque appel avec leur type, sans être pris pour un dépôt vide.
func TestForbiddenAndRateLimited(t *testing.T) {
	sso := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-GitHub-SSO", "required; url=https://github.com/orgs/promo-2026/sso?authorization_request=fake")
		apiWrite(w, http.StatusForbidden, []byte(`{"message":"Resource protected by organization SAML enforcement. You must grant your OAuth token access to this organization.","documentation_url":"https://docs.github.com/articles/authenticating-to-a-github-organization-with-saml-single-sign-on/","status":"403"}`), nil)
	}
	exhausted := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Used", "5000")
		w.Header().Set("X-RateLimit-Reset", "1792000000")
		w.Header().Set("X-RateLimit-Resource", "core")
		apiWrite(w, http.StatusForbidden, []byte(`{"message":"API rate limit exceeded for user ID 1.","documentation_url":"https://docs.github.com/rest/overview/rate-limits-for-the-rest-api","status":"403"}`), nil)
	}
	calls := map[string]func(p *Provider) error{
		"Repository": func(p *Provider) error { _, err := p.Repository(context.Background(), apiOwner, apiName); return err },
		"Branches":   func(p *Provider) error { _, err := p.Branches(context.Background(), apiOwner, apiName); return err },
		"ListCommits": func(p *Provider) error {
			_, err := p.ListCommits(context.Background(), apiOwner, apiName, provider.ListCommitsOptions{})
			return err
		},
		"CompareCommits": func(p *Provider) error {
			_, err := p.CompareCommits(context.Background(), apiOwner, apiName, strings.Repeat("a", 40), strings.Repeat("b", 40))
			return err
		},
		"CommitStats": func(p *Provider) error {
			_, err := p.CommitStats(context.Background(), apiOwner, apiName, strings.Repeat("a", 40))
			return err
		},
	}
	for name, call := range calls {
		t.Run(name+"/sso", func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiRepoPath, sso)
			s.handle(apiRepoPath+"/branches", sso)
			s.handle(apiCommitsPath, sso)
			s.handle(apiRepoPath+"/compare/"+strings.Repeat("a", 40)+"..."+strings.Repeat("b", 40), sso)
			s.handle(apiCommitsPath+"/"+strings.Repeat("a", 40), sso)
			err := call(apiProvider(t, s, Options{}))
			var ssoErr *provider.SSORequiredError
			if !errors.As(err, &ssoErr) || !errors.Is(err, provider.ErrForbidden) {
				t.Fatalf("erreur %v, veut *SSORequiredError", err)
			}
			if ssoErr.Organization != apiOwner {
				t.Errorf("Organization = %q", ssoErr.Organization)
			}
			apiNoLeak(t, err, "SAML enforcement", "authorization_request")
		})
		t.Run(name+"/quota", func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiRepoPath, exhausted)
			s.handle(apiRepoPath+"/branches", exhausted)
			s.handle(apiCommitsPath, exhausted)
			s.handle(apiRepoPath+"/compare/"+strings.Repeat("a", 40)+"..."+strings.Repeat("b", 40), exhausted)
			s.handle(apiCommitsPath+"/"+strings.Repeat("a", 40), exhausted)
			err := call(apiProvider(t, s, Options{}))
			var rle *provider.RateLimitError
			if !errors.As(err, &rle) || !errors.Is(err, provider.ErrRateLimited) || rle.Secondary {
				t.Fatalf("erreur %v, veut *RateLimitError (quota épuisé)", err)
			}
			if !rle.Reset.Equal(time.Unix(1792000000, 0)) {
				t.Errorf("Reset = %v", rle.Reset)
			}
			apiNoLeak(t, err, "user ID")
		})
	}
}

// TestInvalidArgumentsNoRequest vérifie que des owner, nom, branche ou SHA invalides sont refusés
// (ErrInvalidRef) sans qu'aucune requête ne parte.
func TestInvalidArgumentsNoRequest(t *testing.T) {
	s := apiNewServer(t)
	base := &countingTransport{next: http.DefaultTransport}
	p := apiProvider(t, s, Options{Transport: base})
	ctx := context.Background()
	okSHA := strings.Repeat("a", 40)

	badRepos := [][2]string{
		{"../x", apiName}, {"a/b", apiName}, {"a?b", apiName}, {"", apiName}, {"-promo", apiName},
		{"promo_2026", apiName}, {"a#b", apiName}, {"a b", apiName}, {"%2e%2e", apiName},
		{strings.Repeat("a", 40), apiName},
		{apiOwner, "../x"}, {apiOwner, "a/b"}, {apiOwner, "a?b"}, {apiOwner, ""}, {apiOwner, "."},
		{apiOwner, ".."}, {apiOwner, "a#b"}, {apiOwner, "a b"}, {apiOwner, "%2e%2e"},
		{apiOwner, "x\n"}, {apiOwner, strings.Repeat("a", 101)},
	}
	for _, r := range badRepos {
		owner, name := r[0], r[1]
		calls := map[string]error{}
		_, calls["Repository"] = p.Repository(ctx, owner, name)
		_, calls["Branches"] = p.Branches(ctx, owner, name)
		_, calls["ListCommits"] = p.ListCommits(ctx, owner, name, provider.ListCommitsOptions{})
		_, calls["CompareCommits"] = p.CompareCommits(ctx, owner, name, okSHA, strings.Repeat("b", 40))
		_, calls["CommitStats"] = p.CommitStats(ctx, owner, name, okSHA)
		for method, err := range calls {
			if !errors.Is(err, provider.ErrInvalidRef) {
				t.Errorf("%s(%q, %q) : %v, veut ErrInvalidRef", method, owner, name, err)
			}
		}
	}

	badSHAs := []string{"", "abc", strings.Repeat("A", 40), strings.Repeat("a", 39), strings.Repeat("a", 41),
		"../../" + strings.Repeat("a", 34), strings.Repeat("g", 40), "main"}
	for _, sha := range badSHAs {
		if _, err := p.CommitStats(ctx, apiOwner, apiName, sha); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("CommitStats(%q) : %v, veut ErrInvalidRef", sha, err)
		}
		if _, err := p.CompareCommits(ctx, apiOwner, apiName, sha, okSHA); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("CompareCommits(base %q) : %v, veut ErrInvalidRef", sha, err)
		}
		if _, err := p.CompareCommits(ctx, apiOwner, apiName, okSHA, sha); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("CompareCommits(head %q) : %v, veut ErrInvalidRef", sha, err)
		}
	}

	badBranches := []string{"main\n", "ma\x00in", "\xff", "dev\x7f", strings.Repeat("b", provider.MaxBranchBytes+1)}
	for _, branch := range badBranches {
		opts := provider.ListCommitsOptions{Branch: branch}
		if _, err := p.ListCommits(ctx, apiOwner, apiName, opts); !errors.Is(err, provider.ErrInvalidRef) {
			t.Errorf("ListCommits(branche %q) : %v, veut ErrInvalidRef", branch, err)
		}
	}

	if n := len(s.requests()); n != 0 {
		t.Errorf("%d requête(s) reçue(s) par le serveur, veut 0", n)
	}
	if n := base.n.Load(); n != 0 {
		t.Errorf("%d requête(s) ont atteint le transport, veut 0", n)
	}
}

func TestBranches(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiRepoPath+"/branches", apiPaged(t, s, "/repositories/424242/branches", "[]",
		"branches_page1.json", "branches_page2.json"))
	p := apiProvider(t, s, Options{})

	branches, err := p.Branches(context.Background(), apiOwner, apiName)
	if err != nil {
		t.Fatalf("Branches : %v", err)
	}
	want := []provider.Branch{
		{Name: "dev", HeadSHA: "c2e09fb4a95488767c48bcc67e2fe0fc4f375fcc"},
		{Name: "feature/login", HeadSHA: "e7de2cd0585ecc3729db8117ba1e93e306e4866b"},
		{Name: "main", HeadSHA: "0a934d3dea406ccab14c373d35d84c7b619d0e01"},
		{Name: "release/v1", HeadSHA: "6d1c0ad1ec043e07c41dc7844d6b447d8017b143"},
	}
	if fmt.Sprint(branches) != fmt.Sprint(want) {
		t.Errorf("Branches =\n%v\nveut\n%v", branches, want)
	}
	reqs := s.requests()
	if len(reqs) != 2 {
		t.Fatalf("%d requêtes, veut 2 : %+v", len(reqs), reqs)
	}
	for i, r := range reqs {
		if r.Query.Get("per_page") != "100" || r.Query.Get("page") != strconv.Itoa(i+1) || r.Query.Has("protected") {
			t.Errorf("requête %d : paramètres %q", i+1, r.RawQuery)
		}
	}
}

// TestBranchesEmptyRepository : un dépôt vide donne une liste vide non nil, qu'il réponde 200 [] ou
// 409 « Git Repository is empty ».
func TestBranchesEmptyRepository(t *testing.T) {
	for label, h := range map[string]http.HandlerFunc{
		"200 []": apiStatic(http.StatusOK, []byte("[]")),
		"409":    apiStatic(http.StatusConflict, apiFixture(t, "empty_repository.json")),
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiRepoPath+"/branches", h)
			p := apiProvider(t, s, Options{})
			branches, err := p.Branches(context.Background(), apiOwner, apiName)
			if err != nil || branches == nil || len(branches) != 0 {
				t.Errorf("Branches = %#v, %v ; veut une liste vide non nil, sans erreur", branches, err)
			}
		})
	}
}

func TestBranchesErrors(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiRepoPath+"/branches", apiStatic(http.StatusNotFound, apiFixture(t, "not_found.json")))
	p := apiProvider(t, s, Options{})
	_, err := p.Branches(context.Background(), apiOwner, apiName)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("Branches : %v, veut ErrNotFound", err)
	}
	apiAPIError(t, err, "listing branches of promo-2026/projet-alice", http.StatusNotFound)
}

func TestBranchesUnexpected(t *testing.T) {
	for label, body := range map[string]string{
		"sans SHA":      `[{"name":"main","commit":{"url":"x"},"protected":false}]`,
		"SHA invalide":  `[{"name":"main","commit":{"sha":"abc"},"protected":false}]`,
		"sans nom":      `[{"commit":{"sha":"0a934d3dea406ccab14c373d35d84c7b619d0e01"}}]`,
		"commit nul":    `[{"name":"main","commit":null}]`,
		"élément nul":   `[null]`,
		"JSON invalide": `[{"name":`,
	} {
		t.Run(label, func(t *testing.T) {
			s := apiNewServer(t)
			s.handle(apiRepoPath+"/branches", apiStatic(http.StatusOK, []byte(body)))
			p := apiProvider(t, s, Options{})
			_, err := p.Branches(context.Background(), apiOwner, apiName)
			if !errors.Is(err, provider.ErrUnexpected) {
				t.Errorf("Branches : %v, veut ErrUnexpected", err)
			}
		})
	}
}

// apiBranchPage renvoie une page de n branches factices, numérotées à partir de first.
func apiBranchPage(first, n int) []byte {
	var b bytes.Buffer
	b.WriteByte('[')
	for i := range n {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"name":"b%05d","commit":{"sha":"%040x"},"protected":false}`, first+i, first+i+1)
	}
	b.WriteByte(']')
	return b.Bytes()
}

// TestBranchesCap : un serveur qui annonce toujours une page suivante de 100 branches est lu jusqu'au
// plafond provider.MaxBranches, sans aller au-delà.
func TestBranchesCap(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiRepoPath+"/branches", func(w http.ResponseWriter, r *http.Request) {
		page := apiPage(r)
		h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/branches", page, page+1)}}
		apiWrite(w, http.StatusOK, apiBranchPage((page-1)*PerPage, PerPage), h)
	})
	p := apiProvider(t, s, Options{})
	branches, err := p.Branches(context.Background(), apiOwner, apiName)
	if err != nil {
		t.Fatalf("Branches : %v", err)
	}
	if len(branches) != provider.MaxBranches {
		t.Errorf("%d branches, veut le plafond %d", len(branches), provider.MaxBranches)
	}
	if n := len(s.requests()); n != provider.MaxBranches/PerPage {
		t.Errorf("%d requêtes, veut %d", n, provider.MaxBranches/PerPage)
	}
}

// TestPaginationGuards : une page suivante qui ne progresse pas, ou un serveur qui annonce sans fin
// des pages d'une seule branche, n'entraînent pas de boucle sans fin.
func TestPaginationGuards(t *testing.T) {
	t.Run("page suivante qui ne progresse pas", func(t *testing.T) {
		s := apiNewServer(t)
		s.handle(apiRepoPath+"/branches", func(w http.ResponseWriter, r *http.Request) {
			h := http.Header{"Link": {fmt.Sprintf(`<%s/repositories/424242/branches?per_page=100&page=1>; rel="next"`, s.srv.URL)}}
			apiWrite(w, http.StatusOK, apiBranchPage(0, 1), h)
		})
		p := apiProvider(t, s, Options{})
		branches, err := p.Branches(context.Background(), apiOwner, apiName)
		if err != nil || len(branches) != 1 {
			t.Errorf("Branches = %d branches, %v ; veut 1", len(branches), err)
		}
		if n := len(s.requests()); n != 1 {
			t.Errorf("%d requêtes, veut 1", n)
		}
	})
	t.Run("pages sans fin", func(t *testing.T) {
		s := apiNewServer(t)
		s.handle(apiRepoPath+"/branches", func(w http.ResponseWriter, r *http.Request) {
			page := apiPage(r)
			h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/branches", page, page+1)}}
			apiWrite(w, http.StatusOK, apiBranchPage(page, 1), h)
		})
		p := apiProvider(t, s, Options{})
		branches, err := p.Branches(context.Background(), apiOwner, apiName)
		if err != nil {
			t.Fatalf("Branches : %v", err)
		}
		if n := len(s.requests()); n != maxBranchPages || len(branches) != maxBranchPages {
			t.Errorf("%d requêtes, %d branches ; veut %d", n, len(branches), maxBranchPages)
		}
	})
	t.Run("page vide qui annonce une suite", func(t *testing.T) {
		s := apiNewServer(t)
		s.handle(apiRepoPath+"/branches", func(w http.ResponseWriter, r *http.Request) {
			page := apiPage(r)
			h := http.Header{"Link": {apiLink(s.srv.URL, "/repositories/424242/branches", page, page+5)}}
			apiWrite(w, http.StatusOK, []byte("[]"), h)
		})
		p := apiProvider(t, s, Options{})
		branches, err := p.Branches(context.Background(), apiOwner, apiName)
		if err != nil || branches == nil || len(branches) != 0 {
			t.Errorf("Branches = %#v, %v", branches, err)
		}
		if n := len(s.requests()); n != 1 {
			t.Errorf("%d requêtes, veut 1", n)
		}
	})
}

// TestContextCanceled : un contexte annulé arrête l'appel ; l'erreur enveloppe context.Canceled.
func TestContextCanceled(t *testing.T) {
	s := apiNewServer(t)
	s.handle(apiRepoPath, apiStatic(http.StatusOK, apiFixture(t, "repo.json")))
	p := apiProvider(t, s, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Repository(ctx, apiOwner, apiName); !errors.Is(err, context.Canceled) {
		t.Errorf("Repository : %v, veut context.Canceled", err)
	}
	if _, err := p.Branches(ctx, apiOwner, apiName); !errors.Is(err, context.Canceled) {
		t.Errorf("Branches : %v, veut context.Canceled", err)
	}
}

// apiLogger renvoie un logger qui écrit tous les niveaux dans buf (sûr en concurrence).
func apiLogger(buf *apiLogBuffer) *slog.Logger {
	return slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// apiLogBuffer est un tampon de logs sûr en concurrence.
type apiLogBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *apiLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *apiLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
