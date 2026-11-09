package conventions

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// hsha renvoie un SHA complet de test.
func hsha(n int) string { return fmt.Sprintf("%040x", n) }

// at renvoie un instant UTC de test (jour de novembre 2026, heure).
func at(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC) }

// dag est un historique de test : parents de chaque commit.
type dag map[string][]string

// anc teste l'ascendance ; un commit absent du graphe est inconnu.
func (d dag) anc(ancestor, descendant string) Ancestry {
	if _, ok := d[ancestor]; !ok {
		return AncestryUnknown
	}
	if _, ok := d[descendant]; !ok {
		return AncestryUnknown
	}
	seen := map[string]bool{}
	stack := []string{descendant}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if c == ancestor {
			return AncestryYes
		}
		if seen[c] {
			continue
		}
		seen[c] = true
		stack = append(stack, d[c]...)
	}
	return AncestryNo
}

// linear renvoie l'historique 1 ← 2 ← … ← n.
func linear(n int) dag {
	d := dag{hsha(1): nil}
	for i := 2; i <= n; i++ {
		d[hsha(i)] = []string{hsha(i - 1)}
	}
	return d
}

func obs(name string, sha int, first, last time.Time, ended *time.Time, reason string) model.TagObservation {
	return model.TagObservation{RepoID: 1, Name: name, SHA: hsha(sha), FirstSeenAt: first, LastSeenAt: last,
		EndedAt: ended, EndReason: reason}
}

func tptr(t time.Time) *time.Time { return &t }

func TestAncestryHelper(t *testing.T) {
	d := linear(3)
	cases := []struct {
		anc  AncestryFunc
		a, b string
		want Ancestry
	}{
		{nil, hsha(1), hsha(2), AncestryUnknown},
		{nil, hsha(1), hsha(1), AncestryYes},
		{d.anc, "", hsha(1), AncestryUnknown},
		{d.anc, hsha(1), "", AncestryUnknown},
		{d.anc, hsha(1), hsha(3), AncestryYes},
		{d.anc, hsha(3), hsha(1), AncestryNo},
		{d.anc, hsha(9), hsha(1), AncestryUnknown},
	}
	for i, c := range cases {
		if got := ancestry(c.anc, c.a, c.b); got != c.want {
			t.Errorf("cas %d : ancestry = %v, attendu %v", i, got, c.want)
		}
	}
	for s, want := range map[string]bool{hsha(1): true, fmt.Sprintf("%064x", 7): true, "abc": false,
		"G" + hsha(1)[1:]: false, "A" + hsha(1)[1:]: false, "": false} {
		if got := isCommitSHA(s); got != want {
			t.Errorf("isCommitSHA(%q) = %v, attendu %v", s, got, want)
		}
	}
}

func TestPlanTagObservations(t *testing.T) {
	tag := func(name string, sha int) model.Tag { return model.Tag{RepoID: 1, Name: name, SHA: hsha(sha)} }
	open := []model.TagObservation{
		obs("v1.0.0", 1, at(1, 0), at(2, 0), nil, ""),                     // revu
		obs("v1.1.0", 2, at(1, 0), at(2, 0), nil, ""),                     // réécrit
		obs("v2.0.0", 3, at(1, 0), at(2, 0), nil, ""),                     // supprimé
		obs("v0.9.0", 4, at(1, 0), at(1, 0), tptr(at(1, 5)), "deleted"),   // déjà fermé : ignoré
		obs("v1.0.0", 1, at(1, 0), at(2, 0), nil, ""),                     // doublon ouvert : ignoré
		obs("v0.9.0", 5, at(1, 0), at(1, 0), tptr(at(1, 5)), "rewritten"), // déjà fermé : ignoré
	}
	current := []model.Tag{
		tag("v1.0.0", 1), tag("v1.1.0", 9), tag("v3.0.0", 7), tag("v3.0.0", 8),
		{RepoID: 1, Name: "", SHA: hsha(1)}, {RepoID: 1, Name: "bad", SHA: "xyz"}, tag("v0.9.0", 5),
	}
	got := PlanTagObservations(open, current)
	want := TagObservationPlan{
		Seen: []model.Tag{tag("v1.0.0", 1)},
		Close: []TagObservationClose{
			{Name: "v1.1.0", SHA: hsha(2), Reason: model.TagEndRewritten},
			{Name: "v2.0.0", SHA: hsha(3), Reason: model.TagEndDeleted},
		},
		Open: []model.Tag{tag("v0.9.0", 5), tag("v1.1.0", 9), tag("v3.0.0", 7)},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("plan = %+v\nattendu %+v", got, want)
	}
	if got.Empty() {
		t.Error("plan non vide jugé vide")
	}

	empty := PlanTagObservations(nil, nil)
	if !empty.Empty() || empty.Seen == nil || empty.Close == nil || empty.Open == nil {
		t.Errorf("plan vide = %+v", empty)
	}
	// Tous les tags supprimés : tout est fermé.
	all := PlanTagObservations(open[:3], nil)
	if len(all.Close) != 3 || len(all.Seen)+len(all.Open) != 0 {
		t.Errorf("suppression totale = %+v", all)
	}
	for _, c := range all.Close {
		if c.Reason != model.TagEndDeleted {
			t.Errorf("fermeture %+v : attendu deleted", c)
		}
	}
}

