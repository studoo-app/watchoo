// Textes du paquet internal/diagnostic : erreurs de saisie d'une revue (phase 7.4). Préfixe des clés :
// « diagnostic. ».
package i18n

// diagnosticTexts regroupe les textes de internal/diagnostic, enregistrés par init() ci-dessous.
var diagnosticTexts = map[string]entry{
	"diagnostic.review.decision": {
		"choose a decision: assisted, not assisted or undetermined",
		"choisissez une décision : assisté, non assisté ou indéterminé",
	},
	"diagnostic.review.cause": {
		"choose the cause of a lead judged not assisted",
		"choisissez la cause d'une piste jugée non assistée",
	},
	"diagnostic.review.confidence": {
		"choose a confidence: low, medium or high",
		"choisissez une confiance : faible, moyenne ou élevée",
	},
	"diagnostic.review.note": {
		"the note is limited to %d characters",
		"la note est limitée à %d caractères",
	},
}

func init() {
	register(diagnosticTexts)
}
