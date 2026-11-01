// Catalogue des textes de l'interface : une map par domaine d'affichage, dans un fichier
// catalog_<domaine>.go par couche (workspace, provider, cli, tui, web…), enregistrée par le init()
// du fichier via register. Chaque clé a exactement deux textes : l'anglais (position 0) et le
// français (position 1). Le test catalog_test.go vérifie l'exhaustivité (les deux langues).
package i18n

// catalog est le catalogue complet des textes, rempli au démarrage du paquet par les init() des
// fichiers catalog_<domaine>.go via register.
var catalog = map[string]entry{}

// register ajoute les entrées d'un domaine au catalogue. En cas de doublon, la première
// occurrence gagne ; les clés des domaines sont préfixées (workspace., provider., cli.…) pour
// l'éviter.
func register(part map[string]entry) {
	for k, v := range part {
		if _, dup := catalog[k]; !dup {
			catalog[k] = v
		}
	}
}
