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
	"github.com/studoo-app/watchoo/internal/store"
)

// writeSignals calcule et enregistre les signaux des nouveaux commits de s (déjà écrits par
// ApplyRepoSync), sauf les commits de fusion. Un échec n'annule pas la synchro du dépôt : il est
// renvoyé comme avertissement (chaîne vide sinon).
func (e *Engine) writeSignals(ctx context.Context, s *repoSync, repoID int64) string {
	if e.diag == nil || len(s.files) == 0 {
		return ""
	}
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
		rows = append(rows, store.CommitSignalWrite{
			SHA:           c.SHA,
			FilesKept:     sig.FilesKept,
			FilesExcluded: sig.FilesExcluded,
			AdditionsKept: sig.AdditionsKept,
			DeletionsKept: sig.DeletionsKept,
			Tools:         sig.Tools,
			AgentConfig:   sig.AgentConfig,
			BotAuthor:     sig.BotAuthor,
		})
	}
	n, err := e.store.WriteCommitSignals(ctx, repoID, rows, e.now().UTC())
	if err != nil {
		e.logger.Warn("signaux de diagnostic non enregistrés", "repo", s.full, "erreur", errorMessage(err))
		return i18n.T("sync.diagnostic.not_recorded", errorMessage(err))
	}
	if n > 0 {
		e.logger.Debug("signaux de diagnostic enregistrés", "repo", s.full, "commits", n)
	}
	return ""
}

// purgeSignals supprime, en fin de synchro, les signaux des commits écrits avant la durée de
// conservation (diagnostic.retention_days). Rien quand le diagnostic est désactivé ; un échec est
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
	if n > 0 {
		e.logger.Info("signaux de diagnostic purgés", "commits", n, "conservation_jours", days)
	}
}
