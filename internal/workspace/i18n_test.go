package workspace

// Test dedicated to French: a few key messages and the config.yaml template rendered in the forced
// language i18n.French (the rest of the package tests the English texts, forced by TestMain). This
// test is not parallel: it changes the process language.

import (
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestMessagesInFrench(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	// Sentinel errors, rendered at display time.
	if got := ErrNotFound.Error(); got != "aucun espace Watchoo ici, lancez « watchoo init »" {
		t.Errorf("ErrNotFound = %q", got)
	}
	if got := ErrAlreadyInitialized.Error(); got != "ce répertoire est déjà un espace Watchoo" {
		t.Errorf("ErrAlreadyInitialized = %q", got)
	}

	// Message of Load without a workspace, and of Validate with a too short interval.
	assertErrorContains(t, Load(nil), "chargement des réglages : espace non renseigné")
	cfg := DefaultConfig("espace")
	cfg.Sync.Interval = 10 * time.Minute
	assertErrorContains(t, cfg.Validate(),
		"sync.interval : 10m0s est inférieur au minimum de 15 min (0 pour la synchro manuelle seule)")
	cfg = DefaultConfig("espace")
	cfg.Conventions.Readme.MinWords = 0
	cfg.Conventions.Docs.ArchivesScope = "docs"
	assertErrorContains(t, cfg.Validate(), "conventions.readme.min_words : doit valoir au moins 1 (reçu 0)",
		`conventions.docs.archives_scope : "docs" n'est pas admis (attendu "repository" ou "documentation")`)

	// French punctuation: a space before ":" and ";".
	if got := i18n.T("workspace.err.sep"); got != " : " {
		t.Errorf("separator = %q", got)
	}

	// config.yaml template in French: subtitle of the card and a few comments.
	tpl := ConfigTemplate(i18n.French, "licence-projet")
	for _, part := range []string{
		"Réglages de l'espace de supervision Watchoo.",
		"# Seul le nom de l'espace est actif. Les autres réglages sont en commentaire,",
		"# Ordre de priorité, du plus faible au plus fort :",
		"#   auto: true               # false : synchro manuelle seule",
		"#   interval: 30m            # durée (15m, 1h…), au moins 15m ; 0 : synchro manuelle seule",
		`name: "licence-projet"`,
		"#     min_words: 150         # nombre de mots en deçà duquel le README est trop court",
	} {
		if !strings.Contains(tpl, part) {
			t.Errorf("the French template does not contain %q:\n%s", part, tpl)
		}
	}
	// The header card carries the French subtitle.
	if !strings.Contains(tpl, "Réglages de l'espace de supervision  │") {
		t.Errorf("the header card does not carry the French subtitle:\n%s", tpl)
	}
}
