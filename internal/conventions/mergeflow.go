package conventions

// Indicateurs de la phase 8.5 dans le rapport (Build) et la revue de jalon : commits arrivés sur la
// branche par défaut hors PR (PR3, premier parent lu dans le clone existant, FirstParentReader) et
// méthodes de fusion autorisées (BR5, lues en base). PR5, PR6 (buildPulls) et BR2 (buildExtras) sont
// calculés avec les PR. Rien n'est stocké ; aucun message, SHA ni nom n'est journalisé.

import (
	"context"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// FirstParentReader lit le premier parent dans le clone Git local existant, sans le mettre à jour (PR3,
// phase 8.5) ; le provider des clones l'implémente. Facultatif : un CloneReader qui ne l'implémente pas
// rend PR3 inconnu, comme sans clone.
type FirstParentReader interface {
	// FirstParent renvoie les commits du premier parent, du plus récent au plus ancien, sans le commit
	// racine : depuis la branche branch du distant (non vide) ou le commit head, moins les commits
	// accessibles depuis exclude, committés depuis since et jusqu'à until (zéro : sans borne). Une branche
	// ou un commit absent du clone est une erreur.
	FirstParent(ctx context.Context, owner, name, branch, head string, exclude []string, since, until time.Time) ([]string, error)
}

// UnknownNoPushAccess : méthodes de fusion non renvoyées par la forge (compte sans droit push, ou dépôt
// pas encore synchronisé depuis la phase 8.5) ; BR5 inconnu.
const UnknownNoPushAccess = "no_push_access"

// UnknownUndetermined : revue de jalon, PR3 sans commit hors PR mais avec des commits dont l'arrivée
// n'a pas pu être déterminée.
const UnknownUndetermined = "undetermined"

// AllArrivalReasons liste les raisons du classement PR3 dans leur ordre d'affichage : par PR, hors PR,
// indéterminé.
var AllArrivalReasons = []string{
	ArrivalMergeCommitSHA, ArrivalRebaseRun, ArrivalGitHubMerge, ArrivalSquashSuffix,
	ArrivalHumanCommitter, ArrivalLocalMerge, ArrivalWebEdit,
	ArrivalNotInDatabase, ArrivalCommitterUnknown, ArrivalUnmatched, ArrivalCloneOnly,
}

// ArrivalReport est PR3 d'un dépôt : commits du premier parent de la branche par défaut depuis la date de
// début, sans le commit racine, classés par PR, hors PR ou indéterminés. Rapport restreint à des
// contributeurs : seuls leurs commits (par l'auteur) sont gardés et comptés.
type ArrivalReport struct {
	Known  bool
	Reason string // UnknownNoClone ou UnknownCloneUnreadable si inconnu
	// API indique un classement avec les PR de l'API ; faux : clone seul, repli sur les messages de
	// GitHub (dégradé).
	API bool
	// Complete indique une liste des PR relue en entier (repositories.conventions_prs_version) : un
	// commit de GitHub sans PR reconnue est alors une édition web.
	Complete bool
	ArrivalTally
	Reasons map[string]int // commits par raison (toutes les clés d'AllArrivalReasons)
	Lines   []ArrivalLine  // commits classés, du plus récent au plus ancien ; vide non nil si aucun
}

// Outside renvoie les commits arrivés hors PR, du plus récent au plus ancien.
func (a ArrivalReport) Outside() []ArrivalLine {
	out := []ArrivalLine{}
	for _, l := range a.Lines {
		if l.Class == ArrivalOutsidePR {
			out = append(out, l)
		}
	}
	return out
}

// MergeMethodsReport est BR5 d'un dépôt.
type MergeMethodsReport struct {
	MergeMethodsResult
	Reason string // UnknownNoAPI ou UnknownNoPushAccess si inconnu
}

// buildMergeMethods calcule BR5 depuis les réglages du dépôt écrits par la synchro.
func buildMergeMethods(repo model.Repository, opts BuildOptions) MergeMethodsReport {
	if !pullsKnown(repo, opts) {
		return MergeMethodsReport{Reason: UnknownNoAPI}
	}
	out := MergeMethodsReport{MergeMethodsResult: CheckMergeMethods(repo.AllowSquashMerge, repo.AllowMergeCommit,
		repo.AllowRebaseMerge)}
	if !out.Known {
		out.Reason = UnknownNoPushAccess
	}
	return out
}

// commitIndex indexe par SHA les commits rows lus en base. Un rapport restreint à des contributeurs
// (filtered) ne lit que leurs commits : si le commit de fusion d'une de leurs PR fusionnées (pulls) en est
// absent, tous les commits du dépôt sont relus (PR6 porte sur la PR, quel que soit l'auteur du commit).
func commitIndex(ctx context.Context, r Reader, repo model.Repository, rows []model.ConventionCommit, filtered bool,
	pulls []model.ConventionPullRequest) (map[string]model.ConventionCommit, error) {
	idx := make(map[string]model.ConventionCommit, len(rows))
	for _, row := range rows {
		idx[row.SHA] = row
	}
	if !filtered {
		return idx, nil
	}
	missing := slices.ContainsFunc(pulls, func(pr model.ConventionPullRequest) bool {
		_, ok := idx[pr.MergeCommitSHA]
		return pr.Merged && pr.MergeCommitSHA != "" && !ok
	})
	if !missing {
		return idx, nil
	}
	all, err := r.ConventionCommits(ctx, repo.ID, 0)
	if err != nil {
		return nil, err
	}
	for _, row := range all {
		idx[row.SHA] = row
	}
	return idx, nil
}

// firstParentOf renvoie la lecture du premier parent du clone du dépôt ; nil sans clone ou si clones ne
// sait pas la faire.
func firstParentOf(clones CloneReader, repo model.Repository) FirstParentReader {
	if !hasClone(clones, repo) {
		return nil
	}
	fp, _ := clones.(FirstParentReader)
	return fp
}

// arrivalRange borne la lecture du premier parent : depuis la branche par défaut (head vide) ou le commit
// head, moins exclude, entre since et until (zéro : sans borne).
type arrivalRange struct {
	head         string
	exclude      []string
	since, until time.Time
}

// buildArrivals calcule PR3 sur le premier parent borné par rg. pulls sont toutes les PR du dépôt en base
// (lues par l'API ; aucune en clone seul), commits les commits en base par SHA. authorIDs (nil : tous)
// restreint les lignes gardées aux commits de ces auteurs. Un clone absent ou illisible rend PR3 inconnu ;
// seul un contexte terminé est une erreur.
func buildArrivals(ctx context.Context, clones CloneReader, repo model.Repository, authorIDs []int64,
	pulls []model.ConventionPullRequest, commits map[string]model.ConventionCommit, rg arrivalRange,
	opts BuildOptions) (ArrivalReport, error) {
	out := ArrivalReport{Reasons: map[string]int{}, Lines: []ArrivalLine{}}
	for _, k := range AllArrivalReasons {
		out.Reasons[k] = 0
	}
	fp := firstParentOf(clones, repo)
	if fp == nil {
		out.Reason = UnknownNoClone
		return out, nil
	}
	branch := ""
	if rg.head == "" {
		if repo.DefaultBranch == "" {
			out.Reason = UnknownCloneUnreadable
			return out, nil
		}
		branch = repo.DefaultBranch
	}
	shas, err := fp.FirstParent(ctx, repo.Owner, repo.Name, branch, rg.head, rg.exclude, rg.since, rg.until)
	if err := ctx.Err(); err != nil {
		return out, err
	}
	if err != nil {
		out.Reason = UnknownCloneUnreadable
		return out, nil
	}
	out.Known = true
	out.API = pullsKnown(repo, opts)
	out.Complete = out.API && repo.ConventionsPRsVersion >= PRRulesVersion
	lines := ClassifyFirstParent(shas, commits, pulls, FirstParentOptions{DefaultBranch: repo.DefaultBranch,
		API: out.API, Complete: out.Complete})
	for _, l := range lines {
		if authorIDs != nil && (l.AuthorID == 0 || !slices.Contains(authorIDs, l.AuthorID)) {
			continue
		}
		out.Lines = append(out.Lines, l)
		out.Reasons[l.Reason]++
	}
	out.ArrivalTally = TallyArrivals(out.Lines, nil)
	return out, nil
}

// arrivalCriterion juge PR3 = 0 sur l'intervalle d'un jalon : non conforme dès qu'un commit est arrivé
// hors PR ; inconnu si PR3 l'est, sans commit, ou avec des commits indéterminés ; conforme sinon.
func arrivalCriterion(a ArrivalReport) Criterion {
	c := Criterion{ID: CriterionPR3, Status: CriterionConform}
	switch {
	case !a.Known:
		c.Status, c.Reason = CriterionUnknown, a.Reason
	case a.OutsidePR > 0:
		c.Status = CriterionNonconform
	case a.Total() == 0:
		c.Status, c.Reason = CriterionUnknown, UnknownNoCommits
	case a.Undetermined > 0:
		c.Status, c.Reason = CriterionUnknown, UnknownUndetermined
	}
	return c
}
