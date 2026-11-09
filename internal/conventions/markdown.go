package conventions

// Analyse Markdown minimale des fichiers lus dans le clone (phase 8.3) : titres ATX, puces, liens
// [texte](cible) et définitions [réf]: cible, tableaux GFM, nombre de mots. Les blocs de code clôturés
// (trois accents graves ou trois tildes au moins) et les commentaires HTML sont ignorés. Sans dépendance :
// pas de rendu, seulement ce que les critères CL, RD, DOC et TL lisent. Le texte reste en mémoire.

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// mdKind est la nature d'une ligne hors code.
type mdKind int

const (
	mdBlank   mdKind = iota // ligne vide
	mdHeading               // titre ATX
	mdBullet                // élément de liste (puce ou numéro)
	mdText                  // tout autre texte (paragraphe, ligne de tableau, citation…)
	mdLinkDef               // définition de lien « [réf]: cible »
	mdBreak                 // séparateur thématique (---, ***, ___)
)

// mdLine est une ligne hors bloc de code.
type mdLine struct {
	Kind   mdKind
	Level  int    // niveau d'un titre (1 à 6)
	Text   string // texte du titre (sans # ni séquence de fermeture), contenu de la puce, ligne brute sinon
	Indent int    // retrait de la puce, en colonnes (tabulation = 4)
}

// mdLink est un lien d'un document.
type mdLink struct {
	Target string // cible telle qu'écrite, sans chevrons ni titre
	Image  bool   // ![alt](cible)
	Def    bool   // définition « [réf]: cible »
}

// mdTable est un tableau GFM : la ligne d'en-tête et les lignes de données, cellules sans les « | » de bord.
type mdTable struct {
	Header []string
	Rows   [][]string
}

// mdDoc est un document analysé.
type mdDoc struct {
	Lines  []mdLine
	Links  []mdLink
	Tables []mdTable
	Words  int // mots hors code (blocs et code en ligne), hors cibles de liens et balises HTML
}

var (
	mdFenceRe   = regexp.MustCompile("^ {0,3}(`{3,}|~{3,})(.*)$")
	mdBulletRe  = regexp.MustCompile(`^([ \t]*)([-*+]|[0-9]{1,9}[.)])(?:[ \t]+(.*))?$`)
	mdBreakRe   = regexp.MustCompile(`^ {0,3}(?:(?:\*[ \t]*){3,}|(?:-[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	mdLinkDefRe = regexp.MustCompile(`^ {0,3}\[([^\]]+)\]:[ \t]*(<[^<>]*>|\S+)`)
	// mdLinkRe reconnaît [texte](cible "titre") et ![alt](cible) ; le texte admet un niveau de crochets.
	mdLinkRe  = regexp.MustCompile(`(!?)\[((?:[^\[\]]|\[[^\[\]]*\])*)\]\(\s*(<[^<>\n]*>|[^\s()]*(?:\([^\s()]*\)[^\s()]*)*)(?:\s+(?:"[^"]*"|'[^']*'|\([^)]*\)))?\s*\)`)
	mdDelimRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)*\|?\s*$`)
	mdHTMLRe  = regexp.MustCompile(`<[^<>]*>`)
	// mdTargetRe retire la cible d'un lien en ligne, pour ne compter que le texte.
	mdTargetRe = regexp.MustCompile(`\]\([^)]*\)`)
)

// parseMarkdown analyse src (UTF-8 ; les octets invalides sont remplacés).
func parseMarkdown(src []byte) mdDoc {
	text := string(src)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "\uFFFD")
	}
	text = strings.TrimPrefix(text, "\uFEFF")
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var doc mdDoc
	var fence string // clôture ouverte (caractère répété), vide hors bloc
	inComment := false
	for i := 0; i < len(raw); i++ {
		line := strings.TrimRight(raw[i], " \t\r")
		if fence != "" {
			if isFenceClose(line, fence) {
				fence = ""
			}
			continue
		}
		// Un commentaire HTML est reconnu avant les clôtures : une ligne « ``` » dans
		// <!-- … --> n'ouvre pas de bloc de code.
		if inComment {
			if strings.Contains(line, "-->") {
				inComment = false
			}
			continue
		}
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "<!--") {
			if !strings.Contains(t[4:], "-->") {
				inComment = true
			}
			continue
		}
		if m := mdFenceRe.FindStringSubmatch(line); m != nil {
			if m[1][0] != '`' || !strings.Contains(m[2], "`") {
				fence = m[1]
				continue
			}
		}
		// Tableau GFM : une ligne avec « | » suivie d'une ligne de délimitation.
		if strings.Contains(line, "|") && i+1 < len(raw) && isTableDelimiter(raw[i+1], len(splitRow(line))) {
			tbl := mdTable{Header: splitRow(line)}
			doc.addText(line)
			i++
			for i+1 < len(raw) {
				next := strings.TrimRight(raw[i+1], " \t\r")
				if strings.TrimSpace(next) == "" || !strings.Contains(next, "|") || mdFenceRe.MatchString(next) {
					break
				}
				i++
				tbl.Rows = append(tbl.Rows, splitRow(next))
				doc.addText(next)
			}
			doc.Tables = append(doc.Tables, tbl)
			continue
		}
		doc.addLine(line)
	}
	return doc
}

