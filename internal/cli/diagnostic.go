package cli

// « watchoo diagnostic [--repo owner/repo] [--author X] [--since 30d] » (phases 7.2 et 7.3) : pistes
// de réflexion calculées par internal/diagnostic (niveaux « Déclaré » et « À regarder »), réservées au
// formateur, avec leurs signaux, le contexte D4 des dépôts et l'historique des auteurs. Une piste n'est
// jamais un verdict. « watchoo diagnostic backfill [--repo X] » reprend l'historique déjà cloné.

import (
	"context"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// diagnosticReport est la sortie JSON de « watchoo diagnostic ».
type diagnosticReport struct {
	Enabled   bool                  `json:"enabled"`   // réglage diagnostic.enabled de l'espace
	Leads     []diagnosticLeadJSON  `json:"leads"`     // de la plus récente à la plus ancienne
	Contexts  []agentConfigCtxJSON  `json:"contexts"`  // contexte D4, du plus ancien ajout au plus récent
	Histories []authorHistoryJSON   `json:"histories"` // historique des auteurs (signaux de rythme)
	Notice    string                `json:"notice"`    // rappel : une piste n'est pas un verdict
	Filters   diagnosticFiltersJSON `json:"filters"`   // filtres appliqués
}

// diagnosticFiltersJSON rappelle les filtres de la commande (chaînes vides : pas de filtre).
type diagnosticFiltersJSON struct {
	Repository string     `json:"repository"`
	Author     string     `json:"author"`
	Since      *time.Time `json:"since"`
}

// diagnosticLeadJSON décrit une piste de réflexion.
type diagnosticLeadJSON struct {
	Kind       string              `json:"kind"`  // commit ou pull_request
	Level      string              `json:"level"` // declared, probable, to_review
	Repository repoRefJSON         `json:"repository"`
	SHA        string              `json:"sha"`       // vide pour une PR
	ShortSHA   string              `json:"short_sha"` // vide pour une PR
	Number     int                 `json:"number"`    // 0 pour un commit
	Title      string              `json:"title"`     // titre de la PR ; vide pour un commit
	URL        string              `json:"url"`
	Author     authorJSON          `json:"author"`
	Date       *time.Time          `json:"date"`
	Signals    []diagnosticHitJSON `json:"signals"`
}

// diagnosticHitJSON est un signal déclenché et sa valeur.
type diagnosticHitJSON struct {
	ID     string   `json:"id"`     // D1, D2, D3, D5, R1 à R6, F1 à F5, A1 à A7, H1 à H4
	Value  string   `json:"value"`  // outil, compte bot, branche, ou mesure écrite
	Raw    *float64 `json:"raw"`    // mesure d'un signal de rythme ; null pour un signal déclaratif
	Median *float64 `json:"median"` // médiane personnelle ; null si sans objet
	Z      *float64 `json:"z"`      // score z modifié ; null si sans objet
}

// authorHistoryJSON résume l'historique d'un auteur dans un dépôt.
type authorHistoryJSON struct {
	Repository repoRefJSON `json:"repository"`
	Author     authorJSON  `json:"author"`
	Commits    int         `json:"commits"`     // commits mesurés
	WorkedDays int         `json:"worked_days"` // jours avec au moins un commit mesuré
	Sufficient bool        `json:"sufficient"`  // au moins 20 commits mesurés dans les 90 jours
}

// agentConfigCtxJSON est le contexte D4 d'un dépôt.
type agentConfigCtxJSON struct {
	Repository       repoRefJSON `json:"repository"`
	AgentConfigSince *time.Time  `json:"agent_config_since"`
}

// diagnosticFlags regroupe les options de « watchoo diagnostic ».
type diagnosticFlags struct {
	repo, author, since string
}

// newDiagnosticCmd construit « watchoo diagnostic ».
func newDiagnosticCmd(deps Deps, opts *options) *cobra.Command {
	var f diagnosticFlags
	cmd := &cobra.Command{
		Use:   "diagnostic",
		Short: i18n.T("cli.diag.short"),
		Long:  i18n.T("cli.diag.long"),
		Example: `  watchoo diagnostic
  watchoo diagnostic --repo alice/projet-licence --since 30d`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDiagnostic(cmd.Context(), deps, opts, f)
		},
	}
	cmd.Flags().StringVar(&f.repo, "repo", "", i18n.T("cli.sync.flag_repo"))
	cmd.Flags().StringVar(&f.author, "author", "", i18n.T("cli.sync.flag_author"))
	cmd.Flags().StringVar(&f.since, "since", "", i18n.T("cli.sync.flag_since"))
	cmd.AddCommand(newDiagnosticBackfillCmd(deps, opts))
	return cmd
}

