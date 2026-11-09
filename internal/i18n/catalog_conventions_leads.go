// Textes communs à la CLI et au web pour les pistes des conventions (phase 8.7) : CL11, CL12, RD5, DOC6,
// DOC7, DOC9, DOC10, TL4 et TL5. Une piste invite à relire, elle ne juge pas : ni « conforme », ni « non
// conforme », ni note. Préfixe des clés : « conv. ». Les identifiants, états, codes et raisons restent en
// anglais dans le --json (schéma stable).
package i18n

// conventionsLeadsTexts regroupe les textes partagés de la phase 8.7, enregistrés par init() ci-dessous.
var conventionsLeadsTexts = map[string]entry{
	"conv.lead.CL11":  {"CHANGELOG comparison links", "Liens de comparaison du CHANGELOG"},
	"conv.lead.CL12":  {"Copy of git log in the CHANGELOG", "Recopie de git log dans le CHANGELOG"},
	"conv.lead.RD5":   {"Installation in the README", "Installation dans le README"},
	"conv.lead.DOC6":  {"Document headers", "En-têtes des documents"},
	"conv.lead.DOC7":  {"Diagrams and images", "Schémas et images"},
	"conv.lead.DOC9":  {"Documentation freshness", "Fraîcheur de la documentation"},
	"conv.lead.DOC10": {"Dates declared in the INDEX", "Dates déclarées dans l'INDEX"},
	"conv.lead.TL4":   {"CI checking the conventions", "CI qui vérifie les conventions"},
	"conv.lead.TL5":   {"Effect of the tooling", "Effet de l'outillage"},

	"conv.lead_status.none":           {"nothing to look at", "rien à relire"},
	"conv.lead_status.review":         {"to look at", "à relire"},
	"conv.lead_status.unknown":        {"unknown", "inconnue"},
	"conv.lead_status.not_applicable": {"not applicable", "sans objet"},

	"conv.lead_code.missing_link":       {"heading without a comparison link", "titre sans lien de comparaison"},
	"conv.lead_code.copied_subjects":    {"bullets identical to commit subjects", "puces identiques à des sujets de commit"},
	"conv.lead_code.sha_or_merge":       {"bullets with a SHA or starting with Merge", "puces avec un SHA ou commençant par Merge"},
	"conv.lead_code.no_install_section": {"no installation or run section", "ni section installation ni section lancement"},
	"conv.lead_code.no_command_block":   {"no command block under these sections", "aucun bloc de commandes sous ces sections"},
	"conv.lead_code.no_env_example":     {"variables announced without .env.example", "variables annoncées sans .env.example"},
	"conv.lead_code.missing_headers":    {"documents without all the headers", "documents sans tous les en-têtes"},
	"conv.lead_code.broken_image":       {"broken relative images", "images relatives cassées"},
	"conv.lead_code.external_image":     {"external images (badges excluded)", "images externes (badges exclus)"},
	"conv.lead_code.docs_behind":        {"documentation behind the code", "documentation en retard sur le code"},
	"conv.lead_code.declared_newer":     {"declared date newer than the last commit", "date déclarée plus récente que le dernier commit"},
	"conv.lead_code.declared_older": {
		"declared date older than the last commit by more than 30 days",
		"date déclarée plus ancienne que le dernier commit de plus de 30 jours",
	},
	"conv.lead_code.date_unreadable": {"unreadable declared date", "date déclarée illisible"},
	"conv.lead_code.not_found":       {"no workflow cites a known check", "aucun workflow ne cite une vérification connue"},
	"conv.lead_code.no_gain":         {"rate not higher since the tooling", "taux pas plus haut depuis l'outillage"},

	"conv.lead_detail.CL11": {"%d/%d version heading(s) with a comparison link", "%d/%d titre(s) de version avec un lien de comparaison"},
	"conv.lead_detail.CL12": {
		"%d bullet(s): %d%% identical to a commit subject (%d subject(s) compared), %d%% with a SHA or Merge",
		"%d puce(s) : %d %% identiques à un sujet de commit (%d sujet(s) comparé(s)), %d %% avec un SHA ou Merge",
	},
	"conv.lead_detail.RD5":                 {"%d command block(s) under installation or run", "%d bloc(s) de commandes sous installation ou lancement"},
	"conv.lead_detail.env_example_present": {"variables announced, .env.example present", "variables annoncées, .env.example présent"},
	"conv.lead_detail.env_example_absent":  {"variables announced, no .env.example", "variables annoncées, pas de .env.example"},
	"conv.lead_detail.DOC6":                {"%d/%d document(s) with the six headers", "%d/%d document(s) avec les six en-têtes"},
	"conv.lead_detail.DOC6_fields":         {"per header: %s", "par en-tête : %s"},
	"conv.lead_detail.DOC7": {
		"%d relative image(s), %d badge(s), %d mermaid block(s)",
		"%d image(s) relative(s), %d badge(s), %d bloc(s) mermaid",
	},
	"conv.lead_detail.media_present": {"media folder present", "dossier des médias présent"},
	"conv.lead_detail.media_absent":  {"no media folder", "pas de dossier des médias"},
	"conv.lead_detail.DOC9": {
		"%d commit(s) outside the documentation since its last commit, %d day(s) behind the code",
		"%d commit(s) hors documentation depuis son dernier commit, %d jour(s) de retard sur le code",
	},
	"conv.lead_detail.DOC10": {
		"%d INDEX row(s), %d compared, %d without a readable document",
		"%d ligne(s) de l'INDEX, %d comparée(s), %d sans document lisible",
	},
	"conv.lead_detail.DOC10_over": {"%d beyond the limit of %d documents", "%d au-delà du plafond de %d documents"},
	"conv.lead_detail.TL4":        {"%d workflow(s), checks cited: %s", "%d workflow(s), vérifications citées : %s"},
	"conv.lead_detail.TL4_none":   {"%d workflow(s)", "%d workflow(s)"},
	"conv.lead_detail.TL5": {
		"before the tooling %d/%d conforming (%d%%), since %d/%d (%d%%)",
		"avant l'outillage %d/%d conformes (%d %%), depuis %d/%d (%d %%)",
	},
	"conv.lead_detail.TL5_sources": {"%d dated source(s), %d undated", "%d source(s) datée(s), %d non datée(s)"},

	"conv.lead_header.title":     {"title", "titre"},
	"conv.lead_header.objective": {"purpose", "objectif"},
	"conv.lead_header.scope":     {"scope", "périmètre"},
	"conv.lead_header.author":    {"author", "auteur"},
	"conv.lead_header.date":      {"date", "date"},
	"conv.lead_header.version":   {"version", "version"},

	"conv.lead_date.docs":       {"last documentation commit on %s", "dernier commit de documentation le %s"},
	"conv.lead_date.index":      {"INDEX on %s", "INDEX le %s"},
	"conv.lead_date.changelog":  {"CHANGELOG on %s", "CHANGELOG le %s"},
	"conv.lead_date.code":       {"code on %s", "code le %s"},
	"conv.lead_date.introduced": {"tooling introduced on %s", "outillage introduit le %s"},

	"conv.lead_sep": {"; ", " ; "},
}

func init() {
	register(conventionsLeadsTexts)
}
