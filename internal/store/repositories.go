package store

// Dépôts suivis (table repositories) : méthodes de Store en types model, conversions depuis et vers
// les types générés par sqlc.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// CreateRepository ajoute le dépôt r (r.ID est ignoré) et renvoie le dépôt enregistré, avec son ID.
// Renvoie une erreur enveloppant ErrAlreadyExists si (provider, provider_id) ou (provider, owner, name)
// existe déjà. Les dates zéro sont enregistrées à NULL.
func (s *Store) CreateRepository(ctx context.Context, r model.Repository) (model.Repository, error) {
	row, err := s.queries.CreateRepository(ctx, db.CreateRepositoryParams{
		Provider:        r.Provider,
		ProviderID:      r.ProviderID,
		Owner:           r.Owner,
		Name:            r.Name,
		Label:           r.Label,
		DefaultBranch:   r.DefaultBranch,
		URL:             r.URL,
		Stars:           int64(r.Stars),
		OpenIssues:      int64(r.OpenIssues),
		LastSyncedAt:    nullTime(r.LastSyncedAt),
		LastCommitAt:    nullTime(r.LastCommitAt),
		Archived:        r.Archived,
		FlemmeAfterDays: nullInt(r.FlemmeAfterDays),
		DeadAfterDays:   nullInt(r.DeadAfterDays),
		Enabled:         r.Enabled,
		BranchPatterns:  JoinBranchPatterns(r.BranchPatterns),
	})
	if err != nil {
		if isUniqueViolation(err) {
			return model.Repository{}, fmt.Errorf("dépôt %s : %w", r.FullName(), ErrAlreadyExists)
		}
		return model.Repository{}, fmt.Errorf("ajout du dépôt %s : %w", r.FullName(), err)
	}
	return repositoryFromDB(row)
}

// GetRepository renvoie le dépôt d'identifiant interne id, ou une erreur enveloppant ErrNotFound.
func (s *Store) GetRepository(ctx context.Context, id int64) (model.Repository, error) {
	row, err := s.queries.GetRepository(ctx, id)
	if err != nil {
		return model.Repository{}, repositoryLookupError(fmt.Sprintf("dépôt %d", id), err)
	}
	return repositoryFromDB(row)
}

// GetRepositoryByName renvoie le dépôt provider/owner/name (owner et name comparés sans tenir compte de
// la casse), ou une erreur enveloppant ErrNotFound.
func (s *Store) GetRepositoryByName(ctx context.Context, provider, owner, name string) (model.Repository, error) {
	row, err := s.queries.GetRepositoryByName(ctx, db.GetRepositoryByNameParams{
		Provider: provider,
		Owner:    owner,
		Name:     name,
	})
	if err != nil {
		return model.Repository{}, repositoryLookupError(fmt.Sprintf("dépôt %s/%s", owner, name), err)
	}
	return repositoryFromDB(row)
}

// GetRepositoryByProviderID renvoie le dépôt d'identifiant providerID sur la forge provider, ou une
// erreur enveloppant ErrNotFound.
func (s *Store) GetRepositoryByProviderID(ctx context.Context, provider string, providerID int64) (model.Repository, error) {
	row, err := s.queries.GetRepositoryByProviderID(ctx, db.GetRepositoryByProviderIDParams{
		Provider:   provider,
		ProviderID: providerID,
	})
	if err != nil {
		return model.Repository{}, repositoryLookupError(
			fmt.Sprintf("dépôt d'identifiant %d sur %s", providerID, provider), err)
	}
	return repositoryFromDB(row)
}

// ListRepositories renvoie tous les dépôts, triés par owner puis name ; une liste vide (non nil) si
// aucun dépôt n'est suivi.
func (s *Store) ListRepositories(ctx context.Context) ([]model.Repository, error) {
	rows, err := s.queries.ListRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("liste des dépôts : %w", err)
	}
	repos := make([]model.Repository, 0, len(rows))
	for _, row := range rows {
		r, err := repositoryFromDB(row)
		if err != nil {
			return nil, err
		}
		repos = append(repos, r)
	}
	return repos, nil
}

