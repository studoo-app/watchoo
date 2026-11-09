package cli

// « watchoo repo » et ses sous-commandes add, list, remove (import : repo_import.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// newRepoCmd construit « watchoo repo » (add, import, list, remove). Sans sous-commande : son aide.
func newRepoCmd(deps Deps, opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: i18n.T("cli.repo.help.repo_short"),
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return unknownCommandError(cmd, args[0])
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newRepoAddCmd(deps, opts),
		newRepoImportCmd(deps, opts),
		newRepoListCmd(deps, opts),
		newRepoRemoveCmd(deps, opts),
	)
	return cmd
}

// exactArgs exige n arguments positionnels, avec un message traduit qui rappelle l'usage.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return errors.New(i18n.T("cli.repo.args.exact", cmd.CommandPath(), n, plural(n),
				len(args), cmd.UseLine()))
		}
		return nil
	}
}

// maxArgs accepte au plus n arguments positionnels.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return errors.New(i18n.T("cli.repo.args.max", cmd.CommandPath(), n, plural(n),
				strings.Join(args, " "), cmd.UseLine()))
		}
		return nil
	}
}

// plural renvoie « s » si n > 1.
func plural(n int) string {
	if n > 1 {
		return "s"
	}
	return ""
}

// branchesText décrit les branches suivies d'un dépôt : « all branches » ou « branches: a, b ».
func branchesText(patterns []string) string {
	if len(patterns) == 0 {
		return i18n.T("cli.repo.branches.all")
	}
	return i18n.T("cli.repo.branches.list", strings.Join(patterns, ", "))
}

// repoAddReport est la sortie JSON de « watchoo repo add ».
type repoAddReport struct {
	Outcome    wsync.AddOutcome `json:"outcome"` // added, already ou updated
	Repository repositoryJSON   `json:"repository"`
}

// newRepoAddCmd construit « watchoo repo add owner/repo [--branch main,dev,release/*] [--label X]
// [--flemme N] [--dead N] [--conventions-since AAAA-MM-JJ] ».
func newRepoAddCmd(deps Deps, opts *options) *cobra.Command {
	var branch, label, conventionsSince string
	var flemme, dead int
	cmd := &cobra.Command{
		Use:   "add owner/repo",
		Short: i18n.T("cli.repo.help.add_short"),
		Long:  i18n.T("cli.repo.help.add_long"),
		Example: `  watchoo repo add alice/projet-licence --label "Alice Martin"
  watchoo repo add https://github.com/mon-org/site-web --branch main,dev,release/*
  watchoo repo add alice/projet-licence --flemme 3 --dead 10
  watchoo repo add alice/projet-licence --conventions-since 2026-09-01`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, err := repoimport.ParseRef(args[0])
			if err != nil {
				return err
			}
			req := wsync.AddRequest{Ref: ref, Label: strings.TrimSpace(label)}
			if err := repoimport.ValidateLabel(req.Label); err != nil {
				return fmt.Errorf("--%w", err)
			}
			if cmd.Flags().Changed("branch") {
				patterns, err := repoimport.ParseBranchPatterns(branch)
				if err != nil {
					return fmt.Errorf("--branch: %w", err)
				}
				if patterns == nil {
					patterns = []string{} // --branch "" : toutes les branches
				}
				req.BranchPatterns = patterns
			}
			if cmd.Flags().Changed("flemme") {
				req.FlemmeAfterDays = &flemme
			}
			if cmd.Flags().Changed("dead") {
				req.DeadAfterDays = &dead
			}
			if err := repoimport.ValidateThresholds(req.FlemmeAfterDays, req.DeadAfterDays); err != nil {
				return fmt.Errorf("--%w", err)
			}
			if cmd.Flags().Changed("conventions-since") {
				date, err := repoimport.ParseConventionsSince(conventionsSince)
				if err != nil {
					return fmt.Errorf("--conventions-since: %w", err)
				}
				req.ConventionsSince = &date
			}
			return runRepoAdd(cmd.Context(), deps, opts, req)
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "",
		i18n.T("cli.repo.help.add_flag_branch"))
	cmd.Flags().StringVar(&label, "label", "", i18n.T("cli.repo.help.add_flag_label"))
	cmd.Flags().IntVar(&flemme, "flemme", 0,
		i18n.T("cli.repo.help.add_flag_flemme"))
	cmd.Flags().IntVar(&dead, "dead", 0,
		i18n.T("cli.repo.help.add_flag_dead"))
	cmd.Flags().StringVar(&conventionsSince, "conventions-since", "",
		i18n.T("cli.repo.help.add_flag_conventions_since"))
	return cmd
}

