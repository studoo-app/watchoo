package provider

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMessagesFrancais vérifie quelques messages clés du paquet en français : les messages viennent
// du catalogue i18n, dans la langue du processus au moment de l'affichage. Non parallèle : il force
// la langue du processus, restaurée à la fin (l'anglais remis par TestMain).
func TestMessagesFrancais(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	if got := ErrNotFound.Error(); got != "dépôt introuvable ou compte sans accès" {
		t.Errorf("ErrNotFound = %q", got)
	}
	if got := ErrUnauthorized.Error(); !strings.Contains(got, "gh auth login") || !strings.Contains(got, " : ") {
		t.Errorf("ErrUnauthorized = %q", got)
	}
	// Ponctuation française de l'*APIError : espace avant « : ».
	if got := (&APIError{Op: "op", Err: ErrUnexpected}).Error(); got != "op : réponse inattendue de GitHub" {
		t.Errorf("APIError = %q", got)
	}
	reset := time.Date(2026, 10, 26, 14, 4, 5, 0, time.UTC)
	if got := (&RateLimitError{Reset: reset}).Error(); got != "quota de l'API GitHub épuisé jusqu'au 2026-10-26 14:04:05 UTC" {
		t.Errorf("RateLimitError = %q", got)
	}
	if got := (&SSORequiredError{Organization: "ecole-dev"}).Error(); !strings.Contains(got, "« ecole-dev »") ||
		!strings.Contains(got, "authentification unique") {
		t.Errorf("SSORequiredError = %q", got)
	}
	if got := ValidateName("a/b"); !strings.Contains(got.Error(), "nom de dépôt") {
		t.Errorf("ValidateName = %q", got)
	}
	// Les sentinelles restent comparables dans les deux langues : errors.Is traverse l'emballage.
	if !strings.Contains((&APIError{Op: "op", Err: ErrUnknownBase}).Error(), "force-push") {
		t.Errorf("ErrUnknownBase enveloppée = %q", ErrUnknownBase)
	}
}
