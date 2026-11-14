package cli

// « watchoo contributors [--repo owner/repo] [--since 30d | --weeks 12] » : store.ContributorRanking
// (langage principal de chacun, 1.2.0) ;
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
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// contributorJSON est une ligne de la sortie JSON de « watchoo contributors ».
type contributorJSON struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`         // nom Git (ou du compte) ; vide si inconnu
	Login       string `json:"login"`        // login GitHub ; vide si aucun compte connu
	Label       string `json:"label"`        // nom donné dans l'interface web (1.8.0) ; vide si aucun
	DisplayName string `json:"display_name"` // nom donné, sinon nom, sinon login, sinon « inconnu »
	// Accounts : comptes rattachés à ce principal (1.9.0), comptés dans les totaux ; [] si aucun.
	Accounts     []accountJSON `json:"accounts"`
	AvatarURL    string        `json:"avatar_url"`
	Commits      int           `json:"commits"`      // commits dont il est l'auteur
	Additions    int           `json:"additions"`    // lignes ajoutées (statistiques connues seulement)
	Deletions    int           `json:"deletions"`    // lignes supprimées
	Repositories int           `json:"repositories"` // dépôts distincts
	Share        float64       `json:"share"`        // part des commits du classement, en %, une décimale
	LastCommitAt *time.Time    `json:"last_commit_at"`
	// Signatures d'IA (1.2.0) : commits signés, commits analysés, outils ; 0 n'est jamais une preuve de
	// travail sans IA.
	AISignedCommits  int          `json:"ai_signed_commits"`
	AIScannedCommits int          `json:"ai_scanned_commits"`
	AITools          []aiToolJSON `json:"ai_tools"`
	// Langages (1.2.0) : commits dont les langages sont lus dans le clone, lignes ajoutées par langage.
	LanguagesScannedCommits int                       `json:"languages_scanned_commits"`
	Languages               []contributorLanguageJSON `json:"languages"`
	// Weekly : commits par semaine (lundi de la semaine, fuseau d'affichage) sur la période de --weeks,
	// de la plus ancienne à la plus récente ; [] sans --weeks.
	Weekly []activity.WeekCount `json:"weekly"`
}

// aiToolJSON donne le nombre de commits signés par un outil d'IA.
type aiToolJSON struct {
	Tool    string `json:"tool"`
	Commits int    `json:"commits"`
}

// contributorLanguageJSON donne les lignes ajoutées par un contributeur dans un langage.
type contributorLanguageJSON struct {
	Name    string  `json:"name"`
	Lines   int     `json:"lines"`   // lignes ajoutées
	Percent float64 `json:"percent"` // part de ses lignes ajoutées, une décimale
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
	accounts, err := s.store.AttachedAccounts(ctx)
	if err != nil {
		return err
	}
	var total int64
	for _, c := range ranking {
		total += int64(c.Commits)
	}
	if opts.json {
		report := contributorsReport{Contributors: make([]contributorJSON, 0, len(ranking))}
		for _, c := range ranking {
			perWeek := make([]activity.WeekCount, 0, len(weekly.Weeks))
			for i, n := range weekly.Of(c.ID) {
				perWeek = append(perWeek, activity.WeekCount{Week: weekly.Weeks[i], Count: n})
			}
			report.Contributors = append(report.Contributors, contributorJSON{
				ID:                      c.ID,
				Name:                    c.Name,
				Login:                   c.ProviderLogin,
				Label:                   c.Label,
				DisplayName:             c.DisplayName(),
				Accounts:                accountsJSON(accounts[c.ID]),
				AvatarURL:               c.AvatarURL,
				Commits:                 c.Commits,
				Additions:               c.Additions,
				Deletions:               c.Deletions,
				Repositories:            c.Repos,
				Share:                   float64(model.Permille(int64(c.Commits), total)) / 10,
				LastCommitAt:            timeJSON(c.LastCommitAt),
				AISignedCommits:         c.AI.Signed,
				AIScannedCommits:        c.AI.Scanned,
				AITools:                 aiToolsJSON(c.AI.Tools),
				LanguagesScannedCommits: c.Languages.Scanned,
				Languages:               contributorLanguagesJSON(c.Languages),
				Weekly:                  perWeek,
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
	signed := false
	for i, c := range ranking {
		signed = signed || c.AI.Signed > 0
		login := c.ProviderLogin
		if login == "" {
			login = "-"
		}
		if n := len(accounts[c.ID]); n > 0 { // comptes rattachés (1.9.0)
			login += " " + i18n.T("cli.contrib.more_accounts", n)
		}
		rows[i] = []string{
			c.DisplayName(),
			login,
			fmt.Sprint(c.Commits),
			activity.PercentText(model.Permille(int64(c.Commits), total)),
			knownLines(c.KnownCommits > 0, "+", c.Additions),
			knownLines(c.KnownCommits > 0, "-", c.Deletions),
			aiTotalsText(c.AI),
			mainLanguageText(c.Languages),
			fmt.Sprint(c.Repos),
			activity.Relative(c.LastCommitAt, now, s.loc, st.unicode()),
		}
	}
	headers := []string{i18n.T("cli.sync.header_contributor"), "Login", "Commits", i18n.T("cli.sync.header_share"),
		i18n.T("cli.sync.header_additions"), i18n.T("cli.sync.header_deletions"), i18n.T("cli.sync.header_ai"),
		i18n.T("cli.sync.header_language"),
		st.pick(i18n.T("cli.sync.header_repos"), i18n.T("cli.sync.header_repos_ascii")),
		i18n.T("cli.sync.header_last_commit")}
	out := renderTable(st, headers, rows, nil)
	if signed {
		out += i18n.T("cli.sync.ai_note")
	}
	return writeText(deps.Stdout, out)
}

// aiTotalsText écrit « commits signés/commits analysés », « - » si aucun commit n'est encore analysé.
func aiTotalsText(a model.AITotals) string {
	if !a.Known() {
		return "-"
	}
	return fmt.Sprintf("%d/%d", a.Signed, a.Scanned)
}

// mainLanguageText écrit le langage où le contributeur a ajouté le plus de lignes, « - » si aucun.
func mainLanguageText(t model.LanguageTotals) string {
	if t.Main() == "" {
		return "-"
	}
	return t.Main()
}

// contributorLanguagesJSON convertit les langages d'un contributeur ([] si aucun, jamais null).
func contributorLanguagesJSON(t model.LanguageTotals) []contributorLanguageJSON {
	out := make([]contributorLanguageJSON, len(t.Lines))
	for i, l := range t.Lines {
		out[i] = contributorLanguageJSON{Name: l.Language, Lines: l.Lines, Percent: float64(t.Share(l)) / 10}
	}
	return out
}

// aiToolsJSON convertit les outils d'un contributeur ([] si aucun, jamais null).
func aiToolsJSON(tools []model.AIToolCount) []aiToolJSON {
	out := make([]aiToolJSON, len(tools))
	for i, t := range tools {
		out[i] = aiToolJSON{Tool: t.Tool, Commits: t.Commits}
	}
	return out
}

// weeklyContributorsTable renvoie le tableau de « contributors --weeks » : une ligne par contributeur
// du classement, une colonne par semaine (lundi au format « 12/10 », « 10/12 » en anglais), puis le total de la période, et
// une ligne d'explication.
func weeklyContributorsTable(st textStyle, ranking []store.ContributorStats, weekly activity.ContributorWeekCounts) string {
	headers := []string{i18n.T("cli.sync.header_contributor")}
	for _, w := range weekly.Weeks {
		label := w
		if t, err := time.Parse(time.DateOnly, w); err == nil {
			label = t.Format(i18n.T("cli.core.fmt.daymonth"))
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

// accountJSON est un compte rattaché à un contributeur principal (1.9.0) ; jamais d'adresse.
type accountJSON struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Login   string `json:"login"`
	Commits int    `json:"commits"` // commits dont ce compte est l'auteur
}

// accountsJSON convertit les comptes rattachés ; [] si aucun.
func accountsJSON(accounts []store.AttachedAccount) []accountJSON {
	out := make([]accountJSON, 0, len(accounts))
	for _, a := range accounts {
		out = append(out, accountJSON{ID: a.ID, Name: a.Name, Login: a.ProviderLogin, Commits: a.Commits})
	}
	return out
}
