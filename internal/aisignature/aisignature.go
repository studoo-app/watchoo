// Package aisignature reconnaît les signatures d'outils d'IA déclarées dans un commit (1.2.0,
// documentation/fr/02-fonctionnalites.md, « Signatures d'IA ») : co-auteur, pied de message, compte
// d'agent. Fonctions pures, sans accès à la base ni à la forge.
//
// Une signature dit qu'un outil est déclaré sur le commit, pas qui a écrit le code ; son absence ne dit
// rien. Le style du message, le contenu du code et le rythme des commits ne sont jamais lus.
package aisignature

import (
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/studoo-app/watchoo/internal/model"
)

// RulesVersion est la version du catalogue et des règles. L'augmenter fait analyser de nouveau tous les
// commits à la synchro suivante (commits.ai_rules_version), sans requête à la forge.
const RulesVersion = 1

// Types de signature (commit_ai_signatures.kind, clés --json) : noms stables.
const (
	KindAgentAccount    = "agent_account"    // l'auteur du commit est le compte d'un agent
	KindCoauthorTrailer = "coauthor_trailer" // ligne « Co-authored-by » d'un outil
	KindFooter          = "footer"           // pied de message écrit par un outil
)

// noreplyDomain est le domaine des adresses GitHub sans réponse (« 123+login@… ») : la seule forme
// d'adresse qui désigne un compte plutôt qu'une boîte. Exception documentée à la règle « rien de
// spécifique à GitHub hors de internal/provider » : une donnée comparée, sans aucun accès à la forge.
const noreplyDomain = "users.noreply.github.com"

// tool est une entrée du catalogue. Aucune comparaison ne porte sur un nom Git : une personne peut
// s'appeler Claude. Chaque entrée a été observée sur de vrais commits publics.
type tool struct {
	id      string   // identifiant stable (commit_ai_signatures.tool, clés --json)
	name    string   // nom affiché
	emails  []string // adresses de l'outil, en minuscules
	logins  []string // comptes de forge de l'outil, en minuscules
	footers []string // pieds de message, en minuscules, sans symbole de tête ni lien
}

// catalog liste les outils reconnus. Dependabot, Renovate et les autres robots sans IA n'y sont pas.
var catalog = []tool{
	{id: "aider", name: "Aider", emails: []string{"aider@aider.chat", "noreply@aider.chat"}},
	{
		id: "claude", name: "Claude", emails: []string{"noreply@anthropic.com"},
		footers: []string{"generated with [claude code]", "generated with claude code"},
	},
	{id: "codex", name: "Codex", emails: []string{"codex@openai.com", "noreply@openai.com"}},
	{
		id: "copilot", name: "GitHub Copilot", emails: []string{"copilot@github.com"},
		logins: []string{"copilot", "copilot-swe-agent[bot]"},
	},
	{id: "cursor", name: "Cursor", emails: []string{"cursoragent@cursor.com"}},
}

// Input est ce qui est lu d'un commit : déjà en base, donc analysable sans requête.
type Input struct {
	Message      string   // message complet
	AuthorLogin  string   // login de l'auteur sur la forge ; vide si inconnu
	AuthorEmails []string // adresses Git de l'auteur
}

// Detect renvoie les signatures du commit, triées par outil puis par type ; liste vide non nil si aucune.
func Detect(in Input) []model.AISignature {
	found := []model.AISignature{}
	add := func(t *tool, kind string) {
		if t != nil {
			found = append(found, model.AISignature{Tool: t.id, Kind: kind})
		}
	}
	add(byLogin(in.AuthorLogin), KindAgentAccount)
	for _, email := range in.AuthorEmails {
		add(byEmail(email), KindAgentAccount)
	}
	// La première ligne est le sujet du commit : ni un trailer, ni un pied de message.
	lines := strings.Split(in.Message, "\n")
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, " \t\r")
		// Une ligne en retrait est une citation ou un exemple ; la manquer ne fait que donner
		// « aucune signature », ce qui n'accuse personne.
		if line == "" || unicode.IsSpace(rune(line[0])) {
			continue
		}
		if email, ok := coauthorEmail(line); ok {
			add(byEmail(email), KindCoauthorTrailer)
			continue
		}
		add(byFooter(line), KindFooter)
	}
	slices.SortFunc(found, func(a, b model.AISignature) int {
		if c := strings.Compare(a.Tool, b.Tool); c != 0 {
			return c
		}
		return strings.Compare(a.Kind, b.Kind)
	})
	return slices.Compact(found)
}

// ToolName renvoie le nom affiché d'un outil ; l'identifiant lui-même s'il n'est plus au catalogue.
func ToolName(id string) string {
	for i := range catalog {
		if catalog[i].id == id {
			return catalog[i].name
		}
	}
	return id
}

// coauthorEmail renvoie l'adresse d'une ligne « Co-authored-by: Nom <adresse> » (clé sans casse).
func coauthorEmail(line string) (string, bool) {
	key, value, ok := strings.Cut(line, ":")
	if !ok || !strings.EqualFold(key, "co-authored-by") {
		return "", false
	}
	_, rest, ok := strings.Cut(value, "<")
	if !ok {
		return "", false
	}
	email, _, ok := strings.Cut(rest, ">")
	return strings.TrimSpace(email), ok
}

// byLogin renvoie l'outil du compte login ; nil si aucun.
func byLogin(login string) *tool {
	login = strings.ToLower(login)
	if login == "" {
		return nil
	}
	for i := range catalog {
		if slices.Contains(catalog[i].logins, login) {
			return &catalog[i]
		}
	}
	return nil
}

// byEmail renvoie l'outil de l'adresse (comparaison exacte, sans casse) ; nil si aucun. Une adresse
// GitHub sans réponse se compare par son compte.
func byEmail(email string) *tool {
	email = strings.ToLower(email)
	if local, ok := strings.CutSuffix(email, "@"+noreplyDomain); ok {
		if _, login, found := strings.Cut(local, "+"); found {
			local = login
		}
		return byLogin(local)
	}
	if email == "" {
		return nil
	}
	for i := range catalog {
		if slices.Contains(catalog[i].emails, email) {
			return &catalog[i]
		}
	}
	return nil
}

// footerLink est le lien qui peut suivre un pied de message.
var footerLink = regexp.MustCompile(`\(https?://[^)\s]*\)$`)

// byFooter renvoie l'outil dont line est le pied de message, la ligne entière et rien d'autre (sans
// casse, symbole de tête et lien retirés) ; nil si aucun.
func byFooter(line string) *tool {
	line = strings.TrimLeftFunc(line, func(r rune) bool { return !unicode.IsLetter(r) && r != '[' })
	line = strings.ToLower(footerLink.ReplaceAllString(line, ""))
	for i := range catalog {
		if slices.Contains(catalog[i].footers, line) {
			return &catalog[i]
		}
	}
	return nil
}
