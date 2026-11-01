// Textes du paquet internal/tui (« watchoo tui ») : en-tête et onglets, colonnes des listes, bas de
// l'écran et aide des touches, saisie d'ajout d'un dépôt, détail d'un dépôt et d'un contributeur,
// progression et bilans de synchro, avis de nouvelle version. Préfixe des clés : « tui. ». Les
// libellés et dates d'activité viennent de activity., le séparateur « : » de cli.core.err.sep et
// les comptages communs avec la CLI (dépôts, nouveaux commits, reportés, avertissements) de
// cli.sync. Les noms des touches (s, S, a, r, q, ?) ne se traduisent pas.
package i18n

// tuiTexts regroupe les textes de internal/tui, enregistrés dans le catalogue par le init()
// ci-dessous.
var tuiTexts = map[string]entry{
	// Onglets (vues 1, 2, 3) ; « Dépôts » sert aussi de titre de section dans le détail d'un
	// contributeur.
	"tui.view.repos":        {"Repositories", "Dépôts"},
	"tui.view.commits":      {"Commits", "Commits"},
	"tui.view.contributors": {"Contributors", "Contributeurs"},

	// État de la synchro dans l'en-tête (header.go). La première lettre est mise en majuscule.
	"tui.header.sync_running":      {"Sync in progress", "Synchro en cours"},
	"tui.header.sync_auto_running": {"Automatic sync in progress", "Synchro automatique en cours"},
	"tui.header.sync_progress":     {": %d/%d repositories", " : %d/%d dépôts"},
	"tui.header.last_sync":         {"last sync %s", "dernière synchro %s"},
	"tui.header.auto":              {"auto sync every %s, next at %s", "synchro auto toutes les %s, prochaine à %s"},
	"tui.header.manual":            {"manual sync (s)", "synchro manuelle (s)"},

	// Avis de nouvelle version, sous l'en-tête (views.go) : phrase complète, puis forme courte pour
	// un terminal étroit.
	"tui.update.full": {
		"A new version of Watchoo is available: %s %s %s. %s",
		"Nouvelle version de Watchoo disponible : %s %s %s. %s",
	},
	"tui.update.short":     {"Watchoo %s available: %s", "Watchoo %s disponible : %s"},
	"tui.update.how":       {`Quit, then run "watchoo update".`, "Quittez puis lancez « watchoo update »."},
	"tui.update.how_short": {`quit, then run "watchoo update".`, "quittez puis lancez « watchoo update »."},
	"tui.update.docker": {
		"Change the tag of the Docker image to update it.",
		"Changez le tag de l'image Docker pour la mettre à jour.",
	},
	"tui.update.docker_short": {"change the tag of the Docker image.", "changez le tag de l'image Docker."},

	// Bas de l'écran (views.go) : filtre, rappel court des touches.
	"tui.footer.filter":      {"Filter: ", "Filtre : "},
	"tui.footer.filter_keys": {"  (enter to keep, esc to clear)", "  (entrée pour garder, échap pour effacer)"},
	"tui.footer.filter_set":  {"Filter: %s (esc to clear it)", "Filtre : %s (échap pour l'effacer)"},
	"tui.short.views":        {"views", "vues"},
	"tui.short.detail":       {"detail", "détail"},
	"tui.short.filter":       {"filter", "filtre"},
	"tui.short.sync":         {"sync", "synchro"},
	"tui.short.sync_repo":    {"sync repo", "synchro du dépôt"},
	"tui.short.add":          {"add", "ajouter"},
	"tui.short.help":         {"help", "aide"},
	"tui.short.quit":         {"quit", "quitter"},

	// Listes (views.go) : chargement, liste vide, en-têtes de colonnes (en capitales).
	"tui.loading":            {"Loading", "Chargement"},
	"tui.list.load_error":    {`Cannot read the database. "r" to retry.`, "Impossible de lire la base. « r » pour réessayer."},
	"tui.empty.filter":       {`No row matches the filter "%s" (esc to clear it).`, "Aucune ligne ne correspond au filtre « %s » (échap pour l'effacer)."},
	"tui.empty.repos":        {`No tracked repository. Add some with "a" (or "watchoo repo add owner/repo").`, "Aucun dépôt suivi. Ajoutez-en avec « a » (ou « watchoo repo add owner/repo »)."},
	"tui.empty.commits":      {`No commit in the database. "s" starts a sync.`, "Aucun commit en base. « s » lance une synchro."},
	"tui.empty.contributors": {`No contributor in the database. "s" starts a sync.`, "Aucun contributeur en base. « s » lance une synchro."},
	"tui.col.repo":           {"REPOSITORY", "DÉPÔT"},
	"tui.col.repos":          {"REPOS", "DÉPÔTS"},
	"tui.col.last_commit":    {"LAST COMMIT", "DERNIER COMMIT"},
	"tui.col.author":         {"AUTHOR", "AUTEUR"},
	"tui.col.branches":       {"BRANCHES", "BRANCHES"},
	"tui.col.state":          {"STATE", "ÉTAT"},
	"tui.col.when":           {"WHEN", "QUAND"},
	"tui.col.message":        {"MESSAGE", "MESSAGE"},
	"tui.col.contributor":    {"CONTRIBUTOR", "CONTRIBUTEUR"},
	"tui.col.login":          {"LOGIN", "LOGIN"},
	"tui.col.commits":        {"COMMITS", "COMMITS"},
	"tui.col.weeks":          {"%d WK", "%d SEM."},
	"tui.col.per_week":       {"PER WEEK", "PAR SEMAINE"},

	// Écran d'aide (views.go, keys.go) : titre, groupes, libellés des touches, description.
	"tui.help.title":         {"Keys", "Aide des touches"},
	"tui.help.close":         {`  ("?" or esc to close)`, "  (« ? » ou échap pour fermer)"},
	"tui.help.filter_note":   {`The filter ("/") applies to the columns of the current view.`, "Le filtre (« / ») porte sur les colonnes de la vue courante."},
	"tui.help.group.views":   {"Views", "Vues"},
	"tui.help.group.move":    {"Navigation", "Déplacement"},
	"tui.help.group.sync":    {"Sync and data", "Synchro et données"},
	"tui.help.group.general": {"General", "Général"},
	"tui.keyname.shift_tab":  {"shift+tab", "maj+tab"},
	"tui.keyname.pgup":       {"pgup", "pg.préc"},
	"tui.keyname.pgdown":     {"pgdown", "pg.suiv"},
	"tui.keyname.enter":      {"enter", "entrée"},
	"tui.keyname.esc":        {"esc", "échap"},
	"tui.key.next_view":      {"next view", "vue suivante"},
	"tui.key.prev_view":      {"previous view", "vue précédente"},
	"tui.key.view_repos":     {"repositories view", "vue dépôts"},
	"tui.key.view_commits":   {"commits view", "vue commits"},
	"tui.key.view_contribs":  {"contributors view", "vue contributeurs"},
	"tui.key.up":             {"up", "monter"},
	"tui.key.down":           {"down", "descendre"},
	"tui.key.page_up":        {"previous page", "page précédente"},
	"tui.key.page_down":      {"next page", "page suivante"},
	"tui.key.top":            {"top of the list", "début de la liste"},
	"tui.key.bottom":         {"end of the list", "fin de la liste"},
	"tui.key.open":           {"repository or contributor detail", "détail du dépôt ou du contributeur"},
	"tui.key.back":           {"back, clear the filter", "retour, effacer le filtre"},
	"tui.key.filter":         {"filter", "filtrer"},
	"tui.key.sync_all":       {"sync everything", "synchroniser tout"},
	"tui.key.sync_repo":      {"sync this repository", "synchroniser ce dépôt"},
	"tui.key.add":            {"add a repository (Repositories view)", "ajouter un dépôt (vue Dépôts)"},
	"tui.key.reload":         {"reload the database", "relire la base"},
	"tui.key.help":           {"help", "aide"},
	"tui.key.quit":           {"quit", "quitter"},

	// Messages des touches (model.go).
	"tui.err.load":        {"cannot read the database: %s", "lecture de la base impossible : %s"},
	"tui.err.no_repo":     {"no repository selected: pick one in the Repositories or Commits view", "aucun dépôt sélectionné : choisissez-en un dans la vue Dépôts ou Commits"},
	"tui.err.not_tracked": {`this repository is no longer tracked: reload the database ("r")`, "ce dépôt n'est plus suivi : relisez la base (« r »)"},

	// Ajout d'un dépôt (add.go), avec les mots de « watchoo repo add ».
	"tui.add.prompt":          {"Repository to track (owner/name or GitHub URL): ", "Dépôt à suivre (owner/nom ou URL GitHub) : "},
	"tui.add.footer":          {"enter to add, esc to cancel", "entrée pour ajouter, échap pour annuler"},
	"tui.add.only_repos_view": {`"a" adds a repository from the list of the Repositories view (key 1)`, "« a » ajoute un dépôt depuis la liste de la vue Dépôts (touche 1)"},
	"tui.add.unavailable":     {"adding a repository is unavailable", "ajout de dépôt indisponible"},
	"tui.add.busy":            {"an addition is already in progress (%s): please wait", "un ajout est déjà en cours (%s) : patientez"},
	"tui.add.cancelled":       {"addition cancelled", "ajout annulé"},
	"tui.add.checking":        {"adding %s: checking access%s", "ajout de %s : vérification de l'accès%s"},
	"tui.add.missing_result":  {"adding repository %s: missing result", "ajout du dépôt %s : résultat manquant"},
	"tui.add.inaccessible": {
		"repository %s is inaccessible: %s (check its name and that the gh account, or without gh this machine's SSH key or Git credentials, can access it)",
		"dépôt %s inaccessible : %s (vérifiez son nom et que le compte gh, ou sans gh la clé SSH ou les identifiants Git de cette machine, y a accès)",
	},
	"tui.add.error":      {"adding repository %s: %s", "ajout du dépôt %s : %s"},
	"tui.add.added":      {"%s added", "%s ajouté"},
	"tui.add.resumed":    {"%s already tracked: tracking resumed", "%s déjà suivi : suivi repris"},
	"tui.add.unchanged":  {"%s already tracked: nothing to change", "%s déjà suivi : rien à changer"},
	"tui.add.renamed":    {" (%s is now called %s on GitHub)", " (%s s'appelle maintenant %s sur GitHub)"},
	"tui.add.sync_after": {"%s: its sync will start after the current sync (%s)", "%s : sa synchro commencera après la synchro en cours (%s)"},
	"tui.add.syncing":    {"%s: syncing %s%s", "%s : synchro de %s%s"},

	// Détails d'un dépôt et d'un contributeur (detail.go, contributor.go) : textes communs.
	"tui.detail.back":           {"  esc to go back", "  échap pour revenir"},
	"tui.detail.read_error":     {"cannot read: %s", "lecture impossible : %s"},
	"tui.detail.recent_commits": {"Latest commits", "Derniers commits"},
	"tui.detail.no_commit":      {"No commit.", "Aucun commit."},
	"tui.field.last_commit":     {"Last commit", "Dernier commit"},
	"tui.count.commit_one":      {"%d commit", "%d commit"},
	"tui.count.commit_many":     {"%d commits", "%d commits"},
	"tui.lines.uncounted":       {" (%d uncounted)", " (%d non comptés)"},

	// Détail d'un dépôt (detail.go).
	"tui.detail.gone":            {"This repository is no longer tracked. esc to go back to the list.", "Ce dépôt n'est plus suivi. échap pour revenir à la liste."},
	"tui.field.url":              {"URL", "Adresse"},
	"tui.field.state":            {"State", "État"},
	"tui.field.branches":         {"Branches", "Branches"},
	"tui.field.commits_db":       {"Commits stored", "Commits en base"},
	"tui.field.last_sync":        {"Last sync", "Dernière synchro"},
	"tui.field.on_github":        {"On GitHub", "Sur GitHub"},
	"tui.field.pulls":            {"Pull requests", "Pull requests"},
	"tui.detail.thresholds":      {"  thresholds: Slacking after %d d, Dead after %d d%s", "  seuils : Flemme après %d j, Dead après %d j%s"},
	"tui.detail.own_thresholds":  {" (specific to the repository)", " (propres au dépôt)"},
	"tui.detail.last_commit_by":  {" by %s (%s)", " par %s (%s)"},
	"tui.detail.branch_one":      {"%d tracked branch", "%d branche suivie"},
	"tui.detail.branch_many":     {"%d tracked branches", "%d branches suivies"},
	"tui.detail.default_branch":  {", default: %s", ", par défaut : %s"},
	"tui.detail.patterns":        {", patterns: %s", ", motifs : %s"},
	"tui.detail.all_branches":    {", all branches", ", toutes les branches"},
	"tui.detail.commits_lines":   {"%d, lines %s", "%d, lignes %s"},
	"tui.detail.no_forge":        {"stars and issues not available (local Git clone)", "étoiles et issues non disponibles (clone Git local)"},
	"tui.detail.star_one":        {"%d star", "%d étoile"},
	"tui.detail.star_many":       {"%d stars", "%d étoiles"},
	"tui.detail.issue_one":       {"%d open issue", "%d issue ouverte"},
	"tui.detail.issue_many":      {"%d open issues", "%d issues ouvertes"},
	"tui.detail.archived":        {", archived", ", archivé"},
	"tui.detail.stopped":         {", tracking stopped", ", suivi arrêté"},
	"tui.detail.pulls_no_forge":  {"not available (local Git clone, without gh)", "non disponibles (clone Git local, sans gh)"},
	"tui.detail.pulls_open_one":  {"%d open", "%d ouverte"},
	"tui.detail.pulls_open_many": {"%d open", "%d ouvertes"},
	"tui.detail.pulls_none":      {"none open", "aucune ouverte"},
	"tui.detail.no_release":      {", no release", ", aucune release"},
	"tui.detail.latest_release":  {", latest release %s (%s)", ", dernière release %s (%s)"},
	"tui.detail.run_running":     {", in progress", ", en cours"},
	"tui.detail.run_success":     {", successful (%s)", ", réussie (%s)"},
	"tui.detail.top_contribs":    {"Top contributors", "Principaux contributeurs"},
	"tui.detail.activity":        {"  activity over %d weeks, most recent on the right", "  activité sur %d semaines, la plus récente à droite"},

	// Détail d'un contributeur (contributor.go).
	"tui.cdetail.title":        {"Contributor", "Contributeur"},
	"tui.cdetail.no_account":   {"no known GitHub account (Git identity only)", "aucun compte GitHub connu (identité Git seule)"},
	"tui.field.account":        {"Account", "Compte"},
	"tui.field.identities":     {"Git identities", "Identités Git"},
	"tui.field.commits":        {"Commits", "Commits"},
	"tui.field.first_commit":   {"First commit", "Premier commit"},
	"tui.field.weeks":          {"%d weeks", "%d semaines"},
	"tui.cdetail.address_one":  {"%d linked address", "%d adresse rattachée"},
	"tui.cdetail.address_many": {"%d linked addresses", "%d adresses rattachées"},
	"tui.cdetail.not_shown":    {" (not shown)", " (non affichées)"},
	"tui.cdetail.commits_in":   {"%s in %s, %s", "%s dans %s, %s"},
	"tui.cdetail.latest_right": {", most recent week on the right", ", la semaine la plus récente à droite"},
	"tui.cdetail.no_commits":   {"No commit authored by them.", "Aucun commit dont il ou elle est l'auteur."},
	"tui.cdetail.lines_note": {
		`+/- lines: counted in the local Git clone of each repository ("-": not counted yet, clone unavailable).`,
		"Lignes +/- : comptées dans le clone Git local de chaque dépôt (« - » : pas encore comptées, clone indisponible).",
	},

	// Synchros lancées par le TUI (syncer.go) : demande, bilan d'une ligne, problème d'un dépôt.
	"tui.sync.busy":            {"a sync is already running (%s): please wait", "une synchro est déjà en cours (%s) : patientez"},
	"tui.sync.unavailable":     {"sync unavailable", "synchro indisponible"},
	"tui.sync.all_repos":       {"all repositories", "tous les dépôts"},
	"tui.sync.repo_number":     {"repository #%d", "dépôt n° %d"},
	"tui.sync.starting":        {"syncing %s%s", "synchro de %s%s"},
	"tui.sync.starting_auto":   {"automatic sync of %s%s", "synchro automatique de %s%s"},
	"tui.sync.failed":          {"sync failed: %s", "synchro impossible : %s"},
	"tui.sync.done_none":       {"sync finished: no tracked repository", "synchro terminée : aucun dépôt suivi"},
	"tui.sync.done":            {"sync finished: %s, %s", "synchro terminée : %s, %s"},
	"tui.sync.locked_count":    {"%d already running elsewhere", "%d déjà en cours ailleurs"},
	"tui.sync.deferred_resume": {" (low GitHub quota, resuming around %s)", " (quota GitHub bas, reprise vers %s)"},
	"tui.sync.warnings_what":   {" (pull requests, releases or tags)", " (pull requests, releases ou tags)"},
	"tui.sync.deferred_until":  {"deferred: low GitHub quota until %s", "reporté : quota GitHub bas jusqu'à %s"},
	"tui.sync.deferred":        {"deferred: low GitHub quota", "reporté : quota GitHub bas"},
	"tui.sync.locked":          {"already being synced", "déjà en cours de synchronisation"},
	"tui.sync.repo_failed":     {"sync failed", "échec de la synchro"},

	// Erreurs de Run (tui.go), renvoyées à la CLI.
	"tui.err.run":          {"interactive mode: %w", "mode interactif : %w"},
	"tui.err.sync_timeout": {"the running sync did not stop in time", "la synchro en cours ne s'est pas arrêtée à temps"},

	// Refus hors terminal (tui.go, ErrNotTerminal).
	"tui.err.not_terminal": {
		`"watchoo tui" must be run in an interactive terminal (use the other commands, such as "watchoo repo list", in a script)`,
		"« watchoo tui » doit être lancé dans un terminal interactif (utilisez les autres commandes, comme « watchoo repo list », dans un script)",
	},
}

func init() {
	register(tuiTexts)
}
