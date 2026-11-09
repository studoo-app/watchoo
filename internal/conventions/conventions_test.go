package conventions

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseCommit(t *testing.T) {
	long := "feat: " + strings.Repeat("a", 60)    // 66 caractères
	tooLong := "feat: " + strings.Repeat("a", 70) // 76 caractères
	tests := []struct {
		name     string
		message  string
		codes    []string
		conform  bool
		typ      string
		scope    string
		breaking bool
	}{
		// Exemples conformes de la charte.
		{"type and description", "feat: ajoute la connexion", nil, true, "feat", "", false},
		{"scope", "fix(api): corrige le jeton expiré", nil, true, "fix", "api", false},
		{"every default type", "revert: annule l'ajout du cache", nil, true, "revert", "", false},
		{"breaking bang", "feat(auth)!: supprime l'ancienne route", nil, true, "feat", "auth", true},
		{"breaking footer", "refactor: renomme la table\n\nBREAKING CHANGE: la colonne change de nom", nil, true, "refactor", "", true},
		{"breaking-change footer", "refactor: x\n\nBREAKING-CHANGE: y", nil, true, "refactor", "", true},
		{"body and footers", "docs: complète le README\n\nAjoute la section installation.\n\nCloses #12\nCo-authored-by: Alice <alice@example.com>", nil, true, "docs", "", false},
		{"footer value on several lines", "docs: x\n\nRefs: #1\nsuite de la valeur\nReviewed-by: Bob", nil, true, "docs", "", false},
		{"scope with slash and dot", "chore(deps/go.mod): met à jour chi", nil, true, "chore", "deps/go.mod", false},
		{"scope with dash and underscore", "ci(release_v2-x): x", nil, true, "ci", "release_v2-x", false},
		{"acronym is not a capital", "docs: README plus clair", nil, true, "docs", "", false},
		{"acronym with digit", "feat: HTTP2 par défaut", nil, true, "feat", "", false},
		{"acronym before a dot", "docs: CHANGELOG.md à jour", nil, true, "docs", "", false},
		{"squash suffix is ignored", "feat: ajoute la connexion (#12)", nil, true, "feat", "", false},
		{"squash suffix with body", "feat(ui): x (#3)\n\n* feat: a\n* fix: b", nil, true, "feat", "ui", false},
		{"leading blank lines", "\n\n  feat: x\n", nil, true, "feat", "", false},
		{"crlf", "fix: x\r\n\r\nbody\r\n", nil, true, "fix", "", false},
		{"description with colon", "feat: ajoute la route /api: v2", nil, true, "feat", "", false},
		{"exactly 50", "feat: " + strings.Repeat("a", 44), nil, true, "feat", "", false},

		// Bloquants.
		{"empty", "", []string{CodeEmpty}, false, "", "", false},
		{"blank", " \n\t\n", []string{CodeEmpty}, false, "", "", false},
		{"squash suffix only", "(#12)", []string{CodeEmpty}, false, "", "", false},
		{"empty description", "feat:", []string{CodeEmpty}, false, "feat", "", false},
		{"forbidden update", "update", []string{CodeForbidden}, false, "", "", false},
		{"forbidden capitalized with period", "Update.", []string{CodeForbidden}, false, "", "", false},
		{"forbidden maj", "MAJ", []string{CodeForbidden}, false, "", "", false},
		{"forbidden test", "test", []string{CodeForbidden}, false, "", "", false},
		{"forbidden fix", "fix", []string{CodeForbidden}, false, "", "", false},
		{"forbidden modif", "modif", []string{CodeForbidden}, false, "", "", false},
		{"forbidden depot", "Dépôt 1", []string{CodeForbidden}, false, "", "", false},
		{"forbidden depot spaces", "dépôt   1", []string{CodeForbidden}, false, "", "", false},
		{"forbidden dot", ".", []string{CodeForbidden}, false, "", "", false},
		{"forbidden initial commit", "Initial commit", []string{CodeForbidden}, false, "", "", false},
		{"forbidden with squash suffix", "update (#4)", []string{CodeForbidden}, false, "", "", false},
		{"wip prefix", "WIP: page d'accueil", []string{CodeForbidden}, false, "", "", false},
		{"wip bracket", "[wip] page", []string{CodeForbidden}, false, "", "", false},
		{"wip alone", "wip", []string{CodeForbidden}, false, "", "", false},
		{"fixup", "fixup! feat: x", []string{CodeForbidden}, false, "", "", false},
		{"squash", "squash! feat: x", []string{CodeForbidden}, false, "", "", false},
		{"github update", "Update README.md", []string{CodeGitHubDefault}, false, "", "", false},
		{"github create", "Create index.html", []string{CodeGitHubDefault}, false, "", "", false},
		{"github delete", "Delete old.txt", []string{CodeGitHubDefault}, false, "", "", false},
		{"github rename", "Rename a.md to b.md", []string{CodeGitHubDefault}, false, "", "", false},
		{"github upload", "Add files via upload", []string{CodeGitHubDefault}, false, "", "", false},
		{"github update squashed", "Update README.md (#7)", []string{CodeGitHubDefault}, false, "", "", false},
		{"no type", "ajout de la page d'accueil", []string{CodeNoType}, false, "", "", false},
		{"no type capitalized sentence", "Fix typo in readme", []string{CodeNoType}, false, "", "", false},
		{"no type accent", "Modifié: la page", []string{CodeNoType}, false, "", "", false},
		{"unknown type", "feature: ajoute la connexion", []string{CodeUnknownType}, false, "feature", "", false},
		{"unknown type wording", "ajout: page", []string{CodeUnknownType}, false, "ajout", "", false},
		{"type case", "Feat: ajoute la connexion", []string{CodeTypeCase}, false, "Feat", "", false},
		{"type case upper", "FIX(api): x", []string{CodeTypeCase}, false, "FIX", "api", false},
		{"scope uppercase", "feat(API): x", []string{CodeScope}, false, "feat", "API", false},
		{"scope empty", "feat(): x", []string{CodeScope}, false, "feat", "", false},
		{"scope with space", "feat(ui kit): x", []string{CodeScope}, false, "feat", "ui kit", false},
		{"separator no space", "feat:ajoute", []string{CodeSeparator}, false, "feat", "", false},
		{"separator space before", "feat : ajoute", []string{CodeSeparator}, false, "feat", "", false},
		{"separator two spaces", "feat:  ajoute", []string{CodeSeparator}, false, "feat", "", false},
		{"separator missing", "feat ajoute la connexion", []string{CodeSeparator}, false, "feat", "", false},
		{"separator dash", "feat - ajoute la connexion", []string{CodeSeparator}, false, "feat", "", false},
		{"separator scope without colon", "feat(ui) ajoute", []string{CodeSeparator}, false, "feat", "", false},
		{"separator and case", "Feat(ui) ajoute", []string{CodeTypeCase, CodeSeparator}, false, "Feat", "", false},
		{"trailing period", "fix: corrige le calcul.", []string{CodeTrailingPeriod}, false, "fix", "", false},
		{"trailing period and suffix", "fix: corrige le calcul. (#5)", []string{CodeTrailingPeriod}, false, "fix", "", false},
		{"body separator", "fix: x\ncorps collé", []string{CodeBodySeparator}, false, "fix", "", false},
		{"length hard", tooLong, []string{CodeLengthHard}, false, "feat", "", false},
		{"length counted in runes", "feat: " + strings.Repeat("é", 44), nil, true, "feat", "", false},
		{"several blocking", "Feat:Ajoute la connexion.", []string{CodeTypeCase, CodeSeparator, CodeTrailingPeriod, CodeDescCapitalized}, false, "Feat", "", false},

		// Avertissements seuls : conforme.
		{"desc capitalized", "feat: Ajoute la connexion", []string{CodeDescCapitalized}, true, "feat", "", false},
		{"desc single capital", "feat: I18n", []string{CodeDescCapitalized}, true, "feat", "", false},
		{"length soft", long, []string{CodeLengthSoft}, true, "feat", "", false},
		{"footer token with spaces", "fix: x\n\nCloses #1\nSigned off by: Alice", []string{CodeFooterFormat}, true, "fix", "", false},
		{"footer no space after colon", "fix: x\n\nRefs: #1\nCloses:#2", []string{CodeFooterFormat}, true, "fix", "", false},
		{"breaking change lowercase", "feat: x\n\nBreaking change: plus de v1", []string{CodeFooterFormat}, true, "feat", "", false},
		{"breaking change no space", "feat: x\n\nBREAKING CHANGE:plus de v1", []string{CodeFooterFormat}, true, "feat", "", false},
		{"url in footer block is a continuation", "fix: x\n\nRefs: #1\nhttps://example.com/a", nil, true, "fix", "", false},
		{"prose paragraph is not a footer block", "fix: x\n\nNote de version: rien.\nVoir la doc.", nil, true, "fix", "", false},
		{"git revert", "Revert \"feat: ajoute la connexion\"\n\nThis reverts commit abc123.", []string{CodeGitRevert}, true, "", "", false},
		{"git revert squashed", "Revert \"feat: x\" (#9)", []string{CodeGitRevert}, true, "", "", false},
		{"git reapply", "Reapply \"feat: x\"", []string{CodeGitRevert}, true, "", "", false},
		{"git revert body glued", "Revert \"feat: x\"\nThis reverts commit abc.", []string{CodeBodySeparator, CodeGitRevert}, false, "", "", false},
	}
	p := NewParser(DefaultRules())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.Parse(tt.message)
			want := tt.codes
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got.Codes, want) {
				t.Errorf("codes = %v, want %v", got.Codes, want)
			}
			if got.Conform() != tt.conform {
				t.Errorf("Conform = %v, want %v", got.Conform(), tt.conform)
			}
			if got.Type != tt.typ || got.Scope != tt.scope || got.Breaking != tt.breaking {
				t.Errorf("type/scope/breaking = %q/%q/%v, want %q/%q/%v", got.Type, got.Scope, got.Breaking, tt.typ, tt.scope, tt.breaking)
			}
		})
	}
}

