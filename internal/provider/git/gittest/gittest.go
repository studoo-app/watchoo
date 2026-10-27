// Package gittest crée, pour les tests, de faux dépôts « distants » : de vrais dépôts Git nus dans un
// répertoire temporaire, servis par le système de fichiers (file://), sans aucun service extérieur. Le
// provider des clones Git locaux (internal/provider/git) s'y branche par Options.BaseURL = Server.URL.
//
// Les commandes de ce package ignorent la configuration Git de l'utilisateur (GIT_CONFIG_GLOBAL vers
// un fichier vide, GIT_CONFIG_NOSYSTEM) : ni signature, ni hook, ni alias ne s'en mêlent. Les tests qui
// l'utilisent sont sautés si git est absent (la CI l'a).
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

// Server regroupe des dépôts distants rangés comme sur une forge : <root>/<owner>/<name>.git.
type Server struct {
	t    testing.TB
	root string
	work string
	// URL est la racine à passer à git.Options.BaseURL (« file:///… »).
	URL string
}

// Repo est un dépôt distant et la copie de travail qui sert à le remplir.
type Repo struct {
	s           *Server
	Owner, Name string
	Bare        string // chemin du dépôt nu « distant »
	work        string
}

// Skip saute le test si git est absent.
func Skip(t testing.TB) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git absent : test sauté")
	}
}

// NewServer crée un serveur vide dans un répertoire temporaire du test (saute le test sans git).
func NewServer(t testing.TB) *Server {
	t.Helper()
	Skip(t)
	root := t.TempDir()
	work := t.TempDir()
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(root)}
	if !strings.HasPrefix(u.Path, "/") {
		u.Path = "/" + u.Path // Windows : file:///C:/…
	}
	return &Server{t: t, root: root, work: work, URL: u.String()}
}

// Create crée le dépôt distant owner/name, vide, dont la branche par défaut est defaultBranch.
func (s *Server) Create(owner, name, defaultBranch string) *Repo {
	s.t.Helper()
	// En minuscules, comme les cherche le provider (Options.BaseURL) : owner/name sont comparés sans
	// tenir compte de la casse, comme sur GitHub, même sur un système de fichiers qui la distingue.
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

// Remove supprime le dépôt distant (dépôt devenu inaccessible).
func (r *Repo) Remove() {
	r.s.t.Helper()
	if err := os.RemoveAll(r.Bare); err != nil {
		r.s.t.Fatal(err)
	}
}

// Author est l'identité d'un commit de test (données factices).
type Author struct {
	Name, Email string
}

// Commit ajoute à la branche branch (créée depuis la branche courante si elle n'existe pas) un commit
// qui écrit content dans file, publie la branche et renvoie son SHA.
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

// Merge fusionne la branche from dans into (commit de merge, même sans conflit), publie into et
// renvoie le SHA du merge.
func (r *Repo) Merge(into, from, message string, who Author, when time.Time) string {
	r.s.t.Helper()
	r.checkout(into)
	r.commitEnv(who, when, "merge", "--quiet", "--no-ff", "--no-verify", "-m", message, from)
	r.Push()
	return r.Head(into)
}

// Branch crée la branche name sur le commit from (SHA ou branche) et la publie.
func (r *Repo) Branch(name, from string) {
	r.s.t.Helper()
	r.git(r.work, "branch", "--force", name, from)
	r.Push()
}

// DeleteBranch supprime la branche name (localement et sur le distant).
func (r *Repo) DeleteBranch(name string) {
	r.s.t.Helper()
	r.git(r.work, "checkout", "--quiet", "--detach")
	r.git(r.work, "branch", "--quiet", "-D", name)
	r.Push()
}

// Reset replace la branche branch sur le commit sha (historique réécrit) et la publie de force.
func (r *Repo) Reset(branch, sha string) {
	r.s.t.Helper()
	r.checkout(branch)
	r.git(r.work, "reset", "--quiet", "--hard", sha)
	r.Push()
}

// Tag crée le tag name sur rev (annoté si message n'est pas vide) et le publie.
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

// SetDefaultBranch change la branche par défaut (HEAD) du dépôt distant.
func (r *Repo) SetDefaultBranch(branch string) {
	r.s.t.Helper()
	r.git("", "--git-dir="+r.Bare, "symbolic-ref", "HEAD", "refs/heads/"+branch)
}

// Head renvoie le SHA de la tête de branch.
func (r *Repo) Head(branch string) string {
	r.s.t.Helper()
	return strings.TrimSpace(r.git(r.work, "rev-parse", "refs/heads/"+branch))
}

// Push publie toutes les branches et tous les tags de la copie de travail, de force, en retirant du
// distant ce qui a disparu.
func (r *Repo) Push() {
	r.s.t.Helper()
	r.git(r.work, "push", "--quiet", "--force", "--prune", "origin", "+refs/heads/*:refs/heads/*",
		"+refs/tags/*:refs/tags/*")
}

// checkout passe sur branch, créée depuis HEAD si elle n'existe pas (première branche d'un dépôt vide
// comprise).
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

// commitEnv lance une commande qui crée un commit (ou un tag annoté) avec l'identité et la date
// données.
func (r *Repo) commitEnv(who Author, when time.Time, args ...string) {
	r.s.t.Helper()
	date := when.UTC().Format(time.RFC3339)
	env := []string{
		"GIT_AUTHOR_NAME=" + who.Name, "GIT_AUTHOR_EMAIL=" + who.Email, "GIT_AUTHOR_DATE=" + date,
		"GIT_COMMITTER_NAME=" + who.Name, "GIT_COMMITTER_EMAIL=" + who.Email, "GIT_COMMITTER_DATE=" + date,
	}
	if out, err := r.run(r.work, env, args...); err != nil {
		r.s.t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
	}
}

// git lance une commande git (dir vide : répertoire courant) et renvoie sa sortie ; échec fatal.
func (r *Repo) git(dir string, args ...string) string {
	r.s.t.Helper()
	out, err := r.run(dir, nil, args...)
	if err != nil {
		r.s.t.Fatalf("git %s : %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

// run lance git avec une configuration isolée de celle de l'utilisateur.
func (r *Repo) run(dir string, env []string, args ...string) (string, error) {
	// Aucun entretien automatique en arrière-plan : il garderait la sortie ouverte après la fin de git.
	cmd := exec.Command("git", append([]string{"-c", "gc.auto=0", "-c", "maintenance.auto=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	cmd.Env = append(cmd.Env, env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s : %w", args[0], err)
	}
	return out.String(), nil
}
