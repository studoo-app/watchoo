package conventions

// Lecture et rapport des conventions (phases 8.1 à 8.5) : Build lit les commits, pull requests, tags et
// releases d'un dépôt dans la base (Reader, implémenté par *store.Store) et, quand le clone Git local
// existe, le périmètre précis des commits, les branches du distant et les intervalles entre tags
// (CloneReader, implémenté par le provider des clones, lu sans mise à jour), puis calcule les indicateurs.
// Le calcul est fait à la lecture : rien n'est stocké, sauf les critères de fichiers (phase 8.3), mis en
// cache par la synchro et seulement relus ici. Le message de commit et le titre de PR ne sont lus
// qu'en mémoire ; aucun résultat ne les recopie et rien n'est journalisé.

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
)

// Sources du périmètre d'un dépôt (clé source du --json).
const (
	// SourceClone : commits accessibles depuis les branches suivies du clone local (git rev-list),
	// chaque commit une fois.
	SourceClone = "clone"
	// SourceDatabase : repli sans clone (ou clone illisible), commits rattachés en base à une branche
	// suivie (commit_branches) ; peut inclure des commits réécrits, ces rattachements n'étant jamais
	// retirés.
	SourceDatabase = "database"
)

// Origines de la date de début d'un dépôt (clé since_origin du --json).
const (
	SinceOriginWorkspace  = "workspace"  // conventions.since de l'espace
	SinceOriginRepository = "repository" // date propre au dépôt (repositories.conventions_since)
)

// Reader lit la base de l'espace ; *store.Store l'implémente.
type Reader interface {
	ListRepositories(ctx context.Context) ([]model.Repository, error)
	ListBranches(ctx context.Context, repoID int64) ([]model.Branch, error)
	// ConventionCommits renvoie les commits du dépôt repoID de l'auteur authorID (0 = tous).
	ConventionCommits(ctx context.Context, repoID, authorID int64) ([]model.ConventionCommit, error)
	// ConventionPullRequests renvoie les pull requests du dépôt, avec le contributeur de leur auteur.
	ConventionPullRequests(ctx context.Context, repoID int64) ([]model.ConventionPullRequest, error)
	// ConventionTags renvoie les tags du dépôt (nom, SHA du commit visé).
	ConventionTags(ctx context.Context, repoID int64) ([]model.Tag, error)
	// ConventionReleases renvoie les releases du dépôt, brouillons compris.
	ConventionReleases(ctx context.Context, repoID int64) ([]model.Release, error)
	// RepositoryConventions renvoie les résultats en cache des critères de fichiers du dépôt (phase 8.3),
	// écrits par la synchro.
	RepositoryConventions(ctx context.Context, repoID int64) ([]model.RepositoryConventions, error)
	// TagObservations renvoie les périodes d'observation des tags du dépôt (phase 8.4, SV3 et échéances
	// de SV2), écrites par la synchro.
	TagObservations(ctx context.Context, repoID int64) ([]model.TagObservation, error)
	// BranchRewrites renvoie les réécritures observées de la branche par défaut du dépôt (phase 8.4, BR4).
	BranchRewrites(ctx context.Context, repoID int64) ([]model.BranchRewrite, error)
	// ConventionTagDates renvoie la date du commit visé de chaque tag du dépôt dont le commit est en base
	// (phase 8.4, borne basse de « posé entre X et Y »).
	ConventionTagDates(ctx context.Context, repoID int64) (map[string]time.Time, error)
}

// CloneReader lit les clones Git locaux existants, sans les mettre à jour ; le provider des clones
// (internal/provider/git) l'implémente.
type CloneReader interface {
	HasClone(owner, name string) bool
	// RevList renvoie les SHA des commits accessibles depuis les branches origin/<branche> données.
	RevList(ctx context.Context, owner, name string, branches []string) ([]string, error)
	// RemoteBranches renvoie les noms des branches du distant lues au dernier fetch (sans origin/HEAD).
	RemoteBranches(ctx context.Context, owner, name string) ([]string, error)
	// RevRange renvoie les SHA des commits accessibles depuis les branches origin/<branche> et les
	// commits include, mais pas depuis les commits exclude ; un commit include ou exclude absent du clone
	// est une erreur.
	RevRange(ctx context.Context, owner, name string, branches, include, exclude []string) ([]string, error)
}

