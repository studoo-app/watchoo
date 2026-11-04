package cli

// « watchoo diagnostic backfill [--repo owner/repo] » (phase 7.3) : reprise de l'historique du
// diagnostic sur les clones Git locaux (sync.Engine.BackfillSignals). Le bouton de l'interface web
// viendra avec les pages web du diagnostic (phase 7.4).

import (
	"context"
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// backfillRepoJSON est le bilan de la reprise d'un dépôt.
type backfillRepoJSON struct {
	Repository repoRefJSON `json:"repository"`
	Measured   int         `json:"measured"` // commits mesurés
	Merges     int         `json:"merges"`   // commits de fusion écartés
	Missing    int         `json:"missing"`  // commits absents du clone
	Error      string      `json:"error"`    // échec du dépôt ; vide sinon
}

// backfillReport est la sortie JSON de « watchoo diagnostic backfill ».
type backfillReport struct {
	Repositories []backfillRepoJSON `json:"repositories"`
	Measured     int                `json:"measured"` // total des commits mesurés
}

// newDiagnosticBackfillCmd construit « watchoo diagnostic backfill ».
func newDiagnosticBackfillCmd(deps Deps, opts *options) *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:     "backfill",
		Short:   i18n.T("cli.diag.backfill_short"),
		Long:    i18n.T("cli.diag.backfill_long"),
		Example: "  watchoo diagnostic backfill\n  watchoo diagnostic backfill --repo alice/projet-licence",
		Args:    noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiagnosticBackfill(cmd.Context(), deps, opts, repo)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", i18n.T("cli.sync.flag_repo"))
	return cmd
}

// runDiagnosticBackfill reprend l'historique et affiche le bilan ; code non nul si un dépôt a échoué.
func runDiagnosticBackfill(ctx context.Context, deps Deps, opts *options, repoRef string) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	var repoID int64
	if repoRef != "" {
		repo, err := s.findRepository(ctx, repoRef)
		if err != nil {
			return err
		}
		repoID = repo.ID
	}
	eng, err := s.engine()
	if err != nil {
		return err
	}
	res, err := eng.BackfillSignals(ctx, repoID)
	if err != nil {
		return err
	}
	report := backfillReport{Repositories: make([]backfillRepoJSON, 0, len(res))}
	failed := 0
	var lines []string
	for _, r := range res {
		report.Repositories = append(report.Repositories, backfillRepoJSON{
			Repository: repoRefJSON{ID: r.Repo.ID, FullName: r.Repo.FullName(), DisplayName: displayName(r.Repo.Label, r.Repo.FullName())},
			Measured:   r.Measured, Merges: r.Merges, Missing: r.Missing, Error: r.Error,
		})
		report.Measured += r.Measured
		name := displayName(r.Repo.Label, r.Repo.FullName())
		if r.Error != "" {
			failed++
			lines = append(lines, i18n.T("cli.diag.backfill_failed", name, r.Error))
			continue
		}
		lines = append(lines, i18n.T("cli.diag.backfill_repo", name, r.Measured, r.Merges))
	}
	if opts.json {
		if err := writeJSON(deps.Stdout, report); err != nil {
			return err
		}
	} else {
		if len(lines) == 0 {
			lines = append(lines, i18n.T("cli.diag.backfill_none"))
		}
		lines = append(lines, "", i18n.T("cli.diag.backfill_total", report.Measured))
		if err := writeText(deps.Stdout, strings.Join(lines, "\n")); err != nil {
			return err
		}
	}
	if failed > 0 {
		return errors.New(i18n.T("cli.diag.backfill_errors", failed))
	}
	return nil
}

// displayName renvoie le nom affiché d'un dépôt : label, sinon owner/name.
func displayName(label, fullName string) string {
	if label != "" {
		return label
	}
	return fullName
}
