package diagnostic

// Mesures de forme d'un commit (phase 7.5, signaux F1 à F5 ; spécification, section 4.3) : calculées en
// mémoire à partir des lignes AJOUTÉES du commit (diff au premier parent, sans contexte), lues sur le clone
// local. Seules les mesures sont gardées (commit_signals) : ni ligne de code, ni identifiant, ni chemin, ni
// message. Les règles sont volontairement simples (reconnaissance ligne à ligne, sans analyse syntaxique) :
// ces signaux sont des indices d'appoint, jamais suffisants seuls.

import (
	"path"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Paramètres des mesures de forme.
const (
	// MATTRWindow est la fenêtre de la diversité lexicale (moving-average type-token ratio, Covington et
	// McFall, 2010) : la mesure demande au moins MATTRWindow identifiants dans le commit.
	MATTRWindow = 50
	// maxFormLine : une ligne ajoutée plus longue (code minifié, données) n'est pas analysée.
	maxFormLine = 1000
)

// Form sont les mesures de forme d'un commit, sur les fichiers retenus (ni binaires ni exclus).
type Form struct {
	SourceLines         int     // lignes ajoutées non vides des fichiers source reconnus (tests compris)
	CommentLines        int     // parmi elles, lignes de commentaire ou de docstring (F1)
	Functions           int     // définitions de fonctions ou de méthodes ajoutées (F2)
	DocumentedFunctions int     // parmi elles, précédées d'un commentaire ou suivies d'une docstring (F2)
	Identifiers         int     // identifiants du code ajouté, hors mots-clés (F3)
	LexicalDiversity    float64 // MATTR des identifiants (F3) ; voir HasDiversity
	HasDiversity        bool    // au moins MATTRWindow identifiants
	TestLines           int     // lignes ajoutées non vides des fichiers source de test (F5)

	// Mesures de la phase 7.7 (signaux absolus A, contre-indicateurs H).
	DecorativeLines   int // lignes de commentaire décoratives (« // ── Section ── », « # ════ ») (A5)
	FrWords           int // mots français accentués courants repérés dans les commentaires (A6)
	FrWordsUnaccented int // parmi eux, écrits sans leurs accents (« donnees ») (A6)
	WIPMarkers        int // marqueurs de travail en cours : TODO, FIXME, XXX, HACK, appels de debug (A7, H1)
	ShortIdentifiers  int // identifiants d'une lettre (« $i », « j ») (A7, H2)
	PrivateDefs       int // définitions privées parmi Functions
}

// FormMeter mesure la forme d'un commit, ligne ajoutée par ligne ajoutée (Line), puis Result. Les lignes
// ne sont pas gardées : seul l'état de la ligne courante et la fenêtre des derniers identifiants le sont.
type FormMeter struct {
	ex   *Excluder
	form Form

	path   string
	lang   *language // nil : fichier ignoré (non source, binaire, exclu)
	test   bool
	block  string // fin de commentaire bloc en cours (« */ », « --> », « #} ») ; vide sinon
	triple string // délimiteur de chaîne triple en cours (Python) ; vide sinon
	doc    bool   // la chaîne triple en cours est une docstring
	// afterComment : la dernière ligne non vide (hors annotations) du bloc ajouté est un commentaire.
	afterComment bool
	pendingDef   bool // Python : la définition précédente attend sa docstring

	window   []string // derniers identifiants (au plus MATTRWindow), en anneau
	next     int
	counts   map[string]int
	distinct int
	ttrSum   float64
	ttrN     int
}

// NewFormMeter renvoie un compteur pour un commit ; ex applique diagnostic.exclude (nil : rien d'exclu).
func NewFormMeter(ex *Excluder) *FormMeter {
	return &FormMeter{ex: ex, counts: map[string]int{}}
}

// Line ajoute une ligne ajoutée (sans le « + » du diff ni la fin de ligne) du fichier p ; hunk est vrai
// pour la première ligne d'un bloc de changement (les blocs ne sont pas contigus : l'état est remis à
// zéro).
func (m *FormMeter) Line(p, text string, hunk bool) {
	if p != m.path {
		m.path = p
		m.lang, m.test = nil, false
		if !m.ex.Excluded(p) {
			m.lang = languageOf(p)
			m.test = m.lang != nil && isTestPath(p)
		}
		hunk = true
	}
	if m.lang == nil {
		return
	}
	if hunk {
		m.block, m.triple, m.doc, m.afterComment, m.pendingDef = "", "", false, false, false
	}
	if len(text) > maxFormLine || !utf8.ValidString(text) {
		m.afterComment, m.pendingDef = false, false
		return
	}
	line := strings.TrimSpace(text)
	if line == "" {
		return
	}
	m.form.SourceLines++
	if m.test {
		m.form.TestLines++
	}
	switch kind := m.classify(line); kind {
	case lineComment:
		m.form.CommentLines++
		m.commentText(line, true)
		m.afterComment = true
		m.pendingDef = false
	case lineDocstring:
		m.form.CommentLines++
		m.commentText(line, false)
		if m.pendingDef {
			m.form.DocumentedFunctions++
		}
		m.afterComment, m.pendingDef = false, false
	case lineString:
		m.afterComment, m.pendingDef = false, false
	case lineAnnotation:
		// Décorateur ou attribut entre le commentaire et la définition : l'état est gardé.
	default:
		code := m.lang.stripComment(line)
		if trailing := line[len(code):]; trailing != "" {
			m.commentText(trailing, false)
		}
		if m.lang.debug != nil && m.lang.debug.MatchString(code) {
			m.form.WIPMarkers++
		}
		if m.lang.isDef(code) {
			m.form.Functions++
			if m.lang.private != nil && m.lang.private.MatchString(code) {
				m.form.PrivateDefs++
			}
			switch {
			case m.afterComment:
				m.form.DocumentedFunctions++
			case m.lang.docstring:
				m.pendingDef = true
				m.afterComment = false
				m.identifiers(code)
				return
			}
		}
		m.afterComment, m.pendingDef = false, false
		m.identifiers(code)
	}
}

// Result renvoie les mesures du commit.
func (m *FormMeter) Result() Form {
	f := m.form
	if m.ttrN > 0 {
		f.LexicalDiversity, f.HasDiversity = m.ttrSum/float64(m.ttrN), true
	}
	return f
}

// Natures d'une ligne ajoutée.
type lineKind int

const (
	lineCode       lineKind = iota
	lineComment             // commentaire (ligne ou bloc)
	lineDocstring           // ligne d'une docstring Python (comptée comme commentaire)
	lineString              // ligne d'une chaîne triple qui n'est pas une docstring
	lineAnnotation          // décorateur, annotation, attribut (« @… », « #[…] »)
)

// classify range une ligne non vide (déjà sans espaces aux bords) et met à jour l'état des blocs.
func (m *FormMeter) classify(line string) lineKind {
	l := m.lang
	if m.block != "" {
		if strings.Contains(line, m.block) {
			m.block = ""
		}
		return lineComment
	}
	if m.triple != "" {
		kind := lineString
		if m.doc {
			kind = lineDocstring
		}
		if strings.Contains(line, m.triple) {
			m.triple, m.doc = "", false
		}
		return kind
	}
	if l.docstring {
		if d := tripleStart(line); d != "" {
			// Chaîne triple en début de ligne : docstring (ou bloc de commentaire de module).
			if strings.Count(line, d) < 2 {
				m.triple, m.doc = d, true
			}
			return lineDocstring
		}
		if d := tripleInside(line); d != "" && strings.Count(line, d)%2 == 1 {
			m.triple, m.doc = d, false
			return lineCode
		}
	}
	for _, b := range l.blocks {
		if strings.HasPrefix(line, b.open) {
			if !strings.Contains(line[len(b.open):], b.close) {
				m.block = b.close
			}
			return lineComment
		}
	}
	if l.star && (line == "*" || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "*/")) {
		return lineComment // suite d'un commentaire bloc commencé hors du bloc ajouté
	}
	for _, c := range l.line {
		if strings.HasPrefix(line, c) && !(c == "#" && (strings.HasPrefix(line, "#!") || strings.HasPrefix(line, "#["))) {
			return lineComment
		}
	}
	if strings.HasPrefix(line, "@") && l.annotations || strings.HasPrefix(line, "#[") {
		return lineAnnotation
	}
	return lineCode
}

