package web

// Diagnostic (phase 7.4, documentation/fr/12-supervision-diagnostic.md section 9.4) : réservé au
// formateur qui supervise l'espace.
//
//	GET  /diagnostic               liste des pistes de réflexion (filtre ?repo=), bouton de reprise
//	GET  /diagnostic/lead          fiche de revue d'une piste (?repo=<id>&key=<clé>)
//	POST /diagnostic/review        enregistre la revue d'une piste (relecteur unique)
//	POST /diagnostic/backfill      lance la reprise de l'historique en arrière-plan (une à la fois)
//
// Aucune règle métier ici : pistes, niveaux, débit, voisins et règles d'une revue viennent de
// internal/diagnostic ; la reprise et le diff, du moteur (internal/sync). Vie privée : le diff est lu sur
// le clone local à chaque affichage, jamais gardé ; ni le diff, ni la note, ni un message de commit ne
// sont journalisés (le journal d'accès n'écrit que le motif de route).

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// Paramètres de la fiche de revue.
const (
	diagPatchMax     = 200 << 10 // diff affiché : 200 Ko au plus
	diagNeighbors    = 3         // commits voisins de part et d'autre
	diagChartDays    = 90        // graphique du débit
	diagBackfillTime = 30 * time.Minute
	msgReviewSaved   = "saved"
	msgBackfillStart = "backfill_started"
	msgBackfillBusy  = "backfill_running"
)

// backfillState est l'état de la reprise de l'historique lancée depuis le web (une à la fois).
type backfillState struct {
	mu       sync.Mutex
	running  bool
	finished time.Time // fin de la dernière reprise ; zéro si aucune
	measured int       // commits mesurés par la dernière reprise
	failed   int       // dépôts en échec lors de la dernière reprise
	err      bool      // la dernière reprise s'est arrêtée sur une erreur
	wg       sync.WaitGroup
}

// backfillView est l'état de la reprise affiché sur la page.
type backfillView struct {
	Running  bool
	Finished time.Time
	Measured int
	Failed   int
	Error    bool
}

func (b *backfillState) view() backfillView {
	b.mu.Lock()
	defer b.mu.Unlock()
	return backfillView{Running: b.running, Finished: b.finished, Measured: b.measured, Failed: b.failed, Error: b.err}
}

// diagnosticLeadRow est une ligne de la liste des pistes.
type diagnosticLeadRow struct {
	diagnostic.Lead
	Review *model.DiagnosticReview // revue de la piste ; nil si aucune
	Path   string                  // fiche de revue
}

// diagnosticView est le Content de la page « Diagnostic ».
type diagnosticView struct {
	Enabled   bool
	Leads     []diagnosticLeadRow
	Contexts  []store.AgentConfigContext
	Short     int // auteurs à l'historique insuffisant
	Histories int // auteurs mesurés
	Repos     []model.Repository
	RepoID    int64 // filtre ; 0 = tous
	Reviewed  int   // pistes revues
	Backfill  backfillView
	Message   string // message traduit après une action
	MinHist   int
}

// signalRow est un signal de la fiche de revue.
type signalRow struct {
	diagnostic.Hit
	Description string
}

// reviewForm est le formulaire de revue (saisie, ou revue enregistrée).
type reviewForm struct {
	Decision, Cause, Confidence, Note string
	Error                             string
	Saved                             bool
}

// diagnosticLeadView est le Content de la fiche de revue.
type diagnosticLeadView struct {
	Lead        diagnostic.Lead
	Signals     []signalRow
	Review      *model.DiagnosticReview
	Form        reviewForm
	Patch       string
	PatchTrunc  bool
	PatchShown  bool // un diff a été lu sur le clone
	Neighbors   []store.CommitSignalView
	Chart       template.JS // JSON du débit (diagnostic.Throughput)
	ChartDays   int
	AgentSince  time.Time // contexte D4 ; zéro si aucun
	Decisions   []string
	Causes      []string
	Confidences []string
	MaxNote     int
}

// reviewer renvoie le nom du relecteur : compte de l'interface web, sinon nom de l'espace.
func (s *server) reviewer() string {
	if s.auth.Username != "" {
		return s.auth.Username
	}
	return s.cfg.Name
}

// rhythmOptions renvoie les options des signaux de rythme de l'espace.
func (s *server) rhythmOptions() diagnostic.RhythmOptions {
	opts := diagnostic.RhythmOptions{Location: s.loc, SpeedSignal: s.cfg.Diagnostic.SpeedSignal}
	if days := s.cfg.Diagnostic.RetentionDays; days > 0 {
		opts.RetentionCutoff = s.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	}
	return opts
}

// leadPath renvoie l'adresse de la fiche de revue d'une piste.
func leadPath(repoID int64, key string) string {
	p, _ := withQuery("/diagnostic/lead", "repo", repoID, "key", key)
	return p
}

