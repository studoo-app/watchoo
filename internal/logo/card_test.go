package logo

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// ansi reconnaît une séquence d'échappement ANSI de couleur ou d'attribut (« \x1b[32m », « \x1b[0m »).
var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// styledRun reconnaît une séquence ANSI, le texte qu'elle met en forme et la séquence qui la referme.
var styledRun = regexp.MustCompile("(\x1b\\[[0-9;]*m)([^\x1b]*)\x1b\\[0m")

// Cartes de la CLI, utilisées par plusieurs tests.
var (
	helpCard = CardOptions{Title: "Watchoo", Version: "0.1.0", Subtitle: "Qui a travaillé, sur quoi, et quand ?",
		Tip: "Astuce : « watchoo help <commande> » détaille une commande."}
	versionCard = CardOptions{Title: "Watchoo", Version: "v0.1.0", Subtitle: "go1.27.3 · darwin/arm64"}
	initCard    = CardOptions{Title: "Watchoo", Version: "dev",
		Subtitle: "Espace de supervision « licence-projet » créé",
		Tip:      "Astuce : « watchoo help » liste les commandes."}
	// trickyCard a des textes longs, sans espaces, avec des sauts de ligne, des séquences ANSI et des
	// caractères sans équivalent ASCII.
	trickyCard = CardOptions{Title: "Watchoo\n", Version: "1.2.3-rc.1+abcdef0123456789abcdef0123456789",
		Subtitle: "Espace de supervision « promo\tlicence\x1b[31m-2026-groupe-d-informatique-tres-long » créé",
		Tip: "Astuce : « watchoo help <commande> » détaille une commande ; « watchoo init » crée un espace " +
			"de supervision dans le répertoire courant, 日本語 compris\u2028, et « » vide."}
	// nfdCard a des accents décomposés (forme NFD : « e » suivi de la marque combinante U+0301), comme le
	// nom d'un répertoire créé par le Finder de macOS, et des marques sans caractère à accentuer (en
	// début de texte, après une espace).
	nfdCard = CardOptions{Title: "\u0301Watchoo", Version: "de\u0301v\u0302",
		Subtitle: "Espace de supervision « promo-re\u0301seaux » cre\u0301e\u0301",
		Tip: "Astuce : « watchoo help » \u0301liste les commandes de l'espace « promo-re\u0301seaux » " +
			"de l'e\u0301te\u0301, \u20ddsans de\u0301calage. \u0301"}
)

// columns renvoie la largeur affichée d'une ligne sans séquence ANSI : son nombre de caractères, sans
// les marques combinantes (Mn, Me), que le terminal dessine dans la colonne du caractère précédent.
func columns(line string) int {
	n := 0
	for _, r := range line {
		if !unicode.In(r, unicode.Mn, unicode.Me) {
			n++
		}
	}
	return n
}

// plainLines renvoie les lignes de la carte, sans les séquences ANSI.
func plainLines(card string) []string {
	return strings.Split(strings.TrimSuffix(ansi.ReplaceAllString(card, ""), "\n"), "\n")
}

func ExampleCard() {
	fmt.Print(Card(CardOptions{
		Title:    "Watchoo",
		Version:  "0.1.0",
		Subtitle: "Qui a travaillé, sur quoi, et quand ?",
		Tip:      "Astuce : « watchoo help <commande> » détaille une commande.",
		Width:    60,
		Unicode:  true,
	}))
	// Output:
	// ╭──────────────────────────────────────────────────────────╮
	// │  █▄    ▄█  Watchoo v0.1.0                                │
	// │  █ ●██● █  Qui a travaillé, sur quoi, et quand ?         │
	// │  ▀██▙▟██▀                                                │
	// │                                                          │
	// │  Astuce : « watchoo help <commande> » détaille une       │
	// │  commande.                                               │
	// ╰──────────────────────────────────────────────────────────╯
}

