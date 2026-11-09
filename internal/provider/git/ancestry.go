package git

// Lectures de l'historique pour les conventions (phase 8.4), dans le clone EXISTANT, sans le mettre à
// jour (aucun accès réseau) : ascendance de deux commits (réécritures de la branche par défaut BR4,
// cohérence des tags SV4, SV5 et SV7) et dernier commit qui a touché un chemin (fraîcheur du CHANGELOG
// CL10). Aucune sortie de git n'est recopiée dans les erreurs ; ni SHA complet ni chemin dans les logs.

import (
	"context"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// IsAncestor indique si le commit ancestor est ancêtre du commit descendant (un commit est son propre
// ancêtre) dans le clone EXISTANT de owner/name, lu tel quel :
//
//	git merge-base --is-ancestor --end-of-options <ancestor> <descendant>
//
// Un des deux commits absent du clone (historique réécrit puis nettoyé, commit pas encore lu) : erreur
// provider.ErrNotFound, que l'appelant traite comme « inconnu », jamais comme « pas ancêtre ». Sans
// clone : provider.ErrNotFound. Les SHA doivent être complets (provider.ValidateSHA).
func (p *Provider) IsAncestor(ctx context.Context, owner, name, ancestor, descendant string) (bool, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return false, err
	}
	for _, sha := range []string{ancestor, descendant} {
		if err := provider.ValidateSHA(sha); err != nil {
			return false, err
		}
	}
	op := i18n.T("provider.git.op.ancestry", provider.ShortSHA(ancestor), provider.ShortSHA(descendant), owner, name)
	if !p.HasClone(owner, name) {
		return false, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return false, err
	}
	for _, rev := range []string{ancestor, descendant} {
		_, found, err := p.resolve(ctx, op, path, rev)
		if err != nil {
			return false, err
		}
		if !found {
			return false, &provider.APIError{Op: op, Err: provider.ErrNotFound}
		}
	}
	if ancestor == descendant {
		return true, nil
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "merge-base",
		"merge-base", "--is-ancestor", "--end-of-options", ancestor, descendant)
	if err != nil {
		return false, &provider.APIError{Op: op, Err: err}
	}
	switch res.exitCode {
	case 0:
		return true, nil
	case 1:
		return false, nil
	}
	return false, localError(op, res)
}

// LastTouch renvoie le SHA du dernier commit, dans l'historique de head (SHA complet), qui a modifié le
// fichier file (chemin relatif à la racine), dans le clone EXISTANT de owner/name, lu tel quel :
//
//	git --literal-pathspecs log -1 --format=%H --end-of-options <head> -- <file>
//
// found faux si file n'existe pas (ou n'est pas un fichier) dans l'arbre de head, lu d'abord par
//
//	git --literal-pathspecs ls-tree -z --end-of-options <head> -- <file>
//
// (un fichier supprimé n'a pas de dernière modification), ou si aucun commit de head ne l'a touché. head
// absent du clone : provider.ErrNotFound. Sans clone : provider.ErrNotFound. Le chemin n'apparaît jamais
// dans les erreurs.
func (p *Provider) LastTouch(ctx context.Context, owner, name, head, file string) (string, bool, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return "", false, err
	}
	if err := provider.ValidateSHA(head); err != nil {
		return "", false, err
	}
	if err := provider.ValidatePath(file); err != nil {
		return "", false, err
	}
	op := i18n.T("provider.git.op.last_touch", provider.ShortSHA(head), owner, name)
	if !p.HasClone(owner, name) {
		return "", false, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return "", false, err
	}
	_, found, err := p.resolve(ctx, op, path, head)
	if err != nil {
		return "", false, err
	}
	if !found {
		return "", false, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	// Un fichier supprimé de head n'a pas de « dernière modification » : le dernier commit qui l'a touché
	// serait sa suppression.
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "ls-tree",
		"--literal-pathspecs", "ls-tree", "-z", "--end-of-options", head, "--", file)
	if err != nil {
		return "", false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return "", false, localError(op, res)
	}
	if !lsTreeBlob(res.stdout, file) {
		return "", false, nil
	}
	res, err = p.run(ctx, p.localTimeout, gitDir(path), "log",
		"--literal-pathspecs", "log", "-1", "--format=%H", "--no-color", "--end-of-options", head, "--", file)
	if err != nil {
		return "", false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return "", false, localError(op, res)
	}
	sha := strings.TrimSpace(string(res.stdout))
	if sha == "" {
		return "", false, nil
	}
	if provider.ValidateSHA(sha) != nil {
		return "", false, localError(op, res)
	}
	return sha, true, nil
}

// lsTreeBlob indique que la sortie de git ls-tree -z (entrées « <mode> <type> <objet>\t<chemin> »
// terminées par NUL, chemins non échappés) contient le fichier file comme blob.
func lsTreeBlob(out []byte, file string) bool {
	for line := range strings.SplitSeq(string(out), "\x00") {
		meta, path, ok := strings.Cut(line, "\t")
		if !ok || path != file {
			continue
		}
		if f := strings.Fields(meta); len(f) == 3 && f[1] == "blob" {
			return true
		}
	}
	return false
}
