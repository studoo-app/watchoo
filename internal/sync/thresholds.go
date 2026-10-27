package sync

// Seuils d'activité propres à un dépôt (réglage du web ; « repo add » et les imports passent par
// AddRepositories).

import (
	"context"
	"fmt"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/repoimport"
)

// SetThresholds remplace les seuils d'activité propres au dépôt id : nil ou 0 = seuil de l'espace
// (activity.*), sinon un nombre de jours sans commit. Les valeurs sont vérifiées comme celles de
// « repo add » (repoimport.ValidateThresholds : de 1 à repoimport.MaxThresholdDays, Dead strictement
// supérieur à Flemme quand les deux sont donnés) ; une valeur invalide est une erreur et rien n'est
// écrit. Aucun appel à la forge. Renvoie le dépôt à jour ; erreur enveloppant store.ErrNotFound si le
// dépôt n'existe pas.
func (e *Engine) SetThresholds(ctx context.Context, id int64, flemme, dead *int) (model.Repository, error) {
	if err := repoimport.ValidateThresholds(flemme, dead); err != nil {
		return model.Repository{}, err
	}
	repo, err := e.store.SetRepositoryThresholds(ctx, id, thresholdValue(flemme, nil), thresholdValue(dead, nil))
	if err != nil {
		return model.Repository{}, fmt.Errorf("seuils d'activité du dépôt : %w", err)
	}
	e.logger.Info("seuils d'activité du dépôt modifiés", "repo", repo.FullName())
	return repo, nil
}
