package conventions

import (
	"reflect"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/semver"
)

// vtag renvoie un tag de test sur le commit hsha(sha).
func vtag(name string, sha int) model.Tag { return model.Tag{RepoID: 1, Name: name, SHA: hsha(sha)} }

func mustVersion(t *testing.T, s string) semver.Version {
	t.Helper()
	v, ok := semver.ParseVersion(s)
	if !ok {
		t.Fatalf("version %q invalide", s)
	}
	return v
}

func TestValidPrereleaseForm(t *testing.T) {
	for s, want := range map[string]bool{
		"rc": true, "beta": true, "alpha": true, "pre": true, "a": true,
		"": false, "RC": false, "rc1": false, "rc-1": false, "rc.": false, " rc": false, "bêta": false,
		"abcdefghijklmnopqrstuvwxyzabcdef": true, "abcdefghijklmnopqrstuvwxyzabcdefg": false,
	} {
		if got := ValidPrereleaseForm(s); got != want {
			t.Errorf("ValidPrereleaseForm(%q) = %v, attendu %v", s, got, want)
		}
	}
	for _, f := range DefaultPrereleaseForms() {
		if !ValidPrereleaseForm(f) {
			t.Errorf("forme par défaut %q refusée", f)
		}
	}
}

func TestPrereleaseFormConform(t *testing.T) {
	cases := []struct {
		v     string
		forms []string
		want  bool
	}{
		{"1.0.0-rc.1", nil, true},
		{"1.0.0-beta.2", nil, true},
		{"1.0.0-alpha.10", nil, true},
		{"1.0.0-rc.0", nil, true},
		{"1.0.0-rc1", nil, false},
		{"1.0.0-RC.1", nil, false},
		{"1.0.0-pre", nil, false},
		{"1.0.0-rc", nil, false},
		{"1.0.0-rc.1.2", nil, false},
		{"1.0.0-rc.x", nil, false},
		{"1.0.0-rc.1x", nil, false},
		{"1.0.0-1.rc", nil, false},
		{"1.0.0-pre.1", []string{"pre"}, true},
		{"1.0.0-rc.1", []string{"pre"}, false},
		{"1.0.0-rc.1", []string{}, true},
		{"1.0.0", nil, false},
	}
	for _, c := range cases {
		if got := PrereleaseFormConform(mustVersion(t, c.v), c.forms); got != c.want {
			t.Errorf("%s %v : %v, attendu %v", c.v, c.forms, got, c.want)
		}
	}
	// Zéro en tête : rejeté par SemVer, refusé ici aussi.
	if PrereleaseFormConform(semver.Version{Major: 1, Pre: []string{"rc", "01"}}, nil) {
		t.Error("rc.01 accepté")
	}
}

