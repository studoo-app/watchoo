package tui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// lineWith renvoie la première ligne de l'écran qui contient sub, ou "".
func lineWith(screen, sub string) string {
	for line := range strings.Lines(screen) {
		if strings.Contains(line, sub) {
			return strings.TrimRight(line, "\n")
		}
	}
	return ""
}

// mustContain vérifie que l'écran contient chacun des extraits.
func mustContain(t *testing.T, screen string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(screen, p) {
			t.Errorf("l'écran ne contient pas %q :\n%s", p, screen)
		}
	}
}

// mustNotContain vérifie que l'écran ne contient aucun des extraits.
func mustNotContain(t *testing.T, screen string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if strings.Contains(screen, p) {
			t.Errorf("l'écran contient %q :\n%s", p, screen)
		}
	}
}

func TestKeyMsgStrings(t *testing.T) {
	for _, k := range []string{"enter", "esc", "tab", "shift+tab", "backspace", "up", "down", "pgdown",
		"ctrl+c", "ctrl+u", "q", "S", "/", "?", "1"} {
		if got := keyMsg(k).String(); got != k {
			t.Errorf("keyMsg(%q).String() = %q", k, got)
		}
	}
}

func TestReposViewAndHeader(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	screen := l.screen()

	mustContain(t, screen, "█▄    ▄█", "█ ●██● █", "Watchoo v0.1.0", "licence-projet",
		"[1 Dépôts]", "2 Commits", "3 Contributeurs", "Synchro manuelle (s)",
		"DÉPÔT", "DERNIER COMMIT", "AUTEUR", "ÉTAT")
	if strings.Contains(screen, "\x1b") {
		t.Errorf("écran sans couleur avec une séquence d'échappement :\n%q", screen)
	}

	tests := []struct{ repo, when, state string }{
		{"Alice Martin", "il y a 2 h", "-"},
		{"promo/bob", "il y a 9 j", "~ 9 j"},
		{"promo/chloe", "il y a 42 j", "⚠ 42 j"},
		{"promo/dan", "jamais", "jamais"},
	}
	for _, tt := range tests {
		line := lineWith(screen, tt.repo)
		if line == "" || !strings.Contains(line, tt.when) || !strings.HasSuffix(line, tt.state) {
			t.Errorf("ligne de %s = %q ; attendu %q et l'état %q à la fin", tt.repo, line, tt.when, tt.state)
		}
	}
	// La première ligne est sélectionnée.
	if line := lineWith(screen, "Alice Martin"); !strings.HasPrefix(line, "› ") {
		t.Errorf("ligne sélectionnée = %q ; attendu le marqueur « › »", line)
	}
	// L'écran a exactement la hauteur du terminal.
	if n := strings.Count(screen, "\n") + 1; n != defaultHeight {
		t.Errorf("écran de %d lignes ; attendu %d", n, defaultHeight)
	}
	if v := l.m.View(); !v.AltScreen {
		t.Error("View().AltScreen = false ; attendu l'écran alternatif")
	}
}

func TestASCIIAndColor(t *testing.T) {
	deps := testDeps(newFakeData(), &fakeSyncer{})
	deps.Unicode = false
	l := newLoop(t, deps)
	screen := l.screen()
	mustContain(t, screen, `|\    /|`, "| o##o |", "! 42 j", "> Alice Martin")
	mustNotContain(t, screen, "⚠", "●", "›")

	deps = testDeps(newFakeData(), &fakeSyncer{})
	deps.Color = true
	l = newLoop(t, deps)
	if screen := l.screen(); !strings.Contains(screen, "\x1b[") {
		t.Errorf("écran en couleur sans séquence d'échappement :\n%s", screen)
	}
}

