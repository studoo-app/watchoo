package tui

// Rendu de l'écran : en-tête, liste de la vue courante (dépôts, commits, contributeurs), détail d'un
// dépôt ou d'un contributeur, ou aide, puis bas de l'écran (message, filtre, rappel des touches).

import (
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/store"
)

// footerHeight est la hauteur du bas de l'écran : message (ou filtre) et rappel des touches.
const footerHeight = 2

// View rend l'écran (écran alternatif).
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.WindowTitle = "Watchoo"
	return v
}

// size renvoie la taille de l'écran (valeurs par défaut avant le premier tea.WindowSizeMsg).
func (m Model) size() (w, h int) {
	w, h = m.width, m.height
	if w <= 0 {
		w = defaultWidth
	}
	if h <= 0 {
		h = defaultHeight
	}
	return w, h
}

// bodyHeight renvoie le nombre de lignes entre l'en-tête (suivi d'une ligne vide) et le bas.
func (m Model) bodyHeight() int {
	_, h := m.size()
	return max(h-headerHeight-1-footerHeight, 2)
}

// listHeight renvoie le nombre de lignes de données d'une liste (sous la ligne des en-têtes).
func (m Model) listHeight() int {
	return max(m.bodyHeight()-1, 1)
}

// render renvoie le texte de l'écran.
func (m Model) render() string {
	w, _ := m.size()
	lines := m.renderHeader()
	lines = append(lines, "")
	var body []string
	switch {
	case m.showHelp:
		body = m.renderHelp()
	case m.detail.open:
		body = m.renderDetail()
	case m.cdetail.open:
		body = m.renderContribDetail()
	default:
		body = m.renderList()
	}
	h := m.bodyHeight()
	if len(body) > h {
		body = body[:h]
	}
	for len(body) < h {
		body = append(body, "")
	}
	lines = append(lines, body...)
	lines = append(lines, m.renderFooter(w)...)
	return strings.Join(lines, "\n")
}

// renderFooter renvoie le message (ou la saisie du filtre) et le rappel des touches.
func (m Model) renderFooter(w int) []string {
	var msg string
	ls := m.lists[m.view]
	switch {
	case m.filtering:
		msg = "Filtre : " + ls.filter + m.sym("▏", "_") + m.st.Muted.Render("  (entrée pour garder, échap pour effacer)")
	case m.status != "":
		text := truncate(m.status, w, m.ellipsis())
		if m.statusErr {
			msg = m.st.Error.Render(text)
		} else {
			msg = text
		}
	case ls.filter != "" && !m.detail.open && !m.cdetail.open && !m.showHelp:
		msg = m.st.Muted.Render(truncate("Filtre : "+ls.filter+" (échap pour l'effacer)", w, m.ellipsis()))
	}
	var keys []string
	for _, h := range m.keys.shortHelp() {
		keys = append(keys, m.st.Key.Render(h[0])+" "+h[1])
	}
	help := strings.Join(keys, m.st.Muted.Render(" "+m.sym("·", "|")+" "))
	if lipgloss.Width(help) > w {
		help = m.st.Key.Render("?") + " aide " + m.st.Muted.Render(m.sym("·", "|")) + " " + m.st.Key.Render("q") + " quitter"
	}
	return []string{msg, help}
}

// column est une colonne d'une liste.
type column struct {
	title string
	max   int  // largeur maximale ; 0 = sans limite
	flex  bool // prend la place restante
	right bool // alignée à droite (nombres)
}

// row est une ligne d'une liste.
type row struct {
	cells    []string
	gray     bool           // ligne grisée (dépôt hors activité)
	stateCol int            // indice de la colonne colorée selon l'état ; -1 = aucune
	state    activity.State // état de la ligne (stateCol >= 0)
}

