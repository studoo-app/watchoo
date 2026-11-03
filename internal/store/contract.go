package store

// Contrat de la phase 3 entre internal/store et ses appelants (internal/sync, internal/cli,
// internal/tui) : erreurs, types d'écriture d'une synchro et types de lecture (vues). Ces types ne
// contiennent que des types de la bibliothèque standard et de internal/model : jamais de type db.*.
//
// Les méthodes correspondantes sont dans repositories.go, syncruns.go, syncwrite.go et views.go.

import (
	"slices"
	"time"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
)

// ErrLocked : une synchro du dépôt est déjà en cours (sync_run « running » commencé il y a moins de
// staleAfter, voir BeginSyncRun). À tester avec errors.Is.
var ErrLocked error = i18nError{"store.err.locked"}

// InterruptedMessage renvoie le message enregistré dans sync_runs.error quand BeginSyncRun clôt un
// sync_run « running » trop ancien (processus arrêté en pleine synchro), dans la langue du processus
// au moment de la synchro.
func InterruptedMessage() string { return i18n.T("store.runs.interrupted") }

// Identity est l'auteur ou le committer d'un commit à enregistrer (voir ApplyRepoSync). Name et Email
// sont des données personnelles : jamais dans les logs ni dans les erreurs.
type Identity struct {
	Name           string // nom Git
	Email          string // e-mail Git tel que reçu ; normalisé en minuscules par store ; vide = inconnu
	Login          string // login du compte de forge lié ; vide si aucun
	ProviderUserID int64  // identifiant numérique du compte lié ; 0 = aucun compte lié
	AvatarURL      string // avatar du compte lié ; vide si aucun
	ProfileURL     string // page de profil du compte lié (donnée par le provider) ; vide si aucun
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
	Additions   int       // lignes ajoutées ; 0 si les statistiques sont inconnues
	Deletions   int       // lignes supprimées ; 0 si les statistiques sont inconnues
	// StatsKnown : Additions et Deletions ont été calculées (même à 0) ; faux = inconnues
	// (commits.stats_known = 0, reprises plus tard par RepoSyncWrite.Stats).
	StatsKnown bool
	// Languages : lignes ajoutées par langage (1.2.0), lues avec RepoSyncWrite.LanguagesVersion ; nil =
	// inconnues (commits.languages_version = 0, reprises plus tard par RepoSyncWrite.Stats).
	Languages map[string]int
	// Branches sont les noms des branches (présentes dans RepoSyncWrite.Branches) sur lesquelles le
	// commit a été vu pendant cette synchro : une ligne commit_branches par branche.
	Branches []string
}

// CommitStatsWrite est la statistique d'un commit du dépôt calculée pendant une synchro (clone Git
// local), pour un commit nouveau ou déjà en base dont les statistiques étaient inconnues.
type CommitStatsWrite struct {
	SHA       string // SHA complet
	Additions int    // lignes ajoutées (>= 0)
	Deletions int    // lignes supprimées (>= 0)
	// Languages : lignes ajoutées par langage (1.2.0), lues avec RepoSyncWrite.LanguagesVersion ; nil =
	// non lues (langages inchangés).
	Languages map[string]int
}

// BranchState est une branche suivie après filtrage, avec sa tête actuelle sur la forge.
type BranchState struct {
	Name    string // nom de la branche
	HeadSHA string // SHA complet de la tête, enregistré dans branches.head_sha
}

// RepositoryMetadata reprend les métadonnées d'un dépôt lues sur la forge (RepositoryByID), écrites
// dans repositories à chaque synchro réussie (renommage, transfert, archivage compris).
type RepositoryMetadata struct {
	// Partial : source qui ne connaît que la branche par défaut (clone Git local, V2). Seules
	// DefaultBranch (si elle n'est pas vide) et la date de synchro sont alors écrites ; owner, name,
	// url, étoiles, issues, archivé et provider_id gardent leur valeur en base.
	Partial bool
	// ProviderID est l'identifiant du dépôt sur la forge : > 0 = écrit (premier passage par l'API
	// d'un dépôt ajouté sans gh) ; 0 = gardé.
	ProviderID    int64
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
	// Stats sont les statistiques calculées pendant la synchro, écrites APRÈS les commits (un SHA de
	// Commits peut donc y figurer) : additions, deletions et stats_known = 1. Un SHA absent de la base
	// est ignoré.
	Stats []CommitStatsWrite
	// LanguagesVersion est la version des règles de langages (git.LanguageRulesVersion, > 0) avec
	// laquelle les Languages des Commits et des Stats ont été lus ; obligatoire si l'un d'eux en a.
	LanguagesVersion int64
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
	// ReplaceLanguages et Languages : de même pour les langages (clone lu ; RepoID ignoré).
	ReplaceLanguages bool
	Languages        []model.RepositoryLanguage
	SyncedAt         time.Time // fin de la synchro : repositories.last_synced_at et sync_runs.finished_at
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
	// AuthorID filtre sur le contributeur auteur (contributors.id, page d'un contributeur) ; 0 = tous.
	AuthorID int64
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
	// AuthorProfileURL est la page de profil de l'auteur sur la forge ; vide si inconnue.
	AuthorProfileURL string
	AISignatures     []model.AISignature // signatures d'IA, triées par outil puis type ; vide non nil
	AIScanned        bool                // commit déjà analysé (faux : AISignatures ne dit encore rien)
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
	model.Contributor                      // Emails reste nil
	Commits           int                  // nombre de commits
	Additions         int                  // somme des lignes ajoutées (statistiques connues seulement)
	Deletions         int                  // somme des lignes supprimées
	KnownCommits      int                  // commits dont les statistiques sont connues
	Repos             int                  // nombre de dépôts distincts
	LastCommitAt      time.Time            // date du commit le plus récent (committed_at)
	AI                model.AITotals       // signatures d'IA de ces commits
	Languages         model.LanguageTotals // lignes ajoutées par langage dans ces commits (1.2.0)
}

