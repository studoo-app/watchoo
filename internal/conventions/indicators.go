package conventions

// Indicateurs de la phase 8.2, calculés à la lecture par Build : nommage des branches (BR1), titres de
// pull request (PR1, PR2), tags (SV1, SV2 en présence, SV6 en information) et CC1 par intervalle entre
// tags finals. Ni titre de PR, ni message de commit, ni corps n'est gardé dans un résultat.

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
)

// Raisons d'un indicateur « inconnu » (clés --json), en anglais.
const (
	UnknownNoClone         = "no_clone"         // aucun clone local du dépôt
	UnknownCloneUnreadable = "clone_unreadable" // clone illisible (ou commit d'un tag absent du clone)
	UnknownNoAPI           = "no_api"           // PR et releases : dépôt jamais vu par l'API, ou espace en clone seul
	UnknownFiltered        = "filtered"         // rapport restreint à des contributeurs : indicateur de dépôt non calculé
	UnknownNoFinalTag      = "no_final_tag"     // aucun tag final conforme : ni intervalle ni tendance
)

// PRSkipBeforeSince sort de PR1 et PR2 une pull request ouverte avant la date de début.
const PRSkipBeforeSince = "before_since"

// AllPRSkips liste les motifs qui sortent une pull request de PR1, PR2, PR5 et PR6, dans leur ordre
// d'affichage.
var AllPRSkips = []string{PRSkipClosed, PRSkipDraft, PRSkipBot, PRSkipFork, PRSkipBeforeSince}

// buildEnv regroupe ce que Build prépare une fois pour tous les dépôts.
type buildEnv struct {
	parser   *Parser
	branches *BranchChecker
}

// newBuildEnv prépare l'analyse ; erreur si les règles de branche n'ont pas été validées.
func newBuildEnv(opts BuildOptions) (*buildEnv, error) {
	bc, err := NewBranchChecker(opts.Branch)
	if err != nil {
		return nil, err
	}
	return &buildEnv{parser: NewParser(opts.Rules), branches: bc}, nil
}

// BranchLine est un nom de branche évalué (BR1) et son origine.
type BranchLine struct {
	BranchResult
	InClone bool  // branche du distant lue dans le clone (ou suivie en base, sans clone)
	PRs     []int // pull requests dont c'est la branche de tête, numéro croissant ; vide non nil si aucune
	// AuthorIDs sont les contributeurs auteurs de ces PR (seule attribution d'une branche à une
	// personne), croissants ; AuthorNames et AuthorLogins vont de pair. Vides non nil si aucun.
	AuthorIDs    []int64
	AuthorNames  []string
	AuthorLogins []string
}

// BranchReport est BR1 d'un dépôt : l'état courant des noms de branche, chaque nom une fois, hors
// branche par défaut et conventions.branch.exclude.
type BranchReport struct {
	// Source vaut SourceClone (branches du distant lues dans le clone) ou SourceDatabase (sans clone, ou
	// clone illisible : branches suivies en base, liste partielle).
	Source   string
	Checked  int          // noms évalués
	Conform  int          // noms sans écart bloquant (« à revoir » compris)
	Review   int          // noms conformes à revoir (single_segment)
	Branches []BranchLine // par nom
	// Phase 8.5 : BR2, branches de tête (hors forks) dont plus d'une PR a été fusionnée vers la branche par
	// défaut ; ReusedKnown est faux sans PR de l'API (clone seul, dépôt jamais vu par l'API).
	ReusedKnown bool
	Reused      []ReusedBranch // par nom ; vide non nil si aucune
}

// BranchTally compte les branches d'un contributeur (BR1, par l'auteur de la PR).
type BranchTally struct {
	Checked, Conform, Review int
}

func (t *BranchTally) add(r BranchResult) {
	t.Checked++
	if r.Conform() {
		t.Conform++
	}
	if r.Review() {
		t.Review++
	}
}

