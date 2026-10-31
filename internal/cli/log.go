package cli

// « watchoo log [--repo owner/repo] [--author X] [--branch B] [--since 7d] [-n 20] » :
// store.ListRecentCommits, dates relatives en heure locale.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// defaultLogLimit est le nombre de commits affichés par « watchoo log » sans -n.
const defaultLogLimit = 20

// logReport est la sortie JSON de « watchoo log ».
type logReport struct {
	Commits []commitJSON `json:"commits"` // du plus récent au plus ancien
}

// logFlags regroupe les options de « watchoo log ».
type logFlags struct {
	repo, author, branch, since string
	limit                       int
}

// newLogCmd construit « watchoo log ».
func newLogCmd(deps Deps, opts *options) *cobra.Command {
	var f logFlags
	cmd := &cobra.Command{
		Use:   "log",
		Short: i18n.T("cli.sync.log_short"),
		Long:  i18n.T("cli.sync.log_long"),
		Example: `  watchoo log --since 7d
  watchoo log --repo alice/projet-licence --author alice -n 50`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLog(cmd.Context(), deps, opts, f)
		},
	}
	cmd.Flags().StringVar(&f.repo, "repo", "", i18n.T("cli.sync.flag_repo"))
	cmd.Flags().StringVar(&f.author, "author", "", i18n.T("cli.sync.flag_author"))
	cmd.Flags().StringVar(&f.branch, "branch", "", i18n.T("cli.sync.flag_branch"))
	cmd.Flags().StringVar(&f.since, "since", "", i18n.T("cli.sync.flag_since"))
	cmd.Flags().IntVarP(&f.limit, "limit", "n", defaultLogLimit, i18n.T("cli.sync.flag_log_limit"))
	return cmd
}

// sinceTime convertit une option --since en date de début (now − durée) ; zéro si since est vide.
func sinceTime(since string, now time.Time) (time.Time, error) {
	if since == "" {
		return time.Time{}, nil
	}
	d, err := activity.ParseSince(since)
	if err != nil {
		return time.Time{}, err
	}
	return now.Add(-d), nil
}

// runLog affiche les derniers commits filtrés.
func runLog(ctx context.Context, deps Deps, opts *options, f logFlags) (err error) {
	if f.limit < 0 {
		return errors.New(i18n.T("cli.sync.log_bad_limit", f.limit))
	}
	// Les options sont vérifiées avant d'ouvrir l'espace : une erreur de saisie est signalée d'abord.
	since, err := sinceTime(f.since, deps.Now())
	if err != nil {
		return err
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	filter := store.CommitFilter{
		Author: strings.TrimSpace(f.author),
		Branch: strings.TrimSpace(f.branch),
		Since:  since,
		Limit:  f.limit,
	}
	if f.repo != "" {
		repo, err := s.findRepository(ctx, f.repo)
		if err != nil {
			return err
		}
		filter.RepoID = repo.ID
	}
	commits, err := s.store.ListRecentCommits(ctx, filter)
	if err != nil {
		return err
	}
	if opts.json {
		report := logReport{Commits: make([]commitJSON, 0, len(commits))}
		for _, c := range commits {
			report.Commits = append(report.Commits, newCommitJSON(c))
		}
		return writeJSON(deps.Stdout, report)
	}
	if len(commits) == 0 {
		return writeText(deps.Stdout, i18n.T("cli.sync.log_no_match", filterText(f)))
	}
	st := outputStyle(deps, opts)
	now := s.now()
	rows := make([][]string, len(commits))
	for i, c := range commits {
		rows[i] = []string{
			activity.Relative(c.CommittedAt, now, s.loc, st.unicode()),
			c.RepoDisplayName(),
			c.AuthorDisplayName(),
			provider.ShortSHA(c.SHA),
			commitLines(c.Commit),
			st.truncate(c.Message, 60),
		}
	}
	headers := []string{"Date", st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
		i18n.T("cli.sync.header_author"), "Commit", "+/-", "Message"}
	return writeText(deps.Stdout, renderTable(st, headers, rows, nil))
}

// commitLines écrit les lignes ajoutées et supprimées d'un commit : « +12/-3 » (« +0/-0 » compris)
// si elles sont connues, « - » sinon.
func commitLines(c model.Commit) string {
	if !c.StatsKnown {
		return "-"
	}
	return fmt.Sprintf("+%d/-%d", c.Additions, c.Deletions)
}

// filterText décrit les filtres de « watchoo log » pour le message « aucun commit » : vide sans filtre.
func filterText(f logFlags) string {
	var parts []string
	if f.repo != "" {
		parts = append(parts, i18n.T("cli.sync.filter_repo", f.repo))
	}
	if f.author != "" {
		parts = append(parts, i18n.T("cli.sync.filter_author", f.author))
	}
	if f.branch != "" {
		parts = append(parts, i18n.T("cli.sync.filter_branch", f.branch))
	}
	if f.since != "" {
		parts = append(parts, i18n.T("cli.sync.filter_since", f.since))
	}
	if len(parts) == 0 {
		return i18n.T("cli.sync.filter_empty_db")
	}
	return i18n.T("cli.sync.log_filters", strings.Join(parts, ", "))
}
