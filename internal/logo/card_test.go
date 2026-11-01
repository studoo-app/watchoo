package logo

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
)

// ansi matches an ANSI escape sequence of color or attribute ("\x1b[32m", "\x1b[0m").
var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

// styledRun matches an ANSI sequence, the text it styles and the sequence that closes it.
var styledRun = regexp.MustCompile("(\x1b\\[[0-9;]*m)([^\x1b]*)\x1b\\[0m")

// Cards of the CLI, used by several tests.
var (
	helpCard = CardOptions{Title: "Watchoo", Version: "0.1.0", Subtitle: "Qui a travaillé, sur quoi, et quand ?",
		Tip: "Astuce : « watchoo help <commande> » détaille une commande."}
	versionCard = CardOptions{Title: "Watchoo", Version: "v0.1.0", Subtitle: "go1.27.3 · darwin/arm64"}
	initCard    = CardOptions{Title: "Watchoo", Version: "dev",
		Subtitle: "Espace de supervision « licence-projet » créé",
		Tip:      "Astuce : « watchoo help » liste les commandes."}
	// trickyCard has long texts, without spaces, with line breaks, ANSI sequences and characters
	// without an ASCII equivalent.
	trickyCard = CardOptions{Title: "Watchoo\n", Version: "1.2.3-rc.1+abcdef0123456789abcdef0123456789",
		Subtitle: "Espace de supervision « promo\tlicence\x1b[31m-2026-groupe-d-informatique-tres-long » créé",
		Tip: "Astuce : « watchoo help <commande> » détaille une commande ; « watchoo init » crée un espace " +
			"de supervision dans le répertoire courant, 日本語 compris\u2028, et « » vide."}
	// nfdCard has decomposed accents (NFD form: "e" followed by the combining mark U+0301), like the
	// name of a directory created by the macOS Finder, and marks without a character to accent (at the
	// start of a text, after a space).
	nfdCard = CardOptions{Title: "\u0301Watchoo", Version: "de\u0301v\u0302",
		Subtitle: "Espace de supervision « promo-re\u0301seaux » cre\u0301e\u0301",
		Tip: "Astuce : « watchoo help » \u0301liste les commandes de l'espace « promo-re\u0301seaux » " +
			"de l'e\u0301te\u0301, \u20ddsans de\u0301calage. \u0301"}
)

// columns returns the displayed width of a line without ANSI sequences: its number of characters,
// without the combining marks (Mn, Me), which the terminal draws in the previous character's column.
func columns(line string) int {
	n := 0
	for _, r := range line {
		if !unicode.In(r, unicode.Mn, unicode.Me) {
			n++
		}
	}
	return n
}

// plainLines returns the lines of the card, without the ANSI sequences.
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

// Without a tip, the card stops after the owl, without an empty line; its width is the content's.
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

// Whatever the width and the options, all the lines have the same width (in characters, ANSI
// sequences excluded), which exceeds neither the terminal width nor 80 columns; under 40 columns,
// no card. Without Unicode, the card is pure ASCII; without color, it has no escape
// sequence.
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
							t.Errorf("Card(%+v): card on %d columns, want none:\n%s", o, width, card)
						}
						continue
					}
					checkCard(t, o, card, avail)
				}
			}
		}
	}
}

