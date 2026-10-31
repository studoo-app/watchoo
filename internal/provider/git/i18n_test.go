package git

import (
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMessagesFrancais vérifie quelques messages clés du paquet en français : les messages viennent
// du catalogue i18n, dans la langue du processus au moment de l'affichage. Non parallèle : il force
// la langue du processus, restaurée à la fin (l'anglais remis par TestMain).
func TestMessagesFrancais(t *testing.T) {
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
		t.Errorf("New sans répertoire = %q", got)
	}
	_, err = New(Options{Dir: t.TempDir(), Protocol: "ftp"})
	if !strings.Contains(err.Error(), "protocole \"ftp\" inconnu") {
		t.Errorf("New protocole inconnu = %q", err)
	}
}