func ExampleCard_ascii() {
	fmt.Print(Card(CardOptions{
		Title:    "Watchoo",
		Version:  "dev",
		Subtitle: "Qui a travaillé, sur quoi, et quand ?",
		Tip:      "Astuce : « watchoo init » fait de ce répertoire un espace de supervision.",
	}))
	// Output:
	// +---------------------------------------------------------------------------+
	// |  |\    /|  Watchoo dev                                                    |
	// |  | o##o |  Qui a travaille, sur quoi, et quand ?                          |
	// |  \##\/##/                                                                 |
	// |                                                                           |
	// |  Astuce : "watchoo init" fait de ce repertoire un espace de supervision.  |
	// +---------------------------------------------------------------------------+
}

// Sans astuce, la carte s'arrête après le hibou, sans ligne vide ; sa largeur est celle du contenu.
func ExampleCard_withoutTip() {
	fmt.Print(Card(CardOptions{Title: "Watchoo", Version: "v0.1.0", Subtitle: "go1.27.3 · darwin/arm64",
		Width: 120, Unicode: true}))
	// Output:
	// ╭─────────────────────────────────────╮
	// │  █▄    ▄█  Watchoo v0.1.0           │
	// │  █ ●██● █  go1.27.3 · darwin/arm64  │
	// │  ▀██▙▟██▀                           │
	// ╰─────────────────────────────────────╯
}

// Quelles que soient la largeur et les options, toutes les lignes ont la même largeur (en caractères,
// séquences ANSI non comprises), qui ne dépasse ni la largeur du terminal ni 80 colonnes ; sous 40
// colonnes, pas de carte. Sans Unicode, la carte est en ASCII pur ; sans couleur, elle n'a aucune
// séquence d'échappement.
func TestCardWidth(t *testing.T) {
	for _, base := range []CardOptions{helpCard, versionCard, initCard, trickyCard, nfdCard, {}} {
		for width := -1; width <= 130; width++ {
			for _, unicodeOK := range []bool{false, true} {
				for _, color := range []bool{false, true} {
					o := base
					o.Width, o.Unicode, o.Color = width, unicodeOK, color
					card := Card(o)
					avail := width
					if avail <= 0 || avail > 80 {
						avail = 80
					}
					if avail < 40 {
						if card != "" {
							t.Errorf("Card(%+v) : carte sur %d colonnes, attendu aucune :\n%s", o, width, card)
						}
						continue
					}
					checkCard(t, o, card, avail)
				}
			}
		}
	}
}

