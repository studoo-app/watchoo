// Package gittest creates, for tests, fake "remote" repositories: real bare Git repositories in a
// temporary directory, served through the file system (file://), without any external service. The
// local Git clones provider (internal/provider/git) plugs into them with Options.BaseURL = Server.URL.
//
// The commands of this package ignore the user's Git configuration (GIT_CONFIG_GLOBAL pointing to
// an empty file, GIT_CONFIG_NOSYSTEM): no signing, hook or alias interferes. The tests that use it
// are skipped if git is missing (the CI has it).
package gittest

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Server groups remote repositories laid out as on a forge: <root>/<owner>/<name>.git.
type Server struct {
	t    testing.TB
	root string
	work string
	// URL is the root to pass to git.Options.BaseURL ("file:///…").
	URL string
}

// Repo is a remote repository and the working copy used to fill it.
type Repo struct {
	s           *Server
	Owner, Name string
	Bare        string // path of the "remote" bare repository
	work        string
}

// Skip skips the test if git is missing.
func Skip(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git missing: test skipped")
	}
}

// NewServer creates an empty server in a temporary directory of the test (skips the test without git).
func NewServer(t testing.TB) *Server {
	t.Helper()
	Skip(t)
	root := t.TempDir()
	work := t.TempDir()
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(root)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path // Windows: file:///C:/…
	}
	return &Server{t: t, root: root, work: work, URL: u.String()}
}

// Create creates the remote repository owner/name, empty, whose default branch is defaultBranch.
func (s *Server) Create(owner, name, defaultBranch string) *Repo {
	s.t.Helper()
	// In lowercase, as the provider looks for them (Options.BaseURL): owner/name are compared
	// case-insensitively, as on GitHub, even on a case-sensitive file system.
	bare := filepath.Join(s.root, strings.ToLower(owner), strings.ToLower(name)+".git")
	work := filepath.Join(s.work, owner, name)
	for _, d := range []string{filepath.Dir(bare), work} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			s.t.Fatal(err)
		}
	}
	r := &Repo{s: s, Owner: owner, Name: name, Bare: bare, work: work}
	r.git("", "init", "--quiet", "--bare", "--initial-branch="+defaultBranch, bare)
	r.git(work, "init", "--quiet", "--initial-branch="+defaultBranch)
	r.git(work, "remote", "add", "origin", bare)
	return r
}

// Remove deletes the remote repository (repository that became inaccessible).
func (r *Repo) Remove() {
	r.s.t.Helper()
	if err := os.RemoveAll(r.Bare); err != nil {
		r.s.t.Fatal(err)
	}
}

// Author is the identity of a test commit (dummy data).
type Author struct {
	Name, Email string
}

// Commit adds to the branch branch (created from the current branch if it does not exist) a commit
// that writes content to file, publishes the branch and returns its SHA.
func (r *Repo) Commit(branch, file, content, message string, who Author, when time.Time) string {
	r.s.t.Helper()
	r.checkout(branch)
	path := filepath.Join(r.work, filepath.FromSlash(file))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		r.s.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		r.s.t.Fatal(err)
	}
	r.git(r.work, "add", "--", file)
	r.commitEnv(who, when, "commit", "--quiet", "--no-verify", "-m", message)
	r.Push()
	return r.Head(branch)
}

// EmptyCommit adds to the branch branch a commit without any change ("--allow-empty"), publishes
// the branch and returns its SHA: a commit really at +0 −0.
func (r *Repo) EmptyCommit(branch, message string, who Author, when time.Time) string {
	r.s.t.Helper()
	r.checkout(branch)
	r.commitEnv(who, when, "commit", "--quiet", "--allow-empty", "--no-verify", "-m", message)
	r.Push()
	return r.Head(branch)
}

// ManyCommits adds to the branch branch n commits that each rewrite the single line of file
// (+1 −1 each), one minute apart starting from start, then publishes the branch only once.
func (r *Repo) ManyCommits(branch, file string, n int, who Author, start time.Time) {
	r.s.t.Helper()
	r.checkout(branch)
	path := filepath.Join(r.work, filepath.FromSlash(file))
	for i := range n {
		if err := os.WriteFile(path, []byte(fmt.Sprintf("ligne %d\n", i+1)), 0o600); err != nil {
			r.s.t.Fatal(err)
		}
		r.git(r.work, "add", "--", file)
		r.commitEnv(who, start.Add(time.Duration(i)*time.Minute), "commit", "--quiet", "--no-verify",
			"-m", fmt.Sprintf("commit %d", i+1))
	}
	r.Push()
}

