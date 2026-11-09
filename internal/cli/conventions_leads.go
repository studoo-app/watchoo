package cli

// Pistes de « watchoo conventions » (phase 8.7) : CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4 et TL5,
// calculées par internal/conventions. Une piste invite à relire, elle ne juge pas : elle n'entre dans
// aucune grille, aucun taux ni aucune revue de jalon. Les pistes de l'historique (DOC9, DOC10, TL5) et les
// images cassées (DOC7) ne sont calculées que dans la vue détaillée d'un dépôt (watchoo conventions
// owner/repo). Indicateurs de dépôt, jamais attribués à une personne.

import (
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// conventionsLeadsJSON sont les pistes d'un dépôt.
type conventionsLeadsJSON struct {
	ToReview int                   `json:"to_review"` // pistes à l'état review
	Items    []conventionsLeadJSON `json:"items"`     // les 9 pistes, dans l'ordre d'affichage
	// BrokenImages sont les images relatives cassées (DOC7), recalculées depuis le clone en vue détaillée
	// d'un dépôt ; [] sinon (voir docs.lists.computed).
	BrokenImages []conventionsDeadLinkJSON `json:"broken_images"`
}

// conventionsLeadJSON est l'état d'une piste : codes, compteurs et dates, jamais un chemin ni un contenu.
type conventionsLeadJSON struct {
	ID     string               `json:"id"`
	Status string               `json:"status"` // none, review, unknown ou not_applicable
	Reason *string              `json:"reason"` // raison d'un état unknown ou not_applicable ; null sinon
	Codes  []string             `json:"codes"`
	Counts map[string]int       `json:"counts"`
	Dates  map[string]time.Time `json:"dates"` // DOC9, TL5 (UTC) ; {} sinon
}

// newConventionsLeadsJSON convertit les pistes d'un dépôt et les images cassées de la vue détaillée.
func newConventionsLeadsJSON(l conventions.LeadReport, f conventions.FileReport) conventionsLeadsJSON {
	out := conventionsLeadsJSON{ToReview: l.ToReview(), Items: make([]conventionsLeadJSON, 0, len(l.Leads)),
		BrokenImages: []conventionsDeadLinkJSON{}}
	for _, lc := range l.Leads {
		item := conventionsLeadJSON{ID: lc.ID, Status: lc.Status, Reason: optionalString(lc.Reason),
			Codes: nonNil(lc.Codes), Counts: map[string]int{}, Dates: map[string]time.Time{}}
		for k, v := range lc.Counts {
			item.Counts[k] = v
		}
		for k, v := range lc.Dates {
			if !v.IsZero() {
				item.Dates[k] = v.UTC().Truncate(time.Second)
			}
		}
		out.Items = append(out.Items, item)
	}
	if f.Lists != nil {
		for _, d := range f.Lists.BrokenImages {
			out.BrokenImages = append(out.BrokenImages, conventionsDeadLinkJSON{Source: d.Source, Target: d.Target,
				Resolved: d.Resolved, CaseMismatch: d.CaseMismatch})
		}
	}
	return out
}

// leadLines décrit les pistes d'un dépôt : un titre qui rappelle qu'elles ne jugent pas et le nombre à
// relire ; en vue d'ensemble, les seules pistes à relire et la commande du détail ; en vue détaillée,
// chaque piste puis les images cassées.
func (s *session) leadLines(r conventions.RepoReport, detail bool) []string {
	lines := []string{i18n.T("cli.conv.leads_title", r.Leads.ToReview())}
	for _, lc := range r.Leads.Leads {
		if !detail && lc.Status != conventions.LeadReview {
			continue
		}
		lines = append(lines, "  "+s.leadLine(lc))
	}
	if !detail {
		lines = append(lines, "  "+i18n.T("cli.conv.lead_more", r.Repo.FullName()))
		return lines
	}
	if l := r.Files.Lists; l != nil && len(l.BrokenImages) > 0 {
		lines = append(lines, "  "+pathListLine("broken_images", deadLinkTexts(l.BrokenImages)))
	}
	return lines
}

// leadLine décrit une piste : identifiant, état, nom, codes (avec le nombre d'éléments), raison d'un état
// inconnu ou sans objet, et détail (compteurs, dates).
func (s *session) leadLine(lc conventions.LeadCheck) string {
	name := i18n.T("conv.lead." + lc.ID)
	if lc.Status == conventions.CriterionUnknown {
		return i18n.T("cli.conv.lead_unknown", lc.ID, name, i18n.T("cli.conv.reason."+lc.Reason))
	}
	line := i18n.T("cli.conv.lead_line", lc.ID, i18n.T("conv.lead_status."+lc.Status), name)
	if lc.Reason != "" {
		line += " (" + i18n.T("cli.conv.reason."+lc.Reason) + ")"
	}
	if len(lc.Codes) > 0 {
		codes := make([]string, len(lc.Codes))
		for i, code := range lc.Codes {
			codes[i] = code + " (" + i18n.T("conv.lead_code."+code)
			if n := lc.Counts[code]; n > 0 {
				codes[i] += ", " + strconv.Itoa(n)
			}
			codes[i] += ")"
		}
		line += i18n.T("cli.conv.lead_codes", strings.Join(codes, ", "))
	}
	if d := conventions.LeadDetail(i18n.T, func(t time.Time) string {
		return t.In(s.loc).Format(i18n.T("cli.conv.fmt.date"))
	}, lc); d != "" {
		line += " · " + d
	}
	return line
}

// deadLinkTexts écrit des liens morts (ou des images cassées) : « source → cible », casse signalée.
func deadLinkTexts(links []conventions.DeadLink) []string {
	out := make([]string, len(links))
	for i, d := range links {
		out[i] = d.Source + " → " + d.Target
		if d.CaseMismatch {
			out[i] += " (" + i18n.T("cli.conv.dead_link_case") + ")"
		}
	}
	return out
}
