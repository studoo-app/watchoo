package diagnostic

import (
	"path"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Input est ce que Compute lit d'un commit. Le message et les chemins restent en mémoire : rien n'en
// est recopié dans Signals.
type Input struct {
	Message     string                // message complet du commit (donnée personnelle)
	AuthorName  string                // nom Git de l'auteur
	AuthorLogin string                // login du compte lié ; vide si aucun
	Files       []provider.CommitFile // fichiers modifiés (premier parent) ; nil = détail inconnu
}

// Signals sont les données de diagnostic d'un commit, écrites dans commit_signals.
type Signals struct {
	FilesKept     int // fichiers retenus (ni binaires ni exclus par un motif)
	FilesExcluded int // fichiers binaires ou exclus par un motif
	AdditionsKept int // lignes ajoutées dans les fichiers retenus
	DeletionsKept int // lignes supprimées dans les fichiers retenus
	// Tools sont les outils déclarés dans le message, chacun avec son signal, encodés « D1:claude »
	// (trailer) ou « D2:unnamed » (signature) : voir Declaration. Triés, sans doublon ; vide si aucun.
	Tools       []string
	AgentConfig bool // D4 : le commit modifie un fichier de configuration d'agent
	BotAuthor   bool // D3 : l'auteur est un compte bot connu
	// Message est le style du message (F4), mesuré avec le reste ; FormKnown dit si Form (F1 à F3, F5) et
	// Message sont mesurés : la forme demande les lignes ajoutées, lues sur le clone (voir SetForm).
	Message   MessageStyle
	Form      Form
	FormKnown bool
}

// SetForm renseigne les mesures de forme du commit (lignes ajoutées lues sur le clone).
func (s *Signals) SetForm(f Form) { s.Form, s.FormKnown = f, true }

// Compute calcule les signaux d'un commit. ok est faux quand le détail des fichiers est inconnu
// (in.Files nil) : aucune valeur n'est alors inventée, le commit n'a pas de ligne commit_signals.
func Compute(in Input, ex *Excluder) (s Signals, ok bool) {
	if in.Files == nil {
		return Signals{}, false
	}
	for _, f := range in.Files {
		if isAgentConfig(f.Path) {
			s.AgentConfig = true
		}
		if f.Binary || ex.Excluded(f.Path) {
			s.FilesExcluded++
			continue
		}
		s.FilesKept++
		s.AdditionsKept += f.Additions
		s.DeletionsKept += f.Deletions
	}
	s.Tools = EncodeDeclarations(MessageDeclarations(in.Message))
	s.BotAuthor = isBot(in.AuthorName) || isBot(in.AuthorLogin)
	s.Message = MeasureMessage(in.Message)
	return s, true
}

// MessageDeclarations renvoie les outils déclarés dans un message de commit : trailers
// « Co-authored-by: » (signal D1) et signatures (signal D2), triés (signal puis outil), sans doublon ;
// liste vide (non nil) si aucun.
func MessageDeclarations(message string) []Declaration {
	found := []Declaration{}
	add := func(d Declaration) {
		if !slices.Contains(found, d) {
			found = append(found, d)
		}
	}
	for line := range strings.Lines(message) {
		line = strings.TrimRight(line, "\r\n")
		if m := coAuthorTrailer.FindStringSubmatch(line); m != nil {
			for _, p := range coAuthorPatterns {
				if p.re.MatchString(m[1]) {
					add(Declaration{Signal: SignalCoAuthor, Tool: p.tool})
				}
			}
			continue
		}
		for _, p := range signaturePatterns {
			if p.re.MatchString(line) {
				add(Declaration{Signal: SignalSignature, Tool: p.tool})
			}
		}
	}
	sortDeclarations(found)
	return found
}

// PullRequestDeclarations renvoie les outils déclarés dans la description d'une pull request : toute
// mention reconnue (signature ou trailer) y relève du signal D2 (signature dans la PR). Triés, sans
// doublon ; liste vide (non nil) si aucun. La description n'est lue qu'en mémoire.
func PullRequestDeclarations(body string) []Declaration {
	found := []Declaration{}
	for _, d := range MessageDeclarations(body) {
		d.Signal = SignalSignature
		if !slices.Contains(found, d) {
			found = append(found, d)
		}
	}
	sortDeclarations(found)
	return found
}

// AgentBranch renvoie l'outil dont le préfixe commence le nom de branche branch (signal D5 :
// « codex/… », « copilot/… », « claude/… », « cursor/… ») ; ok faux sinon.
func AgentBranch(branch string) (tool string, ok bool) {
	for _, p := range agentBranchPrefixes {
		if strings.HasPrefix(branch, p.prefix) && len(branch) > len(p.prefix) {
			return p.tool, true
		}
	}
	return "", false
}

// isBot indique si un nom ou un login désigne un compte bot connu (signal D3).
func isBot(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s != "" && (strings.HasSuffix(s, "[bot]") || botLogins[s])
}

// isAgentConfig indique si le chemin p est un fichier de configuration d'agent (signal D4).
func isAgentConfig(p string) bool {
	if agentConfigPaths[p] {
		return true
	}
	base := path.Base(p)
	if agentConfigNames[base] {
		return true
	}
	for _, prefix := range agentConfigPrefixes {
		if strings.HasPrefix(base, prefix) {
			return true
		}
	}
	for _, d := range agentConfigDirs {
		if strings.HasPrefix(p, d+"/") || strings.Contains(p, "/"+d+"/") {
			return true
		}
	}
	return false
}