func TestNavigationBetweenViews(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))

	l.press("tab")
	screen := l.screen()
	mustContain(t, screen, "[2 Commits]", "MESSAGE", "Ajoute la page d'accueil", "1111111", "rouge corrige")
	mustNotContain(t, screen, "Détails", "\x1b", "[31m")

	l.press("tab")
	screen = l.screen()
	mustContain(t, screen, "[3 Contributeurs]", "CONTRIBUTEUR", "alice-gh", "+10/-2")
	if line := lineWith(screen, "Bob"); !strings.Contains(line, " - ") {
		t.Errorf("ligne de Bob = %q ; attendu « - » pour un login absent", line)
	}

	l.press("tab")
	mustContain(t, l.screen(), "[1 Dépôts]")
	l.press("shift+tab")
	mustContain(t, l.screen(), "[3 Contributeurs]")
	l.press("1")
	mustContain(t, l.screen(), "[1 Dépôts]")
	l.press("2")
	mustContain(t, l.screen(), "[2 Commits]")
	l.press("3")
	mustContain(t, l.screen(), "[3 Contributeurs]")
}

func TestCursorMovement(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("j", "down")
	if line := lineWith(l.screen(), "promo/chloe"); !strings.HasPrefix(line, "› ") {
		t.Errorf("après deux descentes, ligne = %q ; attendu chloe sélectionné", line)
	}
	l.press("j", "j", "j") // borné à la dernière ligne
	if c := l.m.lists[viewRepos].cursor; c != 3 {
		t.Errorf("curseur = %d ; attendu 3 (dernière ligne)", c)
	}
	l.press("k", "up", "up", "up", "up")
	if c := l.m.lists[viewRepos].cursor; c != 0 {
		t.Errorf("curseur = %d ; attendu 0", c)
	}
	l.press("G")
	if c := l.m.lists[viewRepos].cursor; c != 3 {
		t.Errorf("après G, curseur = %d ; attendu 3", c)
	}
	l.press("g")
	if c := l.m.lists[viewRepos].cursor; c != 0 {
		t.Errorf("après g, curseur = %d ; attendu 0", c)
	}
}

func TestScrollingSmallTerminal(t *testing.T) {
	data := newFakeData()
	for i := range 30 {
		data.repos = append(data.repos, summary(int64(10+i), "promo", "eleve-"+string(rune('a'+i%26))+string(rune('a'+i/26)),
			"", testNow.Add(-time.Hour), "Élève"))
	}
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.send(tea.WindowSizeMsg{Width: 60, Height: 12})
	for range 12 {
		l.press("pgdown")
	}
	screen := l.screen()
	if n := strings.Count(screen, "\n") + 1; n != 12 {
		t.Errorf("écran de %d lignes ; attendu 12", n)
	}
	if c := l.m.lists[viewRepos].cursor; c != len(data.repos)-1 {
		t.Errorf("curseur = %d ; attendu la dernière ligne (%d)", c, len(data.repos)-1)
	}
	if !strings.Contains(screen, "› ") {
		t.Errorf("la ligne sélectionnée n'est pas visible :\n%s", screen)
	}
	for line := range strings.Lines(screen) {
		if w := width(strings.TrimRight(line, "\n")); w > 60 && !strings.Contains(line, "Watchoo") {
			t.Errorf("ligne de %d colonnes pour un terminal de 60 : %q", w, line)
		}
	}
	// Un terminal minuscule ne fait pas paniquer le rendu.
	l.send(tea.WindowSizeMsg{Width: 10, Height: 3})
	_ = l.screen()
}

