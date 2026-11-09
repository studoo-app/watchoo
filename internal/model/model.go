// Package model définit les structs métier de Watchoo, indépendantes du stockage (sqlc) et de la forge
// (GitHub). Elles reprennent les tables décrites dans documentation/fr/04-modele-donnees.md.
//
// Conventions communes à tous les types :
//   - les dates sont des time.Time en UTC, à la seconde ; une date absente (colonne NULL en base) est
//     la valeur zéro de time.Time (tester avec IsZero) ;
//   - une chaîne facultative absente est la chaîne vide ;
//   - un identifiant interne facultatif absent vaut 0 (SQLite n'attribue jamais l'identifiant 0).
package model

import (
	"cmp"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
)

// ProviderGitHub est l'identifiant de la forge GitHub, seule forge du MVP (colonnes provider).
const ProviderGitHub = "github"

// Repository est un dépôt suivi (table repositories).
//
// Unicité : (Provider, ProviderID) quand ProviderID est connu, et (Provider, Owner, Name). Owner et Name
// sont comparés sans tenir compte de la casse, comme sur GitHub.
type Repository struct {
	ID       int64  // identifiant interne
	Provider string // forge d'origine (ProviderGitHub au MVP)
	// ProviderID est l'identifiant numérique du dépôt sur la forge, qui ne change jamais ; 0 = pas
	// encore connu (dépôt ajouté et synchronisé par clone Git local, V2 : NULL en base), résolu à la
	// première synchro par l'API GitHub.
	ProviderID    int64
	Owner         string    // propriétaire (utilisateur ou organisation)
	Name          string    // nom du dépôt
	Label         string    // nom affiché facultatif ; vide = Owner/Name (voir DisplayName)
	DefaultBranch string    // branche par défaut ; vide tant que le dépôt n'a pas été synchronisé
	URL           string    // URL du dépôt sur la forge
	Stars         int       // nombre d'étoiles
	OpenIssues    int       // nombre d'issues ouvertes
	LastSyncedAt  time.Time // dernière synchronisation ; zéro = jamais synchronisé
	LastCommitAt  time.Time // dernier commit connu, toutes branches suivies confondues ; zéro = aucun
	Archived      bool      // dépôt archivé sur la forge
	// FlemmeAfterDays est le seuil « Flemme » propre au dépôt, en jours ; nil = valeur globale
	// (activity.flemme_after_days).
	FlemmeAfterDays *int
	// DeadAfterDays est le seuil « Dead » propre au dépôt, en jours ; nil = valeur globale
	// (activity.dead_after_days).
	DeadAfterDays *int
	// Enabled indique un suivi actif (true) ou arrêté (false, « watchoo repo remove --keep-history » :
	// l'historique reste en base, le dépôt n'est plus synchronisé ; « watchoo repo add » le réactive).
	Enabled bool
	// BranchPatterns liste les noms et motifs de branches à suivre (syntaxe de path.Match, ex.
	// « release/* »), dans l'ordre donné par l'utilisateur ; nil ou vide = toutes les branches du dépôt.
	// En base : colonne branch_patterns, valeurs séparées par des virgules (un motif ne contient donc
	// jamais de virgule).
	BranchPatterns []string
	// ConventionsSince est la date de début des conventions propre au dépôt, date du calendrier
	// AAAA-MM-JJ (interprétée à minuit dans le fuseau de l'espace) ; vide = date de l'espace
	// (conventions.since). En base : colonne conventions_since, minuit UTC de la date au format fixe.
	ConventionsSince string
}

// FullName renvoie « owner/name ».
func (r Repository) FullName() string {
	return r.Owner + "/" + r.Name
}

// ForgeDetails indique si le dépôt a déjà été lu par l'API de la forge (ProviderID connu) : étoiles,
// issues, état archivé, pull requests et releases n'ont de sens qu'alors. Un dépôt suivi seulement par
// clone Git local (V2) ne les a jamais eus : les écrans les masquent.
func (r Repository) ForgeDetails() bool {
	return r.ProviderID > 0
}

