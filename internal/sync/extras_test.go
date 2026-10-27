package sync_test

// Pull requests, releases et tags (V2) : premier passage, synchro incrémentale des PR (UpdatedSince),
// remplacement des releases et tags, échec isolé (avertissement, commits gardés), quota bas et
// épuisé, source sans cette notion (ErrUnsupported), annulation.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// secretTitle est un titre de PR confidentiel : jamais dans les logs ni les rapports.
const secretTitle = "Titre de PR confidentiel"

func init() { forbidden = append(forbidden, secretTitle) }

// fakePR renvoie une PR factice n, modifiée à epoch + h heures, de l'auteur secret.
func fakePR(n int, state string, h int) provider.PullRequest {
	at := epoch.Add(time.Duration(h) * time.Hour)
	pr := provider.PullRequest{
		Number: n, Title: secretTitle, State: state, URL: "https://github.example.test/pull",
		Author:    provider.Account{Login: secretLogin, ID: 4242},
		CreatedAt: epoch, UpdatedAt: at, BaseBranch: "main", HeadBranch: "feature",
	}
	if state == provider.PullRequestClosed {
		pr.ClosedAt = at
	}
	return pr
}

// pulls renvoie les PR en base du dépôt repoID, de la plus récemment modifiée à la plus ancienne.
func (e *env) pulls(repoID int64) []store.PullRequestView {
	e.t.Helper()
	views, err := e.st.ListPullRequests(e.t.Context(), store.PullRequestFilter{RepoID: repoID})
	if err != nil {
		e.t.Fatalf("ListPullRequests : %v", err)
	}
	return views
}

// pullCalls renvoie les options des appels à PullRequests reçus par le faux Provider.
func (e *env) pullCalls() []provider.PullRequestsOptions {
	var out []provider.PullRequestsOptions
	for _, c := range e.fake.CallLog() {
		if c.Method == providertest.MethodPullRequests {
			out = append(out, c.PullOpts)
		}
	}
	return out
}

func TestPullRequestsIncremental(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 2, epoch, time.Hour)
	merged := fakePR(2, provider.PullRequestClosed, 3)
	merged.Merged, merged.MergedAt = true, merged.ClosedAt
	e.fake.SetPullRequests(1, fakePR(1, provider.PullRequestOpen, 1), merged)
	e.fake.SetReleases(1, provider.Release{ID: 7, TagName: "v1", CreatedAt: epoch, PublishedAt: epoch})
	e.fake.SetTags(1, provider.Tag{Name: "v1", SHA: providertest.SHA("x")})

	r := e.syncOK(repo.ID)
	if r.PullRequests != 2 || len(r.Warnings) != 0 || r.Warnings == nil {
		t.Errorf("premier passage : %d PR, avertissements %v", r.PullRequests, r.Warnings)
	}
	if got := e.pulls(repo.ID); len(got) != 2 || got[0].Number != 2 || got[0].Status() != model.PullRequestStatusMerged ||
		got[1].AuthorLogin != secretLogin || got[1].AuthorProviderID != 4242 {
		t.Errorf("PR en base : %+v", got)
	}
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.IsZero() {
		t.Errorf("premier appel : %+v, veut UpdatedSince zéro", calls)
	}

	// Synchro suivante : seulement les PR modifiées depuis la plus récente date en base.
	reopened := fakePR(1, provider.PullRequestClosed, 5)
	e.fake.SetPullRequests(1, reopened, fakePR(3, provider.PullRequestOpen, 4))
	e.fake.ResetCalls()
	r = e.syncOK(repo.ID)
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.Equal(epoch.Add(3*time.Hour)) {
		t.Errorf("appel incrémental : %+v, veut UpdatedSince = %v", calls, epoch.Add(3*time.Hour))
	}
	// La PR 2 (même seconde que le point de reprise) est relue, sans doublon.
	if r.PullRequests != 3 {
		t.Errorf("synchro incrémentale : %d PR lues, veut 3", r.PullRequests)
	}
	got := e.pulls(repo.ID)
	if len(got) != 3 || got[0].Number != 1 || got[0].State != model.PullRequestClosed {
		t.Errorf("PR après mise à jour : %+v", got)
	}

	// Releases et tags remplacés : la release supprimée sur la forge disparaît.
	e.fake.SetReleases(1)
	e.syncOK(repo.ID)
	if rel, _ := e.st.ListReleases(t.Context(), store.ReleaseFilter{RepoID: repo.ID}); len(rel) != 0 {
		t.Errorf("releases après suppression sur la forge : %+v", rel)
	}
	if tags, _ := e.st.ListTags(t.Context(), store.ReleaseFilter{RepoID: repo.ID}); len(tags) != 1 {
		t.Errorf("tags : %+v", tags)
	}
}

