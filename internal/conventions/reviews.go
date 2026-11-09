package conventions

// PR4 (phase 8.6) : pull request relue par un autre membre. Les relectures (GET /pulls/{n}/reviews) ne
// sont lues qu'en mémoire, à la synchro : ReviewTally les réduit à trois compteurs (colonnes
// pull_requests.reviews_by_others, approvals_by_others et approvals_by_contributors), sans jamais garder
// le login d'un relecteur ni le corps d'une relecture. EvalPR4 juge ensuite PR4 à la lecture, depuis ces
// compteurs et les réglages conventions.pr.min_reviews et conventions.pr.reviewer_must_contribute :
// changer ces deux réglages ne demande aucune requête. Fonctions pures : ni base, ni forge, ni journal.

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
)

// États d'une relecture, tels que la forge les renvoie.
const (
	ReviewApproved         = "APPROVED"
	ReviewChangesRequested = "CHANGES_REQUESTED"
	ReviewDismissed        = "DISMISSED"
	ReviewCommented        = "COMMENTED"
	ReviewPending          = "PENDING"
)

// DefaultMinReviews est la valeur par défaut de conventions.pr.min_reviews : un relecteur suffit.
const DefaultMinReviews = 1

// Review est une relecture lue en mémoire : jamais son corps.
type Review struct {
	ReviewerID    int64     // identifiant numérique du relecteur ; 0 : compte supprimé (« user »: null)
	ReviewerLogin string    // login du relecteur (exclusions, bots, contributeurs) ; jamais stocké
	State         string    // l'une des constantes Review* ; tout autre état est ignoré
	SubmittedAt   time.Time // date de dépôt ; zéro pour PENDING
}

// ReviewContext décrit la PR relue et ce qui qualifie un relecteur.
type ReviewContext struct {
	AuthorID int64     // identifiant de l'auteur de la PR ; 0 : inconnu (compte supprimé)
	MergedAt time.Time // date de fusion ; zéro : PR non fusionnée (toutes les relectures comptent)
	// Excluded est conventions.excluded_accounts : un relecteur dont le login y figure (casse ignorée) n'est
	// pas compté.
	Excluded []string
	// ContributorIDs et ContributorLogins (en minuscules) désignent les auteurs d'au moins un commit du
	// dépôt connus de la base : un relecteur y figure par son identifiant ou par son login.
	ContributorIDs    map[int64]bool
	ContributorLogins map[string]bool
}

// ReviewCounts sont les compteurs stockés d'une PR relue.
type ReviewCounts struct {
	// ReviewsByOthers compte les relectures déposées par d'autres comptes qualifiés, tous états sauf
	// PENDING, sans limite de date.
	ReviewsByOthers int
	// ApprovalsByOthers compte les relecteurs qualifiés distincts dont le dernier état parmi APPROVED,
	// CHANGES_REQUESTED et DISMISSED, déposé au plus tard à MergedAt, est APPROVED.
	ApprovalsByOthers int
	// ApprovalsByContributors compte, parmi eux, les auteurs d'au moins un commit du dépôt.
	ApprovalsByContributors int
}

// ReviewTally réduit les relectures d'une PR à ses compteurs. Un relecteur est qualifié s'il a un
// identifiant non nul, différent de celui de l'auteur, et si son login n'est ni un bot
// (aisignature.IsBot) ni un compte exclu. Un relecteur sans compte (identifiant 0) n'est jamais compté :
// rien ne le distingue de l'auteur ni d'un autre relecteur supprimé. COMMENTED et PENDING ne changent pas
// l'état d'un relecteur ; pour une PR fusionnée, une relecture sans date ou déposée après MergedAt est
// ignorée pour les approbations (elle reste comptée dans ReviewsByOthers, sauf PENDING). À dates égales,
// la dernière relecture de la liste (ordre chronologique de la forge) l'emporte.
func ReviewTally(reviews []Review, pr ReviewContext) ReviewCounts {
	var out ReviewCounts
	type last struct {
		state string
		at    time.Time
		login string
	}
	decisive := map[int64]last{}
	var order []int64
	for _, r := range reviews {
		if !r.qualified(pr) {
			continue
		}
		switch r.State {
		case ReviewApproved, ReviewChangesRequested, ReviewDismissed, ReviewCommented:
			out.ReviewsByOthers++
		default:
			continue
		}
		if r.State == ReviewCommented {
			continue
		}
		if r.SubmittedAt.IsZero() && !pr.MergedAt.IsZero() {
			continue
		}
		if !pr.MergedAt.IsZero() && r.SubmittedAt.After(pr.MergedAt) {
			continue
		}
		prev, seen := decisive[r.ReviewerID]
		if !seen {
			order = append(order, r.ReviewerID)
		}
		if !seen || !r.SubmittedAt.Before(prev.at) {
			decisive[r.ReviewerID] = last{state: r.State, at: r.SubmittedAt, login: r.ReviewerLogin}
		}
	}
	for _, id := range order {
		l := decisive[id]
		if l.state != ReviewApproved {
			continue
		}
		out.ApprovalsByOthers++
		if pr.ContributorIDs[id] || pr.ContributorLogins[strings.ToLower(strings.TrimSpace(l.login))] {
			out.ApprovalsByContributors++
		}
	}
	return out
}

