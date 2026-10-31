package github

// Scénarios de bout en bout de la phase 2 : le VRAI Provider (transport, cache ETag, quota, conversion
// des erreurs, appels d'API) contre le faux serveur programmable githubtest. Ils couvrent la section
// « API GitHub simulée » de documentation/fr/09-tests.md. Aucun appel réseau réel : githubtest écoute sur
// 127.0.0.1 et fait échouer le test sur toute requête autre que GET ou HEAD.
//
// Chaque environnement (newScEnv) capture les logs du Provider au niveau Debug et vérifie à la fin du
// test qu'ils ne contiennent ni token, ni en-tête Authorization, ni message de commit, nom, e-mail ou
// login d'auteur, et qu'ils sont tous de niveau Debug. Les erreurs des scénarios passent par scNoLeak.
//
// Helpers préfixés par sc ; ils réutilisent trClock, trEpoch, trLogs et trLogger (transport_test.go).

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

// Données factices des scénarios.
const (
	scOwner      = "ecole-demo"            // organisation propriétaire des dépôts (elle peut apparaître dans les logs)
	scOtherToken = "gho_autre_compte_fake" // token d'un autre compte gh, refusé par le faux serveur
	scBadToken   = "gho_token_revoque_fake"
	scUserAgent  = "watchoo/v0.0.0-test"

	// Identité et message des commits ajoutés par scEnv.commit : ils ne doivent apparaître ni dans un
	// log ni dans une erreur.
	scSecretName    = "Eleve Confidentiel"
	scSecretEmail   = "eleve.confidentiel@example.test"
	scSecretLogin   = "eleve-confidentiel"
	scSecretMessage = "Message de commit confidentiel"
)

// scForbidden est la liste des chaînes interdites dans les logs et dans les messages d'erreur.
var scForbidden = []string{
	testToken, scOtherToken, scBadToken, "gho_", "Bearer", "authorization_request",
	scSecretName, scSecretEmail, scSecretLogin, scSecretMessage,
	// Identité factice de githubtest.AppendCommits.
	"Étudiant Test", "etudiant@example.test", "etudiant-test",
}

// scAppendMessage reconnaît les messages « commit N » de githubtest.AppendCommits (un SHA court cité
// après « commit » a 7 caractères : il ne correspond pas).
var scAppendMessage = regexp.MustCompile(`\bcommit [0-9]{1,4}\b`)

// scEnv est un environnement de scénario : faux serveur GitHub, horloge commune au serveur et aux
// Provider, logs capturés, cache ETag et état du quota partagés entre les Provider construits.
type scEnv struct {
	t     *testing.T
	srv   *githubtest.Server
	clock *trClock
	logs  *trLogs
	cache *Cache
	quota *Quota
	t0    time.Time // date de départ des commits factices (dans le passé de l'horloge)
}

// newScEnv prépare un environnement et programme la vérification des logs à la fin du test.
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

// provider construit un Provider authentifié par token, branché sur le faux serveur, avec le cache et
// l'état du quota de l'environnement : comme le moteur de synchro, qui en construit un par synchro.
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
		e.t.Fatalf("New : %v", err)
	}
	p.tr.now = e.clock.now
	return p
}

// repo ajoute un dépôt privé de scOwner et renvoie le chemin de son API (« /repos/ecole-demo/… »).
func (e *scEnv) repo(id int64, name string) string {
	e.t.Helper()
	e.srv.AddRepo(githubtest.Repo{ID: id, Owner: scOwner, Name: name, Private: true})
	return "/repos/" + scOwner + "/" + name
}

// commit ajoute au dépôt repoID un commit de l'auteur factice confidentiel, daté de at, et renvoie
// son SHA. label rend le message unique.
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

// countLogs renvoie le nombre de logs de message msg.
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

// checkNoLeak vérifie, en fin de test, les logs capturés et le journal du faux serveur.
func (e *scEnv) checkNoLeak() {
	logs := e.logs.String()
	for _, s := range scForbidden {
		if strings.Contains(logs, s) {
			e.t.Errorf("les logs contiennent %q", s)
		}
	}
	if m := scAppendMessage.FindString(logs); m != "" {
		e.t.Errorf("les logs contiennent un message de commit (%q)", m)
	}
	for _, rec := range e.logs.records(e.t) {
		if rec["level"] != slog.LevelDebug.String() {
			e.t.Errorf("log de niveau %v (%v) : seul le niveau Debug est permis", rec["level"], rec["msg"])
		}
	}
	for _, r := range e.srv.Requests() {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			e.t.Errorf("requête %s %s reçue par le faux serveur", r.Method, r.Path)
		}
	}
}

// scNoLeak vérifie que le message de err ne cite ni token, ni donnée personnelle, ni paramètre de
// requête.
func scNoLeak(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	for _, s := range append(append([]string{}, scForbidden...), "per_page", "page=", "sha=") {
		if strings.Contains(msg, s) {
			t.Errorf("le message d'erreur %q contient %q", msg, s)
		}
	}
	if m := scAppendMessage.FindString(msg); m != "" {
		t.Errorf("le message d'erreur %q cite un message de commit", msg)
	}
}

// scAPIError vérifie que err est un *provider.APIError de code status (et de chemin path, s'il n'est
// pas vide) et le renvoie.
func scAPIError(t *testing.T, err error, status int, path string) *provider.APIError {
	t.Helper()
	apiErr, ok := errors.AsType[*provider.APIError](err)
	if !ok {
		t.Fatalf("erreur %v (%T) : *provider.APIError attendue", err, err)
	}
	if apiErr.Status != status || (path != "" && apiErr.Path != path) {
		t.Errorf("APIError {Path %q, Status %d}, veut {Path %q, Status %d} : %v", apiErr.Path, apiErr.Status, path, status, err)
	}
	scNoLeak(t, err)
	return apiErr
}

// scRateLimit vérifie que err est un *provider.RateLimitError conforme.
func scRateLimit(t *testing.T, err error, secondary bool, reset time.Time, retryAfter time.Duration) {
	t.Helper()
	if !errors.Is(err, provider.ErrRateLimited) {
		t.Fatalf("erreur %v : ErrRateLimited attendue", err)
	}
	rle, ok := errors.AsType[*provider.RateLimitError](err)
	if !ok {
		t.Fatalf("erreur %v : *provider.RateLimitError attendue", err)
	}
	if rle.Secondary != secondary || !rle.Reset.Equal(reset) || rle.RetryAfter != retryAfter {
		t.Errorf("RateLimitError = %+v, veut {Secondary %v, Reset %v, RetryAfter %v}", rle, secondary, reset, retryAfter)
	}
	scNoLeak(t, err)
}

