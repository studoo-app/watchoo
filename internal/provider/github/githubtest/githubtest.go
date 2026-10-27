// Package githubtest fournit un faux serveur de l'API REST de GitHub, programmable, basé sur
// net/http/httptest. Il sert aux tests de internal/provider/github (phase 2) et du moteur de synchro
// de bout en bout (phase 3). Aucun appel réseau réel : le serveur écoute sur 127.0.0.1.
//
// Bibliothèque standard seulement : ce package n'importe ni go-github ni internal/provider/github
// (les tests du package github l'importent : pas de cycle). Données factices uniquement.
//
// # Comportement simulé (fidèle à l'API réelle, vérifiée avec gh 2.103 en octobre 2026)
//
// Routes servies (GET et HEAD ; owner et nom comparés sans tenir compte de la casse) :
//   - GET /repos/{owner}/{name} et GET /repositories/{id} : métadonnées du dépôt ;
//   - GET …/branches?per_page&page : branches triées par nom, {"name","commit":{"sha","url"},"protected":false} ;
//   - GET …/commits?sha&since&until&per_page&page : commits atteignables depuis sha (branche ou SHA
//     complet ; vide = branche par défaut), triés par date de commit décroissante (à date égale, le
//     dernier ajouté d'abord), filtrés par since/until (date de commit, bornes incluses, RFC 3339 ;
//     une date illisible → 422) ; sha inconnu → 404 « Not Found » ;
//   - GET …/commits/{ref} (SHA complet ou nom de branche) : un commit avec
//     "stats":{"additions","deletions","total"} et "files":[] ; ref inconnue → 422 « No commit found
//     for SHA: … », comme GitHub ;
//   - GET …/compare/{base}...{head}?per_page&page (SHA complets ou noms de branche) : {"status",
//     "ahead_by","behind_by","total_commits","base_commit","merge_base_commit","commits","files":[]} ;
//     commits = atteignables depuis head mais pas depuis base, du plus ANCIEN au plus récent (date de
//     commit croissante, puis ordre d'ajout), paginés ; base ou head inconnu, ou aucun ancêtre commun
//     → 404 « Not Found » (comme GitHub) ;
//   - GET …/pulls, …/releases, …/tags (V2) : voir pulls.go ;
//   - GET /rate_limit : {"resources":{"core":{…},…},"rate":{…}}, gratuit, toujours servi (même quota
//     épuisé, même pendant une limite secondaire).
//
// Toute autre route répond 404 « Not Found ».
//
// Pour chaque requête, dans l'ordre :
//  1. méthode autre que GET ou HEAD : t.Errorf (le test échoue) et réponse 405, sans autre effet (pas
//     d'entrée dans le journal) ;
//  2. enregistrement dans le journal (Requests) ;
//  3. en-tête Authorization absent, répété ou différent de « Bearer <token> » / « token <token> »
//     (schéma sans tenir compte de la casse) : 401 {"message":"Bad credentials",…}, sans en-têtes de
//     quota, sans consommer de quota ;
//  4. réponse injectée (Inject) pour ce chemin : servie telle quelle (avec les en-têtes de quota) ;
//  5. limite secondaire en cours (TriggerSecondaryLimit) : 403 ou 429 avec Retry-After, hors
//     /rate_limit ;
//  6. quota épuisé (Remaining == 0 et maintenant < Reset) : Quota.Status (403 par défaut) avec
//     X-RateLimit-Remaining: 0 et {"message":"API rate limit exceeded for user ID 1.", …}, hors
//     /rate_limit. Quand maintenant >= Reset, une nouvelle fenêtre commence (Remaining = Limit,
//     Reset = maintenant + 1 h) : ce contrôle a lieu à chaque requête autorisée, avant l'étape 4 ;
//  7. routage : dépôt inconnu ou AccessHidden → 404 {"message":"Not Found","documentation_url":…,
//     "status":"404"} ; AccessSSO → 403 avec « X-GitHub-SSO: required; url=https://github.com/orgs/
//     {owner}/sso?authorization_request=fake » et {"message":"Resource protected by organization SAML
//     enforcement. …"} ; ancien nom d'un dépôt renommé (accessible) → 301, Location =
//     URL()+"repositories/{id}" + reste du chemin + paramètres d'origine, corps {"message":"Moved
//     Permanently","url":…} (AccessHidden et AccessSSO s'appliquent aussi aux anciens noms, sans
//     redirection : rien n'est révélé d'un dépôt inaccessible) ; dépôt sans aucune branche (vide) :
//     …/commits, …/commits/{ref} et …/compare → 409 {"message":"Git Repository is empty."},
//     …/branches → 200 [] ;
//  8. réponse 200 : corps JSON, Content-Type « application/json; charset=utf-8 », ETag
//     W/"<sha256 hex du corps>", Link au format GitHub (rel="prev", "next", "last", "first" ; URL
//     absolues sous URL()+"repositories/{id}/…", comme GitHub) pour les listes paginées. Si
//     If-None-Match correspond à l'ETag (avec ou sans W/, liste séparée par des virgules, ou « * ») :
//     304 sans corps, SANS Link ni Content-Type (comme GitHub), avec un ETag fort "<sha256 hex>", et
//     sans consommer de quota.
//
// Quota : toute réponse aux étapes 4 à 8 (304 comprises) porte X-RateLimit-Limit, -Remaining, -Used,
// -Reset (secondes Unix) et -Resource: core ; toute réponse autorisée, hors 304, hors /rate_limit et
// hors refus de quota ou de limite secondaire, consomme une unité (Remaining--, Used++) avant le
// calcul des en-têtes. L'en-tête Date suit l'horloge du serveur (Options.Now).
//
// Pagination : per_page par défaut 30, au plus 100 (une valeur plus grande est ramenée à 100) ; page
// par défaut 1 ; une page au-delà de la dernière renvoie [] (ou "commits": [] pour compare).
//
// Dates : comme sur GitHub, elles sont à la seconde ; les dates reçues par AddRepo, AddCommit et
// AppendCommits sont converties en UTC et tronquées à la seconde.
//
// Le serveur est sûr pour un usage concurrent. Les méthodes de préparation (AddRepo, AddCommit…) sont
// appelées depuis la goroutine du test et y signalent une erreur de préparation par t.Fatalf ; les
// gestionnaires HTTP n'utilisent que t.Errorf.
package githubtest

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// Token est le faux token attendu par défaut. Les tests vérifient qu'il n'apparaît jamais dans les
// logs ni dans les erreurs.
const Token = "gho_fake_for_tests"