// tripleStart renvoie le délimiteur d'une chaîne triple qui commence la ligne (préfixes r, u, b, f
// permis), vide sinon.
func tripleStart(line string) string {
	s := strings.TrimLeft(line, "rRuUbBfF")
	if len(line)-len(s) > 2 {
		return ""
	}
	for _, d := range []string{`"""`, `'''`} {
		if strings.HasPrefix(s, d) {
			return d
		}
	}
	return ""
}

// tripleInside renvoie le premier délimiteur de chaîne triple de la ligne, vide s'il n'y en a pas.
func tripleInside(line string) string {
	a, b := strings.Index(line, `"""`), strings.Index(line, `'''`)
	switch {
	case a < 0 && b < 0:
		return ""
	case b < 0 || (a >= 0 && a < b):
		return `"""`
	default:
		return `'''`
	}
}

// identifiers ajoute les identifiants d'une ligne de code (sans chaînes ni commentaire de fin de ligne,
// hors mots-clés) à la fenêtre de la diversité lexicale.
func (m *FormMeter) identifiers(code string) {
	for _, id := range identifierTokens(code) {
		m.form.Identifiers++
		if len(strings.TrimLeft(id, "$")) == 1 {
			m.form.ShortIdentifiers++
		}
		m.push(id)
	}
}

