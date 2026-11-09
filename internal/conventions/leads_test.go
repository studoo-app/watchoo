package conventions

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// leadOf renvoie la piste id mesurée sur l'arbre.
func leadOf(t *testing.T, tree FileTree, opts FileOptions, id string) LeadCheck {
	t.Helper()
	leads, _ := CheckLeads(tree, opts)
	l, ok := FindLead(leads, id)
	if !ok {
		t.Fatalf("piste %s absente de %+v", id, leads)
	}
	return l
}

// wantLead vérifie l'état, la raison et les codes d'une piste.
func wantLead(t *testing.T, l LeadCheck, status, reason string, codes ...string) {
	t.Helper()
	if codes == nil {
		codes = []string{}
	}
	if l.Status != status || l.Reason != reason || !reflect.DeepEqual(l.Codes, codes) {
		t.Errorf("%s : état %q raison %q codes %q (compteurs %v), attendu %q %q %q", l.ID, l.Status, l.Reason,
			l.Codes, l.Counts, status, reason, codes)
	}
}

func TestLeadCatalog(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range AllLeadCodes {
		if seen[c] {
			t.Errorf("code %q en double", c)
		}
		seen[c] = true
	}
	for _, id := range LeadCriteria {
		for _, ids := range [][]string{ChangelogCriteria, ReadmeCriteria, DocsCriteria, ToolingCriteria} {
			if slices.Contains(ids, id) {
				t.Errorf("piste %s rangée dans une grille", id)
			}
		}
	}
	for _, id := range TreeLeadCriteria {
		if !slices.Contains(LeadCriteria, id) {
			t.Errorf("piste de l'arbre %s hors de LeadCriteria", id)
		}
	}
	if got := CIChecks(1<<0 | 1<<3); !reflect.DeepEqual(got, []string{CICommitlint, CICocogitto}) {
		t.Errorf("CIChecks = %v", got)
	}
}

func TestLeadsTreeUnusable(t *testing.T) {
	for _, tc := range []struct {
		tree   FileTree
		reason string
	}{
		{FileTree{}, UnknownEmptyRepository},
		{FileTree{Paths: []string{"README.md"}, Truncated: true}, UnknownTreeTruncated},
	} {
		leads, broken := CheckLeads(tc.tree, fileOpts())
		if len(leads) != len(TreeLeadCriteria) || broken != nil {
			t.Fatalf("pistes = %+v, %v", leads, broken)
		}
		for i, l := range leads {
			if l.ID != TreeLeadCriteria[i] {
				t.Errorf("ordre : %s à la place de %s", l.ID, TreeLeadCriteria[i])
			}
			wantLead(t, l, CriterionUnknown, tc.reason)
		}
		if BrokenImages(tc.tree, fileOpts()) != nil {
			t.Error("BrokenImages sur un arbre inutilisable")
		}
	}
}

func TestLeadCL11(t *testing.T) {
	const head = "# Changelog\n\n"
	for name, tc := range map[string]struct {
		changelog string
		status    string
		codes     []string
		titles    int
		linked    int
	}{
		"toutes les définitions": {head + "## [Unreleased]\n\n## [1.1.0] - 2026-02-01\n\n- b\n\n## [1.0.0] - 2026-01-01\n\n- a\n\n" +
			"[unreleased]: https://github.com/o/n/compare/v1.1.0...HEAD\n[1.1.0]: https://github.com/o/n/compare/v1.0.0...v1.1.0\n" +
			"[1.0.0]: https://github.com/o/n/releases/tag/v1.0.0\n", LeadNone, nil, 3, 3},
		"préfixe v d'un côté": {head + "## [v1.0.0] - 2026-01-01\n\n- a\n\n[1.0.0]: https://x/y\n", LeadNone, nil, 1, 1},
		"lien dans le titre":  {head + "## [1.0.0](https://x/y) - 2026-01-01\n\n- a\n", LeadNone, nil, 1, 1},
		"définitions manquantes": {head + "## [Unreleased]\n\n## [1.1.0] - 2026-02-01\n\n- b\n\n## [1.0.0] - 2026-01-01\n\n- a\n\n" +
			"[1.0.0]: https://x/y\n", LeadReview, []string{LeadMissingLink}, 3, 1},
		"définition vide ignorée":    {head + "## [1.0.0] - 2026-01-01\n\n- a\n\n[1.0.0]: <>\n", LeadReview, []string{LeadMissingLink}, 1, 0},
		"définition dans du code":    {head + "## [1.0.0] - 2026-01-01\n\n```\n[1.0.0]: https://x/y\n```\n", LeadReview, []string{LeadMissingLink}, 1, 0},
		"titre illisible non compté": {head + "## Notes diverses\n\n- a\n", LeadNotApplicable, nil, 0, 0},
		"aucun titre":                {head + "Rien.\n", LeadNotApplicable, nil, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			l := leadOf(t, treeOf(map[string]string{"CHANGELOG.md": tc.changelog}), fileOpts(), CriterionCL11)
			reason := ""
			if tc.status == LeadNotApplicable {
				reason = UnknownNoVersion // aucun titre de version : sans objet, pas « rien à relire »
			}
			wantLead(t, l, tc.status, reason, tc.codes...)
			if l.Counts[CountTitles] != tc.titles || l.Counts[CountLinked] != tc.linked ||
				l.Counts[LeadMissingLink] != tc.titles-tc.linked {
				t.Errorf("compteurs = %v, attendu %d titres, %d liés", l.Counts, tc.titles, tc.linked)
			}
		})
	}
}

