package github

// Appels d'API sur les dépôts et les branches. Règles communes aux trois fichiers :
//   - valider les arguments (provider.ValidateRepo, ValidateSHA, ValidateBranch, ValidateID) AVANT
//     tout appel ; l'erreur de validation est renvoyée telle quelle (elle enveloppe ErrInvalidRef) ;
//   - appeler go-github par p.client ; toute erreur de go-github passe par
//     p.convertError(ctx, op, resp, err), avec le contexte de l'appelant (pas de classement local, sauf
//     les deux affinages documentés dans commits.go : 404 de CompareCommits, 422 « No commit found » de
//     CommitStats), une réponse inexploitable par p.unexpected(op, resp) ;
//   - op : « lecture du dépôt alice/projet », « lecture du dépôt n° 123 », « liste des branches de
//     alice/projet », « liste des commits de alice/projet », « comparaison 1a2b3c4...5d6e7f8 dans
//     alice/projet », « statistiques du commit 1a2b3c4 de alice/projet », « lecture du quota de
//     l'API » (SHA courts : provider.ShortSHA) ;
//   - pagination : ListOptions{PerPage: PerPage, Page: n}, page suivante = resp.NextPage ; arrêt quand
//     NextPage vaut 0, quand NextPage <= page courante (garde-fou), au plafond (provider.MaxCommits,
//     provider.MaxBranches), ou au plus tard après plafond/PerPage + 1 pages ;
//   - dates : Timestamp.Time convertie en UTC ; listes vides : slices vides non nil (Parents compris) ;
//   - logs : p.logger, niveau Debug, sans message de commit, nom, e-mail ni login.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Plafonds de pages : au plus tard après plafond/PerPage + 1 pages, la pagination s'arrête, même si
// le serveur annonce encore une page suivante (garde-fou contre une boucle sans fin).
const (
	maxBranchPages = provider.MaxBranches/PerPage + 1
	maxCommitPages = provider.MaxCommits/PerPage + 1
)

// Raisons d'arrêt d'une pagination, pour les logs Debug.
const (
	stopLastPage = "dernière page"
	stopLimit    = "limite atteinte"
	stopCap      = "plafond atteint"
	stopKnownSHA = "SHA connu"
	stopMaxPages = "nombre maximum de pages"
	stopEmpty    = "dépôt vide"
)

// Repository implémente provider.Provider : GET /repos/{owner}/{name} (RepositoriesService.Get), après
// provider.ValidateRepo.
//
// Un dépôt renommé ou transféré répond 301 vers /repositories/{id} : le http.Client suit la
// redirection (GET) et le résultat porte les owner/nom ACTUELS, différents des arguments.
//
// Conversion (toRepository, utilisée aussi par RepositoryByID) : ID = id ; Owner = owner.login ;
// Name = name ; DefaultBranch = default_branch ; URL = html_url ; Stars = stargazers_count ;
// OpenIssues = open_issues_count ; Archived = archived ; Private = private ; PushedAt = pushed_at en
// UTC (zéro si absent). Un dépôt sans id, sans owner.login ou sans name → p.unexpected.
func (p *Provider) Repository(ctx context.Context, owner, name string) (provider.Repository, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.Repository{}, err
	}
	op := "lecture du dépôt " + owner + "/" + name
	repo, resp, err := p.client.Repositories.Get(ctx, owner, name)
	if err != nil {
		return provider.Repository{}, p.convertError(ctx, op, resp, err)
	}
	return p.toRepository(op, resp, repo)
}

// RepositoryByID implémente provider.Provider : GET /repositories/{id} (RepositoriesService.GetByID),
// après provider.ValidateID. Même conversion que Repository.
func (p *Provider) RepositoryByID(ctx context.Context, id int64) (provider.Repository, error) {
	if err := provider.ValidateID(id); err != nil {
		return provider.Repository{}, err
	}
	op := fmt.Sprintf("lecture du dépôt n° %d", id)
	repo, resp, err := p.client.Repositories.GetByID(ctx, id)
	if err != nil {
		return provider.Repository{}, p.convertError(ctx, op, resp, err)
	}
	return p.toRepository(op, resp, repo)
}

