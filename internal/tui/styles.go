package tui

// Styles lipgloss du TUI. Sans couleur (Deps.Color faux : NO_COLOR, TERM=dumb), tous les styles sont
// vides et l'écran ne contient aucune séquence d'échappement : la ligne sélectionnée reste repérable
// par son marqueur « › » (« > » sans Unicode).

import (
	"charm.land/lipgloss/v2"

	"github.com/studoo-app/watchoo/internal/activity"
)

// styles regroupe les styles du TUI.
type styles struct {
	Title     lipgloss.Style // « Watchoo », en gras
	Muted     lipgloss.Style // version, textes secondaires, états gris
	Pupil     lipgloss.Style // pupilles du hibou, en vert
	TabActive lipgloss.Style // onglet de la vue courante
	Tab       lipgloss.Style // autres onglets
	Header    lipgloss.Style // en-têtes de colonnes
	Selected  lipgloss.Style // ligne sélectionnée
	Active    lipgloss.Style // état Actif, en vert
	Flemme    lipgloss.Style // état Flemme, en orange
	Dead      lipgloss.Style // état Dead, en rouge
	Error     lipgloss.Style // messages d'erreur
	OK        lipgloss.Style // messages de réussite
	Key       lipgloss.Style // touches citées dans l'aide
	Notice    lipgloss.Style // avis de nouvelle version, en jaune
}

// newStyles renvoie les styles, colorés seulement si color est vrai.
func newStyles(color bool) styles {
	if !color {
		return styles{} // styles vides : texte brut
	}
	base := lipgloss.NewStyle()
	gray := lipgloss.Color("8")
	return styles{
		Title:     base.Bold(true),
		Muted:     base.Foreground(gray),
		Pupil:     base.Foreground(lipgloss.Color("2")),
		TabActive: base.Bold(true).Reverse(true),
		Tab:       base.Foreground(gray),
		Header:    base.Bold(true),
		Selected:  base.Reverse(true),
		Active:    base.Foreground(lipgloss.Color("2")),
		Flemme:    base.Foreground(lipgloss.Color("208")),
		Dead:      base.Foreground(lipgloss.Color("1")).Bold(true),
		Error:     base.Foreground(lipgloss.Color("1")),
		OK:        base.Foreground(lipgloss.Color("2")),
		Key:       base.Foreground(lipgloss.Color("6")),
		Notice:    base.Foreground(lipgloss.Color("3")),
	}
}

// state renvoie le style d'un état d'activité : vert Actif, orange Flemme, rouge Dead, gris sinon.
func (s styles) state(st activity.State) lipgloss.Style {
	switch st {
	case activity.StateActive:
		return s.Active
	case activity.StateFlemme:
		return s.Flemme
	case activity.StateDead:
		return s.Dead
	default:
		return s.Muted
	}
}
