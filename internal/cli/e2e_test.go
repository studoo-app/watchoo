package cli

// Test de bout en bout de la phase 3 : la CLI complète, avec le vrai accès à gh (auth.GH et le faux
// binaire gh), le vrai provider GitHub (internal/provider/github) branché sur le faux serveur
// githubtest, le vrai moteur de synchro et une base SQLite temporaire. Aucun appel réseau réel
// (127.0.0.1) ; données factices uniquement.

import (
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strings"
	gosync "sync"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/github"
	"github.com/studoo-app/watchoo/internal/provider/github/githubtest"
)

// e2eClock est une horloge réglable, partagée par la CLI et le faux serveur.
type e2eClock struct {
	mu gosync.Mutex
	t  time.Time
}

func (c *e2eClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *e2eClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// Messages de commit factices : ils ne doivent jamais apparaître dans les logs.
const (
	e2eMsgAlice1 = "e2e-msg-alice-premier-jet"
	e2eMsgAlice2 = "e2e-msg-alice-tests"
	e2eMsgDev    = "e2e-msg-bob-essai-dev"
	e2eMsgBob1   = "e2e-msg-bob-init"
	e2eMsgBob2   = "e2e-msg-bob-reprise"
	e2eMsgDave   = "e2e-msg-dave-rendu"
)

func TestEndToEnd(t *testing.T) {
	clk := &e2eClock{t: time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)}
	start := clk.Now()
	srv := githubtest.NewServer(t, githubtest.Options{Now: clk.Now})
	t.Cleanup(srv.Close)

	h := newHarness(t)
	useFakeGH(t, h, "logged-in") // vrai auth.GH, token = githubtest.Token
	t.Setenv("WATCHOO_TIMEZONE", "UTC")
	h.Now = clk.Now
	cache, quota := github.NewCache(0, 0), github.NewQuota()
	h.NewProvider = func(token string) (provider.Provider, error) {
		return github.New(token, github.Options{BaseURL: srv.URL(), Cache: cache, Quota: quota,
			UserAgent: "watchoo/test", Logger: slog.Default()})
	}

	alice := githubtest.Person{Name: "Alice Martin", Email: "alice@example.test", Login: "alice-gh"}
	bob := githubtest.Person{Name: "Bob Durand", Email: "Bob@Example.test"} // sans compte GitHub
	dave := githubtest.Person{Name: "Dave Petit", Email: "dave@example.test", Login: "dave-gh"}
	commit := func(repo int64, msg string, p githubtest.Person, ago time.Duration, parents ...string) string {
		return srv.AddCommit(repo, githubtest.Commit{Message: msg, Author: p, AuthoredAt: start.Add(-ago),
			Parents: parents, Additions: 4, Deletions: 1})
	}
	srv.AddRepo(githubtest.Repo{ID: 1, Owner: "alice", Name: "projet", Private: true})
	a1 := commit(1, e2eMsgAlice1, alice, 50*time.Hour)
	a2 := commit(1, e2eMsgAlice2, alice, 2*time.Hour, a1)
	srv.SetBranch(1, "main", a2)
	srv.SetBranch(1, "dev", commit(1, e2eMsgDev, bob, time.Hour, a2))
	srv.AddRepo(githubtest.Repo{ID: 2, Owner: "bob", Name: "projet", Private: true})
	b1 := commit(2, e2eMsgBob1, bob, 9*24*time.Hour)
	srv.SetBranch(2, "main", b1)
	srv.AddRepo(githubtest.Repo{ID: 3, Owner: "dave", Name: "projet", Private: true})
	srv.SetBranch(3, "main", commit(3, e2eMsgDave, dave, 42*24*time.Hour))
	srv.AddRepo(githubtest.Repo{ID: 4, Owner: "carol", Name: "cache", Private: true})
	srv.SetRepoAccess(4, githubtest.AccessHidden)

	// 1. init : espace créé, gh vérifié.
	h.run("init").mustSucceed(t)

	// 2. repo add avec label et motif de branches.
	var added struct {
		Outcome    string `json:"outcome"`
		Repository struct {
			FullName       string   `json:"full_name"`
			Label          string   `json:"label"`
			BranchPatterns []string `json:"branch_patterns"`
		} `json:"repository"`
	}
	decodeLoose(t, h.run("repo", "add", "alice/projet", "--label", "Alice Martin", "--branch", "main,dev",
		"--json").mustSucceed(t).stdout, &added)
	if added.Outcome != "added" || added.Repository.FullName != "alice/projet" ||
		added.Repository.Label != "Alice Martin" || strings.Join(added.Repository.BranchPatterns, ",") != "main,dev" {
		t.Fatalf("repo add = %+v", added)
	}

	// 3. repo import : CSV avec un dépôt déjà suivi et un dépôt inaccessible (code de sortie 1).
	csvPath := filepath.Join(t.TempDir(), "promo.csv")
	writeFile(t, csvPath, "repo,label,branches\n# promo factice\nbob/projet,Bob Durand,\n"+
		"https://github.com/dave/projet.git,,\nalice/projet,,\ncarol/cache,,\n")
	dry := h.run("repo", "import", csvPath, "--dry-run", "--json")
	if dry.code != 1 || !strings.Contains(dry.stdout, `"dry_run": true`) {
		t.Fatalf("import --dry-run : code %d\n%s\n%s", dry.code, dry.stdout, dry.stderr)
	}
	e2eAssertRepoCount(t, h, 1) // --dry-run n'écrit rien
	imp := h.run("repo", "import", csvPath, "--json")
	if imp.code != 1 {
		t.Fatalf("import : code %d, attendu 1\n%s\n%s", imp.code, imp.stdout, imp.stderr)
	}
	var importRep struct {
		Summary struct {
			Added          int `json:"added"`
			AlreadyTracked int `json:"already_tracked"`
			Inaccessible   int `json:"inaccessible"`
		} `json:"summary"`
	}
	decodeLoose(t, imp.stdout, &importRep)
	if importRep.Summary.Added != 2 || importRep.Summary.AlreadyTracked != 1 || importRep.Summary.Inaccessible != 1 {
		t.Fatalf("bilan d'import = %+v\n%s", importRep.Summary, imp.stdout)
	}
	e2eAssertStates(t, h, map[string]string{"alice/projet": "never_synced", "bob/projet": "never_synced",
		"dave/projet": "never_synced"})

	// 4. Première synchro : tout l'historique.
	syncRep := e2eSync(t, h)
	if syncRep.Summary.Repositories != 3 || syncRep.Summary.Success != 3 || syncRep.Summary.NewCommits != 5 {
		t.Fatalf("première synchro = %+v", syncRep)
	}
	if syncRep.RateLimit == nil || syncRep.RateLimit.Limit != githubtest.DefaultRateLimit {
		t.Errorf("quota de la synchro = %+v", syncRep.RateLimit)
	}

	// 5. log : tous les commits, puis filtres.
	if got := e2eLog(t, h); len(got) != 5 {
		t.Errorf("log = %d commits, attendu 5", len(got))
	}
	if got := e2eLog(t, h, "--repo", "alice/projet", "--branch", "dev"); len(got) != 1 {
		t.Errorf("log --branch dev = %v", got)
	}
	if got := e2eLog(t, h, "--author", "alice-gh"); len(got) != 2 {
		t.Errorf("log --author alice-gh = %v", got)
	}
	if got := e2eLog(t, h, "--since", "3d"); len(got) != 3 {
		t.Errorf("log --since 3d = %v", got)
	}

	// 6. contributors : Alice (compte) 2, Bob (sans compte, e-mail rapproché) 2, Dave 1.
	var contribs struct {
		Contributors []struct {
			DisplayName  string `json:"display_name"`
			Commits      int    `json:"commits"`
			Repositories int    `json:"repositories"`
		} `json:"contributors"`
	}
	decodeLoose(t, h.run("contributors", "--json").mustSucceed(t).stdout, &contribs)
	gotContribs := map[string][2]int{}
	for _, c := range contribs.Contributors {
		gotContribs[c.DisplayName] = [2]int{c.Commits, c.Repositories}
	}
	want := map[string][2]int{"Alice Martin": {2, 1}, "Bob Durand": {2, 2}, "Dave Petit": {1, 1}}
	if len(gotContribs) != len(want) {
		t.Errorf("contributeurs = %v", gotContribs)
	}
	for name, w := range want {
		if gotContribs[name] != w {
			t.Errorf("contributeur %s = %v, attendu %v", name, gotContribs[name], w)
		}
	}

	// 7. repo list : états Actif / Flemme / Dead.
	e2eAssertStates(t, h, map[string]string{"alice/projet": "active", "bob/projet": "flemme", "dave/projet": "dead"})
	list := h.run("repo", "list").mustSucceed(t).stdout
	for _, part := range []string{"Alice Martin", "~ 9 j", "⚠ 42 j"} {
		if !strings.Contains(list, part) {
			t.Errorf("repo list sans %q :\n%s", part, list)
		}
	}

	// 8. status : compte gh, quota, comptes par état.
	var status struct {
		GitHub struct {
			OK    bool   `json:"ok"`
			Login string `json:"login"`
		} `json:"github"`
		RateLimit *struct {
			Limit int `json:"limit"`
		} `json:"rate_limit"`
		Repositories struct {
			Total  int `json:"total"`
			Active int `json:"active"`
			Flemme int `json:"flemme"`
			Dead   int `json:"dead"`
		} `json:"repositories"`
		TrackedBranches int `json:"tracked_branches"`
	}
	decodeLoose(t, h.run("status", "--json").mustSucceed(t).stdout, &status)
	if !status.GitHub.OK || status.GitHub.Login != "watchoo-test" || status.RateLimit == nil ||
		status.Repositories.Total != 3 || status.Repositories.Active != 1 || status.Repositories.Flemme != 1 ||
		status.Repositories.Dead != 1 || status.TrackedBranches != 4 {
		t.Errorf("status = %+v", status)
	}

	// 9. Nouveaux commits, puis synchro incrémentale : comparaison, jamais la liste complète.
	clk.advance(time.Hour)
	srv.SetBranch(2, "main", srv.AddCommit(2, githubtest.Commit{Message: e2eMsgBob2, Author: bob,
		AuthoredAt: clk.Now().Add(-10 * time.Minute), Parents: []string{b1}, Additions: 12, Deletions: 3}))
	srv.ResetRequests()
	syncRep = e2eSync(t, h)
	if syncRep.Summary.Success != 3 || syncRep.Summary.NewCommits != 1 {
		t.Fatalf("synchro incrémentale = %+v", syncRep)
	}
	for _, r := range syncRep.Repositories {
		if r.FullName == "bob/projet" && (r.NewCommits != 1 || r.StatsFetched != 1) {
			t.Errorf("bob/projet = %+v", r)
		}
	}
	for _, r := range srv.Requests() {
		if r.Method != "GET" {
			t.Errorf("requête en écriture : %s %s", r.Method, r.Path)
		}
		if strings.HasSuffix(r.Path, "/commits") {
			t.Errorf("liste des commits relue lors d'une synchro incrémentale : %s", r.Path)
		}
	}
	e2eAssertStates(t, h, map[string]string{"alice/projet": "active", "bob/projet": "active", "dave/projet": "dead"})

	// Synchro sans changement : aucune requête de commits.
	srv.ResetRequests()
	if rep := e2eSync(t, h); rep.Summary.NewCommits != 0 || rep.Summary.Success != 3 {
		t.Errorf("synchro sans changement = %+v", rep)
	}
	for _, r := range srv.Requests() {
		if strings.Contains(r.Path, "/commits") || strings.Contains(r.Path, "/compare/") {
			t.Errorf("requête de commits alors que rien n'a changé : %s", r.Path)
		}
	}

	// 10. repo remove : --keep-history arrête le suivi ; sans, le dépôt et son historique disparaissent.
	h.run("repo", "remove", "dave/projet").mustFail(t, "--yes") // hors terminal, --yes obligatoire
	h.run("repo", "remove", "dave/projet", "--keep-history", "--yes").mustSucceed(t)
	h.run("repo", "remove", "bob/projet", "--yes").mustSucceed(t)
	e2eAssertStates(t, h, map[string]string{"alice/projet": "active", "dave/projet": "stopped"})
	if got := e2eLog(t, h); len(got) != 4 { // alice (3) + dave (1, historique gardé)
		t.Errorf("log après suppression = %d commits, attendu 4", len(got))
	}
	if rep := e2eSync(t, h); rep.Summary.Repositories != 1 {
		t.Errorf("synchro après arrêt du suivi = %+v", rep.Summary)
	}

	// Aucun message de commit dans les logs (le token est vérifié par le harness à chaque commande).
	logs := testLogs.String()
	for _, msg := range []string{e2eMsgAlice1, e2eMsgAlice2, e2eMsgDev, e2eMsgBob1, e2eMsgBob2, e2eMsgDave,
		"alice@example.test", "bob@example.test"} {
		if strings.Contains(logs, msg) {
			t.Errorf("donnée personnelle %q dans les logs", msg)
		}
	}
}

