package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/studoo-app/watchoo/internal/logo"
)

// Valeurs par défaut et bornes des réglages (voir documentation/03-architecture.md et
// documentation/10-espaces-de-supervision.md).
const (
	DefaultSyncInterval    = 30 * time.Minute // sync.interval
	MinSyncInterval        = 15 * time.Minute // minimum de sync.interval quand il n'est pas nul
	DefaultSyncParallelism = 4                // sync.parallelism
	MaxSyncParallelism     = 4                // maximum de sync.parallelism
	DefaultFlemmeAfterDays = 5                // activity.flemme_after_days
	DefaultDeadAfterDays   = 15               // activity.dead_after_days
	DefaultWebAddr         = "127.0.0.1:8080" // web.addr
	DefaultGHPath          = "gh"             // github.gh_path
	GitProtocolSSH         = "ssh"            // git.protocol : git@github.com:owner/name.git (défaut)
	GitProtocolHTTPS       = "https"          // git.protocol : https://github.com/owner/name.git
	EnvPrefix              = "WATCHOO"        // préfixe des variables d'environnement (WATCHOO_SYNC_INTERVAL…)
	UserConfigDirName      = "watchoo"        // sous-répertoire de $XDG_CONFIG_HOME (ou ~/.config)
)

// Config regroupe tous les réglages d'un espace, après fusion des trois niveaux (voir Load).
//
// Les balises mapstructure donnent le nom des clés YAML et viper ; la clé complète d'un champ imbriqué
// est « section.clé » (ex. sync.interval), et sa variable d'environnement WATCHOO_SECTION_CLÉ en
// majuscules (ex. WATCHOO_SYNC_INTERVAL, WATCHOO_ACTIVITY_FLEMME_AFTER_DAYS, WATCHOO_GITHUB_GH_PATH).
// Ces balises sont la seule liste des clés connues : Load les parcourt pour déclarer les valeurs par
// défaut, refuser les clés inconnues et décoder chaque réglage.
type Config struct {
	// Name est le nom de l'espace, affiché dans le web, le TUI et « watchoo status ».
	// Défaut : nom du répertoire racine de l'espace.
	Name     string         `mapstructure:"name"`
	Sync     SyncConfig     `mapstructure:"sync"`
	Activity ActivityConfig `mapstructure:"activity"`
	Branches BranchesConfig `mapstructure:"branches"`
	Web      WebConfig      `mapstructure:"web"`
	// Timezone est le fuseau d'affichage et de regroupement par jour / semaine : vide = fuseau du
	// système, sinon un nom IANA valide (ex. « Europe/Paris »).
	Timezone string       `mapstructure:"timezone"`
	GitHub   GitHubConfig `mapstructure:"github"`
	Git      GitConfig    `mapstructure:"git"`
}

// SyncConfig regroupe les réglages de synchronisation (clés sync.*).
type SyncConfig struct {
	// Auto active la synchro automatique de « serve » et « tui » (défaut true).
	Auto bool `mapstructure:"auto"`
	// Interval est l'intervalle de la synchro automatique (défaut 30m). 0 = synchro manuelle seule ;
	// sinon au moins MinSyncInterval. Écrit en YAML / variable d'environnement sous forme de durée Go
	// (« 30m », « 1h »).
	Interval time.Duration `mapstructure:"interval"`
	// Parallelism est le nombre de dépôts synchronisés en même temps, entre 1 et MaxSyncParallelism
	// (défaut 4).
	Parallelism int `mapstructure:"parallelism"`
}

// ActivityConfig regroupe les seuils d'activité globaux, en jours entiers (clés activity.*).
// La base prévoit des seuils par dépôt (colonnes flemme_after_days / dead_after_days, lus par
// activity.ThresholdsFor), mais aucune commande ne les règle encore en v0.1.0.
type ActivityConfig struct {
	// FlemmeAfterDays : au-delà de ce nombre de jours sans commit, un dépôt passe en « Flemme »
	// (défaut 5, au moins 1).
	FlemmeAfterDays int `mapstructure:"flemme_after_days"`
	// DeadAfterDays : au-delà de ce nombre de jours sans commit, un dépôt passe en « Dead »
	// (défaut 15, strictement supérieur à FlemmeAfterDays).
	DeadAfterDays int `mapstructure:"dead_after_days"`
}

// BranchesConfig regroupe les réglages des branches suivies (clés branches.*).
type BranchesConfig struct {
	// Exclude liste des motifs de branches à ignorer (syntaxe de path.Match, ex. « dependabot/* »),
	// vide par défaut. En variable d'environnement : liste séparée par des virgules.
	Exclude []string `mapstructure:"exclude"`
}

