package conventions

import (
	"errors"
	"strings"
	"time"
)

// Formats de la date de début (conventions.since, repositories.conventions_since).
const (
	// SinceInputLayout est la forme saisie : une date du calendrier, AAAA-MM-JJ.
	SinceInputLayout = "2006-01-02"
	// SinceStoredLayout est la forme stockée : format fixe des dates de Watchoo, minuit UTC de la date.
	SinceStoredLayout = "2006-01-02T15:04:05Z"
)

// ErrInvalidSince signale une date de début qui n'est ni AAAA-MM-JJ ni au format stocké.
var ErrInvalidSince = errors.New("conventions: invalid since date")

// NormalizeSince ramène une date de début saisie (AAAA-MM-JJ) ou déjà stockée à sa forme stockée, minuit
// UTC de la date du calendrier (« 2026-09-01T00:00:00Z »). Une chaîne vide reste vide (pas de date). Seule
// la date du calendrier compte : l'heure d'une valeur stockée est ignorée.
func NormalizeSince(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	d, err := parseSinceDate(s)
	if err != nil {
		return "", err
	}
	return d.Format(SinceStoredLayout), nil
}

// SinceDate ramène une date de début saisie (AAAA-MM-JJ) ou stockée à sa forme saisie, la date du
// calendrier (« 2026-09-01 ») ; une chaîne vide reste vide. C'est la forme des structs métier et de
// l'affichage : la date ne dépend d'aucun fuseau.
func SinceDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	d, err := parseSinceDate(s)
	if err != nil {
		return "", err
	}
	return d.Format(SinceInputLayout), nil
}

// SinceAt renvoie l'instant de début d'une date stockée ou saisie : minuit, dans le fuseau loc (celui de
// l'espace), de la date du calendrier. Zéro pour une chaîne vide (depuis le début). loc nil vaut UTC.
func SinceAt(s string, loc *time.Location) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, nil
	}
	d, err := parseSinceDate(s)
	if err != nil {
		return time.Time{}, err
	}
	if loc == nil {
		loc = time.UTC
	}
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc), nil
}

// EffectiveSince renvoie la date de début d'un dépôt : la sienne si elle est renseignée, sinon celle de
// l'espace (toutes deux en forme saisie ou stockée ; vide : depuis le début).
func EffectiveSince(workspace, repo string) string {
	if strings.TrimSpace(repo) != "" {
		return repo
	}
	return workspace
}

// EffectiveSinceAt renvoie l'instant de début des conventions d'un dépôt : sa date propre (repo,
// model.Repository.ConventionsSince) si elle est renseignée, sinon celle de l'espace (workspace,
// conventions.since), à minuit dans le fuseau loc de l'espace (SinceAt). Zéro si aucune date (depuis
// le début).
func EffectiveSinceAt(workspace, repo string, loc *time.Location) (time.Time, error) {
	return SinceAt(EffectiveSince(workspace, repo), loc)
}

// parseSinceDate lit une date AAAA-MM-JJ ou au format stocké et renvoie minuit UTC de sa date.
func parseSinceDate(s string) (time.Time, error) {
	for _, layout := range []string{SinceInputLayout, SinceStoredLayout} {
		if t, err := time.Parse(layout, s); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, ErrInvalidSince
}