// e2eSyncReport est la partie lue de la sortie JSON de « watchoo sync ».
type e2eSyncReport struct {
	Summary struct {
		Repositories int `json:"repositories"`
		Success      int `json:"success"`
		NewCommits   int `json:"new_commits"`
	} `json:"summary"`
	Repositories []struct {
		FullName     string `json:"full_name"`
		Status       string `json:"status"`
		NewCommits   int    `json:"new_commits"`
		StatsFetched int    `json:"stats_fetched"`
		Error        string `json:"error"`
	} `json:"repositories"`
	RateLimit *struct {
		Limit int `json:"limit"`
	} `json:"rate_limit"`
}

func e2eSync(t *testing.T, h *harness) e2eSyncReport {
	t.Helper()
	var rep e2eSyncReport
	decodeLoose(t, h.run("sync", "--json").mustSucceed(t).stdout, &rep)
	return rep
}

func e2eLog(t *testing.T, h *harness, args ...string) []string {
	t.Helper()
	var out struct {
		Commits []struct {
			SHA string `json:"sha"`
		} `json:"commits"`
	}
	decodeLoose(t, h.run(append([]string{"log", "--json", "-n", "0"}, args...)...).mustSucceed(t).stdout, &out)
	shas := make([]string, 0, len(out.Commits))
	for _, c := range out.Commits {
		shas = append(shas, c.SHA)
	}
	return shas
}