// WebConfig regroupe les réglages de l'interface web (clés web.*).
type WebConfig struct {
	// Addr est l'adresse d'écoute de « watchoo serve », sous la forme hôte:port
	// (défaut « 127.0.0.1:8080 »). Hors boucle locale (127.0.0.0/8, ::1, localhost), « serve »
	// refuse de démarrer sans authentification : web.auth.username (avec le mot de passe dans la
	// variable WATCHOO_WEB_PASSWORD) ou web.trusted_proxy.
	Addr string `mapstructure:"addr"`
	// Auth règle l'authentification basique HTTP de « watchoo serve ».
	Auth WebAuthConfig `mapstructure:"auth"`
	// TrustedProxy déclare qu'un reverse proxy authentifiant protège l'interface (défaut false) :
	// « serve » accepte alors d'écouter hors boucle locale sans authentification basique, avec un
	// avertissement au démarrage.
	TrustedProxy bool `mapstructure:"trusted_proxy"`
	// AllowedHosts liste les noms (ou adresses IP), sans port, acceptés dans l'en-tête Host en plus
	// de localhost, 127.0.0.1, [::1] et de l'hôte écouté (vide par défaut) : les noms publics qu'un
	// reverse proxy transmet. Le contrôle de l'en-tête Host protège du DNS rebinding ; il reste
	// actif avec trusted_proxy, et ce réglage est alors obligatoire hors boucle locale sans
	// authentification basique. En variable d'environnement : liste séparée par des virgules.
	AllowedHosts []string `mapstructure:"allowed_hosts"`
}

// WebAuthConfig regroupe l'authentification basique de l'interface web (clés web.auth.*). Le mot de
// passe n'est JAMAIS dans la configuration : « watchoo serve » le lit dans la variable
// d'environnement WebPasswordEnv, et ne le journalise jamais.
type WebAuthConfig struct {
	// Username est l'identifiant de l'authentification basique ; vide = pas d'authentification
	// basique. Sans « : » ni caractère de contrôle.
	Username string `mapstructure:"username"`
}

// WebPasswordEnv est la variable d'environnement qui porte le mot de passe de l'authentification
// basique de « watchoo serve » (jamais un réglage de fichier).
const WebPasswordEnv = "WATCHOO_WEB_PASSWORD"

// GitHubConfig regroupe les réglages d'accès à GitHub via la CLI gh (clés github.*).
// Aucun token n'est jamais lu ni stocké dans la configuration.
type GitHubConfig struct {
	// User est le compte gh à utiliser ; vide = compte actif de gh pour github.com.
	User string `mapstructure:"user"`
	// GHPath est le chemin (ou le nom, cherché dans le PATH) de l'exécutable gh (défaut « gh »).
	// Les tests le font pointer vers un faux gh.
	GHPath string `mapstructure:"gh_path"`
}

// GitConfig regroupe les réglages des dépôts Git locaux (clés git.*, V2), utilisés quand gh n'est pas
// installé ou pas connecté. Propres à la machine : jamais partagés (watchoo config export).
type GitConfig struct {
	// Protocol est le protocole des clones : GitProtocolSSH (défaut, clé SSH de l'utilisateur) ou
	// GitProtocolHTTPS (gestionnaire d'identifiants de Git). Aucun identifiant n'est jamais lu ni
	// stocké par Watchoo.
	Protocol string `mapstructure:"protocol"`
}

// DefaultConfig renvoie la configuration par défaut d'un espace nommé name :
//
//	name: <name>
//	sync: {auto: true, interval: 30m, parallelism: 4}
//	activity: {flemme_after_days: 5, dead_after_days: 15}
//	branches: {exclude: []}       (slice vide non nil)
//	web: {addr: "127.0.0.1:8080", auth: {username: ""}, trusted_proxy: false, allowed_hosts: []}
//	timezone: ""
//	github: {user: "", gh_path: "gh"}
//	git: {protocol: "ssh"}
func DefaultConfig(name string) Config {
	return Config{
		Name: name,
		Sync: SyncConfig{
			Auto:        true,
			Interval:    DefaultSyncInterval,
			Parallelism: DefaultSyncParallelism,
		},
		Activity: ActivityConfig{
			FlemmeAfterDays: DefaultFlemmeAfterDays,
			DeadAfterDays:   DefaultDeadAfterDays,
		},
		Branches: BranchesConfig{Exclude: []string{}},
		Web:      WebConfig{Addr: DefaultWebAddr, Auth: WebAuthConfig{Username: ""}, TrustedProxy: false, AllowedHosts: []string{}},
		Timezone: "",
		GitHub:   GitHubConfig{User: "", GHPath: DefaultGHPath},
		Git:      GitConfig{Protocol: GitProtocolSSH},
	}
}

