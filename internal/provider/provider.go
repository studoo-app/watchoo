// Package provider définit l'interface Provider, qui isole Watchoo de la forge (GitHub au MVP), ainsi
// que les types et les erreurs qu'elle échange avec le cœur métier (internal/sync).
//
// Ce package ne dépend d'aucune forge ni d'aucune bibliothèque cliente : l'implémentation GitHub
// (internal/provider/github) est le seul endroit de Watchoo où go-github est importé. Le cœur métier ne
// dépend que de ce package ; ses tests utilisent le faux Provider en mémoire de
// internal/provider/providertest.
//
// Conventions communes à tous les types et à toutes les implémentations :
//   - les dates sont des time.Time en UTC ; une date absente est la valeur zéro (tester avec IsZero) ;
//   - une chaîne absente est la chaîne vide ; un identifiant numérique absent vaut 0 ;
//   - une liste sans élément est une slice vide non nil ;
//   - les arguments (owner, nom de dépôt, branche, SHA, identifiant) sont validés AVANT toute requête
//     (voir validate.go) : une valeur invalide donne une erreur enveloppant ErrInvalidRef, sans appel
//     réseau ;
//   - les erreurs se testent avec errors.Is et errors.As (voir errors.go). Leurs messages sont en
//     français et ne contiennent jamais le token, ni le corps brut d'une réponse, ni message de commit,
//     nom de personne ou e-mail ; ils peuvent citer owner/nom du dépôt, le chemin de l'API et le code
//     HTTP ;
//   - l'accès à la forge est en lecture seule : aucune requête autre que GET ou HEAD ne part.
package provider

import (
	"context"
	"time"
)

// Plafonds internes : ils évitent une boucle sans fin (serveur qui annonce toujours une page suivante,
// dépôt démesuré) et bornent la mémoire. Au-delà, la liste est tronquée sans erreur ; l'appelant qui
// reçoit exactement MaxCommits commits (ou MaxBranches branches…) sait qu'elle l'a peut-être été.
const (
	// MaxCommits est le nombre maximum de commits renvoyés par ListCommits et CompareCommits.
	MaxCommits = 10000
	// MaxBranches est le nombre maximum de branches renvoyées par Branches.
	MaxBranches = 1000
	// MaxPullRequests est le nombre maximum de pull requests renvoyées par PullRequests.
	MaxPullRequests = 1000
	// MaxReleases est le nombre maximum de releases renvoyées par Releases.
	MaxReleases = 1000
	// MaxTags est le nombre maximum de tags renvoyés par Tags.
	MaxTags = 1000
)

// États d'une pull request (PullRequest.State), comme sur la forge. Une PR mergée est fermée
// (PullRequestClosed) avec Merged vrai.
const (
	PullRequestOpen   = "open"
	PullRequestClosed = "closed"
)

