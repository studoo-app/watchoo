package sync_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/provider/providertest"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
	wsync "github.com/studoo-app/watchoo/internal/sync"
)

// req construit une demande d'ajout.
func req(line int, owner, name, label string, patterns ...string) wsync.AddRequest {
	r := wsync.AddRequest{Line: line, Ref: repoimport.Ref{Owner: owner, Name: name}, Label: label}
	if patterns != nil {
		r.BranchPatterns = patterns
	}
	return r
}

// forgeRepo ajoute un dépôt au faux Provider seulement.
func forgeRepo(e *env, id int64, name string) {
	e.fake.AddRepository(provider.Repository{ID: id, Owner: testOwner, Name: name, DefaultBranch: "main",
		URL: "https://github.com/" + testOwner + "/" + name, Stars: 2, OpenIssues: 1, Private: true})
}

func TestAddRepositories(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	forgeRepo(e, 3, "sso")
	e.fake.SetRepoError(testOwner, "sso", &provider.SSORequiredError{Organization: testOwner})

	var progress [][2]int
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, "PROMO-DEMO", "alpha", "Alice et Bob", "main", "release/*"),
		req(2, testOwner, "beta", ""),
		req(3, testOwner, "inconnu", ""),
		req(4, testOwner, "sso", ""),
	}, wsync.AddOptions{Progress: func(done, total int) { progress = append(progress, [2]int{done, total}) }})
	if err != nil {
		t.Fatal(err)
	}
	want := []wsync.AddOutcome{wsync.OutcomeAdded, wsync.OutcomeAdded, wsync.OutcomeInaccessible, wsync.OutcomeInaccessible}
	for i, r := range res {
		if r.Outcome != want[i] {
			t.Errorf("résultat %d = %+v, %s attendu", i, r, want[i])
		}
		if r.Request.Line != i+1 {
			t.Errorf("résultat %d : ligne %d", i, r.Request.Line)
		}
	}
	alpha := res[0].Repo
	if alpha.ID == 0 || alpha.Owner != testOwner || alpha.Label != "Alice et Bob" || !alpha.Enabled ||
		!slices.Equal(alpha.BranchPatterns, []string{"main", "release/*"}) || alpha.ProviderID != 1 ||
		alpha.Stars != 2 || alpha.DefaultBranch != "main" || alpha.Provider != model.ProviderGitHub {
		t.Errorf("dépôt ajouté = %+v", alpha)
	}
	if res[2].Error != "dépôt introuvable ou compte sans accès" || res[2].Repo.ID != 0 {
		t.Errorf("inaccessible = %+v", res[2])
	}
	if !strings.Contains(res[3].Error, "SSO") {
		t.Errorf("SSO = %+v", res[3])
	}
	if !slices.Equal(progress, [][2]int{{1, 4}, {2, 4}, {3, 4}, {4, 4}}) {
		t.Errorf("progression = %v", progress)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 4 {
		t.Errorf("Repository appelé %d fois", n)
	}
	repos, _ := e.st.ListRepositories(t.Context())
	if len(repos) != 2 {
		t.Errorf("%d dépôts en base", len(repos))
	}

	// Idempotent : déjà suivi ; label et motifs mis à jour seulement s'ils sont fournis.
	res, err = e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", ""),
		req(2, testOwner, "alpha", "Alice et Bob"),
		req(3, testOwner, "beta", "Bêta", "dev"),
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != wsync.OutcomeAlreadyTracked || res[1].Outcome != wsync.OutcomeAlreadyTracked ||
		res[2].Outcome != wsync.OutcomeUpdated {
		t.Errorf("résultats = %+v", res)
	}
	if res[0].Repo.Label != "Alice et Bob" || !slices.Equal(res[0].Repo.BranchPatterns, []string{"main", "release/*"}) {
		t.Errorf("réglages écrasés : %+v", res[0].Repo)
	}
	beta, _ := e.st.GetRepository(t.Context(), res[2].Repo.ID)
	if beta.Label != "Bêta" || !slices.Equal(beta.BranchPatterns, []string{"dev"}) {
		t.Errorf("beta en base = %+v", beta)
	}

	// Suivi arrêté (repo remove --keep-history) : réactivé par un nouvel ajout.
	if err := e.st.SetRepositoryEnabled(t.Context(), alpha.ID, false); err != nil {
		t.Fatal(err)
	}
	res, _ = e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, testOwner, "alpha", "")}, wsync.AddOptions{})
	if res[0].Outcome != wsync.OutcomeUpdated || !res[0].Repo.Enabled {
		t.Errorf("réactivation = %+v", res[0])
	}
	if stored, _ := e.st.GetRepository(t.Context(), alpha.ID); !stored.Enabled {
		t.Error("suivi non réactivé en base")
	}
}

