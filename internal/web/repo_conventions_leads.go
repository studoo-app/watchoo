package web

// Pistes des conventions (phase 8.7) : CL11, CL12, RD5, DOC6, DOC7, DOC9, DOC10, TL4 et TL5, calculées par
// internal/conventions (aucun calcul ici), dans un bloc replié de la section du dépôt. Une piste invite à
// relire, elle ne juge pas : jamais de pastille « conforme » ni « non conforme », aucune grille, aucun taux.
// Indicateurs de dépôt, jamais attribués à une personne.

import (
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// convLeadsView sont les pistes d'un dépôt.
type convLeadsView struct {
	ToReview int
	Items    []convLeadRow
	// BrokenImages : images relatives cassées (DOC7), recalculées depuis le clone pour la page du dépôt.
	BrokenImages *convPathList
}

// convLeadRow est l'état d'une piste.
type convLeadRow struct {
	ID     string
	Label  string
	Status convStatus
	Reason string     // raison traduite d'un état inconnu ou sans objet
	Codes  []convCode // codes, avec le nombre d'éléments dans le libellé ; jamais bloquants
	Detail string     // compteurs et dates traduits
}

// leadStatus est la pastille d'une piste : neutre (badge-ghost) ou « à relire » (badge-info), jamais
// celle d'un critère conforme ou non conforme.
func leadStatus(tr *i18n.Translator, status string) convStatus {
	label := tr.T("conv.lead_status." + status)
	switch status {
	case conventions.LeadReview:
		return convStatus{Icon: "→", Label: label, Class: "badge-info"}
	case conventions.LeadNone:
		return convStatus{Icon: "·", Label: label, Class: "badge-ghost"}
	case conventions.LeadNotApplicable:
		return convStatus{Icon: "○", Label: label, Class: "badge-ghost"}
	default:
		return convStatus{Icon: "?", Label: label, Class: "badge-ghost"}
	}
}

// newConvLeads traduit les pistes d'un dépôt et la liste des images cassées.
func (s *server) newConvLeads(tr *i18n.Translator, repo conventions.RepoReport) convLeadsView {
	v := convLeadsView{ToReview: repo.Leads.ToReview()}
	date := func(t time.Time) string { return s.formatLocal(t, tr.T("web.fmt.date")) }
	for _, l := range repo.Leads.Leads {
		row := convLeadRow{ID: l.ID, Label: tr.T("conv.lead." + l.ID), Status: leadStatus(tr, l.Status),
			Detail: conventions.LeadDetail(tr.T, date, l)}
		if l.Reason != "" {
			row.Reason = tr.T("web.conv.reason." + l.Reason)
		}
		for _, code := range l.Codes {
			c := convCode{Code: code, Label: tr.T("conv.lead_code." + code)}
			if n := l.Counts[code]; n > 0 {
				c.Label += " (" + pluralText(tr, "web.conv.lead_count", n) + ")"
			}
			row.Codes = append(row.Codes, c)
		}
		v.Items = append(v.Items, row)
	}
	if f := repo.Files.Lists; f != nil && len(f.BrokenImages) > 0 {
		paths := make([]string, len(f.BrokenImages))
		for i, d := range f.BrokenImages {
			paths[i] = d.Source + " → " + d.Target
			if d.CaseMismatch {
				paths[i] += " (" + tr.T("web.conv.dead_link_case") + ")"
			}
		}
		v.BrokenImages = &convPathList{Key: "broken_images", Label: tr.T("web.conv.list.broken_images", len(paths)),
			Paths: paths}
	}
	return v
}