func TestFilter(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("/", "B", "o")
	screen := l.screen()
	mustContain(t, screen, "promo/bob", "Filtre : Bo")
	mustNotContain(t, screen, "Alice Martin", "promo/chloe")

	l.press("backspace", "backspace", "c", "h", "l", "enter")
	screen = l.screen()
	mustContain(t, screen, "promo/chloe", "Filtre : chl (échap pour l'effacer)")
	mustNotContain(t, screen, "promo/bob")
	if l.m.filtering {
		t.Error("saisie du filtre toujours active après entrée")
	}
	// « S » synchronise la ligne filtrée sélectionnée : chloe.
	if id, ok := l.m.selectedRepoID(); !ok || id != 3 {
		t.Errorf("dépôt sélectionné = %d, %v ; attendu 3", id, ok)
	}

	l.press("esc")
	mustContain(t, l.screen(), "promo/bob", "Alice Martin")

	// Aucun résultat, puis échap pendant la saisie efface le filtre.
	l.press("/", "z", "z")
	mustContain(t, l.screen(), "Aucune ligne ne correspond au filtre « zz »")
	l.press("esc")
	mustContain(t, l.screen(), "Alice Martin")

	// Pendant la saisie, « q » s'écrit dans le filtre, ctrl+c quitte.
	l.press("/", "q")
	if l.quit || l.m.lists[viewRepos].filter != "q" {
		t.Errorf("« q » pendant la saisie : quit=%v, filtre=%q", l.quit, l.m.lists[viewRepos].filter)
	}
	l.press("ctrl+u")
	if f := l.m.lists[viewRepos].filter; f != "" {
		t.Errorf("après ctrl+u, filtre = %q", f)
	}
	l.press("ctrl+c")
	l.settle()
	if !l.quit {
		t.Error("ctrl+c pendant la saisie n'a pas quitté")
	}
}

func TestFilterIsPerView(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("/", "b", "o", "b", "enter", "tab")
	mustContain(t, l.screen(), "Ajoute la page d'accueil")
	l.press("/", "a", "l", "i", "enter")
	screen := l.screen()
	mustContain(t, screen, "Ajoute la page d'accueil")
	mustNotContain(t, screen, "corrige")
	l.press("shift+tab")
	screen = l.screen()
	mustContain(t, screen, "promo/bob")
	mustNotContain(t, screen, "Alice Martin")
}

func TestDetail(t *testing.T) {
	data := newFakeData()
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	screen := l.screen()
	mustContain(t, screen, "Alice Martin (promo/alice)", "https://github.com/promo/alice", "Actif",
		"seuils : Flemme après 5 j, Dead après 15 j", "2 branches suivies, par défaut : main, toutes les branches",
		"il y a 2 h par Alice (abcdef0)", "réussie (1 nouveau commit)",
		"Derniers commits", "Ajoute la page d'accueil", "Principaux contributeurs")
	mustNotContain(t, screen, "corrige") // commit d'un autre dépôt

	// Activité par semaine des contributeurs du dépôt (12 semaines, la plus récente à droite) : Alice
	// 3 commits (2 cette semaine, 1 il y a deux semaines) ; Bob, son seul commit hors fenêtre.
	mustContain(t, screen, "activité sur 12 semaines, la plus récente à droite", "12 SEM.", "PAR SEMAINE")
	alice := lineWith(screen, "▁▁▁▁▁▁▁▁▁▅▁█")
	if !strings.Contains(alice, "Alice") || !strings.Contains(alice, " 3 ") {
		t.Errorf("ligne d'Alice = %q ; attendu 3 commits sur 12 semaines et la courbe ▁▁▁▁▁▁▁▁▁▅▁█", alice)
	}
	bob := lineWith(screen, "▁▁▁▁▁▁▁▁▁▁▁▁")
	if !strings.Contains(bob, "Bob") || !strings.Contains(bob, " 0 ") {
		t.Errorf("ligne de Bob = %q ; attendu 0 commit sur 12 semaines", bob)
	}

	data.mu.Lock()
	if !slices.Equal(data.authorReads, []int64{1}) {
		t.Errorf("AuthorCommitTimes lu pour %v, attendu le seul dépôt 1", data.authorReads)
	}
	last := data.commitFilters[len(data.commitFilters)-1]
	data.mu.Unlock()
	if last.RepoID != 1 || last.Limit != detailCommits {
		t.Errorf("filtre du détail = %+v ; attendu RepoID 1, Limit %d", last, detailCommits)
	}

	l.press("esc")
	if l.m.detail.open {
		t.Error("échap n'a pas fermé le détail")
	}
	mustContain(t, l.screen(), "DERNIER COMMIT")

	// Entrée sur un commit ouvre le détail de son dépôt.
	l.press("tab", "j", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "promo/bob", "Flemme (~ 9 j)")
	// Entrée sur un contributeur ne fait rien.
	l.press("3", "enter")
	if l.m.detail.open {
		t.Error("entrée dans la vue contributeurs a ouvert un détail")
	}
}