// runDiagnostic affiche les pistes de réflexion de niveau « Déclaré ».
func runDiagnostic(ctx context.Context, deps Deps, opts *options, f diagnosticFlags) (err error) {
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

	report := diagnosticReport{
		Enabled:   s.ws.Config.Diagnostic.Enabled,
		Leads:     []diagnosticLeadJSON{},
		Contexts:  []agentConfigCtxJSON{},
		Histories: []authorHistoryJSON{},
		Notice:    i18n.T("cli.diag.notice"),
		Filters:   diagnosticFiltersJSON{Author: strings.TrimSpace(f.author), Since: timeJSON(since)},
	}
	filter := store.DiagnosticFilter{Author: report.Filters.Author, Since: since}
	if f.repo != "" {
		repo, err := s.findRepository(ctx, f.repo)
		if err != nil {
			return err
		}
		filter.RepoID = repo.ID
		report.Filters.Repository = repo.FullName()
	}
	if !report.Enabled {
		if opts.json {
			return writeJSON(deps.Stdout, report)
		}
		return writeText(deps.Stdout, i18n.T("cli.diag.disabled"))
	}

	res, err := diagnostic.Build(ctx, s.store, filter, diagnostic.RhythmOptions{Location: s.loc,
		SpeedSignal: s.ws.Config.Diagnostic.SpeedSignal, RetentionCutoff: retentionCutoff(s.now(), s.ws.Config.Diagnostic.RetentionDays)})
	if err != nil {
		return err
	}
	for _, l := range res.Leads {
		report.Leads = append(report.Leads, newDiagnosticLeadJSON(l))
	}
	for _, c := range res.Contexts {
		report.Contexts = append(report.Contexts, agentConfigCtxJSON{
			Repository:       repoRefJSON{ID: c.RepoID, FullName: c.RepoFullName(), DisplayName: c.RepoDisplayName()},
			AgentConfigSince: timeJSON(c.Since),
		})
	}
	for _, h := range res.Histories {
		report.Histories = append(report.Histories, authorHistoryJSON{
			Repository: repoRefJSON{ID: h.RepoID, FullName: h.RepoFullName(), DisplayName: h.RepoDisplayName()},
			Author: authorJSON{Name: h.AuthorName, Login: h.AuthorLogin,
				DisplayName: cmpOr(cmpOr(h.AuthorName, h.AuthorLogin), i18n.T("cli.diag.unknown_author"))},
			Commits: h.Commits, WorkedDays: h.WorkedDays, Sufficient: h.Sufficient,
		})
	}
	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	return writeText(deps.Stdout, s.diagnosticText(outputStyle(deps, opts), res))
}

// newDiagnosticLeadJSON convertit une piste.
func newDiagnosticLeadJSON(l diagnostic.Lead) diagnosticLeadJSON {
	out := diagnosticLeadJSON{
		Kind:       l.Kind,
		Level:      l.Level,
		Repository: repoRefJSON{ID: l.RepoID, FullName: l.RepoFullName(), DisplayName: l.RepoDisplayName()},
		SHA:        l.SHA,
		Number:     l.Number,
		Title:      l.Title,
		URL:        l.URL,
		Author: authorJSON{Name: l.AuthorName, Login: l.AuthorLogin,
			DisplayName: cmpOr(cmpOr(l.AuthorName, l.AuthorLogin), i18n.T("cli.diag.unknown_author"))},
		Date:    timeJSON(l.Date),
		Signals: make([]diagnosticHitJSON, 0, len(l.Hits)),
	}
	if l.SHA != "" {
		out.ShortSHA = provider.ShortSHA(l.SHA)
	}
	for _, h := range l.Hits {
		hit := diagnosticHitJSON{ID: h.Signal, Value: h.Value}
		if strings.HasPrefix(h.Signal, "R") {
			hit.Raw = floatJSON(h.Raw, true)
			hit.Median = floatJSON(h.Median, h.HasMedian)
			hit.Z = floatJSON(h.Z, h.HasZ)
		}
		out.Signals = append(out.Signals, hit)
	}
	return out
}

