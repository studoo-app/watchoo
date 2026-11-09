package aisignature

import "strings"

// botLogins sont des comptes bot connus, en minuscules, en plus de tout nom ou login terminé par
// « [bot] » (Dependabot, Renovate, GitHub Actions…). Liste distincte du catalogue des outils d'IA : un
// bot n'est pas un outil déclaré, et un compte d'agent du catalogue n'est pas un bot pour autant.
var botLogins = map[string]bool{
	"copilot-swe-agent": true,
}

// IsBot indique si un nom Git ou un login de forge désigne un compte bot connu : suffixe « [bot] » (casse
// ignorée) ou compte de botLogins. Sert au signal D3 du diagnostic et aux exclusions CC4 des conventions.
func IsBot(nameOrLogin string) bool {
	s := strings.ToLower(strings.TrimSpace(nameOrLogin))
	return s != "" && (strings.HasSuffix(s, "[bot]") || botLogins[s])
}

// Identité de GitHub quand il crée lui-même un commit (fusion en squash ou par rebase depuis l'interface,
// édition web, « Initial commit » d'un dépôt créé avec un README) : le committer est le compte
// « web-flow » en mode API, l'adresse « noreply@github.com » dans un clone. Même exception documentée que
// noreplyDomain à la règle « rien de spécifique à GitHub hors de internal/provider » : des données
// comparées, sans aucun accès à la forge.
const (
	GitHubCommitterLogin = "web-flow"
	GitHubCommitterEmail = "noreply@github.com"
)

// IsGitHubCommitter indique si le committer d'un commit est GitHub lui-même, d'après son login de forge ou
// l'une de ses adresses Git (comparaisons exactes, sans casse ni espaces autour).
func IsGitHubCommitter(login string, emails ...string) bool {
	if strings.EqualFold(strings.TrimSpace(login), GitHubCommitterLogin) {
		return true
	}
	for _, e := range emails {
		if strings.EqualFold(strings.TrimSpace(e), GitHubCommitterEmail) {
			return true
		}
	}
	return false
}