// qualified indique un relecteur compté : autre compte connu, ni bot ni exclu.
func (r Review) qualified(pr ReviewContext) bool {
	if r.ReviewerID == 0 || r.ReviewerID == pr.AuthorID {
		return false
	}
	login := strings.TrimSpace(r.ReviewerLogin)
	if aisignature.IsBot(login) {
		return false
	}
	for _, x := range pr.Excluded {
		if x = strings.TrimSpace(x); x != "" && strings.EqualFold(x, login) {
			return false
		}
	}
	return true
}

// ReviewRules sont les attentes PR4, tirées des réglages conventions.pr.*.
type ReviewRules struct {
	MinReviews             int  // relecteurs approuvant exigés ; moins de 1 : 1
	ReviewerMustContribute bool // le relecteur doit être auteur d'au moins un commit du dépôt
}

// DefaultReviewRules renvoie les attentes de la charte : un autre compte suffit.
func DefaultReviewRules() ReviewRules {
	return ReviewRules{MinReviews: DefaultMinReviews}
}

// ReviewResult est l'évaluation PR4 d'une PR fusionnée, à partir de ses compteurs.
type ReviewResult struct {
	Known           bool // compteurs connus (PR relue après sa fusion)
	ReviewsByOthers int  // relectures déposées par d'autres comptes (information)
	Approvals       int  // relecteurs approuvant retenus : contributeurs seulement si exigé
	Reviewed        bool // Known et Approvals >= MinReviews
}

// EvalPR4 juge PR4 depuis les compteurs stockés (nil : jamais relue, inconnu) et les attentes r.
// ReviewerMustContribute retient approvalsByContributors au lieu d'approvalsByOthers.
func EvalPR4(reviewsByOthers, approvalsByOthers, approvalsByContributors *int64, r ReviewRules) ReviewResult {
	var out ReviewResult
	approvals := approvalsByOthers
	if r.ReviewerMustContribute {
		approvals = approvalsByContributors
	}
	if approvals == nil {
		return out
	}
	out.Known = true
	out.Approvals = int(*approvals)
	if reviewsByOthers != nil {
		out.ReviewsByOthers = int(*reviewsByOthers)
	}
	out.Reviewed = out.Approvals >= max(r.MinReviews, 1)
	return out
}

// EvalPullReview juge PR4 d'une pull request depuis ses compteurs stockés. Les compteurs d'une PR fusionnée
// ne sont connus que s'ils ont été lus au plus tôt à sa fusion (reviews_checked_at >= merged_at) : lus
// pendant qu'elle était ouverte, ils sont périmés (la synchro suivante les relit). Pour une PR ouverte,
// Reviewed dit « approuvée à la dernière lecture » (information, PR4 ne juge que les fusionnées).
func EvalPullReview(pr model.PullRequest, r ReviewRules) ReviewResult {
	if pr.ReviewsCheckedAt.IsZero() || (pr.Merged && pr.ReviewsCheckedAt.Before(pr.MergedAt)) {
		return ReviewResult{}
	}
	return EvalPR4(pr.ReviewsByOthers, pr.ApprovalsByOthers, pr.ApprovalsByContributors, r)
}

// Raisons PR4 d'un critère inconnu de la revue de jalon (clés --json).
const (
	UnknownReviewsUnchecked = "reviews_unchecked" // PR fusionnées de l'intervalle dont les relectures ne sont pas encore lues
	UnknownNoMergedPR       = "no_merged_pr"      // aucune PR fusionnée évaluée dans l'intervalle
	UnknownNoTagDate        = "no_tag_date"       // date du commit d'un tag de l'intervalle absente de la base
)

