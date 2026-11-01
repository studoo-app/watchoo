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

// assetsDir is the directory of the logo's source files, at the root of the repository.
var assetsDir = filepath.Join("..", "..", "assets", "logo")

// The embedded copies are identical, byte for byte, to the files of assets/logo/, and there are
// neither more nor fewer.
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
		t.Fatalf("embedded copies = %v, files of assets/logo = %v: run \"make logo\" to copy "+
			"the files again", got, want)
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
			t.Errorf("internal/logo/art/%s differs from assets/logo/%s: run \"make logo\" to copy "+
				"the files again", name, name)
		}
	}

	// Every file matches a variant, and conversely.
	var names []string
	for _, v := range Variants() {
		names = append(names, v.File())
	}
	slices.Sort(names)
	if !slices.Equal(names, want) {
		t.Errorf("variants = %v, files = %v", names, want)
	}
}

// The sizes are those of documentation/en/06-web-interface.md, and the pure ASCII variants only
// contain ASCII.
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
			t.Errorf("%s: width %d, want %d", tt.v, got, tt.width)
		}
		if len(lines) != tt.height {
			t.Errorf("%s: height %d, want %d", tt.v, len(lines), tt.height)
		}
		if got := isASCII(strings.Join(lines, "\n")); got == tt.unicode {
			t.Errorf("%s: pure ASCII drawing = %t, want %t", tt.v, got, !tt.unicode)
		}
	}
	if len(Variants()) != 7 {
		t.Errorf("Variants() = %v, want 7 variants", Variants())
	}
	v := Variant("inconnue")
	if v.Width() != 0 || v.Lines() != nil {
		t.Errorf("unknown variant: width %d, lines %q", v.Width(), v.Lines())
	}
}

// The two mini variants of the owl, those of the card, are exactly the expected ones; the pure
// ASCII variant only contains two "o"s, its pupils.
func TestMiniDrawings(t *testing.T) {
	if got, want := Mini.Lines(), []string{"█▄    ▄█", "█ ●██● █", "▀██▙▟██▀"}; !slices.Equal(got, want) {
		t.Errorf("Mini = %q, want %q", got, want)
	}
	if got, want := MiniPure.Lines(), []string{`|\    /|`, "| o##o |", `\##\/##/`}; !slices.Equal(got, want) {
		t.Errorf("MiniPure = %q, want %q", got, want)
	}
	if n := strings.Count(strings.Join(MiniPure.Lines(), ""), "o"); n != 2 {
		t.Errorf("MiniPure contains %d \"o\", want 2 (the pupils)", n)
	}
}

func TestParseDrawing(t *testing.T) {
	d := parseDrawing("ab  \r\n\ncdé \n  f")
	if want := []string{"ab", "", "cdé", "  f"}; !slices.Equal(d.lines, want) {
		t.Errorf("lines = %q, want %q", d.lines, want)
	}
	if d.width != 3 {
		t.Errorf("width = %d, want 3 (in characters, not in bytes)", d.width)
	}
	if d := parseDrawing("e\u0301te\u0301\n"); d.width != 3 {
		t.Errorf("width = %d, want 3 (decomposed accents in their letter's column)", d.width)
	}
}

// isASCII reports whether s only contains ASCII characters.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
