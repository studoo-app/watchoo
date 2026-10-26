package githubtest

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"strconv"
	"strings"
	"time"
)

// Corps JSON au format de l'API REST de GitHub (champs utiles à Watchoo et principaux champs
// voisins, pour que les réponses ressemblent à celles de l'API réelle).

// githubWeb est la racine des pages web (html_url), comme sur GitHub.
const githubWeb = "https://github.com/"

// timeJSON formate une date comme GitHub (ISO 8601, UTC, à la seconde).
func timeJSON(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// nullableTime renvoie timeJSON(t), ou nil (null) si t est zéro.
func nullableTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := timeJSON(t)
	return &s
}

// marshal encode v en JSON sans échapper < > & (comme GitHub). Une erreur d'encodage est un défaut
// du faux serveur : elle fait échouer le test.
func (s *Server) marshal(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		s.tb.Errorf("githubtest : encodage JSON : %v", err)
		return []byte("{}")
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// errorBody est le corps d'une réponse d'erreur de GitHub.
type errorBody struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
	Status           string `json:"status"`
}

// errorJSON renvoie le corps d'erreur {"message","documentation_url","status"}.
func errorJSON(message, docs string, status int) errorBody {
	return errorBody{Message: message, DocumentationURL: docs, Status: strconv.Itoa(status)}
}

// accountID dérive un identifiant de compte stable (et non nul) d'un login, sans tenir compte de la
// casse.
func accountID(login string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(login)))
	return int64(h.Sum32()) + 1
}

// avatarURL renvoie l'avatar factice du compte id.
func avatarURL(id int64) string {
	return "https://avatars.example.test/u/" + strconv.FormatInt(id, 10)
}

// userJSON est un compte GitHub (« simple user »).
type userJSON struct {
	Login      string `json:"login"`
	ID         int64  `json:"id"`
	NodeID     string `json:"node_id"`
	AvatarURL  string `json:"avatar_url"`
	GravatarID string `json:"gravatar_id"`
	URL        string `json:"url"`
	HTMLURL    string `json:"html_url"`
	Type       string `json:"type"`
	SiteAdmin  bool   `json:"site_admin"`
}

// user renvoie le compte login (id 0 = dérivé du login ; avatar vide = avatar factice).
func (s *Server) user(login string, id int64, avatar string) userJSON {
	if id == 0 {
		id = accountID(login)
	}
	if avatar == "" {
		avatar = avatarURL(id)
	}
	return userJSON{
		Login:      login,
		ID:         id,
		NodeID:     "U_fake" + strconv.FormatInt(id, 10),
		AvatarURL:  avatar,
		GravatarID: "",
		URL:        s.URL() + "users/" + login,
		HTMLURL:    githubWeb + login,
		Type:       "User",
		SiteAdmin:  false,
	}
}

// personUser renvoie le compte lié à p, ou nil (null) si aucun compte n'est lié.
func (s *Server) personUser(p Person) *userJSON {
	if p.Login == "" {
		return nil
	}
	u := s.user(p.Login, p.ID, p.AvatarURL)
	return &u
}

// permissionsJSON donne les droits du compte sur le dépôt (lecture seule).
type permissionsJSON struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Triage   bool `json:"triage"`
	Pull     bool `json:"pull"`
}

// repoJSON est un dépôt (« full repository »).
type repoJSON struct {
	ID              int64           `json:"id"`
	NodeID          string          `json:"node_id"`
	Name            string          `json:"name"`
	FullName        string          `json:"full_name"`
	Private         bool            `json:"private"`
	Owner           userJSON        `json:"owner"`
	HTMLURL         string          `json:"html_url"`
	Description     *string         `json:"description"`
	Fork            bool            `json:"fork"`
	URL             string          `json:"url"`
	CreatedAt       *string         `json:"created_at"`
	UpdatedAt       *string         `json:"updated_at"`
	PushedAt        *string         `json:"pushed_at"`
	GitURL          string          `json:"git_url"`
	SSHURL          string          `json:"ssh_url"`
	CloneURL        string          `json:"clone_url"`
	Size            int             `json:"size"`
	StargazersCount int             `json:"stargazers_count"`
	WatchersCount   int             `json:"watchers_count"`
	Language        *string         `json:"language"`
	ForksCount      int             `json:"forks_count"`
	Archived        bool            `json:"archived"`
	Disabled        bool            `json:"disabled"`
	OpenIssuesCount int             `json:"open_issues_count"`
	Visibility      string          `json:"visibility"`
	Forks           int             `json:"forks"`
	OpenIssues      int             `json:"open_issues"`
	Watchers        int             `json:"watchers"`
	DefaultBranch   string          `json:"default_branch"`
	Permissions     permissionsJSON `json:"permissions"`
}

