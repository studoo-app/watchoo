package tui

// Outils des tests du TUI : fausses données, faux Syncer, touches et petite boucle d'événements qui
// exécute les commandes Bubble Tea comme le ferait tea.Program (sans terminal).

import (
	"context"
	"errors"
	"slices"
	gosync "sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// testNow est l'heure fixe des tests.
var testNow = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

// fakeData est une DataSource en mémoire qui compte les lectures et garde les filtres reçus.
type fakeData struct {
	mu            gosync.Mutex
	repos         []store.RepositorySummary
	commits       []store.CommitView
	contribs      []store.ContributorStats
	err           error
	loads         int
	commitFilters []store.CommitFilter
	contribFilter []store.ContributorFilter
	// authorTimes : auteur et date des commits, par dépôt (AuthorCommitTimes).
	authorTimes map[int64][]model.AuthorCommitTime
	authorReads []int64 // dépôts demandés à AuthorCommitTimes
}

func (f *fakeData) RepositorySummaries(context.Context) ([]store.RepositorySummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads++
	return slices.Clone(f.repos), f.err
}

func (f *fakeData) ListRecentCommits(_ context.Context, flt store.CommitFilter) ([]store.CommitView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitFilters = append(f.commitFilters, flt)
	var out []store.CommitView
	for _, c := range f.commits {
		if flt.RepoID == 0 || c.RepoID == flt.RepoID {
			out = append(out, c)
		}
	}
	return out, f.err
}

func (f *fakeData) ContributorRanking(_ context.Context, flt store.ContributorFilter) ([]store.ContributorStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contribFilter = append(f.contribFilter, flt)
	return slices.Clone(f.contribs), f.err
}

func (f *fakeData) AuthorCommitTimes(_ context.Context, repoID int64, since time.Time) ([]model.AuthorCommitTime, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.authorReads = append(f.authorReads, repoID)
	var out []model.AuthorCommitTime
	for id, list := range f.authorTimes {
		if repoID != 0 && id != repoID {
			continue
		}
		for _, at := range list {
			if !at.CommittedAt.Before(since) {
				out = append(out, at)
			}
		}
	}
	return out, f.err
}

// loadCount renvoie le nombre de lectures complètes (RepositorySummaries).
func (f *fakeData) loadCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.loads
}

// summary construit le résumé d'un dépôt synchronisé (LastSyncedAt une heure avant testNow).
func summary(id int64, owner, name, label string, lastCommit time.Time, author string) store.RepositorySummary {
	r := store.RepositorySummary{
		Repository: model.Repository{
			ID: id, Provider: model.ProviderGitHub, ProviderID: 100 + id, Owner: owner, Name: name, Label: label,
			DefaultBranch: "main", URL: "https://github.com/" + owner + "/" + name,
			LastSyncedAt: testNow.Add(-time.Hour), LastCommitAt: lastCommit, Enabled: true,
		},
		Commits:         3,
		TrackedBranches: 2,
		LastRun:         model.SyncRun{ID: id, RepoID: id, StartedAt: testNow.Add(-time.Hour), Status: model.SyncSuccess, NewCommits: 1},
	}
	if !lastCommit.IsZero() {
		r.LastCommit = store.CommitView{
			Commit:     model.Commit{RepoID: id, SHA: "abcdef0123456789", CommittedAt: lastCommit},
			RepoOwner:  owner,
			RepoName:   name,
			AuthorName: author,
		}
	}
	return r
}