func TestDetailGrayAndError(t *testing.T) {
	data := newFakeData()
	data.repos[2].LastRun = model.SyncRun{ID: 9, StartedAt: testNow.Add(-5 * time.Minute), Status: model.SyncError,
		Error: "dépôt introuvable ou inaccessible"}
	data.repos[2].Enabled = false
	data.repos[2].BranchPatterns = []string{"main", "release/*"}
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	if line := lineWith(l.screen(), "promo/chloe"); !strings.HasSuffix(line, "arrêté") {
		t.Errorf("ligne de chloe = %q ; attendu l'état gris « arrêté »", line)
	}
	l.press("j", "j", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "Suivi arrêté", "motifs : main, release/*",
		"il y a 5 min, erreur : dépôt introuvable ou inaccessible", "suivi arrêté")
}

func TestSyncAllWithProgress(t *testing.T) {
	data := newFakeData()
	gate := make(chan struct{})
	syncer := &fakeSyncer{
		repos: []wsync.RepoReport{
			{RepoID: 1, DisplayName: "Alice Martin", Status: wsync.StatusSuccess, NewCommits: 2},
			{RepoID: 2, DisplayName: "promo/bob", Status: wsync.StatusSuccess, NewCommits: 1},
		},
		gate: gate,
	}
	syncer.report = wsync.Report{Repos: syncer.repos}
	l := newLoop(t, testDeps(data, syncer))
	loadsBefore := data.loadCount()

	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 && m.sync.current == "promo/bob" })
	mustContain(t, l.screen(), "Synchro en cours : 1/2 dépôts (promo/bob)…")

	// Une deuxième demande pendant la synchro est ignorée et signalée.
	l.press("S")
	mustContain(t, l.screen(), "une synchro est déjà en cours (tous les dépôts)")
	if all, repos := syncer.counts(); all != 1 || len(repos) != 0 {
		t.Errorf("SyncAll %d fois, SyncRepo %v ; attendu une seule synchro", all, repos)
	}

	close(gate)
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	screen := l.screen()
	mustContain(t, screen, "synchro terminée : 2 dépôts, 3 nouveaux commits", "Dernière synchro à l'instant")
	if data.loadCount() <= loadsBefore {
		t.Error("la base n'a pas été relue après la synchro")
	}
	if l.m.statusErr {
		t.Error("bilan sans erreur affiché comme une erreur")
	}
}

func TestSyncSelectedRepo(t *testing.T) {
	syncer := &fakeSyncer{
		repos: []wsync.RepoReport{{RepoID: 2, DisplayName: "promo/bob", Status: wsync.StatusError,
			Error: "dépôt introuvable ou inaccessible"}},
	}
	syncer.report = wsync.Report{Repos: syncer.repos}
	l := newLoop(t, testDeps(newFakeData(), syncer))
	l.press("j", "S")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	if _, repos := syncer.counts(); len(repos) != 1 || repos[0] != 2 {
		t.Errorf("SyncRepo appelé avec %v ; attendu [2]", repos)
	}
	mustContain(t, l.screen(), "promo/bob : dépôt introuvable ou inaccessible")
	if !l.m.statusErr {
		t.Error("échec d'un dépôt non affiché comme une erreur")
	}

	// Dans la vue contributeurs, aucun dépôt n'est sélectionné.
	l.press("3", "S")
	mustContain(t, l.screen(), "aucun dépôt sélectionné")
}

