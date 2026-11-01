package i18n

import (
	"os"
	"testing"
)

// TestDetect checks the selection of the CLI and TUI language: WATCHOO_LANG first (an unknown value
// is ignored), then the system locale (LC_ALL, LC_MESSAGES, LANG), then English.
func TestDetect(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want Lang
	}{
		{"watchoo_lang_french", map[string]string{"WATCHOO_LANG": "fr"}, French},
		{"watchoo_lang_english", map[string]string{"WATCHOO_LANG": "EN"}, English},
		{"watchoo_lang_unknown_locale_fr", map[string]string{"WATCHOO_LANG": "es", "LC_ALL": "fr_FR.UTF-8"}, French},
		{"locale_lc_all", map[string]string{"LC_ALL": "fr_BE.UTF-8"}, French},
		{"locale_lc_messages", map[string]string{"LC_MESSAGES": "fr_CH"}, French},
		{"locale_lang", map[string]string{"LANG": "fr_FR.UTF-8"}, French},
		{"locale_c_ignored_lang_fr", map[string]string{"LC_ALL": "C", "LANG": "fr_FR.UTF-8"}, French},
		{"locale_posix_ignored", map[string]string{"LC_ALL": "POSIX"}, English},
		{"locale_english", map[string]string{"LANG": "en_US.UTF-8"}, English},
		{"locale_empty", map[string]string{"LANG": ""}, English},
		{"nothing", nil, English},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			getenv := func(name string) string { return c.env[name] }
			if got := Detect(getenv); got != c.want {
				t.Errorf("Detect = %q, want %q", got, c.want)
			}
		})
	}
}

// replaceLookup replaces the catalog for the duration of the test.
func replaceLookup(t *testing.T, entries map[string]entry) {
	t.Helper()
	orig := lookupText
	t.Cleanup(func() { lookupText = orig })
	lookupText = func(key string) (entry, bool) {
		e, ok := entries[key]
		return e, ok
	}
}

// TestValid checks the validation of the workspace's lang setting.
func TestValid(t *testing.T) {
	for l, want := range map[Lang]bool{English: true, French: true, "es": false, "": false} {
		if got := Valid(l); got != want {
			t.Errorf("Valid(%q) = %v, want %v", l, got, want)
		}
	}
}

// TestFor checks that For returns the requested language, and English for an unknown language.
func TestFor(t *testing.T) {
	if got := For(French).Lang(); got != French {
		t.Errorf("For(french).Lang() = %q, want %q", got, French)
	}
	if got := For("es").Lang(); got != English {
		t.Errorf("For(\"es\").Lang() = %q, want %q (back to English)", got, English)
	}
}

// TestTranslator checks the translation: the translator's language first, otherwise the English
// text, otherwise the key itself; the arguments are formatted like fmt.Sprintf.
func TestTranslator(t *testing.T) {
	replaceLookup(t, map[string]entry{
		"test.both":    {"Hello %s!", "Bonjour %s !"},
		"test.noargs":  {"Welcome", "Bienvenue"},
		"test.only_en": {"English only", ""},
		"test.empty":   {"", ""},
	})
	en, fr := For(English), For(French)

	if got := fr.T("test.both", "monde"); got != "Bonjour monde !" {
		t.Errorf("French with argument = %q, want %q", got, "Bonjour monde !")
	}
	if got := en.T("test.both", "world"); got != "Hello world!" {
		t.Errorf("English with argument = %q, want %q", got, "Hello world!")
	}
	if got := fr.T("test.noargs"); got != "Bienvenue" {
		t.Errorf("French without argument = %q, want %q", got, "Bienvenue")
	}
	if got := fr.T("test.only_en"); got != "English only" {
		t.Errorf("missing French text = %q, want the English text %q", got, "English only")
	}
	if got := fr.T("test.empty"); got != "test.empty" {
		t.Errorf("key without text = %q, want the key itself %q", got, "test.empty")
	}
	if got := fr.T("test.inconnue"); got != "test.inconnue" {
		t.Errorf("unknown key = %q, want the key itself %q", got, "test.inconnue")
	}
}

// TestSetProcessForTest checks that the language forced for the tests takes precedence over the
// detection, and that restoring puts the detection back in place.
func TestSetProcessForTest(t *testing.T) {
	restore := SetProcessForTest(French)
	if got := Process().Lang(); got != French {
		t.Errorf("Process().Lang() = %q, want %q (forced language)", got, French)
	}
	if got := T("test.inconnue"); got != "test.inconnue" {
		t.Errorf("T = %q, want the key itself %q", got, "test.inconnue")
	}
	restore()
	// After restoring, the language goes back to the detection (the tests' environment contains
	// neither WATCHOO_LANG nor a French locale): French must no longer be forced.
	if got := Process().Lang(); got == French {
		env := map[string]string{}
		for _, name := range []string{"WATCHOO_LANG", "LC_ALL", "LC_MESSAGES", "LANG"} {
			env[name] = os.Getenv(name)
		}
		if l := Detect(func(name string) string { return env[name] }); l == French {
			t.Skip("the machine is in French: the detection gives French, which is correct")
		}
		t.Errorf("Process().Lang() = %q after restoring, want another language", got)
	}
}

// TestCatalog checks the completeness of the catalog: every key, added by the layers'
// catalog_*.go files, has a text in both languages.
func TestCatalog(t *testing.T) {
	for k, e := range catalog {
		if e[0] == "" {
			t.Errorf("key %q: missing English text", k)
		}
		if e[1] == "" {
			t.Errorf("key %q: missing French text", k)
		}
	}
}
