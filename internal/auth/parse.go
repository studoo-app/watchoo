package auth

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ghStatusOutput est la réponse de « gh auth status --json hosts ».
type ghStatusOutput struct {
	Hosts map[string][]ghAccount `json:"hosts"`
}

// ghAccount est une entrée de « hosts » : un compte connu de gh pour un hôte. Les champs inconnus
// (gitProtocol…) sont ignorés. « error » décrit l'échec de la vérification du token auprès de GitHub
// (ex. « HTTP 401: Bad credentials (https://api.github.com/) », ou une erreur réseau) ; il n'est
// recopié dans une erreur que par summarize (une ligne, raccourcie, tokens masqués).
type ghAccount struct {
	State       string          `json:"state"`
	Error       string          `json:"error"`
	Active      bool            `json:"active"`
	Host        string          `json:"host"`
	Login       string          `json:"login"`
	TokenSource string          `json:"tokenSource"`
	Scopes      json.RawMessage `json:"scopes"` // chaîne « a, b, c » (gh 2.x) ; une liste est aussi acceptée
}

// envTokenSources sont les valeurs de tokenSource d'un token lu par gh dans une variable
// d'environnement, et non dans un compte enregistré (trousseau ou fichier de config).
var envTokenSources = map[string]bool{
	"GH_TOKEN":                true,
	"GITHUB_TOKEN":            true,
	"GH_ENTERPRISE_TOKEN":     true,
	"GITHUB_ENTERPRISE_TOKEN": true,
}

// parseStatus analyse la sortie standard de « gh auth status --json hosts --hostname host » et
// renvoie le compte retenu pour user (vide = compte actif). Les règles sont décrites sur GH.Status.
// Les erreurs ne recopient jamais stdout tel quel.
func parseStatus(stdout []byte, host, user string) (Status, error) {
	var out ghStatusOutput
	if err := json.Unmarshal(stdout, &out); err != nil || out.Hosts == nil {
		// L'erreur JSON n'est pas enveloppée : elle cite des morceaux de la sortie de gh.
		return Status{}, fmt.Errorf("lecture de la réponse de « gh auth status » : %w", ErrUnexpectedOutput)
	}

	accounts := accountsFor(out.Hosts, host)
	if len(accounts) == 0 {
		return Status{}, fmt.Errorf("aucun compte gh connecté pour %s : %w", host, ErrNotLoggedIn)
	}

	acc, err := selectAccount(accounts, host, user)
	if err != nil {
		return Status{}, err
	}

	st := Status{
		Host:        acc.Host,
		Login:       acc.Login,
		TokenSource: acc.TokenSource,
		Active:      acc.Active,
		State:       acc.State,
		Scopes:      parseScopes(acc.Scopes),
	}
	if st.Host == "" {
		st.Host = host
	}
	if st.State != "success" {
		return st, stateError(st, acc.Error, host)
	}
	return st, nil
}

// stateError renvoie l'erreur d'un compte dont l'état n'est pas « success ». gh donne l'état
// « timeout » quand GitHub n'a pas répondu à temps, et « error » aussi bien pour un token refusé
// (champ error « HTTP 401: Bad credentials… ») que pour une erreur réseau (proxy, DNS…) : seul un
// refus du token appelle « gh auth login ».
func stateError(st Status, ghErr, host string) error {
	if st.State == "timeout" || (st.State == "error" && ghErr != "" && !looksUnauthorized(ghErr)) {
		detail := ""
		if summary := summarize(ghErr); summary != "" {
			detail = fmt.Sprintf(", gh a répondu « %s »", summary)
		}
		return fmt.Errorf("le token gh du compte %q sur %s n'a pas pu être vérifié (état « %s »%s) : %w",
			st.Login, host, st.State, detail, ErrCheckFailed)
	}
	return fmt.Errorf("le token gh du compte %q sur %s n'est pas utilisable (état « %s ») : %w",
		st.Login, host, st.State, ErrNotLoggedIn)
}

