// Package logo fournit le logo de Watchoo, un hibou, en texte : les versions des fichiers
// assets/logo/*.txt et la carte d'en-tête de la CLI et des fichiers de configuration (Card, Status),
// construite autour de la version mini (voir documentation/06-interface-web.md, « Versions texte (CLI
// et TUI) », et documentation/05-cli.md, « Carte d'en-tête »).
//
// Les dessins viennent des fichiers assets/logo/*.txt, qui sont la référence. go:embed ne pouvant pas
// remonter hors du package, internal/logo/art/ en garde une copie identique octet pour octet : un test
// le vérifie, et « make logo » refait la copie après une modification de assets/logo/.
package logo

import (
	"embed"
	"fmt"
	"slices"
	"strings"
)

//go:embed art/*.txt
var files embed.FS

// Variant est une version texte du logo. Sa valeur est le nom du fichier, sans l'extension .txt.
type Variant string

// Versions texte du logo. Les tailles sont en colonnes × lignes.
const (
	BlocksName Variant = "ascii-blocs-nom" // blocs Unicode, avec le nom (53 × 8) : écran d'accueil du TUI
	Blocks     Variant = "ascii-blocs"     // blocs Unicode (16 × 8) : écran d'accueil du TUI
	MiniName   Variant = "ascii-mini-nom"  // petit format en blocs Unicode, avec le nom (44 × 4) : en-tête du TUI
	Mini       Variant = "ascii-mini"      // petit format en blocs Unicode (8 × 3) : carte d'en-tête, TUI
	MiniPure   Variant = "ascii-mini-pur"  // petit format en ASCII pur (8 × 3) : carte d'en-tête sans Unicode
	ASCIIName  Variant = "ascii-nom"       // ASCII pur, avec le nom (53 × 8)
	ASCII      Variant = "ascii"           // ASCII pur (16 × 8)
)

// variants liste toutes les versions, dans l'ordre de la documentation.
var variants = []Variant{BlocksName, Blocks, MiniName, Mini, MiniPure, ASCIIName, ASCII}

// drawing est un dessin prêt à afficher.
type drawing struct {
	lines []string // lignes, sans fin de ligne ni espaces de fin
	width int      // largeur en colonnes de la plus longue ligne (voir displayWidth)
}

// drawings contient le dessin de chaque version, lu une fois pour toutes dans les fichiers embarqués.
var drawings = loadDrawings()

// loadDrawings lit les fichiers embarqués. Un fichier absent est une erreur de programmation (les tests
// le détectent) : le programme s'arrête.
func loadDrawings() map[Variant]*drawing {
	out := make(map[Variant]*drawing, len(variants))
	for _, v := range variants {
		data, err := files.ReadFile("art/" + v.File())
		if err != nil {
			panic(fmt.Sprintf("logo : lecture de la version %q embarquée : %v", v, err))
		}
		out[v] = parseDrawing(string(data))
	}
	return out
}

// parseDrawing découpe le contenu d'un fichier en lignes, sans les fins de ligne (« \n » ou « \r\n »)
// ni les espaces de fin, et mesure sa largeur en colonnes avec displayWidth (en runes, pas en octets :
// les blocs Unicode en occupent trois chacun).
func parseDrawing(data string) *drawing {
	d := &drawing{}
	for line := range strings.Lines(data) {
		line = strings.TrimRight(line, " \r\n")
		d.lines = append(d.lines, line)
		d.width = max(d.width, displayWidth(line))
	}
	return d
}

// Variants renvoie toutes les versions texte du logo.
func Variants() []Variant {
	return slices.Clone(variants)
}

// File renvoie le nom du fichier de la version, dans assets/logo/ (ex. « ascii-mini.txt »).
func (v Variant) File() string {
	return string(v) + ".txt"
}

// Lines renvoie les lignes du dessin, sans fin de ligne ni espaces de fin (nil pour une version
// inconnue).
func (v Variant) Lines() []string {
	if d := drawings[v]; d != nil {
		return slices.Clone(d.lines)
	}
	return nil
}

// Width renvoie la largeur de la version, en colonnes (0 pour une version inconnue).
func (v Variant) Width() int {
	if d := drawings[v]; d != nil {
		return d.width
	}
	return 0
}
