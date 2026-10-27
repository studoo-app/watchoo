package web

// Contributeurs « /contributors » (documentation/06-interface-web.md, page 3). PROPRIÉTAIRE : agent
// contributors+settings (ce fichier, templates/contributors.html, contributors_test.go).
//
// La page montre le classement global des auteurs (store.ContributorRanking), complété par leurs
// commits par dépôt (store.ContributorRepoBreakdown, regroupé ici par contributeur). Les chiffres
// viennent tous du store ; ce fichier ne fait que les rassembler pour l'affichage.
//
// Vie privée : le nom affiché est Contributor.DisplayName (nom, sinon login GitHub) ; aucun e-mail
// n'est lu ni affiché. L'avatar (template « avatar ») n'est chargé que depuis
// avatars.githubusercontent.com, sans Referer et en différé ; sinon, l'initiale du nom.

import (
	"cmp"
	"net/http"
	"slices"

	"github.com/studoo-app/watchoo/internal/store"
)

// contributorsView est le Content de la page des contributeurs et de son fragment.
type contributorsView struct {
	Rows         []contributorRow
	TotalCommits int // somme des commits du classement (part de chacun)
	Repos        int // nombre de dépôts distincts où au moins un auteur est connu
}

// contributorRow est une ligne du classement : statistiques globales et détail par dépôt.
type contributorRow struct {
	store.ContributorStats
	Rank  int                          // rang (1 = le plus de commits)
	Repos []store.ContributorRepoStats // ses commits par dépôt, du plus grand nombre au plus petit
}

// ShowLogin indique s'il faut afficher le login GitHub à côté du nom (il existe et n'est pas déjà le
// nom affiché).
func (c contributorRow) ShowLogin() bool {
	return c.ProviderLogin != "" && c.ProviderLogin != c.DisplayName()
}

// buildContributorRows rassemble le classement et le détail par dépôt : une ligne par contributeur du
// classement, dans l'ordre du classement, avec ses dépôts triés par commits décroissants, puis
// dernier commit le plus récent, puis nom affiché. Les lignes du détail dont le contributeur n'est
// pas dans le classement sont ignorées. Fonction pure.
func buildContributorRows(ranking []store.ContributorStats, breakdown []store.ContributorRepoStats) []contributorRow {
	byContributor := make(map[int64][]store.ContributorRepoStats, len(ranking))
	for _, b := range breakdown {
		byContributor[b.ContributorID] = append(byContributor[b.ContributorID], b)
	}
	rows := make([]contributorRow, 0, len(ranking))
	for i, st := range ranking {
		repos := byContributor[st.ID]
		slices.SortStableFunc(repos, func(a, b store.ContributorRepoStats) int {
			if c := cmp.Compare(b.Commits, a.Commits); c != 0 {
				return c
			}
			if c := b.LastCommitAt.Compare(a.LastCommitAt); c != 0 {
				return c
			}
			return cmp.Compare(a.RepoDisplayName(), b.RepoDisplayName())
		})
		rows = append(rows, contributorRow{ContributorStats: st, Rank: i + 1, Repos: repos})
	}
	return rows
}

// contributorsData lit la base et construit le Content de la page.
func (s *server) contributorsData(r *http.Request) (contributorsView, error) {
	ctx := r.Context()
	ranking, err := s.store.ContributorRanking(ctx, store.ContributorFilter{})
	if err != nil {
		return contributorsView{}, err
	}
	breakdown, err := s.store.ContributorRepoBreakdown(ctx)
	if err != nil {
		return contributorsView{}, err
	}
	v := contributorsView{Rows: buildContributorRows(ranking, breakdown)}
	repos := make(map[int64]bool)
	for _, row := range v.Rows {
		v.TotalCommits += row.Commits
		for _, b := range row.Repos {
			repos[b.RepoID] = true
		}
	}
	v.Repos = len(repos)
	return v, nil
}

// handleContributors sert GET /contributors (page complète, Nav navContributors, titre
// « Contributeurs »).
func (s *server) handleContributors(w http.ResponseWriter, r *http.Request) {
	v, err := s.contributorsData(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, pageContributors, s.page(r, "Contributeurs", navContributors, v))
}

// handleContributorsTable sert GET /fragments/contributors (fragment « contributors-table »,
// rechargé après chaque synchro).
func (s *server) handleContributorsTable(w http.ResponseWriter, r *http.Request) {
	v, err := s.contributorsData(r)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	s.renderFragment(w, r, http.StatusOK, pageContributors, "contributors-table",
		s.page(r, "Contributeurs", navContributors, v))
}