// scSHAs renvoie les SHA de commits, dans l'ordre.
func scSHAs(commits []provider.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.SHA)
	}
	return out
}

// scWantSHAs compare les SHA obtenus et attendus.
func scWantSHAs(t *testing.T, what string, got []provider.Commit, want ...string) {
	t.Helper()
	if got == nil {
		t.Errorf("%s : liste nil (slice vide non nil attendue)", what)
	}
	if g := scSHAs(got); !reflect.DeepEqual(g, append([]string{}, want...)) {
		t.Errorf("%s : %d commits %v\nveut %d commits %v", what, len(g), shortSHAs(g), len(want), shortSHAs(want))
	}
}

// shortSHAs raccourcit des SHA pour les messages d'échec.
func shortSHAs(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[i] = provider.ShortSHA(s)
	}
	return out
}

// scReversed renvoie une copie de shas dans l'ordre inverse.
func scReversed(shas []string) []string {
	out := make([]string, len(shas))
	for i, s := range shas {
		out[len(shas)-1-i] = s
	}
	return out
}

// scCheckPages vérifie que les requêtes reçues pour path sont exactement les pages 1 à n, avec
// per_page=100.
func scCheckPages(t *testing.T, reqs []githubtest.Request, path string, n int) {
	t.Helper()
	var pages []string
	for _, r := range reqs {
		if r.Path != path {
			continue
		}
		if pp := r.Query.Get("per_page"); pp != strconv.Itoa(PerPage) {
			t.Errorf("%s : per_page=%q, veut %d", path, pp, PerPage)
		}
		pages = append(pages, r.Query.Get("page"))
	}
	want := make([]string, n)
	for i := range n {
		want[i] = strconv.Itoa(i + 1)
	}
	if !reflect.DeepEqual(pages, want) {
		t.Errorf("%s : pages demandées %v, veut %v", path, pages, want)
	}
}

// scAll304 vérifie que les requêtes reçues sont n requêtes conditionnelles servies en 304.
func scAll304(t *testing.T, reqs []githubtest.Request, n int) {
	t.Helper()
	if len(reqs) != n {
		t.Errorf("%d requêtes reçues, veut %d", len(reqs), n)
	}
	for _, r := range reqs {
		if r.Status != http.StatusNotModified || r.IfNoneMatch == "" {
			t.Errorf("%s ?%s : code %d, If-None-Match %q ; veut 304 avec If-None-Match", r.Path, r.RawQuery, r.Status, r.IfNoneMatch)
		}
	}
}

// TestScenarioPagination : branches et commits sur plusieurs pages (per_page=100), conversion des
// commits, Limit et Since.
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

	// 150 branches : 2 pages, ordre alphabétique de GitHub.
	branches, err := p.Branches(ctx, scOwner, "projet-pagination")
	if err != nil {
		t.Fatalf("Branches : %v", err)
	}
	if len(branches) != 150 || branches[0] != (provider.Branch{Name: "feature/000", HeadSHA: shas[0]}) ||
		branches[149] != (provider.Branch{Name: "main", HeadSHA: shas[249]}) {
		t.Errorf("Branches : %d branches, première %+v, dernière %+v", len(branches), branches[0], branches[len(branches)-1])
	}
	scCheckPages(t, e.srv.Requests(), base+"/branches", 2)
	for _, r := range e.srv.Requests() {
		if !r.ValidToken || r.UserAgent != scUserAgent {
			t.Errorf("requête %s : token valide %v, User-Agent %q", r.Path, r.ValidToken, r.UserAgent)
		}
	}

	// 250 commits : 3 pages, du plus récent au plus ancien.
	e.srv.ResetRequests()
	commits, err := p.ListCommits(ctx, scOwner, "projet-pagination", provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits : %v", err)
	}
	scWantSHAs(t, "ListCommits", commits, scReversed(shas)...)
	scCheckPages(t, e.srv.Requests(), base+"/commits", 3)
	for _, r := range e.srv.Requests() {
		if r.Query.Get("sha") != "main" {
			t.Errorf("paramètre sha = %q, veut main", r.Query.Get("sha"))
		}
	}

	// Conversion d'un commit (identité Git et compte GitHub lié, dates UTC, parents).
	if len(commits) == 250 {
		c := commits[0]
		at := e.t0.Add(249 * time.Minute)
		if c.Author.Name != "Étudiant Test" || c.Author.Email != "etudiant@example.test" ||
			c.Author.Login != "etudiant-test" || c.Author.ID == 0 || c.Author.AvatarURL == "" || c.Committer != c.Author {
			t.Errorf("Author = %+v, Committer = %+v", c.Author, c.Committer)
		}
		if !c.AuthoredAt.Equal(at) || !c.CommittedAt.Equal(at) || c.AuthoredAt.Location() != time.UTC {
			t.Errorf("dates %v / %v, veut %v en UTC", c.AuthoredAt, c.CommittedAt, at)
		}
		if c.Message != "commit 250" || !strings.Contains(c.URL, c.SHA) {
			t.Errorf("Message %q, URL %q", c.Message, c.URL)
		}
		if !reflect.DeepEqual(c.Parents, []string{shas[248]}) {
			t.Errorf("Parents = %v", c.Parents)
		}
		if root := commits[249]; root.Parents == nil || len(root.Parents) != 0 {
			t.Errorf("commit racine : Parents = %#v, veut une slice vide non nil", root.Parents)
		}
	}

	// Limit : seules les pages nécessaires sont demandées.
	e.srv.ResetRequests()
	limited, err := p.ListCommits(ctx, scOwner, "projet-pagination", provider.ListCommitsOptions{Branch: "main", Limit: 150})
	if err != nil {
		t.Fatalf("ListCommits (Limit) : %v", err)
	}
	scWantSHAs(t, "ListCommits (Limit 150)", limited, scReversed(shas)[:150]...)
	scCheckPages(t, e.srv.Requests(), base+"/commits", 2)

	// Since (dans un autre fuseau) : passé en UTC, filtre sur la date de commit, une seule page.
	e.srv.ResetRequests()
	since := e.t0.Add(200 * time.Minute)
	recent, err := p.ListCommits(ctx, scOwner, "projet-pagination", provider.ListCommitsOptions{
		Branch: "main", Since: since.In(time.FixedZone("UTC+2", 2*3600)),
	})
	if err != nil {
		t.Fatalf("ListCommits (Since) : %v", err)
	}
	scWantSHAs(t, "ListCommits (Since)", recent, scReversed(shas[200:])...)
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Query.Get("since") != since.UTC().Format(time.RFC3339) {
		t.Errorf("requêtes %+v, veut une requête avec since=%s", reqs, since.UTC().Format(time.RFC3339))
	}
}

