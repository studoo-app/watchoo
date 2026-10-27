package sync

// Retrait d'un dépôt suivi (« watchoo repo remove », page Réglages) : base et clone Git local.

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/studoo-app/watchoo/internal/model"
	gitprov "github.com/studoo-app/watchoo/internal/provider/git"
	"github.com/studoo-app/watchoo/internal/store"
)

// DeleteRepository supprime le dépôt repo de la base (store.DeleteRepository : branches, commits,
// journal des synchros… en cascade), puis son clone Git local dans reposDir (V2), s'il existe, et
// déplace le clone d'un homonyme qui reprend son nom (règle des homonymes, git.Layout) ; reposDir
// vide : aucun clone à supprimer. « repo remove --keep-history » n'appelle pas cette
// fonction : il arrête le suivi (store.SetRepositoryEnabled) et garde la base comme le clone.
//
// Le clone n'est supprimé qu'une fois la base à jour : un échec sur le clone laisse un répertoire
// orphelin (signalé par l'erreur), jamais un dépôt suivi sans historique.
func DeleteRepository(ctx context.Context, st *store.Store, reposDir string, repo model.Repository) error {
	var refs []gitprov.Ref
	if reposDir != "" {
		all, err := st.ListRepositories(ctx)
		if err != nil {
			return err
		}
		refs = CloneRefs(all) // avant le retrait : emplacements actuels des clones
	}
	if err := st.DeleteRepository(ctx, repo.ID); err != nil {
		return err
	}
	if reposDir == "" {
		return nil
	}
	if err := gitprov.RemoveClone(reposDir, refs, repo.Owner, repo.Name); err != nil {
		return fmt.Errorf("dépôt %s retiré, mais %w", repo.FullName(), err)
	}
	return nil
}

// CloneRefs renvoie les dépôts repos (suivis ou arrêtés) dans l'ordre de leur ajout (identifiant
// croissant), pour la règle des homonymes des clones Git locaux (git.Layout).
func CloneRefs(repos []model.Repository) []gitprov.Ref {
	sorted := slices.Clone(repos)
	slices.SortFunc(sorted, func(a, b model.Repository) int { return cmp.Compare(a.ID, b.ID) })
	refs := make([]gitprov.Ref, len(sorted))
	for i, r := range sorted {
		refs[i] = gitprov.Ref{Owner: r.Owner, Name: r.Name}
	}
	return refs
}
