package cli

// Sortie de la phase 8.4 de « watchoo conventions » : réécritures observées de la branche par défaut (BR4),
// historique des tags (SV3), échéances des jalons (SV2), pré-versions (SV4), sauts de version (SV5),
// précédence (SV7) et fraîcheur du CHANGELOG (CL10). Clés JSON en anglais, ajoutées à celles des phases
// précédentes ; aucune attribution à une personne, aucun message de commit.

import (
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// conventionsRewritesJSON est BR4 d'un dépôt.
type conventionsRewritesJSON struct {
	Known  bool                          `json:"known"`
	Reason *string                       `json:"reason"` // filtered si inconnu ; null sinon
	Count  int                           `json:"count"`
	Last   *time.Time                    `json:"last"` // date de la plus récente ; null si aucune
	Events []conventionsRewriteEventJSON `json:"events"`
}

// conventionsRewriteEventJSON est une réécriture observée, sans attribution.
type conventionsRewriteEventJSON struct {
	Branch     string     `json:"branch"`
	OldSHA     string     `json:"old_sha"`
	NewSHA     string     `json:"new_sha"`
	ObservedAt *time.Time `json:"observed_at"`
}

// conventionsTagHistoryJSON est SV3 d'un dépôt.
type conventionsTagHistoryJSON struct {
	Known  bool                            `json:"known"`
	Reason *string                         `json:"reason"` // not_observed si inconnu ; null sinon
	Since  *time.Time                      `json:"since"`  // première observation du dépôt
	Counts map[string]int                  `json:"counts"` // stable, restored, rewritten, deleted
	Items  []conventionsTagHistoryItemJSON `json:"items"`
}

// conventionsTagHistoryItemJSON est SV3 d'un tag.
type conventionsTagHistoryItemJSON struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"` // stable, restored, rewritten ou deleted
	SHA         *string    `json:"sha"`    // commit visé actuel ; null si supprimé
	FirstSeenAt *time.Time `json:"first_seen_at"`
	Rewrites    int        `json:"rewrites"`
	Deletions   int        `json:"deletions"`
}

// conventionsDeadlineJSON est l'échéance de SV2 d'un jalon.
type conventionsDeadlineJSON struct {
	Status      string     `json:"status"`        // on_time, late ou undetermined
	DueEnd      *time.Time `json:"due_end"`       // fin de l'échéance (borne exclue)
	FirstSeenAt *time.Time `json:"first_seen_at"` // première observation du tag ; null si jamais observé
	CommitAt    *time.Time `json:"commit_at"`     // date du commit visé ; null si inconnue
}

// conventionsVersionNoteJSON est un constat SV4 ou SV7.
type conventionsVersionNoteJSON struct {
	Code  string  `json:"code"`
	Tag   string  `json:"tag"`
	Other *string `json:"other"`
}

// conventionsPrereleasesJSON est SV4 d'un dépôt (avertissements).
type conventionsPrereleasesJSON struct {
	Known   bool                         `json:"known"`
	Reason  *string                      `json:"reason"`
	Checked int                          `json:"checked"`
	Conform int                          `json:"conform"`
	Notes   []conventionsVersionNoteJSON `json:"notes"`
}

// conventionsPrecedenceJSON est SV7 d'un dépôt.
type conventionsPrecedenceJSON struct {
	Known   bool                         `json:"known"`
	Reason  *string                      `json:"reason"`
	Pairs   int                          `json:"pairs"`
	Unknown int                          `json:"unknown"`
	Conform bool                         `json:"conform"`
	Notes   []conventionsVersionNoteJSON `json:"notes"`
}

// conventionsBumpsJSON est SV5 d'un dépôt.
type conventionsBumpsJSON struct {
	Known  bool                      `json:"known"`
	Reason *string                   `json:"reason"`
	Counts map[string]int            `json:"counts"` // consistent, under, undetermined, unknown
	Items  []conventionsBumpItemJSON `json:"items"`
}

// conventionsBumpItemJSON est SV5 d'une paire de tags finals.
type conventionsBumpItemJSON struct {
	From      *string `json:"from"`
	To        string  `json:"to"`
	Status    string  `json:"status"`
	Reason    *string `json:"reason"`
	Actual    *string `json:"actual"`
	Expected  *string `json:"expected"`
	Evaluated int     `json:"evaluated"`
	Conform   int     `json:"conform"`
	Breaking  int     `json:"breaking"`
	Features  int     `json:"features"`
	Missing   int     `json:"missing"`
}