// Une erreur sur une liste ne fait pas échouer le dépôt : commits enregistrés, avertissement, rien
// d'effacé ; les autres listes sont lues ; la synchro suivante reprend au même point.
func TestPullRequestsFailureIsolated(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetPullRequests(1, fakePR(1, provider.PullRequestOpen, 1))
	e.fake.SetReleases(1, provider.Release{ID: 7, TagName: "v1", CreatedAt: epoch})
	e.syncOK(repo.ID)

	e.fake.AppendCommits(1, "main", 1, epoch.Add(5*time.Hour), time.Hour)
	e.fake.SetPullRequests(1, fakePR(2, provider.PullRequestOpen, 6))
	e.fake.SetReleases(1, provider.Release{ID: 8, TagName: "v2", CreatedAt: epoch})
	unavailable := &provider.APIError{Op: "liste des pull requests de promo-demo/projet", Method: "GET",
		Path: "/repos/promo-demo/projet/pulls", Status: 502, Err: provider.ErrUnavailable}
	e.fake.FailNext(providertest.MethodPullRequests, unavailable)
	e.fake.FailNext(providertest.MethodTags, &provider.APIError{Status: 404, Err: provider.ErrNotFound})
	r := e.syncOK(repo.ID)
	if r.NewCommits != 1 {
		t.Errorf("nouveaux commits : %d, veut 1 (commits gardés malgré l'échec des PR)", r.NewCommits)
	}
	if len(r.Warnings) != 2 || !strings.HasPrefix(r.Warnings[0], "pull requests non synchronisées : ") ||
		!strings.Contains(r.Warnings[0], "HTTP 502") || !strings.HasPrefix(r.Warnings[1], "tags non synchronisés : ") {
		t.Errorf("avertissements : %q", r.Warnings)
	}
	if got := e.pulls(repo.ID); len(got) != 1 || got[0].Number != 1 {
		t.Errorf("PR en base après l'échec : %+v (gardées, sans la nouvelle)", got)
	}
	if rel, _ := e.st.ListReleases(t.Context(), store.ReleaseFilter{RepoID: repo.ID}); len(rel) != 1 || rel[0].TagName != "v2" {
		t.Errorf("releases lues malgré l'échec des PR : %+v", rel)
	}
	if run := e.lastRun(repo.ID); run.Status != model.SyncSuccess {
		t.Errorf("sync_run = %+v, veut success", run)
	}
	if !strings.Contains(e.logs.String(), "liste non synchronisée") {
		t.Error("échec non journalisé")
	}

	// Synchro suivante : reprise au même point, la PR 2 est récupérée.
	e.fake.ResetCalls()
	r = e.syncOK(repo.ID)
	if len(r.Warnings) != 0 || len(e.pulls(repo.ID)) != 2 {
		t.Errorf("reprise : avertissements %q, PR %+v", r.Warnings, e.pulls(repo.ID))
	}
	if calls := e.pullCalls(); len(calls) != 1 || !calls[0].UpdatedSince.Equal(epoch.Add(time.Hour)) {
		t.Errorf("reprise : %+v", calls)
	}
}

