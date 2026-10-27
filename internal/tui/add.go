package tui

// Ajout d'un dépôt à suivre depuis le TUI (touche « a », vue Dépôts) : saisie de la référence en bas
// de l'écran, analysée par repoimport.ParseRef comme « watchoo repo add », puis Adder.AddRepositories
// (wsync.Engine, accès vérifié) dans une commande Bubble Tea. Un seul ajout à la fois. Une fois le
// dépôt ajouté (ou son suivi repris), la liste est relue, le dépôt sélectionné et sa synchro lancée ;
// si une synchro tourne déjà, celle du nouveau dépôt attend la fin de la synchro en cours
// (pendingSyncs).
//
// La saisie est faite à la main, comme celle du filtre : bubbles/textinput tirerait un module de plus
// (presse-papiers), hors de la stack documentée. Le collage (tea.PasteMsg) est accepté.

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/repoimport"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// addPrompt est l'invite de la saisie d'un dépôt.
const addPrompt = "Dépôt à suivre (owner/nom ou URL GitHub) : "

// addCharLimit borne la longueur de la saisie, en caractères (une URL GitHub tient largement).
const addCharLimit = 300

// addState est l'état de l'ajout d'un dépôt.
type addState struct {
	open    bool   // saisie ouverte
	input   string // référence saisie
	running bool   // ajout en cours (un seul à la fois)
	label   string // dépôt en cours d'ajout (owner/nom saisi)
}

// addDoneMsg est la fin de l'ajout du dépôt ref.
type addDoneMsg struct {
	ref     repoimport.Ref
	results []wsync.AddResult
	err     error
}

// openAdd ouvre la saisie d'un dépôt (liste de la vue Dépôts seulement, hors détail).
func (m Model) openAdd() (tea.Model, tea.Cmd) {
	switch {
	case m.view != viewRepos || m.detail.open || m.cdetail.open:
		m.setStatus("« a » ajoute un dépôt depuis la liste de la vue Dépôts (touche 1)")
		return m, nil
	case m.deps.Adder == nil:
		m.setError("ajout de dépôt indisponible")
		return m, nil
	case m.add.running:
		m.setStatus("un ajout est déjà en cours (" + m.add.label + ") : patientez")
		return m, nil
	}
	m.add.open, m.add.input = true, ""
	m.status, m.statusErr = "", false
	return m, nil
}

// handleAddKey traite une touche pendant la saisie d'un dépôt : entrée valide, échap annule,
// retour arrière efface un caractère, ctrl+u vide la saisie, le reste s'ajoute à la saisie.
func (m Model) handleAddKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.add.open, m.add.input = false, ""
		m.setStatus("ajout annulé")
	case "enter":
		return m.submitAdd()
	case "backspace":
		if r := []rune(m.add.input); len(r) > 0 {
			m.add.input = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.add.input = ""
	default:
		m.appendAddInput(msg.Text)
	}
	return m, nil
}

// appendAddInput ajoute du texte tapé ou collé à la saisie, nettoyé (séquences d'échappement, retours
// à la ligne) et borné à addCharLimit caractères.
func (m *Model) appendAddInput(text string) {
	text = clean(strings.NewReplacer("\r", "", "\n", "", "\t", "").Replace(text))
	if text == "" {
		return
	}
	r := []rune(m.add.input + text)
	if len(r) > addCharLimit {
		r = r[:addCharLimit]
	}
	m.add.input = string(r)
}

// submitAdd analyse la saisie comme « watchoo repo add » et lance l'ajout. Une référence invalide est
// signalée sans appel au cœur, et la saisie reste ouverte pour la corriger.
func (m Model) submitAdd() (tea.Model, tea.Cmd) {
	raw := m.add.input
	ref, err := repoimport.ParseRef(raw)
	if err != nil {
		// Une saisie avec « @ » peut contenir des identifiants (https://user:token@…) : elle est
		// effacée (ParseRef ne la cite pas), comme dans l'interface web.
		if strings.Contains(raw, "@") {
			m.add.input = ""
		}
		m.setError(clean(err.Error()))
		return m, nil
	}
	m.add.open, m.add.input = false, ""
	m.add.running, m.add.label = true, clean(ref.String())
	m.setStatus("ajout de " + m.add.label + " : vérification de l'accès" + m.ellipsis())
	return m, m.addCmd(ref)
}

// addCmd ajoute le dépôt ref (wsync.Engine.AddRepositories, comme « repo add » sans option : toutes
// les branches, sans label ni seuils propres). Suivi par le tracker, comme les synchros : Run
// l'attend avant que la base ne soit fermée.
func (m Model) addCmd(ref repoimport.Ref) tea.Cmd {
	ctx, adder, tracker := m.ctx, m.deps.Adder, m.tracker
	return func() tea.Msg {
		if ctx.Err() != nil || !tracker.begin() {
			return nil // on quitte : l'ajout ne démarre pas
		}
		defer tracker.done()
		results, err := adder.AddRepositories(ctx, []wsync.AddRequest{{Ref: ref}}, wsync.AddOptions{})
		return addDoneMsg{ref: ref, results: results, err: err}
	}
}