// DisplayName renvoie le nom à afficher : Label s'il est renseigné, sinon « owner/name ».
func (r Repository) DisplayName() string {
	if r.Label != "" {
		return r.Label
	}
	return r.FullName()
}

// Branch est une branche d'un dépôt (table branches). Unicité : (RepoID, Name).
//
// Une branche supprimée sur la forge n'est pas effacée : elle passe à Tracked = false et son historique
// reste en base.
type Branch struct {
	ID      int64  // identifiant interne
	RepoID  int64  // dépôt parent
	Name    string // nom de la branche
	HeadSHA string // SHA du dernier commit connu sur la branche ; vide = jamais synchronisée
	Tracked bool   // branche suivie ou non
}

// Contributor est une personne identifiée (table contributors).
//
// Unicité : (Provider, ProviderUserID) quand un compte de forge est connu. Une personne connue seulement
// par son e-mail Git a Provider vide et ProviderUserID à 0 (NULL en base).
type Contributor struct {
	ID             int64  // identifiant interne
	Provider       string // forge du compte ; vide si aucun compte n'est connu
	ProviderUserID int64  // identifiant numérique du compte sur la forge ; 0 si aucun compte n'est connu
	ProviderLogin  string // login sur la forge, s'il est connu (mis à jour s'il change)
	Name           string // nom affiché
	AvatarURL      string // URL de l'avatar
	// ProfileURL est la page de profil du compte sur la forge (html_url sur GitHub), donnée par le
	// provider ; vide si aucun compte n'est connu ou si elle n'a pas encore été lue (0.5.0).
	ProfileURL string
	// Emails liste les adresses e-mail Git rattachées (table contributor_emails), en minuscules.
	// Elle n'est remplie que par les fonctions qui le précisent ; nil sinon.
	Emails []string
}

// DisplayName renvoie le nom à afficher d'un contributeur : Name s'il est renseigné, sinon
// ProviderLogin, sinon « inconnu » (« unknown » en anglais : langue du processus).
func (c Contributor) DisplayName() string {
	switch {
	case c.Name != "":
		return c.Name
	case c.ProviderLogin != "":
		return c.ProviderLogin
	default:
		return i18n.T("model.contributor.unknown")
	}
}

// Commit est un commit d'un dépôt (table commits, table centrale). Clé : (RepoID, SHA).
//
// Un commit est enregistré une seule fois par dépôt, quel que soit le nombre de branches qui le
// contiennent (voir la table commit_branches).
type Commit struct {
	RepoID      int64     // dépôt
	SHA         string    // identifiant du commit
	AuthorID    int64     // auteur (contributeur) ; 0 = inconnu
	CommitterID int64     // committer (contributeur) ; 0 = inconnu
	Message     string    // message de commit (donnée personnelle : jamais dans les logs)
	AuthoredAt  time.Time // date d'écriture, UTC
	CommittedAt time.Time // date de commit, UTC
	Additions   int       // lignes ajoutées (0 si inconnues)
	Deletions   int       // lignes supprimées (0 si inconnues)
	StatsKnown  bool      // Additions et Deletions calculées (même à 0) ; faux = inconnues
	URL         string    // lien vers le commit sur la forge
	// ParentCount est le nombre de parents (0 = racine, 1 = ordinaire, 2 ou plus = fusion) ; nil =
	// inconnu (commit enregistré avant la migration 00016, pas encore rattrapé par le clone).
	ParentCount *int
}

// ConventionCommit est un commit lu pour les conventions (phase 8.1, internal/conventions) : le message
// n'est lu qu'en mémoire pour être analysé, jamais gardé ni affiché ni journalisé.
type ConventionCommit struct {
	SHA             string
	Message         string    // message de commit (donnée personnelle)
	ParentCount     *int      // nombre de parents ; nil = inconnu (commits.parent_count NULL)
	AuthoredAt      time.Time // date d'écriture, UTC
	AuthorID        int64     // auteur (contributeur) ; 0 = inconnu
	AuthorName      string    // nom de l'auteur ; vide si inconnu
	AuthorLogin     string    // login de forge de l'auteur ; vide si inconnu
	AuthorEmails    []string  // adresses rattachées à l'auteur
	CommitterLogin  string    // login de forge du committer ; vide si inconnu
	CommitterEmails []string  // adresses rattachées au committer
	OnTracked       bool      // rattaché à une branche suivie (commit_branches)
	OnDefault       bool      // rattaché à la branche par défaut du dépôt (commit_branches)
}

