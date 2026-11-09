package conventions

// Critères de fichiers de la phase 8.3, mesurés sur l'arbre d'une révision du clone (tête de la branche
// par défaut ou tag d'un jalon) : CHANGELOG (CL1 à CL9), README (RD1 à RD4), documentation (DOC1 à DOC5,
// DOC8) et outillage (TL1 à TL3). Fonctions pures : l'appelant fournit la liste des chemins de l'arbre et
// le contenu des fichiers demandés par FileBlobPaths (lus par les objets git, jamais par le système de
// fichiers de repos/). Un résultat (FileResults) ne porte que des identifiants, des états, des codes et des
// compteurs : ni chemin, ni contenu, ni cellule d'INDEX. Les listes de chemins (FileLists) sont rendues à
// part, pour la vue détaillée d'un dépôt, et ne sont jamais stockées.

import (
	"crypto/sha256"
	"encoding/hex"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// FileRulesVersion est la version des règles des critères de fichiers. Elle est distincte de RulesVersion
// (règles des commits, calculées à la lecture) : seuls les résultats de fichiers sont mis en cache
// (repository_conventions), et un changement des règles de commit ne doit pas les invalider. Elle change
// quand un même arbre peut recevoir d'autres états, codes ou compteurs.
const FileRulesVersion = 1

// Critères de fichiers (clés --json), dans leur ordre d'affichage.
const (
	CriterionCL1 = "CL1" // CHANGELOG.md présent à la racine
	CriterionCL2 = "CL2" // en-tête et déclarations (Keep a Changelog, SemVer)
	CriterionCL3 = "CL3" // [Unreleased] en tête
	CriterionCL4 = "CL4" // titres de version et dates
	CriterionCL5 = "CL5" // ordre décroissant
	CriterionCL6 = "CL6" // catégories
	CriterionCL7 = "CL7" // entrées vides
	CriterionCL8 = "CL8" // tags et sections cohérents
	CriterionCL9 = "CL9" // section du jalon présente au tag (révision d'un jalon seulement)

	CriterionRD1 = "RD1" // README.md présent à la racine
	CriterionRD2 = "RD2" // sections attendues
	CriterionRD3 = "RD3" // liens vers l'INDEX et le CHANGELOG
	CriterionRD4 = "RD4" // taille (mots hors code)

	CriterionDOC1 = "DOC1" // INDEX présent
	CriterionDOC2 = "DOC2" // tableau de l'INDEX
	CriterionDOC3 = "DOC3" // INDEX exhaustif
	CriterionDOC4 = "DOC4" // liens morts
	CriterionDOC5 = "DOC5" // nommage
	CriterionDOC8 = "DOC8" // archives

	CriterionTL1 = "TL1" // commitlint configuré
	CriterionTL2 = "TL2" // hook commit-msg versionné
	CriterionTL3 = "TL3" // gabarit de pull request
)

// Familles de critères de fichiers (clés --json).
const (
	FamilyChangelog = "changelog"
	FamilyReadme    = "readme"
	FamilyDocs      = "docs"
	FamilyTooling   = "tooling"
)

// Critères de chaque famille, dans leur ordre d'affichage. CL9 n'est mesuré que sur l'arbre du tag d'un
// jalon (FileOptions.Version).
var (
	ChangelogCriteria = []string{CriterionCL1, CriterionCL2, CriterionCL3, CriterionCL4, CriterionCL5,
		CriterionCL6, CriterionCL7, CriterionCL8, CriterionCL9}
	ReadmeCriteria  = []string{CriterionRD1, CriterionRD2, CriterionRD3, CriterionRD4}
	DocsCriteria    = []string{CriterionDOC1, CriterionDOC2, CriterionDOC3, CriterionDOC4, CriterionDOC5, CriterionDOC8}
	ToolingCriteria = []string{CriterionTL1, CriterionTL2, CriterionTL3}
)

// Raisons d'un critère de fichiers inconnu (clés --json), en anglais.
const (
	UnknownEmptyRepository = "empty_repository" // arbre vide (dépôt sans commit) : rien à mesurer
	UnknownTreeTruncated   = "tree_truncated"   // liste des chemins tronquée au plafond : absence non démontrable
	UnknownNoFile          = "no_file"          // le fichier lu par le critère est absent (son absence est comptée ailleurs)
	UnknownBlobTooLarge    = "blob_too_large"   // fichier au-delà du plafond de lecture
	UnknownBlobUnreadable  = "blob_unreadable"  // fichier listé mais non lu
	UnknownNoDocsDir       = "no_docs_dir"      // dossier de documentation absent
	UnknownUnreadDocuments = "unread_documents" // DOC4 : aucun lien mort parmi les documents lus, d'autres non lus
	UnknownNotComputed     = "not_computed"     // aucun résultat en cache : pas encore calculé (watchoo sync)
	UnknownStale           = "stale"            // résultat en cache d'une autre version des règles ou d'autres réglages
)

// Codes des critères de fichiers (clés --json, catalogue i18n), en anglais. Un même code garde le même sens
// d'un critère à l'autre (absent, misnamed, elsewhere pour CL1, RD1 et DOC1).
const (
	// Bloquants : le critère n'est pas conforme.
	FileAbsent    = "absent"    // fichier introuvable
	FileMisnamed  = "misnamed"  // fichier au bon endroit sous un autre nom (casse, extension)
	FileElsewhere = "elsewhere" // fichier trouvé ailleurs qu'à l'emplacement attendu

	FileNoH1             = "no_h1"             // CL2 : aucun titre de niveau 1
	FileH1Title          = "h1_title"          // CL2 : titre de niveau 1 autre que « Changelog »
	FileNoKeepAChangelog = "no_keepachangelog" // CL2 : Keep a Changelog non mentionné avant la première version
	FileNoSemVer         = "no_semver"         // CL2 : SemVer non mentionné avant la première version

	FileUnreleasedAbsent     = "unreleased_absent"      // CL3 : aucune section [Unreleased]
	FileUnreleasedMisplaced  = "unreleased_misplaced"   // CL3 : [Unreleased] n'est pas la première section
	FileUnreleasedDuplicate  = "unreleased_duplicate"   // CL3 : plusieurs sections [Unreleased]
	FileUnreleasedNoBrackets = "unreleased_no_brackets" // CL3 : « ## Unreleased » sans crochets

	FileNoBrackets     = "no_brackets"     // CL4 : version sans crochets
	FileDateFR         = "date_fr"         // CL4 : date au format français (JJ/MM/AAAA)
	FileNoDate         = "no_date"         // CL4 : version sans date
	FileInvalidDate    = "invalid_date"    // CL4 : date illisible ou hors calendrier
	FileFutureDate     = "future_date"     // CL4 : date postérieure au jour du calcul
	FileInvalidVersion = "invalid_version" // CL4 : titre de section qui n'est pas une version SemVer

	FileVersionOrder = "version_order" // CL5 : précédence SemVer non strictement décroissante
	FileDateOrder    = "date_order"    // CL5 : date plus récente que celle de la version précédente

	FileUnknownCategory       = "unknown_category"        // CL6 : catégorie hors Keep a Changelog
	FileDuplicateCategory     = "duplicate_category"      // CL6 : catégorie répétée dans une version
	FileBulletOutsideCategory = "bullet_outside_category" // CL6 : puce hors de toute catégorie

	FileEmptyVersion  = "empty_version"  // CL7 : version publiée sans entrée
	FileEmptyCategory = "empty_category" // CL7 : catégorie sans entrée
	FileEmptyBullet   = "empty_bullet"   // CL7 : puce vide ou « TODO »

	FileTagWithoutSection = "tag_without_section" // CL8 : tag de version finale sans section
	FileSectionWithoutTag = "section_without_tag" // CL8 : section datée (non retirée) sans tag

	FileVersionMissing = "version_missing" // CL9 : pas de section pour la version du jalon

	FileMissingSection = "missing_section" // RD2 : section attendue non trouvée par son titre

	FileIndexLinkAbsent     = "index_link_absent"     // RD3 : aucun lien vers l'INDEX
	FileIndexLinkDead       = "index_link_dead"       // RD3 : lien vers l'INDEX mort
	FileIndexLinkFolder     = "index_link_folder"     // RD3 : lien vers le dossier de documentation seulement
	FileChangelogLinkAbsent = "changelog_link_absent" // RD3 : aucun lien vers le CHANGELOG
	FileChangelogLinkDead   = "changelog_link_dead"   // RD3 : lien vers le CHANGELOG mort

	FileTooShort = "too_short" // RD4 : moins de mots que readme.min_words

	FileNoTable       = "no_table"       // DOC2 : aucun tableau dans l'INDEX
	FileMissingColumn = "missing_column" // DOC2 : colonne attendue absente
	FileNoRows        = "no_rows"        // DOC2 : tableau sans ligne

	FileUnreferenced = "unreferenced"  // DOC3 : document qu'aucun lien de l'INDEX ne vise
	FileDuplicateRow = "duplicate_row" // DOC3 : document visé par plusieurs lignes de l'INDEX

	FileDeadLink     = "dead_link"     // DOC4 : cible relative introuvable
	FileCaseMismatch = "case_mismatch" // DOC4 : cible trouvée avec une autre casse seulement

	FileDirName  = "dir_name"  // DOC5 : dossier hors « NN-thème »
	FileFileName = "file_name" // DOC5 : fichier hors minuscules à tirets

	FileArchive = "archive" // DOC8 : archive versionnée

	FileNotFound = "not_found" // TL1 à TL3 : non trouvé dans le dépôt

	// Avertissements : signalés, sans effet sur la conformité.
	FileFrenchLabel     = "french_label"     // libellé français admis (H1, [Non publié], catégories)
	FileVPrefix         = "v_prefix"         // CL4 : « [v1.2.0] » (tolérance de la charte)
	FileCategoryOrder   = "category_order"   // CL6 : catégories hors de l'ordre de Keep a Changelog
	FileOfficeDocument  = "office_document"  // DOC8 : .docx ou .pdf sous la documentation (information)
	FileHuskyNoPrepare  = "husky_no_prepare" // TL2 : .husky/commit-msg sans script « prepare » qui installe husky
	FileTranslationOnly = "translation_only" // RD1 : seul un README.<langue>.md existe (il ne compte pas)
)

// AllFileCodes liste les codes des critères de fichiers dans leur ordre d'affichage : les bloquants, puis
// les avertissements.
var AllFileCodes = []string{
	FileAbsent, FileMisnamed, FileElsewhere,
	FileNoH1, FileH1Title, FileNoKeepAChangelog, FileNoSemVer,
	FileUnreleasedAbsent, FileUnreleasedMisplaced, FileUnreleasedDuplicate, FileUnreleasedNoBrackets,
	FileNoBrackets, FileDateFR, FileNoDate, FileInvalidDate, FileFutureDate, FileInvalidVersion,
	FileVersionOrder, FileDateOrder,
	FileUnknownCategory, FileDuplicateCategory, FileBulletOutsideCategory,
	FileEmptyVersion, FileEmptyCategory, FileEmptyBullet,
	FileTagWithoutSection, FileSectionWithoutTag, FileVersionMissing,
	FileMissingSection, FileIndexLinkAbsent, FileIndexLinkDead, FileIndexLinkFolder, FileChangelogLinkAbsent,
	FileChangelogLinkDead, FileTooShort,
	FileNoTable, FileMissingColumn, FileNoRows, FileUnreferenced, FileDuplicateRow, FileDeadLink,
	FileCaseMismatch, FileDirName, FileFileName, FileArchive, FileNotFound,
	FileFrenchLabel, FileVPrefix, FileCategoryOrder, FileOfficeDocument, FileHuskyNoPrepare, FileTranslationOnly,
}

// FileSeverityOf renvoie la gravité d'un code de fichiers ; 0 pour un code inconnu.
func FileSeverityOf(code string) Severity {
	switch code {
	case FileFrenchLabel, FileVPrefix, FileCategoryOrder, FileOfficeDocument, FileHuskyNoPrepare,
		FileTranslationOnly:
		return SeverityWarning
	}
	if slices.Contains(AllFileCodes, code) {
		return SeverityBlocking
	}
	return 0
}

// Clés des compteurs (clés --json). Un compteur dont la clé est un code compte les éléments (versions,
// catégories, puces, liens, chemins) qui portent ce code.
const (
	CountVersions     = "versions"             // CL4, CL5 : sections de version évaluées
	CountSections     = "sections"             // CL6 : sections évaluées ([Unreleased] comprise) ; TL3 : sections de la charte trouvées
	CountTags         = "tags"                 // CL8 : tags qui désignent une version, pris en compte
	CountExpected     = "expected"             // RD2 : sections attendues ; TL3 : sections de la charte
	CountFound        = "found"                // RD2 : sections attendues trouvées
	CountFoundMask    = "found_mask"           // RD2 : masque des sections trouvées (bits dans l'ordre d'AllReadmeSections)
	CountExpectedMask = "expected_mask"        // RD2 : masque des sections attendues
	CountLicenseFile  = "license_file"         // RD2 : 1 si un fichier LICENSE (ou COPYING) est à la racine
	CountTranslations = "translations"         // RD1 : fichiers README.<langue>.md à la racine
	CountWords        = "words"                // RD4 : mots hors code
	CountMinWords     = "min_words"            // RD4 : seuil
	CountRows         = "rows"                 // DOC2 : lignes des tableaux complets
	CountColumns      = "columns"              // DOC2 : colonnes attendues trouvées dans le meilleur tableau
	CountTables       = "tables"               // DOC2 : tableaux complets
	CountDocuments    = "documents"            // DOC3 : documents .md de la documentation (hors INDEX et médias)
	CountLinks        = "links"                // DOC4 : liens relatifs vérifiés
	CountSources      = "sources"              // DOC4 : fichiers lus (README, INDEX, documents)
	CountUnread       = "unread"               // DOC4, TL : fichiers non lus (trop gros, illisibles, au-delà du plafond)
	CountDirs         = "dirs"                 // DOC5 : dossiers évalués
	CountFiles        = "files"                // DOC5 : fichiers évalués
	CountConfigs      = "configs"              // TL1 : sources de configuration commitlint trouvées
	CountExtends      = "extends_conventional" // TL1 : 1 si une configuration étend config-conventional
	CountTools        = "tools_mask"           // TL2 : masque des outils (bits dans l'ordre d'AllHookTools)
	CountTemplates    = "templates"            // TL3 : gabarits de pull request
)

// MaxDocuments plafonne le nombre de documents de la documentation lus pour DOC4 (au-delà : « non lus »).
const MaxDocuments = 300

// Réglages par défaut des critères de fichiers (documentation/fr/13-conventions.md, section 8).
const (
	DefaultReadmeMinWords      = 150
	DefaultDocsDir             = "documentation"
	DefaultMediaDir            = "Media"
	ArchivesScopeRepository    = "repository"    // archives interdites dans tout le dépôt (défaut)
	ArchivesScopeDocumentation = "documentation" // archives interdites sous la documentation seulement
)

// FileRules sont les règles des critères de fichiers, tirées des réglages conventions.changelog.*,
// conventions.readme.* et conventions.docs.*.
type FileRules struct {
	CategoriesFR   bool     // libellés français admis (avertissement french_label) ; sinon écarts
	ReadmeSections []string // clés d'AllReadmeSections attendues (RD2) ; une clé inconnue est ignorée
	ReadmeMinWords int      // seuil de RD4 ; 0 ou moins : DefaultReadmeMinWords
	DocsDir        string   // dossier de la documentation, relatif à la racine ; vide : DefaultDocsDir
	MediaDir       string   // dossier des médias sous DocsDir ; vide : DefaultMediaDir
	ArchivesScope  string   // ArchivesScopeRepository ou ArchivesScopeDocumentation ; vide : dépôt
}

// DefaultFileRules renvoie les règles de la charte.
func DefaultFileRules() FileRules {
	return FileRules{
		CategoriesFR:   true,
		ReadmeSections: AllReadmeSections(),
		ReadmeMinWords: DefaultReadmeMinWords,
		DocsDir:        DefaultDocsDir,
		MediaDir:       DefaultMediaDir,
		ArchivesScope:  ArchivesScopeRepository,
	}
}

// normalized complète les valeurs vides par celles de la charte.
func (r FileRules) normalized() FileRules {
	if r.ReadmeMinWords <= 0 {
		r.ReadmeMinWords = DefaultReadmeMinWords
	}
	r.DocsDir = strings.Trim(r.DocsDir, "/")
	if r.DocsDir == "" {
		r.DocsDir = DefaultDocsDir
	}
	if r.MediaDir == "" {
		r.MediaDir = DefaultMediaDir
	}
	if r.ArchivesScope != ArchivesScopeDocumentation {
		r.ArchivesScope = ArchivesScopeRepository
	}
	return r
}

// ValidArchivesScope indique une portée des archives admise.
func ValidArchivesScope(s string) bool {
	return s == ArchivesScopeRepository || s == ArchivesScopeDocumentation
}

// ValidDocsDir indique un dossier de documentation admis : chemin relatif à la racine, segments non vides
// séparés par « / », sans « . » ni « .. », sans caractère de contrôle, au plus 255 octets.
func ValidDocsDir(s string) bool {
	if s == "" || len(s) > 255 || !utf8.ValidString(s) || strings.HasPrefix(s, "/") || strings.HasSuffix(s, "/") {
		return false
	}
	for _, seg := range strings.Split(s, "/") {
		if !validSegment(seg) {
			return false
		}
	}
	return true
}

// ValidMediaDir indique un dossier de médias admis : un seul segment de chemin.
func ValidMediaDir(s string) bool {
	return len(s) <= 255 && utf8.ValidString(s) && validSegment(s)
}

// validSegment refuse un segment vide, « . », « .. », ou contenant « / », « \ » ou un caractère de contrôle.
func validSegment(seg string) bool {
	if seg == "" || seg == "." || seg == ".." || strings.TrimSpace(seg) != seg {
		return false
	}
	return !strings.ContainsFunc(seg, func(r rune) bool { return r == '/' || r == '\\' || unicode.IsControl(r) })
}

// Fingerprint renvoie une empreinte stable (SHA-256, hexadécimal) des règles de fichiers, du dépôt repo
// (« owner/name », sans casse) et des noms de tags qui désignent une version : un résultat en cache n'est
// réutilisable que pour la même empreinte. Le dépôt y entre parce que RD3, DOC3 et DOC4 reconnaissent les
// URL du même dépôt (un renommage ou un transfert fait recalculer) ; les tags parce que CL8 les compare aux
// sections ; l'ordre des listes et les doublons sont sans effet.
func (r FileRules) Fingerprint(repo string, tags []string) string {
	r = r.normalized()
	sections := validReadmeSections(r.ReadmeSections)
	var versions []string
	for _, name := range tags {
		if res := CheckTag(name); res.Designates {
			versions = append(versions, name)
		}
	}
	slices.Sort(versions)
	versions = slices.Compact(versions)
	var b strings.Builder
	b.WriteString("files/" + strconv.Itoa(FileRulesVersion) + "\n")
	b.WriteString("repo=" + strconv.Quote(strings.ToLower(repo)) + "\n")
	b.WriteString("categories_fr=" + strconv.FormatBool(r.CategoriesFR) + "\n")
	b.WriteString("readme.sections=" + strings.Join(sections, ",") + "\n")
	b.WriteString("readme.min_words=" + strconv.Itoa(r.ReadmeMinWords) + "\n")
	b.WriteString("docs.dir=" + strconv.Quote(r.DocsDir) + "\n")
	b.WriteString("docs.media_dir=" + strconv.Quote(r.MediaDir) + "\n")
	b.WriteString("docs.archives_scope=" + r.ArchivesScope + "\n")
	b.WriteString("tags=" + strconv.Quote(strings.Join(versions, "\x00")) + "\n")
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Blob est le contenu d'un fichier lu dans le clone.
type Blob struct {
	Data     []byte
	TooLarge bool // au-delà du plafond de lecture : Data est vide
}

// FileTree est l'arbre d'une révision : les chemins des fichiers ordinaires (ni liens symboliques ni
// sous-modules), relatifs à la racine et séparés par « / », et le contenu des fichiers lus.
type FileTree struct {
	Paths     []string
	Truncated bool            // liste tronquée au plafond : tous les critères sont inconnus
	Blobs     map[string]Blob // contenus lus, par chemin ; un chemin listé sans contenu est « illisible »
}

// FileOptions sont les entrées des critères de fichiers.
type FileOptions struct {
	Rules FileRules
	// Repo est « owner/name », pour reconnaître une URL du même dépôt (RD3) ; vide : aucune URL n'est admise.
	Repo string
	// Tags sont les noms des tags du dépôt (CL8) ; seuls ceux qui désignent une version sont lus.
	Tags []string
	// Version est le nom du tag du jalon (« v1.0.0 ») quand l'arbre est celui de ce tag : CL9 est alors
	// mesuré et CL8 ne considère que les tags de précédence inférieure ou égale. Vide pour la tête.
	Version string
	// Now situe les dates futures de CL4 (zéro : maintenant) ; le jour est lu dans son fuseau.
	Now time.Time
}

// FileCheck est l'état d'un critère de fichiers. Il ne porte ni chemin ni contenu.
type FileCheck struct {
	ID     string         `json:"id"`
	Status string         `json:"status"`           // CriterionConform, CriterionNonconform ou CriterionUnknown
	Reason string         `json:"reason,omitempty"` // raison d'un état inconnu ; vide sinon
	Codes  []string       `json:"codes"`            // dans l'ordre d'AllFileCodes ; vide non nil si aucun
	Counts map[string]int `json:"counts"`           // compteurs ; vide non nil si aucun
}

// FileResults sont les critères de fichiers d'une révision, par famille, dans l'ordre d'affichage. C'est ce
// qui est mis en cache : des identifiants, des états, des codes et des compteurs.
type FileResults struct {
	RulesVersion int         `json:"rules_version"`
	Changelog    []FileCheck `json:"changelog"`
	Readme       []FileCheck `json:"readme"`
	Docs         []FileCheck `json:"docs"`
	Tooling      []FileCheck `json:"tooling"`
}

// Find renvoie le critère id ; faux s'il n'a pas été mesuré.
func (r FileResults) Find(id string) (FileCheck, bool) {
	for _, fam := range [][]FileCheck{r.Changelog, r.Readme, r.Docs, r.Tooling} {
		if i := slices.IndexFunc(fam, func(c FileCheck) bool { return c.ID == id }); i >= 0 {
			return fam[i], true
		}
	}
	return FileCheck{}, false
}

// TallyChecks compte les critères conformes et inconnus d'une grille (« k/n critères conformes, u inconnus »).
func TallyChecks(checks []FileCheck) (conform, unknown int) {
	for _, c := range checks {
		switch c.Status {
		case CriterionConform:
			conform++
		case CriterionUnknown:
			unknown++
		}
	}
	return conform, unknown
}

// DeadLink est un lien relatif mort (DOC4).
type DeadLink struct {
	Source       string // fichier qui contient le lien
	Target       string // cible telle qu'écrite
	Resolved     string // chemin visé depuis la racine
	CaseMismatch bool   // la cible existe avec une autre casse
}

// FileLists sont les listes de chemins des critères DOC3, DOC4, DOC5 et DOC8 : recalculées pour la vue
// détaillée d'un dépôt, jamais stockées ni journalisées.
type FileLists struct {
	Unreferenced    []string   // DOC3 : documents qu'aucun lien de l'INDEX ne vise
	DuplicateRows   []string   // DOC3 : documents visés par plusieurs lignes de l'INDEX
	DeadLinks       []DeadLink // DOC4
	BadNames        []string   // DOC5 : dossiers (terminés par « / ») et fichiers mal nommés
	Archives        []string   // DOC8 : archives
	OfficeDocuments []string   // DOC8 : .docx et .pdf sous la documentation (information)
}

// CheckFiles mesure tous les critères de fichiers d'une révision.
func CheckFiles(tree FileTree, opts FileOptions) (FileResults, FileLists) {
	docs, lists := CheckDocs(tree, opts)
	return FileResults{
		RulesVersion: FileRulesVersion,
		Changelog:    CheckChangelog(tree, opts),
		Readme:       CheckReadme(tree, opts),
		Docs:         docs,
		Tooling:      DetectTooling(tree, opts),
	}, lists
}

// FileBlobPaths renvoie, triés et sans doublon, les chemins de paths dont les critères lisent le contenu :
// CHANGELOG, README, INDEX, documents .md de la documentation (au plus MaxDocuments), fichiers d'outillage
// et gabarits de pull request.
func FileBlobPaths(paths []string, rules FileRules) []string {
	rules = rules.normalized()
	idx := newTreeIndex(paths)
	var out []string
	add := func(p string) {
		if p != "" {
			out = append(out, p)
		}
	}
	add(findChangelog(idx).path)
	add(findReadme(idx).path)
	index := findIndex(idx, rules)
	add(index.path)
	out = append(out, docDocuments(idx, docsRoot(index, rules), rules, index.path, MaxDocuments)...)
	out = append(out, toolingBlobs(idx)...)
	slices.Sort(out)
	return slices.Compact(out)
}

// treeIndex donne accès aux chemins d'un arbre : fichiers, dossiers (déduits des chemins), et leur forme en
// minuscules pour signaler une différence de casse.
type treeIndex struct {
	paths      []string // triés, sans doublon
	files      map[string]bool
	dirs       map[string]bool
	lowerFiles map[string]bool
	lowerDirs  map[string]bool
}

// newTreeIndex indexe les chemins (vides, absolus ou avec « .. » ignorés).
func newTreeIndex(paths []string) *treeIndex {
	idx := &treeIndex{files: map[string]bool{}, dirs: map[string]bool{}, lowerFiles: map[string]bool{},
		lowerDirs: map[string]bool{}}
	for _, p := range paths {
		if p == "" || strings.HasPrefix(p, "/") || path.Clean(p) != p || p == "." || strings.HasPrefix(p, "../") {
			continue
		}
		if idx.files[p] {
			continue
		}
		idx.files[p] = true
		idx.lowerFiles[strings.ToLower(p)] = true
		idx.paths = append(idx.paths, p)
		for d := path.Dir(p); d != "."; d = path.Dir(d) {
			idx.dirs[d] = true
			idx.lowerDirs[strings.ToLower(d)] = true
		}
	}
	slices.Sort(idx.paths)
	return idx
}

// exists indique un fichier ou un dossier ("" ou « . » : la racine).
func (idx *treeIndex) exists(p string) bool {
	return p == "" || p == "." || idx.files[p] || idx.dirs[p]
}

// existsFold indique un fichier ou un dossier qui existe à la casse près.
func (idx *treeIndex) existsFold(p string) bool {
	l := strings.ToLower(p)
	return idx.lowerFiles[l] || idx.lowerDirs[l]
}

// under renvoie les fichiers sous le dossier dir (récursivement).
func (idx *treeIndex) under(dir string) []string {
	var out []string
	prefix := dir + "/"
	for _, p := range idx.paths {
		if strings.HasPrefix(p, prefix) {
			out = append(out, p)
		}
	}
	return out
}

// located est un fichier recherché : son chemin et l'écart d'emplacement (vide s'il est exact).
type located struct {
	path string
	code string // "", FileMisnamed ou FileElsewhere ; FileAbsent si introuvable
}

// pickShallowest choisit, parmi des chemins, le moins profond puis le premier dans l'ordre.
func pickShallowest(paths []string) string {
	best := ""
	for _, p := range paths {
		if best == "" || strings.Count(p, "/") < strings.Count(best, "/") ||
			(strings.Count(p, "/") == strings.Count(best, "/") && p < best) {
			best = p
		}
	}
	return best
}

// isVendored indique un chemin de dépendances versionnées, ignoré par les recherches de fichiers.
func isVendored(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "node_modules" || seg == "vendor" {
			return true
		}
	}
	return false
}

// readBlob renvoie le contenu d'un fichier listé, ou la raison pour laquelle il n'a pas été lu.
func (t FileTree) readBlob(p string) ([]byte, string) {
	b, ok := t.Blobs[p]
	switch {
	case !ok:
		return nil, UnknownBlobUnreadable
	case b.TooLarge:
		return nil, UnknownBlobTooLarge
	}
	return b.Data, ""
}

// treeUnknown renvoie la raison commune d'un arbre inutilisable : tronqué ou vide.
func treeUnknown(tree FileTree, idx *treeIndex) string {
	switch {
	case tree.Truncated:
		return UnknownTreeTruncated
	case len(idx.paths) == 0:
		return UnknownEmptyRepository
	}
	return ""
}

// checkSet accumule les codes et les compteurs d'un critère.
type checkSet struct {
	id     string
	codes  map[string]bool
	counts map[string]int
}

func newCheck(id string) *checkSet {
	return &checkSet{id: id, codes: map[string]bool{}, counts: map[string]int{}}
}

func (c *checkSet) add(code string) { c.codes[code] = true }

// addCount ajoute le code et compte un élément qui le porte.
func (c *checkSet) addCount(code string) {
	c.codes[code] = true
	c.counts[code]++
}

// done renvoie le critère : non conforme s'il porte un code bloquant, conforme sinon.
func (c *checkSet) done() FileCheck {
	fc := FileCheck{ID: c.id, Status: CriterionConform, Codes: []string{}, Counts: c.counts}
	for _, code := range AllFileCodes {
		if c.codes[code] {
			fc.Codes = append(fc.Codes, code)
			if FileSeverityOf(code) == SeverityBlocking {
				fc.Status = CriterionNonconform
			}
		}
	}
	return fc
}

// unknownCheck renvoie un critère inconnu pour la raison donnée.
func unknownCheck(id, reason string) FileCheck {
	return FileCheck{ID: id, Status: CriterionUnknown, Reason: reason, Codes: []string{}, Counts: map[string]int{}}
}

// unknownAll renvoie les critères ids, tous inconnus pour la raison donnée.
func unknownAll(ids []string, reason string) []FileCheck {
	out := make([]FileCheck, 0, len(ids))
	for _, id := range ids {
		out = append(out, unknownCheck(id, reason))
	}
	return out
}

var schemeRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)

