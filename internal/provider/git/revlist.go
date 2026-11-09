package git

// Commits des branches suivies (phase 8.1, conventions) : périmètre précis de l'évaluation des messages
// de commit, lu dans le clone existant sans le mettre à jour.

import (
	"context"
	"strings"

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