// TestCodesSorted vérifie que les codes suivent l'ordre d'AllCodes, quel que soit l'ordre de détection.
func TestCodesSorted(t *testing.T) {
	got := ParseCommit("Feat:Ajoute une très longue description qui dépasse largement soixante-douze caractères.\ncorps", DefaultRules())
	want := []string{CodeTypeCase, CodeSeparator, CodeTrailingPeriod, CodeBodySeparator, CodeLengthHard, CodeDescCapitalized}
	if !reflect.DeepEqual(got.Codes, want) {
		t.Errorf("codes = %v, want %v", got.Codes, want)
	}
}

func TestRules(t *testing.T) {
	t.Run("scopes restricted", func(t *testing.T) {
		r := DefaultRules()
		r.Scopes = []string{"api", "ui"}
		p := NewParser(r)
		if got := p.Parse("feat(api): x"); !got.Conform() {
			t.Errorf("allowed scope: %v", got.Codes)
		}
		if got := p.Parse("feat: x"); !got.Conform() {
			t.Errorf("no scope: %v", got.Codes)
		}
		if got := p.Parse("feat(db): x"); !reflect.DeepEqual(got.Codes, []string{CodeScope}) {
			t.Errorf("other scope: %v", got.Codes)
		}
	})
	t.Run("custom types", func(t *testing.T) {
		r := DefaultRules()
		r.Types = []string{"feat", "wip-x"}
		p := NewParser(r)
		if got := p.Parse("fix: x"); !reflect.DeepEqual(got.Codes, []string{CodeUnknownType}) {
			t.Errorf("fix not admitted: %v", got.Codes)
		}
		if got := p.Parse("feat: x"); !got.Conform() {
			t.Errorf("feat: %v", got.Codes)
		}
	})
	t.Run("no types", func(t *testing.T) {
		p := NewParser(Rules{})
		if got := p.Parse("feat: x"); got.Conform() || !reflect.DeepEqual(got.Codes, []string{CodeUnknownType}) {
			t.Errorf("no types: %v", got.Codes)
		}
		if got := p.Parse(": x"); got.Conform() {
			t.Errorf("empty type accepted: %v", got.Codes)
		}
	})
	t.Run("lengths", func(t *testing.T) {
		r := DefaultRules()
		r.MaxSubjectSoft, r.MaxSubjectHard = 10, 20
		p := NewParser(r)
		if got := p.Parse("feat: abcd"); len(got.Codes) != 0 {
			t.Errorf("10: %v", got.Codes)
		}
		if got := p.Parse("feat: abcde"); !reflect.DeepEqual(got.Codes, []string{CodeLengthSoft}) {
			t.Errorf("11: %v", got.Codes)
		}
		if got := p.Parse("feat: abcdefghijklmno"); !reflect.DeepEqual(got.Codes, []string{CodeLengthHard}) {
			t.Errorf("21: %v", got.Codes)
		}
		r.MaxSubjectSoft, r.MaxSubjectHard = 0, 0
		if got := ParseCommit("feat: "+strings.Repeat("a", 200), r); len(got.Codes) != 0 {
			t.Errorf("no limit: %v", got.Codes)
		}
	})
	t.Run("forbidden list replaced", func(t *testing.T) {
		r := DefaultRules()
		r.Forbidden = []string{"Corrections diverses"}
		p := NewParser(r)
		if got := p.Parse("corrections  diverses"); !reflect.DeepEqual(got.Codes, []string{CodeForbidden}) {
			t.Errorf("custom forbidden: %v", got.Codes)
		}
		if got := p.Parse("update"); !reflect.DeepEqual(got.Codes, []string{CodeNoType}) {
			t.Errorf("update no longer forbidden: %v", got.Codes)
		}
		if got := p.Parse("wip"); !reflect.DeepEqual(got.Codes, []string{CodeForbidden}) {
			t.Errorf("wip prefix always forbidden: %v", got.Codes)
		}
	})
}