// PRCount compte des pull requests évaluées : titres conventionnels (PR1), titres par défaut ou
// interdits (PR2), corps structuré (PR5) et, pour les fusionnées, méthode de fusion (PR6).
type PRCount struct {
	Evaluated int
	Conform   int // PR1
	Default   int // PR2
	// Phase 8.5.
	BodyKnown    int // PR5 : corps lu (masque connu)
	BodyConform  int // PR5 : sections attendues présentes et, si exigé, mot-clé de fermeture
	MergeKnown   int // PR6 (fusionnées) : méthode de fusion déterminée
	Squash       int // PR6 : fusionnées en squash
	MergeConform int // PR6 : squash et message du commit conforme
}

// PRTally compte à part les PR ouvertes et fusionnées.
type PRTally struct {
	Open, Merged PRCount
}

func (t *PRTally) add(p PullLine) {
	c := &t.Open
	if p.Category == PRCategoryMerged {
		c = &t.Merged
	}
	c.Evaluated++
	if p.Conform {
		c.Conform++
	}
	if len(p.Default) > 0 {
		c.Default++
	}
	if p.Body.Known {
		c.BodyKnown++
		if p.Body.Conform {
			c.BodyConform++
		}
	}
	if p.Merge.Method != "" && p.Merge.Known() {
		c.MergeKnown++
		if p.Merge.Method == MergeMethodSquash {
			c.Squash++
		}
		if p.Merge.Conform {
			c.MergeConform++
		}
	}
}

// PullLine est une pull request évaluée, sans son titre : numéro, auteur, branche de tête et codes.
type PullLine struct {
	Number      int
	Category    string // PRCategoryOpen ou PRCategoryMerged
	AuthorID    int64  // contributeur du compte de l'auteur ; 0 si aucun
	AuthorName  string
	AuthorLogin string
	HeadBranch  string
	Codes       []string // PR1 : codes CC2 de l'en-tête
	Default     []string // PR2 : codes d'AllPRTitleCodes ; vide non nil si aucun
	Conform     bool     // PR1 : aucun écart bloquant
	// Phase 8.5.
	Body  PRBodyResult // PR5 : sections du corps et mot-clé de fermeture (Known faux : masque inconnu)
	Merge MergeResult  // PR6 : méthode de fusion d'une PR fusionnée ; Method vide pour une PR ouverte
}

// PullReport est PR1, PR2, PR5 et PR6 d'un dépôt. Mesuré seulement si la base suit les PR par l'API.
type PullReport struct {
	Known  bool
	Reason string // UnknownNoAPI si inconnu
	PRTally
	Skipped map[string]int // PR hors évaluation par motif (toutes les clés d'AllPRSkips)
	Pulls   []PullLine     // PR évaluées, numéro décroissant
	// Methods compte les PR fusionnées évaluées par méthode de fusion (PR6, toutes les clés
	// d'AllMergeMethods).
	Methods map[string]int
}

// TagReport est SV1, SV2 et SV6 d'un dépôt.
type TagReport struct {
	Checked    int               // tags lus
	Conform    int               // SV1 : noms sans écart bloquant
	Tags       []TagResult       // SV1, par nom
	Milestones []MilestoneResult // SV2 : un par jalon configuré, dans l'ordre de la configuration
	Releases   ReleaseReport     // SV6, information seule
}

// ReleaseReport est SV6 d'un dépôt : information seule, hors conformité.
type ReleaseReport struct {
	Known  bool
	Reason string        // UnknownNoAPI si inconnu
	Notes  []ReleaseNote // vide non nil si aucune
}

// IntervalLine est CC1 sur l'intervalle From..To entre deux tags finals successifs.
type IntervalLine struct {
	From  string // tag final précédent ; vide : depuis le début de l'historique
	To    string
	Known bool // faux : intervalle illisible dans le clone
	Tally
	BeforeSince int // commits de l'intervalle antérieurs à la date de début
	// Missing compte les commits de l'intervalle absents de la base (branches non suivies) : sans
	// message, ils ne sont pas évalués.
	Missing int
}

