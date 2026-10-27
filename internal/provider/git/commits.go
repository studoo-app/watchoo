package git

// Lecture du clone : branches, commits, comparaisons, statistiques et tags.

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// logFormat sépare les champs d'un commit par NUL ; « tformat » ajoute une fin de ligne après chaque
// commit, retirée du SHA du commit suivant à la lecture. %B est le message brut, %an / %ae l'identité
// telle qu'écrite dans le commit (sans .mailmap), %aI / %cI les dates ISO 8601 strictes.
const logFormat = "--format=tformat:%H%x00%P%x00%an%x00%ae%x00%aI%x00%cn%x00%ce%x00%cI%x00%B%x00"

// logFields est le nombre de champs de logFormat.
const logFields = 9

// Branches renvoie les branches du dépôt distant telles que le clone les a lues au dernier fetch
// (« for-each-ref refs/remotes/origin/ », sans origin/HEAD), triées par nom, au plus
// provider.MaxBranches. Le clone est d'abord créé ou mis à jour. Dépôt vide : liste vide.
func (p *Provider) Branches(ctx context.Context, owner, name string) ([]provider.Branch, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	return p.branches(ctx, owner, name, path)
}

// branches lit les branches du clone path.
func (p *Provider) branches(ctx context.Context, owner, name, path string) ([]provider.Branch, error) {
	refs, err := p.refs(ctx, owner, name, path, remotePrefix, "%(refname:strip=3)%00%(objectname)")
	if err != nil {
		return nil, err
	}
	branches := make([]provider.Branch, 0, len(refs))
	for _, r := range refs {
		if len(branches) >= provider.MaxBranches {
			break
		}
		if r[0] == "HEAD" {
			continue // origin/HEAD : la branche par défaut, déjà dans la liste
		}
		branches = append(branches, provider.Branch{Name: r[0], HeadSHA: r[1]})
	}
	return branches, nil
}

// Tags renvoie les tags du clone, triés par nom, au plus provider.MaxTags ; SHA du commit visé (tag
// annoté déréférencé). Dépôt vide : liste vide.
func (p *Provider) Tags(ctx context.Context, owner, name string) ([]provider.Tag, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	refs, err := p.refs(ctx, owner, name, path, "refs/tags/", "%(refname:strip=2)%00%(objectname)%00%(*objectname)")
	if err != nil {
		return nil, err
	}
	tags := make([]provider.Tag, 0, len(refs))
	for _, r := range refs {
		if len(tags) >= provider.MaxTags {
			break
		}
		sha := r[1]
		if len(r) > 2 && r[2] != "" {
			sha = r[2] // tag annoté : le commit visé
		}
		tags = append(tags, provider.Tag{Name: r[0], SHA: sha})
	}
	return tags, nil
}

// refs lit les références sous prefix avec format (champs séparés par NUL, une référence par ligne).
func (p *Provider) refs(ctx context.Context, owner, name, path, prefix, format string) ([][]string, error) {
	op := fmt.Sprintf("lecture des références de %s/%s", owner, name)
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "for-each-ref",
		"for-each-ref", "--format="+format, "--", prefix)
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return nil, localError(op, res)
	}
	var refs [][]string
	for line := range strings.SplitSeq(string(res.stdout), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\x00")
		if len(fields) < 2 || fields[0] == "" || provider.ValidateSHA(fields[1]) != nil {
			continue
		}
		refs = append(refs, fields)
	}
	return refs, nil
}

