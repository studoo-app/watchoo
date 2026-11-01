// Package repoimport analyse les références de dépôts saisies par l'utilisateur : argument de
// « watchoo repo add » / « watchoo sync » / option --repo, liste --branch, et fichier de
// « watchoo repo import » (documentation/fr/05-cli.md, « Import groupé »).
//
// Package pur : il lit un io.Reader et valide la syntaxe, sans accès réseau ni base. La vérification
// d'accès et l'enregistrement sont faits par internal/sync (Engine.AddRepositories).
package repoimport

import (
	"bufio"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/provider"
)

// expectedRef rappelle les formes acceptées dans les messages d'erreur.
const expectedRef = "attendu owner/repo ou https://github.com/owner/repo"

// maxQuoted borne la longueur (en caractères) d'une valeur citée dans un message d'erreur.
const maxQuoted = 80

// maxLineBytes borne la longueur d'une ligne du fichier d'import (garde-fou mémoire).
const maxLineBytes = 64 * 1024

// MaxLabelRunes est la longueur maximale d'un label, en caractères.
const MaxLabelRunes = 200

// MaxThresholdDays borne un seuil d'activité propre à un dépôt, en jours (dix ans).
const MaxThresholdDays = 3650

// Ref est une référence de dépôt validée (owner et nom valides selon provider.ValidateRepo).
type Ref struct {
	Owner string
	Name  string
}

// String renvoie « owner/name ».
func (r Ref) String() string { return r.Owner + "/" + r.Name }

// key renvoie la clé de comparaison sans casse, comme sur GitHub.
func (r Ref) key() string { return strings.ToLower(r.String()) }

// ParseRef analyse « owner/repo » ou une URL GitHub « https://github.com/owner/repo », avec ou sans
// « .git » final ni « / » final (http://, www.github.com et la casse de l'hôte acceptés ; autre hôte,
// chemin plus long, paramètres ou fragment refusés). Espaces autour ignorés. Erreur en français qui
// cite la valeur reçue et la forme attendue (elle enveloppe provider.ErrInvalidRef si owner ou nom est
// invalide).
func ParseRef(s string) (Ref, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Ref{}, fmt.Errorf("dépôt vide : %s", expectedRef)
	}
	p := raw
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			// Valeur non citée : une URL illisible peut contenir des identifiants.
			return Ref{}, fmt.Errorf("dépôt : URL illisible (valeur non reproduite), %s", expectedRef)
		}
		scheme := strings.ToLower(u.Scheme)
		host := strings.ToLower(u.Host)
		switch {
		case u.User != nil:
			// La valeur n'est pas citée : elle pourrait contenir un mot de passe ou un token.
			return Ref{}, fmt.Errorf("dépôt : identifiants interdits dans l'URL (valeur non reproduite), %s", expectedRef)
		case scheme != "https" && scheme != "http":
			return Ref{}, fmt.Errorf("dépôt %s : schéma %q non pris en charge, %s", quote(raw), u.Scheme, expectedRef)
		case host != "github.com" && host != "www.github.com":
			return Ref{}, fmt.Errorf("dépôt %s : seul l'hôte github.com est pris en charge, %s", quote(raw), expectedRef)
		case u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#"):
			return Ref{}, fmt.Errorf("dépôt %s : paramètres et fragment interdits dans l'URL, %s", quote(raw), expectedRef)
		}
		p = strings.TrimPrefix(u.Path, "/")
	}
	p = strings.TrimSuffix(p, "/")
	p = strings.TrimSuffix(p, ".git")
	parts := strings.Split(p, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Ref{}, fmt.Errorf("dépôt %s invalide : %s", quote(raw), expectedRef)
	}
	if err := provider.ValidateRepo(parts[0], parts[1]); err != nil {
		return Ref{}, fmt.Errorf("dépôt %s (%s) : %w", quote(raw), expectedRef, err)
	}
	return Ref{Owner: parts[0], Name: parts[1]}, nil
}

// ParseBranchPatterns analyse une liste --branch ou une cellule branches : noms et motifs séparés par
// des virgules (« main,dev,release/* »), espaces autour ignorés, valeurs vides et doublons retirés
// (ordre conservé). Chaque motif doit être valide pour path.Match (sinon erreur qui cite le motif).
// Chaîne vide (ou seulement des virgules et espaces) : nil, soit toutes les branches.
//
// Un motif doit aussi être un nom de branche acceptable (provider.ValidateBranch : pas de caractère de
// contrôle, longueur bornée).
func ParseBranchPatterns(s string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p == "" || seen[p] {
			continue
		}
		if err := provider.ValidateBranch(p); err != nil {
			return nil, fmt.Errorf("motif de branche %s : %w", quote(p), err)
		}
		if _, err := path.Match(p, ""); err != nil {
			return nil, fmt.Errorf("motif de branche %s invalide (syntaxe de motif : *, ?, [a-z]) : %w", quote(p), err)
		}
		seen[p] = true
		out = append(out, p)
	}
	return out, nil
}