// Merge merges the branch from into into (merge commit, even without conflict), publishes into and
// returns the SHA of the merge.
func (r *Repo) Merge(into, from, message string, who Author, when time.Time) string {
	r.s.t.Helper()
	r.checkout(into)
	r.commitEnv(who, when, "merge", "--quiet", "--no-ff", "--no-verify", "-m", message, from)
	r.Push()
	return r.Head(into)
}

// Branch creates the branch name on the commit from (SHA or branch) and publishes it.
func (r *Repo) Branch(name, from string) {
	r.s.t.Helper()
	r.git(r.work, "branch", "--force", name, from)
	r.Push()
}

// DeleteBranch deletes the branch name (locally and on the remote).
func (r *Repo) DeleteBranch(name string) {
	r.s.t.Helper()
	r.git(r.work, "checkout", "--quiet", "--detach")
	r.git(r.work, "branch", "--quiet", "-D", name)
	r.Push()
}

// Reset moves the branch branch back to the commit sha (history rewritten) and force-publishes it.
func (r *Repo) Reset(branch, sha string) {
	r.s.t.Helper()
	r.checkout(branch)
	r.git(r.work, "reset", "--quiet", "--hard", sha)
	r.Push()
}

// Tag creates the tag name on rev (annotated if message is not empty) and publishes it.
func (r *Repo) Tag(name, rev, message string) {
	r.s.t.Helper()
	if message == "" {
		r.git(r.work, "tag", name, rev)
	} else {
		r.commitEnv(Author{Name: "Tagueur", Email: "tag@example.test"}, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			"tag", "-a", "-m", message, name, rev)
	}
	r.Push()
}

// DeleteTag deletes the tag name (locally and on the remote).
func (r *Repo) DeleteTag(name string) {
	r.s.t.Helper()
	r.git(r.work, "tag", "--delete", name)
	r.Push()
}

// SetDefaultBranch changes the default branch (HEAD) of the remote repository.
func (r *Repo) SetDefaultBranch(branch string) {
	r.s.t.Helper()
	r.git("", "--git-dir="+r.Bare, "symbolic-ref", "HEAD", "refs/heads/"+branch)
}

// Head returns the SHA of the head of branch.
func (r *Repo) Head(branch string) string {
	r.s.t.Helper()
	return strings.TrimSpace(r.git(r.work, "rev-parse", "refs/heads/"+branch))
}

// Push publishes all the branches and all the tags of the working copy, by force, removing from the
// remote what has disappeared.
func (r *Repo) Push() {
	r.s.t.Helper()
	r.git(r.work, "push", "--quiet", "--force", "--prune", "origin", "+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*")
}

// checkout switches to branch, created from HEAD if it does not exist (first branch of an empty
// repository included).
func (r *Repo) checkout(branch string) {
	r.s.t.Helper()
	if _, err := r.run(r.work, nil, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		r.git(r.work, "checkout", "--quiet", branch)
		return
	}
	if _, err := r.run(r.work, nil, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		r.git(r.work, "checkout", "--quiet", "--orphan", branch)
		return
	}
	r.git(r.work, "checkout", "--quiet", "-b", branch)
}

// commitEnv runs a command that creates a commit (or an annotated tag) with the given identity and
// date.
func (r *Repo) commitEnv(who Author, when time.Time, args ...string) {
	r.s.t.Helper()
	date := when.UTC().Format(time.RFC3339)
	env := []string{
		"GIT_AUTHOR_NAME=" + who.Name, "GIT_AUTHOR_EMAIL=" + who.Email, "GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=" + who.Name, "GIT_COMMITTER_EMAIL=" + who.Email, "GIT_COMMITTER_DATE=" + date,
	}
	if out, err := r.run(r.work, env, args...); err != nil {
		r.s.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// git runs a git command (empty dir: current directory) and returns its output; fatal on failure.
func (r *Repo) git(dir string, args ...string) string {
	r.s.t.Helper()
	out, err := r.run(dir, nil, args...)
	if err != nil {
		r.s.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// run runs git with a configuration isolated from the user's.
func (r *Repo) run(dir string, env []string, args ...string) (string, error) {
	// No automatic background maintenance: it would keep the output open after git ends.
	cmd := exec.Command("git", append([]string{"-c", "gc.auto=0", "-c", "maintenance.auto=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	cmd.Env = append(cmd.Env, env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w", args[0], err)
	}
	return out.String(), nil
}
