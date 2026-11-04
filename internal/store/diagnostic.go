package store

// Signaux de diagnostic par commit (table commit_signals, phase 7.1).

import (
	"context"
	"database/sql"
	"strconv"
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
	Form          model.CommitForm // Form.Known faux : forme non mesurée
}

// WriteCommitSignals enregistre, dans une seule transaction, les signaux de commits du dépôt repoID
// déjà en base, calculés à computedAt. Un commit qui a déjà ses signaux n'est modifié que pour compléter
// une forme non mesurée (ligne d'avant la phase 7.5) par une forme mesurée ; un SHA absent de la base est
// ignoré. Renvoie le nombre de lignes créées ou complétées.
func (s *Store) WriteCommitSignals(ctx context.Context, repoID int64, rows []CommitSignalWrite, computedAt time.Time) (int, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	created := 0
	err := s.writeTx(ctx, func(q *db.Queries) error {
		for _, r := range rows {
			n, err := q.InsertCommitSignals(ctx, db.InsertCommitSignalsParams{
				RepoID:              repoID,
				SHA:                 r.SHA,
				FilesKept:           int64(max(r.FilesKept, 0)),
				FilesExcluded:       int64(max(r.FilesExcluded, 0)),
				AdditionsKept:       int64(max(r.AdditionsKept, 0)),
				DeletionsKept:       int64(max(r.DeletionsKept, 0)),
				ToolIds:             strings.Join(r.Tools, ","),
				AgentConfig:         r.AgentConfig,
				BotAuthor:           r.BotAuthor,
				ComputedAt:          FormatTime(computedAt),
				FormKnown:           r.Form.Known,
				SourceLines:         int64(max(r.Form.SourceLines, 0)),
				CommentLines:        int64(max(r.Form.CommentLines, 0)),
				Functions:           int64(max(r.Form.Functions, 0)),
				DocumentedFunctions: int64(max(r.Form.DocumentedFunctions, 0)),
				Identifiers:         int64(max(r.Form.Identifiers, 0)),
				LexicalDiversity: sql.NullFloat64{Float64: min(max(r.Form.LexicalDiversity, 0), 1),
					Valid: r.Form.HasDiversity},
				TestLines:           int64(max(r.Form.TestLines, 0)),
				MessageLength:       int64(max(r.Form.MessageLength, 0)),
				MessageBullets:      int64(max(r.Form.MessageBullets, 0)),
				MessageConventional: r.Form.MessageConventional,
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
			Form: formOf(r.FormKnown, r.SourceLines, r.CommentLines, r.Functions, r.DocumentedFunctions,
				r.Identifiers, r.LexicalDiversity, r.TestLines, r.MessageLength, r.MessageBullets,
				r.MessageConventional),
		})
	}
	return out, nil
}

// formOf convertit les colonnes de forme d'une ligne de commit_signals.
func formOf(known bool, source, comment, functions, documented, identifiers int64, diversity sql.NullFloat64,
	tests, msgLength, msgBullets int64, conventional bool) model.CommitForm {
	return model.CommitForm{
		Known: known, SourceLines: int(source), CommentLines: int(comment), Functions: int(functions),
		DocumentedFunctions: int(documented), Identifiers: int(identifiers),
		LexicalDiversity: diversity.Float64, HasDiversity: diversity.Valid, TestLines: int(tests),
		MessageLength: int(msgLength), MessageBullets: int(msgBullets), MessageConventional: conventional,
	}
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

// CommitSignalView est un commit mesuré et de quoi l'afficher.
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

// CommitSignalViews renvoie les signaux de tous les commits mesurés du dépôt repoID (0 = tous), du plus
// ancien au plus récent : historique des signaux de rythme. Liste vide non nil si aucun.
func (s *Store) CommitSignalViews(ctx context.Context, repoID int64) ([]CommitSignalView, error) {
	rows, err := s.queries.ListCommitSignalViews(ctx, optionalID(repoID))
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
	}
	out := make([]CommitSignalView, 0, len(rows))
	for _, r := range rows {
		v, err := commitSignalView(r)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

// commitSignalView convertit une ligne de vue des signaux.
func commitSignalView(r db.ListCommitSignalViewsRow) (CommitSignalView, error) {
	authored, err := ParseTime(r.AuthoredAt)
	if err != nil {
		return CommitSignalView{}, wrapErr(i18n.T("store.diagnostic.list_all"), err)
	}
	computed, err := ParseTime(r.ComputedAt)
	if err != nil {
		return CommitSignalView{}, wrapErr(i18n.T("store.diagnostic.list_all"), err)
	}
	return CommitSignalView{
		CommitSignal: model.CommitSignal{
			RepoID: r.RepoID, SHA: r.SHA, AuthorID: r.AuthorID.Int64, AuthoredAt: authored,
			FilesKept: int(r.FilesKept), FilesExcluded: int(r.FilesExcluded),
			AdditionsKept: int(r.AdditionsKept), DeletionsKept: int(r.DeletionsKept),
			Tools: splitList(r.ToolIds), AgentConfig: r.AgentConfig, BotAuthor: r.BotAuthor,
			ComputedAt: computed,
			Form: formOf(r.FormKnown, r.SourceLines, r.CommentLines, r.Functions, r.DocumentedFunctions,
				r.Identifiers, r.LexicalDiversity, r.TestLines, r.MessageLength, r.MessageBullets,
				r.MessageConventional),
		},
		RepoRef:     RepoRef{RepoOwner: r.RepoOwner, RepoName: r.RepoName, RepoLabel: r.RepoLabel},
		URL:         r.URL,
		AuthorName:  r.AuthorName,
		AuthorLogin: r.AuthorLogin,
	}, nil
}

// MatchingContributorIDs renvoie les contributeurs désignés par author : login ou nom (sans tenir compte
// de la casse) ou e-mail rattaché ; liste vide non nil si aucun (author vide : aucun).
func (s *Store) MatchingContributorIDs(ctx context.Context, author string) ([]int64, error) {
	author = strings.TrimSpace(author)
	if author == "" {
		return []int64{}, nil
	}
	ids, err := s.queries.MatchingContributorIDs(ctx, db.MatchingContributorIDsParams{
		Author: author, AuthorEmail: normalizeEmail(author)})
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.list_all"), err)
	}
	return ids, nil
}

// CommitForSignals est un commit en base sans signaux, avec ce qu'il faut pour les calculer : son
// message (donnée personnelle, lu en mémoire seulement) et son auteur.
type CommitForSignals struct {
	SHA         string
	Message     string
	AuthorName  string
	AuthorLogin string
}

// CommitsWithoutSignals renvoie, par SHA croissant, au plus limit (> 0) commits du dépôt repoID sans
// signaux, écrits à partir de since, strictement après after (vide = depuis le début) : reprise de
// l'historique, page par page.
func (s *Store) CommitsWithoutSignals(ctx context.Context, repoID int64, since time.Time, after string, limit int) ([]CommitForSignals, error) {
	rows, err := s.queries.ListCommitsWithoutSignals(ctx, db.ListCommitsWithoutSignalsParams{
		RepoID: repoID, Since: FormatTime(since), AfterSha: after, RowLimit: int64(max(limit, 1))})
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.list", repoID), err)
	}
	out := make([]CommitForSignals, 0, len(rows))
	for _, r := range rows {
		out = append(out, CommitForSignals{SHA: r.SHA, Message: r.Message, AuthorName: r.AuthorName, AuthorLogin: r.AuthorLogin})
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

// SaveDiagnosticReview enregistre la revue r (remplace la revue précédente de la même piste). Les
// valeurs sont vérifiées par internal/diagnostic avant ; la base refuse une valeur inconnue.
func (s *Store) SaveDiagnosticReview(ctx context.Context, r model.DiagnosticReview) error {
	err := s.queries.UpsertDiagnosticReview(ctx, db.UpsertDiagnosticReviewParams{
		RepoID: r.RepoID, LeadKey: r.LeadKey, Kind: r.Kind, LeadDate: FormatTime(r.LeadDate),
		Signals: strings.Join(r.Signals, ","), Decision: r.Decision, Cause: r.Cause, Confidence: r.Confidence,
		Note: r.Note, Reviewer: r.Reviewer, ReviewedAt: FormatTime(r.ReviewedAt),
	})
	if err != nil {
		return wrapErr(i18n.T("store.diagnostic.review"), err)
	}
	return nil
}

// DiagnosticReviews renvoie les revues des pistes du dépôt repoID (0 = tous), par clé de piste
// (« repoID/clé », voir ReviewKey).
func (s *Store) DiagnosticReviews(ctx context.Context, repoID int64) (map[string]model.DiagnosticReview, error) {
	rows, err := s.queries.ListDiagnosticReviews(ctx, optionalID(repoID))
	if err != nil {
		return nil, wrapErr(i18n.T("store.diagnostic.review"), err)
	}
	out := make(map[string]model.DiagnosticReview, len(rows))
	for _, r := range rows {
		leadDate, err := ParseTime(r.LeadDate)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.review"), err)
		}
		reviewed, err := ParseTime(r.ReviewedAt)
		if err != nil {
			return nil, wrapErr(i18n.T("store.diagnostic.review"), err)
		}
		out[ReviewKey(r.RepoID, r.LeadKey)] = model.DiagnosticReview{
			RepoID: r.RepoID, LeadKey: r.LeadKey, Kind: r.Kind, LeadDate: leadDate, Signals: splitList(r.Signals),
			Decision: r.Decision, Cause: r.Cause, Confidence: r.Confidence, Note: r.Note, Reviewer: r.Reviewer,
			ReviewedAt: reviewed,
		}
	}
	return out, nil
}

// ReviewKey est la clé d'une revue dans la carte de DiagnosticReviews : « repoID/clé de la piste ».
func ReviewKey(repoID int64, leadKey string) string {
	return strconv.FormatInt(repoID, 10) + "/" + leadKey
}

// PurgeDiagnosticReviews supprime les revues des pistes datées d'avant before (tous les dépôts) et
// renvoie leur nombre.
func (s *Store) PurgeDiagnosticReviews(ctx context.Context, before time.Time) (int, error) {
	n, err := s.queries.PurgeDiagnosticReviews(ctx, FormatTime(before))
	if err != nil {
		return 0, wrapErr(i18n.T("store.diagnostic.purge"), err)
	}
	return int(n), nil
}
