package store

// Réglages d'un dépôt modifiables par l'utilisateur (repo add / import / remove --keep-history, seuils
// d'activité du web).

import (
	"context"
	"database/sql"
	"errors"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// UpdateRepositorySettings remplace le label et les motifs de branches du dépôt id (patterns nil ou
// vide = toutes les branches ; écrits avec JoinBranchPatterns) et renvoie le dépôt à jour. Erreur
// enveloppant ErrNotFound si le dépôt n'existe pas.
func (s *Store) UpdateRepositorySettings(ctx context.Context, id int64, label string, patterns []string) (model.Repository, error) {
	row, err := s.queries.UpdateRepositorySettings(ctx, db.UpdateRepositorySettingsParams{
		Label:          label,
		BranchPatterns: JoinBranchPatterns(patterns),
		ID:             id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Repository{}, wrapErr(i18n.T("store.repo.id", id), ErrNotFound)
		}
		return model.Repository{}, wrapErr(i18n.T("store.repo.update", id), err)
	}
	return repositoryFromDB(row)
}

// SetRepositoryThresholds remplace les seuils d'activité propres au dépôt id (nil = valeur globale ;
// une valeur doit valoir au moins 1, contrainte CHECK du schéma) et renvoie le dépôt à jour. La
// cohérence des deux seuils est vérifiée en amont (repoimport.ValidateThresholds). Erreur enveloppant
// ErrNotFound si le dépôt n'existe pas.
func (s *Store) SetRepositoryThresholds(ctx context.Context, id int64, flemme, dead *int) (model.Repository, error) {
	row, err := s.queries.SetRepositoryThresholds(ctx, db.SetRepositoryThresholdsParams{
		FlemmeAfterDays: nullInt(flemme),
		DeadAfterDays:   nullInt(dead),
		ID:              id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Repository{}, wrapErr(i18n.T("store.repo.id", id), ErrNotFound)
		}
		return model.Repository{}, wrapErr(i18n.T("store.repo.thresholds", id), err)
	}
	return repositoryFromDB(row)
}

// SetRepositoryConventionsSince remplace la date de début des conventions propre au dépôt id (date
// AAAA-MM-JJ ; vide = date de l'espace, NULL en base) et renvoie le dépôt à jour. La date est validée en
// amont (repoimport.ParseConventionsSince) ; une date invalide est refusée ici aussi. Erreur enveloppant
// ErrNotFound si le dépôt n'existe pas.
func (s *Store) SetRepositoryConventionsSince(ctx context.Context, id int64, date string) (model.Repository, error) {
	since, err := nullSince(date)
	if err != nil {
		return model.Repository{}, wrapErr(i18n.T("store.repo.conventions_since_set", id), err)
	}
	row, err := s.queries.SetRepositoryConventionsSince(ctx, db.SetRepositoryConventionsSinceParams{
		ConventionsSince: since,
		ID:               id,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Repository{}, wrapErr(i18n.T("store.repo.id", id), ErrNotFound)
		}
		return model.Repository{}, wrapErr(i18n.T("store.repo.conventions_since_set", id), err)
	}
	return repositoryFromDB(row)
}

// SetRepositoryEnabled active (true) ou arrête (false) le suivi du dépôt id, sans toucher à son
// historique. Erreur enveloppant ErrNotFound si le dépôt n'existe pas.
func (s *Store) SetRepositoryEnabled(ctx context.Context, id int64, enabled bool) error {
	n, err := s.queries.SetRepositoryEnabled(ctx, db.SetRepositoryEnabledParams{Enabled: enabled, ID: id})
	if err != nil {
		return wrapErr(i18n.T("store.repo.enabled", id), err)
	}
	if n == 0 {
		return wrapErr(i18n.T("store.repo.id", id), ErrNotFound)
	}
	return nil
}
