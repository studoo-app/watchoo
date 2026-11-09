package cli

// Critères de fichiers de « watchoo conventions » (phase 8.3) : grilles CHANGELOG (CL), README (RD),
// documentation (DOC) et outillage (TL) d'un dépôt, lues dans le cache de la synchro par
// internal/conventions ; listes de chemins (DOC3, DOC4, DOC5, DOC8) en vue détaillée d'un dépôt seulement.
// Aucune note globale : « k/n critères conformes, u inconnus » par famille. Indicateurs de dépôt, jamais
// attribués à une personne.

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// maxListText plafonne les chemins écrits par liste dans le texte (le --json les donne tous).
const maxListText = 20

// conventionsFilesJSON est la grille d'une famille de critères de fichiers d'un dépôt.
type conventionsFilesJSON struct {
	State      string                         `json:"state"`       // computed ou unknown
	Reason     *string                        `json:"reason"`      // not_computed, stale, no_clone, filtered ; null si calculé
	SHA        *string                        `json:"sha"`         // commit mesuré (tête de la branche par défaut)
	ComputedAt *time.Time                     `json:"computed_at"` // date du calcul par la synchro
	Conform    int                            `json:"conform"`
	Unknown    int                            `json:"unknown"`
	Total      int                            `json:"total"`
	Criteria   []conventionsFileCriterionJSON `json:"criteria"`
}

// conventionsFileCriterionJSON est l'état d'un critère de fichiers : codes et compteurs, jamais un chemin.
type conventionsFileCriterionJSON struct {
	ID     string         `json:"id"`
	Status string         `json:"status"` // conform, nonconform ou unknown
	Reason *string        `json:"reason"`
	Codes  []string       `json:"codes"`
	Counts map[string]int `json:"counts"`
}

// conventionsDocsJSON est la grille de la documentation et ses listes de chemins.
type conventionsDocsJSON struct {
	conventionsFilesJSON
	Lists conventionsDocListsJSON `json:"lists"`
}

// conventionsDocListsJSON sont les listes de chemins de DOC3, DOC4, DOC5 et DOC8, recalculées depuis le
// clone en vue détaillée d'un dépôt (watchoo conventions owner/repo) ; vides sinon (computed faux).
type conventionsDocListsJSON struct {
	Computed        bool                      `json:"computed"`
	Reason          *string                   `json:"reason"` // vue détaillée sans liste : no_clone, clone_unreadable…
	Unreferenced    []string                  `json:"unreferenced"`
	DuplicateRows   []string                  `json:"duplicate_rows"`
	DeadLinks       []conventionsDeadLinkJSON `json:"dead_links"`
	BadNames        []string                  `json:"bad_names"`
	Archives        []string                  `json:"archives"`
	OfficeDocuments []string                  `json:"office_documents"`
}

// conventionsDeadLinkJSON est un lien relatif mort (DOC4).
type conventionsDeadLinkJSON struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	Resolved     string `json:"resolved"`
	CaseMismatch bool   `json:"case_mismatch"`
}

// conventionsMilestoneFilesJSON est l'état des critères de fichiers sur l'arbre du tag d'un jalon.
type conventionsMilestoneFilesJSON struct {
	State          string     `json:"state"`  // computed ou unknown
	Reason         *string    `json:"reason"` // raison de SV1 (tag absent), not_computed, stale, no_clone
	SHA            *string    `json:"sha"`
	ComputedAt     *time.Time `json:"computed_at"`
	ReadmeExpected []string   `json:"readme_expected"` // sections du README attendues (information)
	ReadmeFound    []string   `json:"readme_found"`    // sections trouvées au tag (information)
}

// fileState renvoie l'état JSON d'un résultat.
func fileState(known bool) string {
	if known {
		return "computed"
	}
	return conventions.CriterionUnknown
}