func TestObservationReads(t *testing.T) {
	got := ObservationReads([]model.TagObservation{
		obs("a", 1, at(3, 0), at(5, 0), nil, ""),
		obs("b", 2, at(1, 0), at(3, 0), tptr(at(4, 0)), "deleted"),
		obs("c", 3, time.Time{}, time.Time{}, tptr(time.Time{}), "deleted"),
	})
	want := []time.Time{at(1, 0), at(3, 0), at(4, 0), at(5, 0)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("lectures = %v, attendu %v", got, want)
	}
	if got := ObservationReads(nil); len(got) != 0 {
		t.Errorf("sans observation = %v", got)
	}
}

func TestBuildTagHistory(t *testing.T) {
	rows := []model.TagObservation{
		// stable
		obs("v1.0.0", 1, at(2, 0), at(9, 0), nil, ""),
		// réécrit : déplacé de 2 vers 3
		obs("v1.1.0", 2, at(2, 0), at(3, 0), tptr(at(4, 0)), model.TagEndRewritten),
		obs("v1.1.0", 3, at(4, 0), at(9, 0), nil, ""),
		// supprimé
		obs("v2.0.0", 4, at(1, 0), at(5, 0), tptr(at(6, 0)), model.TagEndDeleted),
		// supprimé puis reposé sur le même commit
		obs("v0.1.0", 5, at(3, 0), at(4, 0), tptr(at(5, 0)), model.TagEndDeleted),
		obs("v0.1.0", 5, at(7, 0), at(9, 0), nil, ""),
		// supprimé puis reposé ailleurs : réécrit
		obs("v0.2.0", 6, at(3, 0), at(4, 0), tptr(at(5, 0)), model.TagEndDeleted),
		obs("v0.2.0", 7, at(7, 0), at(9, 0), nil, ""),
		// réécrit puis supprimé : supprimé
		obs("v3.0.0", 8, at(3, 0), at(4, 0), tptr(at(5, 0)), model.TagEndRewritten),
		obs("v3.0.0", 9, at(5, 0), at(6, 0), tptr(at(7, 0)), model.TagEndDeleted),
		{RepoID: 1, Name: ""}, // sans nom : ignoré
	}
	rep := BuildTagHistory(rows)
	if !rep.Observed || !rep.Since.Equal(at(1, 0)) {
		t.Fatalf("Observed = %v, Since = %v", rep.Observed, rep.Since)
	}
	want := map[string]struct {
		status        string
		sha           string
		first         time.Time
		rew, del, per int
		conform       bool
	}{
		"v0.1.0": {TagHistoryRestored, hsha(5), at(3, 0), 0, 1, 2, true},
		"v0.2.0": {TagHistoryRewritten, hsha(7), at(3, 0), 0, 1, 2, false},
		"v1.0.0": {TagHistoryStable, hsha(1), at(2, 0), 0, 0, 1, true},
		"v1.1.0": {TagHistoryRewritten, hsha(3), at(2, 0), 1, 0, 2, false},
		"v2.0.0": {TagHistoryDeleted, "", at(1, 0), 0, 1, 1, false},
		"v3.0.0": {TagHistoryDeleted, "", at(3, 0), 1, 1, 2, false},
	}
	if len(rep.Tags) != len(want) {
		t.Fatalf("%d tags, attendu %d", len(rep.Tags), len(want))
	}
	for i, h := range rep.Tags {
		if i > 0 && rep.Tags[i-1].Name >= h.Name {
			t.Errorf("ordre : %s puis %s", rep.Tags[i-1].Name, h.Name)
		}
		w := want[h.Name]
		if h.Status != w.status || h.SHA != w.sha || !h.FirstSeenAt.Equal(w.first) || h.Rewrites != w.rew ||
			h.Deletions != w.del || len(h.Periods) != w.per || h.Conform() != w.conform {
			t.Errorf("%s = %+v, attendu %+v", h.Name, h, w)
		}
		for j := 1; j < len(h.Periods); j++ {
			if h.Periods[j].FirstSeenAt.Before(h.Periods[j-1].FirstSeenAt) {
				t.Errorf("%s : périodes non triées", h.Name)
			}
		}
	}
	wantCounts := map[string]int{TagHistoryStable: 1, TagHistoryRestored: 1, TagHistoryRewritten: 2, TagHistoryDeleted: 2}
	if !reflect.DeepEqual(rep.Counts, wantCounts) {
		t.Errorf("Counts = %v, attendu %v", rep.Counts, wantCounts)
	}
	if h, ok := FindTagHistory(rep, "v1.1.0"); !ok || h.Name != "v1.1.0" {
		t.Errorf("FindTagHistory = %+v, %v", h, ok)
	}
	if _, ok := FindTagHistory(rep, "v9.0.0"); ok {
		t.Error("tag jamais observé trouvé")
	}

	none := BuildTagHistory(nil)
	if none.Observed || none.Tags == nil || len(none.Tags) != 0 || !none.Since.IsZero() || len(none.Counts) != 4 {
		t.Errorf("sans observation = %+v", none)
	}
}

