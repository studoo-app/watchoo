// Package auth donne accès à la CLI GitHub gh, seule source du token GitHub de Watchoo. gh est
// facultatif depuis la V2 : sans gh installé et connecté (Absent), les dépôts sont suivis par clone Git
// local, sans token.
//
// Règles non négociables (voir documentation/fr/03-architecture.md, « Authentification : gh imposé ») :
//   - le token vient uniquement de « gh auth token » ; Watchoo ne lit jamais GITHUB_TOKEN lui-même
//     et ne stocke jamais le token (ni config, ni base, ni fichier, ni trousseau) ;
//   - le token n'apparaît jamais dans les logs, les erreurs ni aucune sortie : la sortie standard de
//     « gh auth token » n'est jamais journalisée ni recopiée dans un message d'erreur ;
//   - le token est redemandé à gh à chaque besoin (début de chaque synchro) et gardé en mémoire
//     seulement le temps de l'utiliser.
//
// Les autres packages dépendent de l'interface Client ; leurs tests utilisent authtest.Fake. Le type GH
// est l'implémentation réelle, testée avec le faux binaire internal/auth/testdata/fakegh/.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// Valeurs par défaut de GH.
const (
	DefaultPath    = "gh"             // exécutable gh, cherché dans le PATH
	DefaultHost    = "github.com"     // hôte GitHub interrogé
	DefaultTimeout = 10 * time.Second // délai maximum d'un appel à gh
)

// Erreurs du package (à tester avec errors.Is). Les messages sont destinés à l'utilisateur et disent
// quoi faire ; les fonctions les enveloppent avec du contexte, jamais avec le token.
var (
	// ErrGHNotFound : l'exécutable gh est introuvable (absent du PATH ou chemin github.gh_path inexistant).
	ErrGHNotFound = errors.New("la CLI GitHub « gh » est introuvable : installez-la (https://cli.github.com) ou réglez github.gh_path")
	// ErrNotLoggedIn : gh n'est connecté à aucun compte utilisable pour l'hôte (ou pour github.user),
	// ou GitHub a refusé le token (révoqué, expiré).
	ErrNotLoggedIn = errors.New("gh n'est pas connecté à GitHub : lancez « gh auth login »")
	// ErrUnusableUser : le réglage github.user ne désigne pas un compte dont « gh auth token --user »
	// peut lire le token (login écrit avec une autre casse, ou compte que gh ne connaît que par une
	// variable d'environnement comme GH_TOKEN, que « --user » ignore). Elle enveloppe ErrNotLoggedIn,
	// comme l'erreur que renverrait Token dans ce cas ; son message dit quoi changer au réglage.
	ErrUnusableUser error = &userError{
		msg: "corrigez le réglage github.user, ou videz-le pour utiliser le compte actif de gh",
	}
	// ErrCheckFailed : gh n'a pas pu vérifier le token auprès de GitHub (réseau coupé, proxy, panne de
	// GitHub…). Le token n'est pas en cause : se reconnecter avec « gh auth login » n'y changerait rien.
	ErrCheckFailed = errors.New("gh n'a pas pu joindre GitHub pour vérifier le token : vérifiez la connexion " +
		"réseau (proxy, pare-feu) ou l'état de GitHub, puis réessayez")
	// ErrTimeout : gh n'a pas répondu dans le délai maximum.
	ErrTimeout = errors.New("gh n'a pas répondu dans le délai imparti")
	// ErrUnexpectedOutput : gh a répondu quelque chose d'inattendu (JSON invalide, sortie vide…),
	// par exemple avec une version de gh trop ancienne pour « auth status --json ».
	ErrUnexpectedOutput = errors.New("réponse inattendue de gh (version trop ancienne ?)")
)

// userError est le type de ErrUnusableUser : son message donne le remède, sans reprendre celui de
// ErrNotLoggedIn (« lancez gh auth login » ne réglerait pas une faute de casse), qu'il enveloppe pour
// errors.Is.
type userError struct{ msg string }

func (e *userError) Error() string { return e.msg }
func (e *userError) Unwrap() error { return ErrNotLoggedIn }

// Status décrit le compte gh retenu pour un hôte. Il ne contient jamais le token.
type Status struct {
	Host        string   // hôte (ex. « github.com »)
	Login       string   // login GitHub du compte
	TokenSource string   // origine du token selon gh (« keyring », « GH_TOKEN », « oauth_token »…)
	Active      bool     // compte actif de gh pour cet hôte
	State       string   // état selon gh : « success » si le token est valide (sinon « error », « timeout »…)
	Scopes      []string // droits du token (ex. ["gist", "read:org", "repo"]) ; vide si gh n'en donne pas
}