// runRepoAdd vérifie l'accès au dépôt et l'enregistre (wsync.Engine.AddRepositories).
func runRepoAdd(ctx context.Context, deps Deps, opts *options, req wsync.AddRequest) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	engine, err := s.engine()
	if err != nil {
		return err
	}
	results, err := engine.AddRepositories(ctx, []wsync.AddRequest{req}, wsync.AddOptions{})
	if err != nil {
		return err
	}
	if len(results) != 1 {
		return errors.New(i18n.T("cli.repo.add.missing_result", req.Ref))
	}
	res := results[0]
	switch res.Outcome {
	case wsync.OutcomeInaccessible:
		return errors.New(i18n.T("cli.repo.add.inaccessible", req.Ref, res.Error))
	case wsync.OutcomeError:
		return errors.New(i18n.T("cli.repo.add.error", req.Ref, res.Error))
	}
	if opts.json {
		return writeJSON(deps.Stdout, repoAddReport{Outcome: res.Outcome, Repository: newRepositoryJSON(res.Repo)})
	}
	return writeText(deps.Stdout, repoAddText(res))
}

// repoAddText renvoie le compte rendu texte d'un ajout réussi.
func repoAddText(res wsync.AddResult) string {
	repo := res.Repo
	var details []string
	if repo.Label != "" {
		details = append(details, i18n.T("cli.repo.add.details_label", textclean.Clean(repo.Label)))
	}
	details = append(details, branchesText(repo.BranchPatterns))
	if t := thresholdsText(repo.FlemmeAfterDays, repo.DeadAfterDays); t != "" {
		details = append(details, t)
	}
	if repo.ConventionsSince != "" {
		details = append(details, i18n.T("cli.repo.conventions_since", repo.ConventionsSince))
	}
	var b strings.Builder
	switch res.Outcome {
	case wsync.OutcomeAdded:
		fmt.Fprintf(&b, i18n.T("cli.repo.add.added"), repo.FullName(), strings.Join(details, ", "))
	case wsync.OutcomeUpdated:
		fmt.Fprintf(&b, i18n.T("cli.repo.add.updated"), repo.FullName(), strings.Join(details, ", "))
	default:
		fmt.Fprintf(&b, i18n.T("cli.repo.add.already"), repo.FullName(), strings.Join(details, ", "))
	}
	if !strings.EqualFold(res.Request.Ref.String(), repo.FullName()) {
		fmt.Fprintf(&b, i18n.T("cli.repo.add.renamed"), res.Request.Ref, repo.FullName())
	}
	if res.Outcome != wsync.OutcomeAlreadyTracked {
		b.WriteString(i18n.T("cli.repo.sync_hint"))
	}
	return b.String()
}

// thresholdsText décrit les seuils propres à un dépôt (« thresholds: flemme 3 d, dead 10 d »), ou ""
// s'il n'en a aucun (seuils de l'espace).
func thresholdsText(flemme, dead *int) string {
	var parts []string
	if flemme != nil {
		parts = append(parts, i18n.T("cli.repo.days.flemme", *flemme))
	}
	if dead != nil {
		parts = append(parts, i18n.T("cli.repo.days.dead", *dead))
	}
	if len(parts) == 0 {
		return ""
	}
	return i18n.T("cli.repo.days.prefix") + strings.Join(parts, ", ")
}

// writeText écrit s sur w, avec une erreur enveloppée.
func writeText(w io.Writer, s string) error {
	if _, err := w.Write([]byte(s)); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.repo.write_output"), i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// repoListItem est un dépôt de la sortie JSON de « watchoo repo list ».
type repoListItem struct {
	repositoryJSON
	Activity        activityJSON `json:"activity"`
	Commits         int          `json:"commits"`          // commits en base
	TrackedBranches int          `json:"tracked_branches"` // branches suivies
	LastCommit      *commitJSON  `json:"last_commit"`      // null si aucun
	LastSync        *syncRunJSON `json:"last_sync"`        // dernier sync_run, null si aucun
}

// repoListReport est la sortie JSON de « watchoo repo list ».
type repoListReport struct {
	Repositories []repoListItem `json:"repositories"`
}

// newRepoListCmd construit « watchoo repo list ».
func newRepoListCmd(deps Deps, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: i18n.T("cli.repo.help.list_short"),
		Long:  i18n.T("cli.repo.help.list_long"),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoList(cmd.Context(), deps, opts)
		},
	}
}

