// Textes communs à la CLI et au web pour l'historique des conventions (phase 8.4) : réécritures de la
// branche par défaut (BR4), historique des tags (SV3), échéances des jalons (SV2), pré-versions (SV4),
// saut de version (SV5), précédence (SV7) et fraîcheur du CHANGELOG (CL10). Préfixe des clés : « conv. ».
// Les codes, états et raisons restent en anglais dans le --json (schéma stable). Jamais « a poussé » ni
// « jamais réécrit » : seulement ce qui a été observé.
package i18n

// conventionsHistoryTexts regroupe les textes partagés de la phase 8.4, enregistrés par init() ci-dessous.
var conventionsHistoryTexts = map[string]entry{
	"conv.indicator.BR4":  {"Rewrites of the default branch", "Réécritures de la branche par défaut"},
	"conv.indicator.SV3":  {"Tag history", "Historique des tags"},
	"conv.indicator.SV4":  {"Pre-releases", "Pré-versions"},
	"conv.indicator.SV5":  {"Version bumps", "Sauts de version"},
	"conv.indicator.SV7":  {"Precedence and history", "Précédence et historique"},
	"conv.indicator.CL10": {"CHANGELOG freshness", "Fraîcheur du CHANGELOG"},

	// BR4.
	"conv.rewrites.none": {"no rewrite of the default branch observed", "aucune réécriture de la branche par défaut observée"},
	"conv.rewrites.count": {
		"%d rewrite(s) of the default branch observed, the last on %s",
		"%d réécriture(s) de la branche par défaut observée(s), la dernière le %s",
	},
	"conv.rewrites.event": {"%s on %s: %s → %s", "%s le %s : %s → %s"},

	// SV3.
	"conv.history.since": {"no rewrite observed since %s", "aucune réécriture observée depuis le %s"},
	"conv.history.line": {
		"tag history: %d without rewrite observed since %s, %d rewritten, %d deleted",
		"historique des tags : %d sans réécriture observée depuis le %s, %d réécrit(s), %d supprimé(s)",
	},
	"conv.history.status.stable":    {"no rewrite observed", "aucune réécriture observée"},
	"conv.history.status.restored":  {"deleted then set again on the same commit", "supprimé puis reposé sur le même commit"},
	"conv.history.status.rewritten": {"rewritten (other commit)", "réécrit (autre commit)"},
	"conv.history.status.deleted":   {"deleted", "supprimé"},
	"conv.history.first_seen":       {"first seen on %s", "vu pour la première fois le %s"},

	// SV2, échéances.
	"conv.deadline.on_time":      {"on time", "à l'heure"},
	"conv.deadline.late":         {"late", "en retard"},
	"conv.deadline.undetermined": {"undetermined", "indéterminé"},
	"conv.deadline.between":      {"set between %s and %s", "posé entre le %s et le %s"},
	"conv.deadline.unknown_date": {"an unknown date", "une date inconnue"},
	"conv.deadline.first_seen":   {"first seen on %s", "vu pour la première fois le %s"},

	// SV4.
	"conv.prerelease_code.free_form":   {"free form (expected -<form>.N)", "forme libre (attendu -<forme>.N)"},
	"conv.prerelease_code.after_final": {"set after the final version", "posée après la version finale"},
	"conv.prerelease_code.order":       {"precedence contradicts the history", "précédence contraire à l'historique"},
	"conv.prereleases.line":            {"pre-releases: %d/%d in a conforming form", "pré-versions : %d/%d de forme conforme"},
	"conv.prereleases.none":            {"no pre-release", "aucune pré-version"},

	// SV7.
	"conv.precedence_code.precedence_ancestry": {
		"higher version set on an ancestor of the lower one",
		"version supérieure posée sur un ancêtre de la version inférieure",
	},
	"conv.precedence_code.increment_skip": {"increment skipped (information)", "incrément sauté (information)"},
	"conv.precedence.line":                {"precedence: %d pair(s) of final versions checked", "précédence : %d paire(s) de versions finales vérifiée(s)"},
	"conv.precedence.consistent":          {"consistent with the history", "cohérente avec l'historique"},
	"conv.precedence.unread":              {"%d pair(s) not read in the clone", "%d paire(s) non lue(s) dans le clone"},

	// SV5.
	"conv.bump.status.consistent":   {"consistent", "cohérent"},
	"conv.bump.status.under":        {"under-versioned", "sous-versionné"},
	"conv.bump.status.undetermined": {"undetermined", "indéterminé"},
	"conv.bump.status.unknown":      {"unknown", "inconnu"},
	"conv.bump.patch":               {"PATCH", "PATCH"},
	"conv.bump.minor":               {"MINOR", "MINOR"},
	"conv.bump.major":               {"MAJOR", "MAJOR"},
	"conv.bump.expected":            {"%s, %s expected", "%s, %s attendu"},
	"conv.bumps.line":               {"version bumps: %d consistent, %d under-versioned, %d undetermined, %d unknown", "sauts de version : %d cohérent(s), %d sous-versionné(s), %d indéterminé(s), %d inconnu(s)"},
	"conv.bumps.none":               {"no pair of final versions", "aucune paire de versions finales"},

	// CL10.
	"conv.freshness.line": {
		"%d feat and %d fix since the last commit that touched the CHANGELOG (%s)",
		"%d feat et %d fix depuis le dernier commit qui a touché le CHANGELOG (%s)",
	},
	"conv.freshness.missing": {"at least: %d commit(s) not in the database", "au moins : %d commit(s) absent(s) de la base"},
	"conv.freshness.grayed": {
		"information: the CHANGELOG of the last milestone is up to date",
		"information : le CHANGELOG du dernier jalon est à jour",
	},
}

func init() {
	register(conventionsHistoryTexts)
}
