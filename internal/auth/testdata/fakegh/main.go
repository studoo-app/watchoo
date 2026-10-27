// Command fakegh est un faux « gh » pour les tests de internal/auth. Il est compilé par TestMain dans
// un répertoire temporaire, puis désigné par GH.Path (réglage github.gh_path) : les tests n'ont besoin
// ni du vrai gh, ni d'un compte GitHub, ni du réseau.
//
// Il comprend seulement les deux commandes utilisées par Watchoo :
//
//	fakegh auth status --json hosts --hostname <hôte>
//	fakegh auth token --hostname <hôte> [--user <login>]
//
// (les options acceptent aussi la forme --option=valeur). Toute autre commande ou option échoue avec
// le code 4 et « unknown … » sur stderr, comme un gh qui ne la connaît pas.
//
// Variables d'environnement :
//   - FAKEGH_SCENARIO (obligatoire) : comportement simulé, voir la liste des scénarios ci-dessous ;
//   - FAKEGH_RECORD_FILE (facultatif) : chaque appel y ajoute une ligne JSON {"args":[…],"env":{…}}
//     (arguments reçus et valeurs de GH_PROMPT_DISABLED / GH_NO_UPDATE_NOTIFIER), pour vérifier la
//     ligne de commande construite par Watchoo ;
//   - FAKEGH_STATE_FILE (scénario token-rotation) : compteur d'appels à « auth token » ;
//   - FAKEGH_PID_FILE (scénario hang, facultatif) : reçoit le PID du processus avant qu'il se bloque,
//     pour vérifier qu'il a bien été tué au délai.
//
// Scénarios :
//   - logged-in : compte actif « watchoo-test » (trousseau, droits « gist, read:org, repo ») ; le
//     token est gho_fake_for_tests ; « --user » avec un autre login (ou une autre casse, comme le
//     vrai gh) échoue ;
//   - not-logged-in : comme le vrai gh avec --json : {"hosts":{}} et code 0 pour « auth status »
//     (message sur stderr), code 1 pour « auth token » ;
//   - exit-not-logged-in : « auth status » sort avec le code 1 et le message « You are not logged
//     into any GitHub hosts… » (comportement de gh sans --json) ;
//   - multi-account : « watchoo-test » (actif) et « watchoo-other » (inactif, droit « repo », token
//     gho_fake_for_tests_other) ; « --user » choisit le compte ;
//   - env-token : comme avec GH_TOKEN défini : deux entrées « watchoo-test », l'active venant de
//     GH_TOKEN (token gho_fake_for_tests_env), l'autre du trousseau (gho_fake_for_tests, renvoyé
//     avec « --user », qui ignore GH_TOKEN) ;
//   - env-token-only : comme avec GH_TOKEN défini et aucun compte enregistré (Docker, CI) : une seule
//     entrée « watchoo-test », active, venant de GH_TOKEN ; « auth token » sans --user renvoie
//     gho_fake_for_tests_env, « --user watchoo-test » échoue (« no oauth token found for … account … »,
//     comportement vérifié avec gh 2.103) ;
//   - invalid-token : le compte actif a l'état « error » et l'erreur « HTTP 401: Bad credentials »
//     (token révoqué) ;
//   - offline : GitHub injoignable (proxy, réseau coupé) : le compte actif a l'état « error » avec une
//     erreur réseau, qui contient volontairement le faux token pour vérifier qu'il est masqué ;
//     « auth token » fonctionne (il ne contacte pas GitHub) ;
//   - token-rotation : comme logged-in, mais le token change à chaque appel (gho_fake_for_tests_1,
//     _2…), grâce au compteur de FAKEGH_STATE_FILE ;
//   - empty-output : code 0 et sortie vide (blancs seulement pour « auth token ») ;
//   - garbage : « auth status » écrit autre chose que du JSON ; « auth token » écrit plusieurs lignes
//     (dont le faux token) ;
//   - hang : ne répond jamais (dort une heure) ;
//   - error : code 2 avec un message sur stderr, qui contient volontairement le faux token pour
//     vérifier qu'il n'est jamais recopié tel quel.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// Valeurs factices, identiques à celles attendues par les tests (authtest.FakeToken, FakeLogin).
const (
	fakeToken   = "gho_fake_for_tests"
	otherToken  = fakeToken + "_other"
	envToken    = fakeToken + "_env"
	login       = "watchoo-test"
	otherLogin  = "watchoo-other"
	defaultHost = "github.com"
	allScopes   = "gist, read:org, repo"
)