// diagnosticText écrit le rappel, le tableau des pistes puis le contexte D4.
func (s *session) diagnosticText(st textStyle, res diagnostic.Report) string {
	var b strings.Builder
	b.WriteString(st.gray(i18n.T("cli.diag.notice")) + "\n\n")
	if len(res.Leads) == 0 {
		b.WriteString(i18n.T("cli.diag.none") + "\n")
	} else {
		rows := make([][]string, len(res.Leads))
		for i, l := range res.Leads {
			item := "PR #" + strconv.Itoa(l.Number)
			switch l.Kind {
			case diagnostic.KindCommit:
				item = "commit " + provider.ShortSHA(l.SHA)
			case diagnostic.KindDay:
				item = i18n.T("cli.diag.item_day")
			}
			rows[i] = []string{
				l.Date.In(s.loc).Format(i18n.T("cli.diag.fmt.date")),
				l.RepoDisplayName(),
				cmpOr(cmpOr(l.AuthorName, l.AuthorLogin), "-"),
				item,
				i18n.T("cli.diag.level." + l.Level),
				hitsText(l.Hits),
			}
		}
		headers := []string{"Date",
			st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
			i18n.T("cli.sync.header_author"),
			st.pick(i18n.T("cli.diag.header_item"), i18n.T("cli.diag.header_item_ascii")),
			i18n.T("cli.diag.header_level"),
			i18n.T("cli.diag.header_signals"),
		}
		b.WriteString(renderTable(st, headers, rows, nil))
	}
	short := 0
	for _, h := range res.Histories {
		if !h.Sufficient {
			short++
		}
	}
	if short > 0 {
		b.WriteString("\n" + st.gray(i18n.T("cli.diag.history_short", short, len(res.Histories), diagnostic.MinHistory)) + "\n")
	}
	if len(res.Contexts) > 0 {
		b.WriteString("\n" + i18n.T("cli.diag.context_title") + "\n")
		for _, c := range res.Contexts {
			b.WriteString("  " + i18n.T("cli.diag.context_line", c.RepoDisplayName(),
				c.Since.In(s.loc).Format(i18n.T("cli.diag.fmt.date"))) + "\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// hitsText décrit les signaux d'une piste : « D1 claude, D5 codex/fix, R2 388 (médiane 22, z 5.4) ».
func hitsText(hits []diagnostic.Hit) string {
	parts := make([]string, len(hits))
	for i, h := range hits {
		parts[i] = h.Signal + " " + h.Value
		switch {
		case h.HasMedian && h.HasZ:
			parts[i] += " " + i18n.T("cli.diag.hit_median_z", numberText(h.Median), numberText(h.Z))
		case h.HasMedian:
			parts[i] += " " + i18n.T("cli.diag.hit_median", numberText(h.Median))
		}
	}
	return strings.Join(parts, ", ")
}

// numberText écrit une mesure : entière sans décimale, sinon avec une décimale.
func numberText(x float64) string {
	if x == math.Trunc(x) {
		return strconv.FormatFloat(x, 'f', 0, 64)
	}
	return strconv.FormatFloat(x, 'f', 1, 64)
}

// floatJSON renvoie x arrondi au millième si ok, sinon nil (null).
func floatJSON(x float64, ok bool) *float64 {
	if !ok {
		return nil
	}
	r := math.Round(x*1000) / 1000
	return &r
}

// retentionCutoff renvoie la date d'avant laquelle une piste est hors de la durée de conservation ;
// zéro si days <= 0.
func retentionCutoff(now time.Time, days int) time.Time {
	if days <= 0 {
		return time.Time{}
	}
	return now.UTC().Add(-time.Duration(days) * 24 * time.Hour)
}