// Validate vérifie la cohérence des réglages et renvoie nil s'ils sont valides. Sinon elle renvoie
// une erreur qui regroupe (errors.Join) un message en français par problème, chaque message citant la
// clé concernée et la valeur reçue. Règles :
//   - name non vide (après suppression des espaces) ;
//   - sync.interval nul ou supérieur ou égal à MinSyncInterval ; négatif refusé ;
//     ex. « sync.interval : 10m0s est inférieur au minimum de 15 min (0 pour la synchro manuelle seule) » ;
//   - sync.parallelism entre 1 et MaxSyncParallelism ;
//   - activity.flemme_after_days >= 1 et activity.dead_after_days > activity.flemme_after_days ;
//   - branches.exclude : chaque motif non vide et valide pour path.Match ;
//   - web.addr de la forme hôte:port (net.SplitHostPort), port numérique entre 0 et 65535 ;
//   - web.auth.username sans « : » (séparateur de l'authentification basique) ni caractère de
//     contrôle, et sans espace autour ;
//   - timezone vide ou accepté par time.LoadLocation ;
//   - github.gh_path non vide, et soit absolu, soit un simple nom sans séparateur (cherché dans le
//     PATH) : un chemin relatif comme « .watchoo/gh » dépendrait du répertoire courant ;
//   - git.protocol : GitProtocolSSH ou GitProtocolHTTPS.
func (c Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
	}

	if strings.TrimSpace(c.Name) == "" {
		add("name : le nom de l'espace est vide (reçu %q)", c.Name)
	}

	minutes := int(MinSyncInterval / time.Minute)
	switch d := c.Sync.Interval; {
	case d < 0:
		add("sync.interval : %s est négatif (0 pour la synchro manuelle seule, sinon au moins %d min)",
			d, minutes)
	case d > 0 && d < MinSyncInterval:
		add("sync.interval : %s est inférieur au minimum de %d min (0 pour la synchro manuelle seule)",
			d, minutes)
	}

	if p := c.Sync.Parallelism; p < 1 || p > MaxSyncParallelism {
		add("sync.parallelism : %d doit être compris entre 1 et %d", p, MaxSyncParallelism)
	}

	flemme, dead := c.Activity.FlemmeAfterDays, c.Activity.DeadAfterDays
	if flemme < 1 {
		add("activity.flemme_after_days : %d doit valoir au moins 1 (jours sans commit)", flemme)
	}
	if dead <= flemme {
		add("activity.dead_after_days : %d doit être strictement supérieur à "+
			"activity.flemme_after_days (%d)", dead, flemme)
	}

	for i, pattern := range c.Branches.Exclude {
		if strings.TrimSpace(pattern) == "" {
			add("branches.exclude : le motif n° %d est vide", i+1)
			continue
		}
		if _, err := path.Match(pattern, ""); err != nil {
			add("branches.exclude : motif %q invalide (syntaxe de path.Match, ex. « dependabot/* »)", pattern)
		}
	}

	if err := validateAddr(c.Web.Addr); err != nil {
		errs = append(errs, err)
	}
	if u := c.Web.Auth.Username; strings.ContainsRune(u, ':') ||
		strings.ContainsFunc(u, func(r rune) bool { return r < 0x20 || r == 0x7f }) || strings.TrimSpace(u) != u {
		add("web.auth.username : %q invalide (ni « : », ni caractère de contrôle, ni espace autour)", u)
	}
	for i, h := range c.Web.AllowedHosts {
		if !validHostName(h) {
			add("web.allowed_hosts : le nom n° %d (%q) est invalide (nom d'hôte ou adresse IP, sans port, "+
				"ni schéma, ni joker ; ex. « watchoo.example.org »)", i+1, h)
		}
	}

	if c.Timezone != "" {
		if _, err := time.LoadLocation(c.Timezone); err != nil {
			add("timezone : fuseau %q inconnu (nom IANA attendu, ex. « Europe/Paris » ; "+
				"vide pour le fuseau du système)", c.Timezone)
		}
	}

	if p := c.GitHub.GHPath; strings.TrimSpace(p) == "" {
		add("github.gh_path : le chemin de gh est vide (reçu %q ; « gh » pour le chercher dans le PATH)", p)
	} else if seps := `/` + string(filepath.Separator); !filepath.IsAbs(p) &&
		!strings.ContainsAny(p[:1], seps) && strings.ContainsAny(p, seps) {
		// Relatif avec séparateur (« .watchoo/gh ») : refusé. Un chemin qui part de la racine (« \gh »
		// sous Windows, sans lecteur) ne dépend pas du répertoire courant et reste accepté.
		add("github.gh_path : chemin relatif %q refusé, il dépendrait du répertoire courant "+
			"(chemin absolu, ou nom cherché dans le PATH comme « gh »)", p)
	}

	if p := c.Git.Protocol; p != GitProtocolSSH && p != GitProtocolHTTPS {
		add("git.protocol : %q inconnu (%q ou %q)", p, GitProtocolSSH, GitProtocolHTTPS)
	}

	return errors.Join(errs...)
}

// validHostName indique si h est un nom d'hôte (lettres, chiffres, « - » et « . », 253 caractères au
// plus) ou une adresse IP (IPv6 éventuellement entre crochets), sans port.
func validHostName(h string) bool {
	if ip := strings.TrimSuffix(strings.TrimPrefix(h, "["), "]"); strings.Contains(ip, ":") {
		_, err := netip.ParseAddr(ip)
		return err == nil
	}
	if h == "" || len(h) > 253 || strings.HasPrefix(h, ".") || strings.HasPrefix(h, "-") {
		return false
	}
	return !strings.ContainsFunc(h, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.')
	})
}

// validateAddr vérifie web.addr : hôte:port, l'hôte pouvant être vide (« :8080 », toutes les
// interfaces) et le port étant un nombre entre 0 et 65535 (0 = port choisi par le système).
func validateAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("web.addr : %q n'est pas de la forme hôte:port (ex. « 127.0.0.1:8080 »)", addr)
	}
	if port == "" {
		return fmt.Errorf("web.addr : port manquant dans %q (ex. « 127.0.0.1:8080 »)", addr)
	}
	if n, err := strconv.Atoi(port); err != nil || n < 0 || n > 65535 {
		return fmt.Errorf("web.addr : port %q invalide dans %q (nombre entre 0 et 65535 attendu)", port, addr)
	}
	return nil
}

// Location renvoie le fuseau d'affichage : time.Local si Timezone est vide, sinon le fuseau IANA
// correspondant (time.LoadLocation), ou une erreur enveloppée qui cite la valeur de timezone.
func (c Config) Location() (*time.Location, error) {
	if c.Timezone == "" {
		return time.Local, nil
	}
	loc, err := time.LoadLocation(c.Timezone)
	if err != nil {
		return nil, fmt.Errorf("timezone : fuseau %q inconnu : %w", c.Timezone, err)
	}
	return loc, nil
}

