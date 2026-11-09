package cli

// Texte des indicateurs de la phase 8.2 de « watchoo conventions » : lignes de détail d'un dépôt (BR1,
// PR1, PR2, SV1, SV2, SV6, CC5 à CC7, CC1 par intervalle), colonnes des contributeurs (tendance de CC3,
// titres de PR, branches) et revue de jalon (--milestone). Jamais de titre de PR ni de message de commit :
// numéros, noms de branche et de tag, auteurs et codes seulement.

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// conventionsMoreDetail renvoie les lignes de détail de la phase 8.2 d'un dépôt.
func conventionsMoreDetail(r conventions.RepoReport, dash string) []string {
	var lines []string
	lines = append(lines, branchLines(r.Branches)...)
	lines = append(lines, pullLines(r.Pulls)...)
	lines = append(lines, tagLines(r.Tags)...)
	lines = append(lines, traitLines(r.Traits, r.Language)...)
	lines = append(lines, intervalLine(r.Intervals, dash))
	return lines
}

// branchLines décrit BR1 : décompte, puis les noms par code.
func branchLines(b conventions.BranchReport) []string {
	source := i18n.T("cli.conv.branches_source." + b.Source)
	if b.Checked == 0 {
		return []string{i18n.T("cli.conv.branches_none", source)}
	}
	lines := []string{i18n.T("cli.conv.branches_line", source, b.Conform, b.Checked, b.Review)}
	for _, code := range conventions.AllBranchCodes {
		var names []string
		for _, l := range b.Branches {
			if !slices.Contains(l.Codes, code) {
				continue
			}
			name := textclean.Clean(l.Name)
			if len(l.PRs) > 0 {
				name += " (" + prNumbers(l.PRs) + ")"
			}
			names = append(names, name)
		}
		if len(names) > 0 {
			lines = append(lines, "  "+i18n.T("cli.conv.code_names", code, len(names),
				i18n.T("cli.conv.branch_code."+code), i18n.T("cli.conv.severity."+conventions.BranchSeverityOf(code).String()),
				strings.Join(names, ", ")))
		}
	}
	return lines
}

// pullLines décrit PR1 et PR2 : décomptes, PR écartées, puis chaque PR non conforme ou au titre par
// défaut (numéro, auteur, branche de tête et codes, jamais le titre).
func pullLines(p conventions.PullReport) []string {
	if !p.Known {
		return []string{i18n.T("cli.conv.pulls_unknown", i18n.T("cli.conv.reason."+p.Reason))}
	}
	if p.Open.Evaluated+p.Merged.Evaluated == 0 {
		return []string{i18n.T("cli.conv.pulls_none") + skippedText(p.Skipped)}
	}
	lines := []string{i18n.T("cli.conv.pulls_line", p.Open.Conform, p.Open.Evaluated, p.Merged.Conform, p.Merged.Evaluated,
		p.Open.Default+p.Merged.Default) + skippedText(p.Skipped)}
	for _, l := range p.Pulls {
		if l.Conform && len(l.Default) == 0 {
			continue
		}
		codes := append(append([]string{}, l.Codes...), l.Default...)
		author := cmpOr(cmpOr(l.AuthorName, l.AuthorLogin), i18n.T("cli.conv.unknown_author"))
		lines = append(lines, "  "+i18n.T("cli.conv.pull_item", l.Number, textclean.Clean(author),
			textclean.Clean(l.HeadBranch), strings.Join(codes, ", ")))
	}
	return lines
}