// runRepoList affiche le résumé des dépôts (store.RepositorySummaries) et leur état (activity).
func runRepoList(ctx context.Context, deps Deps, opts *options) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	summaries, err := s.store.RepositorySummaries(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	states := make([]activity.Result, len(summaries))
	for i, sum := range summaries {
		states[i] = activity.Evaluate(sum.Repository, sum.LastRun, s.ws.Config.Activity, now, s.loc)
	}
	if opts.json {
		report := repoListReport{Repositories: make([]repoListItem, 0, len(summaries))}
		for i, sum := range summaries {
			item := repoListItem{
				repositoryJSON:  newRepositoryJSON(sum.Repository),
				Activity:        newActivityJSON(states[i]),
				Commits:         sum.Commits,
				TrackedBranches: sum.TrackedBranches,
				LastSync:        newSyncRunJSON(sum.LastRun),
			}
			if sum.LastCommit.SHA != "" {
				c := newCommitJSON(sum.LastCommit)
				item.LastCommit = &c
			}
			report.Repositories = append(report.Repositories, item)
		}
		return writeJSON(deps.Stdout, report)
	}
	if len(summaries) == 0 {
		return writeText(deps.Stdout, i18n.T("cli.repo.list.empty"))
	}
	st := outputStyle(deps, opts)
	rows := make([][]string, len(summaries))
	for i, sum := range summaries {
		author := "-"
		if sum.LastCommit.SHA != "" {
			author = sum.LastCommit.AuthorDisplayName()
		}
		rows[i] = []string{
			sum.DisplayName(),
			activity.Relative(sum.LastCommitAt, now, s.loc, st.unicode()),
			author,
			states[i].Short(st.unicode()),
		}
	}
	// En-têtes traduits ; la variante ASCII (st.pick) ne sert qu'aux accents du texte français.
	headers := []string{
		st.pick(i18n.T("cli.repo.list.header_repo"), i18n.T("cli.repo.list.header_repo_ascii")),
		i18n.T("cli.repo.list.header_commit"),
		i18n.T("cli.repo.list.header_author"),
		st.pick(i18n.T("cli.repo.list.header_state"), i18n.T("cli.repo.list.header_state_ascii")),
	}
	out := renderTable(st, headers, rows, func(row int) bool { return states[row].State.Gray() })
	for _, sum := range summaries {
		if sum.ConventionsSince != "" {
			out += fmt.Sprintf(i18n.T("cli.repo.list.conventions_since"), sum.DisplayName(), sum.ConventionsSince)
		}
	}
	return writeText(deps.Stdout, out)
}

// repoRemoveReport est la sortie JSON de « watchoo repo remove ».
type repoRemoveReport struct {
	// Action : deleted (dépôt et historique supprimés), stopped (suivi arrêté, historique gardé) ou
	// already_stopped (--keep-history sur un suivi déjà arrêté : rien n'a changé).
	Action         string         `json:"action"`
	Repository     repositoryJSON `json:"repository"`
	DeletedCommits int            `json:"deleted_commits"` // commits supprimés de la base (deleted)
}

// newRepoRemoveCmd construit « watchoo repo remove owner/repo [--keep-history] [--yes] ».
func newRepoRemoveCmd(deps Deps, opts *options) *cobra.Command {
	var keepHistory, yes bool
	cmd := &cobra.Command{
		Use:   "remove owner/repo",
		Short: i18n.T("cli.repo.help.remove_short"),
		Long:  i18n.T("cli.repo.help.remove_long"),
		Example: `  watchoo repo remove alice/projet-licence
  watchoo repo remove alice/projet-licence --keep-history --yes`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoRemove(cmd.Context(), deps, opts, args[0], keepHistory, yes)
		},
	}
	cmd.Flags().BoolVar(&keepHistory, "keep-history", false, i18n.T("cli.repo.help.remove_flag_keep"))
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, i18n.T("cli.repo.help.remove_flag_yes"))
	return cmd
}

// runRepoRemove arrête le suivi du dépôt ref, après confirmation, en supprimant (DeleteRepository) ou
// en gardant (SetRepositoryEnabled) son historique.
func runRepoRemove(ctx context.Context, deps Deps, opts *options, ref string, keepHistory, yes bool) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	repo, err := s.findRepository(ctx, ref)
	if err != nil {
		return err
	}
	report := repoRemoveReport{Repository: newRepositoryJSON(repo)}
	if keepHistory && !repo.Enabled {
		report.Action = "already_stopped"
		if opts.json {
			return writeJSON(deps.Stdout, report)
		}
		return writeText(deps.Stdout, i18n.T("cli.repo.remove.already_stopped", repo.FullName()))
	}
	commits, err := s.store.CountCommits(ctx, repo.ID)
	if err != nil {
		return err
	}
	if !yes {
		prompt := i18n.T("cli.repo.remove.prompt_delete", repo.FullName(),
			countText(commits, i18n.T("cli.repo.word.commit"), i18n.T("cli.repo.word.commits")))
		if keepHistory {
			prompt = i18n.T("cli.repo.remove.prompt_keep", repo.FullName())
		}
		ok, err := confirm(ctx, deps, prompt)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New(i18n.T("cli.repo.remove.cancelled"))
		}
	}
	if keepHistory {
		if err := s.store.SetRepositoryEnabled(ctx, repo.ID, false); err != nil {
			return err
		}
		repo.Enabled = false
		report.Action, report.Repository = "stopped", newRepositoryJSON(repo)
		if opts.json {
			return writeJSON(deps.Stdout, report)
		}
		return writeText(deps.Stdout, i18n.T("cli.repo.remove.stopped", repo.FullName(), repo.FullName()))
	}
	if err := wsync.DeleteRepository(ctx, s.store, s.ws.ReposDir(), repo); err != nil {
		return err
	}
	report.Action, report.DeletedCommits = "deleted", commits
	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	return writeText(deps.Stdout, i18n.T("cli.repo.remove.deleted", repo.FullName(),
		countText(commits, i18n.T("cli.repo.word.commit"), i18n.T("cli.repo.word.commits"))))
}

// countText renvoie « 1 commit », « 0 commit » ou « 12 commits ».
func countText(n int, one, many string) string {
	if n > 1 {
		return strconv.Itoa(n) + " " + many
	}
	return strconv.Itoa(n) + " " + one
}
