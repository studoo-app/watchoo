package cli

import (
	"encoding/json"
	"fmt"
	"io"
)

// writeJSON écrit v en JSON indenté, suivi d'une fin de ligne. Les caractères non ASCII (« », accents)
// et HTML sont écrits tels quels.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("écriture de la sortie JSON : %w", err)
	}
	return nil
}

// warn affiche un avertissement sur w (la sortie d'erreur), en texte comme en mode --json, pour que
// la sortie standard reste exploitable par un script.
func warn(w io.Writer, msg string) {
	fmt.Fprintf(w, "attention : %s\n", msg)
}

// hintError est une erreur dont le message, destiné à l'utilisateur, dit quoi faire ; elle enveloppe
// l'erreur d'origine (pour errors.Is) sans en répéter le texte.
type hintError struct {
	msg string
	err error
}

func (e *hintError) Error() string { return e.msg }
func (e *hintError) Unwrap() error { return e.err }