// checkCard checks the shape of card, rendered with o on avail available columns.
func checkCard(t *testing.T, o CardOptions, card string, avail int) {
	t.Helper()
	if !strings.HasSuffix(card, "\n") {
		t.Errorf("Card(%+v) does not end with a line break", o)
		return
	}
	lines := plainLines(card)
	w := columns(lines[0])
	if w > avail {
		t.Errorf("Card(%+v): %d columns, more than the %d available:\n%s", o, w, avail, card)
	}
	corners, side := []string{"╭", "╮", "╰", "╯"}, "│"
	if !o.Unicode {
		corners, side = []string{"+", "+", "+", "+"}, "|"
	}
	last := lines[len(lines)-1]
	if !strings.HasPrefix(lines[0], corners[0]) || !strings.HasSuffix(lines[0], corners[1]) ||
		!strings.HasPrefix(last, corners[2]) || !strings.HasSuffix(last, corners[3]) {
		t.Errorf("Card(%+v): unexpected corners:\n%s", o, card)
	}
	if len(lines) < 5 {
		t.Errorf("Card(%+v): %d lines, want at least 5 (borders and owl)", o, len(lines))
	}
	for i, line := range lines {
		if n := columns(line); n != w {
			t.Errorf("Card(%+v): line %d of %d columns, want %d like the first one:\n%s", o, i+1, n, w, card)
		}
		if i > 0 && i < len(lines)-1 && (!strings.HasPrefix(line, side+"  ") || !strings.HasSuffix(line, "  "+side)) {
			t.Errorf("Card(%+v): line %d without border or margins: %q", o, i+1, line)
		}
	}
	if !o.Unicode && !isASCII(card) {
		t.Errorf("Card(%+v) is not pure ASCII:\n%s", o, card)
	}
	// A combining mark always follows, in the same column, a character it accents: never a border, a
	// margin or a space.
	var prev rune
	for _, r := range ansi.ReplaceAllString(card, "") {
		if unicode.In(r, unicode.Mn, unicode.Me) && (unicode.IsSpace(prev) || strings.ContainsRune("│|", prev)) {
			t.Errorf("Card(%+v): combining mark %q after %q:\n%s", o, r, prev, card)
		}
		prev = r
	}
	if !o.Color && strings.Contains(card, "\x1b") {
		t.Errorf("Card(%+v) contains an escape sequence without color:\n%q", o, card)
	}
	// Only the sequences added by Card remain: no control character from the texts.
	for _, r := range ansi.ReplaceAllString(card, "") {
		if r < ' ' && r != '\n' || r == 0x7f {
			t.Errorf("Card(%+v): control character %q", o, r)
		}
	}
}

// The width follows the content: a long tip widens the card up to 80 columns at most, then is
// wrapped on the spaces.
func TestCardWidthFollowsContent(t *testing.T) {
	o := versionCard
	o.Unicode = true
	if got := columns(plainLines(Card(o))[0]); got != 39 {
		t.Errorf("version card: %d columns, want 39 (content, margins and borders)", got)
	}
	o.Tip = strings.Repeat("astuce ", 30)
	for _, width := range []int{0, 80, 81, 200} {
		o.Width = width
		if got := columns(plainLines(Card(o))[0]); got != 80 {
			t.Errorf("Width %d, long tip: %d columns, want 80", width, got)
		}
	}
	o.Width = 50
	if got := columns(plainLines(Card(o))[0]); got != 50 {
		t.Errorf("Width 50, long tip: %d columns, want 50", got)
	}
}

// The tip is wrapped on the spaces, never after "«" nor before "»" or ":"; no line starts or ends
// with a space; the text is neither lost nor repeated.
func TestCardTipWrapping(t *testing.T) {
	for width := 40; width <= 80; width++ {
		o := helpCard
		o.Width, o.Unicode = width, true
		lines := plainLines(Card(o))
		var tip []string
		for _, line := range lines[5 : len(lines)-1] { // after the borders, the owl and the empty line
			content := strings.TrimSuffix(strings.TrimPrefix(line, "│  "), "  │")
			text := strings.TrimRight(content, " ")
			if text == "" || strings.HasPrefix(text, " ") {
				t.Errorf("Width %d: empty tip line or one starting with a space: %q", width, line)
			}
			if strings.HasSuffix(text, "«") || strings.HasPrefix(text, "»") || strings.HasPrefix(text, ":") {
				t.Errorf("Width %d: forbidden break around %q", width, text)
			}
			tip = append(tip, text)
		}
		if got := strings.Join(tip, " "); got != o.Tip {
			t.Errorf("Width %d: tip %q, want %q", width, got, o.Tip)
		}
	}
}

// Texts too long to the right of the owl go on over the next lines, under the text and not under
// the owl; the words longer than the available room are cut.
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
		t.Errorf("Card =\n%s\nwant\n%s", got, want)
	}
}

