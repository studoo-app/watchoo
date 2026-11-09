package git

// Reading of the "ls-tree -r -l -z" and "cat-file --batch" outputs (phase 8.3).

import (
	"strings"
	"testing"
)

func TestTreeWriter(t *testing.T) {
	raw := "100644 blob aaaa 12\tREADME.md\x00" +
		"120000 blob bbbb 9\tlien\x00" + // symbolic link
		"160000 commit cccc -\tsous-module\x00" +
		"100644 blob dddd 3\tbad\npath\x00" + // control character
		"100755 blob eeee 7\tscripts/run.sh\x00" +
		"100644 blob ffff x\tillisible\x00" +
		"100644 blob 0000 1\t" + strings.Repeat("a", maxTreeRecord+1) + "\x00" + // oversized record
		"100644 blob 1111 0\tvide.md\x00"
	var got []string
	w := &treeWriter{fn: func(file string, e treeEntry) bool {
		got = append(got, file+"="+e.oid)
		return true
	}}
	// Written in small chunks, as a pipe would.
	for i := 0; i < len(raw); i += 7 {
		if _, err := w.Write([]byte(raw[i:min(i+7, len(raw))])); err != nil {
			t.Fatal(err)
		}
	}
	want := "README.md=aaaa scripts/run.sh=eeee vide.md=1111"
	if strings.Join(got, " ") != want {
		t.Errorf("records = %v, want %s", got, want)
	}

	n := 0
	stop := &treeWriter{fn: func(string, treeEntry) bool { n++; return false }}
	if _, err := stop.Write([]byte(raw)); err != nil || n != 1 || !stop.stopped {
		t.Errorf("stop after the first record: n=%d stopped=%v err=%v", n, stop.stopped, err)
	}
}

func TestPathListTruncates(t *testing.T) {
	l := &pathList{limit: 2}
	w := &treeWriter{fn: l.add}
	if _, err := w.Write([]byte("100644 blob a 1\ta\x00100644 blob b 1\tb\x00")); err != nil || l.truncated {
		t.Fatalf("at the cap: %v, truncated=%v", l.paths, l.truncated)
	}
	if _, err := w.Write([]byte("100644 blob c 1\tc\x00100644 blob d 1\td\x00")); err != nil {
		t.Fatal(err)
	}
	if !l.truncated || strings.Join(l.paths, ",") != "a,b" {
		t.Errorf("over the cap: %v, truncated=%v", l.paths, l.truncated)
	}
}

func TestParseCatFile(t *testing.T) {
	got, ok := parseCatFile([]byte("aaaa blob 5\nhello\nbbbb missing\ncccc blob 0\n\ndddd tree 2\nxx\n"))
	if !ok || string(got["aaaa"]) != "hello" || len(got["cccc"]) != 0 || len(got) != 2 {
		t.Errorf("parseCatFile = %q, %v", got, ok)
	}
	for _, bad := range []string{"aaaa blob 5\nhel", "aaaa blob\n", "aaaa blob 3\nabcd", "sans fin"} {
		if _, ok := parseCatFile([]byte(bad)); ok {
			t.Errorf("parseCatFile(%q) accepted", bad)
		}
	}
}
