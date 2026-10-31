// Textes du paquet internal/workspace : espaces de supervision, réglages (chargement, validation,
// modèle de config.yaml), refus d'un espace non fiable. Préfixe des clés : « workspace. ».
package i18n

// workspaceTexts regroupe les textes de internal/workspace, ajoutés au catalogue par catalogParts.
var workspaceTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : » et « ; »,
	// l'anglais n'en met pas.
	"workspace.err.sep":       {": ", " : "},
	"workspace.err.semicolon": {"; ", " ; "},

	// Erreurs sentinelles (workspace.go), testées avec errors.Is.
	"workspace.err.not_found": {
		`no Watchoo workspace here, run "watchoo init"`,
		"aucun espace Watchoo ici, lancez « watchoo init »",
	},
	"workspace.err.already_initialized": {
		"this directory is already a Watchoo workspace",
		"ce répertoire est déjà un espace Watchoo",
	},
	"workspace.err.untrusted": {
		"Watchoo workspace refused",
		"espace Watchoo refusé",
	},

	// Fichiers créés ou réécrits (workspace.go, shared.go).
	"workspace.fs.create": {"creating %s", "création de %s"},
	"workspace.fs.write":  {"writing %s", "écriture de %s"},
	"workspace.fs.perms":  {"setting permissions of %s", "droits de %s"},

	// Find (workspace.go).
	"workspace.find.from": {
		`looking for the workspace from %q`,
		"recherche de l'espace depuis %q",
	},
	"workspace.find.err": {
		"looking for the workspace",
		"recherche de l'espace",
	},
	"workspace.find.searched_from": {
		"searched from %s",
		"recherche depuis %s",
	},

	// Resolve et ResolveFrom (workspace.go).
	"workspace.resolve.cwd": {
		"current directory unavailable",
		"répertoire courant inaccessible",
	},
	"workspace.resolve.origin_flag": {
		"option -C",
		"option -C",
	},
	"workspace.resolve.origin_env": {
		"variable WATCHOO_DIR",
		"variable WATCHOO_DIR",
	},
	"workspace.err.dir_missing": {
		"directory %s does not exist",
		"le répertoire %s n'existe pas",
	},
	"workspace.err.not_a_dir": {
		"%s is not a directory",
		"%s n'est pas un répertoire",
	},

	// Init (workspace.go).
	"workspace.init.prefix": {
		"creating the workspace",
		"création de l'espace",
	},
	"workspace.init.create_q": {
		"creating the workspace %q",
		"création de l'espace %q",
	},
	"workspace.init.in_clones": {
		"creating the workspace: %s is inside the clones directory (%s/) of workspace %s, managed by Watchoo; create the workspace elsewhere",
		"création de l'espace : %s est dans le répertoire des clones (%s/) de l'espace %s, géré par Watchoo ; créez l'espace ailleurs",
	},
	"workspace.init.watchoo_not_dir": {
		"creating the workspace: %s already exists and is not a directory, nothing was modified",
		"création de l'espace : %s existe déjà et n'est pas un répertoire, il n'a pas été modifié",
	},
	"workspace.init.cleanup": {
		"cleaning up %s",
		"nettoyage de %s",
	},

	// Contrôle de fiabilité (trust_unix.go) : espace refusé, chemin cité, remède indiqué.
	"workspace.trust.stat": {
		"checking the workspace",
		"contrôle de l'espace",
	},
	"workspace.trust.other_uid": {
		"%s belongs to another user (uid %d); run the command from your own workspace or designate it with -C (or %s)",
		"%s appartient à un autre utilisateur (uid %d) ; lancez la commande depuis votre propre espace ou désignez-le avec -C (ou %s)",
	},
	"workspace.trust.perms": {
		"%s has the permissions %04o, accessible to others than you (expected %04o); if this really is your workspace: chmod %s %s; otherwise (for example a .watchoo/ that came from a cloned repository), designate your workspace with -C (or %s)",
		"%s a les droits %04o, accessibles à d'autres que vous (attendu %04o) ; s'il s'agit bien de votre espace : chmod %s %s ; sinon (par exemple un .watchoo/ venu d'un dépôt cloné), désignez votre espace avec -C (ou %s)",
	},

	// Chargement des réglages (config.go).
	"workspace.load.no_workspace": {
		"loading the settings: workspace not provided",
		"chargement des réglages : espace non renseigné",
	},
	"workspace.load.no_dir": {
		"loading the settings: directory not provided",
		"chargement des réglages : répertoire non renseigné",
	},
	"workspace.load.invalid": {
		"invalid settings",
		"réglages invalides",
	},
	"workspace.load.of": {
		"settings of workspace %s",
		"réglages de l'espace %s",
	},
	"workspace.load.merge": {
		"merging the settings",
		"fusion des réglages",
	},
	"workspace.userconfig.no_home": {
		"user settings: home directory not found",
		"réglages de l'utilisateur : répertoire personnel introuvable",
	},
	"workspace.settings.read": {
		"reading the settings %s",
		"lecture des réglages %s",
	},
	"workspace.settings.invalid_yaml": {
		"invalid YAML",
		"YAML invalide",
	},
	"workspace.settings.is_section": {
		`%q is a section: give its sub-keys (e.g. "%s")`,
		"%q est une section : indiquez ses sous-clés (ex. « %s »)",
	},
	"workspace.settings.no_subkeys": {
		`%q takes no sub-keys (key %q refused)`,
		"%q n'accepte pas de sous-clés (clé %q refusée)",
	},
	"workspace.settings.unknown_key_suggest": {
		`unknown key %q (did you mean "%s"?)`,
		"clé inconnue %q (vouliez-vous dire « %s » ?)",
	},
	"workspace.settings.unknown_key": {
		`unknown key %q`,
		"clé inconnue %q",
	},

	// Validation des réglages (Config.Validate, validateAddr) : chaque message cite la clé et la
	// valeur reçue.
	"workspace.validate.name_empty": {
		`name: the workspace name is empty (got %q)`,
		"name : le nom de l'espace est vide (reçu %q)",
	},
	"workspace.validate.interval_negative": {
		"sync.interval: %s is negative (0 for manual sync only, otherwise at least %d min)",
		"sync.interval : %s est négatif (0 pour la synchro manuelle seule, sinon au moins %d min)",
	},
	"workspace.validate.interval_too_low": {
		"sync.interval: %s is below the minimum of %d min (0 for manual sync only)",
		"sync.interval : %s est inférieur au minimum de %d min (0 pour la synchro manuelle seule)",
	},
	"workspace.validate.parallelism_range": {
		"sync.parallelism: %d must be between 1 and %d",
		"sync.parallelism : %d doit être compris entre 1 et %d",
	},
	"workspace.validate.flemme_min": {
		"activity.flemme_after_days: %d must be at least 1 (days without a commit)",
		"activity.flemme_after_days : %d doit valoir au moins 1 (jours sans commit)",
	},
	"workspace.validate.dead_above_flemme": {
		"activity.dead_after_days: %d must be strictly greater than activity.flemme_after_days (%d)",
		"activity.dead_after_days : %d doit être strictement supérieur à activity.flemme_after_days (%d)",
	},
	"workspace.validate.pattern_empty": {
		"branches.exclude: pattern #%d is empty",
		"branches.exclude : le motif n° %d est vide",
	},
	"workspace.validate.pattern_invalid": {
		`branches.exclude: invalid pattern %q (path.Match syntax, e.g. "dependabot/*")`,
		"branches.exclude : motif %q invalide (syntaxe de path.Match, ex. « dependabot/* »)",
	},
	"workspace.validate.addr_form": {
		`web.addr: %q is not of the form host:port (e.g. "127.0.0.1:8080")`,
		"web.addr : %q n'est pas de la forme hôte:port (ex. « 127.0.0.1:8080 »)",
	},
	"workspace.validate.addr_port_missing": {
		`web.addr: missing port in %q (e.g. "127.0.0.1:8080")`,
		"web.addr : port manquant dans %q (ex. « 127.0.0.1:8080 »)",
	},
	"workspace.validate.addr_port_invalid": {
		`web.addr: invalid port %q in %q (expected a number between 0 and 65535)`,
		"web.addr : port %q invalide dans %q (nombre entre 0 et 65535 attendu)",
	},
	"workspace.validate.username_invalid": {
		`web.auth.username: %q is invalid (no ":", no control character, no space around it)`,
		"web.auth.username : %q invalide (ni « : », ni caractère de contrôle, ni espace autour)",
	},
	"workspace.validate.host_invalid": {
		`web.allowed_hosts: invalid name #%d (%q) (a host name or IP address, without port, scheme or wildcard; e.g. "watchoo.example.org")`,
		"web.allowed_hosts : le nom n° %d (%q) est invalide (nom d'hôte ou adresse IP, sans port, ni schéma, ni joker ; ex. « watchoo.example.org »)",
	},
	"workspace.validate.timezone_unknown": {
		`timezone: unknown time zone %q (expected an IANA name, e.g. "Europe/Paris"; empty for the system time zone)`,
		"timezone : fuseau %q inconnu (nom IANA attendu, ex. « Europe/Paris » ; vide pour le fuseau du système)",
	},
	"workspace.validate.timezone_unknown_wrap": {
		`timezone: unknown time zone %q`,
		"timezone : fuseau %q inconnu",
	},
	"workspace.validate.gh_path_empty": {
		`github.gh_path: the gh path is empty (got %q; "gh" to look it up in the PATH)`,
		"github.gh_path : le chemin de gh est vide (reçu %q ; « gh » pour le chercher dans le PATH)",
	},
	"workspace.validate.gh_path_relative": {
		`github.gh_path: relative path %q refused, it would depend on the current directory (an absolute path, or a name looked up in the PATH like "gh")`,
		"github.gh_path : chemin relatif %q refusé, il dépendrait du répertoire courant (chemin absolu, ou nom cherché dans le PATH comme « gh »)",
	},
	"workspace.validate.protocol_unknown": {
		`git.protocol: %q unknown (%q or %q)`,
		"git.protocol : %q inconnu (%q ou %q)",
	},

	// Décodage d'une valeur de réglage (decodeSetting et aides) : le message ne cite pas la clé,
	// l'appelant l'ajoute.
	"workspace.decode.unsupported_type": {
		"unsupported setting type: %T",
		"type de réglage non géré : %T",
	},
	"workspace.decode.want_string": {
		`expected a string, got %s (put the value in quotes)`,
		"chaîne de caractères attendue, reçu %s (mettez la valeur entre guillemets)",
	},
	"workspace.decode.want_bool": {
		"expected true or false, got %s",
		"true ou false attendu, reçu %s",
	},
	"workspace.decode.want_int": {
		"expected an integer, got %s",
		"nombre entier attendu, reçu %s",
	},
	"workspace.decode.duration_no_unit_v": {
		"duration without a unit %v (examples: 30m, 1h; 0 accepted)",
		"durée sans unité %v (exemples : 30m, 1h ; 0 accepté)",
	},
	"workspace.decode.duration_no_unit_q": {
		`duration without a unit %q (examples: 30m, 1h; 0 accepted)`,
		"durée sans unité %q (exemples : 30m, 1h ; 0 accepté)",
	},
	"workspace.decode.duration_invalid": {
		`invalid duration %q (examples: 30m, 1h; 0 accepted)`,
		"durée invalide %q (exemples : 30m, 1h ; 0 accepté)",
	},
	"workspace.decode.want_duration": {
		"expected a duration (examples: 30m, 1h), got %s",
		"durée attendue (exemples : 30m, 1h), reçu %s",
	},
	"workspace.decode.list_item": {
		"item #%d",
		"élément n° %d",
	},
	"workspace.decode.want_list": {
		`expected a list (e.g. ["dependabot/*"]), got %s`,
		`liste attendue (ex. ["dependabot/*"]), reçu %s`,
	},
	"workspace.decode.env_var": {
		"variable %s (%s)",
		"variable %s (%s)",
	},

	// Description d'une valeur reçue, réutilisée dans les messages de décodage (describe).
	"workspace.desc.empty": {
		"an empty value",
		"une valeur vide",
	},
	"workspace.desc.bool": {
		"the boolean %t",
		"le booléen %t",
	},
	"workspace.desc.number": {
		"the number %v",
		"le nombre %v",
	},
	"workspace.desc.list": {
		"a list",
		"une liste",
	},
	"workspace.desc.section": {
		"a section",
		"une section",
	},

	// Carte d'en-tête et commentaires du modèle de config.yaml (configHeader, renderConfig).
	"workspace.cfg.card_subtitle": {
		"Workspace settings",
		"Réglages de l'espace de supervision",
	},
	"workspace.cfg.header": {
		"Watchoo workspace settings.",
		"Réglages de l'espace de supervision Watchoo.",
	},
	"workspace.cfg.only_name": {
		`Only the workspace name is active. The other settings are in comments,`,
		"Seul le nom de l'espace est actif. Les autres réglages sont en commentaire,",
	},
	"workspace.cfg.some_active": {
		`The settings without a "#" apply to this workspace. The others are in comments,`,
		"Les réglages sans « # » s'appliquent à cet espace. Les autres sont en commentaire,",
	},
	"workspace.cfg.change_hint": {
		"with their default value: to change one for this workspace only,",
		"avec leur valeur par défaut : pour en changer un dans cet espace seulement,",
	},
	"workspace.cfg.uncomment": {
		`remove the "#" in front of its section (e.g. "sync:") and in front of its line.`,
		"retirez le « # » devant sa section (ex. « sync: ») et devant sa ligne.",
	},
	"workspace.cfg.priority_order": {
		"Order of precedence, from lowest to highest:",
		"Ordre de priorité, du plus faible au plus fort :",
	},
	"workspace.cfg.priority_defaults": {
		"1. Watchoo's defaults;",
		"1. valeurs par défaut de Watchoo ;",
	},
	"workspace.cfg.priority_user": {
		"2. the user's settings, shared by all their workspaces (optional):",
		"2. réglages de l'utilisateur, communs à tous ses espaces (facultatif) :",
	},
	"workspace.cfg.priority_user_paths": {
		"$XDG_CONFIG_HOME/watchoo/config.yaml, or ~/.config/watchoo/config.yaml;",
		"$XDG_CONFIG_HOME/watchoo/config.yaml, sinon ~/.config/watchoo/config.yaml ;",
	},
	"workspace.cfg.priority_this_file": {
		"3. this file;",
		"3. ce fichier ;",
	},
	"workspace.cfg.priority_env": {
		"4. environment variables WATCHOO_<SECTION>_<KEY> (e.g. WATCHOO_SYNC_INTERVAL=1h),",
		"4. variables d'environnement WATCHOO_<SECTION>_<CLÉ> (ex. WATCHOO_SYNC_INTERVAL=1h),",
	},
	"workspace.cfg.priority_cli": {
		"then the command-line options.",
		"puis options de la ligne de commande.",
	},
	"workspace.cfg.unknown_key_refused": {
		"An unknown key is refused, to catch typos.",
		"Une clé inconnue est refusée, pour signaler les fautes de frappe.",
	},
	"workspace.cfg.no_token": {
		`No token here: Watchoo gets it from "gh auth token" and never stores it.`,
		"Aucun token ici : Watchoo l'obtient de « gh auth token » et ne le stocke jamais.",
	},
	"workspace.cfg.name_comment": {
		`Workspace name, shown in the web UI, the TUI and "watchoo status".`,
		"Nom de l'espace, affiché dans le web, le TUI et « watchoo status ».",
	},
	"workspace.cfg.name_default": {
		"Default: the name of the workspace directory.",
		"Par défaut : le nom du répertoire de l'espace.",
	},
	"workspace.cfg.sync_comment": {
		`Automatic synchronization, performed by "watchoo serve" and "watchoo tui".`,
		"Synchronisation automatique, faite par « watchoo serve » et « watchoo tui ».",
	},
	"workspace.cfg.sync_manual": {
		`"watchoo sync" remains available at any time.`,
		"« watchoo sync » reste possible à tout moment.",
	},
	"workspace.cfg.sync_auto": {
		"false: manual sync only",
		"false : synchro manuelle seule",
	},
	"workspace.cfg.sync_interval": {
		"duration (15m, 1h…), at least %s; 0: manual sync only",
		"durée (15m, 1h…), au moins %s ; 0 : synchro manuelle seule",
	},
	"workspace.cfg.sync_parallelism": {
		"repositories synced at the same time, from 1 to %d",
		"dépôts synchronisés en même temps, de 1 à %d",
	},
	"workspace.cfg.activity_comment": {
		"Activity thresholds of the workspace, in days without a commit.",
		"Seuils d'activité de l'espace, en jours sans commit.",
	},
	"workspace.cfg.flemme": {
		`beyond: "Flemme" (at least 1)`,
		"au-delà : « Flemme » (au moins 1)",
	},
	"workspace.cfg.dead": {
		`beyond: "Dead" (greater than flemme_after_days)`,
		"au-delà : « Dead » (plus grand que flemme_after_days)",
	},
	"workspace.cfg.branches_comment": {
		"Ignored branches, by pattern (path.Match syntax).",
		"Branches ignorées, par motifs (syntaxe de path.Match).",
	},
	"workspace.cfg.branches_exclude": {
		`e.g. ["dependabot/*", "renovate/*"]`,
		`ex. ["dependabot/*", "renovate/*"]`,
	},
	"workspace.cfg.web_l1": {
		`Web interface ("watchoo serve"). Listening anywhere other than 127.0.0.1`,
		"Interface web (« watchoo serve »). Écouter ailleurs que sur 127.0.0.1",
	},
	"workspace.cfg.web_l2": {
		"would expose private repository data: authentication becomes mandatory",
		"exposerait les données des dépôts privés : l'authentification devient obligatoire",
	},
	"workspace.cfg.web_l3": {
		"(auth.username and the %s variable, never in this file,",
		"(auth.username et la variable %s, jamais dans ce fichier,",
	},
	"workspace.cfg.web_l4": {
		"or trusted_proxy: true behind an authenticating reverse proxy, with its public",
		"ou trusted_proxy: true derrière un reverse proxy qui authentifie, avec ses noms",
	},
	"workspace.cfg.web_l5": {
		"names in allowed_hosts: the Host header is always checked, against DNS rebinding).",
		"publics dans allowed_hosts : l'en-tête Host est toujours contrôlé, contre le DNS rebinding).",
	},
	"workspace.cfg.web_addr": {
		"host:port",
		"hôte:port",
	},
	"workspace.cfg.trusted_proxy": {
		"true: an authenticating reverse proxy protects access",
		"true : un reverse proxy authentifiant protège l'accès",
	},
	"workspace.cfg.allowed_hosts": {
		`public names accepted in Host, e.g. ["watchoo.example.org"]`,
		`noms publics acceptés dans Host, ex. ["watchoo.example.org"]`,
	},
	"workspace.cfg.auth_username": {
		"basic authentication; password in %s",
		"authentification basique ; mot de passe dans %s",
	},
	"workspace.cfg.timezone_comment": {
		"Time zone used for display and grouping by day or by week.",
		"Fuseau horaire d'affichage et de regroupement par jour ou par semaine.",
	},
	"workspace.cfg.timezone": {
		`empty: system time zone; otherwise an IANA name, e.g. "Europe/Paris"`,
		`vide : fuseau du système ; sinon nom IANA, ex. "Europe/Paris"`,
	},
	"workspace.cfg.github_l1": {
		"Access to the GitHub API through the gh CLI, when it is installed and signed in. With",
		"Accès à l'API GitHub par la CLI gh, quand elle est installée et connectée. Avec",
	},
	"workspace.cfg.github_l2": {
		`GH_TOKEN (Docker, CI), leave user empty: "gh auth token --user" ignores GH_TOKEN.`,
		"GH_TOKEN (Docker, CI), laissez user vide : « gh auth token --user » ignore GH_TOKEN.",
	},
	"workspace.cfg.gh_user": {
		"exact login of a gh account; empty: gh's active account",
		"login exact d'un compte gh ; vide : compte actif de gh",
	},
	"workspace.cfg.gh_path": {
		"absolute path, or a name looked up in the PATH",
		"chemin absolu, ou nom cherché dans le PATH",
	},
	"workspace.cfg.git_l1": {
		"Without gh installed and signed in, repositories are tracked by local Git clones",
		"Sans gh installé et connecté, les dépôts sont suivis par clones Git locaux",
	},
	"workspace.cfg.git_l2": {
		"(repos/ at the workspace root), with this machine's SSH key or Git credentials.",
		"(repos/ à la racine de l'espace), avec la clé SSH ou les identifiants Git de cette machine.",
	},
	"workspace.cfg.git_protocol": {
		`"ssh" (git@github.com:…) or "https" (https://github.com/…)`,
		`"ssh" (git@github.com:…) ou "https" (https://github.com/…)`,
	},

	// Réécriture de config.yaml par les réglages communs (shared.go).
	"workspace.shared.read_no_workspace": {
		"reading the settings: workspace not provided",
		"lecture des réglages : espace non renseigné",
	},
	"workspace.shared.write_no_workspace": {
		"writing the settings: workspace not provided",
		"écriture des réglages : espace non renseigné",
	},
	"workspace.shared.read_file": {
		"reading %s",
		"lecture de %s",
	},
	"workspace.shared.rewrite_invalid": {
		"cannot rewrite %s, the current file is invalid",
		"réécriture de %s impossible, le fichier actuel est invalide",
	},
	"workspace.shared.utf8": {
		"setting %s: invalid UTF-8 text, nothing was written",
		"réglage %s : texte UTF-8 invalide, rien n'a été écrit",
	},
	"workspace.shared.reread": {
		"re-reading the written settings",
		"relecture des réglages écrits",
	},
	"workspace.shared.not_modified": {
		"config.yaml was not modified",
		"config.yaml n'a pas été modifié",
	},
	"workspace.shared.mismatch": {
		"written settings differ from the expected settings",
		"réglages écrits différents des réglages attendus",
	},
	"workspace.shared.backup": {
		"backup copy %s",
		"copie de sauvegarde %s",
	},
	"workspace.shared.replace": {
		"replacing %s",
		"remplacement de %s",
	},
	"workspace.shared.tmp_create": {
		"creating a temporary file in %s",
		"création d'un fichier temporaire dans %s",
	},

	// Comparaison des réglages écrits et attendus (compareValues).
	"workspace.compare.missing": {
		"missing key %s",
		"clé %s absente",
	},
	"workspace.compare.different": {
		"key %s: %s instead of %s",
		"clé %s : %s au lieu de %s",
	},
	"workspace.compare.unexpected": {
		"unexpected key %s",
		"clé %s inattendue",
	},
}
