package cli

// « watchoo repo » et ses sous-commandes add, list, remove (import : repo_import.go).

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// newRepoCmd construit « watchoo repo » (add, import, list, remove). Sans sous-commande : son aide.
func newRepoCmd(deps Deps, opts *options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "repo",
		Short: "Gère les dépôts suivis (add, import, list, remove)",
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

// exactArgs exige n arguments positionnels, avec un message en français qui rappelle l'usage.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return fmt.Errorf("« %s » attend %d argument%s (reçu : %d) ; utilisation : %s",
				cmd.CommandPath(), n, plural(n), len(args), cmd.UseLine())
		}
		return nil
	}
}

// maxArgs accepte au plus n arguments positionnels.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf("« %s » attend au plus %d argument%s (reçu : %s) ; utilisation : %s",
				cmd.CommandPath(), n, plural(n), strings.Join(args, " "), cmd.UseLine())
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

// branchesText décrit les branches suivies d'un dépôt : « toutes les branches » ou « branches : a, b ».
func branchesText(patterns []string) string {
	if len(patterns) == 0 {
		return "toutes les branches"
	}
	return "branches : " + strings.Join(patterns, ", ")
}

// repoAddReport est la sortie JSON de « watchoo repo add ».
type repoAddReport struct {
	Outcome    wsync.AddOutcome `json:"outcome"` // added, already ou updated
	Repository repositoryJSON   `json:"repository"`
}

// newRepoAddCmd construit « watchoo repo add owner/repo [--branch main,dev,release/*] [--label X]
// [--flemme N] [--dead N] ».
func newRepoAddCmd(deps Deps, opts *options) *cobra.Command {
	var branch, label string
	var flemme, dead int
	cmd := &cobra.Command{
		Use:   "add owner/repo",
		Short: "Ajoute un dépôt à surveiller",
		Long: `Ajoute un dépôt à surveiller, après avoir vérifié auprès de GitHub que le compte gh y a
accès (sans gh installé et connecté : que git peut le lire, par « git ls-remote », avec la
clé SSH ou les identifiants Git de cette machine). Le dépôt est désigné par owner/repo ou
par son URL GitHub.

Sans --branch, toutes les branches du dépôt sont suivies, y compris celles créées plus
tard. --branch restreint le suivi à une liste de noms et de motifs (release/* par
exemple) ; --branch "" revient à toutes les branches.

--flemme et --dead règlent les seuils d'activité de ce dépôt, en jours sans commit, à la
place de ceux de l'espace (activity.*) ; 0 revient au seuil de l'espace. Quand les deux
sont donnés, --dead doit être plus grand que --flemme.

Un dépôt déjà suivi n'est pas ajouté en double : son label, ses branches et ses seuils
sont mis à jour s'ils sont donnés, et son suivi reprend s'il avait été arrêté. Aucune synchro
n'est lancée : enchaînez avec « watchoo sync ».`,
		Example: `  watchoo repo add alice/projet-licence --label "Alice Martin"
  watchoo repo add https://github.com/mon-org/site-web --branch main,dev,release/*
  watchoo repo add alice/projet-licence --flemme 3 --dead 10`,
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
					return fmt.Errorf("--branch : %w", err)
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
			return runRepoAdd(cmd.Context(), deps, opts, req)
		},
	}
	cmd.Flags().StringVar(&branch, "branch", "",
		"`branches` à suivre, séparées par des virgules, motifs acceptés (ex. main,dev,release/*) ; défaut : toutes")
	cmd.Flags().StringVar(&label, "label", "", "`nom` affiché à la place de owner/repo (ex. \"Alice Martin\")")
	cmd.Flags().IntVar(&flemme, "flemme", 0,
		"seuil Flemme de ce dépôt, en `jours` sans commit (0 = seuil de l'espace)")
	cmd.Flags().IntVar(&dead, "dead", 0,
		"seuil Dead de ce dépôt, en `jours` sans commit, plus grand que --flemme (0 = seuil de l'espace)")
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
		return fmt.Errorf("ajout du dépôt %s : résultat manquant", req.Ref)
	}
	res := results[0]
	switch res.Outcome {
	case wsync.OutcomeInaccessible:
		return fmt.Errorf("dépôt %s inaccessible : %s (vérifiez son nom et que le compte gh, ou sans gh "+
			"la clé SSH ou les identifiants Git de cette machine, y a accès)", req.Ref, res.Error)
	case wsync.OutcomeError:
		return fmt.Errorf("ajout du dépôt %s : %s", req.Ref, res.Error)
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
		details = append(details, fmt.Sprintf("label « %s »", textclean.Clean(repo.Label)))
	}
	details = append(details, branchesText(repo.BranchPatterns))
	if t := thresholdsText(repo.FlemmeAfterDays, repo.DeadAfterDays); t != "" {
		details = append(details, t)
	}
	var b strings.Builder
	switch res.Outcome {
	case wsync.OutcomeAdded:
		fmt.Fprintf(&b, "Dépôt %s ajouté (%s).\n", repo.FullName(), strings.Join(details, ", "))
	case wsync.OutcomeUpdated:
		fmt.Fprintf(&b, "Dépôt %s déjà suivi, mis à jour (%s).\n", repo.FullName(), strings.Join(details, ", "))
	default:
		fmt.Fprintf(&b, "Dépôt %s déjà suivi : rien à changer (%s).\n", repo.FullName(), strings.Join(details, ", "))
	}
	if !strings.EqualFold(res.Request.Ref.String(), repo.FullName()) {
		fmt.Fprintf(&b, "Le dépôt %s s'appelle maintenant %s sur GitHub.\n", res.Request.Ref, repo.FullName())
	}
	if res.Outcome != wsync.OutcomeAlreadyTracked {
		b.WriteString("Lancez « watchoo sync » pour le synchroniser.\n")
	}
	return b.String()
}

