package cli

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mattn/go-isatty"
)

// TermInfo décrit la sortie standard, pour décider d'y afficher la carte d'en-tête et sous quelle forme.
type TermInfo struct {
	IsTerminal bool // la sortie est un terminal, et non un fichier ou un tube
	Width      int  // largeur du terminal en colonnes ; 0 : inconnue
	Unicode    bool // le terminal affiche l'Unicode (locale UTF-8, Windows Terminal) ; sinon ASCII pur
	Color      bool // les couleurs ANSI sont permises (NO_COLOR vide ou absente, TERM différent de dumb)
}

// DetectTerminal décrit la sortie w. Elle n'est un terminal que si w est un fichier (os.Stdout, par
// exemple) ouvert sur un terminal : un tampon, un fichier ordinaire ou un tube ne le sont jamais, et
// les autres champs restent alors à zéro. Pour un terminal :
//   - largeur : la variable COLUMNS si c'est un entier positif, sinon celle que donne le système
//     (ioctl TIOCGWINSZ sous Unix, console sous Windows), sinon 0 (inconnue) ;
//   - Unicode : si la première variable non vide parmi LC_ALL, LC_CTYPE et LANG contient « UTF-8 » ou
//     « utf8 » (casse ignorée), ou si WT_SESSION est définie (Windows Terminal) ;
//   - couleur : si NO_COLOR est vide ou absente et TERM différente de « dumb » (et, sous Windows, si la
//     console interprète les séquences ANSI).
func DetectTerminal(w io.Writer) TermInfo {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return TermInfo{}
	}
	fd := f.Fd()
	if !isatty.IsTerminal(fd) && !isatty.IsCygwinTerminal(fd) {
		return TermInfo{}
	}
	info := describeTerminal(os.LookupEnv, func() int { return systemWidth(fd) })
	if info.Color && !enableColor(fd) {
		info.Color = false
	}
	return info
}

// describeTerminal décrit un terminal à partir des variables d'environnement (lookupEnv) et de la largeur
// donnée par le système (systemWidth, qui renvoie 0 si elle est inconnue et n'est appelée que si
// COLUMNS ne donne pas la largeur). Voir DetectTerminal pour les règles.
func describeTerminal(lookupEnv func(string) (string, bool), systemWidth func() int) TermInfo {
	return TermInfo{
		IsTerminal: true,
		Width:      terminalWidth(lookupEnv, systemWidth),
		Unicode:    unicodeTerminal(lookupEnv),
		Color:      colorAllowed(lookupEnv),
	}
}

// terminalWidth renvoie la largeur du terminal : COLUMNS si c'est un entier positif, sinon celle du
// système si elle est positive, sinon 0.
func terminalWidth(lookupEnv func(string) (string, bool), systemWidth func() int) int {
	if s, _ := lookupEnv("COLUMNS"); s != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n > 0 {
			return n
		}
	}
	if n := systemWidth(); n > 0 {
		return n
	}
	return 0
}

// unicodeTerminal indique si le terminal affiche l'Unicode : locale UTF-8 (première variable non vide
// parmi LC_ALL, LC_CTYPE et LANG, comme le fait la bibliothèque C) ou Windows Terminal (WT_SESSION
// définie).
func unicodeTerminal(lookupEnv func(string) (string, bool)) bool {
	if _, ok := lookupEnv("WT_SESSION"); ok {
		return true
	}
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if locale, _ := lookupEnv(name); locale != "" {
			locale = strings.ToLower(locale)
			return strings.Contains(locale, "utf-8") || strings.Contains(locale, "utf8")
		}
	}
	return false
}

// colorAllowed indique si les couleurs sont permises : NO_COLOR (https://no-color.org/) vide ou absente,
// et TERM différente de « dumb ».
func colorAllowed(lookupEnv func(string) (string, bool)) bool {
	if v, _ := lookupEnv("NO_COLOR"); v != "" {
		return false
	}
	term, _ := lookupEnv("TERM")
	return term != "dumb"
}
