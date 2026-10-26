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
// Unicité : (Provider, ProviderID) et (Provider, Owner, Name). Owner et Name sont comparés sans tenir
// compte de la casse, comme sur GitHub.
type Repository struct {
	ID            int64     // identifiant interne
	Provider      string    // forge d'origine (ProviderGitHub au MVP)
	ProviderID    int64     // identifiant numérique du dépôt sur la forge, qui ne change jamais
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
	Enabled       bool // suivi actif (true) ou suspendu (false)
}

// FullName renvoie « owner/name ».
func (r Repository) FullName() string {
	return r.Owner + "/" + r.Name
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
