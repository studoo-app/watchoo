package logo

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Largeurs de la carte d'en-tête, en colonnes.
const (
	// MaxCardWidth est la largeur maximale de la carte ; c'est aussi la largeur supposée quand celle du
	// terminal est inconnue.
	MaxCardWidth = 80
	// MinCardWidth est la plus petite largeur disponible pour laquelle une carte est dessinée : en
	// dessous, Card renvoie une chaîne vide.
	MinCardWidth = 40
)

// Mise en page de la carte.
const (
	cardPadding = 2 // espaces entre la bordure et le contenu, de chaque côté
	owlGap      = 2 // espaces entre le hibou et le texte à sa droite
)

// Séquences ANSI de la carte et des lignes d'état. Chacune est refermée par ansiReset.
const (
	ansiBold  = "\x1b[1m"
	ansiGray  = "\x1b[90m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
	ansiReset = "\x1b[0m"
)

// CardOptions décrit une carte d'en-tête et le terminal où elle s'affiche.
type CardOptions struct {
	Title    string // titre, en gras, à droite de la première ligne du hibou (ex. « Watchoo »)
	Version  string // version, en gris, après le titre ; « v » ajouté devant un numéro (0.1.0) ; vide : aucune
	Subtitle string // sous-titre, en gris, à droite de la deuxième ligne du hibou ; vide : aucun
	Tip      string // astuce, sous le hibou, après une ligne vide ; vide : aucune
	Width    int    // largeur du terminal en colonnes ; 0 (ou moins) : inconnue, MaxCardWidth supposée
	Unicode  bool   // le terminal affiche l'Unicode : bordures arrondies et hibou en blocs ; sinon ASCII pur
	Color    bool   // couleurs ANSI ; sinon aucune séquence d'échappement
}

// Card renvoie la carte d'en-tête décrite par o, prête à afficher : chaque ligne suivie d'un « \n ».
//
// La carte est un cadre qui contient, à gauche, le hibou mini (Mini, ou MiniPure sans Unicode) et, à
// sa droite, le titre et la version (première ligne du hibou), puis le sous-titre (deuxième ligne) ;
// la troisième ligne est vide à droite. S'il y a une astuce, elle suit, après une ligne vide. La
// largeur est celle du contenu, plus deux espaces de marge de chaque côté et les bordures, sans
// dépasser la largeur disponible : min(o.Width, MaxCardWidth). Un texte trop long est coupé sur les
// espaces (jamais après « ni avant », :, ?, ! et ;) et continue sur les lignes suivantes. Toutes les
// lignes ont la même largeur affichée, comptée en caractères (runes) et non en octets, séquences ANSI
// non comprises ; les marques combinantes (l'accent de « e » + U+0301, forme décomposée NFD fréquente
// dans les noms de répertoires sous macOS) ne comptent pas : elles s'affichent dans la colonne du
// caractère qu'elles accentuent. Si la largeur disponible est inférieure à MinCardWidth, Card renvoie
// une chaîne vide.
//
// Les caractères de contrôle des textes sont remplacés (une espace pour les sauts de ligne et
// tabulations, « ? » pour les autres), pour qu'aucun texte ne puisse déformer la carte ni envoyer de
// séquence au terminal. Sans o.Unicode, la carte est entièrement en ASCII : bordures « + », « - » et
// « | », hibou MiniPure, guillemets « » remplacés par des guillemets droits, accents retirés (qu'ils
// soient précomposés ou décomposés).
//
// Avec o.Color : bordure en gris, titre en gras, version et sous-titre en gris, pupilles du hibou en
// vert, et dans l'astuce la commande citée (le texte entre « » ou entre guillemets droits) en cyan.
// Sans o.Color, la carte ne contient aucune séquence d'échappement.
func Card(o CardOptions) string {
	avail := o.Width
	if avail <= 0 || avail > MaxCardWidth {
		avail = MaxCardWidth
	}
	if avail < MinCardWidth {
		return ""
	}

	owl, pupil := drawings[Mini], '●'
	topLeft, topRight, bottomLeft, bottomRight, horizontal, vertical := "╭", "╮", "╰", "╯", "─", "│"
	if !o.Unicode {
		owl, pupil = drawings[MiniPure], 'o'
		topLeft, topRight, bottomLeft, bottomRight, horizontal, vertical = "+", "+", "+", "+", "-", "|"
	}

	var heading, subtitle, tip styled
	heading.add(clean(o.Title, o.Unicode), bold)
	if version := clean(displayVersion(o.Version), o.Unicode); version != "" {
		if heading.width() > 0 {
			heading.add(" ", plain)
		}
		heading.add(version, gray)
	}
	subtitle.add(clean(o.Subtitle, o.Unicode), gray)
	tip.add(clean(o.Tip, o.Unicode), plain)
	markQuoted(tip)

	// Largeur du contenu, entre les marges : celle du plus long texte, sans dépasser la place disponible.
	left := owl.width + owlGap
	inner := max(left+max(heading.width(), subtitle.width()), tip.width())
	inner = min(inner, avail-2-2*cardPadding)
	right := wrap(heading, inner-left)
	right = append(right, wrap(subtitle, inner-left)...)

	var b strings.Builder
	paint := func(s, code string) string {
		if o.Color && s != "" {
			return code + s + ansiReset
		}
		return s
	}
	margin := strings.Repeat(" ", cardPadding)
	// row écrit une ligne de la carte dont le contenu, content, occupe width colonnes.
	row := func(content string, width int) {
		b.WriteString(paint(vertical, ansiGray))
		b.WriteString(margin)
		b.WriteString(content)
		b.WriteString(strings.Repeat(" ", inner-width))
		b.WriteString(margin)
		b.WriteString(paint(vertical, ansiGray))
		b.WriteByte('\n')
	}

	b.WriteString(paint(topLeft+strings.Repeat(horizontal, inner+2*cardPadding)+topRight, ansiGray))
	b.WriteByte('\n')
	for i := range max(len(owl.lines), len(right)) {
		var line strings.Builder
		if i < len(owl.lines) {
			line.WriteString(colorPupils(owl.lines[i], pupil, o.Color))
			line.WriteString(strings.Repeat(" ", left-displayWidth(owl.lines[i])))
		} else {
			line.WriteString(strings.Repeat(" ", left))
		}
		width := left
		if i < len(right) {
			line.WriteString(right[i].render(o.Color))
			width += right[i].width()
		}
		row(line.String(), width)
	}
	if tipLines := wrap(tip, inner); len(tipLines) > 0 {
		row("", 0)
		for _, l := range tipLines {
			row(l.render(o.Color), l.width())
		}
	}
	b.WriteString(paint(bottomLeft+strings.Repeat(horizontal, inner+2*cardPadding)+bottomRight, ansiGray))
	b.WriteByte('\n')
	return b.String()
}

