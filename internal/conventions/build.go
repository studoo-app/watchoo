package conventions

// Lecture des commits et rapport des conventions (phase 8.1) : Build lit les commits d'un dépôt dans la
// base (Reader, implémenté par *store.Store) et, quand le clone Git local existe, leur périmètre précis
// (CloneReader, implémenté par le provider des clones, lu sans mise à jour), puis appelle Aggregate. Le
// calcul est fait à la lecture : rien n'est stocké. Le message n'est lu qu'en mémoire ; aucun résultat ne
// le recopie et rien n'est journalisé.

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
}

// CloneReader lit les clones Git locaux existants, sans les mettre à jour ; le provider des clones
// (internal/provider/git) l'implémente.
type CloneReader interface {
	HasClone(owner, name string) bool
	// RevList renvoie les SHA des commits accessibles depuis les branches origin/<branche> données.
	RevList(ctx context.Context, owner, name string, branches []string) ([]string, error)
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
	Location         *time.Location // fuseau de l'espace (lecture de la date de début) ; nil : UTC
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
}

// ContributorReport est le taux CC3 d'un contributeur, tous dépôts du rapport confondus.
type ContributorReport struct {
	AuthorID    int64 // 0 : auteur inconnu
	AuthorName  string
	AuthorLogin string
	Tally
}

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
	authors := map[int64]*ContributorReport{}
	for _, repo := range repos {
		if f.RepoID != 0 && repo.ID != f.RepoID {
			continue
		}
		rep, err := buildRepo(ctx, r, clones, repo, f.AuthorIDs, opts)
		if err != nil {
			return out, err
		}
		out.Total.Merge(rep.Tally)
		for _, c := range rep.Commits {
			if c.BeforeSince {
				continue
			}
			a := authors[c.AuthorID]
			if a == nil {
				a = &ContributorReport{AuthorID: c.AuthorID, AuthorName: c.AuthorName, AuthorLogin: c.AuthorLogin, Tally: newTally()}
				authors[c.AuthorID] = a
			}
			a.add(CommitResult{SHA: c.SHA, Excluded: c.Excluded, Codes: c.Codes, Conform: c.Conform})
		}
		out.Repos = append(out.Repos, rep)
	}
	for _, a := range authors { // chacun a au moins un commit évalué ou exclu
		a.sortIssues()
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
	opts BuildOptions) (RepoReport, error) {
	rep := RepoReport{Repo: repo, Source: SourceDatabase, Tally: newTally(), DefaultBranch: newTally(), Commits: []CommitLine{}}
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
	if err != nil || len(rows) == 0 {
		return rep, err
	}

	inScope := func(c model.ConventionCommit) bool { return c.OnTracked }
	onDefault := func(c model.ConventionCommit) bool { return c.OnDefault }
	if all, def, ok, err := cloneScope(ctx, r, clones, repo); err != nil {
		return rep, err
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
		rep.Commits = append(rep.Commits, CommitLine{
			SHA: c.SHA, AuthorID: row.AuthorID, AuthorName: row.AuthorName, AuthorLogin: row.AuthorLogin,
			AuthoredAt: row.AuthoredAt, BeforeSince: c.BeforeSince, Excluded: c.Excluded, Codes: c.Codes,
			Conform: c.Conform,
		})
	}
	return rep, nil
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
