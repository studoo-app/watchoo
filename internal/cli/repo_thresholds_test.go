package cli

import (
	"path/filepath"
	"strconv"
	"testing"

	"github.com/studoo-app/watchoo/internal/provider/providertest"
)

// thresholdPair décrit les seuils propres d'un dépôt enregistré (« 3/10 », « -/- » = seuils de
// l'espace).
func thresholdPair(t *testing.T, h *harness, owner, name string) string {
	t.Helper()
	repo := storedRepo(t, h, owner, name)
	s := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	return s(repo.FlemmeAfterDays) + "/" + s(repo.DeadAfterDays)
}

// « repo add --flemme / --dead » : seuils enregistrés, affichés, exposés en JSON et appliqués par
// « repo list ».
func TestRepoAddThresholds(t *testing.T) {
	h := newSyncedWorkspace(t) // bob/projet : dernier commit il y a 9 jours (Flemme avec 5 / 15)

	r := h.run("repo", "add", "bob/projet", "--flemme", "10", "--dead", "20").mustSucceed(t)
	assertContains(t, "repo add --flemme", r.stdout,
		"Dépôt bob/projet déjà suivi, mis à jour (toutes les branches, seuils : flemme 10 j, dead 20 j).")
	if got := thresholdPair(t, h, "bob", "projet"); got != "10/20" {
		t.Errorf("seuils enregistrés = %s", got)
	}
	r = h.run("repo", "add", "bob/projet", "--flemme", "10").mustSucceed(t)
	assertContains(t, "mêmes seuils", r.stdout, "rien à changer")

	// repo list : bob est Actif avec un seuil Flemme de 10 jours.
	r = h.run("repo", "list", "--json").mustSucceed(t)
	assertContains(t, "repo list --json", r.stdout, `"flemme_after_days": 10`, `"dead_after_days": 20`)
	var list repoListReport
	decodeStrict(t, r.stdout, &list)
	for _, item := range list.Repositories {
		if item.FullName != "bob/projet" {
			if item.FlemmeAfterDays != nil || item.DeadAfterDays != nil {
				t.Errorf("%s : seuils propres %v/%v, attendu null", item.FullName, item.FlemmeAfterDays, item.DeadAfterDays)
			}
			continue
		}
		if item.Activity.State != "active" || item.Activity.FlemmeAfterDays != 10 || item.Activity.DeadAfterDays != 20 {
			t.Errorf("activité de bob = %+v", item.Activity)
		}
	}

	// --dead seul, plus petit que le seuil Flemme de l'espace : Dead reste au-dessus de Flemme (5).
	r = h.run("repo", "add", "bob/projet", "--flemme", "0", "--dead", "8", "--json").mustSucceed(t)
	var rep repoAddReport
	decodeStrict(t, r.stdout, &rep)
	if rep.Outcome != "updated" || rep.Repository.FlemmeAfterDays != nil || rep.Repository.DeadAfterDays == nil ||
		*rep.Repository.DeadAfterDays != 8 {
		t.Errorf("repo add --flemme 0 --dead 8 --json = %+v", rep)
	}
	r = h.run("repo", "list").mustSucceed(t)
	assertContains(t, "repo list Dead", r.stdout, "bob/projet    il y a 9 j      Bob Durand  ⚠ 9 j")

	// 0 et 0 : retour aux seuils de l'espace.
	h.run("repo", "add", "bob/projet", "--flemme", "0", "--dead", "0").mustSucceed(t)
	if got := thresholdPair(t, h, "bob", "projet"); got != "-/-" {
		t.Errorf("seuils après 0 / 0 = %s", got)
	}
}

func TestRepoAddThresholdsErrors(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--flemme", "10", "--dead", "5").mustFail(t,
		"--seuil dead (5 j) : doit être strictement supérieur au seuil flemme (10 j)")
	h.run("repo", "add", "alice/projet", "--flemme", "-1").mustFail(t, "--seuil flemme : -1 jours invalide")
	h.run("repo", "add", "alice/projet", "--dead", "abc").mustFail(t, "--dead")
	if n := h.Provider.Calls(providertest.MethodRepository); n != 0 {
		t.Errorf("Repository appelé %d fois malgré des seuils invalides", n)
	}
}

// Colonnes flemme et dead du fichier d'import : cellule vide = non fourni.
func TestRepoImportThresholds(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	writeFile(t, filepath.Join(h.cwd, "promo.csv"), "repo,label,flemme,dead\nalice/projet,Alice,3,10\nbob/projet,,,7\n")
	h.run("repo", "import", "promo.csv").mustSucceed(t)
	if got := thresholdPair(t, h, "alice", "projet"); got != "3/10" {
		t.Errorf("alice : %s", got)
	}
	if got := thresholdPair(t, h, "bob", "projet"); got != "-/7" {
		t.Errorf("bob : %s", got)
	}

	// Réimport : cellule vide = seuil gardé, 0 = seuil de l'espace.
	writeFile(t, filepath.Join(h.cwd, "promo.csv"), "repo,flemme,dead\nalice/projet,,0\nbob/projet,,\n")
	r := h.run("repo", "import", "promo.csv", "--json").mustSucceed(t)
	assertContains(t, "réimport", r.stdout, `"updated": 1`, `"already_tracked": 1`)
	if got := thresholdPair(t, h, "alice", "projet"); got != "3/-" {
		t.Errorf("alice après réimport : %s", got)
	}

	// Seuils incohérents : ligne en erreur, rien d'importé.
	writeFile(t, filepath.Join(h.cwd, "faux.csv"), "repo,flemme,dead\ndave/projet,9,4\n")
	h.run("repo", "import", "faux.csv").mustFail(t, "ligne 2", "strictement supérieur")
}

// « config import » : seuils propres dans l'aperçu (texte et JSON), puis appliqués ; « config export »
// les écrit.
func TestConfigImportThresholds(t *testing.T) {
	h := newWorkspace(t)
	seedProvider(h.Provider)
	h.run("repo", "add", "alice/projet", "--dead", "20").mustSucceed(t)
	writeFile(t, filepath.Join(h.cwd, "partage.json"), `{"format":"watchoo-config","version":1,
"repositories":[{"repo":"alice/projet","flemme_after_days":3,"dead_after_days":0}]}`)

	r := h.run("config", "import", "partage.json", "--dry-run").mustSucceed(t)
	assertContains(t, "aperçu", r.stdout, "alice/projet (flemme : espace → 3 j ; dead : 20 j → espace)")
	r = h.run("config", "import", "partage.json", "--dry-run", "--json").mustSucceed(t)
	var rep configImportReport
	decodeStrict(t, r.stdout, &rep)
	c := rep.Repositories[0]
	if c.Action != "update" || c.Before == nil || c.Before.FlemmeAfterDays != nil || c.Before.DeadAfterDays == nil ||
		*c.Before.DeadAfterDays != 20 || c.After.FlemmeAfterDays == nil || *c.After.FlemmeAfterDays != 3 ||
		c.After.DeadAfterDays != nil {
		t.Errorf("aperçu --json = %+v (avant %+v, après %+v)", c, c.Before, c.After)
	}
	if got := thresholdPair(t, h, "alice", "projet"); got != "-/20" {
		t.Errorf("--dry-run a écrit : %s", got)
	}

	h.run("config", "import", "partage.json", "--yes").mustSucceed(t)
	if got := thresholdPair(t, h, "alice", "projet"); got != "3/-" {
		t.Errorf("après import : %s", got)
	}
	r = h.run("config", "export").mustSucceed(t)
	assertContains(t, "export", r.stdout, `"flemme_after_days": 3`)
}
