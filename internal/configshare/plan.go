package configshare

// Aperçu (Plan) et application (Apply) d'un import.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/model"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Action est ce qu'un import fait (ou ferait) d'un dépôt. Valeurs stables (sorties --json).
type Action string

// Valeurs de Action.
const (
	ActionAdd          Action = "add"          // dépôt absent de l'espace : ajouté
	ActionUpdate       Action = "update"       // déjà présent : label, branches ou seuils changés, ou suivi repris
	ActionUnchanged    Action = "unchanged"    // déjà suivi, rien à changer
	ActionInaccessible Action = "inaccessible" // vérification d'accès : introuvable ou sans accès
	ActionError        Action = "error"        // vérification d'accès : autre échec (réseau, quota…)
)

// Codes des avertissements. Valeurs stables (sorties --json).
const (
	// WarnEnvOverride : une variable WATCHOO_* masque un réglage modifié par l'import (Key, EnvVar).
	WarnEnvOverride = "env_override"
	// WarnCommentsLost : config.yaml sera réécrit, ses commentaires personnels perdus (copie .bak).
	WarnCommentsLost = "comments_lost"
)

// Preview est l'aperçu d'un import : ce qu'Apply ferait, sans rien écrire.
type Preview struct {
	Source       Source          // origine du document (textes non fiables, à nettoyer à l'affichage)
	Settings     SettingsPreview // réglages
	Repositories []RepoChange    // un élément par dépôt du document, dans l'ordre du fichier
	Summary      Summary         // décompte prévu (Added = à ajouter, Updated = à mettre à jour…)
	Warnings     []Warning       // avertissements, dans un ordre stable
	// AccessChecked indique que l'accès aux dépôts a été vérifié (PlanOptions.CheckAccess) : les
	// actions viennent alors de wsync.AddRepositories en simulation.
	AccessChecked bool
}

// SettingsPreview décrit les réglages d'un import.
type SettingsPreview struct {
	Provided []string        // clés fournies par le document, dans l'ordre de workspace.SharedKeys
	Changes  []SettingChange // clés dont la valeur change, dans le même ordre
	// Rewrite indique que config.yaml sera réécrit (au moins un réglage change).
	Rewrite bool
	// Masked liste les clés fournies masquées par une variable WATCHOO_* définie, modifiées ou non
	// (chacune donne un WarnEnvOverride).
	Masked []workspace.EnvOverride
}

// SettingChange est un réglage modifié. Before et After sont écrits par
// workspace.FormatSettingValue (chaînes entre guillemets, listes ["a", "b"], durées 30m).
type SettingChange struct {
	Key    string // clé complète (« sync.interval »)
	Before string // valeur en vigueur dans l'espace (celle du fichier si EnvVar est renseigné)
	After  string // valeur importée
	EnvVar string // variable WATCHOO_* définie qui masque ce réglage ; "" sinon
}

// RepoChange est un dépôt du document dans l'aperçu (et, après Apply, son résultat).
type RepoChange struct {
	Index  int    // position dans le document (1 = premier)
	Repo   string // dépôt tel qu'écrit, normalisé en owner/name (repoimport.Ref.String)
	Action Action
	// Reenable : le dépôt est présent mais arrêté, l'import reprend son suivi (Action = update).
	Reenable bool
	// Before est l'état actuel dans l'espace ; nil si le dépôt n'y est pas (ajout).
	Before *RepoState
	// After est l'état après l'import (pour inaccessible / error : celui qui était prévu).
	After RepoState
	// Outcome est le résultat de l'application (wsync.AddOutcome) ; "" dans un aperçu.
	Outcome wsync.AddOutcome
	// Repository est le dépôt enregistré par Apply (owner/name actuels de la forge) ; nil dans un
	// aperçu et pour un dépôt inaccessible ou en erreur.
	Repository *model.Repository
	// Error est le message (français, sans token) d'un dépôt inaccessible ou en erreur ; "" sinon.
	Error string
}

