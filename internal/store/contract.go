package store

// Contrat de la phase 3 entre internal/store et ses appelants (internal/sync, internal/cli,
// internal/tui) : erreurs, types d'écriture d'une synchro et types de lecture (vues). Ces types ne
// contiennent que des types de la bibliothèque standard et de internal/model : jamais de type db.*.
//
// Les méthodes correspondantes sont dans repositories.go, syncruns.go, syncwrite.go et views.go.

import (
	"errors"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// ErrLocked : une synchro du dépôt est déjà en cours (sync_run « running » commencé il y a moins de
// staleAfter, voir BeginSyncRun). À tester avec errors.Is.
var ErrLocked = errors.New("déjà en cours de synchronisation")

// InterruptedMessage est le message enregistré dans sync_runs.error quand BeginSyncRun clôt un
// sync_run « running » trop ancien (processus arrêté en pleine synchro).
const InterruptedMessage = "synchro interrompue"

// Identity est l'auteur ou le committer d'un commit à enregistrer (voir ApplyRepoSync). Name et Email
// sont des données personnelles : jamais dans les logs ni dans les erreurs.
type Identity struct {
	Name           string // nom Git
	Email          string // e-mail Git tel que reçu ; normalisé en minuscules par store ; vide = inconnu
	Login          string // login du compte de forge lié ; vide si aucun
	ProviderUserID int64  // identifiant numérique du compte lié ; 0 = aucun compte lié
	AvatarURL      string // avatar du compte lié ; vide si aucun
}

// CommitWrite est un commit vu pendant une synchro (nouveau ou déjà en base).
type CommitWrite struct {
	SHA         string    // SHA complet
	Message     string    // message (donnée personnelle : jamais dans les logs)
	URL         string    // page web du commit
	Author      Identity  // auteur
	Committer   Identity  // committer
	AuthoredAt  time.Time // date d'écriture (convertie par FormatTime)
	CommittedAt time.Time // date de commit (convertie par FormatTime)
	Additions   int       // lignes ajoutées ; 0 si les statistiques n'ont pas été demandées
	Deletions   int       // lignes supprimées ; 0 si les statistiques n'ont pas été demandées
	// Branches sont les noms des branches (présentes dans RepoSyncWrite.Branches) sur lesquelles le
	// commit a été vu pendant cette synchro : une ligne commit_branches par branche.
	Branches []string
}

// BranchState est une branche suivie après filtrage, avec sa tête actuelle sur la forge.
type BranchState struct {
	Name    string // nom de la branche
	HeadSHA string // SHA complet de la tête, enregistré dans branches.head_sha
}

// RepositoryMetadata reprend les métadonnées d'un dépôt lues sur la forge (RepositoryByID), écrites
// dans repositories à chaque synchro réussie (renommage, transfert, archivage compris).
type RepositoryMetadata struct {
	Owner         string // propriétaire actuel
	Name          string // nom actuel
	DefaultBranch string // branche par défaut
	URL           string // page web du dépôt
	Stars         int    // étoiles
	OpenIssues    int    // issues ouvertes
	Archived      bool   // dépôt archivé
}

// RepoSyncWrite regroupe tout ce qu'une synchro réussie d'un dépôt écrit, appliqué par ApplyRepoSync
// dans UNE transaction.
type RepoSyncWrite struct {
	RepoID   int64              // dépôt synchronisé
	RunID    int64              // sync_run « running » ouvert par BeginSyncRun, clos en « success »
	Metadata RepositoryMetadata // métadonnées actuelles du dépôt
	// Branches sont les branches suivies (après filtrage par branch_patterns et branches.exclude) avec
	// leur tête actuelle : créées si besoin (tracked = 1) ou mises à jour (head_sha, tracked = 1).
	// Toute autre branche connue du dépôt passe à tracked = 0, son historique reste.
	Branches []BranchState
	// Commits sont les commits vus pendant la synchro, dans n'importe quel ordre ; un SHA peut y
	// figurer plusieurs fois (les Branches sont alors réunies). Un commit déjà en base n'est ni
	// réenregistré ni modifié : seules ses lignes commit_branches manquantes sont ajoutées.
	Commits []CommitWrite
	// PullRequests sont les pull requests lues pendant la synchro (modifiées depuis la précédente) :
	// chacune est créée ou mise à jour en entier (RepoID ignoré). Les autres PR du dépôt ne changent pas.
	PullRequests []model.PullRequest
	// ReplaceReleases indique que la liste des releases a été lue : celles du dépôt sont alors
	// remplacées par Releases (liste vide = aucune ; RepoID ignoré). Faux : releases inchangées (liste
	// non lue, en échec, ou non disponible pour cette source).
	ReplaceReleases bool
	Releases        []model.Release
	// ReplaceTags et Tags : de même pour les tags.
	ReplaceTags bool
	Tags        []model.Tag
	SyncedAt    time.Time // fin de la synchro : repositories.last_synced_at et sync_runs.finished_at
}

// RepoSyncResult est le résultat de ApplyRepoSync.
type RepoSyncResult struct {
	NewCommits      int       // commits réellement insérés (déjà en base exclus)
	TrackedBranches int       // branches suivies après la synchro
	LastCommitAt    time.Time // repositories.last_commit_at recalculé ; zéro si aucun commit
}

// CommitFilter règle ListRecentCommits. Sa valeur zéro liste tous les commits, sans limite.
type CommitFilter struct {
	RepoID int64 // dépôt (identifiant interne) ; 0 = tous les dépôts
	// Author filtre sur l'auteur : login de forge, nom ou e-mail rattaché, comparés sans tenir compte
	// de la casse (égalité, pas de sous-chaîne) ; vide = tous.
	Author string
	// Branch filtre sur le nom exact d'une branche (jointure commit_branches) ; vide = toutes.
	Branch string
	Since  time.Time // committed_at >= Since ; zéro = pas de borne
	Limit  int       // nombre maximum de commits ; 0 ou négatif = pas de limite
	// Offset est le nombre de commits sautés avant le premier renvoyé (pagination, phase 4) ; 0 ou
	// négatif = aucun. Ignoré par CountRecentCommits.
	Offset int
}

// CommitView est une ligne de la vue « derniers commits » : le commit, son dépôt et son auteur.
type CommitView struct {
	model.Commit
	RepoOwner   string // propriétaire actuel du dépôt
	RepoName    string // nom actuel du dépôt
	RepoLabel   string // nom affiché du dépôt (vide = owner/name)
	AuthorName  string // nom du contributeur auteur ; vide si inconnu
	AuthorLogin string // login de forge de l'auteur ; vide si inconnu
}

// RepoDisplayName renvoie le nom affiché du dépôt du commit (RepoLabel, sinon owner/name).
func (c CommitView) RepoDisplayName() string {
	if c.RepoLabel != "" {
		return c.RepoLabel
	}
	return c.RepoOwner + "/" + c.RepoName
}

// AuthorDisplayName renvoie le nom affiché de l'auteur (AuthorName, sinon AuthorLogin, sinon
// « inconnu »).
func (c CommitView) AuthorDisplayName() string {
	return model.Contributor{Name: c.AuthorName, ProviderLogin: c.AuthorLogin}.DisplayName()
}

// ContributorFilter règle ContributorRanking. Sa valeur zéro classe tous les contributeurs.
type ContributorFilter struct {
	RepoID int64     // dépôt ; 0 = tous
	Since  time.Time // commits dont committed_at >= Since ; zéro = pas de borne
	Limit  int       // nombre maximum de lignes ; 0 ou négatif = pas de limite
}

// ContributorStats est une ligne du classement des contributeurs (commits dont il est l'AUTEUR,
// comptés sur la table commits seule, donc sans doublon entre branches).
type ContributorStats struct {
	model.Contributor           // Emails reste nil
	Commits           int       // nombre de commits
	Additions         int       // somme des lignes ajoutées (statistiques connues seulement)
	Deletions         int       // somme des lignes supprimées
	Repos             int       // nombre de dépôts distincts
	LastCommitAt      time.Time // date du commit le plus récent (committed_at)
}

// RepositorySummary est une ligne de la vue « résumé par dépôt » (repo list, status, TUI).
type RepositorySummary struct {
	model.Repository
	Commits         int // nombre de commits en base
	TrackedBranches int // branches à tracked = 1
	// LastCommit est le commit le plus récent sur les branches suivies (celui de LastCommitAt ;
	// égalité départagée par SHA croissant) ; valeur zéro (SHA vide) si aucun.
	LastCommit CommitView
	// LastRun est le dernier sync_run du dépôt (started_at le plus récent, puis id) ; ID 0 si aucun.
	LastRun model.SyncRun
	// OpenPullRequests est le nombre de pull requests ouvertes en base (V2).
	OpenPullRequests int
	// LatestRelease est la release publiée (hors brouillons) la plus récente (publication, sinon
	// création) ; valeur zéro (ProviderID 0) si aucune (V2).
	LatestRelease model.Release
}

// PullRequestFilter règle ListPullRequests. Sa valeur zéro liste toutes les pull requests.
type PullRequestFilter struct {
	RepoID int64 // dépôt ; 0 = tous
	// Status filtre sur l'état affiché (model.PullRequestStatus…) ; vide = tous.
	Status model.PullRequestStatus
	Limit  int // nombre maximum de lignes ; 0 ou négatif = pas de limite
}

// RepoRef désigne le dépôt d'une ligne de vue (pull request, release, tag).
type RepoRef struct {
	RepoOwner string // propriétaire actuel du dépôt
	RepoName  string // nom actuel du dépôt
	RepoLabel string // nom affiché du dépôt (vide = owner/name)
}

// RepoFullName renvoie « owner/name ».
func (r RepoRef) RepoFullName() string { return r.RepoOwner + "/" + r.RepoName }

// RepoDisplayName renvoie le nom affiché du dépôt (RepoLabel, sinon owner/name).
func (r RepoRef) RepoDisplayName() string {
	if r.RepoLabel != "" {
		return r.RepoLabel
	}
	return r.RepoFullName()
}

// PullRequestView est une ligne de la vue des pull requests : la PR et son dépôt.
type PullRequestView struct {
	model.PullRequest
	RepoRef
}

// PullRequestCounts compte les pull requests d'un dépôt par état affiché.
type PullRequestCounts struct {
	Open   int // ouvertes (brouillons compris)
	Merged int // mergées
	Closed int // fermées sans merge
}

// ReleaseFilter règle ListReleases et ListTags. Sa valeur zéro liste tout.
type ReleaseFilter struct {
	RepoID int64 // dépôt ; 0 = tous
	Limit  int   // nombre maximum de lignes ; 0 ou négatif = pas de limite
}

// ReleaseView est une ligne de la vue des releases : la release et son dépôt.
type ReleaseView struct {
	model.Release
	RepoRef
}

// TagView est une ligne de la vue des tags : le tag, son dépôt et le commit visé s'il est en base.
type TagView struct {
	model.Tag
	RepoRef
	CommittedAt time.Time // date du commit visé ; zéro s'il n'est pas en base
	CommitURL   string    // lien vers le commit visé ; vide s'il n'est pas en base
}
