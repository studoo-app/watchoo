package tui

// Modèle racine Bubble Tea : état de l'écran, messages et traitement des touches. Les lectures de la
// base et les synchros tournent dans des commandes Bubble Tea (goroutines), jamais dans Update.

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/store"
)

// Valeurs par défaut de l'écran.
const (
	defaultCommitLimit = 200              // commits chargés dans la vue commits (Deps.CommitLimit = 0)
	defaultWidth       = 80               // largeur supposée avant le premier tea.WindowSizeMsg
	defaultHeight      = 24               // hauteur supposée avant le premier tea.WindowSizeMsg
	detailCommits      = 15               // commits affichés dans le détail d'un dépôt
	detailContributors = 5                // contributeurs affichés dans le détail d'un dépôt
	clockInterval      = time.Minute      // rafraîchissement des dates relatives (« il y a 5 min »)
	loadTimeout        = 30 * time.Second // durée maximale d'une lecture de la base
)

// view est une vue du TUI (onglet).
type view int

// Vues, dans l'ordre des onglets (touches 1, 2, 3).
const (
	viewRepos view = iota
	viewCommits
	viewContributors
	viewCount // nombre de vues
)

// title renvoie le nom de l'onglet.
func (v view) title() string {
	switch v {
	case viewRepos:
		return "Dépôts"
	case viewCommits:
		return "Commits"
	default:
		return "Contributeurs"
	}
}

// listState est la position dans une liste : ligne sélectionnée, première ligne affichée et filtre.
type listState struct {
	cursor int    // indice de la ligne sélectionnée parmi les lignes visibles (après filtre)
	offset int    // indice de la première ligne affichée
	filter string // filtre appliqué (sous-chaîne, sans tenir compte de la casse) ; vide = aucun
}

// detailState est le détail d'un dépôt ouvert par « entrée ».
type detailState struct {
	open     bool
	repoID   int64
	loading  bool
	commits  []store.CommitView
	contribs []store.ContributorStats
	err      error
	offset   int // première ligne affichée (défilement)
}

// syncState est l'état de la synchro lancée par le TUI (une seule à la fois).
type syncState struct {
	running bool
	gen     int            // numéro de la synchro, pour ignorer les messages d'une synchro passée
	ch      <-chan tea.Msg // messages de la synchro en cours (progression, fin)
	label   string         // « tous les dépôts » ou nom du dépôt
	auto    bool           // synchro automatique
	done    int            // dépôts terminés
	total   int            // dépôts à traiter (0 tant qu'inconnu)
	current string         // dernier dépôt commencé
	lastAt  time.Time      // fin de la dernière synchro lancée par le TUI ; zéro si aucune
}

// tickFunc programme un message après une durée (tea.Tick ; remplacée dans les tests).
type tickFunc func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd

// Model est le modèle racine du TUI (tea.Model). Construire avec NewModel.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	deps   Deps
	keys   keyMap
	st     styles
	tick   tickFunc

	width, height int
	view          view
	lists         [viewCount]listState

	repos    []store.RepositorySummary
	commits  []store.CommitView
	contribs []store.ContributorStats
	loaded   bool  // au moins une lecture réussie
	loading  bool  // lecture en cours
	loadErr  error // erreur de la dernière lecture

	detail    detailState
	filtering bool // saisie du filtre en cours (« / »)
	showHelp  bool // écran d'aide (« ? »)

	sync     syncState
	tracker  *syncTracker // goroutines de synchro, attendues par Run en quittant
	nextAuto time.Time    // prochaine synchro automatique ; zéro si synchro manuelle seule

	status    string // message du bas de l'écran
	statusErr bool   // le message est une erreur
}

// Vérifie à la compilation que Model implémente tea.Model.
var _ tea.Model = Model{}

// NewModel construit le modèle racine ; ctx borne les synchros et lectures lancées par le TUI, qui
// s'arrêtent aussi quand on quitte (« q », ctrl+c).
func NewModel(ctx context.Context, deps Deps) Model {
	if deps.Location == nil {
		deps.Location = time.Local
	}
	if deps.CommitLimit <= 0 {
		deps.CommitLimit = defaultCommitLimit
	}
	ctx, cancel := context.WithCancel(ctx)
	m := Model{
		ctx:     ctx,
		cancel:  cancel,
		deps:    deps,
		keys:    defaultKeyMap(),
		st:      newStyles(deps.Color),
		tick:    tea.Tick,
		tracker: &syncTracker{},
		loading: true,
	}
	if deps.Interval > 0 {
		m.nextAuto = m.now().Add(deps.Interval)
	}
	return m
}

