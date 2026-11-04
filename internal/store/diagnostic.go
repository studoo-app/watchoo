package store

// Signaux de diagnostic par commit (table commit_signals, phase 7.1).

import (
	"context"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// CommitSignalWrite sont les signaux d'un commit à enregistrer. L'auteur et la date d'écriture sont
// recopiés du commit en base : le commit doit donc y être déjà (sinon la ligne est ignorée).
type CommitSignalWrite struct {
	SHA           string   // SHA complet
	FilesKept     int      // >= 0
	FilesExcluded int      // >= 0
	AdditionsKept int      // >= 0
	DeletionsKept int      // >= 0
	Tools         []string // identifiants d'outils (sans virgule), triés
	AgentConfig   bool
	BotAuthor     bool
}

// WriteCommitSignals enregistre, dans une seule transaction, les signaux de commits du dépôt repoID
// déjà en base, calculés à computedAt. Un commit qui a déjà ses signaux n'est jamais modifié ; un SHA
// absent de la base est ignoré. Renvoie le nombre de lignes créées.
func (s *Store) WriteCommitSignals(ctx context.Context, repoID int64, rows []CommitSignalWrite, computedAt time.Time) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	created := 0
	err := s.writeTx(ctx, func(q *db.Queries) error {
		for _, r := range rows {
			n, err := q.InsertCommitSignals(ctx, db.InsertCommitSignalsParams{
				RepoID:        repoID,
				SHA:           r.SHA,
				FilesKept:     int64(max(r.FilesKept, 0)),
				FilesExcluded: int64(max(r.FilesExcluded, 0)),
				AdditionsKept: int64(max(r.AdditionsKept, 0)),
				DeletionsKept: int64(max(r.DeletionsKept, 0)),
				ToolIds:       strings.Join(r.Tools, ","),
				AgentConfig:   r.AgentConfig,
				BotAuthor:     r.BotAuthor,
				ComputedAt:    FormatTime(computedAt),
			})
			if err != nil {
				return wrapErr(i18n.T("store.diagnostic.write", shortSHA(r.SHA)), err)
			}
			created += int(n)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

// PurgeCommitSignals supprime les signaux des commits écrits avant before (tous les dépôts) et renvoie
// le nombre de lignes supprimées.
func (s *Store) PurgeCommitSignals(ctx context.Context, before time.Time) (int, error) {
	n, err := s.queries.PurgeCommitSignals(ctx, FormatTime(before))
	if err != nil {
		return 0, wrapErr(i18n.T("store.diagnostic.purge"), err)
	}
	return int(n), nil
}

// CommitSignals renvoie les signaux des commits du dépôt repoID, du plus récent au plus ancien.
func (s *Store) CommitSignals(ctx context.Context, repoID int64) ([]model.CommitSignal, error) {
	rows, err := s.queries.ListCommitSignals(ctx, repoID)
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
	}
	out := make([]model.CommitSignal, 0, len(rows))
	for _, r := range rows {
		authored, err := ParseTime(r.AuthoredAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
		}
		computed, err := ParseTime(r.ComputedAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
		}
		tools := []string{}
		if r.ToolIds != "" {
			tools = strings.Split(r.ToolIds, ",")
		}
		out = append(out, model.CommitSignal{
			RepoID:        r.RepoID,
			SHA:           r.SHA,
			AuthorID:      r.AuthorID.Int64,
			AuthoredAt:    authored,
			FilesKept:     int(r.FilesKept),
			FilesExcluded: int(r.FilesExcluded),
			AdditionsKept: int(r.AdditionsKept),
			DeletionsKept: int(r.DeletionsKept),
			Tools:         tools,
			AgentConfig:   r.AgentConfig,
			BotAuthor:     r.BotAuthor,
			ComputedAt:    computed,
		})
	}
	return out, nil
}

// CountCommitSignals renvoie le nombre de commits du dépôt repoID qui ont des signaux.
func (s *Store) CountCommitSignals(ctx context.Context, repoID int64) (int, error) {
	n, err := s.queries.CountCommitSignals(ctx, repoID)
	if err != nil {
		return 0, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
	}
	return int(n), nil
}
