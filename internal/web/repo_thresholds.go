package web

// Seuils d'activité propres à un dépôt : formulaire de la page de détail (« Seuils d'activité ») et
// POST /repos/{id}/thresholds (documentation/06-interface-web.md, page 2).
//
// Champs flemme et dead : nombre de jours sans commit ; vide ou 0 = seuil de l'espace (activity.*).
// La saisie est lue et vérifiée par repoimport (ParseThreshold, ValidateThresholds), puis enregistrée
// par le moteur (wsync.Engine.SetThresholds, mêmes règles que « repo add --flemme / --dead »). Réponse : redirection
// 303 vers /repos/{id}?msg=thresholds, ou la page réaffichée avec l'erreur et la saisie (400).

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/studoo-app/watchoo/internal/repoimport"
)

// Champs du formulaire des seuils et code du message après l'enregistrement.
const (
	fieldFlemme      = "flemme"
	fieldDead        = "dead"
	msgThresholds    = "thresholds"
	thresholdsSaved  = "Seuils enregistrés : l'état du dépôt est recalculé avec ces valeurs."
	thresholdsFormID = "repo-thresholds"
)

// thresholdsForm est le formulaire des seuils d'un dépôt.
type thresholdsForm struct {
	Flemme, Dead string // saisie : valeurs propres actuelles, ou saisie refusée ; vide = seuil de l'espace
	// FlemmeOwn et DeadOwn indiquent un seuil propre au dépôt (sinon celui de l'espace s'applique).
	FlemmeOwn, DeadOwn bool
	// GlobalFlemme et GlobalDead sont les seuils de l'espace (activity.*), rappelés dans le formulaire.
	GlobalFlemme, GlobalDead int
	Error                    string // saisie refusée ; le formulaire est alors ouvert
	Saved                    bool   // enregistrement réussi (msg=thresholds)
}

// Open indique que le formulaire est ouvert à l'affichage (erreur ou enregistrement réussi).
func (f thresholdsForm) Open() bool { return f.Error != "" || f.Saved }

// SavedMessage est le message affiché après l'enregistrement.
func (f thresholdsForm) SavedMessage() string { return thresholdsSaved }

// newThresholdsForm renvoie le formulaire avec les valeurs propres du dépôt (vide = seuil de l'espace).
func (s *server) newThresholdsForm(flemme, dead *int) thresholdsForm {
	f := thresholdsForm{GlobalFlemme: s.cfg.Activity.FlemmeAfterDays, GlobalDead: s.cfg.Activity.DeadAfterDays}
	if flemme != nil {
		f.Flemme, f.FlemmeOwn = strconv.Itoa(*flemme), true
	}
	if dead != nil {
		f.Dead, f.DeadOwn = strconv.Itoa(*dead), true
	}
	return f
}

// parseThresholdsForm lit les deux champs (repoimport.ParseThreshold) et vérifie leur cohérence
// (repoimport.ValidateThresholds), comme « repo add --flemme / --dead ». L'erreur cite le champ.
func parseThresholdsForm(flemme, dead string) (*int, *int, error) {
	f, err := repoimport.ParseThreshold(flemme)
	if err != nil {
		return nil, nil, errors.New("Flemme : " + err.Error())
	}
	d, err := repoimport.ParseThreshold(dead)
	if err != nil {
		return nil, nil, errors.New("Dead : " + err.Error())
	}
	if err := repoimport.ValidateThresholds(f, d); err != nil {
		return nil, nil, err
	}
	return f, d, nil
}

// handleRepoThresholds sert POST /repos/{id}/thresholds (champs flemme et dead).
func (s *server) handleRepoThresholds(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	input := thresholdsForm{Flemme: formValue(r, fieldFlemme), Dead: formValue(r, fieldDead)}
	flemme, dead, err := parseThresholdsForm(input.Flemme, input.Dead)
	if err != nil {
		view, verr := s.repoView(r, id, true)
		if verr != nil {
			s.repoLookupError(w, r, verr)
			return
		}
		form := s.newThresholdsForm(view.Repo.FlemmeAfterDays, view.Repo.DeadAfterDays)
		form.Flemme, form.Dead = truncate(20, input.Flemme), truncate(20, input.Dead)
		form.Error = "Seuils refusés : " + err.Error()
		view.Thresholds = form
		s.render(w, r, http.StatusBadRequest, pageRepo, s.page(r, view.Repo.DisplayName(), "", view))
		return
	}
	if _, err := s.repos.SetThresholds(r.Context(), id, flemme, dead); err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	redirect(w, r, repoPath(id)+"?msg="+msgThresholds+"#"+thresholdsFormID)
}

// thresholdsText décrit des seuils propres pour l'aperçu d'un import : « Flemme 3 j · Dead espace »
// (« espace » = seuil de l'espace), ou « espace » si aucun n'est propre au dépôt.
func thresholdsText(flemme, dead *int) string {
	if flemme == nil && dead == nil {
		return "espace"
	}
	days := func(p *int) string {
		if p == nil {
			return "espace"
		}
		return strconv.Itoa(*p) + " j"
	}
	return "Flemme " + days(flemme) + " · Dead " + days(dead)
}
