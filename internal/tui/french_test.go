package tui

// The French interface: the process language switched to French (not parallel), on a few key screens.

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

func TestFrenchScreens(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	// Header, tabs and list columns.
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	screen := l.screen()
	mustContain(t, screen, "[1 Dépôts]", "2 Commits", "3 Contributeurs", "Synchro manuelle (s)",
		"DÉPÔT", "DERNIER COMMIT", "AUTEUR", "ÉTAT", "il y a 2 h", "~ 9 j", "⚠ 42 j", "jamais")

	// Repository detail.
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "échap pour revenir", "Actif", "seuils : Flemme après 5 j, Dead après 15 j",
		"2 branches suivies, par défaut : main, toutes les branches", "il y a 2 h par Alice (abcdef0)",
		"réussie (1 nouveau commit)", "Derniers commits", "Principaux contributeurs",
		"activité sur 12 semaines, la plus récente à droite", "12 SEM.", "PAR SEMAINE")
	l.press("esc")

	// Key help.
	l.press("?")
	mustContain(t, l.screen(), "Aide des touches  (« ? » ou échap pour fermer)", "Déplacement",
		"maj+tab vue précédente", "entrée détail du dépôt ou du contributeur", "pg.préc", "synchroniser tout",
		"relire la base", "Le filtre (« / ») porte sur les colonnes de la vue courante.")
	l.press("esc")

	// Filter and empty list.
	l.press("/", "z", "z")
	mustContain(t, l.screen(), "Filtre : zz", "Aucune ligne ne correspond au filtre « zz » (échap pour l'effacer).")
	l.press("esc")

	// Sync summaries: French plural for zero, French typography before ":".
	text, _ := l.m.summarize(wsync.Report{Repos: []wsync.RepoReport{
		{DisplayName: "a", Status: wsync.StatusSuccess, Warnings: []model.SyncWarning{{Kind: model.WarnListsNotSynced, Message: "x"}}},
		{DisplayName: "b", Status: wsync.StatusSuccess}}})
	if want := "synchro terminée : 2 dépôts, 0 nouveau commit, 1 avertissement (pull requests, releases ou tags)"; text != want {
		t.Errorf("summarize = %q; want %q", text, want)
	}
	text, _ = l.m.summarize(wsync.Report{Repos: []wsync.RepoReport{{DisplayName: "promo/bob", Status: wsync.StatusLocked}}})
	if want := "promo/bob : déjà en cours de synchronisation"; text != want {
		t.Errorf("summarize = %q; want %q", text, want)
	}

	// Contributor detail.
	data := newFakeData()
	withContributorDetail(data)
	l = newLoop(t, testDeps(data, &fakeSyncer{}))
	l.send(tea.WindowSizeMsg{Width: 120, Height: 40})
	l.press("3", "enter")
	l.until(func(m Model) bool { return m.cdetail.open && !m.cdetail.loading })
	mustContain(t, l.screen(), "Langages          PHP 75,0 %, Go 25,0 %", "Identités Git", "2 adresses rattachées (non affichées)",
		"5 commits dans 2 dépôts, +40/-7 (2 non comptés)", "12 semaines", "Lignes +/- : comptées dans le clone")
}

func TestFrenchASCIIAndNotice(t *testing.T) {
	defer i18n.SetProcessForTest(i18n.French)()

	data := newFakeData()
	data.repos[1].Enabled = false // stopped tracking: short ASCII state without accent
	deps := testDeps(data, &fakeSyncer{})
	deps.Unicode = false
	l := newLoop(t, deps)
	screen := l.screen()
	mustContain(t, screen, "! 42 j", "> Alice Martin", "arrete", "[1 Dépôts]")
	mustNotContain(t, screen, "⚠", "›", "…")

	m := NewModel(t.Context(), deps)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	next, _ = next.(Model).Update(updateMsg{info: newVersion})
	want := "Nouvelle version de Watchoo disponible : v0.6.1 -> v0.6.2. Quittez puis lancez « watchoo update »."
	if got := noticeLine(next.(Model).View().Content); got != want {
		t.Errorf("notice = %q; want %q", got, want)
	}
	// At 80 columns, the short form keeps the instructions.
	next, _ = next.(Model).Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if got := noticeLine(next.(Model).View().Content); !strings.HasPrefix(got, "Watchoo v0.6.2 disponible : quittez") {
		t.Errorf("short notice = %q", got)
	}
}