// linkKind est la nature d'une cible de lien.
type linkKind int

const (
	linkSkip     linkKind = iota // ancre seule, courriel, URL d'un autre dépôt, cible vide
	linkRelative                 // chemin relatif ou absolu depuis la racine
	linkRepoURL                  // URL du même dépôt (github.com/owner/name/blob|tree|raw/<ref>/…)
)

// resolveLink résout la cible d'un lien écrit dans source : chemin depuis la racine (« » pour la racine),
// sans ancre ni requête, décodé (%20). Un chemin qui sort de la racine est rendu tel quel (« ../x »), donc
// introuvable.
func resolveLink(source, target, repo string) (string, linkKind) {
	t := strings.TrimSpace(target)
	if t == "" || strings.HasPrefix(t, "#") {
		return "", linkSkip
	}
	if schemeRe.MatchString(t) || strings.HasPrefix(t, "//") {
		p, ok := repoURLPath(t, repo)
		if !ok {
			return "", linkSkip
		}
		return p, linkRepoURL
	}
	if i := strings.IndexAny(t, "#?"); i >= 0 {
		t = t[:i]
	}
	if t == "" {
		return "", linkSkip
	}
	t = percentDecode(t)
	var p string
	if strings.HasPrefix(t, "/") {
		p = path.Clean(strings.TrimLeft(t, "/"))
	} else {
		p = path.Join(path.Dir(source), t)
	}
	if p == "." || p == "/" {
		p = ""
	}
	return p, linkRelative
}

