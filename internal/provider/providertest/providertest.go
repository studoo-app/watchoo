// Package providertest fournit un faux provider.Provider en mémoire, pour les tests du moteur de
// synchro (phase 3), de la CLI, du web et du TUI. Aucune requête réseau, aucun serveur.
//
// Le Fake reproduit le CONTRAT de provider.Provider (voir internal/provider) : validation des
// arguments avec provider.Validate* (ErrInvalidRef), owner/nom comparés sans tenir compte de la casse,
// suivi des renommages, ordre des commits, Since, Stop, Limit, MaxCommits, MaxBranches, dépôt vide,
// ErrUnknownBase, slices vides non nil, copies indépendantes (modifier un résultat ne modifie pas le
// Fake). Il ne dépend que de internal/provider et internal/model.
//
// Erreurs : comme le Provider GitHub, les erreurs produites par le Fake lui-même (dépôt inconnu, dépôt
// vide, quota épuisé…) sont des *provider.APIError {Op, Method: « GET », Path, Status} qui enveloppent
// l'erreur sentinelle (Op et Path reprennent les libellés du Provider GitHub, par exemple « lecture du
// dépôt alice/projet » et « /repos/alice/projet »). Les erreurs de validation, ctx.Err() et les
// erreurs injectées (SetError, FailNext, SetRepoError) sont renvoyées telles quelles.
//
// Dates : comme sur GitHub, elles sont en UTC et à la seconde ; les dates reçues par AddRepository,
// UpdateRepository, AddCommits et AppendCommits sont converties en UTC et tronquées à la seconde.
//
// Sûr pour un usage concurrent. Les méthodes de préparation (AddRepository, AddCommits…) paniquent sur
// une préparation incohérente (dépôt inconnu, parent inconnu…) : c'est une erreur du test.
//
// Ordre des contrôles dans chaque méthode de provider.Provider :
//  1. comptage de l'appel (Calls, CallLog) ;
//  2. ctx.Err() (contexte terminé) : renvoyée telle quelle ;
//  3. validation des arguments : erreur enveloppant provider.ErrInvalidRef ;
//  4. erreur injectée : d'abord la file de FailNext pour cette méthode, puis SetError ;
//  5. erreur propre au dépôt (SetRepoError), pour les méthodes portant sur un dépôt ;
//  6. quota simulé (SetRateLimit) : voir SetRateLimit ;
//  7. logique en mémoire.
//
// Seule exception, comme pour le Provider GitHub qui n'envoie alors aucune requête : CompareCommits
// avec base == head renvoie une liste vide juste après l'étape 3.
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

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
)

// Valeurs du quota simulé par défaut (comme GitHub : 5 000 requêtes par heure).
const (
	defaultRateLimit = 5000
	rateWindow       = time.Hour
)

// Codes HTTP des erreurs produites par le Fake (ceux que renverrait GitHub).
const (
	statusForbidden = 403
	statusNotFound  = 404
	statusConflict  = 409
)

// Method nomme une méthode de provider.Provider, pour l'injection d'erreurs et le comptage des appels.
type Method string

// Méthodes de provider.Provider (Name et LastRateLimit ne sont ni comptées ni injectables).
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

// valid indique si m est l'une des méthodes ci-dessus.
func (m Method) valid() bool {
	switch m {
	case MethodRepository, MethodRepositoryByID, MethodBranches, MethodListCommits, MethodCompareCommits,
		MethodCommitStats, MethodPullRequests, MethodReleases, MethodTags, MethodRateLimit:
		return true
	}
	return false
}

// Call est un appel reçu par le Fake (CallLog). Seuls les champs utiles à la méthode sont remplis.
type Call struct {
	Method Method
	Owner  string // owner tel que passé (toutes les méthodes portant sur un dépôt nommé)
	Name   string // nom tel que passé
	ID     int64  // RepositoryByID
	// Opts reprend les options de ListCommits, sauf Stop (toujours nil ici) ; StopSet indique si un
	// Stop était fourni.
	Opts    provider.ListCommitsOptions
	StopSet bool
	Base    string // CompareCommits
	Head    string // CompareCommits
	SHA     string // CommitStats
	// PullOpts reprend les options de PullRequests.
	PullOpts provider.PullRequestsOptions
}