// checkCard vérifie la forme de card, rendue avec o sur avail colonnes disponibles.
func checkCard(t *testing.T, o CardOptions, card string, avail int) {
	t.Helper()
	if !strings.HasSuffix(card, "\n") {
		t.Errorf("Card(%+v) ne finit pas par une fin de ligne", o)
		return
	}
	lines := plainLines(card)
	w := columns(lines[0])
	if w > avail {
		t.Errorf("Card(%+v) : %d colonnes, plus que les %d disponibles :\n%s", o, w, avail, card)
	}
	corners, side := []string{"╭", "╮", "╰", "╯"}, "│"
	if !o.Unicode {
		corners, side = []string{"+", "+", "+", "+"}, "|"
	}
	last := lines[len(lines)-1]
	if !strings.HasPrefix(lines[0], corners[0]) || !strings.HasSuffix(lines[0], corners[1]) ||
		!strings.HasPrefix(last, corners[2]) || !strings.HasSuffix(last, corners[3]) {
		t.Errorf("Card(%+v) : coins inattendus :\n%s", o, card)
	}
	if len(lines) < 5 {
		t.Errorf("Card(%+v) : %d lignes, attendu au moins 5 (bordures et hibou)", o, len(lines))
	}
	for i, line := range lines {
		if n := columns(line); n != w {
			t.Errorf("Card(%+v) : ligne %d de %d colonnes, attendu %d comme la première :\n%s", o, i+1, n, w, card)
		}
		if i > 0 && i < len(lines)-1 && (!strings.HasPrefix(line, side+"  ") || !strings.HasSuffix(line, "  "+side)) {
			t.Errorf("Card(%+v) : ligne %d sans bordure ni marges : %q", o, i+1, line)
		}
	}
	if !o.Unicode && !isASCII(card) {
		t.Errorf("Card(%+v) n'est pas en ASCII pur :\n%s", o, card)
	}
	// Une marque combinante suit toujours, dans la même colonne, un caractère qu'elle accentue : jamais
	// une bordure, une marge ou une espace.
	var prev rune
	for _, r := range ansi.ReplaceAllString(card, "") {
		if unicode.In(r, unicode.Mn, unicode.Me) && (unicode.IsSpace(prev) || strings.ContainsRune("│|", prev)) {
			t.Errorf("Card(%+v) : marque combinante %q après %q :\n%s", o, r, prev, card)
		}
		prev = r
	}
	if !o.Color && strings.Contains(card, "\x1b") {
		t.Errorf("Card(%+v) contient une séquence d'échappement sans couleur :\n%q", o, card)
	}
	// Seules les séquences ajoutées par Card restent : aucun caractère de contrôle des textes.
	for _, r := range ansi.ReplaceAllString(card, "") {
		if r < ' ' && r != '\n' || r == 0x7f {
			t.Errorf("Card(%+v) : caractère de contrôle %q", o, r)
		}
	}
}

// La largeur suit le contenu : une astuce longue élargit la carte jusqu'à 80 colonnes au plus, puis
// est coupée sur les espaces.
func TestCardWidthFollowsContent(t *testing.T) {
	o := versionCard
	o.Unicode = true
	if got := columns(plainLines(Card(o))[0]); got != 39 {
		t.Errorf("carte de version : %d colonnes, attendu 39 (contenu, marges et bordures)", got)
	}
	o.Tip = strings.Repeat("astuce ", 30)
	for _, width := range []int{0, 80, 81, 200} {
		o.Width = width
		if got := columns(plainLines(Card(o))[0]); got != 80 {
			t.Errorf("Width %d, astuce longue : %d colonnes, attendu 80", width, got)
		}
	}
	o.Width = 50
	if got := columns(plainLines(Card(o))[0]); got != 50 {
		t.Errorf("Width 50, astuce longue : %d colonnes, attendu 50", got)
	}
}

// L'astuce est coupée sur les espaces, jamais après « ni avant » ou « : » ; aucune ligne ne commence
// ni ne finit par une espace ; le texte n'est ni perdu ni répété.
func TestCardTipWrapping(t *testing.T) {
	for width := 40; width <= 80; width++ {
		o := helpCard
		o.Width, o.Unicode = width, true
		lines := plainLines(Card(o))
		var tip []string
		for _, line := range lines[5 : len(lines)-1] { // après les bordures, le hibou et la ligne vide
			content := strings.TrimSuffix(strings.TrimPrefix(line, "│  "), "  │")
			text := strings.TrimRight(content, " ")
			if text == "" || strings.HasPrefix(text, " ") {
				t.Errorf("Width %d : ligne d'astuce vide ou qui commence par une espace : %q", width, line)
			}
			if strings.HasSuffix(text, "«") || strings.HasPrefix(text, "»") || strings.HasPrefix(text, ":") {
				t.Errorf("Width %d : coupure interdite autour de %q", width, text)
			}
			tip = append(tip, text)
		}
		if got := strings.Join(tip, " "); got != o.Tip {
			t.Errorf("Width %d : astuce %q, attendu %q", width, got, o.Tip)
		}
	}
}