// repoStates renvoie l'état d'activité de chaque dépôt de « watchoo repo list --json ».
func e2eStates(t *testing.T, h *harness) map[string]string {
	t.Helper()
	var out struct {
		Repositories []struct {
			FullName string `json:"full_name"`
			Activity struct {
				State string `json:"state"`
			} `json:"activity"`
		} `json:"repositories"`
	}
	decodeLoose(t, h.run("repo", "list", "--json").mustSucceed(t).stdout, &out)
	states := map[string]string{}
	for _, r := range out.Repositories {
		states[r.FullName] = r.Activity.State
	}
	return states
}

func e2eAssertStates(t *testing.T, h *harness, want map[string]string) {
	t.Helper()
	got := e2eStates(t, h)
	if len(got) != len(want) {
		t.Errorf("dépôts = %v, attendu %v", got, want)
	}
	for name, state := range want {
		if got[name] != state {
			t.Errorf("état de %s = %q, attendu %q", name, got[name], state)
		}
	}
}

func e2eAssertRepoCount(t *testing.T, h *harness, n int) {
	t.Helper()
	if got := e2eStates(t, h); len(got) != n {
		t.Errorf("%d dépôts suivis, attendu %d : %v", len(got), n, got)
	}
}

// decodeLoose décode la sortie JSON stdout dans v (champs inconnus ignorés : le test de bout en bout
// ne lit que ce qu'il vérifie ; le format complet est fixé par les tests de chaque commande).
func decodeLoose(t *testing.T, stdout string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(stdout), v); err != nil {
		t.Fatalf("sortie JSON invalide : %v\n%s", err, stdout)
	}
}
