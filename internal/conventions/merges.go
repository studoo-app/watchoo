package conventions

// Fusions (phase 8.5) : méthode de fusion d'une PR (PR6), commits arrivés sur la branche par défaut hors
// PR (PR3), branches réutilisées (BR2) et méthodes de fusion autorisées (BR5). Fonctions pures sur des
// valeurs déjà lues : ni base, ni forge, ni git, ni journal. Les messages ne sont lus qu'en mémoire ; un
// résultat ne porte que des codes, des numéros et des SHA.

import (
	"cmp"
	"regexp"
	"slices"
	"strconv"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
)

// Méthodes de fusion d'une PR (PR6, clés --json), en anglais.
const (
	MergeMethodSquash  = "squash"  // un parent et le suffixe « (#n) » de la PR
	MergeMethodMerge   = "merge"   // deux parents ou plus : merge commit
	MergeMethodRebase  = "rebase"  // un parent sans suffixe : rebase (ou squash au sujet réécrit)
	MergeMethodUnknown = "unknown" // commit absent de la base, parents inconnus, suffixe d'une autre PR
)

// AllMergeMethods liste les méthodes de fusion dans leur ordre d'affichage.
var AllMergeMethods = []string{MergeMethodSquash, MergeMethodMerge, MergeMethodRebase, MergeMethodUnknown}

// squashNumberRe lit le numéro du suffixe « (#n) » d'un sujet.
var squashNumberRe = regexp.MustCompile(`\(#([0-9]+)\)$`)