// skippedText écrit les PR écartées de PR1 et PR2 (« ; écartées : fermées 1 ») ; vide si aucune.
func skippedText(skipped map[string]int) string {
	var parts []string
	for _, s := range conventions.AllPRSkips {
		if n := skipped[s]; n > 0 {
			parts = append(parts, i18n.T("cli.conv.pr_skip."+s)+" "+strconv.Itoa(n))
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return i18n.T("cli.conv.pulls_skipped", strings.Join(parts, ", "))
}

// tagLines décrit SV1 (noms par code), SV2 (jalons) et SV6 (information).
func tagLines(t conventions.TagReport) []string {
	var lines []string
	if t.Checked == 0 {
		lines = append(lines, i18n.T("cli.conv.tags_none"))
	} else {
		lines = append(lines, i18n.T("cli.conv.tags_line", t.Conform, t.Checked))
	}
	for _, code := range conventions.AllTagCodes {
		var names []string
		for _, tag := range t.Tags {
			if slices.Contains(tag.Codes, code) {
				names = append(names, textclean.Clean(tag.Name))
			}
		}
		if len(names) > 0 {
			lines = append(lines, "  "+i18n.T("cli.conv.code_names", code, len(names),
				i18n.T("cli.conv.tag_code."+code), i18n.T("cli.conv.severity."+conventions.TagSeverityOf(code).String()),
				strings.Join(names, ", ")))
		}
	}
	if len(t.Milestones) > 0 {
		parts := make([]string, len(t.Milestones))
		for i, m := range t.Milestones {
			parts[i] = milestoneStatusText(m)
		}
		lines = append(lines, i18n.T("cli.conv.milestones_line", strings.Join(parts, ", ")))
	}
	switch rel := t.Releases; {
	case !rel.Known:
		lines = append(lines, i18n.T("cli.conv.releases_unknown", i18n.T("cli.conv.reason."+rel.Reason)))
	case len(rel.Notes) == 0:
		lines = append(lines, i18n.T("cli.conv.releases_none"))
	default:
		parts := make([]string, len(rel.Notes))
		for i, n := range rel.Notes {
			parts[i] = i18n.T("cli.conv.release_code."+n.Code) + " (" + textclean.Clean(n.Tag) + ")"
		}
		lines = append(lines, i18n.T("cli.conv.releases_line", strings.Join(parts, ", ")))
	}
	return lines
}

// milestoneStatusText écrit la présence d'un jalon : « v1.0.0 absent », « v1.0.0 nom non conforme (v1.0) ».
func milestoneStatusText(m conventions.MilestoneResult) string {
	s := m.Name + " " + i18n.T("cli.conv.milestone_status."+m.Status)
	if m.Status == conventions.MilestoneNonconformingName && m.Tag != "" {
		s += " (" + textclean.Clean(m.Tag) + ")"
	}
	return s
}

// traitLines décrit CC5 et CC6 (avertissements et information) et CC7 ; aucune ligne sans commit évalué.
func traitLines(t conventions.TraitsTally, l conventions.LanguageSummary) []string {
	if t.Evaluated == 0 {
		return nil
	}
	parts := make([]string, len(conventions.AllWordingCodes))
	for i, c := range conventions.AllWordingCodes {
		parts[i] = i18n.T("cli.conv.wording."+c) + " " + strconv.Itoa(t.Wording[c])
	}
	lang := i18n.T("cli.conv.language_line", l.FR, l.EN, l.Unknown)
	if l.Expected == conventions.LanguageFR || l.Expected == conventions.LanguageEN {
		lang += i18n.T("cli.conv.language_other", l.Other, l.Expected)
	}
	if l.Mixed {
		lang += i18n.T("cli.conv.language_mixed")
	}
	return []string{
		i18n.T("cli.conv.wording_line", strings.Join(parts, ", ")),
		i18n.T("cli.conv.footers_line", t.Footer, t.Evaluated, t.Issue, t.Evaluated),
		lang,
	}
}

// intervalLine décrit CC1 par intervalle entre tags finals, puis depuis le dernier.
func intervalLine(iv conventions.IntervalReport, dash string) string {
	if !iv.Known {
		return i18n.T("cli.conv.intervals_unknown", i18n.T("cli.conv.reason."+iv.Reason))
	}
	parts := make([]string, 0, len(iv.Intervals)+1)
	for _, l := range iv.Intervals {
		from := i18n.T("cli.conv.interval_start")
		if l.From != "" {
			from = textclean.Clean(l.From)
		}
		s := from + "→" + textclean.Clean(l.To) + " "
		if !l.Known {
			s += i18n.T("cli.conv.interval_unreadable")
		} else {
			s += ratioText(l.Tally) + " " + rateText(l.Tally, dash)
			if l.Missing > 0 {
				s += i18n.T("cli.conv.interval_missing", l.Missing)
			}
		}
		parts = append(parts, s)
	}
	parts = append(parts, i18n.T("cli.conv.interval_since", textclean.Clean(iv.LastTag))+" "+
		ratioText(iv.SinceLastTag)+" "+rateText(iv.SinceLastTag, dash))
	return i18n.T("cli.conv.intervals_line", strings.Join(parts, ", "))
}

// trendText écrit un des deux taux de la tendance de CC3 ; dash si la tendance n'est pas mesurée.
func trendText(t conventions.Tally, trend conventions.Trend, dash string) string {
	if !trend.Known() {
		return dash
	}
	return ratioText(t) + " " + rateText(t, dash)
}

// authorPullsText écrit les titres de PR conformes d'un contributeur (ouvertes et fusionnées) ; dash
// sans PR évaluée.
func authorPullsText(p conventions.PRTally, dash string) string {
	n := p.Open.Evaluated + p.Merged.Evaluated
	if n == 0 {
		return dash
	}
	return strconv.Itoa(p.Open.Conform+p.Merged.Conform) + "/" + strconv.Itoa(n)
}

// authorBranchesText écrit les branches conformes d'un contributeur ; dash sans branche.
func authorBranchesText(b conventions.BranchTally, dash string) string {
	if b.Checked == 0 {
		return dash
	}
	return strconv.Itoa(b.Conform) + "/" + strconv.Itoa(b.Checked)
}

// milestoneText écrit la revue d'un jalon : par dépôt, « k/n critères conformes, u inconnus » puis
// l'état de chaque critère ; aucune note globale ni tri.
func milestoneText(st textStyle, rv conventions.MilestoneReview, loc *time.Location) string {
	var b strings.Builder
	b.WriteString(st.gray(i18n.T("cli.conv.milestone_notice")) + "\n\n")
	title := i18n.T("cli.conv.milestone_title", rv.Milestone.Name)
	if rv.Milestone.Due != "" {
		title += i18n.T("cli.conv.milestone_due", rv.Milestone.Due)
	}
	b.WriteString(st.bold(title) + "\n")
	if len(rv.Repos) == 0 {
		b.WriteString(i18n.T("cli.conv.no_repos") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	dash := st.pick("—", "-")
	for _, r := range rv.Repos {
		b.WriteString("\n")
		if !r.Evaluable {
			b.WriteString(i18n.T("cli.conv.milestone_not_evaluable", r.Repo.DisplayName(),
				i18n.T("cli.conv.reason."+r.Reason)) + "\n")
			continue
		}
		b.WriteString(i18n.T("cli.conv.milestone_repo", r.Repo.DisplayName(), r.Conform, len(r.Criteria), r.Unknown) + "\n")
		for _, c := range r.Criteria {
			status := i18n.T("cli.conv.criterion_status." + c.Status)
			if c.Reason != "" {
				status += " (" + i18n.T("cli.conv.reason."+c.Reason) + ")"
			}
			b.WriteString("  " + c.ID + "  " + status + criterionDetail(r, c.ID, dash, loc) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// criterionDetail précise un critère de la revue : intervalle et taux (CC1), branches (BR1), tag (SV1,
// SV2 et son échéance), historique du tag (SV3, phase 8.4), sections du README au tag (RD2, phase 8.3).
func criterionDetail(r conventions.RepoMilestoneReview, id, dash string, loc *time.Location) string {
	switch id {
	case conventions.CriterionCC1:
		iv := r.Interval
		if iv == nil {
			return ""
		}
		from := i18n.T("cli.conv.interval_start")
		if iv.From != "" {
			from = textclean.Clean(iv.From)
		}
		to := textclean.Clean(iv.To)
		if to == "" {
			to = i18n.T("cli.conv.milestone_due_bound", iv.Due)
		}
		s := " — " + from + "→" + to + " " + ratioText(iv.Tally) + " " + rateText(iv.Tally, dash)
		if iv.Missing > 0 {
			s += i18n.T("cli.conv.interval_missing", iv.Missing)
		}
		return s
	case conventions.CriterionBR1:
		if r.Branches.Checked == 0 {
			return ""
		}
		return " — " + i18n.T("cli.conv.milestone_branches", r.Branches.Conform, r.Branches.Checked)
	case conventions.CriterionSV1:
		if r.Tag == nil {
			return ""
		}
		s := " — " + textclean.Clean(r.Tag.Name)
		if len(r.Tag.Codes) > 0 {
			s += " (" + strings.Join(r.Tag.Codes, ", ") + ")"
		}
		return s
	case conventions.CriterionSV2:
		s := " — " + milestoneStatusText(r.Milestone)
		if d := deadlineText(r.Milestone.Deadline, loc); d != "" {
			s += ", " + d
		}
		return s
	case conventions.CriterionSV3:
		if r.History == nil {
			return ""
		}
		s := " — " + i18n.T("conv.history.status."+r.History.Status)
		if r.History.Status == conventions.TagHistoryStable {
			s = " — " + i18n.T("conv.history.since", dateText(r.History.FirstSeenAt, loc))
		}
		return s
	}
	return milestoneFileDetail(r, id)
}

// milestoneNames liste les jalons configurés (« v1.0.0, v2.0.0 ») ; « — » sans jalon.
func milestoneNames(ms []conventions.Milestone) string {
	if len(ms) == 0 {
		return "—"
	}
	names := make([]string, len(ms))
	for i, m := range ms {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}

// prNumbers écrit des numéros de PR : « #1, #4 ».
func prNumbers(prs []int) string {
	parts := make([]string, len(prs))
	for i, n := range prs {
		parts[i] = "#" + strconv.Itoa(n)
	}
	return strings.Join(parts, ", ")
}