// repoURLPath lit le chemin d'une URL github.com du dépôt repo (« owner/name ») : …/blob|tree|raw/<ref>/<chemin>
// (la référence est un seul segment) ou la page du dépôt (racine).
func repoURLPath(u, repo string) (string, bool) {
	const prefix = "https://github.com/"
	if repo == "" || len(u) < len(prefix) || !strings.EqualFold(u[:len(prefix)], prefix) {
		return "", false
	}
	tail := u[len(prefix):]
	if i := strings.IndexAny(tail, "#?"); i >= 0 {
		tail = tail[:i]
	}
	segs := strings.Split(strings.Trim(tail, "/"), "/")
	owner, name, okRepo := strings.Cut(repo, "/")
	if !okRepo || len(segs) < 2 || !strings.EqualFold(segs[0], owner) ||
		!strings.EqualFold(strings.TrimSuffix(segs[1], ".git"), name) {
		return "", false
	}
	if len(segs) == 2 {
		return "", true
	}
	if len(segs) < 4 || (segs[2] != "blob" && segs[2] != "tree" && segs[2] != "raw") {
		return "", false
	}
	p := path.Clean(percentDecode(strings.Join(segs[4:], "/")))
	if p == "." {
		p = ""
	}
	return p, true
}

// percentDecode décode les séquences %XX ; une séquence invalide reste telle quelle.
func percentDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) {
			if v, err := strconv.ParseUint(s[i+1:i+3], 16, 8); err == nil {
				b.WriteByte(byte(v))
				i += 2
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
