package store

// Signaux de diagnostic par commit (table commit_signals, phase 7.1).

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store/db"
)

// CommitSignalWrite sont les signaux d'un commit à enregistrer. L'auteur et la date d'écriture sont
// recopiés du commit en base : le commit doit donc y être déjà (sinon la ligne est ignorée).
type CommitSignalWrite struct {
	SHA           string   // SHA complet
	FilesKept     int      // >= 0
	FilesExcluded int      // >= 0
	AdditionsKept int      // >= 0
	DeletionsKept int      // >= 0
	Tools         []string // identifiants d'outils (sans virgule), triés
	AgentConfig   bool
	BotAuthor     bool
}

// WriteCommitSignals enregistre, dans une seule transaction, les signaux de commits du dépôt repoID
// déjà en base, calculés à computedAt. Un commit qui a déjà ses signaux n'est jamais modifié ; un SHA
// absent de la base est ignoré. Renvoie le nombre de lignes créées.
func (s *Store) WriteCommitSignals(ctx context.Context, repoID int64, rows []CommitSignalWrite, computedAt time.Time) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	created := 0
	err := s.writeTx(ctx, func(q *db.Queries) error {
		for _, r := range rows {
			n, err := q.InsertCommitSignals(ctx, db.InsertCommitSignalsParams{
				RepoID:        repoID,
				SHA:           r.SHA,
				FilesKept:     int64(max(r.FilesKept, 0)),
				FilesExcluded: int64(max(r.FilesExcluded, 0)),
				AdditionsKept: int64(max(r.AdditionsKept, 0)),
				DeletionsKept: int64(max(r.DeletionsKept, 0)),
				ToolIds:       strings.Join(r.Tools, ","),
				AgentConfig:   r.AgentConfig,
				BotAuthor:     r.BotAuthor,
				ComputedAt:    FormatTime(computedAt),
			})
			if err != nil {
				return wrapErr(i18n.T("store.diagnostic.write", shortSHA(r.SHA)), err)
			}
			created += int(n)
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return created, nil
}

// PurgeCommitSignals supprime les signaux des commits écrits avant before (tous les dépôts) et renvoie
// le nombre de lignes supprimées.
func (s *Store) PurgeCommitSignals(ctx context.Context, before time.Time) (int, error) {
	n, err := s.queries.PurgeCommitSignals(ctx, FormatTime(before))
	if err != nil {
		return 0, wrapErr(i18n.T("store.diagnostic.purge"), err)
	}
	return int(n), nil
}

// CommitSignals renvoie les signaux des commits du dépôt repoID, du plus récent au plus ancien.
func (s *Store) CommitSignals(ctx context.Context, repoID int64) ([]model.CommitSignal, error) {
	rows, err := s.queries.ListCommitSignals(ctx, repoID)
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
	}
	out := make([]model.CommitSignal, 0, len(rows))
	for _, r := range rows {
		authored, err := ParseTime(r.AuthoredAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
		}
		computed, err := ParseTime(r.ComputedAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
		}
		tools := splitList(r.ToolIds)
		out = append(out, model.CommitSignal{
			RepoID:        r.RepoID,
			SHA:           r.SHA,
			AuthorID:      r.AuthorID.Int64,
			AuthoredAt:    authored,
			FilesKept:     int(r.FilesKept),
			FilesExcluded: int(r.FilesExcluded),
			AdditionsKept: int(r.AdditionsKept),
			DeletionsKept: int(r.DeletionsKept),
			Tools:         tools,
			AgentConfig:   r.AgentConfig,
			BotAuthor:     r.BotAuthor,
			ComputedAt:    computed,
		})
	}
	return out, nil
}

// CountCommitSignals renvoie le nombre de commits du dépôt repoID qui ont des signaux.
func (s *Store) CountCommitSignals(ctx context.Context, repoID int64) (int, error) {
	n, err := s.queries.CountCommitSignals(ctx, repoID)
	if err != nil {
		return 0, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
	}
	return int(n), nil
}

// splitList relit une liste gardée en texte, séparée par des virgules ; liste vide non nil si vide.
func splitList(v string) []string {
	if v == "" {
		return []string{}
	}
	return strings.Split(v, ",")
}

// DiagnosticFilter filtre les pistes de diagnostic (phase 7.2).
type DiagnosticFilter struct {
	RepoID int64 // dépôt ; 0 = tous
	// Author filtre sur l'auteur : login de forge, nom ou e-mail rattaché, sans tenir compte de la casse
	// (mêmes règles que CommitFilter.Author) ; vide = tous.
	Author string
	Since  time.Time // date d'écriture >= Since ; zéro = pas de borne
}

// CommitSignalView est un commit avec au moins un signal déclaratif, et de quoi l'afficher.
type CommitSignalView struct {
	model.CommitSignal
	RepoRef
	URL         string // page du commit sur la forge
	AuthorName  string // nom du contributeur auteur ; vide si inconnu
	AuthorLogin string // login de forge de l'auteur ; vide si inconnu
}

// AgentConfigContext est le contexte D4 d'un dépôt : date du premier ajout d'un fichier de
// configuration d'agent.
type AgentConfigContext struct {
	RepoID int64
	RepoRef
	Since time.Time // date d'écriture du commit qui a ajouté le premier fichier, UTC
}

// ListDeclaredCommitSignals renvoie les commits qui ont au moins un signal déclaratif (outil déclaré
// ou auteur bot) et satisfont f, du plus récent au plus ancien ; liste vide non nil si aucun.
func (s *Store) ListDeclaredCommitSignals(ctx context.Context, f DiagnosticFilter) ([]CommitSignalView, error) {
	params := db.ListDeclaredCommitSignalsParams{RepoID: optionalID(f.RepoID), Since: optionalTime(f.Since)}
	if author := strings.TrimSpace(f.Author); author != "" {
		params.Author = author
		params.AuthorEmail = sql.NullString{String: normalizeEmail(author), Valid: true}
	}
	rows, err := s.queries.ListDeclaredCommitSignals(ctx, params)
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.declared"), err)
	}
	out := make([]CommitSignalView, 0, len(rows))
	for _, r := range rows {
		authored, err := ParseTime(r.AuthoredAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.declared"), err)
		}
		computed, err := ParseTime(r.ComputedAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.declared"), err)
		}
		out = append(out, CommitSignalView{
			CommitSignal: model.CommitSignal{
				RepoID: r.RepoID, SHA: r.SHA, AuthorID: r.AuthorID.Int64, AuthoredAt: authored,
				FilesKept: int(r.FilesKept), FilesExcluded: int(r.FilesExcluded),
				AdditionsKept: int(r.AdditionsKept), DeletionsKept: int(r.DeletionsKept),
				Tools: splitList(r.ToolIds), AgentConfig: r.AgentConfig, BotAuthor: r.BotAuthor,
				ComputedAt: computed,
			},
			RepoRef:     RepoRef{RepoOwner: r.RepoOwner, RepoName: r.RepoName, RepoLabel: r.RepoLabel},
			URL:         r.URL,
			AuthorName:  r.AuthorName,
			AuthorLogin: r.AuthorLogin,
		})
	}
	return out, nil
}