// DefaultRateLimit est la limite par défaut du quota simulé (5 000 requêtes par heure, comme GitHub).
const DefaultRateLimit = 5000

// quotaWindow est la durée d'une fenêtre de quota (une heure, comme GitHub).
const quotaWindow = time.Hour

// shaPattern : SHA complet en minuscules, SHA-1 (40) ou SHA-256 (64).
var shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// Options règle NewServer. Sa valeur zéro convient à la plupart des tests.
type Options struct {
	// Token est le token attendu dans l'en-tête Authorization ; vide = Token.
	Token string
	// Now est l'horloge du serveur (quota, remise à zéro, Reset par défaut, en-tête Date) ; nil =
	// time.Now. Elle est appelée sous le verrou du serveur.
	Now func() time.Time
}

// Server est un faux serveur de l'API GitHub. Il est arrêté automatiquement à la fin du test
// (t.Cleanup).
type Server struct {
	tb        testing.TB
	srv       *httptest.Server
	token     string
	now       func() time.Time
	done      chan struct{} // fermé par Close : interrompt les attentes (Response.Delay)
	closeOnce sync.Once

	mu         sync.Mutex
	repos      map[int64]*repoState
	names      map[string]int64 // nom actuel (nameKey) → identifiant du dépôt
	redirects  map[string]int64 // ancien nom (nameKey) → identifiant du dépôt
	quota      quotaState
	secondary  *secondaryState // limite secondaire en cours ; nil = aucune
	injections map[string]*injection
	requests   []*Request
}