// repoJSON renvoie les métadonnées de repo, avec ses owner/nom actuels.
func (s *Server) repoJSON(repo *repoState) repoJSON {
	full := repo.Owner + "/" + repo.Name
	visibility := "public"
	if repo.Private {
		visibility = "private"
	}
	pushed := nullableTime(repo.PushedAt)
	return repoJSON{
		ID:              repo.ID,
		NodeID:          "R_fake" + strconv.FormatInt(repo.ID, 10),
		Name:            repo.Name,
		FullName:        full,
		Private:         repo.Private,
		Owner:           s.user(repo.Owner, 0, ""),
		HTMLURL:         githubWeb + full,
		Fork:            false,
		URL:             s.URL() + "repos/" + full,
		CreatedAt:       pushed,
		UpdatedAt:       pushed,
		PushedAt:        pushed,
		GitURL:          "git://github.com/" + full + ".git",
		SSHURL:          "git@github.com:" + full + ".git",
		CloneURL:        githubWeb + full + ".git",
		StargazersCount: repo.Stars,
		WatchersCount:   repo.Stars,
		Archived:        repo.Archived,
		OpenIssuesCount: repo.OpenIssues,
		Visibility:      visibility,
		OpenIssues:      repo.OpenIssues,
		Watchers:        repo.Stars,
		DefaultBranch:   repo.DefaultBranch,
		Permissions:     permissionsJSON{Pull: true},
	}
}

// branchJSON est une branche (« short branch »).
type branchJSON struct {
	Name      string           `json:"name"`
	Commit    branchCommitJSON `json:"commit"`
	Protected bool             `json:"protected"`
}

// branchCommitJSON est le commit de tête d'une branche.
type branchCommitJSON struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}

// branchJSON renvoie la branche name de repo.
func (s *Server) branchJSON(repo *repoState, name string) branchJSON {
	sha := repo.branches[name]
	return branchJSON{
		Name:   name,
		Commit: branchCommitJSON{SHA: sha, URL: s.URL() + "repos/" + repo.Owner + "/" + repo.Name + "/commits/" + sha},
	}
}

// gitPersonJSON est l'identité Git d'un auteur ou d'un committer.
type gitPersonJSON struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

// treeJSON désigne l'arbre d'un commit.
type treeJSON struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}

// verificationJSON est l'état de signature d'un commit (jamais signé ici).
type verificationJSON struct {
	Verified   bool    `json:"verified"`
	Reason     string  `json:"reason"`
	Signature  *string `json:"signature"`
	Payload    *string `json:"payload"`
	VerifiedAt *string `json:"verified_at"`
}

// gitCommitJSON est la partie Git d'un commit.
type gitCommitJSON struct {
	Author       gitPersonJSON    `json:"author"`
	Committer    gitPersonJSON    `json:"committer"`
	Message      string           `json:"message"`
	Tree         treeJSON         `json:"tree"`
	URL          string           `json:"url"`
	CommentCount int              `json:"comment_count"`
	Verification verificationJSON `json:"verification"`
}

// parentJSON désigne un parent d'un commit.
type parentJSON struct {
	SHA     string `json:"sha"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url"`
}

