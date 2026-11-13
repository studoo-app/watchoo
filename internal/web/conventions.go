package web

// Page « /conventions » (phase 8.2) : vue d'ensemble des conventions par dépôt et par contributeur, et
// revue d'un jalon (sélecteur ?milestone=vX.Y.Z, un jalon de conventions.tags.milestones). Conventions
// désactivées : la page explique comment les activer, sans aucune donnée (et la navigation n'a pas
// d'entrée). Les indicateurs viennent de internal/conventions (Build, ReviewMilestone), calculés à la
// lecture ; aucune note globale, aucun tri par conformité (dépôts dans l'ordre de la base, contributeurs
// par nom), aucun message de commit ni titre de pull request. Les indicateurs de dépôt (tags, jalons,
// branches sans PR) ne sont jamais attribués à une personne.

import (
	"cmp"
	"net/http"

	"github.com/studoo-app/watchoo/internal/conventions"
)

// conventionsView est le contenu de la page « /conventions ».
type conventionsView struct {
	Enabled      bool
	Repos        []convRepoRow
	Contributors []convContributorRow
	// Milestones : jalons configurés (sélecteur) ; Milestone : jalon choisi, le premier par défaut ; vide
	// si aucun jalon n'est configuré.
	Milestones []conventions.Milestone
	Milestone  string
	Review     *convReviewView
	LocalOnly  bool // espace en clone seul : PR et releases inconnues
}

// convRepoRow est la ligne d'un dépôt dans la vue d'ensemble.
type convRepoRow struct {
	ID        int64
	Name      string
	Unknown   bool // jamais synchronisé
	Evaluated int
	Conform   int
	Percent   int
	// BR1.
	BranchesChecked, BranchesConform, BranchesReview int
	// PR1 et PR2 ; PullsReason traduit si inconnues.
	PullsKnown  bool
	PullsReason string
	Pulls       convPRCounts
	// SV1 et SV2.
	TagsChecked, TagsConform int
	Milestones               []convMilestoneRow
	// CC6, CC7.
	Footer, Issue int
	Language      convLanguage
	// Critères de fichiers (phase 8.3) : synthèse par famille ; FilesReason traduit si inconnus.
	Files       []convFamilySummary
	FilesReason string
	// Phase 8.4 : réécritures, tags réécrits, sauts sous-versionnés, pré-versions, précédence, fraîcheur.
	History convHistorySummary
	// Phase 8.5 : commits hors PR (PR3 ; ArrivalsReason traduit si inconnu) et méthodes de fusion
	// autorisées (BR5 ; nil si inconnues).
	ArrivalsKnown       bool
	ArrivalsReason      string
	OutsidePR, Arrivals int
	MethodsStatus       *convStatus
	// Phase 8.6 : PR4 (fusionnées relues ; PullsKnown faux sans API).
	Reviews convReviews
}

// convContributorRow est la ligne d'un contributeur dans la vue d'ensemble.
type convContributorRow struct {
	ID        int64 // 0 : auteur inconnu (pas de lien)
	Name      string
	Evaluated int
	Conform   int
	Percent   int
	Trend     convTrend
	Pulls     convPRCounts
	// BR1 des branches de tête de ses PR.
	BranchesChecked, BranchesConform int
	// Phase 8.5 : commits du premier parent de la branche par défaut dont il est l'auteur, et ceux arrivés
	// hors PR (PR3).
	OutsidePR, Arrivals int
}

// convReviewView est la revue d'un jalon : un tableau dépôt × critère.
type convReviewView struct {
	Name     string
	Due      string
	Criteria []string // identifiants des critères mesurés (en-têtes de colonnes)
	Repos    []convReviewRepo
}

