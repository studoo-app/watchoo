package model

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxContributorLabelRunes est la longueur maximale d'un nom donné à un contributeur (1.8.0), en
// caractères ; la même borne est vérifiée par le schéma (contributors.label, migration 00022).
const MaxContributorLabelRunes = 100

// Erreurs de NormalizeContributorLabel : l'appelant les traduit dans la langue de l'interface.
var (
	ErrContributorLabelInvalid = errors.New("contributor label: invalid UTF-8 or control character")
	ErrContributorLabelTooLong = errors.New("contributor label: too long")
)

// NormalizeContributorLabel vérifie un nom donné à un contributeur et le renvoie débarrassé de ses
// espaces de bord : texte UTF-8 valide, sans caractère de contrôle, de MaxContributorLabelRunes
// caractères au plus. Une chaîne vide (ou faite d'espaces) est valide : elle retire le nom donné.
// Fonction pure.
func NormalizeContributorLabel(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case !utf8.ValidString(v), strings.IndexFunc(v, unicode.IsControl) >= 0:
		return "", ErrContributorLabelInvalid
	case utf8.RuneCountInString(v) > MaxContributorLabelRunes:
		return "", ErrContributorLabelTooLong
	}
	return v, nil
}

// CommitName renvoie le nom connu par les commits à montrer à côté d'un nom donné (1.8.0) : Name quand
// Label est renseigné et qu'il en diffère ; vide sinon (le nom affiché est alors déjà Name, ou le login).
func (c Contributor) CommitName() string {
	if c.Label == "" || c.Name == c.Label {
		return ""
	}
	return c.Name
}
