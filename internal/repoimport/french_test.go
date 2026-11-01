package repoimport

import (
	"errors"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// The messages follow the process language: French typography (space before ":" and ";").
// Not parallel: the process language is shared.
func TestMessagesFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	if _, err := ParseRef(""); err == nil || err.Error() != "dépôt vide : attendu owner/repo ou https://github.com/owner/repo" {
		t.Errorf("ParseRef(\"\") = %v", err)
	}
	_, _, err := Parse(strings.NewReader("repo,flemme,dead\nalice/projet,x,\nbob/projet,10,5\n"))
	if err == nil || !strings.HasPrefix(err.Error(), "2 lignes en erreur :\n  ligne 2 : flemme : \"x\" : nombre entier de jours") ||
		!strings.Contains(err.Error(), "ligne 3 : seuil dead (5 j) : doit être strictement supérieur au seuil flemme (10 j)") {
		t.Errorf("Parse = %v", err)
	}
	_, _, err = Parse(strings.NewReader("# rien\n"))
	if !errors.Is(err, ErrNoEntry) || err.Error() != "1 ligne en erreur :\n  ligne 1 : aucun dépôt dans le fichier" {
		t.Errorf("empty file: %v", err)
	}
	if err := ValidateLabel("a\x01b"); err == nil || err.Error() != "label : caractère de contrôle interdit" {
		t.Errorf("ValidateLabel = %v", err)
	}
}
