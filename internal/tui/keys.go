package tui

// Touches du TUI (bubbles/key), affichées par l'aide (« ? »).

import (
	"charm.land/bubbles/v2/key"

	"github.com/studoo-app/watchoo/internal/i18n"
)

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
	Add          key.Binding
	Reload       key.Binding
	Help         key.Binding
	Quit         key.Binding
}

// defaultKeyMap renvoie les touches par défaut, avec leur aide dans la langue du processus.
func defaultKeyMap() keyMap {
	return keyMap{
		NextView:     key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", i18n.T("tui.key.next_view"))),
		PrevView:     key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp(i18n.T("tui.keyname.shift_tab"), i18n.T("tui.key.prev_view"))),
		ViewRepos:    key.NewBinding(key.WithKeys("1"), key.WithHelp("1", i18n.T("tui.key.view_repos"))),
		ViewCommits:  key.NewBinding(key.WithKeys("2"), key.WithHelp("2", i18n.T("tui.key.view_commits"))),
		ViewContribs: key.NewBinding(key.WithKeys("3"), key.WithHelp("3", i18n.T("tui.key.view_contribs"))),
		Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", i18n.T("tui.key.up"))),
		Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", i18n.T("tui.key.down"))),
		PageUp:       key.NewBinding(key.WithKeys("pgup"), key.WithHelp(i18n.T("tui.keyname.pgup"), i18n.T("tui.key.page_up"))),
		PageDown:     key.NewBinding(key.WithKeys("pgdown"), key.WithHelp(i18n.T("tui.keyname.pgdown"), i18n.T("tui.key.page_down"))),
		Top:          key.NewBinding(key.WithKeys("home", "g"), key.WithHelp("g", i18n.T("tui.key.top"))),
		Bottom:       key.NewBinding(key.WithKeys("end", "G"), key.WithHelp("G", i18n.T("tui.key.bottom"))),
		Open:         key.NewBinding(key.WithKeys("enter"), key.WithHelp(i18n.T("tui.keyname.enter"), i18n.T("tui.key.open"))),
		Back:         key.NewBinding(key.WithKeys("esc"), key.WithHelp(i18n.T("tui.keyname.esc"), i18n.T("tui.key.back"))),
		Filter:       key.NewBinding(key.WithKeys("/"), key.WithHelp("/", i18n.T("tui.key.filter"))),
		SyncAll:      key.NewBinding(key.WithKeys("s"), key.WithHelp("s", i18n.T("tui.key.sync_all"))),
		SyncRepo:     key.NewBinding(key.WithKeys("S"), key.WithHelp("S", i18n.T("tui.key.sync_repo"))),
		Add:          key.NewBinding(key.WithKeys("a"), key.WithHelp("a", i18n.T("tui.key.add"))),
		Reload:       key.NewBinding(key.WithKeys("r"), key.WithHelp("r", i18n.T("tui.key.reload"))),
		Help:         key.NewBinding(key.WithKeys("?"), key.WithHelp("?", i18n.T("tui.key.help"))),
		Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", i18n.T("tui.key.quit"))),
	}
}

// helpGroups renvoie les touches regroupées par thème, dans l'ordre de l'écran d'aide.
func (k keyMap) helpGroups() []helpGroup {
	return []helpGroup{
		{Title: i18n.T("tui.help.group.views"), Bindings: []key.Binding{k.NextView, k.PrevView, k.ViewRepos, k.ViewCommits, k.ViewContribs}},
		{Title: i18n.T("tui.help.group.move"), Bindings: []key.Binding{k.Up, k.Down, k.PageUp, k.PageDown, k.Top, k.Bottom,
			k.Open, k.Back, k.Filter}},
		{Title: i18n.T("tui.help.group.sync"), Bindings: []key.Binding{k.SyncAll, k.SyncRepo, k.Add, k.Reload}},
		{Title: i18n.T("tui.help.group.general"), Bindings: []key.Binding{k.Help, k.Quit}},
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
		{k.NextView.Help().Key, i18n.T("tui.short.views")},
		{k.Open.Help().Key, i18n.T("tui.short.detail")},
		{k.Filter.Help().Key, i18n.T("tui.short.filter")},
		{k.SyncAll.Help().Key, i18n.T("tui.short.sync")},
		{k.SyncRepo.Help().Key, i18n.T("tui.short.sync_repo")},
		{k.Add.Help().Key, i18n.T("tui.short.add")},
		{k.Help.Help().Key, i18n.T("tui.short.help")},
		{k.Quit.Help().Key, i18n.T("tui.short.quit")},
	}
}