// RepoState est le nom affiché, les branches, les seuils d'activité et le suivi d'un dépôt.
type RepoState struct {
	Label    string   // "" = pas de nom affiché
	Branches []string // jamais nil ; vide = toutes les branches
	// FlemmeAfterDays et DeadAfterDays sont les seuils propres au dépôt ; nil = seuil de l'espace.
	FlemmeAfterDays *int
	DeadAfterDays   *int
	Enabled         bool // suivi actif
}

// Summary compte les dépôts par résultat (prévu dans un aperçu, obtenu après Apply). Mêmes clés que
// le bilan de « watchoo repo import ».
type Summary struct {
	Added          int // à ajouter / ajoutés
	AlreadyTracked int // inchangés / déjà suivis
	Updated        int // à mettre à jour / mis à jour (reprises comprises)
	Reenabled      int // parmi Updated : suivis repris
	Inaccessible   int
	Errors         int
}

// Warning est un avertissement de l'aperçu.
type Warning struct {
	Code    string // WarnEnvOverride ou WarnCommentsLost
	Key     string // réglage concerné (WarnEnvOverride) ; "" sinon
	Message string // message en français
}

// PlanOptions règle Plan.
type PlanOptions struct {
	// CheckAccess, si non nil, vérifie l'accès à chaque dépôt (AddRepositories en DryRun : une
	// requête par dépôt, rien n'est écrit), comme « repo import --dry-run ». Sans lui, l'aperçu est
	// calculé avec la base seule (dépôts cherchés par owner/name sans tenir compte de la casse : un
	// dépôt renommé sur la forge peut y apparaître « à ajouter »).
	CheckAccess Adder
}

// Plan calcule l'aperçu de l'import de f dans l'espace ws, sans rien écrire (ni base, ni
// config.yaml). Il valide d'abord les réglages fusionnés avec ceux de l'espace
// (f.Settings.ApplyTo(ws.Config).Validate()) : en cas de refus, l'erreur enveloppe ErrInvalid.
//
// Réglages : un réglage fourni dont la valeur diffère de celle en vigueur (ws.Config) est une
// modification ; config.yaml sera réécrit s'il y en a au moins une (WarnCommentsLost si le fichier
// existe), et chaque réglage modifié masqué par une variable WATCHOO_* définie donne WarnEnvOverride.
//
// Dépôts : comme wsync.AddRepositories. Absent : ActionAdd (branches du document, ou toutes).
// Présent : label remplacé si le document en donne un non vide, branches remplacées si le document
// les donne (liste vide = toutes), seuils remplacés si le document les donne (0 = seuil de l'espace),
// suivi repris s'il était arrêté ; ActionUpdate si quelque chose
// change, sinon ActionUnchanged.
func Plan(ctx context.Context, ws *workspace.Workspace, st Store, f *File, opts PlanOptions) (*Preview, error) {
	if ws == nil || st == nil || f == nil {
		return nil, errors.New("aperçu de l'import : espace, base ou fichier non renseigné")
	}
	merged := f.Settings.ApplyTo(ws.Config)
	if err := merged.Validate(); err != nil {
		return nil, fmt.Errorf("%w : réglages refusés pour cet espace ; rien n'a été importé : %w", ErrInvalid, err)
	}
	// Configuration réellement en vigueur après l'import : les variables WATCHOO_* définies
	// reprennent la main sur les valeurs importées, comme au chargement du fichier réécrit
	// (workspace.WriteSharedSettings). L'aperçu refuse donc ce que l'écriture refuserait.
	effective, err := workspace.WithEnv(merged)
	if err != nil {
		return nil, fmt.Errorf("aperçu de l'import : %w", err)
	}
	if err := effective.Validate(); err != nil {
		return nil, fmt.Errorf("%w : réglages refusés pour cet espace une fois les variables WATCHOO_* appliquées "+
			"(retirez la variable ou adaptez le fichier) ; rien n'a été importé : %w", ErrInvalid, err)
	}

	p := &Preview{Source: f.Source, Repositories: make([]RepoChange, 0, len(f.Repositories))}
	p.Settings, err = settingsPreview(ws, f.Settings)
	if err != nil {
		return nil, err
	}
	for _, o := range p.Settings.Masked {
		detail := "la valeur importée sera écrite dans config.yaml"
		if !p.Settings.changes(o.Key) {
			detail = "config.yaml contient déjà la valeur importée"
		}
		p.Warnings = append(p.Warnings, Warning{Code: WarnEnvOverride, Key: o.Key, Message: fmt.Sprintf(
			"la variable %s est définie : elle masque %s (%s, mais la variable l'emporte tant qu'elle est définie)",
			o.Var, o.Key, detail)})
	}
	if p.Settings.Rewrite {
		if _, err := os.Stat(ws.ConfigPath()); err == nil {
			p.Warnings = append(p.Warnings, Warning{Code: WarnCommentsLost, Message: fmt.Sprintf(
				"config.yaml sera réécrit à partir du modèle de « watchoo init » : ses commentaires personnels "+
					"seront perdus (copie de l'ancien fichier dans %s)", workspace.BackupFileName)})
		}
	}

	existing, err := st.ListRepositories(ctx)
	if err != nil {
		return nil, fmt.Errorf("aperçu de l'import : lecture des dépôts : %w", err)
	}
	byName := make(map[string]model.Repository, len(existing))
	byID := make(map[int64]model.Repository, len(existing))
	for _, r := range existing {
		byName[strings.ToLower(r.FullName())] = r
		byID[r.ID] = r
	}
	for _, e := range f.Repositories {
		c := RepoChange{Index: e.Index, Repo: e.Ref.String()}
		if r, ok := byName[strings.ToLower(e.Ref.String())]; ok {
			planExisting(&c, r, e)
		} else {
			c.Action = ActionAdd
			c.After = RepoState{Label: e.Label, Branches: nonNil(e.Branches), Enabled: true,
				FlemmeAfterDays: threshold(e.FlemmeAfterDays, nil), DeadAfterDays: threshold(e.DeadAfterDays, nil)}
		}
		p.Repositories = append(p.Repositories, c)
	}

	if opts.CheckAccess != nil && len(f.Repositories) > 0 {
		results, err := opts.CheckAccess.AddRepositories(ctx, addRequests(f), wsync.AddOptions{DryRun: true})
		if err != nil {
			return nil, fmt.Errorf("aperçu de l'import : %w", err)
		}
		for i, res := range results {
			if i >= len(p.Repositories) {
				break
			}
			applyAccess(&p.Repositories[i], res, byID)
		}
		p.AccessChecked = true
	}
	p.Summary = summarize(p.Repositories)
	return p, nil
}