// ConventionPullRequest est une pull request lue pour les conventions (phase 8.2, internal/conventions) :
// le titre n'est lu qu'en mémoire pour être analysé, jamais gardé ni journalisé. L'auteur est rapproché
// d'un contributeur par son compte de forge (pull_requests.author_provider_id = contributors.provider_user_id).
type ConventionPullRequest struct {
	PullRequest
	AuthorID   int64  // contributeur de l'auteur ; 0 = aucun contributeur de ce compte
	AuthorName string // nom du contributeur ; vide si inconnu
}

// AISignature est une signature d'outil d'IA déclarée dans un commit (commit_ai_signatures) : elle dit
// qu'un outil est déclaré sur le commit, pas qui a écrit le code ; son absence ne dit rien.
type AISignature struct {
	Tool string // identifiant de l'outil (aisignature.ToolName donne le nom affiché)
	Kind string // type de signature (agent_account, coauthor_trailer, footer)
}

// AIToolCount donne le nombre de commits signés par un outil.
type AIToolCount struct {
	Tool    string // identifiant de l'outil
	Commits int    // commits distincts qui portent sa signature
}

// AITotals résume les signatures d'IA d'un ensemble de commits (un contributeur).
type AITotals struct {
	Commits int           // commits de l'ensemble
	Scanned int           // commits déjà analysés
	Signed  int           // commits avec au moins une signature
	Tools   []AIToolCount // outils, du plus de commits au moins ; vide non nil
}

// Known indique si au moins un commit a été analysé (sinon les totaux s'affichent « — »).
func (a AITotals) Known() bool { return a.Scanned > 0 }

// LanguageLines donne les lignes ajoutées dans un langage par un ensemble de commits (1.2.0).
type LanguageLines struct {
	Language string // nom du langage, comme dans RepositoryLanguage
	Lines    int    // lignes ajoutées (au moins 1)
}

// LanguageTotals résume les langages d'un ensemble de commits (un contributeur, 1.2.0) : lignes ajoutées
// par langage, lues dans le clone Git local.
type LanguageTotals struct {
	Commits int             // commits de l'ensemble
	Scanned int             // commits dont les langages sont lus (commits.languages_version > 0)
	Lines   []LanguageLines // du plus de lignes au moins, puis par nom ; vide non nil
}

// Known indique si au moins un commit a été analysé (sinon les langages s'affichent « — »).
func (t LanguageTotals) Known() bool { return t.Scanned > 0 }

// Pending renvoie le nombre de commits dont les langages ne sont pas encore lus.
func (t LanguageTotals) Pending() int { return max(t.Commits-t.Scanned, 0) }

// Total renvoie les lignes ajoutées dans tous les langages.
func (t LanguageTotals) Total() int {
	n := 0
	for _, l := range t.Lines {
		n += l.Lines
	}
	return n
}

// Main renvoie le langage qui a le plus de lignes ajoutées ; vide si aucun.
func (t LanguageTotals) Main() string {
	if len(t.Lines) == 0 {
		return ""
	}
	return t.Lines[0].Language
}

// Share renvoie la part de l dans les lignes ajoutées de t, en pour mille (voir Permille).
func (t LanguageTotals) Share(l LanguageLines) int { return Permille(int64(l.Lines), int64(t.Total())) }

// SortedLanguageLines convertit des lignes ajoutées par langage, triées comme LanguageTotals.Lines (vide
// non nil ; langages sans ligne ignorés).
func SortedLanguageLines(lines map[string]int) []LanguageLines {
	out := make([]LanguageLines, 0, len(lines))
	for lang, n := range lines {
		if n > 0 {
			out = append(out, LanguageLines{Language: lang, Lines: n})
		}
	}
	slices.SortFunc(out, func(a, b LanguageLines) int {
		return cmp.Or(cmp.Compare(b.Lines, a.Lines), cmp.Compare(a.Language, b.Language))
	})
	return out
}