// Status renvoie une ligne d'état, à afficher sous la carte : « ● text » (« * text » sans o.Unicode),
// suivie d'un « \n ». Seuls o.Unicode et o.Color comptent. Le texte est nettoyé comme ceux de la
// carte (caractères de contrôle remplacés, et sans Unicode, ASCII pur). Avec o.Color, la puce et le
// texte sont en gris.
func Status(text string, o CardOptions) string {
	bullet := "●"
	if !o.Unicode {
		bullet = "*"
	}
	line := bullet + " " + clean(text, o.Unicode)
	if o.Color {
		line = ansiGray + line + ansiReset
	}
	return line + "\n"
}

// displayVersion renvoie la version telle que la carte l'affiche : préfixée de « v » si c'est un numéro
// (des chiffres puis un point : « 0.1.0 » devient « v0.1.0 »), telle quelle sinon (« v0.1.0 », « dev »,
// ou un commit court comme « 07e0d1b-dirty », qui commence par un chiffre sans être un numéro).
func displayVersion(v string) string {
	digits := 0
	for digits < len(v) && v[digits] >= '0' && v[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits < len(v) && v[digits] == '.' {
		return "v" + v
	}
	return v
}

// colorPupils entoure de vert chaque pupille (le caractère pupil) d'une ligne du hibou, si color.
func colorPupils(line string, pupil rune, color bool) string {
	if !color {
		return line
	}
	p := string(pupil)
	return strings.ReplaceAll(line, p, ansiGreen+p+ansiReset)
}

// isMark indique si r est une marque combinante sans chasse (catégories Unicode Mn et Me, comme l'accent
// aigu U+0301 d'un « é » décomposé) : le terminal la dessine dans la colonne du caractère qui la précède.
// Les caractères de format (Cf : marques de sens d'écriture, espaces de largeur nulle…) ne sont pas
// concernés : clean les remplace par « ? ».
func isMark(r rune) bool {
	return unicode.In(r, unicode.Mn, unicode.Me)
}

// displayWidth renvoie la largeur affichée de s, en colonnes : son nombre de caractères (runes, pas
// d'octets), marques combinantes non comprises (voir isMark). s ne doit pas contenir de séquence ANSI.
func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		if !isMark(r) {
			n++
		}
	}
	return n
}

// style est l'attribut d'affichage d'un caractère de la carte.
type style uint8

