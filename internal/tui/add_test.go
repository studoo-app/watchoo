package tui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	gosync "sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/studoo-app/watchoo/internal/auth/authtest"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// fakeAdder is a fake Adder: it keeps the requests it receives and returns result (or err). An
// OutcomeAdded result also adds the repository to the data (data), like the real engine would. If
// gate is not nil, it waits for it before answering.
type fakeAdder struct {
	mu     gosync.Mutex
	data   *fakeData
	result wsync.AddResult
	err    error
	gate   chan struct{}
	reqs   []wsync.AddRequest
}

func (f *fakeAdder) AddRepositories(ctx context.Context, reqs []wsync.AddRequest, _ wsync.AddOptions) ([]wsync.AddResult, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, reqs...)
	f.mu.Unlock()
	if f.gate != nil {
		select {
		case <-f.gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	res := f.result
	res.Request = reqs[0]
	if res.Outcome == wsync.OutcomeAdded && f.data != nil {
		r := res.Repo
		s := summary(r.ID, r.Owner, r.Name, "", time.Time{}, "")
		s.LastSyncedAt, s.LastRun, s.Commits, s.TrackedBranches = time.Time{}, model.SyncRun{}, 0, 0
		f.data.mu.Lock()
		f.data.repos = append(f.data.repos, s)
		f.data.mu.Unlock()
	}
	return []wsync.AddResult{res}, nil
}

// requests returns the received requests.
func (f *fakeAdder) requests() []wsync.AddRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reqs)
}

// added returns an addition result for the repository owner/name, with ID id.
func added(outcome wsync.AddOutcome, id int64, owner, name string) wsync.AddResult {
	return wsync.AddResult{Outcome: outcome, Repo: model.Repository{ID: id, Provider: model.ProviderGitHub,
		ProviderID: 100 + id, Owner: owner, Name: name, Enabled: true}}
}