// commentText mesure le texte d'un commentaire (ligne entière, docstring ou fin de ligne) : marqueurs
// de travail, mots français accentués avec ou sans accents, et, pour une ligne de commentaire entière
// (whole), décoration. Le texte n'est pas gardé.
func (m *FormMeter) commentText(text string, whole bool) {
	if wipComment.MatchString(text) {
		m.form.WIPMarkers++
	}
	if whole && isDecorative(text) {
		m.form.DecorativeLines++
	}
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r)
	}) {
		switch {
		case frAccented[w]:
			m.form.FrWords++
		case frUnaccented[w]:
			m.form.FrWords++
			m.form.FrWordsUnaccented++
		}
	}
}

// decorativeRun : au moins 8 caractères de remplissage identiques à la suite.
const decorativeRun = 8

// isDecorative indique si une ligne de commentaire est décorative : elle contient au moins decorativeRun
// caractères de remplissage identiques consécutifs (« ── », « ══ », « == », « -- », « ** », « ~~ », « ## »).
func isDecorative(line string) bool {
	run, prev := 0, rune(0)
	for _, r := range line {
		if strings.ContainsRune("─━═=-*~#", r) && r == prev {
			run++
		} else if strings.ContainsRune("─━═=-*~#", r) {
			run = 1
		} else {
			run = 0
		}
		prev = r
		if run >= decorativeRun {
			return true
		}
	}
	return false
}

// wipComment reconnaît un marqueur de travail en cours dans un commentaire.
var wipComment = regexp.MustCompile(`\b(TODO|FIXME|XXX|HACK)\b`)

// frWords sont des mots français courants qui portent un accent et dont la forme sans accent n'est pas un
// mot anglais courant (« élément » écarté : « element ») : liste fixe (phase 7.7, décision 5).
var frWords = strings.Fields(`
	données donnée accès méthode méthodes requête requêtes paramètre paramètres résultat résultats
	créé créée créés réponse réponses numéro sécurité vérifier vérifie récupère récupérer récupération
	première premier déjà été également très après problème système catégorie catégories période année
	répertoire fenêtre entrée entrées réseau génère générer modèle contrôleur évènement événement état
	étape propriété durée intégré sélectionné sélectionner supprimé supprimer affiché afficher connecté
	déconnexion mémoire écran écrit écrire réussi préférences fichier détruit liée lié liés reçu reçue
	envoyé envoyée enregistré enregistrer modifié modifier créer utilisé utilisée utilisateur utilisateurs
	connexion`)

// frAccented et frUnaccented sont les mots de frWords qui ont un accent, avec et sans leurs accents.
var frAccented, frUnaccented = func() (map[string]bool, map[string]bool) {
	acc, un := map[string]bool{}, map[string]bool{}
	strip := strings.NewReplacer("é", "e", "è", "e", "ê", "e", "ë", "e", "à", "a", "â", "a", "î", "i", "ï", "i",
		"ô", "o", "û", "u", "ù", "u", "ç", "c")
	for _, w := range frWords {
		if plain := strip.Replace(w); plain != w {
			acc[w], un[plain] = true, true
		}
	}
	return acc, un
}()

