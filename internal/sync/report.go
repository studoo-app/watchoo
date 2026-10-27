package sync

// Rapport d'une synchro et événements de progression, communs à la CLI, au TUI et plus tard au web.
// Ils ne contiennent ni token, ni message de commit, ni nom ou e-mail de personne.

import (
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
)

// RepoStatus est le résultat de la synchro d'un dépôt. Valeurs stables (sorties --json).
type RepoStatus string

// Valeurs de RepoStatus.
const (
	StatusSuccess  RepoStatus = "success"  // synchronisé
	StatusError    RepoStatus = "error"    // échec, enregistré dans sync_runs (voir RepoReport.Error)
	StatusLocked   RepoStatus = "locked"   // ignoré : « déjà en cours de synchronisation »
	StatusDeferred RepoStatus = "deferred" // reporté : quota bas ou épuisé, jusqu'à ResumeAt
)

// RepoReport est le résultat de la synchro d'un dépôt.
type RepoReport struct {
	RepoID          int64         // identifiant interne
	FullName        string        // owner/name ACTUELS (après RepositoryByID en cas de renommage)
	DisplayName     string        // label, sinon owner/name
	Status          RepoStatus    // résultat
	NewCommits      int           // commits insérés
	TrackedBranches int           // branches suivies après la synchro (0 si échec)
	StatsFetched    int           // appels CommitStats faits
	PullRequests    int           // pull requests lues (nouvelles ou modifiées depuis la synchro précédente)
	Duration        time.Duration // durée de la synchro du dépôt (0 si ignoré ou reporté)
	// Error est le message d'échec en français (StatusError), « déjà en cours de synchronisation »
	// (StatusLocked) ou « reporté : quota bas jusqu'à HH:MM » (StatusDeferred, heure non convertie :
	// voir ResumeAt) ; vide pour un succès.
	Error    string
	ResumeAt time.Time // StatusDeferred : moment où réessayer (UTC) ; zéro sinon
	// Warnings sont les avertissements d'un dépôt réussi (StatusSuccess) : pull requests, releases ou
	// tags non synchronisés (« releases non synchronisées : … »), en français, sans donnée personnelle ;
	// nil sinon. Ils ne changent pas le statut du dépôt.
	Warnings []string
}

// Warnings renvoie le nombre total d'avertissements du rapport.
func (r Report) Warnings() int {
	n := 0
	for _, repo := range r.Repos {
		n += len(repo.Warnings)
	}
	return n
}

// Report est le rapport d'une synchro (SyncAll ou SyncRepo).
type Report struct {
	StartedAt  time.Time    // début (UTC)
	FinishedAt time.Time    // fin (UTC)
	Repos      []RepoReport // un par dépôt, dans l'ordre de ListRepositories
	// RateLimit est le dernier état du quota observé en fin de synchro ; RateLimitKnown faux si aucun.
	RateLimit      provider.RateLimit
	RateLimitKnown bool
}

// Count renvoie le nombre de dépôts du rapport ayant le statut s.
func (r Report) Count(s RepoStatus) int {
	n := 0
	for _, repo := range r.Repos {
		if repo.Status == s {
			n++
		}
	}
	return n
}

// NewCommits renvoie le total des nouveaux commits du rapport.
func (r Report) NewCommits() int {
	n := 0
	for _, repo := range r.Repos {
		n += repo.NewCommits
	}
	return n
}

// EventKind est le type d'un événement de progression.
type EventKind string

// Valeurs de EventKind.
const (
	EventStarted      EventKind = "started"       // début de la synchro : Total connu
	EventRepoStarted  EventKind = "repo_started"  // début d'un dépôt : Repo.RepoID, FullName, DisplayName
	EventRepoFinished EventKind = "repo_finished" // fin d'un dépôt (succès, erreur, verrou ou report) : Repo complet
	EventFinished     EventKind = "finished"      // fin de la synchro (envoyé aussi en cas d'erreur globale)
)

// Event est un événement de progression (SyncOptions.Progress).
type Event struct {
	Kind  EventKind
	Repo  RepoReport // dépôt concerné (EventRepoStarted, EventRepoFinished)
	Done  int        // dépôts terminés (EventRepoFinished compris)
	Total int        // dépôts à traiter
}
