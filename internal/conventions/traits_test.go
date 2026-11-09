package conventions

import (
	"reflect"
	"testing"
)

func TestWording(t *testing.T) {
	cases := []struct {
		desc string
		want []string
	}{
		{"add login page", nil},
		{"added login page", []string{WordingPastTense}},
		{"Fixed crash on start", []string{WordingPastTense}},
		{"updated, then removed", []string{WordingPastTense}},
		{"ajouté la page de connexion", []string{WordingPastTense}},
		{"ajoutées les routes", []string{WordingPastTense}},
		{"corrigé le bug", []string{WordingPastTense}},
		{"supprimés les fichiers", []string{WordingPastTense}},
		{"créée la base", []string{WordingPastTense}},
		{"mis à jour le README", []string{WordingPastTense}},
		{"mise à jour du README", nil}, // nom, pas participe
		{"ajout de la page", nil},
		{"corrige le bug", nil},
		{"café en terrasse", nil}, // aucune règle sur la seule terminaison « -é »
		{"adding login page", []string{WordingProgressive}},
		{"Fixing: crash", []string{WordingProgressive}},
		{"string escaping", nil},
		{"during build", nil},
		{"logging of errors", nil},
		{"something odd", nil},
		{"ping endpoint", nil},
		{"ing", nil},
		{"I added tests", []string{WordingFirstPerson}},
		{"je corrige le bug", []string{WordingFirstPerson}},
		{"j'ai corrigé le bug", []string{WordingFirstPerson}},
		{"J’ajoute la page", []string{WordingFirstPerson}},
		{"we fix it", []string{WordingFirstPerson}},
		{"nous ajoutons", []string{WordingFirstPerson}},
		{"my fix", []string{WordingFirstPerson}},
		{"mon ajout", []string{WordingFirstPerson}},
		{"ma page", []string{WordingFirstPerson}},
		{"mes tests", []string{WordingFirstPerson}},
		{"image upload", nil},
		{"", nil},
		{"   ", nil},
	}
	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			want := tc.want
			if want == nil {
				want = []string{}
			}
			if got := Wording(tc.desc); !reflect.DeepEqual(got, want) {
				t.Errorf("Wording = %v, want %v", got, want)
			}
		})
	}
}

func TestProgressiveExceptionsAreIng(t *testing.T) {
	for _, w := range ProgressiveExceptions {
		if got := Wording(w + " x"); len(got) != 0 {
			t.Errorf("%s: %v", w, got)
		}
	}
}

func TestDetectLanguage(t *testing.T) {
	cases := map[string]string{
		"add the login page":                 LanguageEN,
		"fix crash at startup":               LanguageEN,
		"on ajoute le support":               LanguageFR, // « on » n'est pas un mot-outil anglais
		"le but du module":                   LanguageFR,
		"ajoute la page de connexion":        LanguageFR,
		"corrige l'affichage":                LanguageFR,
		"amélioration":                       LanguageFR, // lettre accentuée
		"add login page":                     LanguageUnknown,
		"ajoute page":                        LanguageUnknown,
		"":                                   LanguageUnknown,
		"add the page pour le formulaire":    LanguageFR,
		"update of the API for la page":      LanguageEN,
		"mix: the page, la page":             LanguageUnknown, // à égalité
		"README.md: update of the doc (#12)": LanguageEN,
	}
	for s, want := range cases {
		if got := DetectLanguage(s); got != want {
			t.Errorf("DetectLanguage(%q) = %s, want %s", s, got, want)
		}
	}
	// Words common to both languages count for neither.
	for _, w := range []string{"a", "me", "on", "an", "as", "but", "plus", "car", "son"} {
		if functionWordsFR[w] || functionWordsEN[w] {
			t.Errorf("ambiguous word %q is a function word", w)
		}
	}
}