// push ajoute un identifiant à la fenêtre glissante de MATTRWindow identifiants ; chaque fenêtre pleine
// ajoute son rapport « identifiants distincts / MATTRWindow » à la moyenne (MATTR).
func (m *FormMeter) push(id string) {
	if len(m.window) == MATTRWindow {
		old := m.window[m.next]
		if m.counts[old]--; m.counts[old] == 0 {
			delete(m.counts, old)
			m.distinct--
		}
		m.window[m.next] = id
		m.next = (m.next + 1) % MATTRWindow
	} else {
		m.window = append(m.window, id)
	}
	if m.counts[id]++; m.counts[id] == 1 {
		m.distinct++
	}
	if len(m.window) == MATTRWindow {
		m.ttrSum += float64(m.distinct) / MATTRWindow
		m.ttrN++
	}
}

// identifierTokens renvoie les identifiants d'une ligne de code : hors chaînes (« " », « ' », « ` »),
// hors mots-clés courants.
func identifierTokens(code string) []string {
	var out []string
	for i := 0; i < len(code); {
		c := code[i]
		switch {
		case c == '"' || c == '\'' || c == '`':
			i = skipString(code, i)
		case isIdentStart(c):
			j := i + 1
			for j < len(code) && isIdentPart(code[j]) {
				j++
			}
			if id := code[i:j]; !keywords[strings.ToLower(id)] {
				out = append(out, id)
			}
			i = j
		case c >= '0' && c <= '9':
			for i < len(code) && isIdentPart(code[i]) {
				i++
			}
		default:
			i++
		}
	}
	return out
}

// skipString renvoie l'indice qui suit la chaîne commencée en code[i] (fin de ligne si non fermée).
func skipString(code string, i int) int {
	q := code[i]
	for j := i + 1; j < len(code); j++ {
		switch code[j] {
		case '\\':
			j++
		case q:
			return j + 1
		}
	}
	return len(code)
}

func isIdentStart(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool { return isIdentStart(c) || (c >= '0' && c <= '9') }

// language décrit la syntaxe des commentaires et des définitions d'un langage.
type language struct {
	line        []string       // débuts de commentaire de ligne
	blocks      []block        // commentaires blocs
	star        bool           // « * » en début de ligne : suite d'un commentaire bloc
	annotations bool           // « @… » en début de ligne : décorateur ou annotation
	docstring   bool           // Python : docstrings et chaînes triples
	def         *regexp.Regexp // définition de fonction ou de méthode ; nil : non reconnue
	private     *regexp.Regexp // définition privée (parmi def) ; nil : non reconnue
	debug       *regexp.Regexp // appel de debug laissé dans le code ; nil : non reconnu
}

// block est un commentaire bloc.
type block struct{ open, close string }

// controlWords commencent une instruction de contrôle, jamais une définition (« if (x) { »).
var controlWords = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true,
	"else": true, "return": true, "do": true, "try": true, "foreach": true, "elseif": true, "with": true,
	"synchronized": true, "using": true, "lock": true, "new": true, "throw": true, "await": true}

// isDef indique si la ligne de code est une définition de fonction ou de méthode.
func (l *language) isDef(code string) bool {
	if l.def == nil || !l.def.MatchString(code) {
		return false
	}
	end := 0
	for end < len(code) && isIdentPart(code[end]) {
		end++
	}
	return !controlWords[code[:end]]
}

// stripComment coupe le commentaire de fin de ligne d'une ligne de code (hors chaînes).
func (l *language) stripComment(code string) string {
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c == '"' || c == '\'' || c == '`' {
			i = skipString(code, i) - 1
			continue
		}
		for _, lc := range l.line {
			if strings.HasPrefix(code[i:], lc) && !(lc == "#" && strings.HasPrefix(code[i:], "#[")) {
				return code[:i]
			}
		}
		for _, b := range l.blocks {
			if strings.HasPrefix(code[i:], b.open) {
				return code[:i]
			}
		}
	}
	return code
}

