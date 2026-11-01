// Textes du paquet internal/auth : accès à la CLI GitHub gh (erreurs sentinelles et leur contexte).
// Préfixe des clés : « auth. ». Les messages ne citent jamais le token ni la sortie standard de gh.
package i18n

// authTexts regroupe les textes de internal/auth, enregistrés dans le catalogue par le init()
// ci-dessous.
var authTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : », l'anglais non.
	"auth.err.sep": {": ", " : "},

	// Remplace un token reconnu dans un extrait de la sortie d'erreur de gh.
	"auth.redacted": {"[redacted]", "[masqué]"},

	// Erreurs sentinelles (auth.go), testées avec errors.Is.
	"auth.err.gh_not_found": {
		`the GitHub CLI "gh" cannot be found: install it (https://cli.github.com) or set github.gh_path`,
		"la CLI GitHub « gh » est introuvable : installez-la (https://cli.github.com) ou réglez github.gh_path",
	},
	"auth.err.not_logged_in": {
		`gh is not signed in to GitHub: run "gh auth login"`,
		"gh n'est pas connecté à GitHub : lancez « gh auth login »",
	},
	"auth.err.unusable_user": {
		"fix the github.user setting, or empty it to use the active gh account",
		"corrigez le réglage github.user, ou videz-le pour utiliser le compte actif de gh",
	},
	"auth.err.check_failed": {
		"gh could not reach GitHub to check the token: check the network connection (proxy, firewall) " +
			"or the status of GitHub, then try again",
		"gh n'a pas pu joindre GitHub pour vérifier le token : vérifiez la connexion " +
			"réseau (proxy, pare-feu) ou l'état de GitHub, puis réessayez",
	},
	"auth.err.timeout": {
		"gh did not answer in time",
		"gh n'a pas répondu dans le délai imparti",
	},
	"auth.err.unexpected_output": {
		"unexpected answer from gh (version too old?)",
		"réponse inattendue de gh (version trop ancienne ?)",
	},

	// Status et Token (auth.go).
	"auth.status.gh_said": {`, gh said "%s"`, ", gh a répondu « %s »"},
	"auth.status.check_account": {
		"checking the gh account on %s (code %d%s)",
		"vérification du compte gh sur %s (code %d%s)",
	},
	"auth.status.failed": {
		`"gh auth status" failed (code %d%s)`,
		"« gh auth status » a échoué (code %d%s)",
	},
	"auth.status.too_long": {
		`answer of "gh auth status" too long`,
		"réponse de « gh auth status » trop longue",
	},
	"auth.token.target_user": {`%s, account %q`, "%s, compte %q"},
	"auth.token.failed": {
		`"gh auth token" failed (code %d) for %s`,
		"« gh auth token » a échoué (code %d) pour %s",
	},
	"auth.token.too_long": {
		`answer of "gh auth token" too long for %s`,
		"réponse de « gh auth token » trop longue pour %s",
	},
	"auth.token.empty": {
		`"gh auth token" returned no token for %s`,
		"« gh auth token » n'a renvoyé aucun token pour %s",
	},
	"auth.token.unexpected": {
		`"gh auth token" returned an unexpected output for %s`,
		"« gh auth token » a renvoyé une sortie inattendue pour %s",
	},

	// Lancement de gh (exec.go).
	"auth.exec.interrupted":  {`"gh %s" interrupted`, "« gh %s » interrompu"},
	"auth.exec.timeout":      {`"gh %s"`, "« gh %s »"},
	"auth.exec.launch":       {"launching %q", "lancement de %q"},
	"auth.exec.unfinished":   {`"gh %s": output not finished`, "« gh %s » : sortie non terminée"},
	"auth.exec.launch_error": {"cannot launch %q", "lancement de %q impossible"},

	// Analyse de « gh auth status » (parse.go).
	"auth.parse.read": {
		`reading the answer of "gh auth status"`,
		"lecture de la réponse de « gh auth status »",
	},
	"auth.parse.no_account": {
		"no gh account signed in for %s",
		"aucun compte gh connecté pour %s",
	},
	"auth.parse.not_checked": {
		`the gh token of account %q on %s could not be checked (state "%s"%s)`,
		"le token gh du compte %q sur %s n'a pas pu être vérifié (état « %s »%s)",
	},
	"auth.parse.unusable": {
		`the gh token of account %q on %s is not usable (state "%s")`,
		"le token gh du compte %q sur %s n'est pas utilisable (état « %s »)",
	},
	"auth.parse.no_active": {
		"no active gh account for %s",
		"aucun compte gh actif pour %s",
	},
	"auth.parse.user_case": {
		"github.user is %q, but gh knows this account on %s as %q and is case-sensitive",
		"github.user vaut %q, mais gh connaît ce compte sur %s sous le nom %q et distingue les majuscules",
	},
	"auth.parse.user_env": {
		`github.user is %q, but gh only knows this account on %s through the variable %s, which "gh auth token --user" ignores`,
		"github.user vaut %q, mais gh ne connaît ce compte sur %s que par la variable %s, que « gh auth token --user » ignore",
	},
	"auth.parse.user_missing": {
		"account %q (github.user setting) is not signed in to gh for %s",
		"le compte %q (réglage github.user) n'est pas connecté dans gh pour %s",
	},
}

func init() { register(authTexts) }
