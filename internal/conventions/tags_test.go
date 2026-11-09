package conventions

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/semver"
)

func TestCheckTag(t *testing.T) {
	cases := []struct {
		name       string
		codes      []string
		conform    bool
		final      bool
		designates string // version désignée (String) ; vide si aucune
	}{
		{"v1.0.0", nil, true, true, "v1.0.0"},
		{"v0.1.0", nil, true, true, "v0.1.0"},
		{"v10.20.30", nil, true, true, "v10.20.30"},
		{"v2.0.0-rc.1", nil, true, false, "v2.0.0-rc.1"},
		{"v1.0.0-alpha", nil, true, false, "v1.0.0-alpha"},
		{"v1.0.0-0.3.7", nil, true, false, "v1.0.0-0.3.7"},
		{"v1.0.0-x-y.z", nil, true, false, "v1.0.0-x-y.z"},
		{"v1.0.0-rc1", nil, true, false, "v1.0.0-rc1"},
		{"v1.0.0+build.5", []string{TagBuildMetadata}, true, false, "v1.0.0+build.5"},
		{"v1.0.0-rc.1+sha.abc", []string{TagBuildMetadata}, true, false, "v1.0.0-rc.1+sha.abc"},
		{"1.0.0", []string{TagNoV}, false, false, "v1.0.0"},
		{"V1.0.0", []string{TagUpperV}, false, false, "v1.0.0"},
		{"v1.0", []string{TagMissingComponent}, false, false, "v1.0.0"},
		{"v1", []string{TagMissingComponent}, false, false, "v1.0.0"},
		{"1.0", []string{TagNoV, TagMissingComponent}, false, false, "v1.0.0"},
		{"V2", []string{TagUpperV, TagMissingComponent}, false, false, "v2.0.0"},
		{"v01.0.0", []string{TagLeadingZero}, false, false, "v1.0.0"},
		{"v1.00.0", []string{TagLeadingZero}, false, false, "v1.0.0"},
		{"v1.0.0-rc.01", []string{TagLeadingZero}, false, false, "v1.0.0-rc.01"},
		{"V01.0", []string{TagUpperV, TagMissingComponent, TagLeadingZero}, false, false, "v1.0.0"},
		{"v1.0.0-", []string{TagEmptyPrerelease}, false, false, ""},
		{"v1.0.0-rc..1", []string{TagEmptyPrerelease}, false, false, ""},
		{"1.0.0-", []string{TagNoV, TagEmptyPrerelease}, false, false, ""},
		{"v1.0.0+", []string{TagInvalid}, false, false, ""},
		{"v1.0.0+a..b", []string{TagInvalid}, false, false, ""},
		{"v1.0.0+a_b", []string{TagInvalid}, false, false, ""},
		{"v1.0.0-rc_1", []string{TagInvalid}, false, false, ""},
		{"v1.0.0.1", []string{TagInvalid}, false, false, ""},
		{"v1.a.0", []string{TagInvalid}, false, false, ""},
		{"v1..0", []string{TagInvalid}, false, false, ""},
		{"v", []string{TagInvalid}, false, false, ""},
		{"", []string{TagInvalid}, false, false, ""},
		{"release-1", []string{TagInvalid}, false, false, ""},
		{"vfinal", []string{TagInvalid}, false, false, ""},
		{"Version1.0.0", []string{TagInvalid}, false, false, ""},
		{"v 1.0.0", []string{TagInvalid}, false, false, ""},
		{"v1.0.0 ", []string{TagInvalid}, false, false, ""},
		{"v99999999999999999999.0.0", []string{TagInvalid}, false, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckTag(tc.name)
			want := tc.codes
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(got.Codes, want) {
				t.Errorf("codes = %v, want %v", got.Codes, want)
			}
			if got.Conform() != tc.conform || got.Final() != tc.final {
				t.Errorf("conform = %v final = %v, want %v %v", got.Conform(), got.Final(), tc.conform, tc.final)
			}
			switch {
			case tc.designates == "" && got.Designates:
				t.Errorf("designates %v", got.Version)
			case tc.designates != "" && (!got.Designates || got.Version.String() != tc.designates):
				t.Errorf("version = %v (%v), want %s", got.Version, got.Designates, tc.designates)
			}
			// Un nom conforme est accepté tel quel par le comparateur commun.
			if tc.conform {
				if _, ok := semver.ParseVersion(tc.name); !ok {
					t.Error("semver.ParseVersion refuses a conform tag")
				}
			}
		})
	}
}

func TestTagSeverity(t *testing.T) {
	for _, code := range AllTagCodes {
		want := SeverityBlocking
		if code == TagBuildMetadata {
			want = SeverityWarning
		}
		if TagSeverityOf(code) != want {
			t.Errorf("%s", code)
		}
	}
	if TagSeverityOf("x") != 0 {
		t.Error("unknown code")
	}
}

