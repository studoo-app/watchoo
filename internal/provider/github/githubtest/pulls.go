package githubtest

// Pull requests, releases et tags simulés (V2) :
//   - GET …/pulls?state&sort&direction&per_page&page : state open (par défaut), closed ou all ;
//     sort created (par défaut) ou updated ; direction desc par défaut avec sort=created, asc sinon
//     (comme GitHub) ; à date égale, numéro croissant dans le sens de direction ; valeur inconnue →
//     422 « Validation Failed ». Corps : objets « pull request simple » (number, state, title, user,
//     draft, html_url, base.ref, head.ref, created_at, updated_at, closed_at, merged_at ; la liste de
//     GitHub n'a pas de champ merged) ;
//   - GET …/releases?per_page&page : releases triées par created_at décroissant (à date égale,
//     identifiant décroissant), brouillons compris (le faux compte a toujours le droit d'écriture) ;
//   - GET …/tags?per_page&page : tags dans l'ordre de SetTags.
//
// Un dépôt vide (sans branche) répond 200 avec ses éléments, comme GitHub (aucun 409 sur ces routes).

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// docsPulls est le lien de documentation des erreurs de la liste des pull requests.
const docsPulls = "https://docs.github.com/rest/pulls/pulls#list-pull-requests"

// PullRequest est une pull request simulée.
type PullRequest struct {
	Number    int       // numéro (> 0, unique dans le dépôt)
	Title     string    // titre
	Closed    bool      // fermée ("state": "closed") ; faux = ouverte
	Draft     bool      // brouillon
	Author    string    // login de l'auteur ; vide = "user": null (compte supprimé)
	Base      string    // branche visée ; vide = branche par défaut
	Head      string    // branche proposée ; vide = « feature-<numéro> »
	CreatedAt time.Time // ouverture (non nulle)
	UpdatedAt time.Time // dernière modification ; zéro = CreatedAt
	ClosedAt  time.Time // fermeture ; zéro = null (Closed exige une date : zéro = UpdatedAt)
	MergedAt  time.Time // merge ; zéro = null
}

// Release est une release simulée.
type Release struct {
	ID          int64     // identifiant (> 0, unique dans le dépôt)
	TagName     string    // tag (non vide)
	Name        string    // titre ; vide = null
	Draft       bool      // brouillon
	Prerelease  bool      // pré-release
	Author      string    // login de l'auteur ; vide = "author": null
	CreatedAt   time.Time // création (non nulle)
	PublishedAt time.Time // publication ; zéro = null
}

// Tag est un tag simulé.
type Tag struct {
	Name string // nom (non vide)
	SHA  string // SHA complet du commit visé (pas forcément ajouté au dépôt)
}

// SetPullRequests ajoute ou remplace (par numéro) des pull requests du dépôt repoID. Dates converties
// en UTC et tronquées à la seconde. Numéro <= 0, CreatedAt nul ou dépôt inconnu : t.Fatalf.
func (s *Server) SetPullRequests(repoID int64, pulls ...PullRequest) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetPullRequests", repoID)
	if repo.pulls == nil {
		repo.pulls = make(map[int]PullRequest)
	}
	for _, pr := range pulls {
		if pr.Number <= 0 || pr.CreatedAt.IsZero() {
			s.fatalf("SetPullRequests : PR n° %d invalide (numéro > 0 et CreatedAt attendus)", pr.Number)
		}
		pr.CreatedAt, pr.UpdatedAt = utcSecond(pr.CreatedAt), utcSecond(pr.UpdatedAt)
		pr.ClosedAt, pr.MergedAt = utcSecond(pr.ClosedAt), utcSecond(pr.MergedAt)
		if pr.UpdatedAt.IsZero() {
			pr.UpdatedAt = pr.CreatedAt
		}
		if pr.Closed && pr.ClosedAt.IsZero() {
			pr.ClosedAt = pr.UpdatedAt
		}
		repo.pulls[pr.Number] = pr
	}
}

// SetReleases remplace les releases du dépôt repoID. Identifiant <= 0 ou en double, tag vide,
// CreatedAt nul ou dépôt inconnu : t.Fatalf.
func (s *Server) SetReleases(repoID int64, releases ...Release) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetReleases", repoID)
	seen := make(map[int64]bool, len(releases))
	out := make([]Release, 0, len(releases))
	for _, r := range releases {
		if r.ID <= 0 || seen[r.ID] || r.TagName == "" || r.CreatedAt.IsZero() {
			s.fatalf("SetReleases : release %d (tag %q) invalide", r.ID, r.TagName)
		}
		seen[r.ID] = true
		r.CreatedAt, r.PublishedAt = utcSecond(r.CreatedAt), utcSecond(r.PublishedAt)
		out = append(out, r)
	}
	repo.releases = out
}

