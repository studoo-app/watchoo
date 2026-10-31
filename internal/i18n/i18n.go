// Package i18n fournit les textes affichés à l'utilisateur dans les deux langues de l'interface :
// l'anglais par défaut et le français. Sans dépendance : un catalogue de clés vers deux textes,
// formatés avec fmt.Sprintf (voir documentation/fr/03-architecture.md, « Langues et traductions »).
//
// La langue de la CLI et du TUI vient de la variable WATCHOO_LANG, sinon de la locale du système,
// sinon de l'anglais (Detect, Process) ; celle du web vient du réglage lang de l'espace de
// supervision (For). Les textes eux-mêmes sont dans les fichiers catalog_*.go, un par domaine
// d'affichage.
package i18n

import (
	"fmt"
	"os"
	"strings"
	"sync"
)

// Lang est une langue prise en charge par l'interface.
type Lang string

const (
	// English est la langue par défaut de l'interface.
	English Lang = "en"
	// French est la seconde langue de l'interface.
	French Lang = "fr"
)

// entry regroupe les deux textes d'une clé : l'anglais (position 0), puis le français (1).
type entry [2]string

// Valid dit si la langue est prise en charge (réglage lang de l'espace de supervision).
func Valid(l Lang) bool {
	return l == English || l == French
}

// Detect renvoie la langue des messages de la CLI et du TUI : la variable WATCHOO_LANG d'abord
// (« en » ou « fr », une autre valeur est ignorée), sinon la locale du système (LC_ALL,
// LC_MESSAGES, puis LANG ; préfixe « fr » → français, « C » et « POSIX » ignorés), sinon
// l'anglais. getenv est passé en paramètre pour les tests.
func Detect(getenv func(string) string) Lang {
	if l := parseSetting(getenv("WATCHOO_LANG")); l != "" {
		return l
	}
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if l := parseLocale(getenv(name)); l != "" {
			return l
		}
	}
	return English
}

// parseSetting lit un réglage de langue explicite ; une valeur vide ou inconnue donne une langue
// vide, que l'appelant ignore.
func parseSetting(v string) Lang {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case string(English):
		return English
	case string(French):
		return French
	}
	return ""
}

// parseLocale lit une locale du système (« fr_FR.UTF-8 ») et renvoie sa langue ; une valeur vide,
// « C », « POSIX » ou d'une autre langue donne une langue vide, que l'appelant ignore.
func parseLocale(v string) Lang {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	v, _, _ = strings.Cut(v, ".")     // « fr_FR.UTF-8 » → « fr_FR »
	v, _, _ = strings.Cut(v, "@")     // « fr@euro » → « fr »
	lang, _, _ := strings.Cut(v, "_") // « fr_FR » → « fr »
	switch strings.ToLower(lang) {
	case string(English):
		return English
	case string(French):
		return French
	}
	return ""
}

// Le traducteur du processus (CLI et TUI) est calculé une seule fois : la locale du système ne
// change pas en cours d'exécution.
var (
	processOnce sync.Once
	processTr   *Translator
)

// Process renvoie le traducteur de la langue du processus (CLI et TUI) : voir Detect.
func Process() *Translator {
	processOnce.Do(func() {
		processTr = For(Detect(os.Getenv))
	})
	return processTr
}

// T traduit une clé dans la langue du processus (voir Process). Fonction de confort pour les
// couches d'affichage qui n'ont pas de langue propre.
func T(key string, args ...any) string {
	return Process().T(key, args...)
}

// Translator traduit les clés du catalogue dans une langue.
type Translator struct {
	lang Lang
}

// For renvoie le traducteur pour la langue donnée ; une langue inconnue donne l'anglais.
func For(l Lang) *Translator {
	if !Valid(l) {
		l = English
	}
	return &Translator{lang: l}
}

// Lang renvoie la langue du traducteur.
func (t *Translator) Lang() Lang {
	return t.lang
}

// T renvoie le texte de la clé, formaté avec les arguments comme fmt.Sprintf. Une clé absente du
// catalogue, ou sans texte dans la langue du traducteur, retombe sur le texte anglais, puis sur la
// clé elle-même (jamais de panique).
func (t *Translator) T(key string, args ...any) string {
	text := t.text(key)
	if text == "" {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}

// text renvoie le texte brut de la clé : dans la langue du traducteur, sinon en anglais ; une clé
// inconnue donne un texte vide.
func (t *Translator) text(key string) string {
	e, ok := lookupText(key)
	if !ok {
		return ""
	}
	if v := e[t.index()]; v != "" {
		return v
	}
	return e[0]
}

// index renvoie la position du texte de la langue dans une entrée.
func (t *Translator) index() int {
	if t.lang == French {
		return 1
	}
	return 0
}

// lookupText renvoie l'entrée du catalogue pour une clé, ou false si la clé est inconnue. Variable
// de fonction pour pouvoir la remplacer dans les tests du mécanisme.
var lookupText = func(key string) (entry, bool) {
	e, ok := catalog[key]
	return e, ok
}