// IntervalReport est CC1 par intervalle entre tags finals d'un dépôt, lu dans le clone.
type IntervalReport struct {
	Known     bool
	Reason    string         // UnknownNoClone, UnknownCloneUnreadable, UnknownFiltered ou UnknownNoFinalTag
	LastTag   string         // dernier tag final (plus grande précédence) ; vide sans tag final
	Intervals []IntervalLine // dans l'ordre de précédence
	// SinceLastTag est CC1 sur les commits du périmètre arrivés depuis LastTag (non atteignables depuis
	// lui).
	SinceLastTag Tally
}

// pullsKnown indique que la base suit les PR et releases du dépôt par l'API : dépôt déjà vu par l'API
// (identifiant de forge connu) et espace qui n'est pas en clone seul. Sinon les lignes en base, s'il y en
// a, peuvent être périmées.
func pullsKnown(repo model.Repository, opts BuildOptions) bool {
	return !opts.LocalOnly && repo.ProviderID != 0
}

// buildExtras calcule les indicateurs de la phase 8.2 d'un dépôt. rows sont les commits lus en base (tous
// les auteurs si authorIDs est nil), results leurs résultats dans l'ordre de rep.Commits.
func buildExtras(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, authorIDs []int64,
	rows []model.ConventionCommit, results []CommitResult, opts BuildOptions, env *buildEnv, rep *RepoReport) error {
	pulls, err := readPulls(ctx, r, repo, opts)
	if err != nil {
		return err
	}
	mine := pullsOf(pulls, authorIDs)
	commits, err := commitIndex(ctx, r, repo, rows, authorIDs != nil, mine)
	if err != nil {
		return err
	}
	rep.Pulls = buildPulls(mine, pullsKnown(repo, opts), rep.Since, env.parser, opts.PRBody, commits)
	if rep.Branches, err = buildBranches(ctx, r, clones, repo, pulls, env.branches); err != nil {
		return err
	}
	if authorIDs != nil {
		rep.Branches = branchesOf(rep.Branches, authorIDs, mine)
	}
	rep.Branches.Reused = []ReusedBranch{}
	if pullsKnown(repo, opts) {
		rep.Branches.ReusedKnown = true
		rep.Branches.Reused = ReusedBranches(mine, repo.DefaultBranch)
	}
	rep.MergeMethods = buildMergeMethods(repo, opts)
	if rep.Arrivals, err = buildArrivals(ctx, clones, repo, authorIDs, pulls, commits, arrivalRange{since: rep.Since},
		opts); err != nil {
		return err
	}
	tags, err := r.ConventionTags(ctx, repo.ID)
	if err != nil {
		return err
	}
	if rep.Tags, err = buildTags(ctx, r, repo, tags, opts); err != nil {
		return err
	}
	if err := buildIntervals(ctx, clones, repo, authorIDs, rows, results, tags, opts, rep); err != nil {
		return err
	}
	if rep.Files, err = buildFiles(ctx, r, clones, repo, authorIDs != nil, tags, opts); err != nil {
		return err
	}
	return buildHistory(ctx, r, clones, repo, authorIDs != nil, rows, tags, opts, rep)
}

// pullsOf garde les PR des contributeurs authorIDs (nil : toutes) : un rapport restreint à des personnes
// ne montre ni ne compte les PR des autres.
func pullsOf(pulls []model.ConventionPullRequest, authorIDs []int64) []model.ConventionPullRequest {
	if authorIDs == nil {
		return pulls
	}
	var out []model.ConventionPullRequest
	for _, pr := range pulls {
		if pr.AuthorID != 0 && slices.Contains(authorIDs, pr.AuthorID) {
			out = append(out, pr)
		}
	}
	return out
}

