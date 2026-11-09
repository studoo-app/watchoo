package conventions

import (
	"reflect"
	"testing"
)

func TestCheckPRBody(t *testing.T) {
	const (
		ctx  = PRBodyContext
		chg  = PRBodyChanges
		tst  = PRBodyTesting
		scr  = PRBodyScreenshots
		cls  = PRBodyCloses
		full = ctx | chg | tst | scr | cls
	)
	cases := []struct {
		name string
		body string
		want int64
	}{
		{"empty", "", 0},
		{"blank", "  \n\n\t\n", 0},
		{"plain text", "Ajoute la page de connexion.", 0},
		{"charter fr", "## Contexte\nLa page manquait.\n\n## Ce qui change\n- formulaire\n\n## Comment tester\n1. ouvrir /login\n\n## Captures\n![login](https://example.test/a.png)\n\nCloses #12\n", full},
		{"charter en", "## Context\nWhy.\n## What changed\nStuff.\n## How to test\nRun it.\n## Screenshots\n<img src=\"a.png\">\n\nFixes #3", full},
		{"accents and punctuation", "### 1. Contexte :\ntexte\n### Ce qui a changé ?\ntexte\n### Plan de tests\ntexte\n### Captures d'écran\nimage", ctx | chg | tst | scr},
		{"crlf", "## Context\r\ntext\r\n## Testing\r\nok\r\n", ctx | tst},
		{"bom", "\uFEFF## Context\ntext", ctx},
		{"empty sections", "## Context\n\n## Changes\n\n## Testing\n\n## Screenshots\n", 0},
		{"only html comments", "## Context\n<!-- Pourquoi ? -->\n## Changes\n<!--\nDécrire\nles changements\n-->\n## Testing\n<!-- a --> <!-- b -->\n", 0},
		{"comment then text", "## Context\n<!-- Pourquoi ? --> Parce que.\n", ctx},
		{"multiline comment then text", "## Context\n<!--\nx\n--> texte\n", ctx},
		{"section ended by break only", "## Context\n---\n", 0},
		{"code only counts", "## Testing\n```bash\nmake test\n```\n", tst},
		{"empty fence", "## Testing\n```\n```\n", 0},
		{"heading in code ignored", "```\n## Context\ntext\n```\n", 0},
		{"subheading belongs to section", "## Testing\n### Unit\n- go test\n## Screenshots\n", tst},
		{"sibling heading ends section", "## Context\n## Changes\ntext\n", chg},
		{"higher heading ends section", "### Context\n## Other\ntext\n", 0},
		{"unknown heading", "## Notes\ntext\n", 0},
		{"heading contains synonym", "## Contexte et motivation\ntext", ctx},
		{"bold label alone", "**Contexte**\nParce que.\n\n**Tests :**\n- make test\n", ctx | tst},
		{"bold label inline content", "**Context:** because.\n__Changes__: stuff\n", ctx | chg},
		{"bold label colon inside", "**Comment tester :** lancer make\n", tst},
		{"bold label empty", "**Context**\n**Changes**\nstuff\n", chg},
		{"bold label ended by heading", "**Context**\n## Notes\ntext\n", 0},
		{"bold sentence not label", "**This PR adds tests for login**\n", 0},
		{"bold mid line not label", "Voir le **contexte** plus bas.\n", 0},
		{"unclosed bold", "**Context\ntext\n", 0},
		{"one heading two sections", "## Changes and testing\ntext", chg | tst},
		{"closes lowercase colon", "closes: #7", cls},
		{"resolved cross repo", "Resolved octo/repo#7", cls},
		{"fixed", "This fixed #9 finally", cls},
		{"keyword without number", "Closes #", 0},
		{"keyword without hash", "Closes 12", 0},
		{"prefix word", "prefix #12 and unclosed #3", 0},
		{"closes in code span", "`Closes #12`", 0},
		{"closes in fence", "```\nCloses #12\n```", 0},
		{"closes in comment", "<!-- Closes #12 -->", 0},
		{"closes in multiline comment", "<!--\nCloses #12\n-->", 0},
		{"closes in heading", "## Closes #4", cls},
		{"setext not recognised", "Context\n-------\ntext", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckPRBody(tc.body); got != tc.want {
				t.Errorf("CheckPRBody = %05b, want %05b", got, tc.want)
			}
		})
	}
}

