package tui

// Touches du TUI (bubbles/key), affichées par l'aide (« ? »).

import "charm.land/bubbles/v2/key"

// keyMap regroupe les touches du TUI.
type keyMap struct {
	NextView     key.Binding
	PrevView     key.Binding
	ViewRepos    key.Binding
	ViewCommits  key.Binding
	ViewContribs key.Binding
	Up           key.Binding
	Down         key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	Top          key.Binding
	Bottom       key.Binding
	Open         key.Binding
	Back         key.Binding
	Filter       key.Binding
	SyncAll      key.Binding
	SyncRepo     key.Binding
	Reload       key.Binding
	Help         key.Binding
	Quit         key.Binding
}

// defaultKeyMap renvoie les touches par défaut, avec leur aide en français.
func defaultKeyMap() keyMap {
	return keyMap{
		NextView:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "vue suivante")),
		PrevView:     key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("maj+tab", "vue précédente")),
		ViewRepos:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", "vue dépôts")),
		ViewCommits:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", "vue commits")),
		ViewContribs: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", "vue contributeurs")),
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "monter")),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "descendre")),
		PageUp:       key.NewBinding(key.WithKeys("pgup"), key.WithHelp("pg.préc", "page précédente")),
		PageDown:     key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("pg.suiv", "page suivante")),
		Top:          key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", "début de la liste")),
		Bottom:       key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", "fin de la liste")),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp("entrée", "détail du dépôt")),
		Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp("échap", "retour, effacer le filtre")),
		Filter:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filtrer")),
		SyncAll:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "synchroniser tout")),
		SyncRepo:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", "synchroniser ce dépôt")),
		Reload:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "relire la base")),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "aide")),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quitter")),
	}
}

// helpGroups renvoie les touches regroupées par thème, dans l'ordre de l'écran d'aide.
func (k keyMap) helpGroups() []helpGroup {
	return []helpGroup{
		{Title: "Vues", Bindings: []key.Binding{k.NextView, k.PrevView, k.ViewRepos, k.ViewCommits, k.ViewContribs}},
		{Title: "Déplacement", Bindings: []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom,
			k.Open, k.Back, k.Filter}},
		{Title: "Synchro et données", Bindings: []key.Binding{k.SyncAll, k.SyncRepo, k.Reload}},
		{Title: "Général", Bindings: []key.Binding{k.Help, k.Quit}},
	}
}

// helpGroup est un groupe de touches de l'écran d'aide.
type helpGroup struct {
	Title    string
	Bindings []key.Binding
}

// shortHelp renvoie les touches rappelées en bas de l'écran, avec un libellé court (touche, libellé).
func (k keyMap) shortHelp() [][2]string {
	return [][2]string{
		{k.NextView.Help().Key, "vues"},
		{k.Open.Help().Key, "détail"},
		{k.Filter.Help().Key, "filtre"},
		{k.SyncAll.Help().Key, "synchro"},
		{k.SyncRepo.Help().Key, "synchro du dépôt"},
		{k.Help.Help().Key, "aide"},
		{k.Quit.Help().Key, "quitter"},
	}
}