// branchesOf garde les branches de tête des PR des contributeurs authorIDs, seule attribution d'une
// branche à une personne, et recompte BR1 sur elles : les branches non attribuées (indicateur de dépôt)
// et celles des autres auteurs sont écartées. Une ligne gardée ne cite que les auteurs du filtre et
// leurs PR (mine).
func branchesOf(b BranchReport, authorIDs []int64, mine []model.ConventionPullRequest) BranchReport {
	out := BranchReport{Source: b.Source, Branches: []BranchLine{}, ReusedKnown: b.ReusedKnown, Reused: b.Reused}
	numbers := map[int]bool{}
	for _, pr := range mine {
		numbers[pr.Number] = true
	}
	for _, line := range b.Branches {
		kept := BranchLine{BranchResult: line.BranchResult, InClone: line.InClone, PRs: []int{},
			AuthorIDs: []int64{}, AuthorNames: []string{}, AuthorLogins: []string{}}
		for _, n := range line.PRs {
			if numbers[n] {
				kept.PRs = append(kept.PRs, n)
			}
		}
		for i, id := range line.AuthorIDs {
			if slices.Contains(authorIDs, id) {
				kept.AuthorIDs = append(kept.AuthorIDs, id)
				kept.AuthorNames = append(kept.AuthorNames, line.AuthorNames[i])
				kept.AuthorLogins = append(kept.AuthorLogins, line.AuthorLogins[i])
			}
		}
		if len(kept.AuthorIDs) == 0 {
			continue
		}
		out.Checked++
		if line.Conform() {
			out.Conform++
		}
		if line.Review() {
			out.Review++
		}
		out.Branches = append(out.Branches, kept)
	}
	return out
}

// readPulls lit les PR du dépôt si la base les suit par l'API ; aucune sinon.
func readPulls(ctx context.Context, r Reader, repo model.Repository, opts BuildOptions) ([]model.ConventionPullRequest, error) {
	if !pullsKnown(repo, opts) {
		return nil, nil
	}
	return r.ConventionPullRequests(ctx, repo.ID)
}

// buildPulls calcule PR1, PR2, PR5 et PR6. Le titre et le message du commit de fusion ne sont lus qu'ici,
// en mémoire ; le corps n'est connu que par son masque. commits donne les commits en base par SHA (PR6).
func buildPulls(pulls []model.ConventionPullRequest, known bool, since time.Time, p *Parser, body PRBodyRules,
	commits map[string]model.ConventionCommit) PullReport {
	out := PullReport{Known: known, Skipped: map[string]int{}, Pulls: []PullLine{}, Methods: map[string]int{}}
	for _, s := range AllPRSkips {
		out.Skipped[s] = 0
	}
	for _, m := range AllMergeMethods {
		out.Methods[m] = 0
	}
	if !known {
		out.Reason = UnknownNoAPI
		return out
	}
	for _, pr := range pulls {
		cat, ok := PRCategory(pr.PullRequest)
		if ok && !since.IsZero() && pr.CreatedAt.Before(since) {
			cat, ok = PRSkipBeforeSince, false
		}
		if !ok {
			out.Skipped[cat]++
			continue
		}
		res := p.CheckPRTitle(pr.Title, pr.HeadBranch)
		line := PullLine{
			Number: pr.Number, Category: cat, AuthorID: pr.AuthorID, AuthorName: pr.AuthorName,
			AuthorLogin: pr.AuthorLogin, HeadBranch: pr.HeadBranch, Codes: res.Codes, Default: res.Default,
			Conform: res.Conform(), Body: EvalPRBody(pr.BodySections, body),
		}
		if cat == PRCategoryMerged {
			var c *model.ConventionCommit
			if row, ok := commits[pr.MergeCommitSHA]; ok && pr.MergeCommitSHA != "" {
				c = &row
			}
			line.Merge = ClassifyMerge(pr.Number, c, p)
			out.Methods[line.Merge.Method]++
		}
		out.add(line)
		out.Pulls = append(out.Pulls, line)
	}
	slices.SortFunc(out.Pulls, func(a, b PullLine) int { return cmp.Compare(b.Number, a.Number) })
	return out
}

