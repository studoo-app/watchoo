package conventions

// Pistes de la phase 8.7 (documentation/fr/13-conventions.md, sections 5.5 à 5.8) : CL11, CL12, RD5, DOC6,
// DOC7, DOC9, DOC10, TL4 et TL5. Une piste invite à relire, elle ne juge pas : elle n'entre dans aucune
// grille « k/n critères conformes », ni dans la revue de jalon, ni dans aucun taux. Elle a ses propres états
// (LeadNone, LeadReview, CriterionUnknown, LeadNotApplicable) et ses propres codes, hors d'AllFileCodes.
//
// Les pistes de l'arbre (TreeLeadCriteria : CL11, CL12, RD5, DOC6, DOC7, TL4) sont mesurées par des
// fonctions pures sur l'arbre de la tête de la branche par défaut, avec les critères de fichiers, et mises en
// cache avec eux (FileResults.Leads) : des identifiants, des états, des codes et des compteurs, jamais un
// chemin, une valeur d'en-tête, un texte de puce ni un sujet de commit. Les chemins des images cassées
// (DOC7) sont rendus à part (FileLists.BrokenImages), pour la vue détaillée d'un dépôt.

import (
	"path"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// Pistes (clés --json), dans leur ordre d'affichage.
const (
	CriterionCL11  = "CL11"  // liens de comparaison du CHANGELOG
	CriterionCL12  = "CL12"  // recopie de git log dans le CHANGELOG
	CriterionRD5   = "RD5"   // installation décrite dans le README
	CriterionDOC6  = "DOC6"  // en-têtes des documents
	CriterionDOC7  = "DOC7"  // schémas et images
	CriterionDOC9  = "DOC9"  // fraîcheur de la documentation
	CriterionDOC10 = "DOC10" // dates déclarées dans l'INDEX
	CriterionTL4   = "TL4"   // CI qui vérifie les conventions
	CriterionTL5   = "TL5"   // effet de l'outillage
)

// LeadCriteria liste toutes les pistes, dans leur ordre d'affichage.
var LeadCriteria = []string{CriterionCL11, CriterionCL12, CriterionRD5, CriterionDOC6, CriterionDOC7,
	CriterionDOC9, CriterionDOC10, CriterionTL4, CriterionTL5}

// TreeLeadCriteria liste les pistes mesurées sur l'arbre de la tête et mises en cache par la synchro.
var TreeLeadCriteria = []string{CriterionCL11, CriterionCL12, CriterionRD5, CriterionDOC6, CriterionDOC7,
	CriterionTL4}

// États d'une piste (clés --json). Une piste inconnue a l'état CriterionUnknown.
const (
	LeadNone          = "none"           // rien à relire
	LeadReview        = "review"         // à relire : la piste invite à regarder, sans juger
	LeadNotApplicable = "not_applicable" // sans objet ou effectif trop petit pour conclure (CL11, CL12, DOC10, TL5)
)

// Raisons propres aux pistes (clés --json), en anglais.
const (
	UnknownDetailOnly          = "detail_only"          // piste calculée seulement dans la vue détaillée d'un dépôt
	UnknownTooFew              = "too_few"              // effectif sous le seuil (état LeadNotApplicable)
	UnknownNoVersion           = "no_version"           // CL11 : aucun titre de version (état LeadNotApplicable)
	UnknownIntroductionUnknown = "introduction_unknown" // TL5 : date d'introduction de l'outillage non mesurable
)

// Codes des pistes (clés --json, catalogue i18n), en anglais. Ils ne portent aucune gravité : une piste ne
// juge pas.
const (
	LeadMissingLink      = "missing_link"       // CL11 : titre sans définition de lien de comparaison
	LeadCopiedSubjects   = "copied_subjects"    // CL12 : au moins 60 % des puces identiques à un sujet de commit
	LeadShaOrMerge       = "sha_or_merge"       // CL12 : au moins 20 % des puces avec un SHA ou « Merge »
	LeadNoInstallSection = "no_install_section" // RD5 : ni section installation ni section lancement
	LeadNoCommandBlock   = "no_command_block"   // RD5 : aucun bloc de commandes sous ces sections
	LeadNoEnvExample     = "no_env_example"     // RD5 : variables annoncées sans .env.example
	LeadMissingHeaders   = "missing_headers"    // DOC6 : document sans tous les en-têtes
	LeadBrokenImage      = "broken_image"       // DOC7 : image relative introuvable
	LeadExternalImage    = "external_image"     // DOC7 : image externe (badges exclus)
	LeadCINotFound       = "not_found"          // TL4 : aucun workflow ne cite une vérification connue
	LeadDocsBehind       = "docs_behind"        // DOC9 : documentation en retard sur le code
	LeadDeclaredNewer    = "declared_newer"     // DOC10 : date déclarée plus récente que le dernier commit
	LeadDeclaredOlder    = "declared_older"     // DOC10 : date déclarée plus ancienne de plus de 30 jours
	LeadDateUnreadable   = "date_unreadable"    // DOC10 : date de l'INDEX illisible
	LeadNoGain           = "no_gain"            // TL5 : taux CC1 après l'outillage pas plus haut qu'avant
)

// AllLeadCodes liste les codes des pistes dans leur ordre d'affichage.
var AllLeadCodes = []string{LeadMissingLink, LeadCopiedSubjects, LeadShaOrMerge, LeadNoInstallSection,
	LeadNoCommandBlock, LeadNoEnvExample, LeadMissingHeaders, LeadBrokenImage, LeadExternalImage, LeadDocsBehind,
	LeadDeclaredNewer, LeadDeclaredOlder, LeadDateUnreadable, LeadCINotFound, LeadNoGain}

// Clés des compteurs des pistes (clés --json). Un compteur dont la clé est un code compte les éléments qui
// le portent.
const (
	CountTitles          = "titles"            // CL11 : titres de version et [Unreleased]
	CountLinked          = "linked"            // CL11 : titres avec un lien de comparaison
	CountBullets         = "bullets"           // CL12 : puces non vides du CHANGELOG
	CountSubjects        = "subjects"          // CL12 : sujets de commit distincts comparés
	CountSubjectPercent  = "subject_percent"   // CL12 : % des puces identiques à un sujet (arrondi inférieur)
	CountShaMergePercent = "sha_merge_percent" // CL12 : % des puces avec un SHA ou commençant par « Merge »
	CountCommandBlocks   = "command_blocks"    // RD5 : blocs de commandes sous installation ou lancement
	CountEnvAnnounced    = "env_announced"     // RD5 : 1 si le README annonce des variables d'environnement
	CountEnvExample      = "env_example"       // RD5 : 1 si un .env.example est versionné
	CountComplete        = "complete"          // DOC6 : documents avec les six en-têtes
	CountImages          = "images"            // DOC7 : images relatives (ou du même dépôt)
	CountBadges          = "badges"            // DOC7 : badges (images externes non comptées)
	CountMermaid         = "mermaid"           // DOC7 : blocs mermaid
	CountMediaDir        = "media_dir"         // DOC7 : 1 si le dossier des médias existe
	CountWorkflows       = "workflows"         // TL4 : workflows .github/workflows/*.yml
	CountCIMask          = "ci_mask"           // TL4 : masque des vérifications (bits dans l'ordre d'AllCIChecks)
)

// Seuils des pistes (documentation/fr/13-conventions.md, section 11.1 : à valider sur des dépôts réels).
const (
	CL12MinBullets      = 10 // en dessous : « n/a »
	CL12SubjectPercent  = 60 // puces identiques à un sujet de commit
	CL12ShaMergePercent = 20 // puces avec un SHA ou « Merge »
	DOC6HeaderLines     = 40 // lignes lues en tête de document
	maxHeaderKeyRunes   = 40 // longueur maximale d'une clé « Clé : valeur »
)

// LeadCheck est l'état d'une piste. Il ne porte ni chemin ni contenu.
type LeadCheck struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`           // LeadNone, LeadReview, CriterionUnknown ou LeadNotApplicable
	Reason string         `json:"reason,omitempty"` // raison d'un état inconnu ou « n/a » ; vide sinon
	Codes  []string       `json:"codes"`            // dans l'ordre d'AllLeadCodes ; vide non nil si aucun
	Counts map[string]int `json:"counts"`           // compteurs ; vide non nil si aucun
	// Dates sont les dates des pistes de l'historique (DOC9, TL5), en UTC, calculées à la lecture et jamais
	// mises en cache ; nil sinon.
	Dates map[string]time.Time `json:"dates,omitempty"`
}

// leadSet accumule les codes et les compteurs d'une piste.
type leadSet struct {
	id     string
	codes  map[string]bool
	counts map[string]int
}

func newLead(id string) *leadSet {
	return &leadSet{id: id, codes: map[string]bool{}, counts: map[string]int{}}
}

func (l *leadSet) add(code string) { l.codes[code] = true }

// addCount ajoute le code et compte un élément qui le porte.
func (l *leadSet) addCount(code string) {
	l.codes[code] = true
	l.counts[code]++
}

// done renvoie la piste : à relire si elle porte un code, rien à relire sinon.
func (l *leadSet) done() LeadCheck {
	lc := LeadCheck{ID: l.id, Status: LeadNone, Codes: []string{}, Counts: l.counts}
	for _, code := range AllLeadCodes {
		if l.codes[code] {
			lc.Codes = append(lc.Codes, code)
			lc.Status = LeadReview
		}
	}
	return lc
}

// unknownLead renvoie une piste inconnue pour la raison donnée.
func unknownLead(id, reason string) LeadCheck {
	return LeadCheck{ID: id, Status: CriterionUnknown, Reason: reason, Codes: []string{}, Counts: map[string]int{}}
}

// UnknownLeads renvoie les pistes ids, toutes inconnues pour la raison donnée.
func UnknownLeads(ids []string, reason string) []LeadCheck {
	out := make([]LeadCheck, 0, len(ids))
	for _, id := range ids {
		out = append(out, unknownLead(id, reason))
	}
	return out
}

// FindLead renvoie la piste id d'une liste ; faux si elle n'y est pas.
func FindLead(leads []LeadCheck, id string) (LeadCheck, bool) {
	if i := slices.IndexFunc(leads, func(l LeadCheck) bool { return l.ID == id }); i >= 0 {
		return leads[i], true
	}
	return LeadCheck{}, false
}

// CheckLeads mesure les pistes de l'arbre (TreeLeadCriteria) et renvoie les images relatives cassées (DOC7),
// pour la vue détaillée. opts.Subjects sert à CL12 et n'est jamais gardé.
func CheckLeads(tree FileTree, opts FileOptions) ([]LeadCheck, []DeadLink) {
	idx := newTreeIndex(tree.Paths)
	if reason := treeUnknown(tree, idx); reason != "" {
		return UnknownLeads(TreeLeadCriteria, reason), nil
	}
	rules := opts.Rules.normalized()
	cl, clReason := readChangelogDoc(tree, idx)
	doc7, broken := checkDOC7(tree, idx, rules, opts.Repo)
	return []LeadCheck{
		checkCL11(cl, clReason, rules),
		checkCL12(cl, clReason, opts.Subjects),
		checkRD5(tree, idx),
		checkDOC6(tree, idx, rules),
		doc7,
		checkTL4(tree, idx),
	}, broken
}

// BrokenImages renvoie les images relatives cassées de l'arbre (DOC7), pour la vue détaillée d'un dépôt ;
// nil pour un arbre tronqué ou vide.
func BrokenImages(tree FileTree, opts FileOptions) []DeadLink {
	idx := newTreeIndex(tree.Paths)
	if treeUnknown(tree, idx) != "" {
		return nil
	}
	_, broken := checkDOC7(tree, idx, opts.Rules.normalized(), opts.Repo)
	return broken
}

// leadBlobs renvoie les fichiers lus par les pistes en plus des critères de fichiers : les workflows (TL4).
// Le CHANGELOG, le README, l'INDEX et les documents sont déjà lus ; .env.example n'est cherché que dans la
// liste des chemins.
func leadBlobs(idx *treeIndex) []string {
	var out []string
	for _, p := range idx.paths {
		if isWorkflow(p) {
			out = append(out, p)
		}
	}
	return out
}

// readChangelogDoc lit et analyse le CHANGELOG trouvé (même recherche que CL1) ; sinon la raison de
// l'inconnu (no_file, blob_too_large, blob_unreadable).
func readChangelogDoc(tree FileTree, idx *treeIndex) (mdDoc, string) {
	loc := findChangelog(idx)
	if loc.path == "" {
		return mdDoc{}, UnknownNoFile
	}
	data, reason := tree.readBlob(loc.path)
	if reason != "" {
		return mdDoc{}, reason
	}
	return parseMarkdown(data), ""
}

// --- CL11 : liens de comparaison ---

// compareLabel normalise un libellé de titre ou de définition : minuscules, sans espaces autour ni « v »
// devant une version (« [v1.2.0] » et « [1.2.0]: » se répondent).
func compareLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 1 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9' {
		s = s[1:]
	}
	return s
}

// checkCL11 : chaque titre de version (et [Unreleased]) a une définition « [X]: url » de même libellé, ou un
// lien écrit dans le titre (« ## [1.0.0](https://…) »). Un titre illisible (ni version ni [Unreleased])
// n'est pas compté : sa forme relève de CL4.
func checkCL11(doc mdDoc, reason string, rules FileRules) LeadCheck {
	if reason != "" {
		return unknownLead(CriterionCL11, reason)
	}
	l := newLead(CriterionCL11)
	defs := map[string]bool{}
	for _, link := range doc.Links {
		if link.Def && strings.TrimSpace(link.Target) != "" {
			defs[compareLabel(link.Label)] = true
		}
	}
	l.counts[CountTitles], l.counts[CountLinked] = 0, 0
	// La date du jour ne sert qu'aux codes de CL4, sans effet ici.
	for _, s := range parseChangelog(doc, rules, "9999-12-31").sections {
		if !s.unreleased && !s.hasVersion {
			continue
		}
		l.counts[CountTitles]++
		if s.inlineLink || defs[compareLabel(s.label)] {
			l.counts[CountLinked]++
			continue
		}
		l.addCount(LeadMissingLink)
	}
	if l.counts[CountTitles] == 0 {
		// Aucun titre de version ni [Unreleased] : rien à comparer, sans objet plutôt que « rien à relire ».
		lc := l.done()
		lc.Status, lc.Reason = LeadNotApplicable, UnknownNoVersion
		return lc
	}
	return l.done()
}

// --- CL12 : recopie de git log ---

var (
	// shaRe reconnaît un SHA abrégé ou complet (7 à 40 chiffres hexadécimaux) ; checkCL12 exige en plus au
	// moins un chiffre et une lettre, pour écarter des mots (« defaced ») et des nombres.
	shaRe = regexp.MustCompile(`\b[0-9a-fA-F]{7,40}\b`)
)

// hasSHA indique un texte qui contient un SHA (au moins un chiffre et une lettre hexadécimale).
func hasSHA(s string) bool {
	for _, m := range shaRe.FindAllString(s, -1) {
		if strings.ContainsAny(m, "0123456789") && strings.ContainsAny(strings.ToLower(m), "abcdef") {
			return true
		}
	}
	return false
}

// commitSubjectKey normalise un sujet de commit ou une puce pour la comparaison de CL12 : suffixe « (#n) »
// retiré, puis minuscules, sans accent, signes réduits à une espace.
func commitSubjectKey(s string) string {
	return normalizeText(StripSquashSuffix(s))
}

// checkCL12 : au moins CL12MinBullets puces non vides, dont au moins CL12SubjectPercent % identiques (après
// normalisation) à un sujet de commit du dépôt, ou au moins CL12ShaMergePercent % qui contiennent un SHA ou
// commencent par « Merge ». Seuls des pourcentages et des effectifs sont gardés ; les sujets ne servent
// qu'en mémoire.
func checkCL12(doc mdDoc, reason string, subjects []string) LeadCheck {
	if reason != "" {
		return unknownLead(CriterionCL12, reason)
	}
	known := make(map[string]bool, len(subjects))
	for _, s := range subjects {
		if k := commitSubjectKey(s); k != "" {
			known[k] = true
		}
	}
	bullets, same, shaMerge := 0, 0, 0
	for _, line := range doc.Lines {
		if line.Kind != mdBullet {
			continue
		}
		key := commitSubjectKey(line.Text)
		if key == "" {
			continue
		}
		bullets++
		if known[key] {
			same++
		}
		if hasSHA(line.Text) || key == "merge" || strings.HasPrefix(key, "merge ") {
			shaMerge++
		}
	}
	l := newLead(CriterionCL12)
	l.counts[CountBullets] = bullets
	l.counts[CountSubjects] = len(known)
	l.counts[CountSubjectPercent], l.counts[CountShaMergePercent] = 0, 0
	if bullets > 0 {
		l.counts[CountSubjectPercent] = same * 100 / bullets
		l.counts[CountShaMergePercent] = shaMerge * 100 / bullets
	}
	if bullets < CL12MinBullets {
		lc := l.done()
		lc.Status, lc.Reason = LeadNotApplicable, UnknownTooFew
		return lc
	}
	if same*100 >= CL12SubjectPercent*bullets {
		l.add(LeadCopiedSubjects)
	}
	if shaMerge*100 >= CL12ShaMergePercent*bullets {
		l.add(LeadShaOrMerge)
	}
	return l.done()
}

// --- RD5 : installation ---

// commandFences sont les info-strings d'un bloc de commandes ; un bloc sans info-string en est un aussi.
var commandFences = []string{"", "bash", "sh", "shell", "console", "zsh", "fish", "terminal", "shell-session",
	"shellsession", "sh-session", "powershell", "pwsh", "ps", "ps1", "cmd", "bat", "batch", "bash-session"}

// envExampleRe reconnaît un modèle de fichier d'environnement versionné : .env.example, .env.sample,
// .env.dist, .env.template, ou .env.<nom>.example.
var envExampleRe = regexp.MustCompile(`(?i)^\.env(\.[a-z0-9_-]+)?\.(example|sample|dist|template)$`)

// headingChain renvoie les indices (dans doc.Lines) du titre h et de ses titres parents.
func headingChain(doc mdDoc, h int) []int {
	if h < 0 || h >= len(doc.Lines) {
		return nil
	}
	chain := []int{h}
	level := doc.Lines[h].Level
	for i := h - 1; i >= 0 && level > 1; i-- {
		if l := doc.Lines[i]; l.Kind == mdHeading && l.Level < level {
			chain = append(chain, i)
			level = l.Level
		}
	}
	return chain
}

// headingIs indique un titre qui reconnaît la section key (mêmes synonymes que RD2).
func headingIs(text, key string) bool {
	h := normalizeHeading(text)
	return slices.ContainsFunc(readmeSynonyms[key], func(s string) bool { return containsPhrase(h, s) })
}

// envMentionRe reconnaît « .env » comme mot à part, hors code en ligne : en début de ligne ou après une
// espace, une parenthèse, un crochet ou un guillemet ; « .env.example » compte, pas « .envrc »,
// « process.env », « import.meta.env » ni une URL qui contient « /.env ».
var envMentionRe = regexp.MustCompile(`(?i)(?:^|[\s(\["'])\.env(?:$|[^\w-])`)

// checkRD5 : un bloc de commandes est écrit sous une section installation ou lancement (titre reconnu par
// les synonymes de RD2, ou un de ses sous-titres) ; si le README annonce des variables d'environnement
// (section reconnue ou mention de « .env »), un .env.example est versionné (profondeur au plus 2).
func checkRD5(tree FileTree, idx *treeIndex) LeadCheck {
	loc := findReadme(idx)
	if loc.path == "" {
		return unknownLead(CriterionRD5, UnknownNoFile)
	}
	data, reason := tree.readBlob(loc.path)
	if reason != "" {
		return unknownLead(CriterionRD5, reason)
	}
	doc := parseMarkdown(data)
	l := newLead(CriterionRD5)
	found := readmeSectionsFound(doc)
	l.counts[CountCommandBlocks], l.counts[CountEnvAnnounced], l.counts[CountEnvExample] = 0, 0, 0
	if !found[SectionInstall] && !found[SectionRun] {
		l.add(LeadNoInstallSection)
	} else {
		for _, f := range doc.Fences {
			if !slices.Contains(commandFences, f.Info) {
				continue
			}
			if slices.ContainsFunc(headingChain(doc, f.Heading), func(i int) bool {
				return headingIs(doc.Lines[i].Text, SectionInstall) || headingIs(doc.Lines[i].Text, SectionRun)
			}) {
				l.counts[CountCommandBlocks]++
			}
		}
		if l.counts[CountCommandBlocks] == 0 {
			l.add(LeadNoCommandBlock)
		}
	}
	announced := found[SectionEnv]
	for _, line := range doc.Lines {
		if line.Kind != mdBlank && envMentionRe.MatchString(stripCodeSpans(line.Text)) {
			announced = true
		}
	}
	for _, p := range idx.paths {
		if shallow(p) && envExampleRe.MatchString(path.Base(p)) {
			l.counts[CountEnvExample] = 1
		}
	}
	if announced {
		l.counts[CountEnvAnnounced] = 1
		if l.counts[CountEnvExample] == 0 {
			l.add(LeadNoEnvExample)
		}
	}
	return l.done()
}

// --- DOC6 : en-têtes de document ---

// Champs d'en-tête d'un document (DOC6), dans leur ordre d'affichage ; le compteur d'un champ est
// « header_<champ> » (documents qui le portent).
const (
	HeaderTitle     = "title"
	HeaderObjective = "objective"
	HeaderScope     = "scope"
	HeaderAuthor    = "author"
	HeaderDate      = "date"
	HeaderVersion   = "version"
)

// AllHeaderFields liste les champs d'en-tête de DOC6.
var AllHeaderFields = []string{HeaderTitle, HeaderObjective, HeaderScope, HeaderAuthor, HeaderDate, HeaderVersion}

// HeaderCountKey renvoie la clé du compteur d'un champ d'en-tête (« header_title »).
func HeaderCountKey(field string) string { return "header_" + field }

// headerSynonyms sont les clés reconnues de chaque champ, normalisées, comparées à l'identique.
var headerSynonyms = map[string][]string{
	HeaderTitle:     {"titre", "title", "nom", "nom du document", "document", "intitule"},
	HeaderObjective: {"objectif", "objectifs", "objective", "objectives", "purpose", "but", "goal", "goals", "finalite"},
	HeaderScope:     {"perimetre", "scope", "portee", "champ", "champ d application", "domaine"},
	HeaderAuthor: {"auteur", "auteurs", "author", "authors", "redacteur", "redacteurs", "redige par", "written by",
		"responsable", "responsables", "owner", "maintainer", "auteur s"},
	HeaderDate: {"date", "mise a jour", "derniere mise a jour", "date de mise a jour", "last update", "last updated",
		"updated", "cree le", "created", "date de creation", "modifie le", "derniere modification"},
	HeaderVersion: {"version", "revision", "rev"},
}

var (
	// headerKVRe reconnaît « Clé : valeur », « **Clé :** valeur », « - Clé: valeur », « > Clé : valeur ».
	headerKVRe = regexp.MustCompile(`^\s{0,3}(?:>\s*)?(?:[-*+]\s+)?(?:\*\*|__)?([^:|*_]+?)\s*(?:\*\*|__)?\s*:\s*(?:\*\*|__)?\s*(.*)$`)
	// headerRowRe reconnaît une ligne de tableau « | Clé | valeur | ».
	headerRowRe = regexp.MustCompile(`^\s*\|\s*(?:\*\*|__)?([^|*_]+?)(?:\*\*|__)?\s*\|\s*([^|]*)\|`)
	h1Re        = regexp.MustCompile(`^ {0,3}#[ \t]+\S`)
)

// headerField renvoie le champ d'une clé d'en-tête ; vide si elle n'en désigne aucun.
func headerField(key string) string {
	if utf8.RuneCountInString(key) > maxHeaderKeyRunes {
		return ""
	}
	k := normalizeText(key)
	for _, f := range AllHeaderFields {
		if slices.Contains(headerSynonyms[f], k) {
			return f
		}
	}
	return ""
}

// headerValue indique une valeur d'en-tête non vide (marques d'emphase et espaces retirés). La valeur n'est
// jamais gardée.
func headerValue(v string) bool {
	return strings.Trim(v, " \t*_`\"'") != ""
}

// headerFields renvoie les champs d'en-tête présents dans les DOC6HeaderLines premières lignes, hors blocs
// de code clôturés (``` ou ~~~) : front matter YAML (« clé: valeur », ou une clé suivie d'une liste), lignes « Clé : valeur » (FR ou EN, en gras, en
// puce, en citation ou en ligne de tableau) ; un titre de niveau 1 vaut titre. Les valeurs ne sont jamais
// gardées.
func headerFields(data []byte) map[string]bool {
	text := string(data)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "\uFFFD")
	}
	text = strings.TrimPrefix(text, "\uFEFF")
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) > DOC6HeaderLines {
		lines = lines[:DOC6HeaderLines]
	}
	found := map[string]bool{}
	frontMatter := len(lines) > 0 && strings.TrimSpace(lines[0]) == "---"
	fence := "" // marque du bloc de code ouvert hors front matter (``` ou ~~~), dont les lignes sont ignorées
	for i, line := range lines {
		if frontMatter && i > 0 && (strings.TrimSpace(line) == "---" || strings.TrimSpace(line) == "...") {
			frontMatter = false
			continue
		}
		if !frontMatter {
			if fence != "" {
				if t := strings.TrimSpace(line); strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == "" {
					fence = ""
				}
				continue
			}
			if m := mdFenceRe.FindStringSubmatch(line); m != nil && (m[1][0] != '`' || !strings.Contains(m[2], "`")) {
				fence = m[1]
				continue
			}
		}
		if h1Re.MatchString(line) {
			found[HeaderTitle] = true
			continue
		}
		key, value, ok := "", "", false
		if m := headerRowRe.FindStringSubmatch(line); m != nil {
			key, value, ok = m[1], m[2], true
		} else if m := headerKVRe.FindStringSubmatch(line); m != nil {
			key, value, ok = m[1], m[2], true
		}
		if !ok {
			continue
		}
		f := headerField(key)
		if f == "" {
			continue
		}
		// Front matter : « authors: » suivi d'une liste YAML.
		listFollows := frontMatter && i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "- ")
		if headerValue(value) || listFollows {
			found[f] = true
		}
	}
	return found
}

// checkDOC6 : pour chaque document de la documentation (mêmes documents que DOC4, au plus MaxDocuments),
// les champs d'en-tête présents ; compteurs par champ, documents complets, documents à relire.
func checkDOC6(tree FileTree, idx *treeIndex, rules FileRules) LeadCheck {
	index := findIndex(idx, rules)
	root := docsRoot(index, rules)
	if !idx.dirs[root] {
		return unknownLead(CriterionDOC6, UnknownNoDocsDir)
	}
	l := newLead(CriterionDOC6)
	l.counts[CountDocuments], l.counts[CountComplete] = 0, 0
	for _, f := range AllHeaderFields {
		l.counts[HeaderCountKey(f)] = 0
	}
	docs := docDocuments(idx, root, rules, index.path, 0)
	if len(docs) > MaxDocuments {
		l.counts[CountUnread] += len(docs) - MaxDocuments
		docs = docs[:MaxDocuments]
	}
	for _, p := range docs {
		data, reason := tree.readBlob(p)
		if reason != "" {
			l.counts[CountUnread]++
			continue
		}
		l.counts[CountDocuments]++
		fields := headerFields(data)
		for _, f := range AllHeaderFields {
			if fields[f] {
				l.counts[HeaderCountKey(f)]++
			}
		}
		if len(fields) == len(AllHeaderFields) {
			l.counts[CountComplete]++
		} else {
			l.addCount(LeadMissingHeaders)
		}
	}
	lc := l.done()
	if lc.Status == LeadNone && l.counts[CountDocuments] == 0 && l.counts[CountUnread] > 0 {
		lc.Status, lc.Reason = CriterionUnknown, UnknownBlobUnreadable
	}
	return lc
}

// --- DOC7 : schémas et images ---

var (
	// imgSrcRe reconnaît une image HTML <img src="…">.
	imgSrcRe = regexp.MustCompile(`(?i)<img\b[^>]*?\bsrc\s*=\s*["']([^"']+)["']`)
	// httpRe reconnaît une URL absolue http(s) ou sans schéma (« //hôte/… »).
	httpRe = regexp.MustCompile(`(?i)^(https?:)?//`)
)

// badgeHosts sont les hôtes de badges (images externes non comptées).
var badgeHosts = []string{"img.shields.io", "shields.io", "badgen.net", "badge.fury.io", "codecov.io",
	"coveralls.io", "travis-ci.org", "travis-ci.com", "circleci.com", "api.netlify.com", "sonarcloud.io",
	"snyk.io", "goreportcard.com", "app.codacy.com", "api.codeclimate.com", "deepscan.io", "bestpractices.dev",
	"www.bestpractices.dev", "forthebadge.com", "badges.gitter.im"}

// isBadge indique l'URL d'un badge : hôte connu, ou « badge » dans l'URL (badge d'un workflow GitHub).
func isBadge(u string) bool {
	lower := strings.ToLower(u)
	if strings.Contains(lower, "badge") {
		return true
	}
	rest := httpRe.ReplaceAllString(lower, "")
	host, _, _ := strings.Cut(rest, "/")
	return slices.Contains(badgeHosts, host)
}

// imagesIn renvoie les cibles des images d'une ligne hors code : « ![alt](cible) », y compris dans le texte
// d'un lien (« [![badge](img)](lien) »), et <img src="…">.
func imagesIn(s string) []string {
	s = stripCodeSpans(s)
	var out []string
	var walk func(string, int)
	walk = func(s string, depth int) {
		for _, m := range mdLinkRe.FindAllStringSubmatch(s, -1) {
			if m[1] == "!" {
				out = append(out, unbracket(m[3]))
			} else if depth == 0 {
				walk(m[2], 1)
			}
		}
	}
	walk(s, 0)
	for _, m := range imgSrcRe.FindAllStringSubmatch(s, -1) {
		out = append(out, m[1])
	}
	return out
}

// checkDOC7 : images du README, de l'INDEX et des documents (mêmes sources que DOC4) : relatives présentes
// ou cassées (une autre casse est cassée : GitHub distingue la casse), externes hors badges, badges ; blocs
// mermaid ; dossier des médias. Renvoie les images cassées, pour la vue détaillée.
func checkDOC7(tree FileTree, idx *treeIndex, rules FileRules, repo string) (LeadCheck, []DeadLink) {
	l := newLead(CriterionDOC7)
	index := findIndex(idx, rules)
	root := docsRoot(index, rules)
	for _, k := range []string{CountSources, CountImages, CountBadges, CountMermaid, CountMediaDir} {
		l.counts[k] = 0
	}
	media := root + "/" + rules.MediaDir
	if idx.dirs[media] || idx.lowerDirs[strings.ToLower(media)] {
		l.counts[CountMediaDir] = 1
	}
	var sources []string
	if readme := findReadme(idx); readme.path != "" {
		sources = append(sources, readme.path)
	}
	if index.path != "" {
		sources = append(sources, index.path)
	}
	all := docDocuments(idx, root, rules, index.path, 0)
	if len(all) > MaxDocuments {
		l.counts[CountUnread] += len(all) - MaxDocuments
		all = all[:MaxDocuments]
	}
	sources = append(sources, all...)
	var broken []DeadLink
	seen := map[string]bool{}
	for _, src := range sources {
		if seen[src] {
			continue
		}
		seen[src] = true
		data, reason := tree.readBlob(src)
		if reason != "" {
			l.counts[CountUnread]++
			continue
		}
		l.counts[CountSources]++
		doc := parseMarkdown(data)
		for _, f := range doc.Fences {
			if f.Info == "mermaid" {
				l.counts[CountMermaid]++
			}
		}
		for _, line := range doc.Lines {
			if line.Kind == mdBlank || line.Kind == mdLinkDef || line.Kind == mdBreak {
				continue
			}
			for _, target := range imagesIn(line.Text) {
				t := strings.TrimSpace(target)
				if t == "" || strings.HasPrefix(strings.ToLower(t), "data:") {
					continue
				}
				p, kind := resolveLink(src, t, repo)
				switch {
				case kind == linkSkip && httpRe.MatchString(t):
					if isBadge(t) {
						l.counts[CountBadges]++
					} else {
						l.addCount(LeadExternalImage)
					}
					continue
				case kind == linkSkip:
					continue
				}
				l.counts[CountImages]++
				if idx.files[p] {
					continue
				}
				l.addCount(LeadBrokenImage)
				broken = append(broken, DeadLink{Source: src, Target: t, Resolved: p, CaseMismatch: idx.existsFold(p)})
			}
		}
	}
	lc := l.done()
	if lc.Status == LeadNone && l.counts[CountUnread] > 0 {
		lc.Status, lc.Reason = CriterionUnknown, UnknownUnreadDocuments
	}
	return lc, broken
}

// --- TL4 : CI qui vérifie les conventions ---

// Vérifications des conventions reconnues dans un workflow (TL4), dans l'ordre des bits de CountCIMask.
// Seul l'identifiant du motif est gardé.
const (
	CICommitlint          = "commitlint"
	CISemanticPullRequest = "semantic-pull-request"
	CICommitizen          = "commitizen"
	CICocogitto           = "cocogitto"
	CIConventionalCommits = "conventional-commits"
)

// AllCIChecks liste les vérifications de TL4 dans l'ordre des bits de CountCIMask.
var AllCIChecks = []string{CICommitlint, CISemanticPullRequest, CICommitizen, CICocogitto, CIConventionalCommits}

// ciPatterns sont les motifs (en minuscules) de chaque vérification, cherchés dans les lignes hors
// commentaire d'un workflow : actions connues et commandes.
var ciPatterns = map[string][]string{
	CICommitlint:          {"commitlint"}, // wagoid/commitlint-github-action, npx commitlint
	CISemanticPullRequest: {"amannn/action-semantic-pull-request", "semantic-pull-request", "zeke/semantic-pull-requests"},
	CICommitizen:          {"commitizen-tools/commitizen-action", "cz check", "cz-cli"},
	CICocogitto:           {"cocogitto/cocogitto-action", "cog check", "cog verify"},
	CIConventionalCommits: {"webiny/action-conventional-commits", "ytanikin/prconventionalcommits",
		"agenthunt/conventional-commit-checker-action", "conventional-commit-checker"},
}

// isWorkflow indique un workflow GitHub Actions : .github/workflows/<nom>.yml ou .yaml (sans sous-dossier).
func isWorkflow(p string) bool {
	dir, ext := path.Dir(p), strings.ToLower(path.Ext(p))
	return dir == ".github/workflows" && (ext == ".yml" || ext == ".yaml")
}

// ciChecksIn renvoie le masque des vérifications citées par un workflow (lignes de commentaire ignorées).
func ciChecksIn(data []byte) int {
	mask := 0
	for _, line := range strings.Split(strings.ToLower(string(data)), "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		for i, id := range AllCIChecks {
			if slices.ContainsFunc(ciPatterns[id], func(p string) bool { return strings.Contains(t, p) }) {
				mask |= 1 << i
			}
		}
	}
	return mask
}