// handleDiagnostic sert GET /diagnostic.
func (s *server) handleDiagnostic(w http.ResponseWriter, r *http.Request) {
	view := diagnosticView{Enabled: s.cfg.Diagnostic.Enabled, Backfill: s.backfill.view(), MinHist: diagnostic.MinHistory}
	title := s.t(r, "web.nav.diagnostic")
	if !view.Enabled {
		s.render(w, r, http.StatusOK, pageDiagnostic, s.page(r, title, navDiagnostic, view))
		return
	}
	if id, err := strconv.ParseInt(queryValue(r, "repo"), 10, 64); err == nil && id > 0 {
		view.RepoID = id
	}
	switch queryValue(r, "msg") {
	case msgBackfillStart:
		view.Message = s.t(r, "web.diag.backfill_started")
	case msgBackfillBusy:
		view.Message = s.t(r, "web.diag.backfill_busy")
	}
	repos, err := s.store.ListRepositories(r.Context())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	view.Repos = repos
	rep, err := diagnostic.Build(r.Context(), s.store, store.DiagnosticFilter{RepoID: view.RepoID}, s.rhythmOptions())
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	reviews, err := s.store.DiagnosticReviews(r.Context(), view.RepoID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	for _, l := range rep.Leads {
		row := diagnosticLeadRow{Lead: l, Path: leadPath(l.RepoID, l.Key())}
		if rv, ok := reviews[store.ReviewKey(l.RepoID, l.Key())]; ok {
			row.Review = &rv
			view.Reviewed++
		}
		view.Leads = append(view.Leads, row)
	}
	view.Contexts = rep.Contexts
	view.Histories = len(rep.Histories)
	for _, h := range rep.Histories {
		if !h.Sufficient {
			view.Short++
		}
	}
	s.render(w, r, http.StatusOK, pageDiagnostic, s.page(r, title, navDiagnostic, view))
}

// findLead retrouve la piste key du dépôt repoID ; ok faux si elle n'existe pas (ou plus).
func (s *server) findLead(ctx context.Context, repoID int64, key string) (diagnostic.Lead, diagnostic.Report, bool, error) {
	rep, err := diagnostic.Build(ctx, s.store, store.DiagnosticFilter{RepoID: repoID}, s.rhythmOptions())
	if err != nil {
		return diagnostic.Lead{}, rep, false, err
	}
	l, ok := diagnostic.FindLead(rep, repoID, key)
	return l, rep, ok, nil
}

// leadParams lit le dépôt et la clé d'une piste (query ou formulaire).
func leadParams(r *http.Request, form bool) (int64, string, bool) {
	get := queryValue
	if form {
		get = formValue
	}
	id, err := strconv.ParseInt(get(r, "repo"), 10, 64)
	key := get(r, "key")
	if err != nil || id <= 0 || key == "" || len(key) > 200 {
		return 0, "", false
	}
	return id, key, true
}

// handleDiagnosticLead sert GET /diagnostic/lead.
func (s *server) handleDiagnosticLead(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Diagnostic.Enabled {
		s.notFound(w, r)
		return
	}
	repoID, key, ok := leadParams(r, false)
	if !ok {
		s.notFound(w, r)
		return
	}
	s.renderLead(w, r, http.StatusOK, repoID, key, nil)
}

// renderLead affiche la fiche de revue ; input (non nil) est une saisie refusée à réafficher.
func (s *server) renderLead(w http.ResponseWriter, r *http.Request, status int, repoID int64, key string, input *reviewForm) {
	ctx := r.Context()
	lead, rep, ok, err := s.findLead(ctx, repoID, key)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !ok {
		s.notFound(w, r)
		return
	}
	tr := s.tr(r)
	view := diagnosticLeadView{Lead: lead, ChartDays: diagChartDays, Decisions: diagnostic.Decisions,
		Causes: diagnostic.Causes, Confidences: diagnostic.Confidences, MaxNote: diagnostic.MaxNote}
	for _, h := range lead.Hits {
		view.Signals = append(view.Signals, signalRow{Hit: h, Description: tr.T("web.diag.signal." + h.Signal)})
	}
	reviews, err := s.store.DiagnosticReviews(ctx, repoID)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if rv, ok := reviews[store.ReviewKey(repoID, key)]; ok {
		view.Review = &rv
		view.Form = reviewForm{Decision: rv.Decision, Cause: rv.Cause, Confidence: rv.Confidence, Note: rv.Note}
	}
	if input != nil {
		view.Form = *input
	} else if queryValue(r, "msg") == msgReviewSaved {
		view.Form.Saved = true
	}
	for _, c := range rep.Contexts {
		if c.RepoID == repoID {
			view.AgentSince = c.Since
		}
	}
	if lead.Kind != diagnostic.KindPullRequest {
		views, err := s.store.CommitSignalViews(ctx, repoID)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		series := diagnostic.Throughput(views, repoID, lead.AuthorID, lead.Date, diagChartDays, s.loc)
		data, err := json.Marshal(series)
		if err != nil {
			s.serverError(w, r, err)
			return
		}
		view.Chart = template.JS(data) //nolint:gosec // JSON généré ici, écrit dans <script type="application/json">
		if lead.Kind == diagnostic.KindCommit {
			view.Neighbors = diagnostic.Neighbors(views, repoID, lead.AuthorID, lead.SHA, diagNeighbors)
			repo, err := s.store.GetRepository(ctx, repoID)
			if err == nil {
				patch, trunc, found, perr := s.repos.CommitPatch(ctx, repo, lead.SHA, diagPatchMax)
				if perr != nil {
					s.logger.Debug("diff du diagnostic non lu", "repo", repoID, "erreur", perr.Error())
				}
				view.Patch, view.PatchTrunc, view.PatchShown = patch, trunc, found && perr == nil
			}
		}
	}
	s.render(w, r, status, pageDiagnosticLead, s.page(r, tr.T("web.diag.lead_title"), navDiagnostic, view))
}

// handleDiagnosticReview sert POST /diagnostic/review.
func (s *server) handleDiagnosticReview(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Diagnostic.Enabled {
		s.notFound(w, r)
		return
	}
	repoID, key, ok := leadParams(r, true)
	if !ok {
		s.badRequest(w, r, s.t(r, "web.diag.bad_lead"))
		return
	}
	input := reviewForm{Decision: formValue(r, "decision"), Cause: formValue(r, "cause"),
		Confidence: formValue(r, "confidence"), Note: r.PostFormValue("note")}
	lead, _, found, err := s.findLead(r.Context(), repoID, key)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	if !found {
		s.notFound(w, r)
		return
	}
	review, err := diagnostic.NewReview(s.tr(r), lead, diagnostic.ReviewInput{Decision: input.Decision,
		Cause: input.Cause, Confidence: input.Confidence, Note: input.Note}, s.reviewer(), s.now())
	if err != nil {
		input.Error = s.t(r, "web.diag.review_refused", err)
		input.Note = truncateRunes(input.Note, diagnostic.MaxNote+100)
		s.renderLead(w, r, http.StatusBadRequest, repoID, key, &input)
		return
	}
	if err := s.store.SaveDiagnosticReview(r.Context(), review); err != nil {
		s.serverError(w, r, err)
		return
	}
	redirect(w, r, leadPath(repoID, key)+"&msg="+msgReviewSaved+"#review")
}

// handleDiagnosticBackfill sert POST /diagnostic/backfill : lance la reprise en arrière-plan.
func (s *server) handleDiagnosticBackfill(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.Diagnostic.Enabled {
		s.notFound(w, r)
		return
	}
	b := &s.backfill
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		redirect(w, r, "/diagnostic?msg="+msgBackfillBusy)
		return
	}
	b.running = true
	b.wg.Add(1)
	b.mu.Unlock()
	go func() {
		defer b.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), diagBackfillTime)
		defer cancel()
		res, err := s.repos.BackfillSignals(ctx, 0)
		measured, failed := 0, 0
		for _, rr := range res {
			measured += rr.Measured
			if rr.Error != "" {
				failed++
			}
		}
		if err != nil {
			s.logger.Warn("reprise du diagnostic interrompue", "erreur", err.Error())
		}
		b.mu.Lock()
		b.running, b.finished, b.measured, b.failed, b.err = false, s.now(), measured, failed, err != nil
		b.mu.Unlock()
	}()
	redirect(w, r, "/diagnostic?msg="+msgBackfillStart)
}

