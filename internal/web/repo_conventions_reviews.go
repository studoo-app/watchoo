package web

// Conventions de la phase 8.6 : relectures des pull requests (PR4) sur la page d'un dépôt (bloc « fusions »
// et pastille par PR), sur « /conventions » (dépôts, contributeurs, revue de jalon) et sur la page d'un
// contributeur (relectures reçues sur ses PR). Des compteurs seulement : jamais le login d'un relecteur
// (il n'est pas gardé), ni le corps d'une relecture, ni le titre d'une PR.

import (
	"cmp"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// convUnreviewedRow est une PR fusionnée sans relecture : numéro et auteur, jamais le titre.
type convUnreviewedRow struct {
	Number   int
	Author   string // nom, sinon login ; vide si inconnu
	AuthorID int64  // contributeur ; 0 si aucun
}

// convReviews est PR4 d'un dépôt : PR fusionnées évaluées, dont les relectures sont lues, relues, et les
// fusionnées sans relecture (au plus repoConventionsList).
type convReviews struct {
	Merged, Known, Reviewed int
	Rows                    []convUnreviewedRow
	RowsTotal               int
}

// Unread renvoie les PR fusionnées dont les relectures ne sont pas encore lues.
func (c convReviews) Unread() int { return c.Merged - c.Known }

// newConvReviews traduit PR4 d'un dépôt (rapport des PR connu).
func newConvReviews(p conventions.PullReport) convReviews {
	out := convReviews{Merged: p.Merged.Evaluated, Known: p.Merged.ReviewKnown, Reviewed: p.Merged.Reviewed}
	for _, l := range p.MergedWithoutReview() {
		out.RowsTotal++
		if len(out.Rows) < repoConventionsList {
			out.Rows = append(out.Rows, convUnreviewedRow{Number: l.Number, Author: cmp.Or(l.AuthorName, l.AuthorLogin),
				AuthorID: l.AuthorID})
		}
	}
	return out
}

// reviewStatus est la pastille PR4 d'une PR fusionnée ; nil pour une PR ouverte.
func reviewStatus(tr *i18n.Translator, l conventions.PullLine) *convStatus {
	if l.Category != conventions.PRCategoryMerged {
		return nil
	}
	var s convStatus
	switch {
	case !l.Review.Known:
		s = statusUnknown(tr, "web.conv.review_unread")
	case l.Review.Reviewed:
		s = statusConform(tr, "web.conv.review_done")
	default:
		s = statusNonconform(tr, "web.conv.review_missing")
	}
	return &s
}
