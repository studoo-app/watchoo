package sync_test

// AI signatures (1.1.0): every sync analyzes the waiting commits, without any request to the forge.

import (
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

func TestSyncDetectsAISignatures(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "ai")
	person := provider.Person{Name: secretName, Email: secretEmail, Login: secretLogin, ID: 4242}
	agent := provider.Person{Name: "Copilot", Login: "copilot-swe-agent[bot]", ID: 9001}
	add := func(seed, message string, author provider.Person, parents ...string) string {
		sha := providertest.SHA(seed)
		e.fake.AddCommits(1, provider.Commit{SHA: sha, Message: message, URL: "https://github.com/" + testOwner + "/ai/commit/" + sha,
			Author: author, Committer: author, AuthoredAt: epoch, CommittedAt: epoch, Parents: parents})
		return sha
	}
	signed := add("signed", secretMessage+"\n\nCo-authored-by: Claude <noreply@anthropic.com>", person)
	plain := add("plain", secretMessage, person, signed)
	bot := add("bot", secretMessage, agent, plain)
	e.fake.SetBranch(1, "main", bot)

	r := e.syncOK(repo.ID)
	if len(r.Warnings) != 0 {
		t.Errorf("warnings: %v", r.Warnings)
	}
	checks := []struct {
		sha  string
		want []model.AISignature
	}{
		{signed, []model.AISignature{{Tool: "claude", Kind: aisignature.KindCoauthorTrailer}}},
		{plain, []model.AISignature{}},
		{bot, []model.AISignature{{Tool: "copilot", Kind: aisignature.KindAgentAccount}}},
	}
	for _, c := range checks {
		v := e.commitView(repo.ID, c.sha)
		if !v.AIScanned || len(v.AISignatures) != len(c.want) {
			t.Fatalf("commit %s: scanned %v, signatures %v, want %v", provider.ShortSHA(c.sha), v.AIScanned, v.AISignatures, c.want)
		}
		for i := range c.want {
			if v.AISignatures[i] != c.want[i] {
				t.Errorf("commit %s: signatures %v, want %v", provider.ShortSHA(c.sha), v.AISignatures, c.want)
			}
		}
	}

	// A commit from an older version of the rules is analyzed again by a sync without any new commit.
	if _, err := e.st.DB().ExecContext(t.Context(), `DELETE FROM commit_ai_signatures`); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.DB().ExecContext(t.Context(), `UPDATE commits SET ai_rules_version = 0`); err != nil {
		t.Fatal(err)
	}
	if v := e.commitView(repo.ID, signed); v.AIScanned {
		t.Fatal("commit still marked as analyzed")
	}
	if r := e.syncOK(repo.ID); r.NewCommits != 0 {
		t.Errorf("%d new commits on the second sync", r.NewCommits)
	}
	if v := e.commitView(repo.ID, signed); !v.AIScanned || len(v.AISignatures) != 1 {
		t.Errorf("after the new analysis: %+v", v)
	}
}

// The reports of the sync hold no signature text, and the logs neither message nor address (forbidden
// of helpers_test.go is checked by the env itself); the catalog addresses stay out too.
func TestAISignaturesNotLogged(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "ai")
	sha := providertest.SHA("signed")
	person := provider.Person{Name: secretName, Email: secretEmail, Login: secretLogin, ID: 4242}
	e.fake.AddCommits(1, provider.Commit{SHA: sha, Message: secretMessage + "\n\nCo-authored-by: Cursor <cursoragent@cursor.com>",
		Author: person, Committer: person, AuthoredAt: epoch, CommittedAt: epoch})
	e.fake.SetBranch(1, "main", sha)
	e.syncOK(repo.ID)
	if logs := e.logs.String(); strings.Contains(logs, "cursoragent") {
		t.Errorf("signature in the logs:\n%s", logs)
	}
}
