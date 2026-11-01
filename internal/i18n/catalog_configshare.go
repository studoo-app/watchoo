// Textes du paquet internal/configshare : erreurs de lecture et de validation d'un fichier de
// configuration partagée, aperçu et application d'un import. Préfixe des clés : « configshare. ».
// Ces messages sont affichés par la CLI (« watchoo config export / import ») et le web.
package i18n

// configshareTexts regroupe les textes de internal/configshare, enregistrés dans le catalogue par le
// init() ci-dessous.
var configshareTexts = map[string]entry{
	// Ponctuation : « texte : détail » en français, "text: detail" en anglais.
	"configshare.sep":       {": ", " : "},
	"configshare.semicolon": {"; ", " ; "},

	// Erreurs sentinelles (errors.Is), rendues au moment de l'affichage.
	"configshare.err.too_large": {
		"configuration file too large (1 MiB at most)",
		"fichier de configuration trop gros (1 Mio au plus)",
	},
	"configshare.err.unsupported_format": {
		"this file is not a shared Watchoo configuration",
		"ce fichier n'est pas une configuration Watchoo partagée",
	},
	"configshare.err.newer_version": {
		"file created by a newer version of Watchoo",
		"fichier créé par une version plus récente de Watchoo",
	},
	"configshare.err.invalid": {
		"invalid configuration file",
		"fichier de configuration invalide",
	},

	// Export et écriture JSON.
	"configshare.export.no_workspace": {
		"configuration export: workspace or database not provided",
		"export de la configuration : espace ou base non renseigné",
	},
	"configshare.export.read_repos": {
		"configuration export: reading the repositories",
		"export de la configuration : lecture des dépôts",
	},
	"configshare.marshal": {
		"writing the configuration as JSON",
		"écriture de la configuration en JSON",
	},

	// Lecture du document (Parse) et en-tête.
	"configshare.parse.read": {
		"reading the configuration file",
		"lecture du fichier de configuration",
	},
	"configshare.parse.trailing": {
		"text after the JSON object (a single object expected)",
		"texte après l'objet JSON (un seul objet attendu)",
	},
	"configshare.parse.format_key": {
		"(format key missing or different from %q)",
		"(clé format absente ou différente de %q)",
	},
	"configshare.parse.version_missing": {
		"version key missing",
		"clé version absente",
	},
	"configshare.parse.version_int": {
		"version: integer expected",
		"version : nombre entier attendu",
	},
	"configshare.parse.version_newer": {
		"(format version %d, this version of Watchoo reads version %d): update Watchoo",
		"(format version %d, cette version de Watchoo lit la version %d) : mettez Watchoo à jour",
	},
	"configshare.parse.version_invalid": {
		"invalid version %d (%d expected)",
		"version %d invalide (%d attendue)",
	},

	// Erreurs de encoding/json, décrites simplement.
	"configshare.json.syntax": {
		"invalid JSON (byte %d)",
		"JSON invalide (octet %d)",
	},
	"configshare.json.incomplete": {
		"incomplete or empty JSON",
		"JSON incomplet ou vide",
	},
	"configshare.json.want_object": {
		"JSON object expected, got %s",
		"objet JSON attendu, reçu %s",
	},
	"configshare.json.type": {
		"%s: type %s refused (expected: %s)",
		"%s : type %s refusé (attendu : %s)",
	},
	"configshare.json.unknown_key": {
		"unknown key %s (only the keys of the %s v%d format are accepted; github.* and web.* are never shared)",
		"clé inconnue %s (seules les clés du format %s v%d sont acceptées ; github.* et web.* ne se partagent jamais)",
	},
	"configshare.json.exported_at": {
		`exported_at: RFC 3339 date expected (e.g. "2026-10-27T10:00:00Z")`,
		"exported_at : date RFC 3339 attendue (ex. « 2026-10-27T10:00:00Z »)",
	},
	"configshare.json.unreadable": {
		"unreadable JSON",
		"JSON illisible",
	},
	"configshare.type.text":   {"text", "texte"},
	"configshare.type.int":    {"integer", "nombre entier"},
	"configshare.type.bool":   {"true or false", "true ou false"},
	"configshare.type.list":   {"list", "liste"},
	"configshare.type.object": {"object", "objet"},

	// Validation du contenu (validate, parseSettings, parseBranches, checkText).
	"configshare.v.field": {
		"%s: %s",
		"%s : %s",
	},
	"configshare.v.too_many_repos": {
		"%d repositories, %d at most",
		"%d dépôts, %d au plus",
	},
	"configshare.v.repo_n": {
		"repository #%d",
		"dépôt n° %d",
	},
	"configshare.v.repo_n_ref": {
		"repository #%d (%s)",
		"dépôt n° %d (%s)",
	},
	"configshare.v.provider": {
		"%s: provider %s not supported (only %q is)",
		"%s : forge %s non prise en charge (seule %q l'est)",
	},
	"configshare.v.duplicate": {
		"%s: present twice (already #%d)",
		"%s : présent deux fois (déjà en n° %d)",
	},
	"configshare.v.branches": {
		"%s: branches: %s",
		"%s : branches : %s",
	},
	"configshare.v.more_problems_one": {
		"… and %d other problem",
		"… et %d autre problème",
	},
	"configshare.v.more_problems_many": {
		"… and %d other problems",
		"… et %d autres problèmes",
	},
	"configshare.v.nothing_imported_list": {
		"; nothing was imported:",
		" ; rien n'a été importé :",
	},
	"configshare.v.interval": {
		`settings.sync.interval: unreadable duration %s (examples: "30m", "1h30m", "0")`,
		`settings.sync.interval : durée %s illisible (exemples : "30m", "1h30m", "0")`,
	},
	"configshare.v.exclude_empty": {
		"settings.branches.exclude: pattern #%d is empty",
		"settings.branches.exclude : le motif n° %d est vide",
	},
	"configshare.v.exclude_control": {
		"settings.branches.exclude: pattern #%d contains a control character",
		"settings.branches.exclude : le motif n° %d contient un caractère de contrôle",
	},
	"configshare.v.exclude_invalid": {
		`settings.branches.exclude: invalid pattern %s (path.Match syntax, e.g. "dependabot/*")`,
		"settings.branches.exclude : motif %s invalide (syntaxe de path.Match, ex. « dependabot/* »)",
	},
	"configshare.v.lang": {
		`settings.lang: unknown language %s ("en" or "fr")`,
		`settings.lang : langue %s inconnue ("en" ou "fr")`,
	},
	"configshare.v.pattern_empty": {
		"pattern #%d is empty",
		"le motif n° %d est vide",
	},
	"configshare.v.pattern_comma": {
		"pattern %s: comma not allowed (one pattern per list item)",
		"motif %s : virgule interdite (un motif par élément de la liste)",
	},
	"configshare.v.text_utf8": {
		"invalid UTF-8 text",
		"texte UTF-8 invalide",
	},
	"configshare.v.text_control": {
		"control character not allowed",
		"caractère de contrôle interdit",
	},
	"configshare.v.text_long": {
		"text too long (%d characters at most)",
		"texte trop long (%d caractères au plus)",
	},

	// Aperçu et application d'un import (Plan, Apply).
	"configshare.plan.missing": {
		"import preview: workspace, database or file not provided",
		"aperçu de l'import : espace, base ou fichier non renseigné",
	},
	"configshare.plan.refused": {
		"settings refused for this workspace; nothing was imported",
		"réglages refusés pour cet espace ; rien n'a été importé",
	},
	"configshare.plan.refused_env": {
		"settings refused for this workspace once the WATCHOO_* variables are applied (remove the variable or adapt the file); nothing was imported",
		"réglages refusés pour cet espace une fois les variables WATCHOO_* appliquées (retirez la variable ou adaptez le fichier) ; rien n'a été importé",
	},
	"configshare.plan.prefix": {
		"import preview",
		"aperçu de l'import",
	},
	"configshare.plan.read_repos": {
		"import preview: reading the repositories",
		"aperçu de l'import : lecture des dépôts",
	},
	"configshare.plan.read_settings": {
		"import preview: reading the workspace settings",
		"aperçu de l'import : lecture des réglages de l'espace",
	},
	"configshare.warn.env_written": {
		"the imported value will be written to config.yaml",
		"la valeur importée sera écrite dans config.yaml",
	},
	"configshare.warn.env_already": {
		"config.yaml already contains the imported value",
		"config.yaml contient déjà la valeur importée",
	},
	"configshare.warn.env_override": {
		"the %s variable is set: it overrides %s (%s, but the variable wins as long as it is set)",
		"la variable %s est définie : elle masque %s (%s, mais la variable l'emporte tant qu'elle est définie)",
	},
	"configshare.warn.comments_lost": {
		`config.yaml will be rewritten from the "watchoo init" template: its personal comments will be lost (the old file is copied to %s)`,
		"config.yaml sera réécrit à partir du modèle de « watchoo init » : ses commentaires personnels seront perdus (copie de l'ancien fichier dans %s)",
	},
	"configshare.apply.no_adder": {
		"import: adding repositories not available",
		"import : ajout de dépôts non disponible",
	},
	"configshare.apply.write": {
		"writing the settings",
		"écriture des réglages",
	},
	"configshare.apply.nothing_imported": {
		"nothing was imported",
		"rien n'a été importé",
	},
	"configshare.apply.read_repos": {
		"repository import: reading the repositories",
		"import des dépôts : lecture des dépôts",
	},
	"configshare.apply.repos": {
		"repository import",
		"import des dépôts",
	},
	"configshare.apply.interrupted": {
		"not processed: import interrupted",
		"non traité : import interrompu",
	},
}

func init() {
	register(configshareTexts)
}