// Les textes trop longs à droite du hibou continuent sur les lignes suivantes, sous le texte et non
// sous le hibou ; les mots plus longs que la place disponible sont coupés.
func TestCardWrapsRightColumn(t *testing.T) {
	o := CardOptions{Title: "Watchoo", Version: "dev", Unicode: true, Width: 40,
		Subtitle: "Espace de supervision « promotion-licence-informatique » créé"}
	got := Card(o)
	want := strings.Join([]string{
		"╭──────────────────────────────────────╮",
		"│  █▄    ▄█  Watchoo dev               │",
		"│  █ ●██● █  Espace de supervision     │",
		"│  ▀██▙▟██▀  « promotion-licence-info  │",
		"│            rmatique » créé           │",
		"╰──────────────────────────────────────╯",
	}, "\n") + "\n"
	if got != want {
		t.Errorf("Card =\n%s\nattendu\n%s", got, want)
	}
}

// Un texte aux accents décomposés (NFD, « e » + U+0301 : noms de répertoires créés par le Finder) donne la
// même carte que le même texte aux accents précomposés (NFC, « é ») : les accents ne décalent pas la
// bordure de droite ; sans Unicode, ils sont retirés (« promo-reseaux »), et non remplacés par « ? ».
func TestCardDecomposedAccents(t *testing.T) {
	nfc := CardOptions{Title: "Watchoo", Version: "dev", Subtitle: "Espace de supervision « promo-réseaux » créé",
		Tip: "Astuce : « watchoo help » liste les commandes de l'espace « promo-réseaux » de l'été."}
	decompose := strings.NewReplacer("é", "e\u0301")
	nfd := nfc
	nfd.Subtitle, nfd.Tip = decompose.Replace(nfc.Subtitle), decompose.Replace(nfc.Tip)
	if nfd.Subtitle == nfc.Subtitle || !strings.Contains(nfd.Subtitle, "re\u0301seaux") {
		t.Fatalf("sous-titre décomposé : %q", nfd.Subtitle)
	}
	for width := 40; width <= 100; width++ {
		for _, unicodeOK := range []bool{true, false} {
			for _, color := range []bool{false, true} {
				nfc.Width, nfc.Unicode, nfc.Color = width, unicodeOK, color
				nfd.Width, nfd.Unicode, nfd.Color = width, unicodeOK, color
				want, got := Card(nfc), Card(nfd)
				if unicodeOK {
					got = strings.ReplaceAll(got, "e\u0301", "é")
				}
				if got != want {
					t.Errorf("Width %d, Unicode %t, Color %t : carte\n%s\nattendu, comme avec des accents "+
						"précomposés,\n%s", width, unicodeOK, color, Card(nfd), want)
				}
			}
		}
	}
	nfd.Width, nfd.Unicode, nfd.Color = 80, false, false
	if card := Card(nfd); !strings.Contains(card, "promo-reseaux") || strings.Contains(card, "?") {
		t.Errorf("sans Unicode : accents décomposés mal retirés :\n%s", card)
	}
}

// Les sauts de ligne, tabulations et séquences d'échappement des textes ne déforment pas la carte et
// n'atteignent pas le terminal.
func TestCardCleansText(t *testing.T) {
	o := CardOptions{Title: "Wat\nchoo", Version: "dev\x1b[31m", Subtitle: "a\tb\r\nc\u202ed", Unicode: true}
	got := plainLines(Card(o))
	if !strings.Contains(got[1], "Wat choo dev?[31m") || !strings.Contains(got[2], "a b  c?d") {
		t.Errorf("textes mal nettoyés :\n%s", strings.Join(got, "\n"))
	}
}