// newFakeData renvoie des données factices : quatre dépôts (Actif, Flemme, Dead, jamais synchronisé),
// deux commits et deux contributeurs.
func newFakeData() *fakeData {
	never := summary(4, "promo", "dan", "", time.Time{}, "")
	never.LastSyncedAt, never.LastRun, never.Commits, never.TrackedBranches = time.Time{}, model.SyncRun{}, 0, 0
	return &fakeData{
		repos: []store.RepositorySummary{
			summary(1, "promo", "alice", "Alice Martin", testNow.Add(-2*time.Hour), "Alice"),
			summary(2, "promo", "bob", "", testNow.Add(-9*24*time.Hour), "Bob"),
			summary(3, "promo", "chloe", "", testNow.Add(-42*24*time.Hour), "Chloé"),
			never,
		},
		commits: []store.CommitView{
			{Commit: model.Commit{RepoID: 1, SHA: "1111111aaaaaaa", Message: "Ajoute la page d'accueil\n\nDétails",
				CommittedAt: testNow.Add(-2 * time.Hour)}, RepoOwner: "promo", RepoName: "alice",
				RepoLabel: "Alice Martin", AuthorName: "Alice", AuthorLogin: "alice-gh"},
			{Commit: model.Commit{RepoID: 2, SHA: "2222222bbbbbbb", Message: "\x1b[31mrouge\x1b[0m corrige",
				CommittedAt: testNow.Add(-9 * 24 * time.Hour)}, RepoOwner: "promo", RepoName: "bob", AuthorName: "Bob"},
		},
		contribs: []store.ContributorStats{
			{Contributor: model.Contributor{ID: 1, Name: "Alice", ProviderLogin: "alice-gh"}, Commits: 5,
				Additions: 10, Deletions: 2, Repos: 1, LastCommitAt: testNow.Add(-2 * time.Hour)},
			{Contributor: model.Contributor{ID: 2, Name: "Bob"}, Commits: 1, Repos: 1,
				LastCommitAt: testNow.Add(-9 * 24 * time.Hour)},
		},
		authorTimes: map[int64][]model.AuthorCommitTime{
			1: { // dépôt d'Alice : Alice 2 commits cette semaine et 1 il y a deux semaines, Bob 1 il y a 20 semaines
				{ContributorID: 1, CommittedAt: testNow.Add(-2 * time.Hour)},
				{ContributorID: 1, CommittedAt: testNow.Add(-3 * time.Hour)},
				{ContributorID: 1, CommittedAt: testNow.Add(-14 * 24 * time.Hour)},
				{ContributorID: 2, CommittedAt: testNow.Add(-20 * 7 * 24 * time.Hour)},
			},
			2: {{ContributorID: 2, CommittedAt: testNow.Add(-9 * 24 * time.Hour)}},
		},
	}
}

// fakeSyncer est un Syncer factice : il envoie des événements de progression pour chaque dépôt de
// repos et renvoie report (ou err). Si gate n'est pas nil, il attend gate après le premier dépôt.
type fakeSyncer struct {
	mu      gosync.Mutex
	repos   []wsync.RepoReport
	report  wsync.Report
	err     error
	gate    chan struct{}
	allRuns int
	repoIDs []int64
}

func (f *fakeSyncer) SyncAll(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	f.mu.Lock()
	f.allRuns++
	f.mu.Unlock()
	return f.run(ctx, opts)
}

func (f *fakeSyncer) SyncRepo(ctx context.Context, repoID int64, opts wsync.SyncOptions) (wsync.Report, error) {
	f.mu.Lock()
	f.repoIDs = append(f.repoIDs, repoID)
	f.mu.Unlock()
	return f.run(ctx, opts)
}

func (f *fakeSyncer) run(ctx context.Context, opts wsync.SyncOptions) (wsync.Report, error) {
	progress := opts.Progress
	if progress == nil {
		progress = func(wsync.Event) {}
	}
	total := len(f.repos)
	progress(wsync.Event{Kind: wsync.EventStarted, Total: total})
	for i, r := range f.repos {
		progress(wsync.Event{Kind: wsync.EventRepoStarted, Repo: r, Done: i, Total: total})
		if i == 1 && f.gate != nil {
			select {
			case <-f.gate:
			case <-ctx.Done():
				return wsync.Report{}, ctx.Err()
			}
		}
		progress(wsync.Event{Kind: wsync.EventRepoFinished, Repo: r, Done: i + 1, Total: total})
	}
	progress(wsync.Event{Kind: wsync.EventFinished, Done: total, Total: total})
	return f.report, f.err
}