// toRepository convertit le dépôt renvoyé par go-github pour l'opération op. Un dépôt absent (corps
// « null »), sans identifiant, sans owner.login ou sans name est inexploitable : p.unexpected.
func (p *Provider) toRepository(op string, resp *gogithub.Response, r *gogithub.Repository) (provider.Repository, error) {
	if r == nil || r.GetID() <= 0 || r.GetOwner().GetLogin() == "" || r.GetName() == "" {
		return provider.Repository{}, p.unexpected(op, resp)
	}
	return provider.Repository{
		ID:            r.GetID(),
		Owner:         r.GetOwner().GetLogin(),
		Name:          r.GetName(),
		DefaultBranch: r.GetDefaultBranch(),
		URL:           r.GetHTMLURL(),
		Stars:         r.GetStargazersCount(),
		OpenIssues:    r.GetOpenIssuesCount(),
		Archived:      r.GetArchived(),
		Private:       r.GetPrivate(),
		PushedAt:      utc(r.GetPushedAt()),
	}, nil
}

// Branches implémente provider.Provider : GET /repos/{owner}/{name}/branches?per_page=100&page=n
// (RepositoriesService.ListBranches, sans filtre protected), toutes les pages, au plus
// provider.MaxBranches branches, dans l'ordre de l'API.
//
// Conversion : Name = name ; HeadSHA = commit.sha. Une branche sans nom, ou dont le SHA de tête est
// absent ou n'est pas un SHA complet (provider.ValidateSHA), est inexploitable : p.unexpected.
// Erreur enveloppant provider.ErrEmptyRepository (409) → liste vide non nil, sans erreur (GitHub
// répond d'ailleurs 200 [] pour la liste des branches d'un dépôt vide).
func (p *Provider) Branches(ctx context.Context, owner, name string) ([]provider.Branch, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op := "liste des branches de " + owner + "/" + name
	opts := &gogithub.BranchListOptions{ListOptions: gogithub.ListOptions{PerPage: PerPage, Page: 1}}
	branches := make([]provider.Branch, 0)
	pages, stop := 0, stopLastPage

pagination:
	for {
		list, resp, err := p.client.Repositories.ListBranches(ctx, owner, name, opts)
		pages++
		if err != nil {
			err = p.convertError(ctx, op, resp, err)
			if errors.Is(err, provider.ErrEmptyRepository) {
				p.logList(ctx, "branches listées", owner, name, pages, 0, stopEmpty)
				return make([]provider.Branch, 0), nil
			}
			return nil, err
		}
		for _, b := range list {
			sha := b.GetCommit().GetSHA()
			if b.GetName() == "" || provider.ValidateSHA(sha) != nil {
				return nil, p.unexpected(op, resp)
			}
			branches = append(branches, provider.Branch{Name: b.GetName(), HeadSHA: sha})
			if len(branches) >= provider.MaxBranches {
				stop = stopCap
				break pagination
			}
		}
		next, reason := nextPage(resp, opts.Page, pages, len(list), maxBranchPages)
		if next == 0 {
			stop = reason
			break
		}
		opts.Page = next
	}
	p.logList(ctx, "branches listées", owner, name, pages, len(branches), stop)
	return branches, nil
}

// nextPage renvoie la page à demander après la page courante page (la pages-ième lue, qui contenait n
// éléments), ou 0 et la raison de l'arrêt : pas de page suivante annoncée (resp.NextPage nul), page
// annoncée qui ne progresse pas (resp.NextPage <= page : garde-fou), page vide, ou maxPages pages
// déjà lues.
func nextPage(resp *gogithub.Response, page, pages, n, maxPages int) (int, string) {
	switch {
	case resp == nil || resp.NextPage <= page || n == 0:
		return 0, stopLastPage
	case pages >= maxPages:
		return 0, stopMaxPages
	}
	return resp.NextPage, ""
}

// logList journalise, au niveau Debug, la fin d'une liste paginée : dépôt, nombre de pages lues,
// nombre d'éléments et raison de l'arrêt. Jamais de nom de branche, de message, de nom ni d'e-mail.
func (p *Provider) logList(ctx context.Context, msg, owner, name string, pages, n int, stop string) {
	p.logger.DebugContext(ctx, msg,
		slog.String("depot", owner+"/"+name),
		slog.Int("pages", pages),
		slog.Int("elements", n),
		slog.String("arret", stop),
	)
}

// utc convertit une date de go-github en UTC ; la valeur zéro (date absente) reste zéro.
func utc(ts gogithub.Timestamp) time.Time {
	if ts.IsZero() {
		return time.Time{}
	}
	return ts.UTC()
}
