package web

// Critères de fichiers des conventions (phase 8.3) : grilles CHANGELOG, README, documentation et outillage
// d'un dépôt, lues dans le cache de la synchro par internal/conventions (aucun calcul ici) ; listes de
// chemins (DOC3, DOC4, DOC5, DOC8) dans la section du dépôt seulement, repliées. Indicateurs de dépôt,
// jamais attribués à une personne ; aucune note globale.

import (
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// convFilesView sont les critères de fichiers d'un dépôt.
type convFilesView struct {
	Known      bool
	Reason     string // raison traduite d'un résultat inconnu (pas encore calculé, à recalculer…)
	SHA        string
	ComputedAt time.Time
	Families   []convFamily
	// Lists : listes de chemins (section du dépôt) ; ListsReason traduit si elles n'ont pas pu être lues.
	Lists       []convPathList
	ListsReason string
}

// convFamily est la grille d'une famille : « k/n critères conformes, u inconnus ».
type convFamily struct {
	Name    string // clé de la famille (changelog, readme, docs, tooling)
	Label   string
	Conform int
	Unknown int
	Total   int
	Checks  []convFileCheck
}

// convFileCheck est l'état d'un critère de fichiers.
type convFileCheck struct {
	ID     string
	Label  string
	Status convStatus
	Reason string     // raison traduite d'un état inconnu
	Codes  []convCode // écarts et avertissements, avec le nombre d'éléments dans Detail
	Detail string     // compteurs utiles traduits (mots de RD4, sections de RD2)
}

// convFamilySummary est la synthèse d'une famille dans la vue d'ensemble.
type convFamilySummary struct {
	Label   string
	Conform int
	Unknown int
	Total   int
}

// convPathList est une liste de chemins de la vue détaillée.
type convPathList struct {
	Key   string // unreferenced, duplicate_rows, dead_links, bad_names, archives, office_documents
	Label string
	Paths []string
}

// newConvFiles traduit les critères de fichiers d'un dépôt.
func newConvFiles(tr *i18n.Translator, f conventions.FileReport) convFilesView {
	v := convFilesView{Known: f.Known, SHA: f.SHA, ComputedAt: f.ComputedAt}
	if !f.Known {
		v.Reason = tr.T("web.conv.reason." + f.Reason)
	}
	for _, fam := range f.Results.Families() {
		row := convFamily{Name: fam.Name, Label: tr.T("conv.family." + fam.Name), Total: len(fam.Checks)}
		row.Conform, row.Unknown = fam.Tally()
		for _, c := range fam.Checks {
			fc := convFileCheck{ID: c.ID, Label: tr.T("conv.criterion." + c.ID), Status: criterionStatus(tr, c.Status),
				Codes: convCodesOf(tr, "conv.file_code.", c.Codes, conventions.FileSeverityOf)}
			for i, code := range c.Codes {
				if n := c.Counts[code]; n > 0 {
					fc.Codes[i].Label += " (" + tr.T("web.conv.file_count", n) + ")"
				}
			}
			if c.Reason != "" {
				fc.Reason = tr.T("web.conv.reason." + c.Reason)
			}
			switch {
			case c.Status == conventions.CriterionUnknown:
			case c.ID == conventions.CriterionRD2:
				fc.Detail = tr.T("web.conv.file_sections", c.Counts[conventions.CountFound], c.Counts[conventions.CountExpected])
			case c.ID == conventions.CriterionRD4:
				fc.Detail = tr.T("web.conv.file_words", c.Counts[conventions.CountWords], c.Counts[conventions.CountMinWords])
			}
			row.Checks = append(row.Checks, fc)
		}
		v.Families = append(v.Families, row)
	}
	if f.ListsReason != "" {
		v.ListsReason = tr.T("web.conv.reason." + f.ListsReason)
	}
	if l := f.Lists; l != nil {
		dead := make([]string, len(l.DeadLinks))
		for i, d := range l.DeadLinks {
			dead[i] = d.Source + " → " + d.Target
			if d.CaseMismatch {
				dead[i] += " (" + tr.T("web.conv.dead_link_case") + ")"
			}
		}
		for _, list := range []convPathList{
			{Key: "unreferenced", Paths: l.Unreferenced}, {Key: "duplicate_rows", Paths: l.DuplicateRows},
			{Key: "dead_links", Paths: dead}, {Key: "bad_names", Paths: l.BadNames},
			{Key: "archives", Paths: l.Archives}, {Key: "office_documents", Paths: l.OfficeDocuments},
		} {
			if len(list.Paths) == 0 {
				continue
			}
			list.Label = tr.T("web.conv.list."+list.Key, len(list.Paths))
			v.Lists = append(v.Lists, list)
		}
	}
	return v
}

// convFilesSummary renvoie la synthèse par famille d'un dépôt pour la vue d'ensemble ; reason (traduite)
// si le résultat est inconnu.
func convFilesSummary(tr *i18n.Translator, f conventions.FileReport) ([]convFamilySummary, string) {
	if !f.Known {
		return nil, tr.T("web.conv.reason." + f.Reason)
	}
	var out []convFamilySummary
	for _, fam := range f.Results.Families() {
		c, u := fam.Tally()
		out = append(out, convFamilySummary{Label: tr.T("conv.family." + fam.Name), Conform: c, Unknown: u,
			Total: len(fam.Checks)})
	}
	return out, ""
}
