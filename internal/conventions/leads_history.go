package conventions

// Pistes de l'historique (phase 8.7, documentation/fr/13-conventions.md, sections 5.7 et 5.8) : fraîcheur
// de la documentation (DOC9), dates déclarées dans l'INDEX (DOC10) et effet de l'outillage (TL5). Elles
// demandent l'arbre de la tête et l'historique du clone au moment de la lecture : elles ne sont calculées
// que dans la vue détaillée d'un dépôt, jamais mises en cache. Les fonctions de ce fichier sont pures : les
// chemins, les cellules de l'INDEX et les dates de commit ne restent qu'en mémoire ; une piste ne garde que
// des codes, des compteurs et des dates.

import (
	"context"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// HistoryLeadCriteria liste les pistes calculées à la lecture sur l'historique du clone.
var HistoryLeadCriteria = []string{CriterionDOC9, CriterionDOC10, CriterionTL5}

// Raisons propres aux pistes de l'historique (clés --json), en anglais.
const (
	UnknownNoTooling    = "no_tooling"     // TL5 : ni TL1, ni TL2, ni TL4 trouvé (état LeadNotApplicable)
	UnknownNoDateColumn = "no_date_column" // DOC10 : aucun tableau de l'INDEX avec colonnes document et date
)

// Clés des compteurs des pistes de l'historique (clés --json).
const (
	CountCommitsSince  = "commits_since"  // DOC9 : commits hors documentation depuis son dernier commit
	CountDaysBehind    = "days_behind"    // DOC9 : jours entre le dernier commit de documentation et celui du code
	CountCompared      = "compared"       // DOC10 : lignes comparées au dernier commit du document
	CountUnresolved    = "unresolved"     // DOC10 : lignes sans document visé lisible dans l'arbre
	CountOverLimit     = "over_limit"     // DOC10 : lignes au-delà de DOC10MaxDocuments, non comparées
	CountBefore        = "before"         // TL5 : commits évalués avant l'introduction
	CountBeforeConform = "before_conform" // TL5 : dont conformes
	CountAfter         = "after"          // TL5 : commits évalués depuis l'introduction
	CountAfterConform  = "after_conform"  // TL5 : dont conformes
	CountBeforePercent = "before_percent" // TL5 : taux CC1 avant (arrondi inférieur)
	CountAfterPercent  = "after_percent"  // TL5 : taux CC1 après (arrondi inférieur)
	// CountSources (« sources ») compte aussi, pour TL5, les fichiers d'outillage datés par leur premier ajout.
	CountUndatedSources = "undated_sources" // TL5 : outillage déclaré dans un fichier partagé (package.json…), non daté
)

// Clés des dates des pistes de l'historique (clés --json).
const (
	DateDocs       = "docs"       // DOC9 : dernier commit sur le dossier de documentation
	DateIndex      = "index"      // DOC9 : dernier commit sur l'INDEX
	DateChangelog  = "changelog"  // DOC9 : dernier commit sur le CHANGELOG
	DateCode       = "code"       // DOC9 : dernier commit hors documentation
	DateIntroduced = "introduced" // TL5 : plus ancienne introduction de l'outillage
)

// AllLeadDates liste les clés de dates des pistes, dans leur ordre d'affichage.
var AllLeadDates = []string{DateDocs, DateIndex, DateChangelog, DateCode, DateIntroduced}

// Seuils des pistes de l'historique (documentation/fr/13-conventions.md, section 11.1 : à valider sur des
// dépôts réels).
const (
	DOC9MinCommits    = 20 // commits hors documentation depuis son dernier commit : à relire
	DOC9MaxDays       = 30 // jours de retard de la documentation sur le code : à relire
	DOC10MaxDocuments = 50 // lignes de l'INDEX comparées au plus
	DOC10OlderDays    = 30 // date déclarée plus ancienne que le dernier commit de plus de ce nombre de jours
	TL5MinCommits     = 10 // effectif minimal de chaque côté de l'introduction
)

// LeadHistoryReader lit l'historique du clone Git local existant pour les pistes, sans le mettre à jour
// (phase 8.7) ; le provider des clones l'implémente. Facultatif : un CloneReader qui ne l'implémente pas
// rend DOC9, DOC10 et TL5 inconnus, comme sans clone. Les chemins sont relatifs à la racine, pris à la
// lettre ; aucun n'apparaît dans les erreurs.
type LeadHistoryReader interface {
	// LastCommit renvoie le dernier commit de l'historique de head qui a modifié l'un des chemins include
	// (fichiers ou dossiers ; vide : tout l'arbre) hors des chemins exclude, et sa date en UTC ; found
	// faux si aucun.
	LastCommit(ctx context.Context, owner, name, head string, include, exclude []string) (sha string, at time.Time,
		found bool, err error)
	// CountCommits compte les commits de head, pas de since, qui ont modifié include (vide : tout l'arbre)
	// hors de exclude.
	CountCommits(ctx context.Context, owner, name, head, since string, include, exclude []string) (int, error)
	// FirstAdd renvoie le plus ancien commit de head qui a ajouté file, et sa date en UTC ; found faux si
	// aucun.
	FirstAdd(ctx context.Context, owner, name, head, file string) (sha string, at time.Time, found bool, err error)
}

// withDates renvoie la piste avec ses dates (nil si aucune).
func (l *leadSet) withDates(dates map[string]time.Time) LeadCheck {
	lc := l.done()
	if len(dates) > 0 {
		lc.Dates = dates
	}
	return lc
}

// --- DOC9 : fraîcheur de la documentation ---

// docPaths sont les chemins de documentation de DOC9 dans l'arbre de la tête ; vides s'ils sont absents.
type docPaths struct {
	Docs      string // dossier de documentation évalué (celui de l'INDEX trouvé, sinon docs.dir)
	Index     string // INDEX trouvé (même recherche que DOC1)
	Changelog string // CHANGELOG trouvé (même recherche que CL1)
	Readme    string // README trouvé (même recherche que RD1)
}

// all renvoie les chemins de documentation présents, sans doublon : ils sont exclus du « code ».
func (d docPaths) all() []string {
	var out []string
	for _, p := range []string{d.Docs, d.Index, d.Changelog, d.Readme} {
		if p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// docPathsOf cherche les chemins de documentation de l'arbre ; la raison d'un arbre inutilisable sinon.
func docPathsOf(tree FileTree, rules FileRules) (docPaths, string) {
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return docPaths{}, reason
	}
	rules = rules.normalized()
	index := findIndex(idx, rules)
	var d docPaths
	if root := docsRoot(index, rules); idx.dirs[root] {
		d.Docs = root
	}
	d.Index = index.path
	d.Changelog = findChangelog(idx).path
	d.Readme = findReadme(idx).path
	return d, ""
}

// DocStamps sont les dates lues dans l'historique pour DOC9 ; zéro pour un chemin absent ou jamais modifié.
type DocStamps struct {
	Docs, Index, Changelog time.Time
	Code                   time.Time // dernier commit hors documentation (dossier, INDEX, CHANGELOG, README)
	LastDoc                time.Time // dernier commit de documentation (l'un de ces chemins)
	CommitsSince           int       // commits hors documentation depuis LastDoc
}

// CheckDOC9 : la documentation est à relire quand au moins DOC9MinCommits commits hors documentation ont
// suivi son dernier commit, ou quand le dernier commit du code le suit de DOC9MaxDays jours ou plus.
// Sans aucun commit de documentation : inconnu (no_file).
func CheckDOC9(s DocStamps) LeadCheck {
	if s.LastDoc.IsZero() {
		return unknownLead(CriterionDOC9, UnknownNoFile)
	}
	l := newLead(CriterionDOC9)
	days := 0
	if s.Code.After(s.LastDoc) {
		days = int(s.Code.Sub(s.LastDoc) / (24 * time.Hour))
	}
	l.counts[CountCommitsSince], l.counts[CountDaysBehind] = s.CommitsSince, days
	if s.CommitsSince >= DOC9MinCommits || days >= DOC9MaxDays {
		l.add(LeadDocsBehind)
	}
	dates := map[string]time.Time{}
	for key, at := range map[string]time.Time{DateDocs: s.Docs, DateIndex: s.Index, DateChangelog: s.Changelog,
		DateCode: s.Code} {
		if !at.IsZero() {
			dates[key] = at.UTC()
		}
	}
	return l.withDates(dates)
}

// --- DOC10 : dates déclarées dans l'INDEX ---

var (
	cellISODateRe = regexp.MustCompile(`(?:^|[^0-9])(\d{4})[-/.](\d{1,2})[-/.](\d{1,2})(?:[^0-9]|$)`)
	cellFRDateRe  = regexp.MustCompile(`(?:^|[^0-9])(\d{1,2})[-/.](\d{1,2})[-/.](\d{4})(?:[^0-9]|$)`)
)

// declaredDate lit une date dans une cellule de l'INDEX : AAAA-MM-JJ (ou avec « / » ou « . »), sinon
// JJ/MM/AAAA (ou avec « - » ou « . ») ; faux si aucune date valide.
func declaredDate(cell string) (time.Time, bool) {
	ymd := func(y, m, d string) (time.Time, bool) {
		yi, _ := strconv.Atoi(y)
		mi, _ := strconv.Atoi(m)
		di, _ := strconv.Atoi(d)
		t := time.Date(yi, time.Month(mi), di, 0, 0, 0, 0, time.UTC)
		if t.Year() != yi || int(t.Month()) != mi || t.Day() != di {
			return time.Time{}, false
		}
		return t, true
	}
	if m := cellISODateRe.FindStringSubmatch(cell); m != nil {
		return ymd(m[1], m[2], m[3])
	}
	if m := cellFRDateRe.FindStringSubmatch(cell); m != nil {
		return ymd(m[3], m[2], m[1])
	}
	return time.Time{}, false
}

// indexDate est une ligne de l'INDEX à comparer : le document visé et la date déclarée (jour, UTC).
type indexDate struct {
	Path     string
	Declared time.Time
}

// indexDates lit les lignes des tableaux de l'INDEX qui ont une colonne document et une colonne date : les
// lignes à comparer (au plus DOC10MaxDocuments, un document une seule fois) et la piste avec ses compteurs
// (lignes, dates illisibles, documents introuvables, lignes au-delà du plafond). reason non vide : piste
// inconnue (arbre inutilisable, pas d'INDEX, INDEX illisible) ou sans colonne date.
func indexDates(tree FileTree, rules FileRules, repo string) ([]indexDate, *leadSet, string) {
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return nil, nil, reason
	}
	rules = rules.normalized()
	loc := findIndex(idx, rules)
	if loc.path == "" {
		return nil, nil, UnknownNoFile
	}
	data, reason := tree.readBlob(loc.path)
	if reason != "" {
		return nil, nil, reason
	}
	doc := parseMarkdown(data)
	l := newLead(CriterionDOC10)
	for _, key := range []string{CountRows, CountCompared, CountUnresolved, CountOverLimit} {
		l.counts[key] = 0
	}
	var out []indexDate
	seen := map[string]bool{}
	tables := 0
	for _, t := range doc.Tables {
		cols := indexColumnsOf(t.Header)
		dc, okDoc := cols["document"]
		uc, okDate := cols["updated"]
		if !okDoc || !okDate {
			continue
		}
		tables++
		for _, row := range t.Rows {
			cell := func(i int) string {
				if i < len(row) {
					return strings.TrimSpace(row[i])
				}
				return ""
			}
			// Cellule Date vide : rien de déclaré, la ligne n'est pas comptée (DOC2 relève les cellules
			// manquantes).
			if cell(uc) == "" {
				continue
			}
			l.counts[CountRows]++
			declared, ok := declaredDate(cell(uc))
			if !ok {
				l.addCount(LeadDateUnreadable)
				continue
			}
			target := ""
			for _, link := range linksIn(cell(dc)) {
				if p, kind := resolveLink(loc.path, link, repo); kind != linkSkip && idx.files[p] {
					target = p
					break
				}
			}
			switch {
			case target == "":
				l.counts[CountUnresolved]++
			case seen[target]:
				// Document déjà comparé par une autre ligne : DOC3 compte les doublons.
			case len(out) == DOC10MaxDocuments:
				l.counts[CountOverLimit]++
			default:
				seen[target] = true
				out = append(out, indexDate{Path: target, Declared: declared})
			}
		}
	}
	if tables == 0 {
		return nil, nil, UnknownNoDateColumn
	}
	return out, l, ""
}

// checkDOC10 compare chaque date déclarée au dernier commit du document visé (last, par chemin ; absent :
// document jamais modifié, compté introuvable), au jour près dans le fuseau loc (nil : UTC) : plus récente
// que le commit, ou plus ancienne de plus de DOC10OlderDays jours. Aucune cellule n'est gardée.
func checkDOC10(rows []indexDate, l *leadSet, last map[string]time.Time, loc *time.Location) LeadCheck {
	if loc == nil {
		loc = time.UTC
	}
	for _, r := range rows {
		at, ok := last[r.Path]
		if !ok {
			l.counts[CountUnresolved]++
			continue
		}
		l.counts[CountCompared]++
		y, m, d := at.In(loc).Date()
		touched := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		switch {
		case r.Declared.After(touched):
			l.addCount(LeadDeclaredNewer)
		case touched.Sub(r.Declared) > DOC10OlderDays*24*time.Hour:
			l.addCount(LeadDeclaredOlder)
		}
	}
	return l.done()
}

// --- TL5 : effet de l'outillage ---

// toolingSources renvoie les fichiers d'outillage de TL1, TL2 et TL4 datés par leur premier ajout
// (configuration commitlint, .husky/commit-msg, lefthook ou pre-commit avec une entrée commit-msg,
// configuration commitizen ou cocogitto, workflow qui cite une vérification connue) et le nombre de
// sources non datables (clé ou dépendance d'un package.json, [tool.commitizen] d'un pyproject.toml : le
// fichier existait souvent avant l'outil). reason non vide : arbre inutilisable.
func toolingSources(tree FileTree) ([]string, int, string) {
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return nil, 0, reason
	}
	contains := func(p, s string) bool {
		data, reason := tree.readBlob(p)
		return reason == "" && strings.Contains(string(data), s)
	}
	var files []string
	undated := 0
	for _, p := range idx.paths {
		base := path.Base(p)
		root := !strings.Contains(p, "/")
		switch {
		case shallow(p) && commitlintFileRe.MatchString(base):
			files = append(files, p)
		case shallow(p) && (p == ".husky/commit-msg" || strings.HasSuffix(p, "/.husky/commit-msg")):
			files = append(files, p)
		case root && lefthookFileRe.MatchString(base) && contains(p, "commit-msg"):
			files = append(files, p)
		case root && (base == ".pre-commit-config.yaml" || base == ".pre-commit-config.yml") && contains(p, "commit-msg"):
			files = append(files, p)
		case root && (commitizenFileRe.MatchString(base) || base == "cog.toml"):
			files = append(files, p)
		case root && base == "pyproject.toml" && contains(p, "[tool.commitizen]"):
			undated++
		case isWorkflow(p):
			if data, reason := tree.readBlob(p); reason == "" && ciChecksIn(data) != 0 {
				files = append(files, p)
			}
		case shallow(p) && base == "package.json":
			pkg, ok, _ := readPackage(tree, p)
			if !ok {
				continue
			}
			_, dep := pkg.Dependencies["@commitlint/cli"]
			_, dev := pkg.DevDependencies["@commitlint/cli"]
			if len(pkg.Commitlint) > 0 || dep || dev || (root && len(pkg.Config.Commitizen) > 0) {
				undated++
			}
		}
	}
	return files, undated, ""
}