// A text with decomposed accents (NFD, "e" + U+0301: names of directories created by the Finder) gives
// the same card as the same text with precomposed accents (NFC, "é"): the accents do not shift the
// right border; without Unicode, they are removed ("promo-reseaux"), not replaced with "?".
func TestCardDecomposedAccents(t *testing.T) {
	nfc := CardOptions{Title: "Watchoo", Version: "dev", Subtitle: "Espace de supervision « promo-réseaux » créé",
		Tip: "Astuce : « watchoo help » liste les commandes de l'espace « promo-réseaux » de l'été."}
	decompose := strings.NewReplacer("é", "e\u0301")
	nfd := nfc
	nfd.Subtitle, nfd.Tip = decompose.Replace(nfc.Subtitle), decompose.Replace(nfc.Tip)
	if nfd.Subtitle == nfc.Subtitle || !strings.Contains(nfd.Subtitle, "re\u0301seaux") {
		t.Fatalf("decomposed subtitle: %q", nfd.Subtitle)
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
					t.Errorf("Width %d, Unicode %t, Color %t: card\n%s\nwant, as with precomposed "+
						"accents,\n%s", width, unicodeOK, color, Card(nfd), want)
				}
			}
		}
	}
	nfd.Width, nfd.Unicode, nfd.Color = 80, false, false
	if card := Card(nfd); !strings.Contains(card, "promo-reseaux") || strings.Contains(card, "?") {
		t.Errorf("without Unicode: decomposed accents badly removed:\n%s", card)
	}
}

// The line breaks, tabs and escape sequences of the texts do not distort the card and do not
// reach the terminal.
func TestCardCleansText(t *testing.T) {
	o := CardOptions{Title: "Wat\nchoo", Version: "dev\x1b[31m", Subtitle: "a\tb\r\nc\u202ed", Unicode: true}
	got := plainLines(Card(o))
	if !strings.Contains(got[1], "Wat choo dev?[31m") || !strings.Contains(got[2], "a b  c?d") {
		t.Errorf("badly cleaned texts:\n%s", strings.Join(got, "\n"))
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
		{"c\u202ed\u200be", true, "c?d?e"}, // format characters (Cf): "?", never a zero width
		// Combining marks: kept after a character, removed without a character to accent.
		{"promo-re\u0301seaux cre\u0301e\u0301\u0302", true, "promo-re\u0301seaux cre\u0301e\u0301\u0302"},
		{"\u0301Watchoo", true, "Watchoo"},
		{"a \u0301b \u20dd", true, "a b"},
		{"\t\u0301\u0302x", true, "x"},
		{"\x1b\u0301", true, "?\u0301"},
		{"promo-re\u0301seaux \u0301x", false, "promo-reseaux x"},
		{"« Réseaux »\n", false, `"Reseaux"`},
	} {
		if got := clean(tt.in, tt.unicodeOK); got != tt.want {
			t.Errorf("clean(%q, %t) = %q, want %q", tt.in, tt.unicodeOK, got, tt.want)
		}
	}
}

// A combining mark joins the column of the character before it, whatever its style; without a
// character to accent, it is ignored.
func TestStyledMarks(t *testing.T) {
	var s styled
	s.add("\u0301e\u0301t", plain)
	s.add("\u0302", bold)
	s.add("e\u0301", gray)
	if s.width() != 3 || !slices.Equal(s.runes, []rune("ete")) ||
		!slices.Equal(s.marks, []string{"\u0301", "\u0302", "\u0301"}) {
		t.Errorf("styled = %q / %q, width %d, want 3 columns \"é\", \"t̂\", \"é\"", string(s.runes), s.marks,
			s.width())
	}
	if got, want := s.render(false), "e\u0301t\u0302e\u0301"; got != want {
		t.Errorf("render(false) = %q, want %q", got, want)
	}
	if got, want := s.render(true), "e\u0301t\u0302"+ansiGray+"e\u0301"+ansiReset; got != want {
		t.Errorf("render(true) = %q, want %q", got, want)
	}
	if got := s.slice(1, 3).render(false); got != "t\u0302e\u0301" {
		t.Errorf("slice(1, 3) = %q, want columns 1 and 2 with their marks", got)
	}
}

