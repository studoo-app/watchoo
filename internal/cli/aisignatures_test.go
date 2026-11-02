package cli

// AI signatures (1.1.0) in "watchoo log" and "watchoo contributors", in text and in JSON.

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

func TestAISignaturesOutput(t *testing.T) {
	h := newSyncedWorkspace(t)
	addCommits(h.Provider, aliceRepoID, "main", providertest.SHA("a2"), fakeCommit{seed: "ai-signed", ago: 30 * time.Minute,
		author: alicePerson, msg: "Ajoute le cache\n\nCo-authored-by: Cursor <cursoragent@cursor.com>"})
	h.run("sync").mustSucceed(t)

	// Text: the tool on the commit, "-" otherwise, the explanation under the table.
	r := h.run("log", "--repo", "alice/projet", "--author", "alice-gh").mustSucceed(t)
	assertContains(t, "log", r.stdout, "Cursor", "its absence proves nothing")
	row := tableRow(t, r.stdout, "30 min ago")
	if row[5] != "Cursor" {
		t.Errorf("signed row = %q", row)
	}
	r = h.run("contributors", "--repo", "alice/projet").mustSucceed(t)
	assertContains(t, "contributors", r.stdout, "1/", "its absence proves nothing")

	// No signature in the result: no column value and no explanation.
	r = h.run("log", "--author", "bob@example.test").mustSucceed(t)
	assertNotContains(t, "log without a signature", r.stdout, "proves nothing", "Cursor")

	// JSON: tool identifier and kind, never null.
	var logs struct {
		Commits []struct {
			Message      string `json:"message"`
			AIScanned    bool   `json:"ai_scanned"`
			AISignatures []struct {
				Tool string `json:"tool"`
				Kind string `json:"kind"`
			} `json:"ai_signatures"`
		} `json:"commits"`
	}
	r = h.run("log", "--author", "alice-gh", "--json").mustSucceed(t)
	if err := json.Unmarshal([]byte(r.stdout), &logs); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range logs.Commits {
		if !c.AIScanned || c.AISignatures == nil {
			t.Errorf("commit %q: scanned %v, signatures %v", c.Message, c.AIScanned, c.AISignatures)
		}
		if len(c.AISignatures) == 1 && c.AISignatures[0].Tool == "cursor" && c.AISignatures[0].Kind == "coauthor_trailer" {
			found = true
		}
	}
	if !found {
		t.Errorf("signed commit missing:\n%s", r.stdout)
	}
	r = h.run("contributors", "--repo", "alice/projet", "--json").mustSucceed(t)
	assertContains(t, "contributors --json", r.stdout, `"ai_signed_commits": 1`, `"tool": "cursor"`, `"commits": 1`)
	assertNotContains(t, "contributors --json", r.stdout, `"ai_tools": null`, "cursoragent")
}

func TestAINoteInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()
	assertContains(t, "note", i18n.T("cli.sync.ai_note"), "son absence ne prouve rien")
	if got := i18n.T("cli.sync.header_ai"); got != "IA" {
		t.Errorf("header = %q", got)
	}
}
