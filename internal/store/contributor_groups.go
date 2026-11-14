package store

// Groupes de contributeurs (1.9.0, voir documentation/fr/04-modele-donnees.md) : le superviseur
// rattache les comptes d'une personne à un contributeur principal (un seul niveau, réversible).
// commits.person_id et commit_signals.person_id portent le principal, recalculés au rattachement, au
// détachement et à la fusion automatique des identités, et remplis par la synchro pour les nouveaux
// commits : les comptages par personne lisent un index sans jointure.

import (
	"context"
	"database/sql"
	"errors"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// Erreurs des groupes : l'appelant les traduit dans la langue de l'interface.
var (
	ErrGroupSelf        = errors.New("contributor group: attached to itself")
	ErrGroupAttached    = errors.New("contributor group: already attached")
	ErrGroupHasAccounts = errors.New("contributor group: has attached accounts")
	ErrGroupNotPrimary  = errors.New("contributor group: target is attached")
	ErrGroupNotAttached = errors.New("contributor group: not attached")
)

// AttachedAccount est un compte rattaché à un principal, avec son nombre de commits d'auteur.
type AttachedAccount struct {
	model.Contributor
	PrimaryID int64
	Commits   int
}

// AttachContributor rattache le contributeur id au principal primary, et recalcule la personne de leurs
// commits et de leurs signaux. Erreurs : ErrNotFound (l'un des deux n'existe pas), ErrGroupSelf,
// ErrGroupAttached (id est déjà rattaché), ErrGroupHasAccounts (id a des comptes rattachés),
// ErrGroupNotPrimary (primary est lui-même rattaché).
func (s *Store) AttachContributor(ctx context.Context, id, primary int64) error {
	if id == primary {
		return ErrGroupSelf
	}
	return s.writeTx(ctx, func(q *db.Queries) error {
		src, err := q.GetContributorGroup(ctx, id)
		if err != nil {
			return groupLookupErr(id, err)
		}
		dst, err := q.GetContributorGroup(ctx, primary)
		if err != nil {
			return groupLookupErr(primary, err)
		}
		switch {
		case src.PrimaryID.Valid:
			return ErrGroupAttached
		case src.Attached > 0:
			return ErrGroupHasAccounts
		case dst.PrimaryID.Valid:
			return ErrGroupNotPrimary
		}
		if err := q.SetContributorPrimary(ctx, db.SetContributorPrimaryParams{
			PrimaryID: sql.NullInt64{Int64: primary, Valid: true}, ID: id,
		}); err != nil {
			return wrapErr(i18n.T("store.contrib.group", id), err)
		}
		return refreshPersons(ctx, q, id, primary)
	})
}

// DetachContributor détache le contributeur id de son principal et lui rend ses commits et ses signaux.
// Erreurs : ErrNotFound, ErrGroupNotAttached.
func (s *Store) DetachContributor(ctx context.Context, id int64) error {
	return s.writeTx(ctx, func(q *db.Queries) error {
		src, err := q.GetContributorGroup(ctx, id)
		if err != nil {
			return groupLookupErr(id, err)
		}
		if !src.PrimaryID.Valid {
			return ErrGroupNotAttached
		}
		if err := q.SetContributorPrimary(ctx, db.SetContributorPrimaryParams{ID: id}); err != nil {
			return wrapErr(i18n.T("store.contrib.group", id), err)
		}
		return refreshPersons(ctx, q, id, src.PrimaryID.Int64)
	})
}

// AttachedAccounts renvoie les comptes rattachés, par principal puis identifiant : clé = principal.
func (s *Store) AttachedAccounts(ctx context.Context) (map[int64][]AttachedAccount, error) {
	rows, err := s.queries.ListAttachedAccounts(ctx)
	if err != nil {
		return nil, wrapErr(i18n.T("store.contrib.labels"), err)
	}
	out := map[int64][]AttachedAccount{}
	for _, r := range rows {
		a := AttachedAccount{
			Contributor: contributorFromDB(db.Contributor{
				ID: r.ID, Provider: r.Provider, ProviderUserID: r.ProviderUserID, ProviderLogin: r.ProviderLogin,
				Name: r.Name, Label: r.Label, AvatarURL: r.AvatarURL, ProfileURL: r.ProfileURL,
			}),
			PrimaryID: r.PrimaryID.Int64,
			Commits:   int(r.Commits),
		}
		out[a.PrimaryID] = append(out[a.PrimaryID], a)
	}
	return out, nil
}

// PrimaryOf renvoie le principal du contributeur id (id lui-même s'il n'est pas rattaché). Erreur
// enveloppant ErrNotFound si id n'existe pas.
func (s *Store) PrimaryOf(ctx context.Context, id int64) (int64, error) {
	g, err := s.queries.GetContributorGroup(ctx, id)
	if err != nil {
		return 0, groupLookupErr(id, err)
	}
	if g.PrimaryID.Valid {
		return g.PrimaryID.Int64, nil
	}
	return id, nil
}

// groupLookupErr convertit l'échec de la lecture du groupe de id.
func groupLookupErr(id int64, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return wrapErr(i18n.T("store.web.contrib", id), ErrNotFound)
	}
	return wrapErr(i18n.T("store.contrib.group", id), err)
}

