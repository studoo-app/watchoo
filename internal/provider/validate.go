package provider

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Règles de validation, appliquées avant toute requête : go-github n'échappe pas owner et nom de dépôt
// dans les chemins, donc une valeur comme « ../x » ou « a/b » changerait la route appelée.
var (
	// ownerPattern : login d'utilisateur ou d'organisation GitHub (39 caractères au plus, lettres,
	// chiffres et tirets, sans tiret en tête).
	ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})$`)
	// namePattern : nom de dépôt GitHub (100 caractères au plus) ; « . » et « .. » sont refusés à part.
	namePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,100}$`)
	// shaPattern : SHA complet en minuscules, SHA-1 (40) ou SHA-256 (64).
	shaPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
)

// MaxBranchBytes est la longueur maximale d'un nom de branche accepté, en octets (garde-fou : le nom
// passe en paramètre de requête).
const MaxBranchBytes = 1024

// maxQuotedRunes borne la valeur invalide citée dans un message d'erreur.
const maxQuotedRunes = 60

// ValidateOwner vérifie un propriétaire (utilisateur ou organisation) : 1 à 39 caractères parmi
// A-Z a-z 0-9 et « - », sans « - » en tête. Erreur enveloppant ErrInvalidRef sinon.
func ValidateOwner(owner string) error {
	if !ownerPattern.MatchString(owner) {
		return fmt.Errorf("propriétaire de dépôt %s invalide : %w", quote(owner), ErrInvalidRef)
	}
	return nil
}

// ValidateName vérifie un nom de dépôt : 1 à 100 caractères parmi A-Z a-z 0-9 « . » « _ » « - », autre
// que « . » et « .. ». Erreur enveloppant ErrInvalidRef sinon.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) || name == "." || name == ".." {
		return fmt.Errorf("nom de dépôt %s invalide : %w", quote(name), ErrInvalidRef)
	}
	return nil
}

// ValidateRepo vérifie owner (ValidateOwner) puis name (ValidateName).
func ValidateRepo(owner, name string) error {
	if err := ValidateOwner(owner); err != nil {
		return err
	}
	return ValidateName(name)
}

// ValidateSHA vérifie un SHA de commit complet, en minuscules : 40 caractères hexadécimaux (SHA-1) ou
// 64 (SHA-256). Erreur enveloppant ErrInvalidRef sinon.
func ValidateSHA(sha string) error {
	if !shaPattern.MatchString(sha) {
		return fmt.Errorf("SHA de commit %s invalide : %w", quote(sha), ErrInvalidRef)
	}
	return nil
}

// ValidateBranch vérifie un nom de branche (ou de référence) : non vide, UTF-8 valide, au plus
// MaxBranchBytes octets, sans caractère de contrôle. Le reste (« / », espaces exclus par Git…) est
// accepté : le nom passe en paramètre de requête, encodé par le client. Erreur enveloppant
// ErrInvalidRef sinon.
func ValidateBranch(branch string) error {
	switch {
	case branch == "":
		return fmt.Errorf("nom de branche vide : %w", ErrInvalidRef)
	case len(branch) > MaxBranchBytes:
		return fmt.Errorf("nom de branche trop long (%d octets, %d au plus) : %w", len(branch), MaxBranchBytes, ErrInvalidRef)
	case !utf8.ValidString(branch) || strings.IndexFunc(branch, unicode.IsControl) >= 0:
		return fmt.Errorf("nom de branche %s invalide : %w", quote(branch), ErrInvalidRef)
	}
	return nil
}

// ValidateID vérifie un identifiant numérique de dépôt (strictement positif). Erreur enveloppant
// ErrInvalidRef sinon.
func ValidateID(id int64) error {
	if id <= 0 {
		return fmt.Errorf("identifiant de dépôt %d invalide : %w", id, ErrInvalidRef)
	}
	return nil
}

// quote cite une valeur reçue dans un message d'erreur : entre guillemets, caractères de contrôle et
// octets invalides échappés (strconv.Quote), raccourcie à maxQuotedRunes caractères.
func quote(s string) string {
	s = strings.ToValidUTF8(s, "�")
	if utf8.RuneCountInString(s) > maxQuotedRunes {
		s = string([]rune(s)[:maxQuotedRunes]) + "…"
	}
	return strconv.Quote(s)
}
