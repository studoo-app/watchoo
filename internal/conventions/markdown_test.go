package conventions

import (
	"reflect"
	"testing"
)

func headings(doc mdDoc) []string {
	var out []string
	for _, l := range doc.Lines {
		if l.Kind == mdHeading {
			out = append(out, string(rune('0'+l.Level))+":"+l.Text)
		}
	}
	return out
}

func TestParseMarkdownHeadings(t *testing.T) {
	src := "# Titre\n" +
		"## Section ##\n" +
		"   ### Trois espaces\n" +
		"    #### Quatre espaces (code indenté, pas un titre)\n" +
		"#Collé\n" +
		"####### Sept\n" +
		"## C# et F#\n" +
		"#\n" +
		"```\n## Dans un bloc\n```\n" +
		"~~~~ go\n## Tildes\n~~~\n## Toujours dans le bloc (fermeture plus courte)\n~~~~\n" +
		"```` \n```\n## Fermeture trop courte\n````\n" +
		"<!-- \n## Commentaire\n-->\n" +
		"## Après\n"
	got := headings(parseMarkdown([]byte(src)))
	want := []string{"1:Titre", "2:Section", "3:Trois espaces", "2:C# et F#", "1:", "2:Après"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("titres = %q, attendu %q", got, want)
	}
}

// Une clôture dans un commentaire HTML (un modèle de CHANGELOG gardé en
// commentaire) n'ouvre pas de bloc de code : la suite reste lue.
func TestParseMarkdownFenceInComment(t *testing.T) {
	src := "# Changelog\n" +
		"<!--\n## [x.y.z] - AAAA-MM-JJ\n```\nexemple\n-->\n" +
		"## [1.0.0] - 2026-01-01\n" +
		"### Added\n" +
		"<!-- ``` -->\n" +
		"## [0.1.0] - 2025-12-01\n"
	got := headings(parseMarkdown([]byte(src)))
	want := []string{"1:Changelog", "2:[1.0.0] - 2026-01-01", "3:Added", "2:[0.1.0] - 2025-12-01"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("titres = %q, attendu %q", got, want)
	}
}

func TestParseMarkdownUnclosedFence(t *testing.T) {
	got := headings(parseMarkdown([]byte("## Avant\n```\n## Jamais fermé\n")))
	if !reflect.DeepEqual(got, []string{"2:Avant"}) {
		t.Errorf("titres = %q", got)
	}
}

func TestParseMarkdownBullets(t *testing.T) {
	doc := parseMarkdown([]byte("- a\n* b\n+ c\n1. d\n2) e\n-\n  - imbriquée\n---\n* * *\n**gras**\n-pas une puce\n"))
	var bullets []string
	breaks := 0
	for _, l := range doc.Lines {
		switch l.Kind {
		case mdBullet:
			bullets = append(bullets, l.Text)
		case mdBreak:
			breaks++
		}
	}
	want := []string{"a", "b", "c", "d", "e", "", "imbriquée"}
	if !reflect.DeepEqual(bullets, want) || breaks != 2 {
		t.Errorf("puces = %q, séparateurs = %d", bullets, breaks)
	}
}

func TestParseMarkdownLinks(t *testing.T) {
	src := "Voir [l'index](documentation/INDEX.md#haut) et ![logo](img/logo.png \"titre\").\n" +
		"Un [lien <chevrons>](<mon fichier.md>) ; `[pas un lien](code.md)` ; [vide]().\n" +
		"[ref]: ./CHANGELOG.md \"Changelog\"\n" +
		"   [autre]: <docs/a b.md>\n" +
		"[texte [imbriqué]](a.md) [paren](b_(1).md)\n" +
		"```\n[dans le code](c.md)\n```\n"
	doc := parseMarkdown([]byte(src))
	want := []mdLink{
		{Target: "documentation/INDEX.md#haut"},
		{Target: "img/logo.png", Image: true},
		{Target: "mon fichier.md"},
		{Target: ""},
		{Target: "./CHANGELOG.md", Def: true, Label: "ref"},
		{Target: "docs/a b.md", Def: true, Label: "autre"},
		{Target: "a.md"},
		{Target: "b_(1).md"},
	}
	if !reflect.DeepEqual(doc.Links, want) {
		t.Errorf("liens = %+v\nattendu %+v", doc.Links, want)
	}
}

