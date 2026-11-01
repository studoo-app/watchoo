package git

import (
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMessagesInFrench checks a few key messages of the package in French: the messages come from
// the i18n catalog, in the process language at display time. Not parallel: it forces the process
// language, restored at the end (English set back by TestMain).
func TestMessagesInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	if got := ErrGitNotFound.Error(); !strings.Contains(got, "git est introuvable") ||
		!strings.Contains(got, "https://git-scm.com") {
		t.Errorf("ErrGitNotFound = %q", got)
	}
	if got := i18n.T("provider.git.op.clone", "alice", "projet"); got != "clone du dépôt alice/projet" {
		t.Errorf("op clone = %q", got)
	}
	_, err := New(Options{})
	if got := err.Error(); got != "dépôts Git locaux : répertoire des clones non renseigné" {
		t.Errorf("New without directory = %q", got)
	}
	_, err = New(Options{Dir: t.TempDir(), Protocol: "ftp"})
	if !strings.Contains(err.Error(), "protocole \"ftp\" inconnu") {
		t.Errorf("New with unknown protocol = %q", err)
	}
}
