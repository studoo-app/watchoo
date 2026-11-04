package web

// Diagnostic d'un dépôt sur « /repos/{id} » (1.6.0, fragment « repo-diagnostic », seulement si le
// diagnostic est activé) : score du dépôt avec le nombre de pistes par niveau et le décompte des
// signaux, dernières pistes, lien vers la liste filtrée. Score, niveaux et décompte viennent de
// internal/diagnostic (Summarize) ; aucun message de commit n'est affiché.

import (
	"net/http"
	"strconv"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/store"
)

// repoDiagnosticLeads est le nombre de pistes récentes affichées sur la page du dépôt.
const repoDiagnosticLeads = 10

// repoDiagnosticID est l'identifiant HTML du fragment (cible HTMX).
const repoDiagnosticID = "repo-diagnostic"

// repoSignalRow est une ligne du décompte des signaux du dépôt.
type repoSignalRow struct {
	diagnostic.SignalCount
	Description string
}

// RepoDiagnosticView est la partie « diagnostic » de la page du dépôt.
type RepoDiagnosticView struct {
	Summary  diagnostic.Summary
	Signals  []repoSignalRow
	Leads    []diagnosticLeadRow // les repoDiagnosticLeads plus récentes
	Reviewed int                 // pistes revues, sur toutes celles du dépôt
	ListPath string              // /diagnostic?repo=<id>
}

// repoDiagnostic lit le diagnostic du dépôt id ; nil si le diagnostic est désactivé.
func (s *server) repoDiagnostic(r *http.Request, id int64) (*RepoDiagnosticView, error) {
	if !s.cfg.Diagnostic.Enabled {
		return nil, nil
	}
	ctx := r.Context()
	rep, err := diagnostic.Build(ctx, s.store, store.DiagnosticFilter{RepoID: id}, s.rhythmOptions())
	if err != nil {
		return nil, err
	}
	reviews, err := s.store.DiagnosticReviews(ctx, id)
	if err != nil {
		return nil, err
	}
	tr := s.tr(r)
	v := &RepoDiagnosticView{Summary: diagnostic.Summarize(rep.Leads),
		ListPath: "/diagnostic?repo=" + strconv.FormatInt(id, 10)}
	for _, c := range v.Summary.Signals {
		v.Signals = append(v.Signals, repoSignalRow{SignalCount: c, Description: tr.T("web.diag.signal." + c.Signal)})
	}
	for _, l := range rep.Leads {
		row := diagnosticLeadRow{Lead: l, Path: leadPath(l.RepoID, l.Key())}
		if rv, ok := reviews[store.ReviewKey(l.RepoID, l.Key())]; ok {
			row.Review = &rv
			v.Reviewed++
		}
		if len(v.Leads) < repoDiagnosticLeads {
			v.Leads = append(v.Leads, row)
		}
	}
	return v, nil
}