// LineTotals additionne les lignes ajoutées et supprimées d'un ensemble de commits (un dépôt, l'espace).
// Seuls les commits dont les statistiques sont connues (commits.stats_known) sont additionnés.
type LineTotals struct {
	Commits      int // commits de l'ensemble
	KnownCommits int // commits dont les statistiques sont connues
	Additions    int // lignes ajoutées par les commits connus
	Deletions    int // lignes supprimées par les commits connus
}

// Known indique si au moins un commit a des statistiques connues (sinon les totaux s'affichent « — »).
func (l LineTotals) Known() bool { return l.KnownCommits > 0 }

// Uncounted renvoie le nombre de commits dont les statistiques sont encore inconnues.
func (l LineTotals) Uncounted() int { return max(l.Commits-l.KnownCommits, 0) }

// Add renvoie la somme de l et o.
func (l LineTotals) Add(o LineTotals) LineTotals {
	return LineTotals{Commits: l.Commits + o.Commits, KnownCommits: l.KnownCommits + o.KnownCommits,
		Additions: l.Additions + o.Additions, Deletions: l.Deletions + o.Deletions}
}

// AuthorCommitTime est l'auteur et la date de commit d'un commit dont l'auteur est connu : une
// lecture de la table commits (pas une table), d'où sont calculées les séries d'activité par
// contributeur (activity.CountContributorWeeks).
type AuthorCommitTime struct {
	ContributorID int64     // auteur (contributors.id)
	CommittedAt   time.Time // date du commit, UTC
}

// RepoCommitTime est le dépôt et la date de commit d'un commit : une lecture de la table commits (pas
// une table), d'où sont calculées les séries d'activité d'un contributeur par dépôt
// (activity.CountRepoWeeks, page d'un contributeur).
type RepoCommitTime struct {
	RepoID      int64     // dépôt
	CommittedAt time.Time // date du commit, UTC
}

// SyncStatus est le résultat d'une synchronisation (colonne sync_runs.status).
type SyncStatus string

// Valeurs possibles de SyncStatus.
const (
	SyncRunning SyncStatus = "running" // synchro en cours
	SyncSuccess SyncStatus = "success" // synchro terminée sans erreur
	SyncError   SyncStatus = "error"   // synchro terminée en erreur (voir SyncRun.Error)
)

// SyncRun est une entrée du journal des synchronisations (table sync_runs).
type SyncRun struct {
	ID         int64      // identifiant interne
	RepoID     int64      // dépôt synchronisé
	StartedAt  time.Time  // début, UTC
	FinishedAt time.Time  // fin, UTC ; zéro = synchro en cours
	NewCommits int        // nombre de nouveaux commits récupérés
	Status     SyncStatus // résultat
	Error      string     // message d'erreur éventuel ; vide si aucun (jamais de token dedans)
	// Warnings sont les avertissements d'une synchro réussie (1.3.0), dans l'ordre où ils sont apparus ;
	// liste vide si aucun.
	Warnings []SyncWarning
}

// SyncWarningKind est le code stable d'un avertissement de synchro (colonne sync_runs.warnings) : les
// couches d'affichage le traduisent au lieu d'analyser le message.
type SyncWarningKind string

// Valeurs de SyncWarningKind.
const (
	WarnStatsNotComputed     SyncWarningKind = "stats_not_computed"     // dépôt Git local : clone illisible, lignes non calculées
	WarnCloneUnavailable     SyncWarningKind = "clone_unavailable"      // clone illisible, lignes lues par l'API (limitées)
	WarnLanguagesNotComputed SyncWarningKind = "languages_not_computed" // langages non lus dans le clone
	WarnListsNotSynced       SyncWarningKind = "lists_not_synced"       // pull requests, releases ou tags non lus
	WarnAINotScanned         SyncWarningKind = "ai_not_scanned"         // signatures d'IA non analysées
	// WarnDiagnosticNotRecorded : signaux de diagnostic non enregistrés (phase 7.1, diagnostic activé).
	WarnDiagnosticNotRecorded SyncWarningKind = "diagnostic_not_recorded"
)

