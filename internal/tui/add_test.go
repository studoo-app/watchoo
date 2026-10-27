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

// fakeAdder est un Adder factice : il garde les demandes reçues et renvoie result (ou err). Un
// résultat OutcomeAdded ajoute aussi le dépôt aux données (data), comme le ferait le vrai moteur. Si
// gate n'est pas nil, il l'attend avant de répondre.
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

// requests renvoie les demandes reçues.
func (f *fakeAdder) requests() []wsync.AddRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reqs)
}

// added renvoie un résultat d'ajout du dépôt owner/name, identifiant id.
func added(outcome wsync.AddOutcome, id int64, owner, name string) wsync.AddResult {
	return wsync.AddResult{Outcome: outcome, Repo: model.Repository{ID: id, Provider: model.ProviderGitHub,
		ProviderID: 100 + id, Owner: owner, Name: name, Enabled: true}}
}

// typeText tape s dans le TUI, caractère par caractère.
func typeText(l *loop, s string) {
	for _, r := range s {
		l.send(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

// addLoop renvoie une boucle dont l'Adder est adder (ses données sont data).
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
		t.Fatal("« a » n'a pas ouvert la saisie")
	}
	mustContain(t, l.screen(), "Dépôt à suivre (owner/nom ou URL GitHub) : ▏", "entrée pour ajouter, échap pour annuler")
	// Pendant la saisie, les touches s'écrivent : « q » ne quitte pas, « s » ne synchronise pas.
	typeText(l, "promo/qs")
	l.press("backspace")
	if l.quit || l.m.sync.running {
		t.Fatal("une touche de la saisie a été prise pour une commande")
	}
	mustContain(t, l.screen(), "Dépôt à suivre (owner/nom ou URL GitHub) : promo/q▏")
	l.press("esc")
	if l.m.add.open {
		t.Fatal("échap n'a pas fermé la saisie")
	}
	mustContain(t, l.screen(), "ajout annulé")
	mustNotContain(t, l.screen(), "Dépôt à suivre")
	if reqs := adder.requests(); len(reqs) != 0 {
		t.Errorf("ajout appelé après échap : %v", reqs)
	}
	// La saisie rouverte est vide.
	l.press("a")
	if l.m.add.input != "" {
		t.Errorf("saisie rouverte = %q ; attendu vide", l.m.add.input)
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
		t.Errorf("message = %q (erreur %v) ; attendu %q", l.m.status, l.m.statusErr, want)
	}
	if !l.m.add.open || l.m.add.input != "pas-de-barre" {
		t.Errorf("saisie fermée ou effacée après une erreur : ouverte %v, %q", l.m.add.open, l.m.add.input)
	}
	mustContain(t, l.screen(), "Dépôt à suivre (owner/nom ou URL GitHub) : pas-de-barre", `dépôt "pas-de-barre" invalide`)

	// Une URL avec identifiants est refusée et effacée, sans être réaffichée.
	l.press("ctrl+u")
	l.send(tea.PasteMsg{Content: "https://eleve:jeton-secret@github.com/promo/eve"})
	l.press("enter")
	mustContain(t, l.screen(), "identifiants interdits dans l'URL")
	mustNotContain(t, l.screen(), "jeton-secret")
	if l.m.add.input != "" {
		t.Errorf("saisie avec identifiants gardée : %q", l.m.add.input)
	}

	// Saisie vide.
	l.press("enter")
	mustContain(t, l.screen(), "dépôt vide")
	if reqs := adder.requests(); len(reqs) != 0 {
		t.Errorf("ajout appelé pour une référence invalide : %v", reqs)
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
	// Collage d'une URL (retour à la ligne final retiré), séquence d'échappement nettoyée.
	l.send(tea.PasteMsg{Content: "https://github.com/promo/\x1b[31meve\n"})
	mustContain(t, l.screen(), "Dépôt à suivre (owner/nom ou URL GitHub) : https://github.com/promo/eve▏")
	l.press("enter")
	if l.m.add.open || !l.m.add.running {
		t.Fatalf("après entrée : saisie ouverte %v, ajout en cours %v", l.m.add.open, l.m.add.running)
	}
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(), "promo/eve ajouté : synchro de promo/eve…")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })

	reqs := adder.requests()
	if len(reqs) != 1 || reqs[0].Ref != (repoimport.Ref{Owner: "promo", Name: "eve"}) || reqs[0].Label != "" ||
		reqs[0].BranchPatterns != nil || reqs[0].FlemmeAfterDays != nil || reqs[0].DeadAfterDays != nil {
		t.Errorf("demandes = %+v ; attendu promo/eve sans option, comme « repo add »", reqs)
	}
	if _, repos := syncer.counts(); !slices.Equal(repos, []int64{5}) {
		t.Errorf("SyncRepo appelé avec %v ; attendu [5]", repos)
	}
	if data.loadCount() <= loadsBefore {
		t.Error("la liste n'a pas été relue après l'ajout")
	}
	if id, ok := l.m.selectedRepoID(); !ok || id != 5 {
		t.Errorf("dépôt sélectionné = %d (%v) ; attendu le dépôt ajouté (5)", id, ok)
	}
	screen := l.screen()
	mustContain(t, screen, "synchro terminée : 1 dépôt, 4 nouveaux commits", "promo/eve")
	if line := lineWith(screen, "› promo/eve"); line == "" {
		t.Errorf("le dépôt ajouté n'est pas sélectionné ; écran :\n%s", screen)
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
	mustContain(t, l.screen(), "promo/chloe déjà suivi : rien à changer")
	if l.m.statusErr {
		t.Error("dépôt déjà suivi affiché comme une erreur")
	}
	if all, repos := syncer.counts(); all != 0 || len(repos) != 0 {
		t.Errorf("synchro lancée pour un dépôt déjà suivi : SyncAll %d, SyncRepo %v", all, repos)
	}
	if id, _ := l.m.selectedRepoID(); id != 3 {
		t.Errorf("dépôt sélectionné = %d ; attendu 3", id)
	}

	// Dépôt suivi mais arrêté : suivi repris et synchronisé.
	adder.result = added(wsync.OutcomeUpdated, 2, "promo", "bob")
	l.press("a")
	typeText(l, "promo/bob")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(), "promo/bob déjà suivi : suivi repris : synchro de promo/bob…")
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
	if _, repos := syncer.counts(); !slices.Equal(repos, []int64{2}) {
		t.Errorf("SyncRepo appelé avec %v ; attendu [2]", repos)
	}

	// Dépôt renommé sur GitHub : signalé comme par « repo add ».
	l.m.width = 200
	adder.result = added(wsync.OutcomeAlreadyTracked, 1, "promo", "alice")
	l.press("a")
	typeText(l, "promo/alice-old")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running && !m.loading })
	mustContain(t, l.screen(), "promo/alice déjà suivi : rien à changer (promo/alice-old s'appelle maintenant promo/alice")
}

func TestAddFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		result wsync.AddResult
		err    error
		want   string
	}{
		{"inaccessible", wsync.AddResult{Outcome: wsync.OutcomeInaccessible, Error: "dépôt introuvable ou sans accès"},
			nil, "dépôt promo/zoe inaccessible : dépôt introuvable ou sans accès (vérifiez son nom et que le compte gh"},
		{"erreur", wsync.AddResult{Outcome: wsync.OutcomeError, Error: "quota \x1b[31mGitHub épuisé"},
			nil, "ajout du dépôt promo/zoe : quota GitHub épuisé"},
		{"erreur globale", wsync.AddResult{}, errors.New("ajout de dépôts : gh introuvable"),
			"ajout de dépôts : gh introuvable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := newFakeData()
			adder := &fakeAdder{data: data, result: tc.result, err: tc.err}
			syncer := &fakeSyncer{}
			l := addLoop(t, data, syncer, adder)
			l.m.width = 300 // message entier à l'écran
			l.press("a")
			typeText(l, "promo/zoe")
			l.press("enter")
			l.until(func(m Model) bool { return !m.add.running })
			mustContain(t, l.screen(), tc.want)
			mustNotContain(t, l.screen(), "\x1b[31m")
			if !l.m.statusErr {
				t.Error("échec non affiché comme une erreur")
			}
			if all, repos := syncer.counts(); all != 0 || len(repos) != 0 {
				t.Errorf("synchro lancée après un échec : SyncAll %d, SyncRepo %v", all, repos)
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
		"promo/eve ajouté : sa synchro commencera après la synchro en cours (tous les dépôts)")
	if all, repos := syncer.counts(); all != 1 || len(repos) != 0 {
		t.Errorf("SyncAll %d, SyncRepo %v ; attendu la seule synchro en cours", all, repos)
	}

	close(gate)
	l.until(func(m Model) bool {
		_, repos := syncer.counts()
		return len(repos) == 1 && !m.sync.running && !m.loading
	})
	if _, repos := syncer.counts(); !slices.Equal(repos, []int64{5}) {
		t.Errorf("SyncRepo appelé avec %v ; attendu [5] après la synchro en cours", repos)
	}
	if len(l.m.pendingSyncs) != 0 {
		t.Errorf("dépôts encore en attente : %v", l.m.pendingSyncs)
	}
}

// Une synchro de tous les dépôts lancée après l'ajout remplace la synchro en attente.
func TestAddPendingCoveredBySyncAll(t *testing.T) {
	l := addLoop(t, newFakeData(), &fakeSyncer{}, &fakeAdder{})
	l.m.pendingSyncs = []int64{5}
	l.press("s")
	if len(l.m.pendingSyncs) != 0 {
		t.Errorf("dépôts en attente après une synchro de tous les dépôts : %v", l.m.pendingSyncs)
	}
	l.until(func(m Model) bool { return !m.sync.running && !m.loading })
}

func TestAddKeyRefusals(t *testing.T) {
	data := newFakeData()
	gate := make(chan struct{})
	adder := &fakeAdder{data: data, result: added(wsync.OutcomeAdded, 5, "promo", "eve"), gate: gate}
	l := addLoop(t, data, &fakeSyncer{}, adder)
	l.m.width = 200

	// Hors de la liste de la vue Dépôts : la saisie ne s'ouvre pas.
	l.press("2", "a")
	if l.m.add.open {
		t.Error("saisie ouverte depuis la vue Commits")
	}
	mustContain(t, l.screen(), "« a » ajoute un dépôt depuis la liste de la vue Dépôts")
	l.press("1", "enter")
	l.until(func(m Model) bool { return m.detail.open && !m.detail.loading })
	l.press("a")
	if l.m.add.open {
		t.Error("saisie ouverte depuis le détail d'un dépôt")
	}
	l.press("esc")

	// Un seul ajout à la fois.
	l.press("a")
	typeText(l, "promo/eve")
	l.press("enter")
	l.press("a")
	if l.m.add.open {
		t.Error("saisie ouverte pendant un ajout")
	}
	mustContain(t, l.screen(), "un ajout est déjà en cours (promo/eve) : patientez")
	close(gate)
	l.until(func(m Model) bool { return !m.add.running && !m.sync.running && !m.loading })

	// Sans Adder : ajout indisponible.
	l2 := newLoop(t, testDeps(newFakeData(), &fakeSyncer{}))
	l2.press("a")
	if l2.m.add.open || !l2.m.statusErr || l2.m.status != "ajout de dépôt indisponible" {
		t.Errorf("sans Adder : saisie %v, message %q", l2.m.add.open, l2.m.status)
	}
}

func TestAddHelpAndLongInput(t *testing.T) {
	l := addLoop(t, newFakeData(), &fakeSyncer{}, &fakeAdder{})
	l.press("?")
	mustContain(t, l.screen(), "a ajouter un dépôt (vue Dépôts)")
	l.press("?", "a")
	long := strings.Repeat("x", 400)
	l.send(tea.PasteMsg{Content: long})
	if n := len([]rune(l.m.add.input)); n != addCharLimit {
		t.Errorf("saisie de %d caractères ; attendu %d au plus", n, addCharLimit)
	}
	// La fin de la saisie reste visible, sans dépasser la largeur de l'écran.
	line := lineWith(l.screen(), "Dépôt à suivre")
	if width(line) > defaultWidth || !strings.HasSuffix(line, "xx▏") || !strings.Contains(line, "…") {
		t.Errorf("ligne de saisie = %q (largeur %d)", line, width(line))
	}
}

// Ajout avec le vrai moteur et une vraie base : le dépôt est vérifié auprès du provider, enregistré,
// synchronisé, puis affiché avec ses commits.
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
	mustContain(t, l.screen(), "synchro terminée : 1 dépôt, 3 nouveaux commits")

	repos, err := st.ListRepositories(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].FullName() != "promo/neo" || !repos[0].Enabled || repos[0].BranchPatterns != nil {
		t.Fatalf("dépôts en base = %+v ; attendu promo/neo suivi, toutes les branches", repos)
	}
	if line := lineWith(l.screen(), "promo/neo"); !strings.HasPrefix(line, "›") {
		t.Errorf("ligne du dépôt ajouté = %q ; attendu sélectionnée", line)
	}

	// Un dépôt inconnu du provider : inaccessible, rien d'enregistré.
	l.press("a")
	typeText(l, "promo/absent")
	l.press("enter")
	l.until(func(m Model) bool { return !m.add.running })
	mustContain(t, l.screen(), "dépôt promo/absent inaccessible")
	if repos, _ := st.ListRepositories(ctx); len(repos) != 1 {
		t.Errorf("%d dépôts en base ; attendu 1", len(repos))
	}
}
