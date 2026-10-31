package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/logo"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// Textes de la carte d'en-tête (voir documentation/fr/05-cli.md, « Carte d'en-tête »). Les astuces ne
// citent que des commandes qui existent. Elles passent par internal/i18n, sauf tipInitDone et
// tipStatus : affichées par init.go et status.go, qui ne sont pas encore traduits, elles restent des
// constantes provisoires (clés cli.core.tip.init_done et cli.core.tip.status déjà dans le catalogue).
const (
	cardTitle   = "Watchoo"
	tipInitDone = "Astuce : « watchoo repo add owner/repo » ajoute un premier dépôt à suivre."
	tipStatus   = "Astuce : « watchoo sync » met à jour les dépôts suivis."
)

// helpSubtitle est le sous-titre de la carte de l'aide générale.
func helpSubtitle() string { return i18n.T("cli.core.card.subtitle") }

// tipHelp, tipInit, tipWorkspace et noWorkspace sont les textes de la ligne d'état de l'aide
// générale.
func tipHelp() string { return i18n.T("cli.core.tip.help") }
func tipInit() string { return i18n.T("cli.core.tip.init") }
func tipWorkspace() string {
	return i18n.T("cli.core.tip.workspace")
}
func noWorkspace() string { return i18n.T("cli.core.no_workspace") }

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
		Version:  appVersion(),
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
	card := renderCard(term, helpSubtitle(), tip)
	if card == "" {
		return nil
	}
	if _, err := io.WriteString(deps.Stdout, card+"\n"+statusLine(term, status)+"\n"); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.core.err.write_help_header"),
			i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// helpStatus renvoie le texte de la ligne d'état de l'aide générale et l'astuce de la carte, selon
// l'espace trouvé comme le feraient les autres commandes (répertoire courant et ses parents, -C ou
// WATCHOO_DIR). La recherche est locale (workspace.ResolveFrom : aucun appel à gh) et ne fait jamais
// échouer l'aide :
//   - espace trouvé : « Espace : <nom> (<racine>) » (cli.core.status.workspace), et l'astuce de
//     « watchoo repo add » et « watchoo sync » ;
//   - aucun espace : « Aucun espace de supervision ici » (cli.core.no_workspace), et l'astuce de
//     « watchoo init » ;
//   - autre erreur (réglages invalides, répertoire de -C inexistant…) : « Espace : <première ligne de
//     l'erreur> » (cli.core.status.workspace_error), et l'astuce de « watchoo help <commande> ».
func helpStatus(deps Deps, opts *options) (status, tip string) {
	ws, err := resolveForHelp(deps, opts)
	switch {
	case err == nil:
		return i18n.T("cli.core.status.workspace", ws.Config.Name, displayPath(ws.Root)), tipWorkspace()
	case errors.Is(err, workspace.ErrNotFound):
		return noWorkspace(), tipInit()
	default:
		return i18n.T("cli.core.status.workspace_error", firstLine(err.Error())), tipHelp()
	}
}

// resolveForHelp cherche l'espace et charge ses réglages comme les autres commandes, depuis le
// répertoire courant de deps. Un répertoire courant inaccessible n'empêche pas d'utiliser un -C absolu.
func resolveForHelp(deps Deps, opts *options) (*workspace.Workspace, error) {
	cwd, err := deps.Getwd()
	if err != nil {
		if !filepath.IsAbs(opts.workspaceDir) {
			return nil, fmt.Errorf("%s%s%w", i18n.T("cli.core.err.cwd_unavailable"),
				i18n.T("cli.core.err.sep"), err)
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
		i18n.T("cli.core.status.workspace", r.Name, displayPath(r.Root)),
		i18n.T("cli.core.status.settings", displayPath(r.ConfigFile)),
		i18n.T("cli.core.status.database", displayPath(r.Database)),
		initCardAccountLine(r),
	} {
		b.WriteString(statusLine(term, line))
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("%s%s%w", i18n.T("cli.core.err.write_report"),
			i18n.T("cli.core.err.sep"), err)
	}
	return nil
}

// initCardAccountLine décrit l'accès aux dépôts sur la carte de init : compte gh, ou clones Git locaux.
func initCardAccountLine(r initReport) string {
	if r.Source == wsync.SourceGit {
		return i18n.T("cli.core.status.source_git")
	}
	return i18n.T("cli.core.status.gh_account", r.GitHub.Login, r.GitHub.Host, scopesText(r.GitHub.Scopes))
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