// typeText types s into the TUI, character by character.
func typeText(l *loop, s string) {
	for _, r := range s {
		l.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// addLoop returns a loop whose Adder is adder (its data is data).
func addLoop(t *testing.T, data *fakeData, syncer *fakeSyncer, adder *fakeAdder) *loop {
	t.Helper()
	deps := testDeps(data, syncer)
	deps.Adder = adder
	return newLoop(t, deps)
}

func TestAddOpensAndCancels(t *testing.T) {
	data := newFakeData()
	adder := &fakeAdder{data: data}
	l := addLoop(t, data, &fakeSyncer{}, adder)

	l.press("a")
	if !l.m.add.open {
		t.Fatal("\"a\" did not open the input")
	}
	mustContain(t, l.screen(), "Repository to track (owner/name or GitHub URL): ▏", "enter to add, esc to cancel")
	// During the input, keys are typed: "q" does not quit, "s" does not sync.
	typeText(l, "promo/qs")
	l.press("backspace")
	if l.quit || l.m.sync.running {
		t.Fatal("a key of the input was taken for a command")
	}
	mustContain(t, l.screen(), "Repository to track (owner/name or GitHub URL): promo/q▏")
	l.press("esc")
	if l.m.add.open {
		t.Fatal("esc did not close the input")
	}
	mustContain(t, l.screen(), "addition cancelled")
	mustNotContain(t, l.screen(), "Repository to track")
	if reqs := adder.requests(); len(reqs) != 0 {
		t.Errorf("addition called after esc: %v", reqs)
	}
	// The reopened input is empty.
	l.press("a")
	if l.m.add.input != "" {
		t.Errorf("reopened input = %q; want empty", l.m.add.input)
	}
}

func TestAddInvalidReference(t *testing.T) {
	data := newFakeData()
	adder := &fakeAdder{data: data}
	l := addLoop(t, data, &fakeSyncer{}, adder)

	l.press("a")
	typeText(l, "pas-de-barre")
	l.press("enter")
	_, want := repoimport.ParseRef("pas-de-barre")
	if !l.m.statusErr || l.m.status != want.Error() {
		t.Errorf("message = %q (error %v); want %q", l.m.status, l.m.statusErr, want)
	}
	if !l.m.add.open || l.m.add.input != "pas-de-barre" {
		t.Errorf("input closed or cleared after an error: open %v, %q", l.m.add.open, l.m.add.input)
	}
	mustContain(t, l.screen(), "Repository to track (owner/name or GitHub URL): pas-de-barre", `invalid repository "pas-de-barre"`)

	// A URL with credentials is refused and cleared, without being shown again.
	l.press("ctrl+u")
	l.send(tea.PasteMsg{Content: "https://eleve:jeton-secret@github.com/promo/eve"})
	l.press("enter")
	mustContain(t, l.screen(), "credentials not allowed in the URL")
	mustNotContain(t, l.screen(), "jeton-secret")
	if l.m.add.input != "" {
		t.Errorf("input with credentials kept: %q", l.m.add.input)
	}

	// Empty input.
	l.press("enter")
	mustContain(t, l.screen(), "empty repository")
	if reqs := adder.requests(); len(reqs) != 0 {
		t.Errorf("addition called for an invalid reference: %v", reqs)
	}
}

func TestAddSuccessReloadsSelectsAndSyncs(t *testing.T) {
	data := newFakeData()
	adder := &fakeAdder{data: data, result: added(wsync.OutcomeAdded, 5, "promo", "eve")}
	syncer := &fakeSyncer{}
	syncer.report = wsync.Report{Repos: []wsync.RepoReport{{RepoID: 5, DisplayName: "promo/eve",
		Status: wsync.StatusSuccess, NewCommits: 4}}}
	l := addLoop(t, data, syncer, adder)
	loadsBefore := data.loadCount()

	l.press("a")
	// Pasting a URL (final newline removed), escape sequence cleaned.
	l.send(tea.PasteMsg{Content: "https://github.com/promo/\x1b[31meve\n"})
	mustContain(t, l.screen(), "Repository to track (owner/name or GitHub URL): https://github.com/promo/eve▏")
	l.press("enter")
	if l.m.add.open || !l.m.add.running {
		t.Fatalf("after enter: input open %v, addition running %v", l.m.add.open, l.m.add.running)
	}
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(), "promo/eve added: syncing promo/eve…")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })

	reqs := adder.requests()
	if len(reqs) != 1 || reqs[0].Ref != (repoimport.Ref{Owner: "promo", Name: "eve"}) || reqs[0].Label != "" ||
		reqs[0].BranchPatterns != nil || reqs[0].FlemmeAfterDays != nil || reqs[0].DeadAfterDays != nil {
		t.Errorf("requests = %+v; want promo/eve without option, like \"repo add\"", reqs)
	}
	if _, repos := syncer.counts(); !slices.Equal(repos, []int64{5}) {
		t.Errorf("SyncRepo called with %v; want [5]", repos)
	}
	if data.loadCount() <= loadsBefore {
		t.Error("the list was not reread after the addition")
	}
	if id, ok := l.m.selectedRepoID(); !ok || id != 5 {
		t.Errorf("selected repository = %d (%v); want the added repository (5)", id, ok)
	}
	screen := l.screen()
	mustContain(t, screen, "sync finished: 1 repository, 4 new commits", "promo/eve")
	if line := lineWith(screen, "› promo/eve"); line == "" {
		t.Errorf("the added repository is not selected; screen:\n%s", screen)
	}
}

func TestAddAlreadyTrackedAndResumed(t *testing.T) {
	data := newFakeData()
	adder := &fakeAdder{data: data, result: added(wsync.OutcomeAlreadyTracked, 3, "promo", "chloe")}
	syncer := &fakeSyncer{}
	l := addLoop(t, data, syncer, adder)

	l.press("a")
	typeText(l, "Promo/Chloe")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running && !m.loading })
	mustContain(t, l.screen(), "promo/chloe already tracked: nothing to change")
	if l.m.statusErr {
		t.Error("already tracked repository shown as an error")
	}
	if all, repos := syncer.counts(); all != 0 || len(repos) != 0 {
		t.Errorf("sync started for an already tracked repository: SyncAll %d, SyncRepo %v", all, repos)
	}
	if id, _ := l.m.selectedRepoID(); id != 3 {
		t.Errorf("selected repository = %d; want 3", id)
	}

	// Tracked but stopped repository: tracking resumed and synced.
	adder.result = added(wsync.OutcomeUpdated, 2, "promo", "bob")
	l.press("a")
	typeText(l, "promo/bob")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(), "promo/bob already tracked: tracking resumed: syncing promo/bob…")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	if _, repos := syncer.counts(); !slices.Equal(repos, []int64{2}) {
		t.Errorf("SyncRepo called with %v; want [2]", repos)
	}

	// Repository renamed on GitHub: reported like by "repo add".
	l.m.width = 200
	adder.result = added(wsync.OutcomeAlreadyTracked, 1, "promo", "alice")
	l.press("a")
	typeText(l, "promo/alice-old")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running && !m.loading })
	mustContain(t, l.screen(), "promo/alice already tracked: nothing to change (promo/alice-old is now called promo/alice")
}

func TestAddFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result wsync.AddResult
		err    error
		want   string
	}{
		{"inaccessible", wsync.AddResult{Outcome: wsync.OutcomeInaccessible, Error: "dépôt introuvable ou sans accès"},
			nil, "repository promo/zoe is inaccessible: dépôt introuvable ou sans accès (check its name and that the gh account"},
		{"error", wsync.AddResult{Outcome: wsync.OutcomeError, Error: "quota \x1b[31mGitHub épuisé"},
			nil, "adding repository promo/zoe: quota GitHub épuisé"},
		{"global error", wsync.AddResult{}, errors.New("ajout de dépôts : gh introuvable"),
			"ajout de dépôts : gh introuvable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := newFakeData()
			adder := &fakeAdder{data: data, result: tc.result, err: tc.err}
			syncer := &fakeSyncer{}
			l := addLoop(t, data, syncer, adder)
			l.m.width = 300 // whole message on the screen
			l.press("a")
			typeText(l, "promo/zoe")
			l.press("enter")
			l.until(func(m Model) bool { return !m.add.running })
			mustContain(t, l.screen(), tc.want)
			mustNotContain(t, l.screen(), "\x1b[31m")
			if !l.m.statusErr {
				t.Error("failure not shown as an error")
			}
			if all, repos := syncer.counts(); all != 0 || len(repos) != 0 {
				t.Errorf("sync started after a failure: SyncAll %d, SyncRepo %v", all, repos)
			}
		})
	}
}

func TestAddDuringSync(t *testing.T) {
	data := newFakeData()
	gate := make(chan struct{})
	syncer := &fakeSyncer{
		repos: []wsync.RepoReport{
			{RepoID: 1, DisplayName: "Alice Martin", Status: wsync.StatusSuccess},
			{RepoID: 2, DisplayName: "promo/bob", Status: wsync.StatusSuccess},
		},
		gate: gate,
	}
	syncer.report = wsync.Report{Repos: syncer.repos}
	adder := &fakeAdder{data: data, result: added(wsync.OutcomeAdded, 5, "promo", "eve")}
	l := addLoop(t, data, syncer, adder)
	l.m.width = 200

	l.press("s")
	l.until(func(m Model) bool { return m.sync.done == 1 })
	l.press("a")
	typeText(l, "promo/eve")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(),
		"promo/eve added: its sync will start after the current sync (all repositories)")
	if all, repos := syncer.counts(); all != 1 || len(repos) != 0 {
		t.Errorf("SyncAll %d, SyncRepo %v; want only the running sync", all, repos)
	}

	close(gate)
	l.until(func(m Model) bool {
		_, repos := syncer.counts()
		return len(repos) == 1 && !m.sync.running && !m.loading
	})
	if _, repos := syncer.counts(); !slices.Equal(repos, []int64{5}) {
		t.Errorf("SyncRepo called with %v; want [5] after the running sync", repos)
	}
	if len(l.m.pendingSyncs) != 0 {
		t.Errorf("repositories still pending: %v", l.m.pendingSyncs)
	}
}

// A sync of all repositories started after the addition replaces the pending sync.
func TestAddPendingCoveredBySyncAll(t *testing.T) {
	l := addLoop(t, newFakeData(), &fakeSyncer{}, &fakeAdder{})
	l.m.pendingSyncs = []int64{5}
	l.press("s")
	if len(l.m.pendingSyncs) != 0 {
		t.Errorf("repositories pending after a sync of all repositories: %v", l.m.pendingSyncs)
	}
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
}

