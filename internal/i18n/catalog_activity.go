// Textes du paquet internal/activity : libellés des états d'activité, affichage court (colonne
// ÉTAT de « watchoo repo list »), dates relatives, erreur de --since. Préfixe des clés :
// « activity. ». Les libellés des états sont affichés par la CLI, le TUI et le web.
package i18n

// activityTexts regroupe les textes de internal/activity, enregistrés dans le catalogue par le
// init() ci-dessous.
var activityTexts = map[string]entry{
	// Libellés complets des états (activity.State.Label), mêmes valeurs partout.
	"activity.state.active":       {"Active", "Actif"},
	"activity.state.flemme":       {"Slacking", "Flemme"},
	"activity.state.dead":         {"Dead", "Dead"},
	"activity.state.stopped":      {"Tracking stopped", "Suivi arrêté"},
	"activity.state.error":        {"Error", "En erreur"},
	"activity.state.never_synced": {"Never synced", "Jamais synchronisé"},
	"activity.state.archived":     {"Archived", "Archivé"},
	"activity.state.empty":        {"Empty", "Vide"},

	// Affichage court de l'état (activity.Result.Short) : la colonne ÉTAT reste courte.
	"activity.short.flemme":         {"~ %d d", "~ %d j"},
	"activity.short.dead":           {"⚠ %d d", "⚠ %d j"},
	"activity.short.dead_ascii":     {"! %d d", "! %d j"},
	"activity.short.stopped":        {"stopped", "arrêté"},
	"activity.short.stopped_ascii":  {"stopped", "arrete"},
	"activity.short.error":          {"error", "erreur"},
	"activity.short.never":          {"never", "jamais"},
	"activity.short.archived":       {"archived", "archivé"},
	"activity.short.archived_ascii": {"archived", "archive"},
	"activity.short.empty":          {"empty", "vide"},

	// Dates relatives (activity.Relative).
	"activity.rel.never":     {"never", "jamais"},
	"activity.rel.now":       {"just now", "à l'instant"},
	"activity.rel.now_ascii": {"just now", "a l'instant"},
	"activity.rel.minutes":   {"%d min ago", "il y a %d min"},
	"activity.rel.hours":     {"%d h ago", "il y a %d h"},
	"activity.rel.days":      {"%d d ago", "il y a %d j"},

	// Erreur de --since (activity.ParseSince).
	"activity.err.since": {
		`--since %q is invalid: expected a positive whole number followed by d (days), h (hours) or m (minutes), for example 7d, 12h or 30m`,
		"--since %q invalide : attendu un nombre entier positif suivi de d (jours), h (heures) ou m (minutes), par exemple 7d, 12h ou 30m",
	},
}

func init() {
	register(activityTexts)
}
