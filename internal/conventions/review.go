package conventions

// Revue de jalon (RJ, phase 8.2) : pour un jalon configuré, une liste de critères par dépôt, sans note
// globale (« k/n critères conformes, u inconnus »). Phase 8.2 : CC1 sur l'intervalle du jalon, BR1 (état
// courant), SV1 et SV2 du jalon ; phase 8.3 : critères de fichiers sur l'arbre du tag du jalon (CL9, CL8,
// RD1, section installation de RD2, RD3, DOC1, DOC3), lus dans le cache de la synchro ; phase 8.4 :
// échéance de SV2 et SV3 du tag du jalon ; phase 8.5 : PR3 = 0 sur l'intervalle du jalon (premier parent) ;
// phase 8.6 : PR4, PR fusionnées dans l'intervalle du jalon toutes relues. Indicateurs de dépôt : rien
// n'est attribué à une personne.

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// ErrUnknownMilestone signale un jalon absent de conventions.tags.milestones.
var ErrUnknownMilestone = errors.New("conventions: milestone not configured")

// Critères de la revue de jalon de la phase 8.2 (clés --json) ; ceux de fichiers (phase 8.3) sont
// déclarés dans files.go.
const (
	CriterionCC1 = "CC1" // 100 % de commits conformes sur l'intervalle du jalon
	CriterionBR1 = "BR1" // noms de branche conformes (état courant)
	CriterionSV1 = "SV1" // nom du tag du jalon conforme
	CriterionSV2 = "SV2" // jalon présent (à l'heure si une échéance est configurée, phase 8.4)
	CriterionPR3 = "PR3" // aucun commit arrivé hors PR sur l'intervalle du jalon (phase 8.5)
	CriterionPR4 = "PR4" // PR fusionnées dans l'intervalle du jalon toutes relues (phase 8.6)
)

// AllCriteria liste les critères de la revue de jalon, dans leur ordre d'affichage : CC1, BR1 (phase 8.2),
// PR3 (phase 8.5) et PR4 (phase 8.6) : avec BR1, « branches par fonctionnalité, PR relues », SV1 et SV2
// (phase 8.2), SV3 (phase 8.4 : avec SV1 et SV2, « tags de jalon jamais réécrits »), puis les critères de
// fichiers mesurés sur l'arbre du tag du jalon (phase 8.3). Dans la revue, RD2 ne juge que la section installation du
// README.
var AllCriteria = []string{CriterionCC1, CriterionBR1, CriterionPR3, CriterionPR4, CriterionSV1, CriterionSV2, CriterionSV3,
	CriterionCL9, CriterionCL8, CriterionRD1, CriterionRD2, CriterionRD3, CriterionDOC1, CriterionDOC3}

// États d'un critère (clés --json).
const (
	CriterionConform    = "conform"
	CriterionNonconform = "nonconform"
	CriterionUnknown    = "unknown"
)

// Raisons propres à la revue de jalon d'un critère inconnu ou d'un dépôt non évaluable (clés --json).
const (
	UnknownNoSince   = "no_since"   // conventions.since vide (espace et dépôt) : revue non évaluable
	UnknownNoTag     = "no_tag"     // tag du jalon absent, sans échéance : intervalle et SV1 non évaluables
	UnknownNotDue    = "not_due"    // tag absent avant la fin de l'échéance
	UnknownTagAbsent = "tag_absent" // SV1 : tag du jalon toujours absent après l'échéance (aucun nom à juger)
	UnknownNoCommits = "no_commits" // aucun commit évalué dans l'intervalle
	UnknownPartial   = "partial"    // commits de l'intervalle absents de la base : 100 % non vérifiable
	UnknownNoBranch  = "no_branch"  // aucune branche à évaluer
)

// Criterion est l'état d'un critère de la revue de jalon.
type Criterion struct {
	ID     string // CriterionCC1…
	Status string // CriterionConform, CriterionNonconform ou CriterionUnknown
	Reason string // raison d'un état inconnu ; vide sinon
}

// MilestoneInterval est l'intervalle des commits d'un jalon et son CC1.
type MilestoneInterval struct {
	From string // tag final précédent ; vide : depuis le début de l'historique
	To   string // tag qui désigne le jalon ; vide si l'intervalle est borné par l'échéance
	Due  string // échéance AAAA-MM-JJ qui borne l'intervalle (tag absent) ; vide sinon
	Tally
	BeforeSince int // commits de l'intervalle antérieurs à la date de début
	Missing     int // commits de l'intervalle absents de la base (non évalués)
}

