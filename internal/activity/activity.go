// Package activity applique les règles d'activité des dépôts (documentation/02-fonctionnalites.md,
// « Règles fonctionnelles ») : états Actif / Flemme / Dead calculés sur l'âge du dernier commit, et
// états gris prioritaires (suivi arrêté, en erreur, jamais synchronisé, archivé, vide). Il fournit
// aussi les formats courts communs à la CLI et au TUI (état « ~ 9 j », date relative « il y a 2 h »,
// durée « 7d »).
//
// Package pur : aucune entrée-sortie, aucune horloge implicite (now et le fuseau sont passés).
package activity

import (
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// State est l'état d'un dépôt. Les valeurs sont stables : elles figurent dans les sorties --json.
type State string

// Valeurs de State. Ordre de priorité de Evaluate : Stopped, Error, NeverSynced, Archived, Empty, puis
// Active / Flemme / Dead.
const (
	StateActive      State = "active"       // dernier commit il y a 0 à flemme_after_days jours (inclus)
	StateFlemme      State = "flemme"       // au-delà, jusqu'à dead_after_days jours (inclus)
	StateDead        State = "dead"         // au-delà de dead_after_days jours
	StateStopped     State = "stopped"      // suivi arrêté (Repository.Enabled = false), gris
	StateError       State = "error"        // dernier sync_run en erreur, gris
	StateNeverSynced State = "never_synced" // jamais synchronisé avec succès (LastSyncedAt zéro), gris
	StateArchived    State = "archived"     // archivé sur la forge, gris (jamais Dead)
	StateEmpty       State = "empty"        // synchronisé mais aucun commit sur les branches suivies, gris
)

// Gray indique un état hors activité (affiché en gris).
func (s State) Gray() bool {
	switch s {
	case StateActive, StateFlemme, StateDead:
		return false
	default:
		return true
	}
}

// Label renvoie le libellé français de l'état : « Actif », « Flemme », « Dead », « Suivi arrêté »,
// « En erreur », « Jamais synchronisé », « Archivé », « Vide ».
func (s State) Label() string {
	switch s {
	case StateActive:
		return "Actif"
	case StateFlemme:
		return "Flemme"
	case StateDead:
		return "Dead"
	case StateStopped:
		return "Suivi arrêté"
	case StateError:
		return "En erreur"
	case StateNeverSynced:
		return "Jamais synchronisé"
	case StateArchived:
		return "Archivé"
	case StateEmpty:
		return "Vide"
	default:
		return string(s)
	}
}

// Thresholds sont les seuils d'activité en jours entiers (Flemme > 0, Dead > Flemme).
type Thresholds struct {
	FlemmeAfterDays int
	DeadAfterDays   int
}

// ThresholdsFor renvoie les seuils du dépôt : ses colonnes flemme_after_days / dead_after_days quand
// elles sont renseignées, sinon ceux de la configuration (activity.*). Si le mélange donne Dead <=
// Flemme, Dead est ramené à Flemme + 1.
//
// Garde-fous (la configuration est déjà validée par workspace) : une valeur nulle ou négative est
// ignorée au profit de la valeur globale, puis des valeurs par défaut de workspace.
func ThresholdsFor(repo model.Repository, cfg workspace.ActivityConfig) Thresholds {
	flemme := pick(repo.FlemmeAfterDays, cfg.FlemmeAfterDays, workspace.DefaultFlemmeAfterDays)
	dead := pick(repo.DeadAfterDays, cfg.DeadAfterDays, workspace.DefaultDeadAfterDays)
	if dead <= flemme {
		dead = flemme + 1
	}
	return Thresholds{FlemmeAfterDays: flemme, DeadAfterDays: dead}
}

// pick renvoie la valeur du dépôt si elle est renseignée et positive, sinon la valeur globale si elle
// est positive, sinon la valeur par défaut.
func pick(repo *int, global, def int) int {
	switch {
	case repo != nil && *repo > 0:
		return *repo
	case global > 0:
		return global
	default:
		return def
	}
}

// Result est l'état calculé d'un dépôt.
type Result struct {
	State      State
	Days       int        // âge du dernier commit en jours calendaires ; -1 si aucun commit connu
	Thresholds Thresholds // seuils appliqués
}

// Evaluate calcule l'état du dépôt repo, dont lastRun est le dernier sync_run (ID 0 = aucun), à
// l'instant now, les jours étant comptés en jours calendaires dans loc (fuseau d'affichage :
// Config.Location ; nil = time.Local) : Days = date(now dans loc) − date(LastCommitAt dans loc),
// ramené à 0 s'il est négatif.
//
// Priorité : !Enabled → Stopped ; lastRun.Status == error → Error ; LastSyncedAt zéro → NeverSynced ;
// Archived → Archived ; LastCommitAt zéro → Empty ; puis Days <= Flemme → Active, Days <= Dead →
// Flemme, sinon Dead. Days est rempli dès que LastCommitAt est connu, quel que soit l'état.
func Evaluate(repo model.Repository, lastRun model.SyncRun, cfg workspace.ActivityConfig, now time.Time, loc *time.Location) Result {
	res := Result{Days: -1, Thresholds: ThresholdsFor(repo, cfg)}
	if !repo.LastCommitAt.IsZero() {
		res.Days = max(CalendarDays(repo.LastCommitAt, now, loc), 0)
	}
	switch {
	case !repo.Enabled:
		res.State = StateStopped
	case lastRun.ID != 0 && lastRun.Status == model.SyncError:
		res.State = StateError
	case repo.LastSyncedAt.IsZero():
		res.State = StateNeverSynced
	case repo.Archived:
		res.State = StateArchived
	case repo.LastCommitAt.IsZero():
		res.State = StateEmpty
	case res.Days <= res.Thresholds.FlemmeAfterDays:
		res.State = StateActive
	case res.Days <= res.Thresholds.DeadAfterDays:
		res.State = StateFlemme
	default:
		res.State = StateDead
	}
	return res
}

// CalendarDays renvoie le nombre de jours calendaires entre from et to dans loc (nil = time.Local) :
// date(to) − date(from), négatif si to est avant from. Les changements d'heure n'ont pas d'effet.
func CalendarDays(from, to time.Time, loc *time.Location) int {
	if loc == nil {
		loc = time.Local
	}
	return int(civilDay(to, loc).Sub(civilDay(from, loc)).Hours() / 24)
}

// civilDay renvoie la date de t dans loc, à minuit UTC : la différence de deux dates ainsi obtenues
// est un multiple exact de 24 h, sans effet des changements d'heure.
func civilDay(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
