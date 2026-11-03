package sync

// Rapport d'une synchro et événements de progression, communs à la CLI, au TUI et plus tard au web.
// Ils ne contiennent ni token, ni message de commit, ni nom ou e-mail de personne.

import (
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
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

// DeferReason est la raison du report d'un dépôt (StatusDeferred). Valeurs stables : les couches
// d'affichage les traduisent (Text) au lieu d'analyser le message RepoReport.Error.
type DeferReason string

// Valeurs de DeferReason.
const (
	DeferLowQuota       DeferReason = "low_quota"       // quota restant sous QuotaReservePercent
	DeferQuotaExhausted DeferReason = "quota_exhausted" // quota épuisé (RateLimitError)
)

// Text renvoie la raison dans la langue du processus : « low quota » / « quota bas », « quota
// exhausted » / « quota épuisé » ; une raison vide ou inconnue donne celle du quota bas.
func (r DeferReason) Text() string {
	if r == DeferQuotaExhausted {
		return i18n.T("sync.reason.quota_exhausted")
	}
	return i18n.T("sync.reason.low_quota")
}

// RepoReport est le résultat de la synchro d'un dépôt.
type RepoReport struct {
	RepoID          int64         // identifiant interne
	FullName        string        // owner/name ACTUELS (après RepositoryByID en cas de renommage)
	DisplayName     string        // label, sinon owner/name
	Status          RepoStatus    // résultat
	NewCommits      int           // commits insérés
	TrackedBranches int           // branches suivies après la synchro (0 si échec)
	StatsFetched    int           // statistiques calculées par le clone, ou appels CommitStats (repli sur l'API)
	PullRequests    int           // pull requests lues (nouvelles ou modifiées depuis la synchro précédente)
	Duration        time.Duration // durée de la synchro du dépôt (0 si ignoré ou reporté)
	// Error est le message d'échec (StatusError), « already being synced » (StatusLocked) ou
	// « deferred: low quota until HH:MM UTC » (StatusDeferred, heure non convertie : voir ResumeAt et
	// DeferReason), dans la langue du processus lors de la synchro ; vide pour un succès.
	Error    string
	ResumeAt time.Time // StatusDeferred : moment où réessayer (UTC) ; zéro sinon
	// DeferReason est la raison du report (StatusDeferred) ; vide sinon. Les couches d'affichage s'en
	// servent plutôt que d'analyser Error.
	DeferReason DeferReason
	// Warnings sont les avertissements d'un dépôt réussi (StatusSuccess) : pull requests, releases ou
	// tags non synchronisés (« releases not synced: … »), clone Git local indisponible
	// (« local Git clone unavailable, line statistics read through the API (limited): … »)…, avec leur
	// code (Kind) et leur message dans la langue du processus, sans donnée personnelle ; nil sinon.
	// Ils ne changent pas le statut du dépôt et sont aussi enregistrés dans sync_runs.warnings.
	Warnings []model.SyncWarning
}

// WarningMessages renvoie les messages des avertissements du dépôt, dans l'ordre ; liste vide non nil
// si aucun.
func (r RepoReport) WarningMessages() []string {
	msgs := make([]string, 0, len(r.Warnings))
	for _, w := range r.Warnings {
		msgs = append(msgs, w.Message)
	}
	return msgs
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
	// Source est la source utilisée (API GitHub ou clones Git locaux, V2) ; valeur zéro si la synchro
	// n'a pas pu commencer.
	Source Source
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