// SyncWarning est un avertissement de synchro : un dépôt réussi, avec une donnée manquante ou limitée.
type SyncWarning struct {
	Kind SyncWarningKind `json:"kind"`
	// Message est le texte complet, dans la langue du processus lors de la synchro, sans token ni
	// donnée personnelle.
	Message string `json:"message"`
}

// États d'une pull request (colonne pull_requests.state), comme sur la forge. Une PR mergée est fermée
// (PullRequestClosed) avec une date de merge.
const (
	PullRequestOpen   = "open"
	PullRequestClosed = "closed"
)

// PullRequestStatus est l'état affiché d'une pull request. Valeurs stables (sorties --json, option
// --state de « watchoo prs »).
type PullRequestStatus string

// Valeurs de PullRequestStatus.
const (
	PullRequestStatusOpen   PullRequestStatus = "open"   // ouverte (brouillon compris)
	PullRequestStatusMerged PullRequestStatus = "merged" // mergée
	PullRequestStatusClosed PullRequestStatus = "closed" // fermée sans merge
)

// PullRequest est une pull request d'un dépôt (table pull_requests, V2). Clé : (RepoID, Number).
//
// L'auteur est gardé tel quel (login et identifiant du compte de forge), sans lien vers un
// contributeur : un contributeur est l'auteur d'au moins un commit.
type PullRequest struct {
	RepoID           int64     // dépôt
	Number           int       // numéro dans le dépôt
	Title            string    // titre (texte venu du dépôt : jamais dans les logs)
	State            string    // PullRequestOpen ou PullRequestClosed
	Merged           bool      // mergée (fermée avec MergedAt)
	Draft            bool      // brouillon
	AuthorLogin      string    // login de l'auteur sur la forge ; vide si inconnu
	AuthorProviderID int64     // identifiant du compte de l'auteur ; 0 si inconnu
	URL              string    // lien vers la PR sur la forge
	BaseBranch       string    // branche visée
	HeadBranch       string    // branche proposée
	CreatedAt        time.Time // ouverture, UTC
	UpdatedAt        time.Time // dernière modification, UTC
	ClosedAt         time.Time // fermeture, UTC ; zéro si ouverte
	MergedAt         time.Time // merge, UTC ; zéro si non mergée
	// ToolIDs sont les outils déclarés dans la description (signal D2 du diagnostic, phase 7.2),
	// encodés « D2:claude », triés ; vide si aucun ou non mesuré. La description n'est jamais gardée.
	ToolIDs []string
}

// Status renvoie l'état affiché : mergée, fermée sans merge ou ouverte.
func (p PullRequest) Status() PullRequestStatus {
	switch {
	case p.Merged:
		return PullRequestStatusMerged
	case p.State == PullRequestClosed:
		return PullRequestStatusClosed
	default:
		return PullRequestStatusOpen
	}
}

// Release est une release d'un dépôt (table releases, V2). Clé : (RepoID, ProviderID).
type Release struct {
	RepoID      int64     // dépôt
	ProviderID  int64     // identifiant de la release sur la forge
	TagName     string    // tag
	Name        string    // titre ; vide si aucun
	Draft       bool      // brouillon
	Prerelease  bool      // pré-release
	AuthorLogin string    // login de l'auteur ; vide si inconnu
	URL         string    // lien vers la release sur la forge
	CreatedAt   time.Time // création, UTC
	PublishedAt time.Time // publication, UTC ; zéro pour un brouillon
}

// Date renvoie la date affichée d'une release : sa publication, sinon sa création (brouillon).
func (r Release) Date() time.Time {
	if !r.PublishedAt.IsZero() {
		return r.PublishedAt
	}
	return r.CreatedAt
}

// Tag est un tag d'un dépôt (table tags, V2). Clé : (RepoID, Name).
type Tag struct {
	RepoID int64  // dépôt
	Name   string // nom du tag
	SHA    string // SHA du commit visé
}

