package cli

// « watchoo log [--repo owner/repo] [--author X] [--branch B] [--since 7d] [-n 20] » :
// store.ListRecentCommits, dates relatives en heure locale.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
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
		Short: "Affiche les derniers commits, filtrés par dépôt, auteur, branche ou période",
		Long: `Affiche les derniers commits enregistrés par les synchros, du plus récent au plus
ancien, chacun une seule fois même s'il est sur plusieurs branches.

Filtres :
  --repo     un dépôt suivi (owner/repo ou URL)
  --author   login GitHub, nom ou e-mail exact de l'auteur (sans tenir compte de la casse)
  --branch   commits vus sur cette branche lors des synchros : ceux qu'elle partage avec la
             branche de référence (souvent main) n'y sont rattachés que s'ils y ont été vus
  --since    période relative : 7d (jours), 12h (heures), 30m (minutes)
  -n         nombre maximum de commits (20 par défaut, 0 = tous)`,
		Example: `  watchoo log --since 7d
  watchoo log --repo alice/projet-licence --author alice -n 50`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLog(cmd.Context(), deps, opts, f)
		},
	}
	cmd.Flags().StringVar(&f.repo, "repo", "", "limite au `dépôt` owner/repo")
	cmd.Flags().StringVar(&f.author, "author", "", "limite à un `auteur` (login, nom ou e-mail exact)")
	cmd.Flags().StringVar(&f.branch, "branch", "", "limite à une `branche`")
	cmd.Flags().StringVar(&f.since, "since", "", "limite à une `période` récente (ex. 7d, 12h, 30m)")
	cmd.Flags().IntVarP(&f.limit, "limit", "n", defaultLogLimit, "`nombre` maximum de commits (0 = tous)")
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
		return fmt.Errorf("-n %d invalide : attendu un nombre positif (0 = tous les commits)", f.limit)
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
		return writeText(deps.Stdout, "Aucun commit ne correspond"+filterText(f)+".\n")
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
			st.truncate(c.Message, 60),
		}
	}
	headers := []string{"Date", st.pick("Dépôt", "Depot"), "Auteur", "Commit", "Message"}
	return writeText(deps.Stdout, renderTable(st, headers, rows, nil))
}

// filterText décrit les filtres de « watchoo log » pour le message « aucun commit » : vide sans filtre.
func filterText(f logFlags) string {
	var parts []string
	if f.repo != "" {
		parts = append(parts, "dépôt "+f.repo)
	}
	if f.author != "" {
		parts = append(parts, "auteur "+f.author)
	}
	if f.branch != "" {
		parts = append(parts, "branche "+f.branch)
	}
	if f.since != "" {
		parts = append(parts, "depuis "+f.since)
	}
	if len(parts) == 0 {
		return " : la base est vide (lancez « watchoo sync »)"
	}
	return " aux filtres (" + strings.Join(parts, ", ") + ")"
}
