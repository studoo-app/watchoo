package textclean

import "testing"

func TestClean(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"texte simple, accentué é", "texte simple, accentué é"},
		{"a\x1b[31mb\x1b[0m", "ab"},
		{"x\x1b]0;titre\x07y", "xy"},
		{"x\x1b]8;;https://x.test\x1b\\lien", "xlien"},
		{"a\x1bcb", "ab"},         // one-character ESC
		{"fin\x1b", "fin"},        // final ESC
		{"a\x1b[12", "a"},         // unterminated CSI
		{"a\nb\tc\rd", "a b c d"}, // C0 controls
		{"a\u009b31mb", "a 31mb"}, // C1 CSI: replaced, without sequence
		{"‮eman", " eman"},        // writing direction mark (Cf)
		{"zéro​largeur", "zéro largeur"},
		{"\x7f", " "},
	} {
		if got := Clean(tc.in); got != tc.want {
			t.Errorf("Clean(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCleanLines(t *testing.T) {
	in := "ligne 1 :\x1b[2J ok\n\tligne 2\r\x07"
	if got, want := CleanLines(in), "ligne 1 : ok\n\tligne 2  "; got != want {
		t.Errorf("CleanLines = %q, want %q", got, want)
	}
	if got := CleanLines("a\nb"); got != "a\nb" {
		t.Errorf("CleanLines without control = %q", got)
	}
}
