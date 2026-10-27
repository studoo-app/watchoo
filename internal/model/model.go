// Package model définit les structs métier de Watchoo, indépendantes du stockage (sqlc) et de la forge
// (GitHub). Elles reprennent les tables décrites dans documentation/04-modele-donnees.md.
//
// Conventions communes à tous les types :
//   - les dates sont des time.Time en UTC, à la seconde ; une date absente (colonne NULL en base) est
//     la valeur zéro de time.Time (tester avec IsZero) ;
//   - une chaîne facultative absente est la chaîne vide ;
//   - un identifiant interne facultatif absent vaut 0 (SQLite n'attribue jamais l'identifiant 0).
package model

import "time"

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
	// Emails liste les adresses e-mail Git rattachées (table contributor_emails), en minuscules.
	// Elle n'est remplie que par les fonctions qui le précisent ; nil sinon.
	Emails []string
}

// DisplayName renvoie le nom à afficher d'un contributeur : Name s'il est renseigné, sinon
// ProviderLogin, sinon « inconnu ».
func (c Contributor) DisplayName() string {
	switch {
	case c.Name != "":
		return c.Name
	case c.ProviderLogin != "":
		return c.ProviderLogin
	default:
		return "inconnu"
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
	Additions   int       // lignes ajoutées
	Deletions   int       // lignes supprimées
	URL         string    // lien vers le commit sur la forge
}

// AuthorCommitTime est l'auteur et la date de commit d'un commit dont l'auteur est connu : une
// lecture de la table commits (pas une table), d'où sont calculées les séries d'activité par
// contributeur (activity.CountContributorWeeks).
type AuthorCommitTime struct {
	ContributorID int64     // auteur (contributors.id)
	CommittedAt   time.Time // date du commit, UTC
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
