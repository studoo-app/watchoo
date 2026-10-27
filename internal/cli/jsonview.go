package cli

// Structures JSON stables des commandes de la phase 3 (--json). Les noms de champs sont en anglais,
// en snake_case ; les dates sont en UTC au format RFC 3339 (« 2026-10-27T10:00:00Z »), null si
// inconnues. Aucune ne contient le token.

import (
	"time"

	"github.com/studoo-app/watchoo/internal/activity"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/store"
)

// repositoryJSON décrit un dépôt suivi.
type repositoryJSON struct {
	ID            int64  `json:"id"`
	FullName      string `json:"full_name"` // owner/name actuels
	Owner         string `json:"owner"`
	Name          string `json:"name"`
	Label         string `json:"label"`        // vide = aucun
	DisplayName   string `json:"display_name"` // label, sinon owner/name
	URL           string `json:"url"`
	DefaultBranch string `json:"default_branch"`
	// BranchPatterns sont les motifs de --branch ; liste vide = toutes les branches.
	BranchPatterns []string `json:"branch_patterns"`
	// FlemmeAfterDays et DeadAfterDays sont les seuils propres au dépôt (repo add --flemme / --dead) ;
	// null = seuil de l'espace. Les seuils appliqués sont dans l'objet activité.
	FlemmeAfterDays *int       `json:"flemme_after_days"`
	DeadAfterDays   *int       `json:"dead_after_days"`
	Enabled         bool       `json:"enabled"` // faux = suivi arrêté (repo remove --keep-history)
	Archived        bool       `json:"archived"`
	Stars           int        `json:"stars"`
	OpenIssues      int        `json:"open_issues"`
	LastSyncedAt    *time.Time `json:"last_synced_at"`
	LastCommitAt    *time.Time `json:"last_commit_at"`
}

// newRepositoryJSON convertit un dépôt.
func newRepositoryJSON(r model.Repository) repositoryJSON {
	patterns := r.BranchPatterns
	if patterns == nil {
		patterns = []string{}
	}
	return repositoryJSON{
		ID:              r.ID,
		FullName:        r.FullName(),
		Owner:           r.Owner,
		Name:            r.Name,
		Label:           r.Label,
		DisplayName:     r.DisplayName(),
		URL:             r.URL,
		DefaultBranch:   r.DefaultBranch,
		BranchPatterns:  append([]string{}, patterns...),
		FlemmeAfterDays: r.FlemmeAfterDays,
		DeadAfterDays:   r.DeadAfterDays,
		Enabled:         r.Enabled,
		Archived:        r.Archived,
		Stars:           r.Stars,
		OpenIssues:      r.OpenIssues,
		LastSyncedAt:    timeJSON(r.LastSyncedAt),
		LastCommitAt:    timeJSON(r.LastCommitAt),
	}
}

// repoRefJSON désigne le dépôt d'un commit.
type repoRefJSON struct {
	ID          int64  `json:"id"`
	FullName    string `json:"full_name"`
	DisplayName string `json:"display_name"`
}

// authorJSON décrit l'auteur d'un commit (nom et login vides s'il est inconnu).
type authorJSON struct {
	Name        string `json:"name"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"` // nom, sinon login, sinon « inconnu »
}

// commitJSON décrit un commit (watchoo log, dernier commit de repo list).
type commitJSON struct {
	Repository  repoRefJSON `json:"repository"`
	SHA         string      `json:"sha"`
	ShortSHA    string      `json:"short_sha"`
	Message     string      `json:"message"`
	URL         string      `json:"url"`
	Author      authorJSON  `json:"author"`
	AuthoredAt  *time.Time  `json:"authored_at"`
	CommittedAt *time.Time  `json:"committed_at"`
	Additions   int         `json:"additions"` // 0 si les statistiques sont inconnues
	Deletions   int         `json:"deletions"`
}

// newCommitJSON convertit une ligne de la vue des commits.
func newCommitJSON(c store.CommitView) commitJSON {
	return commitJSON{
		Repository: repoRefJSON{
			ID:          c.RepoID,
			FullName:    c.RepoOwner + "/" + c.RepoName,
			DisplayName: c.RepoDisplayName(),
		},
		SHA:      c.SHA,
		ShortSHA: provider.ShortSHA(c.SHA),
		Message:  c.Message,
		URL:      c.URL,
		Author: authorJSON{
			Name:        c.AuthorName,
			Login:       c.AuthorLogin,
			DisplayName: c.AuthorDisplayName(),
		},
		AuthoredAt:  timeJSON(c.AuthoredAt),
		CommittedAt: timeJSON(c.CommittedAt),
		Additions:   c.Additions,
		Deletions:   c.Deletions,
	}
}

// syncRunJSON décrit une synchro enregistrée (sync_runs).
type syncRunJSON struct {
	Status     string     `json:"status"` // running, success ou error
	StartedAt  *time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at"`
	NewCommits int        `json:"new_commits"`
	Error      string     `json:"error"`
}

// newSyncRunJSON convertit un sync_run ; nil s'il n'y en a pas (ID 0).
func newSyncRunJSON(r model.SyncRun) *syncRunJSON {
	if r.ID == 0 {
		return nil
	}
	return &syncRunJSON{
		Status:     string(r.Status),
		StartedAt:  timeJSON(r.StartedAt),
		FinishedAt: timeJSON(r.FinishedAt),
		NewCommits: r.NewCommits,
		Error:      r.Error,
	}
}

// activityJSON décrit l'état d'activité d'un dépôt.
type activityJSON struct {
	State           activity.State `json:"state"`       // active, flemme, dead, stopped, error, never_synced, archived, empty
	Label           string         `json:"state_label"` // libellé français (« Actif »…)
	Gray            bool           `json:"gray"`        // état hors activité
	DaysSinceCommit *int           `json:"days_since_last_commit"`
	FlemmeAfterDays int            `json:"flemme_after_days"` // seuils appliqués
	DeadAfterDays   int            `json:"dead_after_days"`
}

// newActivityJSON convertit un état calculé.
func newActivityJSON(r activity.Result) activityJSON {
	var days *int
	if r.Days >= 0 {
		d := r.Days
		days = &d
	}
	return activityJSON{
		State:           r.State,
		Label:           r.State.Label(),
		Gray:            r.State.Gray(),
		DaysSinceCommit: days,
		FlemmeAfterDays: r.Thresholds.FlemmeAfterDays,
		DeadAfterDays:   r.Thresholds.DeadAfterDays,
	}
}

// rateLimitJSON décrit le quota de l'API GitHub.
type rateLimitJSON struct {
	Limit     int        `json:"limit"`
	Remaining int        `json:"remaining"`
	Used      int        `json:"used"`
	Reset     *time.Time `json:"reset"`
}

// newRateLimitJSON convertit un état du quota.
func newRateLimitJSON(rl provider.RateLimit) *rateLimitJSON {
	return &rateLimitJSON{Limit: rl.Limit, Remaining: rl.Remaining, Used: rl.Used, Reset: timeJSON(rl.Reset)}
}

// timeJSON renvoie t en UTC à la seconde, ou nil s'il est zéro (null en JSON).
func timeJSON(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC().Truncate(time.Second)
	return &u
}