// UserConfigPath renvoie le chemin du fichier de réglages de l'utilisateur, qu'il existe ou non :
// $XDG_CONFIG_HOME/watchoo/config.yaml si XDG_CONFIG_HOME est défini et non vide, sinon
// ~/.config/watchoo/config.yaml (os.UserHomeDir). Ne pas utiliser os.UserConfigDir, qui renvoie
// ~/Library/Application Support sur macOS.
//
// Comme le prévoit la spécification XDG, une valeur relative de XDG_CONFIG_HOME est ignorée.
func UserConfigPath() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" && filepath.IsAbs(xdg) {
		return filepath.Join(xdg, UserConfigDirName, ConfigFileName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("réglages de l'utilisateur : répertoire personnel introuvable : %w", err)
	}
	return filepath.Join(home, ".config", UserConfigDirName, ConfigFileName), nil
}

// Load charge les réglages de ws et les place dans ws.Config, puis les valide (Config.Validate).
// ws.Root et ws.Dir doivent être renseignés. En cas d'erreur, ws.Config n'est pas modifié.
//
// Sous Unix, Load refuse d'abord (erreur enveloppant ErrUntrusted) un espace qui n'appartient pas à
// l'utilisateur courant ou dont .watchoo/ dépasse 0700 ou config.yaml 0600 : un .watchoo/ commité dans
// un dépôt cloné ne doit jamais fournir ses réglages (github.gh_path, web.*…).
//
// Les valeurs par défaut puis les trois niveaux de réglages, du plus faible au plus fort, sont chargés
// avec une instance viper locale (viper.New(), jamais l'instance globale) :
//  1. DefaultConfig(filepath.Base(ws.Root)), déclarées une par une avec SetDefault (indispensable
//     pour que viper associe chaque clé à sa variable d'environnement) ;
//  2. le fichier utilisateur UserConfigPath(), facultatif (absent, ou répertoire personnel inconnu =
//     ignoré) ;
//  3. le fichier de l'espace ws.ConfigPath(), facultatif lui aussi (absent = valeurs précédentes) ;
//     Load fonctionne donc aussi avant Init, pour lire les réglages utilisateur et l'environnement
//     (LoadUserConfig fait de même sans jamais lire le fichier de l'espace) ;
//  4. les variables d'environnement WATCHOO_* (SetEnvPrefix(EnvPrefix), remplacement de « . » par « _ »,
//     AutomaticEnv). Une variable vide est ignorée. WATCHOO_DIR n'est pas un réglage et n'est pas
//     concerné.
//
// Dans un fichier, une clé sans valeur (ex. « timezone: » seul) est ignorée : elle ne remplace pas la
// valeur du niveau précédent. Un fichier présent mais illisible ou au YAML invalide, une clé inconnue
// (décodage strict, pour signaler les fautes de frappe), une durée mal écrite ou une valeur invalide
// renvoient une erreur en français qui cite le fichier (ou la variable) et la clé concernés. Les
// options de ligne de commande propres à une commande (ex. serve --interval) sont appliquées ensuite
// par la CLI, qui revalide la Config.
func Load(ws *Workspace) error {
	if ws == nil || ws.Root == "" || ws.Dir == "" {
		return errors.New("chargement des réglages : espace non renseigné")
	}
	if err := checkTrusted(ws); err != nil {
		return err
	}
	cfg, err := loadConfig(ws.Root, ws.ConfigPath())
	if err != nil {
		return err
	}
	ws.Config = cfg
	return nil
}

// LoadUserConfig renvoie les réglages valables pour un espace qui serait créé dans root, sans lire
// root/.watchoo/config.yaml : valeurs par défaut (nom = filepath.Base(root)), fichier utilisateur
// facultatif, puis variables WATCHOO_*, fusionnés et validés comme par Load.
//
// « watchoo init » s'en sert pour connaître github.gh_path et github.user avant de vérifier gh : un
// .watchoo/ déjà présent n'est jamais lu (Init le refusera ensuite), même si son config.yaml est
// invalide.
func LoadUserConfig(root string) (Config, error) {
	if root == "" {
		return Config{}, errors.New("chargement des réglages : répertoire non renseigné")
	}
	return loadConfig(root, "")
}

// loadConfig fait le travail de Load et de LoadUserConfig : valeurs par défaut pour un espace dont la
// racine est root, fichier utilisateur, puis workspaceFile s'il n'est pas vide, puis environnement.
func loadConfig(root, workspaceFile string) (Config, error) {
	cfg, err := loadLayers(root, workspaceFile, true)
	if err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("réglages invalides : %w", err)
	}
	return cfg, nil
}