// squashNumber renvoie le numéro du suffixe « (#n) » du sujet d'un message ; 0 si aucun.
func squashNumber(message string) int {
	m := squashNumberRe.FindStringSubmatch(Subject(message))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// IsIdentifiedSquash indique un commit de squash identifié, pour les ajustements A1 et A2 du diagnostic :
// committer GitHub (githubCommitter) et sujet terminé par « (#n) » (phase 8.1), ou commit de fusion
// (merge_commit_sha) de la PR mergePR (> 0) à un seul parent (parentCount ; négatif : inconnu) dont le
// sujet se termine par « (#mergePR) » (phase 8.5). Le message n'est lu qu'en mémoire.
func IsIdentifiedSquash(message string, githubCommitter bool, mergePR, parentCount int) bool {
	if message == "" {
		return false
	}
	if githubCommitter && IsSquashSubject(message) {
		return true
	}
	return mergePR > 0 && parentCount == 1 && squashNumber(message) == mergePR
}

// MergeResult est PR6 pour une PR fusionnée. Il ne garde rien du message.
type MergeResult struct {
	Method string // une valeur d'AllMergeMethods
	// MessageConform indique un message de commit conforme au parseur (CC2) ; significatif pour un squash.
	MessageConform bool
	// Conform : fusion en squash ET message conforme.
	Conform bool
}

// Known indique une méthode déterminée.
func (r MergeResult) Known() bool { return r.Method != MergeMethodUnknown }

// ClassifyMerge détermine (PR6) la méthode de fusion de la PR number d'après le commit de son
// merge_commit_sha (nil : commit absent de la base) : deux parents ou plus, merge commit ; un parent et le
// suffixe « (#number) », squash ; un parent sans suffixe, rebase (un squash dont le sujet a été réécrit
// sans le suffixe est aussi classé ici : approximation documentée) ; parents inconnus, suffixe d'une autre
// PR ou commit absent, inconnu. Conforme si squash et message conforme au parseur p.
func ClassifyMerge(number int, commit *model.ConventionCommit, p *Parser) MergeResult {
	out := MergeResult{Method: MergeMethodUnknown}
	if commit == nil || commit.ParentCount == nil {
		return out
	}
	switch n := *commit.ParentCount; {
	case n >= 2:
		out.Method = MergeMethodMerge
	case n == 1:
		switch squashNumber(commit.Message) {
		case number:
			out.Method = MergeMethodSquash
		case 0:
			out.Method = MergeMethodRebase
		}
	}
	if out.Method == MergeMethodSquash {
		out.MessageConform = p.Parse(commit.Message).Conform()
		out.Conform = out.MessageConform
	}
	return out
}

// Classement d'un commit du premier parent de la branche par défaut (PR3, clés --json), en anglais.
const (
	ArrivalByPR         = "by_pr"        // arrivé par une PR
	ArrivalOutsidePR    = "outside_pr"   // arrivé hors PR
	ArrivalUndetermined = "undetermined" // indéterminé
)

// Raisons du classement PR3 (clés --json), en anglais.
const (
	// Par PR.
	ArrivalMergeCommitSHA = "merge_commit_sha" // SHA égal au merge_commit_sha d'une PR fusionnée
	ArrivalRebaseRun      = "rebase_run"       // suite rebasée par GitHub, terminée par la PR (même committed_at)
	ArrivalGitHubMerge    = "github_merge"     // fusion dont le committer est GitHub
	ArrivalSquashSuffix   = "squash_suffix"    // committer GitHub et « (#n) » d'une PR sans merge_commit_sha connu
	// Hors PR.
	ArrivalHumanCommitter = "human_committer" // commit d'un committer humain (push direct)
	ArrivalLocalMerge     = "local_merge"     // fusion faite localement puis poussée
	ArrivalWebEdit        = "web_edit"        // commit de GitHub sans PR (édition web)
	// Indéterminé.
	ArrivalNotInDatabase    = "not_in_db"         // commit absent de la base
	ArrivalCommitterUnknown = "committer_unknown" // committer inconnu
	ArrivalUnmatched        = "unmatched"         // committer GitHub sans PR reconnue (liste incomplète, PR citée absente)
	ArrivalCloneOnly        = "clone_only"        // clone seul : commit de GitHub qui n'est ni fusion ni squash
)

// FirstParentOptions décrit le contexte du classement PR3.
type FirstParentOptions struct {
	// DefaultBranch est la branche par défaut : seules les PR fusionnées vers elle comptent.
	DefaultBranch string
	// API indique des PR connues par l'API ; faux : clone seul, repli sur les messages de GitHub.
	API bool
	// Complete indique une liste des PR relue en entier (merge_commit_sha rempli pour toutes) : un commit
	// de GitHub sans PR reconnue est alors une édition web, sinon il est indéterminé.
	Complete bool
}

// ArrivalLine est le classement PR3 d'un commit du premier parent. Il ne garde rien du message.
type ArrivalLine struct {
	SHA         string
	Class       string // ArrivalByPR, ArrivalOutsidePR ou ArrivalUndetermined
	Reason      string // raison du classement
	PR          int    // PR d'arrivée ; 0 si inconnue ou hors PR
	AuthorID    int64  // auteur du commit (contributeur) ; 0 = inconnu
	AuthorName  string // nom de l'auteur ; vide si inconnu
	AuthorLogin string // login de forge de l'auteur ; vide si inconnu
}

// mergedPR est une PR fusionnée vers la branche par défaut, réduite à ce que PR3 lit.
type mergedPR struct {
	number   int
	sha      string
	mergedAt time.Time
}

// ClassifyFirstParent classe (PR3) les commits shas du premier parent de la branche par défaut (ordre de
// git rev-list --first-parent, du plus récent au plus ancien, commit racine déjà retiré), connus en base
// par byKey, d'après les PR fusionnées pulls.
//
// Avec l'API : par PR si le SHA est le merge_commit_sha d'une PR fusionnée vers la branche par défaut, si
// le commit appartient à une suite rebasée par GitHub (commits consécutifs du committer GitHub, hors
// fusions, au même committed_at que le commit le plus récent de la suite, qui est le merge_commit_sha de la
// PR ; à défaut de merge_commit_sha connu, la suite est rattachée à la PR dont le merged_at est à ±2 s du
// committed_at de son commit le plus récent), si c'est une fusion du committer GitHub, ou un commit de
// GitHub au suffixe « (#n) » d'une PR fusionnée dont le merge_commit_sha est inconnu. Hors PR : committer
// humain (push direct, ou fusion locale poussée), ou commit de GitHub sans PR quand la liste des PR est
// complète (édition web). Un commit de GitHub dont le sujet cite une PR absente de la liste (« (#n) »)
// reste indéterminé, même liste complète : la liste peut être en retard sur les commits. Sinon indéterminé.
//
// En clone seul : par PR si le committer est GitHub (adresse noreply@github.com) et que le sujet est
// « Merge pull request #n … » ou se termine par « (#n) » ; hors PR si le committer est humain ; sinon
// indéterminé. L'attribution est celle de l'auteur du commit, jamais de la personne qui l'a poussé.
func ClassifyFirstParent(shas []string, byKey map[string]model.ConventionCommit,
	pulls []model.ConventionPullRequest, opts FirstParentOptions) []ArrivalLine {
	var merged []mergedPR
	bySHA := map[string]int{}
	byNumber := map[int]mergedPR{}
	known := map[int]bool{} // toutes les PR connues, quelle que soit leur branche de base ou leur état
	if opts.API {
		for _, pr := range pulls {
			known[pr.Number] = true
			if !pr.Merged || pr.MergedAt.IsZero() || (opts.DefaultBranch != "" && pr.BaseBranch != opts.DefaultBranch) {
				continue
			}
			m := mergedPR{number: pr.Number, sha: pr.MergeCommitSHA, mergedAt: pr.MergedAt.UTC().Truncate(time.Second)}
			merged = append(merged, m)
			byNumber[m.number] = m
			if m.sha != "" {
				bySHA[m.sha] = m.number
			}
		}
	}
	// PR fusionnées sans merge_commit_sha connu, par numéro croissant : repli des suites rebasées.
	slices.SortFunc(merged, func(a, b mergedPR) int { return cmp.Compare(a.number, b.number) })
	var noSHA []mergedPR
	for _, m := range merged {
		if m.sha == "" {
			noSHA = append(noSHA, m)
		}
	}

	out := make([]ArrivalLine, 0, len(shas))
	for _, sha := range shas {
		line := ArrivalLine{SHA: sha}
		c, ok := byKey[sha]
		if ok {
			line.AuthorID, line.AuthorName, line.AuthorLogin = c.AuthorID, c.AuthorName, c.AuthorLogin
		}
		switch {
		case !ok:
			if n, hit := bySHA[sha]; hit {
				line.Class, line.Reason, line.PR = ArrivalByPR, ArrivalMergeCommitSHA, n
			} else {
				line.Class, line.Reason = ArrivalUndetermined, ArrivalNotInDatabase
			}
		case opts.API:
			classifyAPI(&line, c, bySHA, byNumber, known, opts.Complete)
		default:
			classifyClone(&line, c)
		}
		out = append(out, line)
	}
	if opts.API {
		attachRebaseRuns(out, byKey, bySHA, noSHA)
	}
	return out
}

// isGitHubCommit indique un commit dont le committer est GitHub.
func isGitHubCommit(c model.ConventionCommit) bool {
	return aisignature.IsGitHubCommitter(c.CommitterLogin, c.CommitterEmails...)
}

// committerKnown indique un committer identifié (login ou adresse).
func committerKnown(c model.ConventionCommit) bool {
	return c.CommitterLogin != "" || slices.ContainsFunc(c.CommitterEmails, func(e string) bool { return e != "" })
}

// isMergeCommit indique une fusion : deux parents ou plus, ou, parents inconnus, message de fusion par
// défaut.
func isMergeCommit(c model.ConventionCommit) bool {
	if c.ParentCount != nil {
		return *c.ParentCount >= 2
	}
	return IsDefaultMergeMessage(c.Message)
}

// mergePRNumber lit le numéro de « Merge pull request #n … » ; 0 si le sujet n'a pas cette forme.
func mergePRNumber(message string) int {
	m := mergePRRe.FindStringSubmatch(Subject(message))
	if m == nil {
		return 0
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0
	}
	return n
}

// mergePRRe est le sujet d'une fusion de PR par GitHub.
var mergePRRe = regexp.MustCompile(`^Merge pull request #([0-9]+)\b`)

// classifyAPI classe un commit connu en base, avec la liste des PR de l'API. Les suites rebasées sont
// reconnues ensuite par attachRebaseRuns, à partir du classement provisoire web_edit ou unmatched.
func classifyAPI(line *ArrivalLine, c model.ConventionCommit, bySHA map[string]int, byNumber map[int]mergedPR,
	known map[int]bool, complete bool) {
	if n, ok := bySHA[c.SHA]; ok {
		line.Class, line.Reason, line.PR = ArrivalByPR, ArrivalMergeCommitSHA, n
		return
	}
	if !committerKnown(c) {
		line.Class, line.Reason = ArrivalUndetermined, ArrivalCommitterUnknown
		return
	}
	if !isGitHubCommit(c) {
		line.Class, line.Reason = ArrivalOutsidePR, ArrivalHumanCommitter
		if isMergeCommit(c) {
			line.Reason = ArrivalLocalMerge
		}
		return
	}
	if isMergeCommit(c) {
		line.Class, line.Reason, line.PR = ArrivalByPR, ArrivalGitHubMerge, mergePRNumber(c.Message)
		return
	}
	if n := squashNumber(c.Message); n != 0 {
		if m, ok := byNumber[n]; ok && m.sha == "" {
			line.Class, line.Reason, line.PR = ArrivalByPR, ArrivalSquashSuffix, n
			return
		}
		if !known[n] {
			// PR citée absente de la liste (liste en retard, reportée pour quota ou en échec) : jamais une
			// édition web.
			line.Class, line.Reason = ArrivalUndetermined, ArrivalUnmatched
			return
		}
	}
	if complete {
		line.Class, line.Reason = ArrivalOutsidePR, ArrivalWebEdit
		return
	}
	line.Class, line.Reason = ArrivalUndetermined, ArrivalUnmatched
}

// rebaseTolerance est l'écart toléré entre le committed_at du commit le plus récent d'une suite rebasée et
// le merged_at d'une PR sans merge_commit_sha connu.
const rebaseTolerance = 2 * time.Second

// isRunCommit indique un commit qui peut appartenir à une suite rebasée par GitHub : connu en base,
// committer GitHub, hors fusion, committed_at connu.
func isRunCommit(c model.ConventionCommit, ok bool) bool {
	return ok && isGitHubCommit(c) && !c.CommittedAt.IsZero() && !isMergeCommit(c)
}

// provisional indique une ligne classée provisoirement faute de PR reconnue (web_edit ou unmatched).
func provisional(l ArrivalLine) bool {
	return l.Reason == ArrivalWebEdit || l.Reason == ArrivalUnmatched
}

// attachRebaseRuns reconnaît les suites rebasées par GitHub (PR3), en partant de leur commit le plus récent
// (lines est du plus récent au plus ancien). Une tête est le merge_commit_sha d'une PR (commit de GitHub,
// hors fusion) : les commits suivants consécutifs de GitHub, hors fusions, au même committed_at (à la
// seconde) que la tête et classés provisoirement (web_edit ou unmatched) sont rattachés à sa PR, quel que
// soit l'écart avec merged_at. Repli, pour les PR sans merge_commit_sha connu : une suite de commits
// provisoires de GitHub au même committed_at est rattachée à la PR dont le merged_at est le plus proche, à
// ±2 s, du committed_at de son commit le plus récent (le plus petit numéro en cas d'égalité).
func attachRebaseRuns(lines []ArrivalLine, byKey map[string]model.ConventionCommit, bySHA map[string]int,
	noSHA []mergedPR) {
	// extend rattache à pr les lignes provisoires consécutives à partir de from, au même committed_at ;
	// renvoie l'indice qui suit la suite.
	extend := func(from int, at time.Time, pr int) int {
		j := from
		for ; j < len(lines); j++ {
			d, ok := byKey[lines[j].SHA]
			if !isRunCommit(d, ok) || !provisional(lines[j]) || !d.CommittedAt.UTC().Truncate(time.Second).Equal(at) {
				break
			}
			lines[j].Class, lines[j].Reason, lines[j].PR = ArrivalByPR, ArrivalRebaseRun, pr
		}
		return j
	}
	for i := 0; i < len(lines); {
		c, ok := byKey[lines[i].SHA]
		if !isRunCommit(c, ok) {
			i++
			continue
		}
		at := c.CommittedAt.UTC().Truncate(time.Second)
		if n, head := bySHA[lines[i].SHA]; head {
			i = extend(i+1, at, n)
			continue
		}
		if !provisional(lines[i]) {
			i++
			continue
		}
		best, gap := 0, rebaseTolerance+1
		for _, m := range noSHA {
			d := m.mergedAt.Sub(at).Abs()
			if d <= rebaseTolerance && d < gap {
				best, gap = m.number, d
			}
		}
		if best == 0 {
			i++
			continue
		}
		i = extend(i, at, best)
	}
}

// classifyClone classe un commit en clone seul, d'après les messages de GitHub.
func classifyClone(line *ArrivalLine, c model.ConventionCommit) {
	switch {
	case !committerKnown(c):
		line.Class, line.Reason = ArrivalUndetermined, ArrivalCommitterUnknown
	case !isGitHubCommit(c):
		line.Class, line.Reason = ArrivalOutsidePR, ArrivalHumanCommitter
		if isMergeCommit(c) {
			line.Reason = ArrivalLocalMerge
		}
	case mergePRNumber(c.Message) != 0:
		line.Class, line.Reason, line.PR = ArrivalByPR, ArrivalGitHubMerge, mergePRNumber(c.Message)
	case squashNumber(c.Message) != 0:
		line.Class, line.Reason, line.PR = ArrivalByPR, ArrivalSquashSuffix, squashNumber(c.Message)
	default:
		line.Class, line.Reason = ArrivalUndetermined, ArrivalCloneOnly
	}
}

// ArrivalTally compte les commits du premier parent par classement.
type ArrivalTally struct {
	ByPR         int
	OutsidePR    int
	Undetermined int
}

// Total renvoie le nombre de commits classés.
func (t ArrivalTally) Total() int { return t.ByPR + t.OutsidePR + t.Undetermined }

// TallyArrivals compte les lignes retenues par keep (nil : toutes).
func TallyArrivals(lines []ArrivalLine, keep func(ArrivalLine) bool) ArrivalTally {
	var t ArrivalTally
	for _, l := range lines {
		if keep == nil || keep(l) {
			t.add(l)
		}
	}
	return t
}

// add compte une ligne.
func (t *ArrivalTally) add(l ArrivalLine) {
	switch l.Class {
	case ArrivalByPR:
		t.ByPR++
	case ArrivalOutsidePR:
		t.OutsidePR++
	default:
		t.Undetermined++
	}
}

// IsFork indique une PR issue d'un fork (FromFork connu et vrai).
func IsFork(pr model.PullRequest) bool { return pr.FromFork != nil && *pr.FromFork }

// ReusedBranch est une branche de tête réutilisée (BR2) : plus d'une PR fusionnée vers la branche par
// défaut.
type ReusedBranch struct {
	Name string
	PRs  []int // PR fusionnées depuis cette branche, numéro croissant
}

// ReusedBranches calcule BR2 : branches de tête (hors forks, hors auteurs bots, nom non vide) dont plus
// d'une PR a été fusionnée vers defaultBranch ; par nom croissant. Vide non nil si aucune. Une PR dont
// l'origine (fork ou non) est inconnue est comptée.
func ReusedBranches(pulls []model.ConventionPullRequest, defaultBranch string) []ReusedBranch {
	byName := map[string][]int{}
	for _, pr := range pulls {
		if !pr.Merged || pr.HeadBranch == "" || pr.BaseBranch != defaultBranch || IsFork(pr.PullRequest) ||
			aisignature.IsBot(pr.AuthorLogin) {
			continue
		}
		byName[pr.HeadBranch] = append(byName[pr.HeadBranch], pr.Number)
	}
	out := []ReusedBranch{}
	for name, prs := range byName {
		if len(prs) > 1 {
			slices.Sort(prs)
			out = append(out, ReusedBranch{Name: name, PRs: prs})
		}
	}
	slices.SortFunc(out, func(a, b ReusedBranch) int { return cmp.Compare(a.Name, b.Name) })
	return out
}

// MergeMethodsResult est BR5 : les méthodes de fusion autorisées sur le dépôt.
type MergeMethodsResult struct {
	Known   bool // les trois réglages sont connus (renvoyés à un compte qui a le droit push)
	Squash  bool // allow_squash_merge
	Merge   bool // allow_merge_commit
	Rebase  bool // allow_rebase_merge
	Conform bool // seul le squash est autorisé
}

// CheckMergeMethods évalue BR5 ; nil = inconnu. Un seul réglage inconnu rend le tout inconnu (GitHub les
// renvoie ensemble).
func CheckMergeMethods(squash, merge, rebase *bool) MergeMethodsResult {
	if squash == nil || merge == nil || rebase == nil {
		return MergeMethodsResult{}
	}
	out := MergeMethodsResult{Known: true, Squash: *squash, Merge: *merge, Rebase: *rebase}
	out.Conform = out.Squash && !out.Merge && !out.Rebase
	return out
}