// Lines renvoie les totaux de lignes des commits du classement.
func (c ContributorStats) Lines() model.LineTotals {
	return model.LineTotals{Commits: c.Commits, KnownCommits: c.KnownCommits, Additions: c.Additions,
		Deletions: c.Deletions}
}

// AITools renvoie les outils d'IA distincts signés sur le commit, triés ; liste vide non nil si aucun.
func (c CommitView) AITools() []string {
	tools := make([]string, 0, len(c.AISignatures))
	for _, sig := range c.AISignatures {
		if !slices.Contains(tools, sig.Tool) {
			tools = append(tools, sig.Tool)
		}
	}
	return tools
}

// SumLines additionne les totaux de lignes des dépôts summaries (totaux de l'espace).
func SumLines(summaries []RepositorySummary) model.LineTotals {
	var t model.LineTotals
	for _, s := range summaries {
		t = t.Add(s.Lines)
	}
	return t
}

// RepositorySummary est une ligne de la vue « résumé par dépôt » (repo list, status, TUI).
type RepositorySummary struct {
	model.Repository
	Commits         int // nombre de commits en base
	TrackedBranches int // branches à tracked = 1
	// Lines additionne les lignes ajoutées et supprimées des commits du dépôt (Lines.Commits = Commits).
	Lines model.LineTotals
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
	// AuthorProviderID filtre sur le compte de forge de l'auteur (pull_requests.author_provider_id,
	// rapproché de contributors.provider_user_id) ; 0 = tous.
	AuthorProviderID int64
	Limit            int // nombre maximum de lignes ; 0 ou négatif = pas de limite
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

// LanguageView est une ligne de la vue des langages : le langage, son dépôt et le total de ce dépôt.
type LanguageView struct {
	model.RepositoryLanguage
	RepoRef
	TotalBytes int64 // somme des langages du dépôt
}

// Permille renvoie la part du langage dans son dépôt, en pour mille.
func (v LanguageView) Permille() int { return model.Permille(v.Bytes, v.TotalBytes) }

// Lines renvoie les totaux de lignes des commits du contributeur.
func (p ContributorProfile) Lines() model.LineTotals {
	return model.LineTotals{Commits: p.Commits, KnownCommits: p.KnownCommits, Additions: p.Additions,
		Deletions: p.Deletions}
}

// ContributorProfile est la fiche d'un contributeur (page d'un contributeur, 0.5.0) : le contributeur
// et les totaux des commits dont il est l'AUTEUR.
type ContributorProfile struct {
	model.Contributor // Emails reste nil : seul leur nombre est lu (Identities)
	// Identities est le nombre d'adresses e-mail Git rattachées au contributeur ; les adresses ne
	// sont jamais lues ici.
	Identities    int
	Commits       int                  // commits dont il est l'auteur
	Additions     int                  // lignes ajoutées (statistiques connues seulement)
	Deletions     int                  // lignes supprimées
	KnownCommits  int                  // commits dont les statistiques sont connues
	Repos         int                  // dépôts distincts
	FirstCommitAt time.Time            // premier commit (committed_at) ; zéro si aucun
	LastCommitAt  time.Time            // dernier commit (committed_at) ; zéro si aucun
	AI            model.AITotals       // signatures d'IA de ses commits
	Languages     model.LanguageTotals // lignes ajoutées par langage dans ses commits (1.2.0)
}

// AIScanCommit est un commit à analyser pour les signatures d'IA, avec son auteur.
type AIScanCommit struct {
	SHA          string
	Message      string
	AuthorLogin  string
	AuthorEmails []string
}

// AIScanResult est le résultat de l'analyse d'un commit.
type AIScanResult struct {
	SHA        string
	Signatures []model.AISignature
}