// Filter restreint le rapport.
type Filter struct {
	RepoID int64 // 0 : tous les dépôts
	// AuthorIDs limite aux commits de ces contributeurs ; nil : tous les auteurs ; vide non nil : aucun.
	AuthorIDs []int64
}

// BuildOptions règlent Build : réglages conventions.* de l'espace (workspace.ConventionsConfig).
type BuildOptions struct {
	Rules            Rules
	Since            string         // conventions.since de l'espace, AAAA-MM-JJ ; vide : depuis le début
	ExcludedAccounts []string       // conventions.excluded_accounts
	Location         *time.Location // fuseau de l'espace (date de début, échéances) ; nil : UTC
	Branch           BranchRules    // conventions.commit.types et conventions.branch.* (BR1)
	Milestones       []Milestone    // conventions.tags.milestones (SV2, SV6, revue de jalon)
	PrereleaseForms  []string       // conventions.tags.prerelease_forms (SV4) ; vide : DefaultPrereleaseForms
	Language         string         // conventions.commit.language : fr, en ou any (CC7, information)
	Files            FileRules      // conventions.changelog.*, readme.* et docs.* (CL, RD, DOC, phase 8.3)
	PRBody           PRBodyRules    // conventions.pr.* (PR5, phase 8.5)
	Review           ReviewRules    // conventions.pr.min_reviews et reviewer_must_contribute (PR4, phase 8.6)
	// LocalOnly indique un espace synchronisé sans l'API (clone seul) : les pull requests et releases en
	// base ne sont plus mises à jour, PR1, PR2 et SV6 sont « inconnus ».
	LocalOnly bool
	// FileDetail demande la vue détaillée d'un dépôt (Filter.RepoID non nul) : les listes de chemins des
	// critères de fichiers (DOC3, DOC4, DOC5, DOC8) sont recalculées depuis le clone, sans être stockées.
	// Sans effet sur un rapport de tous les dépôts.
	FileDetail bool
}

// CommitLine est le résultat d'un commit du périmètre, sans son message.
type CommitLine struct {
	SHA         string
	AuthorID    int64 // 0 : inconnu
	AuthorName  string
	AuthorLogin string
	AuthoredAt  time.Time
	BeforeSince bool     // antérieur à la date de début : ni évalué ni exclu
	Excluded    string   // motif CC4 ; vide si évalué ou antérieur à la date de début
	Codes       []string // codes CC2 ; vide si non évalué
	Conform     bool     // évalué et sans écart bloquant
	Traits      Traits   // CC5 à CC7 (informatifs) ; zéro si non évalué
	// TrendKnown indique que la position du commit par rapport au dernier tag final est connue (clone
	// lisible, au moins un tag final) ; SinceLastTag : le commit n'est pas atteignable depuis ce tag.
	TrendKnown   bool
	SinceLastTag bool
}

// Evaluated indique un commit compté au dénominateur.
func (c CommitLine) Evaluated() bool { return !c.BeforeSince && c.Excluded == "" }

