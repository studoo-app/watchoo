package git

// Lectures de l'historique pour les pistes des conventions (phase 8.7), dans le clone EXISTANT, sans le
// mettre à jour (aucun accès réseau) : dernier commit qui a touché un ensemble de chemins (fraîcheur de la
// documentation DOC9, dates déclarées de l'INDEX DOC10), nombre de commits depuis un commit (DOC9) et
// premier ajout d'un fichier (introduction de l'outillage, TL5). Aucune sortie de git n'est recopiée dans
// les erreurs ; ni SHA complet ni chemin dans les logs.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// stampFormat est le format d'un commit lu par les pistes : SHA complet, NUL, date du committer (ISO 8601
// strict).
const stampFormat = "--format=%H%x00%cI"

// leadPathspecs renvoie les pathspecs de include et exclude, chemins relatifs à la racine pris à la lettre
// (« :(top,literal) » et « :(top,exclude,literal) ») : la magie des pathspecs n'est jamais lue dans un
// chemin du dépôt. Sans include, tout l'arbre est inclus (exclusions seules). Chaque chemin est vérifié
// (provider.ValidatePath).
func leadPathspecs(include, exclude []string) ([]string, error) {
	out := make([]string, 0, len(include)+len(exclude))
	for _, p := range include {
		if err := provider.ValidatePath(p); err != nil {
			return nil, err
		}
		out = append(out, ":(top,literal)"+p)
	}
	for _, p := range exclude {
		if err := provider.ValidatePath(p); err != nil {
			return nil, err
		}
		out = append(out, ":(top,exclude,literal)"+p)
	}
	if len(include) == 0 && len(exclude) > 0 {
		out = append([]string{":(top)"}, out...)
	}
	return out, nil
}

// leadClone vérifie le dépôt et head, puis renvoie le chemin du clone ; head absent du clone, ou pas de
// clone : provider.ErrNotFound.
func (p *Provider) leadClone(ctx context.Context, op, owner, name, head string) (string, error) {
	if !p.HasClone(owner, name) {
		return "", &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	path, err := p.layout.Path(owner, name)
	if err != nil {
		return "", err
	}
	_, found, err := p.resolve(ctx, op, path, head)
	if err != nil {
		return "", err
	}
	if !found {
		return "", &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	return path, nil
}

// parseStamp lit une ligne « <SHA>\x00<date> » de stampFormat ; faux si elle est mal formée.
func parseStamp(line string) (string, time.Time, bool) {
	sha, at, ok := strings.Cut(strings.TrimSpace(line), "\x00")
	if !ok || provider.ValidateSHA(sha) != nil {
		return "", time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return "", time.Time{}, false
	}
	return sha, t.UTC(), true
}

// LastCommit renvoie le dernier commit de l'historique de head (SHA complet) qui a modifié l'un des
// chemins include (fichiers ou dossiers, relatifs à la racine ; vide : tout l'arbre) hors des chemins
// exclude, et sa date de committer en UTC, dans le clone EXISTANT de owner/name, lu tel quel :
//
//	git log -1 --format=%H%x00%cI --end-of-options <head> -- :(top,literal)<include>… :(top,exclude,literal)<exclude>…
//
// found faux si aucun commit ne les a modifiés. Contrairement à LastTouch, un chemin absent de head n'est
// pas écarté (un dossier n'est pas un blob). head absent du clone, ou pas de clone : provider.ErrNotFound.
// Les chemins n'apparaissent jamais dans les erreurs.
func (p *Provider) LastCommit(ctx context.Context, owner, name, head string, include, exclude []string) (string,
	time.Time, bool, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return "", time.Time{}, false, err
	}
	if err := provider.ValidateSHA(head); err != nil {
		return "", time.Time{}, false, err
	}
	specs, err := leadPathspecs(include, exclude)
	if err != nil {
		return "", time.Time{}, false, err
	}
	op := i18n.T("provider.git.op.last_commit", provider.ShortSHA(head), owner, name)
	path, err := p.leadClone(ctx, op, owner, name, head)
	if err != nil {
		return "", time.Time{}, false, err
	}
	args := []string{"log", "-1", stampFormat, "--no-color", "--end-of-options", head}
	if len(specs) > 0 {
		args = append(append(args, "--"), specs...)
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "log", args...)
	if err != nil {
		return "", time.Time{}, false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return "", time.Time{}, false, localError(op, res)
	}
	if strings.TrimSpace(string(res.stdout)) == "" {
		return "", time.Time{}, false, nil
	}
	sha, at, ok := parseStamp(string(res.stdout))
	if !ok {
		return "", time.Time{}, false, localError(op, res)
	}
	return sha, at, true, nil
}

// CountCommits compte les commits de l'historique de head, pas de celui de since (SHA complets), qui ont
// modifié l'un des chemins include (vide : tout l'arbre) hors des chemins exclude, dans le clone EXISTANT
// de owner/name, lu tel quel :
//
//	git rev-list --count --end-of-options <head> ^<since> -- :(top,literal)<include>… :(top,exclude,literal)<exclude>…
//
// head ou since absent du clone, ou pas de clone : provider.ErrNotFound.
func (p *Provider) CountCommits(ctx context.Context, owner, name, head, since string, include, exclude []string) (int,
	error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return 0, err
	}
	for _, sha := range []string{head, since} {
		if err := provider.ValidateSHA(sha); err != nil {
			return 0, err
		}
	}
	specs, err := leadPathspecs(include, exclude)
	if err != nil {
		return 0, err
	}
	op := i18n.T("provider.git.op.count_commits", provider.ShortSHA(since), provider.ShortSHA(head), owner, name)
	path, err := p.leadClone(ctx, op, owner, name, head)
	if err != nil {
		return 0, err
	}
	if _, found, err := p.resolve(ctx, op, path, since); err != nil {
		return 0, err
	} else if !found {
		return 0, &provider.APIError{Op: op, Err: provider.ErrNotFound}
	}
	args := []string{"rev-list", "--count", "--end-of-options", head, "^" + since}
	if len(specs) > 0 {
		args = append(append(args, "--"), specs...)
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "rev-list", args...)
	if err != nil {
		return 0, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return 0, localError(op, res)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(res.stdout)))
	if err != nil || n < 0 {
		return 0, localError(op, res)
	}
	return n, nil
}