// TestScenarioIncrementalSync suit la stratégie de « Synchronisation incrémentale »
// (documentation/fr/03-architecture.md) : premier import de la branche par défaut par ListCommits, autre
// branche par CompareCommits depuis la tête de la branche par défaut, puis synchro incrémentale par
// CompareCommits entre l'ancienne et la nouvelle tête, y compris un merge qui apporte des commits plus
// anciens que l'ancienne tête (et que l'arrêt au premier SHA connu manquerait).
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

	// La base du moteur de synchro, simulée : les SHA déjà enregistrés.
	known := map[string]bool{}
	stop := func(sha string) bool { return known[sha] }
	remember := func(commits []provider.Commit) {
		for _, c := range commits {
			known[c.SHA] = true
		}
	}

	// Premier import : main, la branche par défaut, par la liste des commits (rien n'est en base).
	first, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits main : %v", err)
	}
	scWantSHAs(t, "premier import de main", first, m3, m2, m1)
	remember(first)

	// Autre branche : compare depuis la tête de main ; m2, partagé avec main, n'est pas renvoyé.
	e.srv.ResetRequests()
	dev, err := p.CompareCommits(ctx, scOwner, name, m3, d2)
	if err != nil {
		t.Fatalf("CompareCommits main...dev : %v", err)
	}
	scWantSHAs(t, "premier import de dev", dev, d2, d1)
	if n := e.srv.Count(base + "/compare/" + m3 + "..." + d2); n != 1 || len(e.srv.Requests()) != 1 {
		t.Errorf("%d requêtes (%d pour la comparaison), veut une seule comparaison", len(e.srv.Requests()), n)
	}
	remember(dev)

	// Nouvelle activité sur main : une branche partie de m1, aux commits plus ANCIENS que l'ancienne
	// tête m3, fusionnée dans main, puis un commit.
	f1 := e.commit(id, "f1", h(1.5), m1)
	f2 := e.commit(id, "f2", h(2.5), f1)
	merge := e.commit(id, "merge", h(6), m3, f2)
	m4 := e.commit(id, "m4", h(7), merge)
	e.srv.SetBranch(id, "main", m4)

	// Le piège : dans l'ordre chronologique, f2 et f1 viennent après m3 (déjà connu) ; s'arrêter au
	// premier SHA connu les manque.
	naive, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main", Stop: stop})
	if err != nil {
		t.Fatalf("ListCommits main (2) : %v", err)
	}
	scWantSHAs(t, "ListCommits arrêté au premier SHA connu", naive, m4, merge)

	// La méthode exacte : compare entre l'ancienne et la nouvelle tête, du plus récent au plus ancien.
	got, err := p.CompareCommits(ctx, scOwner, name, m3, m4)
	if err != nil {
		t.Fatalf("CompareCommits : %v", err)
	}
	scWantSHAs(t, "CompareCommits m3...m4", got, m4, merge, f2, f1)
	if len(got) == 4 && !reflect.DeepEqual(got[1].Parents, []string{m3, f2}) {
		t.Errorf("parents du merge = %v", shortSHAs(got[1].Parents))
	}
	if len(got) == 4 && (got[0].Author.Login != scSecretLogin || got[0].Message == "") {
		t.Errorf("commit converti : %+v", got[0].Author)
	}

	// base == head : liste vide, sans requête ; head en retard sur base : liste vide.
	e.srv.ResetRequests()
	same, err := p.CompareCommits(ctx, scOwner, name, m4, m4)
	if err != nil || same == nil || len(same) != 0 || len(e.srv.Requests()) != 0 {
		t.Errorf("CompareCommits(m4, m4) : %v, %v, %d requêtes ; veut liste vide sans requête", same, err, len(e.srv.Requests()))
	}
	behind, err := p.CompareCommits(ctx, scOwner, name, m4, m3)
	if err != nil || behind == nil || len(behind) != 0 {
		t.Errorf("CompareCommits(m4, m3) : %v, %v ; veut liste vide", behind, err)
	}

	// Compare paginé : 130 nouveaux commits, 2 pages, résultat du plus récent au plus ancien.
	news := e.srv.AppendCommits(id, "main", 130, h(8), time.Minute)
	e.srv.ResetRequests()
	got, err = p.CompareCommits(ctx, scOwner, name, m4, news[129])
	if err != nil {
		t.Fatalf("CompareCommits paginé : %v", err)
	}
	scWantSHAs(t, "CompareCommits paginé", got, scReversed(news)...)
	scCheckPages(t, e.srv.Requests(), base+"/compare/"+m4+"..."+news[129], 2)

	// Statistiques d'un nouveau commit.
	stats, err := p.CommitStats(ctx, scOwner, name, merge)
	if err != nil || stats != (provider.CommitStats{Additions: 10, Deletions: 2}) {
		t.Errorf("CommitStats : %+v, %v", stats, err)
	}
}