// planExisting calcule l'action pour un dépôt déjà présent r.
func planExisting(c *RepoChange, r model.Repository, e Entry) {
	before := stateOf(r)
	after := before
	after.Branches = slices.Clone(before.Branches)
	if e.Label != "" {
		after.Label = e.Label
	}
	if e.Branches != nil {
		after.Branches = nonNil(e.Branches)
	}
	after.FlemmeAfterDays = threshold(e.FlemmeAfterDays, before.FlemmeAfterDays)
	after.DeadAfterDays = threshold(e.DeadAfterDays, before.DeadAfterDays)
	after.Enabled = true
	c.Before, c.After = &before, after
	c.Reenable = !before.Enabled
	if reflect.DeepEqual(before, after) {
		c.Action = ActionUnchanged
	} else {
		c.Action = ActionUpdate
	}
}

// applyAccess remplace l'action locale de c par le résultat de la vérification d'accès.
func applyAccess(c *RepoChange, res wsync.AddResult, byID map[int64]model.Repository) {
	switch res.Outcome {
	case wsync.OutcomeAdded:
		c.Action, c.Before, c.Reenable = ActionAdd, nil, false
		c.After = stateOf(res.Repo)
	case wsync.OutcomeAlreadyTracked, wsync.OutcomeUpdated:
		if r, ok := byID[res.Repo.ID]; ok {
			before := stateOf(r)
			c.Before, c.Reenable = &before, !r.Enabled
		}
		c.After = stateOf(res.Repo)
		c.Action = ActionUnchanged
		if res.Outcome == wsync.OutcomeUpdated {
			c.Action = ActionUpdate
		}
	case wsync.OutcomeInaccessible:
		c.Action, c.Error = ActionInaccessible, res.Error
	default:
		c.Action, c.Error = ActionError, res.Error
	}
}

