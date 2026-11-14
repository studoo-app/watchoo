package cli

// Sortie de la phase 8.5 de « watchoo conventions » : commits arrivés sur la branche par défaut hors PR
// (PR3), corps structuré des PR (PR5), méthode de fusion (PR6), branches réutilisées (BR2) et méthodes de
// fusion autorisées (BR5). Clés JSON en anglais, ajoutées à celles des phases précédentes. Jamais de corps
// ni de titre de PR, jamais de message de commit : SHA court, numéros, auteurs et codes seulement ; un
// commit hors PR est attribué à son auteur, jamais à la personne qui l'a poussé.

import (
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// conventionsBodyJSON est PR5 d'une PR : sections reconnues dans le corps (jamais le corps).
type conventionsBodyJSON struct {
	Known   bool     `json:"known"`   // masque lu à la synchro ; faux : inconnu
	Found   []string `json:"found"`   // context, changes, testing, screenshots
	Missing []string `json:"missing"` // sections attendues absentes ou vides
	Closes  bool     `json:"closes"`  // mot-clé de fermeture suivi d'un numéro d'issue
	Conform bool     `json:"conform"`
}

// conventionsMergeJSON est PR6 d'une PR fusionnée.
type conventionsMergeJSON struct {
	Method         string `json:"method"`          // squash, merge, rebase ou unknown
	MessageConform bool   `json:"message_conform"` // message du commit conforme au parseur (squash)
	Conform        bool   `json:"conform"`         // squash et message conforme
}

// conventionsReusedJSON est BR2 d'un dépôt.
type conventionsReusedJSON struct {
	Known  bool                        `json:"known"`
	Reason *string                     `json:"reason"` // no_api si inconnu ; null sinon
	Items  []conventionsReusedItemJSON `json:"items"`  // par nom
}

// conventionsReusedItemJSON est une branche réutilisée.
type conventionsReusedItemJSON struct {
	Name         string `json:"name"`
	PullRequests []int  `json:"pull_requests"` // PR fusionnées vers la branche par défaut depuis cette branche
}

// conventionsMergeMethodsJSON est BR5 d'un dépôt.
type conventionsMergeMethodsJSON struct {
	Known   bool    `json:"known"`
	Reason  *string `json:"reason"` // no_api ou no_push_access si inconnu ; null sinon
	Squash  *bool   `json:"squash"` // null si inconnu
	Merge   *bool   `json:"merge"`
	Rebase  *bool   `json:"rebase"`
	Conform bool    `json:"conform"` // seul le squash est autorisé
}

// conventionsArrivalsJSON est PR3 d'un dépôt (ou de l'intervalle d'un jalon).
type conventionsArrivalsJSON struct {
	Known        bool                            `json:"known"`
	Reason       *string                         `json:"reason"`   // no_clone ou clone_unreadable si inconnu
	Mode         string                          `json:"mode"`     // api, ou clone (repli sur les messages de GitHub)
	Complete     bool                            `json:"complete"` // liste des PR relue en entier
	ByPR         int                             `json:"by_pr"`
	OutsidePR    int                             `json:"outside_pr"`
	Undetermined int                             `json:"undetermined"`
	Reasons      map[string]int                  `json:"reasons"` // commits par raison du classement
	Outside      []conventionsArrivalOutsideJSON `json:"outside"` // commits arrivés hors PR, du plus récent
}

// conventionsArrivalOutsideJSON est un commit arrivé hors PR, attribué à son auteur.
type conventionsArrivalOutsideJSON struct {
	SHA      string     `json:"sha"`
	ShortSHA string     `json:"short_sha"`
	Reason   string     `json:"reason"` // human_committer, local_merge ou web_edit
	Author   authorJSON `json:"author"` // auteur du commit (jamais « a poussé »)
}

// conventionsAuthorArrivalsJSON compte les commits du premier parent dont un contributeur est l'auteur.
type conventionsAuthorArrivalsJSON struct {
	ByPR         int `json:"by_pr"`
	OutsidePR    int `json:"outside_pr"`
	Undetermined int `json:"undetermined"`
}

// newConventionsBodyJSON convertit PR5.
func newConventionsBodyJSON(b conventions.PRBodyResult) conventionsBodyJSON {
	return conventionsBodyJSON{Known: b.Known, Found: nonNil(b.Found), Missing: nonNil(b.Missing), Closes: b.Closes,
		Conform: b.Conform}
}

// newConventionsMergeJSON convertit PR6 ; nil pour une PR ouverte.
func newConventionsMergeJSON(l conventions.PullLine) *conventionsMergeJSON {
	if l.Merge.Method == "" {
		return nil
	}
	return &conventionsMergeJSON{Method: l.Merge.Method, MessageConform: l.Merge.MessageConform, Conform: l.Merge.Conform}
}

// newConventionsReusedJSON convertit BR2.
func newConventionsReusedJSON(b conventions.BranchReport) conventionsReusedJSON {
	out := conventionsReusedJSON{Known: b.ReusedKnown, Items: make([]conventionsReusedItemJSON, 0, len(b.Reused))}
	if !b.ReusedKnown {
		out.Reason = optionalString(conventions.UnknownNoAPI)
	}
	for _, r := range b.Reused {
		out.Items = append(out.Items, conventionsReusedItemJSON{Name: r.Name, PullRequests: append([]int{}, r.PRs...)})
	}
	return out
}

// newConventionsMergeMethodsJSON convertit BR5.
func newConventionsMergeMethodsJSON(m conventions.MergeMethodsReport) conventionsMergeMethodsJSON {
	out := conventionsMergeMethodsJSON{Known: m.Known, Reason: optionalString(m.Reason), Conform: m.Conform}
	if m.Known {
		out.Squash, out.Merge, out.Rebase = &m.Squash, &m.Merge, &m.Rebase
	}
	return out
}

// newConventionsArrivalsJSON convertit PR3.
func newConventionsArrivalsJSON(a conventions.ArrivalReport) conventionsArrivalsJSON {
	out := conventionsArrivalsJSON{Known: a.Known, Reason: optionalString(a.Reason), Mode: arrivalMode(a), Complete: a.Complete,
		ByPR: a.ByPR, OutsidePR: a.OutsidePR, Undetermined: a.Undetermined, Reasons: map[string]int{},
		Outside: []conventionsArrivalOutsideJSON{}}
	for _, k := range conventions.AllArrivalReasons {
		out.Reasons[k] = a.Reasons[k]
	}
	for _, l := range a.Outside() {
		out.Outside = append(out.Outside, conventionsArrivalOutsideJSON{SHA: l.SHA, ShortSHA: provider.ShortSHA(l.SHA),
			Reason: l.Reason, Author: conventionsAuthor(l.AuthorLabel, l.AuthorName, l.AuthorLogin)})
	}
	return out
}

// arrivalMode renvoie « api » ou « clone » (classement dégradé sur les messages de GitHub).
func arrivalMode(a conventions.ArrivalReport) string {
	if a.API {
		return "api"
	}
	return "clone"
}

// mergeFlowLines décrit PR3, PR5, PR6, PR4 (phase 8.6), BR2 et BR5 d'un dépôt.
func mergeFlowLines(r conventions.RepoReport) []string {
	var lines []string
	lines = append(lines, arrivalLines(r.Arrivals, "cli.conv.arrivals_line")...)
	p := r.Pulls
	if p.Known && p.Open.Evaluated+p.Merged.Evaluated > 0 {
		body := conventions.PRCount{BodyKnown: p.Open.BodyKnown + p.Merged.BodyKnown,
			BodyConform: p.Open.BodyConform + p.Merged.BodyConform}
		n := p.Open.Evaluated + p.Merged.Evaluated
		lines = append(lines, i18n.T("cli.conv.bodies_line", body.BodyConform, body.BodyKnown, n-body.BodyKnown))
		var missing []string
		for _, l := range p.Pulls {
			if l.Body.Known && !l.Body.Conform {
				missing = append(missing, "#"+strconv.Itoa(l.Number)+" ("+bodyGapText(l.Body)+")")
			}
		}
		if len(missing) > 0 {
			lines = append(lines, "  "+i18n.T("cli.conv.bodies_gaps", strings.Join(missing, ", ")))
		}
	}
	if p.Known && p.Merged.Evaluated > 0 {
		parts := make([]string, 0, len(conventions.AllMergeMethods))
		for _, m := range conventions.AllMergeMethods {
			parts = append(parts, i18n.T("cli.conv.merge_method."+m)+" "+strconv.Itoa(p.Methods[m]))
		}
		lines = append(lines, i18n.T("cli.conv.merges_line", p.Merged.MergeConform, p.Merged.Evaluated, strings.Join(parts, ", ")))
	}
	lines = append(lines, reviewLines(p)...)
	switch b := r.Branches; {
	case !b.ReusedKnown:
	case len(b.Reused) == 0:
		lines = append(lines, i18n.T("cli.conv.reused_none"))
	default:
		parts := make([]string, len(b.Reused))
		for i, rb := range b.Reused {
			parts[i] = textclean.Clean(rb.Name) + " (" + prNumbers(rb.PRs) + ")"
		}
		lines = append(lines, i18n.T("cli.conv.reused_line", len(b.Reused), strings.Join(parts, ", ")))
	}
	lines = append(lines, mergeMethodsLine(r.MergeMethods))
	return lines
}

// arrivalLines décrit PR3 : décompte (clé key), puis les commits arrivés hors PR (SHA court et auteur).
func arrivalLines(a conventions.ArrivalReport, key string) []string {
	if !a.Known {
		return []string{i18n.T("cli.conv.arrivals_unknown", i18n.T("cli.conv.reason."+a.Reason))}
	}
	line := i18n.T(key, a.OutsidePR, a.Total(), a.ByPR, a.Undetermined)
	if !a.API {
		line += i18n.T("cli.conv.arrivals_clone")
	} else if !a.Complete {
		line += i18n.T("cli.conv.arrivals_incomplete")
	}
	lines := []string{line}
	for _, l := range a.Outside() {
		author := cmpOr(l.DisplayAuthor(), i18n.T("cli.conv.unknown_author"))
		lines = append(lines, "  "+i18n.T("cli.conv.arrival_item", provider.ShortSHA(l.SHA), textclean.Clean(author),
			i18n.T("cli.conv.arrival_reason."+l.Reason)))
	}
	return lines
}

// bodyGapText écrit ce qui manque au corps d'une PR : sections attendues absentes, mot-clé de fermeture.
func bodyGapText(b conventions.PRBodyResult) string {
	parts := make([]string, 0, len(b.Missing)+1)
	for _, s := range b.Missing {
		parts = append(parts, i18n.T("cli.conv.pr_section."+s))
	}
	if !b.Closes {
		parts = append(parts, i18n.T("cli.conv.pr_section.closes"))
	}
	return strings.Join(parts, ", ")
}

// mergeMethodsLine décrit BR5.
func mergeMethodsLine(m conventions.MergeMethodsReport) string {
	if !m.Known {
		return i18n.T("cli.conv.merge_methods_unknown", i18n.T("cli.conv.reason."+m.Reason))
	}
	var allowed []string
	for _, x := range []struct {
		on  bool
		key string
	}{{m.Squash, conventions.MergeMethodSquash}, {m.Merge, conventions.MergeMethodMerge}, {m.Rebase, conventions.MergeMethodRebase}} {
		if x.on {
			allowed = append(allowed, i18n.T("cli.conv.merge_method."+x.key))
		}
	}
	list := strings.Join(allowed, ", ")
	if list == "" {
		list = i18n.T("cli.conv.merge_methods_none")
	}
	status := i18n.T("cli.conv.criterion_status." + conventions.CriterionNonconform)
	if m.Conform {
		status = i18n.T("cli.conv.criterion_status." + conventions.CriterionConform)
	}
	return i18n.T("cli.conv.merge_methods_line", list, status)
}

// authorArrivalsText écrit les commits arrivés hors PR d'un contributeur sur ceux du premier parent ;
// dash sans commit classé.
func authorArrivalsText(a conventions.ArrivalTally, dash string) string {
	if a.Total() == 0 {
		return dash
	}
	return strconv.Itoa(a.OutsidePR) + "/" + strconv.Itoa(a.Total())
}

// authorBodiesText écrit les corps de PR conformes d'un contributeur ; dash sans corps lu.
func authorBodiesText(p conventions.PRTally, dash string) string {
	n := p.Open.BodyKnown + p.Merged.BodyKnown
	if n == 0 {
		return dash
	}
	return strconv.Itoa(p.Open.BodyConform+p.Merged.BodyConform) + "/" + strconv.Itoa(n)
}

// milestoneArrivalsDetail précise PR3 dans la revue de jalon : « 1 commit hors PR sur 12 ».
func milestoneArrivalsDetail(r conventions.RepoMilestoneReview) string {
	a := r.Arrivals
	if a == nil || !a.Known {
		return ""
	}
	s := " — " + i18n.T("cli.conv.milestone_arrivals", a.OutsidePR, a.Total(), a.Undetermined)
	var shas []string
	for _, l := range a.Outside() {
		shas = append(shas, provider.ShortSHA(l.SHA))
	}
	if len(shas) > 0 {
		s += " (" + strings.Join(shas, ", ") + ")"
	}
	return s
}