// DatedCommit est un commit évalué (CC1) du périmètre, daté : seule sa conformité est lue.
type DatedCommit struct {
	At      time.Time
	Conform bool
}

// CheckTL5 compare le taux CC1 des commits évalués avant et depuis intro, la plus ancienne introduction
// datée de l'outillage (zéro : non datée). sources et undated sont les nombres de sources datées et non
// datables (toolingSources). Aucune source : « n/a » (no_tooling) ; aucune source datée : inconnu
// (introduction_unknown) ; un effectif sous TL5MinCommits : « n/a » (too_few), les compteurs restant
// donnés ; un taux après pas plus haut qu'avant : à relire (no_gain). Rien n'est attribué à une personne.
func CheckTL5(intro time.Time, sources, undated int, commits []DatedCommit) LeadCheck {
	if sources == 0 && undated == 0 {
		lc := unknownLead(CriterionTL5, UnknownNoTooling)
		lc.Status = LeadNotApplicable
		return lc
	}
	if intro.IsZero() {
		lc := unknownLead(CriterionTL5, UnknownIntroductionUnknown)
		lc.Counts[CountSources], lc.Counts[CountUndatedSources] = sources, undated
		return lc
	}
	l := newLead(CriterionTL5)
	l.counts[CountSources], l.counts[CountUndatedSources] = sources, undated
	for _, key := range []string{CountBefore, CountBeforeConform, CountAfter, CountAfterConform, CountBeforePercent,
		CountAfterPercent} {
		l.counts[key] = 0
	}
	for _, c := range commits {
		before := c.At.Before(intro)
		switch {
		case before && c.Conform:
			l.counts[CountBeforeConform]++
			fallthrough
		case before:
			l.counts[CountBefore]++
		case c.Conform:
			l.counts[CountAfterConform]++
			fallthrough
		default:
			l.counts[CountAfter]++
		}
	}
	if l.counts[CountBefore] > 0 {
		l.counts[CountBeforePercent] = l.counts[CountBeforeConform] * 100 / l.counts[CountBefore]
	}
	if l.counts[CountAfter] > 0 {
		l.counts[CountAfterPercent] = l.counts[CountAfterConform] * 100 / l.counts[CountAfter]
	}
	dates := map[string]time.Time{DateIntroduced: intro.UTC()}
	if l.counts[CountBefore] < TL5MinCommits || l.counts[CountAfter] < TL5MinCommits {
		lc := l.withDates(dates)
		lc.Status, lc.Reason = LeadNotApplicable, UnknownTooFew
		return lc
	}
	// Comparaison exacte des taux (produits croisés), pas des pourcentages arrondis.
	if l.counts[CountAfterConform]*l.counts[CountBefore] <= l.counts[CountBeforeConform]*l.counts[CountAfter] {
		l.add(LeadNoGain)
	}
	return l.withDates(dates)
}
