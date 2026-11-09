package conventions

// Lecture des critères de fichiers (phase 8.3) par Build et ReviewMilestone : le résultat mis en cache par
// la synchro (repository_conventions) est relu, jamais recalculé ici ; une ligne absente est « pas encore
// calculée », une ligne d'une autre révision, d'une autre version des règles ou d'une autre empreinte des
// réglages est « à recalculer ». Seules les listes de chemins (DOC3, DOC4, DOC5, DOC8) sont recalculées
// depuis le clone, pour la vue détaillée d'un dépôt (BuildOptions.FileDetail), sans être stockées.

import (
	"context"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// FileReport est l'état des critères de fichiers d'un dépôt, mesurés sur la tête de sa branche par défaut.
type FileReport struct {
	// Known indique un résultat en cache à jour ; sinon Reason vaut UnknownNotComputed, UnknownStale,
	// UnknownNoClone ou UnknownFiltered et tous les critères de Results sont inconnus pour cette raison.
	Known      bool
	Reason     string
	SHA        string    // commit mesuré ; vide sans résultat
	ComputedAt time.Time // date du calcul ; zéro sans résultat
	Results    FileResults
	// Lists sont les listes de chemins recalculées depuis le clone (vue détaillée d'un dépôt seulement) ;
	// nil sinon, ListsReason disant pourquoi en vue détaillée.
	Lists       *FileLists
	ListsReason string
}

// Families renvoie les grilles par famille (changelog, readme, docs, tooling), dans l'ordre d'affichage.
func (r FileResults) Families() []FileFamily {
	return []FileFamily{
		{Name: FamilyChangelog, Checks: r.Changelog},
		{Name: FamilyReadme, Checks: r.Readme},
		{Name: FamilyDocs, Checks: r.Docs},
		{Name: FamilyTooling, Checks: r.Tooling},
	}
}

// FileFamily est la grille d'une famille de critères de fichiers.
type FileFamily struct {
	Name   string // FamilyChangelog…
	Checks []FileCheck
}

// Tally compte les critères conformes et inconnus de la grille.
func (f FileFamily) Tally() (conform, unknown int) { return TallyChecks(f.Checks) }

// headCriteria renvoie les critères mesurés sur la tête (CL9 ne l'est que sur le tag d'un jalon).
func headCriteria(ids []string) []string {
	return slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == CriterionCL9 })
}

// UnknownFileResults renvoie les critères de fichiers d'une révision, tous inconnus pour la raison donnée ;
// milestone : révision du tag d'un jalon (CL9 compris).
func UnknownFileResults(reason string, milestone bool) FileResults {
	cl := ChangelogCriteria
	if !milestone {
		cl = headCriteria(cl)
	}
	return FileResults{RulesVersion: FileRulesVersion, Changelog: unknownAll(cl, reason),
		Readme: unknownAll(ReadmeCriteria, reason), Docs: unknownAll(DocsCriteria, reason),
		Tooling: unknownAll(ToolingCriteria, reason)}
}

// cachedFiles cherche le résultat en cache de la révision (kind, tag) d'un dépôt et le juge : à jour
// (sha vide : la révision actuelle n'est pas connue en base, seules la version des règles et l'empreinte
// comptent), à recalculer, ou absent (pas encore calculé, ou pas de clone).
func cachedFiles(rows []model.RepositoryConventions, kind, tag, sha, hash string, hasClone bool) (FileResults,
	model.RepositoryConventions, string) {
	i := slices.IndexFunc(rows, func(r model.RepositoryConventions) bool { return r.RevKind == kind && r.TagName == tag })
	if i < 0 {
		if !hasClone {
			return FileResults{}, model.RepositoryConventions{}, UnknownNoClone
		}
		return FileResults{}, model.RepositoryConventions{}, UnknownNotComputed
	}
	row := rows[i]
	// Révision actuelle inconnue en base (branche par défaut non suivie : seules les branches suivies y
	// sont) : la synchro mesure la tête de la forge et supprime la ligne quand la forge n'a plus de
	// branche par défaut ; seules la version des règles et l'empreinte comptent alors.
	if sha == "" {
		sha = row.RevSHA
	}
	if !FileCacheFresh(row, sha, hash) {
		return FileResults{}, row, UnknownStale
	}
	res, err := DecodeFileResults(row.Results)
	if err != nil || res.RulesVersion != FileRulesVersion {
		return FileResults{}, row, UnknownStale
	}
	return res, row, ""
}

// hasClone indique un clone local du dépôt.
func hasClone(clones CloneReader, repo model.Repository) bool {
	return clones != nil && clones.HasClone(repo.Owner, repo.Name)
}