// Entry est une ligne valide d'un fichier d'import.
type Entry struct {
	Line int // numéro de ligne dans le fichier (1 = première ligne)
	Ref  Ref // dépôt
	// Label est le nom affiché donné par le fichier ; vide = non fourni (le label existant est gardé).
	Label string
	// Branches sont les motifs donnés par le fichier (ParseBranchPatterns) ; nil = non fourni (les
	// motifs existants sont gardés). Une cellule vide ne remet donc pas « toutes les branches ».
	Branches []string
	// FlemmeAfterDays et DeadAfterDays sont les seuils d'activité du dépôt (ParseThreshold) ; nil =
	// non fourni (seuils existants gardés), 0 = revenir à la valeur globale.
	FlemmeAfterDays *int
	DeadAfterDays   *int
}

// Format est la forme reconnue d'un fichier d'import.
type Format string

// Valeurs de Format.
const (
	FormatCSV  Format = "csv"  // CSV avec en-tête (colonne repo obligatoire, label, branches, flemme et dead facultatives)
	FormatList Format = "list" // un dépôt par ligne, sans en-tête
)

// LineError est une erreur sur une ligne du fichier.
type LineError struct {
	Line int    // numéro de ligne (1 = première ligne)
	Msg  string // message en français, sans le préfixe « ligne N »
}

// Error renvoie « ligne N : message ».
func (e LineError) Error() string { return fmt.Sprintf("ligne %d : %s", e.Line, e.Msg) }

// ParseError regroupe toutes les erreurs de lignes d'un fichier (au moins une), dans l'ordre.
type ParseError struct {
	Errors []LineError
}

// Error renvoie les erreurs, une par ligne de texte, précédées de leur nombre.
func (e *ParseError) Error() string {
	var b strings.Builder
	if len(e.Errors) == 1 {
		b.WriteString("1 ligne en erreur :")
	} else {
		fmt.Fprintf(&b, "%d lignes en erreur :", len(e.Errors))
	}
	for _, le := range e.Errors {
		b.WriteString("\n  ")
		b.WriteString(le.Error())
	}
	return b.String()
}

// ErrNoEntry est l'erreur d'un fichier qui ne contient aucun dépôt (enveloppée dans la *ParseError,
// testable avec errors.Is).
var ErrNoEntry = errors.New("aucun dépôt dans le fichier")

// Is permet errors.Is(err, ErrNoEntry) sur la *ParseError d'un fichier vide.
func (e *ParseError) Is(target error) bool {
	return target == ErrNoEntry && len(e.Errors) == 1 && e.Errors[0].Msg == ErrNoEntry.Error()
}

// column est une colonne connue du CSV.
type column int

const (
	colRepo column = iota
	colLabel
	colBranches
	colFlemme
	colDead
)

// columnNames associe les noms d'en-tête (en minuscules) aux colonnes.
var columnNames = map[string]column{
	"repo": colRepo, "label": colLabel, "branches": colBranches, "flemme": colFlemme, "dead": colDead,
}

// line est une ligne retenue du fichier (ni vide ni commentaire).
type line struct {
	num  int
	text string
}

// Parse lit tout le fichier r et renvoie ses entrées dans l'ordre, ou une *ParseError listant TOUTES
// les lignes en erreur (rien n'est alors à importer), ou une erreur de lecture enveloppée.
//
// Règles :
//   - BOM UTF-8 initial ignoré ; lignes vides (ou d'espaces) et lignes dont le premier caractère non
//     blanc est « # » ignorées, dans les deux formats ;
//   - format CSV si la première ligne retenue, découpée en CSV, a pour cellules (casse et espaces
//     ignorés) des noms de colonnes parmi repo, label, branches, flemme, dead, dont repo ; colonne inconnue ou en
//     double : erreur sur la ligne d'en-tête ; nombre de cellules différent de l'en-tête : erreur ;
//     repo vide : erreur ;
//   - sinon format liste : une référence par ligne (ParseRef), rien d'autre sur la ligne ;
//   - même dépôt deux fois (owner/nom sans tenir compte de la casse) : erreur sur la seconde ligne,
//     qui cite la première ;
//   - fichier sans aucune entrée : erreur « aucun dépôt dans le fichier ».
//
// Chaque ligne du fichier est un enregistrement : un champ CSV entre guillemets ne peut pas s'étendre
// sur plusieurs lignes.
func Parse(r io.Reader) ([]Entry, Format, error) {
	lines, err := readLines(r)
	if err != nil {
		return nil, "", err
	}
	if len(lines) == 0 {
		return nil, "", &ParseError{Errors: []LineError{{Line: 1, Msg: ErrNoEntry.Error()}}}
	}

	p := &parser{first: make(map[string]int)}
	format := FormatList
	if cols, isHeader, msg := parseHeader(lines[0].text); isHeader {
		format = FormatCSV
		if msg != "" {
			p.fail(lines[0].num, msg)
		}
		for _, l := range lines[1:] {
			p.csvLine(l, cols)
		}
	} else {
		for _, l := range lines {
			p.listLine(l)
		}
	}

	if len(p.errs) == 0 && len(p.entries) == 0 {
		p.fail(lines[len(lines)-1].num, ErrNoEntry.Error())
	}
	if len(p.errs) > 0 {
		return nil, format, &ParseError{Errors: p.errs}
	}
	return p.entries, format, nil
}

