package diagnostic

import (
	"cmp"
	"slices"
	"strings"
)

// Identifiants des signaux (spécification, section 4). Stables : ils font partie des données de
// l'espace (commit_signals.tool_ids, pull_requests.tool_ids) et des sorties --json.
const (
	SignalCoAuthor    = "D1" // trailer de co-auteur IA
	SignalSignature   = "D2" // signature dans le message ou la description de PR
	SignalBot         = "D3" // auteur bot
	SignalAgentConfig = "D4" // fichier de configuration d'agent (contexte, sans niveau)
	SignalAgentBranch = "D5" // branche nommée par un agent
)

// Declaration est un outil déclaré, avec le signal qui l'a trouvé.
type Declaration struct {
	Signal string // SignalCoAuthor ou SignalSignature
	Tool   string // identifiant d'outil (ToolClaude…)
}

// String encode d (« D1:claude »), forme gardée en base.
func (d Declaration) String() string { return d.Signal + ":" + d.Tool }

// EncodeDeclarations encode une liste de déclarations pour la base (voir Declaration.String).
func EncodeDeclarations(ds []Declaration) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.String()
	}
	return out
}

// DecodeDeclarations relit des déclarations gardées en base, triées. Une valeur sans signal (forme des
// premières versions de la phase 7.1, jamais publiées) est rattachée au signal D1 ; une valeur vide est
// ignorée.
func DecodeDeclarations(values []string) []Declaration {
	out := []Declaration{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		signal, tool, ok := strings.Cut(v, ":")
		if !ok {
			signal, tool = SignalCoAuthor, v
		}
		d := Declaration{Signal: signal, Tool: tool}
		if !slices.Contains(out, d) {
			out = append(out, d)
		}
	}
	sortDeclarations(out)
	return out
}

// sortDeclarations trie par signal puis par outil.
func sortDeclarations(ds []Declaration) {
	slices.SortFunc(ds, func(a, b Declaration) int {
		return cmp.Or(cmp.Compare(a.Signal, b.Signal), cmp.Compare(a.Tool, b.Tool))
	})
}
