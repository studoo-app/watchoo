package git

// Statistiques de lignes par paquets (« toujours cloner », 0.5.0) : le moteur calcule celles de tous
// les commits d'un dépôt dans son clone, avec ou sans gh, sans lancer un processus par commit.

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/provider"
)

// StatsBatchSize est le nombre de SHA traités par paquet dans CommitStatsBatch : deux processus git
// par paquet, et une sortie bornée.
const StatsBatchSize = 500

// Prepare crée ou met à jour le clone de owner/name pour ce Provider (une fois par Provider, comme
// tout autre appel), en extrayant defaultBranch, la branche par défaut donnée par l'appelant (l'API
// GitHub, quand gh est utilisé) : aucun « ls-remote » de plus. defaultBranch vide : celle du dépôt
// distant (ls-remote). Accès refusé : provider.ErrAccessDenied ; emplacement occupé :
// ErrCloneDirOccupied.
func (p *Provider) Prepare(ctx context.Context, owner, name, defaultBranch string) error {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return err
	}
	if defaultBranch != "" {
		if err := provider.ValidateBranch(defaultBranch); err != nil {
			return err
		}
		p.mu.Lock()
		if _, ok := p.defaults[marker(owner, name)]; !ok {
			p.defaults[marker(owner, name)] = defaultBranch
		}
		p.mu.Unlock()
	}
	_, err := p.ensure(ctx, owner, name)
	return err
}

// CommitStatsBatch renvoie les statistiques des commits shas (SHA complets) du clone de owner/name,
// avec les règles de CommitStats (premier parent, arbre vide pour un commit racine, fichiers binaires
// non comptés ; un commit sans changement donne 0/0). Le clone est d'abord créé ou mis à jour (voir
// Prepare). Un SHA absent du clone, ou qui n'est pas un commit, est omis du résultat ; les doublons
// sont permis. Les SHA sont traités par paquets de StatsBatchSize, chacun par deux processus git :
//
//	git rev-list --no-walk=unsorted --parents --ignore-missing --stdin   (parents des commits présents)
//	git diff-tree --stdin --always -r -z --numstat --root …              (« commit premier-parent »)
//
// Le calcul s'arrête dès que ctx est terminé (erreur ctx.Err()).
func (p *Provider) CommitStatsBatch(ctx context.Context, owner, name string, shas []string) (map[string]provider.CommitStats, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	for _, sha := range shas {
		if err := provider.ValidateSHA(sha); err != nil {
			return nil, err
		}
	}
	path, err := p.ensure(ctx, owner, name)
	if err != nil {
		return nil, err
	}
	out := make(map[string]provider.CommitStats, len(shas))
	unique := slices.Compact(slices.Sorted(slices.Values(shas)))
	for batch := range slices.Chunk(unique, StatsBatchSize) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := p.statsBatch(ctx, owner, name, path, batch, out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// statsBatch calcule les statistiques d'un paquet de SHA (validés) et les ajoute à out.
func (p *Provider) statsBatch(ctx context.Context, owner, name, path string, shas []string,
	out map[string]provider.CommitStats) error {
	op := i18n.T("provider.git.op.commits_stats", owner, name)
	input := []byte(strings.Join(shas, "\n") + "\n")
	res, err := p.runInput(ctx, p.localTimeout, gitDir(path), "rev-list", input,
		"rev-list", "--no-walk=unsorted", "--parents", "--ignore-missing", "--stdin")
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return localError(op, res)
	}
	// Une ligne « commit premier-parent » (ou « commit » pour une racine) par commit présent.
	var lines bytes.Buffer
	n := 0
	for line := range strings.SplitSeq(string(res.stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || provider.ValidateSHA(fields[0]) != nil {
			continue
		}
		lines.WriteString(fields[0])
		if len(fields) > 1 {
			lines.WriteString(" " + fields[1])
		}
		lines.WriteByte('\n')
		n++
	}
	if n == 0 {
		return nil
	}
	res, err = p.runInput(ctx, p.localTimeout, gitDir(path), "diff-tree", lines.Bytes(),
		"diff-tree", "--stdin", "--always", "--root", "-r", "-z", "--numstat", "--no-renames", "--no-ext-diff")
	if err != nil {
		return &provider.APIError{Op: op, Err: err}
	}
	if res.exitCode != 0 {
		return localError(op, res)
	}
	parseBatchNumstat(res.stdout, out)
	return nil
}

// parseBatchNumstat lit la sortie de « diff-tree --stdin --always -z --numstat » : pour chaque commit,
// son SHA (« <sha><NUL> »), puis un enregistrement « ajoutées<TAB>supprimées<TAB>chemin<NUL> » par
// fichier modifié (« - » pour un fichier binaire, ignoré). Un SHA sans fichier donne 0/0.
func parseBatchNumstat(raw []byte, out map[string]provider.CommitStats) {
	current := ""
	for rec := range bytes.SplitSeq(raw, []byte{0}) {
		s := strings.TrimLeft(string(rec), "\n")
		if s == "" {
			continue
		}
		if !strings.Contains(s, "\t") {
			if provider.ValidateSHA(s) == nil {
				current = s
				if _, ok := out[current]; !ok {
					out[current] = provider.CommitStats{}
				}
			}
			continue
		}
		if current == "" {
			continue
		}
		st := parseNumstat(append(rec, 0))
		total := out[current]
		total.Additions += st.Additions
		total.Deletions += st.Deletions
		out[current] = total
	}
}