// Messages internes.
type (
	// dataMsg apporte les données des trois vues.
	dataMsg struct {
		repos    []store.RepositorySummary
		commits  []store.CommitView
		contribs []store.ContributorStats
		err      error
	}
	// detailMsg apporte les commits et contributeurs du dépôt détaillé.
	detailMsg struct {
		repoID   int64
		commits  []store.CommitView
		contribs []store.ContributorStats
		err      error
	}
	// autoSyncMsg déclenche la synchro automatique.
	autoSyncMsg struct{}
	// clockMsg rafraîchit les dates relatives.
	clockMsg struct{}
)

// Init charge les données, programme la première synchro automatique si Deps.Interval > 0 et le
// rafraîchissement des dates relatives.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.loadCmd(), m.clockCmd()}
	if m.deps.Interval > 0 {
		cmds = append(cmds, m.autoCmd())
	}
	return tea.Batch(cmds...)
}

// now renvoie l'heure courante (Deps.Now, sinon time.Now).
func (m Model) now() time.Time {
	if m.deps.Now != nil {
		return m.deps.Now()
	}
	return time.Now()
}

// autoCmd programme la prochaine synchro automatique.
func (m Model) autoCmd() tea.Cmd {
	return m.tick(m.deps.Interval, func(time.Time) tea.Msg { return autoSyncMsg{} })
}

// clockCmd programme le prochain rafraîchissement des dates relatives.
func (m Model) clockCmd() tea.Cmd {
	return m.tick(clockInterval, func(time.Time) tea.Msg { return clockMsg{} })
}

// loadCmd relit les trois vues dans la base.
func (m Model) loadCmd() tea.Cmd {
	ctx, data, limit := m.ctx, m.deps.Data, m.deps.CommitLimit
	return func() tea.Msg {
		if data == nil {
			return dataMsg{}
		}
		ctx, cancel := context.WithTimeout(ctx, loadTimeout)
		defer cancel()
		var msg dataMsg
		if msg.repos, msg.err = data.RepositorySummaries(ctx); msg.err != nil {
			return msg
		}
		if msg.commits, msg.err = data.ListRecentCommits(ctx, store.CommitFilter{Limit: limit}); msg.err != nil {
			return msg
		}
		msg.contribs, msg.err = data.ContributorRanking(ctx, store.ContributorFilter{})
		return msg
	}
}

// detailCmd lit les derniers commits et les principaux contributeurs du dépôt repoID.
func (m Model) detailCmd(repoID int64) tea.Cmd {
	ctx, data := m.ctx, m.deps.Data
	return func() tea.Msg {
		msg := detailMsg{repoID: repoID}
		if data == nil {
			return msg
		}
		ctx, cancel := context.WithTimeout(ctx, loadTimeout)
		defer cancel()
		if msg.commits, msg.err = data.ListRecentCommits(ctx,
			store.CommitFilter{RepoID: repoID, Limit: detailCommits}); msg.err != nil {
			return msg
		}
		msg.contribs, msg.err = data.ContributorRanking(ctx,
			store.ContributorFilter{RepoID: repoID, Limit: detailContributors})
		return msg
	}
}

// Update traite un message (touches, taille du terminal, données chargées, progression de synchro).
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampAll()
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case dataMsg:
		m.loading = false
		if msg.err != nil {
			if m.ctx.Err() != nil {
				return m, nil // on quitte : rien à signaler
			}
			m.loadErr = msg.err
			m.setError("lecture de la base impossible : " + msg.err.Error())
			return m, nil
		}
		m.loadErr = nil
		m.loaded = true
		m.repos, m.commits, m.contribs = msg.repos, msg.commits, msg.contribs
		m.clampAll()
		return m, nil
	case detailMsg:
		if !m.detail.open || msg.repoID != m.detail.repoID {
			return m, nil
		}
		m.detail.loading = false
		m.detail.commits, m.detail.contribs, m.detail.err = msg.commits, msg.contribs, msg.err
		return m, nil
	case syncProgressMsg:
		return m.handleSyncProgress(msg)
	case syncDoneMsg:
		return m.handleSyncDone(msg)
	case autoSyncMsg:
		// La synchro automatique suivante est programmée tout de suite : une synchro manuelle ne la
		// décale pas, et une synchro longue ne la retarde pas.
		m.nextAuto = m.now().Add(m.deps.Interval)
		next := m.autoCmd()
		if m.sync.running {
			return m, next // une synchro est déjà en cours : celle-ci est sautée
		}
		var cmd tea.Cmd
		m, cmd = m.startSync(0, true)
		return m, tea.Batch(next, cmd)
	case clockMsg:
		return m, m.clockCmd()
	}
	return m, nil
}