func TestLeadChangelogUnread(t *testing.T) {
	for _, tc := range []struct {
		tree   FileTree
		reason string
	}{
		{treeOf(map[string]string{"README.md": "# R\n"}), UnknownNoFile},
		{FileTree{Paths: []string{"CHANGELOG.md"}, Blobs: map[string]Blob{"CHANGELOG.md": {TooLarge: true}}}, UnknownBlobTooLarge},
		{FileTree{Paths: []string{"CHANGELOG.md"}}, UnknownBlobUnreadable},
	} {
		for _, id := range []string{CriterionCL11, CriterionCL12} {
			wantLead(t, leadOf(t, tc.tree, fileOpts(), id), CriterionUnknown, tc.reason)
		}
	}
	// Un CHANGELOG mal nommé est quand même lu, comme pour CL2 à CL8.
	l := leadOf(t, treeOf(map[string]string{"changelog.md": "## [1.0.0] - 2026-01-01\n"}), fileOpts(), CriterionCL11)
	wantLead(t, l, LeadReview, "", LeadMissingLink)
}

// bullets renvoie un CHANGELOG avec les puces données.
func bullets(items ...string) string {
	var b strings.Builder
	b.WriteString("# Changelog\n\n## [Unreleased]\n\n### Added\n\n")
	for _, it := range items {
		b.WriteString("- " + it + "\n")
	}
	return b.String()
}

// repeat renvoie n puces formées par f.
func repeat(n int, f func(i int) string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = f(i)
	}
	return out
}