// isFenceClose indique une ligne qui ferme le bloc ouvert par fence.
func isFenceClose(line, fence string) bool {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 {
		return false
	}
	n := 0
	for n < len(t) && t[n] == fence[0] {
		n++
	}
	return n >= len(fence) && strings.TrimSpace(t[n:]) == ""
}

// isTableDelimiter reconnaît la ligne « |---|:--:| » d'un tableau de cells colonnes (même nombre de cellules
// que l'en-tête, comme GFM).
func isTableDelimiter(line string, cells int) bool {
	line = strings.TrimRight(line, " \t\r")
	return strings.Contains(line, "-") && mdDelimRe.MatchString(line) && len(splitRow(line)) == cells
}

// splitRow découpe une ligne de tableau en cellules (« \| » reste dans la cellule).
func splitRow(line string) []string {
	const esc = "\x00"
	line = strings.ReplaceAll(strings.TrimSpace(line), `\|`, esc)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	cells := strings.Split(line, "|")
	for i, c := range cells {
		cells[i] = strings.TrimSpace(strings.ReplaceAll(c, esc, "|"))
	}
	return cells
}

// addText ajoute une ligne de texte (ligne de tableau comprise) : liens et mots.
func (d *mdDoc) addText(line string) {
	d.Lines = append(d.Lines, mdLine{Kind: mdText, Text: line})
	d.scan(line)
}

// addLine classe une ligne hors code et en relève les liens et les mots.
func (d *mdDoc) addLine(line string) {
	switch {
	case strings.TrimSpace(line) == "":
		d.Lines = append(d.Lines, mdLine{Kind: mdBlank})
		return
	case mdBreakRe.MatchString(line):
		d.Lines = append(d.Lines, mdLine{Kind: mdBreak})
		return
	}
	if lvl, txt, ok := atxHeading(line); ok {
		d.Lines = append(d.Lines, mdLine{Kind: mdHeading, Level: lvl, Text: txt})
		d.scan(txt)
		return
	}
	if m := mdLinkDefRe.FindStringSubmatch(line); m != nil {
		d.Lines = append(d.Lines, mdLine{Kind: mdLinkDef, Text: line})
		d.Links = append(d.Links, mdLink{Target: unbracket(m[2]), Def: true})
		return
	}
	if m := mdBulletRe.FindStringSubmatch(line); m != nil {
		d.Lines = append(d.Lines, mdLine{Kind: mdBullet, Text: strings.TrimSpace(m[3]), Indent: indentWidth(m[1])})
		d.scan(m[3])
		return
	}
	d.Lines = append(d.Lines, mdLine{Kind: mdText, Text: line})
	d.scan(line)
}

// scan relève les liens d'un fragment de texte et compte ses mots, hors code en ligne.
func (d *mdDoc) scan(s string) {
	s = stripCodeSpans(s)
	for _, m := range mdLinkRe.FindAllStringSubmatch(s, -1) {
		d.Links = append(d.Links, mdLink{Target: unbracket(m[3]), Image: m[1] == "!"})
	}
	d.Words += countWords(mdHTMLRe.ReplaceAllString(mdTargetRe.ReplaceAllString(s, "]"), " "))
}

