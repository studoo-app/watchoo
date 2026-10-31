package i18n

import (
	"fmt"
	"os"
	"testing"
)

// TestDetect vérifie la sélection de la langue de la CLI et du TUI : WATCHOO_LANG d'abord (une
// valeur inconnue est ignorée), puis la locale du système (LC_ALL, LC_MESSAGES, LANG), puis
// l'anglais.
func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Lang
	}{
		{"watchoo_lang_français", map[string]string{"WATCHOO_LANG": "fr"}, French},
		{"watchoo_lang_anglais", map[string]string{"WATCHOO_LANG": "EN"}, English},
		{"watchoo_lang_inconnue_locale_fr", map[string]string{"WATCHOO_LANG": "es", "LC_ALL": "fr_FR.UTF-8"}, French},
		{"locale_lc_all", map[string]string{"LC_ALL": "fr_BE.UTF-8"}, French},
		{"locale_lc_messages", map[string]string{"LC_MESSAGES": "fr_CH"}, French},
		{"locale_lang", map[string]string{"LANG": "fr_FR.UTF-8"}, French},
		{"locale_c_ignoree_lang_fr", map[string]string{"LC_ALL": "C", "LANG": "fr_FR.UTF-8"}, French},
		{"locale_posix_ignoree", map[string]string{"LC_ALL": "POSIX"}, English},
		{"locale_anglaise", map[string]string{"LANG": "en_US.UTF-8"}, English},
		{"locale_vide", map[string]string{"LANG": ""}, English},
		{"rien", nil, English},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := func(name string) string { return c.env[name] }
			if got := Detect(getenv); got != c.want {
				t.Errorf("Detect = %q, attendu %q", got, c.want)
			}
		})
	}
}

// replaceLookup remplace le catalogue pour la durée du test.
func replaceLookup(t *testing.T, entries map[string]entry) {
	t.Helper()
	orig := lookupText
	t.Cleanup(func() { lookupText = orig })
	lookupText = func(key string) (entry, bool) {
		e, ok := entries[key]
		return e, ok
	}
}

// TestValid vérifie la validation du réglage lang de l'espace de supervision.
func TestValid(t *testing.T) {
	for l, want := range map[Lang]bool{English: true, French: true, "es": false, "": false} {
		if got := Valid(l); got != want {
			t.Errorf("Valid(%q) = %v, attendu %v", l, got, want)
		}
	}
}

// TestFor vérifie que For renvoie bien la langue demandée, et l'anglais pour une langue inconnue.
func TestFor(t *testing.T) {
	if got := For(French).Lang(); got != French {
		t.Errorf("For(french).Lang() = %q, attendu %q", got, French)
	}
	if got := For("es").Lang(); got != English {
		t.Errorf("For(\"es\").Lang() = %q, attendu %q (retour à l'anglais)", got, English)
	}
}

// TestTranslator vérifie la traduction : la langue du traducteur d'abord, sinon le texte anglais,
// sinon la clé elle-même ; les arguments sont formatés comme fmt.Sprintf.
func TestTranslator(t *testing.T) {
	replaceLookup(t, map[string]entry{
		"test.both":    {"Hello %s!", "Bonjour %s !"},
		"test.noargs":  {"Welcome", "Bienvenue"},
		"test.only_en": {"English only", ""},
		"test.empty":   {"", ""},
	})
	en, fr := For(English), For(French)

	if got := fr.T("test.both", "monde"); got != "Bonjour monde !" {
		t.Errorf("français avec argument = %q, attendu %q", got, "Bonjour monde !")
	}
	if got := en.T("test.both", "world"); got != "Hello world!" {
		t.Errorf("anglais avec argument = %q, attendu %q", got, "Hello world!")
	}
	if got := fr.T("test.noargs"); got != "Bienvenue" {
		t.Errorf("français sans argument = %q, attendu %q", got, "Bienvenue")
	}
	if got := fr.T("test.only_en"); got != "English only" {
		t.Errorf("texte français manquant = %q, attendu le texte anglais %q", got, "English only")
	}
	if got := fr.T("test.empty"); got != "test.empty" {
		t.Errorf("clé sans texte = %q, attendu la clé elle-même %q", got, "test.empty")
	}
	if got := fr.T("test.inconnue"); got != "test.inconnue" {
		t.Errorf("clé inconnue = %q, attendu la clé elle-même %q", got, "test.inconnue")
	}
}

// TestSetProcessForTest vérifie que la langue forcée pour les tests prime sur la détection, et que
// la restauration remet la détection en place.
func TestSetProcessForTest(t *testing.T) {
	restore := SetProcessForTest(French)
	if got := Process().Lang(); got != French {
		t.Errorf("Process().Lang() = %q, attendu %q (langue forcée)", got, French)
	}
	if got := T("test.inconnue"); got != "test.inconnue" {
		t.Errorf("T = %q, attendu la clé elle-même %q", got, "test.inconnue")
	}
	restore()
	// Après restauration, la langue revient à la détection (l'environnement des tests ne contient
	// ni WATCHOO_LANG ni une locale française) : le français ne doit plus être forcé.
	if got := Process().Lang(); got == French {
		env := map[string]string{}
		for _, name := range []string{"WATCHOO_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
			env[name] = os.Getenv(name)
		}
		if l := Detect(func(name string) string { return env[name] }); l == French {
			t.Skip("la machine est en français : la détection donne le français, ce qui est correct")
		}
		t.Errorf("Process().Lang() = %q après restauration, attendu une autre langue", got)
	}
}

// TestCatalog vérifie l'exhaustivité du catalogue : aucune clé déclarée deux fois, chaque clé a un
// texte dans les deux langues. Les fichiers catalog_*.go des couches ajoutent leurs clés ici.
func TestCatalog(t *testing.T) {
	seen := map[string]string{}
	for i, part := range catalogParts() {
		for k, e := range part {
			if prev := seen[k]; prev != "" {
				t.Errorf("clé %q déclarée deux fois : %s et part %d", k, prev, i)
				continue
			}
			seen[k] = fmt.Sprintf("part %d", i)
			if e[0] == "" {
				t.Errorf("clé %q : texte anglais manquant", k)
			}
			if e[1] == "" {
				t.Errorf("clé %q : texte français manquant", k)
			}
		}
	}
}