// SetAgentConfigSince enregistre le contexte D4 du dépôt repoID : since est la date du premier ajout
// d'un fichier de configuration d'agent (zéro = aucun fichier), calculée à computedAt.
func (s *Store) SetAgentConfigSince(ctx context.Context, repoID int64, since, computedAt time.Time) error {
	err := s.queries.UpsertRepositorySignals(ctx, db.UpsertRepositorySignalsParams{
		RepoID: repoID, AgentConfigSince: nullTime(since), ComputedAt: FormatTime(computedAt),
	})
	if err != nil {
		return wrapErr(i18n.T("store.diagnostic.context", repoID), err)
	}
	return nil
}

// AgentConfigContexts renvoie les dépôts (repoID, 0 = tous) où un fichier de configuration d'agent a
// été trouvé, du plus ancien ajout au plus récent ; liste vide non nil si aucun.
func (s *Store) AgentConfigContexts(ctx context.Context, repoID int64) ([]AgentConfigContext, error) {
	rows, err := s.queries.ListAgentConfigContexts(ctx, optionalID(repoID))
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.context", repoID), err)
	}
	out := make([]AgentConfigContext, 0, len(rows))
	for _, r := range rows {
		since, err := ParseTime(r.AgentConfigSince)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.context", r.RepoID), err)
		}
		out = append(out, AgentConfigContext{RepoID: r.RepoID,
			RepoRef: RepoRef{RepoOwner: r.RepoOwner, RepoName: r.RepoName, RepoLabel: r.RepoLabel}, Since: since})
	}
	return out, nil
}

// PurgePullRequestTools efface les outils déclarés des pull requests ouvertes avant before (tous les
// dépôts) et renvoie le nombre de PR concernées.
func (s *Store) PurgePullRequestTools(ctx context.Context, before time.Time) (int, error) {
	n, err := s.queries.PurgePullRequestTools(ctx, FormatTime(before))
	if err != nil {
		return 0, wrapErr(i18n.T("store.diagnostic.purge"), err)
	}
	return int(n), nil
}