// Provider est l'accès en lecture seule à une forge. Les implémentations sont sûres pour un usage
// concurrent : le moteur de synchro synchronise jusqu'à 4 dépôts en parallèle avec la même instance.
//
// Toutes les méthodes qui reçoivent un contexte s'arrêtent dès qu'il est terminé ; l'erreur renvoyée
// enveloppe alors ctx.Err() (context.Canceled ou context.DeadlineExceeded).
//
// Erreurs communes à toutes les méthodes qui interrogent la forge (en plus de celles propres à chaque
// méthode) :
//   - ErrInvalidRef : argument invalide (aucune requête n'est partie) ;
//   - ErrNotFound : dépôt (ou élément) introuvable. Un dépôt privé auquel le compte n'a pas accès répond
//     ainsi : ne jamais en conclure que le dépôt a été supprimé ;
//   - ErrUnauthorized : token refusé (révoqué, expiré) ; conseiller « gh auth login » ;
//   - ErrForbidden : accès refusé hors quota ; *SSORequiredError (qui satisfait aussi
//     errors.Is(err, ErrForbidden)) quand l'organisation impose le SSO ;
//   - ErrRateLimited, via *RateLimitError : quota épuisé ou limite secondaire atteinte. Le Provider
//     n'attend jamais lui-même la remise à zéro : c'est au moteur de synchro de décider ;
//   - ErrUnavailable : forge injoignable, délai dépassé, erreur serveur (5xx) ;
//   - ErrUnexpected : réponse illisible ou code HTTP imprévu ;
//   - ErrWriteForbidden : une requête en écriture a été bloquée (ne doit jamais arriver) ;
//   - ErrUnsupported : la source ne connaît pas cette notion (PullRequests, Releases ou Tags d'une
//     source qui n'en a pas, comme un clone Git local) ; aucune requête n'est partie. Le moteur de
//     synchro la traite comme « non disponible », pas comme un échec.
//
// Les erreurs issues d'une requête sont de type *APIError (Op, méthode, chemin de l'API, code HTTP),
// qui enveloppe l'une des erreurs ci-dessus.
type Provider interface {
	// Name renvoie l'identifiant de la forge, tel qu'enregistré dans les colonnes provider de la base
	// (« github » pour GitHub, soit model.ProviderGitHub).
	Name() string

	// Repository renvoie les métadonnées du dépôt owner/name (comparés sans tenir compte de la casse).
	// Un dépôt renommé ou transféré est suivi jusqu'à son emplacement actuel (redirection 301 de
	// GitHub) : Owner et Name du résultat sont alors les valeurs ACTUELLES, différentes des arguments.
	Repository(ctx context.Context, owner, name string) (Repository, error)

	// RepositoryByID renvoie les métadonnées du dépôt d'identifiant numérique id (> 0), qui ne change
	// jamais, même après un renommage ou un transfert. Le moteur de synchro s'en sert pour suivre un
	// dépôt par son identifiant.
	RepositoryByID(ctx context.Context, id int64) (Repository, error)

	// Branches renvoie toutes les branches du dépôt (toutes les pages, au plus MaxBranches), dans
	// l'ordre de la forge (alphabétique sur GitHub). Un dépôt vide (aucun commit) renvoie une liste vide
	// sans erreur.
	Branches(ctx context.Context, owner, name string) ([]Branch, error)

	// ListCommits renvoie les commits atteignables depuis opts.Branch, du plus récent au plus ancien
	// (ordre de la forge), en parcourant toutes les pages nécessaires. Voir ListCommitsOptions pour
	// l'arrêt anticipé (Stop), le filtre de date (Since) et la limite (Limit, au plus MaxCommits).
	//
	// Un dépôt vide renvoie une liste vide sans erreur. Une branche inconnue donne ErrNotFound.
	//
	// Attention : l'ordre est chronologique, pas topologique. Après un merge, des commits apportés par
	// la branche fusionnée peuvent apparaître APRÈS un commit déjà connu : s'arrêter au premier SHA
	// connu (Stop) peut donc les manquer, sans aucune erreur, dès que le dépôt a des commits en base.
	// ListCommits sert seulement au premier import d'un dépôt (aucun commit en base) et, sans Stop,
	// borné par Since ou Limit, au repli après ErrUnknownBase. Ailleurs, CompareCommits est la méthode
	// exacte : entre l'ancienne et la nouvelle tête pour une branche déjà synchronisée, depuis la tête
	// de la branche de référence (la branche par défaut) pour une nouvelle branche. Voir « Synchronisation
	// incrémentale » dans documentation/03-architecture.md.
	ListCommits(ctx context.Context, owner, name string, opts ListCommitsOptions) ([]Commit, error)

	// CompareCommits renvoie les commits atteignables depuis head mais pas depuis base (comme
	// « git log base..head »), y compris ceux apportés par un merge, du plus récent au plus ancien
	// (ordre inverse de celui de l'API GitHub, qui les donne du plus ancien au plus récent), toutes les
	// pages comprises (au plus MaxCommits). base et head sont des SHA complets (ValidateSHA).
	//
	// base == head, ou head déjà contenu dans base : liste vide sans erreur. base qui n'existe plus
	// dans le dépôt (historique réécrit, par exemple après un force-push suivi d'un nettoyage) :
	// ErrUnknownBase ; le moteur de synchro se replie alors sur une comparaison depuis la tête de la
	// branche de référence, sinon sur ListCommits sans Stop (voir ListCommits). GitHub répond de la même
	// façon (404) à un head inconnu et à deux historiques sans ancêtre commun : ces cas donnent aussi
	// ErrUnknownBase, et le même repli convient.
	CompareCommits(ctx context.Context, owner, name, base, head string) ([]Commit, error)

	// CommitStats renvoie le nombre de lignes ajoutées et supprimées par le commit sha (SHA complet).
	// Une requête par commit : à réserver aux nouveaux commits.
	CommitStats(ctx context.Context, owner, name, sha string) (CommitStats, error)

	// PullRequests renvoie les pull requests du dépôt, tous états confondus (ouvertes, fermées,
	// mergées), de la plus récemment modifiée à la plus ancienne (UpdatedAt décroissant), en
	// parcourant les pages nécessaires. Voir PullRequestsOptions pour l'arrêt anticipé (UpdatedSince)
	// et la limite (Limit, au plus MaxPullRequests). Un dépôt vide renvoie une liste vide sans erreur.
	// Une source sans pull requests renvoie une erreur enveloppant ErrUnsupported.
	PullRequests(ctx context.Context, owner, name string, opts PullRequestsOptions) ([]PullRequest, error)

	// Releases renvoie les releases du dépôt, de la plus récente à la plus ancienne (ordre de la
	// forge), toutes les pages comprises (au plus MaxReleases). Sur GitHub, les brouillons ne sont
	// renvoyés qu'à un compte qui a le droit d'écriture sur le dépôt. Un dépôt vide renvoie une liste
	// vide sans erreur. Une source sans releases renvoie une erreur enveloppant ErrUnsupported.
	Releases(ctx context.Context, owner, name string) ([]Release, error)

	// Tags renvoie les tags du dépôt dans l'ordre de la forge, toutes les pages comprises (au plus
	// MaxTags). Un dépôt vide renvoie une liste vide sans erreur.
	Tags(ctx context.Context, owner, name string) ([]Tag, error)

	// RateLimit interroge la forge sur l'état du quota (sur GitHub, GET /rate_limit, qui ne consomme
	// pas de quota et répond même quand il est épuisé) et met à jour l'état renvoyé par LastRateLimit.
	RateLimit(ctx context.Context) (RateLimit, error)

	// LastRateLimit renvoie le dernier état du quota observé (en-têtes de chaque réponse, réponses 304
	// comprises, ou RateLimit), sans requête ; false si aucun n'a encore été observé pour ce token.
	LastRateLimit() (RateLimit, bool)
}

