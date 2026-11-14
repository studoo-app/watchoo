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
	// Groupes de contributeurs (1.9.0) : message sur la ligne du principal.
	msgAttached = "attached"
	msgDetached = "detached"
)

// fieldPrimary est le champ du principal choisi (rattachement, 1.9.0).
const fieldPrimary = "primary"

// fieldQuery est le champ de recherche de la page des noms donnés.
const fieldQuery = "q"

// contributorLabels est le contenu de la page des noms donnés.
type contributorLabels struct {
	Rows  []contributorLabelRow // principaux, triés par nom affiché, filtrés par Query
	Total int                   // contributeurs avant le filtre, comptes rattachés compris
	Query string                // recherche (?q=) ; vide = tous
	Max   int                   // longueur maximale d'un nom donné
	// Principals sont les principaux possibles d'un rattachement (1.9.0) : tout contributeur non
	// rattaché, trié par nom affiché.
	Principals []principalOption
	byID       map[int64]store.LabeledContributor // tous les contributeurs (messages)
}

// principalOption est un principal possible dans la liste « Rattacher à… ».
type principalOption struct {
	ID   int64
	Name string
}

// contributorLabelRow est un contributeur de la page des noms donnés.
type contributorLabelRow struct {
	store.LabeledContributor
	Edit  string // saisie du champ : nom donné actuel, ou saisie refusée
	Error string // message d'erreur de la ligne ; vide sinon
	// Accounts sont les comptes rattachés à ce principal (1.9.0), triés par nom affiché.
	Accounts    []contributorLabelRow
	GroupError  string // erreur d'un rattachement ou d'un détachement ; vide sinon
	matchedSelf bool
}

// CanAttach indique si ce contributeur peut être rattaché : il n'a pas de compte rattaché (un seul
// niveau, 1.9.0).
func (c contributorLabelRow) CanAttach() bool { return len(c.Accounts) == 0 }

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
	c := contributorLabels{Rows: []contributorLabelRow{}, Total: len(all), Query: q, Max: model.MaxContributorLabelRunes,
		Principals: []principalOption{}, byID: make(map[int64]store.LabeledContributor, len(all))}
	byPrimary := map[int64][]contributorLabelRow{}
	for _, lc := range all {
		c.byID[lc.ID] = lc
		row := contributorLabelRow{LabeledContributor: lc, Edit: lc.Label, matchedSelf: matchesQuery(lc.Contributor, q)}
		if lc.PrimaryID != 0 {
			byPrimary[lc.PrimaryID] = append(byPrimary[lc.PrimaryID], row)
			continue
		}
		c.Principals = append(c.Principals, principalOption{ID: lc.ID, Name: lc.DisplayName()})
		c.Rows = append(c.Rows, row)
	}
	byName := func(a, b contributorLabelRow) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.DisplayName()), strings.ToLower(b.DisplayName())), cmp.Compare(a.ID, b.ID))
	}
	// Un groupe est montré si son principal ou l'un de ses comptes correspond à la recherche.
	kept := c.Rows[:0]
	for _, row := range c.Rows {
		row.Accounts = byPrimary[row.ID]
		slices.SortFunc(row.Accounts, byName)
		match := row.matchedSelf
		for _, a := range row.Accounts {
			match = match || a.matchedSelf
		}
		if match {
			kept = append(kept, row)
		}
	}
	c.Rows = kept
	slices.SortFunc(c.Rows, byName)
	slices.SortFunc(c.Principals, func(a, b principalOption) int {
		return cmp.Or(cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.ID, b.ID))
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
	row, ok := c.byID[n]
	if !ok {
		return ""
	}
	name := textclean.Clean(row.DisplayName())
	switch msg {
	case msgLabelRemoved:
		return tr.T("web.labels.removed", name)
	case msgAttached:
		return tr.T("web.groups.attached", name)
	case msgDetached:
		return tr.T("web.groups.detached", name)
	}
	return tr.T("web.labels.saved", name)
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

// groupError traduit une erreur d'un rattachement ou d'un détachement dans la langue de tr ; vide pour
// une erreur qui n'est pas une saisie refusée.
func groupError(tr *i18n.Translator, err error) string {
	switch {
	case errors.Is(err, store.ErrGroupSelf):
		return tr.T("web.groups.err_self")
	case errors.Is(err, store.ErrGroupAttached):
		return tr.T("web.groups.err_attached")
	case errors.Is(err, store.ErrGroupHasAccounts):
		return tr.T("web.groups.err_has_accounts")
	case errors.Is(err, store.ErrGroupNotPrimary):
		return tr.T("web.groups.err_not_primary")
	case errors.Is(err, store.ErrGroupNotAttached):
		return tr.T("web.groups.err_not_attached")
	}
	return ""
}

// handleContributorAttach sert POST /settings/contributors/{id}/attach (champ primary) : rattache le
// contributeur id au principal choisi (1.9.0).
func (s *server) handleContributorAttach(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	primary, err := strconv.ParseInt(r.PostFormValue(fieldPrimary), 10, 64)
	if err != nil || primary <= 0 {
		s.groupRefused(w, r, id, s.t(r, "web.groups.err_choose"))
		return
	}
	s.groupResult(w, r, id, primary, msgAttached, s.store.AttachContributor(r.Context(), id, primary))
}

// handleContributorDetach sert POST /settings/contributors/{id}/detach : détache le compte id de son
// principal (1.9.0).
func (s *server) handleContributorDetach(w http.ResponseWriter, r *http.Request) {
	id, ok := repoIDParam(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	primary, err := s.store.PrimaryOf(r.Context(), id)
	if err != nil {
		s.repoLookupError(w, r, err)
		return
	}
	s.groupResult(w, r, id, primary, msgDetached, s.store.DetachContributor(r.Context(), id))
}

// groupResult répond à un rattachement ou à un détachement : redirection vers la ligne du principal,
// page réaffichée avec l'erreur (400), 404 ou 500.
func (s *server) groupResult(w http.ResponseWriter, r *http.Request, id, primary int64, code string, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.notFound(w, r)
		return
	case err != nil:
		if msg := groupError(s.tr(r), err); msg != "" {
			s.groupRefused(w, r, id, msg)
			return
		}
		s.serverError(w, r, err)
		return
	}
	n := strconv.FormatInt(primary, 10)
	redirect(w, r, settingsContributorsPath+"?msg="+code+"&contributor="+n+"#contributor-"+n)
}

// groupRefused réaffiche la page des contributeurs (400) avec l'erreur msg sur la ligne de id (ou de
// son principal).
func (s *server) groupRefused(w http.ResponseWriter, r *http.Request, id int64, msg string) {
	v, err := s.settingsData(r, sectionContributors)
	if err != nil {
		s.serverError(w, r, err)
		return
	}
	found := false
	for i := range v.Contributors.Rows {
		row := &v.Contributors.Rows[i]
		if row.ID == id {
			row.GroupError, found = msg, true
		}
		for j := range row.Accounts {
			if row.Accounts[j].ID == id {
				row.Accounts[j].GroupError, found = msg, true
			}
		}
	}
	if !found {
		if _, known := v.Contributors.byID[id]; !known {
			s.notFound(w, r)
			return
		}
	}
	v.Flash, v.FlashError = msg, true
	s.renderSettings(w, r, http.StatusBadRequest, v)
}
