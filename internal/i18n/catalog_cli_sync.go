// Textes du paquet internal/cli pour les commandes sync, status, log, contributors, serve, update,
// prs, releases et tui : aide des commandes (Short, Long, usage des options), bilans de
// synchronisation, tableaux, quota, avertissements, avis et mise à jour de Watchoo. Préfixe des
// clés : « cli.sync. ». Les libellés communs avec les autres commandes (préfixe d'erreur, ligne
// d'avertissement, compte gh de la carte) viennent de cli.core ; les mots des comptages communs
// (commit, dépôt) de cli.repo.
//
// Les clés JSON des sorties --json ne changent jamais (schéma stable) : seules les valeurs de
// message (erreurs, avertissements, quota) passent par le catalogue. Les dates relatives (« il y a
// 2 h ») et les états courts (« ~ 9 j ») sont produits par internal/activity, traité à part.
package i18n

// cliSyncTexts regroupe les textes de internal/cli (sync et commandes associées), enregistrés dans
// le catalogue par le init() ci-dessous.
var cliSyncTexts = map[string]entry{
	// Séparateur décimal des durées courtes (« 1.2 s » en anglais, « 1,2 s » en français).
	"cli.sync.duration_decimal": {".", ","},

	// sync (sync.go) : aide, rapport texte, bilan, quota, source.
	"cli.sync.sync_short": {
		"Syncs all tracked repositories, or a single one",
		"Synchronise tous les dépôts suivis, ou un seul",
	},
	"cli.sync.sync_long": {
		`Syncs all tracked repositories (at most sync.parallelism at a time), or the single
given repository, even if its tracking is stopped. The sync is incremental: only
new commits are downloaded.

A repository already being synced (by "watchoo tui" or another terminal) is skipped.
When the GitHub API quota drops below 10%, the remaining repositories are postponed,
with the time to try again.

Without gh installed and signed in, repositories are read from local Git clones
(repos/<name> at the workspace root, updated by "git fetch", working tree reset to
the default branch: a local modification of the tracked files is overwritten) with
this machine's SSH key or Git credentials: no quota, but no pull requests or
releases.

The exit code is non-zero if a repository is in error.`,
		`Synchronise tous les dépôts suivis (au plus sync.parallelism à la fois), ou le seul
dépôt donné, même si son suivi est arrêté. La synchro est incrémentale : seuls les
nouveaux commits sont téléchargés.

Un dépôt déjà en cours de synchronisation (par « watchoo tui » ou un autre terminal)
est ignoré. Quand le quota de l'API GitHub passe sous 10 %, les dépôts restants sont
reportés, avec l'heure à laquelle réessayer.

Sans gh installé et connecté, les dépôts sont lus depuis des clones Git locaux
(repos/<nom> à la racine de l'espace, mis à jour par « git fetch », copie de travail
remise sur la branche par défaut : une modification locale des fichiers suivis est
écrasée) avec la clé SSH ou les identifiants Git de cette machine : pas de quota,
mais ni pull requests ni releases.

Le code de retour est non nul si un dépôt est en erreur.`,
	},
	"cli.sync.no_repos": {
		"No repository to sync. Add some with \"watchoo repo add owner/repo\".\n",
		"Aucun dépôt à synchroniser. Ajoutez-en avec « watchoo repo add owner/repo ».\n",
	},
	"cli.sync.header_repo":                {"Repository", "Dépôt"},
	"cli.sync.header_repo_ascii":          {"Repository", "Depot"},
	"cli.sync.header_status":              {"Status", "Statut"},
	"cli.sync.header_new_commits":         {"New commits", "Nouveaux commits"},
	"cli.sync.header_branches":            {"Branches", "Branches"},
	"cli.sync.header_duration":            {"Duration", "Durée"},
	"cli.sync.header_duration_ascii":      {"Duration", "Duree"},
	"cli.sync.header_detail":              {"Details", "Détail"},
	"cli.sync.header_detail_ascii":        {"Details", "Detail"},
	"cli.sync.sync_status_error":          {"error", "erreur"},
	"cli.sync.sync_status_locked":         {"skipped", "ignoré"},
	"cli.sync.sync_status_deferred":       {"deferred", "reporté"},
	"cli.sync.sync_status_deferred_ascii": {"deferred", "reporte"},
	"cli.sync.count_repo_one":             {"%d repository", "%d dépôt"},
	"cli.sync.count_repo_many":            {"%d repositories", "%d dépôts"},
	"cli.sync.count_new_commit_one":       {"%d new commit", "%d nouveau commit"},
	"cli.sync.count_new_commit_many":      {"%d new commits", "%d nouveaux commits"},
	"cli.sync.count_errors":               {"%d in error", "%d en erreur"},
	"cli.sync.count_locked":               {"%d already running", "%d déjà en cours"},
	"cli.sync.count_deferred_one":         {"%d deferred", "%d reporté"},
	"cli.sync.count_deferred_many":        {"%d deferred", "%d reportés"},
	"cli.sync.count_warning_one":          {"%d warning", "%d avertissement"},
	"cli.sync.count_warning_many":         {"%d warnings", "%d avertissements"},
	"cli.sync.summary_line": {
		"\nSync finished in %s: %s.\n",
		"\nSynchro terminée en %s : %s.\n",
	},
	"cli.sync.sync_source_git": {
		"Source: local Git clones (gh absent or signed out; \"gh auth login\" to use the GitHub API instead).\n",
		"Source : clones Git locaux (gh absent ou non connecté ; « gh auth login » pour passer par l'API GitHub).\n",
	},
	"cli.sync.done_errors_one": {
		`sync finished with 1 repository in error (details above, or "watchoo status")`,
		"synchro terminée avec 1 dépôt en erreur (détail ci-dessus, ou « watchoo status »)",
	},
	"cli.sync.done_errors_many": {
		`sync finished with %d repositories in error (details above, or "watchoo status")`,
		"synchro terminée avec %d dépôts en erreur (détail ci-dessus, ou « watchoo status »)",
	},
	// Détail d'un dépôt reporté : la raison vient du message écrit par le moteur (internal/sync),
	// les raisons connues sont traduites, une raison inconnue est affichée telle quelle.
	"cli.sync.deferred_detail": {
		"deferred: %s until %s",
		"reporté : %s jusqu'à %s",
	},
	"cli.sync.deferred_reason_low":       {"low quota", "quota bas"},
	"cli.sync.deferred_reason_exhausted": {"quota exhausted", "quota épuisé"},
	"cli.sync.warnings_join":             {"; ", " ; "},

	// Quota de l'API GitHub (sync.go, repris par status).
	"cli.sync.rate_limit": {
		"GitHub quota: %d / %d requests left",
		"Quota GitHub : %d / %d requêtes restantes",
	},
	"cli.sync.rate_limit_reset": {
		" (resets at %s)",
		" (remise à zéro à %s)",
	},

	// status (status.go) : aide, sous-titre de la carte, résumé texte.
	"cli.sync.status_short": {
		"Summarizes the workspace state: repositories, syncs, source, quota and gh account",
		"Résume l'état de l'espace : dépôts, synchros, source, quota et compte gh",
	},
	"cli.sync.status_long": {
		`Summarizes the workspace state: the gh account in use and its scopes, the sync source
(GitHub API with gh, or local Git clones without gh installed and signed in), the remaining
GitHub API quota (a free read, which consumes no quota) and the guard rail active when less
than 10% of the quota is left (syncs postponed until the reset), the number of repositories
per state (Active, Flemme, Dead, in error, never synced…), the last sync and the number of
tracked branches. The token is never displayed.

A gh problem (not installed, not signed in) is reported in the summary without failing the
command: syncs then go through the local Git clones.`,
		`Résume l'état de l'espace : compte gh utilisé et ses droits, source des synchros (API
GitHub avec gh, ou clones Git locaux sans gh installé et connecté), quota restant de l'API
GitHub (lecture gratuite, qui ne consomme pas de quota) et garde-fou actif s'il reste
moins de 10 % du quota (synchros reportées jusqu'à la remise à zéro), nombre de dépôts par état
(Actif, Flemme, Dead, en erreur, jamais synchronisés…), dernière synchro et nombre de
branches suivies. Le token n'est jamais affiché.

Un problème de gh (non installé, non connecté) est signalé dans le résumé sans faire
échouer la commande : les synchros passent alors par les clones Git locaux.`,
	},
	"cli.sync.status_subtitle": {
		`Status of workspace "%s"`,
		"État de l'espace « %s »",
	},
	"cli.sync.status_gh_error": {"gh account: %s", "Compte gh : %s"},
	"cli.sync.status_source_api": {
		"Source: GitHub API (gh signed in)",
		"Source : API GitHub (gh connecté)",
	},
	"cli.sync.status_source_git": {
		"Source: local Git clones (repos/ in the workspace), with this machine's SSH key or Git " +
			`credentials; "gh auth login" to use the GitHub API instead (pull requests, releases, stars)`,
		"Source : clones Git locaux (repos/ dans l'espace), avec la clé SSH ou les identifiants Git " +
			"de cette machine ; « gh auth login » pour passer par l'API GitHub (pull requests, releases, étoiles)",
	},
	"cli.sync.status_source_unknown": {
		"Source: unknown (%s)",
		"Source : inconnue (%s)",
	},
	"cli.sync.status_rl_na": {
		"not applicable: local Git clones, no quota",
		"sans objet : clones Git locaux, sans quota",
	},
	"cli.sync.status_rl_no_account": {
		"gh account unusable",
		"compte gh inutilisable",
	},
	"cli.sync.status_rate_limit_unknown": {
		"GitHub quota: unknown (%s)",
		"Quota GitHub : inconnu (%s)",
	},
	"cli.sync.status_quota_low_until": {
		"Low quota: syncs postponed until %s (less than 10%% of the quota left).",
		"Quota bas : synchros reportées jusqu'à %s (moins de 10 %% du quota restant).",
	},
	"cli.sync.status_quota_low": {
		"Low quota: syncs postponed (less than 10%% of the quota left).",
		"Quota bas : synchros reportées (moins de 10 %% du quota restant).",
	},
	"cli.sync.status_no_repos": {
		`No tracked repository. Add some with "watchoo repo add owner/repo" or ` +
			`"watchoo repo import file.csv".` + "\n",
		"Aucun dépôt suivi. Ajoutez-en avec « watchoo repo add owner/repo » ou " +
			"« watchoo repo import fichier.csv ».\n",
	},
	"cli.sync.status_repos_count":  {"Repositories: %d", "Dépôts : %d"},
	"cli.sync.status_lbl_active":   {"up to date (Active)", "à jour (Actif)"},
	"cli.sync.status_lbl_flemme":   {"Flemme", "Flemme"},
	"cli.sync.status_lbl_dead":     {"Dead", "Dead"},
	"cli.sync.status_lbl_error":    {"in error", "en erreur"},
	"cli.sync.status_lbl_never":    {"never synced", "jamais synchronisés"},
	"cli.sync.status_lbl_archived": {"archived", "archivés"},
	"cli.sync.status_lbl_empty":    {"empty", "vides"},
	"cli.sync.status_lbl_stopped":  {"tracking stopped", "suivi arrêté"},
	"cli.sync.status_last_sync_never": {
		`Last sync: never (run "watchoo sync")`,
		"Dernière synchro : jamais (lancez « watchoo sync »)",
	},
	"cli.sync.status_last_sync": {
		"Last sync: %s (%s)",
		"Dernière synchro : %s (%s)",
	},
	"cli.sync.status_tracked_branches": {"Tracked branches: %d", "Branches suivies : %d"},

	// Options communes à plusieurs commandes de ce groupe.
	"cli.sync.flag_repo": {
		"limit to the `repository` (owner/repo)",
		"limite au `dépôt` owner/repo",
	},
	"cli.sync.flag_author": {
		"limit to one `author` (exact login, name or e-mail)",
		"limite à un `auteur` (login, nom ou e-mail exact)",
	},
	"cli.sync.flag_branch": {
		"limit to a `branch`",
		"limite à une `branche`",
	},
	"cli.sync.flag_since": {
		"limit to a recent `period` (e.g. 7d, 12h, 30m)",
		"limite à une `période` récente (ex. 7d, 12h, 30m)",
	},
	"cli.sync.flag_since_contrib": {
		"limit to a recent `period` (e.g. 30d, 7d, 12h)",
		"limite à une `période` récente (ex. 30d, 7d, 12h)",
	},
	"cli.sync.flag_interval": {
		"automatic sync `interval` (e.g. 30m, 1h; 0 = manual only); default: sync.interval",
		"`intervalle` de synchro automatique (ex. 30m, 1h ; 0 = manuelle seule) ; défaut : sync.interval",
	},
	"cli.sync.flag_addr": {
		"listening `address` host:port (e.g. 127.0.0.1:9000); default: web.addr",
		"`adresse` d'écoute hôte:port (ex. 127.0.0.1:9000) ; défaut : web.addr",
	},

	// log (log.go) : aide, filtres, message « aucun commit », en-têtes.
	"cli.sync.log_short": {
		"Shows the latest commits, filtered by repository, author, branch or period",
		"Affiche les derniers commits, filtrés par dépôt, auteur, branche ou période",
	},
	"cli.sync.log_long": {
		`Shows the commits recorded by the syncs, from the most recent to the oldest, each one
only once even if it is on several branches.

Filters:
  --repo     one tracked repository (owner/repo or URL)
  --author   GitHub login, name or exact e-mail of the author (case-insensitive)
  --branch   commits seen on this branch during the syncs: those it shares with the
             reference branch (often main) are attached to it only if they were seen on it
  --since    relative period: 7d (days), 12h (hours), 30m (minutes)
  -n         maximum number of commits (20 by default, 0 = all)`,
		`Affiche les derniers commits enregistrés par les synchros, du plus récent au plus
ancien, chacun une seule fois même s'il est sur plusieurs branches.

Filtres :
  --repo     un dépôt suivi (owner/repo ou URL)
  --author   login GitHub, nom ou e-mail exact de l'auteur (sans tenir compte de la casse)
  --branch   commits vus sur cette branche lors des synchros : ceux qu'elle partage avec la
             branche de référence (souvent main) n'y sont rattachés que s'ils y ont été vus
  --since    période relative : 7d (jours), 12h (heures), 30m (minutes)
  -n         nombre maximum de commits (20 par défaut, 0 = tous)`,
	},
	"cli.sync.flag_log_limit": {
		"maximum `number` of commits, 0 = all",
		"`nombre` maximum de commits, 0 = tous",
	},
	"cli.sync.log_bad_limit": {
		"-n %d is invalid: expected a positive number (0 = all commits)",
		"-n %d invalide : attendu un nombre positif (0 = tous les commits)",
	},
	"cli.sync.log_no_match": {
		"No commit matches%s.\n",
		"Aucun commit ne correspond%s.\n",
	},
	"cli.sync.filter_repo":   {"repository %s", "dépôt %s"},
	"cli.sync.filter_author": {"author %s", "auteur %s"},
	"cli.sync.filter_branch": {"branch %s", "branche %s"},
	"cli.sync.filter_since":  {"since %s", "depuis %s"},
	"cli.sync.filter_empty_db": {
		`: the database is empty (run "watchoo sync")`,
		" : la base est vide (lancez « watchoo sync »)",
	},
	"cli.sync.log_filters":   {" the filters (%s)", " aux filtres (%s)"},
	"cli.sync.header_author": {"Author", "Auteur"},

	// contributors (contributors.go) : aide, options, classement, tableau hebdomadaire.
	"cli.sync.contributors_short": {
		"Ranks contributors by number of commits",
		"Classe les contributeurs par nombre de commits",
	},
	"cli.sync.contributors_long": {
		`Ranks commit authors by number of commits, across all repositories or for a single
one, over the whole known period or a recent one.

Lines added and removed are counted in each repository's local Git clone, for all its
commits. They add up the commits whose statistics are known; "-": none is known yet
(local Git clone unavailable).

With --weeks N, the period is that of the last N weeks (Monday to Sunday, in the display
time zone, the current week included) and each contributor has their commits week by
week. With --repo, only the commits of the repository count, once each whatever their
branch.`,
		`Classe les auteurs de commits par nombre de commits, tous dépôts confondus ou pour un
seul dépôt, sur toute la période connue ou une période récente.

Les lignes ajoutées et supprimées sont comptées dans le clone Git local de chaque
dépôt, pour tous ses commits. Elles additionnent les commits dont les statistiques
sont connues ; « - » : aucune n'est encore connue (clone Git local indisponible).

Avec --weeks N, la période est celle des N dernières semaines (du lundi au
dimanche, dans le fuseau d'affichage, la semaine en cours comprise) et chaque
contributeur a ses commits semaine par semaine. Avec --repo, seuls les commits du
dépôt comptent, une fois chacun quelle que soit sa branche.`,
	},
	"cli.sync.flag_weeks": {
		"commits per week over the last `N` weeks (1 to 52)",
		"commits par semaine sur les `N` dernières semaines (1 à 52)",
	},
	"cli.sync.weeks_invalid": {
		"--weeks %d is invalid: expected a number of weeks between 1 and %d",
		"--weeks %d invalide : attendu un nombre de semaines entre 1 et %d",
	},
	"cli.sync.weeks_since_conflict": {
		"--since and --weeks cannot be used together: --weeks already sets the period (the last %d weeks)",
		"--since et --weeks ne vont pas ensemble : --weeks fixe déjà la période (les %d dernières semaines)",
	},
	"cli.sync.contributors_none":       {"No contributor", "Aucun contributeur"},
	"cli.sync.contributors_none_since": {" since %s", " depuis %s"},
	"cli.sync.contributors_none_weeks": {" over the last %d weeks", " sur les %d dernières semaines"},
	"cli.sync.contributors_none_hint": {
		` (contributors come from synced commits: run "watchoo sync").` + "\n",
		" (les contributeurs viennent des commits synchronisés : « watchoo sync »).\n",
	},
	"cli.sync.header_contributor": {"Contributor", "Contributeur"},
	"cli.sync.header_additions":   {"Additions", "Ajouts"},
	"cli.sync.header_deletions":   {"Deletions", "Suppressions"},
	"cli.sync.header_repos":       {"Repositories", "Dépôts"},
	"cli.sync.header_repos_ascii": {"Repositories", "Depots"},
	"cli.sync.header_last_commit": {"Last commit", "Dernier commit"},
	"cli.sync.weekly_note": {
		"Commits per week (Monday to Sunday, columns dated by Monday, display time zone); " +
			"the last week is in progress.\n",
		"Commits par semaine (du lundi au dimanche, colonnes datées du lundi, fuseau d'affichage) ; " +
			"la dernière semaine est en cours.\n",
	},
	"cli.sync.weekly_note_ascii": {
		"Commits per week (Monday to Sunday, columns dated by Monday, display time zone); " +
			"the last week is in progress.\n",
		"Commits par semaine (du lundi au dimanche, colonnes datees du lundi, fuseau d'affichage) ; " +
			"la derniere semaine est en cours.\n",
	},

	// serve (serve.go) : aide, bandeau, arrêt.
	"cli.sync.serve_short": {
		"Starts the local web interface and the periodic sync",
		"Lance l'interface web locale et la synchro périodique",
	},
	"cli.sync.serve_long": {
		`Starts the workspace's web interface (dashboard, repository details, contributors,
settings) and syncs the repositories in the background while it runs.

The interface listens on web.addr (127.0.0.1:8080 by default); --addr overrides this
setting. Anywhere but on the loopback (127.0.0.1, ::1, localhost), it refuses to start
without authentication: set web.auth.username and give the password in the
WATCHOO_WEB_PASSWORD variable (never in a file), or declare an authenticating reverse
proxy with web.trusted_proxy: true and its public names in web.allowed_hosts.

The automatic sync follows sync.interval (30 min by default) if sync.auto is true;
--interval overrides this setting for the process: 0 for manual sync only, otherwise
15 min at least. At startup, a sync is started only if the last one is older than one
interval. Ctrl+C stops the server cleanly.`,
		`Lance l'interface web de l'espace (tableau de bord, détail des dépôts, contributeurs,
réglages) et synchronise les dépôts en arrière-plan tant qu'elle tourne.

L'interface écoute sur web.addr (127.0.0.1:8080 par défaut) ; --addr remplace ce
réglage. Ailleurs que sur la boucle locale (127.0.0.1, ::1, localhost), elle refuse de
démarrer sans authentification : réglez web.auth.username et donnez le mot de passe dans
la variable WATCHOO_WEB_PASSWORD (jamais dans un fichier), ou déclarez un reverse proxy
authentifiant avec web.trusted_proxy: true et ses noms publics dans web.allowed_hosts.

La synchro automatique suit sync.interval (30 min par défaut) si sync.auto est vrai ;
--interval remplace ce réglage le temps du processus : 0 pour la synchro manuelle seule,
sinon 15 min au minimum. Au démarrage, une synchro n'est lancée que si la dernière date
de plus d'un intervalle. Ctrl+C arrête le serveur proprement.`,
	},
	"cli.sync.serve_no_json": {
		`"watchoo serve" has no JSON output: use the other commands with --json`,
		"« watchoo serve » n'a pas de sortie JSON : utilisez les autres commandes avec --json",
	},
	"cli.sync.serve_addr_in_use": {
		`address %s is already in use (another "watchoo serve"?): choose another port with web.addr or --addr`,
		"l'adresse %s est déjà utilisée (un autre « watchoo serve » ?) : choisissez un autre port avec web.addr ou --addr",
	},
	"cli.sync.serve_listen_failed": {
		"cannot listen on %s",
		"écoute sur %s impossible",
	},
	"cli.sync.serve_subtitle": {
		`Web interface of workspace "%s"`,
		"Interface web de l'espace « %s »",
	},
	"cli.sync.serve_tip": {
		"Tip: Ctrl+C stops the server (the running sync is cancelled cleanly).",
		"Astuce : Ctrl+C arrête le serveur (la synchro en cours est annulée proprement).",
	},
	"cli.sync.serve_web_line":  {"Web interface: %s", "Interface web : %s"},
	"cli.sync.serve_sync_line": {"Automatic sync: %s", "Synchro automatique : %s"},
	"cli.sync.serve_ctrl_c": {
		"Ctrl+C to stop.\n",
		"Ctrl+C pour arrêter.\n",
	},
	"cli.sync.serve_stopped": {
		"Web interface stopped.\n",
		"Interface web arrêtée.\n",
	},
	"cli.sync.serve_banner_err": {
		"printing the web interface address",
		"affichage de l'adresse de l'interface",
	},
	"cli.sync.interval_off": {
		"disabled (manual sync from the web interface)",
		"désactivée (synchro manuelle depuis l'interface)",
	},
	"cli.sync.interval_hours":   {"every %d h", "toutes les %d h"},
	"cli.sync.interval_minutes": {"every %d min", "toutes les %d min"},
	"cli.sync.interval_every":   {"every %s", "toutes les %s"},

	// update (update.go) : avis de nouvelle version, vérification, installation.
	"cli.sync.update_short": {
		"Updates Watchoo to the latest published version",
		"Met Watchoo à jour avec la dernière version publiée",
	},
	"cli.sync.update_long": {
		`Updates Watchoo to the latest version published on GitHub: shows the installed and
latest versions, asks for confirmation, downloads the release archive for this system,
verifies its SHA-256 checksum and replaces the binary in one piece.

--check only checks, without installing anything (--json possible). Only the public
pages of Watchoo's releases are read: no token, no GitHub API.

Refused in the Docker image (change the image tag), for a version built from the
sources (go install …@latest, or git pull && make build) and if the binary's directory
is not writable.

A running "watchoo serve" keeps the old version until it is restarted; each workspace's
database is upgraded at the next command.`,
		`Met Watchoo à jour avec la dernière version publiée sur GitHub : affiche la version
installée et la dernière, demande confirmation, télécharge l'archive de la release pour
ce système, vérifie sa somme SHA-256 et remplace le binaire d'un bloc.

--check vérifie seulement, sans rien installer (--json possible). Seules les pages
publiques des releases de Watchoo sont lues : ni token, ni API GitHub.

Refusé dans l'image Docker (changez le tag de l'image), pour une version compilée
depuis les sources (go install …@latest, ou git pull && make build) et si le
répertoire du binaire n'est pas accessible en écriture.

Un « watchoo serve » déjà lancé garde l'ancienne version jusqu'à son redémarrage ; la
base de chaque espace est mise à niveau à la prochaine commande.`,
	},
	"cli.sync.flag_check": {
		"only check whether a new version is published, without installing anything",
		"vérifie seulement si une nouvelle version est publiée, sans rien installer",
	},
	"cli.sync.flag_yes": {
		"install without asking (required outside a terminal)",
		"installe sans poser de question (obligatoire hors terminal)",
	},
	"cli.sync.update_notice": {
		"New Watchoo version available: %s → %s. %s",
		"Nouvelle version de Watchoo disponible : %s → %s. %s",
	},
	"cli.sync.update_how": {
		`Update with "watchoo update".`,
		"Mettez-la à jour avec « watchoo update ».",
	},
	"cli.sync.update_docker_how": {
		"Change the Docker image tag to update it.",
		"Changez le tag de l'image Docker pour la mettre à jour.",
	},
	"cli.sync.update_check_failed": {
		"cannot read the latest Watchoo version",
		"impossible de lire la dernière version de Watchoo",
	},
	"cli.sync.update_check_versions": {
		"Installed version: %s\nLatest version: %s\n",
		"Version installée : %s\nDernière version : %s\n",
	},
	"cli.sync.update_check_available": {
		"New version available: %s\n",
		"Nouvelle version disponible : %s\n",
	},
	"cli.sync.update_check_docker": {
		"Change the Docker image tag to update it.\n",
		"Changez le tag de l'image Docker pour la mettre à jour.\n",
	},
	"cli.sync.update_check_how": {
		"Update with \"watchoo update\".\n",
		"Mettez-la à jour avec « watchoo update ».\n",
	},
	"cli.sync.update_check_dev": {
		"Development version (built from the sources): cannot compare versions.\n",
		"Version de développement (compilée depuis les sources) : comparaison impossible.\n",
	},
	"cli.sync.update_check_uptodate": {
		"Watchoo is up to date.\n",
		"Watchoo est à jour.\n",
	},
	"cli.sync.update_print_check_err": {
		"printing the check",
		"affichage de la vérification",
	},
	"cli.sync.update_no_json": {
		`"watchoo update" has JSON output only with --check: use "watchoo update --check --json"`,
		"« watchoo update » n'a de sortie JSON qu'avec --check : utilisez « watchoo update --check --json »",
	},
	"cli.sync.update_uptodate": {
		"Watchoo %s is up to date (latest published version: %s).\n",
		"Watchoo %s est à jour (dernière version publiée : %s).\n",
	},
	"cli.sync.update_plan": {
		"Updating Watchoo: %s → %s\nRelease notes: %s\nChangelog: %s\nBinary: %s\n",
		"Mise à jour de Watchoo : %s → %s\nNotes de version : %s\nJournal des modifications : %s\nBinaire : %s\n",
	},
	"cli.sync.update_print_err": {
		"printing the update",
		"affichage de la mise à jour",
	},
	"cli.sync.update_confirm": {
		"Install Watchoo %s?",
		"Installer Watchoo %s ?",
	},
	"cli.sync.update_cancelled": {
		"update cancelled: nothing was modified",
		"mise à jour annulée : rien n'a été modifié",
	},
	"cli.sync.update_downloading": {
		"Downloading Watchoo %s (%s/%s)…\n",
		"Téléchargement de Watchoo %s (%s/%s)…\n",
	},
	"cli.sync.update_failed": {
		"update failed (binary unchanged)",
		"mise à jour impossible (binaire inchangé)",
	},
	"cli.sync.update_done": {
		"SHA-256 checksum verified. Watchoo %s installed in %s.\n" +
			`A running "watchoo serve" keeps the old version until it is restarted; each` + "\n" +
			"workspace's database is upgraded (migrations) at the next command.\n",
		"Somme SHA-256 vérifiée. Watchoo %s installé dans %s.\n" +
			"Un « watchoo serve » déjà lancé garde l'ancienne version jusqu'à son redémarrage ; la base de chaque\n" +
			"espace est mise à niveau (migrations) à la prochaine commande.\n",
	},
	"cli.sync.update_err_docker": {
		`"watchoo update" is not used in the Docker image: change the image tag ` +
			"(ghcr.io/studoo-app/watchoo:<version>) then recreate the container, or, with compose, " +
			`change the tag in docker-compose.yml then "docker compose pull && docker compose up -d"`,
		"« watchoo update » ne s'utilise pas dans l'image Docker : changez le tag " +
			"de l'image (ghcr.io/studoo-app/watchoo:<version>) puis recréez le conteneur, ou, avec compose, " +
			"changez le tag dans docker-compose.yml puis « docker compose pull && docker compose up -d »",
	},
	"cli.sync.update_err_devbuild": {
		"version %s was built from the sources: " +
			`"watchoo update" does not replace it; use "go install github.com/studoo-app/watchoo/cmd/watchoo@latest", ` +
			`or "git pull && make build" in the clone`,
		"la version %s a été compilée depuis les sources : " +
			"« watchoo update » ne la remplace pas ; utilisez « go install github.com/studoo-app/watchoo/cmd/watchoo@latest », " +
			"ou « git pull && make build » dans le clone",
	},
	"cli.sync.update_err_not_writable": {
		"the binary directory, %s, is not writable: check its permissions, run again with sudo, " +
			`or reinstall Watchoo in a directory of yours with the install script ` +
			`(WATCHOO_INSTALL_DIR, see the README, "Updating")`,
		"le répertoire du binaire, %s, n'est pas accessible en écriture : " +
			"vérifiez ses droits, relancez avec sudo, ou réinstallez Watchoo dans un répertoire à vous avec le script " +
			"d'installation (WATCHOO_INSTALL_DIR, voir le README, « Mise à jour »)",
	},

	// prs (prs.go) : aide, filtres, états, en-têtes.
	"cli.sync.prs_short": {
		"Shows the pull requests of the tracked repositories",
		"Affiche les pull requests des dépôts suivis",
	},
	"cli.sync.prs_long": {
		`Shows the pull requests recorded by the syncs, from the most recently updated to the
oldest: update date, repository, number, state, author and title.

Filters:
  --repo     one tracked repository (owner/repo or URL)
  --state    open (including drafts), merged, closed (closed without merge) or all
             (all, by default)
  -n         maximum number of pull requests (20 by default, 0 = all)`,
		`Affiche les pull requests enregistrées par les synchros, de la plus récemment
modifiée à la plus ancienne : date de modification, dépôt, numéro, état, auteur et
titre.

Filtres :
  --repo     un dépôt suivi (owner/repo ou URL)
  --state    open (ouvertes, brouillons compris), merged (mergées), closed (fermées
             sans merge) ou all (toutes, par défaut)
  -n         nombre maximum de pull requests (20 par défaut, 0 = toutes)`,
	},
	"cli.sync.flag_state": {
		"limit to a `state`: open, merged, closed or all",
		"limite à un `état` : open, merged, closed ou all",
	},
	"cli.sync.flag_prs_limit": {
		"maximum `number` of pull requests, 0 = all",
		"`nombre` maximum de pull requests, 0 = toutes",
	},
	"cli.sync.prs_bad_state": {
		"--state %q is invalid: expected open, merged, closed or all",
		"--state %q invalide : attendu open, merged, closed ou all",
	},
	"cli.sync.prs_bad_limit": {
		"-n %d is invalid: expected a positive number (0 = all pull requests)",
		"-n %d invalide : attendu un nombre positif (0 = toutes les pull requests)",
	},
	"cli.sync.prs_none":         {"No pull request%s.\n", "Aucune pull request%s.\n"},
	"cli.sync.prs_filter_state": {"state %s", "état %s"},
	"cli.sync.filter_in_db": {
		` in the database (run "watchoo sync")`,
		" en base (lancez « watchoo sync »)",
	},
	"cli.sync.prs_filters":             {" for these filters (%s)", " pour ces filtres (%s)"},
	"cli.sync.header_updated":          {"Updated", "Modifiée"},
	"cli.sync.header_updated_ascii":    {"Updated", "Modifiee"},
	"cli.sync.header_state":            {"State", "État"},
	"cli.sync.header_state_ascii":      {"State", "Etat"},
	"cli.sync.header_title":            {"Title", "Titre"},
	"cli.sync.prs_status_merged":       {"merged", "mergée"},
	"cli.sync.prs_status_merged_ascii": {"merged", "mergee"},
	"cli.sync.prs_status_closed":       {"closed", "fermée"},
	"cli.sync.prs_status_closed_ascii": {"closed", "fermee"},
	"cli.sync.prs_status_draft":        {"draft", "brouillon"},
	"cli.sync.prs_status_open":         {"open", "ouverte"},

	// releases (releases.go) : aide, listes, états, en-têtes.
	"cli.sync.releases_short": {
		"Shows the releases and tags of the tracked repositories",
		"Affiche les releases et les tags des dépôts suivis",
	},
	"cli.sync.releases_long": {
		`Shows the releases recorded by the syncs, from the most recent to the oldest
(publication date, or creation date for a draft), then the tags, from the most recent
commit to the oldest (date of the targeted commit if it is in the database, otherwise -).

Release drafts are only visible if the gh account has write access to the repository
(GitHub's rule).

Options:
  --repo     one tracked repository (owner/repo or URL)
  -n         maximum number of releases, and of tags (20 by default, 0 = all)`,
		`Affiche les releases enregistrées par les synchros, de la plus récente à la plus
ancienne (date de publication, ou de création pour un brouillon), puis les tags, du
commit le plus récent au plus ancien (date du commit visé s'il est en base, sinon -).

Les brouillons de release ne sont visibles que si le compte gh a le droit d'écriture
sur le dépôt (règle de GitHub).

Options :
  --repo     un dépôt suivi (owner/repo ou URL)
  -n         nombre maximum de releases, et de tags (20 par défaut, 0 = tout)`,
	},
	"cli.sync.flag_releases_limit": {
		"maximum `number` of releases and tags, 0 = all",
		"`nombre` maximum de releases et de tags, 0 = tout",
	},
	"cli.sync.releases_bad_limit": {
		"-n %d is invalid: expected a positive number (0 = all)",
		"-n %d invalide : attendu un nombre positif (0 = tout)",
	},
	"cli.sync.releases_filter_repo":            {" for the repository %s", " pour le dépôt %s"},
	"cli.sync.releases_none":                   {"No release%s.\n", "Aucune release%s.\n"},
	"cli.sync.tags_none":                       {"No tag%s.\n", "Aucun tag%s.\n"},
	"cli.sync.header_commit_date":              {"Commit date", "Date du commit"},
	"cli.sync.release_status_prerelease":       {"pre-release", "pré-release"},
	"cli.sync.release_status_prerelease_ascii": {"pre-release", "pre-release"},
	"cli.sync.release_status_published":        {"published", "publiée"},
	"cli.sync.release_status_published_ascii":  {"published", "publiee"},

	// tui (tui.go) : aide et refus de --json ; les textes du mode interactif sont dans internal/tui.
	"cli.sync.tui_short": {
		"Opens the interactive mode in the terminal",
		"Ouvre le mode interactif dans le terminal",
	},
	"cli.sync.tui_long": {
		`Opens the interactive mode: repositories (state, last commit, author), commits and
contributors are browsed with the keyboard, with a filter and a repository detail.

Main keys: tab switches views, ↑ ↓ move the selection, enter opens the detail, /
filters, s syncs everything, S the selected repository, a adds a repository to track
(Repositories view, like "watchoo repo add" without option), ? shows the help, q quits.

While it is open, the interactive mode syncs the repositories automatically every
sync.interval (30 min by default) if sync.auto is true. --interval overrides this
setting for the session: 0 for manual sync only, otherwise 15 min at least. It must be
run in a terminal.`,
		`Ouvre le mode interactif : dépôts (état, dernier commit, auteur), commits et
contributeurs se parcourent au clavier, avec filtre et détail d'un dépôt.

Touches principales : tab change de vue, ↑ ↓ déplacent la sélection, entrée ouvre le
détail, / filtre, s synchronise tout, S le dépôt sélectionné, a ajoute un dépôt à suivre
(vue Dépôts, comme « watchoo repo add » sans option), ? affiche l'aide, q quitte.

Tant qu'il est ouvert, le mode interactif synchronise automatiquement les dépôts toutes
les sync.interval (30 min par défaut) si sync.auto est vrai. --interval remplace ce
réglage le temps de la session : 0 pour la synchro manuelle seule, sinon 15 min au
minimum. Il doit être lancé dans un terminal.`,
	},
	"cli.sync.tui_no_json": {
		`"watchoo tui" has no JSON output: use the other commands with --json`,
		"« watchoo tui » n'a pas de sortie JSON : utilisez les autres commandes avec --json",
	},
}

func init() {
	register(cliSyncTexts)
}