// ListCommitsOptions règle ListCommits.
type ListCommitsOptions struct {
	// Branch est la branche (ou le SHA complet d'un commit) d'où part la liste, passée telle quelle au
	// paramètre sha de l'API ; vide = branche par défaut du dépôt. Validée par ValidateBranch si elle
	// n'est pas vide.
	Branch string
	// Since ne garde que les commits dont la date (date de commit, selon la forge) est postérieure ou
	// égale à Since ; zéro = pas de filtre.
	Since time.Time
	// Stop, si non nil, est appelée sur le SHA de chaque commit, dans l'ordre de la liste. Au PREMIER
	// commit pour lequel elle renvoie true, la liste s'arrête : ce commit et tous les suivants sont
	// exclus du résultat, et aucune page suivante n'est demandée. Elle est appelée depuis la goroutine
	// de l'appelant de ListCommits. Ne pas s'en servir pour s'arrêter au premier SHA déjà en base :
	// après un merge, des commits inconnus peuvent suivre un commit connu (voir ListCommits).
	Stop func(sha string) bool
	// Limit est le nombre maximum de commits renvoyés ; 0 (ou négatif) = pas de limite autre que
	// MaxCommits, et une valeur supérieure à MaxCommits est ramenée à MaxCommits. Aucune page n'est
	// demandée au-delà de ce qui est nécessaire.
	Limit int
}

// PullRequestsOptions règle PullRequests.
type PullRequestsOptions struct {
	// UpdatedSince, s'il n'est pas zéro, arrête la liste à la PREMIÈRE pull request modifiée
	// strictement avant UpdatedSince (comparaison à la seconde) : elle et les suivantes sont exclues,
	// aucune page suivante n'est demandée. La liste étant triée par date de modification décroissante,
	// le résultat contient exactement les PR modifiées depuis UpdatedSince (bornes incluses : celles de
	// la même seconde sont renvoyées). Synchro incrémentale : la plus récente date de modification en
	// base.
	UpdatedSince time.Time
	// Limit est le nombre maximum de pull requests renvoyées ; 0 (ou négatif) = pas de limite autre
	// que MaxPullRequests, et une valeur supérieure est ramenée à MaxPullRequests.
	Limit int
}

// Account est le compte de forge de l'auteur d'une pull request ou d'une release. Login est une donnée
// personnelle : jamais dans les logs.
type Account struct {
	Login     string // login ; vide si inconnu (compte supprimé)
	ID        int64  // identifiant numérique du compte, qui ne change jamais ; 0 si inconnu
	AvatarURL string // avatar ; vide si inconnu
}

