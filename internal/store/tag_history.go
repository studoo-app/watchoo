package store

// Historique des tags et de la branche par défaut (phase 8.4, voir documentation/fr/13-conventions.md) :
// observations des tags (table tag_observations : SV3 et échéances de SV2) et réécritures observées de la
// branche par défaut (table branch_rewrites : BR4). Écrites seulement par la synchro, si conventions.enabled
// (ApplyRepoSync, champs ObserveTags et BranchRewrites : aucune autre méthode exportée n'y écrit) ; aucune
// attribution à une personne. Les messages
// d'erreur ne citent ni nom de tag ni nom de branche, seulement des SHA courts.

import (
	"context"
	"database/sql"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// TagObservations renvoie les périodes d'observation des tags du dépôt repoID (0 = tous les dépôts), par
// dépôt, nom puis première observation ; liste vide non nil si aucune.
func (s *Store) TagObservations(ctx context.Context, repoID int64) ([]model.TagObservation, error) {
	rows, err := s.queries.ListTagObservations(ctx, optionalID(repoID))
	if err != nil {
		return nil, wrapErr(i18n.T("store.conventions.tag_history", repoID), err)
	}
	return tagObservationsFrom(rows, repoID)
}

// BranchRewrites renvoie les réécritures observées de la branche par défaut du dépôt repoID (0 = tous les
// dépôts), par dépôt puis date ; liste vide non nil si aucune.
func (s *Store) BranchRewrites(ctx context.Context, repoID int64) ([]model.BranchRewrite, error) {
	rows, err := s.queries.ListBranchRewrites(ctx, optionalID(repoID))
	if err != nil {
		return nil, wrapErr(i18n.T("store.conventions.rewrites", repoID), err)
	}
	out := make([]model.BranchRewrite, 0, len(rows))
	for _, r := range rows {
		at, err := ParseTime(r.ObservedAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.conventions.rewrites", r.RepoID), err)
		}
		out = append(out, model.BranchRewrite{RepoID: r.RepoID, Branch: r.Branch, OldSHA: r.OldSha,
			NewSHA: r.NewSha, ObservedAt: at})
	}
	return out, nil
}

// ConventionTagDates renvoie, pour chaque tag du dépôt repoID dont le commit visé est en base, la date de
// ce commit (borne basse de « posé entre X et Y » des échéances de SV2) ; map vide non nil si aucun.
func (s *Store) ConventionTagDates(ctx context.Context, repoID int64) (map[string]time.Time, error) {
	views, err := s.ListTags(ctx, ReleaseFilter{RepoID: repoID})
	if err != nil {
		return nil, err
	}
	out := make(map[string]time.Time, len(views))
	for _, v := range views {
		if !v.CommittedAt.IsZero() {
			out[v.Name] = v.CommittedAt
		}
	}
	return out, nil
}

// observeTags applique conventions.PlanTagObservations aux observations ouvertes du dépôt : fermetures
// d'abord (un nom réécrit libère sa période ouverte), puis dernières observations, puis ouvertures.
func observeTags(ctx context.Context, q *db.Queries, repoID int64, tags []model.Tag, at time.Time) error {
	wrap := func(err error) error { return wrapErr(i18n.T("store.sync.tags_observe"), err) }
	rows, err := q.ListOpenTagObservations(ctx, repoID)
	if err != nil {
		return wrap(err)
	}
	open, err := tagObservationsFrom(rows, repoID)
	if err != nil {
		return err
	}
	plan := conventions.PlanTagObservations(open, tags)
	seen := FormatTime(at)
	for _, c := range plan.Close {
		if _, err := q.EndTagObservation(ctx, db.EndTagObservationParams{EndedAt: seen,
			EndReason: sql.NullString{String: c.Reason, Valid: true}, RepoID: repoID, Name: c.Name, SHA: c.SHA}); err != nil {
			return wrap(err)
		}
	}
	for _, t := range plan.Seen {
		if _, err := q.SeeTagObservation(ctx, db.SeeTagObservationParams{SeenAt: seen, RepoID: repoID,
			Name: t.Name, SHA: t.SHA}); err != nil {
			return wrap(err)
		}
	}
	for _, t := range plan.Open {
		if err := q.OpenTagObservation(ctx, db.OpenTagObservationParams{RepoID: repoID, Name: t.Name, SHA: t.SHA,
			SeenAt: seen}); err != nil {
			return wrap(err)
		}
	}
	return nil
}

// insertBranchRewrite enregistre une réécriture de la branche par défaut du dépôt repoID à la date at.
func insertBranchRewrite(ctx context.Context, q *db.Queries, repoID int64, r model.BranchRewrite, at time.Time) error {
	if err := q.InsertBranchRewrite(ctx, db.InsertBranchRewriteParams{RepoID: repoID, Branch: r.Branch,
		OldSha: r.OldSHA, NewSha: r.NewSHA, ObservedAt: FormatTime(at)}); err != nil {
		return wrapErr(i18n.T("store.sync.rewrite_record", shortSHA(r.OldSHA), shortSHA(r.NewSHA)), err)
	}
	return nil
}

// tagObservationsFrom convertit des lignes de tag_observations ; liste vide non nil si aucune.
func tagObservationsFrom(rows []db.TagObservation, repoID int64) ([]model.TagObservation, error) {
	out := make([]model.TagObservation, 0, len(rows))
	for _, r := range rows {
		o := model.TagObservation{RepoID: r.RepoID, Name: r.Name, SHA: r.SHA, EndReason: r.EndReason.String}
		var err error
		if o.FirstSeenAt, err = ParseTime(r.FirstSeenAt); err == nil {
			o.LastSeenAt, err = ParseTime(r.LastSeenAt)
		}
		if err == nil && r.EndedAt.Valid {
			var end time.Time
			if end, err = ParseTime(r.EndedAt.String); err == nil {
				o.EndedAt = &end
			}
		}
		if err != nil {
			return nil, wrapErr(i18n.T("store.conventions.tag_history", repoID), err)
		}
		out = append(out, o)
	}
	return out, nil
}
