package store

// Réglages d'un dépôt modifiables par l'utilisateur (repo add / import / remove --keep-history, seuils
// d'activité du web).

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

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
			return model.Repository{}, fmt.Errorf("dépôt %d : %w", id, ErrNotFound)
		}
		return model.Repository{}, fmt.Errorf("mise à jour du dépôt %d : %w", id, err)
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
			return model.Repository{}, fmt.Errorf("dépôt %d : %w", id, ErrNotFound)
		}
		return model.Repository{}, fmt.Errorf("seuils du dépôt %d : %w", id, err)
	}
	return repositoryFromDB(row)
}

// SetRepositoryEnabled active (true) ou arrête (false) le suivi du dépôt id, sans toucher à son
// historique. Erreur enveloppant ErrNotFound si le dépôt n'existe pas.
func (s *Store) SetRepositoryEnabled(ctx context.Context, id int64, enabled bool) error {
	n, err := s.queries.SetRepositoryEnabled(ctx, db.SetRepositoryEnabledParams{Enabled: enabled, ID: id})
	if err != nil {
		return fmt.Errorf("suivi du dépôt %d : %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("dépôt %d : %w", id, ErrNotFound)
	}
	return nil
}