// newConventionsFilesJSON convertit une grille.
func newConventionsFilesJSON(f conventions.FileReport, checks []conventions.FileCheck) conventionsFilesJSON {
	out := conventionsFilesJSON{State: fileState(f.Known), Reason: optionalString(f.Reason), Total: len(checks),
		Criteria: make([]conventionsFileCriterionJSON, 0, len(checks))}
	if f.Known {
		out.SHA, out.ComputedAt = optionalString(f.SHA), timeJSON(f.ComputedAt)
	}
	out.Conform, out.Unknown = conventions.TallyChecks(checks)
	for _, c := range checks {
		counts := map[string]int{}
		for k, v := range c.Counts {
			counts[k] = v
		}
		out.Criteria = append(out.Criteria, conventionsFileCriterionJSON{ID: c.ID, Status: c.Status,
			Reason: optionalString(c.Reason), Codes: nonNil(c.Codes), Counts: counts})
	}
	return out
}

// newConventionsDocsJSON convertit la grille de la documentation et ses listes (vides hors vue détaillée).
func newConventionsDocsJSON(f conventions.FileReport) conventionsDocsJSON {
	out := conventionsDocsJSON{conventionsFilesJSON: newConventionsFilesJSON(f, f.Results.Docs),
		Lists: conventionsDocListsJSON{Reason: optionalString(f.ListsReason), Unreferenced: []string{},
			DuplicateRows: []string{}, DeadLinks: []conventionsDeadLinkJSON{}, BadNames: []string{},
			Archives: []string{}, OfficeDocuments: []string{}}}
	if l := f.Lists; l != nil {
		out.Lists.Computed = true
		out.Lists.Unreferenced = nonNil(l.Unreferenced)
		out.Lists.DuplicateRows = nonNil(l.DuplicateRows)
		out.Lists.BadNames = nonNil(l.BadNames)
		out.Lists.Archives = nonNil(l.Archives)
		out.Lists.OfficeDocuments = nonNil(l.OfficeDocuments)
		for _, d := range l.DeadLinks {
			out.Lists.DeadLinks = append(out.Lists.DeadLinks, conventionsDeadLinkJSON{Source: d.Source,
				Target: d.Target, Resolved: d.Resolved, CaseMismatch: d.CaseMismatch})
		}
	}
	return out
}

// newConventionsMilestoneFilesJSON convertit l'état des critères de fichiers au tag d'un jalon ; nil si
// la revue n'est pas évaluable.
func newConventionsMilestoneFilesJSON(f *conventions.MilestoneFiles) *conventionsMilestoneFilesJSON {
	if f == nil {
		return nil
	}
	out := &conventionsMilestoneFilesJSON{State: fileState(f.Known), Reason: optionalString(f.Reason),
		ReadmeExpected: nonNil(f.ReadmeExpected), ReadmeFound: nonNil(f.ReadmeFound)}
	if f.Known {
		out.SHA, out.ComputedAt = optionalString(f.SHA), timeJSON(f.ComputedAt)
	}
	return out
}

// fileLines décrit les critères de fichiers d'un dépôt : révision mesurée, grille de chaque famille avec
// ses critères non conformes ou inconnus et leurs codes ; en vue détaillée, les listes de chemins.
func (s *session) fileLines(f conventions.FileReport, detail bool) []string {
	if !f.Known {
		return []string{i18n.T("cli.conv.files_unknown", i18n.T("cli.conv.reason."+f.Reason))}
	}
	lines := []string{i18n.T("cli.conv.files_measured", provider.ShortSHA(f.SHA),
		f.ComputedAt.In(s.loc).Format(i18n.T("cli.conv.fmt.date")))}
	for _, fam := range f.Results.Families() {
		conform, unknown := fam.Tally()
		lines = append(lines, i18n.T("cli.conv.files_line", i18n.T("conv.family."+fam.Name), conform,
			len(fam.Checks), unknown))
		for _, c := range fam.Checks {
			label := i18n.T("conv.criterion." + c.ID)
			switch {
			case c.Status == conventions.CriterionUnknown:
				lines = append(lines, "  "+i18n.T("cli.conv.file_unknown", c.ID, label, i18n.T("cli.conv.reason."+c.Reason)))
			case c.Status == conventions.CriterionNonconform || len(c.Codes) > 0:
				lines = append(lines, "  "+i18n.T("cli.conv.file_criterion", c.ID,
					i18n.T("cli.conv.criterion_status."+c.Status), label, fileCodesText(c)))
			}
		}
	}
	if detail {
		lines = append(lines, docListLines(f)...)
	}
	return lines
}

