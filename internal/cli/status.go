package cli

// « watchoo status » : carte d'en-tête (terminal, hors --json), compte gh et scopes
// (auth.Client.Status), source des synchros (wsync.Engine.Source), quota (wsync.Engine.RateLimit), dépôts à jour / Flemme / Dead / en erreur /
// jamais synchronisés (activity), dernière synchro, branches suivies ; jamais le token.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// statusGitHub décrit le compte gh dans la sortie de « watchoo status » (jamais le token).
type statusGitHub struct {
	OK          bool     `json:"ok"` // gh installé, connecté, token valide
	Host        string   `json:"host"`
	Login       string   `json:"login"`
	TokenSource string   `json:"token_source"`
	Scopes      []string `json:"scopes"`
	Error       string   `json:"error"` // vide si OK
}

// statusSource décrit la source des synchros (V2) : API GitHub avec gh, ou clones Git locaux sans gh.
type statusSource struct {
	// Kind : « api », « git », ou vide si gh est installé mais en panne (Reason dit pourquoi).
	Kind wsync.SourceKind `json:"kind"`
	// Reason : pourquoi les clones Git locaux (gh absent ou non connecté), ou l'erreur de gh ; vide
	// pour l'API.
	Reason string `json:"reason"`
}

// statusRepoCounts compte les dépôts par état (activity.State).
type statusRepoCounts struct {
	Total       int `json:"total"`
	Active      int `json:"active"`
	Flemme      int `json:"flemme"`
	Dead        int `json:"dead"`
	Error       int `json:"error"`
	NeverSynced int `json:"never_synced"`
	Archived    int `json:"archived"`
	Empty       int `json:"empty"`
	Stopped     int `json:"stopped"`
}

// statusRepoError est un dépôt dont la dernière synchro a échoué.
type statusRepoError struct {
	Repo    string `json:"repo"` // owner/name
	Message string `json:"message"`
}

// statusReport est la sortie JSON de « watchoo status ».
type statusReport struct {
	Workspace struct {
		Name string `json:"name"`
		Root string `json:"root"`
	} `json:"workspace"`
	GitHub         statusGitHub   `json:"github"`
	Source         statusSource   `json:"source"`
	RateLimit      *rateLimitJSON `json:"rate_limit"`       // null si inconnu
	RateLimitError string         `json:"rate_limit_error"` // pourquoi le quota est inconnu
	// QuotaLow : garde-fou actif (wsync.QuotaLow, quota restant sous 10 %) : les prochaines synchros
	// reportent les dépôts jusqu'à rate_limit.reset.
	QuotaLow        bool              `json:"quota_low"`
	Repositories    statusRepoCounts  `json:"repositories"`
	Errors          []statusRepoError `json:"errors"`
	LastSyncedAt    *time.Time        `json:"last_synced_at"` // dernière synchro réussie d'un dépôt
	TrackedBranches int               `json:"tracked_branches"`
}

// newStatusCmd construit « watchoo status ».
func newStatusCmd(deps Deps, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: i18n.T("cli.sync.status_short"),
		Long:  i18n.T("cli.sync.status_long"),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(cmd.Context(), deps, opts)
		},
	}
}

// runStatus rassemble et affiche le résumé de l'espace.
func runStatus(ctx context.Context, deps Deps, opts *options) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)

	var report statusReport
	report.Workspace.Name, report.Workspace.Root = s.ws.Config.Name, s.ws.Root
	report.Errors = []statusRepoError{}
	report.GitHub.Scopes = []string{}

	engine, err := s.engine()
	if err != nil {
		return err
	}
	// Source des synchros : choisie par le moteur, selon gh seulement (sans contacter GitHub).
	if src, err := engine.Source(ctx); err != nil {
		report.Source.Reason = err.Error()
	} else {
		report.Source = statusSource{Kind: src.Kind, Reason: src.Reason}
	}

	ghStatus, ghErr := deps.NewAuthClient(s.ws.Config).Status(ctx)
	if ghErr != nil {
		report.GitHub.Error = ghError(ghErr, s.ws.Config).Error()
	} else {
		report.GitHub = statusGitHub{
			OK:          true,
			Host:        ghStatus.Host,
			Login:       ghStatus.Login,
			TokenSource: ghStatus.TokenSource,
			Scopes:      append([]string{}, ghStatus.Scopes...),
		}
		if rl, err := engine.RateLimit(ctx); err != nil {
			report.RateLimitError = err.Error()
		} else {
			report.RateLimit = newRateLimitJSON(rl)
			report.QuotaLow = wsync.QuotaLow(rl, s.now())
		}
	}
	switch {
	case report.Source.Kind == wsync.SourceGit:
		report.RateLimitError = i18n.T("cli.sync.status_rl_na")
	case !report.GitHub.OK:
		report.RateLimitError = i18n.T("cli.sync.status_rl_no_account")
	}

	summaries, err := s.store.RepositorySummaries(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	var lastSynced time.Time
	for _, sum := range summaries {
		c := &report.Repositories
		c.Total++
		switch res := activity.Evaluate(sum.Repository, sum.LastRun, s.ws.Config.Activity, now, s.loc); res.State {
		case activity.StateActive:
			c.Active++
		case activity.StateFlemme:
			c.Flemme++
		case activity.StateDead:
			c.Dead++
		case activity.StateError:
			c.Error++
			report.Errors = append(report.Errors, statusRepoError{Repo: sum.FullName(), Message: sum.LastRun.Error})
		case activity.StateNeverSynced:
			c.NeverSynced++
		case activity.StateArchived:
			c.Archived++
		case activity.StateEmpty:
			c.Empty++
		case activity.StateStopped:
			c.Stopped++
		}
		if sum.Enabled {
			report.TrackedBranches += sum.TrackedBranches
		}
		if sum.LastSyncedAt.After(lastSynced) {
			lastSynced = sum.LastSyncedAt
		}
	}
	report.LastSyncedAt = timeJSON(lastSynced)

	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	var b strings.Builder
	if term, ok := cardTerminal(deps, opts); ok {
		if card := renderCard(term, i18n.T("cli.sync.status_subtitle", report.Workspace.Name),
			i18n.T("cli.core.tip.status")); card != "" {
			b.WriteString(card + "\n")
		}
	}
	b.WriteString(s.statusText(outputStyle(deps, opts), report, lastSynced))
	return writeText(deps.Stdout, b.String())
}