// Fake est un provider.Provider en mémoire. Construire avec New.
type Fake struct {
	mu       sync.Mutex
	repos    map[int64]*fakeRepo
	names    map[string]int64 // nameKey (nom actuel ou ancien) → identifiant du dépôt
	errs     map[Method]error
	next     map[Method][]error
	repoErrs map[string]error // nameKey → erreur (SetRepoError)
	calls    map[Method]int
	log      []Call
	rate     provider.RateLimit // quota simulé (si rateOn)
	rateOn   bool
	now      func() time.Time
}

// fakeRepo est un dépôt en mémoire.
type fakeRepo struct {
	meta     provider.Repository
	keys     []string // nameKey du nom actuel et des anciens noms encore redirigés
	commits  map[string]*fakeCommit
	seq      int               // nombre de commits ajoutés depuis la création (ordre d'ajout)
	branches map[string]string // nom → SHA de tête
	stats    map[string]provider.CommitStats
	pulls    map[int]provider.PullRequest // numéro → pull request (SetPullRequests)
	releases []provider.Release           // SetReleases
	tags     []provider.Tag               // SetTags, dans l'ordre donné
}

// fakeCommit est un commit en mémoire et son rang d'ajout. Il n'est plus jamais modifié après
// AddCommits : il peut donc être lu hors du verrou une fois obtenu sous le verrou.
type fakeCommit struct {
	c   provider.Commit
	seq int
}

// Vérifie à la compilation que *Fake implémente provider.Provider.
var _ provider.Provider = (*Fake)(nil)

// New renvoie un Fake vide : aucun dépôt, pas de quota simulé, horloge time.Now.
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

// clock renvoie l'heure du Fake, en UTC. Appelée avec f.mu verrouillé.
func (f *Fake) clock() time.Time { return f.now().UTC() }

// ----- Préparation -----