// Client est l'accès à gh utilisé par le reste de Watchoo. Les implémentations sont sûres pour un
// usage concurrent.
type Client interface {
	// Status renvoie l'état du compte gh retenu (compte actif, ou celui de la configuration
	// github.user). Elle renvoie une erreur enveloppant ErrNotLoggedIn si aucun compte utilisable
	// n'est connecté ou si GitHub refuse le token (ErrUnusableUser, qui enveloppe ErrNotLoggedIn, si
	// c'est github.user qui est en cause), ErrCheckFailed si gh n'a pas pu joindre GitHub pour
	// vérifier le token, ErrGHNotFound si gh est absent, ErrTimeout si gh ne répond pas,
	// ErrUnexpectedOutput si sa réponse est illisible.
	//
	// Status et Token concordent : si Status réussit, Token peut lire le token du même compte ; si
	// Status renvoie ErrNotLoggedIn faute de compte utilisable, Token échoue aussi.
	Status(ctx context.Context) (Status, error)
	// Token renvoie le token du compte retenu, sans espace ni fin de ligne. Mêmes erreurs que Status,
	// sauf ErrCheckFailed : Token lit le token sans contacter GitHub, donc sans le valider.
	// Le token ne doit être ni journalisé, ni stocké, ni inclus dans une erreur.
	Token(ctx context.Context) (string, error)
}

// GH est l'implémentation réelle de Client : elle exécute la CLI gh (os/exec, sans shell).
//
// Les champs vides prennent leur valeur par défaut. GH est une valeur sans état, copiable et sûre pour
// un usage concurrent ; GH{} et &GH{} implémentent Client.
//
// Exécution de gh :
//   - chaque appel est borné par Timeout (context.WithTimeout + exec.CommandContext, avec WaitDelay
//     pour ne pas rester bloqué sur les tubes) ; si le délai expire, le processus est tué et l'erreur
//     enveloppe ErrTimeout ; si c'est le contexte de l'appelant qui est annulé, l'erreur enveloppe
//     ctx.Err() ;
//   - l'environnement est hérité tel quel (ce qui permet à gh d'utiliser GH_TOKEN en Docker / CI),
//     plus GH_PROMPT_DISABLED=1 et GH_NO_UPDATE_NOTIFIER=1 pour qu'il n'attende jamais de saisie ;
//     l'entrée standard de gh est vide ;
//   - exec.ErrNotFound ou fs.ErrNotExist sur Path → erreur enveloppant ErrGHNotFound ;
//   - un message d'erreur de Status peut reprendre la première ligne de la sortie d'erreur de gh
//     (stderr) ou du champ error de sa réponse JSON (échec de la vérification du token), raccourcie,
//     tout ce qui ressemble à un token GitHub étant masqué, jamais sa sortie standard telle quelle ;
//     les erreurs de Token ne reprennent aucune sortie de gh ;
//   - chaque appel est journalisé au niveau Debug (commande, hôte, durée, code de retour), sans
//     aucune sortie de gh.
type GH struct {
	Path    string        // chemin ou nom de l'exécutable gh ; vide = DefaultPath (réglage github.gh_path)
	User    string        // compte gh à utiliser ; vide = compte actif (réglage github.user)
	Host    string        // hôte GitHub ; vide = DefaultHost
	Timeout time.Duration // délai maximum par appel ; 0 ou négatif = DefaultTimeout
}

// Vérifie à la compilation que GH implémente Client.
var (
	_ Client = GH{}
	_ Client = (*GH)(nil)
)

// path renvoie l'exécutable gh à lancer (Path, ou DefaultPath).
func (g GH) path() string {
	if g.Path == "" {
		return DefaultPath
	}
	return g.Path
}

// host renvoie l'hôte GitHub interrogé (Host, ou DefaultHost).
func (g GH) host() string {
	if g.Host == "" {
		return DefaultHost
	}
	return g.Host
}

// timeout renvoie le délai maximum d'un appel (Timeout, ou DefaultTimeout s'il est nul ou négatif).
func (g GH) timeout() time.Duration {
	if g.Timeout <= 0 {
		return DefaultTimeout
	}
	return g.Timeout
}

