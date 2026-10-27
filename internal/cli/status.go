package cli

// « watchoo status » : carte d'en-tête (terminal, hors --json), compte gh et scopes
// (auth.Client.Status), quota (wsync.Engine.RateLimit), dépôts à jour / Flemme / Dead / en erreur /
// jamais synchronisés (activity), dernière synchro, branches suivies ; jamais le token.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// statusSubtitle est le sous-titre de la carte de « watchoo status ».
const statusSubtitle = "État de l'espace « %s »"

// statusGitHub décrit le compte gh dans la sortie de « watchoo status » (jamais le token).
type statusGitHub struct {
	OK          bool     `json:"ok"` // gh installé, connecté, token valide
	Host        string   `json:"host"`
	Login       string   `json:"login"`
	TokenSource string   `json:"token_source"`
	Scopes      []string `json:"scopes"`
	Error       string   `json:"error"` // vide si OK
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
		Short: "Résume l'état de l'espace : dépôts, synchros, quota et compte gh",
		Long: `Résume l'état de l'espace : compte gh utilisé et ses droits, quota restant de l'API
GitHub (lecture gratuite, qui ne consomme pas de quota) et garde-fou actif s'il reste
moins de 10 % du quota (synchros reportées jusqu'à la remise à zéro), nombre de dépôts par état
(Actif, Flemme, Dead, en erreur, jamais synchronisés…), dernière synchro et nombre de
branches suivies. Le token n'est jamais affiché.

Un problème de gh (non installé, non connecté) est signalé dans le résumé sans faire
échouer la commande.`,
		Args: noArgs,
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
		engine, err := s.engine()
		if err != nil {
			return err
		}
		if rl, err := engine.RateLimit(ctx); err != nil {
			report.RateLimitError = err.Error()
		} else {
			report.RateLimit = newRateLimitJSON(rl)
			report.QuotaLow = wsync.QuotaLow(rl, s.now())
		}
	}
	if !report.GitHub.OK {
		report.RateLimitError = "compte gh inutilisable"
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
		if card := renderCard(term, fmt.Sprintf(statusSubtitle, report.Workspace.Name), tipStatus); card != "" {
			b.WriteString(card + "\n")
		}
	}
	b.WriteString(s.statusText(outputStyle(deps, opts), report, lastSynced))
	return writeText(deps.Stdout, b.String())
}

// statusText renvoie le résumé texte de « watchoo status ».
func (s *session) statusText(st textStyle, r statusReport, lastSynced time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Espace : %s (%s)\n", textclean.Clean(r.Workspace.Name), textclean.Clean(displayPath(r.Workspace.Root)))
	if r.GitHub.OK {
		fmt.Fprintf(&b, "Compte gh : %s sur %s (%s)\n", textclean.Clean(r.GitHub.Login), textclean.Clean(r.GitHub.Host),
			textclean.Clean(scopesText(r.GitHub.Scopes)))
	} else {
		fmt.Fprintf(&b, "Compte gh : %s\n", textclean.Clean(r.GitHub.Error))
	}
	if r.RateLimit != nil {
		rl := provider.RateLimit{Limit: r.RateLimit.Limit, Remaining: r.RateLimit.Remaining}
		if r.RateLimit.Reset != nil {
			rl.Reset = *r.RateLimit.Reset
		}
		b.WriteString(s.rateLimitText(rl) + "\n")
		if r.QuotaLow {
			text := "Quota bas : synchros reportées"
			if !rl.Reset.IsZero() {
				text += " jusqu'à " + s.clockText(rl.Reset)
			}
			b.WriteString(text + " (moins de 10 % du quota restant).\n")
		}
	} else {
		fmt.Fprintf(&b, "Quota GitHub : inconnu (%s)\n", textclean.Clean(r.RateLimitError))
	}

	c := r.Repositories
	b.WriteString("\n")
	if c.Total == 0 {
		b.WriteString("Aucun dépôt suivi. Ajoutez-en avec « watchoo repo add owner/repo » ou " +
			"« watchoo repo import fichier.csv ».\n")
		return b.String()
	}
	fmt.Fprintf(&b, "Dépôts : %d\n", c.Total)
	var errRepos []string
	for _, e := range r.Errors {
		errRepos = append(errRepos, e.Repo)
	}
	lines := []struct {
		label string
		n     int
		extra string
	}{
		{"à jour (Actif)", c.Active, ""},
		{"Flemme", c.Flemme, ""},
		{"Dead", c.Dead, ""},
		{"en erreur", c.Error, strings.Join(errRepos, ", ")},
		{"jamais synchronisés", c.NeverSynced, ""},
		{"archivés", c.Archived, ""},
		{"vides", c.Empty, ""},
		{"suivi arrêté", c.Stopped, ""},
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
		b.WriteString("Dernière synchro : jamais (lancez « watchoo sync »)\n")
	} else {
		fmt.Fprintf(&b, "Dernière synchro : %s (%s)\n",
			activity.Relative(lastSynced, s.now(), s.loc, st.unicode()), lastSynced.In(s.loc).Format("02/01/2006 15:04"))
	}
	fmt.Fprintf(&b, "Branches suivies : %d\n", r.TrackedBranches)
	return b.String()
}