// FirstAdd renvoie le plus ancien commit de l'historique de head (SHA complet) qui a ajouté le fichier
// file (chemin relatif à la racine), et sa date de committer en UTC, dans le clone EXISTANT de owner/name,
// lu tel quel :
//
//	git --literal-pathspecs log --diff-filter=A --no-renames --full-history --format=%H%x00%cI --end-of-options <head> -- <file>
//
// Un fichier supprimé puis réajouté donne son premier ajout (le plus ancien par date de committer) ; un
// renommage compte comme un ajout (--no-renames) ; les ajouts des branches fusionnées comptent
// (--full-history). found faux si aucun commit de head ne l'a ajouté. head absent du clone, ou pas de clone :
// provider.ErrNotFound. Le chemin n'apparaît jamais dans les erreurs.
func (p *Provider) FirstAdd(ctx context.Context, owner, name, head, file string) (string, time.Time, bool, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return "", time.Time{}, false, err
	}
	if err := provider.ValidateSHA(head); err != nil {
		return "", time.Time{}, false, err
	}
	if err := provider.ValidatePath(file); err != nil {
		return "", time.Time{}, false, err
	}
	op := i18n.T("provider.git.op.first_add", provider.ShortSHA(head), owner, name)
	path, err := p.leadClone(ctx, op, owner, name, head)
	if err != nil {
		return "", time.Time{}, false, err
	}
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "log",
		"--literal-pathspecs", "log", "--diff-filter=A", "--no-renames", "--full-history", stampFormat, "--no-color",
		"--end-of-options", head, "--", file)
	if err != nil {
		return "", time.Time{}, false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return "", time.Time{}, false, localError(op, res)
	}
	var first string
	var firstAt time.Time
	for line := range strings.SplitSeq(string(res.stdout), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		sha, at, ok := parseStamp(line)
		if !ok {
			return "", time.Time{}, false, localError(op, res)
		}
		// Le plus ancien par date de committer : l'ordre de git log n'est pas strictement chronologique.
		if first == "" || at.Before(firstAt) {
			first, firstAt = sha, at
		}
	}
	return first, firstAt, first != "", nil
}