// Codes de retour.
const (
	exitOK       = 0
	exitFailure  = 1 // échec « normal » de gh (pas de token…)
	exitError    = 2 // scénario error
	exitBadSetup = 3 // scénario inconnu ou variable manquante : erreur du test lui-même
	exitUsage    = 4 // commande ou option inconnue
)

// invocation est la commande reçue, une fois les arguments analysés.
type invocation struct {
	command string // « status » ou « token »
	host    string // --hostname (défaut github.com)
	user    string // --user (auth token)
	json    string // --json (auth status)
}

// account est une entrée de « hosts », dans l'ordre des champs du vrai gh.
type account struct {
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	Active      bool   `json:"active"`
	Host        string `json:"host"`
	Login       string `json:"login"`
	TokenSource string `json:"tokenSource"`
	Scopes      string `json:"scopes,omitempty"`
	GitProtocol string `json:"gitProtocol"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run exécute le faux gh et renvoie son code de retour.
func run(args []string, stdout, stderr io.Writer) int {
	if err := record(args); err != nil {
		fmt.Fprintf(stderr, "fakegh : enregistrement de l'appel : %v\n", err)
		return exitBadSetup
	}
	inv, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitUsage
	}

	scenario := os.Getenv("FAKEGH_SCENARIO")
	switch scenario {
	case "logged-in":
		return answer(inv, stdout, stderr, []account{
			stored(inv.host, login, true, "success", allScopes),
		}, map[string]string{"": fakeToken, login: fakeToken})
	case "multi-account":
		return answer(inv, stdout, stderr, []account{
			stored(inv.host, login, true, "success", allScopes),
			stored(inv.host, otherLogin, false, "success", "repo"),
		}, map[string]string{"": fakeToken, login: fakeToken, otherLogin: otherToken})
	case "env-token":
		envAccount := stored(inv.host, login, true, "success", "")
		envAccount.TokenSource = "GH_TOKEN"
		return answer(inv, stdout, stderr, []account{
			envAccount,
			stored(inv.host, login, false, "success", allScopes),
		}, map[string]string{"": envToken, login: fakeToken})
	case "env-token-only":
		envAccount := stored(inv.host, login, true, "success", "")
		envAccount.TokenSource = "GH_TOKEN"
		return answer(inv, stdout, stderr, []account{envAccount}, map[string]string{"": envToken})
	case "invalid-token":
		revoked := stored(inv.host, login, true, "error", "")
		revoked.Error = "HTTP 401: Bad credentials (https://api.github.com/)"
		return answer(inv, stdout, stderr, []account{revoked}, map[string]string{"": fakeToken, login: fakeToken})
	case "offline":
		unreachable := stored(inv.host, login, true, "error", "")
		unreachable.Error = `Get "https://api.` + inv.host + `/": proxyconnect tcp: dial tcp 127.0.0.1:9: ` +
			"connect: connection refused (token " + fakeToken + ")"
		return answer(inv, stdout, stderr, []account{unreachable}, map[string]string{"": fakeToken, login: fakeToken})
	case "token-rotation":
		if inv.command == "status" {
			return answer(inv, stdout, stderr, []account{
				stored(inv.host, login, true, "success", allScopes),
			}, nil)
		}
		n, err := nextCount()
		if err != nil {
			fmt.Fprintf(stderr, "fakegh : %v\n", err)
			return exitBadSetup
		}
		fmt.Fprintf(stdout, "%s_%d\n", fakeToken, n)
		return exitOK
	case "not-logged-in":
		if inv.command == "status" {
			fmt.Fprintf(stderr, "You are not logged into any accounts on %s\n", inv.host)
			fmt.Fprintln(stdout, `{"hosts":{}}`)
			return exitOK
		}
		return noToken(inv, stderr)
	case "exit-not-logged-in":
		if inv.command == "status" {
			fmt.Fprintln(stderr, "You are not logged into any GitHub hosts. To log in, run: gh auth login")
			return exitFailure
		}
		return noToken(inv, stderr)
	case "empty-output":
		if inv.command == "token" {
			fmt.Fprint(stdout, " \n\t\n")
		}
		return exitOK
	case "garbage":
		if inv.command == "status" {
			fmt.Fprintln(stdout, "github.com\n  ✓ Logged in to github.com account watchoo-test (keyring)")
			return exitOK
		}
		fmt.Fprintf(stdout, "%s\nwarning: something unexpected\n", fakeToken)
		return exitOK
	case "hang":
		if path := os.Getenv("FAKEGH_PID_FILE"); path != "" {
			if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
				fmt.Fprintf(stderr, "fakegh : %v\n", err)
				return exitBadSetup
			}
		}
		time.Sleep(time.Hour)
		return exitOK
	case "error":
		fmt.Fprintf(stderr, "erreur simulée de gh : HTTP 401 Bad credentials (token %s refusé)\nsecond line\n", fakeToken)
		return exitError
	case "":
		fmt.Fprintln(stderr, "fakegh : FAKEGH_SCENARIO n'est pas défini")
		return exitBadSetup
	default:
		fmt.Fprintf(stderr, "fakegh : scénario inconnu %q\n", scenario)
		return exitBadSetup
	}
}

// stored renvoie un compte enregistré dans le trousseau.
func stored(host, login string, active bool, state, scopes string) account {
	return account{
		State:       state,
		Active:      active,
		Host:        host,
		Login:       login,
		TokenSource: "keyring",
		Scopes:      scopes,
		GitProtocol: "https",
	}
}

// answer répond à « auth status » avec accounts, et à « auth token » avec tokens (clé : login de
// --user, ou "" sans --user).
func answer(inv invocation, stdout, stderr io.Writer, accounts []account, tokens map[string]string) int {
	if inv.command == "status" {
		out := map[string]map[string][]account{"hosts": {inv.host: accounts}}
		if err := json.NewEncoder(stdout).Encode(out); err != nil {
			fmt.Fprintf(stderr, "fakegh : %v\n", err)
			return exitBadSetup
		}
		return exitOK
	}
	token, ok := tokens[inv.user]
	if !ok {
		return noToken(inv, stderr)
	}
	fmt.Fprintln(stdout, token)
	return exitOK
}

// noToken imite l'échec de « gh auth token » sans token pour l'hôte ou le compte.
func noToken(inv invocation, stderr io.Writer) int {
	if inv.user != "" {
		fmt.Fprintf(stderr, "no oauth token found for %s account %s\n", inv.host, inv.user)
	} else {
		fmt.Fprintf(stderr, "no oauth token found for %s\n", inv.host)
	}
	return exitFailure
}

// parseArgs analyse « auth status|token [options] ».
func parseArgs(args []string) (invocation, error) {
	if len(args) < 2 || args[0] != "auth" {
		return invocation{}, fmt.Errorf("unknown command %q for \"gh\"", strings.Join(args, " "))
	}
	inv := invocation{command: args[1], host: defaultHost}
	var allowed map[string]*string
	switch inv.command {
	case "status":
		allowed = map[string]*string{"--hostname": &inv.host, "--json": &inv.json}
	case "token":
		allowed = map[string]*string{"--hostname": &inv.host, "--user": &inv.user}
	default:
		return invocation{}, fmt.Errorf("unknown command %q for \"gh auth\"", inv.command)
	}

	rest := args[2:]
	for len(rest) > 0 {
		arg := rest[0]
		rest = rest[1:]
		name, value, hasValue := strings.Cut(arg, "=")
		target, ok := allowed[name]
		if !ok {
			return invocation{}, fmt.Errorf("unknown flag: %s", name)
		}
		if !hasValue {
			if len(rest) == 0 {
				return invocation{}, fmt.Errorf("flag needs an argument: %s", name)
			}
			value, rest = rest[0], rest[1:]
		}
		*target = value
	}

	if inv.command == "status" && inv.json != "hosts" {
		return invocation{}, errors.New("fakegh : « auth status » attend --json hosts")
	}
	return inv, nil
}

// record ajoute l'appel à FAKEGH_RECORD_FILE, si la variable est définie.
func record(args []string) error {
	path := os.Getenv("FAKEGH_RECORD_FILE")
	if path == "" {
		return nil
	}
	line, err := json.Marshal(map[string]any{
		"args": args,
		"env": map[string]string{
			"GH_PROMPT_DISABLED":    os.Getenv("GH_PROMPT_DISABLED"),
			"GH_NO_UPDATE_NOTIFIER": os.Getenv("GH_NO_UPDATE_NOTIFIER"),
		},
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// nextCount incrémente le compteur de FAKEGH_STATE_FILE et renvoie sa nouvelle valeur (1 au premier
// appel).
func nextCount() (int, error) {
	path := os.Getenv("FAKEGH_STATE_FILE")
	if path == "" {
		return 0, errors.New("FAKEGH_STATE_FILE n'est pas défini")
	}
	n := 0
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if n, err = strconv.Atoi(strings.TrimSpace(string(data))); err != nil {
			return 0, fmt.Errorf("compteur illisible : %w", err)
		}
	case !errors.Is(err, os.ErrNotExist):
		return 0, err
	}
	n++
	if err := os.WriteFile(path, []byte(strconv.Itoa(n)), 0o600); err != nil {
		return 0, err
	}
	return n, nil
}