func TestLeadCL12(t *testing.T) {
	subjects := repeat(20, func(i int) string { return fmt.Sprintf("feat: ajoute l'écran %d (#%d)", i, i+10) })
	human := func(i int) string { return fmt.Sprintf("Nouvel écran de connexion numéro %d", i) }
	for name, tc := range map[string]struct {
		items          []string
		status, reason string
		codes          []string
		subjectPct     int
		shaPct         int
	}{
		"trop peu de puces": {repeat(9, func(i int) string { return subjects[i] }), LeadNotApplicable, UnknownTooFew, nil, 100, 0},
		"rédigé à la main":  {repeat(10, human), LeadNone, "", nil, 0, 0},
		// 6 sur 10 identiques après normalisation (casse, ponctuation, suffixe « (#n) ») : seuil atteint.
		"recopie des sujets": {append(repeat(6, func(i int) string { return strings.ToUpper(fmt.Sprintf("feat ajoute l'ecran %d", i)) }),
			repeat(4, human)...), LeadReview, "", []string{LeadCopiedSubjects}, 60, 0},
		"juste sous le seuil": {append(repeat(5, func(i int) string { return subjects[i] }), repeat(5, human)...),
			LeadNone, "", nil, 50, 0},
		// 2 sur 10 avec un SHA ou « Merge » : seuil atteint.
		"SHA et Merge": {append([]string{"a1b2c3d corrige le menu", "Merge branch 'dev'"}, repeat(8, human)...),
			LeadReview, "", []string{LeadShaOrMerge}, 0, 20},
		// « defaced » (lettres seules) et « 2026091 » (chiffres seuls) ne sont pas des SHA.
		"mots hexadécimaux": {append([]string{"defaced logo", "2026091 lignes", "Mergers et acquisitions"}, repeat(7, human)...),
			LeadNone, "", nil, 0, 0},
		"les deux": {repeat(10, func(i int) string { return subjects[i] + " " + strings.Repeat("ab12", 2) }),
			LeadReview, "", []string{LeadShaOrMerge}, 0, 100},
	} {
		t.Run(name, func(t *testing.T) {
			opts := fileOpts()
			opts.Subjects = subjects
			l := leadOf(t, treeOf(map[string]string{"CHANGELOG.md": bullets(tc.items...)}), opts, CriterionCL12)
			wantLead(t, l, tc.status, tc.reason, tc.codes...)
			if l.Counts[CountBullets] != len(tc.items) || l.Counts[CountSubjects] != 20 ||
				l.Counts[CountSubjectPercent] != tc.subjectPct || l.Counts[CountShaMergePercent] != tc.shaPct {
				t.Errorf("compteurs = %v", l.Counts)
			}
		})
	}
	// Recopie et SHA ensemble ; puces vides non comptées ; aucun sujet connu.
	items := append(repeat(10, func(i int) string { return subjects[i] + " (a1b2c3d)" }), "", "TODO")
	opts := fileOpts()
	opts.Subjects = append([]string{"", "   "}, repeat(10, func(i int) string { return subjects[i] + " (a1b2c3d)" })...)
	l := leadOf(t, treeOf(map[string]string{"CHANGELOG.md": bullets(items...)}), opts, CriterionCL12)
	wantLead(t, l, LeadReview, "", LeadCopiedSubjects, LeadShaOrMerge)
	if l.Counts[CountBullets] != 11 || l.Counts[CountSubjects] != 10 {
		t.Errorf("compteurs = %v", l.Counts)
	}
	l = leadOf(t, treeOf(map[string]string{"CHANGELOG.md": bullets(repeat(10, human)...)}), fileOpts(), CriterionCL12)
	if l.Status != LeadNone || l.Counts[CountSubjects] != 0 {
		t.Errorf("sans sujets : %+v", l)
	}
}

func TestLeadRD5(t *testing.T) {
	for name, tc := range map[string]struct {
		files   map[string]string
		codes   []string
		blocks  int
		env     int
		example int
	}{
		"commandes sous installation": {map[string]string{"README.md": "# P\n\n## Installation\n\n```bash\nnpm ci\n```\n"},
			nil, 1, 0, 0},
		"sous-titre d'une section lancement, bloc sans langage": {map[string]string{
			"README.md": "# P\n\n## Utilisation\n\n### Backend\n\n```\nmake run\n```\n\n```console\n$ x\n```\n"}, nil, 2, 0, 0},
		"bloc dans une autre section": {map[string]string{
			"README.md": "# P\n\n## Installation\n\nVoir le wiki.\n\n## Exemple\n\n```bash\nls\n```\n"},
			[]string{LeadNoCommandBlock}, 0, 0, 0},
		"bloc d'un autre langage": {map[string]string{"README.md": "# P\n\n## Installation\n\n```json\n{}\n```\n"},
			[]string{LeadNoCommandBlock}, 0, 0, 0},
		"bloc avant tout titre": {map[string]string{"README.md": "```bash\nls\n```\n\n## Installation\n\nRien.\n"},
			[]string{LeadNoCommandBlock}, 0, 0, 0},
		"ni installation ni lancement": {map[string]string{"README.md": "# P\n\n## Présentation\n\n```bash\nls\n```\n"},
			[]string{LeadNoInstallSection}, 0, 0, 0},
		"variables annoncées sans modèle": {map[string]string{
			"README.md": "# P\n\n## Installation\n\n```sh\nnpm ci\n```\n\n## Variables d'environnement\n\nVoir plus bas.\n"},
			[]string{LeadNoEnvExample}, 1, 1, 0},
		".env mentionné, modèle présent": {map[string]string{
			"README.md":            "# P\n\n## Installation\n\nCopier `.env.example` vers .env puis :\n\n```sh\nnpm ci\n```\n",
			"backend/.env.example": "PORT=3000\n"}, nil, 1, 1, 1},
		".env en code ou dans un autre mot": {map[string]string{
			"README.md": "# P\n\n## Installation\n\n```sh\nnpm ci\n```\n\nLe port est lu dans `process.env.PORT`, " +
				"via import.meta.env ou process.env ; voir .envrc et https://x.example/.env/doc.\n"}, nil, 1, 0, 0},
		".env cité seul entre parenthèses": {map[string]string{
			"README.md": "# P\n\n## Installation\n\n```sh\nnpm ci\n```\n\nRenseigner le fichier (.env) avant de lancer.\n"},
			[]string{LeadNoEnvExample}, 1, 1, 0},
		"modèle sans annonce": {map[string]string{"README.md": "# P\n\n## Installation\n\n```sh\nnpm ci\n```\n",
			".env.sample": ""}, nil, 1, 0, 1},
		"modèle trop profond ou vendu": {map[string]string{"README.md": "# P\n\n## Installation\n\n```sh\nx\n```\n\n## Configuration\n",
			"a/b/c/.env.example": "", "node_modules/x/.env.example": ""}, []string{LeadNoEnvExample}, 1, 1, 0},
	} {
		t.Run(name, func(t *testing.T) {
			l := leadOf(t, treeOf(tc.files), fileOpts(), CriterionRD5)
			status := LeadNone
			if len(tc.codes) > 0 {
				status = LeadReview
			}
			wantLead(t, l, status, "", tc.codes...)
			if l.Counts[CountCommandBlocks] != tc.blocks || l.Counts[CountEnvAnnounced] != tc.env ||
				l.Counts[CountEnvExample] != tc.example {
				t.Errorf("compteurs = %v", l.Counts)
			}
		})
	}
	wantLead(t, leadOf(t, treeOf(map[string]string{"CHANGELOG.md": ""}), fileOpts(), CriterionRD5), CriterionUnknown, UnknownNoFile)
	wantLead(t, leadOf(t, FileTree{Paths: []string{"README.md"}, Blobs: map[string]Blob{"README.md": {TooLarge: true}}},
		fileOpts(), CriterionRD5), CriterionUnknown, UnknownBlobTooLarge)
}

