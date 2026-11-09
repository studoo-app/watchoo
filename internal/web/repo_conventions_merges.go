package web

// Conventions de la phase 8.5 sur la page d'un dépôt (bloc « fusions » du fragment « repo-conventions »),
// sur « /conventions » et sur la page d'un contributeur : commits arrivés sur la branche par défaut hors
// PR (PR3 : SHA court et auteur du commit, jamais « a poussé »), corps structuré des PR (PR5), méthode de
// fusion (PR6), branches réutilisées (BR2) et méthodes de fusion autorisées (BR5). Ni corps ni titre de
// PR, ni message de commit : codes, numéros, SHA courts et auteurs seulement.

import (
	"cmp"
	"strings"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// convArrivalRow est un commit arrivé hors PR, attribué à son auteur.
type convArrivalRow struct {
	SHA      string
	Author   string // nom, sinon login ; vide si inconnu
	AuthorID int64  // contributeur ; 0 si aucun
	Reason   string // raison traduite (poussé directement, fusion locale, édition web)
	Repo     string // dépôt (page d'un contributeur) ; vide sur la page du dépôt
	RepoID   int64
}

// convArrivals est PR3 d'un dépôt, ou d'un contributeur tous dépôts confondus.
type convArrivals struct {
	Known  bool
	Reason string // raison traduite si inconnu
	// Note précise un classement dégradé (clone seul) ou une liste des PR pas encore relue ; vide sinon.
	Note                                 string
	ByPR, OutsidePR, Undetermined, Total int
	Rows                                 []convArrivalRow // au plus repoConventionsList, du plus récent
	RowsTotal                            int
}

// convReusedRow est une branche réutilisée (BR2).
type convReusedRow struct {
	Name string
	PRs  []int
}

// convMergesView est le bloc « fusions » de la page du dépôt.
type convMergesView struct {
	Arrivals convArrivals
	// PR5 (toutes les PR évaluées) et PR6 (fusionnées) ; PullsKnown faux sans API (rien n'est affiché).
	PullsKnown                            bool
	BodyEvaluated, BodyKnown, BodyConform int
	MergedEvaluated, MergeConform         int
	Methods                               []convCount // PR fusionnées par méthode, méthodes à au moins une PR
	// BR2 ; ReusedKnown faux sans API.
	ReusedKnown bool
	Reused      []convReusedRow
	// BR5 : pastille et méthodes autorisées (libellés) ; MethodsReason traduit si inconnu.
	MethodsKnown  bool
	MethodsReason string
	MethodsStatus convStatus
	Allowed       []string
}

// newConvArrivals traduit PR3 d'un dépôt ; repo nomme le dépôt des lignes (page d'un contributeur).
func newConvArrivals(tr *i18n.Translator, a conventions.ArrivalReport) convArrivals {
	out := convArrivals{Known: a.Known}
	if !a.Known {
		out.Reason = tr.T("web.conv.reason." + a.Reason)
		return out
	}
	switch {
	case !a.API:
		out.Note = tr.T("web.conv.arrivals_clone")
	case !a.Complete:
		out.Note = tr.T("web.conv.arrivals_incomplete")
	}
	out.addArrivals(tr, a, "", 0)
	return out
}

// addArrivals ajoute les décomptes et les commits hors PR de a.
func (c *convArrivals) addArrivals(tr *i18n.Translator, a conventions.ArrivalReport, repo string, repoID int64) {
	c.ByPR += a.ByPR
	c.OutsidePR += a.OutsidePR
	c.Undetermined += a.Undetermined
	c.Total += a.Total()
	for _, l := range a.Outside() {
		c.RowsTotal++
		if len(c.Rows) < repoConventionsList {
			c.Rows = append(c.Rows, convArrivalRow{SHA: l.SHA, Author: cmp.Or(l.AuthorName, l.AuthorLogin),
				AuthorID: l.AuthorID, Reason: tr.T("web.conv.arrival_reason." + l.Reason), Repo: repo, RepoID: repoID})
		}
	}
}

// newConvMerges remplit le bloc « fusions » d'un dépôt.
func newConvMerges(tr *i18n.Translator, repo conventions.RepoReport) convMergesView {
	v := convMergesView{Arrivals: newConvArrivals(tr, repo.Arrivals)}
	p := repo.Pulls
	if v.PullsKnown = p.Known; p.Known {
		v.BodyEvaluated = p.Open.Evaluated + p.Merged.Evaluated
		v.BodyKnown = p.Open.BodyKnown + p.Merged.BodyKnown
		v.BodyConform = p.Open.BodyConform + p.Merged.BodyConform
		v.MergedEvaluated, v.MergeConform = p.Merged.Evaluated, p.Merged.MergeConform
		for _, m := range conventions.AllMergeMethods {
			if n := p.Methods[m]; n > 0 {
				v.Methods = append(v.Methods, convCount{Code: m, Label: tr.T("web.conv.merge_method." + m), Count: n})
			}
		}
	}
	v.ReusedKnown = repo.Branches.ReusedKnown
	for _, b := range repo.Branches.Reused {
		v.Reused = append(v.Reused, convReusedRow{Name: b.Name, PRs: b.PRs})
	}
	m := repo.MergeMethods
	v.MethodsKnown = m.Known
	if !m.Known {
		v.MethodsReason = tr.T("web.conv.reason." + m.Reason)
		return v
	}
	v.MethodsStatus = statusNonconform(tr, "web.conv.methods_nonconform")
	if m.Conform {
		v.MethodsStatus = statusConform(tr, "web.conv.methods_conform")
	}
	for _, x := range []struct {
		on  bool
		key string
	}{{m.Squash, conventions.MergeMethodSquash}, {m.Merge, conventions.MergeMethodMerge}, {m.Rebase, conventions.MergeMethodRebase}} {
		if x.on {
			v.Allowed = append(v.Allowed, tr.T("web.conv.merge_method."+x.key))
		}
	}
	return v
}

// bodyStatus est la pastille PR5 d'une PR.
func bodyStatus(tr *i18n.Translator, b conventions.PRBodyResult) convStatus {
	switch {
	case !b.Known:
		return statusUnknown(tr, "web.conv.body_unknown")
	case b.Conform:
		return statusConform(tr, "web.conv.body_conform")
	}
	parts := make([]string, 0, len(b.Missing)+1)
	for _, s := range b.Missing {
		parts = append(parts, tr.T("web.conv.pr_section."+s))
	}
	if !b.Closes {
		parts = append(parts, tr.T("web.conv.pr_section.closes"))
	}
	return convStatus{Icon: "✗", Label: tr.T("web.conv.body_missing", strings.Join(parts, ", ")), Class: "badge-error"}
}

// mergeStatus est la pastille PR6 d'une PR fusionnée ; nil pour une PR ouverte.
func mergeStatus(tr *i18n.Translator, m conventions.MergeResult) *convStatus {
	var s convStatus
	switch {
	case m.Method == "":
		return nil
	case m.Conform:
		s = statusConform(tr, "web.conv.merge_method.squash")
	case m.Method == conventions.MergeMethodSquash:
		s = statusNonconform(tr, "web.conv.merge_squash_message")
	case m.Method == conventions.MergeMethodUnknown:
		s = statusUnknown(tr, "web.conv.merge_method.unknown")
	default:
		s = statusNonconform(tr, "web.conv.merge_method."+m.Method)
	}
	return &s
}

// pullNeedsRow indique une PR à lister sur la page du dépôt : titre non conforme ou par défaut (PR1,
// PR2), corps connu non conforme (PR5) ou fusion connue autre qu'un squash au message conforme (PR6).
func pullNeedsRow(l conventions.PullLine) bool {
	return !l.Conform || len(l.Default) > 0 || (l.Body.Known && !l.Body.Conform) ||
		(l.Merge.Method != "" && l.Merge.Known() && !l.Merge.Conform)
}
