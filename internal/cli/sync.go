package cli

// « watchoo sync [owner/repo] » : synchro manuelle (wsync.Engine.SyncAll / SyncRepo), rapport par
// dépôt en tableau ou en JSON ; code de sortie non nul si un dépôt est en erreur.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// syncRepoJSON est le résultat d'un dépôt dans la sortie JSON de « watchoo sync ».
type syncRepoJSON struct {
	ID              int64            `json:"id"`
	FullName        string           `json:"full_name"`
	DisplayName     string           `json:"display_name"`
	Status          wsync.RepoStatus `json:"status"` // success, error, locked ou deferred
	NewCommits      int              `json:"new_commits"`
	TrackedBranches int              `json:"tracked_branches"`
	StatsFetched    int              `json:"stats_fetched"` // statistiques de commits demandées
	DurationMS      int64            `json:"duration_ms"`
	Error           string           `json:"error"`     // vide pour un succès
	ResumeAt        *time.Time       `json:"resume_at"` // deferred : quand réessayer ; null sinon
	// Warnings : pull requests, releases ou tags non synchronisés d'un dépôt réussi ; [] sinon.
	Warnings []string `json:"warnings"`
}

// syncSummary compte les résultats d'une synchro.
type syncSummary struct {
	Repositories int `json:"repositories"`
	Success      int `json:"success"`
	Error        int `json:"error"`
	Locked       int `json:"locked"`
	Deferred     int `json:"deferred"`
	NewCommits   int `json:"new_commits"`
}

// syncReport est la sortie JSON de « watchoo sync ».
type syncReport struct {
	StartedAt    *time.Time     `json:"started_at"`
	FinishedAt   *time.Time     `json:"finished_at"`
	Summary      syncSummary    `json:"summary"`
	Repositories []syncRepoJSON `json:"repositories"`
	RateLimit    *rateLimitJSON `json:"rate_limit"` // dernier quota observé ; null si inconnu
	// Source : « api » (API GitHub, gh connecté) ou « git » (clones Git locaux, sans gh) ; vide si
	// aucun dépôt n'a été synchronisé.
	Source wsync.SourceKind `json:"source"`
}

// newSyncCmd construit « watchoo sync [owner/repo] ».
func newSyncCmd(deps Deps, opts *options) *cobra.Command {
	return &cobra.Command{
		Use:   "sync [owner/repo]",
		Short: "Synchronise tous les dépôts suivis, ou un seul",
		Long: `Synchronise tous les dépôts suivis (au plus sync.parallelism à la fois), ou le seul
dépôt donné, même si son suivi est arrêté. La synchro est incrémentale : seuls les
nouveaux commits sont téléchargés.

Un dépôt déjà en cours de synchronisation (par « watchoo tui » ou un autre terminal)
est ignoré. Quand le quota de l'API GitHub passe sous 10 %, les dépôts restants sont
reportés, avec l'heure à laquelle réessayer.

Sans gh installé et connecté, les dépôts sont lus depuis des clones Git locaux
(.watchoo/repos/, mis à jour par « git fetch ») avec la clé SSH ou les identifiants
Git de cette machine : pas de quota, mais ni pull requests ni releases.

Le code de retour est non nul si un dépôt est en erreur.`,
		Example: `  watchoo sync
  watchoo sync alice/projet-licence --json`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := ""
			if len(args) == 1 {
				ref = args[0]
			}
			return runSync(cmd.Context(), deps, opts, ref)
		},
	}
}

// runSync lance la synchro et affiche son rapport.
func runSync(ctx context.Context, deps Deps, opts *options, ref string) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)
	engine, err := s.engine()
	if err != nil {
		return err
	}
	var report wsync.Report
	if ref != "" {
		repo, err := s.findRepository(ctx, ref)
		if err != nil {
			return err
		}
		report, err = engine.SyncRepo(ctx, repo.ID, wsync.SyncOptions{})
		if err != nil {
			return err
		}
	} else {
		report, err = engine.SyncAll(ctx, wsync.SyncOptions{})
		if err != nil {
			return err
		}
	}

	summary := syncSummary{
		Repositories: len(report.Repos),
		Success:      report.Count(wsync.StatusSuccess),
		Error:        report.Count(wsync.StatusError),
		Locked:       report.Count(wsync.StatusLocked),
		Deferred:     report.Count(wsync.StatusDeferred),
		NewCommits:   report.NewCommits(),
	}
	if opts.json {
		out := syncReport{
			StartedAt:    timeJSON(report.StartedAt),
			FinishedAt:   timeJSON(report.FinishedAt),
			Summary:      summary,
			Repositories: make([]syncRepoJSON, 0, len(report.Repos)),
			Source:       report.Source.Kind,
		}
		for _, r := range report.Repos {
			out.Repositories = append(out.Repositories, syncRepoJSON{
				ID:              r.RepoID,
				FullName:        r.FullName,
				DisplayName:     r.DisplayName,
				Status:          r.Status,
				NewCommits:      r.NewCommits,
				TrackedBranches: r.TrackedBranches,
				StatsFetched:    r.StatsFetched,
				DurationMS:      r.Duration.Milliseconds(),
				Error:           r.Error,
				ResumeAt:        timeJSON(r.ResumeAt),
				Warnings:        append([]string{}, r.Warnings...),
			})
		}
		if report.RateLimitKnown {
			out.RateLimit = newRateLimitJSON(report.RateLimit)
		}
		err = writeJSON(deps.Stdout, out)
	} else {
		err = writeText(deps.Stdout, s.syncText(outputStyle(deps, opts), report, summary))
	}
	if err != nil {
		return err
	}
	if summary.Error > 0 {
		return fmt.Errorf("synchro terminée avec %d dépôt%s en erreur (détail ci-dessus, ou « watchoo status »)",
			summary.Error, plural(summary.Error))
	}
	return nil
}