// conventionsFreshnessJSON est CL10 d'un dépôt (information).
type conventionsFreshnessJSON struct {
	Known   bool    `json:"known"`
	Reason  *string `json:"reason"`
	Since   *string `json:"since"` // dernier commit qui a touché le CHANGELOG
	Feat    int     `json:"feat"`
	Fix     int     `json:"fix"`
	Missing int     `json:"missing"`
	Grayed  bool    `json:"grayed"`
}

// conventionsMilestoneHistoryJSON est SV3 du tag d'un jalon dans la revue.
type conventionsMilestoneHistoryJSON struct {
	Status      string     `json:"status"`
	FirstSeenAt *time.Time `json:"first_seen_at"`
	Rewrites    int        `json:"rewrites"`
	Deletions   int        `json:"deletions"`
}

// newConventionsRewritesJSON convertit BR4.
func newConventionsRewritesJSON(r conventions.RewriteReport) conventionsRewritesJSON {
	out := conventionsRewritesJSON{Known: r.Known, Reason: optionalString(r.Reason), Count: r.Count, Last: timeJSON(r.Last),
		Events: make([]conventionsRewriteEventJSON, 0, len(r.Events))}
	for _, e := range r.Events {
		out.Events = append(out.Events, conventionsRewriteEventJSON{Branch: e.Branch, OldSHA: e.OldSHA, NewSHA: e.NewSHA,
			ObservedAt: timeJSON(e.ObservedAt)})
	}
	return out
}

// newConventionsTagHistoryJSON convertit SV3.
func newConventionsTagHistoryJSON(h conventions.TagHistoryReport) conventionsTagHistoryJSON {
	out := conventionsTagHistoryJSON{Known: h.Observed, Reason: optionalString(h.Reason), Since: timeJSON(h.Since),
		Counts: map[string]int{}, Items: make([]conventionsTagHistoryItemJSON, 0, len(h.Tags))}
	for _, s := range conventions.AllTagHistoryStatuses {
		out.Counts[s] = h.Counts[s]
	}
	for _, t := range h.Tags {
		out.Items = append(out.Items, conventionsTagHistoryItemJSON{Name: t.Name, Status: t.Status, SHA: optionalString(t.SHA),
			FirstSeenAt: timeJSON(t.FirstSeenAt), Rewrites: t.Rewrites, Deletions: t.Deletions})
	}
	return out
}

// newConventionsDeadlineJSON convertit l'échéance d'un jalon ; nil sans échéance jugée.
func newConventionsDeadlineJSON(d conventions.MilestoneDeadline) *conventionsDeadlineJSON {
	if d.Status == "" {
		return nil
	}
	return &conventionsDeadlineJSON{Status: d.Status, DueEnd: timeJSON(d.DueEnd), FirstSeenAt: timeJSON(d.FirstSeenAt),
		CommitAt: timeJSON(d.CommitAt)}
}

// newVersionNotesJSON convertit des constats SV4 ou SV7.
func newVersionNotesJSON(notes []conventions.VersionNote) []conventionsVersionNoteJSON {
	out := make([]conventionsVersionNoteJSON, 0, len(notes))
	for _, n := range notes {
		out = append(out, conventionsVersionNoteJSON{Code: n.Code, Tag: n.Tag, Other: optionalString(n.Other)})
	}
	return out
}

// newConventionsPrereleasesJSON convertit SV4.
func newConventionsPrereleasesJSON(p conventions.PrereleaseReport) conventionsPrereleasesJSON {
	return conventionsPrereleasesJSON{Known: p.Known, Reason: optionalString(p.Reason), Checked: p.Checked,
		Conform: p.Conform, Notes: newVersionNotesJSON(p.Notes)}
}

// newConventionsPrecedenceJSON convertit SV7.
func newConventionsPrecedenceJSON(p conventions.PrecedenceReport) conventionsPrecedenceJSON {
	return conventionsPrecedenceJSON{Known: p.Known, Reason: optionalString(p.Reason), Pairs: p.Pairs, Unknown: p.Unknown,
		Conform: p.Conform(), Notes: newVersionNotesJSON(p.Notes)}
}

// newConventionsBumpsJSON convertit SV5.
func newConventionsBumpsJSON(b conventions.BumpReport) conventionsBumpsJSON {
	out := conventionsBumpsJSON{Known: b.Known, Reason: optionalString(b.Reason), Counts: map[string]int{},
		Items: make([]conventionsBumpItemJSON, 0, len(b.Lines))}
	for _, s := range conventions.AllBumpStatuses {
		out.Counts[s] = b.Counts[s]
	}
	for _, l := range b.Lines {
		out.Items = append(out.Items, conventionsBumpItemJSON{From: optionalString(l.From), To: l.To, Status: l.Status,
			Reason: optionalString(l.Reason), Actual: optionalString(l.Actual), Expected: optionalString(l.Expected),
			Evaluated: l.Evaluated, Conform: l.Conform, Breaking: l.Breaking, Features: l.Features, Missing: l.Missing})
	}
	return out
}

