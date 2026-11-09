package cli

// « watchoo conventions [owner/repo] [--commits] [--failing] [--contributor X] [--milestone vX.Y.Z] »
// (phases 8.1 à 8.5) : taux de commits conformes aux conventions (CC1 à CC4), puis nommage des branches
// (BR1), titres de PR (PR1, PR2), tags et jalons (SV1, SV2), releases en information (SV6), formulation,
// pieds et langue des commits (CC5 à CC7), CC1 par intervalle entre tags, tendance de CC3 et revue de
// jalon, calculés par internal/conventions à la lecture ; critères de fichiers (CL, RD, DOC, TL, phase 8.3)
// lus dans le cache de la synchro, avec les listes de chemins en vue détaillée d'un dépôt. Aucune note globale : des taux n/m expliqués
// (dénominateur et exclusions), réservés au formateur. Ni message de commit ni titre de PR n'est affiché :
// SHA court, numéro de PR, auteur, noms de branche et de tag, et codes d'écart seulement.

import (
	"context"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// conventionsReport est la sortie JSON de « watchoo conventions » : mêmes clés de premier niveau quelles
// que soient les options (commits vaut [] sans --commits ni --failing).
type conventionsReport struct {
	Enabled      bool                    `json:"enabled"`       // réglage conventions.enabled de l'espace
	RulesVersion int                     `json:"rules_version"` // version des règles du parseur
	Repositories []conventionsRepoJSON   `json:"repositories"`  // dans l'ordre de repo list
	Contributors []conventionsAuthorJSON `json:"contributors"`  // CC3, par nom (aucun classement)
	Commits      []conventionsCommitJSON `json:"commits"`       // --commits / --failing ; du plus récent au plus ancien
	Notice       string                  `json:"notice"`        // rappel : des taux, pas une note
	Filters      conventionsFiltersJSON  `json:"filters"`       // filtres appliqués
	// Milestone est la revue du jalon de --milestone (phase 8.2) ; null sans --milestone.
	Milestone *conventionsMilestoneJSON `json:"milestone"`
}

// conventionsFiltersJSON rappelle les filtres et options de la commande.
type conventionsFiltersJSON struct {
	Repository  string `json:"repository"`  // vide : tous les dépôts
	Contributor string `json:"contributor"` // vide : tous les auteurs
	Commits     bool   `json:"commits"`     // liste des commits évalués demandée
	Failing     bool   `json:"failing"`     // seulement les commits non conformes
	Milestone   string `json:"milestone"`   // --milestone ; vide sans revue de jalon
}

// conventionsRepoJSON est le rapport d'un dépôt.
type conventionsRepoJSON struct {
	Repository     repoRefJSON              `json:"repository"`
	Source         string                   `json:"source"`       // clone ou database
	Since          *string                  `json:"since"`        // date de début effective, AAAA-MM-JJ ; null : depuis le début
	SinceOrigin    *string                  `json:"since_origin"` // workspace ou repository ; null sans date
	Commits        conventionsTallyJSON     `json:"commits"`      // CC1, CC2 et CC4, toutes branches suivies
	BeforeSince    int                      `json:"before_since"`
	ParentsUnknown int                      `json:"parents_unknown"`
	DefaultBranch  conventionsDefaultBranch `json:"default_branch"` // vue informative
	// Phase 8.2.
	Traits       conventionsTraitsJSON    `json:"traits"`        // CC5, CC6 (informatifs)
	Language     conventionsLanguageJSON  `json:"language"`      // CC7 (information)
	Branches     conventionsBranchesJSON  `json:"branches"`      // BR1
	PullRequests conventionsPullsJSON     `json:"pull_requests"` // PR1, PR2 (numéros et codes, jamais le titre)
	Tags         conventionsTagsJSON      `json:"tags"`          // SV1, SV2, SV6
	Intervals    conventionsIntervalsJSON `json:"intervals"`     // CC1 par intervalle entre tags finals
	// Phase 8.3 : critères de fichiers sur la tête de la branche par défaut (cache de la synchro).
	Changelog conventionsFilesJSON `json:"changelog"` // CL1 à CL8
	Readme    conventionsFilesJSON `json:"readme"`    // RD1 à RD4
	Docs      conventionsDocsJSON  `json:"docs"`      // DOC1 à DOC5, DOC8 ; listes en vue détaillée
	Tooling   conventionsFilesJSON `json:"tooling"`   // TL1 à TL3
	// Phase 8.4 : historique des tags et de la branche par défaut, versions, fraîcheur du CHANGELOG.
	Rewrites           conventionsRewritesJSON    `json:"rewrites"`            // BR4
	TagHistory         conventionsTagHistoryJSON  `json:"tag_history"`         // SV3
	Prereleases        conventionsPrereleasesJSON `json:"prereleases"`         // SV4 (avertissements)
	Bumps              conventionsBumpsJSON       `json:"bumps"`               // SV5
	Precedence         conventionsPrecedenceJSON  `json:"precedence"`          // SV7
	ChangelogFreshness conventionsFreshnessJSON   `json:"changelog_freshness"` // CL10 (information)
	// Phase 8.5 (PR5 et PR6 dans pull_requests, BR2 dans branches.reused).
	Arrivals     conventionsArrivalsJSON     `json:"arrivals"`      // PR3 : commits arrivés hors PR
	MergeMethods conventionsMergeMethodsJSON `json:"merge_methods"` // BR5
}

// conventionsTallyJSON est un taux n/m, ses exclusions et ses écarts.
type conventionsTallyJSON struct {
	Evaluated int                    `json:"evaluated"` // dénominateur
	Conform   int                    `json:"conform"`   // numérateur
	Rate      *float64               `json:"rate"`      // conform / evaluated, au millième ; null sans commit évalué
	Excluded  map[string]int         `json:"excluded"`  // CC4 : merge, bot, initial_commit, excluded_account
	Issues    []conventionsIssueJSON `json:"issues"`    // CC2, dans l'ordre des codes
}

// conventionsIssueJSON est un code d'écart et les commits qui l'ont reçu.
type conventionsIssueJSON struct {
	Code     string   `json:"code"`
	Severity string   `json:"severity"` // blocking ou warning
	Count    int      `json:"count"`
	SHAs     []string `json:"shas"`
}

// conventionsDefaultBranch est la vue de la seule branche par défaut.
type conventionsDefaultBranch struct {
	Name      string   `json:"name"`
	Evaluated int      `json:"evaluated"`
	Conform   int      `json:"conform"`
	Rate      *float64 `json:"rate"`
}

// conventionsAuthorJSON est le taux CC3 d'un contributeur.
type conventionsAuthorJSON struct {
	Author  authorJSON           `json:"author"`
	Commits conventionsTallyJSON `json:"commits"`
	// Phase 8.2.
	Traits       conventionsTraitsJSON         `json:"traits"`        // CC5, CC6 de ses commits
	Trend        conventionsTrendJSON          `json:"trend"`         // CC3 avant et depuis le dernier tag final
	PullRequests conventionsAuthorPullsJSON    `json:"pull_requests"` // PR1, PR2 de ses PR
	Branches     conventionsAuthorBranchesJSON `json:"branches"`      // BR1 des branches de tête de ses PR
	// Phase 8.5.
	Arrivals conventionsAuthorArrivalsJSON `json:"arrivals"` // PR3 : commits du premier parent dont il est l'auteur
}

// conventionsCommitJSON est un commit évalué, sans son message.
type conventionsCommitJSON struct {
	Repository repoRefJSON `json:"repository"`
	SHA        string      `json:"sha"`
	ShortSHA   string      `json:"short_sha"`
	Author     authorJSON  `json:"author"`
	Date       *time.Time  `json:"date"`
	Conform    bool        `json:"conform"`
	Codes      []string    `json:"codes"`
}

// conventionsFlags regroupe les options de « watchoo conventions ».
type conventionsFlags struct {
	commits, failing bool
	contributor      string
	milestone        string
}

// newConventionsCmd construit « watchoo conventions ».
func newConventionsCmd(deps Deps, opts *options) *cobra.Command {
	var f conventionsFlags
	cmd := &cobra.Command{
		Use:   "conventions [owner/repo]",
		Short: i18n.T("cli.conv.short"),
		Long:  i18n.T("cli.conv.long"),
		Example: `  watchoo conventions
  watchoo conventions alice/projet-licence --commits --failing
  watchoo conventions --contributor alice --json
  watchoo conventions --milestone v1.0.0`,
		Args: maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := ""
			if len(args) == 1 {
				ref = args[0]
			}
			return runConventions(cmd.Context(), deps, opts, ref, f)
		},
	}
	cmd.Flags().BoolVar(&f.commits, "commits", false, i18n.T("cli.conv.flag_commits"))
	cmd.Flags().BoolVar(&f.failing, "failing", false, i18n.T("cli.conv.flag_failing"))
	cmd.Flags().StringVar(&f.contributor, "contributor", "", i18n.T("cli.conv.flag_contributor"))
	cmd.Flags().StringVar(&f.milestone, "milestone", "", i18n.T("cli.conv.flag_milestone"))
	return cmd
}