func TestPRBodySections(t *testing.T) {
	if got := AllPRBodySections(); !reflect.DeepEqual(got, []string{"context", "changes", "testing", "screenshots"}) {
		t.Errorf("AllPRBodySections = %v", got)
	}
	if !reflect.DeepEqual(DefaultPRBodySections(), AllPRBodySections()) {
		t.Error("DefaultPRBodySections differs from AllPRBodySections")
	}
	for _, k := range AllPRBodySections() {
		if !ValidPRBodySection(k) || PRBodySectionBit(k) == 0 {
			t.Errorf("section %q: valid %v, bit %d", k, ValidPRBodySection(k), PRBodySectionBit(k))
		}
		if len(prBodySynonyms[k]) == 0 {
			t.Errorf("section %q has no synonym", k)
		}
		for _, s := range prBodySynonyms[k] {
			if normalizeHeading(s) != s {
				t.Errorf("synonym %q of %q is not normalized", s, k)
			}
		}
	}
	for _, k := range []string{"", "closes", "Context", "summary"} {
		if ValidPRBodySection(k) || PRBodySectionBit(k) != 0 {
			t.Errorf("section %q accepted", k)
		}
	}
	if r := DefaultPRBodyRules(); !r.RequireCloses || !reflect.DeepEqual(r.Sections, AllPRBodySections()) {
		t.Errorf("DefaultPRBodyRules = %+v", r)
	}
}

func TestEvalPRBody(t *testing.T) {
	mask := func(v int64) *int64 { return &v }
	all := DefaultPRBodyRules()
	cases := []struct {
		name  string
		mask  *int64
		rules PRBodyRules
		want  PRBodyResult
	}{
		{"unknown", nil, all, PRBodyResult{Found: []string{}, Missing: []string{}}},
		{"empty body", mask(0), all, PRBodyResult{Known: true, Found: []string{},
			Missing: []string{"context", "changes", "testing", "screenshots"}}},
		{"full", mask(PRBodyContext | PRBodyChanges | PRBodyTesting | PRBodyScreenshots | PRBodyCloses), all,
			PRBodyResult{Known: true, Found: []string{"context", "changes", "testing", "screenshots"}, Missing: []string{},
				Closes: true, Conform: true}},
		{"sections without closes", mask(PRBodyContext | PRBodyChanges | PRBodyTesting | PRBodyScreenshots), all,
			PRBodyResult{Known: true, Found: []string{"context", "changes", "testing", "screenshots"}, Missing: []string{}}},
		{"closes not required", mask(PRBodyContext), PRBodyRules{Sections: []string{"context"}},
			PRBodyResult{Known: true, Found: []string{"context"}, Missing: []string{}, Conform: true}},
		{"nothing expected", mask(0), PRBodyRules{}, PRBodyResult{Known: true, Found: []string{}, Missing: []string{}, Conform: true}},
		{"only closes expected", mask(PRBodyCloses), PRBodyRules{RequireCloses: true},
			PRBodyResult{Known: true, Found: []string{}, Missing: []string{}, Closes: true, Conform: true}},
		{"unknown key ignored", mask(PRBodyTesting), PRBodyRules{Sections: []string{"testing", "bogus", "closes"}},
			PRBodyResult{Known: true, Found: []string{"testing"}, Missing: []string{}, Conform: true}},
		{"missing in charter order", mask(PRBodyChanges), PRBodyRules{Sections: []string{"screenshots", "context"}},
			PRBodyResult{Known: true, Found: []string{"changes"}, Missing: []string{"context", "screenshots"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvalPRBody(tc.mask, tc.rules); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("EvalPRBody = %+v, want %+v", got, tc.want)
			}
		})
	}
}
