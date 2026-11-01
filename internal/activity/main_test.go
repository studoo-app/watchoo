package activity

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain sets the language of the messages for all the tests of the package: English, regardless
// of the machine's locale. The French tests change it themselves (SetProcessForTest).
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
