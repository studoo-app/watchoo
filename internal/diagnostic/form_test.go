package diagnostic

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// measure feeds the added lines of files (path → lines, one hunk per file) to a meter.
func measure(t *testing.T, ex *Excluder, files ...[2]string) Form {
	t.Helper()
	m := NewFormMeter(ex)
	for _, f := range files {
		for i, l := range strings.Split(f[1], "\n") {
			m.Line(f[0], l, i == 0)
		}
	}
	return m.Result()
}

func TestFormGo(t *testing.T) {
	f := measure(t, nil, [2]string{"cmd/main.go", `package main

// Add returns the sum of a and b.
func Add(a, b int) int {
	return a + b // sum
}

func sub(a, b int) int {
	/* not
	   documented */
	s := "if x { // not a comment"
	return a - b
}

// Doc
// on two lines
func (r *Repo) Name() string { return r.name }`})
	want := Form{SourceLines: 14, CommentLines: 5, Functions: 3, DocumentedFunctions: 2}
	got := f
	got.Identifiers, got.LexicalDiversity, got.HasDiversity = 0, 0, false
	if got != want {
		t.Errorf("form = %+v, want %+v", got, want)
	}
	if f.Identifiers == 0 || f.HasDiversity {
		t.Errorf("identifiers = %d, diversity %v (fewer than %d)", f.Identifiers, f.HasDiversity, MATTRWindow)
	}
}

func TestFormPython(t *testing.T) {
	f := measure(t, nil, [2]string{"app/models.py", `"""Module docstring."""
import os


@decorator
def documented(x):
    """Return x.

    Long description.
    """
    return x


def plain(y):
    text = """not a
    docstring"""
    return y

# helper
async def helper():
    pass`})
	if f.Functions != 3 || f.DocumentedFunctions != 2 {
		t.Errorf("functions = %d, documented = %d; want 3, 2", f.Functions, f.DocumentedFunctions)
	}
	// Comments: module docstring (1), docstring (3 non-blank lines), « # helper » (1).
	if f.CommentLines != 5 || f.SourceLines != 15 {
		t.Errorf("comment lines = %d, source lines = %d; want 5, 15", f.CommentLines, f.SourceLines)
	}
}

func TestFormJSAndPHP(t *testing.T) {
	f := measure(t, nil,
		[2]string{"src/app.ts", `/**
 * Fetches the user.
 */
export async function fetchUser(id: string): Promise<User> {
  if (id) {
    return api.get(id);
  }
}
const add = (a, b) => a + b;
class A {
  // Method
  run(x) {
    for (const y of x) {
    }
  }
}`},
		[2]string{"src/Controller.php", `<?php
#[Route('/')]
/** Index. */
public function index(): Response
{
    # comment
    return $this->render('x');
}`})
	// fetchUser (documented), add, run (documented), index (documented through the attribute? no:
	// the attribute comes before the doc comment, which stays the line just before).
	if f.Functions != 4 || f.DocumentedFunctions != 3 {
		t.Errorf("functions = %d, documented = %d; want 4, 3", f.Functions, f.DocumentedFunctions)
	}
	if f.CommentLines != 6 {
		t.Errorf("comment lines = %d, want 6", f.CommentLines)
	}
}

func TestFormIgnoredFilesAndTests(t *testing.T) {
	ex, err := NewExcluder([]string{"vendor/"})
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", maxFormLine+1)
	f := measure(t, ex,
		[2]string{"README.md", "# Title\ntext"},
		[2]string{"vendor/lib/a.go", "// c\nfunc A() {}"},
		[2]string{"pkg/a_test.go", "func TestA(t *testing.T) {\n\t" + long + "\n}"},
		[2]string{"tests/unit/test_b.py", "def test_b():\n    assert True"},
		[2]string{"web/__tests__/c.js", "it('works', () => {})"},
		[2]string{"src/Util.java", "public static int twice(int x) {\n  return 2 * x;\n}"},
	)
	// Test files: 2 (Go, the long line skipped) + 2 (Python) + 1 (JS); Java: 3 lines.
	if f.SourceLines != 8 || f.TestLines != 5 || f.CommentLines != 0 {
		t.Errorf("form = %+v", f)
	}
	if f.Functions != 3 {
		t.Errorf("functions = %d, want 3 (TestA, test_b, twice)", f.Functions)
	}
	for p, want := range map[string]bool{
		"a_test.go": true, "x/test_a.py": true, "a.spec.ts": true, "FooTest.java": true, "spec/a_spec.rb": true,
		"Tests/Unit/BarTest.php": true, "src/testdata.go": false, "latest.go": false, "contest/a.py": false,
	} {
		if got := isTestPath(p); got != want {
			t.Errorf("isTestPath(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestFormLexicalDiversity(t *testing.T) {
	// 100 distinct identifiers: every window is fully diverse.
	var b strings.Builder
	for i := range 100 {
		fmt.Fprintf(&b, "v%d := w%d\n", i, i)
	}
	f := measure(t, nil, [2]string{"a.go", b.String()})
	if !f.HasDiversity || f.LexicalDiversity != 1 || f.Identifiers != 200 {
		t.Errorf("diverse: %+v", f)
	}
	// Two identifiers repeated: 2 distinct in each window of 50.
	b.Reset()
	for range 60 {
		b.WriteString("count = count + total // " + `"ignored"` + "\n")
	}
	f = measure(t, nil, [2]string{"a.go", b.String()})
	if !f.HasDiversity || math.Abs(f.LexicalDiversity-2.0/MATTRWindow) > 1e-9 {
		t.Errorf("repetitive: %+v", f)
	}
	if got := identifierTokens(`if x := m["key"]; x != nil && y2 > 10 { return 'c' }`); strings.Join(got, ",") != "x,m,x,y2" {
		t.Errorf("identifierTokens = %v", got)
	}
}

func TestMeasureMessage(t *testing.T) {
	for _, tc := range []struct {
		msg  string
		want MessageStyle
	}{
		{"fix typo", MessageStyle{Length: 8}},
		{"feat(api): add users\n\n- list\n- create\n* delete\n\nCo-authored-by: Claude <noreply@anthropic.com>",
			MessageStyle{Length: 46, Bullets: 3, Conventional: true}},
		{"Ajout du menu\n\n1. accueil\n2) contact\n\n🤖 Generated with [Claude Code](https://claude.com/claude-code)",
			MessageStyle{Length: 36, Bullets: 2}},
		{"- not a bullet on the subject", MessageStyle{Length: 29}},
		{"Feature: something", MessageStyle{Length: 18}}, // a trailer-like subject is kept
	} {
		if got := MeasureMessage(tc.msg); got != tc.want {
			t.Errorf("MeasureMessage(%q) = %+v, want %+v", tc.msg, got, tc.want)
		}
	}
}