// TestScenarioNewBranchWithMerge : cas courant chez les étudiants, non-régression de la stratégie de
// « Synchronisation incrémentale » (documentation/fr/03-architecture.md). main est en base (M0 puis M1) ;
// un étudiant pousse une nouvelle branche partie de M0, avec F1 (daté entre M0 et M1), puis un merge de
// main (X). L'arrêt au premier SHA connu perd F1 sans erreur ; la comparaison depuis la tête de la
// branche par défaut le donne, et la synchro suivante compare depuis la nouvelle tête.
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

	// Premier import de main.
	known := map[string]bool{}
	first, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits main : %v", err)
	}
	scWantSHAs(t, "premier import de main", first, m1, m0)
	for _, c := range first {
		known[c.SHA] = true
	}

	// Nouvelle branche : F1 depuis M0, puis merge de main.
	f1 := e.commit(id, "f1", h(1), m0)
	x := e.commit(id, "merge de main", h(3), f1, m1)
	e.srv.SetBranch(id, "feature", x)

	// Le piège : la liste chronologique donne X, M1, F1, M0 ; l'arrêt au premier SHA connu (M1) perd F1.
	all, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "feature"})
	if err != nil {
		t.Fatalf("ListCommits feature : %v", err)
	}
	scWantSHAs(t, "liste chronologique de feature", all, x, m1, f1, m0)
	naive, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{
		Branch: "feature", Stop: func(sha string) bool { return known[sha] },
	})
	if err != nil {
		t.Fatalf("ListCommits feature (Stop) : %v", err)
	}
	scWantSHAs(t, "ListCommits arrêté au premier SHA connu (piège)", naive, x)

	// La méthode documentée : compare depuis la tête de la branche par défaut.
	branches, err := p.Branches(ctx, scOwner, name)
	if err != nil {
		t.Fatalf("Branches : %v", err)
	}
	heads := map[string]string{}
	for _, b := range branches {
		heads[b.Name] = b.HeadSHA
	}
	got, err := p.CompareCommits(ctx, scOwner, name, heads["main"], heads["feature"])
	if err != nil {
		t.Fatalf("CompareCommits main...feature : %v", err)
	}
	scWantSHAs(t, "CompareCommits depuis la tête de main", got, x, f1)
	for _, c := range got {
		known[c.SHA] = true
	}

	// Synchro suivante : compare depuis l'ancienne tête de feature.
	f2 := e.commit(id, "f2", h(4), x)
	e.srv.SetBranch(id, "feature", f2)
	got, err = p.CompareCommits(ctx, scOwner, name, x, f2)
	if err != nil {
		t.Fatalf("CompareCommits X...F2 : %v", err)
	}
	scWantSHAs(t, "synchro suivante de feature", got, f2)
	for _, c := range got {
		known[c.SHA] = true
	}
	for _, sha := range []string{m0, m1, f1, x, f2} {
		if !known[sha] {
			t.Errorf("commit %s jamais rapporté", provider.ShortSHA(sha))
		}
	}
}

// TestScenarioForcePush : historique réécrit. Tant que GitHub garde l'ancienne tête, compare donne les
// nouveaux commits ; après son nettoyage, ErrUnknownBase (et pas ErrNotFound), puis repli sur
// ListCommits sans Stop.
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
		t.Fatalf("ListCommits : %v", err)
	}
	scWantSHAs(t, "première synchro", first, scReversed(old)...)
	for _, c := range first {
		known[c.SHA] = true
	}

	// Force-push : main revient à old[2], puis deux nouveaux commits.
	oldHead := old[4]
	e.srv.SetBranch(id, "main", old[2])
	rewritten := e.srv.AppendCommits(id, "main", 2, e.t0.Add(10*time.Hour), time.Hour)
	newHead := rewritten[1]

	got, err := p.CompareCommits(ctx, scOwner, name, oldHead, newHead)
	if err != nil {
		t.Fatalf("CompareCommits avant nettoyage : %v", err)
	}
	scWantSHAs(t, "historiques divergents", got, rewritten[1], rewritten[0])

	// Nettoyage de GitHub : l'ancienne tête n'existe plus.
	e.srv.RemoveCommit(id, oldHead)
	e.srv.ResetRequests()
	_, err = p.CompareCommits(ctx, scOwner, name, oldHead, newHead)
	if !errors.Is(err, provider.ErrUnknownBase) || errors.Is(err, provider.ErrNotFound) {
		t.Fatalf("CompareCommits après nettoyage : %v ; veut ErrUnknownBase sans ErrNotFound", err)
	}
	scAPIError(t, err, http.StatusNotFound, base+"/compare/"+oldHead+"..."+newHead)
	if !strings.Contains(err.Error(), "force-push") {
		t.Errorf("le message doit évoquer un force-push : %v", err)
	}
	if n := e.srv.Count(base); n != 1 {
		t.Errorf("%d lectures du dépôt pour départager le 404, veut 1", n)
	}

	// Repli du moteur (main est la branche de référence : pas d'autre base à essayer) : ListCommits sans
	// Stop, borné par Limit, en ignorant les SHA déjà en base.
	again, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Limit: 100})
	if err != nil {
		t.Fatalf("ListCommits après force-push : %v", err)
	}
	scWantSHAs(t, "repli sur ListCommits", again, rewritten[1], rewritten[0], old[2], old[1], old[0])
	var unknown []provider.Commit
	for _, c := range again {
		if !known[c.SHA] {
			unknown = append(unknown, c)
		}
	}
	scWantSHAs(t, "nouveaux commits après force-push", unknown, rewritten[1], rewritten[0])

	// Statistiques du commit disparu : GitHub répond 422, le Provider renvoie ErrNotFound.
	_, err = p.CommitStats(ctx, scOwner, name, oldHead)
	if !errors.Is(err, provider.ErrNotFound) {
		t.Errorf("CommitStats d'un commit disparu : %v ; veut ErrNotFound", err)
	}
	scAPIError(t, err, http.StatusUnprocessableEntity, base+"/commits/"+oldHead)
}

// scSnapshot est le résultat d'une lecture complète d'un dépôt.
type scSnapshot struct {
	Repo     provider.Repository
	Branches []provider.Branch
	Commits  []provider.Commit
}

// scRead lit le dépôt name, ses branches et les commits de main.
func scRead(t *testing.T, p *Provider, name string) scSnapshot {
	t.Helper()
	ctx := t.Context()
	repo, err := p.Repository(ctx, scOwner, name)
	if err != nil {
		t.Fatalf("Repository : %v", err)
	}
	branches, err := p.Branches(ctx, scOwner, name)
	if err != nil {
		t.Fatalf("Branches : %v", err)
	}
	commits, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main"})
	if err != nil {
		t.Fatalf("ListCommits : %v", err)
	}
	return scSnapshot{Repo: repo, Branches: branches, Commits: commits}
}