// settingsPreview compare les réglages fournis à ceux de l'espace. Un réglage non masqué est comparé
// à sa valeur en vigueur (ws.Config) ; un réglage masqué par une variable WATCHOO_* définie est
// comparé à sa valeur au niveau des fichiers (workspace.FileConfig), puisque c'est elle que l'import
// réécrit : la valeur de la variable ne compte pas, et réimporter le même fichier ne change rien.
func settingsPreview(ws *workspace.Workspace, s workspace.SharedSettings) (SettingsPreview, error) {
	out := SettingsPreview{Provided: s.Keys()}
	out.Masked = workspace.EnvOverrides(out.Provided)
	current := workspace.SharedSettingsFrom(ws.Config).Values()
	masked := map[string]string{}
	if len(out.Masked) > 0 {
		fileCfg, err := workspace.FileConfig(ws)
		if err != nil {
			return SettingsPreview{}, fmt.Errorf("aperçu de l'import : lecture des réglages de l'espace : %w", err)
		}
		fileValues := workspace.SharedSettingsFrom(fileCfg).Values()
		for _, o := range out.Masked {
			masked[o.Key] = o.Var
			current[o.Key] = fileValues[o.Key]
		}
	}
	imported := s.Values()
	for _, key := range out.Provided {
		if reflect.DeepEqual(current[key], imported[key]) {
			continue
		}
		out.Changes = append(out.Changes, SettingChange{
			Key:    key,
			Before: workspace.FormatSettingValue(current[key]),
			After:  workspace.FormatSettingValue(imported[key]),
			EnvVar: masked[key],
		})
	}
	out.Rewrite = len(out.Changes) > 0
	return out, nil
}

// changes indique que le réglage key fait partie des modifications.
func (p SettingsPreview) changes(key string) bool {
	return slices.ContainsFunc(p.Changes, func(c SettingChange) bool { return c.Key == key })
}

// ApplyOptions règle Apply.
type ApplyOptions struct {
	// Progress, si non nil, reçoit l'avancement de la vérification des dépôts (wsync.AddOptions).
	Progress func(done, total int)
}

// SettingsResult dit ce qu'Apply a fait de config.yaml.
type SettingsResult struct {
	Written    bool   // config.yaml réécrit
	Path       string // chemin de config.yaml ; "" si non réécrit
	BackupPath string // copie de l'ancien fichier ; "" si non réécrit ou s'il n'existait pas
}

// Result est le bilan d'Apply.
type Result struct {
	// Preview est l'aperçu recalculé au moment d'appliquer (sans vérification d'accès préalable) ;
	// ses Repositories portent ensuite le résultat de chaque dépôt (Outcome, Repository, Error, et
	// Action / Before / After mis à jour d'après la forge).
	Preview  *Preview
	Settings SettingsResult
	Summary  Summary // décompte des résultats obtenus
}

// Incomplete indique qu'il reste des dépôts inaccessibles ou en erreur (code de sortie non nul de la
// CLI, comme « repo import »).
func (r *Result) Incomplete() bool {
	return r != nil && r.Summary.Inaccessible+r.Summary.Errors > 0
}