// repoState est l'état d'un dépôt simulé.
type repoState struct {
	Repo
	access   Access
	commits  map[string]*commitState
	seq      int               // nombre de commits ajoutés depuis la création (ordre d'ajout)
	branches map[string]string // nom → SHA de tête
	pulls    map[int]PullRequest
	releases []Release
	tags     []Tag
}

// commitState est un commit simulé et son rang d'ajout dans le dépôt. Il n'est plus modifié après
// AddCommit.
type commitState struct {
	Commit
	seq int
}

// quotaState est l'état interne du quota simulé.
type quotaState struct {
	limit, remaining, used int
	reset                  time.Time
	status                 int
}

// secondaryState est une limite secondaire en cours.
type secondaryState struct {
	retryAfter time.Duration
	status     int
	left       int // requêtes encore refusées
}

// injection est une réponse imposée par Inject.
type injection struct {
	resp Response
	left int // requêtes restantes ; 0 = sans limite
}

// NewServer démarre un faux serveur (httptest.NewServer) et programme son arrêt par t.Cleanup. Le
// quota initial est {Limit: DefaultRateLimit, Remaining: DefaultRateLimit, Reset: maintenant + 1 h,
// tronqué à la seconde, Status: 403}.
func NewServer(t testing.TB, opts Options) *Server {
	t.Helper()
	s := &Server{
		tb:         t,
		token:      opts.Token,
		now:        opts.Now,
		done:       make(chan struct{}),
		repos:      make(map[int64]*repoState),
		names:      make(map[string]int64),
		redirects:  make(map[string]int64),
		injections: make(map[string]*injection),
	}
	if s.token == "" {
		s.token = Token
	}
	if s.now == nil {
		s.now = time.Now
	}
	s.quota = quotaState{
		limit:     DefaultRateLimit,
		remaining: DefaultRateLimit,
		reset:     s.clock().Add(quotaWindow).Truncate(time.Second),
		status:    http.StatusForbidden,
	}
	s.srv = httptest.NewServer(http.HandlerFunc(s.serveHTTP))
	t.Cleanup(s.Close)
	return s
}

// URL renvoie la racine de l'API simulée, avec « / » final (ex. « http://127.0.0.1:54321/ »), à passer
// à github.Options.BaseURL.
func (s *Server) URL() string { return s.srv.URL + "/" }

// Token renvoie le token attendu par le serveur.
func (s *Server) Token() string { return s.token }

// Close arrête le serveur ; sans effet s'il est déjà arrêté (il l'est aussi par t.Cleanup). Les
// réponses retardées (Response.Delay) sont abandonnées.
func (s *Server) Close() {
	s.closeOnce.Do(func() { close(s.done) })
	s.srv.Close()
}

// clock renvoie l'heure du serveur, en UTC.
func (s *Server) clock() time.Time { return s.now().UTC() }

// fatalf signale une erreur de préparation (goroutine du test seulement).
func (s *Server) fatalf(format string, args ...any) {
	s.tb.Helper()
	s.tb.Fatalf("githubtest : "+format, args...)
}

// nameKey renvoie la clé d'un dépôt : « owner/name » en minuscules (GitHub ignore la casse).
func nameKey(owner, name string) string {
	return strings.ToLower(owner + "/" + name)
}

// utcSecond convertit t en UTC à la seconde ; la valeur zéro reste zéro.
func utcSecond(t time.Time) time.Time {
	if t.IsZero() {
		return time.Time{}
	}
	return t.UTC().Truncate(time.Second)
}

