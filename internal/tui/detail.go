package tui

// Détail d'un dépôt (« entrée » sur un dépôt ou un commit) : métadonnées, état d'activité, dernière
// synchro, pull requests ouvertes et dernière release (V2), derniers commits et principaux contributeurs, avec leurs commits par semaine sur
// activity.ContributorWeeks semaines (total et courbe d'une ligne, activity.Sparkline).

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
)

// renderDetail renvoie les lignes du détail, à partir du défilement courant.
func (m Model) renderDetail() []string {
	lines := m.detailLines()
	// Le défilement s'arrête quand la dernière ligne est visible.
	off := min(m.detail.offset, max(len(lines)-m.bodyHeight(), 0))
	return lines[off:]
}

// detailLines renvoie toutes les lignes du détail du dépôt ouvert.
func (m Model) detailLines() []string {
	r, ok := m.repoByID(m.detail.repoID)
	if !ok {
		return []string{"Ce dépôt n'est plus suivi. échap pour revenir à la liste."}
	}
	now, loc, uni := m.now(), m.deps.Location, m.deps.Unicode
	res := m.evaluate(r)
	field := func(name, value string) string {
		return "  " + m.st.Muted.Render(pad(name, 18, false, "")) + value
	}

	title := clean(r.DisplayName())
	if r.Label != "" {
		title += m.st.Muted.Render(" (" + clean(r.FullName()) + ")")
	}
	lines := []string{m.st.Title.Render(title) + m.st.Muted.Render("  échap pour revenir")}
	if r.URL != "" {
		lines = append(lines, field("Adresse", clean(r.URL)))
	}

	state := m.st.state(res.State).Render(res.State.Label())
	if s := res.Short(uni); !res.State.Gray() && s != "-" {
		state += " (" + s + ")"
	}
	own := ""
	if r.FlemmeAfterDays != nil || r.DeadAfterDays != nil {
		own = " (propres au dépôt)"
	}
	state += m.st.Muted.Render(fmt.Sprintf("  seuils : Flemme après %d j, Dead après %d j%s",
		res.Thresholds.FlemmeAfterDays, res.Thresholds.DeadAfterDays, own))
	lines = append(lines, field("État", state))

	last := activity.Relative(r.LastCommitAt, now, loc, uni)
	if r.LastCommit.SHA != "" {
		last += " par " + clean(r.LastCommit.AuthorDisplayName()) + " (" + shortSHA(r.LastCommit.SHA) + ")"
	}
	lines = append(lines, field("Dernier commit", last))

	branches := plural(r.TrackedBranches, "branche suivie", "branches suivies")
	if r.DefaultBranch != "" {
		branches += ", par défaut : " + clean(r.DefaultBranch)
	}
	if len(r.BranchPatterns) > 0 {
		branches += ", motifs : " + clean(strings.Join(r.BranchPatterns, ", "))
	} else {
		branches += ", toutes les branches"
	}
	lines = append(lines, field("Branches", branches))
	lines = append(lines, field("Commits en base", strconv.Itoa(r.Commits)))
	lines = append(lines, field("Dernière synchro", m.lastRunText(r.LastRun)))

	// Dépôt jamais lu par l'API (suivi par clone Git local, sans gh, V2) : étoiles, issues, état
	// archivé, pull requests et releases sont inconnus.
	extra := "étoiles et issues non disponibles (clone Git local)"
	if r.ForgeDetails() {
		extra = fmt.Sprintf("%s, %s", plural(r.Stars, "étoile", "étoiles"),
			plural(r.OpenIssues, "issue ouverte", "issues ouvertes"))
	}
	if r.Archived {
		extra += ", archivé"
	}
	if !r.Enabled {
		extra += ", suivi arrêté"
	}
	lines = append(lines, field("Sur GitHub", extra))
	pulls := "non disponibles (clone Git local, sans gh)"
	if r.ForgeDetails() {
		pulls = m.pullsText(r.OpenPullRequests, r.LatestRelease)
	}
	lines = append(lines, field("Pull requests", pulls))

	lines = append(lines, "", m.st.Header.Render("Derniers commits"))
	switch {
	case m.detail.err != nil:
		lines = append(lines, "  "+m.st.Error.Render("lecture impossible : "+clean(m.detail.err.Error())))
	case m.detail.loading:
		lines = append(lines, "  Chargement"+m.ellipsis())
	case len(m.detail.commits) == 0:
		lines = append(lines, "  Aucun commit.")
	default:
		cols := []column{{title: "QUAND"}, {title: "AUTEUR", max: 18}, {title: "SHA"}, {title: "MESSAGE", flex: true}}
		rows := make([]row, 0, len(m.detail.commits))
		for _, c := range m.detail.commits {
			rows = append(rows, row{cells: []string{activity.Relative(c.CommittedAt, now, loc, uni),
				clean(c.AuthorDisplayName()), shortSHA(c.SHA), firstLine(c.Message)}, stateCol: -1})
		}
		lines = append(lines, m.renderTable(cols, rows, -1, 0, len(rows), false)...)
	}

	if m.detail.err == nil && !m.detail.loading && len(m.detail.contribs) > 0 {
		weeks := len(m.detail.weekly.Weeks)
		lines = append(lines, "", m.st.Header.Render("Principaux contributeurs")+
			m.st.Muted.Render(fmt.Sprintf("  activité sur %d semaines, la plus récente à droite", weeks)))
		cols := []column{{title: "CONTRIBUTEUR", flex: true}, {title: "COMMITS", right: true},
			{title: fmt.Sprintf("%d SEM.", weeks), right: true}, {title: "PAR SEMAINE"}, {title: "DERNIER COMMIT"}}
		rows := make([]row, 0, len(m.detail.contribs))
		for _, c := range m.detail.contribs {
			rows = append(rows, row{cells: []string{clean(c.DisplayName()), strconv.Itoa(c.Commits),
				strconv.Itoa(m.detail.weekly.TotalOf(c.ID)), activity.Sparkline(m.detail.weekly.Of(c.ID), uni),
				activity.Relative(c.LastCommitAt, now, loc, uni)}, stateCol: -1})
		}
		lines = append(lines, m.renderTable(cols, rows, -1, 0, len(rows), false)...)
	}
	return lines
}

// pullsText décrit les pull requests ouvertes et la dernière release publiée d'un dépôt (V2) :
// « 2 ouvertes, dernière release v1.0 (il y a 3 j) ».
func (m Model) pullsText(open int, latest model.Release) string {
	text := plural(open, "ouverte", "ouvertes")
	if open == 0 {
		text = "aucune ouverte"
	}
	if latest.ProviderID == 0 {
		return text + ", aucune release"
	}
	return text + ", dernière release " + clean(latest.TagName) + " (" +
		activity.Relative(latest.Date(), m.now(), m.deps.Location, m.deps.Unicode) + ")"
}

// lastRunText décrit le dernier sync_run d'un dépôt.
func (m Model) lastRunText(run model.SyncRun) string {
	if run.ID == 0 {
		return "jamais"
	}
	when := activity.Relative(run.StartedAt, m.now(), m.deps.Location, m.deps.Unicode)
	switch run.Status {
	case model.SyncRunning:
		return when + ", en cours"
	case model.SyncError:
		msg := "erreur"
		if run.Error != "" {
			msg += " : " + clean(run.Error)
		}
		return when + ", " + m.st.Error.Render(msg)
	default:
		return when + ", réussie (" + plural(run.NewCommits, "nouveau commit", "nouveaux commits") + ")"
	}
}