// With color: borders in gray, title in bold, version and subtitle in gray, owl pupils in green
// and the tip's command in cyan, every sequence closed; without the sequences, the card is the
// one without color.
func TestCardColors(t *testing.T) {
	for _, unicodeOK := range []bool{true, false} {
		o := helpCard
		o.Unicode, o.Width = unicodeOK, 80
		plain := Card(o)
		o.Color = true
		colored := Card(o)
		if got := ansi.ReplaceAllString(colored, ""); got != plain {
			t.Errorf("Unicode %t: color card, without its sequences:\n%s\nwant\n%s", unicodeOK, got, plain)
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
			t.Errorf("Unicode %t: bold %q, want the title only", unicodeOK, got)
		}
		if got := runs[ansiGreen]; !slices.Equal(got, []string{pupil, pupil}) {
			t.Errorf("Unicode %t: green %q, want the two pupils", unicodeOK, got)
		}
		if got := runs[ansiCyan]; !slices.Equal(got, []string{"watchoo help <commande>"}) {
			t.Errorf("Unicode %t: cyan %q, want the tip's command", unicodeOK, got)
		}
		lines := plainLines(plain)
		subtitle := "Qui a travaillé, sur quoi, et quand ?"
		if !unicodeOK {
			subtitle = "Qui a travaille, sur quoi, et quand ?"
		}
		// Top border; line 1: border, version, border; line 2: border, subtitle, border;
		// lines 3 to 5 (owl, empty line, tip): borders; bottom border.
		wantGray := []string{lines[0], side, "v0.1.0", side, side, subtitle, side, side, side, side, side, side,
			side, lines[len(lines)-1]}
		if got := runs[ansiGray]; !slices.Equal(got, wantGray) {
			t.Errorf("Unicode %t: gray\n%q\nwant the borders, the version and the subtitle\n%q", unicodeOK, got,
				wantGray)
		}
		// Every opened sequence is closed before the next one.
		if n, closed := len(ansi.FindAllString(colored, -1)), len(styledRun.FindAllString(colored, -1)); n != 2*closed {
			t.Errorf("Unicode %t: %d sequences for %d styled texts, want %d", unicodeOK, n, closed, 2*closed)
		}
	}
}

// Without Unicode, the "o"s of the text are not taken for pupils: only the owl's are in
// green.
func TestCardColorsOnlyOwlPupils(t *testing.T) {
	colored := Card(CardOptions{Title: "oooo", Subtitle: "o o o", Tip: "ooo « o »", Color: true})
	if n := strings.Count(colored, ansiGreen); n != 2 {
		t.Errorf("%d green sequences, want 2 (the pupils):\n%q", n, colored)
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
		"07e0d1b":        "07e0d1b", // short commit (git describe without tag): not a number
		"07e0d1b-dirty":  "07e0d1b-dirty",
		"1":              "1",
		"2024a.b":        "2024a.b",
		"0.1.0-3-gabcde": "v0.1.0-3-gabcde",
	} {
		if got := displayVersion(v); got != want {
			t.Errorf("displayVersion(%q) = %q, want %q", v, got, want)
		}
	}
	for _, tt := range []struct{ version, want string }{{"0.1.0", "Watchoo v0.1.0"}, {"dev", "Watchoo dev"},
		{"", "Watchoo   "}} {
		lines := plainLines(Card(CardOptions{Title: "Watchoo", Version: tt.version, Subtitle: "sous-titre", Unicode: true}))
		if !strings.Contains(lines[1], "  "+tt.want) {
			t.Errorf("version %q: first line %q, want %q", tt.version, lines[1], tt.want)
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
		// Decomposed accents (NFD): kept with Unicode, removed without (no "?").
		{"Espace : promo-re\u0301seaux", CardOptions{Unicode: true}, "● Espace : promo-re\u0301seaux\n"},
		{"Espace : promo-re\u0301seaux (~/promo-re\u0301seaux)", CardOptions{},
			"* Espace : promo-reseaux (~/promo-reseaux)\n"},
		{"\u0301Espace :  \u0301x", CardOptions{Unicode: true}, "● Espace :  x\n"},
	} {
		if got := Status(tt.text, tt.o); got != tt.want {
			t.Errorf("Status(%q, %+v) = %q, want %q", tt.text, tt.o, got, tt.want)
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
			t.Errorf("toASCII(%q) = %q, want %q", in, got, want)
		}
		if !isASCII(toASCII(in)) {
			t.Errorf("toASCII(%q) is not ASCII", in)
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
		// Zero or negative width: one character per line, without looping.
		{"ab c", 0, []string{"a", "b", "c"}},
		{"ab", -3, []string{"a", "b"}},
		{"ab abcdefghij cd", 4, []string{"ab", "abcd", "efgh", "ij", "cd"}},
		{"voir « watchoo init » : oui", 12, []string{"voir", "« watchoo", "init » : oui"}},
		{"Qui ? Quand ! Où ; quoi", 7, []string{"Qui ?", "Quand !", "Où ;", "quoi"}},
		// A too long non-breaking piece is cut, without a space at the edge of the lines.
		{"Quand ! Où", 6, []string{"Quand", "! Où"}},
		// The widths are counted in columns: a decomposed accent does not count and stays with its letter.
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
			t.Errorf("wrap(%q, %d) = %q, want %q", tt.text, tt.width, got, tt.want)
		}
	}
}