// convReviewRepo est la ligne d'un dépôt dans la revue de jalon.
type convReviewRepo struct {
	ID        int64
	Name      string
	Evaluable bool
	Reason    string // raison traduite d'un dépôt non évaluable
	Cells     []convReviewCell
	// Attention : critères non conformes puis inconnus, à regarder ; Conforming : critères conformes,
	// regroupés sur une ligne (carte du dépôt).
	Attention  []convReviewCell
	Conforming []convReviewCell
	Conform    int
	Unknown    int
	Total      int
	Tag        string // tag qui désigne le jalon ; vide si absent
	// Interval : intervalle des commits de CC1 (From vide : depuis le début ; To vide : borné par Due).
	HasInterval bool
	From, To    string
	Due         string
	Missing     int
	// Phase 8.4 : échéance de SV2 (libellé et précision traduits, vides sans échéance jugée) et SV3 du tag.
	Deadline       string
	DeadlineDetail string
	History        string
	// Phase 8.5 : PR3 sur l'intervalle du jalon (HasArrivals faux si non lu).
	HasArrivals                            bool
	OutsidePR, ArrivalsTotal, Undetermined int
	// Phase 8.6 : PR4 sur l'intervalle du jalon (HasReviews faux si non évaluable) ; Unreviewed, numéros des
	// fusionnées sans relecture.
	HasReviews                           bool
	ReviewsMerged, ReviewsRead, Reviewed int
	Unreviewed                           []int
}

// convReviewCell est l'état d'un critère.
type convReviewCell struct {
	ID     string
	Label  string // libellé traduit du critère
	Status convStatus
	Reason string // raison traduite d'un état inconnu ; vide sinon
}

