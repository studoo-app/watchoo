package cli

// « watchoo contributors [--repo owner/repo] [--since 30d | --weeks 12] » : store.ContributorRanking ;
// avec --weeks, commits par semaine de chacun (store.AuthorCommitTimes, activity.CountContributorWeeks).

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
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
	// Weekly : commits par semaine (lundi de la semaine, fuseau d'affichage) sur la période de --weeks,
	// de la plus ancienne à la plus récente ; [] sans --weeks.
	Weekly []activity.WeekCount `json:"weekly"`
}

// maxContributorWeeks borne --weeks (un an).
const maxContributorWeeks = 52

// contributorsReport est la sortie JSON de « watchoo contributors ».
type contributorsReport struct {
	Contributors []contributorJSON `json:"contributors"` // du plus actif au moins actif
}

// newContributorsCmd construit « watchoo contributors ».
func newContributorsCmd(deps Deps, opts *options) *cobra.Command {
	var repo, since string
	var weeks int
	cmd := &cobra.Command{
		Use:   "contributors",
		Short: i18n.T("cli.sync.contributors_short"),
		Long:  i18n.T("cli.sync.contributors_long"),
		Example: `  watchoo contributors --since 30d
  watchoo contributors --repo mon-org/projet-groupe-3 --weeks 12
  watchoo contributors --repo mon-org/projet-groupe-3 --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runContributors(cmd.Context(), deps, opts, repo, since, weeks)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", i18n.T("cli.sync.flag_repo"))
	cmd.Flags().StringVar(&since, "since", "", i18n.T("cli.sync.flag_since_contrib"))
	cmd.Flags().IntVar(&weeks, "weeks", 0, i18n.T("cli.sync.flag_weeks"))
	return cmd
}

// runContributors affiche le classement des contributeurs.
func runContributors(ctx context.Context, deps Deps, opts *options, repoRef, since string, weeks int) (err error) {
	// Les options sont vérifiées avant d'ouvrir l'espace : une erreur de saisie est signalée d'abord.
	if weeks < 0 || weeks > maxContributorWeeks {
		return errors.New(i18n.T("cli.sync.weeks_invalid", weeks, maxContributorWeeks))
	}
	if weeks > 0 && since != "" {
		return errors.New(i18n.T("cli.sync.weeks_since_conflict", weeks))
	}
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
	var weekly activity.ContributorWeekCounts
	if weeks > 0 {
		filter.Since = activity.WeeksStart(s.now(), weeks, s.loc)
		times, err := s.store.AuthorCommitTimes(ctx, filter.RepoID, filter.Since)
		if err != nil {
			return err
		}
		weekly = activity.CountContributorWeeks(times, s.now(), weeks, s.loc)
	}
	ranking, err := s.store.ContributorRanking(ctx, filter)
	if err != nil {
		return err
	}
	if opts.json {
		report := contributorsReport{Contributors: make([]contributorJSON, 0, len(ranking))}
		for _, c := range ranking {
			perWeek := make([]activity.WeekCount, 0, len(weekly.Weeks))
			for i, n := range weekly.Of(c.ID) {
				perWeek = append(perWeek, activity.WeekCount{Week: weekly.Weeks[i], Count: n})
			}
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
				Weekly:       perWeek,
			})
		}
		return writeJSON(deps.Stdout, report)
	}
	if len(ranking) == 0 {
		msg := i18n.T("cli.sync.contributors_none")
		if since != "" {
			msg += i18n.T("cli.sync.contributors_none_since", since)
		}
		if weeks > 0 {
			msg += i18n.T("cli.sync.contributors_none_weeks", weeks)
		}
		return writeText(deps.Stdout, msg+i18n.T("cli.sync.contributors_none_hint"))
	}
	st := outputStyle(deps, opts)
	if weeks > 0 {
		return writeText(deps.Stdout, weeklyContributorsTable(st, ranking, weekly))
	}
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
			knownLines(c.KnownCommits > 0, "+", c.Additions),
			knownLines(c.KnownCommits > 0, "-", c.Deletions),
			fmt.Sprint(c.Repos),
			activity.Relative(c.LastCommitAt, now, s.loc, st.unicode()),
		}
	}
	headers := []string{i18n.T("cli.sync.header_contributor"), "Login", "Commits",
		i18n.T("cli.sync.header_additions"), i18n.T("cli.sync.header_deletions"),
		st.pick(i18n.T("cli.sync.header_repos"), i18n.T("cli.sync.header_repos_ascii")),
		i18n.T("cli.sync.header_last_commit")}
	return writeText(deps.Stdout, renderTable(st, headers, rows, nil))
}

// weeklyContributorsTable renvoie le tableau de « contributors --weeks » : une ligne par contributeur
// du classement, une colonne par semaine (lundi au format « 12/10 »), puis le total de la période, et
// une ligne d'explication.
func weeklyContributorsTable(st textStyle, ranking []store.ContributorStats, weekly activity.ContributorWeekCounts) string {
	headers := []string{i18n.T("cli.sync.header_contributor")}
	for _, w := range weekly.Weeks {
		label := w
		if t, err := time.Parse(time.DateOnly, w); err == nil {
			label = t.Format("02/01")
		}
		headers = append(headers, label)
	}
	headers = append(headers, "Total")
	rows := make([][]string, len(ranking))
	for i, c := range ranking {
		row := []string{c.DisplayName()}
		for _, n := range weekly.Of(c.ID) {
			row = append(row, fmt.Sprint(n))
		}
		rows[i] = append(row, fmt.Sprint(weekly.TotalOf(c.ID)))
	}
	note := st.pick(i18n.T("cli.sync.weekly_note"), i18n.T("cli.sync.weekly_note_ascii"))
	return renderTable(st, headers, rows, nil) + note
}

// knownLines écrit un total de lignes précédé de sign, ou « - » s'il est inconnu.
func knownLines(known bool, sign string, n int) string {
	if !known {
		return "-"
	}
	return sign + strconv.Itoa(n)
}