func TestMilestoneSV3(t *testing.T) {
	hist := BuildTagHistory([]model.TagObservation{
		obs("v1.0.0", 1, at(2, 0), at(9, 0), nil, ""),
		obs("V2", 2, at(2, 0), at(3, 0), tptr(at(4, 0)), model.TagEndRewritten),
		obs("V2", 3, at(4, 0), at(9, 0), nil, ""),
		obs("v3.0.0", 4, at(2, 0), at(3, 0), tptr(at(4, 0)), model.TagEndDeleted),
		obs("v3.0.0", 4, at(5, 0), at(9, 0), nil, ""),
	})
	cases := []struct {
		ms     MilestoneResult
		absent string
		want   Criterion
	}{
		{MilestoneResult{Status: MilestoneAbsent}, "", Criterion{ID: CriterionSV3, Status: CriterionUnknown, Reason: UnknownNoTag}},
		{MilestoneResult{Status: MilestoneAbsent}, UnknownNotDue, Criterion{ID: CriterionSV3, Status: CriterionUnknown, Reason: UnknownNotDue}},
		{MilestoneResult{Status: MilestonePresent, Tag: "v1.0.0"}, "", Criterion{ID: CriterionSV3, Status: CriterionConform}},
		{MilestoneResult{Status: MilestoneNonconformingName, Tag: "V2"}, "", Criterion{ID: CriterionSV3, Status: CriterionNonconform}},
		{MilestoneResult{Status: MilestonePresent, Tag: "v3.0.0"}, "", Criterion{ID: CriterionSV3, Status: CriterionConform}},
		{MilestoneResult{Status: MilestonePresent, Tag: "v4.0.0"}, "", Criterion{ID: CriterionSV3, Status: CriterionUnknown, Reason: UnknownNotObserved}},
	}
	for i, c := range cases {
		if got := MilestoneSV3(c.ms, hist, c.absent); got != c.want {
			t.Errorf("cas %d : %+v, attendu %+v", i, got, c.want)
		}
	}
}

