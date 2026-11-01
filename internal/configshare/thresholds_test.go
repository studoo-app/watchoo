package configshare_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/studoo-app/watchoo/internal/configshare"
)

// pair describes per-repository thresholds ("3/10", "-/-" = thresholds of the workspace).
func pair(flemme, dead *int) string {
	s := func(p *int) string {
		if p == nil {
			return "-"
		}
		return strconv.Itoa(*p)
	}
	return s(flemme) + "/" + s(dead)
}

// The per-repository thresholds are exported (only if there are any), read back, previewed and
// applied.
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
		t.Errorf("thresholds missing from the export:\n%s", data)
	}
	// bob has no threshold of its own: no key (file readable by the previous versions).
	_, bobPart, _ := strings.Cut(string(data), `"repo": "bob/projet"`)
	if strings.Count(string(data), "flemme_after_days") != 2 || strings.Contains(bobPart, "dead_after_days") {
		t.Errorf("thresholds written for a repository without its own:\n%s", data)
	}

	// b: alice absent, bob present with its own threshold that the document does not give (kept).
	b := newEnv(t, "licence-b")
	b.forge(1, "alice", "projet")
	bob := b.track(2, "bob", "projet", "")
	seven := 7
	if _, err := b.st.SetRepositoryThresholds(t.Context(), bob.ID, nil, &seven); err != nil {
		t.Fatal(err)
	}
	f := mustParse(t, string(data))
	if got := pair(f.Repositories[0].FlemmeAfterDays, f.Repositories[0].DeadAfterDays); got != "3/10" {
		t.Errorf("thresholds read back = %s", got)
	}
	p, err := configshare.Plan(t.Context(), b.ws, b.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := p.Repositories[0]; c.Action != configshare.ActionAdd || pair(c.After.FlemmeAfterDays, c.After.DeadAfterDays) != "3/10" {
		t.Errorf("preview alice = %+v", c)
	}
	if c := p.Repositories[1]; c.Action != configshare.ActionUnchanged || pair(c.After.FlemmeAfterDays, c.After.DeadAfterDays) != "-/7" {
		t.Errorf("preview bob = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	repos := b.repos()
	if r := repos["alice/projet"]; pair(r.FlemmeAfterDays, r.DeadAfterDays) != "3/10" {
		t.Errorf("imported alice = %s", pair(r.FlemmeAfterDays, r.DeadAfterDays))
	}
	if r := repos["bob/projet"]; pair(r.FlemmeAfterDays, r.DeadAfterDays) != "-/7" {
		t.Errorf("bob = %s", pair(r.FlemmeAfterDays, r.DeadAfterDays))
	}

	// 0 = threshold of the workspace: update previewed, then applied.
	f = mustParse(t, doc("", `[{"repo": "bob/projet", "dead_after_days": 0}]`))
	p, err = configshare.Plan(t.Context(), b.ws, b.st, f, configshare.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if c := p.Repositories[0]; c.Action != configshare.ActionUpdate || pair(c.Before.FlemmeAfterDays, c.Before.DeadAfterDays) != "-/7" ||
		pair(c.After.FlemmeAfterDays, c.After.DeadAfterDays) != "-/-" {
		t.Errorf("preview 0 = %+v", c)
	}
	if _, err := configshare.Apply(t.Context(), b.ws, b.st, b.eng, f, configshare.ApplyOptions{}); err != nil {
		t.Fatal(err)
	}
	if r := b.repos()["bob/projet"]; r.DeadAfterDays != nil {
		t.Errorf("bob after 0: %s", pair(r.FlemmeAfterDays, r.DeadAfterDays))
	}
}

func TestThresholdsParseRejects(t *testing.T) {
	for in, want := range map[string]string{
		`[{"repo": "alice/projet", "flemme_after_days": 10, "dead_after_days": 4}]`: "repository #1 (alice/projet): dead threshold (4 d): must be strictly greater",
		`[{"repo": "alice/projet", "flemme_after_days": -1}]`:                       "flemme threshold: -1 days is invalid",
		`[{"repo": "alice/projet", "dead_after_days": "5"}]`:                        "type",
	} {
		_, err := configshare.Parse(strings.NewReader(doc("", in)))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v, want %q", in, err, want)
		}
	}
}
