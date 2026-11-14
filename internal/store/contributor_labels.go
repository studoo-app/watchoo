package store

// Noms donnés aux contributeurs (1.8.0, voir documentation/fr/04-modele-donnees.md) : écrits seulement
// depuis la page /settings/contributors de l'interface web, jamais par la synchro, jamais exportés.

import (
	"context"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// LabeledContributor est une ligne de la page des noms donnés : le contributeur, le nombre d'adresses
// e-mail rattachées (jamais les adresses) et le nombre de ses commits d'auteur.
type LabeledContributor struct {
	model.Contributor
	Identities int
	Commits    int
}

// SetContributorLabel donne le nom label au contributeur id (vide : retire le nom donné). label est
// vérifié et débarrassé de ses espaces de bord par model.NormalizeContributorLabel (erreur
// ErrContributorLabelInvalid ou ErrContributorLabelTooLong) ; le nom enregistré est renvoyé. Erreur
// enveloppant ErrNotFound si le contributeur n'existe pas.
func (s *Store) SetContributorLabel(ctx context.Context, id int64, label string) (string, error) {
	label, err := model.NormalizeContributorLabel(label)
	if err != nil {
		return "", err
	}
	n, err := s.queries.SetContributorLabel(ctx, db.SetContributorLabelParams{Label: label, ID: id})
	if err != nil {
		return "", wrapErr(i18n.T("store.contrib.label", id), err)
	}
	if n == 0 {
		return "", wrapErr(i18n.T("store.web.contrib", id), ErrNotFound)
	}
	return label, nil
}

// ContributorLabels renvoie les noms donnés aux contributeurs, par identifiant (seulement ceux qui en
// ont un) : affichage des rapports du diagnostic et des conventions, qui ne connaissent que l'auteur.
func (s *Store) ContributorLabels(ctx context.Context) (map[int64]string, error) {
	rows, err := s.queries.ListContributorLabels(ctx)
	if err != nil {
		return nil, wrapErr(i18n.T("store.contrib.labels"), err)
	}
	out := make(map[int64]string, len(rows))
	for _, r := range rows {
		out[r.ID] = r.Label
	}
	return out, nil
}

// LabeledContributors renvoie les contributeurs auteurs d'au moins un commit, par identifiant croissant,
// pour la page des noms donnés.
func (s *Store) LabeledContributors(ctx context.Context) ([]LabeledContributor, error) {
	rows, err := s.queries.ListContributorsForLabels(ctx)
	if err != nil {
		return nil, wrapErr(i18n.T("store.contrib.labels"), err)
	}
	out := make([]LabeledContributor, 0, len(rows))
	for _, r := range rows {
		out = append(out, LabeledContributor{
			Contributor: contributorFromDB(db.Contributor{
				ID: r.ID, Provider: r.Provider, ProviderUserID: r.ProviderUserID, ProviderLogin: r.ProviderLogin,
				Name: r.Name, Label: r.Label, AvatarURL: r.AvatarURL, ProfileURL: r.ProfileURL,
			}),
			Identities: int(r.Emails),
			Commits:    int(r.Commits),
		})
	}
	return out, nil
}