func TestClean(t *testing.T) {
	for _, tt := range []struct {
		in        string
		unicodeOK bool
		want      string
	}{
		{"  Watchoo\n", true, "Watchoo"},
		{"a\tb\x1b[2J", true, "a b?[2J"},
		{"c\u202ed\u200be", true, "c?d?e"}, // caractères de format (Cf) : « ? », jamais de largeur nulle
		// Marques combinantes : gardées après un caractère, retirées sans caractère à accentuer.
		{"promo-re\u0301seaux cre\u0301e\u0301\u0302", true, "promo-re\u0301seaux cre\u0301e\u0301\u0302"},
		{"\u0301Watchoo", true, "Watchoo"},
		{"a \u0301b \u20dd", true, "a b"},
		{"\t\u0301\u0302x", true, "x"},
		{"\x1b\u0301", true, "?\u0301"},
		{"promo-re\u0301seaux \u0301x", false, "promo-reseaux x"},
		{"« Réseaux »\n", false, `"Reseaux"`},
	} {
		if got := clean(tt.in, tt.unicodeOK); got != tt.want {
			t.Errorf("clean(%q, %t) = %q, attendu %q", tt.in, tt.unicodeOK, got, tt.want)
		}
	}
}

// Une marque combinante rejoint la colonne du caractère qui la précède, quel que soit son style ; sans
// caractère à accentuer, elle est ignorée.
func TestStyledMarks(t *testing.T) {
	var s styled
	s.add("\u0301e\u0301t", plain)
	s.add("\u0302", bold)
	s.add("e\u0301", gray)
	if s.width() != 3 || !slices.Equal(s.runes, []rune("ete")) ||
		!slices.Equal(s.marks, []string{"\u0301", "\u0302", "\u0301"}) {
		t.Errorf("styled = %q / %q, largeur %d, attendu 3 colonnes « é », « t̂ », « é »", string(s.runes), s.marks,
			s.width())
	}
	if got, want := s.render(false), "e\u0301t\u0302e\u0301"; got != want {
		t.Errorf("render(false) = %q, attendu %q", got, want)
	}
	if got, want := s.render(true), "e\u0301t\u0302"+ansiGray+"e\u0301"+ansiReset; got != want {
		t.Errorf("render(true) = %q, attendu %q", got, want)
	}
	if got := s.slice(1, 3).render(false); got != "t\u0302e\u0301" {
		t.Errorf("slice(1, 3) = %q, attendu les colonnes 1 et 2 avec leurs marques", got)
	}
}

// Avec la couleur : bordures en gris, titre en gras, version et sous-titre en gris, pupilles du hibou en
// vert et commande de l'astuce en cyan, chaque séquence refermée ; sans les séquences, la carte est
// celle sans couleur.
func TestCardColors(t *testing.T) {
	for _, unicodeOK := range []bool{true, false} {
		o := helpCard
		o.Unicode, o.Width = unicodeOK, 80
		plain := Card(o)
		o.Color = true
		colored := Card(o)
		if got := ansi.ReplaceAllString(colored, ""); got != plain {
			t.Errorf("Unicode %t : carte en couleur, sans ses séquences :\n%s\nattendu\n%s", unicodeOK, got, plain)
		}
		pupil, side := "●", "│"
		if !unicodeOK {
			pupil, side = "o", "|"
		}
		runs := map[string][]string{}
		for _, m := range styledRun.FindAllStringSubmatch(colored, -1) {
			runs[m[1]] = append(runs[m[1]], m[2])
		}
		if got := runs[ansiBold]; !slices.Equal(got, []string{"Watchoo"}) {
			t.Errorf("Unicode %t : en gras %q, attendu le titre seul", unicodeOK, got)
		}
		if got := runs[ansiGreen]; !slices.Equal(got, []string{pupil, pupil}) {
			t.Errorf("Unicode %t : en vert %q, attendu les deux pupilles", unicodeOK, got)
		}
		if got := runs[ansiCyan]; !slices.Equal(got, []string{"watchoo help <commande>"}) {
			t.Errorf("Unicode %t : en cyan %q, attendu la commande de l'astuce", unicodeOK, got)
		}
		lines := plainLines(plain)
		subtitle := "Qui a travaillé, sur quoi, et quand ?"
		if !unicodeOK {
			subtitle = "Qui a travaille, sur quoi, et quand ?"
		}
		// Bordure du haut ; ligne 1 : bordure, version, bordure ; ligne 2 : bordure, sous-titre, bordure ;
		// lignes 3 à 5 (hibou, ligne vide, astuce) : bordures ; bordure du bas.
		wantGray := []string{lines[0], side, "v0.1.0", side, side, subtitle, side, side, side, side, side, side,
			side, lines[len(lines)-1]}
		if got := runs[ansiGray]; !slices.Equal(got, wantGray) {
			t.Errorf("Unicode %t : en gris\n%q\nattendu les bordures, la version et le sous-titre\n%q", unicodeOK, got,
				wantGray)
		}
		// Toute séquence ouverte est refermée avant la suivante.
		if n, closed := len(ansi.FindAllString(colored, -1)), len(styledRun.FindAllString(colored, -1)); n != 2*closed {
			t.Errorf("Unicode %t : %d séquences pour %d textes en forme, attendu %d", unicodeOK, n, closed, 2*closed)
		}
	}
}

