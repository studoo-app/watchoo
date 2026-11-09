package conventions

// Revue de jalon (RJ, phase 8.2) : pour un jalon configuré, une liste de critères par dépôt, sans note
// globale (« k/n critères conformes, u inconnus »). Phase 8.2 : CC1 sur l'intervalle du jalon, BR1 (état
// courant), SV1 et SV2 du jalon ; les critères de fichiers (CL, RD, DOC) arrivent avec la phase 8.3, PR3
// avec la phase 8.5, PR4 avec la phase 8.6. Indicateurs de dépôt : rien n'est attribué à une personne.

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// ErrUnknownMilestone signale un jalon absent de conventions.tags.milestones.
var ErrUnknownMilestone = errors.New("conventions: milestone not configured")

// Critères de la revue de jalon en phase 8.2 (clés --json), dans leur ordre d'affichage.
const (
	CriterionCC1 = "CC1" // 100 % de commits conformes sur l'intervalle du jalon
	CriterionBR1 = "BR1" // noms de branche conformes (état courant)
	CriterionSV1 = "SV1" // nom du tag du jalon conforme
	CriterionSV2 = "SV2" // jalon présent
)

// AllCriteria liste les critères de la revue de jalon de la phase 8.2.
var AllCriteria = []string{CriterionCC1, CriterionBR1, CriterionSV1, CriterionSV2}

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
	Milestone MilestoneResult    // SV2 du jalon dans le dépôt
	Tag       *TagResult         // SV1 du tag qui désigne le jalon ; nil si absent
	Interval  *MilestoneInterval // CC1 de l'intervalle ; nil si non mesuré
	Branches  BranchReport       // BR1, état courant
	Criteria  []Criterion        // dans l'ordre d'AllCriteria ; vide non nil si non évaluable
	Conform   int                // critères conformes
	Unknown   int                // critères inconnus
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
	sv2 := Criterion{ID: CriterionSV2, Status: CriterionNonconform}
	switch out.Milestone.Status {
	case MilestonePresent:
		sv2.Status = CriterionConform
	case MilestoneAbsent:
		if end, ok := m.DueEnd(opts.Location); ok && now.Before(end) {
			sv2.Status, sv2.Reason = CriterionUnknown, UnknownNotDue
		}
	}
	out.Criteria = []Criterion{cc1, br1, sv1, sv2}
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
	end, hasDue := m.DueEnd(opts.Location)
	if ms.Tag == "" && !hasDue {
		return unknown(UnknownNoTag)
	}
	if ms.Tag == "" && now.Before(end) {
		return unknown(UnknownNotDue)
	}
	if clones == nil || !clones.HasClone(repo.Owner, repo.Name) {
		return unknown(UnknownNoClone)
	}
	iv := MilestoneInterval{To: ms.Tag, Tally: newTally()}
	var branches, include, exclude []string
	if prev, ok := PreviousFinal(tags, m.Version); ok {
		iv.From = prev.Name
		exclude = []string{prev.SHA}
	}
	var keep func(model.ConventionCommit) bool
	if ms.Tag != "" {
		i := slices.IndexFunc(tags, func(t model.Tag) bool { return t.Name == ms.Tag })
		include = []string{tags[i].SHA}
	} else {
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
