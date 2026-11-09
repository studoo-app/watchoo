package conventions

import (
	"reflect"
	"testing"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestCheckFreshness(t *testing.T) {
	one, two := 1, 2
	row := func(sha int, msg string) model.ConventionCommit {
		return model.ConventionCommit{SHA: hsha(sha), Message: msg, ParentCount: &one, AuthorName: "Ada"}
	}
	byKey := map[string]model.ConventionCommit{}
	for _, r := range []model.ConventionCommit{
		row(1, "feat: ajoute x"),
		row(2, "feat(api)!: retire y"),
		row(3, "fix: corrige z"),
		row(4, "docs: met à jour"),
		row(5, "Fix typo"),      // non conforme
		row(6, "feat ajoute w"), // non conforme (séparateur)
		{SHA: hsha(7), Message: "Merge branch 'fix/a'", ParentCount: &two, AuthorName: "Ada"},
		{SHA: hsha(8), Message: "fix(deps): bump x", ParentCount: &one, AuthorLogin: "dependabot[bot]"},
		{SHA: hsha(9), Message: "Merge pull request #3 from a/b", AuthorName: "Ada"}, // parents inconnus : message
	} {
		byKey[r.SHA] = r
	}
	shas := []string{hsha(1), hsha(2), hsha(3), hsha(4), hsha(5), hsha(6), hsha(7), hsha(8), hsha(9), hsha(42), hsha(3)}
	got := CheckFreshness(hsha(100), shas, byKey, DefaultRules(), false)
	want := ChangelogFreshness{Known: true, Since: hsha(100), Feat: 2, Fix: 1, Missing: 1}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CL10 = %+v\nattendu %+v", got, want)
	}
	if c := got.Counts(); !reflect.DeepEqual(c, map[string]int{CountFeat: 2, CountFix: 1, CountMissing: 1}) {
		t.Errorf("Counts = %v", c)
	}

	grayed := CheckFreshness(hsha(100), nil, byKey, DefaultRules(), true)
	if !grayed.Known || !grayed.Grayed || grayed.Feat+grayed.Fix+grayed.Missing != 0 {
		t.Errorf("CL10 à jour = %+v", grayed)
	}

	for _, reason := range []string{UnknownNoClone, UnknownNoFile, UnknownCloneUnreadable} {
		u := UnknownFreshness(reason)
		if u.Known || u.Reason != reason || u.Counts() == nil || len(u.Counts()) != 0 {
			t.Errorf("CL10 inconnu = %+v", u)
		}
	}
}