// runConventions affiche le rapport des conventions.
func runConventions(ctx context.Context, deps Deps, opts *options, ref string, f conventionsFlags) (err error) {
	s, err := openSession(ctx, deps, opts)
	if err != nil {
		return err
	}
	defer closeSession(s, &err)

	report := conventionsReport{
		Enabled:      s.ws.Config.Conventions.Enabled,
		RulesVersion: conventions.RulesVersion,
		Repositories: []conventionsRepoJSON{},
		Contributors: []conventionsAuthorJSON{},
		Commits:      []conventionsCommitJSON{},
		Notice:       i18n.T("cli.conv.notice"),
		Filters: conventionsFiltersJSON{Contributor: strings.TrimSpace(f.contributor),
			Commits: f.commits || f.failing, Failing: f.failing, Milestone: strings.TrimSpace(f.milestone)},
	}
	bopts := s.ws.Config.Conventions.BuildOptions(s.loc)
	if report.Filters.Milestone != "" {
		if _, ok := conventions.FindMilestone(bopts.Milestones, report.Filters.Milestone); !ok {
			return errors.New(i18n.T("cli.conv.bad_milestone", report.Filters.Milestone, milestoneNames(bopts.Milestones)))
		}
	}
	filter := conventions.Filter{}
	if ref != "" {
		repo, err := s.findRepository(ctx, ref)
		if err != nil {
			return err
		}
		filter.RepoID = repo.ID
		report.Filters.Repository = repo.FullName()
		// Vue détaillée d'un dépôt : listes de chemins des critères de fichiers, recalculées depuis le clone.
		bopts.FileDetail = true
	}
	if !report.Enabled {
		if opts.json {
			return writeJSON(deps.Stdout, report)
		}
		return writeText(deps.Stdout, i18n.T("cli.conv.disabled"))
	}
	if report.Filters.Contributor != "" {
		if filter.AuthorIDs, err = s.store.MatchingContributorIDs(ctx, report.Filters.Contributor); err != nil {
			return err
		}
	}
	eng, err := s.engine()
	if err != nil {
		return err
	}
	clones, err := eng.ConventionsClones(ctx)
	if err != nil {
		return err
	}
	// PR et releases : « inconnues » en clone seul (règle du cœur, commune avec le web).
	if bopts.LocalOnly, err = wsync.ConventionsLocalOnly(ctx, eng.Source); err != nil {
		return err
	}
	res, err := conventions.Build(ctx, s.store, clones, filter, bopts)
	if err != nil {
		return err
	}
	var review *conventions.MilestoneReview
	if report.Filters.Milestone != "" {
		rv, err := conventions.ReviewMilestone(ctx, s.store, clones, conventions.Filter{RepoID: filter.RepoID},
			report.Filters.Milestone, bopts, time.Time{})
		if err != nil {
			return err
		}
		review = &rv
		report.Milestone = newConventionsMilestoneJSON(rv)
	}

	for _, r := range res.Repos {
		ref := repoRefJSON{ID: r.Repo.ID, FullName: r.Repo.FullName(), DisplayName: r.Repo.DisplayName()}
		def := conventionsDefaultBranch{Name: r.Repo.DefaultBranch, Evaluated: r.DefaultBranch.Evaluated,
			Conform: r.DefaultBranch.Conform}
		def.Rate = rateJSON(r.DefaultBranch)
		report.Repositories = append(report.Repositories, conventionsRepoJSON{
			Repository: ref, Source: r.Source, Since: optionalString(r.SinceDate),
			SinceOrigin: optionalString(r.SinceOrigin), Commits: newConventionsTallyJSON(r.Tally),
			BeforeSince: r.BeforeSince, ParentsUnknown: r.ParentsUnknown, DefaultBranch: def,
			Traits: newConventionsTraitsJSON(r.Traits), Language: newConventionsLanguageJSON(r.Language),
			Branches: newConventionsBranchesJSON(r.Branches), PullRequests: newConventionsPullsJSON(r.Pulls),
			Tags: newConventionsTagsJSON(r.Tags), Intervals: newConventionsIntervalsJSON(r.Intervals),
			Changelog: newConventionsFilesJSON(r.Files, r.Files.Results.Changelog),
			Readme:    newConventionsFilesJSON(r.Files, r.Files.Results.Readme),
			Docs:      newConventionsDocsJSON(r.Files),
			Tooling:   newConventionsFilesJSON(r.Files, r.Files.Results.Tooling),
			Rewrites:  newConventionsRewritesJSON(r.Rewrites), TagHistory: newConventionsTagHistoryJSON(r.TagHistory),
			Prereleases: newConventionsPrereleasesJSON(r.Prereleases), Bumps: newConventionsBumpsJSON(r.Bumps),
			Precedence:         newConventionsPrecedenceJSON(r.Precedence),
			ChangelogFreshness: newConventionsFreshnessJSON(r.Freshness),
			Arrivals:           newConventionsArrivalsJSON(r.Arrivals),
			MergeMethods:       newConventionsMergeMethodsJSON(r.MergeMethods),
		})
		if !report.Filters.Commits {
			continue
		}
		for _, c := range r.Commits {
			if !c.Evaluated() || (f.failing && c.Conform) {
				continue
			}
			report.Commits = append(report.Commits, conventionsCommitJSON{
				Repository: ref, SHA: c.SHA, ShortSHA: provider.ShortSHA(c.SHA),
				Author: conventionsAuthor(c.AuthorName, c.AuthorLogin), Date: timeJSON(c.AuthoredAt),
				Conform: c.Conform, Codes: c.Codes,
			})
		}
	}
	for _, c := range res.Contributors {
		report.Contributors = append(report.Contributors, conventionsAuthorJSON{
			Author: conventionsAuthor(c.AuthorName, c.AuthorLogin), Commits: newConventionsTallyJSON(c.Tally),
			Traits: newConventionsTraitsJSON(c.Traits), Trend: newConventionsTrendJSON(c.Trend),
			PullRequests: conventionsAuthorPullsJSON{Open: newPRCountJSON(c.Pulls.Open), Merged: newPRCountJSON(c.Pulls.Merged)},
			Branches:     conventionsAuthorBranchesJSON{Checked: c.Branches.Checked, Conform: c.Branches.Conform, Review: c.Branches.Review},
			Arrivals: conventionsAuthorArrivalsJSON{ByPR: c.Arrivals.ByPR, OutsidePR: c.Arrivals.OutsidePR,
				Undetermined: c.Arrivals.Undetermined},
		})
	}
	if opts.json {
		return writeJSON(deps.Stdout, report)
	}
	if review != nil {
		return writeText(deps.Stdout, milestoneText(outputStyle(deps, opts), *review, s.loc))
	}
	return writeText(deps.Stdout, s.conventionsText(outputStyle(deps, opts), res, report))
}