// TestScenarioETag : deuxième appel identique servi depuis le cache (304, quota non décompté), cache
// partagé entre deux Provider du même token, vidé par un autre token.
func TestScenarioETag(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1004, "projet-etag"
	e.repo(id, name)
	shas := e.srv.AppendCommits(id, "main", 250, e.t0, time.Minute)
	e.srv.SetBranch(id, "dev", shas[100])
	ctx := t.Context()

	// Première lecture : 5 requêtes (dépôt, branches, 3 pages de commits), toutes décomptées.
	p1 := e.provider(testToken)
	first := scRead(t, p1, name)
	remaining := e.srv.Quota().Remaining
	if remaining != githubtest.DefaultRateLimit-5 {
		t.Errorf("quota restant %d, veut %d", remaining, githubtest.DefaultRateLimit-5)
	}
	if rl, ok := p1.LastRateLimit(); !ok || rl.Remaining != remaining || rl.Limit != githubtest.DefaultRateLimit {
		t.Errorf("LastRateLimit = %+v, %v", rl, ok)
	}
	if e.cache.Len() != 5 {
		t.Errorf("%d entrées en cache, veut 5", e.cache.Len())
	}

	// Deuxième lecture identique : 304 partout, mêmes données (pagination comprise), quota intact.
	served := e.countLogs("304 servi depuis le cache")
	e.srv.ResetRequests()
	if second := scRead(t, p1, name); !reflect.DeepEqual(second, first) {
		t.Error("les données servies depuis le cache diffèrent de la première lecture")
	}
	scAll304(t, e.srv.Requests(), 5)
	if got := e.srv.Quota().Remaining; got != remaining {
		t.Errorf("quota restant %d après des 304, veut %d (inchangé)", got, remaining)
	}
	if rl, ok := p1.LastRateLimit(); !ok || rl.Remaining != remaining {
		t.Errorf("LastRateLimit après des 304 = %+v, %v", rl, ok)
	}
	if n := e.countLogs("304 servi depuis le cache") - served; n != 5 {
		t.Errorf("%d logs « 304 servi depuis le cache », veut 5", n)
	}

	// Synchro suivante : nouveau Provider, même token, même Cache et même Quota.
	p2 := e.provider(testToken)
	if rl, ok := p2.LastRateLimit(); !ok || rl.Remaining != remaining {
		t.Errorf("quota partagé : LastRateLimit du nouveau Provider = %+v, %v", rl, ok)
	}
	e.srv.ResetRequests()
	if third := scRead(t, p2, name); !reflect.DeepEqual(third, first) {
		t.Error("cache partagé : données différentes")
	}
	scAll304(t, e.srv.Requests(), 5)
	if got := e.srv.Quota().Remaining; got != remaining {
		t.Errorf("quota restant %d, veut %d (inchangé)", got, remaining)
	}

	// Un changement sur GitHub invalide l'entrée : réponse 200, nouvelles données, une unité de quota.
	added := e.srv.AppendCommits(id, "main", 1, e.t0.Add(300*time.Minute), time.Minute)
	e.srv.ResetRequests()
	latest, err := p2.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{Branch: "main", Limit: 1})
	if err != nil {
		t.Fatalf("ListCommits après un push : %v", err)
	}
	scWantSHAs(t, "après un push", latest, added[0])
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Status != http.StatusOK || reqs[0].IfNoneMatch == "" {
		t.Errorf("requêtes %+v, veut une requête conditionnelle répondue 200", reqs)
	}
	if got := e.srv.Quota().Remaining; got != remaining-1 {
		t.Errorf("quota restant %d, veut %d", got, remaining-1)
	}

	// Autre token (autre compte gh) : le cache est vidé avant l'envoi, aucune réponse du premier compte
	// ne lui est servie.
	other := e.provider(scOtherToken)
	e.srv.ResetRequests()
	_, err = other.Repository(ctx, scOwner, name)
	if !errors.Is(err, provider.ErrUnauthorized) {
		t.Errorf("Repository avec un autre token : %v ; veut ErrUnauthorized (token refusé par le faux serveur)", err)
	}
	scAPIError(t, err, http.StatusUnauthorized, "/repos/"+scOwner+"/"+name)
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].IfNoneMatch != "" || reqs[0].ValidToken {
		t.Errorf("requêtes %+v, veut une requête sans If-None-Match", reqs)
	}
	if n := e.cache.Len(); n != 0 {
		t.Errorf("%d entrées en cache après un changement de token, veut 0", n)
	}
	if _, ok := other.LastRateLimit(); ok {
		t.Error("LastRateLimit d'un autre token : aucun état attendu")
	}

	// Retour au premier compte : plus rien en cache, réponse 200 complète, quota décompté.
	before := e.srv.Quota().Remaining
	e.srv.ResetRequests()
	repo, err := p1.Repository(ctx, scOwner, name)
	if err != nil || repo != first.Repo {
		t.Errorf("Repository : %+v, %v", repo, err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Status != http.StatusOK || reqs[0].IfNoneMatch != "" {
		t.Errorf("requêtes %+v, veut une requête non conditionnelle répondue 200", reqs)
	}
	if got := e.srv.Quota().Remaining; got != before-1 {
		t.Errorf("quota restant %d, veut %d", got, before-1)
	}
}

// TestScenarioQuotaExhausted : quota épuisé (403 puis 429) → RateLimitError avec Reset ; requêtes
// suivantes retenues sans envoi (même par un autre Provider qui partage le Quota) ; /rate_limit permis ;
// reprise après la remise à zéro.
func TestScenarioQuotaExhausted(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1005, "projet-quota"
	base := e.repo(id, name)
	shas := e.srv.AppendCommits(id, "main", 3, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()
	if _, err := p.Repository(ctx, scOwner, name); err != nil {
		t.Fatalf("Repository : %v", err)
	}

	// Quota épuisé dans la fenêtre courante.
	q := e.srv.Quota()
	q.Remaining = 0
	e.srv.SetQuota(q)
	reset := q.Reset
	_, err := p.Branches(ctx, scOwner, name)
	scRateLimit(t, err, false, reset, 0)
	scAPIError(t, err, http.StatusForbidden, base+"/branches")
	if !strings.Contains(err.Error(), reset.Format("2006-01-02 15:04:05 UTC")) {
		t.Errorf("le message doit donner la remise à zéro : %v", err)
	}

	// Requêtes suivantes retenues sans envoi, y compris par le Provider de la synchro suivante.
	e.srv.ResetRequests()
	_, err = p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{})
	scRateLimit(t, err, false, reset, 0)
	scAPIError(t, err, 0, base+"/commits")
	_, err = e.provider(testToken).Repository(ctx, scOwner, name)
	scRateLimit(t, err, false, reset, 0)
	if n := len(e.srv.Requests()); n != 0 {
		t.Errorf("%d requêtes envoyées alors que le quota est connu comme épuisé, veut 0", n)
	}

	// GET /rate_limit reste permis (et gratuit).
	rl, err := p.RateLimit(ctx)
	if err != nil || rl.Remaining != 0 || !rl.Reset.Equal(reset) || rl.Limit != githubtest.DefaultRateLimit || !rl.ObservedAt.Equal(trEpoch) {
		t.Errorf("RateLimit : %+v, %v", rl, err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Path != "/rate_limit" {
		t.Errorf("requêtes %+v, veut GET /rate_limit seulement", reqs)
	}
	if last, ok := p.LastRateLimit(); !ok || last.Remaining != 0 {
		t.Errorf("LastRateLimit = %+v, %v", last, ok)
	}

	// Après la remise à zéro, les requêtes repartent : le Provider n'attend jamais lui-même.
	e.clock.set(reset.Add(time.Second))
	if _, err := p.Branches(ctx, scOwner, name); err != nil {
		t.Fatalf("Branches après la remise à zéro : %v", err)
	}
	if last, ok := p.LastRateLimit(); !ok || last.Remaining != githubtest.DefaultRateLimit-1 || !last.Reset.After(reset) {
		t.Errorf("LastRateLimit après la remise à zéro = %+v, %v", last, ok)
	}

	// Variante 429.
	q = e.srv.Quota()
	q.Remaining, q.Status = 0, http.StatusTooManyRequests
	e.srv.SetQuota(q)
	_, err = p.CommitStats(ctx, scOwner, name, shas[0])
	scRateLimit(t, err, false, q.Reset, 0)
	scAPIError(t, err, http.StatusTooManyRequests, base+"/commits/"+shas[0])
}

// TestScenarioSecondaryLimit : limite secondaire avec Retry-After (RetryAfter, Reset) puis sans
// (une minute d'attente, comme le recommande GitHub) ; requêtes retenues pendant l'attente.
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

	// Pendant l'attente : requêtes retenues sans envoi ; /rate_limit reste permis.
	e.srv.ResetRequests()
	_, err = p.Repository(ctx, scOwner, name)
	scRateLimit(t, err, true, trEpoch.Add(90*time.Second), 90*time.Second)
	if _, err := p.RateLimit(ctx); err != nil {
		t.Errorf("RateLimit pendant une limite secondaire : %v", err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Path != "/rate_limit" {
		t.Errorf("requêtes %+v, veut GET /rate_limit seulement", reqs)
	}
	e.clock.add(91 * time.Second)
	if _, err := p.Branches(ctx, scOwner, name); err != nil {
		t.Fatalf("Branches après Retry-After : %v", err)
	}

	// 429 sans Retry-After (message seul) : une minute d'attente.
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
		t.Errorf("%d requêtes envoyées pendant la limite secondaire, veut 0", n)
	}
	e.clock.add(31 * time.Second)
	if _, err := p.Branches(ctx, scOwner, name); err != nil {
		t.Fatalf("Branches après une minute : %v", err)
	}
}

// scCall est un appel du Provider portant sur un dépôt.
type scCall struct {
	name string
	call func() error
}

// scCalls renvoie un appel de chaque méthode du Provider sur le dépôt owner/name (id), avec des SHA
// factices pour CompareCommits et CommitStats.
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

// TestScenarioAccess : dépôt privé sans accès (404), organisation qui impose le SSO (403), token
// refusé (401), sur toutes les méthodes.
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

	// 404 : jamais « supprimé », jamais ErrUnknownBase (CompareCommits relit le dépôt).
	for _, c := range scCalls(t, p, scOwner, "projet-prive", hiddenID) {
		err := c.call()
		if !errors.Is(err, provider.ErrNotFound) || errors.Is(err, provider.ErrUnknownBase) || errors.Is(err, provider.ErrForbidden) {
			t.Errorf("%s sur un dépôt privé sans accès : %v ; veut ErrNotFound", c.name, err)
			continue
		}
		scAPIError(t, err, http.StatusNotFound, "")
		if !strings.Contains(err.Error(), "dépôt introuvable ou compte sans accès") {
			t.Errorf("%s : message %q", c.name, err)
		}
	}

	// 403 SSO : *SSORequiredError, qui satisfait ErrForbidden ; le lien n'est pas cité dans le message.
	for _, c := range scCalls(t, p, "ecole-sso", "projet-sso", ssoID) {
		err := c.call()
		sso, ok := errors.AsType[*provider.SSORequiredError](err)
		if !ok || !errors.Is(err, provider.ErrForbidden) {
			t.Errorf("%s sous SSO : %v ; veut *SSORequiredError", c.name, err)
			continue
		}
		if sso.Organization != "ecole-sso" || sso.URL != "https://github.com/orgs/ecole-sso/sso?authorization_request=fake" {
			t.Errorf("%s : SSORequiredError = %+v", c.name, sso)
		}
		scAPIError(t, err, http.StatusForbidden, "")
		if msg := err.Error(); !strings.Contains(msg, "ecole-sso") || !strings.Contains(msg, "SSO") {
			t.Errorf("%s : message %q", c.name, msg)
		}
	}

	// 401 : token refusé (révoqué), « gh auth login » conseillé, token jamais cité.
	bad := e.provider(scBadToken)
	calls := append(scCalls(t, bad, scOwner, "projet-prive", hiddenID),
		scCall{"RateLimit", func() error { _, err := bad.RateLimit(t.Context()); return err }})
	for _, c := range calls {
		err := c.call()
		if !errors.Is(err, provider.ErrUnauthorized) {
			t.Errorf("%s avec un token refusé : %v ; veut ErrUnauthorized", c.name, err)
			continue
		}
		scAPIError(t, err, http.StatusUnauthorized, "")
		if !strings.Contains(err.Error(), "gh auth login") {
			t.Errorf("%s : le message doit conseiller « gh auth login » : %v", c.name, err)
		}
	}
}

// TestScenarioRenamedRepository : dépôt renommé puis transféré ; l'ancien nom suit la redirection 301
// vers /repositories/{id} et les résultats portent les owner/nom actuels.
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
		t.Fatalf("Repository après renommage : %+v, %v", repo, err)
	}
	reqs := e.srv.Requests()
	if len(reqs) != 2 || reqs[0].Status != http.StatusMovedPermanently || reqs[0].Path != "/repos/"+scOwner+"/projet-ancien" ||
		reqs[1].Status != http.StatusOK || reqs[1].Path != "/repositories/4242" || !reqs[1].ValidToken {
		t.Errorf("requêtes %+v, veut 301 puis 200 sur /repositories/4242", reqs)
	}

	// Transfert vers une autre organisation : le nom d'origine suit encore.
	e.srv.RenameRepo(id, "autre-ecole", "projet-transfere")
	repo, err = p.Repository(ctx, scOwner, "projet-ancien")
	if err != nil || repo.ID != id || repo.FullName() != "autre-ecole/projet-transfere" {
		t.Fatalf("Repository après transfert : %+v, %v", repo, err)
	}
	byID, err := p.RepositoryByID(ctx, id)
	if err != nil || byID != repo {
		t.Errorf("RepositoryByID : %+v, %v ; veut %+v", byID, err, repo)
	}

	// Les autres appels suivent aussi la redirection.
	commits, err := p.ListCommits(ctx, scOwner, "projet-ancien", provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits par l'ancien nom : %v", err)
	}
	scWantSHAs(t, "ListCommits par l'ancien nom", commits, scReversed(shas)...)
	branches, err := p.Branches(ctx, scOwner, "projet-renomme")
	if err != nil || len(branches) != 1 || branches[0] != (provider.Branch{Name: "main", HeadSHA: shas[2]}) {
		t.Errorf("Branches par un nom intermédiaire : %+v, %v", branches, err)
	}
	got, err := p.CompareCommits(ctx, scOwner, "projet-ancien", shas[0], shas[2])
	if err != nil {
		t.Fatalf("CompareCommits par l'ancien nom : %v", err)
	}
	scWantSHAs(t, "CompareCommits par l'ancien nom", got, shas[2], shas[1])
	if _, err := p.CommitStats(ctx, scOwner, "projet-ancien", shas[1]); err != nil {
		t.Errorf("CommitStats par l'ancien nom : %v", err)
	}
	// Le nouveau nom répond directement.
	if repo, err := p.Repository(ctx, "autre-ecole", "projet-transfere"); err != nil || repo.ID != id {
		t.Errorf("Repository par le nouveau nom : %+v, %v", repo, err)
	}
}

