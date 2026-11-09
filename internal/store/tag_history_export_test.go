package store

// Écritures directes de l'historique des tags et de la branche par défaut, réservées aux tests du paquet :
// en production, seule ApplyRepoSync écrit dans tag_observations et branch_rewrites (si l'appelant a
// conventions.enabled).

import (
	"context"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// observeTagsForTest met à jour les observations des tags du dépôt repoID d'après la liste complète tags lue à
// l'instant at (voir RepoSyncWrite.ObserveTags), dans une transaction. À n'appeler qu'après une lecture
// des tags réussie et non tronquée.
func (s *Store) observeTagsForTest(ctx context.Context, repoID int64, tags []model.Tag, at time.Time) error {
	return s.writeTx(ctx, func(q *db.Queries) error {
		return observeTags(ctx, q, repoID, tags, at)
	})
}

// recordBranchRewriteForTest enregistre une réécriture observée de la branche par défaut (r.RepoID, à la date
// r.ObservedAt). La base refuse un dépôt inconnu, une branche vide, un SHA mal formé ou deux têtes égales.
func (s *Store) recordBranchRewriteForTest(ctx context.Context, r model.BranchRewrite) error {
	return s.writeTx(ctx, func(q *db.Queries) error {
		return insertBranchRewrite(ctx, q, r.RepoID, r, r.ObservedAt)
	})
}