func TestInspect(t *testing.T) {
	cases := []struct {
		name, msg string
		want      Traits
	}{
		{"plain", "feat: add login page", Traits{Wording: []string{}, Language: LanguageUnknown}},
		{"header stripped", "feat(auth): added the login page", Traits{Wording: []string{WordingPastTense}, Language: LanguageEN}},
		{"no header", "Ajouté la page", Traits{Wording: []string{WordingPastTense}, Language: LanguageFR}},
		{"squash suffix", "fix: crash at start (#12)", Traits{Wording: []string{}, Language: LanguageEN}},
		{"footer", "feat: add x\n\nbody\n\nRefs: #12\nReviewed-by: bob", Traits{Wording: []string{}, Footer: true, Issue: true, Language: LanguageUnknown}},
		{"footer right after subject", "feat: add x\n\nCo-authored-by: bob <b@x>", Traits{Wording: []string{}, Footer: true, Language: LanguageUnknown}},
		{"breaking footer", "feat!: drop\n\nBREAKING CHANGE: no more php", Traits{Wording: []string{}, Footer: true, Language: LanguageUnknown}},
		{"no footer block", "feat: add x\n\nsome body text", Traits{Wording: []string{}, Language: LanguageUnknown}},
		{"body glued", "feat: add x\nRefs: #1", Traits{Wording: []string{}, Issue: true, Language: LanguageUnknown}},
		{"issue in subject", "fix: crash, closes #4", Traits{Wording: []string{}, Issue: true, Language: LanguageUnknown}},
		{"issue keyword case", "fix: crash\n\nFIXES #4", Traits{Wording: []string{}, Footer: true, Issue: true, Language: LanguageUnknown}},
		{"issue resolves", "fix: x\n\nResolves #10", Traits{Wording: []string{}, Footer: true, Issue: true, Language: LanguageUnknown}},
		{"no issue number", "fix: x\n\nCloses: JIRA-1", Traits{Wording: []string{}, Footer: true, Language: LanguageUnknown}},
		{"not a keyword", "fix: x\n\nPrefixes #10", Traits{Wording: []string{}, Footer: true, Language: LanguageUnknown}},
		{"squash ref is not an issue", "fix: x (#12)", Traits{Wording: []string{}, Language: LanguageUnknown}},
		{"empty", "", Traits{Wording: []string{}, Language: LanguageUnknown}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Inspect(tc.msg); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Inspect = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestTraitsTally(t *testing.T) {
	var zero TraitsTally
	zero.Add(Traits{Wording: []string{WordingProgressive}, Language: "xx"})
	if zero.Evaluated != 1 || zero.Wording[WordingProgressive] != 1 || zero.Languages[LanguageUnknown] != 1 {
		t.Errorf("zero value Add = %+v", zero)
	}
	a := NewTraitsTally()
	for _, tr := range []Traits{
		{Wording: []string{WordingPastTense}, Footer: true, Issue: true, Language: LanguageFR},
		{Language: LanguageFR},
		{Language: LanguageEN},
		{Language: LanguageUnknown},
		{Wording: []string{WordingPastTense, WordingFirstPerson}, Footer: true, Language: LanguageFR},
	} {
		a.Add(tr)
	}
	if a.Evaluated != 5 || a.Footer != 2 || a.Issue != 1 || a.Wording[WordingPastTense] != 2 ||
		a.Wording[WordingFirstPerson] != 1 || a.Wording[WordingProgressive] != 0 {
		t.Errorf("tally = %+v", a)
	}
	sum := a.Merge(zero)
	if sum.Evaluated != 6 || sum.Wording[WordingProgressive] != 1 || sum.Languages[LanguageFR] != 3 {
		t.Errorf("merge = %+v", sum)
	}
	if a.Evaluated != 5 {
		t.Error("Merge changed its receiver")
	}

	// FR 3, EN 1 : l'anglais fait 25 % des sujets reconnus, au-delà de 20 % : mélange.
	s := a.LanguageSummary(LanguageFR)
	if s.FR != 3 || s.EN != 1 || s.Unknown != 1 || s.Other != 1 || !s.Mixed || s.Expected != LanguageFR {
		t.Errorf("summary fr = %+v", s)
	}
	if s := a.LanguageSummary(LanguageEN); s.Other != 3 {
		t.Errorf("summary en = %+v", s)
	}
	if s := a.LanguageSummary("any"); s.Other != 0 || !s.Mixed {
		t.Errorf("summary any = %+v", s)
	}
	// FR 4, EN 1 : 20 % pile ne dépasse pas le seuil.
	a.Add(Traits{Language: LanguageFR})
	if s := a.LanguageSummary("any"); s.Mixed {
		t.Errorf("20%% counted as mixed: %+v", s)
	}
	if s := NewTraitsTally().LanguageSummary(LanguageFR); s.Mixed || s.Other != 0 {
		t.Errorf("empty summary = %+v", s)
	}
}

func TestAggregateTraits(t *testing.T) {
	rep := Aggregate([]Commit{
		{SHA: "a", AuthorKey: "alice", Message: "feat: added the page", ParentsKnown: true, ParentCount: 1},
		{SHA: "m", AuthorKey: "alice", Message: "Merge branch 'x'", ParentsKnown: true, ParentCount: 2},
	}, Options{Rules: DefaultRules()})
	if got := rep.Commits[0].Traits; !reflect.DeepEqual(got.Wording, []string{WordingPastTense}) || got.Language != LanguageEN {
		t.Errorf("evaluated traits = %+v", got)
	}
	if got := rep.Commits[1].Traits; got.Wording != nil || got.Language != "" {
		t.Errorf("excluded commit traits = %+v", got)
	}
}

func TestTallyOf(t *testing.T) {
	results := []CommitResult{
		{SHA: "a", Conform: true, Codes: []string{}},
		{SHA: "b", Codes: []string{CodeNoType}},
		{SHA: "c", Codes: []string{CodeLengthSoft, CodeNoType}},
		{SHA: "d", Excluded: ExclusionMerge},
		{SHA: "e", BeforeSince: true},
	}
	in := map[string]bool{"a": true, "c": true, "d": true, "e": true}
	got := TallyOf(results, func(r CommitResult) bool { return in[r.SHA] })
	if got.Evaluated != 2 || got.Conform != 1 || got.Excluded[ExclusionMerge] != 1 || got.ExcludedTotal() != 1 {
		t.Errorf("tally = %+v", got)
	}
	wantIssues := []Issue{
		{Code: CodeNoType, Severity: SeverityBlocking, Count: 1, SHAs: []string{"c"}},
		{Code: CodeLengthSoft, Severity: SeverityWarning, Count: 1, SHAs: []string{"c"}},
	}
	if !reflect.DeepEqual(got.Issues, wantIssues) {
		t.Errorf("issues = %+v", got.Issues)
	}
	none := TallyOf(results, func(CommitResult) bool { return false })
	if _, ok := none.Rate(); ok || len(none.Excluded) != len(AllExclusions) {
		t.Errorf("empty = %+v", none)
	}
}
