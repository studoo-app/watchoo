package github

import (
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// TestMessagesInFrench checks a few key messages of the package in French: the messages come from
// the i18n catalog, in the process language at display time. Not parallel: it forces the process
// language, restored at the end (English set back by TestMain).
func TestMessagesInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	// The operation of an API error, composed at creation, is translated.
	if got := i18n.T("provider.op.repo", "alice", "projet"); got != "lecture du dépôt alice/projet" {
		t.Errorf("op repository = %q", got)
	}
	if got := i18n.T("provider.github.op.commits", "alice", "projet"); got != "liste des commits de alice/projet" {
		t.Errorf("op commits = %q", got)
	}
	// The nature of a transport failure is translated.
	if got := failureNature(trLookalike{}); got != "délai dépassé" {
		t.Errorf("failureNature = %q", got)
	}
	// The sentinels of the provider package are translated into the process language.
	if got := provider.ErrUnauthorized.Error(); !strings.Contains(got, "token GitHub refusé") ||
		!strings.Contains(got, "gh auth login") {
		t.Errorf("ErrUnauthorized = %q", got)
	}
}