func TestHeaderFields(t *testing.T) {
	all := map[string]bool{HeaderTitle: true, HeaderObjective: true, HeaderScope: true, HeaderAuthor: true,
		HeaderDate: true, HeaderVersion: true}
	for name, tc := range map[string]struct {
		doc  string
		want map[string]bool
	}{
		"front matter":      {"---\ntitle: Vision\nobjective: cadrer\nscope: tout\nauthors:\n  - Alice\ndate: 2026-01-01\nversion: 1.0\n---\n\nTexte.\n", all},
		"lignes FR en gras": {"# Vision\n\n**Objectif :** cadrer\n**Périmètre :** tout\n**Auteur :** Alice\n**Date :** 01/01/2026\n**Version :** 1.0\n", all},
		"puces et ligne de tableau": {"# Vision\n\n- Purpose: frame\n- Scope: all\n\n| Author | Alice |\n|---|---|\n| Last update | 2026 |\n| Version | 2 |\n",
			all},
		"valeurs vides":                {"Titre :\nAuteur : **\nVersion: 1\n", map[string]bool{HeaderVersion: true}},
		"clé inconnue ou trop longue":  {"Remarque : rien\n" + strings.Repeat("x", 41) + " auteur : Alice\n", map[string]bool{}},
		"au-delà des 40 lignes":        {strings.Repeat("texte\n", 40) + "Auteur : Alice\n", map[string]bool{}},
		"liste YAML hors front matter": {"Auteurs:\n  - Alice\n", map[string]bool{}},
		"BOM et CRLF":                  {"\uFEFF# Titre\r\nVersion : 3\r\n", map[string]bool{HeaderTitle: true, HeaderVersion: true}},
		"blocs de code ignorés": {"Texte.\n\n```sh\n# installer\nversion: 2\n```\n\n~~~~yaml\nauteur: Alice\n```\ndate: 2026\n~~~~\n" +
			"Objectif : cadrer\n", map[string]bool{HeaderObjective: true}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := headerFields([]byte(tc.doc)); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("headerFields = %v, attendu %v", got, tc.want)
			}
		})
	}
}

