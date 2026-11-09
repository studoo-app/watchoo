package web

// Conventions d'un dépôt sur « /repos/{id} » (1.7.0, fragment « repo-conventions », seulement si les
// conventions sont activées) et ligne « Commits conformes n/m (x %) » de la page d'un contributeur. Les
// taux, écarts et exclusions viennent de internal/conventions (Build), calculés à la lecture ; aucune
// note, aucun classement, aucun message de commit affiché (SHA court, auteur et codes seulement).
// Séparé du diagnostic : ni donnée, ni clé, ni fonction de template en commun.

import (
	"cmp"
	"net/http"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
)

// repoConventionsID est l'identifiant HTML du fragment (cible HTMX).
const repoConventionsID = "repo-conventions"

// repoConventionsFailing est le nombre de commits non conformes récents affichés sur la page du dépôt.
const repoConventionsFailing = 10

// convCount est un motif d'exclusion et son nombre de commits.
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
}

// conventionsReport calcule le rapport des conventions pour f (clones lus sans mise à jour, sinon repli
// sur la base).
func (s *server) conventionsReport(r *http.Request, f conventions.Filter) (conventions.BuildReport, error) {
	ctx := r.Context()
	clones, err := s.repos.ConventionsClones(ctx)
	if err != nil {
		return conventions.BuildReport{}, err
	}
	return conventions.Build(ctx, s.store, clones, f, s.cfg.Conventions.BuildOptions(s.loc))
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
	for _, is := range repo.Issues {
		v.Issues = append(v.Issues, convIssueRow{Code: is.Code, Label: tr.T("web.conv.code." + is.Code),
			Blocking: is.Severity == conventions.SeverityBlocking,
			Severity: tr.T("web.conv.severity." + is.Severity.String()), Count: is.Count})
	}
	for _, c := range repo.Commits {
		if !c.Evaluated() || c.Conform {
			continue
		}
		v.FailingTotal++
		if len(v.Failing) < repoConventionsFailing {
			v.Failing = append(v.Failing, convCommitRow{SHA: c.SHA, Author: cmp.Or(c.AuthorName, c.AuthorLogin),
				Date: c.AuthoredAt, Codes: c.Codes})
		}
	}
	if !repo.Since.IsZero() {
		v.Since, v.SinceOrigin = repo.Since, tr.T("web.conv.since_origin."+repo.SinceOrigin)
	}
	return v, nil
}

// contributorConventions est la ligne « Commits conformes n/m (x %) » de la page d'un contributeur (CC3,
// tous dépôts confondus), avec ses écarts.
type contributorConventions struct {
	Evaluated int
	Conform   int
	Percent   int
	Excluded  int
	Issues    []convIssueRow
}

// contributorConventionsOf calcule la ligne CC3 du contributeur id ; nil si les conventions sont
// désactivées.
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
		Excluded: t.ExcludedTotal()}
	for _, is := range t.Issues {
		v.Issues = append(v.Issues, convIssueRow{Code: is.Code, Label: tr.T("web.conv.code." + is.Code),
			Blocking: is.Severity == conventions.SeverityBlocking,
			Severity: tr.T("web.conv.severity." + is.Severity.String()), Count: is.Count})
	}
	return v, nil
}
