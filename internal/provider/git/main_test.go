package git

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain forces the process language for the whole package: the tests' expected texts are
// written in English, regardless of WATCHOO_LANG and the machine's locale (see
// documentation/en/09-testing.md, "Languages in tests"). No restoring: English stays the
// language of the tests; the test dedicated to French forces its own language and restores it.
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