// PullRequest est une pull request (merge request) d'un dépôt.
type PullRequest struct {
	Number     int       // numéro dans le dépôt (> 0)
	Title      string    // titre (donnée venue du dépôt : jamais dans les logs)
	State      string    // PullRequestOpen ou PullRequestClosed
	Merged     bool      // mergée (MergedAt non nul) ; une PR mergée est fermée
	Draft      bool      // brouillon
	Author     Account   // auteur
	URL        string    // page web de la PR (html_url sur GitHub)
	BaseBranch string    // branche visée
	HeadBranch string    // branche proposée
	CreatedAt  time.Time // ouverture, UTC
	UpdatedAt  time.Time // dernière modification, UTC
	ClosedAt   time.Time // fermeture, UTC ; zéro si ouverte
	MergedAt   time.Time // merge, UTC ; zéro si non mergée
}

// Release est une release d'un dépôt.
type Release struct {
	ID          int64     // identifiant de la release sur la forge (> 0)
	TagName     string    // tag de la release
	Name        string    // titre ; vide si aucun (donnée venue du dépôt : jamais dans les logs)
	Draft       bool      // brouillon
	Prerelease  bool      // pré-release
	Author      Account   // auteur
	URL         string    // page web de la release (html_url sur GitHub)
	CreatedAt   time.Time // création, UTC
	PublishedAt time.Time // publication, UTC ; zéro pour un brouillon
}

// Tag est un tag d'un dépôt.
type Tag struct {
	Name string // nom du tag
	SHA  string // SHA complet du commit visé
}

// Repository décrit un dépôt sur la forge.
type Repository struct {
	ID            int64     // identifiant numérique sur la forge, qui ne change jamais (model.Repository.ProviderID)
	Owner         string    // propriétaire ACTUEL (utilisateur ou organisation), avec la casse de la forge
	Name          string    // nom ACTUEL, avec la casse de la forge
	DefaultBranch string    // branche par défaut
	URL           string    // page web du dépôt (html_url sur GitHub)
	Stars         int       // nombre d'étoiles
	OpenIssues    int       // issues ouvertes (sur GitHub, pull requests ouvertes comprises)
	Archived      bool      // dépôt archivé
	Private       bool      // dépôt privé (ou interne)
	PushedAt      time.Time // dernier push, UTC ; zéro si inconnu
}

// FullName renvoie « owner/name ».
func (r Repository) FullName() string {
	return r.Owner + "/" + r.Name
}

// Branch est une branche d'un dépôt.
type Branch struct {
	Name    string // nom de la branche
	HeadSHA string // SHA complet du commit de tête
}

// Person est l'auteur ou le committer d'un commit : identité Git (nom, e-mail) et, si la forge a pu la
// rattacher à un compte, ce compte. Name et Email sont des données personnelles : jamais dans les logs.
type Person struct {
	Name      string // nom Git
	Email     string // e-mail Git, tel qu'écrit dans le commit (non normalisé)
	Login     string // login du compte lié ; vide si aucun compte n'est lié à cet e-mail
	ID        int64  // identifiant numérique du compte lié, qui ne change jamais ; 0 = aucun compte lié
	AvatarURL string // avatar du compte lié ; vide si aucun compte
}

// Commit est un commit tel que renvoyé par la forge (sans statistiques : voir CommitStats).
type Commit struct {
	SHA         string    // SHA complet
	Message     string    // message complet (donnée personnelle : jamais dans les logs)
	URL         string    // page web du commit (html_url sur GitHub)
	Author      Person    // auteur
	Committer   Person    // committer
	AuthoredAt  time.Time // date d'écriture, UTC
	CommittedAt time.Time // date de commit, UTC
	Parents     []string  // SHA des parents, dans l'ordre ; vide pour un commit racine, 2 ou plus pour un merge
}

// CommitStats donne la taille d'un commit.
type CommitStats struct {
	Additions int // lignes ajoutées
	Deletions int // lignes supprimées
}

// RateLimit est l'état du quota de l'API (ressource « core » sur GitHub : 5 000 requêtes par heure).
type RateLimit struct {
	Limit      int       // requêtes autorisées par fenêtre
	Remaining  int       // requêtes restantes dans la fenêtre courante
	Used       int       // requêtes consommées dans la fenêtre courante
	Reset      time.Time // fin de la fenêtre courante (remise à zéro), UTC
	ObservedAt time.Time // moment où cet état a été observé (réception de la réponse), UTC
}

// ShortSHA renvoie les 7 premiers caractères de sha (sha entier s'il est plus court), pour les logs et
// les messages : ils ne contiennent que des identifiants courts.
func ShortSHA(sha string) string {
	const n = 7
	if len(sha) <= n {
		return sha
	}
	return sha[:n]
}
