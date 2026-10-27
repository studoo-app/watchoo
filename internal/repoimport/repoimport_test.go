package repoimport

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/studoo-app/watchoo/internal/provider"
)

func TestParseRef(t *testing.T) {
	ok := map[string]Ref{
		"alice/projet":                                {"alice", "projet"},
		"  alice/projet  ":                            {"alice", "projet"},
		"alice/projet.git":                            {"alice", "projet"},
		"alice/projet/":                               {"alice", "projet"},
		"mon-org/projet_groupe.3":                     {"mon-org", "projet_groupe.3"},
		"https://github.com/chloe/projet-licence":     {"chloe", "projet-licence"},
		"https://github.com/chloe/projet-licence.git": {"chloe", "projet-licence"},
		"https://github.com/chloe/projet-licence/":    {"chloe", "projet-licence"},
		"http://github.com/chloe/projet":              {"chloe", "projet"},
		"https://www.github.com/chloe/projet":         {"chloe", "projet"},
		"HTTPS://GitHub.COM/Chloe/Projet":             {"Chloe", "Projet"},
	}
	for in, want := range ok {
		got, err := ParseRef(in)
		if err != nil || got != want {
			t.Errorf("ParseRef(%q) = %+v, %v ; attendu %+v", in, got, err, want)
		}
	}
	if s := (Ref{"alice", "projet"}).String(); s != "alice/projet" {
		t.Errorf("String = %q", s)
	}

	bad := []string{
		"", "   ", "alice", "alice/", "/projet", "alice/projet/extra", "../x/y", "alice/..", "-alice/projet",
		"alice/pro jet", "https://gitlab.com/alice/projet", "https://github.com/alice",
		"https://github.com/alice/projet/tree/main", "https://github.com/alice/projet?tab=readme",
		"https://github.com/alice/projet#readme", "https://github.com/alice/projet?", "ftp://github.com/alice/projet",
		"https://user:pass@github.com/alice/projet", "https://github.com:8443/alice/projet", "github.com/alice/projet",
		"git@github.com:alice/projet.git", "https://user:secret@gitlab.com/alice/projet", "https://tok%zz@github.com/a/b",
	}
	for _, in := range bad {
		_, err := ParseRef(in)
		if err == nil {
			t.Errorf("ParseRef(%q) : erreur attendue", in)
			continue
		}
		if !strings.Contains(err.Error(), "owner/repo") {
			t.Errorf("ParseRef(%q) : le message doit citer la forme attendue : %v", in, err)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "pass") || strings.Contains(err.Error(), "tok") {
			t.Errorf("ParseRef(%q) : des identifiants apparaissent dans le message : %v", in, err)
		}
	}
	// Owner ou nom invalide : enveloppe provider.ErrInvalidRef et cite la valeur.
	_, err := ParseRef("-alice/projet")
	if !errors.Is(err, provider.ErrInvalidRef) || !strings.Contains(err.Error(), `"-alice/projet"`) {
		t.Errorf("ParseRef(-alice/projet) = %v", err)
	}
}

func TestParseBranchPatterns(t *testing.T) {
	ok := map[string][]string{
		"":                         nil,
		"  ":                       nil,
		", ,":                      nil,
		"main":                     {"main"},
		"main,dev,release/*":       {"main", "dev", "release/*"},
		" main , dev ,, main,dev ": {"main", "dev"},
		"feat-[a-z]*,v?":           {"feat-[a-z]*", "v?"},
	}
	for in, want := range ok {
		got, err := ParseBranchPatterns(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseBranchPatterns(%q) = %#v, %v ; attendu %#v", in, got, err, want)
		}
	}
	for _, in := range []string{"main,[abc", "release/[", "a\\", "ma\tin"} {
		_, err := ParseBranchPatterns(in)
		if err == nil {
			t.Errorf("ParseBranchPatterns(%q) : erreur attendue", in)
		}
	}
	_, err := ParseBranchPatterns("main,[abc")
	if err == nil || !strings.Contains(err.Error(), `"[abc"`) {
		t.Errorf("le message doit citer le motif : %v", err)
	}
}

func parseString(t *testing.T, s string) ([]Entry, Format, error) {
	t.Helper()
	return Parse(strings.NewReader(s))
}