const (
	plain style = iota // attributs par défaut du terminal
	bold               // titre
	gray               // version, sous-titre
	cyan               // commande citée dans l'astuce
)

// code renvoie la séquence ANSI qui active le style (chaîne vide pour plain).
func (s style) code() string {
	switch s {
	case bold:
		return ansiBold
	case gray:
		return ansiGray
	case cyan:
		return ansiCyan
	}
	return ""
}

// styled est un texte découpé en colonnes : chaque colonne est un caractère de base (runes[i]), suivi
// des marques combinantes qui l'accentuent (marks[i], souvent vide), avec son style (styles[i]). Sa
// largeur est son nombre de colonnes ; une coupure de ligne ne sépare jamais un caractère de ses
// marques.
type styled struct {
	runes  []rune
	marks  []string
	styles []style
}

// add ajoute s au texte, avec le style st. Une marque combinante rejoint la colonne du caractère qui la
// précède ; en début de texte, sans caractère à accentuer, elle est ignorée (clean les retire déjà).
func (t *styled) add(s string, st style) {
	for _, r := range s {
		if isMark(r) {
			if n := len(t.runes); n > 0 {
				t.marks[n-1] += string(r)
			}
			continue
		}
		t.runes = append(t.runes, r)
		t.marks = append(t.marks, "")
		t.styles = append(t.styles, st)
	}
}

// width renvoie la largeur du texte, en colonnes.
func (t styled) width() int {
	return len(t.runes)
}

// slice renvoie les colonnes i à j (exclu) du texte.
func (t styled) slice(i, j int) styled {
	return styled{runes: t.runes[i:j], marks: t.marks[i:j], styles: t.styles[i:j]}
}

// render renvoie le texte, avec, si color, une séquence ANSI au début de chaque suite de caractères
// d'un même style (autre que plain), refermée par ansiReset.
func (t styled) render(color bool) string {
	var b strings.Builder
	cur := plain
	for i, r := range t.runes {
		if st := t.styles[i]; color && st != cur {
			if cur != plain {
				b.WriteString(ansiReset)
			}
			b.WriteString(st.code())
			cur = st
		}
		b.WriteRune(r)
		b.WriteString(t.marks[i])
	}
	if cur != plain {
		b.WriteString(ansiReset)
	}
	return b.String()
}

// quotePairs associe à chaque guillemet ouvrant son guillemet fermant.
var quotePairs = map[rune]rune{'«': '»', '"': '"'}

// markQuoted met en cyan le texte cité entre « » ou entre guillemets droits (sans les guillemets ni les
// espaces qui les bordent) : la commande citée dans une astuce.
func markQuoted(t styled) {
	for i := 0; i < len(t.runes); i++ {
		closer, ok := quotePairs[t.runes[i]]
		if !ok {
			continue
		}
		j := slices.Index(t.runes[i+1:], closer)
		if j < 0 {
			return
		}
		j += i + 1
		start, end := i+1, j
		for start < end && unicode.IsSpace(t.runes[start]) {
			start++
		}
		for end > start && unicode.IsSpace(t.runes[end-1]) {
			end--
		}
		for k := start; k < end; k++ {
			t.styles[k] = cyan
		}
		i = j
	}
}

// wrap coupe t en lignes d'au plus width caractères. Les coupures se font sur les espaces, sauf après
// « et avant », :, ?, ! et ; (typographie française) ; un mot plus long que width est coupé là où il
// dépasse. Les espaces des coupures disparaissent : aucune ligne ne commence ni ne finit par une
// espace. Un texte vide (ou fait d'espaces) ne donne aucune ligne. Une largeur inférieure à 1 compte
// pour 1 (Card ne demande jamais moins).
func wrap(t styled, width int) []styled {
	width = max(width, 1)
	var lines []styled
	start, end := -1, -1 // ligne en cours : t.runes[start:end] ; start < 0 : aucune
	flush := func() {
		if start >= 0 {
			lines = append(lines, t.slice(start, end))
			start = -1
		}
	}
	for _, u := range breakUnits(t.runes) {
		s, e := u[0], u[1]
		for s < e && t.runes[s] == ' ' {
			s++
		}
		for e > s && t.runes[e-1] == ' ' {
			e--
		}
		if s == e {
			continue
		}
		if start >= 0 && e-start <= width {
			end = e
			continue
		}
		flush()
		// Morceau trop long : coupé tous les width caractères, sans espace au bord des morceaux.
		for e-s > width {
			cut := s + width
			chunkEnd := cut
			for chunkEnd > s && t.runes[chunkEnd-1] == ' ' {
				chunkEnd--
			}
			if chunkEnd > s {
				lines = append(lines, t.slice(s, chunkEnd))
			}
			for s = cut; s < e && t.runes[s] == ' '; s++ {
			}
		}
		if s < e {
			start, end = s, e
		}
	}
	flush()
	return lines
}

