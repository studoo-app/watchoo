package web

// Noms donnés aux contributeurs « /settings/contributors » (1.8.0, documentation/fr/06-interface-web.md,
// section 5.2). Les commits ne donnent qu'un pseudo (login, nom Git) : le superviseur donne à un
// contributeur le nom de son choix, affiché partout à la place du pseudo. Le nom est vérifié par
// model.NormalizeContributorLabel et enregistré par store.SetContributorLabel ; la synchro ne l'écrit
// jamais et le partage de la configuration ne l'exporte pas.
//
// POST /settings/contributors/{id} (champ label, CSRF) : redirection 303 vers
// /settings/contributors?msg=<code>&contributor=<id>#contributor-<id> (codes fixes), ou la page
// réaffichée avec l'erreur sur la ligne (400), 404 pour un contributeur inconnu.

import (
	"cmp"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	"github.com/studoo-app/watchoo/internal/textclean"
)

// Codes des messages de la page des noms donnés (paramètre msg).
const (
	msgLabelSaved   = "label_saved"
	msgLabelRemoved = "label_removed"
)

// fieldQuery est le champ de recherche de la page des noms donnés.
const fieldQuery = "q"

// contributorLabels est le contenu de la page des noms donnés.
type contributorLabels struct {
	Rows  []contributorLabelRow // triées par nom affiché, filtrées par Query
	Total int                   // contributeurs avant le filtre
	Query string                // recherche (?q=) ; vide = tous
	Max   int                   // longueur maximale d'un nom donné
}

// contributorLabelRow est un contributeur de la page des noms donnés.
type contributorLabelRow struct {
	store.LabeledContributor
	Edit  string // saisie du champ : nom donné actuel, ou saisie refusée
	Error string // message d'erreur de la ligne ; vide sinon
}

// Anchor renvoie l'ancre de la ligne (contributor-<id>).
func (c contributorLabelRow) Anchor() string { return "contributor-" + strconv.FormatInt(c.ID, 10) }

// ShowLogin indique s'il faut afficher le login à côté du nom (il existe et n'est pas déjà le nom).
func (c contributorLabelRow) ShowLogin() bool {
	return c.ProviderLogin != "" && c.ProviderLogin != c.DisplayName()
}

// matchesQuery indique si le nom donné, le nom ou le login de c contient q (sans casse). Fonction pure.
func matchesQuery(c model.Contributor, q string) bool {
	if q == "" {
		return true
	}
	q = strings.ToLower(q)
	for _, v := range []string{c.Label, c.Name, c.ProviderLogin} {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

// settingsContributors complète v avec les contributeurs, filtrés par ?q= et triés par nom affiché
// (sans casse), puis identifiant.
func (s *server) settingsContributors(r *http.Request, v settingsView) (settingsView, error) {
	all, err := s.store.LabeledContributors(r.Context())
	if err != nil {
		return settingsView{}, err
	}
	q := truncate(100, queryValue(r, fieldQuery))
	c := contributorLabels{Rows: []contributorLabelRow{}, Total: len(all), Query: q, Max: model.MaxContributorLabelRunes}
	for _, lc := range all {
		if matchesQuery(lc.Contributor, q) {
			c.Rows = append(c.Rows, contributorLabelRow{LabeledContributor: lc, Edit: lc.Label})
		}
	}
	slices.SortFunc(c.Rows, func(a, b contributorLabelRow) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName())), cmp.Compare(a.ID, b.ID))
	})
	v.Contributors = c
	return v, nil
}

// flash renvoie le message du code msg pour le contributeur id (texte de la query), nommé par son nom
// affiché actuel ; vide si id est inconnu.
func (c contributorLabels) flash(tr *i18n.Translator, msg, id string) string {
	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil || n <= 0 {
		return ""
	}
	for _, row := range c.Rows {
		if row.ID != n {
			continue
		}
		name := textclean.Clean(row.DisplayName())
		if msg == msgLabelRemoved {
			return tr.T("web.labels.removed", name)
		}
		return tr.T("web.labels.saved", name)
	}
	return ""
}

// labelError traduit une erreur de model.NormalizeContributorLabel dans la langue de tr.
func labelError(tr *i18n.Translator, err error) string {
	if errors.Is(err, model.ErrContributorLabelTooLong) {
		return tr.T("web.labels.too_long", model.MaxContributorLabelRunes)
	}
	return tr.T("web.labels.invalid")
}

// handleContributorLabel sert POST /settings/contributors/{id} (champ label ; vide : retire le nom donné).
func (s *server) handleContributorLabel(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	input := r.PostFormValue(fieldLabel)
	label, err := s.store.SetContributorLabel(r.Context(), id, input)
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.notFound(w, r)
		return
	case errors.Is(err, model.ErrContributorLabelInvalid), errors.Is(err, model.ErrContributorLabelTooLong):
		v, lerr := s.settingsData(r, sectionContributors)
		if lerr != nil {
			s.serverError(w, r, lerr)
			return
		}
		found := false
		for i := range v.Contributors.Rows {
			if v.Contributors.Rows[i].ID == id {
				// Une saisie avec des caractères de contrôle n'est pas réaffichée telle quelle.
				v.Contributors.Rows[i].Edit = textclean.Clean(truncate(200, input))
				v.Contributors.Rows[i].Error, found = labelError(s.tr(r), err), true
			}
		}
		if !found {
			s.notFound(w, r)
			return
		}
		v.Flash, v.FlashError = s.t(r, "web.labels.refused"), true
		s.renderSettings(w, r, http.StatusBadRequest, v)
		return
	case err != nil:
		s.serverError(w, r, err)
		return
	}
	code := msgLabelSaved
	if label == "" {
		code = msgLabelRemoved
	}
	n := strconv.FormatInt(id, 10)
	redirect(w, r, settingsContributorsPath+"?msg="+code+"&contributor="+n+"#contributor-"+n)
}