// thresholdsText décrit les seuils propres à un dépôt (« seuils : flemme 3 j, dead 10 j »), ou "" s'il
// n'en a aucun (seuils de l'espace).
func thresholdsText(flemme, dead *int) string {
	var parts []string
	if flemme != nil {
		parts = append(parts, fmt.Sprintf("flemme %d j", *flemme))
	}
	if dead != nil {
		parts = append(parts, fmt.Sprintf("dead %d j", *dead))
	}
	if len(parts) == 0 {
		return ""
	}
	return "seuils : " + strings.Join(parts, ", ")
}

// writeText écrit s sur w, avec une erreur enveloppée.
func writeText(w io.Writer, s string) error {
	if _, err := w.Write([]byte(s)); err != nil {
		return fmt.Errorf("écriture de la sortie : %w", err)
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
		Short: "Liste les dépôts suivis : dernier commit, auteur, état",
		Long: `Liste les dépôts de l'espace avec leur dernier commit (toutes branches suivies
confondues), son auteur et leur état :
  -        Actif   (dernier commit il y a 0 à 5 jours, par défaut)
  ~ 9 j    Flemme  (6 à 15 jours)
  ⚠ 42 j   Dead    (plus de 15 jours)
En gris : arrêté (suivi arrêté), erreur (dernière synchro en échec), jamais
(jamais synchronisé), archivé (archivé sur GitHub), vide (aucun commit).`,
		Args: noArgs,
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
		return writeText(deps.Stdout, "Aucun dépôt suivi dans cet espace. Ajoutez-en avec "+
			"« watchoo repo add owner/repo » ou « watchoo repo import fichier.csv ».\n")
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
	headers := []string{st.pick("Dépôt", "Depot"), "Dernier commit", "Auteur", st.pick("État", "Etat")}
	return writeText(deps.Stdout, renderTable(st, headers, rows, func(row int) bool { return states[row].State.Gray() }))
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
		Short: "Arrête le suivi d'un dépôt et supprime son historique",
		Long: `Arrête le suivi d'un dépôt et supprime de la base son historique : branches, commits
et journal des synchros, ainsi que son clone Git local (repos/ dans l'espace) s'il
existe. Les contributeurs restent (ils peuvent avoir travaillé sur d'autres dépôts).
Rien n'est modifié sur GitHub.

Avec --keep-history, le suivi s'arrête mais l'historique reste consultable (le dépôt
apparaît « arrêté » dans « watchoo repo list ») et le clone Git local est gardé ;
« watchoo repo add » le reprend.

La commande demande confirmation dans un terminal ; dans un script (entrée standard
qui n'est pas un terminal), l'option --yes est obligatoire.`,
		Example: `  watchoo repo remove alice/projet-licence
  watchoo repo remove alice/projet-licence --keep-history --yes`,
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRepoRemove(cmd.Context(), deps, opts, args[0], keepHistory, yes)
		},
	}
	cmd.Flags().BoolVar(&keepHistory, "keep-history", false, "arrête le suivi mais garde l'historique en base")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirme sans poser de question (obligatoire hors terminal)")
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
		return writeText(deps.Stdout, fmt.Sprintf("Le suivi de %s est déjà arrêté : rien n'a changé.\n", repo.FullName()))
	}
	commits, err := s.store.CountCommits(ctx, repo.ID)
	if err != nil {
		return err
	}
	if !yes {
		prompt := fmt.Sprintf("Arrêter le suivi de %s et supprimer son historique (%s) ?",
			repo.FullName(), countText(commits, "commit", "commits"))
		if keepHistory {
			prompt = fmt.Sprintf("Arrêter le suivi de %s (l'historique est gardé) ?", repo.FullName())
		}
		ok, err := confirm(ctx, deps, prompt)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("suppression annulée : rien n'a été modifié")
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
		return writeText(deps.Stdout, fmt.Sprintf(
			"Suivi de %s arrêté ; son historique est gardé (« watchoo repo add %s » le reprend).\n",
			repo.FullName(), repo.FullName()))
	}
	if err := wsync.DeleteRepository(ctx, s.store, s.ws.ReposDir(), repo); err != nil {
		return err
	}
	report.Action, report.DeletedCommits = "deleted", commits
	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	return writeText(deps.Stdout, fmt.Sprintf("Dépôt %s retiré : suivi arrêté, historique supprimé (%s).\n",
		repo.FullName(), countText(commits, "commit", "commits")))
}

// countText renvoie « 1 commit », « 0 commit » ou « 12 commits ».
func countText(n int, one, many string) string {
	if n > 1 {
		return strconv.Itoa(n) + " " + many
	}
	return strconv.Itoa(n) + " " + one
}
