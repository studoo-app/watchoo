package sync

// Reprise de l'historique du diagnostic (phase 7.3, « watchoo diagnostic backfill ») : mesure, sur les
// clones Git locaux, les commits déjà en base sans signaux et plus récents que la durée de
// conservation (hors fusions), comme la synchro le fait pour les nouveaux commits. Le message n'est lu
// qu'en mémoire ; les logs ne citent ni message, ni e-mail, ni chemin.

import (
	"context"
	"errors"
	"time"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// ErrDiagnosticDisabled : la reprise est demandée alors que diagnostic.enabled est faux.
var ErrDiagnosticDisabled = i18nError{"sync.backfill.disabled"}

// backfillPage est le nombre de commits repris par paquet (une lecture en base, un calcul sur le clone).
const backfillPage = 500

// BackfillRepo est le bilan de la reprise d'un dépôt.
type BackfillRepo struct {
	Repo     model.Repository
	Measured int    // commits mesurés (lignes commit_signals créées)
	Merges   int    // commits de fusion écartés
	Missing  int    // commits absents du clone (branche non suivie localement, historique réécrit…)
	Error    string // échec du dépôt (clone indisponible…), dans la langue du processus ; vide sinon
}

// BackfillSignals reprend l'historique des dépôts suivis (repoID, 0 = tous). Erreur ErrDiagnosticDisabled
// si le diagnostic est désactivé ; une erreur de base ou un contexte terminé arrête tout ; l'échec d'un
// clone est noté dans le bilan de son dépôt. Les signaux trop anciens pour la durée de conservation ne
// sont pas calculés.
func (e *Engine) BackfillSignals(ctx context.Context, repoID int64) ([]BackfillRepo, error) {
	if e.diag == nil {
		return nil, ErrDiagnosticDisabled
	}
	all, err := e.store.ListRepositories(ctx)
	if err != nil {
		return nil, err
	}
	gp, err := e.newGit(CloneRefs(all))
	if err == nil && gp == nil {
		err = errors.New(i18n.T("sync.engine.no_repos_dir"))
	}
	if err != nil {
		return nil, errWrapped(i18n.T("sync.backfill.clones"), err)
	}
	cs, ok := gp.(CloneStats)
	if !ok {
		return nil, errors.New(i18n.T("sync.engine.no_repos_dir"))
	}
	since := time.Time{}
	if days := e.cfg.Diagnostic.RetentionDays; days > 0 {
		since = e.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	}
	out := []BackfillRepo{}
	for _, repo := range all {
		if (repoID != 0 && repo.ID != repoID) || !repo.Enabled || repo.DefaultBranch == "" {
			continue
		}
		rep, err := e.backfillRepo(ctx, cs, repo, since)
		if err != nil {
			return out, err
		}
		out = append(out, rep)
	}
	return out, nil
}

// localClone est implémenté par le provider des clones Git locaux : lecture d'un clone existant sans
// mise à jour.
type localClone interface {
	HasClone(owner, name string) bool
	CommitStatsBatchLocal(ctx context.Context, owner, name string, shas []string) (map[string]provider.CommitStats, error)
}

// backfillRepo reprend un dépôt ; erreur seulement pour une base en échec ou un contexte terminé.
//
// Un clone qui existe déjà est lu tel quel, sans « git fetch » (la synchro le tient à jour) : la reprise
// peut tourner pendant la synchro d'un autre processus (« watchoo serve ») sans toucher au clone. Sans
// clone, il est créé d'abord (Prepare).
func (e *Engine) backfillRepo(ctx context.Context, cs CloneStats, repo model.Repository, since time.Time) (BackfillRepo, error) {
	rep := BackfillRepo{Repo: repo}
	full := repo.FullName()
	stats := cs.CommitStatsBatch
	if local, ok := cs.(localClone); ok && local.HasClone(repo.Owner, repo.Name) {
		stats = local.CommitStatsBatchLocal
	} else if err := cs.Prepare(ctx, repo.Owner, repo.Name, repo.DefaultBranch); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return rep, ctxErr
		}
		e.logger.Info("reprise du diagnostic : clone indisponible", "repo", full, "erreur", errorMessage(err))
		rep.Error = errorMessage(err)
		return rep, nil
	}
	after := ""
	for {
		page, err := e.store.CommitsWithoutSignals(ctx, repo.ID, since, after, backfillPage)
		if err != nil {
			return rep, err
		}
		if len(page) == 0 {
			break
		}
		shas := make([]string, len(page))
		for i, c := range page {
			shas[i] = c.SHA
		}
		got, err := stats(ctx, repo.Owner, repo.Name, shas)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return rep, ctxErr
			}
			rep.Error = errorMessage(err)
			return rep, nil
		}
		measured := make([]string, 0, len(page))
		for _, c := range page {
			if st, ok := got[c.SHA]; ok && !st.Merge && st.Files != nil {
				measured = append(measured, c.SHA)
			}
		}
		forms := e.measureForms(ctx, cs, repo.Owner, repo.Name, full, measured)
		if err := ctx.Err(); err != nil {
			return rep, err
		}
		rows := make([]store.CommitSignalWrite, 0, len(page))
		for _, c := range page {
			st, ok := got[c.SHA]
			switch {
			case !ok:
				rep.Missing++
				continue
			case st.Merge:
				rep.Merges++
				continue
			}
			sig, ok := diagnostic.Compute(diagnostic.Input{Message: c.Message, AuthorName: c.AuthorName,
				AuthorLogin: c.AuthorLogin, Files: st.Files}, e.diag)
			if !ok {
				rep.Missing++
				continue
			}
			if f, ok := forms[c.SHA]; ok {
				sig.SetForm(f)
			}
			rows = append(rows, signalWrite(c.SHA, sig))
		}
		n, err := e.store.WriteCommitSignals(ctx, repo.ID, rows, e.now().UTC())
		if err != nil {
			return rep, err
		}
		rep.Measured += n
		if len(page) < backfillPage {
			break
		}
		after = page[len(page)-1].SHA
	}
	s := &repoSync{e: e, owner: repo.Owner, name: repo.Name, full: full, clone: cs}
	e.updateAgentContext(ctx, s, repo.ID)
	e.logger.Info("reprise du diagnostic", "repo", full, "commits", rep.Measured, "fusions", rep.Merges,
		"absents", rep.Missing)
	return rep, nil
}

// patcher est implémenté par le provider des clones Git locaux (*git.Provider) : diff d'un commit lu
// sur le clone existant, sans le mettre à jour.
type patcher interface {
	CommitPatch(ctx context.Context, owner, name, sha string, maxBytes int) (string, bool, bool, error)
}

// CommitPatch renvoie le diff du commit sha du dépôt repo, lu sur son clone Git local existant (aucun
// accès réseau), coupé au-delà de maxBytes ; found faux si le clone ou le commit est absent. Sert à la
// fiche de revue du diagnostic ; le diff n'est jamais gardé ni journalisé.
func (e *Engine) CommitPatch(ctx context.Context, repo model.Repository, sha string, maxBytes int) (patch string, truncated, found bool, err error) {
	all, err := e.store.ListRepositories(ctx)
	if err != nil {
		return "", false, false, err
	}
	gp, err := e.newGit(CloneRefs(all))
	if err != nil || gp == nil {
		return "", false, false, err
	}
	p, ok := gp.(patcher)
	if !ok {
		return "", false, false, nil
	}
	return p.CommitPatch(ctx, repo.Owner, repo.Name, sha, maxBytes)
}