// syncText renvoie le rapport texte d'une synchro : un tableau par dépôt, puis le bilan et le quota.
func (s *session) syncText(st textStyle, report wsync.Report, sum syncSummary) string {
	if len(report.Repos) == 0 {
		return "Aucun dépôt à synchroniser. Ajoutez-en avec « watchoo repo add owner/repo ».\n"
	}
	rows := make([][]string, len(report.Repos))
	for i, r := range report.Repos {
		rows[i] = []string{
			r.DisplayName,
			syncStatusText(st, r.Status),
			fmt.Sprint(r.NewCommits),
			fmt.Sprint(r.TrackedBranches),
			durationText(r.Duration),
			st.truncate(s.syncDetail(r), 70),
		}
		if r.Status != wsync.StatusSuccess {
			rows[i][2], rows[i][3], rows[i][4] = "-", "-", "-"
			if r.Status == wsync.StatusError && r.Duration > 0 {
				rows[i][4] = durationText(r.Duration)
			}
		}
	}
	headers := []string{st.pick("Dépôt", "Depot"), "Statut", "Nouveaux commits", "Branches", st.pick("Durée", "Duree"),
		st.pick("Détail", "Detail")}
	var b strings.Builder
	b.WriteString(renderTable(st, headers, rows, func(row int) bool {
		return report.Repos[row].Status != wsync.StatusSuccess && report.Repos[row].Status != wsync.StatusError
	}))
	parts := []string{
		countText(sum.Repositories, "dépôt", "dépôts"),
		countText(sum.NewCommits, "nouveau commit", "nouveaux commits"),
	}
	if sum.Error > 0 {
		parts = append(parts, fmt.Sprintf("%d en erreur", sum.Error))
	}
	if sum.Locked > 0 {
		parts = append(parts, fmt.Sprintf("%d déjà en cours", sum.Locked))
	}
	if sum.Deferred > 0 {
		parts = append(parts, fmt.Sprintf("%d reporté%s", sum.Deferred, plural(sum.Deferred)))
	}
	if n := report.Warnings(); n > 0 {
		parts = append(parts, countText(n, "avertissement", "avertissements"))
	}
	fmt.Fprintf(&b, "\nSynchro terminée en %s : %s.\n",
		durationText(report.FinishedAt.Sub(report.StartedAt)), strings.Join(parts, ", "))
	if report.RateLimitKnown {
		b.WriteString(s.rateLimitText(report.RateLimit) + "\n")
	}
	if report.Source.Kind == wsync.SourceGit {
		b.WriteString("Source : clones Git locaux (gh absent ou non connecté ; « gh auth login » pour passer par l'API GitHub).\n")
	}
	return b.String()
}

// syncStatusText renvoie le libellé français du résultat d'un dépôt.
func syncStatusText(st textStyle, status wsync.RepoStatus) string {
	switch status {
	case wsync.StatusSuccess:
		return "ok"
	case wsync.StatusError:
		return "erreur"
	case wsync.StatusLocked:
		return "ignoré"
	case wsync.StatusDeferred:
		return st.pick("reporté", "reporte")
	default:
		return string(status)
	}
}

// syncDetail renvoie le détail d'un dépôt : message d'erreur, verrou, ou report avec l'heure de reprise
// en heure locale (le moteur la donne en UTC).
func (s *session) syncDetail(r wsync.RepoReport) string {
	if r.Status == wsync.StatusDeferred && !r.ResumeAt.IsZero() {
		reason, _, _ := strings.Cut(strings.TrimPrefix(r.Error, "reporté : "), " jusqu'à")
		if reason == "" {
			reason = "quota bas"
		}
		return fmt.Sprintf("reporté : %s jusqu'à %s", reason, s.clockText(r.ResumeAt))
	}
	if r.Status == wsync.StatusSuccess && len(r.Warnings) > 0 {
		return strings.Join(r.Warnings, " ; ")
	}
	return r.Error
}

// clockText renvoie l'heure de t dans le fuseau d'affichage : « 15:04 », précédée de la date si ce
// n'est pas aujourd'hui.
func (s *session) clockText(t time.Time) string {
	local, now := t.In(s.loc), s.now().In(s.loc)
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format("15:04")
	}
	return local.Format("02/01/2006 15:04")
}

// durationText renvoie une durée courte : « 0,4 s », « 12 s », « 3 min 05 s ».
func durationText(d time.Duration) string {
	switch {
	case d < 0:
		return "0 s"
	case d < 10*time.Second:
		tenths := int64(d / (100 * time.Millisecond))
		return fmt.Sprintf("%d,%d s", tenths/10, tenths%10)
	case d < time.Minute:
		return fmt.Sprintf("%d s", int64(d/time.Second))
	default:
		return fmt.Sprintf("%d min %02d s", int64(d/time.Minute), int64(d%time.Minute/time.Second))
	}
}

// rateLimitText décrit le quota de l'API GitHub, avec l'heure de remise à zéro en heure locale :
// « Quota GitHub : 4987 / 5000 requêtes restantes (remise à zéro à 15:04). »
func (s *session) rateLimitText(rl provider.RateLimit) string {
	text := fmt.Sprintf("Quota GitHub : %d / %d requêtes restantes", rl.Remaining, rl.Limit)
	if !rl.Reset.IsZero() {
		text += fmt.Sprintf(" (remise à zéro à %s)", s.clockText(rl.Reset))
	}
	return text + "."
}