// breakUnits découpe r aux espaces où une coupure de ligne est permise, et renvoie les morceaux sous
// forme d'intervalles [début, fin) de r.
func breakUnits(r []rune) [][2]int {
	var units [][2]int
	start := 0
	for i, c := range r {
		if c != ' ' || (i > 0 && r[i-1] == '«') || (i+1 < len(r) && strings.ContainsRune("»:?!;", r[i+1])) {
			continue
		}
		units = append(units, [2]int{start, i})
		start = i + 1
	}
	return append(units, [2]int{start, len(r)})
}

// clean prépare un texte pour la carte ou une ligne d'état : les sauts de ligne, tabulations et autres
// espaces de contrôle deviennent des espaces, les autres caractères non affichables (dont ESC, qui
// commence les séquences ANSI, et les marques de sens d'écriture) deviennent « ? », les marques
// combinantes sans caractère à accentuer (en début de texte ou après une espace) sont retirées, et les
// espaces de début et de fin aussi. Sans unicodeOK, le texte est ensuite ramené à l'ASCII (voir
// toASCII).
func clean(s string, unicodeOK bool) string {
	var b strings.Builder
	prev := ' ' // dernier caractère écrit ; une espace au départ : rien à accentuer
	for _, r := range s {
		switch {
		case unicode.IsSpace(r) && unicode.IsControl(r):
			r = ' '
		case !unicode.IsGraphic(r):
			r = '?'
		case isMark(r) && unicode.IsSpace(prev):
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	s = strings.TrimSpace(b.String())
	if !unicodeOK {
		s = toASCII(s)
	}
	return s
}

// quoteSpaces retire les espaces (ordinaires ou insécables) collées aux guillemets « », que toASCII
// remplace par des guillemets droits : « « watchoo init » » devient « "watchoo init" ».
var quoteSpaces = strings.NewReplacer(
	"« ", "«", "«\u00a0", "«", "«\u202f", "«",
	" »", "»", "\u00a0»", "»", "\u202f»", "»",
)

// asciiFold donne l'équivalent ASCII des caractères non ASCII courants dans les textes de Watchoo :
// lettres accentuées, ligatures, guillemets, tirets, points de suspension, puces, espaces insécables.
var asciiFold = func() map[rune]string {
	groups := map[string]string{
		"àáâãäåā": "a", "ÀÁÂÃÄÅĀ": "A", "æ": "ae", "Æ": "AE", "çćč": "c", "ÇĆČ": "C", "ďđ": "d", "ĎĐ": "D",
		"èéêëēėęě": "e", "ÈÉÊËĒĖĘĚ": "E", "ìíîïī": "i", "ÌÍÎÏĪ": "I", "ł": "l", "Ł": "L", "ñńň": "n",
		"ÑŃŇ": "N", "òóôõöøō": "o", "ÒÓÔÕÖØŌ": "O", "œ": "oe", "Œ": "OE", "ŕř": "r", "ŔŘ": "R", "śşš": "s",
		"ŚŞŠ": "S", "ß": "ss", "ţť": "t", "ŢŤ": "T", "ùúûüūůű": "u", "ÙÚÛÜŪŮŰ": "U", "ýÿ": "y", "ÝŸ": "Y",
		"źżž": "z", "ŹŻŽ": "Z", "«»“”„″": `"`, "‘’‚′‹›": "'", "…": "...", "‐‑‒–—―−": "-", "·": "-",
		"•●": "*", "×": "x", "\u00a0\u202f\u2002\u2003\u2009": " ",
	}
	fold := make(map[rune]string)
	for from, to := range groups {
		for _, r := range from {
			fold[r] = to
		}
	}
	return fold
}()

// toASCII ramène s à l'ASCII : guillemets « » remplacés par des guillemets droits (sans les espaces qui
// les bordent à l'intérieur), accents retirés, qu'ils soient précomposés (« é ») ou décomposés (« e »
// suivi de la marque combinante U+0301, qui disparaît), et les autres caractères sans équivalent
// remplacés par « ? ».
func toASCII(s string) string {
	s = quoteSpaces.Replace(s)
	var b strings.Builder
	for _, r := range s {
		switch ascii, ok := asciiFold[r]; {
		case r < utf8.RuneSelf:
			b.WriteRune(r)
		case ok:
			b.WriteString(ascii)
		case isMark(r):
			// Accent décomposé : la lettre qui le précède est déjà écrite, sans lui.
		default:
			b.WriteByte('?')
		}
	}
	return b.String()
}
