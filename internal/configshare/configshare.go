// Package configshare partage la configuration d'un espace de supervision entre les personnes qui
// supervisent le même groupe (documentation/fr/05-cli.md, « Partage de la configuration »).
//
// Cœur métier de « watchoo config export / import » et de la page Réglages du web, qui ne font
// qu'afficher :
//   - Export lit les dépôts suivis (store) et les réglages communs en vigueur (ws.Config) et renvoie
//     le Document, que Marshal écrit en JSON indenté ;
//   - Parse lit un document venu d'une autre personne (DONNÉE NON FIABLE) et le valide entièrement :
//     format, version, clés inconnues, taille, nombre de dépôts, doublons, labels, motifs, réglages ;
//   - Plan calcule l'aperçu d'un import (dépôts à ajouter / à mettre à jour / inchangés, réglages
//     modifiés, avertissements) sans rien écrire, après avoir validé les réglages fusionnés avec ceux de
//     l'espace ;
//   - Apply recalcule l'aperçu (tout est revalidé), réécrit config.yaml si un réglage change
//     (workspace.WriteSharedSettings), puis ajoute ou met à jour les dépôts
//     (wsync.Engine.AddRepositories, accès vérifié). Rien n'est jamais retiré.
//
// Jamais partagés : name, github.* (dont gh_path : un fichier venu d'ailleurs ne doit jamais choisir
// le programme lancé), git.* (protocole des clones Git locaux), web.*, token, commits, contributeurs,
// e-mails, chemins locaux.
package configshare

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Constantes du format.
const (
	Format          = "watchoo-config" // valeur de la clé format
	Version         = 1                // version du format écrite par Export, la seule acceptée par Parse
	MaxFileSize     = 1 << 20          // taille maximale d'un document (1 Mio)
	MaxRepositories = 1000             // nombre maximal de dépôts dans un document
	// MaxTextRunes borne les textes informatifs (workspace.name, watchoo_version) et le fuseau.
	MaxTextRunes = 200
)

// Erreurs de Parse et de Plan (à tester avec errors.Is ; le message dit ce qui ne va pas, en français).
var (
	// ErrTooLarge : le document dépasse MaxFileSize.
	ErrTooLarge = errors.New("fichier de configuration trop gros (1 Mio au plus)")
	// ErrUnsupportedFormat : clé format absente ou différente de Format (ce n'est pas un fichier de
	// configuration Watchoo).
	ErrUnsupportedFormat = errors.New("ce fichier n'est pas une configuration Watchoo partagée")
	// ErrNewerVersion : version plus grande que Version.
	ErrNewerVersion = errors.New("fichier créé par une version plus récente de Watchoo")
	// ErrInvalid : tout autre refus (JSON invalide, clé inconnue, dépôt, label, motif ou réglage
	// invalide, doublon, trop de dépôts, version absente ou inférieure à 1).
	ErrInvalid = errors.New("fichier de configuration invalide")
)

// Document est le fichier de partage (format v1). Les clés JSON sont en snake_case ; voir
// documentation/fr/05-cli.md pour leur sens. Les champs facultatifs à l'import sont des pointeurs (nil =
// absent) ; Export les renseigne tous.
type Document struct {
	Format         string        `json:"format"`
	Version        int           `json:"version"`
	ExportedAt     time.Time     `json:"exported_at"`     // UTC, à la seconde ; informatif
	WatchooVersion string        `json:"watchoo_version"` // informatif
	Workspace      WorkspaceInfo `json:"workspace"`
	Settings       *Settings     `json:"settings,omitempty"`
	Repositories   []Repository  `json:"repositories"`
}

// WorkspaceInfo décrit l'espace exporté (informatif : l'import ne renomme jamais l'espace).
type WorkspaceInfo struct {
	Name string `json:"name"`
}

// Settings sont les réglages communs. Chaque champ nil (clé absente ou null) n'est pas fourni.
type Settings struct {
	Sync     *SyncSettings     `json:"sync,omitempty"`
	Activity *ActivitySettings `json:"activity,omitempty"`
	Branches *BranchesSettings `json:"branches,omitempty"`
	Timezone *string           `json:"timezone,omitempty"`
}

// SyncSettings : sync.auto, sync.interval (durée Go écrite en chaîne : « 30m », « 1h30m », « 0 »),
// sync.parallelism.
type SyncSettings struct {
	Auto        *bool   `json:"auto,omitempty"`
	Interval    *string `json:"interval,omitempty"`
	Parallelism *int    `json:"parallelism,omitempty"`
}

// ActivitySettings : activity.flemme_after_days, activity.dead_after_days.
type ActivitySettings struct {
	FlemmeAfterDays *int `json:"flemme_after_days,omitempty"`
	DeadAfterDays   *int `json:"dead_after_days,omitempty"`
}

// BranchesSettings : branches.exclude ([] = aucune branche exclue).
type BranchesSettings struct {
	Exclude *[]string `json:"exclude,omitempty"`
}