// statusText renvoie le résumé texte de « watchoo status ».
func (s *session) statusText(st textStyle, r statusReport, lastSynced time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", i18n.T("cli.core.status.workspace",
		textclean.Clean(r.Workspace.Name), textclean.Clean(displayPath(r.Workspace.Root))))
	if r.GitHub.OK {
		fmt.Fprintf(&b, "%s\n", i18n.T("cli.core.status.gh_account", textclean.Clean(r.GitHub.Login),
			textclean.Clean(r.GitHub.Host), textclean.Clean(scopesText(r.GitHub.Scopes))))
	} else {
		fmt.Fprintf(&b, "%s\n", i18n.T("cli.sync.status_gh_error", textclean.Clean(r.GitHub.Error)))
	}
	switch r.Source.Kind {
	case wsync.SourceAPI:
		b.WriteString(i18n.T("cli.sync.status_source_api") + "\n")
	case wsync.SourceGit:
		b.WriteString(i18n.T("cli.sync.status_source_git") + "\n")
	default:
		fmt.Fprintf(&b, "%s\n", i18n.T("cli.sync.status_source_unknown", textclean.Clean(r.Source.Reason)))
	}
	if r.RateLimit != nil {
		rl := provider.RateLimit{Limit: r.RateLimit.Limit, Remaining: r.RateLimit.Remaining}
		if r.RateLimit.Reset != nil {
			rl.Reset = *r.RateLimit.Reset
		}
		b.WriteString(s.rateLimitText(rl) + "\n")
		if r.QuotaLow {
			var text string
			if !rl.Reset.IsZero() {
				text = i18n.T("cli.sync.status_quota_low_until", s.clockText(rl.Reset))
			} else {
				text = i18n.T("cli.sync.status_quota_low")
			}
			b.WriteString(text + "\n")
		}
	} else {
		fmt.Fprintf(&b, "%s\n", i18n.T("cli.sync.status_rate_limit_unknown", textclean.Clean(r.RateLimitError)))
	}

	c := r.Repositories
	b.WriteString("\n")
	if c.Total == 0 {
		b.WriteString(i18n.T("cli.sync.status_no_repos"))
		return b.String()
	}
	fmt.Fprintf(&b, "%s\n", i18n.T("cli.sync.status_repos_count", c.Total))
	var errRepos []string
	for _, e := range r.Errors {
		errRepos = append(errRepos, e.Repo)
	}
	lines := []struct {
		label string
		n     int
		extra string
	}{
		{i18n.T("cli.sync.status_lbl_active"), c.Active, ""},
		{i18n.T("cli.sync.status_lbl_flemme"), c.Flemme, ""},
		{i18n.T("cli.sync.status_lbl_dead"), c.Dead, ""},
		{i18n.T("cli.sync.status_lbl_error"), c.Error, strings.Join(errRepos, ", ")},
		{i18n.T("cli.sync.status_lbl_never"), c.NeverSynced, ""},
		{i18n.T("cli.sync.status_lbl_archived"), c.Archived, ""},
		{i18n.T("cli.sync.status_lbl_empty"), c.Empty, ""},
		{i18n.T("cli.sync.status_lbl_stopped"), c.Stopped, ""},
	}
	for i, l := range lines {
		if l.n == 0 && i >= 3 { // Actif, Flemme et Dead toujours ; les états gris s'ils existent
			continue
		}
		text := fmt.Sprintf("  %-20s %3d", l.label, l.n)
		if l.extra != "" {
			text += "   " + st.truncate(l.extra, 60)
		}
		b.WriteString(text + "\n")
	}
	b.WriteString("\n")
	if lastSynced.IsZero() {
		b.WriteString(i18n.T("cli.sync.status_last_sync_never") + "\n")
	} else {
		fmt.Fprintf(&b, "%s\n", i18n.T("cli.sync.status_last_sync",
			activity.Relative(lastSynced, s.now(), s.loc, st.unicode()), lastSynced.In(s.loc).Format("02/01/2006 15:04")))
	}
	fmt.Fprintf(&b, "%s\n", i18n.T("cli.sync.status_tracked_branches", r.TrackedBranches))
	return b.String()
}