// loadLayers fusionne, sans valider, les valeurs par défaut d'un espace de racine root, le fichier
// utilisateur, workspaceFile s'il n'est pas vide, puis, si withEnv, les variables WATCHOO_*.
func loadLayers(root, workspaceFile string, withEnv bool) (Config, error) {
	defaults := DefaultConfig(filepath.Base(root))
	v := viper.New()
	v.SetConfigType("yaml")
	for _, f := range settingFields(&defaults) {
		v.SetDefault(f.key, reflect.ValueOf(f.ptr).Elem().Interface())
	}

	var files []string
	if userPath, err := UserConfigPath(); err == nil {
		files = append(files, userPath)
	} else {
		slog.Debug("réglages de l'utilisateur ignorés", "erreur", err)
	}
	if workspaceFile != "" {
		files = append(files, workspaceFile)
	}

	var errs []error
	for _, file := range files {
		values, err := readSettingsFile(file)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if err := v.MergeConfigMap(values); err != nil {
			errs = append(errs, fmt.Errorf("%s : fusion des réglages : %w", file, err))
		}
	}
	if withEnv {
		errs = append(errs, checkEnv()...)
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}

	if withEnv {
		v.SetEnvPrefix(EnvPrefix)
		v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
		v.AutomaticEnv()
	}

	var cfg Config
	for _, f := range settingFields(&cfg) {
		if err := decodeSetting(f.ptr, v.Get(f.key)); err != nil {
			errs = append(errs, fmt.Errorf("%s : %w", f.key, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, fmt.Errorf("réglages de l'espace %s : %w", root, err)
	}
	return cfg, nil
}

// readSettingsFile lit un fichier de réglages YAML et renvoie ses valeurs sous forme de carte
// imbriquée, prête pour viper.MergeConfigMap. Un fichier absent renvoie (nil, nil). Chaque clé est
// vérifiée (clé connue, type de valeur décodable) ; les clés sans valeur sont omises. Toutes les
// erreurs du fichier sont regroupées et citent son chemin.
func readSettingsFile(file string) (map[string]any, error) {
	data, err := os.ReadFile(file)
	if isNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture des réglages %s : %w", file, err)
	}

	fv := viper.New()
	fv.SetConfigType("yaml")
	if err := fv.ReadConfig(bytes.NewReader(data)); err != nil {
		return nil, fmt.Errorf("%s : YAML invalide : %w", file, err)
	}

	var scratch Config
	fields := settingFields(&scratch)
	known := make(map[string]any, len(fields))
	leaves := make([]string, 0, len(fields))
	for _, f := range fields {
		known[f.key] = f.ptr
		leaves = append(leaves, f.key)
	}

	keys := fv.AllKeys()
	slices.Sort(keys)
	values := map[string]any{}
	var errs []error
	for _, key := range keys {
		raw := fv.Get(key)
		ptr, ok := known[key]
		if !ok {
			if err := unknownKeyError(key, raw, leaves); err != nil {
				errs = append(errs, fmt.Errorf("%s : %w", file, err))
			}
			continue
		}
		if raw == nil {
			continue
		}
		if err := decodeSetting(ptr, raw); err != nil {
			errs = append(errs, fmt.Errorf("%s : %s : %w", file, key, err))
			continue
		}
		setNested(values, key, raw)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return values, nil
}

// unknownKeyError explique pourquoi key (absente de leaves, la liste des clés connues) est refusée, ou
// renvoie nil s'il s'agit d'une section connue laissée vide (ex. « sync: » sans sous-clé).
func unknownKeyError(key string, raw any, leaves []string) error {
	var sectionLeaf string
	for _, leaf := range leaves {
		if strings.HasPrefix(leaf, key+".") {
			sectionLeaf = leaf
			break
		}
	}
	if sectionLeaf != "" {
		if m, isMap := raw.(map[string]any); raw == nil || (isMap && len(m) == 0) {
			return nil
		}
		return fmt.Errorf("%q est une section : indiquez ses sous-clés (ex. « %s »)", key, sectionLeaf)
	}
	for _, leaf := range leaves {
		if strings.HasPrefix(key, leaf+".") {
			return fmt.Errorf("%q n'accepte pas de sous-clés (clé %q refusée)", leaf, key)
		}
	}
	if suggestion := suggestKey(key, leaves); suggestion != "" {
		return fmt.Errorf("clé inconnue %q (vouliez-vous dire « %s » ?)", key, suggestion)
	}
	return fmt.Errorf("clé inconnue %q", key)
}

// suggestKey propose la clé connue la plus proche de key (faute de frappe, ou clé écrite hors de sa
// section), ou "" si aucune n'est assez proche.
func suggestKey(key string, leaves []string) string {
	best, bestDist := "", 3 // au plus 2 modifications
	for _, leaf := range leaves {
		if d := editDistance(key, leaf); d < bestDist {
			best, bestDist = leaf, d
		}
	}
	if best != "" {
		return best
	}
	last := key[strings.LastIndex(key, ".")+1:]
	for _, leaf := range leaves {
		if strings.HasSuffix(leaf, "."+last) {
			return leaf
		}
	}
	return ""
}

// editDistance calcule la distance de Levenshtein entre a et b (en octets : les clés sont en ASCII).
func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// setNested place value dans m sous la clé pointée key (« sync.interval » → m["sync"]["interval"]).
func setNested(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	for _, part := range parts[:len(parts)-1] {
		sub, ok := m[part].(map[string]any)
		if !ok {
			sub = map[string]any{}
			m[part] = sub
		}
		m = sub
	}
	m[parts[len(parts)-1]] = value
}

// checkEnv vérifie chaque variable WATCHOO_* correspondant à un réglage et renvoie une erreur par
// variable dont la valeur ne peut pas être décodée. Les variables vides sont ignorées, comme dans viper.
func checkEnv() []error {
	var scratch Config
	var errs []error
	for _, f := range settingFields(&scratch) {
		name := envVarName(f.key)
		value := os.Getenv(name)
		if value == "" {
			continue
		}
		if err := decodeSetting(f.ptr, value); err != nil {
			errs = append(errs, fmt.Errorf("variable %s (%s) : %w", name, f.key, err))
		}
	}
	return errs
}

// envVarName renvoie le nom de la variable d'environnement d'une clé : « sync.interval » →
// « WATCHOO_SYNC_INTERVAL ».
func envVarName(key string) string {
	return EnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
}

// settingField associe une clé complète (« sync.interval ») au pointeur vers le champ de Config
// correspondant (*string, *bool, *int, *time.Duration ou *[]string).
type settingField struct {
	key string
	ptr any
}

// settingFields renvoie les réglages de c, dans l'ordre des champs, d'après les balises mapstructure.
func settingFields(c *Config) []settingField {
	var out []settingField
	collectFields(reflect.ValueOf(c).Elem(), "", &out)
	return out
}

func collectFields(v reflect.Value, prefix string, out *[]settingField) {
	t := v.Type()
	for i := range t.NumField() {
		tag := t.Field(i).Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}
		key := tag
		if prefix != "" {
			key = prefix + "." + tag
		}
		field := v.Field(i)
		if field.Kind() == reflect.Struct {
			collectFields(field, key, out)
			continue
		}
		*out = append(*out, settingField{key: key, ptr: field.Addr().Interface()})
	}
}

// decodeSetting convertit raw (valeur venue d'un fichier YAML, d'une variable d'environnement ou des
// valeurs par défaut) vers le type du champ pointé par ptr, et l'y range. Les messages d'erreur sont en
// français et ne citent pas la clé (l'appelant l'ajoute).
func decodeSetting(ptr, raw any) error {
	switch p := ptr.(type) {
	case *string:
		s, err := toString(raw)
		if err != nil {
			return err
		}
		*p = s
	case *bool:
		b, err := toBool(raw)
		if err != nil {
			return err
		}
		*p = b
	case *int:
		n, err := toInt(raw)
		if err != nil {
			return err
		}
		*p = n
	case *time.Duration:
		d, err := toDuration(raw)
		if err != nil {
			return err
		}
		*p = d
	case *[]string:
		list, err := toStringSlice(raw)
		if err != nil {
			return err
		}
		*p = list
	default:
		return fmt.Errorf("type de réglage non géré : %T", ptr)
	}
	return nil
}

// describe décrit une valeur reçue, pour les messages d'erreur.
func describe(raw any) string {
	switch v := raw.(type) {
	case nil:
		return "une valeur vide"
	case string:
		return fmt.Sprintf("%q", v)
	case bool:
		return fmt.Sprintf("le booléen %t", v)
	case int, int64, uint64, float64:
		return fmt.Sprintf("le nombre %v", v)
	case []any, []string:
		return "une liste"
	case map[string]any:
		return "une section"
	default:
		return fmt.Sprintf("%v", v)
	}
}

func toString(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	}
	return "", fmt.Errorf("chaîne de caractères attendue, reçu %s (mettez la valeur entre guillemets)",
		describe(raw))
}

func toBool(raw any) (bool, error) {
	switch v := raw.(type) {
	case bool:
		return v, nil
	case string:
		if b, err := strconv.ParseBool(strings.TrimSpace(v)); err == nil {
			return b, nil
		}
	}
	return false, fmt.Errorf("true ou false attendu, reçu %s", describe(raw))
}

func toInt(raw any) (int, error) {
	switch v := raw.(type) {
	case int:
		return v, nil
	case int64:
		if n := int(v); int64(n) == v {
			return n, nil
		}
	case uint64:
		if n := int(v); n >= 0 && uint64(n) == v {
			return n, nil
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("nombre entier attendu, reçu %s", describe(raw))
}

func toDuration(raw any) (time.Duration, error) {
	switch v := raw.(type) {
	case time.Duration:
		return v, nil
	case int, int64, uint64:
		// Seul 0 a un sens sans unité (« interval: 0 » = synchro manuelle seule).
		if n, err := toInt(v); err == nil && n == 0 {
			return 0, nil
		}
		return 0, fmt.Errorf("durée sans unité %v (exemples : 30m, 1h ; 0 accepté)", v)
	case string:
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err == nil {
			return d, nil
		}
		if _, convErr := strconv.Atoi(strings.TrimSpace(v)); convErr == nil {
			return 0, fmt.Errorf("durée sans unité %q (exemples : 30m, 1h ; 0 accepté)", v)
		}
		return 0, fmt.Errorf("durée invalide %q (exemples : 30m, 1h ; 0 accepté)", v)
	}
	return 0, fmt.Errorf("durée attendue (exemples : 30m, 1h), reçu %s", describe(raw))
}

// toStringSlice accepte une liste YAML de chaînes, ou une chaîne de valeurs séparées par des virgules
// (variables d'environnement : « dependabot/*, renovate/* »), chaque valeur étant débarrassée des
// espaces qui l'entourent. Une chaîne vide donne une liste vide.
func toStringSlice(raw any) ([]string, error) {
	switch v := raw.(type) {
	case []string:
		return slices.Clone(v), nil
	case []any:
		out := make([]string, 0, len(v))
		for i, item := range v {
			s, err := toString(item)
			if err != nil {
				return nil, fmt.Errorf("élément n° %d : %w", i+1, err)
			}
			out = append(out, s)
		}
		return out, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return []string{}, nil
		}
		parts := strings.Split(v, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts, nil
	}
	return nil, fmt.Errorf("liste attendue (ex. [\"dependabot/*\"]), reçu %s", describe(raw))
}

// configHeaderSubtitle est le sous-titre de la carte d'en-tête des fichiers de configuration.
const configHeaderSubtitle = "Réglages de l'espace de supervision"

// configHeader renvoie la carte d'en-tête des fichiers de configuration, en commentaires YAML : la carte
// de logo.Card (hibou mini en blocs Unicode, titre « Watchoo » sans version, sous-titre
// configHeaderSubtitle, sans astuce ni couleur, 80 colonnes au plus), chaque ligne précédée de « # ».
// Le fichier est déjà en UTF-8 (accents, « »). Le nom de l'espace n'y figure pas : il pourrait contenir
// un saut de ligne, qui ferait sortir la suite du commentaire.
func configHeader() string {
	card := logo.Card(logo.CardOptions{
		Title:    "Watchoo",
		Subtitle: configHeaderSubtitle,
		Width:    logo.MaxCardWidth,
		Unicode:  true,
	})
	var b strings.Builder
	for line := range strings.Lines(card) {
		b.WriteString("# ")
		b.WriteString(line)
	}
	return b.String()
}

// ConfigTemplate renvoie le contenu du config.yaml écrit par Init pour un espace nommé name.
//
// Le fichier commence par la carte d'en-tête de Watchoo, en commentaires (voir configHeader). Il est
// commenté en français. Seule la clé name y est active (valeur écrite comme une chaîne
// YAML entre guillemets, correctement échappée, pour supporter « : », « # », guillemets…). Tous les
// autres réglages y figurent en commentaire, avec leur valeur par défaut et une explication, afin que
// les réglages de l'utilisateur ($XDG_CONFIG_HOME/watchoo/config.yaml, sinon ~/.config/watchoo/config.yaml)
// continuent de s'appliquer.
// Propriété attendue : ce contenu, relu par Load sans autre source, donne exactement DefaultConfig(name).
// Décommenter toutes les lignes de réglage donne aussi DefaultConfig(name) (un test le vérifie).
//
// config.example.yaml, à la racine du dépôt, est égal à ConfigTemplate("licence-projet") (un test le
// vérifie ; « go test ./internal/workspace -run TestConfigExampleFile -update » le régénère).
func ConfigTemplate(name string) string {
	return renderConfig(name, map[string]any{"name": name})
}

// renderConfig écrit le config.yaml du modèle de Init pour un espace dont le nom par défaut est
// defaultName, avec les réglages de active (clé complète → valeur typée : string, bool, int,
// time.Duration ou []string) écrits ACTIFS, chacun à sa place dans le modèle ; les autres restent en
// commentaire avec leur valeur par défaut. Une section (« sync: ») est décommentée dès qu'une de ses
// clés est active. Les valeurs passent par FormatSettingValue (chaînes entre guillemets, échappées) :
// aucune valeur ne peut créer une autre clé. Une clé de active inconnue du modèle est ignorée ici :
// WriteSharedSettings relit le fichier écrit et le compare aux valeurs attendues.
//
// Sans « name » dans active, la ligne du nom est écrite en commentaire, avec defaultName.
func renderConfig(defaultName string, active map[string]any) string {
	defaults := DefaultConfig(defaultName)
	defaultValues := map[string]any{}
	for _, f := range settingFields(&defaults) {
		defaultValues[f.key] = reflect.ValueOf(f.ptr).Elem().Interface()
	}
	sectionActive := func(prefix string) bool {
		for key := range active {
			if strings.HasPrefix(key, prefix+".") {
				return true
			}
		}
		return false
	}

	var b strings.Builder
	text := func(lines ...string) {
		for _, line := range lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	// section écrit l'en-tête d'une section (« sync: »), actif si l'une de ses clés l'est.
	section := func(indent, key string) {
		leaf := key[strings.LastIndex(key, ".")+1:]
		if sectionActive(key) {
			text(indent + leaf + ":")
			return
		}
		text("# " + strings.TrimPrefix(indent, "  ") + leaf + ":")
	}
	// setting écrit une ligne de réglage (en commentaire, ou active si la clé est dans active), avec son
	// explication alignée à droite.
	setting := func(indent, key, comment string) {
		leaf := key[strings.LastIndex(key, ".")+1:]
		if v, ok := active[key]; ok {
			fmt.Fprintf(&b, "%-28s # %s\n", indent+leaf+": "+FormatSettingValue(v), comment)
			return
		}
		fmt.Fprintf(&b, "%-28s # %s\n", "# "+indent+leaf+": "+FormatSettingValue(defaultValues[key]), comment)
	}

	b.WriteString(configHeader())
	text(
		"#",
		"# Réglages de l'espace de supervision Watchoo.",
		"#",
	)
	if onlyName := len(active) == 0 || (len(active) == 1 && active["name"] != nil); onlyName {
		text("# Seul le nom de l'espace est actif. Les autres réglages sont en commentaire,")
	} else {
		text("# Les réglages sans « # » s'appliquent à cet espace. Les autres sont en commentaire,")
	}
	text(
		"# avec leur valeur par défaut : pour en changer un dans cet espace seulement,",
		"# retirez le « # » devant sa section (ex. « sync: ») et devant sa ligne.",
		"#",
		"# Ordre de priorité, du plus faible au plus fort :",
		"#   1. valeurs par défaut de Watchoo ;",
		"#   2. réglages de l'utilisateur, communs à tous ses espaces (facultatif) :",
		"#      $XDG_CONFIG_HOME/watchoo/config.yaml, sinon ~/.config/watchoo/config.yaml ;",
		"#   3. ce fichier ;",
		"#   4. variables d'environnement WATCHOO_<SECTION>_<CLÉ> (ex. WATCHOO_SYNC_INTERVAL=1h),",
		"#      puis options de la ligne de commande.",
		"#",
		"# Une clé inconnue est refusée, pour signaler les fautes de frappe.",
		"# Aucun token ici : Watchoo l'obtient de « gh auth token » et ne le stocke jamais.",
		"",
		"# Nom de l'espace, affiché dans le web, le TUI et « watchoo status ».",
		"# Par défaut : le nom du répertoire de l'espace.",
	)
	if v, ok := active["name"]; ok {
		text("name: " + FormatSettingValue(v))
	} else {
		text("# name: " + FormatSettingValue(defaultName))
	}
	text(
		"",
		"# Synchronisation automatique, faite par « watchoo serve » et « watchoo tui ».",
		"# « watchoo sync » reste possible à tout moment.",
	)
	section("", "sync")
	setting("  ", "sync.auto", "false : synchro manuelle seule")
	setting("  ", "sync.interval",
		fmt.Sprintf("durée (15m, 1h…), au moins %s ; 0 : synchro manuelle seule", shortDuration(MinSyncInterval)))
	setting("  ", "sync.parallelism", fmt.Sprintf("dépôts synchronisés en même temps, de 1 à %d", MaxSyncParallelism))
	text(
		"",
		"# Seuils d'activité de l'espace, en jours sans commit.",
	)
	section("", "activity")
	setting("  ", "activity.flemme_after_days", "au-delà : « Flemme » (au moins 1)")
	setting("  ", "activity.dead_after_days", "au-delà : « Dead » (plus grand que flemme_after_days)")
	text(
		"",
		"# Branches ignorées, par motifs (syntaxe de path.Match).",
	)
	section("", "branches")
	setting("  ", "branches.exclude", `ex. ["dependabot/*", "renovate/*"]`)
	text(
		"",
		"# Interface web (« watchoo serve »). Écouter ailleurs que sur 127.0.0.1",
		"# exposerait les données des dépôts privés : l'authentification devient obligatoire",
		"# (auth.username et la variable "+WebPasswordEnv+", jamais dans ce fichier,",
		"# ou trusted_proxy: true derrière un reverse proxy qui authentifie, avec ses noms",
		"# publics dans allowed_hosts : l'en-tête Host est toujours contrôlé, contre le DNS rebinding).",
	)
	section("", "web")
	setting("  ", "web.addr", "hôte:port")
	setting("  ", "web.trusted_proxy", "true : un reverse proxy authentifiant protège l'accès")
	setting("  ", "web.allowed_hosts", `noms publics acceptés dans Host, ex. ["watchoo.example.org"]`)
	if sectionActive("web.auth") {
		text("  auth:")
	} else {
		text("#   auth:")
	}
	setting("    ", "web.auth.username", "authentification basique ; mot de passe dans "+WebPasswordEnv)
	text(
		"",
		"# Fuseau horaire d'affichage et de regroupement par jour ou par semaine.",
	)
	setting("", "timezone", `vide : fuseau du système ; sinon nom IANA, ex. "Europe/Paris"`)
	text(
		"",
		"# Accès à l'API GitHub par la CLI gh, quand elle est installée et connectée. Avec",
		"# GH_TOKEN (Docker, CI), laissez user vide : « gh auth token --user » ignore GH_TOKEN.",
	)
	section("", "github")
	setting("  ", "github.user", "login exact d'un compte gh ; vide : compte actif de gh")
	setting("  ", "github.gh_path", "chemin absolu, ou nom cherché dans le PATH")
	text(
		"",
		"# Sans gh installé et connecté, les dépôts sont suivis par clones Git locaux",
		"# (.watchoo/repos/), avec la clé SSH ou les identifiants Git de cette machine.",
	)
	section("", "git")
	setting("  ", "git.protocol", `"ssh" (git@github.com:…) ou "https" (https://github.com/…)`)
	return b.String()
}

// FormatSettingValue écrit une valeur de réglage (string, bool, int, time.Duration ou []string) comme
// une valeur YAML sur une ligne, telle que config.yaml l'écrit : chaîne entre guillemets doubles et
// échappée (strconv.Quote, dont les séquences sont aussi celles de YAML), liste en flux
// (["a", "b"], [] si vide), durée courte (30m, 1h30m), booléen et entier tels quels. Sert aussi à
// afficher les valeurs avant / après d'un import (internal/configshare).
func FormatSettingValue(v any) string {
	switch x := v.(type) {
	case string:
		return strconv.Quote(x)
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case time.Duration:
		return shortDuration(x)
	case []string:
		quoted := make([]string, len(x))
		for i, s := range x {
			quoted[i] = strconv.Quote(s)
		}
		return "[" + strings.Join(quoted, ", ") + "]"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// shortDuration écrit une durée sans les zéros inutiles : 30m plutôt que 30m0s, 1h plutôt que 1h0m0s.
func shortDuration(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = strings.TrimSuffix(s, "0s")
	}
	if strings.HasSuffix(s, "h0m") {
		s = strings.TrimSuffix(s, "0m")
	}
	return s
}