func TestAddKeyRefusals(t *testing.T) {
	data := newFakeData()
	gate := make(chan struct{})
	adder := &fakeAdder{data: data, result: added(wsync.OutcomeAdded, 5, "promo", "eve"), gate: gate}
	l := addLoop(t, data, &fakeSyncer{}, adder)
	l.m.width = 200

	// Outside the list of the Repositories view: the input does not open.
	l.press("2", "a")
	if l.m.add.open {
		t.Error("input opened from the Commits view")
	}
	mustContain(t, l.screen(), `"a" adds a repository from the list of the Repositories view`)
	l.press("1", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	l.press("a")
	if l.m.add.open {
		t.Error("input opened from a repository detail")
	}
	l.press("esc")

	// A single addition at a time.
	l.press("a")
	typeText(l, "promo/eve")
	l.press("enter")
	l.press("a")
	if l.m.add.open {
		t.Error("input opened during an addition")
	}
	mustContain(t, l.screen(), "an addition is already in progress (promo/eve): please wait")
	close(gate)
	l.until(func(m Model) bool { return !m.add.running && !m.sync.running && !m.loading })

	// Without an Adder: addition unavailable.
	l2 := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l2.press("a")
	if l2.m.add.open || !l2.m.statusErr || l2.m.status != "adding a repository is unavailable" {
		t.Errorf("without an Adder: input %v, message %q", l2.m.add.open, l2.m.status)
	}
}

func TestAddHelpAndLongInput(t *testing.T) {
	l := addLoop(t, newFakeData(), &fakeSyncer{}, &fakeAdder{})
	l.press("?")
	mustContain(t, l.screen(), "a add a repository (Repositories view)")
	l.press("?", "a")
	long := strings.Repeat("x", 400)
	l.send(tea.PasteMsg{Content: long})
	if n := len([]rune(l.m.add.input)); n != addCharLimit {
		t.Errorf("input of %d characters; want %d at most", n, addCharLimit)
	}
	// The end of the input stays visible, without exceeding the screen width.
	line := lineWith(l.screen(), "Repository to track")
	if width(line) > defaultWidth || !strings.HasSuffix(line, "xx▏") || !strings.Contains(line, "…") {
		t.Errorf("input line = %q (width %d)", line, width(line))
	}
}

// Addition with the real engine and a real database: the repository is checked with the provider,
// stored, synced, then shown with its commits.
func TestAddWithRealEngine(t *testing.T) {
	ctx := t.Context()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "watchoo.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	fake := providertest.New()
	fake.AddRepository(provider.Repository{ID: 7, Owner: "promo", Name: "neo", DefaultBranch: "main"})
	fake.AppendCommits(7, "main", 3, testNow.Add(-5*time.Hour), time.Hour)
	engine, err := wsync.New(wsync.Options{
		Store:       st,
		Auth:        authtest.NewLoggedIn(""),
		Config:      workspace.DefaultConfig("promo"),
		NewProvider: func(string) (provider.Provider, error) { return fake, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	deps := testDeps(st, engine)
	deps.Adder = engine
	l := newLoop(t, deps)
	l.m.width = 200

	l.press("a")
	typeText(l, "https://github.com/promo/neo")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running && !m.sync.running && !m.loading })
	mustContain(t, l.screen(), "sync finished: 1 repository, 3 new commits")

	repos, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].FullName() != "promo/neo" || !repos[0].Enabled || repos[0].BranchPatterns != nil {
		t.Fatalf("repositories in the database = %+v; want promo/neo tracked, all branches", repos)
	}
	if line := lineWith(l.screen(), "promo/neo"); !strings.HasPrefix(line, "›") {
		t.Errorf("line of the added repository = %q; want selected", line)
	}

	// A repository unknown to the provider: inaccessible, nothing stored.
	l.press("a")
	typeText(l, "promo/absent")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(), "repository promo/absent is inaccessible")
	if repos, _ := st.ListRepositories(ctx); len(repos) != 1 {
		t.Errorf("%d repositories in the database; want 1", len(repos))
	}
}
