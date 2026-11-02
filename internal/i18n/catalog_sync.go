// Textes du paquet internal/sync : rapports de synchro (échecs, verrou, reports, avertissements),
// messages consignés dans sync_runs.error (dans la langue active lors de la synchro), erreurs du
// moteur et du coordinateur. Préfixe des clés : « sync. ». Les messages ne citent ni token, ni message
// de commit, ni nom, ni e-mail.
package i18n

// syncTexts regroupe les textes de internal/sync, enregistrés dans le catalogue par le init()
// ci-dessous.
var syncTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : », l'anglais non.
	"sync.err.sep": {": ", " : "},

	// Rapports par dépôt (run.go).
	"sync.locked":   {"already being synced", "déjà en cours de synchronisation"},
	"sync.canceled": {"sync canceled", "synchro annulée"},
	"sync.deadline": {"sync interrupted: deadline exceeded", "synchro interrompue : délai dépassé"},
	// Raisons d'un report (RepoReport.DeferReason) et message du report.
	"sync.reason.low_quota":       {"low quota", "quota bas"},
	"sync.reason.quota_exhausted": {"quota exhausted", "quota épuisé"},
	"sync.deferred":               {"deferred: %s", "reporté : %s"},
	"sync.deferred_until":         {"deferred: %s until %s UTC", "reporté : %s jusqu'à %s UTC"},
	// Remplace le token dans un message (défense supplémentaire, le Provider ne le cite jamais).
	"sync.token_redacted": {"[token redacted]", "[token masqué]"},

	// Source de la synchro (engine.go).
	"sync.source.api": {"GitHub API", "API GitHub"},
	"sync.source.git": {"local Git clones", "clones Git locaux"},

	// Erreurs du moteur (engine.go).
	"sync.engine.no_store": {"sync engine: database not set", "moteur de synchro : base non renseignée"},
	"sync.engine.no_auth":  {"sync engine: gh access not set", "moteur de synchro : accès à gh non renseigné"},
	"sync.engine.sync":     {"sync", "synchronisation"},
	"sync.engine.sync_id":  {"sync of repository %d", "synchronisation du dépôt %d"},
	"sync.engine.sync_repo": {
		"sync of repository %s",
		"synchronisation du dépôt %s",
	},
	"sync.engine.rate_limit": {"reading the quota", "lecture du quota"},
	"sync.engine.token":      {"reading the GitHub token", "lecture du token GitHub"},
	"sync.engine.no_repos_dir": {
		"directory of the local Git clones not set",
		"répertoire des clones Git locaux non renseigné",
	},
	"sync.engine.git_source": {
		"local Git repositories (gh missing or not signed in)",
		"dépôts Git locaux (gh absent ou non connecté)",
	},
	"sync.engine.connect": {"connecting to GitHub", "connexion à GitHub"},

	// Erreurs du coordinateur (coordinator.go) ; les trois premières sont des erreurs sentinelles.
	"sync.coord.running":     {"sync already running", "synchro déjà en cours"},
	"sync.coord.closed":      {"shutting down: no new sync", "arrêt en cours : aucune nouvelle synchro"},
	"sync.coord.not_started": {"sync coordinator not started", "coordinateur de synchro non démarré"},
	"sync.coord.no_syncer": {
		"sync coordinator: engine not set",
		"coordinateur de synchro : moteur non renseigné",
	},
	"sync.coord.interval_negative": {
		"interval %s is negative (0 for manual sync only, otherwise at least %d min)",
		"intervalle %s négatif (0 pour la synchro manuelle seule, sinon au moins %d min)",
	},
	"sync.coord.interval_too_short": {
		"interval %s is below the minimum of %d min (0 for manual sync only)",
		"intervalle %s inférieur au minimum de %d min (0 pour la synchro manuelle seule)",
	},
	"sync.coord.bad_repo_id": {
		"sync of repository %d: invalid identifier",
		"synchro du dépôt %d : identifiant invalide",
	},
	"sync.coord.close_timeout": {
		"the running sync did not stop in time",
		"la synchro en cours ne s'est pas arrêtée à temps",
	},

	// Intervalle de la synchro automatique (branches.go).
	"sync.interval.negative": {
		"%s: %s is negative (0 for manual sync only, otherwise at least %d min)",
		"%s : %s est négatif (0 pour la synchro manuelle seule, sinon au moins %d min)",
	},
	"sync.interval.too_short": {
		"%s: %s is below the minimum of %d min (0 for manual sync only)",
		"%s : %s est inférieur au minimum de %d min (0 pour la synchro manuelle seule)",
	},

	// Ajout et retrait de dépôts (add.go, remove.go, thresholds.go).
	"sync.add.repo":         {"adding repository %s", "ajout du dépôt %s"},
	"sync.add.repos":        {"adding repositories", "ajout de dépôts"},
	"sync.remove.clone":     {"repository %s removed, but ", "dépôt %s retiré, mais "},
	"sync.thresholds.error": {"activity thresholds of the repository", "seuils d'activité du dépôt"},

	// Statistiques de lignes (clonestats.go, repo.go).
	"sync.stats.error":     {"commit statistics", "statistiques des commits"},
	"sync.languages.error": {"reading the languages", "lecture des langages"},
	"sync.languages.not_computed": {
		"languages not computed: %s",
		"langages non calculés : %s",
	},
	"sync.stats.not_computed": {
		"line statistics not computed: %s",
		"statistiques de lignes non calculées : %s",
	},
	"sync.stats.clone_unavailable": {
		"local Git clone unavailable, line statistics read through the API (limited): %s",
		"clone Git local indisponible, statistiques de lignes lues par l'API (limitées) : %s",
	},

	// Pull requests, releases et tags non synchronisés (extras.go) : « <listes> non synchronisées :
	// <raison> » ; le français accorde au masculin dès que les tags en sont.
	"sync.extras.not_synced":      {"%s not synced: %s", "%s non synchronisées : %s"},
	"sync.extras.not_synced_masc": {"%s not synced: %s", "%s non synchronisés : %s"},
	"sync.extras.and":             {" and ", " et "},
}

func init() { register(syncTexts) }
