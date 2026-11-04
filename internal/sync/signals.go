package sync

// Signaux de diagnostic (phase 7.1, voir documentation/fr/12-supervision-diagnostic.md) : calculés
// pour les nouveaux commits dont le clone donne le détail des fichiers, quand diagnostic.enabled est
// actif. Le message de commit n'est lu qu'en mémoire (internal/diagnostic) ; les logs ne citent ni
// message, ni e-mail, ni chemin de fichier.

import (
	"context"
	"time"

	"github.com/studoo-app/watchoo/internal/diagnostic"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// writeSignals calcule et enregistre les signaux des nouveaux commits de s (déjà écrits par
// ApplyRepoSync), sauf les commits de fusion. Un échec n'annule pas la synchro du dépôt : il est
// renvoyé comme avertissement (Kind vide sinon).
func (e *Engine) writeSignals(ctx context.Context, s *repoSync, repoID int64) model.SyncWarning {
	if e.diag == nil || len(s.files) == 0 {
		return model.SyncWarning{}
	}
	shas := make([]string, 0, len(s.files))
	for _, c := range s.commits {
		if _, ok := s.files[c.SHA]; ok && !s.merges[c.SHA] {
			shas = append(shas, c.SHA)
		}
	}
	forms := e.measureForms(ctx, s.clone, s.owner, s.name, s.full, shas)
	rows := make([]store.CommitSignalWrite, 0, len(s.files))
	for _, c := range s.commits {
		files, ok := s.files[c.SHA]
		if !ok || s.merges[c.SHA] {
			continue
		}
		sig, ok := diagnostic.Compute(diagnostic.Input{
			Message:     c.Message,
			AuthorName:  c.Author.Name,
			AuthorLogin: c.Author.Login,
			Files:       files,
		}, e.diag)
		if !ok {
			continue
		}
		if f, ok := forms[c.SHA]; ok {
			sig.SetForm(f)
		}
		rows = append(rows, signalWrite(c.SHA, sig))
	}
	n, err := e.store.WriteCommitSignals(ctx, repoID, rows, e.now().UTC())
	if err != nil {
		e.logger.Warn("signaux de diagnostic non enregistrés", "repo", s.full, "erreur", errorMessage(err))
		return model.SyncWarning{Kind: model.WarnDiagnosticNotRecorded,
			Message: i18n.T("sync.diagnostic.not_recorded", errorMessage(err))}
	}
	if n > 0 {
		e.logger.Debug("signaux de diagnostic enregistrés", "repo", s.full, "commits", n)
	}
	return model.SyncWarning{}
}

// signalWrite convertit les signaux calculés d'un commit en ligne à écrire.
func signalWrite(sha string, sig diagnostic.Signals) store.CommitSignalWrite {
	return store.CommitSignalWrite{
		SHA:           sha,
		FilesKept:     sig.FilesKept,
		FilesExcluded: sig.FilesExcluded,
		AdditionsKept: sig.AdditionsKept,
		DeletionsKept: sig.DeletionsKept,
		Tools:         sig.Tools,
		AgentConfig:   sig.AgentConfig,
		BotAuthor:     sig.BotAuthor,
		Form:          commitForm(sig),
	}
}

// commitForm convertit les mesures de forme d'un commit (zéro si elles sont inconnues).
func commitForm(sig diagnostic.Signals) model.CommitForm {
	if !sig.FormKnown {
		return model.CommitForm{}
	}
	f := sig.Form
	return model.CommitForm{
		Known: true, SourceLines: f.SourceLines, CommentLines: f.CommentLines, Functions: f.Functions,
		DocumentedFunctions: f.DocumentedFunctions, Identifiers: f.Identifiers,
		LexicalDiversity: f.LexicalDiversity, HasDiversity: f.HasDiversity, TestLines: f.TestLines,
		MessageLength: sig.Message.Length, MessageBullets: sig.Message.Bullets,
		MessageConventional: sig.Message.Conventional,
	}
}

// lineReader est implémenté par le provider des clones Git locaux (*git.Provider) : lignes ajoutées des
// commits, lues en flux sur le clone existant.
type lineReader interface {
	AddedLines(ctx context.Context, owner, name string, shas []string, fn func(sha, path, line string, hunk bool)) error
}

// measureForms mesure la forme (signaux F1 à F5) des commits shas sur le clone existant de owner/name.
// Les lignes ajoutées ne sont lues qu'en mémoire. Un échec, ou un clone qui ne lit pas les lignes, donne
// des formes inconnues (map vide) : la reprise de l'historique les mesurera ; seul le motif de l'échec
// est journalisé.
func (e *Engine) measureForms(ctx context.Context, clone any, owner, name, full string, shas []string) map[string]diagnostic.Form {
	out := map[string]diagnostic.Form{}
	lr, ok := clone.(lineReader)
	if !ok || len(shas) == 0 || e.diag == nil {
		return out
	}
	meters := make(map[string]*diagnostic.FormMeter, len(shas))
	for _, sha := range shas {
		meters[sha] = diagnostic.NewFormMeter(e.diag)
	}
	err := lr.AddedLines(ctx, owner, name, shas, func(sha, path, line string, hunk bool) {
		if m, ok := meters[sha]; ok {
			m.Line(path, line, hunk)
		}
	})
	if err != nil {
		e.logger.Debug("forme des commits non mesurée", "repo", full, "erreur", errorMessage(err))
		return out
	}
	for sha, m := range meters {
		out[sha] = m.Result()
	}
	return out
}

// firstAdder est implémenté par le provider des clones Git locaux (*git.Provider) : date du premier
// ajout d'un fichier correspondant à des pathspecs.
type firstAdder interface {
	FirstAdded(ctx context.Context, owner, name string, pathspecs []string) (time.Time, bool, error)
}

// updateAgentContext met à jour le contexte D4 du dépôt (premier ajout d'un fichier de configuration
// d'agent), lu sur le clone quand il est à jour pour cette synchro. Un échec est seulement journalisé :
// le contexte précédent reste.
func (e *Engine) updateAgentContext(ctx context.Context, s *repoSync, repoID int64) {
	if e.diag == nil || s.clone == nil || ctx.Err() != nil {
		return
	}
	fa, ok := s.clone.(firstAdder)
	if !ok {
		return
	}
	since, found, err := fa.FirstAdded(ctx, s.owner, s.name, diagnostic.AgentConfigPathspecs())
	if err != nil {
		e.logger.Debug("contexte de diagnostic non lu", "repo", s.full, "erreur", errorMessage(err))
		return
	}
	if !found {
		since = time.Time{}
	}
	if err := e.store.SetAgentConfigSince(ctx, repoID, since, e.now().UTC()); err != nil {
		e.logger.Warn("contexte de diagnostic non enregistré", "repo", s.full, "erreur", errorMessage(err))
	}
}

// purgeSignals supprime, en fin de synchro, les signaux des commits écrits avant la durée de
// conservation (diagnostic.retention_days), les outils des PR ouvertes avant elle, et les revues des
// pistes datées d'avant elle. Rien quand le diagnostic est désactivé ; un échec est
// seulement journalisé (la purge suivante reprendra).
func (e *Engine) purgeSignals(ctx context.Context) {
	if e.diag == nil || ctx.Err() != nil {
		return
	}
	days := e.cfg.Diagnostic.RetentionDays
	if days < 1 {
		return
	}
	before := e.now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	n, err := e.store.PurgeCommitSignals(ctx, before)
	if err != nil {
		e.logger.Warn("purge des signaux de diagnostic impossible", "erreur", errorMessage(err))
		return
	}
	prs, err := e.store.PurgePullRequestTools(ctx, before)
	if err != nil {
		e.logger.Warn("purge des signaux de diagnostic impossible", "erreur", errorMessage(err))
		return
	}
	reviews, err := e.store.PurgeDiagnosticReviews(ctx, before)
	if err != nil {
		e.logger.Warn("purge des signaux de diagnostic impossible", "erreur", errorMessage(err))
		return
	}
	if n > 0 || prs > 0 || reviews > 0 {
		e.logger.Info("signaux de diagnostic purgés", "commits", n, "pull_requests", prs, "revues", reviews,
			"conservation_jours", days)
	}
}