// RepoReport est le rapport d'un dépôt.
type RepoReport struct {
	Repo   model.Repository
	Source string // SourceClone ou SourceDatabase
	// Since est l'instant de début effectif (zéro : depuis le début) ; SinceDate sa date du calendrier
	// (AAAA-MM-JJ, vide sans date) et SinceOrigin son origine (vide sans date).
	Since          time.Time
	SinceDate      string
	SinceOrigin    string
	Tally              // CC1, CC2 par motif et CC4, toutes branches suivies
	BeforeSince    int // commits antérieurs à la date de début, comptés à part
	ParentsUnknown int // commits comptés au nombre de parents inconnu (fusions lues sur le message)
	// DefaultBranch est la vue informative de la seule branche par défaut (Repo.DefaultBranch).
	DefaultBranch Tally
	Commits       []CommitLine // commits du périmètre, du plus récent au plus ancien

	// Phase 8.2.
	Traits    TraitsTally     // CC5 à CC7 des commits évalués du périmètre (informatifs)
	Language  LanguageSummary // CC7 : langues des sujets et mélange
	Branches  BranchReport    // BR1
	Pulls     PullReport      // PR1, PR2
	Tags      TagReport       // SV1, SV2 (présence), SV6 (information)
	Intervals IntervalReport  // CC1 par intervalle entre tags finals, et depuis le dernier

	// Phase 8.3.
	Files FileReport // CL, RD, DOC et TL sur la tête de la branche par défaut (cache de la synchro)

	// Phase 8.4 (échéances de SV2 : Tags.Milestones[i].Deadline).
	Rewrites    RewriteReport      // BR4 : réécritures observées de la branche par défaut
	TagHistory  TagHistoryReport   // SV3 : tags réécrits ou supprimés
	Prereleases PrereleaseReport   // SV4 : formes et pose des pré-versions (avertissements)
	Bumps       BumpReport         // SV5 : saut de version cohérent avec les commits
	Precedence  PrecedenceReport   // SV7 : précédence et historique
	Freshness   ChangelogFreshness // CL10 : fraîcheur du CHANGELOG (information)

	// Phase 8.5 (PR5 et PR6 : Pulls ; BR2 : Branches.Reused).
	Arrivals     ArrivalReport      // PR3 : commits arrivés sur la branche par défaut hors PR
	MergeMethods MergeMethodsReport // BR5 : méthodes de fusion autorisées

	// Phase 8.7.
	Leads LeadReport // pistes : invitent à relire, ne jugent pas ; hors de toute grille et de tout taux
}

// ContributorReport est le taux CC3 d'un contributeur, tous dépôts du rapport confondus.
type ContributorReport struct {
	AuthorID    int64 // 0 : auteur inconnu
	AuthorName  string
	AuthorLogin string
	Tally

	// Phase 8.2.
	Traits   TraitsTally // CC5 à CC7 de ses commits évalués (CC6 par contributeur)
	Trend    Trend       // CC3 avant et depuis le dernier tag final de chaque dépôt
	Pulls    PRTally     // PR1, PR2, PR5 et PR6 des PR dont il est l'auteur (rapprochées par le compte de forge)
	Branches BranchTally // BR1 des branches de tête de ses PR

	// Phase 8.5.
	Arrivals ArrivalTally // PR3 : commits du premier parent de la branche par défaut dont il est l'auteur
}

// Trend est la tendance de CC3 : deux taux, avant et depuis le dernier tag final de chaque dépôt, sans
// flèche calculée. Un commit n'est placé que si son dépôt a un clone lisible et au moins un tag final.
type Trend struct {
	Before     Tally // commits atteignables depuis le dernier tag final de leur dépôt
	Since      Tally // commits arrivés depuis (non atteignables depuis ce tag)
	Repos      int   // dépôts où la position a pu être lue
	Unmeasured int   // commits évalués de dépôts sans clone lisible ni tag final : non placés
}

// Known indique une tendance mesurée dans au moins un dépôt.
func (t Trend) Known() bool { return t.Repos > 0 }

// BuildReport est le rapport des conventions.
type BuildReport struct {
	RulesVersion int
	Repos        []RepoReport        // dans l'ordre de ListRepositories
	Contributors []ContributorReport // par nom, login puis identifiant (aucun classement)
	Total        Tally               // tous dépôts confondus
}

