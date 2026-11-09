package cli

// Sortie de la phase 8.6 de « watchoo conventions » : relectures des pull requests (PR4). Clés JSON en
// anglais, ajoutées à celles des phases précédentes. Seuls des compteurs sont affichés : jamais le login
// d'un relecteur (il n'est pas gardé), jamais le corps d'une relecture ni le titre d'une PR.

import (
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// conventionsReviewJSON est PR4 d'une PR : compteurs de relectures lus à la synchro.
type conventionsReviewJSON struct {
	Known     bool `json:"known"`     // relectures lues (fusionnée : après la fusion) ; faux : inconnu
	Reviews   int  `json:"reviews"`   // relectures déposées par d'autres comptes (sauf PENDING) ; 0 si inconnu
	Approvals int  `json:"approvals"` // relecteurs approuvant retenus (contributeurs seulement si exigé)
	Reviewed  bool `json:"reviewed"`  // fusionnée : PR4 conforme ; ouverte : approuvée à la dernière lecture
}

// conventionsUnreviewedJSON est une PR fusionnée sans relecture (numéro et auteur, jamais le titre).
type conventionsUnreviewedJSON struct {
	Number int        `json:"number"`
	Author authorJSON `json:"author"`
}

// conventionsMilestoneReviewsJSON est PR4 sur l'intervalle d'un jalon.
type conventionsMilestoneReviewsJSON struct {
	Merged     int   `json:"merged"`     // PR fusionnées de l'intervalle
	Known      int   `json:"known"`      // dont les relectures sont lues après la fusion
	Reviewed   int   `json:"reviewed"`   // relues
	Unreviewed []int `json:"unreviewed"` // numéros des fusionnées sans relecture, décroissants
}

// newConventionsReviewJSON convertit PR4 d'une PR.
func newConventionsReviewJSON(r conventions.ReviewResult) conventionsReviewJSON {
	return conventionsReviewJSON{Known: r.Known, Reviews: r.ReviewsByOthers, Approvals: r.Approvals, Reviewed: r.Reviewed}
}

// newConventionsUnreviewedJSON convertit la vue « fusionnées sans relecture ».
func newConventionsUnreviewedJSON(p conventions.PullReport) []conventionsUnreviewedJSON {
	lines := p.MergedWithoutReview()
	out := make([]conventionsUnreviewedJSON, 0, len(lines))
	for _, l := range lines {
		out = append(out, conventionsUnreviewedJSON{Number: l.Number, Author: conventionsAuthor(l.AuthorName, l.AuthorLogin)})
	}
	return out
}

// newConventionsMilestoneReviewsJSON convertit PR4 d'un jalon ; nil si non évaluable.
func newConventionsMilestoneReviewsJSON(m *conventions.MilestoneReviews) *conventionsMilestoneReviewsJSON {
	if m == nil {
		return nil
	}
	return &conventionsMilestoneReviewsJSON{Merged: m.Merged, Known: m.Known, Reviewed: m.Reviewed,
		Unreviewed: append([]int{}, m.Unreviewed...)}
}

// reviewLines décrit PR4 d'un dépôt : PR fusionnées relues sur celles dont les relectures sont lues, les
// autres comptées à part, puis les fusionnées sans relecture (numéro et auteur).
func reviewLines(p conventions.PullReport) []string {
	if !p.Known || p.Merged.Evaluated == 0 {
		return nil
	}
	m := p.Merged
	lines := []string{i18n.T("cli.conv.reviews_line", m.Reviewed, m.ReviewKnown, m.Evaluated-m.ReviewKnown)}
	var items []string
	for _, l := range p.MergedWithoutReview() {
		author := cmpOr(cmpOr(l.AuthorName, l.AuthorLogin), i18n.T("cli.conv.unknown_author"))
		items = append(items, "#"+strconv.Itoa(l.Number)+" ("+textclean.Clean(author)+")")
	}
	if len(items) > 0 {
		lines = append(lines, "  "+i18n.T("cli.conv.reviews_missing", len(items), strings.Join(items, ", ")))
	}
	return lines
}

// authorReviewsText écrit PR4 des PR d'un contributeur et les relectures reçues : « 1/2, 3 relectures » ;
// les relectures reçues seules si aucune PR fusionnée n'a ses relectures lues (pas de « 0/0 ») ; dash
// sans PR, ouverte ou fusionnée, dont les relectures sont lues (même règle que le web).
func authorReviewsText(p conventions.PRTally, dash string) string {
	received := p.Open.ReviewsReceived + p.Merged.ReviewsReceived
	switch {
	case p.Open.ReviewKnown+p.Merged.ReviewKnown == 0:
		return dash
	case p.Merged.ReviewKnown == 0:
		return i18n.T("cli.conv.author_reviews_open", received)
	}
	return i18n.T("cli.conv.author_reviews", p.Merged.Reviewed, p.Merged.ReviewKnown, received)
}

// milestoneReviewsDetail précise PR4 dans la revue de jalon : « 2/3 PR fusionnées relues (#4) ».
func milestoneReviewsDetail(r conventions.RepoMilestoneReview) string {
	m := r.Reviews
	if m == nil || m.Merged == 0 {
		return ""
	}
	s := " — " + i18n.T("cli.conv.milestone_reviews", m.Reviewed, m.Merged, m.Merged-m.Known)
	if len(m.Unreviewed) > 0 {
		s += " (" + prNumbers(m.Unreviewed) + ")"
	}
	return s
}