// renderList renvoie la liste de la vue courante, ou un message si elle est vide.
func (m Model) renderList() []string {
	if !m.loaded {
		if m.loadErr != nil {
			return []string{m.st.Error.Render("Impossible de lire la base. « r » pour réessayer.")}
		}
		return []string{"Chargement" + m.ellipsis()}
	}
	cols, rows := m.tableData(m.view)
	if len(rows) == 0 {
		return []string{m.emptyMessage()}
	}
	ls := m.lists[m.view]
	return m.renderTable(cols, rows, ls.cursor, ls.offset, m.listHeight(), true)
}

// emptyMessage renvoie le message d'une liste vide.
func (m Model) emptyMessage() string {
	if m.lists[m.view].filter != "" {
		return "Aucune ligne ne correspond au filtre « " + m.lists[m.view].filter + " » (échap pour l'effacer)."
	}
	switch m.view {
	case viewRepos:
		return "Aucun dépôt suivi. Ajoutez-en avec « watchoo repo add owner/repo », puis « s » pour synchroniser."
	case viewCommits:
		return "Aucun commit en base. « s » lance une synchro."
	default:
		return "Aucun contributeur en base. « s » lance une synchro."
	}
}

// tableData renvoie les colonnes et les lignes (filtrées) de la vue v.
func (m Model) tableData(v view) ([]column, []row) {
	now, loc, uni := m.now(), m.deps.Location, m.deps.Unicode
	vis := m.visible(v)
	rows := make([]row, 0, len(vis))
	switch v {
	case viewRepos:
		cols := []column{{title: "DÉPÔT", flex: true}, {title: "DERNIER COMMIT"}, {title: "AUTEUR", max: 20},
			{title: "BRANCHES", right: true}, {title: "ÉTAT"}}
		for _, i := range vis {
			r := m.repos[i]
			res := m.evaluate(r)
			author := "-"
			if r.LastCommit.SHA != "" {
				author = clean(r.LastCommit.AuthorDisplayName())
			}
			rows = append(rows, row{
				cells: []string{clean(r.DisplayName()), activity.Relative(r.LastCommitAt, now, loc, uni), author,
					strconv.Itoa(r.TrackedBranches), res.Short(uni)},
				gray: res.State.Gray(), stateCol: 4, state: res.State,
			})
		}
		return cols, rows
	case viewCommits:
		cols := []column{{title: "QUAND"}, {title: "DÉPÔT", max: 24}, {title: "AUTEUR", max: 18},
			{title: "SHA"}, {title: "+/-", right: true}, {title: "MESSAGE", flex: true}}
		for _, i := range vis {
			c := m.commits[i]
			rows = append(rows, row{
				cells: []string{activity.Relative(c.CommittedAt, now, loc, uni), clean(c.RepoDisplayName()),
					clean(c.AuthorDisplayName()), shortSHA(c.SHA), commitLines(c.Commit), firstLine(c.Message)},
				stateCol: -1,
			})
		}
		return cols, rows
	default:
		cols := []column{{title: "CONTRIBUTEUR", flex: true}, {title: "LOGIN", max: 20},
			{title: "COMMITS", right: true}, {title: "+/-", right: true}, {title: "DÉPÔTS", right: true},
			{title: "DERNIER COMMIT"}}
		for _, i := range vis {
			c := m.contribs[i]
			login := clean(c.ProviderLogin)
			if login == "" {
				login = "-"
			}
			rows = append(rows, row{
				cells: []string{clean(c.DisplayName()), login, strconv.Itoa(c.Commits),
					lineTotals(c.Lines()), strconv.Itoa(c.Repos),
					activity.Relative(c.LastCommitAt, now, loc, uni)},
				stateCol: -1,
			})
		}
		return cols, rows
	}
}

// evaluate renvoie l'état d'activité d'un dépôt (règles de internal/activity).
func (m Model) evaluate(r store.RepositorySummary) activity.Result {
	return activity.Evaluate(r.Repository, r.LastRun, m.deps.Config.Activity, m.now(), m.deps.Location)
}

