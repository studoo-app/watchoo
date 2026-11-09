// Textes du paquet internal/cli pour « watchoo conventions » (phase 8.1) : aide, rappel, tableaux des
// dépôts, des contributeurs et des commits, libellés des codes d'écart (CC2) et des motifs d'exclusion
// (CC4). Préfixe des clés : « cli.conv. ». Les clés et valeurs JSON (codes, motifs, sources) ne passent
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

Commit messages are never shown: short SHA, author and issue codes only. The conventions must be
enabled in the workspace (conventions.enabled); the exit code does not depend on the rates.

Options:
  --commits       list the evaluated commits
  --failing       only the commits that do not conform (implies --commits)
  --contributor   one author (exact login, name or e-mail)`,
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

Les messages de commit ne sont jamais affichés : SHA court, auteur et codes d'écart seulement. Les
conventions doivent être activées dans l'espace (conventions.enabled) ; le code de sortie ne dépend
pas des taux.

Options :
  --commits       liste les commits évalués
  --failing       seulement les commits non conformes (implique --commits)
  --contributor   un auteur (login, nom ou e-mail exact)`,
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
}

func init() {
	register(cliConventionsTexts)
}
