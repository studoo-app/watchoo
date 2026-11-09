package conventions

import (
	"errors"
	"reflect"
	"testing"
)

func defaultBranchChecker(t *testing.T) *BranchChecker {
	t.Helper()
	c, err := NewBranchChecker(BranchRules{Types: DefaultTypes(), Exclude: DefaultBranchExclude()})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestCheckBranchBuiltin(t *testing.T) {
	c := defaultBranchChecker(t)
	cases := []struct {
		name    string
		want    []string
		conform bool
		review  bool
	}{
		{"feat/login-page", nil, true, false},
		{"fix/ui-2", nil, true, false},
		{"docs/readme-v2-install", nil, true, false},
		{"chore/42-bump-deps", nil, true, false},
		{"feat/login", []string{BranchSingleSegment}, true, true},
		{"fix/2024", []string{BranchSingleSegment}, true, true},
		{"feature/login-page", []string{BranchUnknownType}, false, false},
		{"Feat/login-page", []string{BranchUppercase}, false, false},
		{"feat/Login-Page", []string{BranchUppercase}, false, false},
		{"feat/login_page", []string{BranchUnderscore, BranchSingleSegment}, false, false},
		{"feat/login page", []string{BranchSpace, BranchSingleSegment}, false, false},
		{"feat/éclair-page", []string{BranchAccent}, false, false},
		{"feat/Éclair-page", []string{BranchUppercase, BranchAccent}, false, false},
		{"login-page", []string{BranchNoSlash}, false, false},
		{"dev", []string{BranchNoSlash}, false, false},
		{"Dev_Branch", []string{BranchUppercase, BranchUnderscore, BranchNoSlash}, false, false},
		{"wip/Test_1", []string{BranchUnknownType, BranchUppercase, BranchUnderscore, BranchSingleSegment}, false, false},
		{"feat/", []string{BranchPattern}, false, false},
		{"feat/login-", []string{BranchPattern}, false, false},
		{"feat/-login", []string{BranchPattern}, false, false},
		{"feat/login.page-x", []string{BranchPattern}, false, false},
		{"feat/auth/login-page", []string{BranchPattern}, false, false},
		{"feat/login--page", []string{BranchPattern}, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.Check(tc.name)
			want := tc.want
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got.Codes, want) {
				t.Errorf("codes = %v, want %v", got.Codes, want)
			}
			if got.Conform() != tc.conform || got.Review() != tc.review {
				t.Errorf("conform = %v review = %v, want %v %v", got.Conform(), got.Review(), tc.conform, tc.review)
			}
			if got.Name != tc.name {
				t.Errorf("name = %q", got.Name)
			}
		})
	}
}

