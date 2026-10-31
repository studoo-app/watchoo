package workspace

// Test dédié au français : quelques messages clés et le modèle de config.yaml rendus dans la langue
// forcée i18n.French (le reste du paquet teste les textes anglais, forcée par TestMain). Ce test n'est
// pas parallèle : il change la langue du processus.

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestMessagesInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	// Erreurs sentinelles, rendues au moment de l'affichage.
	if got := ErrNotFound.Error(); got != "aucun espace Watchoo ici, lancez « watchoo init »" {
		t.Errorf("ErrNotFound = %q", got)
	}
	if got := ErrAlreadyInitialized.Error(); got != "ce répertoire est déjà un espace Watchoo" {
		t.Errorf("ErrAlreadyInitialized = %q", got)
	}

	// Message de Load sans espace, et de Validate avec un intervalle trop court.
	assertErrorContains(t, Load(nil), "chargement des réglages : espace non renseigné")
	cfg := DefaultConfig("espace")
	cfg.Sync.Interval = 10 * time.Minute
	assertErrorContains(t, cfg.Validate(),
		"sync.interval : 10m0s est inférieur au minimum de 15 min (0 pour la synchro manuelle seule)")

	// Ponctuation française : une espace avant « : » et « ; ».
	if got := i18n.T("workspace.err.sep"); got != " : " {
		t.Errorf("séparateur = %q", got)
	}

	// Modèle de config.yaml en français : sous-titre de la carte et quelques commentaires.
	tpl := ConfigTemplate(i18n.French, "licence-projet")
	for _, part := range []string{
		"Réglages de l'espace de supervision Watchoo.",
		"# Seul le nom de l'espace est actif. Les autres réglages sont en commentaire,",
		"# Ordre de priorité, du plus faible au plus fort :",
		"#   auto: true               # false : synchro manuelle seule",
		"#   interval: 30m            # durée (15m, 1h…), au moins 15m ; 0 : synchro manuelle seule",
		`name: "licence-projet"`,
	} {
		if !strings.Contains(tpl, part) {
			t.Errorf("le modèle français ne contient pas %q :\n%s", part, tpl)
		}
	}
	// La carte d'en-tête porte le sous-titre français.
	if !strings.Contains(tpl, "Réglages de l'espace de supervision  │") {
		t.Errorf("la carte d'en-tête ne porte pas le sous-titre français :\n%s", tpl)
	}
}
