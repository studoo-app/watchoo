package aisignature

import (
	"reflect"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
)

func sig(tool, kind string) model.AISignature { return model.AISignature{Tool: tool, Kind: kind} }

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want []model.AISignature
	}{
		{"no signature", Input{Message: "feat: add login\n\nA body."}, []model.AISignature{}},
		{"claude trailer", Input{Message: "fix: x\n\nCo-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"},
			[]model.AISignature{sig("claude", KindCoauthorTrailer)}},
		{"key and address ignore case", Input{Message: "fix: x\n\nco-authored-BY: C <NoReply@Anthropic.com>\r\n"},
			[]model.AISignature{sig("claude", KindCoauthorTrailer)}},
		{"claude footer and trailer, one signature per kind", Input{Message: "fix: x\n\n" +
			"🤖 Generated with [Claude Code](https://claude.ai/code)\n\n" +
			"Co-authored-by: Claude <noreply@anthropic.com>\nCo-authored-by: Claude <noreply@anthropic.com>"},
			[]model.AISignature{sig("claude", KindCoauthorTrailer), sig("claude", KindFooter)}},
		{"cursor", Input{Message: "x\n\nCo-authored-by: Cursor <cursoragent@cursor.com>"},
			[]model.AISignature{sig("cursor", KindCoauthorTrailer)}},
		{"aider with a model in the name", Input{Message: "x\n\nCo-authored-by: aider (gpt-5) <aider@aider.chat>"},
			[]model.AISignature{sig("aider", KindCoauthorTrailer)}},
		{"codex", Input{Message: "x\n\nCo-authored-by: Codex <noreply@openai.com>"},
			[]model.AISignature{sig("codex", KindCoauthorTrailer)}},
		{"copilot noreply address", Input{Message: "x\n\nCo-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"},
			[]model.AISignature{sig("copilot", KindCoauthorTrailer)}},
		{"agent account by login", Input{Message: "x", AuthorLogin: "copilot-swe-agent[bot]"},
			[]model.AISignature{sig("copilot", KindAgentAccount)}},
		{"agent account by address", Input{Message: "x", AuthorEmails: []string{"alice@example.test", "cursoragent@cursor.com"}},
			[]model.AISignature{sig("cursor", KindAgentAccount)}},
		{"several tools", Input{Message: "x\n\nCo-authored-by: Cursor <cursoragent@cursor.com>\nCo-authored-by: Codex <codex@openai.com>"},
			[]model.AISignature{sig("codex", KindCoauthorTrailer), sig("cursor", KindCoauthorTrailer)}},

		// False positives that must stay silent.
		{"a person named Claude", Input{Message: "x\n\nCo-authored-by: Claude Dupont <claude.dupont@example.test>",
			AuthorLogin: "claude", AuthorEmails: []string{"claude@example.test"}}, []model.AISignature{}},
		{"tool names in free text", Input{Message: "fix cursor position\n\nClaude and Copilot are mentioned here."},
			[]model.AISignature{}},
		{"signature in the subject", Input{Message: "Co-authored-by: C <noreply@anthropic.com>"}, []model.AISignature{}},
		{"quoted trailer", Input{Message: "docs: x\n\n    Co-authored-by: C <noreply@anthropic.com>\n\t🤖 Generated with [Claude Code]"},
			[]model.AISignature{}},
		{"footer inside a sentence", Input{Message: "x\n\nThis was not generated with [Claude Code] at all."},
			[]model.AISignature{}},
		{"automation without AI", Input{Message: "x", AuthorLogin: "dependabot[bot]",
			AuthorEmails: []string{"49699333+dependabot[bot]@users.noreply.github.com"}}, []model.AISignature{}},
		{"trailer without an address", Input{Message: "x\n\nCo-authored-by: Claude"}, []model.AISignature{}},
		{"other noreply account", Input{Message: "x\n\nCo-authored-by: A <12+alice@users.noreply.github.com>"},
			[]model.AISignature{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Detect(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Detect = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToolName(t *testing.T) {
	if got := ToolName("copilot"); got != "GitHub Copilot" {
		t.Errorf("ToolName(copilot) = %q", got)
	}
	if got := ToolName("removed-tool"); got != "removed-tool" {
		t.Errorf("ToolName(removed-tool) = %q", got)
	}
}

// The catalog is compared in lowercase: an uppercase entry would never match.
func TestCatalogLowercase(t *testing.T) {
	seen := map[string]bool{}
	for _, tl := range catalog {
		if tl.id == "" || tl.name == "" || seen[tl.id] {
			t.Errorf("bad or duplicate entry %q", tl.id)
		}
		seen[tl.id] = true
		for _, group := range [][]string{tl.emails, tl.logins, tl.footers} {
			for _, v := range group {
				if v != strings.ToLower(v) {
					t.Errorf("%s: %q is not in lowercase", tl.id, v)
				}
			}
		}
	}
}
