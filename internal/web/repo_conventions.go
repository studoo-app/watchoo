package web

// Conventions d'un dépôt sur « /repos/{id} » (1.7.0, fragment « repo-conventions », seulement si les
// conventions sont activées) et lignes « Commits conformes n/m (x %) », « Titres de PR conformes » et
// tendance de la page d'un contributeur. Les taux, écarts et exclusions viennent de internal/conventions
// (Build), calculés à la lecture ; aucune note, aucun classement, aucun message de commit ni titre de pull
// request affiché (SHA court, numéro, auteur, noms de branche et de tag, codes). Phase 8.2 : branches non
// conformes (BR1), badges par PR (PR1, PR2), pastille par tag (SV1), jalons (SV2), releases (SV6, en
// information), CC1 par intervalle entre tags, formulation, pieds et langue des sujets (CC5 à CC7).
// Séparé du diagnostic : ni donnée, ni clé, ni fonction de template en commun.

import (
	"net/http"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// repoConventionsID est l'identifiant HTML du fragment (cible HTMX).
const repoConventionsID = "repo-conventions"

// repoConventionsFailing est le nombre de commits non conformes récents affichés sur la page du dépôt.
const repoConventionsFailing = 10

// repoConventionsList est le nombre de lignes affichées par liste de la phase 8.2 (branches, PR, tags,
// intervalles) sur la page du dépôt ; le total est toujours donné.
const repoConventionsList = 20

// convCount est un motif d'exclusion (ou un code informatif) et son nombre de commits.
type convCount struct {
	Code  string
	Label string
	Count int
}

// convIssueRow est un code d'écart du dépôt.
type convIssueRow struct {
	Code     string
	Label    string
	Blocking bool
	Severity string // libellé traduit de la gravité
	Count    int
}

// convCommitRow est un commit non conforme, sans son message.
type convCommitRow struct {
	SHA    string
	Author string // nom, sinon login ; vide si inconnu
	Date   time.Time
	Codes  []string // codes bloquants puis avertissements
}

// convCode est un code d'écart et son libellé traduit (badge : bloquant ou avertissement).
type convCode struct {
	Code     string
	Label    string
	Blocking bool
}

// convStatus est un état affiché en pastille : une icône, toujours suivie de son texte.
type convStatus struct {
	Icon  string // symbole décoratif (aria-hidden)
	Label string // texte traduit
	Class string // classe du badge (badge-success…)
}

// convBranchRow est une branche non conforme ou à revoir (BR1).
type convBranchRow struct {
	Name   string
	Review bool // conforme, à revoir (single_segment)
	Codes  []convCode
	PRs    []int // pull requests dont c'est la branche de tête
}

// convPullRow est une pull request au titre non conforme ou par défaut : numéro, auteur, branche de tête et
// codes, jamais le titre.
type convPullRow struct {
	Number     int
	Merged     bool
	Author     string // nom, sinon login ; vide si inconnu
	AuthorID   int64  // contributeur ; 0 si aucun
	HeadBranch string
	Codes      []convCode // PR1 puis PR2
	// Phase 8.5 : Body, pastille PR5 (sections et mot-clé de fermeture) ; Merge, pastille PR6 d'une PR
	// fusionnée (nil si ouverte).
	Body  convStatus
	Merge *convStatus
	// Phase 8.6 : pastille PR4 d'une PR fusionnée (nil si ouverte).
	Review *convStatus
}

// convTagRow est la pastille d'un tag (SV1).
type convTagRow struct {
	Name   string
	Status convStatus
	Codes  []convCode
}

// convMilestoneRow est un jalon configuré dans un dépôt (SV2 : présence, et échéance depuis la phase 8.4).
type convMilestoneRow struct {
	Name   string
	Due    string // AAAA-MM-JJ ; vide sans échéance
	Status convStatus
	Tag    string // tag qui le désigne sous un nom non conforme ; vide sinon
	// Deadline est la pastille de l'échéance (à l'heure, en retard, indéterminée) ; nil sans échéance jugée.
	Deadline       *convStatus
	DeadlineDetail string // première observation, ou « posé entre le … et le … »
}

// convReleaseRow est une remarque SV6 (information seule).
type convReleaseRow struct {
	Label string
	Tag   string
}

// convIntervalRow est CC1 sur un intervalle entre deux tags finals.
type convIntervalRow struct {
	From      string // vide : depuis le début de l'historique
	To        string
	Known     bool
	Evaluated int
	Conform   int
	Percent   int
	Missing   int
}

// convPRCounts sont les titres de PR évalués d'un dépôt ou d'une personne, ouvertes et fusionnées à part,
// et (phase 8.5) leurs corps (PR5) et fusions en squash au message conforme (PR6).
type convPRCounts struct {
	OpenEvaluated, OpenConform, OpenDefault       int
	MergedEvaluated, MergedConform, MergedDefault int
	BodyKnown, BodyConform                        int
	MergeConform                                  int
	// Phase 8.6 (PR4) : fusionnées dont les relectures sont lues et relues ; relectures reçues (ouvertes et
	// fusionnées) ; ReviewsRead compte les PR, ouvertes et fusionnées, dont les relectures sont lues (rien
	// n'est affiché sans elles, comme dans la CLI).
	ReviewKnown, Reviewed, ReviewsReceived, ReviewsRead int
}

func newConvPRCounts(t conventions.PRTally) convPRCounts {
	return convPRCounts{
		OpenEvaluated: t.Open.Evaluated, OpenConform: t.Open.Conform, OpenDefault: t.Open.Default,
		MergedEvaluated: t.Merged.Evaluated, MergedConform: t.Merged.Conform, MergedDefault: t.Merged.Default,
		BodyKnown: t.Open.BodyKnown + t.Merged.BodyKnown, BodyConform: t.Open.BodyConform + t.Merged.BodyConform,
		MergeConform: t.Merged.MergeConform,
		ReviewKnown:  t.Merged.ReviewKnown, Reviewed: t.Merged.Reviewed,
		ReviewsReceived: t.Open.ReviewsReceived + t.Merged.ReviewsReceived,
		ReviewsRead:     t.Open.ReviewKnown + t.Merged.ReviewKnown,
	}
}

// Evaluated renvoie le nombre de PR évaluées, ouvertes et fusionnées.
func (c convPRCounts) Evaluated() int { return c.OpenEvaluated + c.MergedEvaluated }

// convLanguage est le résumé CC7 d'un dépôt (information).
type convLanguage struct {
	FR, EN, Unknown int
	Expected        string // fr, en ou any
	ExpectedLabel   string // langue attendue traduite ; vide si any
	Other           int
	Mixed           bool
}

// convTrend est la tendance CC3 d'un contributeur : deux taux, avant et depuis le dernier tag final.
type convTrend struct {
	Known                          bool
	BeforeEvaluated, BeforeConform int
	BeforePercent                  int
	SinceEvaluated, SinceConform   int
	SincePercent                   int
	Unmeasured                     int
}

func newConvTrend(t conventions.Trend) convTrend {
	return convTrend{
		Known:           t.Known(),
		BeforeEvaluated: t.Before.Evaluated, BeforeConform: t.Before.Conform,
		BeforePercent:  percent(t.Before.Conform, t.Before.Evaluated),
		SinceEvaluated: t.Since.Evaluated, SinceConform: t.Since.Conform,
		SincePercent: percent(t.Since.Conform, t.Since.Evaluated),
		Unmeasured:   t.Unmeasured,
	}
}

// RepoConventionsView est la partie « conventions » de la page du dépôt.
type RepoConventionsView struct {
	// Unknown : dépôt jamais synchronisé (taux inconnu, pas 0 %).
	Unknown   bool
	Evaluated int
	Conform   int
	Percent   int // Conform / Evaluated, arrondi ; sans objet si Evaluated vaut 0
	// DefaultBranch : vue informative de la seule branche par défaut (vide si inconnue).
	DefaultBranch    string
	DefaultEvaluated int
	DefaultConform   int
	DefaultPercent   int
	Excluded         int         // commits exclus (CC4), tous motifs
	Exclusions       []convCount // motifs à au moins un commit
	BeforeSince      int         // commits antérieurs à la date de début
	Issues           []convIssueRow
	Failing          []convCommitRow // les repoConventionsFailing plus récents
	FailingTotal     int
	// Database : repli sur la base (pas de clone lisible) ; peut inclure des commits réécrits.
	Database bool
	// Since : date de début effective (minuit dans le fuseau de l'espace), zéro si aucune ; SinceOrigin
	// son origine traduite (espace ou propre au dépôt).
	Since          time.Time
	SinceOrigin    string
	ParentsUnknown int

	// Phase 8.2.
	// Wording : CC5 (avertissements) par code, codes à au moins un commit ; Footer et Issue : CC6 ;
	// Language : CC7 (information).
	Wording  []convCount
	Footer   int
	Issue    int
	Language convLanguage
	// BR1 : branches évaluées (BranchesDatabase : liste partielle des branches suivies en base, sans
	// clone) ; BranchRows : non conformes et à revoir, au plus repoConventionsList.
	BranchesChecked, BranchesConform, BranchesReview int
	BranchesDatabase                                 bool
	BranchRows                                       []convBranchRow
	BranchRowsTotal                                  int
	// PR1 et PR2 : PullsReason (traduit) si inconnues ; PullRows : titres non conformes ou par défaut.
	PullsKnown    bool
	PullsReason   string
	Pulls         convPRCounts
	PullsSkipped  []convCount
	PullRows      []convPullRow
	PullRowsTotal int
	// SV1 (pastille par tag), SV2 (jalons) et SV6 (information).
	TagsChecked    int
	TagsConform    int
	Tags           []convTagRow
	TagsTotal      int
	Milestones     []convMilestoneRow
	ReleasesKnown  bool
	ReleasesReason string
	Releases       []convReleaseRow
	// CC1 par intervalle entre tags finals, et depuis le dernier.
	IntervalsKnown   bool
	IntervalsReason  string
	Intervals        []convIntervalRow
	LastTag          string
	SinceLastEval    int
	SinceLastConform int
	SinceLastPercent int

	// Phase 8.3 : CHANGELOG, README, documentation et outillage (cache de la synchro), listes de chemins.
	Files convFilesView

	// Phase 8.4 : réécritures de la branche par défaut, historique des tags, versions, fraîcheur.
	History convHistoryView

	// Phase 8.5 : commits hors PR, corps et fusion des PR, branches réutilisées, méthodes de fusion.
	Merges convMergesView

	// Phase 8.7 : pistes, hors de toute grille et de tout taux.
	Leads convLeadsView
}

// conventionsInputs renvoie l'accès aux clones (lus sans mise à jour, sinon repli sur la base) et les
// options de calcul de l'espace ; en clone seul (synchro sans gh), PR et releases sont « inconnues ». Si
// la source ne peut pas être lue, le mode API est supposé, comme dans la CLI.
func (s *server) conventionsInputs(r *http.Request) (conventions.CloneReader, conventions.BuildOptions, error) {
	ctx := r.Context()
	opts := s.cfg.Conventions.BuildOptions(s.loc)
	clones, err := s.repos.ConventionsClones(ctx)
	if err != nil {
		return nil, opts, err
	}
	if opts.LocalOnly, err = wsync.ConventionsLocalOnly(ctx, s.repos.Source); err != nil {
		return nil, opts, err
	}
	return clones, opts, nil
}

// conventionsReport calcule le rapport des conventions pour f ; pour un seul dépôt (section de la page du
// dépôt), les listes de chemins des critères de fichiers sont recalculées depuis le clone.
func (s *server) conventionsReport(r *http.Request, f conventions.Filter) (conventions.BuildReport, error) {
	clones, opts, err := s.conventionsInputs(r)
	if err != nil {
		return conventions.BuildReport{}, err
	}
	opts.FileDetail = f.RepoID != 0 && f.AuthorIDs == nil
	return conventions.Build(r.Context(), s.store, clones, f, opts)
}

// convIssues traduit les écarts d'un décompte.
func convIssues(tr *i18n.Translator, t conventions.Tally) []convIssueRow {
	var out []convIssueRow
	for _, is := range t.Issues {
		out = append(out, convIssueRow{Code: is.Code, Label: tr.T("web.conv.code." + is.Code),
			Blocking: is.Severity == conventions.SeverityBlocking,
			Severity: tr.T("web.conv.severity." + is.Severity.String()), Count: is.Count})
	}
	return out
}

// convCodesOf traduit des codes (préfixe de clé du catalogue) avec leur gravité.
func convCodesOf(tr *i18n.Translator, prefix string, codes []string, severity func(string) conventions.Severity) []convCode {
	out := make([]convCode, 0, len(codes))
	for _, c := range codes {
		out = append(out, convCode{Code: c, Label: tr.T(prefix + c), Blocking: severity(c) == conventions.SeverityBlocking})
	}
	return out
}

// prDefaultSeverity : un titre par défaut ou interdit (PR2) est un écart bloquant.
func prDefaultSeverity(string) conventions.Severity { return conventions.SeverityBlocking }

// Pastilles d'état : une icône décorative, toujours accompagnée d'un texte.
func statusConform(tr *i18n.Translator, key string) convStatus {
	return convStatus{Icon: "✓", Label: tr.T(key), Class: "badge-success"}
}

func statusWarning(tr *i18n.Translator, key string) convStatus {
	return convStatus{Icon: "!", Label: tr.T(key), Class: "badge-warning"}
}

func statusNonconform(tr *i18n.Translator, key string) convStatus {
	return convStatus{Icon: "✗", Label: tr.T(key), Class: "badge-error"}
}

func statusUnknown(tr *i18n.Translator, key string) convStatus {
	return convStatus{Icon: "?", Label: tr.T(key), Class: "badge-ghost"}
}

// milestoneStatus est la pastille SV2 d'un jalon.
func milestoneStatus(tr *i18n.Translator, status string) convStatus {
	key := "web.conv.milestone_status." + status
	switch status {
	case conventions.MilestonePresent:
		return statusConform(tr, key)
	case conventions.MilestoneNonconformingName:
		return statusWarning(tr, key)
	default:
		return convStatus{Icon: "○", Label: tr.T(key), Class: "badge-ghost"}
	}
}

// criterionStatus est la pastille d'un critère de la revue de jalon.
func criterionStatus(tr *i18n.Translator, status string) convStatus {
	key := "web.conv.criterion_status." + status
	switch status {
	case conventions.CriterionConform:
		return statusConform(tr, key)
	case conventions.CriterionNonconform:
		return statusNonconform(tr, key)
	default:
		return statusUnknown(tr, key)
	}
}

// tagRow est la pastille SV1 d'un tag.
func tagRow(tr *i18n.Translator, t conventions.TagResult) convTagRow {
	row := convTagRow{Name: t.Name, Codes: convCodesOf(tr, "web.conv.tag_code.", t.Codes, conventions.TagSeverityOf)}
	switch {
	case !t.Conform():
		row.Status = statusNonconform(tr, "web.conv.tag_nonconform")
	case len(t.Codes) > 0:
		row.Status = statusWarning(tr, "web.conv.tag_warning")
	default:
		row.Status = statusConform(tr, "web.conv.tag_conform")
	}
	return row
}

// convLanguageOf traduit le résumé CC7.
func convLanguageOf(tr *i18n.Translator, l conventions.LanguageSummary) convLanguage {
	out := convLanguage{FR: l.FR, EN: l.EN, Unknown: l.Unknown, Expected: l.Expected, Other: l.Other, Mixed: l.Mixed}
	if l.Expected == conventions.LanguageFR || l.Expected == conventions.LanguageEN {
		out.ExpectedLabel = tr.T("web.conv.language." + l.Expected)
	}
	return out
}

// repoConventions lit les conventions du dépôt id ; nil si les conventions sont désactivées.
func (s *server) repoConventions(r *http.Request, id int64) (*RepoConventionsView, error) {
	if !s.cfg.Conventions.Enabled {
		return nil, nil
	}
	rep, err := s.conventionsReport(r, conventions.Filter{RepoID: id})
	if err != nil {
		return nil, err
	}
	v := &RepoConventionsView{}
	if len(rep.Repos) == 0 {
		v.Unknown = true
		return v, nil
	}
	tr := s.tr(r)
	repo := rep.Repos[0]
	v.Unknown = repo.Repo.LastSyncedAt.IsZero()
	v.Evaluated, v.Conform, v.Percent = repo.Evaluated, repo.Conform, percent(repo.Conform, repo.Evaluated)
	v.DefaultBranch = repo.Repo.DefaultBranch
	v.DefaultEvaluated, v.DefaultConform = repo.DefaultBranch.Evaluated, repo.DefaultBranch.Conform
	v.DefaultPercent = percent(repo.DefaultBranch.Conform, repo.DefaultBranch.Evaluated)
	v.Excluded, v.BeforeSince, v.ParentsUnknown = repo.ExcludedTotal(), repo.BeforeSince, repo.ParentsUnknown
	v.Database = repo.Source == conventions.SourceDatabase
	for _, e := range conventions.AllExclusions {
		if n := repo.Excluded[e]; n > 0 {
			v.Exclusions = append(v.Exclusions, convCount{Code: e, Label: tr.T("web.conv.exclusion." + e), Count: n})
		}
	}
	v.Issues = convIssues(tr, repo.Tally)
	for _, c := range repo.Commits {
		if !c.Evaluated() || c.Conform {
			continue
		}
		v.FailingTotal++
		if len(v.Failing) < repoConventionsFailing {
			v.Failing = append(v.Failing, convCommitRow{SHA: c.SHA, Author: c.DisplayAuthor(),
				Date: c.AuthoredAt, Codes: c.Codes})
		}
	}
	if !repo.Since.IsZero() {
		v.Since, v.SinceOrigin = repo.Since, tr.T("web.conv.since_origin."+repo.SinceOrigin)
	}
	s.repoConventionsTraits(tr, repo, v)
	s.repoConventionsBranches(tr, repo, v)
	s.repoConventionsPulls(tr, repo, v)
	s.repoConventionsTags(tr, repo, v)
	s.repoConventionsIntervals(tr, repo, v)
	v.Files = newConvFiles(tr, repo.Files)
	v.History = newConvHistory(tr, repo)
	v.Merges = newConvMerges(tr, repo)
	v.Leads = s.newConvLeads(tr, repo)
	return v, nil
}

// repoConventionsTraits remplit CC5 à CC7.
func (s *server) repoConventionsTraits(tr *i18n.Translator, repo conventions.RepoReport, v *RepoConventionsView) {
	for _, c := range conventions.AllWordingCodes {
		if n := repo.Traits.Wording[c]; n > 0 {
			v.Wording = append(v.Wording, convCount{Code: c, Label: tr.T("web.conv.wording." + c), Count: n})
		}
	}
	v.Footer, v.Issue = repo.Traits.Footer, repo.Traits.Issue
	v.Language = convLanguageOf(tr, repo.Language)
}

// repoConventionsBranches remplit BR1 : les branches non conformes et à revoir.
func (s *server) repoConventionsBranches(tr *i18n.Translator, repo conventions.RepoReport, v *RepoConventionsView) {
	b := repo.Branches
	v.BranchesChecked, v.BranchesConform, v.BranchesReview = b.Checked, b.Conform, b.Review
	v.BranchesDatabase = b.Source == conventions.SourceDatabase
	for _, l := range b.Branches {
		if l.Conform() && !l.Review() {
			continue
		}
		v.BranchRowsTotal++
		if len(v.BranchRows) < repoConventionsList {
			v.BranchRows = append(v.BranchRows, convBranchRow{Name: l.Name, Review: l.Conform(), PRs: l.PRs,
				Codes: convCodesOf(tr, "web.conv.branch_code.", l.Codes, conventions.BranchSeverityOf)})
		}
	}
}

// repoConventionsPulls remplit PR1, PR2 et, phase 8.5, les pastilles PR5 et PR6 : numéro, auteur, branche
// de tête et codes, jamais le titre ni le corps.
func (s *server) repoConventionsPulls(tr *i18n.Translator, repo conventions.RepoReport, v *RepoConventionsView) {
	p := repo.Pulls
	v.PullsKnown = p.Known
	if !p.Known {
		v.PullsReason = tr.T("web.conv.reason." + p.Reason)
		return
	}
	v.Pulls = newConvPRCounts(p.PRTally)
	for _, k := range conventions.AllPRSkips {
		if n := p.Skipped[k]; n > 0 {
			v.PullsSkipped = append(v.PullsSkipped, convCount{Code: k, Label: tr.T("web.conv.pr_skip." + k), Count: n})
		}
	}
	for _, l := range p.Pulls {
		if !pullNeedsRow(l) {
			continue
		}
		v.PullRowsTotal++
		if len(v.PullRows) >= repoConventionsList {
			continue
		}
		codes := convCodesOf(tr, "web.conv.code.", l.Codes, conventions.SeverityOf)
		codes = append(codes, convCodesOf(tr, "web.conv.pr_default.", l.Default, prDefaultSeverity)...)
		v.PullRows = append(v.PullRows, convPullRow{Number: l.Number, Merged: l.Category == conventions.PRCategoryMerged,
			Author: l.DisplayAuthor(), AuthorID: l.AuthorID, HeadBranch: l.HeadBranch, Codes: codes,
			Body: bodyStatus(tr, l.Body), Merge: mergeStatus(tr, l.Merge), Review: reviewStatus(tr, l)})
	}
}

// repoConventionsTags remplit SV1 (pastille par tag), SV2 (jalons) et SV6 (information).
func (s *server) repoConventionsTags(tr *i18n.Translator, repo conventions.RepoReport, v *RepoConventionsView) {
	t := repo.Tags
	v.TagsChecked, v.TagsConform, v.TagsTotal = t.Checked, t.Conform, len(t.Tags)
	for _, tag := range t.Tags {
		if len(v.Tags) >= repoConventionsList {
			break
		}
		v.Tags = append(v.Tags, tagRow(tr, tag))
	}
	for _, m := range t.Milestones {
		row := convMilestoneRow{Name: m.Name, Due: m.Due, Status: milestoneStatus(tr, m.Status),
			Deadline: deadlineStatus(tr, m.Deadline), DeadlineDetail: s.deadlineDetail(tr, m.Deadline)}
		if m.Status == conventions.MilestoneNonconformingName {
			row.Tag = m.Tag
		}
		v.Milestones = append(v.Milestones, row)
	}
	v.ReleasesKnown = t.Releases.Known
	if !t.Releases.Known {
		v.ReleasesReason = tr.T("web.conv.reason." + t.Releases.Reason)
		return
	}
	for _, n := range t.Releases.Notes {
		v.Releases = append(v.Releases, convReleaseRow{Label: tr.T("web.conv.release_code." + n.Code), Tag: n.Tag})
	}
}

// repoConventionsIntervals remplit CC1 par intervalle entre tags finals.
func (s *server) repoConventionsIntervals(tr *i18n.Translator, repo conventions.RepoReport, v *RepoConventionsView) {
	iv := repo.Intervals
	v.IntervalsKnown = iv.Known
	if !iv.Known {
		v.IntervalsReason = tr.T("web.conv.reason." + iv.Reason)
		return
	}
	v.LastTag = iv.LastTag
	v.SinceLastEval, v.SinceLastConform = iv.SinceLastTag.Evaluated, iv.SinceLastTag.Conform
	v.SinceLastPercent = percent(iv.SinceLastTag.Conform, iv.SinceLastTag.Evaluated)
	// Les plus récents d'abord : la liste est coupée aux repoConventionsList derniers intervalles.
	for i := len(iv.Intervals) - 1; i >= 0 && len(v.Intervals) < repoConventionsList; i-- {
		l := iv.Intervals[i]
		v.Intervals = append(v.Intervals, convIntervalRow{From: l.From, To: l.To, Known: l.Known,
			Evaluated: l.Evaluated, Conform: l.Conform, Percent: percent(l.Conform, l.Evaluated), Missing: l.Missing})
	}
}

// contributorConventions est la ligne « Commits conformes n/m (x %) » de la page d'un contributeur (CC3,
// tous dépôts confondus), avec ses écarts, ses titres de PR (PR1, PR2) et la tendance de CC3.
type contributorConventions struct {
	Evaluated int
	Conform   int
	Percent   int
	Excluded  int
	Issues    []convIssueRow
	// Phase 8.2.
	Pulls convPRCounts
	Trend convTrend
	// Phase 8.5 : commits de la branche par défaut dont il est l'auteur arrivés hors PR (PR3), tous dépôts
	// confondus ; Arrivals.Known faux si PR3 n'est connu dans aucun dépôt.
	Arrivals convArrivals
}

// contributorConventionsOf calcule les lignes des conventions du contributeur id ; nil si les conventions
// sont désactivées.
func (s *server) contributorConventionsOf(r *http.Request, id int64) (*contributorConventions, error) {
	if !s.cfg.Conventions.Enabled {
		return nil, nil
	}
	rep, err := s.conventionsReport(r, conventions.Filter{AuthorIDs: []int64{id}})
	if err != nil {
		return nil, err
	}
	tr := s.tr(r)
	t := rep.Total
	v := &contributorConventions{Evaluated: t.Evaluated, Conform: t.Conform, Percent: percent(t.Conform, t.Evaluated),
		Excluded: t.ExcludedTotal(), Issues: convIssues(tr, t)}
	for _, c := range rep.Contributors {
		if c.AuthorID == id {
			v.Pulls, v.Trend = newConvPRCounts(c.Pulls), newConvTrend(c.Trend)
			break
		}
	}
	for _, repo := range rep.Repos {
		if repo.Arrivals.Known {
			v.Arrivals.Known = true
			v.Arrivals.addArrivals(tr, repo.Arrivals, repo.Repo.DisplayName(), repo.Repo.ID)
		}
	}
	return v, nil
}
