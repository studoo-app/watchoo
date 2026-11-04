// Textes du paquet internal/web : pages, fragments, messages des handlers, pages d'erreur, textes des
// graphiques (static/app.js) et messages de démarrage de « watchoo serve ». Préfixe des clés : « web. ».
//
// Conventions :
//   - les clés « web.n.<nom>.one » / « web.n.<nom>.other » sont les deux formes d'un nombre (fonction
//     de template tn) : « %d commit » / « %d commits » ; le français met 0 et 1 au singulier, l'anglais
//     1 seulement ;
//   - « web.fmt.* » sont des formats de date Go (time.Format), sans verbe fmt ;
//   - les textes des pages sont rendus par html/template, qui les échappe : pas de HTML ici.
package i18n

// webTexts regroupe les textes de internal/web, enregistrés dans le catalogue par le init()
// ci-dessous.
var webTexts = map[string]entry{
	// Ponctuation et éléments communs.
	"web.colon":     {": ", " : "},
	"web.semicolon": {"; ", " ; "},
	"web.by":        {"by", "par"},
	"web.cancel":    {"Cancel", "Annuler"},
	"web.prev":      {"« Previous", "« Précédent"},
	"web.next":      {"Next »", "Suivant »"},
	"web.page_of":   {"page %d of %d", "page %d sur %d"},
	"web.percent":   {"%d%%", "%d %%"},
	"web.breadcrumb": {
		"Breadcrumb",
		"Fil d'Ariane",
	},

	// Formats de date (time.Format), dans le fuseau d'affichage de l'espace.
	"web.fmt.date":     {"Jan 2, 2006", "02/01/2006"},
	"web.fmt.datetime": {"Jan 2, 2006 15:04", "02/01/2006 à 15:04"},
	"web.fmt.daymonth": {"Jan 2", "02/01"},

	// Textes de static/app.js (graphiques), transmis en JSON par le layout.
	"web.js.locale":  {"en-US", "fr-FR"},
	"web.js.commit":  {"{n} commit", "{n} commit"},
	"web.js.commits": {"{n} commits", "{n} commits"},
	"web.js.week":    {"Week of {start} to {end}", "Semaine du {start} au {end}"},
	"web.js.total":   {"Total: {n}", "Total : {n}"},

	// Nombres (fonction de template tn).
	"web.n.commit.one":                {"%d commit", "%d commit"},
	"web.n.commit.other":              {"%d commits", "%d commits"},
	"web.n.commit_not_analyzed.one":   {"%d commit not analyzed yet.", "%d commit pas encore analysé."},
	"web.n.commit_not_analyzed.other": {"%d commits not analyzed yet.", "%d commits pas encore analysés."},
	"web.n.line_added.one":            {"%d line", "%d ligne"},
	"web.n.line_added.other":          {"%d lines", "%d lignes"},
	"web.n.commit_db.one":             {"%d commit in the database", "%d commit en base"},
	"web.n.commit_db.other":           {"%d commits in the database", "%d commits en base"},
	"web.n.new_commit.one":            {"%d new commit", "%d nouveau commit"},
	"web.n.new_commit.other":          {"%d new commits", "%d nouveaux commits"},
	"web.n.uncounted.one":             {"%d commit not counted", "%d commit non compté"},
	"web.n.uncounted.other":           {"%d commits not counted", "%d commits non comptés"},
	"web.n.repo.one":                  {"%d repository", "%d dépôt"},
	"web.n.repo.other":                {"%d repositories", "%d dépôts"},
	"web.n.contributor.one":           {"%d contributor", "%d contributeur"},
	"web.n.contributor.other":         {"%d contributors", "%d contributeurs"},
	"web.n.day.one":                   {"%d day", "%d jour"},
	"web.n.day.other":                 {"%d days", "%d jours"},
	"web.n.identity.one":              {"%d known Git identity", "%d identité Git connue"},
	"web.n.identity.other":            {"%d known Git identities", "%d identités Git connues"},
	"web.n.tracked_branch.one":        {"%d tracked branch", "%d branche suivie"},
	"web.n.tracked_branch.other":      {"%d tracked branches", "%d branches suivies"},
	"web.n.open_pr.one":               {"%d open PR", "%d PR ouverte"},
	"web.n.open_pr.other":             {"%d open PRs", "%d PR ouvertes"},
	"web.n.pr_open.one":               {"%d open", "%d ouverte"},
	"web.n.pr_open.other":             {"%d open", "%d ouvertes"},
	"web.n.pr_merged.one":             {"%d merged", "%d mergée"},
	"web.n.pr_merged.other":           {"%d merged", "%d mergées"},
	"web.n.pr_closed.one":             {"%d closed without merge", "%d fermée sans merge"},
	"web.n.pr_closed.other":           {"%d closed without merge", "%d fermées sans merge"},
	"web.n.star.one":                  {"%d star", "%d étoile"},
	"web.n.star.other":                {"%d stars", "%d étoiles"},
	"web.n.open_issue.one":            {"%d open issue", "%d issue ouverte"},
	"web.n.open_issue.other":          {"%d open issues", "%d issues ouvertes"},
	"web.n.warning.one":               {"%d warning", "%d avertissement"},
	"web.n.warning.other":             {"%d warnings", "%d avertissements"},
	"web.n.repo_error.one":            {"%d repository in error", "%d dépôt en erreur"},
	"web.n.repo_error.other":          {"%d repositories in error", "%d dépôts en erreur"},
	"web.n.repo_deferred.one":         {"%d repository deferred", "%d dépôt reporté"},
	"web.n.repo_deferred.other":       {"%d repositories deferred", "%d dépôts reportés"},
	"web.n.repo_added.one":            {"%d repository added", "%d dépôt ajouté"},
	"web.n.repo_added.other":          {"%d repositories added", "%d dépôts ajoutés"},
	"web.n.repo_updated.one":          {"%d repository updated", "%d dépôt mis à jour"},
	"web.n.repo_updated.other":        {"%d repositories updated", "%d dépôts mis à jour"},
	"web.n.repo_already.one":          {"%d repository already tracked", "%d dépôt déjà suivi"},
	"web.n.repo_already.other":        {"%d repositories already tracked", "%d dépôts déjà suivis"},
	"web.n.repo_inaccessible.one":     {"%d repository inaccessible", "%d dépôt inaccessible"},
	"web.n.repo_inaccessible.other":   {"%d repositories inaccessible", "%d dépôts inaccessibles"},

	// Layout : en-tête, navigation, bandeau de nouvelle version, pied de page.
	"web.layout.home_aria":       {"Watchoo, dashboard", "Watchoo, tableau de bord"},
	"web.layout.workspace_title": {"Workspace", "Espace de supervision"},
	"web.layout.theme_aria": {
		"Switch between the light and dark themes",
		"Basculer entre le thème clair et le thème sombre",
	},
	"web.layout.theme_title": {"Light / dark theme", "Thème clair / sombre"},
	"web.layout.nav_aria":    {"Main navigation", "Navigation principale"},
	"web.nav.dashboard":      {"Dashboard", "Tableau de bord"},
	"web.nav.contributors":   {"Contributors", "Contributeurs"},
	"web.nav.settings":       {"Settings", "Réglages"},
	"web.update.available": {
		"New version of Watchoo available:",
		"Nouvelle version de Watchoo disponible :",
	},
	"web.update.docker": {
		"Change the tag of the Docker image to update it.",
		"Changez le tag de l'image Docker pour la mettre à jour.",
	},
	"web.update.cli_before": {"Update it with", "Mettez-la à jour avec"},
	"web.update.cli_middle": {", then restart", ", puis redémarrez"},
	"web.update.notes":      {"Release notes", "Notes de version"},
	"web.footer": {
		"Watchoo %s · read-only: nothing is ever written to GitHub · data kept on this machine",
		"Watchoo %s · lecture seule : rien n'est jamais écrit sur GitHub · données gardées sur cette machine",
	},

	// Pastille d'état, lignes, liens communs (partials.html).
	"web.state.title":     {"State: %s", "État : %s"},
	"web.state.with_days": {"%s · %d d", "%s · %d j"},
	"web.ai.col":          {"AI signatures", "Signatures d'IA"},
	"web.ai.badge":        {"AI: %s", "IA : %s"},
	"web.ai.note": {
		"An AI signature says a tool was declared on the commit, not who wrote the code; its absence proves nothing.",
		"Une signature d'IA dit qu'un outil est déclaré sur le commit, pas qui a écrit le code ; son absence ne prouve rien.",
	},
	"web.ai.commit_title": {
		"Tool declared on this commit; this says nothing about who wrote the code",
		"Outil déclaré sur ce commit ; cela ne dit pas qui a écrit le code",
	},
	"web.ai.totals_title": {
		"Commits with a declared AI signature / commits analyzed",
		"Commits avec une signature d'IA déclarée / commits analysés",
	},
	"web.ai.unscanned_title": {
		"Not analyzed yet: done at the next sync",
		"Pas encore analysés : c'est fait à la prochaine synchro",
	},
	"web.ai.signed_scanned": {"signed / analyzed commits", "commits signés / analysés"},
	"web.lines.unknown_title": {
		"Lines unknown: not counted yet (local Git clone unavailable)",
		"Lignes inconnues : pas encore comptées (clone Git local indisponible)",
	},
	"web.lines.uncounted_title": {
		"Commits whose lines are not counted yet (local Git clone unavailable)",
		"Commits dont les lignes ne sont pas encore comptées (clone Git local indisponible)",
	},
	"web.lines.including":    {"(including %s)", "(dont %s)"},
	"web.commit.view_github": {"View the commit on GitHub", "Voir le commit sur GitHub"},
	"web.commit.view_forge":  {"View the commit on the forge", "Voir le commit sur la forge"},
	"web.profile.of":         {"GitHub profile of %s", "Profil GitHub de %s"},
	"web.profile.of_unknown": {"GitHub profile of this contributor", "Profil GitHub de ce contributeur"},
	"web.col.repo":           {"Repository", "Dépôt"},
	"web.col.repos":          {"Repositories", "Dépôts"},
	"web.col.contributor":    {"Contributor", "Contributeur"},
	"web.col.commits":        {"Commits", "Commits"},
	"web.col.lines":          {"Lines", "Lignes"},
	"web.col.total":          {"Total", "Total"},

	// État des synchros (navbar) et boutons de synchro.
	"web.sync.running":       {"Sync: %d/%d", "Synchro : %d/%d"},
	"web.sync.running_auto":  {"Automatic sync: %d/%d", "Synchro automatique : %d/%d"},
	"web.sync.progress_aria": {"Sync progress", "Progression de la synchro"},
	"web.sync.last":          {"Last sync", "Synchro"},
	"web.sync.failed":        {"failed", "échec"},
	"web.sync.none":          {"No sync since startup", "Aucune synchro depuis le lancement"},
	"web.sync.none_title": {
		"Last sync of each repository: repository page and settings",
		"Dernière synchro de chaque dépôt : page du dépôt et réglages",
	},
	"web.sync.warnings_menu": {"Repositories with warnings", "Dépôts avec avertissements"},

	// Avertissements de synchro (syncwarnings.go) : libellé court par code, conséquence sur la carte
	// d'un dépôt, alerte de la page d'un dépôt.
	"web.warn.label.stats_not_computed":      {"Line statistics not computed", "Statistiques de lignes non calculées"},
	"web.warn.label.clone_unavailable":       {"Local Git clone unavailable", "Clone Git local indisponible"},
	"web.warn.label.languages_not_computed":  {"Languages not computed", "Langages non calculés"},
	"web.warn.label.lists_not_synced":        {"Pull requests, releases or tags not read", "Pull requests, releases ou tags non lus"},
	"web.warn.label.ai_not_scanned":          {"AI signatures not analyzed", "Signatures d'IA non analysées"},
	"web.warn.label.diagnostic_not_recorded": {"Diagnosis signals not recorded", "Signaux de diagnostic non enregistrés"},
	"web.warn.label.other":                   {"Sync warning", "Avertissement de synchro"},
	"web.warn.effect.stats_not_computed":     {"Lines of the new commits not counted", "Lignes des nouveaux commits non comptées"},
	"web.warn.effect.clone_unavailable": {
		"Line statistics read through the API (limited)",
		"Statistiques de lignes lues par l'API (limitées)",
	},
	"web.warn.effect.languages_not_computed": {"Languages not updated", "Langages non mis à jour"},
	"web.warn.effect.lists_not_synced": {
		"Pull requests, releases or tags may be out of date",
		"Pull requests, releases ou tags peut-être pas à jour",
	},
	"web.warn.effect.ai_not_scanned": {"AI signatures analyzed at the next sync", "Signatures d'IA analysées à la prochaine synchro"},
	"web.warn.effect.diagnostic_not_recorded": {
		"New commits not measured for the diagnosis (see watchoo diagnostic backfill)",
		"Nouveaux commits non mesurés pour le diagnostic (voir watchoo diagnostic backfill)",
	},
	"web.warn.badge_aria":   {"Warnings of the last sync: %s", "Avertissements de la dernière synchro : %s"},
	"web.warn.alert_title":  {"Last sync finished with %s", "La dernière synchro s'est terminée avec %s"},
	"web.warn.alert_failed": {"Last sync failed", "La dernière synchro a échoué"},
	"web.warn.clone_hint": {
		"Fix the cause shown above (SSH access or Git credentials of this machine), then sync again.",
		"Corrigez la cause indiquée ci-dessus (accès SSH ou identifiants Git de cette machine), puis relancez une synchro.",
	},
	"web.sync.quota_low": {"quota low", "quota bas"},
	"web.sync.quota_low_title": {
		"Less than 10% of the GitHub quota left: syncs are deferred until it resets",
		"Moins de 10 % du quota GitHub : les synchros sont reportées jusqu'à sa remise à zéro",
	},
	"web.sync.button_all":  {"Sync now", "Synchroniser maintenant"},
	"web.sync.button_repo": {"Sync this repository", "Synchroniser ce dépôt"},
	"web.sync.started":     {"Sync started.", "Synchro lancée."},
	"web.sync.already_running": {
		"Sync already running: wait for it to finish.",
		"Synchro déjà en cours : attendez qu'elle se termine.",
	},
	"web.sync.closing": {
		"Watchoo is shutting down: no new sync.",
		"Arrêt de Watchoo en cours : aucune nouvelle synchro.",
	},
	"web.sync.impossible": {"Sync not possible", "Synchro impossible"},

	// Graphiques (partials.html « contributors-chart », charts.go).
	"web.chart.summary_repos":        {"%s in %s since %s", "%s dans %s depuis le %s"},
	"web.chart.none_repos":           {"No commit since %s", "Aucun commit depuis le %s"},
	"web.chart.summary_contributors": {"%s by %s since %s", "%s de %s depuis le %s"},
	"web.chart.none_contributors": {
		"No commit by a known author since %s",
		"Aucun commit d'auteur connu depuis le %s",
	},
	"web.chart.aria_repos": {
		"Commits per week in their most active repositories%s over the last %d weeks: %s in total",
		"Commits par semaine dans ses dépôts les plus actifs%s sur les %d dernières semaines : %s au total",
	},
	"web.chart.aria_contributors": {
		"Commits per week of the most active contributors%s over the last %d weeks: %s in total",
		"Commits par semaine des contributeurs les plus actifs%s sur les %d dernières semaines : %s au total",
	},
	"web.chart.legend_repos": {
		"Commits per week (Monday to Sunday, commit date, display time zone of the workspace) in the %d repositories where they have the most%s over the period; the others are grouped.",
		"Commits par semaine (du lundi au dimanche, date de commit, fuseau d'affichage de l'espace) dans les %d dépôts où il en a le plus%s sur la période ; les autres sont regroupés.",
	},
	"web.chart.legend_contributors": {
		"Commits per week (Monday to Sunday, commit date, display time zone of the workspace) of the %d most active contributors%s over the period; the others are grouped.",
		"Commits par semaine (du lundi au dimanche, date de commit, fuseau d'affichage de l'espace) des %d contributeurs les plus actifs%s sur la période ; les autres sont regroupés.",
	},
	"web.chart.data":                 {"Chart data", "Données du graphique"},
	"web.chart.caption_repos":        {"Commits per week and per repository", "Commits par semaine et par dépôt"},
	"web.chart.caption_contributors": {"Commits per week and per contributor", "Commits par semaine et par contributeur"},
	"web.chart.no_commit":            {"No commit over the period.", "Aucun commit sur la période."},
	"web.chart.peak":                 {", at most %d on %s", ", au plus %d le %s"},
	"web.chart.other_contributors":   {"Other contributors", "Autres contributeurs"},
	"web.chart.other_repos":          {"Other repositories", "Autres dépôts"},

	// Tableau de bord.
	"web.dash.intro": {
		"What moved, when, and who did it, in the %s workspace.",
		"Ce qui a bougé, quand, et qui l'a fait, dans l'espace %s.",
	},
	"web.dash.repos_title": {"Tracked repositories", "Dépôts suivis"},
	"web.dash.stat_active": {"Active", "Actifs"},
	"web.dash.stat_other":  {"Other", "Autres"},
	"web.dash.out_of":      {"out of %s", "sur %s"},
	"web.dash.more_than":   {"more than %s without a commit", "plus de %s sans commit"},
	"web.dash.other_desc": {
		"never synced, in error, archived…",
		"jamais synchronisés, en erreur, archivés…",
	},
	"web.dash.lines_desc":         {"added / deleted, %s", "ajoutées / supprimées, %s"},
	"web.dash.no_commit":          {"No known commit.", "Aucun commit connu."},
	"web.dash.no_commit_unsynced": {"No known commit: not synced yet.", "Aucun commit connu : pas encore synchronisé."},
	"web.dash.empty_title":        {"No tracked repository", "Aucun dépôt suivi"},
	"web.dash.empty_text": {
		`Add a repository in the settings, or with "watchoo repo add".`,
		"Ajoutez un dépôt dans les réglages, ou avec « watchoo repo add ».",
	},
	"web.dash.feed_title": {"Latest commits", "Derniers commits"},
	"web.dash.feed_empty": {
		"No commit yet. Run a sync to fill the feed.",
		"Aucun commit pour l'instant. Lancez une synchro pour remplir le fil.",
	},
	"web.dash.activity_title": {"Workspace activity over %d days", "Activité de l'espace sur %d jours"},
	"web.dash.activity_aria": {
		"Commits per day over the last %d days, all repositories: %s in total",
		"Commits par jour sur les %d derniers jours, tous dépôts confondus : %s au total",
	},
	"web.dash.activity_legend": {
		"Commits per day (commit date), all repositories, in the display time zone of the workspace.",
		"Commits par jour (date de commit), tous dépôts confondus, dans le fuseau d'affichage de l'espace.",
	},

	// Détail d'un dépôt.
	"web.repo.activity_title": {"Activity over %d days", "Activité sur %d jours"},
	"web.repo.activity_aria": {
		"Commits per day over the last %d days: %s in total",
		"Commits par jour sur les %d derniers jours : %s au total",
	},
	"web.repo.activity_legend": {
		"Commits per day (commit date), in the display time zone of the workspace.",
		"Commits par jour (date de commit), dans le fuseau d'affichage de l'espace.",
	},
	"web.repo.contributors_chart_title": {"Activity per contributor over %d weeks", "Activité par contributeur sur %d semaines"},
	"web.repo.chart_scope":              {" in this repository", " de ce dépôt"},
	"web.repo.chart_note": {
		"All tracked branches; a commit present on several branches is counted once.",
		"Toutes les branches suivies ; un commit présent sur plusieurs branches n'est compté qu'une fois.",
	},
	"web.repo.archived": {"archived", "archivé"},
	"web.repo.stopped":  {"tracking stopped", "suivi arrêté"},
	"web.repo.thresholds_line": {
		"Thresholds: Slacking after %s (%s), Dead after %s (%s) without a commit.",
		"Seuils : Flemme après %s (%s), Dead après %s (%s) sans commit.",
	},
	"web.repo.threshold_own":       {"this repository's", "propre à ce dépôt"},
	"web.repo.threshold_workspace": {"the workspace's", "de l'espace"},
	"web.repo.edit":                {"Edit", "Modifier"},
	"web.repo.last_commit":         {"Last commit", "Dernier commit"},
	"web.repo.last_sync":           {"Last sync", "Dernière synchro"},
	"web.repo.running":             {"running", "en cours"},
	"web.repo.lines":               {"Lines:", "Lignes :"},
	"web.repo.branches":            {"Branches", "Branches"},
	"web.repo.default_branch":      {"default:", "par défaut :"},
	"web.repo.patterns":            {"Patterns:", "Motifs :"},
	"web.repo.all_branches":        {"All branches of the repository", "Toutes les branches du dépôt"},
	"web.repo.on_github":           {"On GitHub", "Sur GitHub"},
	"web.repo.no_forge_details": {
		"Stars and issues not available: repository tracked by local Git clone (without gh).",
		"Étoiles et issues non disponibles : dépôt suivi par clone Git local (sans gh).",
	},
	"web.repo.thresholds_title": {"Activity thresholds of this repository", "Seuils d'activité de ce dépôt"},
	"web.repo.thresholds_help": {
		"Number of days without a commit before switching to Slacking, then to Dead. An empty field (or 0) uses the workspace threshold: Slacking after %s, Dead after %s.",
		"Nombre de jours sans commit avant de passer en Flemme, puis en Dead. Un champ vide (ou 0) reprend le seuil de l'espace : Flemme après %s, Dead après %s.",
	},
	"web.repo.flemme_field":            {"Slacking after (days)", "Flemme après (jours)"},
	"web.repo.dead_field":              {"Dead after (days)", "Dead après (jours)"},
	"web.repo.save_thresholds":         {"Save the thresholds", "Enregistrer les seuils"},
	"web.repo.contributor_last_commit": {"last commit", "dernier commit"},
	"web.repo.no_commit_db":            {"No commit in the database for this repository.", "Aucun commit en base pour ce dépôt."},
	"web.repo.history":                 {"History", "Historique"},
	"web.repo.matching_filters":        {"matching the filters", "correspondant aux filtres"},
	"web.repo.filter_branch":           {"Branch", "Branche"},
	"web.repo.all_branches_option":     {"All branches", "Toutes les branches"},
	"web.repo.tracked_branches":        {"Tracked branches", "Branches suivies"},
	"web.repo.other_branches":          {"Other branches", "Autres branches"},
	"web.repo.unknown_branch":          {"(unknown)", "(inconnue)"},
	"web.repo.unknown_author":          {"(unknown)", "(inconnu)"},
	"web.repo.filter_author":           {"Author", "Auteur"},
	"web.repo.all_authors":             {"All authors", "Tous les auteurs"},
	"web.repo.filter":                  {"Filter", "Filtrer"},
	"web.repo.clear_filters":           {"Clear the filters", "Effacer les filtres"},
	"web.repo.email_filter_ignored": {
		"Filtering by e-mail address is not offered here: choose an author in the list.",
		"Le filtre par adresse e-mail n'est pas proposé ici : choisissez un auteur dans la liste.",
	},
	"web.repo.no_match": {"No commit matches these filters.", "Aucun commit ne correspond à ces filtres."},
	"web.repo.no_commit_sync": {
		"No commit in the database for this repository: run a sync.",
		"Aucun commit en base pour ce dépôt : lancez une synchro.",
	},
	"web.repo.history_pages": {"History pages", "Pages de l'historique"},

	// Seuils propres à un dépôt (repo_thresholds.go).
	"web.thresholds.saved": {
		"Thresholds saved: the state of the repository is computed again with these values.",
		"Seuils enregistrés : l'état du dépôt est recalculé avec ces valeurs.",
	},
	"web.thresholds.refused":     {"Thresholds refused: %v", "Seuils refusés : %v"},
	"web.thresholds.field_error": {"%s: %v", "%s : %v"},
	"web.thresholds.workspace":   {"workspace", "espace"},
	"web.thresholds.days":        {"%d d", "%d j"},

	// Pull requests, releases et tags.
	"web.pr.title": {"Pull requests", "Pull requests"},
	"web.pr.unavailable": {
		`Not available: this repository is tracked by local Git clone, without gh. Sign in to gh ("gh auth login") to read them through the GitHub API.`,
		"Non disponibles : ce dépôt est suivi par clone Git local, sans gh. Connectez gh (« gh auth login ») pour les lire par l'API GitHub.",
	},
	"web.pr.open_heading":        {"Open", "Ouvertes"},
	"web.pr.merged_heading":      {"Latest merged", "Dernières mergées"},
	"web.pr.none_repo":           {"No pull request in the database for this repository.", "Aucune pull request en base pour ce dépôt."},
	"web.pr.opened_at":           {"opened", "ouverte"},
	"web.pr.merged_at":           {"merged", "mergée"},
	"web.pr.closed_at":           {"closed", "fermée"},
	"web.pr.badge_open":          {"open", "ouverte"},
	"web.pr.badge_merged":        {"merged", "mergée"},
	"web.pr.badge_closed":        {"closed", "fermée"},
	"web.pr.badge_draft":         {"draft", "brouillon"},
	"web.release.title":          {"Releases and tags", "Releases et tags"},
	"web.release.releases":       {"Releases", "Releases"},
	"web.release.tags":           {"Tags", "Tags"},
	"web.release.prerelease":     {"pre-release", "pré-release"},
	"web.release.tag_commit":     {"View the tag's commit on GitHub", "Voir le commit du tag sur GitHub"},
	"web.repo.contributor_share": {"Share of the commits of this repository", "Part des commits de ce dépôt"},
	"web.repo.languages_title":   {"Languages", "Langages"},
	"web.repo.languages_note": {
		"Share of the bytes on the default branch, read from the local clone. The figures can differ slightly from those of GitHub.",
		"Part des octets sur la branche par défaut, lue dans le clone local. Les chiffres peuvent différer un peu de ceux de GitHub.",
	},
	"web.repo.languages_none": {
		"No language yet: they appear after the next sync.",
		"Aucun langage pour l'instant : ils apparaissent après la prochaine synchro.",
	},
	"web.release.none": {
		"No release and no tag in the database for this repository.",
		"Aucune release ni aucun tag en base pour ce dépôt.",
	},
	"web.release.local": {
		"Releases not available through a local Git clone (without gh); tags are read.",
		"Releases non disponibles par clone Git local (sans gh) ; les tags, eux, sont lus.",
	},

	// Contributeurs.
	"web.contributors.intro": {
		"Authors of the commits of all tracked repositories, from the most to the least active.",
		"Auteurs des commits de tous les dépôts suivis, du plus actif au moins actif.",
	},
	"web.contributors.chart_title": {"Activity over %d weeks", "Activité sur %d semaines"},
	"web.contributors.empty_title": {"No contributor yet", "Aucun contributeur pour l'instant"},
	"web.contributors.empty_text": {
		"Authors appear after the first sync of the repositories.",
		"Les auteurs apparaissent après la première synchro des dépôts.",
	},
	"web.contributors.manage":        {"Manage repositories", "Gérer les dépôts"},
	"web.contributors.caption":       {"Contributor ranking", "Classement des contributeurs"},
	"web.contributors.per_repo":      {"Commits per repository", "Commits par dépôt"},
	"web.contributors.last_activity": {"Last activity", "Dernière activité"},
	"web.contributors.share":         {"Share of all commits", "Part de tous les commits"},
	"web.contributors.last_in_repo":  {"Last commit in this repository: %s", "Dernier commit dans ce dépôt : %s"},

	// Page d'un contributeur.
	"web.contributor.activity_aria": {
		"Commits per day over the last %d days, all their repositories: %s in total",
		"Commits par jour sur les %d derniers jours, tous ses dépôts : %s au total",
	},
	"web.contributor.activity_legend": {
		"Commits per day (commit date) they authored, all repositories, in the display time zone of the workspace.",
		"Commits par jour (date de commit) dont il ou elle est l'auteur, tous dépôts confondus, dans le fuseau d'affichage de l'espace.",
	},
	"web.contributor.chart_title": {"Activity per repository over %d weeks", "Activité par dépôt sur %d semaines"},
	"web.contributor.chart_note": {
		"A commit is counted once, whatever its branch.",
		"Un commit n'est compté qu'une fois, quelle que soit sa branche.",
	},
	"web.contributor.in_repos":      {"in %s", "dans %s"},
	"web.contributor.added_deleted": {"added / deleted", "ajoutées / supprimées"},
	"web.contributor.first_commit":  {"First commit", "Premier commit"},
	"web.contributor.overview_note": {
		`Commits they authored. Added and deleted lines are counted in the local Git clone of each repository, for all its commits; "—" marks lines not counted yet (clone unavailable). The Active, Slacking and Dead states are those of the repositories (list below); e-mail addresses are never shown.`,
		"Commits dont il ou elle est l'auteur. Les lignes ajoutées et supprimées sont comptées dans le clone Git local de chaque dépôt, pour tous ses commits ; « — » signale des lignes pas encore comptées (clone indisponible). Les états Actif, Flemme et Dead sont ceux des dépôts (liste ci-dessous) ; les adresses e-mail ne sont jamais affichées.",
	},
	"web.contributor.repos_caption":     {"Their commits per repository", "Ses commits par dépôt"},
	"web.contributor.repo_state":        {"Repository state", "État du dépôt"},
	"web.contributor.their_last_commit": {"Their last commit", "Son dernier commit"},
	"web.contributor.main_language":     {"Main language", "Langage principal"},
	"web.contributor.languages_others":  {"Other languages", "Autres langages"},
	"web.contributor.languages_unknown": {
		"No known language: they are read from the local clone at the next sync.",
		"Aucun langage connu : ils sont lus dans le clone local à la prochaine synchro.",
	},
	"web.contributor.languages_none": {
		"Their commits add no line in a recognized language.",
		"Ses commits n'ajoutent aucune ligne dans un langage reconnu.",
	},
	"web.contributor.languages_note": {
		"Lines added by their commits in each language, all repositories together, read from the local clone. Merge commits, dependencies and generated files do not count; lines added measure neither quality nor skill.",
		"Lignes ajoutées par ses commits dans chaque langage, tous dépôts confondus, lues dans le clone local. Les commits de fusion, les dépendances et les fichiers générés ne comptent pas ; des lignes ajoutées ne mesurent ni la qualité ni la compétence.",
	},
	"web.contributor.no_commit": {
		"No commit they authored in the database.",
		"Aucun commit dont il ou elle est l'auteur en base.",
	},
	"web.contributor.pulls_no_account": {
		"Not matched: no GitHub account is known for this contributor (Git identity only).",
		"Non rapprochées : aucun compte GitHub n'est connu pour ce contributeur (identité Git seule).",
	},
	"web.contributor.pulls_recent": {
		"The %d most recently updated, all repositories.",
		"Les %d plus récemment modifiées, tous dépôts confondus.",
	},
	"web.contributor.pulls_none":   {"No pull request from this account in the database.", "Aucune pull request de ce compte en base."},
	"web.contributor.commit_pages": {"Commit pages", "Pages des commits"},

	// Réglages.
	"web.settings.intro": {
		"Tracked repositories, synchronization and GitHub API quota. Nothing is ever written to GitHub.",
		"Dépôts suivis, synchronisation et quota de l'API GitHub. Rien n'est jamais écrit sur GitHub.",
	},
	"web.settings.empty": {
		`No repository in this workspace. Add one with the "Add a repository" form.`,
		"Aucun dépôt dans cet espace. Ajoutez-en un avec le formulaire « Ajouter un dépôt ».",
	},
	"web.settings.all_branches":   {"all branches", "toutes les branches"},
	"web.settings.branches_label": {"Branches:", "Branches :"},
	"web.settings.commits_label":  {"Commits:", "Commits :"},
	"web.settings.sync_label":     {"Sync:", "Synchro :"},
	"web.settings.edit_summary":   {"Edit the display name and the branches", "Modifier le nom affiché et les branches"},
	"web.settings.label_field_edit": {
		"Display name (empty: %s)",
		"Nom affiché (vide : %s)",
	},
	"web.settings.branches_field_edit": {
		"Tracked branches, separated by commas (empty: all)",
		"Branches suivies, séparées par des virgules (vide : toutes)",
	},
	"web.settings.save": {"Save", "Enregistrer"},
	"web.settings.resume_title": {
		"Checks the access with GitHub and resumes the tracking",
		"Vérifie l'accès auprès de GitHub et reprend le suivi",
	},
	"web.settings.resume":        {"Resume tracking", "Reprendre le suivi"},
	"web.settings.remove_button": {"Remove…", "Retirer…"},
	"web.settings.add_title":     {"Add a repository", "Ajouter un dépôt"},
	"web.settings.add_help": {
		"The access is checked with GitHub using the gh account (without gh: through git, with the SSH key or the Git credentials of this machine); no sync is started.",
		"L'accès est vérifié auprès de GitHub avec le compte gh (sans gh : par git, avec la clé SSH ou les identifiants Git de cette machine) ; aucune synchro n'est lancée.",
	},
	"web.settings.repo_field":     {"Repository (owner/repo or GitHub URL)", "Dépôt (owner/repo ou URL GitHub)"},
	"web.settings.label_field":    {"Display name (optional)", "Nom affiché (facultatif)"},
	"web.settings.branches_field": {"Branches (optional, empty: all)", "Branches (facultatif, vide : toutes)"},
	"web.settings.add":            {"Add", "Ajouter"},
	"web.settings.auto_title":     {"Automatic synchronization", "Synchronisation automatique"},
	"web.settings.current_interval": {
		"Current interval:",
		"Intervalle en vigueur :",
	},
	"web.settings.next_sync":     {"Next sync:", "Prochaine synchro :"},
	"web.settings.file_label":    {"config.yaml file:", "Fichier config.yaml :"},
	"web.settings.auto_disabled": {"automatic sync disabled (sync.auto)", "synchro automatique désactivée (sync.auto)"},
	"web.settings.file_changed": {
		`config.yaml changed since "watchoo serve" started: this value will apply at the next start.`,
		"config.yaml a changé depuis le lancement de « watchoo serve » : cette valeur vaudra au prochain lancement.",
	},
	"web.settings.overridden": {
		"The interval was changed for this process (--interval option or this page).",
		"L'intervalle a été changé pour ce processus (option --interval ou cette page).",
	},
	"web.settings.new_interval": {
		"New interval (0 = manual only, otherwise at least %s)",
		"Nouvel intervalle (0 = manuelle seule, sinon au moins %s)",
	},
	"web.settings.apply": {"Apply", "Appliquer"},
	"web.settings.interval_note": {
		`The change lasts until "watchoo serve" stops: this form never rewrites config.yaml. For a lasting setting, change sync.interval in the file or import a shared configuration (below).`,
		"Le changement vaut jusqu'à l'arrêt de « watchoo serve » : ce formulaire ne réécrit jamais config.yaml. Pour un réglage durable, modifiez sync.interval dans le fichier ou importez une configuration partagée (ci-dessous).",
	},
	"web.settings.branches_prefix":   {"branches", "branches"},
	"web.settings.change_impossible": {"Change not possible", "Changement impossible"},
	"web.settings.edit_refused":      {"Change refused: %v", "Modification refusée : %v"},
	"web.settings.inaccessible": {
		"Repository %s inaccessible: %s. Check its name and that the gh account (without gh: the SSH key or the Git credentials of this machine) has access to it.",
		"Dépôt %s inaccessible : %s. Vérifiez son nom et que le compte gh (sans gh : la clé SSH ou les identifiants Git de cette machine) y a accès.",
	},
	"web.settings.add_failed": {
		`Adding the repository %s is not possible for now (GitHub unreachable, quota exhausted or database error). The details are in the "watchoo serve" log.`,
		"Ajout du dépôt %s impossible pour le moment (GitHub injoignable, quota épuisé ou erreur de la base). Le détail est dans le journal de « watchoo serve ».",
	},
	"web.interval.manual":        {"manual only", "manuelle seule"},
	"web.interval.minutes":       {"%d min", "%d min"},
	"web.interval.hours":         {"%d h", "%d h"},
	"web.interval.hours_minutes": {"%d h %02d min", "%d h %02d"},
	"web.interval.empty": {
		"empty interval: enter for example 30m, 1h, or 0 for manual sync only",
		"intervalle vide : indiquez par exemple 30m, 1h ou 0 pour la synchro manuelle seule",
	},
	"web.interval.out_of_bounds": {
		"invalid interval %q: number of minutes out of bounds",
		"intervalle %q invalide : nombre de minutes hors des bornes",
	},
	"web.interval.unreadable": {
		"unreadable interval %q: enter a duration like 30m, 1h or 1h30m, or 0 for manual sync only",
		"intervalle %q illisible : indiquez une durée comme 30m, 1h ou 1h30m, ou 0 pour la synchro manuelle seule",
	},

	// Messages après une action des réglages (paramètre msg).
	"web.flash.the_repo":       {"The repository", "Le dépôt"},
	"web.flash.this_repo":      {"this repository", "ce dépôt"},
	"web.flash.the_repo_named": {`The repository "%s"`, "Le dépôt « %s »"},
	"web.flash.repo_named":     {`the repository "%s"`, "le dépôt « %s »"},
	"web.flash.added": {
		`%s is tracked. It will be synced at the next sync; "Sync now" starts it right away.`,
		"%s est suivi. Il sera synchronisé à la prochaine synchro ; « Synchroniser maintenant » la lance tout de suite.",
	},
	"web.flash.updated": {
		"%s was already tracked: its settings are updated (and its tracking resumed if it was stopped).",
		"%s était déjà suivi : ses réglages sont mis à jour (et son suivi repris s'il était arrêté).",
	},
	"web.flash.already": {"%s is already tracked: nothing to change.", "%s est déjà suivi : rien à changer."},
	"web.flash.saved": {
		"%s is changed. The new branches will be taken into account at the next sync.",
		"%s est modifié. Les nouvelles branches seront prises en compte à la prochaine synchro.",
	},
	"web.flash.stopped": {
		`Tracking stopped for %s; its history is kept. "Resume tracking" starts it again.`,
		"Suivi arrêté pour %s ; son historique est gardé. « Reprendre le suivi » le relance.",
	},
	"web.flash.already_stopped": {
		"The tracking of this repository was already stopped: nothing changed.",
		"Le suivi de ce dépôt était déjà arrêté : rien n'a changé.",
	},
	"web.flash.deleted": {
		"The repository and its history are deleted from the database. Nothing was changed on GitHub.",
		"Le dépôt et son historique sont supprimés de la base. Rien n'a été modifié sur GitHub.",
	},
	"web.flash.interval": {
		"Sync interval changed for this process. config.yaml is not changed.",
		"Intervalle de synchro changé pour ce processus. config.yaml n'est pas modifié.",
	},

	// Quota de l'API GitHub.
	"web.quota.title":   {"GitHub API quota", "Quota de l'API GitHub"},
	"web.quota.loading": {"Reading the quota…", "Lecture du quota…"},
	"web.quota.note": {
		"Free read (it does not consume quota). Below 10% left, syncs are deferred until the reset.",
		"Lecture gratuite (elle ne consomme pas de quota). Sous 10 % restants, les synchros sont reportées jusqu'à la remise à zéro.",
	},
	"web.quota.remaining":      {"/ %d requests left", "/ %d requêtes restantes"},
	"web.quota.share_aria":     {"Share of the quota left", "Part du quota restante"},
	"web.quota.reset_at":       {"Reset at", "Remise à zéro à"},
	"web.quota.deferred_until": {"; syncs deferred until then", " ; synchros reportées jusque-là"},
	"web.quota.not_applicable": {"not applicable", "sans objet"},
	"web.quota.local": {
		`gh is not installed or not signed in: the repositories are tracked by local Git clones (repos/ at the workspace root), with the SSH key or the Git credentials of this machine, without quota. Pull requests, releases and stars are then not read; "gh auth login" switches back to the GitHub API.`,
		"gh n'est pas installé ou pas connecté : les dépôts sont suivis par clones Git locaux (repos/ à la racine de l'espace), avec la clé SSH ou les identifiants Git de cette machine, sans quota. Pull requests, releases et étoiles ne sont alors pas lus ; « gh auth login » fait repasser par l'API GitHub.",
	},
	"web.quota.unknown": {"quota unknown", "quota inconnu"},

	// Retrait d'un dépôt.
	"web.remove.title":    {"Remove %s", "Retirer %s"},
	"web.remove.crumb":    {"Remove a repository", "Retirer un dépôt"},
	"web.remove.question": {`Remove "%s"?`, "Retirer « %s » ?"},
	"web.remove.intro": {
		"Nothing is changed on GitHub: only the tracking by Watchoo changes. The contributors stay (they may have worked on other repositories).",
		"Rien n'est modifié sur GitHub : seul le suivi par Watchoo change. Les contributeurs restent (ils peuvent avoir travaillé sur d'autres dépôts).",
	},
	"web.remove.stop_title": {"Stop tracking", "Arrêter le suivi"},
	"web.remove.stop_help": {
		`The history stays available; the repository is no longer synced ("Resume tracking" starts it again).`,
		"L'historique reste consultable ; le dépôt n'est plus synchronisé (« Reprendre le suivi » le relance).",
	},
	"web.remove.stop_button":     {"Stop tracking, keep the history", "Arrêter le suivi, garder l'historique"},
	"web.remove.already_stopped": {"tracking already stopped", "suivi déjà arrêté"},
	"web.remove.delete_title":    {"Delete with the history", "Supprimer avec l'historique"},
	"web.remove.delete_help": {
		"The repository, its branches, the commits in the database (%s) and the log of its syncs are erased from the database. Irreversible.",
		"Le dépôt, ses branches, les commits en base (%s) et le journal de ses synchros sont effacés de la base. Irréversible.",
	},
	"web.remove.running": {
		"A sync was running when this page was opened: while it runs, the deletion is refused; try again when it is finished.",
		"Une synchro était en cours à l'ouverture de cette page : tant qu'elle tourne, la suppression est refusée ; réessayez quand elle est terminée.",
	},
	"web.remove.delete_button": {"Delete the repository and its history", "Supprimer le dépôt et son historique"},
	"web.remove.impossible":    {"Deletion not possible", "Suppression impossible"},
	"web.remove.impossible_text": {
		"A sync is running: try again when it is finished. Nothing was deleted.",
		"Une synchro est en cours : réessayez quand elle sera terminée. Rien n'a été supprimé.",
	},

	// Partage de la configuration (page Réglages).
	"web.share.title": {"Share the configuration", "Partager la configuration"},
	"web.share.intro": {
		"A JSON file with the tracked repositories (display names, branches) and the common settings, for the other people who supervise this group. No commit, no e-mail, no token, no setting specific to this machine.",
		"Un fichier JSON avec les dépôts suivis (noms affichés, branches) et les réglages communs, pour les autres personnes qui supervisent ce groupe. Ni commit, ni e-mail, ni token, ni réglage propre à cette machine.",
	},
	"web.share.export":         {"Export", "Exporter"},
	"web.share.import_field":   {"Import a file (JSON, 1 MiB at most)", "Importer un fichier (JSON, 1 Mio au plus)"},
	"web.share.preview_button": {"See the preview", "Voir l'aperçu"},
	"web.share.note": {
		"A preview is shown first: nothing is applied without your confirmation. The import adds and updates repositories, it never removes any. The file may reveal the list of the tracked students: share it only with the people concerned.",
		"Un aperçu s'affiche d'abord : rien n'est appliqué sans votre confirmation. L'import ajoute et met à jour des dépôts, il n'en retire jamais. Le fichier peut révéler la liste des étudiants suivis : partagez-le seulement avec les personnes concernées.",
	},
	"web.share.unavailable": {"Sharing unavailable", "Partage indisponible"},
	"web.share.unavailable_text": {
		`Sharing the configuration is not available for this server. Use "watchoo config export" or "watchoo config import" in the workspace.`,
		"Le partage de la configuration n'est pas disponible pour ce serveur. Utilisez « watchoo config export » ou « watchoo config import » dans l'espace.",
	},
	"web.share.unreadable": {"Settings unreadable", "Réglages illisibles"},
	"web.share.unreadable_text": {
		"The settings of the workspace cannot be read again: %s. Fix config.yaml, then try again. Nothing was changed.",
		"Les réglages de l'espace ne peuvent pas être relus : %s. Corrigez config.yaml, puis recommencez. Rien n'a été modifié.",
	},

	// Import d'une configuration (aperçu, refus, bilan).
	"web.import.title":   {"Import a configuration", "Importer une configuration"},
	"web.import.file":    {"File: %s", "Fichier : %s"},
	"web.import.refused": {"File refused", "Fichier refusé"},
	"web.import.back":    {"Back to the settings", "Retour aux réglages"},
	"web.import.preview_intro": {
		"Import preview:",
		"Aperçu de l'import :",
	},
	"web.import.preview_nothing":       {"nothing is applied yet", "rien n'est encore appliqué"},
	"web.import.preview_never_removed": {". Already tracked repositories are never removed.", ". Les dépôts déjà suivis ne sont jamais retirés."},
	"web.import.source":                {"File exported", "Fichier exporté"},
	"web.import.source_workspace":      {`from the "%s" workspace`, "de l'espace « %s »"},
	"web.import.source_on":             {"on", "le"},
	"web.import.source_version":        {"with Watchoo %s", "avec Watchoo %s"},
	"web.import.no_rename":             {"The import does not rename this workspace.", "L'import ne renomme pas cet espace."},
	"web.import.count_add":             {"%d to add", "%d à ajouter"},
	"web.import.count_update":          {"%d to update", "%d à mettre à jour"},
	"web.import.count_resume":          {"including %d to resume", "dont %d à reprendre"},
	"web.import.count_unchanged":       {"%d unchanged", "%d inchangés"},
	"web.import.access_note": {
		`The access to the repositories is checked with GitHub when applying: a renamed repository may appear here as "to add" and turn out to be already tracked; an inaccessible repository will be reported without blocking the others.`,
		"L'accès aux dépôts est vérifié auprès de GitHub au moment d'appliquer : un dépôt renommé peut apparaître ici « à ajouter » et se révéler déjà suivi ; un dépôt inaccessible sera signalé sans bloquer les autres.",
	},
	"web.import.col_action":     {"Action", "Action"},
	"web.import.col_label":      {"Display name", "Nom affiché"},
	"web.import.col_thresholds": {"Thresholds", "Seuils"},
	"web.import.no_change":      {"No repository to add or update.", "Aucun dépôt à ajouter ni à mettre à jour."},
	"web.import.unchanged":      {"Unchanged repositories (%d)", "Dépôts inchangés (%d)"},
	"web.import.masked":         {"overridden by the %s variable", "masqué par la variable %s"},
	"web.import.rewrite_note": {
		"config.yaml will be rewritten (the old file is copied to config.yaml.bak). The settings specific to this machine (name, github.*, web.*) are kept. This server will take them into account at its next start, except the language of the pages, applied right away.",
		"config.yaml sera réécrit (copie de l'ancien fichier dans config.yaml.bak). Les réglages propres à cette machine (name, github.*, web.*) sont gardés. Ce serveur les prendra en compte à son prochain lancement, sauf la langue des pages, appliquée tout de suite.",
	},
	"web.import.settings_same": {
		"The settings of the file are identical to those of this workspace: config.yaml will not be changed.",
		"Les réglages du fichier sont identiques à ceux de cet espace : config.yaml ne sera pas modifié.",
	},
	"web.import.settings_none": {
		"The file contains no setting: config.yaml will not be changed.",
		"Le fichier ne contient aucun réglage : config.yaml ne sera pas modifié.",
	},
	"web.import.apply_note": {
		"The file is checked again before being applied. No sync is started.",
		"Le fichier est vérifié de nouveau avant d'être appliqué. Aucune synchro n'est lancée.",
	},
	"web.import.apply_button": {"Apply these changes", "Appliquer ces changements"},
	"web.import.nothing_to_apply": {
		"Nothing to apply: this workspace already has this configuration.",
		"Rien à appliquer : cet espace a déjà cette configuration.",
	},
	"web.import.other_file": {"Choose another file (JSON, 1 MiB at most)", "Choisir un autre fichier (JSON, 1 Mio au plus)"},
	"web.import.no_file": {
		`No file received: choose the JSON file exported by "watchoo config export" or by the "Export" link.`,
		"Aucun fichier reçu : choisissez le fichier JSON exporté par « watchoo config export » ou par le lien « Exporter ».",
	},
	"web.import.unreadable": {
		"The file sent could not be read. Nothing was changed.",
		"Le fichier envoyé n'a pas pu être lu. Rien n'a été modifié.",
	},
	"web.import.no_document": {
		"No document to apply: send the file first to see the preview.",
		"Aucun document à appliquer : envoyez d'abord le fichier pour voir l'aperçu.",
	},
	"web.import.busy": {"Import already running", "Import déjà en cours"},
	"web.import.busy_text": {
		"Another configuration import is running: wait for it to finish, then try again. Nothing was changed.",
		"Un autre import de configuration est en cours : attendez qu'il se termine, puis recommencez. Rien n'a été modifié.",
	},
	"web.import.failed":              {"Import not possible: %s", "Import impossible : %s"},
	"web.import.nothing_imported":    {"Nothing was imported.", "Rien n'a été importé."},
	"web.import.action_add":          {"to add", "à ajouter"},
	"web.import.action_resume":       {"to resume", "à reprendre"},
	"web.import.action_update":       {"to update", "à mettre à jour"},
	"web.import.action_unchanged":    {"unchanged", "inchangé"},
	"web.import.action_inaccessible": {"inaccessible", "inaccessible"},
	"web.import.action_error":        {"in error", "en erreur"},
	"web.import.flash_resumed":       {" (including %d resumed)", " (dont %d repris)"},
	"web.import.flash_nothing":       {"Configuration imported: nothing changed.", "Configuration importée : rien n'a changé."},
	"web.import.flash_done_parts":    {"Configuration imported: %s.", "Configuration importée : %s."},
	"web.import.flash_done":          {"Configuration imported.", "Configuration importée."},
	"web.import.flash_written": {
		`config.yaml is rewritten (old file kept in %s): the imported settings will be taken into account at the next start of "watchoo serve", except the language of the pages, applied right away.`,
		"config.yaml est réécrit (ancien fichier gardé dans %s) : les réglages importés seront pris en compte au prochain lancement de « watchoo serve », sauf la langue des pages, appliquée tout de suite.",
	},
	"web.import.flash_not_imported": {
		`Not imported: %s. Check their name and the access of the gh account, then import the file again (no risk); the details are in the "watchoo serve" log and "watchoo config import --dry-run" checks each repository.`,
		"Non importés : %s. Vérifiez leur nom et l'accès du compte gh, puis importez de nouveau le fichier (sans risque) ; le détail est dans le journal de « watchoo serve » et « watchoo config import --dry-run » vérifie chaque dépôt.",
	},

	// Pages d'erreur.
	"web.error.back":      {"Back to the dashboard", "Retour au tableau de bord"},
	"web.error.not_found": {"Page not found", "Page introuvable"},
	"web.error.not_found_text": {
		"This page does not exist, or the requested repository is no longer tracked in this workspace.",
		"Cette page n'existe pas, ou le dépôt demandé n'est plus suivi dans cet espace.",
	},
	"web.error.method":      {"Action not possible", "Action impossible"},
	"web.error.method_text": {"This address does not accept this type of request.", "Cette adresse n'accepte pas ce type de requête."},
	"web.error.bad_request": {"Invalid request", "Requête invalide"},
	"web.error.internal":    {"Internal error", "Erreur interne"},
	"web.error.internal_text": {
		`An error occurred. The details are in the "watchoo serve" log.`,
		"Une erreur est survenue. Le détail est dans le journal de « watchoo serve ».",
	},
	"web.error.render": {
		`The page could not be displayed. The details are in the "watchoo serve" log.`,
		"La page n'a pas pu être affichée. Le détail est dans le journal de « watchoo serve ».",
	},
	"web.error.panic": {
		`An unexpected error occurred. The details are in the "watchoo serve" log.`,
		"Une erreur inattendue est survenue. Le détail est dans le journal de « watchoo serve ».",
	},
	"web.error.host": {"Unexpected address", "Adresse inattendue"},
	"web.error.host_text": {
		`Open the interface through http://localhost or http://127.0.0.1, with the port of "watchoo serve".`,
		"Ouvrez l'interface par http://localhost ou http://127.0.0.1, avec le port de « watchoo serve ».",
	},
	"web.error.auth":      {"Authentication required", "Authentification requise"},
	"web.error.auth_text": {"Sign in to access the Watchoo interface.", "Identifiez-vous pour accéder à l'interface de Watchoo."},
	"web.error.refused":   {"Request refused", "Requête refusée"},
	"web.error.foreign_origin": {
		"This action must be started from the Watchoo interface.",
		"Cette action doit être lancée depuis l'interface de Watchoo.",
	},
	"web.error.csrf": {
		"The security token of the page has expired: reload the page, then try again.",
		"Le jeton de sécurité de la page a expiré : rechargez la page, puis recommencez.",
	},
	"web.error.too_large":      {"Upload too large", "Envoi trop volumineux"},
	"web.error.too_large_text": {"The file sent exceeds the allowed size (1 MiB).", "Le fichier envoyé dépasse la taille autorisée (1 Mio)."},

	// Démarrage et arrêt de « watchoo serve » (langue du processus : affichés par la CLI).
	"web.start.no_store":    {"web interface: database not provided", "interface web : base non renseignée"},
	"web.start.no_sync":     {"web interface: sync coordinator not provided", "interface web : coordinateur de synchro non renseigné"},
	"web.start.no_engine":   {"web interface: sync engine not provided", "interface web : moteur de synchro non renseigné"},
	"web.start.no_listener": {"web interface: listener or handler not provided", "interface web : écoute ou handler non renseigné"},
	"web.start.empty_password": {
		"web interface: empty password for the web.auth.username login (%s variable)",
		"interface web : mot de passe vide pour l'identifiant de web.auth.username (variable %s)",
	},
	"web.start.addr_invalid":      {"web interface: invalid listen address %q", "interface web : adresse d'écoute %q invalide"},
	"web.start.templates_invalid": {"web interface: invalid common templates", "interface web : templates communs invalides"},
	"web.start.template_missing":  {"web interface: template %s missing", "interface web : template %s absent"},
	"web.start.template_invalid":  {"web interface: invalid template %s", "interface web : template %s invalide"},
	"web.start.no_block": {
		`web interface: %s does not define "%s"`,
		"interface web : %s ne définit pas « %s »",
	},
	"web.serve.server":         {"web server", "serveur web"},
	"web.serve.shutdown":       {"shutting down the web server", "arrêt du serveur web"},
	"web.serve.syncs_shutdown": {"shutting down the syncs", "arrêt des synchros"},
	"web.expose.addr_invalid": {
		"invalid listen address %q (host:port expected)",
		"adresse d'écoute %q invalide (hôte:port attendu)",
	},
	"web.expose.no_password": {
		"web.auth.username is set but the %s variable is empty: set the password in this variable (never in a settings file)",
		"web.auth.username est réglé mais la variable %s est vide : définissez le mot de passe dans cette variable (jamais dans un fichier de réglages)",
	},
	"web.expose.no_username": {
		"the %s variable is set but web.auth.username is empty: set the login in config.yaml",
		"la variable %s est définie mais web.auth.username est vide : réglez l'identifiant dans config.yaml",
	},
	"web.expose.short_password": {
		"password in %s too short: at least %d characters",
		"mot de passe de %s trop court : au moins %d caractères",
	},
	"web.expose.proxy_no_hosts": {
		"listening on %s behind a reverse proxy (web.trusted_proxy) refused without web.allowed_hosts: list the public name(s) that the proxy passes in the Host header (protection against DNS rebinding)",
		"écoute sur %s derrière un reverse proxy (web.trusted_proxy) refusée sans web.allowed_hosts : listez le ou les noms publics que le proxy transmet dans l'en-tête Host (protection contre le DNS rebinding)",
	},
	"web.expose.proxy_warning": {
		"listening on %s without basic authentication: web.trusted_proxy declares that an authenticating reverse proxy protects the access; without it, the data of the private repositories would be exposed",
		"écoute sur %s sans authentification basique : web.trusted_proxy déclare qu'un reverse proxy authentifiant protège l'accès ; sans lui, les données des dépôts privés seraient exposées",
	},
	"web.expose.refused": {
		"listening on %s (outside the loopback) refused without authentication: set web.auth.username and the %s variable, or web.trusted_proxy: true (with web.allowed_hosts) behind an authenticating reverse proxy, or listen on 127.0.0.1",
		"écoute sur %s (hors de la boucle locale) refusée sans authentification : réglez web.auth.username et la variable %s, ou web.trusted_proxy: true (avec web.allowed_hosts) derrière un reverse proxy authentifiant, ou écoutez sur 127.0.0.1",
	},
}

func init() {
	register(webTexts)
}
