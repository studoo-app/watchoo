package githubtest

// Simulated pull requests, releases and tags (V2):
//   - GET …/pulls?state&sort&direction&per_page&page: state open (default), closed or all;
//     sort created (default) or updated; direction desc by default with sort=created, asc otherwise
//     (like GitHub); at equal dates, ascending number in the direction's sense; unknown value →
//     422 "Validation Failed". Body: "pull request simple" objects (number, state, title, user,
//     draft, html_url, base.ref, head.ref, created_at, updated_at, closed_at, merged_at; GitHub's
//     list has no merged field);
//   - GET …/releases?per_page&page: releases sorted by descending created_at (at equal dates,
//     descending identifier), drafts included (the fake account always has write access);
//   - GET …/tags?per_page&page: tags in the order of SetTags.
//
// An empty repository (without branch) answers 200 with its items, like GitHub (no 409 on these routes).

import (
	"cmp"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"
)

// docsPulls is the documentation link of the errors of the pull request list.
const docsPulls = "https://docs.github.com/rest/pulls/pulls#list-pull-requests"

// PullRequest is a simulated pull request.
type PullRequest struct {
	Number    int       // number (> 0, unique in the repository)
	Title     string    // title
	Closed    bool      // closed ("state": "closed"); false = open
	Draft     bool      // draft
	Author    string    // author's login; empty = "user": null (deleted account)
	Base      string    // target branch; empty = default branch
	Head      string    // proposed branch; empty = "feature-<number>"
	CreatedAt time.Time // opening (not zero)
	UpdatedAt time.Time // last update; zero = CreatedAt
	ClosedAt  time.Time // closing; zero = null (Closed requires a date: zero = UpdatedAt)
	MergedAt  time.Time // merge; zero = null
}

// Release is a simulated release.
type Release struct {
	ID          int64     // identifier (> 0, unique in the repository)
	TagName     string    // tag (not empty)
	Name        string    // title; empty = null
	Draft       bool      // draft
	Prerelease  bool      // pre-release
	Author      string    // author's login; empty = "author": null
	CreatedAt   time.Time // creation (not zero)
	PublishedAt time.Time // publication; zero = null
}

// Tag is a simulated tag.
type Tag struct {
	Name string // name (not empty)
	SHA  string // full SHA of the target commit (not necessarily added to the repository)
}

// SetPullRequests adds or replaces (by number) pull requests of the repository repoID. Dates converted
// to UTC and truncated to the second. Number <= 0, zero CreatedAt or unknown repository: t.Fatalf.
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
			s.fatalf("SetPullRequests: invalid PR #%d (want number > 0 and CreatedAt)", pr.Number)
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

// SetReleases replaces the releases of the repository repoID. Identifier <= 0 or duplicated, empty
// tag, zero CreatedAt or unknown repository: t.Fatalf.
func (s *Server) SetReleases(repoID int64, releases ...Release) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetReleases", repoID)
	seen := make(map[int64]bool, len(releases))
	out := make([]Release, 0, len(releases))
	for _, r := range releases {
		if r.ID <= 0 || seen[r.ID] || r.TagName == "" || r.CreatedAt.IsZero() {
			s.fatalf("SetReleases: invalid release %d (tag %q)", r.ID, r.TagName)
		}
		seen[r.ID] = true
		r.CreatedAt, r.PublishedAt = utcSecond(r.CreatedAt), utcSecond(r.PublishedAt)
		out = append(out, r)
	}
	repo.releases = out
}

// SetTags replaces the tags of the repository repoID, served in that order. Empty name, malformed SHA
// or unknown repository: t.Fatalf.
func (s *Server) SetTags(repoID int64, tags ...Tag) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetTags", repoID)
	for _, t := range tags {
		if t.Name == "" || !shaPattern.MatchString(t.SHA) {
			s.fatalf("SetTags: invalid tag %q (SHA %q)", t.Name, t.SHA)
		}
	}
	repo.tags = slices.Clone(tags)
}

// ----- Responses -----

// pullsReply serves GET …/pulls.
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

// releasesReply serves GET …/releases.
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

// tagsReply serves GET …/tags.
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

// pullRefJSON is the base or the head of a pull request.
type pullRefJSON struct {
	Label string `json:"label"`
	Ref   string `json:"ref"`
	SHA   string `json:"sha"`
}

// pullJSON is a pull request ("pull request simple").
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

// pullJSON returns the pull request pr of repo.
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

// releaseJSON is a release.
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

// releaseJSON returns the release r of repo.
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

// tagJSON is a tag.
type tagJSON struct {
	Name       string           `json:"name"`
	Commit     branchCommitJSON `json:"commit"`
	ZipballURL string           `json:"zipball_url"`
	TarballURL string           `json:"tarball_url"`
	NodeID     string           `json:"node_id"`
}
