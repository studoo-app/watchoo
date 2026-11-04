package cli

// « watchoo languages [--repo owner/repo] » (1.1.0) : store.ListLanguages, un tableau avec la part de
// chaque langage dans son dépôt.

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// languageJSON décrit un langage d'un dépôt.
type languageJSON struct {
	Repository repoRefJSON `json:"repository"`
	Name       string      `json:"name"`
	Bytes      int64       `json:"bytes"`   // taille de ses fichiers sur la branche par défaut
	Percent    float64     `json:"percent"` // part du dépôt, une décimale
}

// languagesReport est la sortie JSON de « watchoo languages ».
type languagesReport struct {
	Languages []languageJSON `json:"languages"` // dépôt après dépôt, plus gros langage d'abord
}

// newLanguagesCmd construit « watchoo languages ».
func newLanguagesCmd(deps Deps, opts *options) *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "languages",
		Short: i18n.T("cli.sync.languages_short"),
		Long:  i18n.T("cli.sync.languages_long"),
		Example: `  watchoo languages
  watchoo languages --repo alice/projet-licence --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLanguages(cmd.Context(), deps, opts, repo)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", i18n.T("cli.sync.flag_repo"))
	return cmd
}

// runLanguages affiche les langages des dépôts suivis.
func runLanguages(ctx context.Context, deps Deps, opts *options, repoRef string) (err error) {
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
	languages, err := s.store.ListLanguages(ctx, repoID)
	if err != nil {
		return err
	}
	if opts.json {
		report := languagesReport{Languages: make([]languageJSON, 0, len(languages))}
		for _, l := range languages {
			report.Languages = append(report.Languages, languageJSON{
				Repository: repoRefJSON{ID: l.RepoID, FullName: l.RepoFullName(), DisplayName: l.RepoDisplayName()},
				Name:       l.Language,
				Bytes:      l.Bytes,
				Percent:    float64(l.Permille()) / 10,
			})
		}
		return writeJSON(deps.Stdout, report)
	}
	if len(languages) == 0 {
		return writeText(deps.Stdout, i18n.T("cli.sync.languages_none"))
	}
	st := outputStyle(deps, opts)
	rows := make([][]string, len(languages))
	for i, l := range languages {
		rows[i] = []string{l.RepoDisplayName(), l.Language, activity.PercentText(l.Permille()), byteSize(l.Bytes)}
	}
	headers := []string{st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
		i18n.T("cli.sync.header_language"), "%", i18n.T("cli.sync.header_size")}
	return writeText(deps.Stdout, renderTable(st, headers, rows, nil))
}

// byteSize écrit une taille en octets, Ko ou Mo (1 024 octets = 1 Ko, chiffres entiers).
func byteSize(n int64) string {
	const kib, mib = 1024, 1024 * 1024
	switch {
	case n >= mib:
		return fmt.Sprintf(i18n.T("cli.sync.size_mb"), n/mib)
	case n >= kib:
		return fmt.Sprintf(i18n.T("cli.sync.size_kb"), n/kib)
	}
	return fmt.Sprintf(i18n.T("cli.sync.size_b"), n)
}