// RepoMilestoneReview est la revue d'un jalon pour un dépôt.
type RepoMilestoneReview struct {
	Repo model.Repository
	// Evaluable est faux sans date de début (Reason vaut alors UnknownNoSince, sans critère).
	Evaluable bool
	Reason    string
	SinceDate string             // date de début effective, AAAA-MM-JJ ; vide si aucune
	Milestone MilestoneResult    // SV2 du jalon dans le dépôt (échéance dans Milestone.Deadline)
	Tag       *TagResult         // SV1 du tag qui désigne le jalon ; nil si absent
	History   *TagHistory        // SV3 du tag qui désigne le jalon ; nil s'il est absent ou jamais observé
	Interval  *MilestoneInterval // CC1 de l'intervalle ; nil si non mesuré
	Branches  BranchReport       // BR1, état courant
	// Arrivals est PR3 sur l'intervalle du jalon (premier parent, tous auteurs) ; nil si l'intervalle n'est
	// pas évaluable (tag absent sans échéance, ou avant la fin de l'échéance).
	Arrivals *ArrivalReport
	// Reviews est PR4 sur l'intervalle du jalon (phase 8.6) ; nil si non évaluable (intervalle, API ou date
	// de tag).
	Reviews  *MilestoneReviews
	Files    *MilestoneFiles // critères de fichiers sur l'arbre du tag (phase 8.3) ; nil si non évaluable
	Criteria []Criterion     // dans l'ordre d'AllCriteria ; vide non nil si non évaluable
	Conform  int             // critères conformes
	Unknown  int             // critères inconnus
}

// MilestoneReview est la revue d'un jalon, dépôt par dépôt (ordre de ListRepositories, aucun tri par
// conformité).
type MilestoneReview struct {
	Milestone Milestone
	Repos     []RepoMilestoneReview
}

// ReviewMilestone calcule la revue du jalon name (un jalon de opts.Milestones, sinon ErrUnknownMilestone)
// pour les dépôts de f (f.AuthorIDs est ignoré : la revue porte sur le dépôt). now situe l'échéance
// (zéro : maintenant). clones peut être nil : CC1 est alors inconnu.
func ReviewMilestone(ctx context.Context, r Reader, clones CloneReader, f Filter, name string, opts BuildOptions,
	now time.Time) (MilestoneReview, error) {
	m, ok := FindMilestone(opts.Milestones, name)
	if !ok {
		return MilestoneReview{}, ErrUnknownMilestone
	}
	if now.IsZero() {
		now = time.Now()
	}
	out := MilestoneReview{Milestone: m, Repos: []RepoMilestoneReview{}}
	env, err := newBuildEnv(opts)
	if err != nil {
		return out, err
	}
	repos, err := r.ListRepositories(ctx)
	if err != nil {
		return out, err
	}
	for _, repo := range repos {
		if f.RepoID != 0 && repo.ID != f.RepoID {
			continue
		}
		rv, err := reviewRepo(ctx, r, clones, repo, m, opts, env, now)
		if err != nil {
			return out, err
		}
		out.Repos = append(out.Repos, rv)
	}
	return out, nil
}