// looksUnauthorized indique si le champ error de gh signale un token refusé par GitHub, par exemple
// « HTTP 401: Bad credentials (https://api.github.com/) » ou, avec gh 2.103,
// « non-200 OK status code: 401 Unauthorized body: … ».
func looksUnauthorized(ghErr string) bool {
	s := strings.ToLower(ghErr)
	for _, marker := range []string{"http 401", "status code: 401", "401 unauthorized", "bad credentials"} {
		if strings.Contains(s, marker) {
			return true
		}
	}
	return false
}

// accountsFor renvoie les comptes de l'hôte host. gh écrit les hôtes en minuscules ; la recherche
// ne tient pas compte de la casse, par prudence.
func accountsFor(hosts map[string][]ghAccount, host string) []ghAccount {
	if accounts, ok := hosts[host]; ok {
		return accounts
	}
	for h, accounts := range hosts {
		if strings.EqualFold(h, host) {
			return accounts
		}
	}
	return nil
}

// selectAccount choisit le compte, de sorte que Token lise ensuite le token de ce même compte :
//   - user vide : l'entrée active, que lit « gh auth token » sans --user (y compris un token
//     d'environnement comme GH_TOKEN) ;
//   - sinon : le compte enregistré dont le login est exactement user, seul compte que lit
//     « gh auth token --user », qui distingue les majuscules et ignore GH_TOKEN. Une entrée qui ne
//     correspond qu'à la casse près, ou seulement par un token d'environnement, donne une erreur
//     enveloppant ErrUnusableUser, qui dit quoi changer.
//
// Les erreurs enveloppent ErrNotLoggedIn.
func selectAccount(accounts []ghAccount, host, user string) (ghAccount, error) {
	if user == "" {
		for _, a := range accounts {
			if a.Active {
				return a, nil
			}
		}
		return ghAccount{}, fmt.Errorf("aucun compte gh actif pour %s : %w", host, ErrNotLoggedIn)
	}

	// Entrées qui correspondent à user sans être lisibles par « gh auth token --user » : compte
	// enregistré dont le login n'a pas la même casse, entrée d'un token d'environnement.
	var otherCase, envOnly *ghAccount
	for i := range accounts {
		a := &accounts[i]
		if !strings.EqualFold(a.Login, user) {
			continue
		}
		switch {
		case envTokenSources[a.TokenSource]:
			envOnly = a
		case a.Login == user:
			return *a, nil
		default:
			otherCase = a
		}
	}
	switch {
	case otherCase != nil:
		return ghAccount{}, fmt.Errorf("github.user vaut %q, mais gh connaît ce compte sur %s sous le nom %q "+
			"et distingue les majuscules : %w", user, host, otherCase.Login, ErrUnusableUser)
	case envOnly != nil:
		return ghAccount{}, fmt.Errorf("github.user vaut %q, mais gh ne connaît ce compte sur %s que par la "+
			"variable %s, que « gh auth token --user » ignore : %w", user, host, envOnly.TokenSource, ErrUnusableUser)
	}
	return ghAccount{}, fmt.Errorf("le compte %q (réglage github.user) n'est pas connecté dans gh pour %s : %w",
		user, host, ErrNotLoggedIn)
}

// parseScopes lit le champ scopes : une chaîne séparée par des virgules (« gist, read:org, repo »)
// ou, par tolérance, une liste de chaînes. Absent, nul ou illisible : liste vide. Le résultat n'est
// jamais nil.
func parseScopes(raw json.RawMessage) []string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return splitScopes(s)
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err == nil {
		return splitScopes(strings.Join(list, ","))
	}
	return []string{}
}

// splitScopes découpe « gist, read:org, repo » sur les virgules et retire les espaces et les
// éléments vides. Le résultat n'est jamais nil.
func splitScopes(s string) []string {
	scopes := []string{}
	for part := range strings.SplitSeq(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			scopes = append(scopes, part)
		}
	}
	return scopes
}