var (
	cBlock    = block{"/*", "*/"}
	htmlBlock = block{"<!--", "-->"}
	twigBlock = block{"{#", "#}"}

	defGo     = regexp.MustCompile(`^func\b`)
	defPython = regexp.MustCompile(`^(async\s+)?def\s+\w+`)
	defJS     = regexp.MustCompile(`^(export\s+)?(default\s+)?(async\s+)?function\b|` +
		`^(export\s+)?(const|let|var)\s+[\w$]+\s*=\s*(async\s+)?(\([^)]*\)|[\w$]+)\s*=>|` +
		`^((public|private|protected|static|async|readonly|override)\s+)*#?[\w$]+\s*\([^)]*\)\s*(:\s*[^{]+)?\{\s*$`)
	defPHP  = regexp.MustCompile(`^((public|private|protected|static|abstract|final)\s+)*function\s+&?\w+`)
	defJava = regexp.MustCompile(`^((public|private|protected|internal|static|final|abstract|override|virtual|async|synchronized|open|suspend)\s+)+` +
		`[\w<>\[\],.?\s]*\b\w+\s*\([^;]*$|^(fun|fn|func|def)\s+[\w<>]+`)
	defRuby = regexp.MustCompile(`^def\s+[\w.?!]+`)

	privJS     = regexp.MustCompile(`^(static\s+)?(async\s+)?#[\w$]+\s*\(`)
	privPHP    = regexp.MustCompile(`^((static|abstract|final)\s+)*private\b`)
	privJava   = regexp.MustCompile(`^((static|final|abstract|override|open|suspend|async)\s+)*private\b`)
	privPython = regexp.MustCompile(`^(async\s+)?def\s+_\w*`)
	dbgJS      = regexp.MustCompile(`\bconsole\.(log|debug|dir|table)\s*\(|^debugger\b`)
	dbgPHP     = regexp.MustCompile(`\b(var_dump|print_r|dd|dump|var_export)\s*\(`)
	dbgJava    = regexp.MustCompile(`\bSystem\.(out|err)\.print(ln)?\s*\(|\bprintStackTrace\s*\(`)
	dbgPython  = regexp.MustCompile(`^print\s*\(|\bbreakpoint\s*\(|\bpdb\.set_trace\s*\(`)

	langC      = &language{line: []string{"//"}, blocks: []block{cBlock}, star: true, annotations: true}
	langGo     = &language{line: []string{"//"}, blocks: []block{cBlock}, star: true, def: defGo}
	langJS     = &language{line: []string{"//"}, blocks: []block{cBlock, htmlBlock}, star: true, annotations: true, def: defJS, private: privJS, debug: dbgJS}
	langPHP    = &language{line: []string{"//", "#"}, blocks: []block{cBlock}, star: true, def: defPHP, private: privPHP, debug: dbgPHP}
	langJava   = &language{line: []string{"//"}, blocks: []block{cBlock}, star: true, annotations: true, def: defJava, private: privJava, debug: dbgJava}
	langPython = &language{line: []string{"#"}, annotations: true, docstring: true, def: defPython, private: privPython, debug: dbgPython}
	langRuby   = &language{line: []string{"#"}, def: defRuby}
	langHash   = &language{line: []string{"#"}}
	langDash   = &language{line: []string{"--"}, blocks: []block{cBlock}}
	langHTML   = &language{blocks: []block{htmlBlock, twigBlock}}
	langCSS    = &language{blocks: []block{cBlock}, star: true}
)

// languages associe une extension (en minuscules) à sa syntaxe : fichiers source reconnus.
var languages = map[string]*language{
	".go": langGo,
	".js": langJS, ".jsx": langJS, ".mjs": langJS, ".cjs": langJS, ".ts": langJS, ".tsx": langJS,
	".vue": langJS, ".svelte": langJS,
	".php":  langPHP,
	".java": langJava, ".kt": langJava, ".kts": langJava, ".cs": langJava, ".scala": langJava,
	".swift": langJava, ".rs": langJava, ".dart": langJava,
	".c": langC, ".h": langC, ".cc": langC, ".cpp": langC, ".hpp": langC, ".cxx": langC,
	".py": langPython,
	".rb": langRuby,
	".sh": langHash, ".bash": langHash, ".zsh": langHash, ".r": langHash, ".pl": langHash, ".ps1": langHash,
	".sql": langDash, ".lua": langDash, ".hs": langDash,
	".html": langHTML, ".htm": langHTML, ".twig": langHTML, ".xml": langHTML,
	".css": langCSS, ".scss": langCSS, ".less": langCSS,
}

// languageOf renvoie la syntaxe du fichier p, nil s'il n'est pas un fichier source reconnu.
func languageOf(p string) *language {
	return languages[strings.ToLower(path.Ext(p))]
}