// newConventionsFreshnessJSON convertit CL10.
func newConventionsFreshnessJSON(f conventions.ChangelogFreshness) conventionsFreshnessJSON {
	return conventionsFreshnessJSON{Known: f.Known, Reason: optionalString(f.Reason), Since: optionalString(f.Since),
		Feat: f.Feat, Fix: f.Fix, Missing: f.Missing, Grayed: f.Grayed}
}

// newConventionsMilestoneHistoryJSON convertit SV3 du tag d'un jalon ; nil s'il est absent ou jamais observé.
func newConventionsMilestoneHistoryJSON(h *conventions.TagHistory) *conventionsMilestoneHistoryJSON {
	if h == nil {
		return nil
	}
	return &conventionsMilestoneHistoryJSON{Status: h.Status, FirstSeenAt: timeJSON(h.FirstSeenAt), Rewrites: h.Rewrites,
		Deletions: h.Deletions}
}

// Texte.

// dateText écrit une date dans le fuseau loc ; « une date inconnue » si elle est zéro.
func dateText(t time.Time, loc *time.Location) string {
	if t.IsZero() {
		return i18n.T("conv.deadline.unknown_date")
	}
	if loc == nil {
		loc = time.UTC
	}
	return t.In(loc).Format(i18n.T("cli.conv.fmt.date"))
}

// unknownLine écrit « <indicateur> : inconnu (<raison>) ».
func unknownLine(id, reason string) string {
	return i18n.T("cli.conv.history_unknown", i18n.T("conv.indicator."+id), i18n.T("cli.conv.reason."+reason))
}

// historyLines décrit les indicateurs de la phase 8.4 d'un dépôt : BR4, SV3, échéances de SV2, SV4, SV7,
// SV5 et CL10.
func historyLines(r conventions.RepoReport, loc *time.Location) []string {
	var lines []string
	lines = append(lines, rewriteLines(r.Rewrites, loc)...)
	lines = append(lines, tagHistoryLines(r.TagHistory, loc)...)
	if l := deadlinesLine(r.Tags.Milestones, loc); l != "" {
		lines = append(lines, l)
	}
	lines = append(lines, prereleaseLines(r.Prereleases)...)
	lines = append(lines, precedenceLines(r.Precedence)...)
	lines = append(lines, bumpLines(r.Bumps)...)
	lines = append(lines, freshnessLines(r.Freshness)...)
	return lines
}

// rewriteLines décrit BR4 : nombre, date de la dernière, puis chaque événement (SHA courts).
func rewriteLines(r conventions.RewriteReport, loc *time.Location) []string {
	switch {
	case !r.Known:
		return []string{unknownLine("BR4", r.Reason)}
	case r.Count == 0:
		return []string{i18n.T("conv.rewrites.none")}
	}
	lines := []string{i18n.T("conv.rewrites.count", r.Count, dateText(r.Last, loc))}
	for _, e := range r.Events {
		lines = append(lines, "  "+i18n.T("conv.rewrites.event", textclean.Clean(e.Branch), dateText(e.ObservedAt, loc),
			provider.ShortSHA(e.OldSHA), provider.ShortSHA(e.NewSHA)))
	}
	return lines
}

// tagHistoryLines décrit SV3 : décompte, puis les tags réécrits, supprimés ou reposés.
func tagHistoryLines(h conventions.TagHistoryReport, loc *time.Location) []string {
	if !h.Observed {
		return []string{unknownLine("SV3", h.Reason)}
	}
	lines := []string{i18n.T("conv.history.line", h.Counts[conventions.TagHistoryStable]+h.Counts[conventions.TagHistoryRestored],
		dateText(h.Since, loc), h.Counts[conventions.TagHistoryRewritten], h.Counts[conventions.TagHistoryDeleted])}
	for _, t := range h.Tags {
		if t.Status == conventions.TagHistoryStable {
			continue
		}
		lines = append(lines, "  "+i18n.T("cli.conv.history_item", textclean.Clean(t.Name),
			i18n.T("conv.history.status."+t.Status), i18n.T("conv.history.first_seen", dateText(t.FirstSeenAt, loc))))
	}
	return lines
}

