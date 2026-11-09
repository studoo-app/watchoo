package conventions

import (
	"reflect"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestPRCategory(t *testing.T) {
	cases := []struct {
		name string
		pr   model.PullRequest
		cat  string
		ok   bool
	}{
		{"open", model.PullRequest{State: model.PullRequestOpen, AuthorLogin: "alice"}, PRCategoryOpen, true},
		{"merged", model.PullRequest{State: model.PullRequestClosed, Merged: true, AuthorLogin: "alice"}, PRCategoryMerged, true},
		{"merged former draft", model.PullRequest{State: model.PullRequestClosed, Merged: true, Draft: true}, PRCategoryMerged, true},
		{"closed", model.PullRequest{State: model.PullRequestClosed, AuthorLogin: "alice"}, PRSkipClosed, false},
		{"draft", model.PullRequest{State: model.PullRequestOpen, Draft: true, AuthorLogin: "alice"}, PRSkipDraft, false},
		{"bot", model.PullRequest{State: model.PullRequestOpen, AuthorLogin: "dependabot[bot]"}, PRSkipBot, false},
		{"bot merged", model.PullRequest{State: model.PullRequestClosed, Merged: true, AuthorLogin: "renovate[bot]"}, PRSkipBot, false},
		{"closed bot", model.PullRequest{State: model.PullRequestClosed, AuthorLogin: "dependabot[bot]"}, PRSkipClosed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cat, ok := PRCategory(tc.pr)
			if cat != tc.cat || ok != tc.ok {
				t.Errorf("PRCategory = %q %v, want %q %v", cat, ok, tc.cat, tc.ok)
			}
		})
	}
}

func TestCheckPRTitle(t *testing.T) {
	p := NewParser(DefaultRules())
	cases := []struct {
		name, title, branch string
		codes               []string
		def                 []string
		conform             bool
	}{
		{"conform", "feat(auth): add login page", "feat/login-page", nil, nil, true},
		{"conform breaking", "feat!: drop php 7", "feat/drop-php", nil, nil, true},
		{"no type", "Login page", "feat/login-page", []string{CodeNoType}, nil, false},
		{"unknown type", "feature: add login", "feature/login", []string{CodeUnknownType}, nil, false},
		{"capitalized warning", "feat: Add login", "x", []string{CodeDescCapitalized}, nil, true},
		{"trailing period", "fix: crash.", "fix/crash", []string{CodeTrailingPeriod}, nil, false},
		{"forbidden", "update", "", []string{CodeForbidden}, []string{PRTitleForbidden}, false},
		{"forbidden wip", "WIP: login", "feat/login", []string{CodeForbidden}, []string{PRTitleForbidden}, false},
		{"github default", "Update README.md", "patch-1", []string{CodeGitHubDefault}, []string{PRTitleGitHubDefault}, false},
		// Titre proposé par GitHub depuis le nom de la branche.
		{"head branch", "Feat/login page", "feat/login-page", []string{CodeNoType}, []string{PRTitleHeadBranch}, false},
		{"head branch dashes", "Login page", "login-page", []string{CodeNoType}, []string{PRTitleHeadBranch}, false},
		{"head branch underscores", "add  login PAGE", "add_login_page", []string{CodeNoType}, []string{PRTitleHeadBranch}, false},
		{"head branch conform title", "fix: crash", "fix: crash", nil, []string{PRTitleHeadBranch}, true},
		{"slash not normalized", "feat login page", "feat/login-page", []string{CodeSeparator}, nil, false},
		{"empty branch", "Login page", "", []string{CodeNoType}, nil, false},
		{"first line only", "feat: add login\nbody text without blank line", "x", nil, nil, true},
		{"empty", "", "", []string{CodeEmpty}, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := p.CheckPRTitle(tc.title, tc.branch)
			codes, def := tc.codes, tc.def
			if codes == nil {
				codes = []string{}
			}
			if def == nil {
				def = []string{}
			}
			if !reflect.DeepEqual(got.Codes, codes) || !reflect.DeepEqual(got.Default, def) {
				t.Errorf("codes = %v default = %v, want %v %v", got.Codes, got.Default, codes, def)
			}
			if got.Conform() != tc.conform || got.IsDefault() != (len(def) > 0) {
				t.Errorf("conform = %v default = %v", got.Conform(), got.IsDefault())
			}
		})
	}
	got := p.CheckPRTitle("feat(api)!: rename route", "")
	if got.Type != "feat" || got.Scope != "api" || !got.Breaking {
		t.Errorf("header = %+v", got)
	}
}
