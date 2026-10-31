// Textes du paquet internal/cli : commande racine et son aide (modèle d'usage de cobra, erreurs
// d'options, commande inconnue), carte d'en-tête et astuces, ouverture de l'espace et de la base,
// confirmation interactive, avertissements et sortie JSON, niveau de log, version. Préfixe des
// clés : « cli.core. ».
package i18n

// cliCoreTexts regroupe les textes de internal/cli, enregistrés dans le catalogue par le init()
// ci-dessous.
var cliCoreTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : », l'anglais non.
	"cli.core.err.sep": {": ", " : "},

	// Commande racine (root.go) : description, options globales.
	"cli.core.root.short": {
		"watch GitHub repositories: what moved, when, and who did it",
		"Supervise des dépôts GitHub : ce qui a bougé, quand, et qui l'a fait",
	},
	"cli.core.root.long": {
		`Watchoo watches GitHub repositories, mostly private, and answers three questions:
what moved, when, and who did it.

Each group of tracked repositories lives in its own workspace, created by
"watchoo init". The other commands find the workspace in the current directory
or its parents, like Git, or in the one given by -C or the WATCHOO_DIR variable.

Prerequisite: the GitHub CLI gh, installed and signed in ("gh auth login").`,
		`Watchoo supervise des dépôts GitHub, surtout privés, et répond à trois questions :
qu'est-ce qui a bougé, quand, et qui l'a fait.

Chaque groupe de dépôts suivis vit dans son propre espace de supervision, créé par
« watchoo init ». Les autres commandes trouvent l'espace dans le répertoire courant
ou ses parents, comme Git, ou dans celui donné par -C ou la variable WATCHOO_DIR.

Prérequis : la CLI GitHub gh, installée et connectée (« gh auth login »).`,
	},
	"cli.core.flag.json": {
		"print the output as JSON, for scripts",
		"affiche le résultat au format JSON, pour les scripts",
	},
	"cli.core.flag.workspace": {
		"work on the workspace at this `path` instead of the one found from the current directory",
		"travaille sur l'espace situé à ce `chemin` au lieu de celui trouvé depuis le répertoire courant",
	},
	"cli.core.flag.verbose": {
		"print detailed logs (debug level) to standard error",
		"affiche les logs détaillés (niveau debug) sur la sortie d'erreur",
	},
	"cli.core.flag.version": {
		`print the Watchoo version (like "watchoo version")`,
		"affiche la version de Watchoo (comme « watchoo version »)",
	},

	// Erreurs de la commande racine (root.go) : arguments en trop, commande inconnue, interruption,
	// ligne d'erreur. Les valeurs citées (%s, %q) viennent de la ligne de commande.
	"cli.core.err.version_args": {
		`"%s --version" takes no argument (got: %s)`,
		"« %s --version » ne prend pas d'argument (reçu : %s)",
	},
	"cli.core.err.interrupted": {
		"error: command interrupted (%v)",
		"erreur : commande interrompue (%v)",
	},
	"cli.core.err.line": {
		"error: %s",
		"erreur : %s",
	},
	"cli.core.err.no_args": {
		`%q takes no argument (got: %s)`,
		"« %s » ne prend pas d'argument (reçu : %s)",
	},
	"cli.core.err.unknown_command": {
		"unknown command %q",
		"commande inconnue « %s »",
	},
	"cli.core.err.suggestion_sep": {
		`", "`,
		" », « ",
	},
	"cli.core.err.suggestion": {
		`; did you mean "%s"?`,
		" ; vouliez-vous dire « %s » ?",
	},
	"cli.core.err.command_list": {
		` (command list: "%s --help")`,
		" (liste des commandes : « %s --help »)",
	},

	// Modèle d'usage de cobra (help.go) : titres des sections et dernière ligne.
	"cli.core.usage":          {"Usage:", "Utilisation :"},
	"cli.core.usage_command":  {"[command]", "[commande]"},
	"cli.core.aliases":        {"Aliases:", "Alias :"},
	"cli.core.examples":       {"Examples:", "Exemples :"},
	"cli.core.commands":       {"Commands:", "Commandes :"},
	"cli.core.options":        {"Options:", "Options :"},
	"cli.core.global_options": {"Global options:", "Options globales :"},
	"cli.core.usage_more": {
		`Run "%s [command] --help" for more information about a command.`,
		"Lancez « %s [commande] --help » pour l'aide d'une commande.",
	},

	// Option --help et valeurs par défaut ou obsolètes dans la liste des options (help.go).
	"cli.core.flag.help": {
		"show help for a command",
		"affiche l'aide de la commande",
	},
	"cli.core.flag.default": {
		"(default %s)",
		"(%s par défaut)",
	},
	"cli.core.flag.default_string": {
		`"%s"`,
		"« %s »",
	},
	"cli.core.flag.deprecated": {
		"(deprecated: %s)",
		"(obsolète : %s)",
	},

	// Commande help (help.go).
	"cli.core.help.use":   {"help [command]", "help [commande]"},
	"cli.core.help.short": {"show help for a command", "Affiche l'aide d'une commande"},
	"cli.core.help.long": {
		`show help for a command: "watchoo help init", for example.`,
		"Affiche l'aide d'une commande : « watchoo help init », par exemple.",
	},
	"cli.core.help.unknown": {
		`help: unknown command %q (command list: "%s --help")`,
		"aide : commande inconnue « %s » (liste des commandes : « %s --help »)",
	},

	// Erreurs d'options (help.go) : seuls le nom et la valeur tapés par l'utilisateur sont repris.
	"cli.core.flagerr.unknown_short": {
		"unknown short option: -%s",
		"option courte inconnue : -%s",
	},
	"cli.core.flagerr.unknown": {
		"unknown option: --%s",
		"option inconnue : --%s",
	},
	"cli.core.flagerr.value_required": {
		"option %s%s requires a value",
		"l'option %s%s attend une valeur",
	},
	"cli.core.flagerr.invalid_value": {
		"invalid value %q for --%s",
		"valeur invalide %q pour --%s",
	},
	"cli.core.flagerr.malformed": {
		"malformed option: %s",
		"option mal écrite : %s",
	},
	"cli.core.flagerr.hint": {
		` (help: "%s --help")`,
		" (aide : « %s --help »)",
	},

	// Valeur attendue par une option, selon son type (help.go).
	"cli.core.expected.bool":     {"expected true or false", "true ou false attendu"},
	"cli.core.expected.duration": {"expected a duration, e.g. 30m or 1h", "durée attendue, ex. 30m ou 1h"},
	"cli.core.expected.int":      {"expected an integer", "nombre entier attendu"},

	// Carte d'en-tête (card.go) : sous-titre et astuces. Les astuces ne citent que des commandes qui
	// existent. tip.init_done et tip.status sont aussi affichés par init.go et status.go, qui ne sont
	// pas encore traduits : leurs constantes provisoires gardent le texte français en attendant.
	"cli.core.card.subtitle": {
		"Who worked, on what, and when?",
		"Qui a travaillé, sur quoi, et quand ?",
	},
	"cli.core.tip.help": {
		`Tip: "watchoo help <command>" details a command.`,
		"Astuce : « watchoo help <commande> » détaille une commande.",
	},
	"cli.core.tip.init": {
		`Tip: "watchoo init" makes this directory a workspace.`,
		"Astuce : « watchoo init » fait de ce répertoire un espace de supervision.",
	},
	"cli.core.tip.workspace": {
		`Tip: "watchoo repo add owner/repo", then "watchoo sync".`,
		"Astuce : « watchoo repo add owner/repo » puis « watchoo sync ».",
	},
	"cli.core.tip.init_done": {
		`Tip: "watchoo repo add owner/repo" adds a first repository to track.`,
		"Astuce : « watchoo repo add owner/repo » ajoute un premier dépôt à suivre.",
	},
	"cli.core.tip.status": {
		`Tip: "watchoo sync" updates the tracked repositories.`,
		"Astuce : « watchoo sync » met à jour les dépôts suivis.",
	},
	"cli.core.no_workspace": {
		"No workspace here",
		"Aucun espace de supervision ici",
	},

	// Lignes d'état de la carte (card.go) : espace trouvé, réglages, base, accès aux dépôts.
	"cli.core.status.workspace": {
		"Workspace: %s (%s)",
		"Espace : %s (%s)",
	},
	"cli.core.status.workspace_error": {
		"Workspace: %s",
		"Espace : %s",
	},
	"cli.core.status.settings": {
		"Settings: %s",
		"Réglages : %s",
	},
	"cli.core.status.database": {
		"Database: %s",
		"Base : %s",
	},
	"cli.core.status.source_git": {
		"Source: local Git clones (gh absent or not signed in)",
		"Source : clones Git locaux (gh absent ou non connecté)",
	},
	"cli.core.status.gh_account": {
		"gh account: %s on %s (%s)",
		"Compte gh : %s sur %s (%s)",
	},

	// Erreurs d'affichage et de résolution (card.go).
	"cli.core.err.write_help_header": {
		"writing the help header",
		"affichage de l'en-tête de l'aide",
	},
	"cli.core.err.write_report": {
		"writing the report",
		"affichage du compte rendu",
	},
	"cli.core.err.cwd_unavailable": {
		"current directory unavailable",
		"répertoire courant inaccessible",
	},

	// Ouverture de l'espace et de la base (session.go).
	"cli.core.session.not_found_hint": {
		" (or designate a workspace with -C <directory>)",
		" (ou indiquez un espace avec -C <répertoire>)",
	},
	"cli.core.session.open_db": {
		"opening the workspace database",
		"ouverture de la base de l'espace",
	},
	"cli.core.session.migrate_db": {
		"migrating the workspace database",
		"mise à jour de la base de l'espace",
	},
	"cli.core.err.close_db": {
		"closing the database",
		"fermeture de la base",
	},
	"cli.core.session.repo_not_tracked": {
		`repository %s is not tracked in this workspace (tracked repositories: "watchoo repo list")`,
		"le dépôt %s n'est pas suivi dans cet espace (dépôts suivis : « watchoo repo list »)",
	},

	// Confirmation interactive (confirm.go).
	"cli.core.confirm.need_terminal": {
		"cannot confirm: standard input is not a terminal, run again with --yes to confirm",
		"confirmation impossible : l'entrée standard n'est pas un terminal, relancez avec --yes pour confirmer",
	},
	"cli.core.confirm.yes_no": {
		"[y/N] ",
		"[o/N] ",
	},

	// Erreurs d'entrée-sortie de la confirmation (confirm.go).
	"cli.core.err.write_question": {
		"writing the question",
		"affichage de la question",
	},
	"cli.core.err.read_answer": {
		"reading the answer",
		"lecture de la réponse",
	},

	// Sortie JSON et avertissements (output.go).
	"cli.core.err.write_json": {
		"writing the JSON output",
		"écriture de la sortie JSON",
	},
	"cli.core.warn.line": {
		"warning: %s",
		"attention : %s",
	},

	// Niveau de log inconnu (logging.go).
	"cli.core.log.level_unknown": {
		"%s=%q unknown: warn level used (values: debug, info, warn, error)",
		"%s=%q inconnu : niveau warn utilisé (valeurs : debug, info, warn, error)",
	},

	// Commande version (version.go).
	"cli.core.version.short": {
		"show the Watchoo version",
		"Affiche la version de Watchoo",
	},
	"cli.core.err.write_version": {
		"writing the version",
		"affichage de la version",
	},
}

func init() {
	register(cliCoreTexts)
}