// CommitSignal sont les signaux de diagnostic d'un commit (table commit_signals, phase 7.1), mesurés
// à la synchro par internal/diagnostic. Ni message, ni e-mail, ni chemin de fichier. Clé : (RepoID, SHA).
type CommitSignal struct {
	RepoID        int64     // dépôt
	SHA           string    // commit
	AuthorID      int64     // auteur (contributeur) ; 0 = inconnu
	AuthoredAt    time.Time // date d'écriture du commit, UTC
	FilesKept     int       // fichiers retenus (ni binaires ni exclus)
	FilesExcluded int       // fichiers binaires ou exclus par diagnostic.exclude
	AdditionsKept int       // lignes ajoutées dans les fichiers retenus
	DeletionsKept int       // lignes supprimées dans les fichiers retenus
	Tools         []string  // outils déclarés (signaux D1 et D2), triés ; vide si aucun
	AgentConfig   bool      // signal D4 : le commit modifie un fichier de configuration d'agent
	BotAuthor     bool      // signal D3 : auteur bot connu
	ComputedAt    time.Time // date du calcul, UTC
	Form          CommitForm
}

// CommitForm sont les mesures de forme d'un commit (signaux F1 à F5, phase 7.5), sans aucun contenu.
type CommitForm struct {
	Known               bool    // mesures présentes (sinon toutes nulles)
	SourceLines         int     // lignes ajoutées non vides des fichiers source (tests compris)
	CommentLines        int     // lignes de commentaire ou de docstring parmi elles
	Functions           int     // définitions de fonctions ajoutées
	DocumentedFunctions int     // définitions documentées
	Identifiers         int     // identifiants du code ajouté
	LexicalDiversity    float64 // MATTR des identifiants ; voir HasDiversity
	HasDiversity        bool    // au moins 50 identifiants
	TestLines           int     // lignes ajoutées non vides des fichiers de test
	MessageLength       int     // caractères du message, hors trailers et signatures
	MessageBullets      int     // lignes en liste du message
	MessageConventional bool    // sujet au format Conventional Commits

	// Phase 7.7 : signaux absolus (A) et contre-indicateurs (H).
	DecorativeLines   int  // lignes de commentaire décoratives
	FrWords           int  // mots français accentués courants repérés dans les commentaires
	FrWordsUnaccented int  // parmi eux, écrits sans accents
	WIPMarkers        int  // marqueurs de travail en cours (TODO, FIXME, debug…)
	ShortIdentifiers  int  // identifiants d'une lettre
	PrivateDefs       int  // définitions privées
	MessageRevert     bool // message de « git revert »
	MessageFix        bool // sujet de correction
}

// DiagnosticReview est la revue humaine d'une piste de diagnostic (table diagnostic_reviews, phase 7.4).
// Clé : (RepoID, LeadKey). Note est une donnée personnelle (jamais dans les logs).
type DiagnosticReview struct {
	RepoID     int64
	LeadKey    string    // clé stable de la piste (internal/diagnostic, Lead.Key)
	Kind       string    // commit, pull_request ou day
	LeadDate   time.Time // date de la piste (conservation), UTC
	Signals    []string  // identifiants des signaux présents lors de la revue, triés
	Decision   string    // assisted, not_assisted ou undetermined
	Cause      string    // cause si non assisté ; vide sinon
	Confidence string    // low, medium ou high
	Note       string    // note libre du relecteur
	Reviewer   string    // relecteur (compte de l'interface web, sinon nom de l'espace)
	ReviewedAt time.Time // date de la revue, UTC
}

// RepositoryLanguage est un langage d'un dépôt, sur sa branche par défaut (1.1.0).
type RepositoryLanguage struct {
	RepoID   int64  // dépôt
	Language string // nom du langage
	Bytes    int64  // taille de ses fichiers, en octets (> 0)
}

// Permille renvoie part/total en pour mille, arrondi au plus proche (624 = 62,4 %) ; 0 si total <= 0.
func Permille(part, total int64) int {
	if total <= 0 {
		return 0
	}
	return int((part*1000 + total/2) / total)
}
