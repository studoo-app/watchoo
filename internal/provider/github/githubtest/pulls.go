package githubtest

// Simulated pull requests, releases and tags (V2):
//   - GET …/pulls?state&sort&direction&per_page&page: state open (default), closed or all;
//     sort created (default) or updated; direction desc by default with sort=created, asc otherwise
//     (like GitHub); at equal dates, ascending number in the direction's sense; unknown value →
//     422 "Validation Failed". Body: "pull request simple" objects (number, state, title, user,
//     draft, html_url, body, merge_commit_sha, base.{ref, repo}, head.{ref, repo}, created_at,
//     updated_at, closed_at, merged_at; GitHub's list has no merged field). base.repo is the
//     repository; head.repo is the same repository by default, a distinct fork repository
//     (PullRequest.Fork) or null (PullRequest.HeadRepoDeleted: fork deleted), phase 8.5;
//   - GET …/pulls/{number}/reviews?per_page&page (phase 8.6): reviews of the pull request in the order
//     of SetReviews (chronological, like GitHub); unknown pull request or malformed number → 404. Body:
//     id, node_id, user (null for a deleted account; "type": "Bot" for Review.Bot), body (served so
//     that the tests check it is never kept), state, html_url, pull_request_url, commit_id,
//     author_association and submitted_at (absent when Review.SubmittedAt is zero, like GitHub for
//     PENDING);
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
	Number int    // number (> 0, unique in the repository)
	Title  string // title
	Closed bool   // closed ("state": "closed"); false = open
	Draft  bool   // draft
	Author string // author's login; empty = "user": null (deleted account)
	Base   string // target branch; empty = default branch
	Head   string // proposed branch; empty = "feature-<number>"
	Body   string // description; empty = null
	// MergeCommitSHA is "merge_commit_sha"; empty = null. Served as is, whatever the state (GitHub also
	// fills it for an open PR: test merge commit).
	MergeCommitSHA  string
	Fork            bool      // head.repo is a fork (other identifier, "fork": true)
	HeadRepoDeleted bool      // head.repo is null (fork deleted); takes precedence over Fork
	CreatedAt       time.Time // opening (not zero)
	UpdatedAt       time.Time // last update; zero = CreatedAt
	ClosedAt        time.Time // closing; zero = null (Closed requires a date: zero = UpdatedAt)
	MergedAt        time.Time // merge; zero = null
}