// ----- Données -----

// Repo décrit un dépôt simulé.
type Repo struct {
	ID            int64     // identifiant numérique, > 0 et unique ; il ne change jamais
	Owner         string    // propriétaire (login), unique avec Name sans tenir compte de la casse
	Name          string    // nom
	Private       bool      // dépôt privé ("private": true, "visibility": "private")
	Archived      bool      // dépôt archivé
	DefaultBranch string    // branche par défaut ; vide = « main »
	Stars         int       // "stargazers_count"
	OpenIssues    int       // "open_issues_count"
	PushedAt      time.Time // "pushed_at" ; zéro = null
}

// Access règle la réponse du serveur pour un dépôt.
type Access int

// Valeurs de Access.
const (
	// AccessGranted : accès normal.
	AccessGranted Access = iota
	// AccessHidden : 404 « Not Found » sur toutes les routes du dépôt, exactement comme un dépôt privé
	// sans accès (ou supprimé).
	AccessHidden
	// AccessSSO : 403 avec l'en-tête X-GitHub-SSO sur toutes les routes du dépôt (l'organisation
	// impose le SSO et le token n'y est pas autorisé).
	AccessSSO
)

// Person est l'auteur ou le committer d'un commit simulé.
type Person struct {
	Name  string // nom Git ("commit.author.name")
	Email string // e-mail Git ("commit.author.email")
	// Login est le compte GitHub lié ; vide = aucun compte lié : le champ "author" (ou "committer") du
	// JSON vaut null.
	Login     string
	ID        int64  // identifiant du compte lié ; 0 = dérivé de Login (stable)
	AvatarURL string // vide = « https://avatars.example.test/u/{ID} »
}

// Commit est un commit simulé.
type Commit struct {
	SHA         string    // SHA complet (40 ou 64 hex minuscules) ; vide = SHA déterministe dérivé du contenu et de l'ordre d'ajout
	Message     string    // message
	Author      Person    // auteur
	Committer   Person    // committer ; zéro = Author
	AuthoredAt  time.Time // date d'écriture ; zéro = CommittedAt
	CommittedAt time.Time // date de commit ; zéro = AuthoredAt (les deux zéro : erreur de préparation)
	Parents     []string  // SHA des parents, déjà ajoutés au même dépôt ; vide = commit racine
	Additions   int       // "stats.additions"
	Deletions   int       // "stats.deletions"
}

// AddRepo ajoute un dépôt (sans branche : il est vide tant qu'aucune branche n'est posée). ID ou
// owner/nom déjà pris, ID <= 0, owner ou nom vide ou contenant un caractère réservé des URL (« / »,
// « ? », « # », « % », « \ ») ou de contrôle : t.Fatalf. Un ancien nom d'un dépôt renommé peut être
// repris : sa redirection disparaît, comme sur GitHub.
func (s *Server) AddRepo(r Repo) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case r.ID <= 0:
		s.fatalf("AddRepo : identifiant %d invalide (> 0 attendu)", r.ID)
	case !validRepoName(r.Owner) || !validRepoName(r.Name):
		s.fatalf("AddRepo : owner %q ou nom %q invalide", r.Owner, r.Name)
	}
	if _, ok := s.repos[r.ID]; ok {
		s.fatalf("AddRepo : identifiant %d déjà pris", r.ID)
	}
	key := nameKey(r.Owner, r.Name)
	if _, ok := s.names[key]; ok {
		s.fatalf("AddRepo : dépôt %s/%s déjà présent", r.Owner, r.Name)
	}
	if r.DefaultBranch == "" {
		r.DefaultBranch = "main"
	}
	r.PushedAt = utcSecond(r.PushedAt)
	delete(s.redirects, key)
	s.names[key] = r.ID
	s.repos[r.ID] = &repoState{
		Repo:     r,
		commits:  make(map[string]*commitState),
		branches: make(map[string]string),
	}
}

