package web

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain sets the process language for every test of the package: English, whatever the locale
// of the machine (messages of the core packages, startup errors). The language of the pages comes
// from the lang setting of the test workspace: "en" by default (withLang changes it).
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
