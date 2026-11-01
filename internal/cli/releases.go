package cli

// « watchoo releases [--repo owner/repo] [-n 20] » (V2) : store.ListReleases puis store.ListTags,
// deux tableaux (releases, puis tags), dates relatives en heure locale.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// releaseJSON décrit une release.
type releaseJSON struct {
	Repository  repoRefJSON `json:"repository"`
	ID          int64       `json:"id"` // identifiant sur la forge
	TagName     string      `json:"tag_name"`
	Name        string      `json:"name"`
	Draft       bool        `json:"draft"`
	Prerelease  bool        `json:"prerelease"`
	Author      loginJSON   `json:"author"`
	URL         string      `json:"url"`
	CreatedAt   *time.Time  `json:"created_at"`
	PublishedAt *time.Time  `json:"published_at"` // null pour un brouillon
}

// tagJSON décrit un tag et le commit visé.
type tagJSON struct {
	Repository  repoRefJSON `json:"repository"`
	Name        string      `json:"name"`
	SHA         string      `json:"sha"`
	ShortSHA    string      `json:"short_sha"`
	CommitURL   string      `json:"commit_url"`   // "" si le commit n'est pas en base
	CommittedAt *time.Time  `json:"committed_at"` // null si le commit n'est pas en base
}

// releasesReport est la sortie JSON de « watchoo releases ».
type releasesReport struct {
	Releases []releaseJSON `json:"releases"` // de la plus récente à la plus ancienne
	Tags     []tagJSON     `json:"tags"`     // commit le plus récent d'abord
}

// newReleasesCmd construit « watchoo releases ».
func newReleasesCmd(deps Deps, opts *options) *cobra.Command {
	var repo string
	var limit int
	cmd := &cobra.Command{
		Use:   "releases",
		Short: i18n.T("cli.sync.releases_short"),
		Long:  i18n.T("cli.sync.releases_long"),
		Example: `  watchoo releases
  watchoo releases --repo alice/projet-licence --json`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReleases(cmd.Context(), deps, opts, repo, limit)
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", i18n.T("cli.sync.flag_repo"))
	cmd.Flags().IntVarP(&limit, "limit", "n", defaultPRLimit, i18n.T("cli.sync.flag_releases_limit"))
	return cmd
}

// runReleases affiche les releases puis les tags.
func runReleases(ctx context.Context, deps Deps, opts *options, repoRef string, limit int) (err error) {
	if limit < 0 {
		return errors.New(i18n.T("cli.sync.releases_bad_limit", limit))
	}
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	filter := store.ReleaseFilter{Limit: limit}
	if repoRef != "" {
		repo, err := s.findRepository(ctx, repoRef)
		if err != nil {
			return err
		}
		filter.RepoID = repo.ID
	}
	releases, err := s.store.ListReleases(ctx, filter)
	if err != nil {
		return err
	}
	tags, err := s.store.ListTags(ctx, filter)
	if err != nil {
		return err
	}
	if opts.json {
		report := releasesReport{Releases: make([]releaseJSON, 0, len(releases)), Tags: make([]tagJSON, 0, len(tags))}
		for _, r := range releases {
			report.Releases = append(report.Releases, releaseJSON{
				Repository:  repoRefJSON{ID: r.RepoID, FullName: r.RepoFullName(), DisplayName: r.RepoDisplayName()},
				ID:          r.ProviderID,
				TagName:     r.TagName,
				Name:        r.Name,
				Draft:       r.Draft,
				Prerelease:  r.Prerelease,
				Author:      loginJSON{Login: r.AuthorLogin},
				URL:         r.URL,
				CreatedAt:   timeJSON(r.CreatedAt),
				PublishedAt: timeJSON(r.PublishedAt),
			})
		}
		for _, t := range tags {
			report.Tags = append(report.Tags, tagJSON{
				Repository:  repoRefJSON{ID: t.RepoID, FullName: t.RepoFullName(), DisplayName: t.RepoDisplayName()},
				Name:        t.Name,
				SHA:         t.SHA,
				ShortSHA:    provider.ShortSHA(t.SHA),
				CommitURL:   t.CommitURL,
				CommittedAt: timeJSON(t.CommittedAt),
			})
		}
		return writeJSON(deps.Stdout, report)
	}

	where := i18n.T("cli.sync.filter_in_db")
	if repoRef != "" {
		where = i18n.T("cli.sync.releases_filter_repo", repoRef)
	}
	st := outputStyle(deps, opts)
	now := s.now()
	var b strings.Builder
	b.WriteString(st.bold("Releases") + "\n")
	if len(releases) == 0 {
		b.WriteString(i18n.T("cli.sync.releases_none", where))
	} else {
		rows := make([][]string, len(releases))
		for i, r := range releases {
			rows[i] = []string{
				activity.Relative(r.Date(), now, s.loc, st.unicode()),
				r.RepoDisplayName(),
				r.TagName,
				st.truncate(cmpOr(r.Name, "-"), 40),
				releaseStatusText(st, r.Release),
				cmpOr(r.AuthorLogin, "-"),
			}
		}
		headers := []string{"Date", st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
			"Tag", i18n.T("cli.sync.header_title"),
			st.pick(i18n.T("cli.sync.header_state"), i18n.T("cli.sync.header_state_ascii")),
			i18n.T("cli.sync.header_author")}
		b.WriteString(renderTable(st, headers, rows, func(row int) bool { return releases[row].Draft }))
	}
	b.WriteString("\n" + st.bold("Tags") + "\n")
	if len(tags) == 0 {
		b.WriteString(i18n.T("cli.sync.tags_none", where))
	} else {
		rows := make([][]string, len(tags))
		for i, t := range tags {
			date := "-"
			if !t.CommittedAt.IsZero() {
				date = activity.Relative(t.CommittedAt, now, s.loc, st.unicode())
			}
			rows[i] = []string{t.RepoDisplayName(), t.Name, provider.ShortSHA(t.SHA), date}
		}
		headers := []string{st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
			"Tag", "Commit", i18n.T("cli.sync.header_commit_date")}
		b.WriteString(renderTable(st, headers, rows, nil))
	}
	return writeText(deps.Stdout, b.String())
}

// releaseStatusText renvoie l'état affiché d'une release : brouillon, pré-release ou publiée ; la
// variante ASCII (st.pick) ne sert qu'aux accents du texte français.
func releaseStatusText(st textStyle, r model.Release) string {
	switch {
	case r.Draft:
		return i18n.T("cli.sync.prs_status_draft")
	case r.Prerelease:
		return st.pick(i18n.T("cli.sync.release_status_prerelease"), i18n.T("cli.sync.release_status_prerelease_ascii"))
	default:
		return st.pick(i18n.T("cli.sync.release_status_published"), i18n.T("cli.sync.release_status_published_ascii"))
	}
}