// Review is a simulated pull request review (phase 8.6).
type Review struct {
	ID          int64     // identifier (> 0, unique in the pull request)
	Reviewer    string    // reviewer's login; empty = "user": null (deleted account)
	Bot         bool      // reviewer is an app account ("type": "Bot")
	State       string    // APPROVED, CHANGES_REQUESTED, COMMENTED, DISMISSED or PENDING (served as is)
	SubmittedAt time.Time // submission; zero = submitted_at absent (PENDING)
	Body        string    // body; empty = ""
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

// SetReviews replaces the reviews of the pull request number of the repository repoID, served in that
// order. Dates converted to UTC and truncated to the second. The pull request must have been set by
// SetPullRequests. Identifier <= 0 or duplicated, empty state, unknown pull request or repository:
// t.Fatalf.
func (s *Server) SetReviews(repoID int64, number int, reviews ...Review) {
	s.tb.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	repo := s.repoLocked("SetReviews", repoID)
	if _, ok := repo.pulls[number]; !ok {
		s.fatalf("SetReviews: unknown PR #%d", number)
	}
	seen := make(map[int64]bool, len(reviews))
	out := make([]Review, 0, len(reviews))
	for _, r := range reviews {
		if r.ID <= 0 || seen[r.ID] || r.State == "" {
			s.fatalf("SetReviews: invalid review %d (state %q)", r.ID, r.State)
		}
		seen[r.ID] = true
		r.SubmittedAt = utcSecond(r.SubmittedAt)
		out = append(out, r)
	}
	if repo.reviews == nil {
		repo.reviews = make(map[int][]Review)
	}
	repo.reviews[number] = out
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

// reviewsReply serves GET …/pulls/{number}/reviews.
func (s *Server) reviewsReply(repo *repoState, num string, q url.Values, linkBase string, now time.Time) reply {
	number, err := strconv.Atoi(num)
	if err != nil || number <= 0 || strconv.Itoa(number) != num {
		return s.notFound(now)
	}
	pr, ok := repo.pulls[number]
	if !ok {
		return s.notFound(now)
	}
	list := repo.reviews[number]
	p := paginate(len(list), q)
	items := make([]reviewJSON, 0, p.end-p.start)
	for _, r := range list[p.start:p.end] {
		items = append(items, s.reviewJSON(repo, pr, r))
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
	// Repo is the repository of the ref; null (no omitempty) for the head of a deleted fork.
	Repo *pullRepoJSON `json:"repo"`
}

// pullRepoJSON is the (shortened) repository of the base or the head of a pull request.
type pullRepoJSON struct {
	ID       int64  `json:"id"`
	NodeID   string `json:"node_id"`
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Fork     bool   `json:"fork"`
}

// forkOwner is the owner of the simulated fork repositories.
const forkOwner = "fork-owner"

// forkIDOffset separates the identifiers of the simulated forks from the repositories'.
const forkIDOffset = 1_000_000_000

// pullJSON is a pull request ("pull request simple").
type pullJSON struct {
	URL       string    `json:"url"`
	ID        int64     `json:"id"`
	NodeID    string    `json:"node_id"`
	HTMLURL   string    `json:"html_url"`
	Number    int       `json:"number"`
	State     string    `json:"state"`
	Locked    bool      `json:"locked"`
	Title     string    `json:"title"`
	User      *userJSON `json:"user"`
	Body      *string   `json:"body"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
	ClosedAt  *string   `json:"closed_at"`
	MergedAt  *string   `json:"merged_at"`
	// MergeCommit is "merge_commit_sha" (null if unknown).
	MergeCommit *string     `json:"merge_commit_sha"`
	Draft       bool        `json:"draft"`
	Head        pullRefJSON `json:"head"`
	Base        pullRefJSON `json:"base"`
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
	baseRepo := &pullRepoJSON{ID: repo.ID, NodeID: "R_fake" + strconv.FormatInt(repo.ID, 10),
		Name: repo.Name, FullName: full}
	headRepo := baseRepo
	switch {
	case pr.HeadRepoDeleted:
		headRepo = nil
	case pr.Fork:
		id := repo.ID + forkIDOffset
		headRepo = &pullRepoJSON{ID: id, NodeID: "R_fake" + strconv.FormatInt(id, 10), Name: repo.Name,
			FullName: forkOwner + "/" + repo.Name, Fork: true}
	}
	return pullJSON{
		URL:         s.URL() + "repos/" + full + "/pulls/" + strconv.Itoa(pr.Number),
		ID:          repo.ID*100000 + int64(pr.Number),
		NodeID:      "PR_fake" + strconv.FormatInt(repo.ID, 10) + "_" + strconv.Itoa(pr.Number),
		HTMLURL:     githubWeb + full + "/pull/" + strconv.Itoa(pr.Number),
		Number:      pr.Number,
		State:       state,
		Title:       pr.Title,
		User:        user,
		Body:        nullableString(pr.Body),
		CreatedAt:   timeJSON(pr.CreatedAt),
		UpdatedAt:   timeJSON(pr.UpdatedAt),
		ClosedAt:    nullableTime(pr.ClosedAt),
		MergedAt:    nullableTime(pr.MergedAt),
		MergeCommit: nullableString(pr.MergeCommitSHA),
		Draft:       pr.Draft,
		Head:        pullRefJSON{Label: repo.Owner + ":" + head, Ref: head, SHA: repo.branches[head], Repo: headRepo},
		Base:        pullRefJSON{Label: repo.Owner + ":" + base, Ref: base, SHA: repo.branches[base], Repo: baseRepo},
	}
}

// reviewJSON is a pull request review.
type reviewJSON struct {
	ID                int64     `json:"id"`
	NodeID            string    `json:"node_id"`
	User              *userJSON `json:"user"`
	Body              string    `json:"body"`
	State             string    `json:"state"`
	HTMLURL           string    `json:"html_url"`
	PullRequestURL    string    `json:"pull_request_url"`
	CommitID          string    `json:"commit_id"`
	AuthorAssociation string    `json:"author_association"`
	SubmittedAt       *string   `json:"submitted_at,omitempty"`
}

// reviewJSON returns the review r of the pull request pr of repo.
func (s *Server) reviewJSON(repo *repoState, pr PullRequest, r Review) reviewJSON {
	full := repo.Owner + "/" + repo.Name
	var user *userJSON
	if r.Reviewer != "" {
		u := s.user(r.Reviewer, 0, "")
		if r.Bot {
			u.Type = "Bot"
		}
		user = &u
	}
	head := pr.Head
	if head == "" {
		head = fmt.Sprintf("feature-%d", pr.Number)
	}
	return reviewJSON{
		ID:                r.ID,
		NodeID:            "PRR_fake" + strconv.FormatInt(r.ID, 10),
		User:              user,
		Body:              r.Body,
		State:             r.State,
		HTMLURL:           githubWeb + full + "/pull/" + strconv.Itoa(pr.Number) + "#pullrequestreview-" + strconv.FormatInt(r.ID, 10),
		PullRequestURL:    s.URL() + "repos/" + full + "/pulls/" + strconv.Itoa(pr.Number),
		CommitID:          repo.branches[head],
		AuthorAssociation: "COLLABORATOR",
		SubmittedAt:       nullableTime(r.SubmittedAt),
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