func TestCheckPrereleases(t *testing.T) {
	d := linear(10)
	tags := []model.Tag{
		vtag("v1.0.0", 5),
		vtag("v1.0.0-rc.1", 4),
		vtag("v1.0.0-rc.2", 3),  // plus grande, posée sur un ancêtre de rc.1 : order
		vtag("v1.0.0-rc1", 6),   // forme libre, descendant de v1.0.0 : after_final
		vtag("v1.0.0-RC.1", 11), // forme libre, absent du clone : première observation après v1.0.0
		vtag("v1.0.0-beta.01", 2),
		vtag("1.0.0-rc.1", 2),
		vtag("v2.0.0-alpha.1", 9),
		vtag("v3.0.0+build", 9),
		vtag("v3.0.0-rc.1", 9), // pas de finale v3.0.0 (build seulement)
	}
	firstSeen := map[string]time.Time{"v1.0.0": at(3, 0), "v1.0.0-RC.1": at(5, 0), "v1.0.0-rc.1": at(1, 0)}
	got := CheckPrereleases(tags, nil, firstSeen, d.anc)
	want := PrereleaseReport{Known: true, Checked: 6, Conform: 4, Notes: []VersionNote{
		{Code: PrereleaseFreeForm, Tag: "v1.0.0-RC.1"},
		{Code: PrereleaseFreeForm, Tag: "v1.0.0-rc1"},
		{Code: PrereleaseAfterFinal, Tag: "v1.0.0-RC.1", Other: "v1.0.0"},
		{Code: PrereleaseAfterFinal, Tag: "v1.0.0-rc1", Other: "v1.0.0"},
		{Code: PrereleaseOrder, Tag: "v1.0.0-rc.2", Other: "v1.0.0-rc.1"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SV4 = %+v\nattendu %+v", got, want)
	}

	// Sans clone : l'ordre n'est pas jugé, after_final seulement par la première observation.
	got = CheckPrereleases(tags, nil, firstSeen, nil)
	want.Notes = []VersionNote{
		{Code: PrereleaseFreeForm, Tag: "v1.0.0-RC.1"},
		{Code: PrereleaseFreeForm, Tag: "v1.0.0-rc1"},
		{Code: PrereleaseAfterFinal, Tag: "v1.0.0-RC.1", Other: "v1.0.0"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SV4 sans clone = %+v\nattendu %+v", got, want)
	}

	// Formes configurées.
	got = CheckPrereleases([]model.Tag{vtag("v1.0.0-pre.1", 1), vtag("v1.0.0-rc.1", 2)}, []string{"pre"}, nil, d.anc)
	if got.Checked != 2 || got.Conform != 1 || len(got.Notes) != 1 || got.Notes[0].Tag != "v1.0.0-rc.1" {
		t.Errorf("forme pre = %+v", got)
	}

	empty := CheckPrereleases(nil, nil, nil, nil)
	if empty.Checked != 0 || empty.Notes == nil {
		t.Errorf("sans tag = %+v", empty)
	}
}

func TestPrereleaseAfter(t *testing.T) {
	// 1 ← 2 ← 3 et 1 ← 4 (branche sans lien avec 3).
	d := dag{hsha(1): nil, hsha(2): {hsha(1)}, hsha(3): {hsha(2)}, hsha(4): {hsha(1)}}
	seen := map[string]time.Time{"final": at(2, 0), "pre": at(3, 0)}
	early := map[string]time.Time{"final": at(3, 0), "pre": at(2, 0)}
	pre := func(sha int) model.Tag { return model.Tag{Name: "pre", SHA: hsha(sha)} }
	final := func(sha int) model.Tag { return model.Tag{Name: "final", SHA: hsha(sha)} }
	cases := []struct {
		name      string
		pre, fin  model.Tag
		firstSeen map[string]time.Time
		anc       AncestryFunc
		want      bool
	}{
		{"descendant de la finale", pre(3), final(2), early, d.anc, true},
		{"ancêtre de la finale", pre(2), final(3), seen, d.anc, false},
		{"sans lien, observée après", pre(4), final(3), seen, d.anc, true},
		{"sans lien, observée avant", pre(4), final(3), early, d.anc, false},
		{"même commit, observée après", pre(3), final(3), seen, d.anc, true},
		{"même commit, observée avant", pre(3), final(3), early, d.anc, false},
		{"absente du clone, observée après", pre(9), final(3), seen, d.anc, true},
		{"sans clone, observée après", pre(2), final(3), seen, nil, true},
		{"sans observation", pre(4), final(3), nil, d.anc, false},
		{"observations au même instant", pre(4), final(3), map[string]time.Time{"final": at(2, 0), "pre": at(2, 0)}, d.anc, false},
	}
	for _, c := range cases {
		if got := prereleaseAfter(c.pre, c.fin, c.firstSeen, c.anc); got != c.want {
			t.Errorf("%s : %v, attendu %v", c.name, got, c.want)
		}
	}
}

func TestIncrementSkipped(t *testing.T) {
	cases := []struct {
		lo, hi string
		want   bool
	}{
		{"1.0.0", "1.0.1", false},
		{"1.0.0", "1.1.0", false},
		{"1.4.2", "2.0.0", false},
		{"0.9.3", "1.0.0", false},
		{"1.0.0", "1.0.2", true},
		{"1.0.0", "1.2.0", true},
		{"1.0.0", "1.1.1", true},
		{"1.4.0", "3.0.0", true},
		{"1.4.0", "2.1.0", true},
		{"1.4.0", "2.0.1", true},
	}
	for _, c := range cases {
		if got := incrementSkipped(mustVersion(t, c.lo), mustVersion(t, c.hi)); got != c.want {
			t.Errorf("%s → %s : %v, attendu %v", c.lo, c.hi, got, c.want)
		}
	}
}

func TestCheckPrecedence(t *testing.T) {
	d := linear(10)
	tags := []model.Tag{
		vtag("v1.0.0", 5),
		vtag("v1.1.0", 3), // posée sur un ancêtre de v1.0.0
		vtag("v3.0.0", 9), // saut depuis v1.1.0
		vtag("v3.0.1", 9), // même commit : cohérent
		vtag("v3.0.2", 99),
		vtag("v4.0.0-rc.1", 1), // pré-version : hors SV7
		vtag("V5.0.0", 1),      // non conforme : hors SV7
		vtag("v6.0.0+b", 1),    // build : hors SV7
	}
	got := CheckPrecedence(tags, d.anc)
	want := PrecedenceReport{Known: true, Pairs: 4, Unknown: 1, Notes: []VersionNote{
		{Code: PrecedenceAncestry, Tag: "v1.1.0", Other: "v1.0.0"},
		{Code: PrecedenceSkip, Tag: "v3.0.0", Other: "v1.1.0"},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SV7 = %+v\nattendu %+v", got, want)
	}
	if got.Conform() {
		t.Error("SV7 incohérent jugé conforme")
	}

	got = CheckPrecedence(tags, nil)
	want = PrecedenceReport{Reason: UnknownNoClone, Pairs: 4, Notes: []VersionNote{
		{Code: PrecedenceSkip, Tag: "v3.0.0", Other: "v1.1.0"},
	}}
	if !reflect.DeepEqual(got, want) || got.Conform() {
		t.Errorf("SV7 sans clone = %+v\nattendu %+v", got, want)
	}

	ok := CheckPrecedence([]model.Tag{vtag("v1.0.0", 1), vtag("v1.0.1", 2), vtag("v1.1.0", 3)}, d.anc)
	if !ok.Conform() || ok.Pairs != 2 || len(ok.Notes) != 0 {
		t.Errorf("SV7 cohérent = %+v", ok)
	}
	partial := CheckPrecedence([]model.Tag{vtag("v1.0.0", 1), vtag("v1.0.1", 42)}, d.anc)
	if partial.Conform() || partial.Unknown != 1 {
		t.Errorf("SV7 illisible = %+v", partial)
	}
	if none := CheckPrecedence(nil, d.anc); none.Pairs != 0 || none.Notes == nil || !none.Conform() {
		t.Errorf("SV7 sans tag = %+v", none)
	}
}

func TestBumpOfAndExpected(t *testing.T) {
	for _, c := range []struct{ from, to, want string }{
		{"1.0.0", "2.0.0", BumpMajor}, {"1.0.0", "1.1.0", BumpMinor}, {"1.0.0", "1.0.1", BumpPatch},
		{"1.2.3", "2.0.0", BumpMajor}, {"0.1.0", "0.2.0", BumpMinor}, {"1.0.0", "1.0.0", BumpPatch},
	} {
		if got := BumpOf(mustVersion(t, c.from), mustVersion(t, c.to)); got != c.want {
			t.Errorf("BumpOf(%s, %s) = %s, attendu %s", c.from, c.to, got, c.want)
		}
	}
	for _, c := range []struct {
		breaking, feature bool
		base, want        string
	}{
		{true, false, "1.0.0", BumpMajor}, {true, true, "2.3.0", BumpMajor}, {true, false, "0.4.0", BumpMinor},
		{false, true, "1.0.0", BumpMinor}, {false, true, "0.4.0", BumpMinor}, {false, false, "1.0.0", BumpPatch},
		{false, false, "0.1.0", BumpPatch},
	} {
		if got := ExpectedBump(c.breaking, c.feature, mustVersion(t, c.base)); got != c.want {
			t.Errorf("ExpectedBump(%v, %v, %s) = %s, attendu %s", c.breaking, c.feature, c.base, got, c.want)
		}
	}
}

// bc renvoie un commit de test (un parent, auteur humain).
func bc(sha int, msg string) Commit {
	return Commit{SHA: hsha(sha), AuthorName: "Ada", Message: msg, ParentCount: 1, ParentsKnown: true}
}

func TestEvalBump(t *testing.T) {
	rules := DefaultRules()
	pair := func(from, to string) BumpPair {
		return BumpPair{From: vtag(from, 1), To: vtag(to, 2)}
	}
	merge := bc(90, "Merge branch 'feat/x'")
	merge.ParentCount = 2
	bot := bc(91, "feat!: bot casse tout")
	bot.AuthorLogin = "dependabot[bot]"
	cases := []struct {
		name     string
		p        BumpPair
		commits  []Commit
		missing  int
		status   string
		reason   string
		actual   string
		expected string
		eval     int
		conform  int
	}{
		{"rupture sous-versionnée", pair("v1.0.0", "v1.1.0"), []Commit{bc(3, "feat!: retire l'API v1")}, 0,
			BumpUnder, "", BumpMinor, BumpMajor, 1, 1},
		{"rupture par pied", pair("v1.0.0", "v1.0.1"), []Commit{bc(3, "fix: corrige x\n\nBREAKING CHANGE: format changé")}, 0,
			BumpUnder, "", BumpPatch, BumpMajor, 1, 1},
		{"rupture avant 1.0.0", pair("v0.3.0", "v0.4.0"), []Commit{bc(3, "feat!: retire l'API v1")}, 0,
			BumpConsistent, "", BumpMinor, BumpMinor, 1, 1},
		{"fonctionnalité en patch", pair("v1.0.0", "v1.0.1"), []Commit{bc(3, "feat: ajoute x"), bc(4, "fix: corrige y")}, 0,
			BumpUnder, "", BumpPatch, BumpMinor, 2, 2},
		{"sur-versionnement non signalé", pair("v1.0.0", "v2.0.0"), []Commit{bc(3, "fix: corrige y")}, 0,
			BumpConsistent, "", BumpMajor, BumpPatch, 1, 1},
		{"intervalle vide", pair("v1.0.0", "v1.0.1"), nil, 0, BumpConsistent, "", BumpPatch, BumpPatch, 0, 0},
		{"fusions, bots et doublons écartés", pair("v1.0.0", "v1.0.1"),
			[]Commit{merge, bot, bc(3, "fix: corrige y"), bc(3, "fix: corrige y")}, 0,
			BumpConsistent, "", BumpPatch, BumpPatch, 1, 1},
		{"commits absents", pair("v1.0.0", "v1.1.0"), []Commit{bc(3, "feat: ajoute x")}, 2,
			BumpUndetermined, UnknownPartial, BumpMinor, BumpMinor, 1, 1},
		{"75 % conformes", pair("v1.0.0", "v1.0.1"),
			[]Commit{bc(3, "fix: a"), bc(4, "fix: b"), bc(5, "fix: c"), bc(6, "n'importe quoi")}, 0,
			BumpUndetermined, UnknownLowConform, BumpPatch, BumpPatch, 4, 3},
		{"80 % conformes", pair("v1.0.0", "v1.0.1"),
			[]Commit{bc(3, "fix: a"), bc(4, "fix: b"), bc(5, "fix: c"), bc(6, "feat: d"), bc(7, "n'importe quoi")}, 0,
			BumpUnder, "", BumpPatch, BumpMinor, 5, 4},
		{"rupture non conforme ignorée", pair("v1.0.0", "v1.0.1"),
			[]Commit{bc(3, "fix: a"), bc(4, "fix: b"), bc(5, "fix: c"), bc(6, "fix: d"), bc(7, "Feat!: Casse.")}, 0,
			BumpConsistent, "", BumpPatch, BumpPatch, 5, 4},
		{"base inconnue", BumpPair{To: vtag("v1.1.0", 2), Reason: UnknownCloneUnreadable}, []Commit{bc(3, "feat: x")}, 0,
			BumpUnknown, UnknownCloneUnreadable, "", "", 0, 0},
		{"sans base", BumpPair{To: vtag("v1.1.0", 2)}, nil, 0, BumpUnknown, UnknownNoBase, "", "", 0, 0},
	}
	for _, c := range cases {
		got := EvalBump(c.p, c.commits, c.missing, rules)
		if got.Status != c.status || got.Reason != c.reason || got.Actual != c.actual || got.Expected != c.expected ||
			got.Evaluated != c.eval || got.Conform != c.conform || got.To != c.p.To.Name || got.From != c.p.From.Name {
			t.Errorf("%s : %+v", c.name, got)
		}
	}
	line := EvalBump(pair("v1.0.0", "v2.0.0"), []Commit{bc(3, "feat!: a"), bc(4, "feat: b"), bc(5, "fix: c")}, 0, rules)
	if line.Breaking != 1 || line.Features != 2 {
		t.Errorf("compteurs = %+v", line)
	}
}

func TestBumpBasesAndCheckBumps(t *testing.T) {
	d := linear(10)
	d[hsha(20)] = []string{hsha(4)} // branche partie de 4
	tags := []model.Tag{
		vtag("v0.1.0", 1),
		vtag("v0.2.0", 3),
		vtag("v1.0.0", 5),
		vtag("v1.0.1", 7),
		vtag("v1.1.0", 9),
		vtag("v2.0.0", 20),
		vtag("v3.0.0", 99), // absent du clone
		vtag("v0.0.1", 30), // hors du clone mais plus petit : premier tag, non évalué
	}
	pairs := BumpBases(tags, d.anc)
	wantPairs := []struct{ from, to, reason string }{
		{"", "v0.1.0", UnknownCloneUnreadable},
		{"v0.1.0", "v0.2.0", ""},
		{"v0.2.0", "v1.0.0", ""},
		{"v1.0.0", "v1.0.1", ""},
		{"v1.0.1", "v1.1.0", ""},
		{"v0.2.0", "v2.0.0", ""},
		{"", "v3.0.0", UnknownCloneUnreadable},
	}
	if len(pairs) != len(wantPairs) {
		t.Fatalf("paires = %+v", pairs)
	}
	for i, w := range wantPairs {
		if p := pairs[i]; p.From.Name != w.from || p.To.Name != w.to || p.Reason != w.reason {
			t.Errorf("paire %d = %+v, attendu %+v", i, p, w)
		}
	}
	if got := BumpBases(tags, nil); len(got) != 0 || got == nil {
		t.Errorf("sans clone = %+v", got)
	}
	// Aucun tag inférieur ancêtre : no_base.
	d2 := dag{hsha(1): nil, hsha(2): nil}
	if got := BumpBases([]model.Tag{vtag("v1.0.0", 1), vtag("v1.1.0", 2)}, d2.anc); len(got) != 1 || got[0].Reason != UnknownNoBase {
		t.Errorf("sans base = %+v", got)
	}

	ranges := map[string][]Commit{
		"v0.2.0": {bc(2, "feat!: retire x"), bc(3, "docs: met à jour")},
		"v1.0.0": {bc(4, "fix: corrige y")},
		"v1.0.1": {bc(6, "feat: ajoute z"), bc(7, "fix: corrige z")},
		"v2.0.0": {bc(20, "feat: ajoute w")},
	}
	read := func(p BumpPair) ([]Commit, int, bool) {
		switch p.To.Name {
		case "v1.1.0":
			return nil, 0, false
		case "v2.0.0":
			return ranges[p.To.Name], 1, true
		}
		return ranges[p.To.Name], 0, true
	}
	rep := CheckBumps(tags, d.anc, read, DefaultRules())
	want := []struct{ to, status, reason string }{
		{"v0.1.0", BumpUnknown, UnknownCloneUnreadable},
		{"v0.2.0", BumpConsistent, ""},
		{"v1.0.0", BumpConsistent, ""},
		{"v1.0.1", BumpUnder, ""},
		{"v1.1.0", BumpUnknown, UnknownCloneUnreadable},
		{"v2.0.0", BumpUndetermined, UnknownPartial},
		{"v3.0.0", BumpUnknown, UnknownCloneUnreadable},
	}
	if !rep.Known || rep.Reason != "" || len(rep.Lines) != len(want) {
		t.Fatalf("SV5 = %+v", rep)
	}
	for i, w := range want {
		if l := rep.Lines[i]; l.To != w.to || l.Status != w.status || l.Reason != w.reason {
			t.Errorf("ligne %d = %+v, attendu %+v", i, l, w)
		}
	}
	if rep.Lines[4].From != "v1.0.1" {
		t.Errorf("intervalle illisible : base perdue %+v", rep.Lines[4])
	}
	wantCounts := map[string]int{BumpConsistent: 2, BumpUnder: 1, BumpUndetermined: 1, BumpUnknown: 3}
	if !reflect.DeepEqual(rep.Counts, wantCounts) {
		t.Errorf("Counts = %v, attendu %v", rep.Counts, wantCounts)
	}

	none := CheckBumps(tags, nil, read, DefaultRules())
	if none.Known || none.Reason != UnknownNoClone || none.Lines == nil || len(none.Lines) != 0 || len(none.Counts) != 4 {
		t.Errorf("SV5 sans clone = %+v", none)
	}
}