func TestCheckDeadline(t *testing.T) {
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("fuseau Europe/Paris indisponible")
	}
	ms := mustMilestones(t, "v1.0.0@2026-11-10", "v2.0.0")
	present := MilestoneResult{Milestone: ms[0], Status: MilestonePresent, Tag: "v1.0.0"}
	// Fin de l'échéance à Paris : 2026-11-11 00:00 CET = 2026-11-10 23:00 UTC.
	end := time.Date(2026, 11, 10, 23, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		r         MilestoneResult
		firstSeen time.Time
		commitAt  time.Time
		reads     []time.Time
		want      string
	}{
		{"sans échéance", MilestoneResult{Milestone: ms[1], Status: MilestonePresent, Tag: "v2.0.0"}, at(1, 0), at(1, 0), nil, ""},
		{"absent", MilestoneResult{Milestone: ms[0], Status: MilestoneAbsent}, at(1, 0), at(1, 0), nil, ""},
		{"nom non conforme", MilestoneResult{Milestone: ms[0], Status: MilestoneNonconformingName, Tag: "v1.0"}, at(1, 0), at(1, 0), nil, ""},
		{"jamais observé", present, time.Time{}, at(1, 0), []time.Time{at(5, 0)}, DeadlineUndetermined},
		{"à l'heure", present, at(9, 0), at(8, 0), nil, DeadlineOnTime},
		{"à l'heure, dernière seconde", present, end.Add(-time.Second), at(8, 0), nil, DeadlineOnTime},
		{"borne exclue", present, end, at(8, 0), nil, DeadlineUndetermined},
		{"en retard", present, at(12, 0), at(8, 0), []time.Time{at(7, 0), at(9, 0), at(12, 0)}, DeadlineLate},
		{"lecture à la date du commit", present, at(12, 0), at(8, 0), []time.Time{at(8, 0)}, DeadlineLate},
		{"lecture avant le commit", present, at(12, 0), at(8, 0), []time.Time{at(7, 0)}, DeadlineUndetermined},
		{"lecture à la fin de l'échéance", present, at(12, 0), at(8, 0), []time.Time{end}, DeadlineUndetermined},
		{"date du commit inconnue", present, at(12, 0), time.Time{}, []time.Time{at(9, 0)}, DeadlineUndetermined},
		{"commit après l'échéance", present, at(12, 0), at(11, 0), []time.Time{at(9, 0)}, DeadlineUndetermined},
		{"aucune lecture", present, at(12, 0), at(8, 0), nil, DeadlineUndetermined},
	}
	for _, c := range cases {
		d := CheckDeadline(c.r, paris, c.firstSeen, c.commitAt, c.reads)
		if d.Status != c.want {
			t.Errorf("%s : %q, attendu %q", c.name, d.Status, c.want)
		}
		if c.want != "" && (!d.DueEnd.Equal(end) || !d.FirstSeenAt.Equal(c.firstSeen) || !d.CommitAt.Equal(c.commitAt)) {
			t.Errorf("%s : bornes %+v", c.name, d)
		}
		if c.want == "" && d != (MilestoneDeadline{}) {
			t.Errorf("%s : %+v, attendu vide", c.name, d)
		}
	}
	// Fuseau nil : UTC.
	if d := CheckDeadline(present, nil, time.Date(2026, 11, 10, 23, 30, 0, 0, time.UTC), at(8, 0), nil); d.Status != DeadlineOnTime {
		t.Errorf("UTC : %+v", d)
	}
}

func TestMilestoneSV2(t *testing.T) {
	ms := mustMilestones(t, "v1.0.0@2026-11-10", "v2.0.0")
	before, after := at(10, 12), at(11, 1)
	cases := []struct {
		name string
		r    MilestoneResult
		d    MilestoneDeadline
		now  time.Time
		want Criterion
	}{
		{"présent sans échéance lue", MilestoneResult{Milestone: ms[0], Status: MilestonePresent}, MilestoneDeadline{}, after,
			Criterion{ID: CriterionSV2, Status: CriterionConform}},
		{"à l'heure", MilestoneResult{Milestone: ms[0], Status: MilestonePresent}, MilestoneDeadline{Status: DeadlineOnTime}, after,
			Criterion{ID: CriterionSV2, Status: CriterionConform}},
		{"en retard", MilestoneResult{Milestone: ms[0], Status: MilestonePresent}, MilestoneDeadline{Status: DeadlineLate}, after,
			Criterion{ID: CriterionSV2, Status: CriterionNonconform}},
		{"indéterminé", MilestoneResult{Milestone: ms[0], Status: MilestonePresent}, MilestoneDeadline{Status: DeadlineUndetermined}, after,
			Criterion{ID: CriterionSV2, Status: CriterionUnknown, Reason: UnknownDeadline}},
		{"absent avant l'échéance", MilestoneResult{Milestone: ms[0], Status: MilestoneAbsent}, MilestoneDeadline{}, before,
			Criterion{ID: CriterionSV2, Status: CriterionUnknown, Reason: UnknownNotDue}},
		{"absent après l'échéance", MilestoneResult{Milestone: ms[0], Status: MilestoneAbsent}, MilestoneDeadline{}, after,
			Criterion{ID: CriterionSV2, Status: CriterionNonconform}},
		{"absent sans échéance", MilestoneResult{Milestone: ms[1], Status: MilestoneAbsent}, MilestoneDeadline{}, before,
			Criterion{ID: CriterionSV2, Status: CriterionNonconform}},
		{"nom non conforme", MilestoneResult{Milestone: ms[0], Status: MilestoneNonconformingName}, MilestoneDeadline{}, before,
			Criterion{ID: CriterionSV2, Status: CriterionNonconform}},
	}
	for _, c := range cases {
		if got := MilestoneSV2(c.r, c.d, time.UTC, c.now); got != c.want {
			t.Errorf("%s : %+v, attendu %+v", c.name, got, c.want)
		}
	}
}