// handleKey traite une touche.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m.quit()
	}
	if m.filtering {
		return m.handleFilterKey(msg), nil
	}
	k := m.keys
	switch {
	case key.Matches(msg, k.Quit):
		return m.quit()
	case key.Matches(msg, k.Help):
		m.showHelp = !m.showHelp
		return m, nil
	}
	if m.showHelp {
		if key.Matches(msg, k.Back) {
			m.showHelp = false
		}
		return m, nil
	}

	switch {
	case key.Matches(msg, k.NextView):
		m.switchView((m.view + 1) % viewCount)
	case key.Matches(msg, k.PrevView):
		m.switchView((m.view + viewCount - 1) % viewCount)
	case key.Matches(msg, k.ViewRepos):
		m.switchView(viewRepos)
	case key.Matches(msg, k.ViewCommits):
		m.switchView(viewCommits)
	case key.Matches(msg, k.ViewContribs):
		m.switchView(viewContributors)
	case key.Matches(msg, k.Up):
		m.move(-1)
	case key.Matches(msg, k.Down):
		m.move(1)
	case key.Matches(msg, k.PageUp):
		m.move(-m.pageSize())
	case key.Matches(msg, k.PageDown):
		m.move(m.pageSize())
	case key.Matches(msg, k.Top):
		m.move(-1 << 30)
	case key.Matches(msg, k.Bottom):
		m.move(1 << 30)
	case key.Matches(msg, k.Open):
		return m.openDetail()
	case key.Matches(msg, k.Back):
		switch {
		case m.detail.open:
			m.detail = detailState{}
		case m.lists[m.view].filter != "":
			m.setFilter("")
		}
	case key.Matches(msg, k.Filter):
		if !m.detail.open {
			m.filtering = true
		}
	case key.Matches(msg, k.SyncAll):
		return m.startSync(0, false)
	case key.Matches(msg, k.SyncRepo):
		id, ok := m.selectedRepoID()
		if !ok {
			m.setError("aucun dépôt sélectionné : choisissez-en un dans la vue Dépôts ou Commits")
			return m, nil
		}
		return m.startSync(id, false)
	case key.Matches(msg, k.Reload):
		m.loading = true
		cmds := []tea.Cmd{m.loadCmd()}
		if m.detail.open {
			m.detail.loading = true
			cmds = append(cmds, m.detailCmd(m.detail.repoID))
		}
		return m, tea.Batch(cmds...)
	}
	return m, nil
}

// handleFilterKey traite une touche pendant la saisie du filtre : le filtre s'applique au fil de la
// frappe ; entrée le garde, échap l'efface.
func (m Model) handleFilterKey(msg tea.KeyPressMsg) Model {
	cur := m.lists[m.view].filter
	switch msg.String() {
	case "enter":
		m.filtering = false
	case "esc":
		m.filtering = false
		m.setFilter("")
	case "backspace":
		if r := []rune(cur); len(r) > 0 {
			m.setFilter(string(r[:len(r)-1]))
		}
	case "ctrl+u":
		m.setFilter("")
	default:
		if msg.Text != "" {
			m.setFilter(cur + clean(msg.Text))
		}
	}
	return m
}

// quit arrête les lectures et la synchro en cours, puis quitte.
func (m Model) quit() (tea.Model, tea.Cmd) {
	if m.cancel != nil {
		m.cancel()
	}
	return m, tea.Quit
}

// setStatus affiche un message d'information en bas de l'écran.
func (m *Model) setStatus(s string) {
	m.status, m.statusErr = s, false
}

// setError affiche un message d'erreur en bas de l'écran.
func (m *Model) setError(s string) {
	m.status, m.statusErr = s, true
}

// switchView affiche la vue v et ferme le détail.
func (m *Model) switchView(v view) {
	m.view = v
	m.detail = detailState{}
	m.clamp(v)
}