// Build calcule le rapport des conventions des dépôts qui satisfont f. clones peut être nil (aucun
// clone : repli sur la base pour tous les dépôts). Un clone illisible se replie sur la base ; seules une
// erreur de base, une date de début invalide ou un contexte terminé arrêtent le calcul.
func Build(ctx context.Context, r Reader, clones CloneReader, f Filter, opts BuildOptions) (BuildReport, error) {
	out := BuildReport{RulesVersion: RulesVersion, Repos: []RepoReport{}, Contributors: []ContributorReport{}, Total: newTally()}
	repos, err := r.ListRepositories(ctx)
	if err != nil {
		return out, err
	}
	if f.RepoID == 0 {
		opts.FileDetail = false
	}
	env, err := newBuildEnv(opts)
	if err != nil {
		return out, err
	}
	authors := map[int64]*ContributorReport{}
	author := func(id int64, name, login string) *ContributorReport {
		a := authors[id]
		if a == nil {
			a = &ContributorReport{AuthorID: id, AuthorName: name, AuthorLogin: login, Tally: newTally(),
				Traits: NewTraitsTally(), Trend: Trend{Before: newTally(), Since: newTally()}}
			authors[id] = a
		}
		return a
	}
	allowed := func(id int64) bool { return id != 0 && (f.AuthorIDs == nil || slices.Contains(f.AuthorIDs, id)) }
	for _, repo := range repos {
		if f.RepoID != 0 && repo.ID != f.RepoID {
			continue
		}
		rep, err := buildRepo(ctx, r, clones, repo, f.AuthorIDs, opts, env)
		if err != nil {
			return out, err
		}
		out.Total.Merge(rep.Tally)
		measured := map[int64]bool{}
		for _, c := range rep.Commits {
			if c.BeforeSince {
				continue
			}
			a := author(c.AuthorID, c.AuthorName, c.AuthorLogin)
			res := CommitResult{SHA: c.SHA, Excluded: c.Excluded, Codes: c.Codes, Conform: c.Conform}
			a.add(res)
			if !c.Evaluated() {
				continue
			}
			a.Traits.Add(c.Traits)
			switch {
			case !c.TrendKnown:
				a.Trend.Unmeasured++
			case c.SinceLastTag:
				a.Trend.Since.add(res)
				measured[c.AuthorID] = true
			default:
				a.Trend.Before.add(res)
				measured[c.AuthorID] = true
			}
		}
		for id := range measured {
			authors[id].Trend.Repos++
		}
		for _, p := range rep.Pulls.Pulls {
			if allowed(p.AuthorID) {
				author(p.AuthorID, p.AuthorName, p.AuthorLogin).Pulls.add(p)
			}
		}
		for _, b := range rep.Branches.Branches {
			for i, id := range b.AuthorIDs {
				if allowed(id) {
					author(id, b.AuthorNames[i], b.AuthorLogins[i]).Branches.add(b.BranchResult)
				}
			}
		}
		for _, l := range rep.Arrivals.Lines {
			if allowed(l.AuthorID) {
				author(l.AuthorID, l.AuthorName, l.AuthorLogin).Arrivals.add(l)
			}
		}
		out.Repos = append(out.Repos, rep)
	}
	for _, a := range authors { // chacun a un commit évalué ou exclu, une PR ou une branche
		a.sortIssues()
		a.Trend.Before.sortIssues()
		a.Trend.Since.sortIssues()
		out.Contributors = append(out.Contributors, *a)
	}
	slices.SortFunc(out.Contributors, func(a, b ContributorReport) int {
		return cmp.Or(
			cmp.Compare(strings.ToLower(a.AuthorName), strings.ToLower(b.AuthorName)),
			cmp.Compare(strings.ToLower(a.AuthorLogin), strings.ToLower(b.AuthorLogin)),
			cmp.Compare(a.AuthorID, b.AuthorID))
	})
	out.Total.sortIssues()
	return out, nil
}

