package tui

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain runs the package tests in English, whatever the machine's locale or WATCHOO_LANG; the
// French tests switch the language themselves (i18n.SetProcessForTest).
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
