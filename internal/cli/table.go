package cli

// Sortie texte en tableau (lipgloss/table), commune aux commandes.

import (
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"

	"github.com/studoo-app/watchoo/internal/textclean"
)

// Couleurs de la sortie texte (palette ANSI de base, comme la carte d'en-tête).
var (
	colorGray = lipgloss.BrightBlack
)

// textStyle décide de la mise en forme de la sortie texte : couleurs seulement sur un terminal qui
// les permet (TermInfo.Color, donc sans NO_COLOR ni TERM=dumb), Unicode selon TermInfo.Unicode
// (sinon symboles et accents ASCII, comme la carte). Une sortie redirigée n'a ni couleur ni carte.
type textStyle struct {
	term TermInfo
}

// outputStyle renvoie le style de la sortie standard (deps.Terminal()) ; en --json, valeur zéro.
func outputStyle(deps Deps, opts *options) textStyle {
	if opts.json {
		return textStyle{}
	}
	return textStyle{term: deps.Terminal()}
}

// color indique si les couleurs ANSI sont permises : terminal qui les accepte.
func (st textStyle) color() bool {
	return st.term.IsTerminal && st.term.Color
}

// unicode indique si les symboles Unicode (« ⚠ », « … », accents des en-têtes) peuvent être écrits :
// sur un terminal, selon sa locale ; hors terminal (fichier, tube), toujours, la sortie étant en UTF-8.
func (st textStyle) unicode() bool {
	return !st.term.IsTerminal || st.term.Unicode
}

// pick renvoie withAccents si l'Unicode est permis, sinon ascii.
func (st textStyle) pick(withAccents, ascii string) string {
	if st.unicode() {
		return withAccents
	}
	return ascii
}

// ellipsis renvoie le signe d'un texte coupé : « … », ou « ... » sans Unicode.
func (st textStyle) ellipsis() string {
	return st.pick("…", "...")
}

// gray met s en gris si les couleurs sont permises.
func (st textStyle) gray(s string) string {
	if !st.color() || s == "" {
		return s
	}
	return lipgloss.NewStyle().Foreground(colorGray).Render(s)
}

// bold met s en gras si les couleurs sont permises.
func (st textStyle) bold(s string) string {
	if !st.color() || s == "" {
		return s
	}
	return lipgloss.NewStyle().Bold(true).Render(s)
}

// renderTable renvoie un tableau texte : une ligne d'en-têtes (headers, en majuscules), puis une ligne
// par élément de rows, colonnes alignées et séparées par au moins deux espaces, sans bordure (comme
// l'exemple de « watchoo repo list » dans documentation/fr/05-cli.md), terminé par une fin de ligne.
// gray, si non nil, indique les lignes à afficher en gris (états hors activité) quand la couleur est
// permise.
//
// Chaque cellule est nettoyée (textclean.Clean) : messages de commit, noms Git et labels viennent en
// partie des dépôts des étudiants et ne doivent envoyer aucune séquence d'échappement au terminal.
//
// Le tableau est d'abord calculé sans aucun style (largeurs exactes, accents compris), les espaces de
// fin de ligne retirés ; les couleurs sont ensuite appliquées ligne par ligne : en-têtes en gras,
// lignes grises en gris.
func renderTable(st textStyle, headers []string, rows [][]string, gray func(row int) bool) string {
	upper := make([]string, len(headers))
	for i, h := range headers {
		upper[i] = strings.ToUpper(textclean.Clean(h))
	}
	cleaned := make([][]string, len(rows))
	for i, row := range rows {
		cleaned[i] = make([]string, len(row))
		for j, cell := range row {
			cleaned[i][j] = textclean.Clean(cell)
		}
	}
	rows = cleaned
	last := len(upper) - 1
	t := table.New().
		Headers(upper...).
		Rows(rows...).
		Border(lipgloss.HiddenBorder()).
		BorderTop(false).BorderBottom(false).BorderLeft(false).BorderRight(false).
		BorderHeader(false).BorderColumn(false).
		Wrap(false).
		StyleFunc(func(_, col int) lipgloss.Style {
			if col == last {
				return lipgloss.NewStyle()
			}
			return lipgloss.NewStyle().PaddingRight(2)
		})
	lines := strings.Split(t.String(), "\n")
	var b strings.Builder
	for i, line := range lines {
		line = strings.TrimRight(line, " ")
		switch {
		case i == 0:
			line = st.bold(line)
		case gray != nil && gray(i-1):
			line = st.gray(line)
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// truncate coupe s à max colonnes au plus (signe de coupure compris), sur les caractères ; s est
// d'abord réduit à sa première ligne et nettoyé (textclean.Clean).
func (st textStyle) truncate(s string, maxWidth int) string {
	s = strings.TrimSpace(textclean.Clean(firstLine(strings.TrimSpace(s))))
	if maxWidth <= 0 || lipgloss.Width(s) <= maxWidth {
		return s
	}
	ell := st.ellipsis()
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+lipgloss.Width(ell) > maxWidth {
		runes = runes[:len(runes)-1]
	}
	return strings.TrimRight(string(runes), " ") + ell
}
