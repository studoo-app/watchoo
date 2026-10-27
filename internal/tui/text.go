package tui

// Outils de texte : nettoyage des données affichées, largeur, troncature et alignement.

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/studoo-app/watchoo/internal/textclean"
)

// clean rend affichable un texte venu de la base (nom de dépôt, de personne, message de commit) :
// voir textclean.Clean, partagé avec la sortie texte de la CLI.
func clean(s string) string {
	return textclean.Clean(s)
}

// firstLine renvoie la première ligne non vide de s (titre d'un message de commit), nettoyée.
func firstLine(s string) string {
	for line := range strings.Lines(s) {
		if t := strings.TrimSpace(line); t != "" {
			return clean(t)
		}
	}
	return ""
}

// width renvoie la largeur affichée de s, en colonnes (s ne contient pas de séquence ANSI).
func width(s string) int {
	return lipgloss.Width(s)
}

// truncate coupe s à w colonnes au plus, en terminant par ellipsis s'il a fallu couper.
func truncate(s string, w int, ellipsis string) string {
	if w <= 0 {
		return ""
	}
	if width(s) <= w {
		return s
	}
	ew := width(ellipsis)
	if ew >= w {
		ellipsis, ew = "", 0
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		rw := width(string(r))
		if used+rw > w-ew {
			break
		}
		b.WriteRune(r)
		used += rw
	}
	b.WriteString(ellipsis)
	return b.String()
}

// pad complète s par des espaces jusqu'à w colonnes (à gauche si right), après l'avoir tronqué.
func pad(s string, w int, right bool, ellipsis string) string {
	s = truncate(s, w, ellipsis)
	fill := strings.Repeat(" ", max(w-width(s), 0))
	if right {
		return fill + s
	}
	return s + fill
}

// shortSHA renvoie les 7 premiers caractères d'un SHA.
func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}