// Repository est un dépôt du document.
type Repository struct {
	// Provider est la forge ; seule model.ProviderGitHub est acceptée (absent = github).
	Provider string `json:"provider"`
	// Repo est « owner/repo » (une URL GitHub est aussi acceptée à l'import).
	Repo string `json:"repo"`
	// Label est le nom affiché ; "" = aucun (à l'import : le nom affiché existant est gardé).
	Label string `json:"label"`
	// Branches sont les motifs suivis : [] = toutes les branches ; à l'import, absent ou null = garder
	// les branches existantes (nouveau dépôt : toutes).
	Branches []string `json:"branches"`
	// FlemmeAfterDays et DeadAfterDays sont les seuils d'activité propres au dépôt, en jours. Export
	// ne les écrit que pour un dépôt qui en a (clés absentes sinon : le fichier reste lisible par les
	// versions de Watchoo qui ne les connaissent pas). À l'import, absent ou null = garder les seuils
	// existants (nouveau dépôt : ceux de l'espace), 0 = revenir au seuil de l'espace.
	FlemmeAfterDays *int `json:"flemme_after_days,omitempty"`
	DeadAfterDays   *int `json:"dead_after_days,omitempty"`
}

// Store est ce dont configshare a besoin de la base ; *store.Store le satisfait.
type Store interface {
	ListRepositories(ctx context.Context) ([]model.Repository, error)
}

// Adder ajoute des dépôts avec vérification d'accès ; *wsync.Engine le satisfait (et l'interface
// web.RepoManager l'englobe).
type Adder interface {
	AddRepositories(ctx context.Context, reqs []wsync.AddRequest, opts wsync.AddOptions) ([]wsync.AddResult, error)
}

// ExportOptions règle Export.
type ExportOptions struct {
	Now            time.Time // date d'export ; zéro = time.Now()
	WatchooVersion string    // version de Watchoo (« v0.1.0 », « dev »)
}

// Export renvoie le document de partage de l'espace : nom de l'espace (ws.Config.Name), réglages
// communs EN VIGUEUR (ws.Config, après fusion des fichiers et des variables WATCHOO_*), et dépôts dont
// le suivi est actif (Enabled), triés par owner/name sans tenir compte de la casse, avec label,
// motifs ([] = toutes les branches) et seuils d'activité propres (s'il y en a). Aucun appel à gh ni à la forge.
func Export(ctx context.Context, ws *workspace.Workspace, st Store, opts ExportOptions) (*Document, error) {
	if ws == nil || st == nil {
		return nil, errors.New("export de la configuration : espace ou base non renseigné")
	}
	repos, err := st.ListRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("export de la configuration : lecture des dépôts : %w", err)
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	doc := &Document{
		Format:         Format,
		Version:        Version,
		ExportedAt:     now.UTC().Truncate(time.Second),
		WatchooVersion: opts.WatchooVersion,
		Workspace:      WorkspaceInfo{Name: ws.Config.Name},
		Settings:       settingsFromConfig(ws.Config),
		Repositories:   []Repository{},
	}
	for _, r := range repos {
		if !r.Enabled {
			continue
		}
		branches := slices.Clone(r.BranchPatterns)
		if branches == nil {
			branches = []string{}
		}
		doc.Repositories = append(doc.Repositories, Repository{
			Provider: cmp.Or(r.Provider, model.ProviderGitHub),
			Repo:     r.FullName(),
			Label:    r.Label,
			Branches: branches,
			// Copies : le document ne partage rien avec les dépôts lus.
			FlemmeAfterDays: clonePtr(r.FlemmeAfterDays),
			DeadAfterDays:   clonePtr(r.DeadAfterDays),
		})
	}
	slices.SortFunc(doc.Repositories, func(a, b Repository) int {
		return cmp.Or(strings.Compare(strings.ToLower(a.Repo), strings.ToLower(b.Repo)), strings.Compare(a.Repo, b.Repo))
	})
	return doc, nil
}

// clonePtr renvoie une copie de *p, ou nil.
func clonePtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// settingsFromConfig renvoie tous les réglages communs de c.
func settingsFromConfig(c workspace.Config) *Settings {
	interval := formatDuration(c.Sync.Interval)
	exclude := slices.Clone(c.Branches.Exclude)
	if exclude == nil {
		exclude = []string{}
	}
	return &Settings{
		Sync: &SyncSettings{Auto: &c.Sync.Auto, Interval: &interval, Parallelism: &c.Sync.Parallelism},
		Activity: &ActivitySettings{
			FlemmeAfterDays: &c.Activity.FlemmeAfterDays,
			DeadAfterDays:   &c.Activity.DeadAfterDays,
		},
		Branches: &BranchesSettings{Exclude: &exclude},
		Timezone: &c.Timezone,
	}
}

// formatDuration écrit une durée comme config.yaml (30m, 1h30m, 0s).
func formatDuration(d time.Duration) string {
	return workspace.FormatSettingValue(d)
}

// Marshal renvoie le document en JSON indenté (deux espaces), suivi d'une fin de ligne.
func (d *Document) Marshal() ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return nil, fmt.Errorf("écriture de la configuration en JSON : %w", err)
	}
	return buf.Bytes(), nil
}

// FileName renvoie un nom de fichier sûr pour l'export d'un espace nommé workspaceName :
// « watchoo-config-<nom>.json », où <nom> ne garde que les lettres ASCII, chiffres, « - » et « _ »
// (les autres caractères deviennent « - », répétitions et bords retirés, 60 caractères au plus) ;
// « watchoo-config.json » s'il ne reste rien. Sert à l'en-tête Content-Disposition du web.
func FileName(workspaceName string) string {
	var b strings.Builder
	dash := false
	for _, r := range workspaceName {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		switch {
		case ok && r != '-':
			b.WriteRune(r)
			dash = false
		case !dash && b.Len() > 0:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= 60 {
			break
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "watchoo-config.json"
	}
	return "watchoo-config-" + slug + ".json"
}
