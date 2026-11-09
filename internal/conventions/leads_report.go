package conventions

// Pistes d'un dépôt dans le rapport (Build, phase 8.7) : les pistes de l'arbre sont relues dans le cache
// des critères de fichiers (FileResults.Leads), celles de l'historique (DOC9, DOC10, TL5) sont calculées à
// la lecture, seulement dans la vue détaillée d'un dépôt (BuildOptions.FileDetail), depuis le clone
// existant (LeadHistoryReader) et la base. Rien n'est stocké ni journalisé ; une piste n'entre dans aucune
// grille, ni dans la revue de jalon, ni dans aucun taux.

import (
	"context"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// LeadReport est l'état des pistes d'un dépôt.
type LeadReport struct {
	Leads []LeadCheck // toutes les pistes, dans l'ordre de LeadCriteria
}

// Find renvoie la piste id ; faux si elle n'y est pas.
func (r LeadReport) Find(id string) (LeadCheck, bool) { return FindLead(r.Leads, id) }

// ToReview compte les pistes à relire (LeadReview).
func (r LeadReport) ToReview() int {
	n := 0
	for _, l := range r.Leads {
		if l.Status == LeadReview {
			n++
		}
	}
	return n
}

// UnknownLeadReport renvoie toutes les pistes inconnues pour la raison donnée.
func UnknownLeadReport(reason string) LeadReport {
	return LeadReport{Leads: UnknownLeads(LeadCriteria, reason)}
}

// buildLeads assemble les pistes du dépôt : celles de l'arbre depuis rep.Files (même raison d'inconnu que
// les critères de fichiers), celles de l'historique calculées à la lecture. Rapport restreint à des
// contributeurs : toutes inconnues (UnknownFiltered). kept est l'arbre lu par buildFiles, réutilisé s'il est
// celui de la tête. Seule une erreur de base ou un contexte terminé est une erreur.
func buildLeads(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, filtered bool,
	rows []model.ConventionCommit, tags []model.Tag, opts BuildOptions, kept detailTree, rep *RepoReport) error {
	if filtered {
		rep.Leads = UnknownLeadReport(UnknownFiltered)
		return nil
	}
	history, err := historyLeads(ctx, r, clones, repo, rows, opts, kept, rep.Commits)
	if err != nil {
		return err
	}
	fallback := rep.Files.Reason
	if fallback == "" {
		fallback = UnknownStale
	}
	out := make([]LeadCheck, 0, len(LeadCriteria))
	for _, id := range LeadCriteria {
		if l, ok := history[id]; ok {
			out = append(out, l)
			continue
		}
		if l, ok := FindLead(rep.Files.Results.Leads, id); ok {
			out = append(out, l)
			continue
		}
		out = append(out, unknownLead(id, fallback))
	}
	rep.Leads = LeadReport{Leads: out}
	return nil
}

// historyLeads calcule DOC9, DOC10 et TL5 sur l'arbre et l'historique de la tête de la branche par défaut
// (dernier commit connu en base). Hors vue détaillée : inconnues (detail_only) ; sans clone ou sans
// lecteur d'historique : inconnues (no_clone). Branche par défaut non suivie : la tête est le commit de
// l'arbre lu par buildFiles (celui que la synchro a mesuré), sinon inconnues (not_computed) ; arbre
// illisible : inconnues (clone_unreadable).
func historyLeads(ctx context.Context, r Reader, clones CloneReader, repo model.Repository,
	rows []model.ConventionCommit, opts BuildOptions, kept detailTree, commits []CommitLine) (map[string]LeadCheck, error) {
	unknown := func(reason string) map[string]LeadCheck {
		out := map[string]LeadCheck{}
		for _, id := range HistoryLeadCriteria {
			out[id] = unknownLead(id, reason)
		}
		return out
	}
	if !opts.FileDetail {
		return unknown(UnknownDetailOnly), nil
	}
	hr, okHistory := clones.(LeadHistoryReader)
	fr, okFiles := clones.(FileReader)
	if !hasClone(clones, repo) || !okHistory || !okFiles {
		return unknown(UnknownNoClone), nil
	}
	head, err := defaultHeadSHA(ctx, r, repo)
	if err != nil {
		return nil, err
	}
	if head == "" {
		// Branche par défaut non suivie : la tête mesurée par la synchro (celle de l'arbre lu par
		// buildFiles) tient lieu de tête ; sans elle, rien n'est encore calculé.
		if kept.sha == "" {
			return unknown(UnknownNotComputed), nil
		}
		head = kept.sha
	}
	tree := kept.tree
	if kept.sha != head {
		tree, err = ReadFileTree(ctx, fr, repo.Owner, repo.Name, head, opts.Files)
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if err != nil {
			return unknown(UnknownCloneUnreadable), nil
		}
	}
	lr := leadReader{hr: hr, repo: repo, head: head}
	out := map[string]LeadCheck{}
	if out[CriterionDOC9], err = lr.doc9(ctx, tree, opts.Files); err != nil {
		return nil, err
	}
	if out[CriterionDOC10], err = lr.doc10(ctx, tree, opts); err != nil {
		return nil, err
	}
	if out[CriterionTL5], err = lr.tl5(ctx, tree, rows, commits); err != nil {
		return nil, err
	}
	return out, nil
}

// leadReader lit l'historique de la tête d'un dépôt pour les pistes.
type leadReader struct {
	hr   LeadHistoryReader
	repo model.Repository
	head string
}

// errUnreadable signale une lecture du clone en échec : la piste est inconnue (clone_unreadable).
type errUnreadable struct{}

func (errUnreadable) Error() string { return UnknownCloneUnreadable }

// readErr traduit l'erreur d'une lecture du clone : contexte terminé renvoyé tel quel, autre erreur en
// errUnreadable.
func readErr(ctx context.Context, err error) error {
	if cerr := ctx.Err(); cerr != nil {
		return cerr
	}
	if err != nil {
		return errUnreadable{}
	}
	return nil
}

// leadResult renvoie la piste, inconnue (clone_unreadable) si err est errUnreadable ; une autre erreur
// (contexte terminé) est rendue.
func leadResult(id string, lc LeadCheck, err error) (LeadCheck, error) {
	switch err.(type) {
	case nil:
		return lc, nil
	case errUnreadable:
		return unknownLead(id, UnknownCloneUnreadable), nil
	}
	return LeadCheck{}, err
}

// last renvoie la date du dernier commit qui a touché include hors exclude (zéro si aucun) et son SHA.
func (lr leadReader) last(ctx context.Context, include, exclude []string) (string, time.Time, error) {
	sha, at, found, err := lr.hr.LastCommit(ctx, lr.repo.Owner, lr.repo.Name, lr.head, include, exclude)
	if err := readErr(ctx, err); err != nil {
		return "", time.Time{}, err
	}
	if !found {
		return "", time.Time{}, nil
	}
	return sha, at, nil
}

// doc9 calcule DOC9 : dates du dernier commit sur le dossier de documentation, l'INDEX, le CHANGELOG et le
// reste de l'arbre (hors ces chemins et le README), puis commits hors documentation depuis le dernier
// commit de documentation.
func (lr leadReader) doc9(ctx context.Context, tree FileTree, rules FileRules) (LeadCheck, error) {
	paths, reason := docPathsOf(tree, rules)
	if reason != "" {
		return unknownLead(CriterionDOC9, reason), nil
	}
	docs := paths.all()
	if len(docs) == 0 {
		return unknownLead(CriterionDOC9, UnknownNoFile), nil
	}
	lc, err := func() (LeadCheck, error) {
		var s DocStamps
		for _, f := range []struct {
			path string
			at   *time.Time
		}{{paths.Docs, &s.Docs}, {paths.Index, &s.Index}, {paths.Changelog, &s.Changelog}} {
			if f.path == "" {
				continue
			}
			_, at, err := lr.last(ctx, []string{f.path}, nil)
			if err != nil {
				return LeadCheck{}, err
			}
			*f.at = at
		}
		since, at, err := lr.last(ctx, docs, nil)
		if err != nil {
			return LeadCheck{}, err
		}
		s.LastDoc = at
		if _, s.Code, err = lr.last(ctx, nil, docs); err != nil {
			return LeadCheck{}, err
		}
		if since != "" {
			n, err := lr.hr.CountCommits(ctx, lr.repo.Owner, lr.repo.Name, lr.head, since, nil, docs)
			if err := readErr(ctx, err); err != nil {
				return LeadCheck{}, err
			}
			s.CommitsSince = n
		}
		return CheckDOC9(s), nil
	}()
	return leadResult(CriterionDOC9, lc, err)
}

// doc10 calcule DOC10 : date déclarée de chaque ligne de l'INDEX comparée au dernier commit du document.
func (lr leadReader) doc10(ctx context.Context, tree FileTree, opts BuildOptions) (LeadCheck, error) {
	rows, l, reason := indexDates(tree, opts.Files, lr.repo.FullName())
	switch reason {
	case "":
	case UnknownNoDateColumn:
		lc := unknownLead(CriterionDOC10, reason)
		lc.Status = LeadNotApplicable
		return lc, nil
	default:
		return unknownLead(CriterionDOC10, reason), nil
	}
	last := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		sha, at, err := lr.last(ctx, []string{row.Path}, nil)
		if err != nil {
			return leadResult(CriterionDOC10, LeadCheck{}, err)
		}
		if sha != "" {
			last[row.Path] = at
		}
	}
	return checkDOC10(rows, l, last, opts.Location), nil
}

