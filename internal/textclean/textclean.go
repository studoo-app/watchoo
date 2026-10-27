// Package textclean rend affichable sur un terminal un texte venu de la base ou de la forge (nom de
// dépôt, de personne, label, message de commit, message d'erreur), sans logique métier. Il est
// partagé par la sortie texte de la CLI (internal/cli) et le mode interactif (internal/tui).
//
// Ces textes viennent en partie des dépôts des étudiants : un message de commit ou un user.name Git
// peut contenir des séquences d'échappement (changement du titre de la fenêtre, effacement de
// l'écran, liens OSC 8, presse-papiers OSC 52) ou des marques de sens d'écriture qui maquillent un
// nom. La sortie JSON n'en a pas besoin : encoding/json échappe déjà les caractères de contrôle.
package textclean

import (
	"strings"
	"unicode"
)

// Clean retire de s les séquences d'échappement ANSI (CSI « ESC [ … », OSC « ESC ] … » terminée par
// BEL ou ESC \, et les autres séquences ESC à un caractère) et remplace les autres caractères de
// contrôle (sauts de ligne, BEL, CSI C1 0x9b…) et de format (Unicode Cf : marques de sens d'écriture,
// caractères invisibles) par une espace. Un texte sans tel caractère est renvoyé tel quel.
func Clean(s string) string {
	if !strings.ContainsFunc(s, unsafe) {
		return s
	}
	var b strings.Builder
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case r == 0x1b:
			i = skipEscape(rs, i)
		case unsafe(r):
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CleanLines est Clean en gardant les sauts de ligne (« \n ») et les tabulations : pour un texte
// sur plusieurs lignes (message d'erreur qui en regroupe plusieurs). Un « \r » est remplacé par une
// espace (il ramènerait le curseur en début de ligne et masquerait le texte déjà écrit).
func CleanLines(s string) string {
	if !strings.ContainsFunc(s, func(r rune) bool { return r != '\n' && r != '\t' && unsafe(r) }) {
		return s
	}
	parts := strings.Split(s, "\n")
	for i, p := range parts {
		fields := strings.Split(p, "\t")
		for j, f := range fields {
			fields[j] = Clean(f)
		}
		parts[i] = strings.Join(fields, "\t")
	}
	return strings.Join(parts, "\n")
}

// unsafe indique si r est un caractère de contrôle (C0, DEL, C1) ou de format (Unicode Cf).
func unsafe(r rune) bool {
	return unicode.IsControl(r) || unicode.Is(unicode.Cf, r)
}

// skipEscape renvoie l'indice du dernier caractère de la séquence d'échappement qui commence en
// rs[i] (ESC) : CSI jusqu'à son octet final (0x40 à 0x7E), OSC jusqu'à BEL ou ESC \, sinon le
// caractère qui suit ESC.
func skipEscape(rs []rune, i int) int {
	if i+1 >= len(rs) {
		return i
	}
	switch rs[i+1] {
	case '[':
		for j := i + 2; j < len(rs); j++ {
			if rs[j] >= 0x40 && rs[j] <= 0x7e {
				return j
			}
		}
		return len(rs) - 1
	case ']':
		for j := i + 2; j < len(rs); j++ {
			if rs[j] == 0x07 {
				return j
			}
			if rs[j] == 0x1b && j+1 < len(rs) && rs[j+1] == '\\' {
				return j + 1
			}
		}
		return len(rs) - 1
	default:
		return i + 1
	}
}
