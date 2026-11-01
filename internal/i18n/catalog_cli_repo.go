// Textes du paquet internal/cli pour les commandes init, repo (add, list, remove), repo import et
// config (export, import) : résumés, comptes rendus, aperçus, bilans, confirmations, erreurs
// d'usage et avertissements, et l'aide de ces commandes (Short, Long, usage des options : clés
// « cli.repo.help. »). Préfixe des clés : « cli.repo. ».
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

	// Aide des commandes init, repo, repo add / list / remove / import, config, config export / import
	// (Short, Long, nom des arguments dans Use, usage des options).
	"cli.repo.help.file_arg": {"file", "fichier"},
	"cli.repo.help.init_short": {
		"Makes the current directory a workspace",
		"Fait du répertoire courant un espace de supervision",
	},
	"cli.repo.help.init_long": {
		`Makes the current directory (or the one given by -C) a workspace: checks the
GitHub CLI gh, then creates .watchoo/ with config.yaml, the SQLite database at the
right schema and a .gitignore. The .watchoo/ directory is readable by you only
(permissions 0700, files in 0600).

Without gh installed and signed in, init warns that the repositories will be tracked
through local Git clones (git must then be installed, otherwise init stops) and
reminds "gh auth login" to go through the GitHub API.

init refuses a directory that is already a workspace, so as to never overwrite data,
and warns if the created workspace is nested in another workspace. It neither asks
for nor stores any token: gh keeps it.`,
		`Fait du répertoire courant (ou de celui donné par -C) un espace de supervision :
vérifie la CLI GitHub gh, puis crée .watchoo/ avec config.yaml, la base SQLite au bon
schéma et un .gitignore. Le répertoire .watchoo/ est lisible par vous seul (droits
0700, fichiers en 0600).

Sans gh installé et connecté, init prévient que les dépôts seront suivis par clones
Git locaux (git doit alors être installé, sinon init s'arrête) et rappelle
« gh auth login » pour passer par l'API GitHub.

init refuse un répertoire qui est déjà un espace, pour ne jamais écraser de données,
et prévient si l'espace créé est imbriqué dans un autre espace. Il ne demande ni ne
stocke aucun token : gh le garde.`,
	},
	"cli.repo.help.repo_short": {
		"Manages the tracked repositories (add, import, list, remove)",
		"Gère les dépôts suivis (add, import, list, remove)",
	},
	"cli.repo.help.add_short": {
		"Adds a repository to watch",
		"Ajoute un dépôt à surveiller",
	},
	"cli.repo.help.add_long": {
		`Adds a repository to watch, after checking with GitHub that the gh account can
access it (without gh installed and signed in: that git can read it, with
"git ls-remote", using the SSH key or the Git credentials of this machine). The
repository is given as owner/repo or by its GitHub URL.

Without --branch, all the branches of the repository are tracked, including those
created later. --branch restricts the tracking to a list of names and patterns
(release/* for example); --branch "" goes back to all the branches.

--flemme and --dead set the activity thresholds of this repository, in days without
a commit, instead of those of the workspace (activity.*); 0 goes back to the
threshold of the workspace. When both are given, --dead must be greater than
--flemme.

A repository already tracked is not added twice: its label, branches and thresholds
are updated if they are given, and its tracking resumes if it had been stopped. No
sync is started: follow up with "watchoo sync".`,
		`Ajoute un dépôt à surveiller, après avoir vérifié auprès de GitHub que le compte gh y a
accès (sans gh installé et connecté : que git peut le lire, par « git ls-remote », avec la
clé SSH ou les identifiants Git de cette machine). Le dépôt est désigné par owner/repo ou
par son URL GitHub.

Sans --branch, toutes les branches du dépôt sont suivies, y compris celles créées plus
tard. --branch restreint le suivi à une liste de noms et de motifs (release/* par
exemple) ; --branch "" revient à toutes les branches.

--flemme et --dead règlent les seuils d'activité de ce dépôt, en jours sans commit, à la
place de ceux de l'espace (activity.*) ; 0 revient au seuil de l'espace. Quand les deux
sont donnés, --dead doit être plus grand que --flemme.

Un dépôt déjà suivi n'est pas ajouté en double : son label, ses branches et ses seuils
sont mis à jour s'ils sont donnés, et son suivi reprend s'il avait été arrêté. Aucune synchro
n'est lancée : enchaînez avec « watchoo sync ».`,
	},
	"cli.repo.help.add_flag_branch": {
		"`branches` to track, separated by commas, patterns accepted (e.g. main,dev,release/*); default: all",
		"`branches` à suivre, séparées par des virgules, motifs acceptés (ex. main,dev,release/*) ; défaut : toutes",
	},
	"cli.repo.help.add_flag_label": {
		"`name` displayed instead of owner/repo (e.g. \"Alice Martin\")",
		"`nom` affiché à la place de owner/repo (ex. \"Alice Martin\")",
	},
	"cli.repo.help.add_flag_flemme": {
		"Flemme threshold of this repository, in `days` without a commit (0 = threshold of the workspace)",
		"seuil Flemme de ce dépôt, en `jours` sans commit (0 = seuil de l'espace)",
	},
	"cli.repo.help.add_flag_dead": {
		"Dead threshold of this repository, in `days` without a commit, greater than --flemme (0 = threshold of the workspace)",
		"seuil Dead de ce dépôt, en `jours` sans commit, plus grand que --flemme (0 = seuil de l'espace)",
	},
	"cli.repo.help.list_short": {
		"Lists the tracked repositories: last commit, author, state",
		"Liste les dépôts suivis : dernier commit, auteur, état",
	},
	"cli.repo.help.list_long": {
		`Lists the repositories of the workspace with their last commit (all tracked
branches together), its author and their state:
  -        Active    (last commit 0 to 5 days ago, by default)
  ~ 9 d    Slacking  (6 to 15 days)
  ⚠ 42 d   Dead      (more than 15 days)
In gray: stopped (tracking stopped), error (last sync failed), never (never
synced), archived (archived on GitHub), empty (no commit).`,
		`Liste les dépôts de l'espace avec leur dernier commit (toutes branches suivies
confondues), son auteur et leur état :
  -        Actif   (dernier commit il y a 0 à 5 jours, par défaut)
  ~ 9 j    Flemme  (6 à 15 jours)
  ⚠ 42 j   Dead    (plus de 15 jours)
En gris : arrêté (suivi arrêté), erreur (dernière synchro en échec), jamais
(jamais synchronisé), archivé (archivé sur GitHub), vide (aucun commit).`,
	},
	"cli.repo.help.remove_short": {
		"Stops tracking a repository and deletes its history",
		"Arrête le suivi d'un dépôt et supprime son historique",
	},
	"cli.repo.help.remove_long": {
		`Stops tracking a repository and deletes its history from the database: branches,
commits and sync log, as well as its local Git clone (repos/ in the workspace) if it
exists. The contributors stay (they may have worked on other repositories). Nothing is
changed on GitHub.

With --keep-history, the tracking stops but the history can still be viewed (the
repository appears "stopped" in "watchoo repo list") and the local Git clone is kept;
"watchoo repo add" resumes it.

The command asks for confirmation in a terminal; in a script (standard input that is
not a terminal), the --yes option is required.`,
		`Arrête le suivi d'un dépôt et supprime de la base son historique : branches, commits
et journal des synchros, ainsi que son clone Git local (repos/ dans l'espace) s'il
existe. Les contributeurs restent (ils peuvent avoir travaillé sur d'autres dépôts).
Rien n'est modifié sur GitHub.

Avec --keep-history, le suivi s'arrête mais l'historique reste consultable (le dépôt
apparaît « arrêté » dans « watchoo repo list ») et le clone Git local est gardé ;
« watchoo repo add » le reprend.

La commande demande confirmation dans un terminal ; dans un script (entrée standard
qui n'est pas un terminal), l'option --yes est obligatoire.`,
	},
	"cli.repo.help.remove_flag_keep": {
		"stops the tracking but keeps the history in the database",
		"arrête le suivi mais garde l'historique en base",
	},
	"cli.repo.help.remove_flag_yes": {
		"confirms without asking (required outside a terminal)",
		"confirme sans poser de question (obligatoire hors terminal)",
	},
	"cli.repo.help.import_short": {
		"Adds a list of repositories at once (CSV or one repository per line)",
		"Ajoute d'un coup une liste de dépôts (CSV ou un dépôt par ligne)",
	},
	"cli.repo.help.import_long": {
		`Adds a list of repositories at once, for example those of a class.

The file is a CSV with a header line repo,label,branches (only the repo column is
required; flemme and dead, optional, give the activity thresholds of the repository
in days, 0 to go back to the thresholds of the workspace), or a plain text with one
repository per line. Empty lines and those starting with # are ignored. "-" reads
the list from standard input.

The whole file is validated before anything is added: a malformed line stops the
import and gives its number. The access to each repository is then checked with
GitHub; an inaccessible repository is reported without blocking the others. A
repository already tracked is not added twice: its label, branches and thresholds are
updated if the file gives them (an empty cell keeps the existing value). The import
can therefore be run again safely.

The exit code is non-zero if inaccessible repositories remain. The import does not
start a sync: follow up with "watchoo sync".`,
		`Ajoute d'un coup une liste de dépôts, par exemple ceux d'une promotion.

Le fichier est un CSV avec une ligne d'en-tête repo,label,branches (seule la colonne
repo est obligatoire ; flemme et dead, facultatives, donnent les seuils d'activité du
dépôt en jours, 0 pour revenir aux seuils de l'espace), ou un simple texte avec un dépôt par ligne. Les lignes vides et
celles qui commencent par # sont ignorées. « - » lit la liste sur l'entrée standard.

Tout le fichier est validé avant d'ajouter quoi que ce soit : une ligne mal formée
arrête l'import en indiquant son numéro. L'accès à chaque dépôt est ensuite vérifié
auprès de GitHub ; un dépôt inaccessible est signalé sans bloquer les autres. Un dépôt
déjà suivi n'est pas ajouté en double : son label, ses branches et ses seuils sont mis à
jour si le fichier en donne (une cellule vide garde la valeur existante). L'import peut donc être
relancé sans risque.

Le code de retour est non nul s'il reste des dépôts inaccessibles. L'import ne lance
pas de synchro : enchaînez avec « watchoo sync ».`,
	},
	"cli.repo.help.import_flag_dry_run": {
		"validates the file and checks the access to the repositories, without recording anything",
		"valide le fichier et vérifie l'accès aux dépôts, sans rien enregistrer",
	},
	"cli.repo.help.config_short": {
		"Shares the configuration of the workspace with other supervisors (export, import)",
		"Partage la configuration de l'espace avec d'autres superviseurs (export, import)",
	},
	"cli.repo.help.config_long": {
		`Shares the configuration of the workspace with the other people who supervise the
same group: "watchoo config export" writes the tracked repositories and the common
settings to a JSON file, "watchoo config import" merges it into another workspace,
after a preview. Nothing is ever removed by the import.`,
		`Partage la configuration de l'espace avec les autres personnes qui supervisent le
même groupe : « watchoo config export » écrit les dépôts suivis et les réglages
communs dans un fichier JSON, « watchoo config import » le fusionne dans un autre
espace, après un aperçu. Rien n'est jamais retiré à l'import.`,
	},
	"cli.repo.help.export_short": {
		"Writes the shareable configuration of the workspace to a JSON file",
		"Écrit la configuration partageable de l'espace dans un fichier JSON",
	},
	"cli.repo.help.export_long": {
		`Writes the shareable configuration of the workspace to a JSON file: the tracked
repositories (display name, branches) and the common settings (sync.*, activity.*,
branches.exclude, timezone, lang). Never the settings specific to this machine
(github.*, web.*), nor token, nor commit, nor e-mail.

Without -o, the document is written to standard output. With -o, the file is created
in 0600; an existing file is overwritten only with --force.`,
		`Écrit la configuration partageable de l'espace dans un fichier JSON : les dépôts
suivis (nom affiché, branches) et les réglages communs (sync.*, activity.*,
branches.exclude, timezone, lang). Jamais les réglages propres à cette machine (github.*,
web.*), ni token, ni commit, ni e-mail.

Sans -o, le document est écrit sur la sortie standard. Avec -o, le fichier est créé
en 0600 ; un fichier existant n'est écrasé qu'avec --force.`,
	},
	"cli.repo.help.export_flag_output": {
		"writes the document to this `file` (created in 0600) instead of standard output",
		"écrit le document dans ce `fichier` (créé en 0600) au lieu de la sortie standard",
	},
	"cli.repo.help.export_flag_force": {
		"overwrites the file of -o if it already exists",
		"écrase le fichier de -o s'il existe déjà",
	},
	"cli.repo.help.config_import_short": {
		"Merges a shared configuration file into the workspace, after a preview",
		"Fusionne un fichier de configuration partagé dans l'espace, après un aperçu",
	},
	"cli.repo.help.config_import_long": {
		`Merges into the workspace a file written by "watchoo config export", after a preview.

The whole file is validated before anything is changed. The missing repositories are
added (access checked with GitHub), the repositories already tracked receive the
display name and the branches of the file, a stopped repository resumes its tracking;
nothing is ever removed. If a common setting changes, .watchoo/config.yaml is
rewritten from the commented template (copy of the old one in config.yaml.bak).

On a terminal, the preview is followed by a question; otherwise --yes is required, as
well as with --json (without --dry-run). "-" reads the file from standard input (--yes
required then). The exit code is non-zero if inaccessible repositories remain. The
import does not start a sync: follow up with "watchoo sync".`,
		`Fusionne dans l'espace un fichier écrit par « watchoo config export », après un aperçu.

Tout le fichier est validé avant de modifier quoi que ce soit. Les dépôts absents
sont ajoutés (accès vérifié auprès de GitHub), les dépôts déjà suivis reçoivent le nom
affiché et les branches du fichier, un dépôt arrêté reprend son suivi ; rien n'est
jamais retiré. Si un réglage commun change, .watchoo/config.yaml est réécrit à partir
du modèle commenté (copie de l'ancien dans config.yaml.bak).

Sur un terminal, l'aperçu est suivi d'une question ; sinon --yes est obligatoire,
ainsi qu'avec --json (sans --dry-run). « - » lit le fichier sur l'entrée standard
(--yes obligatoire alors). Le code de retour est non nul s'il reste des dépôts
inaccessibles. L'import ne lance pas de synchro : enchaînez avec « watchoo sync ».`,
	},
	"cli.repo.help.config_import_flag_dry_run": {
		"shows the preview and checks the access to the repositories, without writing anything",
		"affiche l'aperçu et vérifie l'accès aux dépôts, sans rien écrire",
	},
	"cli.repo.help.config_import_flag_yes": {
		"applies without asking for confirmation",
		"applique sans demander de confirmation",
	},
}

func init() {
	register(cliRepoTexts)
}