func TestParseCSV(t *testing.T) {
	in := "\ufeffrepo,label,branches\n" +
		"alice/projet-licence,Alice Martin,\n" +
		"\n" +
		"# commentaire\n" +
		"bob/projet-licence,Bob Durand,\"main,dev\"\r\n" +
		"https://github.com/chloe/projet-licence.git, \"Chloé Petit\",\n" +
		"   # commentaire indenté\n" +
		"mon-org/projet-groupe-3,\"Groupe 3 (Dan, Eva)\",release/*\n"
	entries, format, err := parseString(t, in)
	if err != nil {
		t.Fatalf("Parse : %v", err)
	}
	if format != FormatCSV {
		t.Errorf("format %s, attendu csv", format)
	}
	want := []Entry{
		{Line: 2, Ref: Ref{"alice", "projet-licence"}, Label: "Alice Martin"},
		{Line: 5, Ref: Ref{"bob", "projet-licence"}, Label: "Bob Durand", Branches: []string{"main", "dev"}},
		{Line: 6, Ref: Ref{"chloe", "projet-licence"}, Label: "Chloé Petit"},
		{Line: 8, Ref: Ref{"mon-org", "projet-groupe-3"}, Label: "Groupe 3 (Dan, Eva)", Branches: []string{"release/*"}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entrées :\n%#v\nattendu :\n%#v", entries, want)
	}
}

func TestParseCSVColumnsOrderAndCase(t *testing.T) {
	entries, format, err := parseString(t, " Branches , REPO \nmain,alice/x\n,bob/y\n")
	if err != nil || format != FormatCSV {
		t.Fatalf("Parse : %v (%s)", err, format)
	}
	want := []Entry{
		{Line: 2, Ref: Ref{"alice", "x"}, Branches: []string{"main"}},
		{Line: 3, Ref: Ref{"bob", "y"}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entrées %#v", entries)
	}
	// Une seule colonne repo.
	entries, _, err = parseString(t, "repo\nalice/x\n")
	if err != nil || len(entries) != 1 || entries[0].Label != "" || entries[0].Branches != nil {
		t.Errorf("repo seul : %#v, %v", entries, err)
	}
}

func TestParseList(t *testing.T) {
	in := "# promotion 2026\n\nalice/projet\r\n  https://github.com/bob/projet  \n\t\n# fin\ncarol/projet.git\n"
	entries, format, err := parseString(t, in)
	if err != nil {
		t.Fatalf("Parse : %v", err)
	}
	if format != FormatList {
		t.Errorf("format %s, attendu list", format)
	}
	want := []Entry{
		{Line: 3, Ref: Ref{"alice", "projet"}},
		{Line: 4, Ref: Ref{"bob", "projet"}},
		{Line: 7, Ref: Ref{"carol", "projet"}},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entrées %#v", entries)
	}
}

func TestParseListWithBOM(t *testing.T) {
	entries, format, err := parseString(t, "\ufeffalice/projet\n")
	if err != nil || format != FormatList || len(entries) != 1 || entries[0].Ref != (Ref{"alice", "projet"}) {
		t.Errorf("Parse = %#v, %s, %v", entries, format, err)
	}
}

// lineErrors vérifie que err est une *ParseError et renvoie ses erreurs.
func lineErrors(t *testing.T, err error) []LineError {
	t.Helper()
	var pe *ParseError
	if !errors.As(err, &pe) {
		t.Fatalf("erreur %v (%T), attendu *ParseError", err, err)
	}
	if len(pe.Errors) == 0 {
		t.Fatal("ParseError sans erreur")
	}
	return pe.Errors
}

func errLines(errs []LineError) []int {
	var out []int
	for _, e := range errs {
		out = append(out, e.Line)
	}
	return out
}

func TestParseCSVErrorsAllReported(t *testing.T) {
	in := "repo,label,branches\n" +
		"alice/projet,Alice,\n" + // 2 : ok
		"pas un depot,Bob,\n" + // 3 : référence invalide
		",Carol,\n" + // 4 : repo vide
		"dan/projet,Dan\n" + // 5 : nombre de cellules
		"eva/projet,Eva,\"main,[x\"\n" + // 6 : motif invalide
		"ALICE/Projet,Alice bis,\n" + // 7 : doublon de la ligne 2
		"fred/projet,\"Fred,\n" + // 8 : guillemets
		"gus/projet,\"a\x01b\",\n" // 9 : label avec caractère de contrôle
	entries, format, err := parseString(t, in)
	if entries != nil {
		t.Errorf("aucune entrée attendue en cas d'erreur : %#v", entries)
	}
	if format != FormatCSV {
		t.Errorf("format %s", format)
	}
	errs := lineErrors(t, err)
	if got, want := errLines(errs), []int{3, 4, 5, 6, 7, 8, 9}; !reflect.DeepEqual(got, want) {
		t.Fatalf("lignes en erreur %v, attendu %v (%v)", got, want, err)
	}
	checks := map[int]string{
		3: `"pas un depot"`,
		4: "repo vide",
		5: "2 cellule(s) au lieu de 3",
		6: `"[x"`,
		7: "déjà présent ligne 2",
		8: "guillemets",
		9: "contrôle",
	}
	for _, e := range errs {
		if !strings.Contains(e.Msg, checks[e.Line]) {
			t.Errorf("ligne %d : %q ne contient pas %q", e.Line, e.Msg, checks[e.Line])
		}
		if strings.HasPrefix(e.Msg, "ligne") {
			t.Errorf("Msg ne doit pas porter le préfixe : %q", e.Msg)
		}
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "7 lignes en erreur :") || !strings.Contains(msg, "\n  ligne 7 : ") {
		t.Errorf("Error() = %q", msg)
	}
	if got := strings.Count(msg, "\n"); got != 7 {
		t.Errorf("une erreur par ligne de texte attendue, %d sauts de ligne : %q", got, msg)
	}
}

func TestParseHeaderErrors(t *testing.T) {
	tests := map[string]string{
		"repo,label,commentaire\nalice/x,A,b\n": `colonne "commentaire" inconnue`,
		"repo,label,Repo\nalice/x,A,b\n":        `colonne "repo" en double`,
	}
	for in, want := range tests {
		_, format, err := parseString(t, in)
		errs := lineErrors(t, err)
		if format != FormatCSV || errs[0].Line != 1 || !strings.Contains(errs[0].Msg, want) {
			t.Errorf("%q : %v (%s)", in, err, format)
		}
	}
	// En-tête après commentaires : son numéro de ligne réel est donné.
	_, _, err := parseString(t, "# import\n\nrepo,nom\nalice/x,A\n")
	if errs := lineErrors(t, err); errs[0].Line != 3 {
		t.Errorf("ligne d'en-tête %d, attendu 3", errs[0].Line)
	}
	// Les lignes de données mal découpées sont aussi signalées malgré l'en-tête invalide.
	_, _, err = parseString(t, "repo,nom\nalice/x,\"A\n")
	if got := errLines(lineErrors(t, err)); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Errorf("lignes %v", got)
	}
}

func TestParseListErrors(t *testing.T) {
	in := "alice/projet\nbob/projet,Bob\ncarol\nAlice/PROJET.git\ndan/projet extra\n"
	_, format, err := parseString(t, in)
	if format != FormatList {
		t.Errorf("format %s", format)
	}
	errs := lineErrors(t, err)
	if got := errLines(errs); !reflect.DeepEqual(got, []int{2, 3, 4, 5}) {
		t.Fatalf("lignes %v (%v)", got, err)
	}
	if !strings.Contains(errs[0].Msg, "repo,label,branches") {
		t.Errorf("ligne 2 : l'astuce CSV manque : %q", errs[0].Msg)
	}
	if !strings.Contains(errs[2].Msg, "ligne 1") {
		t.Errorf("ligne 4 : le doublon doit citer la ligne 1 : %q", errs[2].Msg)
	}
	if !strings.HasPrefix(err.Error(), "4 lignes en erreur") {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestParseEmpty(t *testing.T) {
	for _, in := range []string{"", "\ufeff", "\n\n  \n", "# rien\n# toujours rien\n", "repo,label,branches\n", "repo\n# aucun\n"} {
		entries, _, err := parseString(t, in)
		if entries != nil {
			t.Errorf("%q : entrées %#v", in, entries)
		}
		errs := lineErrors(t, err)
		if len(errs) != 1 || errs[0].Msg != "aucun dépôt dans le fichier" {
			t.Errorf("%q : %v", in, err)
		}
		if !errors.Is(err, ErrNoEntry) {
			t.Errorf("%q : errors.Is(ErrNoEntry) faux", in)
		}
		if err.Error() != "1 ligne en erreur :\n  "+errs[0].Error() {
			t.Errorf("%q : Error() = %q", in, err.Error())
		}
	}
	// Une autre erreur n'est pas ErrNoEntry.
	_, _, err := parseString(t, "carol\n")
	if errors.Is(err, ErrNoEntry) {
		t.Error("errors.Is(ErrNoEntry) vrai pour une référence invalide")
	}
}

func TestParseReadError(t *testing.T) {
	boom := errors.New("disque illisible")
	_, _, err := Parse(iotest.ErrReader(boom))
	var pe *ParseError
	if !errors.Is(err, boom) || errors.As(err, &pe) {
		t.Errorf("erreur %v, attendu l'erreur de lecture enveloppée", err)
	}
	_, _, err = Parse(strings.NewReader(strings.Repeat("a", maxLineBytes+10)))
	if err == nil || errors.As(err, &pe) {
		t.Errorf("ligne trop longue : %v", err)
	}
}

func TestLineError(t *testing.T) {
	if s := (LineError{Line: 12, Msg: "colonne repo vide"}).Error(); s != "ligne 12 : colonne repo vide" {
		t.Errorf("Error() = %q", s)
	}
}

func TestQuoteTruncates(t *testing.T) {
	_, err := ParseRef(strings.Repeat("x", 300))
	if err == nil || len(err.Error()) > 250 {
		t.Errorf("message trop long : %v", err)
	}
}

func intp(n int) *int { return &n }

func TestParseThreshold(t *testing.T) {
	for in, want := range map[string]*int{"": nil, "  ": nil, "0": intp(0), " 7 ": intp(7), "3650": intp(3650)} {
		got, err := ParseThreshold(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseThreshold(%q) = %v, %v ; attendu %v", in, got, err, want)
		}
	}
	for _, in := range []string{"-1", "abc", "1.5", "3651", "99999999999999999999"} {
		if _, err := ParseThreshold(in); err == nil || !strings.Contains(err.Error(), "nombre entier de jours") {
			t.Errorf("ParseThreshold(%q) : erreur %v", in, err)
		}
	}
}

func TestValidateThresholds(t *testing.T) {
	ok := [][2]*int{{nil, nil}, {intp(3), nil}, {nil, intp(2)}, {intp(3), intp(4)}, {intp(0), intp(2)}, {intp(5), intp(0)}, {intp(0), intp(0)}}
	for _, c := range ok {
		if err := ValidateThresholds(c[0], c[1]); err != nil {
			t.Errorf("ValidateThresholds(%v, %v) : %v", c[0], c[1], err)
		}
	}
	bad := map[[2]*int]string{
		{intp(5), intp(5)}:  "strictement supérieur",
		{intp(10), intp(3)}: "strictement supérieur",
		{intp(-1), nil}:     "seuil flemme",
		{nil, intp(4000)}:   "seuil dead",
	}
	for c, want := range bad {
		if err := ValidateThresholds(c[0], c[1]); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ValidateThresholds(%v, %v) : %v, attendu %q", c[0], c[1], err, want)
		}
	}
}

func TestParseCSVThresholds(t *testing.T) {
	in := "repo,label,flemme,DEAD\n" +
		"alice/projet,Alice,3,10\n" +
		"bob/projet,,,\n" +
		"carol/projet,,0,\n"
	entries, _, err := parseString(t, in)
	if err != nil {
		t.Fatalf("Parse : %v", err)
	}
	want := []Entry{
		{Line: 2, Ref: Ref{"alice", "projet"}, Label: "Alice", FlemmeAfterDays: intp(3), DeadAfterDays: intp(10)},
		{Line: 3, Ref: Ref{"bob", "projet"}},
		{Line: 4, Ref: Ref{"carol", "projet"}, FlemmeAfterDays: intp(0)},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Errorf("entrées :\n%#v\nattendu :\n%#v", entries, want)
	}

	_, _, err = parseString(t, "repo,flemme,dead\nalice/projet,x,\nbob/projet,10,5\ncarol/projet,2,-3\n")
	var pe *ParseError
	if !errors.As(err, &pe) || len(pe.Errors) != 3 {
		t.Fatalf("erreurs : %v", err)
	}
	for i, want := range []string{"flemme : \"x\"", "strictement supérieur", "dead : \"-3\""} {
		if !strings.Contains(pe.Errors[i].Msg, want) {
			t.Errorf("erreur %d = %q, attendu %q", i, pe.Errors[i].Msg, want)
		}
	}
}
