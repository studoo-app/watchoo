package sync

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain sets the process language for the whole package: the expected texts of the tests (error
// messages coming from the provider in particular) are written in English, whatever WATCHOO_LANG and
// the machine locale (see documentation/en/09-testing.md, "Languages in tests"). No restore: English
// stays the language of the tests.
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