// Dépôt renommé : demandé par son ancien nom, enregistré sous le nom actuel ; retrouvé ensuite par
// son identifiant de forge.
func TestAddRenomme(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "ancien")
	e.fake.UpdateRepository(provider.Repository{ID: 1, Owner: "nouvelle-orga", Name: "nouveau"})
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, testOwner, "ancien", "")}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != wsync.OutcomeAdded || res[0].Repo.FullName() != "nouvelle-orga/nouveau" {
		t.Errorf("résultat = %+v", res[0])
	}
	res, _ = e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, "nouvelle-orga", "nouveau", "")}, wsync.AddOptions{})
	if res[0].Outcome != wsync.OutcomeAlreadyTracked {
		t.Errorf("second ajout = %+v", res[0])
	}
}

// DryRun : mêmes résultats, aucune écriture, doublons du lot cohérents.
func TestAddDryRun(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	existing, err := e.st.CreateRepository(t.Context(), model.Repository{Provider: model.ProviderGitHub, ProviderID: 2,
		Owner: testOwner, Name: "beta", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", "A"),
		req(2, testOwner, "alpha", "A"),
		req(3, testOwner, "beta", "B"),
		req(4, testOwner, "beta", "B"),
	}, wsync.AddOptions{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	want := []wsync.AddOutcome{wsync.OutcomeAdded, wsync.OutcomeAlreadyTracked, wsync.OutcomeUpdated, wsync.OutcomeAlreadyTracked}
	for i, r := range res {
		if r.Outcome != want[i] {
			t.Errorf("résultat %d = %s, %s attendu", i, r.Outcome, want[i])
		}
	}
	if res[0].Repo.Label != "A" || res[0].Repo.ID != 0 || res[2].Repo.Label != "B" {
		t.Errorf("résultats = %+v", res)
	}
	repos, _ := e.st.ListRepositories(t.Context())
	if len(repos) != 1 || repos[0].Label != existing.Label {
		t.Errorf("DryRun a écrit : %+v", repos)
	}
}

// Quota épuisé : ce dépôt et les suivants en erreur, sans nouvelle requête.
func TestAddQuotaEpuise(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	forgeRepo(e, 3, "gamma")
	e.fake.FailNext(providertest.MethodRepository, &provider.APIError{Op: "lecture", Status: 403,
		Err: &provider.RateLimitError{Reset: time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)}})
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", ""), req(2, testOwner, "beta", ""), req(3, testOwner, "gamma", ""),
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range res {
		if r.Outcome != wsync.OutcomeError || !strings.Contains(r.Error, "quota") {
			t.Errorf("résultat = %+v", r)
		}
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 1 {
		t.Errorf("Repository appelé %d fois", n)
	}
	if _, err := e.st.GetRepositoryByProviderID(t.Context(), model.ProviderGitHub, 2); err == nil {
		t.Error("dépôt ajouté malgré le quota")
	}
}

// Autre échec (réseau) : OutcomeError pour ce dépôt seulement ; token masqué.
func TestAddAutreErreur(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	forgeRepo(e, 2, "beta")
	e.fake.FailNext(providertest.MethodRepository, &provider.APIError{Op: "lecture", Err: provider.ErrUnavailable})
	res, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{
		req(1, testOwner, "alpha", ""), req(2, testOwner, "beta", ""),
	}, wsync.AddOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Outcome != wsync.OutcomeError || res[1].Outcome != wsync.OutcomeAdded {
		t.Errorf("résultats = %+v", res)
	}
	for _, r := range res {
		assertNoLeak(t, "résultat", r.Error)
	}
	if _, err := e.st.GetRepositoryByProviderID(t.Context(), model.ProviderGitHub, 1); err == nil {
		t.Error("dépôt en erreur ajouté")
	} else if !strings.Contains(err.Error(), store.ErrNotFound.Error()) {
		t.Errorf("lecture : %v", err)
	}
}

// Contexte annulé : erreur globale.
func TestAddAnnule(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := e.eng.AddRepositories(ctx, []wsync.AddRequest{req(1, testOwner, "alpha", "")}, wsync.AddOptions{}); err == nil {
		t.Error("contexte annulé : erreur attendue")
	}
}

// Un label invalide (caractère de contrôle) est refusé avant tout appel à la forge, comme dans
// l'import.
func TestAddLabelInvalide(t *testing.T) {
	e := newEnv(t)
	forgeRepo(e, 1, "alpha")
	_, err := e.eng.AddRepositories(t.Context(), []wsync.AddRequest{req(0, testOwner, "alpha", "a\x1b[2Jb")},
		wsync.AddOptions{})
	if err == nil || !strings.Contains(err.Error(), "caractère de contrôle interdit") {
		t.Fatalf("erreur = %v", err)
	}
	if n := e.fake.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository appelé %d fois", n)
	}
	if repos, err := e.st.ListRepositories(t.Context()); err != nil || len(repos) != 0 {
		t.Errorf("dépôts en base = %v, %v", repos, err)
	}
}