// readLines lit r et renvoie les lignes retenues (ni vides ni commentaires), avec leur numéro.
func readLines(r io.Reader) ([]line, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 4096), maxLineBytes)
	var out []line
	for num := 1; sc.Scan(); num++ {
		text := sc.Text()
		if num == 1 {
			text = strings.TrimPrefix(text, "\ufeff")
		}
		text = strings.TrimSuffix(text, "\r")
		trimmed := strings.TrimSpace(text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out = append(out, line{num: num, text: text})
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return nil, fmt.Errorf("lecture du fichier d'import : ligne de plus de %d octets", maxLineBytes)
		}
		return nil, fmt.Errorf("lecture du fichier d'import : %w", err)
	}
	return out, nil
}

// splitCSV découpe une ligne en cellules CSV (guillemets « " » pris en charge), espaces autour retirés.
func splitCSV(text string) ([]string, error) {
	cr := csv.NewReader(strings.NewReader(text))
	cr.FieldsPerRecord = -1
	cr.TrimLeadingSpace = true
	rec, err := cr.Read()
	if err != nil {
		return nil, err
	}
	for i := range rec {
		rec[i] = strings.TrimSpace(rec[i])
	}
	return rec, nil
}

// parseHeader dit si text est une ligne d'en-tête CSV (au moins une cellule « repo »). Si oui, il
// renvoie l'ordre des colonnes et, le cas échéant, le message d'erreur de l'en-tête.
func parseHeader(text string) (cols []column, isHeader bool, msg string) {
	cells, err := splitCSV(text)
	if err != nil {
		return nil, false, ""
	}
	hasRepo := false
	for _, c := range cells {
		if strings.EqualFold(c, "repo") {
			hasRepo = true
		}
	}
	if !hasRepo {
		return nil, false, ""
	}
	seen := make(map[column]bool)
	for _, c := range cells {
		col, ok := columnNames[strings.ToLower(c)]
		switch {
		case !ok:
			return nil, true, fmt.Sprintf("colonne %s inconnue dans l'en-tête (colonnes possibles : repo, label, branches, flemme, dead)", quote(c))
		case seen[col]:
			return nil, true, fmt.Sprintf("colonne %s en double dans l'en-tête", quote(strings.ToLower(c)))
		}
		seen[col] = true
		cols = append(cols, col)
	}
	return cols, true, ""
}

// parser accumule les entrées et les erreurs d'un fichier.
type parser struct {
	entries []Entry
	errs    []LineError
	first   map[string]int // clé du dépôt → ligne de sa première apparition
}

// fail enregistre une erreur sur la ligne num.
func (p *parser) fail(num int, msg string) {
	p.errs = append(p.errs, LineError{Line: num, Msg: msg})
}

// add enregistre une entrée, ou une erreur si le dépôt est déjà apparu.
func (p *parser) add(e Entry) {
	if prev, ok := p.first[e.Ref.key()]; ok {
		p.fail(e.Line, fmt.Sprintf("dépôt %s en double (déjà présent ligne %d)", e.Ref, prev))
		return
	}
	p.first[e.Ref.key()] = e.Line
	p.entries = append(p.entries, e)
}

// listLine analyse une ligne du format liste.
func (p *parser) listLine(l line) {
	text := strings.TrimSpace(l.text)
	if strings.ContainsAny(text, ", \t;") {
		p.fail(l.num, fmt.Sprintf("un seul dépôt par ligne attendu, reçu %s (pour un CSV, commencer par "+
			"l'en-tête repo,label,branches)", quote(text)))
		return
	}
	ref, err := ParseRef(text)
	if err != nil {
		p.fail(l.num, err.Error())
		return
	}
	p.add(Entry{Line: l.num, Ref: ref})
}