func TestParseMarkdownFences(t *testing.T) {
	src := "```bash\nnpm ci\n```\n# Titre\n\n## Installation\n\n~~~ Shell {.numbered}\nx\n~~~\n" +
		"```{.mermaid}\ngraph TD\n```\n### Sous-titre\n````\n```js\n````\n<!--\n```python\n-->\n"
	doc := parseMarkdown([]byte(src))
	want := []mdFence{{Info: "bash", Heading: -1}, {Info: "shell", Heading: 2}, {Info: "mermaid", Heading: 2},
		{Info: "", Heading: 4}}
	if !reflect.DeepEqual(doc.Fences, want) {
		t.Errorf("blocs = %+v\nattendu %+v", doc.Fences, want)
	}
	if doc.Lines[2].Text != "Installation" || doc.Lines[4].Text != "Sous-titre" {
		t.Errorf("titres = %+v", doc.Lines)
	}
	// Les blocs n'entrent ni dans les lignes ni dans les mots.
	if doc.Words != 4 {
		t.Errorf("mots = %d", doc.Words)
	}
	if got := headingChain(doc, 4); !reflect.DeepEqual(got, []int{4, 2, 0}) {
		t.Errorf("headingChain = %v", got)
	}
	if headingChain(doc, -1) != nil || headingChain(doc, 99) != nil {
		t.Error("headingChain hors bornes")
	}
}

func TestParseMarkdownTables(t *testing.T) {
	src := "| Document | Thème |\n|---|:--:|\n| [a](a.md) | x \\| y |\n| b | |\n\nTexte\n" +
		"a | b\n---\n" + // une seule colonne de délimitation : pas un tableau
		"Doc | Ver\n--- | ---\nx | 1\n"
	doc := parseMarkdown([]byte(src))
	if len(doc.Tables) != 2 {
		t.Fatalf("tableaux = %d, attendu 2 : %+v", len(doc.Tables), doc.Tables)
	}
	first := doc.Tables[0]
	if !reflect.DeepEqual(first.Header, []string{"Document", "Thème"}) ||
		!reflect.DeepEqual(first.Rows, [][]string{{"[a](a.md)", "x | y"}, {"b", ""}}) {
		t.Errorf("premier tableau = %+v", first)
	}
	if !reflect.DeepEqual(doc.Tables[1].Header, []string{"Doc", "Ver"}) || len(doc.Tables[1].Rows) != 1 {
		t.Errorf("second tableau = %+v", doc.Tables[1])
	}
}

func TestParseMarkdownWords(t *testing.T) {
	src := "# Mon projet\n\nUn texte de `code ignoré` six mots [lien](https://exemple.fr/a/b/c).\n" +
		"<img src=\"x.png\" alt=\"rien\">\n```\nbeaucoup de mots dans le code\n```\n"
	// « Mon projet » (2) + « Un texte de six mots lien » (6).
	if got := parseMarkdown([]byte(src)).Words; got != 8 {
		t.Errorf("mots = %d, attendu 8", got)
	}
}

func TestParseMarkdownInvalidUTF8AndBOM(t *testing.T) {
	doc := parseMarkdown([]byte("\uFEFF# Titre\r\n## Deux\xff\r\n"))
	if got := headings(doc); !reflect.DeepEqual(got, []string{"1:Titre", "2:Deux\uFFFD"}) {
		t.Errorf("titres = %q", got)
	}
}

func TestNormalizeHeading(t *testing.T) {
	cases := map[string]string{
		"Installation":                 "installation",
		"🚀 Installation & Lancement":   "installation lancement",
		"2. Variables d'environnement": "variables d environnement",
		"III) Prérequis":               "iii prerequis",
		"1 - Présentation":             "presentation",
		"SÉCURITÉ":                     "securite",
		"  Œuvre  —  Cœur ":            "oeuvre coeur",
		"Dernière mise à jour":         "derniere mise a jour",
		"[1.0.0] - 2026-01-01":         "",
		"":                             "",
	}
	for in, want := range cases {
		if got := normalizeHeading(in); got != want {
			t.Errorf("normalizeHeading(%q) = %q, attendu %q", in, got, want)
		}
	}
	if !containsPhrase("installation et lancement", "lancement") || containsPhrase("installations", "installation") {
		t.Error("containsPhrase doit comparer mot pour mot")
	}
}

func TestStripCodeSpans(t *testing.T) {
	cases := map[string]string{
		"a `b` c":       "a   c",
		"a ``b ` c`` d": "a   d",
		"a `non fermé":  "a `non fermé",
		"``a` b":        "``a` b",
		"sans code":     "sans code",
	}
	for in, want := range cases {
		if got := stripCodeSpans(in); got != want {
			t.Errorf("stripCodeSpans(%q) = %q, attendu %q", in, got, want)
		}
	}
}