// DeleteRepository supprime le dépôt d'identifiant id et, par cascade, ses branches, commits,
// appartenances commit/branche et journaux de synchro. Renvoie une erreur enveloppant ErrNotFound si
// aucun dépôt n'a cet identifiant.
func (s *Store) DeleteRepository(ctx context.Context, id int64) error {
	n, err := s.queries.DeleteRepository(ctx, id)
	if err != nil {
		return fmt.Errorf("suppression du dépôt %d : %w", id, err)
	}
	if n == 0 {
		return fmt.Errorf("dépôt %d : %w", id, ErrNotFound)
	}
	return nil
}

// repositoryLookupError traduit l'erreur d'une lecture de dépôt : sql.ErrNoRows devient ErrNotFound,
// les autres erreurs sont enveloppées avec what (ex. « dépôt 42 »).
func repositoryLookupError(what string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s : %w", what, ErrNotFound)
	}
	return fmt.Errorf("lecture du %s : %w", what, err)
}

// repositoryFromDB convertit une ligne sqlc en model.Repository. Une date mal formée en base (ce que
// les contraintes CHECK rendent improbable) est signalée plutôt qu'ignorée.
func repositoryFromDB(row db.Repository) (model.Repository, error) {
	lastSyncedAt, err := parseNullTime(row.LastSyncedAt)
	if err != nil {
		return model.Repository{}, fmt.Errorf("dépôt %d, last_synced_at : %w", row.ID, err)
	}
	lastCommitAt, err := parseNullTime(row.LastCommitAt)
	if err != nil {
		return model.Repository{}, fmt.Errorf("dépôt %d, last_commit_at : %w", row.ID, err)
	}
	return model.Repository{
		ID:              row.ID,
		Provider:        row.Provider,
		ProviderID:      row.ProviderID,
		Owner:           row.Owner,
		Name:            row.Name,
		Label:           row.Label,
		DefaultBranch:   row.DefaultBranch,
		URL:             row.URL,
		Stars:           int(row.Stars),
		OpenIssues:      int(row.OpenIssues),
		LastSyncedAt:    lastSyncedAt,
		LastCommitAt:    lastCommitAt,
		Archived:        row.Archived,
		FlemmeAfterDays: intPtr(row.FlemmeAfterDays),
		DeadAfterDays:   intPtr(row.DeadAfterDays),
		Enabled:         row.Enabled,
		BranchPatterns:  SplitBranchPatterns(row.BranchPatterns),
	}, nil
}

// JoinBranchPatterns convertit les motifs de branches vers la colonne branch_patterns : valeurs
// débarrassées des espaces autour, vides retirées, jointes par des virgules ; nil ou vide donne « ».
// Un motif ne contient jamais de virgule (repoimport.ParseBranchPatterns le garantit en amont).
func JoinBranchPatterns(patterns []string) string {
	kept := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ",")
}

// SplitBranchPatterns est la conversion inverse de JoinBranchPatterns : « » donne nil (toutes les
// branches), sinon les valeurs non vides, dans l'ordre.
func SplitBranchPatterns(s string) []string {
	var patterns []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			patterns = append(patterns, p)
		}
	}
	return patterns
}

// nullTime convertit une date facultative vers une colonne TEXT nullable : la valeur zéro donne NULL,
// toute autre date est enregistrée avec FormatTime.
func nullTime(t time.Time) sql.NullString {
	if t.IsZero() {
		return sql.NullString{}
	}
	return sql.NullString{String: FormatTime(t), Valid: true}
}

// parseNullTime est la conversion inverse de nullTime : NULL donne la valeur zéro de time.Time.
func parseNullTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	return ParseTime(s.String)
}

// nullInt convertit un entier facultatif (nil = absent) vers une colonne INTEGER nullable.
func nullInt(p *int) sql.NullInt64 {
	if p == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: int64(*p), Valid: true}
}

// intPtr est la conversion inverse de nullInt : NULL donne nil.
func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}