// buildRepo calcule le rapport d'un dépôt.
func buildRepo(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, authorIDs []int64,
	opts BuildOptions, env *buildEnv) (RepoReport, error) {
	rep := RepoReport{Repo: repo, Source: SourceDatabase, Tally: newTally(), DefaultBranch: newTally(), Commits: []CommitLine{},
		Traits: NewTraitsTally()}
	since, err := EffectiveSinceAt(opts.Since, repo.ConventionsSince, opts.Location)
	if err != nil {
		return rep, fmt.Errorf("%s: %w", repo.FullName(), err)
	}
	rep.Since = since
	if !since.IsZero() {
		rep.SinceDate = since.Format(SinceInputLayout)
		rep.SinceOrigin = SinceOriginWorkspace
		if strings.TrimSpace(repo.ConventionsSince) != "" {
			rep.SinceOrigin = SinceOriginRepository
		}
	}
	rows, err := readCommits(ctx, r, repo.ID, authorIDs)
	if err != nil {
		return rep, err
	}
	results, err := buildCommits(ctx, r, clones, repo, rows, opts, &rep)
	if err != nil {
		return rep, err
	}
	rep.Language = rep.Traits.LanguageSummary(opts.Language)
	if err := buildExtras(ctx, r, clones, repo, authorIDs, rows, results, opts, env, &rep); err != nil {
		return rep, err
	}
	return rep, nil
}

// buildCommits calcule CC1 à CC7 sur les commits du périmètre (rows lus en base) et renvoie leurs
// résultats, dans l'ordre de rep.Commits.
func buildCommits(ctx context.Context, r Reader, clones CloneReader, repo model.Repository,
	rows []model.ConventionCommit, opts BuildOptions, rep *RepoReport) ([]CommitResult, error) {
	if len(rows) == 0 {
		return nil, nil
	}
	since := rep.Since
	inScope := func(c model.ConventionCommit) bool { return c.OnTracked }
	onDefault := func(c model.ConventionCommit) bool { return c.OnDefault }
	if all, def, ok, err := cloneScope(ctx, r, clones, repo); err != nil {
		return nil, err
	} else if ok {
		rep.Source = SourceClone
		inScope = func(c model.ConventionCommit) bool { return all[c.SHA] }
		onDefault = func(c model.ConventionCommit) bool { return def[c.SHA] }
	}

	var commits, defaults []Commit
	byKey := map[string]model.ConventionCommit{}
	for _, row := range rows {
		if !inScope(row) {
			continue
		}
		c := toCommit(row)
		commits = append(commits, c)
		byKey[row.SHA] = row
		if onDefault(row) {
			defaults = append(defaults, c)
		}
	}
	aopts := Options{Rules: opts.Rules, Since: since, ExcludedAccounts: opts.ExcludedAccounts}
	res := Aggregate(commits, aopts)
	rep.Tally, rep.BeforeSince = res.Tally, res.BeforeSince
	rep.DefaultBranch = Aggregate(defaults, aopts).Tally
	for _, c := range res.Commits {
		row := byKey[c.SHA]
		if !c.BeforeSince && row.ParentCount == nil {
			rep.ParentsUnknown++
		}
		if c.Evaluated() {
			rep.Traits.Add(c.Traits)
		}
		rep.Commits = append(rep.Commits, CommitLine{
			SHA: c.SHA, AuthorID: row.AuthorID, AuthorName: row.AuthorName, AuthorLogin: row.AuthorLogin,
			AuthoredAt: row.AuthoredAt, BeforeSince: c.BeforeSince, Excluded: c.Excluded, Codes: c.Codes,
			Conform: c.Conform, Traits: c.Traits,
		})
	}
	return res.Commits, nil
}