// statsJSON est la taille d'un commit (GET …/commits/{sha} seulement).
type statsJSON struct {
	Total     int `json:"total"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// commitJSON est un commit (« commit » de l'API REST).
type commitJSON struct {
	SHA         string          `json:"sha"`
	NodeID      string          `json:"node_id"`
	Commit      gitCommitJSON   `json:"commit"`
	URL         string          `json:"url"`
	HTMLURL     string          `json:"html_url"`
	CommentsURL string          `json:"comments_url"`
	Author      *userJSON       `json:"author"`
	Committer   *userJSON       `json:"committer"`
	Parents     []parentJSON    `json:"parents"`
	Stats       *statsJSON      `json:"stats,omitempty"`
	Files       json.RawMessage `json:"files,omitempty"`
}

// commitJSON renvoie le commit c de repo ; withStats ajoute "stats" et "files":[] (GET d'un commit).
func (s *Server) commitJSON(repo *repoState, c *commitState, withStats bool) commitJSON {
	api := s.URL() + "repos/" + repo.Owner + "/" + repo.Name
	web := githubWeb + repo.Owner + "/" + repo.Name
	parents := make([]parentJSON, 0, len(c.Parents))
	for _, p := range c.Parents {
		parents = append(parents, parentJSON{SHA: p, URL: api + "/commits/" + p, HTMLURL: web + "/commit/" + p})
	}
	tree := FakeSHA("tree/" + c.SHA)
	out := commitJSON{
		SHA:    c.SHA,
		NodeID: "C_fake" + c.SHA[:12],
		Commit: gitCommitJSON{
			Author:       gitPersonJSON{Name: c.Author.Name, Email: c.Author.Email, Date: timeJSON(c.AuthoredAt)},
			Committer:    gitPersonJSON{Name: c.Committer.Name, Email: c.Committer.Email, Date: timeJSON(c.CommittedAt)},
			Message:      c.Message,
			Tree:         treeJSON{SHA: tree, URL: api + "/git/trees/" + tree},
			URL:          api + "/git/commits/" + c.SHA,
			Verification: verificationJSON{Reason: "unsigned"},
		},
		URL:         api + "/commits/" + c.SHA,
		HTMLURL:     web + "/commit/" + c.SHA,
		CommentsURL: api + "/commits/" + c.SHA + "/comments",
		Author:      s.personUser(c.Author),
		Committer:   s.personUser(c.Committer),
		Parents:     parents,
	}
	if withStats {
		out.Stats = &statsJSON{Total: c.Additions + c.Deletions, Additions: c.Additions, Deletions: c.Deletions}
		out.Files = json.RawMessage("[]")
	}
	return out
}

// compareJSON est le résultat d'une comparaison (« commit comparison »).
type compareJSON struct {
	URL             string       `json:"url"`
	HTMLURL         string       `json:"html_url"`
	PermalinkURL    string       `json:"permalink_url"`
	DiffURL         string       `json:"diff_url"`
	PatchURL        string       `json:"patch_url"`
	BaseCommit      commitJSON   `json:"base_commit"`
	MergeBaseCommit commitJSON   `json:"merge_base_commit"`
	Status          string       `json:"status"`
	AheadBy         int          `json:"ahead_by"`
	BehindBy        int          `json:"behind_by"`
	TotalCommits    int          `json:"total_commits"`
	Commits         []commitJSON `json:"commits"`
	Files           []struct{}   `json:"files"`
}

// compareJSON renvoie la comparaison spec (« base...head ») de repo.
func (s *Server) compareJSON(repo *repoState, spec string, base, head, mergeBase *commitState, status string,
	ahead, behind int, commits []commitJSON) compareJSON {
	full := repo.Owner + "/" + repo.Name
	web := githubWeb + full + "/compare/" + spec
	return compareJSON{
		URL:             s.URL() + "repos/" + full + "/compare/" + spec,
		HTMLURL:         web,
		PermalinkURL:    githubWeb + full + "/compare/" + repo.Owner + ":" + base.SHA[:7] + "..." + repo.Owner + ":" + head.SHA[:7],
		DiffURL:         web + ".diff",
		PatchURL:        web + ".patch",
		BaseCommit:      s.commitJSON(repo, base, false),
		MergeBaseCommit: s.commitJSON(repo, mergeBase, false),
		Status:          status,
		AheadBy:         ahead,
		BehindBy:        behind,
		TotalCommits:    ahead,
		Commits:         commits,
		Files:           []struct{}{},
	}
}

// rateJSON est l'état d'une ressource de quota.
type rateJSON struct {
	Limit     int   `json:"limit"`
	Used      int   `json:"used"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

// rateLimitBody est le corps de GET /rate_limit.
type rateLimitBody struct {
	Resources map[string]rateJSON `json:"resources"`
	Rate      rateJSON            `json:"rate"`
}

// rateLimitJSON renvoie le corps de GET /rate_limit : quota core simulé, autres ressources pleines
// (remises à zéro avec core, pour que le corps et son ETag ne changent qu'avec le quota). Appelée
// avec s.mu verrouillé.
func (s *Server) rateLimitJSON() rateLimitBody {
	reset := s.quota.reset.Unix()
	core := rateJSON{Limit: s.quota.limit, Used: s.quota.used, Remaining: s.quota.remaining, Reset: reset}
	return rateLimitBody{
		Resources: map[string]rateJSON{
			"core":    core,
			"graphql": {Limit: DefaultRateLimit, Remaining: DefaultRateLimit, Reset: reset},
			"search":  {Limit: 30, Remaining: 30, Reset: reset},
		},
		Rate: core,
	}
}
