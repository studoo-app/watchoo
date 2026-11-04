package git

import (
	"slices"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Renames (-M): "a\td\t\0old\0new\0" keeps the new path and the real line counts (0/0 for a pure move);
// a rename path that looks like a SHA is not taken for a commit.
func TestParseNumstatRenames(t *testing.T) {
	sha1, sha2 := strings.Repeat("a", 40), strings.Repeat("b", 40)
	raw := sha1 + "\x00" +
		"3\t1\tmain.go\x00" +
		"0\t0\t\x00src/old.go\x00lib/new.go\x00" +
		"2\t0\t\x00" + sha2 + "\x00" + sha2 + ".txt\x00" + // renamed file named like a SHA
		"-\t-\t\x00img/a.png\x00img/b.png\x00" +
		"\n" + sha2 + "\x00" +
		"1\t1\tx.go\x00"
	out := map[string]provider.CommitStats{}
	parseBatchNumstat([]byte(raw), nil, out)
	want1 := []provider.CommitFile{{Path: "main.go", Additions: 3, Deletions: 1}, {Path: "lib/new.go"},
		{Path: sha2 + ".txt", Additions: 2}, {Path: "img/b.png", Binary: true}}
	if st := out[sha1]; st.Additions != 5 || st.Deletions != 1 || !slices.Equal(st.Files, want1) {
		t.Errorf("commit 1: %+v", st)
	}
	if st := out[sha2]; st.Additions != 1 || !slices.Equal(st.Files, []provider.CommitFile{{Path: "x.go", Additions: 1, Deletions: 1}}) {
		t.Errorf("commit 2: %+v", st)
	}
	if len(out) != 2 {
		t.Errorf("%d commits, want 2: %v", len(out), out)
	}

	single := parseNumstat([]byte("0\t0\t\x00a.go\x00b.go\x004\t2\tc.go\x00"))
	if single.Additions != 4 || single.Deletions != 2 ||
		!slices.Equal(single.Files, []provider.CommitFile{{Path: "b.go"}, {Path: "c.go", Additions: 4, Deletions: 2}}) {
		t.Errorf("parseNumstat: %+v", single)
	}
}