// TestScenarioEmptyAndArchived : dépôt vide (tout juste créé par un étudiant) et dépôt archivé.
func TestScenarioEmptyAndArchived(t *testing.T) {
	e := newScEnv(t)
	base := e.repo(1009, "projet-vide")
	p := e.provider(testToken)
	ctx := t.Context()

	repo, err := p.Repository(ctx, scOwner, "projet-vide")
	if err != nil || repo.ID != 1009 || repo.DefaultBranch != "main" || !repo.Private || repo.Archived {
		t.Errorf("Repository d'un dépôt vide : %+v, %v", repo, err)
	}
	branches, err := p.Branches(ctx, scOwner, "projet-vide")
	if err != nil || branches == nil || len(branches) != 0 {
		t.Errorf("Branches d'un dépôt vide : %#v, %v ; veut liste vide non nil", branches, err)
	}
	commits, err := p.ListCommits(ctx, scOwner, "projet-vide", provider.ListCommitsOptions{})
	if err != nil || commits == nil || len(commits) != 0 {
		t.Errorf("ListCommits d'un dépôt vide : %#v, %v ; veut liste vide non nil", commits, err)
	}
	a, b := githubtest.FakeSHA("a"), githubtest.FakeSHA("b")
	_, err = p.CompareCommits(ctx, scOwner, "projet-vide", a, b)
	if !errors.Is(err, provider.ErrEmptyRepository) {
		t.Errorf("CompareCommits d'un dépôt vide : %v ; veut ErrEmptyRepository", err)
	} else {
		scAPIError(t, err, http.StatusConflict, base+"/compare/"+a+"..."+b)
	}
	_, err = p.CommitStats(ctx, scOwner, "projet-vide", a)
	if !errors.Is(err, provider.ErrEmptyRepository) || !strings.Contains(err.Error(), "dépôt vide") {
		t.Errorf("CommitStats d'un dépôt vide : %v ; veut ErrEmptyRepository", err)
	} else {
		scAPIError(t, err, http.StatusConflict, base+"/commits/"+a)
	}

	// Dépôt archivé, branche par défaut « master » : métadonnées et commits toujours lisibles.
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
		t.Errorf("Repository d'un dépôt archivé : %+v, %v\nveut %+v", repo, err, want)
	}
	e.srv.ResetRequests()
	commits, err = p.ListCommits(ctx, scOwner, "projet-archive", provider.ListCommitsOptions{})
	if err != nil {
		t.Fatalf("ListCommits d'un dépôt archivé : %v", err)
	}
	scWantSHAs(t, "ListCommits (branche par défaut)", commits, shas[1], shas[0])
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Query.Has("sha") {
		t.Errorf("requêtes %+v, veut une requête sans paramètre sha (branche par défaut)", reqs)
	}
}

