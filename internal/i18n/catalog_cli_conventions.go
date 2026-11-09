// Textes du paquet internal/cli pour « watchoo conventions » (phases 8.1 à 8.4) : aide, rappel, tableaux
// des dépôts, des contributeurs et des commits, libellés des codes d'écart (CC2) et des motifs d'exclusion
// (CC4), puis lignes de branches (BR1), PR (PR1, PR2), tags et jalons (SV1, SV2), releases (SV6),
// formulation, pieds et langue (CC5 à CC7), intervalles entre tags et revue de jalon. Préfixe des clés : « cli.conv. ». Les clés et valeurs JSON (codes, motifs, sources) ne passent
// pas par le catalogue (schéma stable) ; seul le rappel « notice » y est traduit.
package i18n

// cliConventionsTexts regroupe les textes de « watchoo conventions », enregistrés par init() ci-dessous.
var cliConventionsTexts = map[string]entry{
	"cli.conv.short": {
		"Shows the rates of conventional commits of the tracked repositories (trainer only)",
		"Affiche les taux de commits conformes aux conventions des dépôts suivis (réservé au formateur)",
	},
	"cli.conv.long": {
		`Shows, for the trainer who supervises the workspace, how the commit messages of the tracked
repositories follow the Conventional Commits charter (conventions.commit.* settings). There is no
grade: each rate n/m gives its denominator and its exclusions.

  CC1  rate of conforming commits: commits without blocking issue / commits evaluated
  CC2  issues by code: blocking (empty, forbidden, github_default, no_type, unknown_type, type_case,
       scope, separator, trailing_period, body_separator, length_hard) or warning
       (desc_capitalized, length_soft, footer_format, git_revert)
  CC3  rate per contributor
  CC4  exclusions, counted apart: merges, bots, "Initial commit" created by GitHub, and the
       accounts of conventions.excluded_accounts

Scope: all the tracked branches, each commit once, read from the local clone (git rev-list) when it
exists; otherwise the commits attached to the tracked branches in the database, which may include
rewritten commits. Commits written before the start date (conventions.since, or the repository's own
date set by "watchoo repo add --conventions-since") are counted apart. The default branch alone is
shown for information.

Each repository also gives: the branch names (BR1: remote branches of the clone and head branches of
the pull requests, outside the default branch and conventions.branch.exclude), the pull request
titles (PR1 conventional title, PR2 default title; open and merged apart, API mode only), the tag
names (SV1, SemVer with a lowercase "v"), the configured milestones (SV2, conventions.tags.milestones),
the releases (SV6, information only), the wording of the subjects (CC5, warnings), the footers and
issue references (CC6) and the language of the subjects (CC7), then CC1 per interval between final
tags (read in the clone). Per contributor: the rates before and since the last final tag (CC3
trend), the conforming pull request titles and branches.

File criteria, measured by "watchoo sync" on the local clone and read from its cache: CHANGELOG
(CL1-CL8), README (RD1-RD4), documentation (DOC1-DOC5, DOC8) and tooling (TL1-TL3), one grid per
family. With a repository (watchoo conventions owner/repo), the path lists are recomputed from the
clone: documents not referenced by the INDEX, dead links, badly named paths, archives.

History, observed by "watchoo sync" while the conventions are enabled and read from the clone: rewrites
of the default branch (BR4, never attributed to anyone), tags rewritten or deleted (SV3: "no rewrite
observed since" the first observation), the due dates of the milestones compared to the first
observation of their tag (SV2), the pre-releases (SV4, warnings), the version bumps compared to the
declared commits (SV5, under-versioning only), the precedence of the versions against the history
(SV7) and the "feat" and "fix" commits since the last change of the CHANGELOG (CL10, information).

--milestone vX.Y.Z shows the review of a configured milestone, repository by repository: CC1 on the
milestone interval, BR1, SV1, SV2 (with its due date) and SV3, then CL9, CL8, RD1, RD2 (installation section), RD3, DOC1 and
DOC3 on the tree of the milestone tag, each conform, not conform or unknown ("k/n criteria conform,
u unknown"), never an overall grade. It requires a start date (conventions.since).

Commit messages and pull request titles are never shown: short SHA, pull request number, author,
branch and tag names and codes only. The conventions must be enabled in the workspace
(conventions.enabled); the exit code does not depend on the rates.

Options:
  --commits       list the evaluated commits
  --failing       only the commits that do not conform (implies --commits)
  --contributor   one author (exact login, name or e-mail)
  --milestone     review of a configured milestone (vX.Y.Z)`,
		`Affiche, pour le formateur qui supervise l'espace, comment les messages de commit des dépôts
suivis respectent la charte Conventional Commits (réglages conventions.commit.*). Aucune note :
chaque taux n/m donne son dénominateur et ses exclusions.

  CC1  taux de commits conformes : commits sans écart bloquant / commits évalués
  CC2  écarts par code : bloquants (empty, forbidden, github_default, no_type, unknown_type,
       type_case, scope, separator, trailing_period, body_separator, length_hard) ou avertissements
       (desc_capitalized, length_soft, footer_format, git_revert)
  CC3  taux par contributeur
  CC4  exclusions, comptées à part : fusions, bots, « Initial commit » créé par GitHub et comptes de
       conventions.excluded_accounts

Périmètre : toutes les branches suivies, chaque commit une fois, lues dans le clone local (git
rev-list) quand il existe ; sinon les commits rattachés aux branches suivies en base, qui peuvent
inclure des commits réécrits. Les commits écrits avant la date de début (conventions.since, ou la date
propre au dépôt fixée par « watchoo repo add --conventions-since ») sont comptés à part. La branche
par défaut seule est donnée pour information.

Chaque dépôt donne aussi : les noms de branche (BR1 : branches du distant lues dans le clone et
branches de tête des pull requests, hors branche par défaut et conventions.branch.exclude), les
titres de pull request (PR1 titre conventionnel, PR2 titre par défaut ; ouvertes et fusionnées à part,
mode API seulement), les noms de tag (SV1, SemVer avec un « v » minuscule), les jalons configurés
(SV2, conventions.tags.milestones), les releases (SV6, information seule), la formulation des sujets
(CC5, avertissements), les pieds et références d'issue (CC6) et la langue des sujets (CC7), puis CC1
par intervalle entre tags finals (lu dans le clone). Par contributeur : les taux avant et depuis le
dernier tag final (tendance de CC3), les titres de pull request et branches conformes.

Critères de fichiers, mesurés par « watchoo sync » sur le clone local et lus dans son cache :
CHANGELOG (CL1-CL8), README (RD1-RD4), documentation (DOC1-DOC5, DOC8) et outillage (TL1-TL3), une
grille par famille. Avec un dépôt (watchoo conventions owner/repo), les listes de chemins sont
recalculées depuis le clone : documents non référencés par l'INDEX, liens morts, chemins mal nommés,
archives.

Historique, observé par « watchoo sync » quand les conventions sont activées et lu dans le clone :
réécritures de la branche par défaut (BR4, attribuées à personne), tags réécrits ou supprimés (SV3 :
« aucune réécriture observée depuis » la première observation), échéances des jalons comparées à la
première observation de leur tag (SV2), pré-versions (SV4, avertissements), sauts de version comparés
aux commits déclarés (SV5, sous-versionnement seulement), précédence des versions face à l'historique
(SV7) et commits « feat » et « fix » depuis la dernière modification du CHANGELOG (CL10, information).

--milestone vX.Y.Z affiche la revue d'un jalon configuré, dépôt par dépôt : CC1 sur l'intervalle du
jalon, BR1, SV1, SV2 (avec son échéance) et SV3, puis CL9, CL8, RD1, RD2 (section installation), RD3, DOC1 et DOC3 sur l'arbre
du tag du jalon, chacun conforme, non conforme ou inconnu (« k/n critères conformes, u inconnus »),
jamais de note globale. Elle exige une date de début (conventions.since).

Ni message de commit ni titre de pull request n'est affiché : SHA court, numéro de pull request,
auteur, noms de branche et de tag et codes seulement. Les conventions doivent être activées dans
l'espace (conventions.enabled) ; le code de sortie ne dépend pas des taux.

Options :
  --commits       liste les commits évalués
  --failing       seulement les commits non conformes (implique --commits)
  --contributor   un auteur (login, nom ou e-mail exact)
  --milestone     revue d'un jalon configuré (vX.Y.Z)`,
	},
	"cli.conv.flag_commits":     {"list the evaluated commits", "liste les commits évalués"},
	"cli.conv.flag_failing":     {"only the commits that do not conform (implies --commits)", "seulement les commits non conformes (implique --commits)"},
	"cli.conv.flag_contributor": {"one author (exact login, name or e-mail)", "un auteur (login, nom ou e-mail exact)"},
	"cli.conv.disabled": {
		"Conventions are disabled in this workspace (conventions.enabled: false).",
		"Les conventions sont désactivées dans cet espace (conventions.enabled: false).",
	},
	"cli.conv.notice": {
		"Rates n/m of the evaluated commits, not a grade; commit messages are never shown.",
		"Taux n/m des commits évalués, pas une note ; les messages de commit ne sont jamais affichés.",
	},
	"cli.conv.no_repos":       {"No tracked repository.", "Aucun dépôt suivi."},
	"cli.conv.unknown_author": {"unknown", "inconnu"},
	"cli.conv.percent":        {"%d%%", "%d %%"},
	"cli.conv.fmt.date":       {"01/02/2006", "02/01/2006"},

	"cli.conv.header_conform":       {"Conform", "Conformes"},
	"cli.conv.header_rate":          {"Rate", "Taux"},
	"cli.conv.header_default":       {"Default branch", "Branche par défaut"},
	"cli.conv.header_default_ascii": {"Default branch", "Branche par defaut"},
	"cli.conv.header_excluded":      {"Excluded", "Exclus"},
	"cli.conv.header_since":         {"Since", "Depuis"},
	"cli.conv.header_source":        {"Source", "Source"},
	"cli.conv.header_issues":        {"Issues", "Écarts"},
	"cli.conv.header_issues_ascii":  {"Issues", "Ecarts"},
	"cli.conv.header_result":        {"Result", "Résultat"},
	"cli.conv.header_result_ascii":  {"Result", "Resultat"},

	"cli.conv.source.clone":    {"clone", "clone"},
	"cli.conv.source.database": {"database", "base"},
	"cli.conv.fallback": {
		"* database: no readable local clone, the commits attached to the tracked branches; may include rewritten commits.",
		"* base : pas de clone local lisible, les commits rattachés aux branches suivies ; peut inclure des commits réécrits.",
	},
	"cli.conv.parents_unknown": {
		"%d commits with an unknown parent count: merges recognized by their default message.",
		"%d commits au nombre de parents inconnu : fusions reconnues à leur message par défaut.",
	},
	"cli.conv.since_workspace":  {"start date: %s (workspace)", "date de début : %s (espace)"},
	"cli.conv.since_repository": {"start date: %s (own to the repository)", "date de début : %s (propre au dépôt)"},
	"cli.conv.excluded_line":    {"excluded: %s", "exclus : %s"},
	"cli.conv.issue_line":       {"%s (%d): %s [%s]", "%s (%d) : %s [%s]"},
	"cli.conv.before_since": {
		"before the start date, not evaluated: %d",
		"antérieurs à la date de début, non évalués : %d",
	},
	"cli.conv.contributors_title": {"Contributors:", "Contributeurs :"},
	"cli.conv.commits_title":      {"Evaluated commits:", "Commits évalués :"},
	"cli.conv.no_commits":         {"No commit.", "Aucun commit."},
	"cli.conv.conform":            {"conform", "conforme"},
	"cli.conv.failing":            {"not conform", "non conforme"},

	"cli.conv.severity.blocking": {"blocking", "bloquant"},
	"cli.conv.severity.warning":  {"warning", "avertissement"},

	"cli.conv.exclusion.merge":            {"merges", "fusions"},
	"cli.conv.exclusion.bot":              {"bots", "bots"},
	"cli.conv.exclusion.initial_commit":   {"GitHub initial commit", "commit initial de GitHub"},
	"cli.conv.exclusion.excluded_account": {"excluded accounts", "comptes exclus"},

	"cli.conv.code.empty":            {"empty message or description", "message ou description vide"},
	"cli.conv.code.forbidden":        {"forbidden subject (wip, update, fixup!…)", "sujet interdit (wip, maj, fixup!…)"},
	"cli.conv.code.github_default":   {"message proposed by GitHub", "message proposé par GitHub"},
	"cli.conv.code.no_type":          {"no \"type:\" header", "pas d'en-tête « type: »"},
	"cli.conv.code.unknown_type":     {"type not allowed", "type non admis"},
	"cli.conv.code.type_case":        {"type with the wrong case", "type à la mauvaise casse"},
	"cli.conv.code.scope":            {"scope badly formed or not allowed", "scope mal formé ou non admis"},
	"cli.conv.code.separator":        {"separator other than \": \"", "séparateur autre que « : »"},
	"cli.conv.code.trailing_period":  {"subject ending with a period", "sujet terminé par un point"},
	"cli.conv.code.body_separator":   {"no blank line before the body", "pas de ligne vide avant le corps"},
	"cli.conv.code.length_hard":      {"subject too long", "sujet trop long"},
	"cli.conv.code.desc_capitalized": {"description starting with a capital", "description commençant par une majuscule"},
	"cli.conv.code.length_soft":      {"long subject", "sujet long"},
	"cli.conv.code.footer_format":    {"badly formed footer", "pied de message mal formé"},
	"cli.conv.code.git_revert":       {"revert produced by Git", "revert produit par Git"},

	// Phase 8.2.
	"cli.conv.flag_milestone": {"review of a configured milestone (vX.Y.Z)", "revue d'un jalon configuré (vX.Y.Z)"},
	"cli.conv.bad_milestone": {
		"unknown milestone %q: configured milestones (conventions.tags.milestones): %s",
		"jalon %q inconnu : jalons configurés (conventions.tags.milestones) : %s",
	},
	"cli.conv.header_before_tag": {"Up to last tag", "Jusqu'au dernier tag"},
	"cli.conv.header_since_tag":  {"Since last tag", "Depuis le dernier tag"},
	"cli.conv.header_pr_titles":  {"PR titles", "Titres de PR"},
	"cli.conv.header_branches":   {"Branches", "Branches"},
	"cli.conv.contributors_note": {
		"Last tag: rates up to and since the last final tag of each repository (CC3 trend, \"—\" without clone or final tag); PR titles and branches: those of the contributor's pull requests.",
		"Dernier tag : taux jusqu'au et depuis le dernier tag final de chaque dépôt (tendance de CC3, « — » sans clone ni tag final) ; titres de PR et branches : ceux des pull requests du contributeur.",
	},

	"cli.conv.branches_source.clone":    {"clone", "clone"},
	"cli.conv.branches_source.database": {"database, tracked branches only", "base, branches suivies seulement"},
	"cli.conv.branches_none":            {"branches (%s): none to evaluate", "branches (%s) : aucune à évaluer"},
	"cli.conv.branches_line": {
		"branches (%s): %d/%d conform, %d to review",
		"branches (%s) : %d/%d conformes, %d à revoir",
	},
	"cli.conv.code_names": {"%s (%d): %s [%s]: %s", "%s (%d) : %s [%s] : %s"},

	"cli.conv.branch_code.unknown_type":   {"type not allowed before \"/\"", "type non admis avant « / »"},
	"cli.conv.branch_code.uppercase":      {"uppercase letter", "lettre majuscule"},
	"cli.conv.branch_code.underscore":     {"underscore \"_\"", "tiret bas « _ »"},
	"cli.conv.branch_code.space":          {"space", "espace"},
	"cli.conv.branch_code.accent":         {"non-ASCII letter", "lettre hors ASCII"},
	"cli.conv.branch_code.no_slash":       {"no \"type/\" prefix", "pas de préfixe « type/ »"},
	"cli.conv.branch_code.pattern":        {"does not follow the pattern", "ne suit pas le motif"},
	"cli.conv.branch_code.single_segment": {"single word after \"/\", to review", "un seul mot après « / », à revoir"},

	"cli.conv.pulls_unknown": {"pull requests: unknown (%s)", "pull requests : inconnu (%s)"},
	"cli.conv.pulls_none":    {"pull requests: none evaluated", "pull requests : aucune évaluée"},
	"cli.conv.pulls_line": {
		"pull requests: conventional titles open %d/%d, merged %d/%d; default titles %d",
		"pull requests : titres conventionnels ouvertes %d/%d, fusionnées %d/%d ; titres par défaut %d",
	},
	"cli.conv.pulls_skipped":              {"; set aside: %s", " ; écartées : %s"},
	"cli.conv.pull_item":                  {"#%d %s (%s): %s", "#%d %s (%s) : %s"},
	"cli.conv.pr_skip.closed":             {"closed without merge", "fermées sans fusion"},
	"cli.conv.pr_skip.draft":              {"drafts", "brouillons"},
	"cli.conv.pr_skip.bot":                {"bots", "bots"},
	"cli.conv.pr_skip.before_since":       {"before the start date", "avant la date de début"},
	"cli.conv.tags_none":                  {"tags: none", "tags : aucun"},
	"cli.conv.tags_line":                  {"tags: %d/%d conform", "tags : %d/%d conformes"},
	"cli.conv.tag_code.no_v":              {"no \"v\" prefix", "pas de préfixe « v »"},
	"cli.conv.tag_code.upper_v":           {"uppercase \"V\"", "« V » majuscule"},
	"cli.conv.tag_code.missing_component": {"fewer than three numbers", "moins de trois nombres"},
	"cli.conv.tag_code.leading_zero":      {"number with a leading zero", "nombre avec un zéro en tête"},
	"cli.conv.tag_code.empty_prerelease":  {"empty pre-release", "pré-version vide"},
	"cli.conv.tag_code.invalid":           {"not a SemVer version", "pas une version SemVer"},
	"cli.conv.tag_code.build_metadata":    {"build metadata \"+…\"", "métadonnées de build « +… »"},

	"cli.conv.milestones_line":                        {"milestones: %s", "jalons : %s"},
	"cli.conv.milestone_status.present":               {"present", "présent"},
	"cli.conv.milestone_status.nonconforming_name":    {"nonconforming name", "nom non conforme"},
	"cli.conv.milestone_status.absent":                {"absent", "absent"},
	"cli.conv.releases_unknown":                       {"releases (information): unknown (%s)", "releases (information) : inconnu (%s)"},
	"cli.conv.releases_none":                          {"releases (information): nothing to report", "releases (information) : rien à signaler"},
	"cli.conv.releases_line":                          {"releases (information): %s", "releases (information) : %s"},
	"cli.conv.release_code.milestone_without_release": {"milestone without release", "jalon sans release"},
	"cli.conv.release_code.release_without_tag":       {"release without tag", "release sans tag"},
	"cli.conv.release_code.prerelease_mismatch":       {"inconsistent pre-release flag", "drapeau de pré-version incohérent"},

	"cli.conv.wording.past_tense":   {"past tense", "passé"},
	"cli.conv.wording.progressive":  {"\"-ing\" form", "forme en « -ing »"},
	"cli.conv.wording.first_person": {"first person", "première personne"},
	"cli.conv.wording_line":         {"wording (warnings): %s", "formulation (avertissements) : %s"},
	"cli.conv.footers_line": {
		"footers: %d/%d commits, issue references %d/%d",
		"pieds : %d/%d commits, références d'issue %d/%d",
	},
	"cli.conv.language_line":  {"subject language: fr %d, en %d, unknown %d", "langue des sujets : fr %d, en %d, inconnue %d"},
	"cli.conv.language_other": {"; %d not in %s", " ; %d hors %s"},
	"cli.conv.language_mixed": {"; mixed", " ; mélange"},

	"cli.conv.intervals_unknown":   {"CC1 between tags: unknown (%s)", "CC1 entre tags : inconnu (%s)"},
	"cli.conv.intervals_line":      {"CC1 between tags: %s", "CC1 entre tags : %s"},
	"cli.conv.interval_start":      {"start", "début"},
	"cli.conv.interval_unreadable": {"unreadable", "illisible"},
	"cli.conv.interval_missing":    {" (+%d not in the database)", " (+%d absents de la base)"},
	"cli.conv.interval_since":      {"since %s", "depuis %s"},

	"cli.conv.milestone_notice": {
		"Milestone review: repository indicators, k/n criteria conform, not a grade.",
		"Revue de jalon : indicateurs de dépôt, k/n critères conformes, pas une note.",
	},
	"cli.conv.milestone_title":         {"Milestone review %s", "Revue du jalon %s"},
	"cli.conv.milestone_due":           {" (due %s)", " (échéance %s)"},
	"cli.conv.milestone_due_bound":     {"due %s", "échéance %s"},
	"cli.conv.milestone_not_evaluable": {"%s: not evaluable (%s)", "%s : non évaluable (%s)"},
	"cli.conv.milestone_repo": {
		"%s: %d/%d criteria conform, %d unknown",
		"%s : %d/%d critères conformes, %d inconnus",
	},
	"cli.conv.milestone_branches":          {"%d/%d branches conform", "%d/%d branches conformes"},
	"cli.conv.criterion_status.conform":    {"conform", "conforme"},
	"cli.conv.criterion_status.nonconform": {"not conform", "non conforme"},
	"cli.conv.criterion_status.unknown":    {"unknown", "inconnu"},

	"cli.conv.reason.no_clone":              {"no local clone", "pas de clone local"},
	"cli.conv.reason.clone_unreadable":      {"unreadable clone", "clone illisible"},
	"cli.conv.reason.no_api":                {"no data from the GitHub API", "pas de données de l'API GitHub"},
	"cli.conv.reason.filtered":              {"report restricted to contributors", "rapport restreint à des contributeurs"},
	"cli.conv.reason.no_final_tag":          {"no conforming final tag", "aucun tag final conforme"},
	"cli.conv.reason.no_since":              {"missing start date", "date de début manquante"},
	"cli.conv.reason.no_tag":                {"milestone tag missing, no due date", "tag du jalon absent, sans échéance"},
	"cli.conv.reason.tag_absent":            {"milestone tag still missing after the due date", "tag du jalon toujours absent après l'échéance"},
	"cli.conv.reason.not_due":               {"before the due date", "avant l'échéance"},
	"cli.conv.reason.no_commits":            {"no evaluated commit", "aucun commit évalué"},
	"cli.conv.reason.partial":               {"commits not in the database", "commits absents de la base"},
	"cli.conv.reason.no_branch":             {"no branch to evaluate", "aucune branche à évaluer"},
	"cli.conv.reason.not_observed":          {"tag never seen by a complete read of the tags", "tag jamais vu par une lecture complète des tags"},
	"cli.conv.reason.deadline_undetermined": {"tagging date undetermined against the due date", "date de pose indéterminée par rapport à l'échéance"},
	"cli.conv.reason.no_base":               {"no lower final tag is an ancestor", "aucun tag final inférieur n'en est ancêtre"},
	"cli.conv.reason.low_conform":           {"fewer than 80% conforming commits", "moins de 80 % de commits conformes"},

	// Phase 8.3 : critères de fichiers (CHANGELOG, README, documentation, outillage).
	"cli.conv.reason.empty_repository": {"empty repository", "dépôt vide"},
	"cli.conv.reason.tree_truncated":   {"too many files, list truncated", "trop de fichiers, liste tronquée"},
	"cli.conv.reason.no_file":          {"file absent", "fichier absent"},
	"cli.conv.reason.blob_too_large":   {"file too large to be read", "fichier trop gros pour être lu"},
	"cli.conv.reason.blob_unreadable":  {"file not read", "fichier non lu"},
	"cli.conv.reason.no_docs_dir":      {"no documentation folder", "pas de dossier de documentation"},
	"cli.conv.reason.unread_documents": {"documents not read", "documents non lus"},
	"cli.conv.reason.not_computed": {
		"not computed yet, run watchoo sync",
		"pas encore calculé, lancez watchoo sync",
	},
	"cli.conv.reason.stale": {
		"to recompute (rules, settings or revision changed), run watchoo sync",
		"à recalculer (règles, réglages ou révision changés), lancez watchoo sync",
	},
	"cli.conv.files_unknown": {
		"File criteria (CHANGELOG, README, documentation, tooling): unknown (%s)",
		"Critères de fichiers (CHANGELOG, README, documentation, outillage) : inconnus (%s)",
	},
	"cli.conv.files_measured": {
		"File criteria measured on %s, computed %s",
		"Critères de fichiers mesurés sur %s, calculés le %s",
	},
	"cli.conv.files_line": {
		"%s: %d/%d criteria conform, %d unknown",
		"%s : %d/%d critères conformes, %d inconnus",
	},
	"cli.conv.file_criterion": {"%s %s — %s: %s", "%s %s — %s : %s"},
	"cli.conv.file_unknown":   {"%s unknown — %s (%s)", "%s inconnu — %s (%s)"},
	"cli.conv.file_counts":    {"words %d (minimum %d)", "mots %d (minimum %d)"},
	"cli.conv.file_sections":  {"sections found %d/%d", "sections trouvées %d/%d"},
	"cli.conv.lists_unavailable": {
		"Path lists unavailable (%s)",
		"Listes de chemins indisponibles (%s)",
	},
	"cli.conv.list.unreferenced": {
		"Documents not referenced by the INDEX (DOC3, %d): %s",
		"Documents non référencés par l'INDEX (DOC3, %d) : %s",
	},
	"cli.conv.list.duplicate_rows": {
		"Documents listed by several INDEX rows (DOC3, %d): %s",
		"Documents cités par plusieurs lignes de l'INDEX (DOC3, %d) : %s",
	},
	"cli.conv.list.dead_links": {"Dead links (DOC4, %d): %s", "Liens morts (DOC4, %d) : %s"},
	"cli.conv.list.bad_names":  {"Badly named (DOC5, %d): %s", "Mal nommés (DOC5, %d) : %s"},
	"cli.conv.list.archives":   {"Archives (DOC8, %d): %s", "Archives (DOC8, %d) : %s"},
	"cli.conv.list.office_documents": {
		"Office documents under the documentation (DOC8, information, %d): %s",
		"Documents bureautiques sous la documentation (DOC8, information, %d) : %s",
	},
	"cli.conv.list_more":      {"… and %d more", "… et %d de plus"},
	"cli.conv.dead_link_case": {"case only", "casse seulement"},
	"cli.conv.milestone_readme": {
		"README sections at the tag: %s",
		"Sections du README au tag : %s",
	},
	"cli.conv.milestone_readme_none": {"none found", "aucune trouvée"},

	// Phase 8.4 (libellés des codes et états : clés « conv. » communes avec le web).
	"cli.conv.history_unknown": {"%s: unknown (%s)", "%s : inconnu (%s)"},
	"cli.conv.history_item":    {"%s: %s (%s)", "%s : %s (%s)"},
	"cli.conv.label_value":     {"%s: %s", "%s : %s"},
	"cli.conv.sep":             {"; ", " ; "},
	"cli.conv.deadlines_line":  {"due dates: %s", "échéances : %s"},
}

func init() {
	register(cliConventionsTexts)
}
