package workspace

// Réglages communs partagés entre superviseurs (« watchoo config import », page Réglages) et
// réécriture de .watchoo/config.yaml (documentation/10-espaces-de-supervision.md, « Partager les
// réglages avec d'autres superviseurs »). Le format du fichier de partage et sa validation sont dans
// internal/configshare ; ce fichier ne connaît que les réglages et config.yaml.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// BackupFileName est la copie de l'ancien config.yaml gardée par WriteSharedSettings, dans DirName.
const BackupFileName = ConfigFileName + ".bak"

// Clés des réglages communs, dans l'ordre du modèle de config.yaml. Ce sont les seules qu'un import
// peut modifier : name, github.* (compte et chemin de gh : un fichier venu d'ailleurs ne doit jamais
// choisir le programme lancé), git.* (protocole des clones Git locaux) et web.* restent propres à la
// machine.
const (
	KeySyncAuto        = "sync.auto"
	KeySyncInterval    = "sync.interval"
	KeySyncParallelism = "sync.parallelism"
	KeyFlemmeAfterDays = "activity.flemme_after_days"
	KeyDeadAfterDays   = "activity.dead_after_days"
	KeyBranchesExclude = "branches.exclude"
	KeyTimezone        = "timezone"
)

// SharedKeys liste les clés des réglages communs, dans l'ordre du modèle de config.yaml.
var SharedKeys = []string{
	KeySyncAuto, KeySyncInterval, KeySyncParallelism,
	KeyFlemmeAfterDays, KeyDeadAfterDays,
	KeyBranchesExclude, KeyTimezone,
}

// SharedSettings regroupe des valeurs de réglages communs. Chaque champ nil veut dire « non fourni » :
// la valeur en place est gardée. La valeur zéro ne fournit donc rien (IsZero).
type SharedSettings struct {
	SyncAuto        *bool
	SyncInterval    *time.Duration
	SyncParallelism *int
	FlemmeAfterDays *int
	DeadAfterDays   *int
	BranchesExclude *[]string // liste vide (non nil) = aucune branche exclue
	Timezone        *string   // "" = fuseau du système
}

// SharedSettingsFrom renvoie tous les réglages communs de c (chaque champ renseigné, listes copiées ;
// branches.exclude vaut une liste vide plutôt que nil).
func SharedSettingsFrom(c Config) SharedSettings {
	exclude := slices.Clone(c.Branches.Exclude)
	if exclude == nil {
		exclude = []string{}
	}
	return SharedSettings{
		SyncAuto:        &c.Sync.Auto,
		SyncInterval:    &c.Sync.Interval,
		SyncParallelism: &c.Sync.Parallelism,
		FlemmeAfterDays: &c.Activity.FlemmeAfterDays,
		DeadAfterDays:   &c.Activity.DeadAfterDays,
		BranchesExclude: &exclude,
		Timezone:        &c.Timezone,
	}
}

// Values renvoie les réglages fournis, clé complète → valeur typée (bool, time.Duration, int,
// []string copiée et jamais nil, string).
func (s SharedSettings) Values() map[string]any {
	out := map[string]any{}
	if s.SyncAuto != nil {
		out[KeySyncAuto] = *s.SyncAuto
	}
	if s.SyncInterval != nil {
		out[KeySyncInterval] = *s.SyncInterval
	}
	if s.SyncParallelism != nil {
		out[KeySyncParallelism] = *s.SyncParallelism
	}
	if s.FlemmeAfterDays != nil {
		out[KeyFlemmeAfterDays] = *s.FlemmeAfterDays
	}
	if s.DeadAfterDays != nil {
		out[KeyDeadAfterDays] = *s.DeadAfterDays
	}
	if s.BranchesExclude != nil {
		list := slices.Clone(*s.BranchesExclude)
		if list == nil {
			list = []string{}
		}
		out[KeyBranchesExclude] = list
	}
	if s.Timezone != nil {
		out[KeyTimezone] = *s.Timezone
	}
	return out
}