// buildBranches calcule BR1 : branches du distant lues dans le clone (sans clone ou clone illisible :
// branches suivies en base) et branches de tête des PR (hors bots et, phase 8.5, hors forks), chaque nom
// une fois.
func buildBranches(ctx context.Context, r Reader, clones CloneReader, repo model.Repository,
	pulls []model.ConventionPullRequest, bc *BranchChecker) (BranchReport, error) {
	out := BranchReport{Source: SourceDatabase, Branches: []BranchLine{}}
	var names []string
	inClone := map[string]bool{}
	read := false
	if clones != nil && clones.HasClone(repo.Owner, repo.Name) {
		got, err := clones.RemoteBranches(ctx, repo.Owner, repo.Name)
		if err := ctx.Err(); err != nil {
			return out, err
		}
		if err == nil {
			names, read, out.Source = got, true, SourceClone
		}
	}
	if !read {
		branches, err := r.ListBranches(ctx, repo.ID)
		if err != nil {
			return out, err
		}
		for _, b := range branches {
			if b.Tracked {
				names = append(names, b.Name)
			}
		}
	}
	for _, n := range names {
		inClone[n] = true
	}
	type who struct {
		id          int64
		name, login string
	}
	prs := map[string][]int{}
	authors := map[string][]who{}
	for _, pr := range pulls {
		if pr.HeadBranch == "" || aisignature.IsBot(pr.AuthorLogin) || IsFork(pr.PullRequest) {
			continue // une branche de fork n'est pas une branche du dépôt (phase 8.5)
		}
		names = append(names, pr.HeadBranch)
		prs[pr.HeadBranch] = append(prs[pr.HeadBranch], pr.Number)
		if pr.AuthorID != 0 && !slices.ContainsFunc(authors[pr.HeadBranch], func(w who) bool { return w.id == pr.AuthorID }) {
			authors[pr.HeadBranch] = append(authors[pr.HeadBranch], who{pr.AuthorID, pr.AuthorName, pr.AuthorLogin})
		}
	}
	for _, res := range bc.CheckAll(names, repo.DefaultBranch) {
		line := BranchLine{BranchResult: res, InClone: inClone[res.Name], PRs: slices.Sorted(slices.Values(prs[res.Name])),
			AuthorIDs: []int64{}, AuthorNames: []string{}, AuthorLogins: []string{}}
		if line.PRs == nil {
			line.PRs = []int{}
		}
		ws := authors[res.Name]
		slices.SortFunc(ws, func(a, b who) int { return cmp.Compare(a.id, b.id) })
		for _, w := range ws {
			line.AuthorIDs = append(line.AuthorIDs, w.id)
			line.AuthorNames = append(line.AuthorNames, w.name)
			line.AuthorLogins = append(line.AuthorLogins, w.login)
		}
		out.Checked++
		if res.Conform() {
			out.Conform++
		}
		if res.Review() {
			out.Review++
		}
		out.Branches = append(out.Branches, line)
	}
	return out, nil
}

// buildTags calcule SV1, SV2 et SV6.
func buildTags(ctx context.Context, r Reader, repo model.Repository, tags []model.Tag, opts BuildOptions) (TagReport, error) {
	out := TagReport{Tags: []TagResult{}, Releases: ReleaseReport{Notes: []ReleaseNote{}}}
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	names = sortedNames(names)
	for _, n := range names {
		res := CheckTag(n)
		out.Checked++
		if res.Conform() {
			out.Conform++
		}
		out.Tags = append(out.Tags, res)
	}
	out.Milestones = CheckMilestones(opts.Milestones, names)
	if !pullsKnown(repo, opts) {
		out.Releases.Reason = UnknownNoAPI
		return out, nil
	}
	releases, err := r.ConventionReleases(ctx, repo.ID)
	if err != nil {
		return out, err
	}
	out.Releases.Known = true
	out.Releases.Notes = CheckReleases(out.Milestones, names, releases)
	return out, nil
}