// deadlineText écrit l'échéance d'un jalon : « à l'heure », « en retard (vu pour la première fois le …) »,
// « indéterminé (posé entre le … et le …) » ; vide sans échéance jugée.
func deadlineText(d conventions.MilestoneDeadline, loc *time.Location) string {
	switch d.Status {
	case "":
		return ""
	case conventions.DeadlineUndetermined:
		return i18n.T("conv.deadline."+d.Status) + " (" +
			i18n.T("conv.deadline.between", dateText(d.CommitAt, loc), dateText(d.FirstSeenAt, loc)) + ")"
	}
	return i18n.T("conv.deadline."+d.Status) + " (" + i18n.T("conv.deadline.first_seen", dateText(d.FirstSeenAt, loc)) + ")"
}

// deadlinesLine décrit les échéances de SV2 jugées ; vide si aucune.
func deadlinesLine(ms []conventions.MilestoneResult, loc *time.Location) string {
	var parts []string
	for _, m := range ms {
		if s := deadlineText(m.Deadline, loc); s != "" {
			parts = append(parts, m.Name+" "+s)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return i18n.T("cli.conv.deadlines_line", strings.Join(parts, ", "))
}

// noteText écrit un constat SV4 ou SV7 : « v1.0.0-rc1 (forme libre) », « v2.0.0 / v1.9.0 (…) ».
func noteText(prefix string, n conventions.VersionNote) string {
	s := textclean.Clean(n.Tag)
	if n.Other != "" {
		s += " / " + textclean.Clean(n.Other)
	}
	return s + " (" + i18n.T(prefix+n.Code) + ")"
}

// prereleaseLines décrit SV4 (avertissements).
func prereleaseLines(p conventions.PrereleaseReport) []string {
	switch {
	case !p.Known:
		return []string{unknownLine("SV4", p.Reason)}
	case p.Checked == 0:
		return []string{i18n.T("conv.prereleases.none")}
	}
	lines := []string{i18n.T("conv.prereleases.line", p.Conform, p.Checked)}
	for _, n := range p.Notes {
		lines = append(lines, "  "+noteText("conv.prerelease_code.", n))
	}
	return lines
}

// precedenceLines décrit SV7.
func precedenceLines(p conventions.PrecedenceReport) []string {
	if p.Reason == conventions.UnknownFiltered {
		return []string{unknownLine("SV7", p.Reason)}
	}
	line := i18n.T("conv.precedence.line", p.Pairs)
	switch {
	case !p.Known:
		line += " (" + i18n.T("cli.conv.reason."+p.Reason) + ")"
	case p.Conform():
		line = i18n.T("cli.conv.label_value", line, i18n.T("conv.precedence.consistent"))
	case p.Unknown > 0:
		line += i18n.T("cli.conv.sep") + i18n.T("conv.precedence.unread", p.Unknown)
	}
	lines := []string{line}
	for _, n := range p.Notes {
		lines = append(lines, "  "+noteText("conv.precedence_code.", n))
	}
	return lines
}

// bumpLines décrit SV5 : décompte, puis chaque paire qui n'est pas cohérente.
func bumpLines(b conventions.BumpReport) []string {
	switch {
	case !b.Known:
		return []string{unknownLine("SV5", b.Reason)}
	case len(b.Lines) == 0:
		return []string{i18n.T("conv.bumps.none")}
	}
	lines := []string{i18n.T("conv.bumps.line", b.Counts[conventions.BumpConsistent], b.Counts[conventions.BumpUnder],
		b.Counts[conventions.BumpUndetermined], b.Counts[conventions.BumpUnknown])}
	for _, l := range b.Lines {
		if l.Status == conventions.BumpConsistent {
			continue
		}
		from := l.From
		if from == "" {
			from = "?"
		}
		s := textclean.Clean(from) + "→" + textclean.Clean(l.To) + " " + i18n.T("conv.bump.status."+l.Status)
		switch {
		case l.Status == conventions.BumpUnder:
			s += " (" + i18n.T("conv.bump.expected", i18n.T("conv.bump."+l.Actual), i18n.T("conv.bump."+l.Expected)) + ")"
		case l.Reason != "":
			s += " (" + i18n.T("cli.conv.reason."+l.Reason) + ")"
		}
		lines = append(lines, "  "+s)
	}
	return lines
}

// freshnessLines décrit CL10 (information).
func freshnessLines(f conventions.ChangelogFreshness) []string {
	if !f.Known {
		return []string{unknownLine("CL10", f.Reason)}
	}
	line := i18n.T("conv.freshness.line", f.Feat, f.Fix, provider.ShortSHA(f.Since))
	if f.Missing > 0 {
		line += i18n.T("cli.conv.sep") + i18n.T("conv.freshness.missing", f.Missing)
	}
	if f.Grayed {
		line += i18n.T("cli.conv.sep") + i18n.T("conv.freshness.grayed")
	}
	return []string{line}
}