// SetTags remplace les tags du dépôt repoID, servis dans cet ordre. Nom vide, SHA mal formé ou dépôt
// inconnu : t.Fatalf.
func (s *Server) SetTags(repoID int64, tags ...Tag) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetTags", repoID)
	for _, t := range tags {
		if t.Name == "" || !shaPattern.MatchString(t.SHA) {
			s.fatalf("SetTags : tag %q (SHA %q) invalide", t.Name, t.SHA)
		}
	}
	repo.tags = slices.Clone(tags)
}

// ----- Réponses -----

// pullsReply sert GET …/pulls.
func (s *Server) pullsReply(repo *repoState, q url.Values, linkBase string, now time.Time) reply {
	state := q.Get("state")
	if state == "" {
		state = "open"
	}
	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = "created"
	}
	direction := q.Get("direction")
	if direction == "" {
		direction = "asc"
		if sortBy == "created" {
			direction = "desc"
		}
	}
	if (state != "open" && state != "closed" && state != "all") ||
		(sortBy != "created" && sortBy != "updated") || (direction != "asc" && direction != "desc") {
		return s.jsonReply(now, http.StatusUnprocessableEntity, errorJSON(messageValidation, docsPulls, http.StatusUnprocessableEntity))
	}
	list := make([]PullRequest, 0, len(repo.pulls))
	for _, pr := range repo.pulls {
		if state == "all" || (state == "closed") == pr.Closed {
			list = append(list, pr)
		}
	}
	slices.SortFunc(list, func(a, b PullRequest) int {
		ta, tb := a.CreatedAt, b.CreatedAt
		if sortBy == "updated" {
			ta, tb = a.UpdatedAt, b.UpdatedAt
		}
		c := cmp.Or(ta.Compare(tb), cmp.Compare(a.Number, b.Number))
		if direction == "desc" {
			return -c
		}
		return c
	})
	p := paginate(len(list), q)
	items := make([]pullJSON, 0, p.end-p.start)
	for _, pr := range list[p.start:p.end] {
		items = append(items, s.pullJSON(repo, pr))
	}
	rep := s.jsonReply(now, http.StatusOK, items)
	setLink(rep.header, linkBase, q, p)
	return rep
}

// releasesReply sert GET …/releases.
func (s *Server) releasesReply(repo *repoState, q url.Values, linkBase string, now time.Time) reply {
	list := slices.Clone(repo.releases)
	slices.SortStableFunc(list, func(a, b Release) int {
		return cmp.Or(b.CreatedAt.Compare(a.CreatedAt), cmp.Compare(b.ID, a.ID))
	})
	p := paginate(len(list), q)
	items := make([]releaseJSON, 0, p.end-p.start)
	for _, r := range list[p.start:p.end] {
		items = append(items, s.releaseJSON(repo, r))
	}
	rep := s.jsonReply(now, http.StatusOK, items)
	setLink(rep.header, linkBase, q, p)
	return rep
}

// tagsReply sert GET …/tags.
func (s *Server) tagsReply(repo *repoState, q url.Values, linkBase string, now time.Time) reply {
	p := paginate(len(repo.tags), q)
	items := make([]tagJSON, 0, p.end-p.start)
	api := s.URL() + "repos/" + repo.Owner + "/" + repo.Name
	for _, t := range repo.tags[p.start:p.end] {
		esc := url.PathEscape(t.Name)
		items = append(items, tagJSON{
			Name:       t.Name,
			Commit:     branchCommitJSON{SHA: t.SHA, URL: api + "/commits/" + t.SHA},
			ZipballURL: api + "/zipball/refs/tags/" + esc,
			TarballURL: api + "/tarball/refs/tags/" + esc,
			NodeID:     "REF_fake" + t.SHA[:12],
		})
	}
	rep := s.jsonReply(now, http.StatusOK, items)
	setLink(rep.header, linkBase, q, p)
	return rep
}

// ----- JSON -----

// pullRefJSON est la base ou la tête d'une pull request.
type pullRefJSON struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
}