// TestScenarioReadOnly : le transport refuse POST, PATCH, PUT et DELETE avant tout envoi, que la
// requête vienne de go-github ou d'un http.Client branché sur lui. Le faux serveur ferait de toute
// façon échouer le test s'il en recevait une.
func TestScenarioReadOnly(t *testing.T) {
	e := newScEnv(t)
	base := e.repo(1011, "projet-lecture")
	e.srv.AppendCommits(1011, "main", 1, e.t0, time.Hour)
	p := e.provider(testToken)
	ctx := t.Context()

	for _, method := range []string{http.MethodPost, http.MethodPatch, http.MethodPut, http.MethodDelete} {
		req, err := p.client.NewRequest(ctx, method, "repos/"+scOwner+"/projet-lecture/issues", map[string]string{"title": "x"})
		if err != nil {
			t.Fatalf("NewRequest %s : %v", method, err)
		}
		_, err = p.client.Do(req, nil)
		if !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s par go-github : %v ; veut ErrWriteForbidden", method, err)
		}
		conv := p.convertError(ctx, "écriture", nil, err)
		if !errors.Is(conv, provider.ErrWriteForbidden) {
			t.Errorf("%s converti : %v ; veut ErrWriteForbidden", method, conv)
		}
		if apiErr := scAPIError(t, conv, 0, base+"/issues"); apiErr.Method != method {
			t.Errorf("%s : APIError.Method = %q", method, apiErr.Method)
		}

		hreq, err := http.NewRequestWithContext(ctx, method, e.srv.URL()+"repos/"+scOwner+"/projet-lecture", strings.NewReader(`{"archived":true}`))
		if err != nil {
			t.Fatalf("NewRequest : %v", err)
		}
		resp, err := (&http.Client{Transport: p.tr}).Do(hreq)
		if resp != nil {
			_ = resp.Body.Close()
		}
		if !errors.Is(err, provider.ErrWriteForbidden) {
			t.Errorf("%s par http.Client : %v ; veut ErrWriteForbidden", method, err)
		}
		scNoLeak(t, err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 0 {
		t.Errorf("%d requêtes reçues par le faux serveur, veut 0 : %+v", len(reqs), reqs)
	}

	// Une lecture passe normalement.
	if _, err := p.Repository(ctx, scOwner, "projet-lecture"); err != nil {
		t.Errorf("Repository : %v", err)
	}
	if reqs := e.srv.Requests(); len(reqs) != 1 || reqs[0].Method != http.MethodGet {
		t.Errorf("requêtes %+v, veut un GET", reqs)
	}
}

// TestScenarioNoLeak : un parcours qui produit des logs Debug de chaque sorte (requêtes, 304, refus
// préventif, erreurs) et des erreurs de chaque sorte ; ni le token ni une donnée personnelle
// n'apparaissent (vérifié aussi par checkNoLeak à la fin du test). Un logger de niveau Info ne reçoit
// rien.
func TestScenarioNoLeak(t *testing.T) {
	e := newScEnv(t)
	const id, name = 1012, "projet-fuites"
	e.repo(id, name)
	m1 := e.commit(id, "m1", e.t0)
	m2 := e.commit(id, "m2", e.t0.Add(time.Hour), m1)
	e.srv.SetBranch(id, "main", m2)
	e.srv.SetBranch(id, "feature/"+scSecretLogin, m1) // nom de branche jamais journalisé
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
		for range 2 { // la deuxième fois, depuis le cache
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
		_, err = p.Branches(ctx, scOwner, name) // retenue
		keep(err)
		e.clock.add(2 * time.Minute)
		_, err = e.provider(scBadToken).Repository(ctx, scOwner, name)
		keep(err)
		return errs
	}

	errs := run(e.provider(testToken))
	if len(errs) != 5 {
		t.Errorf("%d erreurs, veut 5 : %v", len(errs), errs)
	}
	for _, err := range errs {
		scNoLeak(t, err)
	}
	for _, msg := range []string{"requête à l'API GitHub", "304 servi depuis le cache", "requête retenue : limite secondaire de l'API en cours", "commits listés", "commits comparés"} {
		if e.countLogs(msg) == 0 {
			t.Errorf("aucun log %q : la capture des logs Debug ne fonctionne pas", msg)
		}
	}
	if strings.Contains(e.logs.String(), "feature/") {
		t.Error("les logs citent un nom de branche")
	}

	// Niveau Info : aucun log.
	var info trLogs
	p, err := New(testToken, Options{BaseURL: e.srv.URL(), Logger: trLogger(&info, slog.LevelInfo)})
	if err != nil {
		t.Fatalf("New : %v", err)
	}
	if _, err := p.ListCommits(ctx, scOwner, name, provider.ListCommitsOptions{}); err != nil {
		t.Errorf("ListCommits : %v", err)
	}
	if _, err := p.Repository(ctx, scOwner, "projet-cache"); err == nil {
		t.Error("Repository d'un dépôt caché : erreur attendue")
	}
	if s := info.String(); s != "" {
		t.Errorf("logs au niveau Info : %s", s)
	}
}

// scRepoSpec décrit un dépôt du scénario concurrent.
type scRepoSpec struct {
	id        int64
	name      string
	main, dev []string
}

// scSyncResult est ce qu'une synchro simulée lit d'un dépôt.
type scSyncResult struct {
	Repo     provider.Repository
	Branches []provider.Branch
	Commits  map[string][]string // branche → SHA lus (liste pour la branche par défaut, compare sinon)
	Compare  []string
	Stats    provider.CommitStats
}

// scSync lit un dépôt comme le ferait le moteur de synchro au premier import (« Synchronisation
// incrémentale », documentation/fr/03-architecture.md) : métadonnées, branches, commits de la branche par
// défaut par ListCommits, ceux de chaque autre branche par CompareCommits depuis la tête de la branche
// par défaut, puis une comparaison et des statistiques.
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

// TestScenarioConcurrentRepositories : 4 dépôts synchronisés en parallèle avec un même Provider, puis
// une deuxième synchro (nouveau Provider, même Cache et même Quota) entièrement servie en 304, sous
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
				t.Fatalf("synchro de %s : %v", specs[i].name, err)
			}
		}
		return results
	}

	first := round(e.provider(testToken))
	for i, r := range first {
		s := specs[i]
		if len(r.Commits["main"]) != 150 || len(r.Commits["dev"]) != 30 || len(r.Compare) != 50 ||
			r.Repo.ID != s.id || r.Stats != (provider.CommitStats{}) {
			t.Errorf("%s : main %d, dev %d, compare %d commits, dépôt %d, stats %+v",
				s.name, len(r.Commits["main"]), len(r.Commits["dev"]), len(r.Compare), r.Repo.ID, r.Stats)
		}
		if len(r.Commits["main"]) == 150 && r.Commits["main"][0] != s.main[149] {
			t.Errorf("%s : premier commit de main %s, veut %s", s.name, provider.ShortSHA(r.Commits["main"][0]), provider.ShortSHA(s.main[149]))
		}
		if len(r.Commits["dev"]) == 30 && r.Commits["dev"][0] != s.dev[29] {
			t.Errorf("%s : premier commit de dev %s, veut %s", s.name, provider.ShortSHA(r.Commits["dev"][0]), provider.ShortSHA(s.dev[29]))
		}
	}

	// Deuxième synchro : tout est servi depuis le cache partagé, sans quota, sauf les statistiques d'un
	// commit, jamais mises en cache (le moteur de synchro ne les redemande d'ailleurs pas).
	remaining := e.srv.Quota().Remaining
	e.srv.ResetRequests()
	second := round(e.provider(testToken))
	if !reflect.DeepEqual(second, first) {
		t.Error("la deuxième synchro, servie depuis le cache, diffère de la première")
	}
	var cached, stats []githubtest.Request
	for _, r := range e.srv.Requests() {
		if strings.Contains(r.Path, "/commits/") {
			stats = append(stats, r)
		} else {
			cached = append(cached, r)
		}
	}
	// Par dépôt : dépôt, branches, 2 pages de main, compare de dev, compare, puis les stats.
	scAll304(t, cached, 4*6)
	if len(stats) != 4 {
		t.Errorf("%d requêtes de statistiques, veut 4", len(stats))
	}
	for _, r := range stats {
		if r.Status != http.StatusOK || r.IfNoneMatch != "" || r.Query.Get("per_page") != "1" {
			t.Errorf("%s ?%s : code %d, If-None-Match %q ; veut 200 sans If-None-Match, per_page=1",
				r.Path, r.RawQuery, r.Status, r.IfNoneMatch)
		}
	}
	if got := e.srv.Quota().Remaining; got != remaining-len(stats) {
		t.Errorf("quota restant %d après la deuxième synchro, veut %d", got, remaining-len(stats))
	}
}
