package provider

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain force la langue du processus pour tout le paquet : les textes attendus des tests sont
// écrits en anglais, indépendamment de WATCHOO_LANG et de la locale de la machine (voir
// documentation/en/09-testing.md, « Languages in tests »). Pas de restauration : l'anglais reste la
// langue des tests ; le test dédié au français force sa propre langue avec sa restauration.
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
