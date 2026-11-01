package githubtest

import (
	"bytes"
	"encoding/json"
	"hash/fnv"
	"strconv"
	"strings"
	"time"
)

// JSON bodies in the format of GitHub's REST API (fields useful to Watchoo and the main neighboring
// fields, so that the responses look like those of the real API).

// githubWeb is the root of the web pages (html_url), as on GitHub.
const githubWeb = "https://github.com/"

// timeJSON formats a date like GitHub (ISO 8601, UTC, to the second).
func timeJSON(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// nullableTime returns timeJSON(t), or nil (null) if t is zero.
func nullableTime(t time.Time) *string {
	if t.IsZero() {
		return nil
	}
	s := timeJSON(t)
	return &s
}

// marshal encodes v as JSON without escaping < > & (like GitHub). An encoding error is a defect of
// the fake server: it fails the test.
func (s *Server) marshal(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		s.tb.Errorf("githubtest: JSON encoding: %v", err)
		return []byte("{}")
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// errorBody is the body of a GitHub error response.
type errorBody struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
	Status           string `json:"status"`
}

// errorJSON returns the error body {"message","documentation_url","status"}.
func errorJSON(message, docs string, status int) errorBody {
	return errorBody{Message: message, DocumentationURL: docs, Status: strconv.Itoa(status)}
}

// accountID derives a stable (and non-zero) account identifier from a login, case-insensitively.
func accountID(login string) int64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(login)))
	return int64(h.Sum32()) + 1
}

// avatarURL returns the dummy avatar of the account id.
func avatarURL(id int64) string {
	return "https://avatars.example.test/u/" + strconv.FormatInt(id, 10)
}

// userJSON is a GitHub account ("simple user").
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

// user returns the account login (id 0 = derived from the login; empty avatar = dummy avatar).
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

// personUser returns the account linked to p, or nil (null) if no account is linked.
func (s *Server) personUser(p Person) *userJSON {
	if p.Login == "" {
		return nil
	}
	u := s.user(p.Login, p.ID, p.AvatarURL)
	return &u
}

// permissionsJSON gives the account's rights on the repository (read-only).
type permissionsJSON struct {
	Admin    bool `json:"admin"`
	Maintain bool `json:"maintain"`
	Push     bool `json:"push"`
	Triage   bool `json:"triage"`
	Pull     bool `json:"pull"`
}

// repoJSON is a repository ("full repository").
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

// repoJSON returns the metadata of repo, with its current owner/name.
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

// branchJSON is a branch ("short branch").
type branchJSON struct {
	Name      string           `json:"name"`
	Commit    branchCommitJSON `json:"commit"`
	Protected bool             `json:"protected"`
}

// branchCommitJSON is the head commit of a branch.
type branchCommitJSON struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}

// branchJSON returns the branch name of repo.
func (s *Server) branchJSON(repo *repoState, name string) branchJSON {
	sha := repo.branches[name]
	return branchJSON{
		Name:   name,
		Commit: branchCommitJSON{SHA: sha, URL: s.URL() + "repos/" + repo.Owner + "/" + repo.Name + "/commits/" + sha},
	}
}

// gitPersonJSON is the Git identity of an author or a committer.
type gitPersonJSON struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Date  string `json:"date"`
}

// treeJSON designates the tree of a commit.
type treeJSON struct {
	SHA string `json:"sha"`
	URL string `json:"url"`
}

// verificationJSON is the signature state of a commit (never signed here).
type verificationJSON struct {
	Verified   bool    `json:"verified"`
	Reason     string  `json:"reason"`
	Signature  *string `json:"signature"`
	Payload    *string `json:"payload"`
	VerifiedAt *string `json:"verified_at"`
}

// gitCommitJSON is the Git part of a commit.
type gitCommitJSON struct {
	Author       gitPersonJSON    `json:"author"`
	Committer    gitPersonJSON    `json:"committer"`
	Message      string           `json:"message"`
	Tree         treeJSON         `json:"tree"`
	URL          string           `json:"url"`
	CommentCount int              `json:"comment_count"`
	Verification verificationJSON `json:"verification"`
}

// parentJSON designates a parent of a commit.
type parentJSON struct {
	SHA     string `json:"sha"`
	URL     string `json:"url"`
	HTMLURL string `json:"html_url"`
}

// statsJSON is the size of a commit (GET …/commits/{sha} only).
type statsJSON struct {
	Total     int `json:"total"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// commitJSON is a commit ("commit" of the REST API).
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

// commitJSON returns the commit c of repo; withStats adds "stats" and "files":[] (GET of a commit).
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

// compareJSON is the result of a comparison ("commit comparison").
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

// compareJSON returns the comparison spec ("base...head") of repo.
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

// rateJSON is the state of a quota resource.
type rateJSON struct {
	Limit     int   `json:"limit"`
	Used      int   `json:"used"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

// rateLimitBody is the body of GET /rate_limit.
type rateLimitBody struct {
	Resources map[string]rateJSON `json:"resources"`
	Rate      rateJSON            `json:"rate"`
}

// rateLimitJSON returns the body of GET /rate_limit: simulated core quota, other resources full
// (reset with core, so that the body and its ETag only change with the quota). Called with s.mu
// locked.
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