// readCommits lit les commits du dépôt, de tous les auteurs (authorIDs nil) ou des seuls authorIDs.
func readCommits(ctx context.Context, r Reader, repoID int64, authorIDs []int64) ([]model.ConventionCommit, error) {
	if authorIDs == nil {
		return r.ConventionCommits(ctx, repoID, 0)
	}
	var rows []model.ConventionCommit
	for _, id := range slices.Compact(slices.Sorted(slices.Values(authorIDs))) {
		if id == 0 {
			continue
		}
		got, err := r.ConventionCommits(ctx, repoID, id)
		if err != nil {
			return nil, err
		}
		rows = append(rows, got...)
	}
	slices.SortStableFunc(rows, func(a, b model.ConventionCommit) int { return b.AuthoredAt.Compare(a.AuthoredAt) })
	return rows, nil
}

// cloneScope lit le périmètre précis dans le clone : commits des branches suivies (all) et de la branche
// par défaut (def). ok est faux sans clone ou si le clone est illisible (repli sur la base) ; erreur
// seulement pour la base ou un contexte terminé.
func cloneScope(ctx context.Context, r Reader, clones CloneReader, repo model.Repository) (all, def map[string]bool, ok bool, err error) {
	if clones == nil || !clones.HasClone(repo.Owner, repo.Name) {
		return nil, nil, false, nil
	}
	branches, err := r.ListBranches(ctx, repo.ID)
	if err != nil {
		return nil, nil, false, err
	}
	var tracked []string
	for _, b := range branches {
		if b.Tracked {
			tracked = append(tracked, b.Name)
		}
	}
	if len(tracked) == 0 && repo.DefaultBranch != "" {
		tracked = []string{repo.DefaultBranch}
	}
	read := func(names []string) (map[string]bool, bool) {
		set := map[string]bool{}
		if len(names) == 0 {
			return set, true
		}
		shas, err := clones.RevList(ctx, repo.Owner, repo.Name, names)
		if err != nil {
			return nil, false
		}
		for _, s := range shas {
			set[s] = true
		}
		return set, true
	}
	all, okAll := read(tracked)
	var defNames []string
	if repo.DefaultBranch != "" {
		defNames = []string{repo.DefaultBranch}
	}
	def, okDef := read(defNames)
	if err := ctx.Err(); err != nil {
		return nil, nil, false, err
	}
	if !okAll || !okDef {
		return nil, nil, false, nil
	}
	return all, def, true, nil
}

// toCommit convertit un commit lu en base en commit à évaluer.
func toCommit(row model.ConventionCommit) Commit {
	c := Commit{
		SHA:             row.SHA,
		AuthorName:      row.AuthorName,
		AuthorLogin:     row.AuthorLogin,
		AuthorEmails:    row.AuthorEmails,
		Message:         row.Message,
		AuthoredAt:      row.AuthoredAt,
		GitHubCommitter: aisignature.IsGitHubCommitter(row.CommitterLogin, row.CommitterEmails...),
	}
	if row.AuthorID != 0 {
		c.AuthorKey = strconv.FormatInt(row.AuthorID, 10)
	}
	if row.ParentCount != nil {
		c.ParentCount, c.ParentsKnown = *row.ParentCount, true
	}
	return c
}

// Merge ajoute le décompte o à t (rapport de plusieurs dépôts) ; les écarts restent dans l'ordre
// d'AllCodes.
func (t *Tally) Merge(o Tally) {
	if t.Excluded == nil {
		t.Excluded = map[string]int{}
	}
	t.Evaluated += o.Evaluated
	t.Conform += o.Conform
	for k, v := range o.Excluded {
		t.Excluded[k] += v
	}
	for _, is := range o.Issues {
		i := slices.IndexFunc(t.Issues, func(x Issue) bool { return x.Code == is.Code })
		if i < 0 {
			t.Issues = append(t.Issues, Issue{Code: is.Code, Severity: is.Severity, SHAs: []string{}})
			i = len(t.Issues) - 1
		}
		t.Issues[i].Count += is.Count
		t.Issues[i].SHAs = append(t.Issues[i].SHAs, is.SHAs...)
	}
	t.sortIssues()
}