// AddRepository ajoute un dépôt (ID > 0 unique ; Owner et Name valides et uniques sans tenir compte
// de la casse ; DefaultBranch vide = « main »). Le dépôt n'a ni branche ni commit : il est vide. Un
// ancien nom d'un dépôt renommé peut être repris : il désigne alors le nouveau dépôt, comme sur GitHub.
func (f *Fake) AddRepository(r provider.Repository) {
	if err := provider.ValidateID(r.ID); err != nil {
		panic("providertest.AddRepository : " + err.Error())
	}
	if err := provider.ValidateRepo(r.Owner, r.Name); err != nil {
		panic("providertest.AddRepository : " + err.Error())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.repos[r.ID]; ok {
		panic(fmt.Sprintf("providertest.AddRepository : identifiant %d déjà pris", r.ID))
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

// normalizeRepo applique les valeurs par défaut et les conventions de date.
func normalizeRepo(r provider.Repository) provider.Repository {
	if r.DefaultBranch == "" {
		r.DefaultBranch = "main"
	}
	r.PushedAt = utcSecond(r.PushedAt)
	return r
}

// claimName réserve key pour le dépôt id : panique si key est le nom ACTUEL d'un autre dépôt ; un
// ancien nom d'un autre dépôt lui est retiré. Appelée avec f.mu verrouillé.
func (f *Fake) claimName(key string, id int64, op string) {
	other, ok := f.names[key]
	if !ok || other == id {
		return
	}
	o := f.repos[other]
	if nameKey(o.meta.Owner, o.meta.Name) == key {
		panic(fmt.Sprintf("%s : dépôt %s déjà présent (identifiant %d)", op, o.meta.FullName(), other))
	}
	o.keys = slices.DeleteFunc(o.keys, func(k string) bool { return k == key })
	delete(f.names, key)
}

// UpdateRepository remplace les métadonnées du dépôt r.ID (qui doit exister ; DefaultBranch vide =
// « main »). Si Owner ou Name changent, c'est un renommage ou un transfert : Repository(ancien owner,
// ancien nom) et les autres méthodes appelées avec l'ancien nom continuent de trouver le dépôt (comme
// la redirection 301 de GitHub) et renvoient les valeurs actuelles.
func (f *Fake) UpdateRepository(r provider.Repository) {
	if err := provider.ValidateRepo(r.Owner, r.Name); err != nil {
		panic("providertest.UpdateRepository : " + err.Error())
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

// RemoveRepository retire le dépôt id : toutes les méthodes répondent ensuite ErrNotFound pour lui,
// comme pour un dépôt privé devenu inaccessible. Ses noms (actuel et anciens) sont libérés.
func (f *Fake) RemoveRepository(id int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.RemoveRepository", id)
	for _, k := range repo.keys {
		delete(f.names, k)
	}
	delete(f.repos, id)
}

// repoLocked renvoie le dépôt id ; panique s'il est inconnu. Appelée avec f.mu verrouillé.
func (f *Fake) repoLocked(op string, id int64) *fakeRepo {
	repo, ok := f.repos[id]
	if !ok {
		panic(fmt.Sprintf("%s : dépôt %d inconnu", op, id))
	}
	return repo
}

// AddCommits ajoute des commits au dépôt repoID, dans l'ordre (chaque parent doit déjà exister dans le
// dépôt ou figurer plus tôt dans l'appel). Les commits sont copiés. Ajouter un commit ne déplace aucune
// branche (voir SetBranch). SHA invalide (provider.ValidateSHA) ou déjà présent : panique.
func (f *Fake) AddCommits(repoID int64, commits ...provider.Commit) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.AddCommits", repoID)
	for _, c := range commits {
		f.addCommitLocked(repo, c)
	}
}

// addCommitLocked ajoute une copie de c à repo. Appelée avec f.mu verrouillé.
func (f *Fake) addCommitLocked(repo *fakeRepo, c provider.Commit) {
	if err := provider.ValidateSHA(c.SHA); err != nil {
		panic("providertest.AddCommits : " + err.Error())
	}
	if _, ok := repo.commits[c.SHA]; ok {
		panic(fmt.Sprintf("providertest.AddCommits : commit %s déjà présent dans le dépôt %d", c.SHA, repo.meta.ID))
	}
	for _, p := range c.Parents {
		if _, ok := repo.commits[p]; !ok {
			panic(fmt.Sprintf("providertest.AddCommits : parent %s du commit %s inconnu dans le dépôt %d",
				provider.ShortSHA(p), provider.ShortSHA(c.SHA), repo.meta.ID))
		}
	}
	c.AuthoredAt, c.CommittedAt = utcSecond(c.AuthoredAt), utcSecond(c.CommittedAt)
	c.Parents = append([]string{}, c.Parents...)
	repo.seq++
	repo.commits[c.SHA] = &fakeCommit{c: c, seq: repo.seq}
}

// AppendCommits ajoute n commits linéaires au-dessus de la tête de branch (ou un historique neuf si la
// branche n'existe pas), de SHA déterministes (SHA de « <repoID>/<branch>/<k> », k = rang du commit
// dans le dépôt, augmenté si besoin) distincts de ceux déjà présents, datés start, start+step…
// (AuthoredAt = CommittedAt, UTC), de messages « commit <k> », d'URL
// « https://github.com/<owner>/<name>/commit/<sha> », par l'auteur factice {Name: « Étudiant Test »,
// Email: « etudiant@example.test », Login: « etudiant-test », ID: 1001, AvatarURL:
// « https://avatars.example.test/u/1001 »} (Committer identique), puis place branch sur le dernier.
// Renvoie les commits créés dans l'ordre de création (du plus ancien au plus récent ; slice vide non
// nil si n vaut 0, sans toucher à la branche). n négatif ou branche invalide : panique.
func (f *Fake) AppendCommits(repoID int64, branch string, n int, start time.Time, step time.Duration) []provider.Commit {
	if err := provider.ValidateBranch(branch); err != nil {
		panic("providertest.AppendCommits : " + err.Error())
	}
	if n < 0 {
		panic(fmt.Sprintf("providertest.AppendCommits : nombre de commits %d négatif", n))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.AppendCommits", repoID)
	author := provider.Person{
		Name:      "Étudiant Test",
		Email:     "etudiant@example.test",
		Login:     "etudiant-test",
		ID:        1001,
		AvatarURL: "https://avatars.example.test/u/1001",
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
		repo.seq = k - 1 // addCommitLocked le porte à k
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

// SetBranch crée la branche name du dépôt repoID ou la déplace (y compris en arrière : force-push)
// vers headSHA, qui doit exister dans le dépôt (sinon panique ; nom invalide aussi).
func (f *Fake) SetBranch(repoID int64, name, headSHA string) {
	if err := provider.ValidateBranch(name); err != nil {
		panic("providertest.SetBranch : " + err.Error())
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetBranch", repoID)
	if _, ok := repo.commits[headSHA]; !ok {
		panic(fmt.Sprintf("providertest.SetBranch : commit %s inconnu dans le dépôt %d", headSHA, repoID))
	}
	repo.branches[name] = headSHA
}

// DeleteBranch supprime la branche name du dépôt repoID (les commits restent). Branche inconnue :
// panique.
func (f *Fake) DeleteBranch(repoID int64, name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.DeleteBranch", repoID)
	if _, ok := repo.branches[name]; !ok {
		panic(fmt.Sprintf("providertest.DeleteBranch : branche %q inconnue dans le dépôt %d", name, repoID))
	}
	delete(repo.branches, name)
}

// RemoveCommit retire le commit sha du dépôt repoID (nettoyage après un force-push) : CompareCommits
// avec ce commit pour base renvoie ensuite ErrUnknownBase. Le commit ne doit plus être la tête d'une
// branche (sinon panique) ; les commits qui le référencent comme parent le gardent dans Parents (le
// parcours de l'historique s'y arrête).
func (f *Fake) RemoveCommit(repoID int64, sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.RemoveCommit", repoID)
	if _, ok := repo.commits[sha]; !ok {
		panic(fmt.Sprintf("providertest.RemoveCommit : commit %s inconnu dans le dépôt %d", sha, repoID))
	}
	for name, head := range repo.branches {
		if head == sha {
			panic(fmt.Sprintf("providertest.RemoveCommit : le commit %s est la tête de la branche %q", sha, name))
		}
	}
	delete(repo.commits, sha)
	delete(repo.stats, sha)
}

// SetStats fixe les statistiques renvoyées par CommitStats pour le commit sha du dépôt repoID (zéro
// par défaut pour un commit connu). Commit inconnu : panique.
func (f *Fake) SetStats(repoID int64, sha string, s provider.CommitStats) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetStats", repoID)
	if _, ok := repo.commits[sha]; !ok {
		panic(fmt.Sprintf("providertest.SetStats : commit %s inconnu dans le dépôt %d", sha, repoID))
	}
	repo.stats[sha] = s
}

// SetPullRequests ajoute ou remplace (par numéro) des pull requests du dépôt repoID. Les dates sont
// converties en UTC et tronquées à la seconde. Numéro <= 0, état autre que provider.PullRequestOpen ou
// provider.PullRequestClosed, UpdatedAt nul ou dépôt inconnu : panique.
func (f *Fake) SetPullRequests(repoID int64, pulls ...provider.PullRequest) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetPullRequests", repoID)
	for _, pr := range pulls {
		switch {
		case pr.Number <= 0:
			panic(fmt.Sprintf("providertest.SetPullRequests : numéro %d invalide", pr.Number))
		case pr.State != provider.PullRequestOpen && pr.State != provider.PullRequestClosed:
			panic(fmt.Sprintf("providertest.SetPullRequests : état %q invalide", pr.State))
		case pr.UpdatedAt.IsZero():
			panic(fmt.Sprintf("providertest.SetPullRequests : PR n° %d sans UpdatedAt", pr.Number))
		}
		pr.CreatedAt, pr.UpdatedAt = utcSecond(pr.CreatedAt), utcSecond(pr.UpdatedAt)
		pr.ClosedAt, pr.MergedAt = utcSecond(pr.ClosedAt), utcSecond(pr.MergedAt)
		repo.pulls[pr.Number] = pr
	}
}

// SetReleases remplace les releases du dépôt repoID (dates en UTC à la seconde). Identifiant <= 0,
// tag vide ou dépôt inconnu : panique.
func (f *Fake) SetReleases(repoID int64, releases ...provider.Release) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetReleases", repoID)
	out := make([]provider.Release, 0, len(releases))
	for _, r := range releases {
		if r.ID <= 0 || r.TagName == "" {
			panic(fmt.Sprintf("providertest.SetReleases : release %d (tag %q) invalide", r.ID, r.TagName))
		}
		r.CreatedAt, r.PublishedAt = utcSecond(r.CreatedAt), utcSecond(r.PublishedAt)
		out = append(out, r)
	}
	repo.releases = out
}

// SetTags remplace les tags du dépôt repoID, renvoyés par Tags dans cet ordre. Nom vide, SHA invalide
// (provider.ValidateSHA) ou dépôt inconnu : panique (le commit visé n'a pas besoin d'exister).
func (f *Fake) SetTags(repoID int64, tags ...provider.Tag) {
	f.mu.Lock()
	defer f.mu.Unlock()
	repo := f.repoLocked("providertest.SetTags", repoID)
	for _, t := range tags {
		if t.Name == "" {
			panic("providertest.SetTags : tag sans nom")
		}
		if err := provider.ValidateSHA(t.SHA); err != nil {
			panic("providertest.SetTags : " + err.Error())
		}
	}
	repo.tags = slices.Clone(tags)
}

// SHA renvoie un SHA-1 factice et déterministe (40 hex minuscules) dérivé de seed.
func SHA(seed string) string {
	sum := sha1.Sum([]byte(seed))
	return hex.EncodeToString(sum[:])
}

// ----- Erreurs injectées -----

// SetError fait échouer tous les appels suivants à m avec err (nil = rétablir le comportement normal).
// Méthode inconnue : panique.
func (f *Fake) SetError(m Method, err error) {
	if !m.valid() {
		panic(fmt.Sprintf("providertest.SetError : méthode %q inconnue", m))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err == nil {
		delete(f.errs, m)
		return
	}
	f.errs[m] = err
}

// FailNext fait échouer le prochain appel à m avec err ; plusieurs FailNext s'enchaînent dans l'ordre
// (file), avant SetError. Méthode inconnue ou err nil : panique.
func (f *Fake) FailNext(m Method, err error) {
	if !m.valid() {
		panic(fmt.Sprintf("providertest.FailNext : méthode %q inconnue", m))
	}
	if err == nil {
		panic("providertest.FailNext : erreur nil")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next[m] = append(f.next[m], err)
}

// SetRepoError fait échouer tous les appels portant sur le dépôt owner/name (sans tenir compte de la
// casse, nom actuel ou ancien), quelle que soit la méthode (RepositoryByID compris, par l'ID du dépôt
// s'il est connu), avec err ; nil = rétablir. Exemple : un *provider.SSORequiredError pour un seul
// dépôt d'une organisation. L'erreur peut être posée avant l'ajout du dépôt.
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

// ----- Appels -----

// Calls renvoie le nombre d'appels reçus par m (y compris ceux qui ont échoué).
func (f *Fake) Calls(m Method) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[m]
}

// CallLog renvoie une copie du journal des appels, dans l'ordre.
func (f *Fake) CallLog() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.log)
}

// ResetCalls vide le journal et les compteurs.
func (f *Fake) ResetCalls() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.log = nil
	clear(f.calls)
}

// ----- Quota simulé -----

// SetRateLimit active le quota simulé avec l'état rl (Reset en UTC, tronqué à la seconde ; zéro =
// maintenant + 1 h ; Limit nul ou négatif = 5 000 ; Used = Limit - Remaining si Used vaut 0 ;
// Remaining négatif : panique). Dès lors, chaque appel de Repository, RepositoryByID, Branches,
// ListCommits, CompareCommits, CommitStats, PullRequests, Releases ou Tags, après les étapes 1 à 5 :
//   - si maintenant >= Reset : nouvelle fenêtre (Remaining = Limit, Used = 0, Reset avancé d'heure en
//     heure jusqu'à dépasser maintenant) ;
//   - si Remaining == 0 : erreur *provider.RateLimitError{Reset} (errors.Is ErrRateLimited), dans un
//     *provider.APIError de code 403 ;
//   - sinon : Remaining--, Used++, et l'appel continue.
//
// LastRateLimit renvoie alors l'état courant (ObservedAt = maintenant) et true ; RateLimit aussi, sans
// consommer. Sans SetRateLimit, LastRateLimit renvoie false (même après RateLimit) et RateLimit
// renvoie {Limit: 5000, Remaining: 5000, Reset: maintenant + 1 h tronqué à la seconde, ObservedAt:
// maintenant}.
func (f *Fake) SetRateLimit(rl provider.RateLimit) {
	if rl.Remaining < 0 {
		panic(fmt.Sprintf("providertest.SetRateLimit : Remaining %d négatif", rl.Remaining))
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

// SetClock remplace l'horloge du Fake (quota, ObservedAt) ; nil = time.Now. L'horloge est appelée sous
// le verrou du Fake.
func (f *Fake) SetClock(now func() time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if now == nil {
		now = time.Now
	}
	f.now = now
}

// renewRate ouvre une nouvelle fenêtre de quota si la précédente est échue. Appelée avec f.mu
// verrouillé et le quota simulé actif.
func (f *Fake) renewRate(now time.Time) {
	if now.Before(f.rate.Reset) {
		return
	}
	f.rate.Remaining, f.rate.Used = f.rate.Limit, 0
	windows := now.Sub(f.rate.Reset)/rateWindow + 1
	f.rate.Reset = f.rate.Reset.Add(windows * rateWindow)
}

// consume décompte un appel sur le quota simulé (étape 6). Appelée avec f.mu verrouillé.
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

// ----- Étapes communes -----

// begin compte l'appel (étape 1) et renvoie ctx.Err() (étape 2).
func (f *Fake) begin(ctx context.Context, c Call) error {
	f.mu.Lock()
	f.calls[c.Method]++
	f.log = append(f.log, c)
	f.mu.Unlock()
	return ctx.Err()
}

// injected renvoie l'erreur injectée pour m (étape 4). Appelée avec f.mu verrouillé.
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

// repoErrByName renvoie l'erreur posée par SetRepoError pour owner/name ou pour l'un des noms du dépôt
// qu'il désigne (étape 5). Appelée avec f.mu verrouillé.
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

// repoErrByID renvoie l'erreur posée par SetRepoError pour l'un des noms du dépôt id. Appelée avec
// f.mu verrouillé.
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

// prepare enchaîne les étapes 4 à 6 pour un appel portant sur owner/name. Appelée avec f.mu verrouillé.
func (f *Fake) prepare(m Method, owner, name, op, path string) error {
	if err := f.injected(m); err != nil {
		return err
	}
	if err := f.repoErrByName(owner, name); err != nil {
		return err
	}
	return f.consume(op, path)
}

// lookup renvoie le dépôt désigné par owner/name (nom actuel ou ancien), ou nil. Appelée avec f.mu
// verrouillé.
func (f *Fake) lookup(owner, name string) *fakeRepo {
	id, ok := f.names[nameKey(owner, name)]
	if !ok {
		return nil
	}
	return f.repos[id]
}

// apiError renvoie l'erreur d'une requête GET simulée.
func apiError(op, path string, status int, err error) error {
	return &provider.APIError{Op: op, Method: "GET", Path: path, Status: status, Err: err}
}

// repoPath renvoie le chemin de l'API du dépôt owner/name, tel que passé.
func repoPath(owner, name string) string {
	return "/repos/" + owner + "/" + name
}

// copyCommit renvoie une copie indépendante de c (Parents non nil).
func copyCommit(c provider.Commit) provider.Commit {
	c.Parents = append([]string{}, c.Parents...)
	return c
}

// ----- Graphe des commits -----

// resolve résout ref en SHA : nom de branche d'abord, puis SHA complet d'un commit présent.
func (r *fakeRepo) resolve(ref string) (string, bool) {
	if sha, ok := r.branches[ref]; ok {
		return sha, true
	}
	if _, ok := r.commits[ref]; ok {
		return ref, true
	}
	return "", false
}

// reachable renvoie l'ensemble des SHA atteignables depuis head par les parents (head compris), en
// ignorant les commits retirés (RemoveCommit).
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

// oldestFirst ordonne deux commits du plus ancien au plus récent : CommittedAt, puis ordre d'ajout.
func oldestFirst(a, b *fakeCommit) int {
	if c := a.c.CommittedAt.Compare(b.c.CommittedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.seq, b.seq)
}

// sorted renvoie les commits de set du plus ancien au plus récent (oldestFirst).
func (r *fakeRepo) sorted(set map[string]bool) []*fakeCommit {
	out := make([]*fakeCommit, 0, len(set))
	for sha := range set {
		out = append(out, r.commits[sha])
	}
	slices.SortFunc(out, oldestFirst)
	return out
}

// ----- provider.Provider -----

// Name implémente provider.Provider : « github » (model.ProviderGitHub), car le Fake tient lieu de
// GitHub dans les tests.
func (f *Fake) Name() string { return model.ProviderGitHub }

// Repository implémente provider.Provider : dépôt trouvé par son nom actuel ou un ancien nom (les
// valeurs renvoyées sont les actuelles) ; inconnu → erreur enveloppant provider.ErrNotFound.
func (f *Fake) Repository(ctx context.Context, owner, name string) (provider.Repository, error) {
	if err := f.begin(ctx, Call{Method: MethodRepository, Owner: owner, Name: name}); err != nil {
		return provider.Repository{}, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.Repository{}, err
	}
	op, path := fmt.Sprintf("lecture du dépôt %s/%s", owner, name), repoPath(owner, name)
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

// RepositoryByID implémente provider.Provider ; id inconnu → provider.ErrNotFound.
func (f *Fake) RepositoryByID(ctx context.Context, id int64) (provider.Repository, error) {
	if err := f.begin(ctx, Call{Method: MethodRepositoryByID, ID: id}); err != nil {
		return provider.Repository{}, err
	}
	if err := provider.ValidateID(id); err != nil {
		return provider.Repository{}, err
	}
	op, path := fmt.Sprintf("lecture du dépôt n° %d", id), fmt.Sprintf("/repositories/%d", id)
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

// Branches implémente provider.Provider : branches triées par nom, au plus provider.MaxBranches ;
// dépôt vide → liste vide non nil.
func (f *Fake) Branches(ctx context.Context, owner, name string) ([]provider.Branch, error) {
	if err := f.begin(ctx, Call{Method: MethodBranches, Owner: owner, Name: name}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := fmt.Sprintf("liste des branches de %s/%s", owner, name), repoPath(owner, name)+"/branches"
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

// ListCommits implémente provider.Provider : commits atteignables depuis opts.Branch (nom de branche ou
// SHA complet connu ; vide = DefaultBranch) par les parents, triés par CommittedAt décroissant (à date
// égale, le dernier ajouté d'abord), filtrés par Since (CommittedAt >= Since tronqué à la seconde,
// comme le since envoyé à GitHub), coupés au premier commit pour lequel Stop renvoie true (exclu ; Stop
// est appelé hors du verrou du Fake, dans l'ordre, et plus après la limite) et à la limite effective
// (Limit, au plus provider.MaxCommits). Branche inconnue → provider.ErrNotFound ; dépôt vide (aucune
// branche) → liste vide non nil, sans erreur.
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

// listCandidates applique les étapes 4 à 7 de ListCommits sous le verrou et renvoie les commits filtrés
// par Since, du plus récent au plus ancien (avant Stop et Limit).
func (f *Fake) listCandidates(owner, name string, opts provider.ListCommitsOptions) ([]*fakeCommit, error) {
	op, path := fmt.Sprintf("liste des commits de %s/%s", owner, name), repoPath(owner, name)+"/commits"
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
		return nil, nil // dépôt vide : liste vide, sans erreur
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
	// Comme le Provider GitHub, qui envoie since au format RFC 3339 à la seconde : Since est tronqué à
	// la seconde (les dates des commits le sont déjà), et GitHub garde les commits de cette seconde.
	since := utcSecond(opts.Since)
	return slices.DeleteFunc(all, func(c *fakeCommit) bool { return c.c.CommittedAt.Before(since) }), nil
}

// CompareCommits implémente provider.Provider : commits atteignables depuis head mais pas depuis base,
// du plus récent au plus ancien (même ordre que ListCommits), au plus provider.MaxCommits (comme le
// Provider GitHub, qui lit les pages du plus ancien au plus récent : au-delà, ce sont les plus
// anciens qui sont gardés) ; base == head (sans aucun contrôle après la validation, voir l'ordre des
// contrôles) ou head contenu dans base → liste vide non nil ; dépôt vide (aucune branche) →
// provider.ErrEmptyRepository ; base ou head inconnu, ou aucun ancêtre commun entre base et head
// (GitHub répond 404 dans les trois cas, et le Provider GitHub, qui ne peut pas les distinguer sur un
// dépôt accessible, renvoie ErrUnknownBase) → provider.ErrUnknownBase (et pas ErrNotFound) ; dépôt
// inconnu → provider.ErrNotFound.
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
	op := fmt.Sprintf("comparaison %s...%s dans %s/%s", provider.ShortSHA(base), provider.ShortSHA(head), owner, name)
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

// CommitStats implémente provider.Provider : statistiques fixées par SetStats (zéro par défaut) ;
// dépôt vide (aucune branche) → provider.ErrEmptyRepository ; commit inconnu → provider.ErrNotFound.
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
	op := fmt.Sprintf("statistiques du commit %s de %s/%s", provider.ShortSHA(sha), owner, name)
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

// PullRequests implémente provider.Provider : pull requests posées par SetPullRequests, triées par
// UpdatedAt décroissant (à date égale, numéro décroissant), coupées à la première modifiée strictement
// avant opts.UpdatedSince tronqué à la seconde (exclue) et à la limite effective (Limit, au plus
// provider.MaxPullRequests) ; dépôt inconnu → provider.ErrNotFound. Un dépôt vide (aucune branche)
// renvoie ses PR comme les autres (GitHub répond 200).
func (f *Fake) PullRequests(ctx context.Context, owner, name string, opts provider.PullRequestsOptions) ([]provider.PullRequest, error) {
	if err := f.begin(ctx, Call{Method: MethodPullRequests, Owner: owner, Name: name, PullOpts: opts}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := fmt.Sprintf("liste des pull requests de %s/%s", owner, name), repoPath(owner, name)+"/pulls"
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

// Releases implémente provider.Provider : releases posées par SetReleases, triées par CreatedAt
// décroissant (à date égale, identifiant décroissant), au plus provider.MaxReleases ; dépôt inconnu →
// provider.ErrNotFound.
func (f *Fake) Releases(ctx context.Context, owner, name string) ([]provider.Release, error) {
	if err := f.begin(ctx, Call{Method: MethodReleases, Owner: owner, Name: name}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := fmt.Sprintf("liste des releases de %s/%s", owner, name), repoPath(owner, name)+"/releases"
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

// Tags implémente provider.Provider : tags posés par SetTags, dans leur ordre, au plus
// provider.MaxTags ; dépôt inconnu → provider.ErrNotFound.
func (f *Fake) Tags(ctx context.Context, owner, name string) ([]provider.Tag, error) {
	if err := f.begin(ctx, Call{Method: MethodTags, Owner: owner, Name: name}); err != nil {
		return nil, err
	}
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op, path := fmt.Sprintf("liste des tags de %s/%s", owner, name), repoPath(owner, name)+"/tags"
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

// RateLimit implémente provider.Provider (voir SetRateLimit) ; ne consomme pas de quota. Seules les
// étapes 1, 2 et 4 s'appliquent.
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

// LastRateLimit implémente provider.Provider (voir SetRateLimit) ; ni compté ni injectable.
func (f *Fake) LastRateLimit() (provider.RateLimit, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.rateOn {
		return provider.RateLimit{}, false
	}
	return f.currentRate(f.clock()), true
}

// currentRate renvoie l'état du quota simulé à now (fenêtre renouvelée si besoin). Appelée avec f.mu
// verrouillé et le quota simulé actif.
func (f *Fake) currentRate(now time.Time) provider.RateLimit {
	f.renewRate(now)
	rl := f.rate
	rl.ObservedAt = now
	return rl
}
