// Textes de l'interface web des conventions (1.7.0, phase 8.1, internal/web/repo_conventions.go, section
// « repo-conventions » de repo.html et ligne de contributor.html). Préfixe des clés : « web.conv. ».
// Séparés du diagnostic : aucune clé « web.diag. » n'est réutilisée.
package i18n

// webConventionsTexts regroupe les textes des conventions, enregistrés par init() ci-dessous.
var webConventionsTexts = map[string]entry{
	"web.conv.title": {"Conventions", "Conventions"},
	"web.conv.notice": {
		"Rates n/m of the commits evaluated against the conventions of the workspace, for the trainer only: not a grade. Commit messages are never shown.",
		"Taux n/m des commits évalués selon les conventions de l'espace, réservés au formateur : pas une note. Les messages de commit ne sont jamais affichés.",
	},
	"web.conv.unknown": {
		"Unknown: the repository has not been synced yet.",
		"Inconnu : le dépôt n'a pas encore été synchronisé.",
	},
	"web.conv.none": {
		"No commit evaluated (all excluded, or written before the start date).",
		"Aucun commit évalué (tous exclus, ou écrits avant la date de début).",
	},
	"web.conv.conform":      {"Conforming commits", "Commits conformes"},
	"web.conv.conform_desc": {"all the tracked branches", "toutes les branches suivies"},
	"web.conv.default_branch": {
		"Default branch (%s)",
		"Branche par défaut (%s)",
	},
	"web.conv.default_desc": {"for information", "pour information"},
	"web.conv.excluded": {
		"Excluded commits, counted apart: %d",
		"Commits exclus, comptés à part : %d",
	},
	"web.conv.before_since": {
		"Commits written before the start date, not evaluated: %d",
		"Commits écrits avant la date de début, non évalués : %d",
	},
	"web.conv.since":                      {"Start date: %s (%s)", "Date de début : %s (%s)"},
	"web.conv.since_origin.workspace":     {"workspace", "espace"},
	"web.conv.since_origin.repository":    {"own to the repository", "propre au dépôt"},
	"web.conv.issues_caption":             {"Issues by code", "Écarts par code"},
	"web.conv.col_code":                   {"Code", "Code"},
	"web.conv.col_severity":               {"Severity", "Gravité"},
	"web.conv.col_codes":                  {"Codes", "Codes"},
	"web.conv.failing_caption":            {"Latest non-conforming commits (%d of %d)", "Derniers commits non conformes (%d sur %d)"},
	"web.conv.severity.blocking":          {"blocking", "bloquant"},
	"web.conv.severity.warning":           {"warning", "avertissement"},
	"web.conv.exclusion.merge":            {"merges", "fusions"},
	"web.conv.exclusion.bot":              {"bots", "bots"},
	"web.conv.exclusion.initial_commit":   {"GitHub initial commit", "commit initial de GitHub"},
	"web.conv.exclusion.excluded_account": {"excluded accounts", "comptes exclus"},
	"web.conv.source_clone": {
		"Scope: all the tracked branches, read from the local clone, each commit once.",
		"Périmètre : toutes les branches suivies, lues dans le clone local, chaque commit une fois.",
	},
	"web.conv.source_database": {
		"Scope: commits attached to the tracked branches in the database (no readable local clone); may include rewritten commits.",
		"Périmètre : commits rattachés aux branches suivies en base (pas de clone local lisible) ; peut inclure des commits réécrits.",
	},
	"web.conv.parents_unknown": {
		"%d commits with an unknown parent count: merges recognized by their default message.",
		"%d commits au nombre de parents inconnu : fusions reconnues à leur message par défaut.",
	},
	"web.conv.contributor_line": {
		"Conforming commits %d/%d (%s)",
		"Commits conformes %d/%d (%s)",
	},
	"web.conv.contributor_none": {
		"Conforming commits: no commit evaluated",
		"Commits conformes : aucun commit évalué",
	},
	"web.conv.contributor_note": {
		"Conventions, all the repositories: commits by this person, %d excluded (merges, bots…); a rate, not a grade.",
		"Conventions, tous les dépôts : commits dont la personne est l'auteur, %d exclus (fusions, bots…) ; un taux, pas une note.",
	},

	"web.conv.code.empty":            {"empty message or description", "message ou description vide"},
	"web.conv.code.forbidden":        {"forbidden subject (wip, update, fixup!…)", "sujet interdit (wip, maj, fixup!…)"},
	"web.conv.code.github_default":   {"message proposed by GitHub", "message proposé par GitHub"},
	"web.conv.code.no_type":          {"no \"type:\" header", "pas d'en-tête « type: »"},
	"web.conv.code.unknown_type":     {"type not allowed", "type non admis"},
	"web.conv.code.type_case":        {"type with the wrong case", "type à la mauvaise casse"},
	"web.conv.code.scope":            {"scope badly formed or not allowed", "scope mal formé ou non admis"},
	"web.conv.code.separator":        {"separator other than \": \"", "séparateur autre que « : »"},
	"web.conv.code.trailing_period":  {"subject ending with a period", "sujet terminé par un point"},
	"web.conv.code.body_separator":   {"no blank line before the body", "pas de ligne vide avant le corps"},
	"web.conv.code.length_hard":      {"subject too long", "sujet trop long"},
	"web.conv.code.desc_capitalized": {"description starting with a capital", "description commençant par une majuscule"},
	"web.conv.code.length_soft":      {"long subject", "sujet long"},
	"web.conv.code.footer_format":    {"badly formed footer", "pied de message mal formé"},
	"web.conv.code.git_revert":       {"revert produced by Git", "revert produit par Git"},
}

func init() {
	register(webConventionsTexts)
}