// validRepoName indique si s peut servir d'owner ou de nom de dépôt dans un chemin : non vide, sans
// caractère réservé des URL ni caractère de contrôle.
func validRepoName(s string) bool {
	return s != "" && !strings.ContainsAny(s, "/\\?#%") && strings.IndexFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f }) < 0
}

// RenameRepo renomme ou transfère le dépôt id vers newOwner/newName. Toutes les routes de l'ancien
// nom répondent ensuite 301 vers /repositories/{id}/… ; les anciens noms successifs restent redirigés.
// Nom déjà pris par un autre dépôt : t.Fatalf.
func (s *Server) RenameRepo(id int64, newOwner, newName string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("RenameRepo", id)
	if !validRepoName(newOwner) || !validRepoName(newName) {
		s.fatalf("RenameRepo : owner %q ou nom %q invalide", newOwner, newName)
	}
	newKey := nameKey(newOwner, newName)
	if other, ok := s.names[newKey]; ok && other != id {
		s.fatalf("RenameRepo : dépôt %s/%s déjà présent", newOwner, newName)
	}
	oldKey := nameKey(repo.Owner, repo.Name)
	delete(s.names, oldKey)
	if oldKey != newKey {
		s.redirects[oldKey] = id
	}
	delete(s.redirects, newKey)
	s.names[newKey] = id
	repo.Owner, repo.Name = newOwner, newName
}

// SetRepoAccess règle l'accès au dépôt id (AccessGranted par défaut).
func (s *Server) SetRepoAccess(id int64, a Access) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetRepoAccess", id)
	if a < AccessGranted || a > AccessSSO {
		s.fatalf("SetRepoAccess : accès %d inconnu", a)
	}
	repo.access = a
}

// repoLocked renvoie le dépôt id (t.Fatalf s'il est inconnu). Appelée avec s.mu verrouillé.
func (s *Server) repoLocked(op string, id int64) *repoState {
	s.tb.Helper()
	repo, ok := s.repos[id]
	if !ok {
		s.fatalf("%s : dépôt %d inconnu", op, id)
	}
	return repo
}

// AddCommit ajoute un commit au dépôt repoID et renvoie son SHA. SHA mal formé ou déjà présent, parent
// inconnu, dates toutes deux nulles ou dépôt inconnu : t.Fatalf. Ajouter un commit ne déplace aucune
// branche (voir SetBranch).
func (s *Server) AddCommit(repoID int64, c Commit) string {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addCommitLocked(s.repoLocked("AddCommit", repoID), c)
}

// addCommitLocked ajoute c à repo. Appelée avec s.mu verrouillé.
func (s *Server) addCommitLocked(repo *repoState, c Commit) string {
	s.tb.Helper()
	if c.CommittedAt.IsZero() && c.AuthoredAt.IsZero() {
		s.fatalf("AddCommit : commit sans date (AuthoredAt et CommittedAt nuls)")
	}
	if c.CommittedAt.IsZero() {
		c.CommittedAt = c.AuthoredAt
	}
	if c.AuthoredAt.IsZero() {
		c.AuthoredAt = c.CommittedAt
	}
	c.AuthoredAt, c.CommittedAt = utcSecond(c.AuthoredAt), utcSecond(c.CommittedAt)
	if c.Committer == (Person{}) {
		c.Committer = c.Author
	}
	for _, p := range c.Parents {
		if _, ok := repo.commits[p]; !ok {
			s.fatalf("AddCommit : parent %s inconnu dans le dépôt %d", p, repo.ID)
		}
	}
	c.Parents = append([]string{}, c.Parents...)
	repo.seq++
	if c.SHA == "" {
		c.SHA = FakeSHA(fmt.Sprintf("githubtest/%d/%d/%s/%s/%s", repo.ID, repo.seq, c.Message,
			c.CommittedAt.Format(time.RFC3339), strings.Join(c.Parents, ",")))
	} else if !shaPattern.MatchString(c.SHA) {
		s.fatalf("AddCommit : SHA %q mal formé (40 ou 64 hex minuscules attendus)", c.SHA)
	}
	if _, ok := repo.commits[c.SHA]; ok {
		s.fatalf("AddCommit : commit %s déjà présent dans le dépôt %d", c.SHA, repo.ID)
	}
	repo.commits[c.SHA] = &commitState{Commit: c, seq: repo.seq}
	return c.SHA
}