// Keys renvoie les clés des réglages fournis, dans l'ordre de SharedKeys.
func (s SharedSettings) Keys() []string {
	values := s.Values()
	var keys []string
	for _, k := range SharedKeys {
		if _, ok := values[k]; ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// IsZero indique qu'aucun réglage n'est fourni.
func (s SharedSettings) IsZero() bool {
	return len(s.Values()) == 0
}

// ApplyTo renvoie c avec les réglages fournis à la place des siens (listes copiées). c n'est pas
// modifié. Le résultat n'est pas validé : appeler Validate.
func (s SharedSettings) ApplyTo(c Config) Config {
	out := c
	out.Branches.Exclude = slices.Clone(c.Branches.Exclude)
	out.Web.AllowedHosts = slices.Clone(c.Web.AllowedHosts)
	if s.SyncAuto != nil {
		out.Sync.Auto = *s.SyncAuto
	}
	if s.SyncInterval != nil {
		out.Sync.Interval = *s.SyncInterval
	}
	if s.SyncParallelism != nil {
		out.Sync.Parallelism = *s.SyncParallelism
	}
	if s.FlemmeAfterDays != nil {
		out.Activity.FlemmeAfterDays = *s.FlemmeAfterDays
	}
	if s.DeadAfterDays != nil {
		out.Activity.DeadAfterDays = *s.DeadAfterDays
	}
	if s.BranchesExclude != nil {
		out.Branches.Exclude = slices.Clone(*s.BranchesExclude)
		if out.Branches.Exclude == nil {
			out.Branches.Exclude = []string{}
		}
	}
	if s.Timezone != nil {
		out.Timezone = *s.Timezone
	}
	return out
}

// EnvOverride signale une variable d'environnement définie (non vide) qui masque un réglage : la
// valeur du fichier ne s'applique pas tant qu'elle est définie.
type EnvOverride struct {
	Key string // clé du réglage (« sync.interval »)
	Var string // variable (« WATCHOO_SYNC_INTERVAL »)
}

// EnvVarName renvoie le nom de la variable d'environnement d'une clé : « sync.interval » →
// « WATCHOO_SYNC_INTERVAL ».
func EnvVarName(key string) string {
	return envVarName(key)
}

// EnvOverrides renvoie, dans l'ordre de keys, les variables WATCHOO_* définies et non vides qui masquent
// ces clés.
func EnvOverrides(keys []string) []EnvOverride {
	var out []EnvOverride
	for _, key := range keys {
		name := envVarName(key)
		if os.Getenv(name) != "" {
			out = append(out, EnvOverride{Key: key, Var: name})
		}
	}
	return out
}

// FileConfig renvoie les réglages de l'espace au niveau des fichiers, SANS les variables WATCHOO_* :
// valeurs par défaut, fichier utilisateur puis config.yaml de l'espace. C'est la configuration qui
// serait en vigueur si aucune variable n'était définie ; elle n'est pas validée (une variable peut
// corriger un fichier qui, seul, serait refusé). Même contrôle des droits que Load.
//
// L'aperçu d'un import s'en sert pour comparer un réglage masqué par une variable à la valeur du
// fichier, et non à celle de la variable.
func FileConfig(ws *Workspace) (Config, error) {
	if ws == nil || ws.Root == "" || ws.Dir == "" {
		return Config{}, errors.New("lecture des réglages : espace non renseigné")
	}
	if err := checkTrusted(ws); err != nil {
		return Config{}, err
	}
	return loadLayers(ws.Root, ws.ConfigPath(), false)
}

// WithEnv renvoie c avec, à la place des siennes, les valeurs des variables WATCHOO_* définies et non
// vides (comme Load les applique après les fichiers). c n'est pas modifié ; le résultat n'est pas
// validé. Une variable illisible renvoie une erreur, comme Load.
func WithEnv(c Config) (Config, error) {
	out := c
	out.Branches.Exclude = slices.Clone(c.Branches.Exclude)
	out.Web.AllowedHosts = slices.Clone(c.Web.AllowedHosts)
	var errs []error
	for _, f := range settingFields(&out) {
		name := envVarName(f.key)
		value := os.Getenv(name)
		if value == "" {
			continue
		}
		if err := decodeSetting(f.ptr, value); err != nil {
			errs = append(errs, fmt.Errorf("variable %s (%s) : %w", name, f.key, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return out, nil
}

// WriteResult est le résultat de WriteSharedSettings.
type WriteResult struct {
	Path string // config.yaml réécrit
	// BackupPath est la copie de l'ancien fichier (Dir/config.yaml.bak) ; vide s'il n'y avait pas de
	// config.yaml.
	BackupPath string
	// Config est la configuration relue avec le nouveau fichier (utilisateur, espace, environnement),
	// validée. ws.Config n'est PAS modifié : un processus en cours (« watchoo serve ») garde sa
	// configuration jusqu'à son prochain lancement.
	Config Config
}

// writeTemp crée un fichier temporaire dans dir ; c'est une variable pour que les tests simulent un
// échec d'écriture.
var writeTemp = writeTempFile

// WriteSharedSettings réécrit ws.ConfigPath() avec les réglages communs s, à partir du modèle commenté
// de Init (renderConfig) :
//   - les clés propres à la machine présentes dans l'ancien fichier (name, github.*, git.*, web.*) y sont
//     reprises telles quelles, de même que les réglages communs que s ne fournit pas ;
//   - les réglages fournis par s sont écrits actifs, échappés par FormatSettingValue ;
//   - les commentaires personnels de l'ancien fichier sont perdus ; il est copié dans
//     Dir/BackupFileName (FileMode, remplacé à chaque appel) ;
//   - le nouveau fichier est écrit dans un fichier temporaire de Dir (FileMode), relu
//     (readSettingsFile) et comparé clé par clé aux valeurs attendues (aucune clé en plus ni en moins),
//     puis chargé comme par Load (fichier utilisateur, nouveau fichier, environnement) et validé ; en
//     cas d'écart ou d'erreur, rien n'est changé (le fichier temporaire est supprimé) ;
//   - il remplace enfin config.yaml par renommage (atomique sur un même système de fichiers).
//
// L'espace doit appartenir à l'utilisateur (même contrôle que Load). L'appelant décide s'il faut
// écrire : WriteSharedSettings écrit même si rien ne change (le résultat est alors identique, à la
// copie .bak près). s ne doit fournir que des valeurs valides (sinon : erreur, rien n'est écrit).
func WriteSharedSettings(ws *Workspace, s SharedSettings) (result WriteResult, err error) {
	if ws == nil || ws.Root == "" || ws.Dir == "" {
		return WriteResult{}, errors.New("écriture des réglages : espace non renseigné")
	}
	if err := checkTrusted(ws); err != nil {
		return WriteResult{}, err
	}
	path := ws.ConfigPath()
	old, err := os.ReadFile(path)
	exists := err == nil
	if err != nil && !isNotExist(err) {
		return WriteResult{}, fmt.Errorf("lecture de %s : %w", path, err)
	}
	oldValues, err := fileValues(path)
	if err != nil {
		return WriteResult{}, fmt.Errorf("réécriture de %s impossible, le fichier actuel est invalide : %w", path, err)
	}

	expected := map[string]any{}
	for key, v := range oldValues {
		expected[key] = v // clés machine et réglages communs non fournis : gardés
	}
	for key, v := range s.Values() {
		expected[key] = v
	}
	for key, v := range expected {
		if str, ok := v.(string); ok && !utf8.ValidString(str) {
			return WriteResult{}, fmt.Errorf("réglage %s : texte UTF-8 invalide, rien n'a été écrit", key)
		}
		if list, ok := v.([]string); ok && slices.ContainsFunc(list, func(x string) bool { return !utf8.ValidString(x) }) {
			return WriteResult{}, fmt.Errorf("réglage %s : texte UTF-8 invalide, rien n'a été écrit", key)
		}
	}
	content := renderConfig(filepath.Base(ws.Root), expected)

	tmp, err := writeTemp(ws.Dir, ".config.yaml.tmp-*", []byte(content))
	if err != nil {
		return WriteResult{}, err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmp)
		}
	}()

	written, err := fileValues(tmp)
	if err != nil {
		return WriteResult{}, fmt.Errorf("relecture des réglages écrits : %w ; config.yaml n'a pas été modifié", err)
	}
	if err := compareValues(expected, written); err != nil {
		return WriteResult{}, fmt.Errorf("réglages écrits différents des réglages attendus (%w) ; "+
			"config.yaml n'a pas été modifié", err)
	}
	cfg, err := loadConfig(ws.Root, tmp)
	if err != nil {
		return WriteResult{}, fmt.Errorf("%w ; config.yaml n'a pas été modifié", err)
	}

	result = WriteResult{Path: path, Config: cfg}
	if exists {
		backup := filepath.Join(ws.Dir, BackupFileName)
		bakTmp, err := writeTemp(ws.Dir, ".config.yaml.bak.tmp-*", old)
		if err != nil {
			return WriteResult{}, err
		}
		if err := os.Rename(bakTmp, backup); err != nil {
			_ = os.Remove(bakTmp)
			return WriteResult{}, fmt.Errorf("copie de sauvegarde %s : %w ; config.yaml n'a pas été modifié", backup, err)
		}
		result.BackupPath = backup
	}
	if err := os.Rename(tmp, path); err != nil {
		return WriteResult{}, fmt.Errorf("remplacement de %s : %w", path, err)
	}
	return result, nil
}

// fileValues lit un fichier de réglages (readSettingsFile : clés connues, valeurs décodables) et
// renvoie ses réglages présents, clé complète → valeur typée (comme les champs de Config). Fichier
// absent : carte vide.
func fileValues(file string) (map[string]any, error) {
	nested, err := readSettingsFile(file)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	var scratch Config
	for _, f := range settingFields(&scratch) {
		raw, ok := lookupNested(nested, f.key)
		if !ok {
			continue
		}
		if err := decodeSetting(f.ptr, raw); err != nil {
			return nil, fmt.Errorf("%s : %s : %w", file, f.key, err)
		}
		v := reflect.ValueOf(f.ptr).Elem().Interface()
		if list, ok := v.([]string); ok && list == nil {
			v = []string{}
		}
		out[f.key] = v
	}
	return out, nil
}

// lookupNested cherche la clé pointée key (« sync.interval ») dans une carte imbriquée.
func lookupNested(m map[string]any, key string) (any, bool) {
	parts := strings.Split(key, ".")
	for _, part := range parts[:len(parts)-1] {
		sub, ok := m[part].(map[string]any)
		if !ok {
			return nil, false
		}
		m = sub
	}
	v, ok := m[parts[len(parts)-1]]
	return v, ok
}

// compareValues vérifie que got contient exactement les clés et valeurs de want.
func compareValues(want, got map[string]any) error {
	var errs []error
	for key, w := range want {
		g, ok := got[key]
		switch {
		case !ok:
			errs = append(errs, fmt.Errorf("clé %s absente", key))
		case !reflect.DeepEqual(w, g):
			errs = append(errs, fmt.Errorf("clé %s : %s au lieu de %s", key, FormatSettingValue(g), FormatSettingValue(w)))
		}
	}
	for key := range got {
		if _, ok := want[key]; !ok {
			errs = append(errs, fmt.Errorf("clé %s inattendue", key))
		}
	}
	slices.SortFunc(errs, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })
	return errors.Join(errs...)
}

// writeTempFile crée dans dir un fichier temporaire (motif pattern, os.CreateTemp : jamais
// d'écrasement), force FileMode, y écrit content, le synchronise sur disque et renvoie son chemin. En
// cas d'erreur, le fichier est supprimé.
func writeTempFile(dir, pattern string, content []byte) (path string, err error) {
	f, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", fmt.Errorf("création d'un fichier temporaire dans %s : %w", dir, err)
	}
	path = f.Name()
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("écriture de %s : %w", path, cerr)
		}
		if err != nil {
			_ = os.Remove(path)
		}
	}()
	if err := f.Chmod(FileMode); err != nil {
		return "", fmt.Errorf("droits de %s : %w", path, err)
	}
	if _, err := f.Write(content); err != nil {
		return "", fmt.Errorf("écriture de %s : %w", path, err)
	}
	if err := f.Sync(); err != nil {
		return "", fmt.Errorf("écriture de %s : %w", path, err)
	}
	return path, nil
}
