package conventions

// Critères de fichiers (phase 8.3) : lecture d'une révision par les objets Git du clone, révisions à
// mesurer (tête de la branche par défaut, tags des jalons) et encodage du résultat mis en cache par la
// synchro (table repository_conventions). Aucun accès à la base ni à la forge ici.

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// FileReader lit les fichiers d'une révision dans le clone Git local existant, par les objets Git, sans
// mise à jour ; le provider des clones (internal/provider/git) l'implémente. rev est un SHA complet.
type FileReader interface {
	// TreePaths renvoie les chemins des fichiers ordinaires de l'arbre de rev ; truncated si la liste est
	// coupée au plafond du provider.
	TreePaths(ctx context.Context, owner, name, rev string) (paths []string, truncated bool, err error)
	// ReadBlobs lit le contenu des fichiers paths de l'arbre de rev : fn une fois par fichier lu, tooLarge
	// pour un fichier au-delà du plafond (data nil) ; rien pour un fichier absent ou non lu.
	ReadBlobs(ctx context.Context, owner, name, rev string, paths []string,
		fn func(path string, data []byte, tooLarge bool)) error
}

// ReadFileTree lit l'arbre de la révision rev de owner/name et le contenu des fichiers que les critères
// de fichiers lisent (FileBlobPaths). Un arbre tronqué n'est pas lu plus avant : tous les critères seront
// inconnus. Les chemins et contenus ne restent qu'en mémoire.
func ReadFileTree(ctx context.Context, fr FileReader, owner, name, rev string, rules FileRules) (FileTree, error) {
	paths, truncated, err := fr.TreePaths(ctx, owner, name, rev)
	if err != nil {
		return FileTree{}, err
	}
	tree := FileTree{Paths: paths, Truncated: truncated, Blobs: map[string]Blob{}}
	if truncated {
		return tree, nil
	}
	want := FileBlobPaths(paths, rules)
	if len(want) == 0 {
		return tree, nil
	}
	err = fr.ReadBlobs(ctx, owner, name, rev, want, func(path string, data []byte, tooLarge bool) {
		tree.Blobs[path] = Blob{Data: data, TooLarge: tooLarge}
	})
	if err != nil {
		return FileTree{}, err
	}
	return tree, nil
}

// FileTarget est une révision dont les critères de fichiers sont mesurés : la tête de la branche par
// défaut (Kind model.ConventionRevHead, Tag vide) ou le tag d'un jalon (model.ConventionRevTag).
type FileTarget struct {
	Kind string
	Tag  string // nom du tag ; vide pour la tête
	SHA  string // commit mesuré
}

// FileTargets renvoie les révisions à mesurer d'un dépôt : la tête headSHA de la branche par défaut (si
// connue), puis, dans l'ordre des jalons, le tag de chaque jalon présent (nom exact, ou premier nom non
// conforme qui désigne sa version : même règle que la revue de jalon) ; un tag n'apparaît qu'une fois.
// Un tag sans SHA connu est ignoré.
func FileTargets(headSHA string, tags []model.Tag, ms []Milestone) []FileTarget {
	var out []FileTarget
	if headSHA != "" {
		out = append(out, FileTarget{Kind: model.ConventionRevHead, SHA: headSHA})
	}
	shas := make(map[string]string, len(tags))
	for _, t := range tags {
		shas[t.Name] = t.SHA
	}
	seen := map[string]bool{}
	for _, r := range CheckMilestones(ms, repoTagNames(tags)) {
		if r.Tag == "" || seen[r.Tag] || shas[r.Tag] == "" {
			continue
		}
		seen[r.Tag] = true
		out = append(out, FileTarget{Kind: model.ConventionRevTag, Tag: r.Tag, SHA: shas[r.Tag]})
	}
	return out
}

// repoTagNames renvoie les noms des tags, triés.
func repoTagNames(tags []model.Tag) []string {
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		out = append(out, t.Name)
	}
	slices.Sort(out)
	return out
}

// FileConfigHash renvoie l'empreinte des réglages, du dépôt repo (« owner/name ») et des tags de version
// qui influencent les critères de fichiers d'un dépôt (config_hash du cache).
func FileConfigHash(rules FileRules, repo string, tags []model.Tag) string {
	return rules.Fingerprint(repo, repoTagNames(tags))
}

// FileOptionsFor renvoie les entrées des critères de fichiers de la révision target du dépôt repo
// (« owner/name ») : le tag d'un jalon fixe Version (CL9, CL8 limité aux versions antérieures).
func FileOptionsFor(target FileTarget, repo string, tags []model.Tag, rules FileRules, now time.Time) FileOptions {
	return FileOptions{Rules: rules, Repo: repo, Tags: repoTagNames(tags), Version: target.Tag, Now: now}
}

// EncodeFileResults encode le résultat mis en cache (objet JSON d'identifiants, d'états, de codes et de
// compteurs).
func EncodeFileResults(r FileResults) (string, error) {
	raw, err := json.Marshal(r)
	if err != nil {
		return "", fmt.Errorf("conventions: encode file results: %w", err)
	}
	return string(raw), nil
}

// DecodeFileResults décode un résultat mis en cache par EncodeFileResults.
func DecodeFileResults(raw string) (FileResults, error) {
	var r FileResults
	if err := json.Unmarshal([]byte(raw), &r); err != nil {
		return FileResults{}, fmt.Errorf("conventions: decode file results: %w", err)
	}
	return r, nil
}

// FileCacheFresh indique qu'un résultat en cache vaut pour la révision sha avec l'empreinte hash et la
// version actuelle des règles de fichiers ; sinon il est « à recalculer ».
func FileCacheFresh(row model.RepositoryConventions, sha, hash string) bool {
	return row.RevSHA == sha && row.RulesVersion == FileRulesVersion && row.ConfigHash == hash
}