// fileCodesText écrit les codes d'un critère avec leur libellé, le nombre d'éléments qui les portent, et
// les compteurs utiles (mots de RD4, sections de RD2).
func fileCodesText(c conventions.FileCheck) string {
	parts := make([]string, 0, len(c.Codes)+1)
	for _, code := range c.Codes {
		p := code + " (" + i18n.T("conv.file_code."+code)
		if n := c.Counts[code]; n > 0 {
			p += ", " + strconv.Itoa(n)
		}
		parts = append(parts, p+")")
	}
	switch c.ID {
	case conventions.CriterionRD2:
		parts = append(parts, i18n.T("cli.conv.file_sections", c.Counts[conventions.CountFound], c.Counts[conventions.CountExpected]))
	case conventions.CriterionRD4:
		parts = append(parts, i18n.T("cli.conv.file_counts", c.Counts[conventions.CountWords], c.Counts[conventions.CountMinWords]))
	}
	return strings.Join(parts, ", ")
}

// docListLines écrit les listes de chemins de la vue détaillée (chemins nettoyés, au plus maxListText
// par liste).
func docListLines(f conventions.FileReport) []string {
	l := f.Lists
	if l == nil {
		if f.ListsReason == "" {
			return nil
		}
		return []string{i18n.T("cli.conv.lists_unavailable", i18n.T("cli.conv.reason."+f.ListsReason))}
	}
	dead := deadLinkTexts(l.DeadLinks)
	var lines []string
	for _, list := range []struct {
		key   string
		paths []string
	}{
		{"unreferenced", l.Unreferenced}, {"duplicate_rows", l.DuplicateRows}, {"dead_links", dead},
		{"bad_names", l.BadNames}, {"archives", l.Archives}, {"office_documents", l.OfficeDocuments},
	} {
		if len(list.paths) == 0 {
			continue
		}
		lines = append(lines, pathListLine(list.key, list.paths))
	}
	return lines
}

// pathListLine écrit une liste de chemins non vide de la vue détaillée (cli.conv.list.<key>) : chemins
// nettoyés, au plus maxListText, puis le nombre des autres.
func pathListLine(key string, paths []string) string {
	shown := paths[:min(len(paths), maxListText)]
	clean := make([]string, len(shown))
	for i, p := range shown {
		clean[i] = textclean.Clean(p)
	}
	text := strings.Join(clean, ", ")
	if more := len(paths) - len(shown); more > 0 {
		text += ", " + i18n.T("cli.conv.list_more", more)
	}
	return i18n.T("cli.conv.list."+key, len(paths), text)
}

// milestoneFileDetail précise un critère de fichiers de la revue : pour RD2 (section installation), les
// sections du README trouvées au tag, en information.
func milestoneFileDetail(r conventions.RepoMilestoneReview, id string) string {
	if id != conventions.CriterionRD2 || r.Files == nil || !r.Files.Known {
		return ""
	}
	if i := slices.IndexFunc(r.Criteria, func(c conventions.Criterion) bool { return c.ID == id }); i < 0 ||
		r.Criteria[i].Status == conventions.CriterionUnknown {
		return ""
	}
	found := i18n.T("cli.conv.milestone_readme_none")
	if len(r.Files.ReadmeFound) > 0 {
		names := make([]string, len(r.Files.ReadmeFound))
		for i, k := range r.Files.ReadmeFound {
			names[i] = i18n.T("conv.readme_section." + k)
		}
		found = strings.Join(names, ", ")
	}
	return " — " + i18n.T("cli.conv.milestone_readme", found)
}