// truncateRunes coupe s à n caractères.
func truncateRunes(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

// diagnosticFuncs sont les fonctions des templates du diagnostic.
func diagnosticFuncs(tr *i18n.Translator) template.FuncMap {
	return template.FuncMap{
		"diagLevel": func(l string) string { return tr.T("web.diag.level." + l) },
		"diagLevelClass": func(l string) string {
			switch l {
			case diagnostic.LevelDeclared:
				return "badge-info"
			case diagnostic.LevelProbable:
				return "badge-error"
			}
			return "badge-warning"
		},
		"diagKind":       func(k string) string { return tr.T("web.diag.kind." + k) },
		"diagDecision":   func(d string) string { return tr.T("web.diag.decision." + d) },
		"diagCause":      func(c string) string { return tr.T("web.diag.cause." + c) },
		"diagConfidence": func(c string) string { return tr.T("web.diag.confidence." + c) },
		"diagNum": func(x float64) string {
			if x == float64(int64(x)) {
				return strconv.FormatInt(int64(x), 10)
			}
			return strconv.FormatFloat(x, 'f', 1, 64)
		},
		"diagHits": func(hits []diagnostic.Hit) string {
			parts := make([]string, len(hits))
			for i, h := range hits {
				parts[i] = h.Signal + " " + h.Value
			}
			return strings.Join(parts, ", ")
		},
		"leadPath": leadPath,
	}
}