// refreshPersons recalcule la personne des commits et des signaux des groupes de a et b.
func refreshPersons(ctx context.Context, q *db.Queries, a, b int64) error {
	if err := q.RefreshCommitPersons(ctx, db.RefreshCommitPersonsParams{A: a, B: b}); err != nil {
		return wrapErr(i18n.T("store.contrib.persons"), err)
	}
	if err := q.RefreshSignalPersons(ctx, db.RefreshSignalPersonsParams{A: a, B: b}); err != nil {
		return wrapErr(i18n.T("store.contrib.persons"), err)
	}
	return nil
}

// mergeGroups applique les règles des groupes à la fusion automatique de from dans into (les commits et
// les signaux de from sont déjà réattribués à into ; from sera supprimé ensuite) :
//   - les comptes rattachés à from le sont au principal de into, ou à into lui-même s'il en faisait
//     partie (into devient alors principal) ;
//   - si from était rattaché à un principal Q et que into n'est dans aucun groupe, into est rattaché à Q ;
//   - la personne des commits et des signaux des groupes concernés est recalculée.
func mergeGroups(ctx context.Context, q *db.Queries, from, into db.Contributor) error {
	intoGroup, err := q.GetContributorGroup(ctx, into.ID)
	if err != nil {
		return wrapErr(i18n.T("store.contrib.group", into.ID), err)
	}
	target := into.ID
	switch {
	case intoGroup.PrimaryID.Valid && intoGroup.PrimaryID.Int64 == from.ID:
		// into était rattaché à from : il devient le principal du groupe.
		if err := q.SetContributorPrimary(ctx, db.SetContributorPrimaryParams{ID: into.ID}); err != nil {
			return wrapErr(i18n.T("store.contrib.group", into.ID), err)
		}
		intoGroup.PrimaryID = sql.NullInt64{}
	case intoGroup.PrimaryID.Valid:
		target = intoGroup.PrimaryID.Int64
	}
	inNoGroup := !intoGroup.PrimaryID.Valid && intoGroup.Attached == 0
	if err := q.MoveAttachedAccounts(ctx, db.MoveAttachedAccountsParams{
		ToID: sql.NullInt64{Int64: target, Valid: true}, FromID: sql.NullInt64{Int64: from.ID, Valid: true}}); err != nil {
		return wrapErr(i18n.T("store.contrib.group", into.ID), err)
	}
	if inNoGroup && from.PrimaryID.Valid && from.PrimaryID.Int64 != into.ID {
		if err := q.SetContributorPrimary(ctx, db.SetContributorPrimaryParams{PrimaryID: from.PrimaryID, ID: into.ID}); err != nil {
			return wrapErr(i18n.T("store.contrib.group", into.ID), err)
		}
		target = from.PrimaryID.Int64
	}
	return refreshPersons(ctx, q, into.ID, target)
}