func TestSyncSummaryDeferredLocalTime(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("fuseau Europe/Paris indisponible")
	}
	resume := time.Date(2026, 10, 27, 13, 30, 0, 0, time.UTC) // 14:30 à Paris (heure d'hiver)
	syncer := &fakeSyncer{}
	syncer.report = wsync.Report{Repos: []wsync.RepoReport{
		{RepoID: 1, Status: wsync.StatusSuccess, NewCommits: 1},
		{RepoID: 2, Status: wsync.StatusError, Error: "erreur"},
		{RepoID: 3, Status: wsync.StatusLocked, Error: "déjà en cours de synchronisation"},
		{RepoID: 4, Status: wsync.StatusDeferred, ResumeAt: resume},
	}}
	deps := testDeps(newFakeData(), syncer)
	deps.Location = paris
	l := newLoop(t, deps)
	l.send(tea.WindowSizeMsg{Width: 200, Height: 30})
	l.press("s")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	mustContain(t, l.screen(), "synchro terminée : 4 dépôts, 1 nouveau commit, 1 en erreur, "+
		"1 déjà en cours ailleurs, 1 reporté (quota GitHub bas, reprise vers 14:30)")
	if !l.m.statusErr {
		t.Error("bilan avec erreurs non signalé comme une erreur")
	}

	// Un seul dépôt reporté : son heure de reprise locale.
	m := l.m
	text, bad := m.summarize(wsync.Report{Repos: []wsync.RepoReport{
		{DisplayName: "promo/bob", Status: wsync.StatusDeferred, ResumeAt: resume}}})
	if text != "promo/bob : reporté : quota GitHub bas jusqu'à 14:30" || !bad {
		t.Errorf("summarize = %q, %v", text, bad)
	}
	if text, _ := m.summarize(wsync.Report{}); text != "synchro terminée : aucun dépôt suivi" {
		t.Errorf("summarize(vide) = %q", text)
	}
}

func TestSyncGlobalError(t *testing.T) {
	syncer := &fakeSyncer{err: errors.New("gh n'est pas connecté : lancez « gh auth login »")}
	l := newLoop(t, testDeps(newFakeData(), syncer))
	l.press("s")
	l.until(func(m Model) bool { return !m.sync.running })
	mustContain(t, l.screen(), "synchro impossible : gh n'est pas connecté")
}

func TestAutoSync(t *testing.T) {
	syncer := &fakeSyncer{repos: []wsync.RepoReport{{RepoID: 1, Status: wsync.StatusSuccess}}}
	syncer.report = wsync.Report{Repos: syncer.repos}
	deps := testDeps(newFakeData(), syncer)
	deps.Interval = 30 * time.Minute
	l := newLoop(t, deps)
	l.settle()

	ticks := l.ticksOf(30 * time.Minute)
	if len(ticks) != 1 {
		t.Fatalf("%d synchros automatiques programmées ; attendu 1", len(ticks))
	}
	mustContain(t, l.screen(), "Synchro auto toutes les 30 min, prochaine à 12:30")

	l.send(ticks[0].msg)
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	if all, _ := syncer.counts(); all != 1 {
		t.Errorf("SyncAll appelé %d fois ; attendu 1 (synchro automatique)", all)
	}
	if n := len(l.ticksOf(30 * time.Minute)); n != 2 {
		t.Errorf("%d synchros automatiques programmées ; attendu 2 (la suivante)", n)
	}

	// Une synchro automatique pendant une synchro en cours est sautée, la suivante reste programmée.
	gate := make(chan struct{})
	syncer.gate = gate
	syncer.repos = append(syncer.repos, wsync.RepoReport{RepoID: 2})
	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 })
	l.send(autoSyncMsg{})
	l.until(func(Model) bool { return len(l.ticksOf(30*time.Minute)) == 3 })
	close(gate)
	l.until(func(m Model) bool { return !m.sync.running })
	if all, _ := syncer.counts(); all != 2 {
		t.Errorf("SyncAll appelé %d fois ; attendu 2 (la synchro automatique sautée)", all)
	}
}

