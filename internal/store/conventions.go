package store

// Lecture des commits pour les conventions (phase 8.1) : le calcul est fait par internal/conventions, à
// la lecture ; rien n'est stocké. Le message n'est lu qu'en mémoire et n'est jamais journalisé.

import (
	"context"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// ConventionCommits renvoie les commits du dépôt repoID, de l'auteur authorID (0 = tous les auteurs),
// du plus récent au plus ancien, avec leur rattachement aux branches suivies et à la branche par défaut ;
// liste vide non nil si aucun.
func (s *Store) ConventionCommits(ctx context.Context, repoID, authorID int64) ([]model.ConventionCommit, error) {
	rows, err := s.queries.ListConventionCommits(ctx, db.ListConventionCommitsParams{
		RepoID: repoID, AuthorID: optionalID(authorID)})
	if err != nil {
		return nil, wrapErr(i18n.T("store.conventions.commits", repoID), err)
	}
	out := make([]model.ConventionCommit, 0, len(rows))
	for _, r := range rows {
		at, err := ParseTime(r.AuthoredAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.conventions.commits", repoID), err)
		}
		c := model.ConventionCommit{
			SHA:             r.SHA,
			Message:         r.Message,
			AuthoredAt:      at,
			AuthorID:        r.AuthorID,
			AuthorName:      r.AuthorName,
			AuthorLogin:     r.AuthorLogin,
			AuthorEmails:    strings.Fields(r.AuthorEmails),
			CommitterLogin:  r.CommitterLogin,
			CommitterEmails: strings.Fields(r.CommitterEmails),
			OnTracked:       r.OnTracked != 0,
			OnDefault:       r.OnDefault != 0,
		}
		if r.ParentCount.Valid {
			n := int(r.ParentCount.Int64)
			c.ParentCount = &n
		}
		out = append(out, c)
	}
	return out, nil
}
