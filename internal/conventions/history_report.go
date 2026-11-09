package conventions

// Indicateurs de la phase 8.4 dans le rapport (Build) et la revue de jalon : réécritures observées de la
// branche par défaut (BR4), historique des tags (SV3) et échéances de SV2, d'après les tables de la synchro
// (branch_rewrites, tag_observations) ; pré-versions (SV4), saut de version (SV5), précédence (SV7) et
// fraîcheur du CHANGELOG (CL10), calculés à la lecture depuis le clone existant (HistoryReader) et la base.
// Rien n'est stocké ; aucun message, nom de tag ni SHA n'est journalisé. Indicateurs de dépôt : rien n'est
// attribué à une personne.

import (
	"context"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// HistoryReader lit l'historique du clone Git local existant, sans le mettre à jour (phase 8.4) ; le
// provider des clones l'implémente. Facultatif : un CloneReader qui ne l'implémente pas rend SV4 (pose par
// ascendance), SV5, SV7 (ascendance) et CL10 inconnus, comme sans clone.
type HistoryReader interface {
	// IsAncestor indique si ancestor est ancêtre de descendant (un commit est son propre ancêtre) ; un
	// commit absent du clone est une erreur, lue comme « inconnu ».
	IsAncestor(ctx context.Context, owner, name, ancestor, descendant string) (bool, error)
	// LastTouch renvoie le dernier commit de l'historique de head qui a modifié file (chemin relatif à la
	// racine) ; found faux si aucun.
	LastTouch(ctx context.Context, owner, name, head, file string) (sha string, found bool, err error)
}

// ancestryOf renvoie le test d'ascendance du clone du dépôt, mémorisé pour un calcul ; nil sans clone ou si
// clones ne sait pas le lire. Toute erreur (commit absent, clone illisible, contexte terminé) donne
// AncestryUnknown : l'appelant vérifie le contexte après usage.
func ancestryOf(ctx context.Context, clones CloneReader, repo model.Repository) AncestryFunc {
	if !hasClone(clones, repo) {
		return nil
	}
	hr, ok := clones.(HistoryReader)
	if !ok {
		return nil
	}
	memo := map[[2]string]Ancestry{}
	return func(ancestor, descendant string) Ancestry {
		key := [2]string{ancestor, descendant}
		if a, ok := memo[key]; ok {
			return a
		}
		a := AncestryUnknown
		if ctx.Err() == nil {
			switch yes, err := hr.IsAncestor(ctx, repo.Owner, repo.Name, ancestor, descendant); {
			case err != nil:
			case yes:
				a = AncestryYes
			default:
				a = AncestryNo
			}
		}
		memo[key] = a
		return a
	}
}

// buildHistory calcule les indicateurs de la phase 8.4 d'un dépôt : BR4, SV3 et les échéances de SV2
// (base, toujours), puis SV4, SV5, SV7 et CL10 (clone et base ; rapport restreint à des contributeurs :
// inconnus, UnknownFiltered). rows sont les commits du dépôt lus en base (tous les auteurs si non filtré).
// Seule une erreur de base ou un contexte terminé est une erreur.
func buildHistory(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, filtered bool,
	rows []model.ConventionCommit, tags []model.Tag, opts BuildOptions, rep *RepoReport) error {
	rewrites, err := r.BranchRewrites(ctx, repo.ID)
	if err != nil {
		return err
	}
	rep.Rewrites = BuildRewriteReport(rewrites)
	obs, err := r.TagObservations(ctx, repo.ID)
	if err != nil {
		return err
	}
	rep.TagHistory = BuildTagHistory(obs)
	if err := applyDeadlines(ctx, r, repo, obs, rep.TagHistory, rep.Tags.Milestones, opts.Location); err != nil {
		return err
	}
	if filtered {
		rep.Prereleases = PrereleaseReport{Reason: UnknownFiltered, Notes: []VersionNote{}}
		rep.Bumps = unknownBumps(UnknownFiltered)
		rep.Precedence = PrecedenceReport{Reason: UnknownFiltered, Notes: []VersionNote{}}
		rep.Freshness = UnknownFreshness(UnknownFiltered)
		return nil
	}
	anc := ancestryOf(ctx, clones, repo)
	rep.Prereleases = CheckPrereleases(tags, opts.PrereleaseForms, firstSeenByName(rep.TagHistory), anc)
	rep.Precedence = CheckPrecedence(tags, anc)
	byKey := make(map[string]model.ConventionCommit, len(rows))
	for _, row := range rows {
		byKey[row.SHA] = row
	}
	read := func(p BumpPair) ([]Commit, int, bool) {
		shas, err := revList(ctx, clones, repo, nil, []string{p.To.SHA}, []string{p.From.SHA})
		if err != nil {
			return nil, 0, false
		}
		var commits []Commit
		missing := 0
		for _, s := range shas {
			row, ok := byKey[s]
			if !ok {
				missing++
				continue
			}
			commits = append(commits, toCommit(row))
		}
		return commits, missing, true
	}
	rep.Bumps = CheckBumps(tags, anc, read, opts.Rules)
	if err := ctx.Err(); err != nil {
		return err
	}
	rep.Freshness, err = buildFreshness(ctx, r, clones, repo, byKey, rep.Tags.Milestones, tags, opts)
	if err != nil {
		return err
	}
	return ctx.Err()
}

// unknownBumps renvoie un SV5 inconnu pour la raison reason.
func unknownBumps(reason string) BumpReport {
	rep := BumpReport{Reason: reason, Lines: []BumpLine{}, Counts: map[string]int{}}
	for _, s := range AllBumpStatuses {
		rep.Counts[s] = 0
	}
	return rep
}

// firstSeenByName renvoie la première observation de chaque nom de tag.
func firstSeenByName(h TagHistoryReport) map[string]time.Time {
	out := make(map[string]time.Time, len(h.Tags))
	for _, t := range h.Tags {
		out[t.Name] = t.FirstSeenAt
	}
	return out
}

// applyDeadlines calcule l'échéance de SV2 de chaque jalon (ms, modifiés sur place) : première observation
// du tag de nom exact, date de son commit visé (base) et lectures complètes des tags prouvées par les
// observations (ObservationReads).
func applyDeadlines(ctx context.Context, r Reader, repo model.Repository, obs []model.TagObservation,
	hist TagHistoryReport, ms []MilestoneResult, loc *time.Location) error {
	need := false
	for _, m := range ms {
		if _, ok := m.DueEnd(loc); ok && m.Status == MilestonePresent {
			need = true
		}
	}
	if !need {
		return nil
	}
	dates, err := r.ConventionTagDates(ctx, repo.ID)
	if err != nil {
		return err
	}
	reads := ObservationReads(obs)
	seen := firstSeenByName(hist)
	for i := range ms {
		ms[i].Deadline = CheckDeadline(ms[i], loc, seen[ms[i].Tag], dates[ms[i].Tag], reads)
	}
	return nil
}

// buildFreshness calcule CL10 : commits « feat » et « fix » de la branche par défaut depuis le dernier
// commit qui a touché le CHANGELOG à la racine de sa tête connue en base, lus dans le clone ; grisé si CL9
// du dernier jalon posé (plus grande version parmi les jalons dont un tag existe) est conforme dans le
// cache de la synchro.
func buildFreshness(ctx context.Context, r Reader, clones CloneReader, repo model.Repository,
	byKey map[string]model.ConventionCommit, ms []MilestoneResult, tags []model.Tag, opts BuildOptions) (ChangelogFreshness, error) {
	hr, ok := clones.(HistoryReader)
	if !hasClone(clones, repo) || !ok {
		return UnknownFreshness(UnknownNoClone), nil
	}
	head, err := defaultHeadSHA(ctx, r, repo)
	if err != nil {
		return ChangelogFreshness{}, err
	}
	if head == "" {
		return UnknownFreshness(UnknownCloneUnreadable), nil
	}
	since, found, err := hr.LastTouch(ctx, repo.Owner, repo.Name, head, ChangelogFile)
	if cerr := ctx.Err(); cerr != nil {
		return ChangelogFreshness{}, cerr
	}
	switch {
	case err != nil:
		return UnknownFreshness(UnknownCloneUnreadable), nil
	case !found:
		return UnknownFreshness(UnknownNoFile), nil
	}
	shas, err := revList(ctx, clones, repo, nil, []string{head}, []string{since})
	if cerr := ctx.Err(); cerr != nil {
		return ChangelogFreshness{}, cerr
	}
	if err != nil {
		return UnknownFreshness(UnknownCloneUnreadable), nil
	}
	grayed, err := lastMilestoneCL9(ctx, r, clones, repo, ms, tags, opts)
	if err != nil {
		return ChangelogFreshness{}, err
	}
	return CheckFreshness(since, shas, byKey, opts.Rules, grayed), nil
}

// lastMilestoneCL9 indique que CL9 du dernier jalon posé (plus grande version parmi les jalons dont un tag
// existe) est conforme dans le cache de la synchro ; faux sans jalon posé ou sans résultat à jour.
func lastMilestoneCL9(ctx context.Context, r Reader, clones CloneReader, repo model.Repository,
	ms []MilestoneResult, tags []model.Tag, opts BuildOptions) (bool, error) {
	last := -1
	for i, m := range ms {
		if m.Tag != "" && (last < 0 || m.Version.Compare(ms[last].Version) > 0) {
			last = i
		}
	}
	if last < 0 {
		return false, nil
	}
	rows, err := r.RepositoryConventions(ctx, repo.ID)
	if err != nil {
		return false, err
	}
	sha := ""
	for _, t := range tags {
		if t.Name == ms[last].Tag {
			sha = t.SHA
		}
	}
	res, _, reason := cachedFiles(rows, model.ConventionRevTag, ms[last].Tag, sha,
		FileConfigHash(opts.Files, repo.FullName(), tags), hasClone(clones, repo))
	if reason != "" {
		return false, nil
	}
	c, ok := res.Find(CriterionCL9)
	return ok && c.Status == CriterionConform, nil
}