// reviewRepo calcule la revue du jalon m pour un dépôt.
func reviewRepo(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, m Milestone,
	opts BuildOptions, env *buildEnv, now time.Time) (RepoMilestoneReview, error) {
	out := RepoMilestoneReview{Repo: repo, Criteria: []Criterion{}, Branches: BranchReport{Branches: []BranchLine{}}}
	since, err := EffectiveSinceAt(opts.Since, repo.ConventionsSince, opts.Location)
	if err != nil {
		return out, err
	}
	tags, err := r.ConventionTags(ctx, repo.ID)
	if err != nil {
		return out, err
	}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	out.Milestone = CheckMilestones([]Milestone{m}, sortedNames(names))[0]
	if since.IsZero() {
		out.Reason = UnknownNoSince
		return out, nil
	}
	out.Evaluable = true
	out.SinceDate = since.Format(SinceInputLayout)

	cc1, err := reviewInterval(ctx, r, clones, repo, m, out.Milestone, tags, since, opts, now, &out)
	if err != nil {
		return out, err
	}
	pulls, err := readPulls(ctx, r, repo, opts)
	if err != nil {
		return out, err
	}
	if out.Branches, err = buildBranches(ctx, r, clones, repo, pulls, env.branches); err != nil {
		return out, err
	}
	br1 := Criterion{ID: CriterionBR1, Status: CriterionConform}
	switch b := out.Branches; {
	case b.Checked == 0:
		br1.Status, br1.Reason = CriterionUnknown, UnknownNoBranch
	case b.Conform < b.Checked:
		br1.Status = CriterionNonconform
	}
	pr3, err := reviewArrivals(ctx, r, clones, repo, m, out.Milestone, tags, since, pulls, opts, now, &out)
	if err != nil {
		return out, err
	}
	pr4, err := reviewPullReviews(ctx, r, repo, m, out.Milestone, tags, since, pulls, opts, now, &out)
	if err != nil {
		return out, err
	}

	sv1 := Criterion{ID: CriterionSV1, Status: CriterionUnknown, Reason: UnknownNoTag}
	if end, ok := m.DueEnd(opts.Location); ok && out.Milestone.Tag == "" {
		sv1.Reason = UnknownTagAbsent
		if now.Before(end) {
			sv1.Reason = UnknownNotDue
		}
	}
	if out.Milestone.Tag != "" {
		t := CheckTag(out.Milestone.Tag)
		out.Tag = &t
		sv1 = Criterion{ID: CriterionSV1, Status: CriterionNonconform}
		if t.Conform() {
			sv1.Status = CriterionConform
		}
	}
	obs, err := r.TagObservations(ctx, repo.ID)
	if err != nil {
		return out, err
	}
	hist := BuildTagHistory(obs)
	ms := []MilestoneResult{out.Milestone}
	if err := applyDeadlines(ctx, r, repo, obs, hist, ms, opts.Location); err != nil {
		return out, err
	}
	out.Milestone = ms[0]
	sv2 := MilestoneSV2(out.Milestone, out.Milestone.Deadline, opts.Location, now)
	sv3Reason := ""
	if out.Milestone.Tag == "" {
		sv3Reason = sv1.Reason
	}
	sv3 := MilestoneSV3(out.Milestone, hist, sv3Reason)
	if h, ok := FindTagHistory(hist, out.Milestone.Tag); ok && out.Milestone.Tag != "" {
		out.History = &h
	}
	files, mf, err := reviewFiles(ctx, r, clones, repo, out.Milestone, tags, sv1.Reason, opts)
	if err != nil {
		return out, err
	}
	out.Files = mf
	out.Criteria = append([]Criterion{cc1, br1, pr3, pr4, sv1, sv2, sv3}, files...)
	for _, c := range out.Criteria {
		switch c.Status {
		case CriterionConform:
			out.Conform++
		case CriterionUnknown:
			out.Unknown++
		}
	}
	return out, nil
}

// reviewInterval calcule CC1 sur l'intervalle du jalon : du tag final précédent au tag qui désigne le
// jalon (nom exact ou non conforme) ; tag absent : commits des branches suivies depuis le tag final
// précédent, écrits avant la fin de l'échéance ; sans échéance, non évaluable. Lu dans le clone.
func reviewInterval(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, m Milestone,
	ms MilestoneResult, tags []model.Tag, since time.Time, opts BuildOptions, now time.Time,
	out *RepoMilestoneReview) (Criterion, error) {
	unknown := func(reason string) (Criterion, error) {
		return Criterion{ID: CriterionCC1, Status: CriterionUnknown, Reason: reason}, nil
	}
	rg, reason := milestoneRangeOf(m, ms, tags, opts, now)
	if reason != "" {
		return unknown(reason)
	}
	if clones == nil || !clones.HasClone(repo.Owner, repo.Name) {
		return unknown(UnknownNoClone)
	}
	iv := MilestoneInterval{To: ms.Tag, From: rg.from, Tally: newTally()}
	var branches, include, exclude []string
	if rg.prevSHA != "" {
		exclude = []string{rg.prevSHA}
	}
	var keep func(model.ConventionCommit) bool
	if rg.tagSHA != "" {
		include = []string{rg.tagSHA}
	} else {
		end := rg.end
		iv.Due = m.Due
		all, err := r.ListBranches(ctx, repo.ID)
		if err != nil {
			return Criterion{}, err
		}
		for _, b := range all {
			if b.Tracked {
				branches = append(branches, b.Name)
			}
		}
		if len(branches) == 0 && repo.DefaultBranch != "" {
			branches = []string{repo.DefaultBranch}
		}
		keep = func(c model.ConventionCommit) bool { return c.AuthoredAt.Before(end) }
	}
	shas, err := revList(ctx, clones, repo, branches, include, exclude)
	if err := ctx.Err(); err != nil {
		return Criterion{}, err
	}
	if err != nil {
		return unknown(UnknownCloneUnreadable)
	}
	rows, err := r.ConventionCommits(ctx, repo.ID, 0)
	if err != nil {
		return Criterion{}, err
	}
	byKey := make(map[string]model.ConventionCommit, len(rows))
	for _, row := range rows {
		byKey[row.SHA] = row
	}
	aopts := Options{Rules: opts.Rules, Since: since, ExcludedAccounts: opts.ExcludedAccounts}
	iv.Tally, iv.BeforeSince, iv.Missing = evalRange(shas, byKey, keep, aopts)
	out.Interval = &iv
	switch {
	case iv.Conform < iv.Evaluated:
		return Criterion{ID: CriterionCC1, Status: CriterionNonconform}, nil
	case iv.Missing > 0:
		return unknown(UnknownPartial)
	case iv.Evaluated == 0:
		return unknown(UnknownNoCommits)
	}
	return Criterion{ID: CriterionCC1, Status: CriterionConform}, nil
}

