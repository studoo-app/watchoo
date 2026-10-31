package github

import (
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// TestMessagesFrancais vérifie quelques messages clés du paquet en français : les messages viennent
// du catalogue i18n, dans la langue du processus au moment de l'affichage. Non parallèle : il force
// la langue du processus, restaurée à la fin (l'anglais remis par TestMain).
func TestMessagesFrancais(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	// L'opération d'une erreur d'API, composée à la création, est traduite.
	if got := i18n.T("provider.op.repo", "alice", "projet"); got != "lecture du dépôt alice/projet" {
		t.Errorf("op dépôt = %q", got)
	}
	if got := i18n.T("provider.github.op.commits", "alice", "projet"); got != "liste des commits de alice/projet" {
		t.Errorf("op commits = %q", got)
	}
	// La nature d'un échec de transport est traduite.
	if got := failureNature(trLookalike{}); got != "délai dépassé" {
		t.Errorf("failureNature = %q", got)
	}
	// Les sentinelles du paquet provider sont traduites dans la langue du processus.
	if got := provider.ErrUnauthorized.Error(); !strings.Contains(got, "token GitHub refusé") ||
		!strings.Contains(got, "gh auth login") {
		t.Errorf("ErrUnauthorized = %q", got)
	}
}