// SetBranch crée la branche name du dépôt repoID, ou la déplace (y compris en arrière : force-push),
// vers le commit headSHA, qui doit exister (sinon t.Fatalf). Nom vide : t.Fatalf.
func (s *Server) SetBranch(repoID int64, name, headSHA string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetBranch", repoID)
	if name == "" {
		s.fatalf("SetBranch : nom de branche vide")
	}
	if _, ok := repo.commits[headSHA]; !ok {
		s.fatalf("SetBranch : commit %s inconnu dans le dépôt %d", headSHA, repoID)
	}
	repo.branches[name] = headSHA
}

// DeleteBranch supprime la branche name du dépôt repoID (les commits restent). Branche inconnue :
// t.Fatalf.
func (s *Server) DeleteBranch(repoID int64, name string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("DeleteBranch", repoID)
	if _, ok := repo.branches[name]; !ok {
		s.fatalf("DeleteBranch : branche %q inconnue dans le dépôt %d", name, repoID)
	}
	delete(repo.branches, name)
}

// RemoveCommit retire le commit sha du dépôt repoID, comme le nettoyage de GitHub après un force-push :
// un compare dont c'est la base répond ensuite 404. Le commit ne doit plus être la tête d'une branche
// (sinon t.Fatalf) ; les commits qui le référencent comme parent le gardent dans "parents" (le
// parcours de l'historique s'y arrête).
func (s *Server) RemoveCommit(repoID int64, sha string) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("RemoveCommit", repoID)
	if _, ok := repo.commits[sha]; !ok {
		s.fatalf("RemoveCommit : commit %s inconnu dans le dépôt %d", sha, repoID)
	}
	for name, head := range repo.branches {
		if head == sha {
			s.fatalf("RemoveCommit : le commit %s est la tête de la branche %q", sha, name)
		}
	}
	delete(repo.commits, sha)
}

// AppendCommits ajoute n commits linéaires au-dessus de la tête de branch (ou un historique neuf si la
// branche n'existe pas), datés start, start+step, start+2*step… (date d'écriture = date de commit),
// de messages « commit 1 » … « commit n », par l'auteur factice {Name: « Étudiant Test », Email:
// « etudiant@example.test », Login: « etudiant-test »} (committer identique), puis place branch sur
// le dernier. Renvoie les SHA dans l'ordre de création (du plus ancien au plus récent ; slice vide
// non nil si n vaut 0, sans toucher à la branche). n négatif, branche vide ou start nul : t.Fatalf.
func (s *Server) AppendCommits(repoID int64, branch string, n int, start time.Time, step time.Duration) []string {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("AppendCommits", repoID)
	switch {
	case branch == "":
		s.fatalf("AppendCommits : nom de branche vide")
	case n < 0:
		s.fatalf("AppendCommits : nombre de commits %d négatif", n)
	case n > 0 && start.IsZero():
		s.fatalf("AppendCommits : date de départ nulle")
	}
	author := Person{Name: "Étudiant Test", Email: "etudiant@example.test", Login: "etudiant-test"}
	parent := repo.branches[branch]
	shas := make([]string, 0, n)
	for i := range n {
		at := start.Add(time.Duration(i) * step)
		c := Commit{Message: fmt.Sprintf("commit %d", i+1), Author: author, AuthoredAt: at, CommittedAt: at}
		if parent != "" {
			c.Parents = []string{parent}
		}
		parent = s.addCommitLocked(repo, c)
		shas = append(shas, parent)
	}
	if n > 0 {
		repo.branches[branch] = parent
	}
	return shas
}