func TestParseMilestone(t *testing.T) {
	m, err := ParseMilestone("v1.0.0")
	if err != nil || m.Name != "v1.0.0" || m.Version.Major != 1 || m.HasDue() {
		t.Fatalf("v1.0.0 = %+v, %v", m, err)
	}
	if _, ok := m.DueEnd(time.UTC); ok {
		t.Error("no due date expected")
	}
	m, err = ParseMilestone(" v2.1.0@2026-12-01 ")
	if err != nil || m.Name != "v2.1.0" || m.Due != "2026-12-01" || !m.HasDue() {
		t.Fatalf("with due = %+v, %v", m, err)
	}
	paris, _ := time.LoadLocation("Europe/Paris")
	end, ok := m.DueEnd(paris)
	if !ok || !end.Equal(time.Date(2026, 12, 2, 0, 0, 0, 0, paris)) {
		t.Errorf("DueEnd = %v %v", end, ok)
	}
	if end, _ := m.DueEnd(nil); !end.Equal(time.Date(2026, 12, 2, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("DueEnd(nil) = %v", end)
	}
	for _, s := range []string{"1.0.0", "V1.0.0", "v1.0", "v01.0.0", "v1.0.0-rc.1", "v1.0.0+b", "", "@2026-01-01", "v1.0.0-"} {
		if _, err := ParseMilestone(s); !errors.Is(err, ErrMilestoneVersion) {
			t.Errorf("%q: err = %v", s, err)
		}
	}
	for _, s := range []string{"v1.0.0@", "v1.0.0@2026-13-01", "v1.0.0@2026-02-30", "v1.0.0@01/12/2026", "v1.0.0@2026-1-1"} {
		if _, err := ParseMilestone(s); !errors.Is(err, ErrMilestoneDue) {
			t.Errorf("%q: err = %v", s, err)
		}
	}
	ms, err := ParseMilestones(DefaultMilestones())
	if err != nil || len(ms) != 3 || ms[2].Name != "v3.0.0" {
		t.Errorf("defaults = %+v, %v", ms, err)
	}
	if _, err := ParseMilestones([]string{"v1.0.0", "bad"}); err == nil {
		t.Error("invalid list accepted")
	}
	if got, ok := FindMilestone(ms, "v2.0.0"); !ok || got.Version.Major != 2 {
		t.Errorf("FindMilestone = %+v %v", got, ok)
	}
	if _, ok := FindMilestone(ms, "v4.0.0"); ok {
		t.Error("v4.0.0 found")
	}
}

func mustMilestones(t *testing.T, entries ...string) []Milestone {
	t.Helper()
	ms, err := ParseMilestones(entries)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func TestCheckMilestones(t *testing.T) {
	ms := mustMilestones(t, "v1.0.0", "v2.0.0@2027-01-15", "v3.0.0", "v4.0.0", "v5.0.0")
	tags := []string{
		"v1.0.0", "1.0.0", // présent, malgré un doublon non conforme
		"v2.0.0-rc.1", "V2.0.0", "v2.0", // pré-version ignorée, deux noms non conformes
		"v3.0.0-rc.1", "v3.0.0-rc.2", // pré-versions seules : absent
		"v4.0.0+build", // build : nom non conforme au jalon
		"v0.9.0", "release-5",
	}
	got := CheckMilestones(ms, tags)
	want := []struct {
		status, tag string
		others      []string
	}{
		{MilestonePresent, "v1.0.0", []string{"1.0.0"}},
		{MilestoneNonconformingName, "V2.0.0", []string{"v2.0"}},
		{MilestoneAbsent, "", []string{}},
		{MilestoneNonconformingName, "v4.0.0+build", []string{}},
		{MilestoneAbsent, "", []string{}},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d", len(got))
	}
	for i, w := range want {
		if got[i].Status != w.status || got[i].Tag != w.tag || !reflect.DeepEqual(got[i].Others, w.others) {
			t.Errorf("%s: %+v, want %+v", got[i].Name, got[i], w)
		}
	}
	if got[1].Due != "2027-01-15" {
		t.Errorf("due lost: %+v", got[1])
	}
	if empty := CheckMilestones(ms, nil); empty[0].Status != MilestoneAbsent {
		t.Errorf("no tags = %+v", empty[0])
	}
}

func TestCheckReleases(t *testing.T) {
	ms := CheckMilestones(mustMilestones(t, "v1.0.0", "v2.0.0", "v3.0.0"), []string{"v1.0.0", "V2.0.0", "v2.1.0-rc.1", "v0.5.0"})
	releases := []model.Release{
		{TagName: "v0.5.0", Prerelease: true},       // incohérent : version finale marquée pré-release
		{TagName: "v2.1.0-rc.1", Prerelease: false}, // incohérent : pré-version non marquée
		{TagName: "v1.0.0", Draft: true},            // brouillon : ignoré, donc v1.0.0 sans release
		{TagName: "v9.9.9"},                         // tag absent
		{TagName: "nightly", Prerelease: true},      // tag illisible et absent : pas de jugement de pré-version
		{TagName: "V2.0.0"},                         // release du jalon v2.0.0 posé sous un nom non conforme
	}
	got := CheckReleases(ms, []string{"v1.0.0", "V2.0.0", "v2.1.0-rc.1", "v0.5.0"}, releases)
	want := []ReleaseNote{
		{ReleaseMilestoneMissing, "v1.0.0"},
		{ReleaseTagMissing, "nightly"},
		{ReleaseTagMissing, "v9.9.9"},
		{ReleasePrerelease, "v0.5.0"},
		{ReleasePrerelease, "v2.1.0-rc.1"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("notes = %+v\nwant %+v", got, want)
	}
	if got := CheckReleases(nil, nil, nil); got == nil || len(got) != 0 {
		t.Errorf("empty = %#v", got)
	}
	// Plusieurs releases sur un même tag : une seule note.
	dup := CheckReleases(nil, nil, []model.Release{{TagName: "v1.0.0", Prerelease: true}, {TagName: "v1.0.0", Prerelease: true}})
	if len(dup) != 2 || dup[0].Code != ReleaseTagMissing || dup[1].Code != ReleasePrerelease {
		t.Errorf("dup = %+v", dup)
	}
}

func tagsOf(names ...string) []model.Tag {
	out := make([]model.Tag, 0, len(names))
	for _, n := range names {
		out = append(out, model.Tag{RepoID: 1, Name: n, SHA: "sha-" + n})
	}
	return out
}

func tagNames(ts []model.Tag) []string {
	out := []string{}
	for _, t := range ts {
		out = append(out, t.Name)
	}
	return out
}

func TestFinalTagsAndIntervals(t *testing.T) {
	tags := tagsOf("v1.10.0", "v1.2.0", "v2.0.0-rc.1", "1.5.0", "v1.0.0+b", "v0.9.0", "V3.0.0", "v2.0.0", "junk")
	if got := tagNames(FinalTags(tags)); !reflect.DeepEqual(got, []string{"v0.9.0", "v1.2.0", "v1.10.0", "v2.0.0"}) {
		t.Errorf("FinalTags = %v", got)
	}
	ivs := TagIntervals(tags)
	var pairs [][2]string
	for _, iv := range ivs {
		pairs = append(pairs, [2]string{iv.From.Name, iv.To.Name})
	}
	wantPairs := [][2]string{{"", "v0.9.0"}, {"v0.9.0", "v1.2.0"}, {"v1.2.0", "v1.10.0"}, {"v1.10.0", "v2.0.0"}}
	if !reflect.DeepEqual(pairs, wantPairs) {
		t.Errorf("intervals = %v", pairs)
	}
	if ivs[1].From.SHA != "sha-v0.9.0" || ivs[1].To.SHA != "sha-v1.2.0" {
		t.Errorf("SHAs lost: %+v", ivs[1])
	}
	if iv, ok := IntervalTo(tags, "v2.0.0"); !ok || iv.From.Name != "v1.10.0" {
		t.Errorf("IntervalTo = %+v %v", iv, ok)
	}
	for _, name := range []string{"v2.0.0-rc.1", "V3.0.0", "v9.0.0"} {
		if _, ok := IntervalTo(tags, name); ok {
			t.Errorf("IntervalTo(%s) found", name)
		}
	}
	v3, _ := semver.ParseVersion("v3.0.0")
	if prev, ok := PreviousFinal(tags, v3); !ok || prev.Name != "v2.0.0" {
		t.Errorf("PreviousFinal(v3) = %+v %v", prev, ok)
	}
	v1, _ := semver.ParseVersion("v1.2.0")
	if prev, ok := PreviousFinal(tags, v1); !ok || prev.Name != "v0.9.0" {
		t.Errorf("PreviousFinal(v1.2.0) = %+v %v", prev, ok)
	}
	v0, _ := semver.ParseVersion("v0.9.0")
	if _, ok := PreviousFinal(tags, v0); ok {
		t.Error("PreviousFinal(v0.9.0) found")
	}
	if last, ok := LastFinal(tags); !ok || last.Name != "v2.0.0" {
		t.Errorf("LastFinal = %+v %v", last, ok)
	}
	if _, ok := LastFinal(tagsOf("1.0.0", "v1.0.0-rc.1")); ok {
		t.Error("LastFinal without final tag")
	}
	if got := TagIntervals(nil); got == nil || len(got) != 0 {
		t.Errorf("TagIntervals(nil) = %#v", got)
	}
}
