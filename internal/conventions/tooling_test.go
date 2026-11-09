package conventions

import (
	"reflect"
	"testing"
)

func toolingChecks(files map[string]string) []FileCheck {
	return DetectTooling(treeOf(files), fileOpts())
}

func TestToolingCommitlint(t *testing.T) {
	cases := []struct {
		files   map[string]string
		configs int
		extends int
	}{
		{map[string]string{"commitlint.config.js": "module.exports = {extends: ['@commitlint/config-conventional']}"}, 1, 1},
		{map[string]string{".commitlintrc.json": `{"rules": {}}`}, 1, 0},
		{map[string]string{".commitlintrc": "extends: ['@commitlint/config-conventional']"}, 1, 1},
		{map[string]string{"front/app/commitlint.config.ts": "x"}, 1, 0},
		{map[string]string{"package.json": `{"commitlint": {"extends": ["@commitlint/config-conventional"]}}`}, 1, 1},
		{map[string]string{"front/package.json": `{"devDependencies": {"@commitlint/cli": "^19"}}`}, 1, 0},
		{map[string]string{"package.json": `{"dependencies": {"@commitlint/cli": "^19"}}`}, 1, 0},
	}
	for _, tc := range cases {
		c := byID(t, toolingChecks(tc.files), CriterionTL1)
		wantCheck(t, c, CriterionConform)
		if c.Counts[CountConfigs] != tc.configs || c.Counts[CountExtends] != tc.extends {
			t.Errorf("%v : TL1 = %v", tc.files, c.Counts)
		}
	}
	for _, files := range []map[string]string{
		{"a/b/c/commitlint.config.js": "x"},                                 // trop profond
		{"node_modules/x/commitlint.config.js": "x"},                        // dépendance versionnée
		{"package.json": `{"devDependencies": {"@commitlint/types": "1"}}`}, // autre paquet
		{"package.json": "pas du JSON"},
		{"README.md": "commitlint"},
	} {
		wantCheck(t, byID(t, toolingChecks(files), CriterionTL1), CriterionNonconform, FileNotFound)
	}
	// package.json non lu et rien d'autre : inconnu.
	tree := FileTree{Paths: []string{"package.json"}, Blobs: map[string]Blob{"package.json": {TooLarge: true}}}
	if c := DetectTooling(tree, fileOpts())[0]; c.Status != CriterionUnknown || c.Reason != UnknownBlobUnreadable {
		t.Errorf("TL1 = %+v", c)
	}
}

func TestToolingHooks(t *testing.T) {
	bit := func(tools ...string) int {
		m := 0
		for _, tool := range tools {
			for i, known := range AllHookTools {
				if known == tool {
					m |= 1 << i
				}
			}
		}
		return m
	}
	cases := []struct {
		files map[string]string
		tools int
		codes []string
	}{
		{map[string]string{".husky/commit-msg": "npx commitlint --edit", "package.json": `{"scripts": {"prepare": "husky"}}`},
			bit(HookHusky), nil},
		{map[string]string{".husky/commit-msg": "x", "package.json": `{"scripts": {"test": "go test"}}`},
			bit(HookHusky), []string{FileHuskyNoPrepare}},
		{map[string]string{".husky/commit-msg": "x"}, bit(HookHusky), nil},
		{map[string]string{"front/.husky/commit-msg": "x", "front/package.json": `{"scripts": {"prepare": "cd .. && husky front/.husky"}}`},
			bit(HookHusky), nil},
		{map[string]string{".husky/pre-commit": "x"}, 0, []string{FileNotFound}},
		{map[string]string{"lefthook.yml": "commit-msg:\n  commands:\n    lint: {run: commitlint}\n"}, bit(HookLefthook), nil},
		{map[string]string{".lefthook.yaml": "pre-commit:\n  commands: {}\n"}, 0, []string{FileNotFound}},
		{map[string]string{".pre-commit-config.yaml": "default_install_hook_types: [commit-msg]\n"}, bit(HookPreCommit), nil},
		{map[string]string{".cz.toml": "[tool.commitizen]"}, bit(HookCommitizen), nil},
		{map[string]string{"pyproject.toml": "[tool.commitizen]\nname = \"cz_conventional_commits\""}, bit(HookCommitizen), nil},
		{map[string]string{"pyproject.toml": "[tool.poetry]"}, 0, []string{FileNotFound}},
		{map[string]string{"package.json": `{"config": {"commitizen": {"path": "cz-conventional-changelog"}}}`},
			bit(HookCommitizen), nil},
		{map[string]string{"cog.toml": "x", "lefthook.yml": "commit-msg: {}"}, bit(HookLefthook, HookCocogitto), nil},
	}
	for _, tc := range cases {
		c := byID(t, toolingChecks(tc.files), CriterionTL2)
		state := CriterionConform
		if tc.tools == 0 {
			state = CriterionNonconform
		}
		wantCheck(t, c, state, tc.codes...)
		if c.Counts[CountTools] != tc.tools {
			t.Errorf("%v : outils = %b, attendu %b", tc.files, c.Counts[CountTools], tc.tools)
		}
	}
	tree := FileTree{Paths: []string{"lefthook.yml"}}
	if c := DetectTooling(tree, fileOpts())[1]; c.Status != CriterionUnknown || c.Reason != UnknownBlobUnreadable {
		t.Errorf("TL2 = %+v", c)
	}
}

func TestToolingPRTemplate(t *testing.T) {
	charter := "## Contexte\n\n## Ce qui change\n\n## Comment tester\n\n## Captures\n\nCloses #\n"
	cases := []struct {
		files     map[string]string
		templates int
		sections  int
	}{
		{map[string]string{".github/pull_request_template.md": charter}, 1, 5},
		{map[string]string{".github/PULL_REQUEST_TEMPLATE.md": charter}, 1, 5},
		{map[string]string{"Pull_Request_Template.md": "## Description\n## How to test\n"}, 1, 2},
		{map[string]string{"docs/pull_request_template.md": "## Changes\nFixes #12\n"}, 1, 2},
		{map[string]string{".GitHub/PULL_REQUEST_TEMPLATE/feature.md": charter,
			".github/PULL_REQUEST_TEMPLATE/bug.md": "## Screenshots\n"}, 2, 5},
		{map[string]string{".github/pull_request_template.md": "<!-- Closes #1 -->\n```\n## Contexte\n```\n"}, 1, 0},
	}
	for _, tc := range cases {
		c := byID(t, toolingChecks(tc.files), CriterionTL3)
		wantCheck(t, c, CriterionConform)
		if c.Counts[CountTemplates] != tc.templates || c.Counts[CountSections] != tc.sections || c.Counts[CountExpected] != 5 {
			t.Errorf("%v : TL3 = %v", tc.files, c.Counts)
		}
	}
	for _, files := range []map[string]string{
		{"src/pull_request_template.md": charter},
		{".github/ISSUE_TEMPLATE/bug.md": charter},
		{".github/PULL_REQUEST_TEMPLATE/notes.txt": charter},
		{"README.md": charter},
	} {
		wantCheck(t, byID(t, toolingChecks(files), CriterionTL3), CriterionNonconform, FileNotFound)
	}
	if got := prSectionsIn([]byte(charter)); !reflect.DeepEqual(got, AllPRSections) {
		t.Errorf("sections = %q", got)
	}
}
