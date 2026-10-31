// Textes du paquet internal/cli pour les commandes init, repo (add, list, remove), repo import et
// config (export, import) : résumés, comptes rendus, aperçus, bilans, confirmations, erreurs
// d'usage et avertissements. Préfixe des clés : « cli.repo. ». L'aide des commandes (Short, Long,
// usage des options, exemples) et les textes des autres commandes viendront dans leurs domaines.
//
// Les clés JSON des sorties --json ne changent jamais (schéma stable) : seules les valeurs de
// message (erreurs, avertissements) passent par le catalogue.
package i18n

// cliRepoTexts regroupe les textes de internal/cli (init, repo, repo import, config), enregistrés
// dans le catalogue par le init() ci-dessous.
var cliRepoTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « ; », l'anglais non.
	// Le séparateur « : » est commun à toute la CLI (cli.core.err.sep).
	"cli.repo.sep.semicolon": {"; ", " ; "},

	// Conseils gh (init.go) : que faire quand gh est absent ou non connecté.
	"cli.repo.hint.install_gh": {
		"install the GitHub CLI gh: https://cli.github.com/",
		"installez la CLI GitHub gh : https://cli.github.com/",
	},
	"cli.repo.hint.login_gh": {
		"run \"gh auth login\"",
		"lancez « gh auth login »",
	},

	// init (init.go).
	"cli.repo.init.need_git": {
		"%s; without gh, repositories are tracked by local Git clones, but %s",
		"%s ; sans gh, les dépôts sont suivis par clones Git locaux, mais %s",
	},
	"cli.repo.init.nothing_modified": {
		"nothing was modified",
		"rien n'a été modifié",
	},
	"cli.repo.init.cleanup": {
		"cleaning up %s",
		"nettoyage de %s",
	},
	"cli.repo.cwd_unavailable": {
		"current directory unavailable",
		"répertoire courant inaccessible",
	},
	"cli.repo.init.db_create": {
		"creating the workspace database",
		"création de la base de l'espace",
	},
	"cli.repo.init.db_close": {
		"closing database %s",
		"fermeture de la base %s",
	},
	"cli.repo.init.reread_settings": {
		"re-reading the settings of the created workspace",
		"relecture des réglages de l'espace créé",
	},
	"cli.repo.init.no_gh_local": {
		"repositories will be tracked by local Git clones (repos/ in the workspace), with this " +
			"machine's SSH key or Git credentials (no pull requests, releases or stars)",
		"les dépôts seront suivis par clones Git locaux (repos/ dans l'espace), avec la clé SSH ou les " +
			"identifiants Git de cette machine (sans pull requests, releases ni étoiles)",
	},
	"cli.repo.init.gh_not_found": {
		"gh not found (github.gh_path: %q): %s; %s, then run \"gh auth login\" to go through the GitHub API",
		"gh introuvable (github.gh_path : %q) : %s ; %s, puis lancez « gh auth login » pour " +
			"passer par l'API GitHub",
	},
	"cli.repo.init.gh_not_logged": {
		"gh is not logged in to any GitHub account: %s; %s to go through the GitHub API",
		"gh n'est connecté à aucun compte GitHub : %s ; %s pour passer par l'API GitHub",
	},
	"cli.repo.init.gh_not_found_short": {
		"gh not found (github.gh_path: %q): %s",
		"gh introuvable (github.gh_path : %q) : %s",
	},
	"cli.repo.init.checking_gh": {
		"checking gh",
		"vérification de gh",
	},
	"cli.repo.init.nested": {
		"%s is inside workspace %s: the new workspace is nested in it and independent " +
			"(separate repositories, settings and data)",
		"%s est dans l'espace %s : le nouvel espace est imbriqué et indépendant du parent " +
			"(dépôts, réglages et données séparés)",
	},
	"cli.repo.init.env_err": {
		"variable %s is set to %q: while it is set, the other commands will fail (%v)",
		"la variable %s vaut %q : tant qu'elle est définie, les autres commandes échoueront (%v)",
	},
	"cli.repo.init.env_other": {
		"variable %s is set to %q: while it is set, the other commands will use workspace %s, not this one",
		"la variable %s vaut %q : tant qu'elle est définie, les autres commandes utiliseront " +
			"l'espace %s et non celui-ci",
	},
	"cli.repo.init.created_card": {
		"Workspace %q created",
		"Espace de supervision « %s » créé",
	},
	"cli.repo.init.summary": {
		"Workspace %q created.\n  directory   %s\n  settings    %s\n  database    %s\n  %s\n",
		"Espace de supervision « %s » créé.\n  répertoire  %s\n  réglages    %s\n  base        %s\n  %s\n",
	},
	"cli.repo.init.account_git": {
		"source      local Git clones (gh absent or not logged in)",
		"source      clones Git locaux (gh absent ou non connecté)",
	},
	"cli.repo.init.account_gh": {
		"gh account  %s on %s (%s)",
		"compte gh   %s sur %s (%s)",
	},
	"cli.repo.init.scopes": {
		"scopes: %s",
		"droits : %s",
	},
	"cli.repo.init.scopes_unknown": {
		"scopes not reported by gh",
		"droits non indiqués par gh",
	},

	// Erreurs d'usage (repo.go).
	"cli.repo.args.exact": {
		"%q expects %d argument%s (got: %d); usage: %s",
		"« %s » attend %d argument%s (reçu : %d) ; utilisation : %s",
	},
	"cli.repo.args.max": {
		"%q expects at most %d argument%s (got: %s); usage: %s",
		"« %s » attend au plus %d argument%s (reçu : %s) ; utilisation : %s",
	},

	// Mots réutilisés dans les comptages (countText) et les bilans.
	"cli.repo.word.commit":       {"commit", "commit"},
	"cli.repo.word.commits":      {"commits", "commits"},
	"cli.repo.word.repository":   {"repository", "dépôt"},
	"cli.repo.word.repositories": {"repositories", "dépôts"},
	"cli.repo.word.setting":      {"setting", "réglage"},
	"cli.repo.word.settings":     {"settings", "réglages"},

	// repo add (repo.go).
	"cli.repo.branches.all":  {"all branches", "toutes les branches"},
	"cli.repo.branches.list": {"branches: %s", "branches : %s"},
	"cli.repo.add.missing_result": {
		"adding repository %s: missing result",
		"ajout du dépôt %s : résultat manquant",
	},
	"cli.repo.add.inaccessible": {
		"repository %s inaccessible: %s (check its name and that the gh account has access, or " +
			"without gh the SSH key or the Git credentials of this machine)",
		"dépôt %s inaccessible : %s (vérifiez son nom et que le compte gh, ou sans gh " +
			"la clé SSH ou les identifiants Git de cette machine, y a accès)",
	},
	"cli.repo.add.error":         {"adding repository %s: %s", "ajout du dépôt %s : %s"},
	"cli.repo.add.details_label": {"label %q", "label « %s »"},
	"cli.repo.add.added":         {"Repository %s added (%s).\n", "Dépôt %s ajouté (%s).\n"},
	"cli.repo.add.updated":       {"Repository %s already tracked, updated (%s).\n", "Dépôt %s déjà suivi, mis à jour (%s).\n"},
	"cli.repo.add.already":       {"Repository %s already tracked: nothing to change (%s).\n", "Dépôt %s déjà suivi : rien à changer (%s).\n"},
	"cli.repo.add.renamed":       {"Repository %s is now named %s on GitHub.\n", "Le dépôt %s s'appelle maintenant %s sur GitHub.\n"},
	"cli.repo.sync_hint":         {"Run \"watchoo sync\" to sync it.\n", "Lancez « watchoo sync » pour le synchroniser.\n"},
	"cli.repo.sync_hint_plural":  {"Run \"watchoo sync\" to sync the repositories.\n", "Lancez « watchoo sync » pour synchroniser les dépôts.\n"},
	"cli.repo.write_output":      {"writing output", "écriture de la sortie"},

	// Seuils propres d'un dépôt (repo.go, config_cmd.go).
	"cli.repo.days.flemme":    {"flemme %d d", "flemme %d j"},
	"cli.repo.days.dead":      {"dead %d d", "dead %d j"},
	"cli.repo.days.prefix":    {"thresholds: ", "seuils : "},
	"cli.repo.days.value":     {"%d d", "%d j"},
	"cli.repo.days.workspace": {"workspace", "espace"},

	// repo list (repo.go).
	"cli.repo.list.empty": {
		"No repository tracked in this workspace. Add some with \"watchoo repo add owner/repo\" " +
			"or \"watchoo repo import file.csv\".\n",
		"Aucun dépôt suivi dans cet espace. Ajoutez-en avec « watchoo repo add owner/repo » ou " +
			"« watchoo repo import fichier.csv ».\n",
	},
	"cli.repo.list.header_repo":        {"Repository", "Dépôt"},
	"cli.repo.list.header_repo_ascii":  {"Repository", "Depot"},
	"cli.repo.list.header_commit":      {"Last commit", "Dernier commit"},
	"cli.repo.list.header_author":      {"Author", "Auteur"},
	"cli.repo.list.header_state":       {"State", "État"},
	"cli.repo.list.header_state_ascii": {"State", "Etat"},

	// repo remove (repo.go).
	"cli.repo.remove.already_stopped": {
		"Tracking of %s is already stopped: nothing changed.\n",
		"Le suivi de %s est déjà arrêté : rien n'a changé.\n",
	},
	"cli.repo.remove.prompt_delete": {
		"Stop tracking %s and delete its history (%s)?",
		"Arrêter le suivi de %s et supprimer son historique (%s) ?",
	},
	"cli.repo.remove.prompt_keep": {
		"Stop tracking %s (its history is kept)?",
		"Arrêter le suivi de %s (l'historique est gardé) ?",
	},
	"cli.repo.remove.cancelled": {
		"removal cancelled: nothing was modified",
		"suppression annulée : rien n'a été modifié",
	},
	"cli.repo.remove.stopped": {
		"Tracking of %s stopped; its history is kept (\"watchoo repo add %s\" resumes it).\n",
		"Suivi de %s arrêté ; son historique est gardé (« watchoo repo add %s » le reprend).\n",
	},
	"cli.repo.remove.deleted": {
		"Repository %s removed: tracking stopped, history deleted (%s).\n",
		"Dépôt %s retiré : suivi arrêté, historique supprimé (%s).\n",
	},

	// repo import (repo_import.go) : lecture du fichier, bilan, échec.
	"cli.repo.import.file_not_found": {
		"import file %s not found",
		"fichier d'import %s introuvable",
	},
	"cli.repo.import.file_open": {
		"opening the import file",
		"ouverture du fichier d'import",
	},
	"cli.repo.import.file_read": {
		"reading the import file %s",
		"lecture du fichier d'import %s",
	},
	"cli.repo.import.nothing_imported": {
		"nothing was imported",
		"rien n'a été importé",
	},
	"cli.repo.import.dry_run_title": {
		"  (dry run: nothing was saved)",
		"  (simulation : rien n'a été enregistré)",
	},
	"cli.repo.import.line_detail": {
		"  line %d  %s: %s",
		"  ligne %d  %s : %s",
	},
	"cli.repo.import.failure_title": {
		"import incomplete: %s (%s); run the import again once access has been granted",
		"import incomplet : %s (%s) ; relancez l'import une fois l'accès donné",
	},
	"cli.repo.import.failure_inaccessible_one": {
		"1 inaccessible repository",
		"1 dépôt inaccessible",
	},
	"cli.repo.import.failure_inaccessible_many": {
		"%d inaccessible repositories",
		"%d dépôts inaccessibles",
	},
	"cli.repo.import.failure_errored_one": {
		"1 repository in error",
		"1 dépôt en erreur",
	},
	"cli.repo.import.failure_errored_many": {
		"%d repositories in error",
		"%d dépôts en erreur",
	},

	// Libellés des bilans (repo_import.go, config_cmd.go).
	"cli.repo.summary.added":         {"added", "ajoutés"},
	"cli.repo.summary.to_add":        {"to add", "à ajouter"},
	"cli.repo.summary.already":       {"already tracked", "déjà suivis"},
	"cli.repo.summary.updated":       {"updated", "mis à jour"},
	"cli.repo.summary.to_update":     {"to update", "à mettre à jour"},
	"cli.repo.summary.to_reenable":   {"to resume", "à reprendre"},
	"cli.repo.summary.reenabled":     {"resumed", "dont repris"},
	"cli.repo.summary.unchanged":     {"unchanged", "inchangés"},
	"cli.repo.summary.inaccessible":  {"inaccessible", "inaccessibles"},
	"cli.repo.summary.errored":       {"in error", "en erreur"},
	"cli.repo.summary.lines_errored": {"lines in error", "lignes en erreur"},

	// config export (config_cmd.go).
	"cli.repo.config_export_exists": {
		"file %s already exists; run again with --force to overwrite it",
		"le fichier %s existe déjà ; relancez avec --force pour l'écraser",
	},
	"cli.repo.export_exists": {"file %s already exists", "le fichier %s existe déjà"},
	"cli.repo.export_is_dir": {
		"%s is a directory: give a file name",
		"%s est un répertoire : indiquez un nom de fichier",
	},
	"cli.repo.export_replace": {"replacing %s", "remplacement de %s"},
	"cli.repo.export_create":  {"creating %s", "création de %s"},
	"cli.repo.export_write":   {"writing %s", "écriture de %s"},
	"cli.repo.config_export_done": {
		"Configuration exported to %s: %s, %s.\n",
		"Configuration exportée dans %s : %s, %s.\n",
	},

	// config import (config_cmd.go) : contrôles d'options, fichier, confirmation.
	"cli.repo.config_import.stdin_yes": {
		"\"-\" reads the file from standard input, which can no longer be used to confirm: " +
			"run again with --yes (or --dry-run for a preview)",
		"« - » lit le fichier sur l'entrée standard, qui ne peut plus servir à confirmer : " +
			"relancez avec --yes (ou --dry-run pour un aperçu)",
	},
	"cli.repo.config_import.json_yes": {
		"--json without --dry-run asks no question: run again with --yes to apply " +
			"(or --dry-run for the preview)",
		"--json sans --dry-run ne pose pas de question : relancez avec --yes pour appliquer " +
			"(ou --dry-run pour l'aperçu)",
	},
	"cli.repo.config_import.dry_run_footer": {
		"\nPreview only (--dry-run): nothing was modified.\n",
		"\nAperçu seulement (--dry-run) : rien n'a été modifié.\n",
	},
	"cli.repo.config_import.nothing_to_change": {
		"Nothing to change: the workspace is already up to date.\n",
		"Rien à changer : l'espace est déjà à jour.\n",
	},
	"cli.repo.config_import.apply_q": {"Apply these changes?", "Appliquer ces changements ?"},
	"cli.repo.config_import.cancelled": {
		"import cancelled: nothing was modified",
		"import annulé : rien n'a été modifié",
	},
	"cli.repo.config_import.no_result": {"import: no result", "import : aucun résultat"},
	"cli.repo.config.file_not_found": {
		"configuration file %s not found",
		"fichier de configuration %s introuvable",
	},
	"cli.repo.config.file_open": {
		"opening the configuration file",
		"ouverture du fichier de configuration",
	},
	"cli.repo.config.is_dir": {
		"%s is a directory, not a configuration file",
		"%s est un répertoire, pas un fichier de configuration",
	},

	// Aperçu d'un config import (config_cmd.go).
	"cli.repo.config_preview.origin_ws":       {"workspace %q", "espace « %s »"},
	"cli.repo.config_preview.exported":        {"exported on %s", "exporté le %s"},
	"cli.repo.config_preview.date_fmt":        {"01/02/2006", "02/01/2006"},
	"cli.repo.config_preview.with_version":    {"with Watchoo %s", "avec Watchoo %s"},
	"cli.repo.config_preview.settings_header": {"Settings\n", "Réglages\n"},
	"cli.repo.config_preview.repos_header":    {"Repositories\n", "Dépôts\n"},
	"cli.repo.config_preview.none_in_file":    {"  none in the file\n", "  aucun dans le fichier\n"},
	"cli.repo.config_preview.unchanged_settings": {
		"  unchanged (%s, identical to those of the workspace)\n",
		"  inchangés (%s, identiques à ceux de l'espace)\n",
	},
	"cli.repo.config_preview.env_masked": {"   (overridden by %s)", "   (masqué par %s)"},
	"cli.repo.config_preview.access_later": {
		"(repository access will be checked against GitHub when applying)\n",
		"(l'accès aux dépôts sera vérifié auprès de GitHub au moment d'appliquer)\n",
	},

	// Détail d'un changement de dépôt dans l'aperçu (config_cmd.go).
	"cli.repo.change.label":        {"label: %s → %s", "nom : %s → %s"},
	"cli.repo.change.branches":     {"branches: %s → %s", "branches : %s → %s"},
	"cli.repo.change.flemme":       {"flemme: %s → %s", "flemme : %s → %s"},
	"cli.repo.change.dead":         {"dead: %s → %s", "dead : %s → %s"},
	"cli.repo.change.all_branches": {"all", "toutes"},
	"cli.repo.change.no_label":     {"none", "aucun"},
	"cli.repo.change.label_quoted": {"%q", "« %s »"},
	"cli.repo.change.failure_detail": {
		"  repository #%d  %s: %s",
		"  dépôt n° %d  %s : %s",
	},

	// Bilan d'un config import appliqué (config_cmd.go).
	"cli.repo.result.title":            {"SUMMARY\n", "BILAN\n"},
	"cli.repo.result.settings_written": {"  settings written in %s", "  réglages écrits dans %s"},
	"cli.repo.result.backup":           {" (old file: %s)", " (ancien fichier : %s)"},
	"cli.repo.result.settings_unchanged": {
		"  settings unchanged: config.yaml was not modified\n",
		"  réglages inchangés : config.yaml n'a pas été modifié\n",
	},
	"cli.repo.result.settings_note": {
		"The settings apply to the commands run afterwards; an already open \"watchoo serve\" or " +
			"\"watchoo tui\" will pick them up on its next start.\n",
		"Les réglages valent pour les commandes lancées ensuite ; un « watchoo serve » ou " +
			"« watchoo tui » déjà ouvert les prendra en compte à son prochain lancement.\n",
	},
}

func init() {
	register(cliRepoTexts)
}