func TestLeadDOC6(t *testing.T) {
	complete := "---\ntitle: A\nobjective: B\nscope: C\nauthor: Alice Secrète\ndate: 2026-01-01\nversion: 1\n---\n"
	tree := treeOf(map[string]string{
		"documentation/INDEX.md":           "# Index\n",
		"documentation/01-a/complet.md":    complete,
		"documentation/01-a/partiel.md":    "# Partiel\n\nAuteur : Alice Secrète\n",
		"documentation/Media/notes.md":     "# Ignoré\n",
		"documentation/01-a/schema.png":    "",
		"documentation/01-a/vide.md":       "",
		"documentation/02-b/sans-titre.md": "Rien.\n",
	})
	l := leadOf(t, tree, fileOpts(), CriterionDOC6)
	wantLead(t, l, LeadReview, "", LeadMissingHeaders)
	want := map[string]int{CountDocuments: 4, CountComplete: 1, LeadMissingHeaders: 3, "header_title": 2,
		"header_objective": 1, "header_scope": 1, "header_author": 2, "header_date": 1, "header_version": 1}
	if !reflect.DeepEqual(l.Counts, want) {
		t.Errorf("compteurs = %v, attendu %v", l.Counts, want)
	}
	raw, _ := json.Marshal(l)
	for _, leak := range []string{"Alice", "Secr", "2026", "partiel", "sans-titre", ".md"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("DOC6 contient %q : %s", leak, raw)
		}
	}

	wantLead(t, leadOf(t, treeOf(map[string]string{"documentation/INDEX.md": "", "documentation/a.md": complete}),
		fileOpts(), CriterionDOC6), LeadNone, "")
	wantLead(t, leadOf(t, treeOf(map[string]string{"documentation/INDEX.md": ""}), fileOpts(), CriterionDOC6), LeadNone, "")
	wantLead(t, leadOf(t, treeOf(map[string]string{"README.md": ""}), fileOpts(), CriterionDOC6), CriterionUnknown, UnknownNoDocsDir)
	unread := FileTree{Paths: []string{"documentation/a.md"}, Blobs: map[string]Blob{}}
	wantLead(t, leadOf(t, unread, fileOpts(), CriterionDOC6), CriterionUnknown, UnknownBlobUnreadable)

	// Au-delà de MaxDocuments : les suivants sont « non lus ».
	many := map[string]string{}
	for i := range MaxDocuments + 2 {
		many[fmt.Sprintf("documentation/d%03d.md", i)] = complete
	}
	l = leadOf(t, treeOf(many), fileOpts(), CriterionDOC6)
	if l.Status != LeadNone || l.Counts[CountDocuments] != MaxDocuments || l.Counts[CountUnread] != 2 {
		t.Errorf("plafond : %+v", l)
	}
}

func TestLeadDOC7(t *testing.T) {
	tree := treeOf(map[string]string{
		"README.md": "# P\n\n[![CI](https://github.com/o/n/actions/workflows/ci.yml/badge.svg)](https://x)\n" +
			"![logo](assets/logo.png)\n![cassé](assets/absent.png)\n" +
			"<img src=\"https://example.com/photo.jpg\" alt=\"x\">\n![shield](https://img.shields.io/badge/x-y-green)\n" +
			"`![code](absent-en-code.png)`\n",
		"assets/logo.png":               "",
		"documentation/INDEX.md":        "# Index\n\n![schéma](Media/archi.PNG)\n",
		"documentation/Media/archi.png": "",
		"documentation/01-a/diagramme.md": "# D\n\n```mermaid\ngraph TD\n```\n\n```{.mermaid}\nA-->B\n```\n\n![ok](../Media/archi.png)\n" +
			"![même dépôt](https://github.com/promo/projet/blob/main/assets/logo.png)\n![data](data:image/png;base64,xx)\n",
	})
	leads, broken := CheckLeads(tree, fileOpts())
	l, _ := FindLead(leads, CriterionDOC7)
	wantLead(t, l, LeadReview, "", LeadBrokenImage, LeadExternalImage)
	want := map[string]int{CountSources: 3, CountImages: 5, LeadBrokenImage: 2, LeadExternalImage: 1, CountBadges: 2,
		CountMermaid: 2, CountMediaDir: 1}
	if !reflect.DeepEqual(l.Counts, want) {
		t.Errorf("compteurs = %v, attendu %v", l.Counts, want)
	}
	wantBroken := []DeadLink{
		{Source: "README.md", Target: "assets/absent.png", Resolved: "assets/absent.png"},
		{Source: "documentation/INDEX.md", Target: "Media/archi.PNG", Resolved: "documentation/Media/archi.PNG", CaseMismatch: true},
	}
	if !reflect.DeepEqual(broken, wantBroken) {
		t.Errorf("images cassées = %+v", broken)
	}
	if got := BrokenImages(tree, fileOpts()); !reflect.DeepEqual(got, wantBroken) {
		t.Errorf("BrokenImages = %+v", got)
	}
	raw, _ := json.Marshal(l)
	for _, leak := range []string{"png", "assets", "README", "example.com"} {
		if strings.Contains(string(raw), leak) {
			t.Errorf("DOC7 contient %q : %s", leak, raw)
		}
	}

	// Rien à relire, aucun dossier des médias ; documents non lus sans image cassée trouvée : inconnu.
	clean := treeOf(map[string]string{"README.md": "# P\n\n![l](l.png)\n", "l.png": ""})
	l = leadOf(t, clean, fileOpts(), CriterionDOC7)
	wantLead(t, l, LeadNone, "")
	if l.Counts[CountMediaDir] != 0 || l.Counts[CountImages] != 1 {
		t.Errorf("compteurs = %v", l.Counts)
	}
	unread := FileTree{Paths: []string{"README.md"}, Blobs: map[string]Blob{}}
	wantLead(t, leadOf(t, unread, fileOpts(), CriterionDOC7), CriterionUnknown, UnknownUnreadDocuments)
	// Dossier des médias à une autre casse : présent.
	l = leadOf(t, treeOf(map[string]string{"documentation/media/a.png": ""}), fileOpts(), CriterionDOC7)
	if l.Counts[CountMediaDir] != 1 {
		t.Errorf("médias à une autre casse : %v", l.Counts)
	}
}