// Source sans pull requests ni releases (futur provider Git local) : ignorées sans avertissement,
// rien d'effacé, les tags sont lus.
func TestPullRequestsUnsupported(t *testing.T) {
	e := newEnv(t)
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e.fake.SetReleases(1, provider.Release{ID: 7, TagName: "v1", CreatedAt: epoch})
	e.syncOK(repo.ID)

	e.fake.SetError(providertest.MethodPullRequests, provider.ErrUnsupported)
	e.fake.SetError(providertest.MethodReleases, provider.ErrUnsupported)
	e.fake.SetTags(1, provider.Tag{Name: "v1", SHA: providertest.SHA("x")})
	r := e.syncOK(repo.ID)
	if len(r.Warnings) != 0 {
		t.Errorf("avertissements pour une source sans PR : %q", r.Warnings)
	}
	if rel, _ := e.st.ListReleases(t.Context(), store.ReleaseFilter{}); len(rel) != 1 {
		t.Errorf("releases effacées : %+v", rel)
	}
	if tags, _ := e.st.ListTags(t.Context(), store.ReleaseFilter{}); len(tags) != 1 {
		t.Errorf("tags non lus : %+v", tags)
	}
}

// Quota bas : PR, releases et tags non demandés ; quota épuisé pendant la lecture : dépôt réussi,
// listes restantes non lues, dépôts suivants reportés.
func TestPullRequestsQuota(t *testing.T) {
	e := newEnv(t, withParallelism(1))
	e.addRepo(1, "a")
	e.addRepo(2, "b")
	for _, id := range []int64{1, 2} {
		e.fake.AppendCommits(id, "main", 1, epoch, time.Hour)
	}
	reset := time.Date(2026, 10, 20, 11, 5, 0, 0, time.UTC)
	e.fake.FailNext(providertest.MethodReleases, &provider.APIError{Status: 403, Err: &provider.RateLimitError{Reset: reset}})
	rep := e.syncAll()
	first := rep.Repos[0]
	if first.Status != wsync.StatusSuccess || len(first.Warnings) != 1 ||
		!strings.HasPrefix(first.Warnings[0], "releases et tags non synchronisés : ") {
		t.Errorf("premier dépôt : %+v", first)
	}
	if n := e.fake.Calls(providertest.MethodTags); n != 0 {
		t.Errorf("Tags appelé %d fois après le quota épuisé", n)
	}
	if second := rep.Repos[1]; second.Status != wsync.StatusDeferred || !second.ResumeAt.Equal(reset) {
		t.Errorf("dépôt suivant : %+v", second)
	}

	// Quota bas avant les listes (mais pas avant le dépôt) : rien n'est demandé.
	e2 := newEnv(t)
	repo := e2.addRepo(1, "a")
	e2.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	e2.fake.SetRateLimit(provider.RateLimit{Limit: 100, Remaining: 12}) // 9 après dépôt, branches et commits
	r := e2.syncOK(repo.ID)
	if n := e2.fake.Calls(providertest.MethodPullRequests) + e2.fake.Calls(providertest.MethodReleases) +
		e2.fake.Calls(providertest.MethodTags); n != 0 {
		t.Errorf("%d listes demandées sous le quota bas", n)
	}
	if len(r.Warnings) != 1 || r.Warnings[0] != "pull requests, releases et tags non synchronisés : quota bas" {
		t.Errorf("avertissements : %q", r.Warnings)
	}
}

// Annulation pendant la lecture des PR : le dépôt échoue (synchro annulée), rien n'est écrit.
func TestPullRequestsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e := newEnv(t, withWrap(func(p provider.Provider) provider.Provider {
		return &pullsHook{Provider: p, before: cancel}
	}))
	repo := e.addRepo(1, "projet")
	e.fake.AppendCommits(1, "main", 1, epoch, time.Hour)
	rep, err := e.eng.SyncRepo(ctx, repo.ID, wsync.SyncOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("SyncRepo : %v, veut context.Canceled", err)
	}
	if r := rep.Repos[0]; r.Status != wsync.StatusError || r.Error != "synchro annulée" {
		t.Errorf("rapport : %+v", r)
	}
	if got := e.commitSHAs(repo.ID, ""); len(got) != 0 {
		t.Errorf("commits écrits malgré l'annulation : %d", len(got))
	}
}

// pullsHook appelle before avant PullRequests.
type pullsHook struct {
	provider.Provider
	before func()
}

func (h *pullsHook) PullRequests(ctx context.Context, owner, name string, opts provider.PullRequestsOptions) ([]provider.PullRequest, error) {
	h.before()
	return h.Provider.PullRequests(ctx, owner, name, opts)
}