// buildFiles lit les critères de fichiers du dépôt en cache (tête de la branche par défaut). Rapport
// restreint à des contributeurs : non lus (indicateurs de dépôt, attribués à personne). Vue détaillée :
// recalcule les listes de chemins depuis le clone, à la révision du résultat en cache.
func buildFiles(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, filtered bool,
	tags []model.Tag, opts BuildOptions) (FileReport, error) {
	if filtered {
		return FileReport{Reason: UnknownFiltered, Results: UnknownFileResults(UnknownFiltered, false)}, nil
	}
	rows, err := r.RepositoryConventions(ctx, repo.ID)
	if err != nil {
		return FileReport{}, err
	}
	head, err := defaultHeadSHA(ctx, r, repo)
	if err != nil {
		return FileReport{}, err
	}
	cloned := hasClone(clones, repo)
	res, row, reason := cachedFiles(rows, model.ConventionRevHead, "", head, FileConfigHash(opts.Files, repo.FullName(), tags), cloned)
	if reason != "" {
		out := FileReport{Reason: reason, Results: UnknownFileResults(reason, false)}
		if opts.FileDetail {
			out.ListsReason = reason
		}
		return out, nil
	}
	out := FileReport{Known: true, SHA: row.RevSHA, ComputedAt: row.ComputedAt, Results: res}
	if !opts.FileDetail {
		return out, nil
	}
	fr, ok := clones.(FileReader)
	if !cloned || !ok {
		out.ListsReason = UnknownNoClone
		return out, nil
	}
	tree, err := ReadFileTree(ctx, fr, repo.Owner, repo.Name, row.RevSHA, opts.Files)
	if cerr := ctx.Err(); cerr != nil {
		return FileReport{}, cerr
	}
	switch {
	case err != nil:
		out.ListsReason = UnknownCloneUnreadable
	case tree.Truncated:
		out.ListsReason = UnknownTreeTruncated
	default:
		_, lists := CheckDocs(tree, FileOptions{Rules: opts.Files, Repo: repo.FullName(), Tags: repoTagNames(tags)})
		out.Lists = &lists
	}
	return out, nil
}

// defaultHeadSHA renvoie le dernier commit connu de la branche par défaut du dépôt ; vide s'il ne l'est pas.
func defaultHeadSHA(ctx context.Context, r Reader, repo model.Repository) (string, error) {
	if repo.DefaultBranch == "" {
		return "", nil
	}
	branches, err := r.ListBranches(ctx, repo.ID)
	if err != nil {
		return "", err
	}
	for _, b := range branches {
		if b.Name == repo.DefaultBranch {
			return b.HeadSHA, nil
		}
	}
	return "", nil
}

// Critères de fichiers de la revue de jalon (phase 8.3), mesurés sur l'arbre du tag du jalon : CHANGELOG à
// jour au jalon (CL9, CL8), README avec installation (RD1, section installation de RD2, RD3), INDEX
// exhaustif (DOC1, DOC3). Dans la revue, RD2 ne juge que la section installation ; les autres sections
// sont données en information (MilestoneFiles).
var milestoneFileCriteria = []string{CriterionCL9, CriterionCL8, CriterionRD1, CriterionRD2, CriterionRD3,
	CriterionDOC1, CriterionDOC3}

// MilestoneFiles est l'état des critères de fichiers sur l'arbre du tag d'un jalon.
type MilestoneFiles struct {
	// Known indique un résultat en cache à jour pour le tag ; sinon Reason dit pourquoi (tag absent :
	// raison de SV1 ; pas encore calculé ; à recalculer ; pas de clone).
	Known      bool
	Reason     string
	SHA        string
	ComputedAt time.Time
	// ReadmeExpected et ReadmeFound sont les sections du README attendues et trouvées au tag (clés
	// d'AllReadmeSections), en information ; vides si RD2 n'a pas été mesuré.
	ReadmeExpected []string
	ReadmeFound    []string
}

// reviewFiles calcule les critères de fichiers de la revue d'un jalon. sv1Reason est la raison de SV1
// quand le tag du jalon est absent.
func reviewFiles(ctx context.Context, r Reader, clones CloneReader, repo model.Repository, ms MilestoneResult,
	tags []model.Tag, sv1Reason string, opts BuildOptions) ([]Criterion, *MilestoneFiles, error) {
	files := &MilestoneFiles{ReadmeExpected: []string{}, ReadmeFound: []string{}}
	unknown := func(reason string) ([]Criterion, *MilestoneFiles, error) {
		files.Reason = reason
		out := make([]Criterion, len(milestoneFileCriteria))
		for i, id := range milestoneFileCriteria {
			out[i] = Criterion{ID: id, Status: CriterionUnknown, Reason: reason}
		}
		return out, files, nil
	}
	if ms.Tag == "" {
		return unknown(sv1Reason)
	}
	rows, err := r.RepositoryConventions(ctx, repo.ID)
	if err != nil {
		return nil, nil, err
	}
	sha := ""
	if i := slices.IndexFunc(tags, func(t model.Tag) bool { return t.Name == ms.Tag }); i >= 0 {
		sha = tags[i].SHA
	}
	res, row, reason := cachedFiles(rows, model.ConventionRevTag, ms.Tag, sha, FileConfigHash(opts.Files, repo.FullName(), tags),
		hasClone(clones, repo))
	if reason != "" {
		return unknown(reason)
	}
	files.Known, files.SHA, files.ComputedAt = true, row.RevSHA, row.ComputedAt
	out := make([]Criterion, 0, len(milestoneFileCriteria))
	for _, id := range milestoneFileCriteria {
		c, ok := res.Find(id)
		if !ok {
			out = append(out, Criterion{ID: id, Status: CriterionUnknown, Reason: UnknownStale})
			continue
		}
		cr := Criterion{ID: id, Status: c.Status, Reason: c.Reason}
		if id == CriterionRD2 && c.Status != CriterionUnknown {
			cr.Status = CriterionNonconform
			if ReadmeSectionFound(c, SectionInstall) {
				cr.Status = CriterionConform
			}
			for i, key := range AllReadmeSections() {
				if c.Counts[CountExpectedMask]&(1<<i) != 0 {
					files.ReadmeExpected = append(files.ReadmeExpected, key)
				}
				if ReadmeSectionFound(c, key) {
					files.ReadmeFound = append(files.ReadmeFound, key)
				}
			}
		}
		out = append(out, cr)
	}
	return out, files, nil
}