func TestRewriteObserved(t *testing.T) {
	base := HeadChange{PrevBranch: "main", PrevSHA: hsha(1), PrevTracked: true, Branch: "main", SHA: hsha(2), Tracked: true}
	cases := []struct {
		name       string
		mut        func(h *HeadChange)
		a          Ancestry
		comparable bool
		want       bool
	}{
		{"réécrite", func(*HeadChange) {}, AncestryNo, true, true},
		{"avance rapide", func(*HeadChange) {}, AncestryYes, true, false},
		{"ancien objet absent", func(*HeadChange) {}, AncestryUnknown, true, false},
		{"tête inchangée", func(h *HeadChange) { h.SHA = h.PrevSHA }, AncestryNo, false, false},
		{"branche par défaut changée", func(h *HeadChange) { h.PrevBranch = "master" }, AncestryNo, false, false},
		{"non suivie avant", func(h *HeadChange) { h.PrevTracked = false }, AncestryNo, false, false},
		{"non suivie après", func(h *HeadChange) { h.Tracked = false }, AncestryNo, false, false},
		{"première synchro", func(h *HeadChange) { h.PrevSHA, h.PrevBranch = "", "" }, AncestryNo, false, false},
		{"SHA invalide", func(h *HeadChange) { h.SHA = "abc" }, AncestryNo, false, false},
		{"sans branche", func(h *HeadChange) { h.Branch, h.PrevBranch = "", "" }, AncestryNo, false, false},
	}
	for _, c := range cases {
		h := base
		c.mut(&h)
		if got := h.Comparable(); got != c.comparable {
			t.Errorf("%s : Comparable = %v", c.name, got)
		}
		if got := RewriteObserved(h, c.a); got != c.want {
			t.Errorf("%s : RewriteObserved = %v", c.name, got)
		}
	}
}

func TestBuildRewriteReport(t *testing.T) {
	rows := []model.BranchRewrite{
		{RepoID: 1, Branch: "main", OldSHA: hsha(1), NewSHA: hsha(2), ObservedAt: at(3, 0)},
		{RepoID: 1, Branch: "main", OldSHA: hsha(3), NewSHA: hsha(4), ObservedAt: at(5, 0)},
		{RepoID: 1, Branch: "master", OldSHA: hsha(5), NewSHA: hsha(6), ObservedAt: at(1, 0)},
	}
	rep := BuildRewriteReport(rows)
	if rep.Count != 3 || !rep.Last.Equal(at(5, 0)) {
		t.Fatalf("Count = %d, Last = %v", rep.Count, rep.Last)
	}
	wantOld := []string{hsha(3), hsha(1), hsha(5)}
	for i, e := range rep.Events {
		if e.OldSHA != wantOld[i] {
			t.Errorf("événement %d : %+v", i, e)
		}
	}
	if rep.Events[0].NewSHA != hsha(4) || rep.Events[2].Branch != "master" {
		t.Errorf("événements = %+v", rep.Events)
	}
	none := BuildRewriteReport(nil)
	if none.Count != 0 || none.Events == nil || !none.Last.IsZero() {
		t.Errorf("sans réécriture = %+v", none)
	}
}