func TestCheckBranchCustomTypes(t *testing.T) {
	c, err := NewBranchChecker(BranchRules{Types: []string{"feat", "hotfix"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Check("hotfix/crash-on-start"); len(got.Codes) != 0 {
		t.Errorf("hotfix = %v", got.Codes)
	}
	if got := c.Check("fix/crash-on-start"); !reflect.DeepEqual(got.Codes, []string{BranchUnknownType}) {
		t.Errorf("fix = %v", got.Codes)
	}
	none, err := NewBranchChecker(BranchRules{})
	if err != nil {
		t.Fatal(err)
	}
	if got := none.Check("feat/login-page"); !reflect.DeepEqual(got.Codes, []string{BranchUnknownType}) {
		t.Errorf("no types = %v", got.Codes)
	}
}

func TestCheckBranchCustomPattern(t *testing.T) {
	c, err := NewBranchChecker(BranchRules{Types: DefaultTypes(), Pattern: `^(feat|fix)/[A-Z]+-[0-9]+$`})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]string{
		"feat/JIRA-12":    {},
		"fix/ABC-1":       {},
		"feat/login-page": {BranchPattern},
		"Feat_x":          {BranchPattern},
	} {
		got := c.Check(name)
		if !reflect.DeepEqual(got.Codes, want) {
			t.Errorf("%s: codes = %v, want %v", name, got.Codes, want)
		}
		if got.Conform() != (len(want) == 0) || got.Review() {
			t.Errorf("%s: conform = %v review = %v", name, got.Conform(), got.Review())
		}
	}
}

func TestBranchValidation(t *testing.T) {
	if err := ValidBranchPattern(""); err != nil {
		t.Errorf("empty pattern: %v", err)
	}
	if err := ValidBranchPattern(`^feat/.+$`); err != nil {
		t.Errorf("valid pattern: %v", err)
	}
	for _, p := range []string{`^(feat`, `[a-`, `*x`} {
		if err := ValidBranchPattern(p); !errors.Is(err, ErrInvalidBranchPattern) {
			t.Errorf("%q: err = %v", p, err)
		}
	}
	for _, g := range DefaultBranchExclude() {
		if err := ValidBranchExclude(g); err != nil {
			t.Errorf("%q: %v", g, err)
		}
	}
	for _, g := range []string{"", "  ", "release/[", `x\`} {
		if err := ValidBranchExclude(g); !errors.Is(err, ErrInvalidBranchExclude) {
			t.Errorf("%q: err = %v", g, err)
		}
	}
	if _, err := NewBranchChecker(BranchRules{Pattern: "("}); !errors.Is(err, ErrInvalidBranchPattern) {
		t.Errorf("checker pattern err = %v", err)
	}
	if _, err := NewBranchChecker(BranchRules{Exclude: []string{"["}}); !errors.Is(err, ErrInvalidBranchExclude) {
		t.Errorf("checker exclude err = %v", err)
	}
}

func TestBranchExcluded(t *testing.T) {
	c := defaultBranchChecker(t)
	for name, want := range map[string]bool{
		"main":                                   true, // branche par défaut
		"":                                       true,
		"gh-pages":                               true,
		"dependabot/npm_and_yarn/lodash-4.17.21": true, // sous dependabot/* : préfixe de chemin
		"dependabot/github_actions":              true,
		"renovate/react-18.x":                    true,
		"renovate":                               false, // « renovate/* » ne désigne pas « renovate »
		"dependabot":                             false,
		"feat/gh-pages":                          false,
		"gh-pages-old":                           false,
		"master":                                 false,
		"feat/login-page":                        false,
	} {
		if got := c.Excluded(name, "main"); got != want {
			t.Errorf("Excluded(%q) = %v, want %v", name, got, want)
		}
	}
	custom, err := NewBranchChecker(BranchRules{Types: DefaultTypes(), Exclude: []string{"release/*", "prof"}})
	if err != nil {
		t.Fatal(err)
	}
	if !custom.Excluded("release/1.0", "main") || !custom.Excluded("prof", "main") || custom.Excluded("gh-pages", "main") {
		t.Error("custom exclusions")
	}
}

func TestCheckAllBranches(t *testing.T) {
	c := defaultBranchChecker(t)
	got := c.CheckAll([]string{
		"main", "feat/login-page", "Dev", "feat/login-page", "gh-pages", "dependabot/npm_and_yarn/x-1", "fix/a",
	}, "main")
	var names []string
	for _, r := range got {
		names = append(names, r.Name)
	}
	if !reflect.DeepEqual(names, []string{"Dev", "feat/login-page", "fix/a"}) {
		t.Fatalf("names = %v", names)
	}
	if got[0].Conform() || !got[1].Conform() || !got[2].Review() {
		t.Errorf("results = %+v", got)
	}
	if empty := c.CheckAll(nil, "main"); empty == nil || len(empty) != 0 {
		t.Errorf("empty = %#v", empty)
	}
}

func TestBranchSeverity(t *testing.T) {
	for _, code := range AllBranchCodes {
		want := SeverityBlocking
		if code == BranchSingleSegment {
			want = SeverityWarning
		}
		if got := BranchSeverityOf(code); got != want {
			t.Errorf("%s: %v", code, got)
		}
	}
	if BranchSeverityOf("nope") != 0 {
		t.Error("unknown code")
	}
}