// milestoneRange borne l'intervalle d'un jalon : du tag final précédent (prevSHA, nom from ; vides : depuis
// le début de l'historique) au tag qui désigne le jalon (tagSHA), ou, tag absent, jusqu'à la fin de
// l'échéance (end).
type milestoneRange struct {
	from, prevSHA string
	tagSHA        string
	end           time.Time
}

// milestoneRangeOf renvoie l'intervalle du jalon m (présence ms) ; reason non vide s'il n'est pas
// évaluable : tag absent sans échéance (UnknownNoTag) ou avant la fin de l'échéance (UnknownNotDue).
func milestoneRangeOf(m Milestone, ms MilestoneResult, tags []model.Tag, opts BuildOptions, now time.Time) (milestoneRange, string) {
	var rg milestoneRange
	end, hasDue := m.DueEnd(opts.Location)
	if ms.Tag == "" && !hasDue {
		return rg, UnknownNoTag
	}
	if ms.Tag == "" && now.Before(end) {
		return rg, UnknownNotDue
	}
	if prev, ok := PreviousFinal(tags, m.Version); ok {
		rg.from, rg.prevSHA = prev.Name, prev.SHA
	}
	if ms.Tag != "" {
		i := slices.IndexFunc(tags, func(t model.Tag) bool { return t.Name == ms.Tag })
		rg.tagSHA = tags[i].SHA
	} else {
		rg.end = end
	}
	return rg, ""
}

// reviewArrivals calcule PR3 = 0 sur l'intervalle du jalon : premier parent du tag qui désigne le jalon
// (tag absent : de la branche par défaut, committé avant la fin de l'échéance) depuis le tag final
// précédent et la date de début, tous auteurs. Inconnu sans clone, clone illisible, sans commit ou avec des
// commits indéterminés (clone seul surtout).
func reviewArrivals(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, m Milestone,
	ms MilestoneResult, tags []model.Tag, since time.Time, pulls []model.ConventionPullRequest, opts BuildOptions,
	now time.Time, out *RepoMilestoneReview) (Criterion, error) {
	rg, reason := milestoneRangeOf(m, ms, tags, opts, now)
	if reason != "" {
		return Criterion{ID: CriterionPR3, Status: CriterionUnknown, Reason: reason}, nil
	}
	ar := arrivalRange{head: rg.tagSHA, since: since, until: rg.end}
	if rg.prevSHA != "" {
		ar.exclude = []string{rg.prevSHA}
	}
	var commits map[string]model.ConventionCommit
	if firstParentOf(clones, repo) != nil {
		rows, err := r.ConventionCommits(ctx, repo.ID, 0)
		if err != nil {
			return Criterion{}, err
		}
		if commits, err = commitIndex(ctx, r, repo, rows, false, nil); err != nil {
			return Criterion{}, err
		}
	}
	a, err := buildArrivals(ctx, clones, repo, nil, pulls, commits, ar, opts)
	if err != nil {
		return Criterion{}, err
	}
	out.Arrivals = &a
	return arrivalCriterion(a), nil
}
