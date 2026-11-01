package logo

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

// assetsDir est le répertoire des fichiers source du logo, à la racine du dépôt.
var assetsDir = filepath.Join("..", "..", "assets", "logo")

// Les copies embarquées sont identiques, octet pour octet, aux fichiers de assets/logo/, et il n'y en a
// ni en plus ni en moins.
func TestArtMatchesAssets(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join(assetsDir, "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var want []string
	for _, src := range sources {
		want = append(want, filepath.Base(src))
	}
	copies, err := fs.Glob(files, "art/*.txt")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, c := range copies {
		got = append(got, strings.TrimPrefix(c, "art/"))
	}
	slices.Sort(want)
	slices.Sort(got)
	if len(want) == 0 || !slices.Equal(got, want) {
		t.Fatalf("copies embarquées = %v, fichiers de assets/logo = %v : lancez « make logo » pour recopier "+
			"les fichiers", got, want)
	}
	for _, name := range want {
		src, err := os.ReadFile(filepath.Join(assetsDir, name))
		if err != nil {
			t.Fatal(err)
		}
		dst, err := files.ReadFile("art/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(src, dst) {
			t.Errorf("internal/logo/art/%s diffère de assets/logo/%s : lancez « make logo » pour recopier "+
				"les fichiers", name, name)
		}
	}

	// Chaque fichier correspond à une version, et inversement.
	var names []string
	for _, v := range Variants() {
		names = append(names, v.File())
	}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("versions = %v, fichiers = %v", names, want)
	}
}

// Les tailles sont celles de documentation/fr/06-interface-web.md, et les versions en ASCII pur ne
// contiennent que de l'ASCII.
func TestVariantSizes(t *testing.T) {
	for _, tt := range []struct {
		v             Variant
		width, height int
		unicode       bool
	}{
		{BlocksName, 53, 8, true},
		{Blocks, 16, 8, true},
		{MiniName, 44, 4, true},
		{Mini, 8, 3, true},
		{MiniPure, 8, 3, false},
		{ASCIIName, 53, 8, false},
		{ASCII, 16, 8, false},
	} {
		lines := tt.v.Lines()
		if got := tt.v.Width(); got != tt.width {
			t.Errorf("%s : largeur %d, attendu %d", tt.v, got, tt.width)
		}
		if len(lines) != tt.height {
			t.Errorf("%s : hauteur %d, attendu %d", tt.v, len(lines), tt.height)
		}
		if got := isASCII(strings.Join(lines, "\n")); got == tt.unicode {
			t.Errorf("%s : dessin en ASCII pur = %t, attendu %t", tt.v, got, !tt.unicode)
		}
	}
	if len(Variants()) != 7 {
		t.Errorf("Variants() = %v, attendu 7 versions", Variants())
	}
	v := Variant("inconnue")
	if v.Width() != 0 || v.Lines() != nil {
		t.Errorf("version inconnue : largeur %d, lignes %q", v.Width(), v.Lines())
	}
}

// Les deux versions mini du hibou, celles de la carte, sont exactement celles attendues ; la version en
// ASCII pur ne contient que deux « o », ses pupilles.
func TestMiniDrawings(t *testing.T) {
	if got, want := Mini.Lines(), []string{"█▄    ▄█", "█ ●██● █", "▀██▙▟██▀"}; !slices.Equal(got, want) {
		t.Errorf("Mini = %q, attendu %q", got, want)
	}
	if got, want := MiniPure.Lines(), []string{`|\    /|`, "| o##o |", `\##\/##/`}; !slices.Equal(got, want) {
		t.Errorf("MiniPure = %q, attendu %q", got, want)
	}
	if n := strings.Count(strings.Join(MiniPure.Lines(), ""), "o"); n != 2 {
		t.Errorf("MiniPure contient %d « o », attendu 2 (les pupilles)", n)
	}
}

func TestParseDrawing(t *testing.T) {
	d := parseDrawing("ab  \r\n\ncdé \n  f")
	if want := []string{"ab", "", "cdé", "  f"}; !slices.Equal(d.lines, want) {
		t.Errorf("lignes = %q, attendu %q", d.lines, want)
	}
	if d.width != 3 {
		t.Errorf("largeur = %d, attendu 3 (en caractères, pas en octets)", d.width)
	}
	if d := parseDrawing("e\u0301te\u0301\n"); d.width != 3 {
		t.Errorf("largeur = %d, attendu 3 (accents décomposés dans la colonne de leur lettre)", d.width)
	}
}

// isASCII indique si s ne contient que des caractères ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
