package provider

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMessagesInFrench checks a few key messages of the package in French: the messages come from
// the i18n catalog, in the process language at display time. Not parallel: it forces the process
// language, restored at the end (English set back by TestMain).
func TestMessagesInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	if got := ErrNotFound.Error(); got != "dépôt introuvable ou compte sans accès" {
		t.Errorf("ErrNotFound = %q", got)
	}
	if got := ErrUnauthorized.Error(); !strings.Contains(got, "gh auth login") || !strings.Contains(got, " : ") {
		t.Errorf("ErrUnauthorized = %q", got)
	}
	// French punctuation of *APIError: a space before " : ".
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
	// Sentinels stay comparable in both languages: errors.Is goes through the wrapping.
	if !strings.Contains((&APIError{Op: "op", Err: ErrUnknownBase}).Error(), "force-push") {
		t.Errorf("wrapped ErrUnknownBase = %q", ErrUnknownBase)
	}
}