func TestImagesIn(t *testing.T) {
	got := imagesIn(`[![a](b.svg)](c) ![d](<e f.png>) <IMG SRC='g.gif'> [h](i.png) ` + "`![j](k.png)`")
	if want := []string{"b.svg", "e f.png", "g.gif"}; !reflect.DeepEqual(got, want) {
		t.Errorf("imagesIn = %q, attendu %q", got, want)
	}
	for u, badge := range map[string]bool{
		"https://img.shields.io/x": true, "https://github.com/o/n/actions/workflows/ci.yml/badge.svg": true,
		"//codecov.io/gh/o/n/graph.svg": true, "https://example.com/a.png": false,
	} {
		if isBadge(u) != badge {
			t.Errorf("isBadge(%q) = %v", u, !badge)
		}
	}
}

func TestLeadTL4(t *testing.T) {
	for name, tc := range map[string]struct {
		files     map[string]string
		status    string
		reason    string
		codes     []string
		workflows int
		mask      int
	}{
		"commitlint et semantic-pull-request": {map[string]string{
			".github/workflows/lint.yml":  "jobs:\n  c:\n    steps:\n      - uses: wagoid/commitlint-github-action@v6\n",
			".github/workflows/pr.yaml":   "      - uses: amannn/action-semantic-pull-request@v5\n",
			".github/workflows/build.yml": "      - run: go test ./...\n",
		}, LeadNone, "", nil, 3, 1<<0 | 1<<1},
		"commande cog et cz": {map[string]string{".github/workflows/ci.yml": "      - run: cog check --from-latest-tag\n      - run: cz check --rev-range HEAD~1..HEAD\n"},
			LeadNone, "", nil, 1, 1<<2 | 1<<3},
		"action webiny": {map[string]string{".github/workflows/ci.yml": "uses: webiny/action-conventional-commits@v1\n"},
			LeadNone, "", nil, 1, 1 << 4},
		"commentaire seulement": {map[string]string{".github/workflows/ci.yml": "# TODO: commitlint\n  # uses: wagoid/commitlint-github-action\n"},
			LeadReview, "", []string{LeadCINotFound}, 1, 0},
		"hors de .github/workflows": {map[string]string{".github/workflows/sub/ci.yml": "commitlint\n",
			"ci/commitlint.yml": "commitlint\n", ".github/workflows/notes.md": "commitlint\n"},
			LeadReview, "", []string{LeadCINotFound}, 0, 0},
		"aucun workflow": {map[string]string{"README.md": ""}, LeadReview, "", []string{LeadCINotFound}, 0, 0},
	} {
		t.Run(name, func(t *testing.T) {
			l := leadOf(t, treeOf(tc.files), fileOpts(), CriterionTL4)
			wantLead(t, l, tc.status, tc.reason, tc.codes...)
			if l.Counts[CountWorkflows] != tc.workflows || l.Counts[CountCIMask] != tc.mask {
				t.Errorf("compteurs = %v", l.Counts)
			}
		})
	}
	unread := FileTree{Paths: []string{".github/workflows/ci.yml"}, Blobs: map[string]Blob{}}
	l := leadOf(t, unread, fileOpts(), CriterionTL4)
	wantLead(t, l, CriterionUnknown, UnknownBlobUnreadable)
	if l.Counts[CountWorkflows] != 1 || l.Counts[CountUnread] != 1 {
		t.Errorf("compteurs = %v", l.Counts)
	}
}

