package cli

// « watchoo prs [--repo owner/repo] [--state open|merged|closed|all] [-n 20] » (V2) :
// store.ListPullRequests, de la plus récemment modifiée à la plus ancienne, dates relatives en heure
// locale.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// defaultPRLimit est le nombre de pull requests affichées par « watchoo prs » sans -n (et de
// releases et de tags par « watchoo releases »).
const defaultPRLimit = 20

// prStateAll est la valeur de --state qui ne filtre pas.
const prStateAll = "all"

// loginJSON désigne l'auteur d'une pull request ou d'une release (login vide s'il est inconnu).
type loginJSON struct {
	Login string `json:"login"`
}

// pullRequestJSON décrit une pull request.
type pullRequestJSON struct {
	Repository repoRefJSON `json:"repository"`
	Number     int         `json:"number"`
	Title      string      `json:"title"`
	State      string      `json:"state"` // open, merged ou closed (fermée sans merge)
	Draft      bool        `json:"draft"`
	Author     loginJSON   `json:"author"`
	URL        string      `json:"url"`
	BaseBranch string      `json:"base_branch"`
	HeadBranch string      `json:"head_branch"`
	CreatedAt  *time.Time  `json:"created_at"`
	UpdatedAt  *time.Time  `json:"updated_at"`
	ClosedAt   *time.Time  `json:"closed_at"`
	MergedAt   *time.Time  `json:"merged_at"`
}

// newPullRequestJSON convertit une ligne de la vue des pull requests.
func newPullRequestJSON(p store.PullRequestView) pullRequestJSON {
	return pullRequestJSON{
		Repository: repoRefJSON{ID: p.RepoID, FullName: p.RepoFullName(), DisplayName: p.RepoDisplayName()},
		Number:     p.Number,
		Title:      p.Title,
		State:      string(p.Status()),
		Draft:      p.Draft,
		Author:     loginJSON{Login: p.AuthorLogin},
		URL:        p.URL,
		BaseBranch: p.BaseBranch,
		HeadBranch: p.HeadBranch,
		CreatedAt:  timeJSON(p.CreatedAt),
		UpdatedAt:  timeJSON(p.UpdatedAt),
		ClosedAt:   timeJSON(p.ClosedAt),
		MergedAt:   timeJSON(p.MergedAt),
	}
}

// prsReport est la sortie JSON de « watchoo prs ».
type prsReport struct {
	PullRequests []pullRequestJSON `json:"pull_requests"` // de la plus récemment modifiée à la plus ancienne
}

// newPRsCmd construit « watchoo prs ».
func newPRsCmd(deps Deps, opts *options) *cobra.Command {
	var repo, state string
	var limit int
	cmd := &cobra.Command{
		Use:   "prs",
		Short: "Affiche les pull requests des dépôts suivis",
		Long: `Affiche les pull requests enregistrées par les synchros, de la plus récemment
modifiée à la plus ancienne : date de modification, dépôt, numéro, état, auteur et
titre.

Filtres :
  --repo     un dépôt suivi (owner/repo ou URL)
  --state    open (ouvertes, brouillons compris), merged (mergées), closed (fermées
             sans merge) ou all (toutes, par défaut)
  -n         nombre maximum de pull requests (20 par défaut, 0 = toutes)`,
		Example: `  watchoo prs --state open
  watchoo prs --repo alice/projet-licence --state merged -n 50`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPRs(cmd.Context(), deps, opts, repo, state, limit)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "limite au `dépôt` owner/repo")
	cmd.Flags().StringVar(&state, "state", prStateAll, "limite à un `état` : open, merged, closed ou all")
	cmd.Flags().IntVarP(&limit, "limit", "n", defaultPRLimit, "`nombre` maximum de pull requests, 0 = toutes")
	return cmd
}

// parsePRState convertit --state en filtre du store (vide = toutes).
func parsePRState(state string) (model.PullRequestStatus, error) {
	switch s := model.PullRequestStatus(strings.ToLower(strings.TrimSpace(state))); s {
	case prStateAll, "":
		return "", nil
	case model.PullRequestStatusOpen, model.PullRequestStatusMerged, model.PullRequestStatusClosed:
		return s, nil
	}
	return "", fmt.Errorf("--state %q invalide : attendu open, merged, closed ou all", state)
}

// runPRs affiche les pull requests filtrées.
func runPRs(ctx context.Context, deps Deps, opts *options, repoRef, state string, limit int) (err error) {
	// Les options sont vérifiées avant d'ouvrir l'espace : une erreur de saisie est signalée d'abord.
	if limit < 0 {
		return fmt.Errorf("-n %d invalide : attendu un nombre positif (0 = toutes les pull requests)", limit)
	}
	status, err := parsePRState(state)
	if err != nil {
		return err
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	filter := store.PullRequestFilter{Status: status, Limit: limit}
	if repoRef != "" {
		repo, err := s.findRepository(ctx, repoRef)
		if err != nil {
			return err
		}
		filter.RepoID = repo.ID
	}
	pulls, err := s.store.ListPullRequests(ctx, filter)
	if err != nil {
		return err
	}
	if opts.json {
		report := prsReport{PullRequests: make([]pullRequestJSON, 0, len(pulls))}
		for _, p := range pulls {
			report.PullRequests = append(report.PullRequests, newPullRequestJSON(p))
		}
		return writeJSON(deps.Stdout, report)
	}
	if len(pulls) == 0 {
		return writeText(deps.Stdout, "Aucune pull request"+prFilterText(repoRef, status)+".\n")
	}
	st := outputStyle(deps, opts)
	now := s.now()
	rows := make([][]string, len(pulls))
	for i, p := range pulls {
		rows[i] = []string{
			activity.Relative(p.UpdatedAt, now, s.loc, st.unicode()),
			p.RepoDisplayName(),
			"#" + strconv.Itoa(p.Number),
			prStatusText(st, p.PullRequest),
			cmpOr(p.AuthorLogin, "-"),
			st.truncate(p.Title, 60),
		}
	}
	headers := []string{st.pick("Modifiée", "Modifiee"), st.pick("Dépôt", "Depot"), "PR", st.pick("État", "Etat"),
		"Auteur", "Titre"}
	return writeText(deps.Stdout, renderTable(st, headers, rows, func(row int) bool {
		return pulls[row].Status() == model.PullRequestStatusClosed
	}))
}

// prStatusText renvoie l'état affiché d'une pull request : ouverte, brouillon, mergée ou fermée.
func prStatusText(st textStyle, p model.PullRequest) string {
	switch p.Status() {
	case model.PullRequestStatusMerged:
		return st.pick("mergée", "mergee")
	case model.PullRequestStatusClosed:
		return st.pick("fermée", "fermee")
	}
	if p.Draft {
		return "brouillon"
	}
	return "ouverte"
}

// prFilterText décrit les filtres de « watchoo prs » pour le message « aucune pull request ».
func prFilterText(repoRef string, status model.PullRequestStatus) string {
	var parts []string
	if repoRef != "" {
		parts = append(parts, "dépôt "+repoRef)
	}
	if status != "" {
		parts = append(parts, "état "+string(status))
	}
	if len(parts) == 0 {
		return " en base (lancez « watchoo sync »)"
	}
	return " pour ces filtres (" + strings.Join(parts, ", ") + ")"
}

// cmpOr renvoie s, ou def s'il est vide.
func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