// renderTable renvoie la ligne des en-têtes puis au plus height lignes à partir de offset ; la ligne
// cursor est sélectionnée si selectable.
func (m Model) renderTable(cols []column, rows []row, cursor, offset, height int, selectable bool) []string {
	w, _ := m.size()
	const gap = 2
	prefix := 2 // marqueur de sélection
	widths := make([]int, len(cols))
	flex := -1
	fixed := prefix + gap*(len(cols)-1)
	for j, c := range cols {
		cw := width(c.title)
		for _, r := range rows {
			cw = max(cw, width(r.cells[j]))
		}
		if c.max > 0 {
			cw = min(cw, max(c.max, width(c.title)))
		}
		widths[j] = cw
		if c.flex && flex < 0 {
			flex = j
			continue
		}
		fixed += cw
	}
	if flex >= 0 {
		widths[flex] = min(widths[flex], max(w-fixed, width(cols[flex].title), 8))
	}

	ell := m.ellipsis()
	cellsLine := func(cells []string, style func(j int, s string) string) string {
		parts := make([]string, len(cols))
		for j, c := range cols {
			s := pad(cells[j], widths[j], c.right, ell)
			if j == len(cols)-1 && !c.right {
				s = strings.TrimRight(s, " ")
			}
			parts[j] = style(j, s)
		}
		return strings.Join(parts, strings.Repeat(" ", gap))
	}
	titles := make([]string, len(cols))
	for j, c := range cols {
		titles[j] = c.title
	}
	out := []string{strings.Repeat(" ", prefix) + cellsLine(titles, func(_ int, s string) string {
		return m.st.Header.Render(s)
	})}
	end := min(offset+height, len(rows))
	for i := offset; i < end; i++ {
		r := rows[i]
		sel := selectable && i == cursor
		if sel {
			plain := cellsLine(r.cells, func(_ int, s string) string { return s })
			out = append(out, m.sym("›", ">")+" "+m.st.Selected.Render(plain))
			continue
		}
		out = append(out, "  "+cellsLine(r.cells, func(j int, s string) string {
			switch {
			case j == r.stateCol:
				return m.st.state(r.state).Render(s)
			case r.gray:
				return m.st.Muted.Render(s)
			}
			return s
		}))
	}
	if len(rows) > height && end < len(rows) {
		// Indique qu'il reste des lignes sous l'écran, sur la dernière ligne affichée, si la place le
		// permet.
		last := out[len(out)-1] + m.st.Muted.Render(fmt.Sprintf("  (%d/%d)", cursor+1, len(rows)))
		if lipgloss.Width(last) <= w {
			out[len(out)-1] = last
		}
	}
	return out
}

// renderHelp renvoie l'écran d'aide des touches : un groupe par ligne (ou plusieurs si la largeur
// l'impose).
func (m Model) renderHelp() []string {
	w, _ := m.size()
	out := []string{m.st.Title.Render("Aide des touches") + m.st.Muted.Render("  (« ? » ou échap pour fermer)"), ""}
	for _, g := range m.keys.helpGroups() {
		out = append(out, m.st.Header.Render(g.Title))
		line, lineW := "", 0
		for _, b := range g.Bindings {
			h := b.Help()
			item := m.st.Key.Render(h.Key) + " " + h.Desc
			itemW := width(h.Key) + 1 + width(h.Desc)
			if lineW > 0 && 2+lineW+3+itemW > w {
				out = append(out, "  "+line)
				line, lineW = "", 0
			}
			if lineW > 0 {
				line += m.st.Muted.Render(" " + m.sym("·", "|") + " ")
				lineW += 3
			}
			line += item
			lineW += itemW
		}
		if lineW > 0 {
			out = append(out, "  "+line)
		}
	}
	out = append(out, "", m.st.Muted.Render("Le filtre (« / ») porte sur les colonnes de la vue courante."))
	return out
}
