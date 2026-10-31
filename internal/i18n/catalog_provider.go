// Textes du paquet internal/provider et de ses implémentations (github, git) : erreurs sentinelles,
// erreurs typées (APIError, RateLimitError, SSORequiredError), validation des arguments, messages de
// construction du client et de la source Git locale, opérations citées en tête des erreurs. Préfixe
// des clés : « provider. » (provider.github.*, provider.git.* pour les implémentations).
//
// Règle de sécurité (identique à celle des erreurs) : aucun texte ne cite le token, le corps d'une
// réponse, un message de commit, un nom de personne ni un e-mail. Les arguments formatés (%s, %d,
// %q) ne portent que owner/nom du dépôt, un chemin d'API, un code HTTP, un SHA court, une durée ou
// une valeur d'option déjà nettoyée par l'appelant.
package i18n

// providerTexts regroupe les textes de internal/provider et de ses sous-paquets, enregistrés dans
// le catalogue par le init() ci-dessous.
var providerTexts = map[string]entry{
	// Ponctuation des messages composés : le français garde une espace avant « : », l'anglais non.
	"provider.err.sep": {": ", " : "},

	// Erreurs sentinelles (internal/provider/errors.go), testées avec errors.Is.
	"provider.err.not_found": {
		"repository not found or account without access",
		"dépôt introuvable ou compte sans accès",
	},
	"provider.err.unauthorized": {
		`GitHub token refused: sign in again with "gh auth login"`,
		"token GitHub refusé : reconnectez-vous avec « gh auth login »",
	},
	"provider.err.forbidden": {
		"access denied by GitHub",
		"accès refusé par GitHub",
	},
	"provider.err.rate_limited": {
		"GitHub API rate limit exhausted",
		"quota de l'API GitHub épuisé",
	},
	"provider.err.empty_repository": {
		"empty repository: no commit",
		"dépôt vide : aucun commit",
	},
	"provider.err.unknown_base": {
		"base commit not found: the history may have been rewritten (force-push)",
		"commit de base introuvable : l'historique a peut-être été réécrit (force-push)",
	},
	"provider.err.write_forbidden": {
		"write request refused: Watchoo accesses GitHub read-only",
		"requête en écriture refusée : Watchoo n'accède à GitHub qu'en lecture",
	},
	"provider.err.invalid_ref": {
		"invalid reference",
		"référence invalide",
	},
	"provider.err.unavailable": {
		"GitHub is unreachable or unavailable: try again later",
		"GitHub est injoignable ou indisponible : réessayez plus tard",
	},
	"provider.err.unexpected": {
		"unexpected response from GitHub",
		"réponse inattendue de GitHub",
	},
	"provider.err.unsupported": {
		"not available for this source",
		"non disponible pour cette source",
	},
	"provider.err.access_denied": {
		"access denied: check the SSH key or the Git credentials of this machine",
		"accès refusé : vérifiez la clé SSH ou les identifiants Git de cette machine",
	},

	// RateLimitError (internal/provider/errors.go) : quota épuisé ou limite secondaire, avec le
	// moment où réessayer. Le premier %s est le texte de ErrRateLimited (ou de la limite secondaire),
	// le second une date UTC ou une durée.
	"provider.ratelimit.until": {
		"%s until %s",
		"%s jusqu'au %s",
	},
	"provider.ratelimit.secondary": {
		"secondary GitHub API rate limit reached (requests too close together)",
		"limite secondaire de l'API GitHub atteinte (requêtes trop rapprochées)",
	},
	"provider.ratelimit.secondary_at": {
		"%s: try again after %s",
		"%s : réessayez après le %s",
	},
	"provider.ratelimit.secondary_in": {
		"%s: try again in %s",
		"%s : réessayez dans %s",
	},
	"provider.ratelimit.secondary_later": {
		"%s: try again later",
		"%s : réessayez plus tard",
	},

	// SSORequiredError (internal/provider/errors.go) : l'organisation impose le SAML SSO. Le message
	// cite le nom de l'organisation (%s), ou « de ce dépôt » s'il est inconnu.
	"provider.sso.org_default": {
		"of this repository",
		"de ce dépôt",
	},
	"provider.sso.org_named": {
		`"%s"`,
		"« %s »",
	},
	"provider.sso.msg": {
		"the organization %s requires single sign-on (SAML SSO): authorize GitHub CLI for this " +
			`organization ("gh auth refresh", then grant access to the organization`,
		"l'organisation %s impose l'authentification unique (SAML SSO) : autorisez GitHub CLI " +
			"pour cette organisation (« gh auth refresh », puis accordez l'accès à l'organisation",
	},
	"provider.sso.or_open": {
		", or open %s",
		", ou ouvrez %s",
	},

	// Validation des arguments (internal/provider/validate.go) : la valeur citée (%s, %d) est
	// nettoyée par quote, le message est suivi de la cause ErrInvalidRef.
	"provider.validate.owner": {
		"repository owner %s is invalid",
		"propriétaire de dépôt %s invalide",
	},
	"provider.validate.name": {
		"repository name %s is invalid",
		"nom de dépôt %s invalide",
	},
	"provider.validate.sha": {
		"commit SHA %s is invalid",
		"SHA de commit %s invalide",
	},
	"provider.validate.branch_empty": {
		"branch name is empty",
		"nom de branche vide",
	},
	"provider.validate.branch_too_long": {
		"branch name too long (%d bytes, %d at most)",
		"nom de branche trop long (%d octets, %d au plus)",
	},
	"provider.validate.branch": {
		"branch name %s is invalid",
		"nom de branche %s invalide",
	},
	"provider.validate.id": {
		"repository ID %d is invalid",
		"identifiant de dépôt %d invalide",
	},

	// Construction du client GitHub (internal/provider/github/client.go).
	"provider.github.client.token": {
		"GitHub token empty or malformed",
		"token GitHub vide ou mal formé",
	},
	"provider.github.client.useragent": {
		"User-Agent header is invalid: control character",
		"en-tête User-Agent invalide : caractère de contrôle",
	},
	"provider.github.client.timeout_negative": {
		"GitHub requests timeout is negative (%s)",
		"délai des requêtes GitHub négatif (%s)",
	},
	"provider.github.client.create": {
		"creating the GitHub client",
		"création du client GitHub",
	},
	"provider.github.client.baseurl_invalid": {
		"GitHub API URL is invalid",
		"URL de l'API GitHub invalide",
	},
	"provider.github.client.baseurl_not_absolute": {
		"GitHub API URL %q is invalid: absolute URL expected",
		"URL de l'API GitHub %q invalide : URL absolue attendue",
	},
	"provider.github.client.baseurl_credentials": {
		"GitHub API URL is invalid: it must not contain credentials",
		"URL de l'API GitHub invalide : elle ne doit pas contenir d'identifiants",
	},
	"provider.github.client.baseurl_query": {
		"GitHub API URL %q is invalid: no query or fragment",
		"URL de l'API GitHub %q invalide : ni paramètres ni fragment",
	},
	"provider.github.client.baseurl_https": {
		"GitHub API URL %q is refused: https required (http only to 127.0.0.1, ::1 or localhost)",
		"URL de l'API GitHub %q refusée : https obligatoire (http seulement vers 127.0.0.1, ::1 ou localhost)",
	},
	"provider.github.client.too_many_redirects": {
		"more than %d redirects",
		"plus de %d redirections",
	},
	"provider.github.client.redirect_other_host": {
		"redirect to another host (%s) refused",
		"redirection vers un autre hôte (%s) refusée",
	},

	// Conversion des erreurs (internal/provider/github/errors.go) : cause d'une requête qui a
	// échoué, sans jamais citer la réponse du serveur.
	"provider.github.err.unreadable": {
		"unreadable response (invalid JSON)",
		"réponse illisible (JSON invalide)",
	},
	"provider.github.err.server": {
		"server error",
		"erreur du serveur",
	},
	"provider.github.err.caller_timeout": {
		"caller's deadline exceeded",
		"délai de l'appelant dépassé",
	},
	"provider.github.err.canceled": {
		"request canceled",
		"requête annulée",
	},
	"provider.github.err.dns": {
		"host name not found",
		"nom d'hôte introuvable",
	},
	"provider.github.err.timeout": {
		"timed out",
		"délai dépassé",
	},
	"provider.github.err.tls": {
		"secure connection (TLS) failed",
		"connexion sécurisée (TLS) impossible",
	},
	"provider.github.err.dial": {
		"connection failed",
		"connexion impossible",
	},
	"provider.github.err.conn_reset": {
		"connection interrupted",
		"connexion interrompue",
	},
	"provider.github.err.network": {
		"network error",
		"erreur réseau",
	},
	"provider.github.commit_unknown": {
		"commit %s unknown to GitHub",
		"commit %s inconnu de GitHub",
	},

	// Transport (internal/provider/github/transport.go) : requête en écriture bloquée avant envoi.
	"provider.github.transport.write_blocked": {
		"request %s %s blocked",
		"requête %s %s bloquée",
	},

	// Opérations citées en tête des erreurs, communes aux deux implémentations.
	"provider.op.repo": {
		"reading repository %s/%s",
		"lecture du dépôt %s/%s",
	},

	// Opérations citées en tête des erreurs du client GitHub (champ Op des *APIError).
	"provider.github.op.repo_id": {
		"reading repository #%d",
		"lecture du dépôt n° %d",
	},
	"provider.github.op.branches": {
		"listing branches of %s/%s",
		"liste des branches de %s/%s",
	},
	"provider.github.op.pulls": {
		"listing pull requests of %s/%s",
		"liste des pull requests de %s/%s",
	},
	"provider.github.op.releases": {
		"listing releases of %s/%s",
		"liste des releases de %s/%s",
	},
	"provider.github.op.tags": {
		"listing tags of %s/%s",
		"liste des tags de %s/%s",
	},
	"provider.github.op.commits": {
		"listing commits of %s/%s",
		"liste des commits de %s/%s",
	},
	"provider.github.op.compare": {
		"comparing %s...%s in %s/%s",
		"comparaison %s...%s dans %s/%s",
	},
	"provider.github.op.commit_stats": {
		"statistics of commit %s in %s/%s",
		"statistiques du commit %s de %s/%s",
	},
	"provider.github.op.rate_limit": {
		"reading the GitHub API rate limit",
		"lecture du quota de l'API",
	},

	// Opérations citées en tête des erreurs de la source Git locale (internal/provider/git).
	"provider.git.op.repo_id": {
		"reading repository #%d",
		"lecture du dépôt d'identifiant %d",
	},
	"provider.git.op.pulls": {
		"listing pull requests of %s/%s",
		"lecture des pull requests de %s/%s",
	},
	"provider.git.op.releases": {
		"listing releases of %s/%s",
		"lecture des releases de %s/%s",
	},
	"provider.git.op.rate_limit": {
		"reading the rate limit",
		"lecture du quota",
	},
	"provider.git.op.refs": {
		"reading refs of %s/%s",
		"lecture des références de %s/%s",
	},
	"provider.git.op.commits": {
		"listing commits of %s/%s",
		"lecture des commits de %s/%s",
	},
	"provider.git.op.compare": {
		"comparing %s...%s in %s/%s",
		"comparaison %s...%s de %s/%s",
	},
	"provider.git.op.commit_stats": {
		"statistics of commit %s in %s/%s",
		"statistiques du commit %s de %s/%s",
	},
	"provider.git.op.commits_stats": {
		"statistics of commits of %s/%s",
		"statistiques des commits de %s/%s",
	},
	"provider.git.op.clone": {
		"cloning repository %s/%s",
		"clone du dépôt %s/%s",
	},
	"provider.git.op.update": {
		"updating repository %s/%s",
		"mise à jour du dépôt %s/%s",
	},
	"provider.git.op.checkout": {
		"updating the working tree of %s/%s",
		"mise à jour de la copie de travail de %s/%s",
	},
	"provider.git.op.in_dir": {
		"%s in %s",
		"%s dans %s",
	},
	"provider.git.op.git_suffix": {
		"%s (git, exit code %d)",
		"%s (git, code %d)",
	},
	"provider.git.op.local_suffix": {
		"%s (local Git repository)",
		"%s (dépôt Git local)",
	},

	// Erreurs propres à la source Git locale (internal/provider/git) : git absent, source mal
	// configurée, clone occupé par autre chose, sortie de git illisible.
	"provider.git.err.not_found": {
		"git not found: install Git (https://git-scm.com) to track repositories without gh",
		"git est introuvable : installez Git (https://git-scm.com) pour suivre les dépôts sans gh",
	},
	"provider.git.err.dir_missing": {
		"local Git repositories: clones directory not provided",
		"dépôts Git locaux : répertoire des clones non renseigné",
	},
	"provider.git.err.protocol_unknown": {
		"local Git repositories: unknown protocol %q (%q or %q)",
		"dépôts Git locaux : protocole %q inconnu (%q ou %q)",
	},
	"provider.git.err.launch": {
		"starting git",
		"lancement de git",
	},
	"provider.git.err.launch_cmd": {
		"starting git %s",
		"lancement de git %s",
	},
	"provider.git.err.timeout": {
		"git %s: %s timeout exceeded",
		"git %s : délai de %s dépassé",
	},
	"provider.git.err.clone_occupied": {
		"clone location occupied by a directory that Watchoo did not create for this repository",
		"emplacement du clone occupé par un répertoire que Watchoo n'a pas créé pour ce dépôt",
	},
	"provider.git.err.remove_clone": {
		"removing the clone of %s/%s",
		"suppression du clone de %s/%s",
	},
	"provider.git.err.log_unreadable": {
		"unreadable git log output",
		"sortie de git log illisible",
	},
	"provider.git.err.date_unreadable": {
		"unreadable date in commit %s",
		"date illisible dans le commit %s",
	},
}

func init() {
	register(providerTexts)
}