// handleAddDone affiche le résultat de l'ajout, avec les mots de « watchoo repo add » ; un dépôt
// ajouté ou repris est relu, sélectionné et synchronisé (ou mis en attente pendant une synchro).
func (m Model) handleAddDone(msg addDoneMsg) (tea.Model, tea.Cmd) {
	m.add.running, m.add.label = false, ""
	ref := clean(msg.ref.String())
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return m, nil // on quitte
		}
		m.setError(clean(msg.err.Error()))
		return m, nil
	}
	if len(msg.results) != 1 {
		m.setError(fmt.Sprintf("ajout du dépôt %s : résultat manquant", ref))
		return m, nil
	}
	res := msg.results[0]
	name := clean(res.Repo.FullName())
	var text string
	switch res.Outcome {
	case wsync.OutcomeInaccessible:
		m.setError(fmt.Sprintf("dépôt %s inaccessible : %s (vérifiez son nom et que le compte gh, ou sans gh "+
			"la clé SSH ou les identifiants Git de cette machine, y a accès)", ref, clean(res.Error)))
		return m, nil
	case wsync.OutcomeAdded:
		text = name + " ajouté"
	case wsync.OutcomeUpdated:
		text = name + " déjà suivi : suivi repris"
	case wsync.OutcomeAlreadyTracked:
		text = name + " déjà suivi : rien à changer"
	default:
		m.setError(fmt.Sprintf("ajout du dépôt %s : %s", ref, clean(res.Error)))
		return m, nil
	}
	if !strings.EqualFold(msg.ref.String(), res.Repo.FullName()) {
		text += " (" + ref + " s'appelle maintenant " + name + " sur GitHub)"
	}

	m.selectRepo = res.Repo.ID
	m.loading = true
	cmds := []tea.Cmd{m.loadCmd()}
	if res.Outcome == wsync.OutcomeAlreadyTracked {
		m.setStatus(text)
		return m, tea.Batch(cmds...)
	}
	if m.sync.running {
		if !slices.Contains(m.pendingSyncs, res.Repo.ID) {
			m.pendingSyncs = append(m.pendingSyncs, res.Repo.ID)
		}
		m.setStatus(text + " : sa synchro commencera après la synchro en cours (" + m.sync.label + ")")
		return m, tea.Batch(cmds...)
	}
	var cmd tea.Cmd
	m, cmd = m.startSyncNamed(res.Repo.ID, name)
	if m.sync.running {
		m.setStatus(text + " : synchro de " + name + m.ellipsis())
	} else if !m.statusErr {
		m.setStatus(text)
	}
	return m, tea.Batch(append(cmds, cmd)...)
}

// startSyncNamed lance la synchro du dépôt id, nommé name (le dépôt vient d'être ajouté : il n'est
// pas encore dans la liste chargée).
func (m Model) startSyncNamed(id int64, name string) (Model, tea.Cmd) {
	m, cmd := m.startSync(id, false)
	if m.sync.running && m.sync.label == fmt.Sprintf("dépôt n° %d", id) {
		m.sync.label = name
	}
	return m, cmd
}

// startPendingSync lance la synchro du premier dépôt en attente (ajouté pendant une synchro).
func (m Model) startPendingSync() (Model, tea.Cmd) {
	if len(m.pendingSyncs) == 0 || m.sync.running || m.ctx.Err() != nil {
		return m, nil
	}
	id := m.pendingSyncs[0]
	m.pendingSyncs = slices.Delete(slices.Clone(m.pendingSyncs), 0, 1)
	return m.startSync(id, false)
}

// selectAfterLoad sélectionne, après une relecture, le dépôt demandé par selectRepo (dépôt ajouté),
// s'il est visible dans la vue Dépôts.
func (m *Model) selectAfterLoad() {
	if m.selectRepo == 0 {
		return
	}
	id := m.selectRepo
	m.selectRepo = 0
	for pos, i := range m.visible(viewRepos) {
		if m.repos[i].ID == id {
			m.lists[viewRepos].cursor = pos
			m.clamp(viewRepos)
			return
		}
	}
}

// renderAddFooter renvoie le bas de l'écran pendant la saisie d'un dépôt : la saisie, puis l'erreur
// éventuelle ou le rappel des touches.
func (m Model) renderAddFooter(w int) []string {
	second := m.st.Muted.Render(truncate("entrée pour ajouter, échap pour annuler", w, m.ellipsis()))
	if m.status != "" && m.statusErr {
		second = m.st.Error.Render(truncate(m.status, w, m.ellipsis()))
	}
	input := addPrompt + m.add.input + m.sym("▏", "_")
	if over := width(input) - w; over > 0 {
		// Saisie plus longue que l'écran : on montre sa fin, là où l'on tape.
		r := []rune(m.add.input)
		input = addPrompt + m.ellipsis() + string(r[min(over+1, len(r)):]) + m.sym("▏", "_")
	}
	return []string{input, second}
}