func TestManualOnlyHasNoAutoTick(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.settle()
	for _, tk := range l.ticks {
		if tk.d != clockInterval {
			t.Errorf("tick inattendu de %s en synchro manuelle", tk.d)
		}
	}
	if len(l.ticksOf(clockInterval)) != 1 {
		t.Error("rafraîchissement des dates non programmé")
	}
	l.send(clockMsg{})
	l.until(func(Model) bool { return len(l.ticksOf(clockInterval)) == 2 })
}

func TestQuitCancelsContext(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	ctx := l.m.ctx
	l.press("q")
	l.settle()
	if !l.quit {
		t.Error("« q » n'a pas quitté")
	}
	if ctx.Err() == nil {
		t.Error("le contexte du modèle n'est pas annulé en quittant")
	}
}

func TestQuitStopsRunningSync(t *testing.T) {
	gate := make(chan struct{}) // jamais fermé : seule l'annulation arrête la synchro
	syncer := &fakeSyncer{repos: []wsync.RepoReport{{RepoID: 1}, {RepoID: 2}}, gate: gate}
	l := newLoop(t, testDeps(newFakeData(), syncer))
	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 })
	l.press("q")
	l.settle()
	if !l.quit {
		t.Error("« q » n'a pas quitté pendant la synchro")
	}
	if strings.Contains(l.m.status, "impossible") {
		t.Errorf("erreur affichée en quittant : %q", l.m.status)
	}
}

func TestHelpScreen(t *testing.T) {
	l := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l.press("?")
	mustContain(t, l.screen(), "Aide des touches", "synchroniser tout", "synchroniser ce dépôt", "relire la base",
		"quitter")
	l.press("j") // ignoré
	l.press("esc")
	mustContain(t, l.screen(), "DERNIER COMMIT")
	l.press("?", "?")
	mustNotContain(t, l.screen(), "Aide des touches")
}

func TestReloadAndLoadError(t *testing.T) {
	data := newFakeData()
	data.err = errBoom
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	mustContain(t, l.screen(), "Impossible de lire la base", "lecture de la base impossible : base indisponible")

	data.mu.Lock()
	data.err = nil
	data.mu.Unlock()
	l.press("r")
	l.until(func(m Model) bool { return !m.loading })
	mustContain(t, l.screen(), "Alice Martin")
}

func TestEmptyWorkspace(t *testing.T) {
	l := newLoop(t, testDeps(&fakeData{}, &fakeSyncer{}))
	mustContain(t, l.screen(), "Aucun dépôt suivi", "watchoo repo add owner/repo")
	l.press("tab")
	mustContain(t, l.screen(), "Aucun commit en base")
	l.press("tab")
	mustContain(t, l.screen(), "Aucun contributeur en base")
	l.press("enter", "S")
	if l.m.detail.open {
		t.Error("détail ouvert sans dépôt")
	}
}

func TestNewModelDefaults(t *testing.T) {
	m := NewModel(t.Context(), Deps{})
	if m.deps.Location != time.Local || m.deps.CommitLimit != defaultCommitLimit {
		t.Errorf("valeurs par défaut : Location=%v, CommitLimit=%d", m.deps.Location, m.deps.CommitLimit)
	}
	if m.Init() == nil {
		t.Error("Init ne renvoie aucune commande")
	}
	// Sans Syncer, la synchro est refusée proprement.
	next, _ := m.startSync(0, false)
	if !strings.Contains(next.status, "indisponible") {
		t.Errorf("status = %q", next.status)
	}
}