// Sans Unicode, les « o » du texte ne sont pas pris pour des pupilles : seules celles du hibou sont en
// vert.
func TestCardColorsOnlyOwlPupils(t *testing.T) {
	colored := Card(CardOptions{Title: "oooo", Subtitle: "o o o", Tip: "ooo « o »", Color: true})
	if n := strings.Count(colored, ansiGreen); n != 2 {
		t.Errorf("%d séquences vertes, attendu 2 (les pupilles) :\n%q", n, colored)
	}
}

func TestDisplayVersion(t *testing.T) {
	for v, want := range map[string]string{
		"0.1.0":          "v0.1.0",
		"1.2.3-rc.1":     "v1.2.3-rc.1",
		"10.0":           "v10.0",
		"v0.1.0":         "v0.1.0",
		"v0.1.0-dirty":   "v0.1.0-dirty",
		"dev":            "dev",
		"":               "",
		"07e0d1b":        "07e0d1b", // commit court (git describe sans tag) : pas un numéro
		"07e0d1b-dirty":  "07e0d1b-dirty",
		"1":              "1",
		"2024a.b":        "2024a.b",
		"0.1.0-3-gabcde": "v0.1.0-3-gabcde",
	} {
		if got := displayVersion(v); got != want {
			t.Errorf("displayVersion(%q) = %q, attendu %q", v, got, want)
		}
	}
	for _, tt := range []struct{ version, want string }{{"0.1.0", "Watchoo v0.1.0"}, {"dev", "Watchoo dev"},
		{"", "Watchoo   "}} {
		lines := plainLines(Card(CardOptions{Title: "Watchoo", Version: tt.version, Subtitle: "sous-titre", Unicode: true}))
		if !strings.Contains(lines[1], "  "+tt.want) {
			t.Errorf("version %q : première ligne %q, attendu %q", tt.version, lines[1], tt.want)
		}
	}
}

func TestStatus(t *testing.T) {
	for _, tt := range []struct {
		text string
		o    CardOptions
		want string
	}{
		{"Espace : licence-projet (~/licence-projet)", CardOptions{Unicode: true},
			"● Espace : licence-projet (~/licence-projet)\n"},
		{"Espace : licence-projet", CardOptions{Unicode: true, Color: true},
			"\x1b[90m● Espace : licence-projet\x1b[0m\n"},
		{"Espace : « Réseaux » (~/réseaux)", CardOptions{}, "* Espace : \"Reseaux\" (~/reseaux)\n"},
		{"Aucun espace", CardOptions{Color: true}, "\x1b[90m* Aucun espace\x1b[0m\n"},
		{"nom\navec\x1b[2Jsauts", CardOptions{Unicode: true}, "● nom avec?[2Jsauts\n"},
		// Accents décomposés (NFD) : gardés avec Unicode, retirés sans (pas de « ? »).
		{"Espace : promo-re\u0301seaux", CardOptions{Unicode: true}, "● Espace : promo-re\u0301seaux\n"},
		{"Espace : promo-re\u0301seaux (~/promo-re\u0301seaux)", CardOptions{},
			"* Espace : promo-reseaux (~/promo-reseaux)\n"},
		{"\u0301Espace :  \u0301x", CardOptions{Unicode: true}, "● Espace :  x\n"},
	} {
		if got := Status(tt.text, tt.o); got != tt.want {
			t.Errorf("Status(%q, %+v) = %q, attendu %q", tt.text, tt.o, got, tt.want)
		}
	}
}