// counts renvoie le nombre de SyncAll et les dépôts passés à SyncRepo.
func (f *fakeSyncer) counts() (int, []int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.allRuns, slices.Clone(f.repoIDs)
}

// testDeps renvoie des dépendances de test sans couleur, en Unicode, en UTC.
func testDeps(data DataSource, syncer Syncer) Deps {
	return Deps{
		Data:     data,
		Syncer:   syncer,
		Config:   workspace.DefaultConfig("licence-projet"),
		Location: time.UTC,
		Now:      func() time.Time { return testNow },
		Version:  "0.1.0",
		Unicode:  true,
	}
}

// scheduledTick remplace tea.Tick dans les tests : le message est gardé, pas envoyé.
type scheduledTick struct {
	d   time.Duration
	msg tea.Msg
}

// loop exécute le modèle comme tea.Program : chaque commande tourne dans une goroutine et son message
// repasse par Update.
type loop struct {
	t     *testing.T
	m     Model
	msgs  chan tea.Msg
	ticks []scheduledTick
	quit  bool
}

// newLoop construit le modèle (ticks interceptés), lance Init et attend le premier chargement.
func newLoop(t *testing.T, deps Deps) *loop {
	t.Helper()
	m := NewModel(t.Context(), deps)
	m.tick = func(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
		return func() tea.Msg { return scheduledTick{d: d, msg: fn(testNow.Add(d))} }
	}
	l := &loop{t: t, m: m, msgs: make(chan tea.Msg, 64)}
	l.do(m.Init())
	l.until(func(m Model) bool { return !m.loading })
	return l
}

// do exécute cmd dans une goroutine.
func (l *loop) do(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() { l.msgs <- cmd() }()
}

// send traite un message : lots de commandes, ticks gardés, Quit noté, le reste passé à Update.
func (l *loop) send(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			l.do(c)
		}
	case scheduledTick:
		l.ticks = append(l.ticks, msg)
	case tea.QuitMsg:
		l.quit = true
	default:
		next, cmd := l.m.Update(msg)
		l.m = next.(Model)
		l.do(cmd)
	}
}

// press envoie une suite de touches.
func (l *loop) press(keys ...string) {
	for _, k := range keys {
		l.send(keyMsg(k))
	}
}

// until traite les messages jusqu'à ce que cond soit vraie (5 s au plus).
func (l *loop) until(cond func(Model) bool) {
	l.t.Helper()
	timeout := time.After(5 * time.Second)
	for !cond(l.m) {
		select {
		case msg := <-l.msgs:
			l.send(msg)
		case <-timeout:
			l.t.Fatalf("condition jamais remplie ; écran :\n%s", l.screen())
		}
	}
}

// settle traite les messages déjà prêts ou qui arrivent dans les 50 ms.
func (l *loop) settle() {
	for {
		select {
		case msg := <-l.msgs:
			l.send(msg)
		case <-time.After(50 * time.Millisecond):
			return
		}
	}
}

// screen renvoie le contenu de l'écran.
func (l *loop) screen() string {
	return l.m.View().Content
}

// ticksOf renvoie les ticks programmés de durée d.
func (l *loop) ticksOf(d time.Duration) []scheduledTick {
	var out []scheduledTick
	for _, tk := range l.ticks {
		if tk.d == d {
			out = append(out, tk)
		}
	}
	return out
}

// keyMsg construit l'appui sur une touche nommée comme dans keyMap (« enter », « shift+tab », « q »…).
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "shift+tab":
		return tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "pgdown":
		return tea.KeyPressMsg{Code: tea.KeyPgDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	}
	r := []rune(s)[0]
	return tea.KeyPressMsg{Code: r, Text: s}
}

// errBoom est une erreur factice.
var errBoom = errors.New("base indisponible")