// Status exécute « gh auth status --json hosts --hostname <Host> » et analyse sa sortie JSON, de la forme
//
//	{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"alice",
//	  "tokenSource":"keyring","scopes":"gist, read:org, repo","gitProtocol":"ssh"}]}}
//
// Avec --json, gh sort avec le code 0 même s'il n'est pas connecté : il faut se fier au contenu.
//   - Compte retenu : si User est vide, l'entrée dont active vaut true (« gh auth token » sans
//     --user lit ce compte, y compris un token GH_TOKEN) ; sinon le compte enregistré (trousseau ou
//     fichier de gh) dont login est exactement User, car c'est le seul que lit
//     « gh auth token --user » : gh distingue les majuscules et ignore alors GH_TOKEN.
//   - User qui ne correspond qu'à une entrée de variable d'environnement (GH_TOKEN…), ou seulement
//     à un login écrit avec une autre casse : erreur enveloppant ErrUnusableUser (donc
//     ErrNotLoggedIn), qui dit quoi changer au réglage ; Token échouerait dans les deux cas.
//   - Aucune entrée pour l'hôte, ou aucun compte retenu : erreur enveloppant ErrNotLoggedIn.
//   - Compte retenu dont state n'est pas « success » (le Status partiel est alors renvoyé avec
//     l'erreur, pour afficher le login concerné) : ErrCheckFailed si gh n'a pas pu joindre GitHub
//     (state « timeout », ou « error » sans refus du token par GitHub), le champ error de gh étant
//     repris, raccourci et tokens masqués ; sinon (token refusé, « HTTP 401 », ou état inconnu)
//     ErrNotLoggedIn.
//   - scopes est une chaîne séparée par des virgules : découpée et nettoyée des espaces ; chaîne vide
//     → Scopes vide (non nil).
//   - Code de retour non nul : ErrNotLoggedIn si stderr l'indique, sinon ErrUnexpectedOutput
//     (ex. option --json inconnue d'un vieux gh). JSON invalide : ErrUnexpectedOutput.
func (g GH) Status(ctx context.Context) (Status, error) {
	host := g.host()
	res, err := g.run(ctx, "auth status", "auth", "status", "--json", "hosts", "--hostname", host)
	if err != nil {
		return Status{}, err
	}
	if res.exitCode != 0 {
		detail := ""
		if summary := summarize(res.stderr); summary != "" {
			detail = fmt.Sprintf(", gh a répondu « %s »", summary)
		}
		if looksNotLoggedIn(res.stderr) {
			return Status{}, fmt.Errorf("vérification du compte gh sur %s (code %d%s) : %w",
				host, res.exitCode, detail, ErrNotLoggedIn)
		}
		return Status{}, fmt.Errorf("« gh auth status » a échoué (code %d%s) : %w",
			res.exitCode, detail, ErrUnexpectedOutput)
	}
	if res.overflow {
		return Status{}, fmt.Errorf("réponse de « gh auth status » trop longue : %w", ErrUnexpectedOutput)
	}
	return parseStatus(res.stdout, host, g.User)
}

// Token exécute « gh auth token --hostname <Host> », plus « --user <User> » si User n'est pas vide,
// et renvoie la sortie standard débarrassée des espaces et fins de ligne.
//   - Code de retour non nul : erreur enveloppant ErrNotLoggedIn (gh n'a pas de token pour cet hôte
//     ou ce compte).
//   - Sortie vide, trop longue ou sur plusieurs lignes : erreur enveloppant ErrUnexpectedOutput.
//
// La sortie standard (le token) n'est jamais journalisée ni incluse dans une erreur ; la sortie
// d'erreur de gh non plus, par prudence.
func (g GH) Token(ctx context.Context) (string, error) {
	host := g.host()
	args := []string{"auth", "token", "--hostname", host}
	target := host
	if g.User != "" {
		args = append(args, "--user", g.User)
		target = fmt.Sprintf("%s, compte %q", host, g.User)
	}
	res, err := g.run(ctx, "auth token", args...)
	if err != nil {
		return "", err
	}
	if res.exitCode != 0 {
		return "", fmt.Errorf("« gh auth token » a échoué (code %d) pour %s : %w",
			res.exitCode, target, ErrNotLoggedIn)
	}
	if res.overflow {
		return "", fmt.Errorf("réponse de « gh auth token » trop longue pour %s : %w", target, ErrUnexpectedOutput)
	}
	token := strings.TrimSpace(string(res.stdout))
	if token == "" {
		return "", fmt.Errorf("« gh auth token » n'a renvoyé aucun token pour %s : %w", target, ErrUnexpectedOutput)
	}
	// Un token est une seule chaîne sans espace ni caractère de contrôle ; sinon gh a écrit autre
	// chose (plusieurs lignes, message…), qui n'est surtout pas recopié dans l'erreur.
	if strings.IndexFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", fmt.Errorf("« gh auth token » a renvoyé une sortie inattendue pour %s : %w", target, ErrUnexpectedOutput)
	}
	return token, nil
}

// Absent indique une erreur de Token (ou de Status) qui veut dire que gh n'est pas utilisable sur
// cette machine : gh introuvable (ErrGHNotFound) ou connecté à aucun compte utilisable
// (ErrNotLoggedIn, ErrUnusableUser compris). Le moteur de synchro passe alors par les clones Git
// locaux (V2). Ni ErrCheckFailed (gh connecté mais GitHub injoignable), ni ErrTimeout, ni
// ErrUnexpectedOutput n'en font partie : ce sont des pannes, pas l'absence de gh.
func Absent(err error) bool {
	return errors.Is(err, ErrGHNotFound) || errors.Is(err, ErrNotLoggedIn)
}