// setFilter change le filtre de la vue courante et revient en haut de la liste.
func (m *Model) setFilter(f string) {
	ls := &m.lists[m.view]
	ls.filter = f
	ls.cursor, ls.offset = 0, 0
}

// move déplace la sélection de delta lignes (ou le défilement du détail).
func (m *Model) move(delta int) {
	if m.detail.open {
		m.detail.offset = max(m.detail.offset+delta, 0) // borné à l'affichage
		return
	}
	m.lists[m.view].cursor += delta
	m.clamp(m.view)
}

// pageSize renvoie le nombre de lignes d'une page de liste.
func (m Model) pageSize() int {
	return max(m.listHeight()-1, 1)
}

// clampAll recadre la sélection de toutes les vues (après un rechargement ou un redimensionnement).
func (m *Model) clampAll() {
	for v := range viewCount {
		m.clamp(v)
	}
}

// clamp garde la sélection de la vue v dans la liste et visible à l'écran.
func (m *Model) clamp(v view) {
	ls := &m.lists[v]
	n := len(m.visible(v))
	ls.cursor = min(max(ls.cursor, 0), max(n-1, 0))
	h := m.listHeight()
	if ls.cursor < ls.offset {
		ls.offset = ls.cursor
	}
	if ls.cursor >= ls.offset+h {
		ls.offset = ls.cursor - h + 1
	}
	ls.offset = min(max(ls.offset, 0), max(n-h, 0))
}

// visible renvoie les indices (dans les données de la vue v) des lignes qui passent le filtre.
func (m Model) visible(v view) []int {
	f := strings.ToLower(m.lists[v].filter)
	var n int
	switch v {
	case viewRepos:
		n = len(m.repos)
	case viewCommits:
		n = len(m.commits)
	default:
		n = len(m.contribs)
	}
	out := make([]int, 0, n)
	for i := range n {
		if f == "" || strings.Contains(strings.ToLower(m.searchText(v, i)), f) {
			out = append(out, i)
		}
	}
	return out
}

// searchText renvoie le texte sur lequel porte le filtre pour la ligne i de la vue v.
func (m Model) searchText(v view, i int) string {
	switch v {
	case viewRepos:
		r := m.repos[i]
		return strings.Join([]string{r.DisplayName(), r.FullName(), r.LastCommit.AuthorName,
			r.LastCommit.AuthorLogin, m.evaluate(r).State.Label()}, "\n")
	case viewCommits:
		c := m.commits[i]
		return strings.Join([]string{c.RepoDisplayName(), c.RepoOwner + "/" + c.RepoName, c.AuthorName,
			c.AuthorLogin, c.SHA, firstLine(c.Message)}, "\n")
	default:
		c := m.contribs[i]
		return strings.Join([]string{c.Name, c.ProviderLogin}, "\n")
	}
}

// selected renvoie l'indice (dans les données de la vue courante) de la ligne sélectionnée.
func (m Model) selected() (int, bool) {
	vis := m.visible(m.view)
	c := m.lists[m.view].cursor
	if c < 0 || c >= len(vis) {
		return 0, false
	}
	return vis[c], true
}

// selectedRepoID renvoie le dépôt sélectionné : celui du détail ouvert, de la ligne de la vue dépôts
// ou du commit sélectionné.
func (m Model) selectedRepoID() (int64, bool) {
	if m.detail.open {
		return m.detail.repoID, true
	}
	i, ok := m.selected()
	if !ok {
		return 0, false
	}
	switch m.view {
	case viewRepos:
		return m.repos[i].ID, true
	case viewCommits:
		return m.commits[i].RepoID, true
	}
	return 0, false
}

// openDetail ouvre le détail du dépôt sélectionné.
func (m Model) openDetail() (tea.Model, tea.Cmd) {
	if m.view == viewContributors || m.detail.open {
		return m, nil
	}
	id, ok := m.selectedRepoID()
	if !ok {
		return m, nil
	}
	if _, found := m.repoByID(id); !found {
		m.setError("ce dépôt n'est plus suivi : relisez la base (« r »)")
		return m, nil
	}
	m.detail = detailState{open: true, repoID: id, loading: true}
	return m, m.detailCmd(id)
}

// repoByID renvoie le résumé du dépôt id.
func (m Model) repoByID(id int64) (store.RepositorySummary, bool) {
	for _, r := range m.repos {
		if r.ID == id {
			return r, true
		}
	}
	return store.RepositorySummary{}, false
}