func TestToASCII(t *testing.T) {
	for in, want := range map[string]string{
		"Astuce : « watchoo init » fait de ce répertoire un espace.": `Astuce : "watchoo init" fait de ce repertoire un espace.`,
		"«\u00a0insécables\u202f»":                                   `"insecables"`,
		"go1.27.3 · darwin/arm64":                                    "go1.27.3 - darwin/arm64",
		"Œuvre, cœur, À côté, ÇA, ß":                                 "OEuvre, coeur, A cote, CA, ss",
		"l’été… — “citation”":                                        `l'ete... - "citation"`,
		"日本語 ✓":                                                      "??? ?",
		"promo-re\u0301seaux, cre\u0301e\u0301":                      "promo-reseaux, cree",
		"e\u0301\u0302\u20dd, É\u0301, 日\u0301":                      "e, E, ?",
		"ASCII seul : rien ne change.":                               "ASCII seul : rien ne change.",
	} {
		if got := toASCII(in); got != want {
			t.Errorf("toASCII(%q) = %q, attendu %q", in, got, want)
		}
		if !isASCII(toASCII(in)) {
			t.Errorf("toASCII(%q) n'est pas en ASCII", in)
		}
	}
}

func TestWrap(t *testing.T) {
	for _, tt := range []struct {
		text  string
		width int
		want  []string
	}{
		{"", 10, nil},
		{"   ", 10, nil},
		{"un deux trois", 20, []string{"un deux trois"}},
		{"un deux trois", 7, []string{"un deux", "trois"}},
		{"un deux trois", 6, []string{"un", "deux", "trois"}},
		{"  un   deux  ", 20, []string{"un   deux"}},
		{"abcdefghij", 4, []string{"abcd", "efgh", "ij"}},
		// Largeur nulle ou négative : un caractère par ligne, sans boucler.
		{"ab c", 0, []string{"a", "b", "c"}},
		{"ab", -3, []string{"a", "b"}},
		{"ab abcdefghij cd", 4, []string{"ab", "abcd", "efgh", "ij", "cd"}},
		{"voir « watchoo init » : oui", 12, []string{"voir", "« watchoo", "init » : oui"}},
		{"Qui ? Quand ! Où ; quoi", 7, []string{"Qui ?", "Quand !", "Où ;", "quoi"}},
		// Un morceau insécable trop long est coupé, sans espace au bord des lignes.
		{"Quand ! Où", 6, []string{"Quand", "! Où"}},
		// Les largeurs se comptent en colonnes : un accent décomposé ne compte pas et reste avec sa lettre.
		{"e\u0301te\u0301 e\u0301te\u0301", 7, []string{"e\u0301te\u0301 e\u0301te\u0301"}},
		{"e\u0301te\u0301 e\u0301te\u0301", 6, []string{"e\u0301te\u0301", "e\u0301te\u0301"}},
		{"e\u0301e\u0301e\u0301e\u0301e\u0301", 2, []string{"e\u0301e\u0301", "e\u0301e\u0301", "e\u0301"}},
	} {
		var s styled
		s.add(tt.text, plain)
		var got []string
		for _, l := range wrap(s, tt.width) {
			got = append(got, l.render(false))
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("wrap(%q, %d) = %q, attendu %q", tt.text, tt.width, got, tt.want)
		}
	}
}