// Apply importe f dans l'espace ws. Tout est d'abord revalidé et l'aperçu recalculé (Plan) : jamais
// de confiance dans un aperçu montré plus tôt. Ensuite, dans cet ordre :
//  1. si au moins un réglage change, workspace.WriteSharedSettings réécrit config.yaml (copie .bak) ;
//     une erreur ici arrête tout, sans toucher aux dépôts. ws.Config n'est pas modifié ;
//  2. si le document contient des dépôts, adder.AddRepositories les vérifie et les enregistre (un
//     dépôt inaccessible ne bloque pas les autres ; rien n'est retiré, aucune synchro n'est lancée).
//
// Une erreur globale de l'étape 2 (token, contexte annulé) est renvoyée AVEC le résultat : les
// réglages peuvent alors déjà être écrits (Result.Settings).
func Apply(ctx context.Context, ws *workspace.Workspace, st Store, adder Adder, f *File, opts ApplyOptions) (*Result, error) {
	p, err := Plan(ctx, ws, st, f, PlanOptions{})
	if err != nil {
		return nil, err
	}
	if len(f.Repositories) > 0 && adder == nil {
		return nil, errors.New("import : ajout de dépôts non disponible")
	}
	res := &Result{Preview: p}
	if p.Settings.Rewrite {
		w, err := workspace.WriteSharedSettings(ws, f.Settings)
		if err != nil {
			return nil, fmt.Errorf("écriture des réglages : %w ; rien n'a été importé", err)
		}
		res.Settings = SettingsResult{Written: true, Path: w.Path, BackupPath: w.BackupPath}
	}
	if len(f.Repositories) == 0 {
		return res, nil
	}

	existing, err := st.ListRepositories(ctx)
	if err != nil {
		return res, fmt.Errorf("import des dépôts : lecture des dépôts : %w", err)
	}
	byID := make(map[int64]model.Repository, len(existing))
	for _, r := range existing {
		byID[r.ID] = r
	}
	results, err := adder.AddRepositories(ctx, addRequests(f), wsync.AddOptions{Progress: opts.Progress})
	for i, r := range results {
		if i >= len(p.Repositories) {
			break
		}
		c := &p.Repositories[i]
		applyAccess(c, r, byID)
		c.Outcome = r.Outcome
		if r.Outcome != wsync.OutcomeInaccessible && r.Outcome != wsync.OutcomeError {
			repo := r.Repo
			c.Repository = &repo
		}
	}
	// Dépôts non traités (erreur globale en cours de route) : comptés en erreur.
	for i := len(results); i < len(p.Repositories); i++ {
		c := &p.Repositories[i]
		c.Action, c.Outcome = ActionError, wsync.OutcomeError
		if c.Error == "" {
			c.Error = "non traité : import interrompu"
		}
	}
	res.Summary = summarize(p.Repositories)
	p.Summary = res.Summary
	if err != nil {
		return res, fmt.Errorf("import des dépôts : %w", err)
	}
	return res, nil
}

// addRequests convertit les dépôts du document en demandes d'ajout (Line = position dans le document).
func addRequests(f *File) []wsync.AddRequest {
	reqs := make([]wsync.AddRequest, len(f.Repositories))
	for i, e := range f.Repositories {
		reqs[i] = wsync.AddRequest{Line: e.Index, Ref: e.Ref, Label: e.Label, BranchPatterns: slices.Clone(e.Branches),
			FlemmeAfterDays: clonePtr(e.FlemmeAfterDays), DeadAfterDays: clonePtr(e.DeadAfterDays)}
		if e.Branches != nil && reqs[i].BranchPatterns == nil {
			reqs[i].BranchPatterns = []string{}
		}
	}
	return reqs
}

// summarize compte les dépôts par action.
func summarize(changes []RepoChange) Summary {
	var s Summary
	for _, c := range changes {
		switch c.Action {
		case ActionAdd:
			s.Added++
		case ActionUpdate:
			s.Updated++
			if c.Reenable {
				s.Reenabled++
			}
		case ActionUnchanged:
			s.AlreadyTracked++
		case ActionInaccessible:
			s.Inaccessible++
		default:
			s.Errors++
		}
	}
	return s
}

// stateOf renvoie l'état (label, branches, seuils, suivi) d'un dépôt.
func stateOf(r model.Repository) RepoState {
	return RepoState{Label: r.Label, Branches: nonNil(r.BranchPatterns), Enabled: r.Enabled,
		FlemmeAfterDays: clonePtr(r.FlemmeAfterDays), DeadAfterDays: clonePtr(r.DeadAfterDays)}
}

// threshold renvoie un seuil après l'import : current si le document ne le donne pas (nil), nil
// (seuil de l'espace) s'il vaut 0, sinon une copie de la valeur du document. Même règle que
// wsync.AddRepositories.
func threshold(doc, current *int) *int {
	switch {
	case doc == nil:
		return clonePtr(current)
	case *doc == 0:
		return nil
	default:
		return clonePtr(doc)
	}
}

// nonNil renvoie une copie de list, vide plutôt que nil.
func nonNil(list []string) []string {
	out := slices.Clone(list)
	if out == nil {
		out = []string{}
	}
	return out
}
