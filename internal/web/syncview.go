package web

// Synchros depuis l'interface : boutons « Synchroniser maintenant » (POST /sync) et « Synchroniser ce
// dépôt » (POST /repos/{id}/sync), fragment de progression GET /fragments/sync (navbar, id
// « sync-status »).
//
// Le fragment se rafraîchit toutes les 2 s pendant une synchro, toutes les 30 s sinon (pour voir
// démarrer une synchro automatique). Quand il constate la fin d'une synchro qu'il a vue tourner (ou
// d'une synchro plus récente que celle qu'il affichait), sa réponse porte l'en-tête
// « HX-Trigger: sync-finished » : les fragments des pages qui écoutent
// hx-trigger="sync-finished from:body" se rechargent alors.

import (
	"errors"
	"net/http"
	"strconv"

	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// SyncFinishedEvent est l'événement HTMX envoyé à la fin d'une synchro (en-tête HX-Trigger).
const SyncFinishedEvent = "sync-finished"

// Messages des synchros manuelles.
const (
	msgSyncStarted = "Synchro lancée."
	msgSyncRunning = "Synchro déjà en cours : attendez qu'elle se termine."
	msgSyncClosed  = "Arrêt de Watchoo en cours : aucune nouvelle synchro."
)

// SyncView est l'état des synchros affiché par le fragment « sync-status ».
type SyncView struct {
	wsync.SyncState
	Percent      int    // progression de la synchro en cours, de 0 à 100
	Message      string // message d'une action (synchro lancée, déjà en cours) ; vide sinon
	MessageError bool   // Message signale un refus
	QuotaLow     bool   // le dernier quota connu est sous le garde-fou (wsync.QuotaLow)
	Errors       int    // dépôts en erreur dans le dernier rapport
	Deferred     int    // dépôts reportés (quota) dans le dernier rapport
	NewCommits   int    // nouveaux commits du dernier rapport
}

// syncView renvoie l'état courant des synchros, avec un éventuel message.
func (s *server) syncView(message string) SyncView {
	st := s.sync.State()
	v := SyncView{SyncState: st, Message: message, Percent: percent(st.Done, st.Total)}
	if st.HasReport {
		v.Errors = st.LastReport.Count(wsync.StatusError)
		v.Deferred = st.LastReport.Count(wsync.StatusDeferred)
		v.NewCommits = st.LastReport.NewCommits()
		v.QuotaLow = st.LastReport.RateLimitKnown && wsync.QuotaLow(st.LastReport.RateLimit, s.now())
	}
	return v
}

// handleSyncStatus sert GET /fragments/sync?gen=N&run=0|1 (fragment « sync-status »). gen et run
// décrivent l'état affiché par le fragment qui demande la mise à jour.
func (s *server) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	data := s.page(r, "", "", nil)
	st := data.Sync
	gen, err := strconv.ParseUint(r.URL.Query().Get("gen"), 10, 64)
	sawRunning := r.URL.Query().Get("run") == "1"
	if !st.Running && err == nil && (sawRunning || gen != st.Generation) && st.Generation > 0 {
		w.Header().Set("HX-Trigger", SyncFinishedEvent)
	}
	s.renderFragment(w, r, http.StatusOK, pageErrors, "sync-status", data)
}

// handleSyncAll sert POST /sync : synchro de tous les dépôts.
func (s *server) handleSyncAll(w http.ResponseWriter, r *http.Request) {
	s.startSync(w, r, s.sync.SyncAll(), "/settings")
}

// handleSyncRepo sert POST /repos/{id}/sync : synchro d'un seul dépôt (404 s'il n'existe pas).
func (s *server) handleSyncRepo(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if _, err := s.store.GetRepository(r.Context(), id); err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	s.startSync(w, r, s.sync.SyncRepo(id), repoPath(id))
}

// startSync répond à une demande de synchro dont le lancement a donné err :
//   - HTMX : fragment « sync-status » avec le message (200, même en cas de refus : HTMX ne remplace
//     pas le contenu sur une réponse 4xx) ; si la synchro lancée est déjà finie au rendu, la réponse
//     porte HX-Trigger: sync-finished (le fragment ne la verrait pas finir) ;
//   - sans HTMX : redirection 303 vers back si la synchro est lancée, sinon page d'erreur 409.
func (s *server) startSync(w http.ResponseWriter, r *http.Request, err error, back string) {
	msg, refused := msgSyncStarted, false
	switch {
	case err == nil:
	case errors.Is(err, wsync.ErrSyncRunning):
		msg, refused = msgSyncRunning, true
	case errors.Is(err, wsync.ErrCoordinatorClosed):
		msg, refused = msgSyncClosed, true
	default:
		s.serverError(w, r, err)
		return
	}
	if !isHTMX(r) {
		if refused {
			s.renderError(w, r, http.StatusConflict, "Synchro impossible", msg)
			return
		}
		redirect(w, r, back)
		return
	}
	data := s.page(r, "", "", nil)
	data.Sync.Message, data.Sync.MessageError = msg, refused
	if !refused && !data.Sync.Running {
		// La synchro lancée est déjà terminée (échec immédiat du moteur, dépôt vite synchronisé) : le
		// fragment rendu porte sa génération avec run=0, et le sondage suivant ne verrait jamais sa
		// fin. On signale donc la fin tout de suite.
		w.Header().Set("HX-Trigger", SyncFinishedEvent)
	}
	s.renderFragment(w, r, http.StatusOK, pageErrors, "sync-status", data)
}
