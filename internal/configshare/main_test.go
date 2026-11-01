package configshare

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain sets the message language for every test of the package: English, whatever the locale of
// the machine. The French tests change it themselves (SetProcessForTest).
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
