package git

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// FirstAdded renvoie la date d'écriture (UTC) du plus ancien commit de la branche par défaut du clone
// de owner/name (HEAD, après Prepare) qui ajoute un fichier correspondant à l'un des pathspecs ; ok
// faux si aucun. Sert au contexte D4 du diagnostic (premier fichier de configuration d'agent) :
//
//	git log --diff-filter=A --no-renames --format=%aI HEAD -- <pathspecs>
//
// Les pathspecs viennent de Watchoo (internal/diagnostic), jamais d'un dépôt. Le clone est d'abord créé
// ou mis à jour (voir CommitStatsBatch). Aucun chemin n'est écrit dans les logs.
func (p *Provider) FirstAdded(ctx context.Context, owner, name string, pathspecs []string) (time.Time, bool, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return time.Time{}, false, err
	}
	if len(pathspecs) == 0 {
		return time.Time{}, false, nil
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return time.Time{}, false, err
	}
	op := i18n.T("provider.git.op.first_added", owner, name)
	if _, found, err := p.resolve(ctx, op, path, "HEAD"); err != nil || !found {
		return time.Time{}, false, err // dépôt vide : rien d'ajouté
	}
	args := []string{"log", "--diff-filter=A", "--no-renames", "--no-color", "--format=%aI", "HEAD", "--"}
	args = append(args, pathspecs...)
	res, err := p.run(ctx, p.localTimeout, gitDir(path), "log", args...)
	if err != nil {
		return time.Time{}, false, &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return time.Time{}, false, localError(op, res)
	}
	return oldestDate(res.stdout)
}

// oldestDate lit des dates ISO 8601 (une par ligne, %aI) et renvoie la plus ancienne, en UTC ; ok faux
// si aucune ligne n'est une date.
func oldestDate(out []byte) (time.Time, bool, error) {
	var oldest time.Time
	for line := range bytes.Lines(out) {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(string(line)))
		if err != nil {
			continue
		}
		if oldest.IsZero() || t.Before(oldest) {
			oldest = t
		}
	}
	if oldest.IsZero() {
		return time.Time{}, false, nil
	}
	return oldest.UTC().Truncate(time.Second), true, nil
}
