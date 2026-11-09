package aisignature

import "testing"

// TestIsBot reprend la table du signal D3 du diagnostic (comportement conservé).
func TestIsBot(t *testing.T) {
	for name, want := range map[string]bool{
		"dependabot[bot]":        true,
		"Copilot[BOT]":           true,
		"copilot-swe-agent":      true,
		"copilot-swe-agent[bot]": true,
		"  renovate[bot]  ":      true,
		"github-actions[bot]":    true,
		"Alice":                  false,
		"":                       false,
		"   ":                    false,
		"robot":                  false,
		"copilot":                false, // compte d'agent du catalogue, pas un bot
		"bot":                    false,
		"[bot] alice":            false,
		"copilot-swe-agent-fake": false,
	} {
		if got := IsBot(name); got != want {
			t.Errorf("IsBot(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestIsGitHubCommitter(t *testing.T) {
	tests := []struct {
		login  string
		emails []string
		want   bool
	}{
		{"web-flow", nil, true},
		{"Web-Flow", nil, true},
		{"", []string{"noreply@github.com"}, true},
		{"", []string{"alice@example.com", "NoReply@GitHub.com "}, true},
		{"alice", []string{"alice@example.com"}, false},
		{"", []string{"123+alice@users.noreply.github.com"}, false},
		{"", nil, false},
		{"web-flow-bis", nil, false},
	}
	for _, tt := range tests {
		if got := IsGitHubCommitter(tt.login, tt.emails...); got != tt.want {
			t.Errorf("IsGitHubCommitter(%q, %v) = %v, want %v", tt.login, tt.emails, got, tt.want)
		}
	}
}
