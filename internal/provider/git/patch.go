package git

import (
	"context"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// CommitPatch renvoie le diff du commit sha (par rapport à son premier parent, à l'arbre vide pour un
// commit racine), lu sur le clone EXISTANT de owner/name, sans le créer ni le mettre à jour (aucun accès
// réseau). Au-delà de maxBytes (> 0), le diff est coupé à la dernière fin de ligne et truncated est
// vrai. found est faux si le clone n'existe pas ou ne contient pas le commit. Le diff est le code d'un
// dépôt privé : il n'est ni gardé, ni écrit dans les logs ; il sert à la fiche de revue du diagnostic.
func (p *Provider) CommitPatch(ctx context.Context, owner, name, sha string, maxBytes int) (patch string, truncated, found bool, err error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return "", false, false, err
	}
	if err := provider.ValidateSHA(sha); err != nil {
		return "", false, false, err
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return "", false, false, err
	}
	if state, err := inspect(path, owner, name); err != nil || state != stateOwned {
		return "", false, false, nil
	}
	op := i18n.T("provider.git.op.commit_patch", provider.ShortSHA(sha), owner, name)
	if _, ok, err := p.resolve(ctx, op, path, sha); err != nil || !ok {
		return "", false, false, err
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "show", "show", "--format=", "--patch", "--no-color",
		"--no-ext-diff", "--no-textconv", "--no-renames", "--diff-merges=first-parent", "--end-of-options", sha)
	if err != nil {
		return "", false, false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return "", false, false, localError(op, res)
	}
	out := res.stdout
	if maxBytes > 0 && len(out) > maxBytes {
		out, truncated = out[:maxBytes], true
		if i := lastNewline(out); i > 0 {
			out = out[:i+1]
		}
	}
	// Un fichier dans un autre encodage : octets invalides remplacés par U+FFFD.
	return strings.ToValidUTF8(string(out), "\uFFFD"), truncated, true, nil
}

// lastNewline renvoie l'indice du dernier « \n » de b, -1 s'il n'y en a pas.
func lastNewline(b []byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			return i
		}
	}
	return -1
}
