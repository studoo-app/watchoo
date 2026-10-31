package activity

import (
	"os"
	"testing"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// TestMain fixe la langue des messages pour tous les tests du paquet : l'anglais, indépendamment
// de la locale de la machine. Les tests du français la changent eux-mêmes (SetProcessForTest).
func TestMain(m *testing.M) {
	i18n.SetProcessForTest(i18n.English)
	os.Exit(m.Run())
}
