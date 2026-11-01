package tui

// Détail d'un contributeur (0.5.0, « entrée » dans la vue Contributeurs) : nom, login, nombre
// d'identités Git (jamais les adresses), totaux (commits, lignes, dépôts, premier et dernier commit),
// activité sur activity.ContributorWeeks semaines (total et courbe d'une ligne, activity.Sparkline),
// ses dépôts (commits, lignes, courbe, dernier commit, état du dépôt) et ses derniers commits.
// « échap » revient à la liste. Les données viennent de DataSource (internal/store), les états des
// dépôts de activity.Evaluate.

import (
	"context"
	"fmt"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// contribDetailCommits est le nombre de commits affichés dans le détail d'un contributeur.
const contribDetailCommits = 15

// contribDetailState est le détail d'un contributeur ouvert par « entrée » dans la vue Contributeurs.
type contribDetailState struct {
	open    bool
	id      int64
	loading bool
	profile store.ContributorProfile
	repos   []store.ContributorRepoStats
	commits []store.CommitView
	// weekly : ses commits par semaine et par dépôt sur activity.ContributorWeeks semaines
	// (activity.CountRepoWeeks).
	weekly activity.ContributorWeekCounts
	err    error
	offset int // première ligne affichée (défilement)
}

// contribDetailMsg apporte les données du contributeur détaillé.
type contribDetailMsg struct {
	id      int64
	profile store.ContributorProfile
	repos   []store.ContributorRepoStats
	commits []store.CommitView
	weekly  activity.ContributorWeekCounts
	err     error
}

// contribDetailCmd lit la fiche, les dépôts, les derniers commits et l'activité par semaine du
// contributeur id.
func (m Model) contribDetailCmd(id int64) tea.Cmd {
	ctx, data, now, loc := m.ctx, m.deps.Data, m.now(), m.deps.Location
	return func() tea.Msg {
		msg := contribDetailMsg{id: id}
		if data == nil {
			return msg
		}
		ctx, cancel := context.WithTimeout(ctx, loadTimeout)
		defer cancel()
		if msg.profile, msg.err = data.ContributorProfile(ctx, id); msg.err != nil {
			return msg
		}
		if msg.repos, msg.err = data.ContributorRepos(ctx, id); msg.err != nil {
			return msg
		}
		if msg.commits, msg.err = data.ListRecentCommits(ctx,
			store.CommitFilter{AuthorID: id, Limit: contribDetailCommits}); msg.err != nil {
			return msg
		}
		times, err := data.ContributorCommitTimes(ctx, id, activity.WeeksStart(now, activity.ContributorWeeks, loc))
		if err != nil {
			msg.err = err
			return msg
		}
		msg.weekly = activity.CountRepoWeeks(times, now, activity.ContributorWeeks, loc)
		return msg
	}
}

// openContribDetail ouvre le détail du contributeur sélectionné (vue Contributeurs).
func (m Model) openContribDetail() (tea.Model, tea.Cmd) {
	i, ok := m.selected()
	if !ok {
		return m, nil
	}
	id := m.contribs[i].ID
	m.cdetail = contribDetailState{open: true, id: id, loading: true}
	return m, m.contribDetailCmd(id)
}

// renderContribDetail renvoie les lignes du détail du contributeur, à partir du défilement courant.
func (m Model) renderContribDetail() []string {
	lines := m.contribDetailLines()
	off := min(m.cdetail.offset, max(len(lines)-m.bodyHeight(), 0))
	return lines[off:]
}

// contribDetailLines renvoie toutes les lignes du détail du contributeur ouvert.
func (m Model) contribDetailLines() []string {
	d := m.cdetail
	back := m.st.Muted.Render(i18n.T("tui.detail.back"))
	switch {
	case d.err != nil:
		return []string{m.st.Title.Render(i18n.T("tui.cdetail.title")) + back, "",
			"  " + m.st.Error.Render(i18n.T("tui.detail.read_error", clean(d.err.Error())))}
	case d.loading:
		return []string{m.st.Title.Render(i18n.T("tui.cdetail.title")) + back, "", "  " + i18n.T("tui.loading") + m.ellipsis()}
	}
	p := d.profile
	now, loc, uni := m.now(), m.deps.Location, m.deps.Unicode
	field := func(name, value string) string {
		return "  " + m.st.Muted.Render(pad(name, 18, false, "")) + value
	}

	title := clean(p.DisplayName())
	if p.ProviderLogin != "" && p.ProviderLogin != p.DisplayName() {
		title += m.st.Muted.Render(" (@" + clean(p.ProviderLogin) + ")")
	}
	lines := []string{m.st.Title.Render(title) + back}
	account := i18n.T("tui.cdetail.no_account")
	if p.ProviderLogin != "" {
		account = "@" + clean(p.ProviderLogin)
	}
	lines = append(lines, field(i18n.T("tui.field.account"), account))
	lines = append(lines, field(i18n.T("tui.field.identities"),
		plural(p.Identities, "tui.cdetail.address_one", "tui.cdetail.address_many")+
			m.st.Muted.Render(i18n.T("tui.cdetail.not_shown"))))
	lines = append(lines, field(i18n.T("tui.field.commits"), i18n.T("tui.cdetail.commits_in",
		plural(p.Commits, "tui.count.commit_one", "tui.count.commit_many"),
		plural(p.Repos, "cli.sync.count_repo_one", "cli.sync.count_repo_many"), lineTotals(p.Lines()))))
	lines = append(lines, field(i18n.T("tui.field.first_commit"), activity.Relative(p.FirstCommitAt, now, loc, uni)))
	lines = append(lines, field(i18n.T("tui.field.last_commit"), activity.Relative(p.LastCommitAt, now, loc, uni)))
	weeks := len(d.weekly.Weeks)
	overall := make([]int, weeks)
	for _, per := range d.weekly.Counts {
		for i, n := range per {
			overall[i] += n
		}
	}
	lines = append(lines, field(i18n.T("tui.field.weeks", weeks), activity.Sparkline(overall, uni)+"  "+
		plural(d.weekly.Total, "tui.count.commit_one", "tui.count.commit_many")+
		m.st.Muted.Render(i18n.T("tui.cdetail.latest_right"))))

	lines = append(lines, "", m.st.Header.Render(i18n.T("tui.view.repos")))
	if len(d.repos) == 0 {
		lines = append(lines, "  "+i18n.T("tui.cdetail.no_commits"))
	} else {
		cols := []column{{title: i18n.T("tui.col.repo"), flex: true}, {title: i18n.T("tui.col.commits"), right: true},
			{title: "+/-", right: true}, {title: i18n.T("tui.col.weeks", weeks), right: true},
			{title: i18n.T("tui.col.per_week")}, {title: i18n.T("tui.col.last_commit")}, {title: i18n.T("tui.col.state")}}
		rows := make([]row, 0, len(d.repos))
		for _, r := range d.repos {
			state, short, gray, col := activity.State(""), "-", false, -1
			if sum, ok := m.repoByID(r.RepoID); ok {
				res := m.evaluate(sum)
				state, short, gray, col = res.State, res.Short(uni), res.State.Gray(), 6
			}
			rows = append(rows, row{cells: []string{clean(r.RepoDisplayName()), strconv.Itoa(r.Commits),
				lineTotals(r.Lines()), strconv.Itoa(d.weekly.TotalOf(r.RepoID)),
				activity.Sparkline(d.weekly.Of(r.RepoID), uni), activity.Relative(r.LastCommitAt, now, loc, uni), short},
				gray: gray, stateCol: col, state: state})
		}
		lines = append(lines, m.renderTable(cols, rows, -1, 0, len(rows), false)...)
	}

	lines = append(lines, "", m.st.Header.Render(i18n.T("tui.detail.recent_commits")))
	if len(d.commits) == 0 {
		lines = append(lines, "  "+i18n.T("tui.detail.no_commit"))
	} else {
		cols := []column{{title: i18n.T("tui.col.when")}, {title: i18n.T("tui.col.repo"), max: 24}, {title: "SHA"},
			{title: "+/-", right: true}, {title: i18n.T("tui.col.message"), flex: true}}
		rows := make([]row, 0, len(d.commits))
		for _, c := range d.commits {
			rows = append(rows, row{cells: []string{activity.Relative(c.CommittedAt, now, loc, uni),
				clean(c.RepoDisplayName()), shortSHA(c.SHA), commitLines(c.Commit), firstLine(c.Message)},
				stateCol: -1})
		}
		lines = append(lines, m.renderTable(cols, rows, -1, 0, len(rows), false)...)
	}
	lines = append(lines, "", m.st.Muted.Render(i18n.T("tui.cdetail.lines_note")))
	return lines
}

// commitLines écrit les lignes ajoutées et supprimées d'un commit : « +12/-3 » (« +0/-0 » compris)
// si elles sont connues, « - » sinon.
func commitLines(c model.Commit) string {
	if !c.StatsKnown {
		return "-"
	}
	return fmt.Sprintf("+%d/-%d", c.Additions, c.Deletions)
}

// lineTotals écrit les totaux de lignes d'un ensemble de commits : « +12/-3 », suivi de
// « (N non comptés) » si une partie des commits n'a pas encore de statistiques, ou « - » si aucun
// n'en a.
func lineTotals(t model.LineTotals) string {
	if !t.Known() {
		return "-"
	}
	s := fmt.Sprintf("+%d/-%d", t.Additions, t.Deletions)
	if n := t.Uncounted(); n > 0 {
		s += i18n.T("tui.lines.uncounted", n)
	}
	return s
}