// checkTL4 : un workflow .github/workflows/*.yml cite une vérification connue des conventions ; seuls le
// nombre de workflows et le masque des vérifications sont gardés.
func checkTL4(tree FileTree, idx *treeIndex) LeadCheck {
	l := newLead(CriterionTL4)
	l.counts[CountWorkflows], l.counts[CountCIMask] = 0, 0
	for _, p := range idx.paths {
		if !isWorkflow(p) {
			continue
		}
		l.counts[CountWorkflows]++
		data, reason := tree.readBlob(p)
		if reason != "" {
			l.counts[CountUnread]++
			continue
		}
		l.counts[CountCIMask] |= ciChecksIn(data)
	}
	if l.counts[CountCIMask] == 0 {
		if l.counts[CountUnread] > 0 {
			lc := unknownLead(CriterionTL4, UnknownBlobUnreadable)
			lc.Counts[CountWorkflows], lc.Counts[CountUnread] = l.counts[CountWorkflows], l.counts[CountUnread]
			return lc
		}
		l.add(LeadCINotFound)
	}
	return l.done()
}

// CIChecks renvoie les vérifications d'un masque CountCIMask, dans l'ordre d'AllCIChecks.
func CIChecks(mask int) []string {
	var out []string
	for i, id := range AllCIChecks {
		if mask&(1<<i) != 0 {
			out = append(out, id)
		}
	}
	return out
}