// testDirs sont les répertoires de tests (comparés sans casse).
var testDirs = map[string]bool{"test": true, "tests": true, "__tests__": true, "spec": true, "specs": true,
	"testing": true}

// testName reconnaît un nom de fichier de test.
var testName = regexp.MustCompile(`(?i)(_test\.go|^test_.*\.py|_test\.py|\.(test|spec)\.[a-z]+|` +
	`(Test|Tests|IT)\.(java|kt|php|cs|scala|swift|dart)|_spec\.rb)$`)

// isTestPath indique si le fichier source p est un fichier de test (nom ou répertoire).
func isTestPath(p string) bool {
	if testName.MatchString(path.Base(p)) {
		return true
	}
	segs := strings.Split(p, "/")
	for _, s := range segs[:len(segs)-1] {
		if testDirs[strings.ToLower(s)] {
			return true
		}
	}
	return false
}

// keywords sont les mots-clés courants des langages reconnus (en minuscules), écartés des identifiants
// (F3) : la diversité lexicale porte sur les noms choisis par l'auteur.
var keywords = func() map[string]bool {
	m := map[string]bool{}
	for _, k := range strings.Fields(`
		abstract and as assert async await break case catch class const continue def default defer del
		delete do echo elif else elseif end enum except export extends false final finally fn for foreach
		from func function global go goto if impl implements import in instanceof interface is lambda let
		loop match mod module mut namespace new nil none nonlocal not null of or override package pass pub
		private protected public raise range readonly return select self static struct super switch then
		this throw throws trait true try type typeof undefined unless until use using val var void where
		while with yield int string bool boolean float double char long short byte void any number object
		array list dict map set str`) {
		m[k] = true
	}
	return m
}()

// MessageStyle sont les mesures du style d'un message de commit (F4), sans le message.
type MessageStyle struct {
	Length       int  // caractères du message, hors trailers et signatures d'outils
	Bullets      int  // lignes en liste à puces ou numérotée
	Conventional bool // sujet au format Conventional Commits (« feat: … », « fix(x): … »)
	Revert       bool // message de « git revert » (« Revert "…" ») (A4)
	Fix          bool // sujet de correction (« fix … », « corrige … », « oups ») (H4)
}

var (
	bulletLine   = regexp.MustCompile(`^([-*•+]|\d+[.)])\s+\S`)
	conventional = regexp.MustCompile(`(?i)^(feat|fix|docs|style|refactor|perf|test|tests|build|ci|chore|revert)(\([^)]*\))?!?: \S`)
	trailerLine  = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9-]*: \S`)
	fixSubject   = regexp.MustCompile(`(?i)^(fix|fixe[sd]?|fixing|hotfix|bugfix|corrige[rs]?|corrigé|correction|correctif|oups|oops)\b`)
)

// MeasureMessage mesure le style d'un message de commit ; le message n'est lu qu'en mémoire.
func MeasureMessage(message string) MessageStyle {
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(message), "\r\n", "\n"), "\n")
	// Dernier paragraphe fait seulement de trailers (« Co-authored-by: … », « Signed-off-by: … ») : retiré.
	last := len(lines)
	for last > 0 && strings.TrimSpace(lines[last-1]) != "" {
		last--
	}
	if last > 0 {
		allTrailers := true
		for _, l := range lines[last:] {
			if !trailerLine.MatchString(strings.TrimSpace(l)) {
				allTrailers = false
				break
			}
		}
		if allTrailers {
			lines = lines[:last]
		}
	}
	var s MessageStyle
	var kept []string
	for i, l := range lines {
		t := strings.TrimSpace(l)
		signature := false
		for _, p := range signaturePatterns {
			if p.re.MatchString(t) {
				signature = true
				break
			}
		}
		if signature {
			continue
		}
		if i > 0 && bulletLine.MatchString(t) {
			s.Bullets++
		}
		kept = append(kept, l)
	}
	s.Length = utf8.RuneCountInString(strings.TrimSpace(strings.Join(kept, "\n")))
	if len(lines) > 0 {
		subject := strings.TrimSpace(lines[0])
		s.Conventional = conventional.MatchString(subject)
		s.Revert = strings.HasPrefix(subject, `Revert "`)
		s.Fix = fixSubject.MatchString(subject)
	}
	return s
}