// handleConventions sert GET /conventions.
func (s *server) handleConventions(w http.ResponseWriter, r *http.Request) {
	view := conventionsView{Enabled: s.cfg.Conventions.Enabled}
	title := s.t(r, "web.nav.conventions")
	if !view.Enabled {
		s.render(w, r, http.StatusOK, pageConventions, s.page(r, title, navConventions, view))
		return
	}
	clones, opts, err := s.conventionsInputs(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view.LocalOnly, view.Milestones = opts.LocalOnly, opts.Milestones
	view.Milestone = queryValue(r, "milestone")
	if view.Milestone == "" && len(opts.Milestones) > 0 {
		view.Milestone = opts.Milestones[0].Name
	}
	if view.Milestone != "" {
		if _, ok := conventions.FindMilestone(opts.Milestones, view.Milestone); !ok {
			s.badRequest(w, r, s.t(r, "web.conv.bad_milestone", view.Milestone))
			return
		}
	}

	tr := s.tr(r)
	rep, err := conventions.Build(r.Context(), s.store, clones, conventions.Filter{}, opts)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, repo := range rep.Repos {
		row := convRepoRow{ID: repo.Repo.ID, Name: repo.Repo.DisplayName(), Unknown: repo.Repo.LastSyncedAt.IsZero(),
			Evaluated: repo.Evaluated, Conform: repo.Conform, Percent: percent(repo.Conform, repo.Evaluated),
			BranchesChecked: repo.Branches.Checked, BranchesConform: repo.Branches.Conform,
			BranchesReview: repo.Branches.Review, PullsKnown: repo.Pulls.Known,
			Pulls:       newConvPRCounts(repo.Pulls.PRTally),
			TagsChecked: repo.Tags.Checked, TagsConform: repo.Tags.Conform,
			Footer: repo.Traits.Footer, Issue: repo.Traits.Issue, Language: convLanguageOf(tr, repo.Language)}
		if !repo.Pulls.Known {
			row.PullsReason = tr.T("web.conv.reason." + repo.Pulls.Reason)
		} else {
			row.Reviews = newConvReviews(repo.Pulls)
		}
		row.Files, row.FilesReason = convFilesSummary(tr, repo.Files)
		for _, m := range repo.Tags.Milestones {
			row.Milestones = append(row.Milestones, convMilestoneRow{Name: m.Name, Due: m.Due,
				Status: milestoneStatus(tr, m.Status), Deadline: deadlineStatus(tr, m.Deadline),
				DeadlineDetail: s.deadlineDetail(tr, m.Deadline)})
		}
		row.History = newConvHistorySummary(repo)
		row.ArrivalsKnown, row.OutsidePR, row.Arrivals = repo.Arrivals.Known, repo.Arrivals.OutsidePR, repo.Arrivals.Total()
		if !repo.Arrivals.Known {
			row.ArrivalsReason = tr.T("web.conv.reason." + repo.Arrivals.Reason)
		}
		if mv := newConvMerges(tr, repo); mv.MethodsKnown {
			row.MethodsStatus = &mv.MethodsStatus
		}
		view.Repos = append(view.Repos, row)
	}
	for _, c := range rep.Contributors {
		view.Contributors = append(view.Contributors, convContributorRow{ID: c.AuthorID,
			Name: cmp.Or(c.AuthorName, c.AuthorLogin), Evaluated: c.Evaluated, Conform: c.Conform,
			Percent: percent(c.Conform, c.Evaluated), Trend: newConvTrend(c.Trend), Pulls: newConvPRCounts(c.Pulls),
			BranchesChecked: c.Branches.Checked, BranchesConform: c.Branches.Conform,
			OutsidePR: c.Arrivals.OutsidePR, Arrivals: c.Arrivals.Total()})
	}

	if view.Milestone != "" {
		rv, err := conventions.ReviewMilestone(r.Context(), s.store, clones, conventions.Filter{}, view.Milestone, opts, s.now())
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		view.Review = newConvReview(s, r, rv)
	}
	s.render(w, r, http.StatusOK, pageConventions, s.page(r, title, navConventions, view))
}

// newConvReview traduit la revue d'un jalon.
func newConvReview(s *server, r *http.Request, rv conventions.MilestoneReview) *convReviewView {
	tr := s.tr(r)
	out := &convReviewView{Name: rv.Milestone.Name, Due: rv.Milestone.Due, Criteria: conventions.AllCriteria}
	for _, rr := range rv.Repos {
		row := convReviewRepo{ID: rr.Repo.ID, Name: rr.Repo.DisplayName(), Evaluable: rr.Evaluable,
			Conform: rr.Conform, Unknown: rr.Unknown, Total: len(rr.Criteria)}
		if !rr.Evaluable {
			row.Reason = tr.T("web.conv.reason." + rr.Reason)
		}
		if rr.Tag != nil {
			row.Tag = rr.Tag.Name
		}
		if a := rr.Arrivals; a != nil && a.Known {
			row.HasArrivals, row.OutsidePR, row.ArrivalsTotal, row.Undetermined = true, a.OutsidePR, a.Total(), a.Undetermined
		}
		if m := rr.Reviews; m != nil {
			row.HasReviews, row.ReviewsMerged, row.ReviewsRead, row.Reviewed, row.Unreviewed = true, m.Merged, m.Known,
				m.Reviewed, m.Unreviewed
		}
		if iv := rr.Interval; iv != nil {
			row.HasInterval, row.From, row.To, row.Due, row.Missing = true, iv.From, iv.To, iv.Due, iv.Missing
		}
		if d := rr.Milestone.Deadline; d.Status != "" {
			row.Deadline, row.DeadlineDetail = tr.T("conv.deadline."+d.Status), s.deadlineDetail(tr, d)
		}
		if h := rr.History; h != nil {
			row.History = tr.T("conv.history.status." + h.Status)
			if h.Status == conventions.TagHistoryStable {
				row.History = tr.T("conv.history.since", s.formatLocal(h.FirstSeenAt, tr.T("web.fmt.date")))
			}
		}
		var unknown []convReviewCell
		for _, c := range rr.Criteria {
			cell := convReviewCell{ID: c.ID, Label: tr.T("web.conv.criterion." + c.ID),
				Status: criterionStatus(tr, c.Status)}
			if c.Reason != "" {
				cell.Reason = tr.T("web.conv.reason." + c.Reason)
			}
			row.Cells = append(row.Cells, cell)
			switch c.Status {
			case conventions.CriterionConform:
				row.Conforming = append(row.Conforming, cell)
			case conventions.CriterionNonconform:
				row.Attention = append(row.Attention, cell)
			default:
				unknown = append(unknown, cell)
			}
		}
		// Les non conformes d'abord, puis les inconnus, chacun dans l'ordre des critères.
		row.Attention = append(row.Attention, unknown...)
		out.Repos = append(out.Repos, row)
	}
	return out
}