// ListCommits renvoie les commits atteignables depuis opts.Branch (branche du distant, ou SHA
// complet ; vide = branche par défaut, origin/HEAD), du plus récent au plus ancien (ordre de « git log »), filtrés par opts.Since (date de
// commit), arrêtés par opts.Stop, au plus opts.Limit (ou provider.MaxCommits). Dépôt vide : liste
// vide ; branche ou commit inconnu : provider.ErrNotFound.
func (p *Provider) ListCommits(ctx context.Context, owner, name string, opts provider.ListCommitsOptions) ([]provider.Commit, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if opts.Branch != "" {
		if err := provider.ValidateBranch(opts.Branch); err != nil {
			return nil, err
		}
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	op := fmt.Sprintf("lecture des commits de %s/%s", owner, name)
	rev := remoteHEAD
	switch {
	case opts.Branch == "":
	case provider.ValidateSHA(opts.Branch) == nil:
		rev = opts.Branch
	default:
		rev = remotePrefix + opts.Branch
	}
	sha, found, err := p.resolve(ctx, op, path, rev)
	if err != nil {
		return nil, err
	}
	if !found {
		branches, err := p.branches(ctx, owner, name, path)
		if err != nil {
			return nil, err
		}
		if len(branches) == 0 {
			return []provider.Commit{}, nil // dépôt vide (GitHub : 409, traduit en liste vide)
		}
		return nil, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	limit := opts.Limit
	if limit <= 0 || limit > provider.MaxCommits {
		limit = provider.MaxCommits
	}
	maxCount := limit
	if !opts.Since.IsZero() || opts.Stop != nil {
		maxCount = provider.MaxCommits // filtrés ensuite
	}
	commits, err := p.log(ctx, op, owner, name, path, maxCount, sha)
	if err != nil {
		return nil, err
	}
	out := make([]provider.Commit, 0, min(len(commits), limit))
	for _, c := range commits {
		if opts.Stop != nil && opts.Stop(c.SHA) {
			break
		}
		if !opts.Since.IsZero() && c.CommittedAt.Before(opts.Since) {
			continue
		}
		out = append(out, c)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// CompareCommits renvoie les commits atteignables depuis head mais pas depuis base (« git log
// base..head »), merges compris, du plus récent au plus ancien, au plus provider.MaxCommits. base ou
// head absent du clone : provider.ErrUnknownBase. Deux historiques sans ancêtre commun donnent
// exactement les commits de head absents de base (GitHub répond 404 dans ce cas).
func (p *Provider) CompareCommits(ctx context.Context, owner, name, base, head string) ([]provider.Commit, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if err := provider.ValidateSHA(base); err != nil {
		return nil, err
	}
	if err := provider.ValidateSHA(head); err != nil {
		return nil, err
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	op := fmt.Sprintf("comparaison %s...%s de %s/%s", provider.ShortSHA(base), provider.ShortSHA(head), owner, name)
	for _, rev := range []string{base, head} {
		_, found, err := p.resolve(ctx, op, path, rev)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, &provider.APIError{Op: op, Err: provider.ErrUnknownBase}
		}
	}
	if base == head {
		return []provider.Commit{}, nil
	}
	return p.log(ctx, op, owner, name, path, provider.MaxCommits, "^"+base, head)
}

// CommitStats renvoie les lignes ajoutées et supprimées par le commit sha, par rapport à son premier
// parent (à l'arbre vide pour un commit racine), comme GitHub ; fichiers binaires non comptés.
// Commit absent du clone : provider.ErrNotFound.
func (p *Provider) CommitStats(ctx context.Context, owner, name, sha string) (provider.CommitStats, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.CommitStats{}, err
	}
	if err := provider.ValidateSHA(sha); err != nil {
		return provider.CommitStats{}, err
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return provider.CommitStats{}, err
	}
	op := fmt.Sprintf("statistiques du commit %s de %s/%s", provider.ShortSHA(sha), owner, name)
	_, found, err := p.resolve(ctx, op, path, sha)
	if err != nil {
		return provider.CommitStats{}, err
	}
	if !found {
		return provider.CommitStats{}, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	// Comparaison au premier parent (un merge compte ce qu'il apporte à sa branche) ; un commit
	// racine est comparé à l'arbre vide (--root).
	args := []string{"diff-tree", "-r", "-z", "--numstat", "--no-renames", "--no-ext-diff", "--no-commit-id"}
	if parent, ok, err := p.resolve(ctx, op, path, sha+"^1"); err != nil {
		return provider.CommitStats{}, err
	} else if ok {
		args = append(args, parent, sha)
	} else {
		args = append(args, "--root", sha)
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "diff-tree", args...)
	if err != nil {
		return provider.CommitStats{}, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return provider.CommitStats{}, localError(op, res)
	}
	return parseNumstat(res.stdout), nil
}

// parseNumstat additionne la sortie de « diff-tree -z --numstat » : « ajoutées<TAB>supprimées<TAB>
// chemin<NUL> » par fichier (« - » pour un fichier binaire, ignoré).
func parseNumstat(out []byte) provider.CommitStats {
	var st provider.CommitStats
	for rec := range bytes.SplitSeq(out, []byte{0}) {
		fields := strings.SplitN(string(rec), "\t", 3)
		if len(fields) < 3 {
			continue
		}
		add, err1 := strconv.Atoi(strings.TrimSpace(fields[0]))
		del, err2 := strconv.Atoi(strings.TrimSpace(fields[1]))
		if err1 != nil || err2 != nil {
			continue
		}
		st.Additions += add
		st.Deletions += del
	}
	return st
}

// resolve renvoie le SHA du commit désigné par rev dans le clone path ; found faux s'il n'existe pas.
func (p *Provider) resolve(ctx context.Context, op, path, rev string) (string, bool, error) {
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "rev-parse",
		"rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return "", false, nil
	}
	sha := strings.TrimSpace(string(res.stdout))
	if provider.ValidateSHA(sha) != nil {
		return "", false, localError(op, res)
	}
	return sha, true, nil
}

// log lance « git log » sur revs (SHA, éventuellement précédés de « ^ ») et lit au plus maxCount
// commits.
func (p *Provider) log(ctx context.Context, op, owner, name, path string, maxCount int, revs ...string) ([]provider.Commit, error) {
	args := []string{"log", logFormat, "--no-color", "--max-count=" + strconv.Itoa(maxCount), "--end-of-options"}
	args = append(args, revs...)
	args = append(args, "--")
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "log", args...)
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return nil, localError(op, res)
	}
	commits, err := parseLog(res.stdout, owner, name)
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	return commits, nil
}

// parseLog lit la sortie de « git log » au format logFormat.
func parseLog(out []byte, owner, name string) ([]provider.Commit, error) {
	fields := strings.Split(string(out), "\x00")
	commits := make([]provider.Commit, 0, len(fields)/logFields)
	for i := 0; i+logFields <= len(fields); i += logFields {
		f := fields[i : i+logFields]
		sha := strings.TrimLeft(f[0], "\n")
		if provider.ValidateSHA(sha) != nil {
			return nil, fmt.Errorf("sortie de git log illisible : %w", provider.ErrUnexpected)
		}
		authored, err1 := parseDate(f[4])
		committed, err2 := parseDate(f[7])
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("date illisible dans le commit %s : %w", provider.ShortSHA(sha), provider.ErrUnexpected)
		}
		parents := strings.Fields(f[1])
		if parents == nil {
			parents = []string{}
		}
		commits = append(commits, provider.Commit{
			SHA:         sha,
			Message:     strings.TrimRight(f[8], "\n"),
			URL:         webURL(owner, name) + "/commit/" + sha,
			Author:      provider.Person{Name: f[2], Email: f[3]},
			Committer:   provider.Person{Name: f[5], Email: f[6]},
			AuthoredAt:  authored,
			CommittedAt: committed,
			Parents:     parents,
		})
	}
	return commits, nil
}

// parseDate lit une date ISO 8601 stricte de git (%aI) et la renvoie en UTC ; chaîne vide : zéro.
func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}
