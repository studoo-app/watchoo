package configshare_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
)

// pair décrit des seuils propres (« 3/10 », « -/- » = seuils de l'espace).
func pair(flemme, dead *int) string {
	s := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	return s(flemme) + "/" + s(dead)
}

// Les seuils propres d'un dépôt sont exportés (seulement s'il en a), relus, prévus et appliqués.
func TestThresholdsExportImport(t *testing.T) {
	isolate(t)
	a := newEnv(t, "licence-a")
	alice := a.track(1, "alice", "projet", "Alice")
	a.track(2, "bob", "projet", "")
	three, ten := 3, 10
	if _, err := a.st.SetRepositoryThresholds(t.Context(), alice.ID, &three, &ten); err != nil {
		t.Fatal(err)
	}
	d, err := configshare.Export(t.Context(), a.ws, a.st, configshare.ExportOptions{Now: testNow})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := d.Marshal()
	if !strings.Contains(string(data), `"flemme_after_days": 3,`) || !strings.Contains(string(data), `"dead_after_days": 10`) {
		t.Errorf("seuils absents de l'export :\n%s", data)
	}
	// bob n'a pas de seuil propre : pas de clé (fichier lisible par les versions précédentes).
	_, bobPart, _ := strings.Cut(string(data), `"repo": "bob/projet"`)
	if strings.Count(string(data), "flemme_after_days") != 2 || strings.Contains(bobPart, "dead_after_days") {
		t.Errorf("seuils écrits pour un dépôt sans seuil propre :\n%s", data)
	}

	// b : alice absente, bob présent avec un seuil propre que le document ne donne pas (gardé).
	b := newEnv(t, "licence-b")
	b.forge(1, "alice", "projet")
	bob := b.track(2, "bob", "projet", "")
	seven := 7
	if _, err := b.st.SetRepositoryThresholds(t.Context(), bob.ID, nil, &seven); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, string(data))
	if got := pair(f.Repositories[0].FlemmeAfterDays, f.Repositories[0].DeadAfterDays); got != "3/10" {
		t.Errorf("seuils relus = %s", got)
	}
	p, err := configshare.Plan(t.Context(), b.ws, b.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := p.Repositories[0]; c.Action != configshare.ActionAdd || pair(c.After.FlemmeAfterDays, c.After.DeadAfterDays) != "3/10" {
		t.Errorf("aperçu alice = %+v", c)
	}
	if c := p.Repositories[1]; c.Action != configshare.ActionUnchanged || pair(c.After.FlemmeAfterDays, c.After.DeadAfterDays) != "-/7" {
		t.Errorf("aperçu bob = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	repos := b.repos()
	if r := repos["alice/projet"]; pair(r.FlemmeAfterDays, r.DeadAfterDays) != "3/10" {
		t.Errorf("alice importée = %s", pair(r.FlemmeAfterDays, r.DeadAfterDays))
	}
	if r := repos["bob/projet"]; pair(r.FlemmeAfterDays, r.DeadAfterDays) != "-/7" {
		t.Errorf("bob = %s", pair(r.FlemmeAfterDays, r.DeadAfterDays))
	}

	// 0 = seuil de l'espace : mise à jour prévue puis appliquée.
	f = mustParse(t, doc("", `[{"repo": "bob/projet", "dead_after_days": 0}]`))
	p, err = configshare.Plan(t.Context(), b.ws, b.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := p.Repositories[0]; c.Action != configshare.ActionUpdate || pair(c.Before.FlemmeAfterDays, c.Before.DeadAfterDays) != "-/7" ||
		pair(c.After.FlemmeAfterDays, c.After.DeadAfterDays) != "-/-" {
		t.Errorf("aperçu 0 = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if r := b.repos()["bob/projet"]; r.DeadAfterDays != nil {
		t.Errorf("bob après 0 : %s", pair(r.FlemmeAfterDays, r.DeadAfterDays))
	}
}

func TestThresholdsParseRejects(t *testing.T) {
	for in, want := range map[string]string{
		`[{"repo": "alice/projet", "flemme_after_days": 10, "dead_after_days": 4}]`: "dépôt n° 1 (alice/projet) : seuil dead (4 j) : doit être strictement supérieur",
		`[{"repo": "alice/projet", "flemme_after_days": -1}]`:                       "seuil flemme : -1 jours invalide",
		`[{"repo": "alice/projet", "dead_after_days": "5"}]`:                        "type",
	} {
		_, err := configshare.Parse(strings.NewReader(doc("", in)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, attendu %q", in, err, want)
		}
	}
}