// pullJSON est une pull request (« pull request simple »).
type pullJSON struct {
	URL       string      `json:"url"`
	ID        int64       `json:"id"`
	NodeID    string      `json:"node_id"`
	HTMLURL   string      `json:"html_url"`
	Number    int         `json:"number"`
	State     string      `json:"state"`
	Locked    bool        `json:"locked"`
	Title     string      `json:"title"`
	User      *userJSON   `json:"user"`
	Body      *string     `json:"body"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
	ClosedAt  *string     `json:"closed_at"`
	MergedAt  *string     `json:"merged_at"`
	Draft     bool        `json:"draft"`
	Head      pullRefJSON `json:"head"`
	Base      pullRefJSON `json:"base"`
}

// pullJSON renvoie la pull request pr de repo.
func (s *Server) pullJSON(repo *repoState, pr PullRequest) pullJSON {
	full := repo.Owner + "/" + repo.Name
	state := "open"
	if pr.Closed {
		state = "closed"
	}
	base := pr.Base
	if base == "" {
		base = repo.DefaultBranch
	}
	head := pr.Head
	if head == "" {
		head = fmt.Sprintf("feature-%d", pr.Number)
	}
	var user *userJSON
	if pr.Author != "" {
		u := s.user(pr.Author, 0, "")
		user = &u
	}
	return pullJSON{
		URL:       s.URL() + "repos/" + full + "/pulls/" + strconv.Itoa(pr.Number),
		ID:        repo.ID*100000 + int64(pr.Number),
		NodeID:    "PR_fake" + strconv.FormatInt(repo.ID, 10) + "_" + strconv.Itoa(pr.Number),
		HTMLURL:   githubWeb + full + "/pull/" + strconv.Itoa(pr.Number),
		Number:    pr.Number,
		State:     state,
		Title:     pr.Title,
		User:      user,
		CreatedAt: timeJSON(pr.CreatedAt),
		UpdatedAt: timeJSON(pr.UpdatedAt),
		ClosedAt:  nullableTime(pr.ClosedAt),
		MergedAt:  nullableTime(pr.MergedAt),
		Draft:     pr.Draft,
		Head:      pullRefJSON{Label: repo.Owner + ":" + head, Ref: head, SHA: repo.branches[head]},
		Base:      pullRefJSON{Label: repo.Owner + ":" + base, Ref: base, SHA: repo.branches[base]},
	}
}

// releaseJSON est une release.
type releaseJSON struct {
	URL         string     `json:"url"`
	HTMLURL     string     `json:"html_url"`
	ID          int64      `json:"id"`
	NodeID      string     `json:"node_id"`
	TagName     string     `json:"tag_name"`
	Name        *string    `json:"name"`
	Draft       bool       `json:"draft"`
	Prerelease  bool       `json:"prerelease"`
	Author      *userJSON  `json:"author"`
	CreatedAt   string     `json:"created_at"`
	PublishedAt *string    `json:"published_at"`
	Assets      []struct{} `json:"assets"`
	Body        *string    `json:"body"`
}

// releaseJSON renvoie la release r de repo.
func (s *Server) releaseJSON(repo *repoState, r Release) releaseJSON {
	full := repo.Owner + "/" + repo.Name
	var name *string
	if r.Name != "" {
		n := r.Name
		name = &n
	}
	var author *userJSON
	if r.Author != "" {
		u := s.user(r.Author, 0, "")
		author = &u
	}
	return releaseJSON{
		URL:         s.URL() + "repos/" + full + "/releases/" + strconv.FormatInt(r.ID, 10),
		HTMLURL:     githubWeb + full + "/releases/tag/" + url.PathEscape(r.TagName),
		ID:          r.ID,
		NodeID:      "RE_fake" + strconv.FormatInt(r.ID, 10),
		TagName:     r.TagName,
		Name:        name,
		Draft:       r.Draft,
		Prerelease:  r.Prerelease,
		Author:      author,
		CreatedAt:   timeJSON(r.CreatedAt),
		PublishedAt: nullableTime(r.PublishedAt),
		Assets:      []struct{}{},
	}
}

// tagJSON est un tag.
type tagJSON struct {
	Name       string           `json:"name"`
	Commit     branchCommitJSON `json:"commit"`
	ZipballURL string           `json:"zipball_url"`
	TarballURL string           `json:"tarball_url"`
	NodeID     string           `json:"node_id"`
}