// csvLine analyse une ligne de données du format CSV ; cols est l'ordre des colonnes de l'en-tête
// (nil si l'en-tête est en erreur : les lignes sont alors seulement découpées).
func (p *parser) csvLine(l line, cols []column) {
	cells, err := splitCSV(l.text)
	if err != nil {
		p.fail(l.num, "CSV mal formé (guillemets à vérifier)")
		return
	}
	if cols == nil {
		return
	}
	if len(cells) != len(cols) {
		p.fail(l.num, fmt.Sprintf("%d cellule(s) au lieu de %d (comme l'en-tête)", len(cells), len(cols)))
		return
	}
	e := Entry{Line: l.num}
	var msgs []string
	for i, col := range cols {
		v := cells[i]
		switch col {
		case colRepo:
			if v == "" {
				msgs = append(msgs, "colonne repo vide")
				continue
			}
			ref, err := ParseRef(v)
			if err != nil {
				msgs = append(msgs, err.Error())
				continue
			}
			e.Ref = ref
		case colLabel:
			if msg := checkLabel(v); msg != "" {
				msgs = append(msgs, msg)
				continue
			}
			e.Label = v
		case colBranches:
			pats, err := ParseBranchPatterns(v)
			if err != nil {
				msgs = append(msgs, err.Error())
				continue
			}
			e.Branches = pats
		case colFlemme, colDead:
			name := "flemme"
			if col == colDead {
				name = "dead"
			}
			n, err := ParseThreshold(v)
			if err != nil {
				msgs = append(msgs, name+" : "+err.Error())
				continue
			}
			if col == colFlemme {
				e.FlemmeAfterDays = n
			} else {
				e.DeadAfterDays = n
			}
		}
	}
	if err := ValidateThresholds(e.FlemmeAfterDays, e.DeadAfterDays); err != nil && len(msgs) == 0 {
		msgs = append(msgs, err.Error())
	}
	if len(msgs) > 0 {
		p.fail(l.num, strings.Join(msgs, " ; "))
		return
	}
	p.add(e)
}

// ValidateLabel vérifie un label (nom affiché d'un dépôt), quelle que soit sa source (fichier
// d'import, « repo add --label », plus tard le web) : UTF-8 valide, aucun caractère de contrôle (ESC,
// saut de ligne…), MaxLabelRunes caractères au plus. Erreur en français sinon.
func ValidateLabel(v string) error {
	if msg := checkLabel(v); msg != "" {
		return errors.New(msg)
	}
	return nil
}

// checkLabel renvoie le message d'erreur d'un label invalide, ou "" s'il est acceptable.
func checkLabel(v string) string {
	switch {
	case !utf8.ValidString(v):
		return "label : texte UTF-8 invalide"
	case strings.IndexFunc(v, unicode.IsControl) >= 0:
		return "label : caractère de contrôle interdit"
	case utf8.RuneCountInString(v) > MaxLabelRunes:
		return fmt.Sprintf("label trop long (%d caractères au plus)", MaxLabelRunes)
	}
	return ""
}

// ParseThreshold analyse un seuil d'activité saisi (cellule flemme ou dead, champ du web) : nombre
// entier de jours, espaces autour ignorés. Chaîne vide : nil (non fourni) ; « 0 » : revenir à la
// valeur globale. Erreur en français sinon (nombre négatif ou illisible).
func ParseThreshold(s string) (*int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 || n > MaxThresholdDays {
		return nil, fmt.Errorf("%s : nombre entier de jours attendu, de 1 à %d (0 = valeur globale)", quote(s), MaxThresholdDays)
	}
	return &n, nil
}

// ValidateThresholds vérifie les seuils d'activité demandés pour un dépôt, quelle que soit leur source
// (« repo add --flemme / --dead », fichier d'import, partage de la configuration, web) : nil = non
// fourni, 0 = valeur globale, sinon de 1 à MaxThresholdDays jours ; quand les deux sont donnés (non
// nuls), Dead doit être strictement supérieur à Flemme. Erreur en français sinon.
func ValidateThresholds(flemme, dead *int) error {
	for _, t := range []struct {
		name string
		v    *int
	}{{"flemme", flemme}, {"dead", dead}} {
		if t.v != nil && (*t.v < 0 || *t.v > MaxThresholdDays) {
			return fmt.Errorf("seuil %s : %d jours invalide (de 1 à %d, ou 0 pour la valeur globale)", t.name, *t.v, MaxThresholdDays)
		}
	}
	if flemme != nil && dead != nil && *flemme > 0 && *dead > 0 && *dead <= *flemme {
		return fmt.Errorf("seuil dead (%d j) : doit être strictement supérieur au seuil flemme (%d j)", *dead, *flemme)
	}
	return nil
}

// quote cite une valeur entre guillemets, tronquée à maxQuoted caractères.
func quote(s string) string {
	if utf8.RuneCountInString(s) > maxQuoted {
		s = string([]rune(s)[:maxQuoted]) + "…"
	}
	return fmt.Sprintf("%q", s)
}