// FakeSHA renvoie un SHA-1 factice et déterministe (40 hex minuscules) dérivé de seed.
func FakeSHA(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// ----- Quota -----

// Quota est l'état du quota simulé (ressource core).
type Quota struct {
	Limit     int       // requêtes par fenêtre ; 0 = DefaultRateLimit
	Remaining int       // requêtes restantes
	Reset     time.Time // fin de la fenêtre (tronquée à la seconde) ; zéro = maintenant + 1 h
	Status    int       // code renvoyé quand le quota est épuisé : 403 (par défaut, 0) ou 429
}

// SetQuota remplace l'état du quota (Used = Limit - Remaining). Remaining à 0 avec Reset futur : les
// requêtes suivantes (hors /rate_limit) reçoivent le refus de quota. Limit ou Remaining négatif,
// Status autre que 0, 403 ou 429 : t.Fatalf.
func (s *Server) SetQuota(q Quota) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case q.Limit < 0 || q.Remaining < 0:
		s.fatalf("SetQuota : Limit (%d) et Remaining (%d) doivent être positifs", q.Limit, q.Remaining)
	case q.Status != 0 && q.Status != http.StatusForbidden && q.Status != http.StatusTooManyRequests:
		s.fatalf("SetQuota : code %d invalide (403 ou 429 attendu)", q.Status)
	}
	if q.Limit == 0 {
		q.Limit = DefaultRateLimit
	}
	if q.Reset.IsZero() {
		q.Reset = s.clock().Add(quotaWindow)
	}
	if q.Status == 0 {
		q.Status = http.StatusForbidden
	}
	s.quota = quotaState{
		limit:     q.Limit,
		remaining: q.Remaining,
		used:      max(q.Limit-q.Remaining, 0),
		reset:     q.Reset.UTC().Truncate(time.Second),
		status:    q.Status,
	}
}

// Quota renvoie l'état courant du quota (après les consommations ; une fenêtre échue n'est
// renouvelée qu'à la requête suivante).
func (s *Server) Quota() Quota {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Quota{Limit: s.quota.limit, Remaining: s.quota.remaining, Reset: s.quota.reset, Status: s.quota.status}
}

// SecondaryLimit décrit une limite secondaire (anti-abus) à simuler.
type SecondaryLimit struct {
	// RetryAfter est la valeur de l'en-tête Retry-After, arrondie à la seconde (1 s au moins) ; 0 =
	// pas d'en-tête Retry-After (le message « secondary rate limit » reste présent).
	RetryAfter time.Duration
	// Status est le code renvoyé : 403 (par défaut, 0) ou 429.
	Status int
	// Requests est le nombre de requêtes refusées avant le retour à la normale ; 0 = 1.
	Requests int
}

// TriggerSecondaryLimit fait refuser les prochaines requêtes (hors /rate_limit) par une limite
// secondaire : code l.Status, Retry-After si l.RetryAfter > 0, en-têtes de quota normaux
// (X-RateLimit-Remaining non nul), corps {"message":"You have exceeded a secondary rate limit. Please
// wait a few minutes before you try again.","documentation_url":"https://docs.github.com/rest/overview/
// rate-limits-for-the-rest-api#about-secondary-rate-limits"}. Ces refus ne consomment pas de quota. Un
// nouvel appel remplace la limite en cours.
func (s *Server) TriggerSecondaryLimit(l SecondaryLimit) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case l.RetryAfter < 0 || l.Requests < 0:
		s.fatalf("TriggerSecondaryLimit : RetryAfter (%s) et Requests (%d) doivent être positifs", l.RetryAfter, l.Requests)
	case l.Status != 0 && l.Status != http.StatusForbidden && l.Status != http.StatusTooManyRequests:
		s.fatalf("TriggerSecondaryLimit : code %d invalide (403 ou 429 attendu)", l.Status)
	}
	if l.Status == 0 {
		l.Status = http.StatusForbidden
	}
	if l.Requests == 0 {
		l.Requests = 1
	}
	s.secondary = &secondaryState{retryAfter: l.RetryAfter, status: l.Status, left: l.Requests}
}