// tl5 calcule TL5 : plus ancienne introduction datée de l'outillage (premier ajout de ses fichiers), puis
// taux CC1 des commits évalués du périmètre avant et depuis, datés par leur committer (sinon leur auteur).
func (lr leadReader) tl5(ctx context.Context, tree FileTree, rows []model.ConventionCommit,
	commits []CommitLine) (LeadCheck, error) {
	files, undated, reason := toolingSources(tree)
	if reason != "" {
		return unknownLead(CriterionTL5, reason), nil
	}
	var intro time.Time
	for _, f := range files {
		_, at, found, err := lr.hr.FirstAdd(ctx, lr.repo.Owner, lr.repo.Name, lr.head, f)
		if err := readErr(ctx, err); err != nil {
			return leadResult(CriterionTL5, LeadCheck{}, err)
		}
		if found && (intro.IsZero() || at.Before(intro)) {
			intro = at
		}
	}
	committed := make(map[string]time.Time, len(rows))
	for _, row := range rows {
		committed[row.SHA] = row.CommittedAt
	}
	var dated []DatedCommit
	for _, c := range commits {
		if !c.Evaluated() {
			continue
		}
		at := committed[c.SHA]
		if at.IsZero() {
			at = c.AuthoredAt
		}
		dated = append(dated, DatedCommit{At: at, Conform: c.Conform})
	}
	return CheckTL5(intro, len(files), undated, dated), nil
}