// conventionsAuthor décrit un auteur (« inconnu » sans nom ni login).
func conventionsAuthor(name, login string) authorJSON {
	return authorJSON{Name: name, Login: login, DisplayName: cmpOr(cmpOr(name, login), i18n.T("cli.conv.unknown_author"))}
}

// rateJSON renvoie le taux de t au millième, nil sans commit évalué.
func rateJSON(t conventions.Tally) *float64 {
	r, ok := t.Rate()
	return floatJSON(r, ok)
}

// newConventionsTallyJSON convertit un taux et ses écarts.
func newConventionsTallyJSON(t conventions.Tally) conventionsTallyJSON {
	out := conventionsTallyJSON{Evaluated: t.Evaluated, Conform: t.Conform, Rate: rateJSON(t),
		Excluded: map[string]int{}, Issues: make([]conventionsIssueJSON, 0, len(t.Issues))}
	for _, e := range conventions.AllExclusions {
		out.Excluded[e] = t.Excluded[e]
	}
	for _, is := range t.Issues {
		out.Issues = append(out.Issues, conventionsIssueJSON{Code: is.Code, Severity: is.Severity.String(),
			Count: is.Count, SHAs: is.SHAs})
	}
	return out
}

// conventionsText écrit le rappel, le tableau des dépôts, le détail de leurs exclusions et écarts, les
// contributeurs puis, avec --commits, les commits évalués.
func (s *session) conventionsText(st textStyle, res conventions.BuildReport, report conventionsReport) string {
	var b strings.Builder
	b.WriteString(st.gray(i18n.T("cli.conv.notice")) + "\n\n")
	if len(res.Repos) == 0 {
		b.WriteString(i18n.T("cli.conv.no_repos") + "\n")
		return strings.TrimRight(b.String(), "\n")
	}
	dash := st.pick("—", "-")
	rows := make([][]string, len(res.Repos))
	fallback, unknownParents := false, 0
	for i, r := range res.Repos {
		source := i18n.T("cli.conv.source." + r.Source)
		if r.Source == conventions.SourceDatabase {
			source += "*"
			fallback = true
		}
		since := dash
		if r.SinceDate != "" {
			since = r.SinceDate
		}
		def := dash
		if r.Repo.DefaultBranch != "" {
			def = ratioText(r.DefaultBranch) + " " + rateText(r.DefaultBranch, dash)
		}
		rows[i] = []string{r.Repo.DisplayName(), ratioText(r.Tally), rateText(r.Tally, dash), def,
			strconv.Itoa(r.ExcludedTotal()), since, source}
		unknownParents += r.ParentsUnknown
	}
	headers := []string{
		st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
		i18n.T("cli.conv.header_conform"),
		i18n.T("cli.conv.header_rate"),
		st.pick(i18n.T("cli.conv.header_default"), i18n.T("cli.conv.header_default_ascii")),
		i18n.T("cli.conv.header_excluded"),
		i18n.T("cli.conv.header_since"),
		i18n.T("cli.conv.header_source"),
	}
	b.WriteString(renderTable(st, headers, rows, nil))
	if fallback {
		b.WriteString(st.gray(i18n.T("cli.conv.fallback")) + "\n")
	}
	if unknownParents > 0 {
		b.WriteString(st.gray(i18n.T("cli.conv.parents_unknown", unknownParents)) + "\n")
	}

	for _, r := range res.Repos {
		lines := append(conventionsDetail(r), conventionsMoreDetail(r, dash)...)
		lines = append(lines, s.fileLines(r.Files, report.Filters.Repository != "")...)
		if report.Filters.Contributor == "" {
			lines = append(lines, historyLines(r, s.loc)...)
		}
		if len(lines) == 0 {
			continue
		}
		b.WriteString("\n" + r.Repo.DisplayName() + "\n")
		for _, l := range lines {
			b.WriteString("  " + l + "\n")
		}
	}

	if len(res.Contributors) > 0 {
		b.WriteString("\n" + i18n.T("cli.conv.contributors_title") + "\n")
		rows := make([][]string, len(res.Contributors))
		for i, c := range res.Contributors {
			rows[i] = []string{cmpOr(cmpOr(c.AuthorName, c.AuthorLogin), i18n.T("cli.conv.unknown_author")),
				ratioText(c.Tally), rateText(c.Tally, dash), trendText(c.Trend.Before, c.Trend, dash),
				trendText(c.Trend.Since, c.Trend, dash), authorPullsText(c.Pulls, dash), authorBodiesText(c.Pulls, dash),
				authorBranchesText(c.Branches, dash), authorArrivalsText(c.Arrivals, dash), issuesText(c.Issues)}
		}
		b.WriteString(renderTable(st, []string{i18n.T("cli.sync.header_author"), i18n.T("cli.conv.header_conform"),
			i18n.T("cli.conv.header_rate"), i18n.T("cli.conv.header_before_tag"), i18n.T("cli.conv.header_since_tag"),
			i18n.T("cli.conv.header_pr_titles"), i18n.T("cli.conv.header_pr_bodies"), i18n.T("cli.conv.header_branches"),
			i18n.T("cli.conv.header_outside_pr"),
			st.pick(i18n.T("cli.conv.header_issues"), i18n.T("cli.conv.header_issues_ascii"))},
			rows, nil))
		b.WriteString(st.gray(i18n.T("cli.conv.contributors_note")) + "\n")
	}

	if report.Filters.Commits {
		b.WriteString("\n" + i18n.T("cli.conv.commits_title") + "\n")
		if len(report.Commits) == 0 {
			b.WriteString(i18n.T("cli.conv.no_commits") + "\n")
		} else {
			rows := make([][]string, len(report.Commits))
			for i, c := range report.Commits {
				result := i18n.T("cli.conv.conform")
				if !c.Conform {
					result = i18n.T("cli.conv.failing")
				}
				if len(c.Codes) > 0 {
					result += " (" + strings.Join(c.Codes, ", ") + ")"
				}
				date := dash
				if c.Date != nil {
					date = c.Date.In(s.loc).Format(i18n.T("cli.conv.fmt.date"))
				}
				rows[i] = []string{date, c.Repository.DisplayName, c.ShortSHA, c.Author.DisplayName, result}
			}
			b.WriteString(renderTable(st, []string{"Date",
				st.pick(i18n.T("cli.sync.header_repo"), i18n.T("cli.sync.header_repo_ascii")),
				"SHA", i18n.T("cli.sync.header_author"),
				st.pick(i18n.T("cli.conv.header_result"), i18n.T("cli.conv.header_result_ascii"))}, rows, nil))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// conventionsDetail renvoie les lignes de détail d'un dépôt : date de début, exclusions, commits
// antérieurs et écarts avec leur libellé.
func conventionsDetail(r conventions.RepoReport) []string {
	var lines []string
	if r.SinceDate != "" {
		lines = append(lines, i18n.T("cli.conv.since_"+r.SinceOrigin, r.SinceDate))
	}
	if r.ExcludedTotal() > 0 {
		var parts []string
		for _, e := range conventions.AllExclusions {
			if n := r.Excluded[e]; n > 0 {
				parts = append(parts, i18n.T("cli.conv.exclusion."+e)+" "+strconv.Itoa(n))
			}
		}
		lines = append(lines, i18n.T("cli.conv.excluded_line", strings.Join(parts, ", ")))
	}
	if r.BeforeSince > 0 {
		lines = append(lines, i18n.T("cli.conv.before_since", r.BeforeSince))
	}
	for _, is := range r.Issues {
		lines = append(lines, i18n.T("cli.conv.issue_line", is.Code, is.Count,
			i18n.T("cli.conv.code."+is.Code), i18n.T("cli.conv.severity."+is.Severity.String())))
	}
	return lines
}

// ratioText écrit « conformes/évalués ».
func ratioText(t conventions.Tally) string {
	return strconv.Itoa(t.Conform) + "/" + strconv.Itoa(t.Evaluated)
}

// rateText écrit le taux en pour cent entiers ; dash sans commit évalué (inconnu, pas 0 %).
func rateText(t conventions.Tally, dash string) string {
	r, ok := t.Rate()
	if !ok {
		return dash
	}
	return i18n.T("cli.conv.percent", int(math.Round(r*100)))
}

// issuesText résume les écarts : « no_type 3, length_soft 1 » ; vide sans écart.
func issuesText(issues []conventions.Issue) string {
	parts := make([]string, len(issues))
	for i, is := range issues {
		parts[i] = is.Code + " " + strconv.Itoa(is.Count)
	}
	return strings.Join(parts, ", ")
}