// ----- Réponses injectées -----

// Response est une réponse imposée par Inject.
type Response struct {
	Status int         // code HTTP ; 0 = 200
	Header http.Header // en-têtes ajoutés (ils l'emportent sur les en-têtes du serveur : quota, Content-Type)
	Body   string      // corps, servi tel quel (Content-Type « application/json; charset=utf-8 » sauf en-tête contraire)
	Times  int         // nombre de requêtes concernées ; 0 = toutes, jusqu'à ClearInjections
	// Delay est attendu avant de répondre (pour tester Options.Timeout) ; l'attente s'interrompt si
	// le client abandonne la requête (aucune réponse n'est alors écrite) ou si le serveur s'arrête.
	Delay time.Duration
}

// Inject impose la réponse r aux requêtes autorisées dont le chemin (r.URL.Path) vaut exactement
// path, par exemple « /repos/alice/projet » ; une nouvelle injection pour le même chemin remplace la
// précédente. Ces réponses consomment du quota comme les autres (sauf code 304 et /rate_limit).
// Chemin sans « / » initial, Times ou Delay négatif : t.Fatalf.
func (s *Server) Inject(path string, r Response) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case !strings.HasPrefix(path, "/"):
		s.fatalf("Inject : chemin %q sans « / » initial", path)
	case r.Times < 0 || r.Delay < 0:
		s.fatalf("Inject : Times (%d) et Delay (%s) doivent être positifs", r.Times, r.Delay)
	}
	if r.Status == 0 {
		r.Status = http.StatusOK
	}
	r.Header = r.Header.Clone()
	s.injections[path] = &injection{resp: r, left: r.Times}
}

// ClearInjections retire toutes les réponses injectées.
func (s *Server) ClearInjections() {
	s.mu.Lock()
	defer s.mu.Unlock()
	clear(s.injections)
}

// ----- Journal -----

// Request est une requête reçue par le serveur. Elle ne garde jamais la valeur de l'en-tête
// Authorization.
type Request struct {
	Method           string     // méthode HTTP
	Path             string     // chemin (r.URL.Path), ex. « /repos/alice/projet/commits »
	RawQuery         string     // paramètres bruts, ex. « per_page=100&sha=main »
	Query            url.Values // paramètres décodés
	IfNoneMatch      string     // en-tête If-None-Match reçu ; vide si absent
	UserAgent        string     // en-tête User-Agent reçu
	HasAuthorization bool       // l'en-tête Authorization était présent
	ValidToken       bool       // il portait le token attendu
	Status           int        // code HTTP renvoyé (ou prévu, si le client a abandonné pendant Response.Delay)
}

// Requests renvoie une copie du journal des requêtes reçues, dans l'ordre d'arrivée.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.requests))
	for i, r := range s.requests {
		out[i] = *r
		out[i].Query = cloneValues(r.Query)
	}
	return out
}

// ResetRequests vide le journal.
func (s *Server) ResetRequests() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = nil
}

// Count renvoie le nombre de requêtes reçues dont le chemin vaut exactement path.
func (s *Server) Count(path string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.requests {
		if r.Path == path {
			n++
		}
	}
	return n
}

// cloneValues copie v (nil reste nil).
func cloneValues(v url.Values) url.Values {
	if v == nil {
		return nil
	}
	out := make(url.Values, len(v))
	for k, vs := range v {
		out[k] = append([]string(nil), vs...)
	}
	return out
}