// MilestoneReviews est PR4 sur l'intervalle d'un jalon : PR fusionnées évaluées (hors fermées, brouillons,
// bots, forks et PR ouvertes avant la date de début) dont la fusion tombe dans l'intervalle.
type MilestoneReviews struct {
	Merged     int   // PR fusionnées de l'intervalle
	Known      int   // dont les relectures sont lues après la fusion
	Reviewed   int   // relues (PR4)
	Unreviewed []int // numéros des fusionnées sans relecture, décroissants ; vide non nil
}

// reviewCriterion juge PR4 sur l'intervalle d'un jalon : non conforme dès qu'une PR fusionnée connue n'est
// pas relue ; inconnu si des relectures ne sont pas lues, ou sans PR fusionnée ; conforme sinon.
func reviewCriterion(m MilestoneReviews) Criterion {
	c := Criterion{ID: CriterionPR4, Status: CriterionConform}
	switch {
	case len(m.Unreviewed) > 0:
		c.Status = CriterionNonconform
	case m.Known < m.Merged:
		c.Status, c.Reason = CriterionUnknown, UnknownReviewsUnchecked
	case m.Merged == 0:
		c.Status, c.Reason = CriterionUnknown, UnknownNoMergedPR
	}
	return c
}

// reviewPullReviews calcule PR4 sur l'intervalle du jalon, borné par dates : fusion postérieure à la date
// du commit visé par le tag final précédent et à la date de début, jusqu'à la date du commit visé par le
// tag du jalon (comprise ; la PR dont la fusion a produit ce commit est comptée) ou, tag absent, avant la
// fin de l'échéance. Approximation : la date du commit visé tient lieu de date de pose du tag ; la PR dont
// la fusion a produit le commit du tag précédent en est exclue (comptée dans le jalon précédent). Inconnu
// sans l'API (no_api), hors intervalle évaluable, ou sans date de commit d'un tag en base (no_tag_date).
func reviewPullReviews(ctx context.Context, r Reader, repo model.Repository, m Milestone, ms MilestoneResult,
	tags []model.Tag, since time.Time, pulls []model.ConventionPullRequest, opts BuildOptions, now time.Time,
	out *RepoMilestoneReview) (Criterion, error) {
	unknown := func(reason string) (Criterion, error) {
		return Criterion{ID: CriterionPR4, Status: CriterionUnknown, Reason: reason}, nil
	}
	rg, reason := milestoneRangeOf(m, ms, tags, opts, now)
	if reason != "" {
		return unknown(reason)
	}
	if !pullsKnown(repo, opts) {
		return unknown(UnknownNoAPI)
	}
	var dates map[string]time.Time
	if rg.from != "" || rg.tagSHA != "" {
		var err error
		if dates, err = r.ConventionTagDates(ctx, repo.ID); err != nil {
			return Criterion{}, err
		}
	}
	var after, until time.Time
	if rg.from != "" {
		d, ok := dates[rg.from]
		if !ok {
			return unknown(UnknownNoTagDate)
		}
		after = d
	}
	if rg.tagSHA != "" {
		d, ok := dates[ms.Tag]
		if !ok {
			return unknown(UnknownNoTagDate)
		}
		until = d
	}
	mr := MilestoneReviews{Unreviewed: []int{}}
	for _, pr := range pulls {
		cat, ok := PRCategory(pr.PullRequest)
		if !ok || cat != PRCategoryMerged || (!since.IsZero() && pr.CreatedAt.Before(since)) {
			continue
		}
		at := pr.MergedAt
		merge := pr.MergeCommitSHA
		in := at.After(after) && (since.IsZero() || !at.Before(since)) && (merge == "" || merge != rg.prevSHA)
		if rg.tagSHA != "" {
			in = in && (!at.After(until) || (merge != "" && merge == rg.tagSHA))
		} else {
			in = in && at.Before(rg.end)
		}
		if !in {
			continue
		}
		mr.Merged++
		res := EvalPullReview(pr.PullRequest, opts.Review)
		if !res.Known {
			continue
		}
		mr.Known++
		if res.Reviewed {
			mr.Reviewed++
		} else {
			mr.Unreviewed = append(mr.Unreviewed, pr.Number)
		}
	}
	slices.SortFunc(mr.Unreviewed, func(a, b int) int { return cmp.Compare(b, a) })
	out.Reviews = &mr
	return reviewCriterion(mr), nil
}
