package tui

// En-tête du TUI : le hibou mini (internal/logo) aux pupilles vertes et, à sa droite, le titre, les
// onglets et l'état de la synchro.

import (
	"fmt"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/logo"
)

// headerHeight est la hauteur de l'en-tête (celle du hibou mini), en lignes.
const headerHeight = 3

// owlGap est l'espace entre le hibou et le texte à sa droite.
const owlGap = "  "

// sym renvoie le symbole Unicode u, ou sa version ASCII a sur un terminal sans Unicode.
func (m Model) sym(u, a string) string {
	if m.deps.Unicode {
		return u
	}
	return a
}

// ellipsis renvoie la marque de texte coupé.
func (m Model) ellipsis() string {
	return m.sym("…", ".")
}

// renderHeader renvoie les lignes de l'en-tête.
func (m Model) renderHeader() []string {
	variant, pupil := logo.Mini, "●"
	if !m.deps.Unicode {
		variant, pupil = logo.MiniPure, "o"
	}
	owl := variant.Lines()
	right := []string{m.titleLine(), m.tabsLine(), m.syncLine()}
	lines := make([]string, headerHeight)
	for i := range headerHeight {
		var o string
		if i < len(owl) {
			o = owl[i]
		}
		o = pad(o, variant.Width(), false, "")
		o = strings.ReplaceAll(o, pupil, m.st.Pupil.Render(pupil))
		lines[i] = o + owlGap + right[i]
	}
	return lines
}

// titleLine renvoie « Watchoo v0.1.0 · nom de l'espace ».
func (m Model) titleLine() string {
	s := m.st.Title.Render("Watchoo")
	if v := displayVersion(m.deps.Version); v != "" {
		s += " " + m.st.Muted.Render(clean(v))
	}
	if name := clean(m.deps.Config.Name); name != "" {
		s += m.st.Muted.Render(" "+m.sym("·", "-")+" ") + name
	}
	return s
}

// displayVersion préfixe de « v » un numéro de version (0.1.0), comme la carte d'en-tête de la CLI.
func displayVersion(v string) string {
	if v != "" && v[0] >= '0' && v[0] <= '9' && strings.Contains(v, ".") {
		return "v" + v
	}
	return v
}

// tabsLine renvoie les onglets, la vue courante mise en avant (entre crochets sans couleur).
func (m Model) tabsLine() string {
	parts := make([]string, 0, viewCount)
	for v := range viewCount {
		label := fmt.Sprintf("%d %s", v+1, v.title())
		if v == m.view {
			if m.deps.Color {
				parts = append(parts, m.st.TabActive.Render(" "+label+" "))
			} else {
				parts = append(parts, "["+label+"]")
			}
			continue
		}
		parts = append(parts, m.st.Tab.Render(" "+label+" "))
	}
	return strings.Join(parts, " ")
}

// syncLine renvoie l'état de la synchro : en cours (progression), sinon la dernière et la prochaine.
func (m Model) syncLine() string {
	now := m.now()
	if m.sync.running {
		s := "Synchro en cours"
		if m.sync.auto {
			s = "Synchro automatique en cours"
		}
		if m.sync.total > 0 {
			s += fmt.Sprintf(" : %d/%d dépôts", m.sync.done, m.sync.total)
		}
		if m.sync.current != "" {
			s += " (" + m.sync.current + ")"
		}
		return s + m.ellipsis()
	}
	var parts []string
	if !m.sync.lastAt.IsZero() {
		parts = append(parts, "dernière synchro "+activity.Relative(m.sync.lastAt, now, m.deps.Location, m.deps.Unicode))
	}
	if m.deps.Interval > 0 && !m.nextAuto.IsZero() {
		parts = append(parts, fmt.Sprintf("synchro auto toutes les %s, prochaine à %s",
			formatInterval(m.deps.Interval), m.nextAuto.In(m.deps.Location).Format("15:04")))
	} else {
		parts = append(parts, "synchro manuelle (s)")
	}
	s := strings.Join(parts, " "+m.sym("·", "-")+" ")
	return m.st.Muted.Render(strings.ToUpper(s[:1]) + s[1:])
}

// formatInterval écrit un intervalle en minutes ou en heures (« 30 min », « 2 h », « 1 h 30 »).
func formatInterval(d time.Duration) string {
	total := int(d.Minutes())
	h, mins := total/60, total%60
	switch {
	case h == 0:
		return fmt.Sprintf("%d min", mins)
	case mins == 0:
		return fmt.Sprintf("%d h", h)
	default:
		return fmt.Sprintf("%d h %02d", h, mins)
	}
}
