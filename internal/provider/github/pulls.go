package github

// Appels d'API sur les pull requests, les releases et les tags (V2 ; voir les règles communes en tête
// de repos.go). Les titres de PR, noms de release et logins ne sont jamais journalisés.

import (
	"context"
	"errors"
	"time"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Plafonds de pages des listes de la V2 (voir maxBranchPages).
const (
	maxPullPages    = provider.MaxPullRequests/PerPage + 1
	maxReleasePages = provider.MaxReleases/PerPage + 1
	maxTagPages     = provider.MaxTags/PerPage + 1
)

// stopUpdatedBefore : raison d'arrêt de la liste des pull requests à la première PR modifiée avant
// PullRequestsOptions.UpdatedSince.
const stopUpdatedBefore = "PR plus ancienne"

// PullRequests implémente provider.Provider : GET /repos/{owner}/{name}/pulls?state=all&sort=updated
// &direction=desc&per_page=100&page=n (PullRequestsService.List), après provider.ValidateRepo.
//
//   - les PR sont parcourues dans l'ordre de l'API (modification la plus récente d'abord) ; si
//     opts.UpdatedSince n'est pas zéro, la liste s'arrête à la première PR dont updated_at (à la
//     seconde) est strictement antérieure à UpdatedSince tronqué à la seconde : elle et les suivantes
//     sont exclues, aucune page suivante n'est demandée ;
//   - limite effective : opts.Limit si 0 < Limit < provider.MaxPullRequests, sinon
//     provider.MaxPullRequests ;
//   - erreur enveloppant provider.ErrEmptyRepository (409) → liste vide non nil (GitHub répond
//     d'ailleurs 200 [] pour un dépôt vide).
//
// Conversion (toPullRequest) : Number = number ; Title = title ; State = state (« open » ou
// « closed ») ; MergedAt = merged_at ; Merged = merged, ou merged_at non nul (la liste de l'API ne
// donne pas le champ merged) ; Draft = draft ; Author = user.{login, id, avatar_url} ; URL = html_url ;
// BaseBranch = base.ref ; HeadBranch = head.ref ; dates en UTC. Une PR sans numéro positif, d'état
// inconnu ou sans updated_at → p.unexpected.
func (p *Provider) PullRequests(ctx context.Context, owner, name string, opts provider.PullRequestsOptions) ([]provider.PullRequest, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op := "liste des pull requests de " + owner + "/" + name
	limit := opts.Limit
	if limit <= 0 || limit > provider.MaxPullRequests {
		limit = provider.MaxPullRequests
	}
	since := opts.UpdatedSince.UTC().Truncate(time.Second)
	lo := &gogithub.PullRequestListOptions{
		State:       "all",
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: gogithub.ListOptions{PerPage: PerPage, Page: 1},
	}

	pulls := make([]provider.PullRequest, 0)
	pages, stop := 0, stopLastPage
pagination:
	for {
		list, resp, err := p.client.PullRequests.List(ctx, owner, name, lo)
		pages++
		if err != nil {
			err = p.convertError(ctx, op, resp, err)
			if errors.Is(err, provider.ErrEmptyRepository) {
				p.logList(ctx, "pull requests listées", owner, name, pages, 0, stopEmpty)
				return make([]provider.PullRequest, 0), nil
			}
			return nil, err
		}
		for _, pr := range list {
			c, ok := toPullRequest(pr)
			if !ok {
				return nil, p.unexpected(op, resp)
			}
			if !opts.UpdatedSince.IsZero() && c.UpdatedAt.Before(since) {
				stop = stopUpdatedBefore
				break pagination
			}
			pulls = append(pulls, c)
			if len(pulls) >= limit {
				stop = stopLimit
				if limit == provider.MaxPullRequests {
					stop = stopCap
				}
				break pagination
			}
		}
		next, reason := nextPage(resp, lo.Page, pages, len(list), maxPullPages)
		if next == 0 {
			stop = reason
			break
		}
		lo.Page = next
	}
	p.logList(ctx, "pull requests listées", owner, name, pages, len(pulls), stop)
	return pulls, nil
}

// toPullRequest convertit une pull request de go-github ; false si elle est inexploitable.
func toPullRequest(pr *gogithub.PullRequest) (provider.PullRequest, bool) {
	if pr == nil || pr.GetNumber() <= 0 || pr.GetUpdatedAt().IsZero() {
		return provider.PullRequest{}, false
	}
	state := pr.GetState()
	if state != provider.PullRequestOpen && state != provider.PullRequestClosed {
		return provider.PullRequest{}, false
	}
	mergedAt := utc(pr.GetMergedAt())
	return provider.PullRequest{
		Number:     pr.GetNumber(),
		Title:      pr.GetTitle(),
		State:      state,
		Merged:     pr.GetMerged() || !mergedAt.IsZero(),
		Draft:      pr.GetDraft(),
		Author:     toAccount(pr.GetUser()),
		URL:        pr.GetHTMLURL(),
		BaseBranch: pr.GetBase().GetRef(),
		HeadBranch: pr.GetHead().GetRef(),
		CreatedAt:  utc(pr.GetCreatedAt()),
		UpdatedAt:  utc(pr.GetUpdatedAt()),
		ClosedAt:   utc(pr.GetClosedAt()),
		MergedAt:   mergedAt,
	}, true
}

// toAccount convertit le compte d'un auteur (nil : compte inconnu, valeur zéro).
func toAccount(u *gogithub.User) provider.Account {
	return provider.Account{Login: u.GetLogin(), ID: u.GetID(), AvatarURL: u.GetAvatarURL()}
}

// Releases implémente provider.Provider : GET /repos/{owner}/{name}/releases?per_page=100&page=n
// (RepositoriesService.ListReleases), toutes les pages, au plus provider.MaxReleases, dans l'ordre de
// l'API (la plus récente d'abord). 409 (dépôt vide) → liste vide non nil.
//
// Conversion : ID = id ; TagName = tag_name ; Name = name ; Draft = draft ; Prerelease = prerelease ;
// Author = author.{login, id, avatar_url} ; URL = html_url ; CreatedAt = created_at ; PublishedAt =
// published_at (null pour un brouillon : zéro). Une release sans identifiant positif ou sans tag →
// p.unexpected.
func (p *Provider) Releases(ctx context.Context, owner, name string) ([]provider.Release, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op := "liste des releases de " + owner + "/" + name
	lo := &gogithub.ListOptions{PerPage: PerPage, Page: 1}
	releases := make([]provider.Release, 0)
	pages, stop := 0, stopLastPage
pagination:
	for {
		list, resp, err := p.client.Repositories.ListReleases(ctx, owner, name, lo)
		pages++
		if err != nil {
			err = p.convertError(ctx, op, resp, err)
			if errors.Is(err, provider.ErrEmptyRepository) {
				p.logList(ctx, "releases listées", owner, name, pages, 0, stopEmpty)
				return make([]provider.Release, 0), nil
			}
			return nil, err
		}
		for _, r := range list {
			if r == nil || r.GetID() <= 0 || r.GetTagName() == "" {
				return nil, p.unexpected(op, resp)
			}
			releases = append(releases, provider.Release{
				ID:          r.GetID(),
				TagName:     r.GetTagName(),
				Name:        r.GetName(),
				Draft:       r.GetDraft(),
				Prerelease:  r.GetPrerelease(),
				Author:      toAccount(r.GetAuthor()),
				URL:         r.GetHTMLURL(),
				CreatedAt:   utc(r.GetCreatedAt()),
				PublishedAt: utc(r.GetPublishedAt()),
			})
			if len(releases) >= provider.MaxReleases {
				stop = stopCap
				break pagination
			}
		}
		next, reason := nextPage(resp, lo.Page, pages, len(list), maxReleasePages)
		if next == 0 {
			stop = reason
			break
		}
		lo.Page = next
	}
	p.logList(ctx, "releases listées", owner, name, pages, len(releases), stop)
	return releases, nil
}

// Tags implémente provider.Provider : GET /repos/{owner}/{name}/tags?per_page=100&page=n
// (RepositoriesService.ListTags), toutes les pages, au plus provider.MaxTags, dans l'ordre de l'API.
// 409 (dépôt vide) → liste vide non nil.
//
// Conversion : Name = name ; SHA = commit.sha. Un tag sans nom ou dont le SHA n'est pas un SHA complet
// (provider.ValidateSHA) → p.unexpected.
func (p *Provider) Tags(ctx context.Context, owner, name string) ([]provider.Tag, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	op := "liste des tags de " + owner + "/" + name
	lo := &gogithub.ListOptions{PerPage: PerPage, Page: 1}
	tags := make([]provider.Tag, 0)
	pages, stop := 0, stopLastPage
pagination:
	for {
		list, resp, err := p.client.Repositories.ListTags(ctx, owner, name, lo)
		pages++
		if err != nil {
			err = p.convertError(ctx, op, resp, err)
			if errors.Is(err, provider.ErrEmptyRepository) {
				p.logList(ctx, "tags listés", owner, name, pages, 0, stopEmpty)
				return make([]provider.Tag, 0), nil
			}
			return nil, err
		}
		for _, t := range list {
			sha := t.GetCommit().GetSHA()
			if t.GetName() == "" || provider.ValidateSHA(sha) != nil {
				return nil, p.unexpected(op, resp)
			}
			tags = append(tags, provider.Tag{Name: t.GetName(), SHA: sha})
			if len(tags) >= provider.MaxTags {
				stop = stopCap
				break pagination
			}
		}
		next, reason := nextPage(resp, lo.Page, pages, len(list), maxTagPages)
		if next == 0 {
			stop = reason
			break
		}
		lo.Page = next
	}
	p.logList(ctx, "tags listés", owner, name, pages, len(tags), stop)
	return tags, nil
}
