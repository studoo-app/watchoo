package web

// AI signatures (1.2.0): badge on the commits, totals on the contributors pages, the same wording
// everywhere ("declared", never "written by").

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/store"
)

func TestAISignaturesPages(t *testing.T) {
	e := newTestEnv(t)
	alice := store.Identity{Name: "Alice Martin", Email: "alice@example.test"}
	id := e.addRepo(1, "alice", "")
	signed := e.commitWrite(1, alice, 2*time.Hour, "commit signé\n\nCo-authored-by: Claude <noreply@anthropic.com>", "main")
	plain := e.commitWrite(2, alice, time.Hour, "commit sans signature", "main")
	e.applySync(id, "alice", signed, plain)

	// Before the analysis: no badge, "—" for the totals.
	// Since 1.7.0 the ranking shows the AI score (diagnosis) instead of the signatures.
	assertContains(t, e.get("/contributors"), "AI score", "the diagnosis is disabled")
	assertNotContains(t, e.get("/contributors"), "AI signatures")
	assertNotContains(t, e.get(repoPath(id)), "AI: Claude")

	ctx := t.Context()
	todo, err := e.store.CommitsToScanForAI(ctx, id, aisignature.RulesVersion, 10)
	if err != nil {
		t.Fatal(err)
	}
	var results []store.AIScanResult
	for _, c := range todo {
		results = append(results, store.AIScanResult{SHA: c.SHA, Signatures: aisignature.Detect(aisignature.Input{Message: c.Message})})
	}
	if err := e.store.WriteAIScans(ctx, id, aisignature.RulesVersion, results); err != nil {
		t.Fatal(err)
	}

	// Commits: one badge, on the signed commit (history and dashboard feed).
	w := e.get(repoPath(id))
	assertStatus(t, w, http.StatusOK)
	if n := strings.Count(w.Body.String(), "AI: Claude"); n != 1 {
		t.Errorf("%d badges in the history, want 1", n)
	}
	assertContains(t, e.get("/"), "AI: Claude")

	// Contributor page: signed / analyzed, the tool and the warning.
	note := "its absence proves nothing"
	contributor := e.contributorIDs()["Alice Martin"]
	w = e.get(fmt.Sprintf("/contributors/%d", contributor))
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "AI signatures", "1 / 2", "Claude 1", note, "AI: Claude")
	assertNotContains(t, w, "noreply@anthropic.com")
}