func TestFileBlobPathsLeads(t *testing.T) {
	paths := []string{".github/workflows/ci.yml", ".github/workflows/pr.yaml", ".github/workflows/sub/x.yml",
		".github/workflows/notes.md", ".env.example", "src/main.go"}
	got := FileBlobPaths(paths, DefaultFileRules())
	if want := []string{".github/workflows/ci.yml", ".github/workflows/pr.yaml"}; !reflect.DeepEqual(got, want) {
		t.Errorf("FileBlobPaths = %q, attendu %q", got, want)
	}
}

func TestCheckFilesLeads(t *testing.T) {
	tree := treeOf(map[string]string{
		"CHANGELOG.md":             bullets(repeat(10, func(i int) string { return fmt.Sprintf("feat: sujet secret %d", i) })...),
		"README.md":                "# Projet\n\n## Installation\n\n```bash\nnpm ci # secret\n```\n\n![x](Secret.png)\n",
		"documentation/INDEX.md":   "# Index\n",
		"documentation/a.md":       "Auteur : Alice Martin\nDate : 2026-03-04\n",
		".github/workflows/ci.yml": "uses: wagoid/commitlint-github-action@v6\n",
	})
	opts := fileOpts()
	opts.Subjects = repeat(10, func(i int) string { return fmt.Sprintf("feat: sujet secret %d", i) })
	res, lists := CheckFiles(tree, opts)
	var ids []string
	for _, l := range res.Leads {
		ids = append(ids, l.ID)
	}
	if !reflect.DeepEqual(ids, TreeLeadCriteria) {
		t.Fatalf("pistes = %v", ids)
	}
	if l, _ := FindLead(res.Leads, CriterionCL12); l.Status != LeadReview {
		t.Errorf("CL12 = %+v", l)
	}
	if len(lists.BrokenImages) != 1 || lists.BrokenImages[0].Target != "Secret.png" {
		t.Errorf("images cassées = %+v", lists.BrokenImages)
	}
	// Les pistes n'entrent dans aucune grille ni dans Find.
	for _, id := range LeadCriteria {
		if _, ok := res.Find(id); ok {
			t.Errorf("Find(%s) trouve une piste", id)
		}
	}
	for _, fam := range res.Families() {
		for _, c := range fam.Checks {
			if slices.Contains(LeadCriteria, c.ID) {
				t.Errorf("piste %s dans la grille %s", c.ID, fam.Name)
			}
		}
	}
	raw, err := EncodeFileResults(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"ecret", "Alice", "Martin", "2026", "npm", ".png", ".md", ".github", "ci.yml", "wagoid", "documentation"} {
		if strings.Contains(raw, leak) {
			t.Errorf("le résultat encodé contient %q : %s", leak, raw)
		}
	}
	back, err := DecodeFileResults(raw)
	if err != nil || !reflect.DeepEqual(back, res) {
		t.Errorf("aller-retour : %v\n%+v\n%+v", err, back, res)
	}

	// Tag d'un jalon : aucune piste, aucune image cassée.
	opts.Version = "v1.0.0"
	res, lists = CheckFiles(tree, opts)
	if res.Leads != nil || lists.BrokenImages != nil {
		t.Errorf("pistes sur un jalon : %+v %+v", res.Leads, lists.BrokenImages)
	}
	if raw, _ := EncodeFileResults(res); strings.Contains(raw, "leads") {
		t.Errorf("clé leads sur un jalon : %s", raw)
	}
}

func TestUnknownFileResultsLeads(t *testing.T) {
	head := UnknownFileResults(UnknownStale, false)
	if len(head.Leads) != len(TreeLeadCriteria) {
		t.Fatalf("pistes = %+v", head.Leads)
	}
	for _, l := range head.Leads {
		wantLead(t, l, CriterionUnknown, UnknownStale)
	}
	if UnknownFileResults(UnknownStale, true).Leads != nil {
		t.Error("pistes sur un jalon")
	}
}