// buildIntervals calcule CC1 par intervalle entre tags finals et place chaque commit du périmètre avant
// ou depuis le dernier tag final (tendance de CC3). Un clone illisible rend l'indicateur inconnu ; seul
// un contexte terminé est une erreur.
func buildIntervals(ctx context.Context, clones CloneReader, repo model.Repository, authorIDs []int64,
	rows []model.ConventionCommit, results []CommitResult, tags []model.Tag, opts BuildOptions, rep *RepoReport) error {
	iv := IntervalReport{Intervals: []IntervalLine{}, SinceLastTag: newTally()}
	defer func() { rep.Intervals = iv }()
	last, ok := LastFinal(tags)
	if ok {
		iv.LastTag = last.Name
	}
	switch {
	case clones == nil || !clones.HasClone(repo.Owner, repo.Name):
		iv.Reason = UnknownNoClone
		return nil
	case !ok:
		iv.Reason = UnknownNoFinalTag
		return nil
	}
	reach, err := revSet(ctx, clones, repo, nil, []string{last.SHA}, nil)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err != nil {
		iv.Reason = UnknownCloneUnreadable
		return nil
	}
	for i := range rep.Commits {
		rep.Commits[i].TrendKnown = true
		rep.Commits[i].SinceLastTag = !reach[rep.Commits[i].SHA]
	}
	if authorIDs != nil {
		iv.Reason = UnknownFiltered
		return nil
	}
	iv.Known = true
	iv.SinceLastTag = TallyOf(results, func(c CommitResult) bool { return !reach[c.SHA] })
	byKey := make(map[string]model.ConventionCommit, len(rows))
	for _, row := range rows {
		byKey[row.SHA] = row
	}
	aopts := Options{Rules: opts.Rules, Since: rep.Since, ExcludedAccounts: opts.ExcludedAccounts}
	for _, t := range TagIntervals(tags) {
		line := IntervalLine{From: t.From.Name, To: t.To.Name, Tally: newTally()}
		var exclude []string
		if t.From.Name != "" {
			exclude = []string{t.From.SHA}
		}
		shas, err := revList(ctx, clones, repo, nil, []string{t.To.SHA}, exclude)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err == nil {
			line.Known = true
			line.Tally, line.BeforeSince, line.Missing = evalRange(shas, byKey, nil, aopts)
		}
		iv.Intervals = append(iv.Intervals, line)
	}
	return nil
}

// revList lit un intervalle de commits dans le clone.
func revList(ctx context.Context, clones CloneReader, repo model.Repository, branches, include, exclude []string) ([]string, error) {
	return clones.RevRange(ctx, repo.Owner, repo.Name, branches, include, exclude)
}

// revSet lit un intervalle de commits dans le clone, en ensemble.
func revSet(ctx context.Context, clones CloneReader, repo model.Repository, branches, include, exclude []string) (map[string]bool, error) {
	shas, err := revList(ctx, clones, repo, branches, include, exclude)
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(shas))
	for _, s := range shas {
		set[s] = true
	}
	return set, nil
}

// evalRange évalue (CC1) les commits shas d'un intervalle connus en base (byKey) et retenus par keep
// (nil : tous) ; missing compte ceux qui sont absents de la base. Un commit absent n'a pas de date :
// keep (borne d'échéance) ne peut pas l'écarter, il est donc compté dans missing même s'il était
// postérieur à l'échéance (choix prudent : CC1 « partiel » plutôt qu'un 100 % non vérifié).
func evalRange(shas []string, byKey map[string]model.ConventionCommit, keep func(model.ConventionCommit) bool,
	opts Options) (t Tally, beforeSince, missing int) {
	var commits []Commit
	for _, s := range shas {
		row, ok := byKey[s]
		switch {
		case !ok:
			missing++
		case keep == nil || keep(row):
			commits = append(commits, toCommit(row))
		}
	}
	res := Aggregate(commits, opts)
	return res.Tally, res.BeforeSince, missing
}

// sortedNames renvoie les noms triés, sans doublon.
func sortedNames(names []string) []string {
	out := slices.Clone(names)
	slices.SortFunc(out, strings.Compare)
	return slices.Compact(out)
}