func TestDefaults(t *testing.T) {
	if got := DefaultTypes(); len(got) != 11 || got[0] != "feat" || got[10] != "revert" {
		t.Errorf("DefaultTypes = %v", got)
	}
	r := DefaultRules()
	if r.MaxSubjectSoft != 50 || r.MaxSubjectHard != 72 || len(r.Scopes) != 0 {
		t.Errorf("DefaultRules = %+v", r)
	}
	// Les listes renvoyées sont des copies.
	DefaultTypes()[0] = "x"
	if DefaultTypes()[0] != "feat" {
		t.Error("DefaultTypes shares its slice")
	}
	for _, f := range DefaultForbidden() {
		if normalize(f) != f {
			t.Errorf("forbidden %q is not normalized", f)
		}
	}
}

func TestSeverity(t *testing.T) {
	blocking := map[string]bool{
		CodeEmpty: true, CodeForbidden: true, CodeGitHubDefault: true, CodeNoType: true, CodeUnknownType: true,
		CodeTypeCase: true, CodeScope: true, CodeSeparator: true, CodeTrailingPeriod: true,
		CodeBodySeparator: true, CodeLengthHard: true,
	}
	seen := map[string]bool{}
	for _, c := range AllCodes {
		if seen[c] {
			t.Errorf("duplicate code %q", c)
		}
		seen[c] = true
		want := SeverityWarning
		if blocking[c] {
			want = SeverityBlocking
		}
		if SeverityOf(c) != want || IsBlocking(c) != blocking[c] {
			t.Errorf("SeverityOf(%q) = %v", c, SeverityOf(c))
		}
	}
	if len(AllCodes) != 15 {
		t.Errorf("%d codes", len(AllCodes))
	}
	if SeverityOf("nope") != 0 || IsBlocking("nope") || Severity(0).String() != "" {
		t.Error("unknown code")
	}
	if SeverityBlocking.String() != "blocking" || SeverityWarning.String() != "warning" {
		t.Error("severity names")
	}
}