// linksIn renvoie les liens (hors images) d'un fragment de texte : sert aux cellules de l'INDEX.
func linksIn(s string) []string {
	var out []string
	for _, m := range mdLinkRe.FindAllStringSubmatch(stripCodeSpans(s), -1) {
		if m[1] != "!" {
			out = append(out, unbracket(m[3]))
		}
	}
	return out
}

// atxHeading lit un titre ATX : au plus trois espaces, 1 à 6 « # », puis une espace ou la fin de ligne.
func atxHeading(line string) (level int, text string, ok bool) {
	t := strings.TrimLeft(line, " ")
	if len(line)-len(t) > 3 || !strings.HasPrefix(t, "#") {
		return 0, "", false
	}
	for level < len(t) && t[level] == '#' {
		level++
	}
	if level > 6 {
		return 0, "", false
	}
	rest := t[level:]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	rest = strings.TrimSpace(rest)
	// Séquence de fermeture : des « # » précédés d'une espace (ou le titre entier).
	if trimmed := strings.TrimRight(rest, "#"); trimmed != rest {
		if trimmed == "" {
			rest = ""
		} else if strings.HasSuffix(trimmed, " ") || strings.HasSuffix(trimmed, "\t") {
			rest = strings.TrimSpace(trimmed)
		}
	}
	return level, rest, true
}

// unbracket retire les chevrons d'une cible « <…> ».
func unbracket(s string) string {
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		return s[1 : len(s)-1]
	}
	return s
}

// indentWidth mesure un retrait en colonnes (tabulation = 4).
func indentWidth(s string) int {
	n := 0
	for _, r := range s {
		if r == '\t' {
			n += 4
		} else {
			n++
		}
	}
	return n
}

// stripCodeSpans retire le code en ligne (`…`, “…“) : une suite d'accents graves se ferme par une suite
// de même longueur ; sans fermeture, les accents restent du texte.
func stripCodeSpans(s string) string {
	if !strings.Contains(s, "`") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] != '`' {
			b.WriteByte(s[i])
			i++
			continue
		}
		n := 0
		for i+n < len(s) && s[i+n] == '`' {
			n++
		}
		end := findRun(s, i+n, n)
		if end < 0 {
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		b.WriteByte(' ')
		i = end + n
	}
	return b.String()
}

// findRun cherche, à partir de from, une suite d'exactement n accents graves.
func findRun(s string, from, n int) int {
	for i := from; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		if j-i == n {
			return i
		}
		i = j
	}
	return -1
}

// countWords compte les mots : suites de lettres ou de chiffres.
func countWords(s string) int {
	return len(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
}

// foldAccents remplace les lettres accentuées latines courantes par leur lettre de base.
var foldAccents = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
	"ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ì", "i",
	"ô", "o", "ö", "o", "ó", "o", "ò", "o", "õ", "o",
	"ù", "u", "û", "u", "ü", "u", "ú", "u", "ÿ", "y", "ñ", "n",
	"œ", "oe", "æ", "ae", "ß", "ss",
)

// normalizeHeading normalise un titre pour le comparer à des synonymes : minuscules, sans accent, toute suite
// de signes (ponctuation, émoji, espaces) réduite à une espace, numérotation de tête retirée (« 2. »,
// « 1) », « 3 - »).
func normalizeHeading(s string) string {
	fields := strings.FieldsFunc(foldAccents.Replace(strings.ToLower(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for len(fields) > 0 && isDigits(fields[0]) {
		fields = fields[1:]
	}
	return strings.Join(fields, " ")
}

// normalizeText normalise un texte libre (minuscules, sans accent, signes réduits à une espace), sans
// retirer de numérotation.
func normalizeText(s string) string {
	return strings.Join(strings.FieldsFunc(foldAccents.Replace(strings.ToLower(s)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), " ")
}

// containsPhrase indique que le texte normalisé contient la phrase normalisée, mot pour mot.
func containsPhrase(text, phrase string) bool {
	if phrase == "" {
		return false
	}
	return strings.Contains(" "+text+" ", " "+phrase+" ")
}
