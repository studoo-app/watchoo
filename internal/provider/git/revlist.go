package git

// Lectures des conventions dans le clone existant, sans le mettre à jour : commits des branches suivies
// (phase 8.1, périmètre de l'évaluation des messages de commit), branches du distant et intervalles de
// commits entre tags (phase 8.2), premier parent de la branche par défaut (PR3, phase 8.5).

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// RevList renvoie les SHA des commits accessibles depuis les branches branches (origin/<branche>) du
// clone EXISTANT de owner/name, lu tel quel, sans mise à jour (aucun accès réseau), chaque commit une
// fois, dans l'ordre de « git rev-list » :
//
//	git rev-list --ignore-missing --stdin   (entrée : refs/remotes/origin/<branche>)
//
// Une branche absente du clone est ignorée ; aucune branche : liste vide. Sans clone : erreur
// provider.ErrNotFound. Les noms de branches passent par l'entrée standard, jamais en argument.
func (p *Provider) RevList(ctx context.Context, owner, name string, branches []string) ([]string, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	for _, b := range branches {
		if err := provider.ValidateBranch(b); err != nil {
			return nil, err
		}
	}
	op := i18n.T("provider.git.op.rev_list", owner, name)
	if !p.HasClone(owner, name) {
		return nil, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	if len(branches) == 0 {
		return []string{}, nil
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return nil, err
	}
	var input strings.Builder
	for _, b := range branches {
		input.WriteString(remotePrefix + b + "\n")
	}
	res, err := p.runInput(ctx, p.localTimeout, gitDir(path), "rev-list", []byte(input.String()),
		"rev-list", "--ignore-missing", "--stdin")
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return nil, localError(op, res)
	}
	out := []string{}
	for line := range strings.SplitSeq(string(res.stdout), "\n") {
		if sha := strings.TrimSpace(line); provider.ValidateSHA(sha) == nil {
			out = append(out, sha)
		}
	}
	return out, nil
}

// RemoteBranches renvoie les noms des branches du distant telles que le clone EXISTANT de owner/name les
// a lues au dernier fetch (« for-each-ref refs/remotes/origin/ », sans origin/HEAD), triés, au plus
// provider.MaxBranches, sans mise à jour (aucun accès réseau) : nommage des branches des conventions
// (BR1, phase 8.2). Sans clone : erreur provider.ErrNotFound.
func (p *Provider) RemoteBranches(ctx context.Context, owner, name string) ([]string, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if !p.HasClone(owner, name) {
		return nil, &provider.APIError{Op: i18n.T("provider.git.op.refs", owner, name), Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return nil, err
	}
	branches, err := p.branches(ctx, owner, name, path)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(branches))
	for _, b := range branches {
		out = append(out, b.Name)
	}
	slices.Sort(out)
	return out, nil
}