func TestHelpers(t *testing.T) {
	if got := truncate("abcdef", 4, "…"); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("abc", 1, "…"); got != "a" {
		t.Errorf("truncate étroit = %q", got)
	}
	if got := pad("12", 4, true, ""); got != "  12" {
		t.Errorf("pad à droite = %q", got)
	}
	if got := clean("a\x1b[31mb\u202ec\nd\x1b]8;;http://x\x07e\x1b[0m"); got != "ab c de" {
		t.Errorf("clean = %q", got)
	}
	if got := firstLine("\n  titre  \ncorps"); got != "titre" {
		t.Errorf("firstLine = %q", got)
	}
	for d, want := range map[time.Duration]string{15 * time.Minute: "15 min", time.Hour: "1 h",
		90 * time.Minute: "1 h 30"} {
		if got := formatInterval(d); got != want {
			t.Errorf("formatInterval(%s) = %q ; attendu %q", d, got, want)
		}
	}
	for v, want := range map[string]string{"0.1.0": "v0.1.0", "v0.1.0": "v0.1.0", "dev": "dev", "": ""} {
		if got := displayVersion(v); got != want {
			t.Errorf("displayVersion(%q) = %q", v, got)
		}
	}
	if newStyles(false).Title.Render("x") != "x" {
		t.Error("style sans couleur avec séquence d'échappement")
	}
}

// TestWithRealStore lit une vraie base SQLite temporaire, remplie par l'API du store.
func TestWithRealStore(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	repo, err := st.CreateRepository(ctx, model.Repository{Provider: model.ProviderGitHub, ProviderID: 42,
		Owner: "promo", Name: "zoe", Label: "Zoé Leroy", URL: "https://github.com/promo/zoe", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.BeginSyncRun(ctx, repo.ID, testNow.Add(-time.Minute), wsync.LockStaleAfter)
	if err != nil {
		t.Fatal(err)
	}
	zoe := store.Identity{Name: "Zoé", Email: "zoe@example.test", Login: "zoe-gh", ProviderUserID: 7}
	if _, err := st.ApplyRepoSync(ctx, store.RepoSyncWrite{
		RepoID: repo.ID, RunID: run.ID,
		Metadata: store.RepositoryMetadata{Owner: "promo", Name: "zoe", DefaultBranch: "main",
			URL: "https://github.com/promo/zoe"},
		Branches: []store.BranchState{{Name: "main", HeadSHA: "cafe000000000000000000000000000000000001"}},
		Commits: []store.CommitWrite{{SHA: "cafe000000000000000000000000000000000001", Message: "Premier rendu",
			Author: zoe, Committer: zoe, AuthoredAt: testNow.Add(-3 * time.Hour),
			CommittedAt: testNow.Add(-3 * time.Hour), Branches: []string{"main"}}},
		SyncedAt: testNow,
	}); err != nil {
		t.Fatal(err)
	}

	l := newLoop(t, testDeps(st, &fakeSyncer{}))
	if line := lineWith(l.screen(), "Zoé Leroy"); !strings.Contains(line, "il y a 3 h") ||
		!strings.Contains(line, "Zoé") || !strings.HasSuffix(line, "-") {
		t.Errorf("ligne du dépôt = %q", line)
	}
	l.press("tab")
	mustContain(t, l.screen(), "Premier rendu", "cafe000")
	l.press("tab")
	mustContain(t, l.screen(), "zoe-gh")
	l.press("1", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "1 branche suivie, par défaut : main", "Premier rendu", "Principaux contributeurs")
	mustNotContain(t, l.screen(), "zoe@example.test")
	// Activité par semaine lue dans la vraie base : le commit de Zoé, cette semaine.
	if line := lineWith(l.screen(), "▁▁▁▁▁▁▁▁▁▁▁█"); !strings.Contains(line, "Zoé") {
		t.Errorf("ligne de Zoé = %q ; attendu la courbe ▁▁▁▁▁▁▁▁▁▁▁█", line)
	}
}

// Seuils propres au dépôt : appliqués et signalés dans le détail.
func TestDetailOwnThresholds(t *testing.T) {
	data := newFakeData()
	flemme := 1
	data.repos[0].FlemmeAfterDays = &flemme // Alice : dernier commit il y a 2 h, toujours Actif
	l := newLoop(t, testDeps(data, &fakeSyncer{}))
	l.press("enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	mustContain(t, l.screen(), "seuils : Flemme après 1 j, Dead après 15 j (propres au dépôt)")
}
