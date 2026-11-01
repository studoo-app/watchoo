package configshare_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMessagesInFrench checks a few key messages rendered in the forced French language (the rest
// of the package tests the English texts, forced by TestMain). Not parallel: it changes the process
// language.
func TestMessagesInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	if got := configshare.ErrInvalid.Error(); got != "fichier de configuration invalide" {
		t.Errorf("ErrInvalid = %q", got)
	}
	_, err := configshare.Parse(strings.NewReader(doc(`{"lang": "de"}`, `[{"repo": "a/b"}, {"repo": "A/B"}]`)))
	if !errors.Is(err, configshare.ErrInvalid) {
		t.Fatalf("Parse = %v, want ErrInvalid", err)
	}
	for _, want := range []string{
		"fichier de configuration invalide ; rien n'a été importé :",
		`settings.lang : langue "de" inconnue`,
		"dépôt n° 2 (A/B) : présent deux fois (déjà en n° 1)",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("message without %q:\n%v", want, err)
		}
	}
	_, err = configshare.Parse(strings.NewReader(`{"format": "watchoo-config", "version": 2}`))
	if !errors.Is(err, configshare.ErrNewerVersion) || !strings.Contains(err.Error(), "mettez Watchoo à jour") {
		t.Errorf("newer version = %v", err)
	}
}
