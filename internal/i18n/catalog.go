// Catalogue des textes de l'interface, composé d'une map par domaine d'affichage : un fichier
// catalog_<domaine>.go par couche (workspace, provider, cli, tui…), ajouté à catalogParts ici.
// Chaque clé a exactement deux textes : l'anglais (position 0) et le français (position 1).
// catalog_test.go vérifie l'exhaustivité (les deux langues) et l'absence de doublon.
package i18n

// catalog est le catalogue complet, fusionné à l'initialisation du paquet à partir des maps de
// chaque domaine ; en cas de doublon, la première occurrence gagne (le test le signale).
var catalog = mergeCatalog()

// mergeCatalog fusionne les maps de chaque domaine.
func mergeCatalog() map[string]entry {
	m := make(map[string]entry)
	for _, part := range catalogParts() {
		for k, v := range part {
			if _, dup := m[k]; !dup {
				m[k] = v
			}
		}
	}
	return m
}

// catalogParts liste les maps de chaque domaine d'affichage, une par fichier catalog_<domaine>.go.
// Ajouter le fichier d'une nouvelle couche ici.
func catalogParts() []map[string]entry {
	return []map[string]entry{
		workspaceTexts,
	}
}
