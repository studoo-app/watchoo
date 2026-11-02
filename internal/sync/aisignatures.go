package sync

// Signatures d'IA (1.1.0, documentation/fr/03-architecture.md, « Signatures d'IA ») : après chaque
// écriture d'une synchro, les commits du dépôt pas encore analysés avec la version courante des règles
// le sont, à partir de ce qui est déjà en base, sans aucune requête à la forge.

import (
	"context"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/store"
)

// aiScanPage est le nombre de commits analysés et écrits à la fois.
const aiScanPage = 2000

// scanAISignatures analyse les commits du dépôt en attente, par pages, jusqu'à ce qu'il n'en reste plus.
func (e *Engine) scanAISignatures(ctx context.Context, repoID int64) error {
	for {
		commits, err := e.store.CommitsToScanForAI(ctx, repoID, aisignature.RulesVersion, aiScanPage)
		if err != nil {
			return err
		}
		if len(commits) == 0 {
			return nil
		}
		results := make([]store.AIScanResult, len(commits))
		for i, c := range commits {
			results[i] = store.AIScanResult{SHA: c.SHA, Signatures: aisignature.Detect(aisignature.Input{
				Message: c.Message, AuthorLogin: c.AuthorLogin, AuthorEmails: c.AuthorEmails,
			})}
		}
		if err := e.store.WriteAIScans(ctx, repoID, aisignature.RulesVersion, results); err != nil {
			return err
		}
	}
}