func TestPredicates(t *testing.T) {
	squash := map[string]bool{
		"feat: x (#12)":                true,
		"feat: x (#12)\n\n* feat: a":   true,
		"\nfix: y(#3)":                 true,
		"feat: x\n\nSee (#12)":         false, // suffixe dans le corps seulement
		"feat: x (#12) suite":          false,
		"feat: x (#a)":                 false,
		"feat: x #12":                  false,
		"":                             false,
		"Merge pull request #4 from a": false,
	}
	for m, want := range squash {
		if got := IsSquashSubject(m); got != want {
			t.Errorf("IsSquashSubject(%q) = %v", m, got)
		}
	}
	merge := map[string]bool{
		"Merge pull request #12 from alice/feat-x\n\nfeat: x": true,
		"Merge branch 'main' into feat-x":                     true,
		"Merge branch 'dev' of https://github.com/o/r":        true,
		"Merge remote-tracking branch 'origin/main'":          true,
		"merge branch 'main'":                                 false,
		"Merge dev into main":                                 false,
		"Merged the login page":                               false,
		"feat: merge two lists":                               false,
	}
	for m, want := range merge {
		if got := IsDefaultMergeMessage(m); got != want {
			t.Errorf("IsDefaultMergeMessage(%q) = %v", m, got)
		}
	}
	revert := map[string]bool{
		"Revert \"feat: x\"\n\nThis reverts commit abc.": true,
		"Revert \"feat: x\" (#9)":                        true,
		"Reapply \"Revert \\\"feat: x\\\"\"":             true,
		"revert: annule x":                               false,
		"Revert feat x":                                  false,
	}
	for m, want := range revert {
		if got := IsGitRevert(m); got != want {
			t.Errorf("IsGitRevert(%q) = %v", m, got)
		}
	}
	if got := Subject("\n  feat: x (#1)  \nbody"); got != "feat: x (#1)" {
		t.Errorf("Subject = %q", got)
	}
	if got := StripSquashSuffix("feat: x (#1)"); got != "feat: x" {
		t.Errorf("StripSquashSuffix = %q", got)
	}
}

func TestValidTypeAndScope(t *testing.T) {
	for _, tc := range []struct {
		s          string
		typ, scope bool
	}{
		{"feat", true, true},
		{"fix", true, true},
		{"Feat", false, false},
		{"", false, false},
		{"ci2", false, true},
		{"web-ui", false, true},
		{"docs/fr", false, true},
		{"v1.2", false, true},
		{"a_b", false, true},
		{"0", false, true},
		{"-x", false, false},
		{"api ", false, false},
	} {
		if got := ValidType(tc.s); got != tc.typ {
			t.Errorf("ValidType(%q) = %v, want %v", tc.s, got, tc.typ)
		}
		if got := ValidScope(tc.s); got != tc.scope {
			t.Errorf("ValidScope(%q) = %v, want %v", tc.s, got, tc.scope)
		}
	}
}
