package cli

// « watchoo contributors [--repo owner/repo] [--since 30d] » : store.ContributorRanking.

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/store"
)

// contributorJSON est une ligne de la sortie JSON de « watchoo contributors ».
type contributorJSON struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`         // nom Git (ou du compte) ; vide si inconnu
	Login        string     `json:"login"`        // login GitHub ; vide si aucun compte connu
	DisplayName  string     `json:"display_name"` // nom, sinon login, sinon « inconnu »
	AvatarURL    string     `json:"avatar_url"`
	Commits      int        `json:"commits"`      // commits dont il est l'auteur
	Additions    int        `json:"additions"`    // lignes ajoutées (statistiques connues seulement)
	Deletions    int        `json:"deletions"`    // lignes supprimées
	Repositories int        `json:"repositories"` // dépôts distincts
	LastCommitAt *time.Time `json:"last_commit_at"`
}

// contributorsReport est la sortie JSON de « watchoo contributors ».
type contributorsReport struct {
	Contributors []contributorJSON `json:"contributors"` // du plus actif au moins actif
}

// newContributorsCmd construit « watchoo contributors ».
func newContributorsCmd(deps Deps, opts *options) *cobra.Command {
	var repo, since string
	cmd := &cobra.Command{
		Use:   "contributors",
		Short: "Classe les contributeurs par nombre de commits",
		Long: `Classe les auteurs de commits par nombre de commits, tous dépôts confondus ou pour un
seul dépôt, sur toute la période connue ou une période récente.

Les lignes ajoutées et supprimées ne sont connues que pour les commits découverts
après le premier import d'un dépôt (au plus 100 par dépôt et par synchro) : elles
donnent un ordre de grandeur, pas un total exact.`,
		Example: `  watchoo contributors --since 30d
  watchoo contributors --repo mon-org/projet-groupe-3 --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContributors(cmd.Context(), deps, opts, repo, since)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "limite au `dépôt` owner/repo")
	cmd.Flags().StringVar(&since, "since", "", "limite à une `période` récente (ex. 30d, 7d, 12h)")
	return cmd
}

// runContributors affiche le classement des contributeurs.
func runContributors(ctx context.Context, deps Deps, opts *options, repoRef, since string) (err error) {
	from, err := sinceTime(since, deps.Now())
	if err != nil {
		return err
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	filter := store.ContributorFilter{Since: from}
	if repoRef != "" {
		repo, err := s.findRepository(ctx, repoRef)
		if err != nil {
			return err
		}
		filter.RepoID = repo.ID
	}
	ranking, err := s.store.ContributorRanking(ctx, filter)
	if err != nil {
		return err
	}
	if opts.json {
		report := contributorsReport{Contributors: make([]contributorJSON, 0, len(ranking))}
		for _, c := range ranking {
			report.Contributors = append(report.Contributors, contributorJSON{
				ID:           c.ID,
				Name:         c.Name,
				Login:        c.ProviderLogin,
				DisplayName:  c.DisplayName(),
				AvatarURL:    c.AvatarURL,
				Commits:      c.Commits,
				Additions:    c.Additions,
				Deletions:    c.Deletions,
				Repositories: c.Repos,
				LastCommitAt: timeJSON(c.LastCommitAt),
			})
		}
		return writeJSON(deps.Stdout, report)
	}
	if len(ranking) == 0 {
		msg := "Aucun contributeur"
		if since != "" {
			msg += " depuis " + since
		}
		return writeText(deps.Stdout, msg+" (les contributeurs viennent des commits synchronisés : « watchoo sync »).\n")
	}
	st := outputStyle(deps, opts)
	now := s.now()
	rows := make([][]string, len(ranking))
	for i, c := range ranking {
		login := c.ProviderLogin
		if login == "" {
			login = "-"
		}
		rows[i] = []string{
			c.DisplayName(),
			login,
			fmt.Sprint(c.Commits),
			fmt.Sprintf("+%d", c.Additions),
			fmt.Sprintf("-%d", c.Deletions),
			fmt.Sprint(c.Repos),
			activity.Relative(c.LastCommitAt, now, s.loc, st.unicode()),
		}
	}
	headers := []string{"Contributeur", "Login", "Commits", "Ajouts", "Suppressions", st.pick("Dépôts", "Depots"),
		"Dernier commit"}
	return writeText(deps.Stdout, renderTable(st, headers, rows, nil))
}
