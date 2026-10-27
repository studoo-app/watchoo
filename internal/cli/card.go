package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/studoo-app/watchoo/internal/logo"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Textes de la carte d'en-tête (voir documentation/05-cli.md, « Carte d'en-tête »). Les astuces ne
// citent que des commandes qui existent.
const (
	cardTitle    = "Watchoo"
	helpSubtitle = "Qui a travaillé, sur quoi, et quand ?"
	tipHelp      = "Astuce : « watchoo help <commande> » détaille une commande."
	tipInit      = "Astuce : « watchoo init » fait de ce répertoire un espace de supervision."
	tipWorkspace = "Astuce : « watchoo repo add owner/repo » puis « watchoo sync »."
	tipInitDone  = "Astuce : « watchoo repo add owner/repo » ajoute un premier dépôt à suivre."
	tipStatus    = "Astuce : « watchoo sync » met à jour les dépôts suivis."
	noWorkspace  = "Aucun espace de supervision ici"
)

// cardTerminal renvoie la description de la sortie standard et true si une carte d'en-tête peut y être
// affichée : sortie texte (pas --json) sur un terminal. Pour que Watchoo reste scriptable, une sortie
// redirigée (fichier, tube) ou demandée en JSON n'a jamais de carte ; la sortie d'erreur non plus. Le
// terminal n'est consulté qu'en sortie texte.
func cardTerminal(deps Deps, opts *options) (TermInfo, bool) {
	if opts.json {
		return TermInfo{}, false
	}
	term := deps.Terminal()
	return term, term.IsTerminal
}

// renderCard renvoie la carte d'en-tête de Watchoo (titre, version, sous-titre, astuce) pour le
// terminal term ; chaîne vide s'il est trop étroit (voir logo.Card). La commande affiche alors sa
// sortie habituelle, celle d'une sortie redirigée.
func renderCard(term TermInfo, subtitle, tip string) string {
	return logo.Card(logo.CardOptions{
		Title:    cardTitle,
		Version:  Version,
		Subtitle: subtitle,
		Tip:      tip,
		Width:    term.Width,
		Unicode:  term.Unicode,
		Color:    term.Color,
	})
}

// statusLine renvoie une ligne d'état « ● text » pour le terminal term (voir logo.Status).
func statusLine(term TermInfo, text string) string {
	return logo.Status(text, logo.CardOptions{Unicode: term.Unicode, Color: term.Color})
}

// writeHelpHeader affiche, en haut de l'aide générale et seulement sur un terminal (voir cardTerminal),
// la carte d'en-tête, une ligne vide, la ligne d'état de l'espace (helpStatus) et une ligne vide. Si
// le terminal est trop étroit pour la carte, rien n'est affiché : l'aide reste celle d'une sortie
// redirigée.
func writeHelpHeader(deps Deps, opts *options) error {
	term, ok := cardTerminal(deps, opts)
	if !ok {
		return nil
	}
	status, tip := helpStatus(deps, opts)
	card := renderCard(term, helpSubtitle, tip)
	if card == "" {
		return nil
	}
	if _, err := io.WriteString(deps.Stdout, card+"\n"+statusLine(term, status)+"\n"); err != nil {
		return fmt.Errorf("affichage de l'en-tête de l'aide : %w", err)
	}
	return nil
}

// helpStatus renvoie le texte de la ligne d'état de l'aide générale et l'astuce de la carte, selon
// l'espace trouvé comme le feraient les autres commandes (répertoire courant et ses parents, -C ou
// WATCHOO_DIR). La recherche est locale (workspace.ResolveFrom : aucun appel à gh) et ne fait jamais
// échouer l'aide :
//   - espace trouvé : « Espace : <nom> (<racine>) », et l'astuce de « watchoo repo add » et
//     « watchoo sync » ;
//   - aucun espace : « Aucun espace de supervision ici », et l'astuce de « watchoo init » ;
//   - autre erreur (réglages invalides, répertoire de -C inexistant…) : « Espace : <première ligne de
//     l'erreur> », et l'astuce de « watchoo help <commande> ».
func helpStatus(deps Deps, opts *options) (status, tip string) {
	ws, err := resolveForHelp(deps, opts)
	switch {
	case err == nil:
		return fmt.Sprintf("Espace : %s (%s)", ws.Config.Name, displayPath(ws.Root)), tipWorkspace
	case errors.Is(err, workspace.ErrNotFound):
		return noWorkspace, tipInit
	default:
		return "Espace : " + firstLine(err.Error()), tipHelp
	}
}

// resolveForHelp cherche l'espace et charge ses réglages comme les autres commandes, depuis le
// répertoire courant de deps. Un répertoire courant inaccessible n'empêche pas d'utiliser un -C absolu.
func resolveForHelp(deps Deps, opts *options) (*workspace.Workspace, error) {
	cwd, err := deps.Getwd()
	if err != nil {
		if !filepath.IsAbs(opts.workspaceDir) {
			return nil, fmt.Errorf("répertoire courant inaccessible : %w", err)
		}
		cwd = opts.workspaceDir
	}
	return workspace.ResolveFrom(opts.workspaceDir, cwd)
}

// writeInitCard affiche le compte rendu de « watchoo init » sur un terminal : la carte card, une ligne
// vide, puis le compte rendu en lignes d'état.
func writeInitCard(w io.Writer, term TermInfo, card string, r initReport) error {
	var b strings.Builder
	b.WriteString(card)
	b.WriteByte('\n')
	for _, line := range []string{
		fmt.Sprintf("Espace : %s (%s)", r.Name, displayPath(r.Root)),
		"Réglages : " + displayPath(r.ConfigFile),
		"Base : " + displayPath(r.Database),
		fmt.Sprintf("Compte gh : %s sur %s (%s)", r.GitHub.Login, r.GitHub.Host, scopesText(r.GitHub.Scopes)),
	} {
		b.WriteString(statusLine(term, line))
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("affichage du compte rendu : %w", err)
	}
	return nil
}

// displayPath renvoie path pour l'affichage sur un terminal, avec « ~ » à la place du répertoire
// personnel (os.UserHomeDir) quand path est ce répertoire ou se trouve dessous. Le chemin est rendu
// tel quel si le répertoire personnel est inconnu ou est la racine du système de fichiers.
func displayPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	home = filepath.Clean(home)
	if home == filepath.Dir(home) { // racine (« / ») : tout chemin serait sous « ~ »
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+string(filepath.Separator)); ok {
		return "~" + string(filepath.Separator) + rest
	}
	return path
}

// firstLine renvoie la première ligne de s (une erreur peut en regrouper plusieurs, errors.Join).
func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