// RevRange renvoie les SHA des commits accessibles depuis les branches branches (origin/<branche>) et
// les commits include, mais pas depuis les commits exclude, dans le clone EXISTANT de owner/name, sans
// mise à jour (aucun accès réseau) : intervalles entre tags des conventions (phase 8.2).
//
//	git rev-list --stdin   (entrée : refs/remotes/origin/<branche>, <sha>, ^<sha>)
//
// Une branche absente du clone est ignorée, mais des branches demandées toutes absentes (sans commit
// include) sont une erreur provider.ErrNotFound : l'intervalle serait vide à tort. Un commit include ou
// exclude absent du clone est une erreur (jamais ignoré : un intervalle mal borné serait faux). Rien de
// demandé : liste vide. Sans clone : erreur provider.ErrNotFound. Noms et SHA passent par l'entrée standard, jamais en argument.
func (p *Provider) RevRange(ctx context.Context, owner, name string, branches, include, exclude []string) ([]string, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	for _, b := range branches {
		if err := provider.ValidateBranch(b); err != nil {
			return nil, err
		}
	}
	for _, sha := range slices.Concat(include, exclude) {
		if err := provider.ValidateSHA(sha); err != nil {
			return nil, err
		}
	}
	op := i18n.T("provider.git.op.rev_list", owner, name)
	if !p.HasClone(owner, name) {
		return nil, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return nil, err
	}
	var present []string
	if len(branches) > 0 {
		known, err := p.branches(ctx, owner, name, path)
		if err != nil {
			return nil, err
		}
		for _, b := range branches {
			if slices.ContainsFunc(known, func(k provider.Branch) bool { return k.Name == b }) {
				present = append(present, b)
			}
		}
	}
	if len(present) == 0 && len(include) == 0 {
		if len(branches) > 0 {
			return nil, &provider.APIError{Op: op, Err: provider.ErrNotFound}
		}
		return []string{}, nil
	}
	var input strings.Builder
	for _, b := range present {
		input.WriteString(remotePrefix + b + "\n")
	}
	for _, sha := range include {
		input.WriteString(sha + "\n")
	}
	for _, sha := range exclude {
		input.WriteString("^" + sha + "\n")
	}
	res, err := p.runInput(ctx, p.localTimeout, gitDir(path), "rev-list", []byte(input.String()), "rev-list", "--stdin")
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return nil, localError(op, res)
	}
	out := []string{}
	for line := range strings.SplitSeq(string(res.stdout), "\n") {
		if sha := strings.TrimSpace(line); provider.ValidateSHA(sha) == nil {
			out = append(out, sha)
		}
	}
	return out, nil
}

// FirstParent renvoie les SHA des commits du premier parent (PR3, phase 8.5) dans le clone EXISTANT de
// owner/name, lu tel quel, sans mise à jour (aucun accès réseau), du plus récent au plus ancien, sans le
// commit racine (aucun parent) :
//
//	git rev-list --first-parent --parents [--since=…] [--until=…] --stdin
//	  (entrée : refs/remotes/origin/<branche> ou <head>, puis ^<sha> pour chaque commit exclude)
//
// La chaîne part de la branche branch du distant si elle n'est pas vide, sinon du commit head. since et
// until (zéro : sans borne) filtrent sur la date du committer, la plus proche de l'arrivée du commit sur
// la branche. Une branche absente du clone, un commit head ou exclude absent : erreur (jamais une liste
// vide à tort). Sans clone : provider.ErrNotFound. Noms et SHA passent par l'entrée standard.
func (p *Provider) FirstParent(ctx context.Context, owner, name, branch, head string, exclude []string,
	since, until time.Time) ([]string, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	switch {
	case branch != "":
		if err := provider.ValidateBranch(branch); err != nil {
			return nil, err
		}
	default:
		if err := provider.ValidateSHA(head); err != nil {
			return nil, err
		}
	}
	for _, sha := range exclude {
		if err := provider.ValidateSHA(sha); err != nil {
			return nil, err
		}
	}
	op := i18n.T("provider.git.op.first_parent", owner, name)
	if !p.HasClone(owner, name) {
		return nil, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return nil, err
	}
	var input strings.Builder
	if branch != "" {
		input.WriteString(remotePrefix + branch + "\n")
	} else {
		input.WriteString(head + "\n")
	}
	for _, sha := range exclude {
		input.WriteString("^" + sha + "\n")
	}
	args := []string{"rev-list", "--first-parent", "--parents"}
	const gitDate = "2006-01-02 15:04:05 +0000"
	if !since.IsZero() {
		args = append(args, "--since="+since.UTC().Format(gitDate))
	}
	if !until.IsZero() {
		args = append(args, "--until="+until.UTC().Format(gitDate))
	}
	args = append(args, "--stdin")
	res, err := p.runInput(ctx, p.localTimeout, gitDir(path), "rev-list", []byte(input.String()), args...)
	if err != nil {
		return nil, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return nil, localError(op, res)
	}
	out := []string{}
	for line := range strings.SplitSeq(string(res.stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || provider.ValidateSHA(fields[0]) != nil {
			continue // ligne vide, ou commit racine (aucun parent)
		}
		out = append(out, fields[0])
	}
	return out, nil
}
