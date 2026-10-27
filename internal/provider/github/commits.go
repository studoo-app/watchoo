package github

// Appels d'API sur les commits (voir les règles communes en tête de repos.go).

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	gogithub "github.com/google/go-github/v93/github"

	"github.com/studoo-app/watchoo/internal/provider"
)

// Statuts d'une comparaison (champ status de GET …/compare) pour lesquels head n'apporte aucun commit
// par rapport à base.
const (
	compareBehind    = "behind"
	compareIdentical = "identical"
)

// ListCommits implémente provider.Provider : GET /repos/{owner}/{name}/commits?sha=<Branch>&since=
// <Since>&per_page=100&page=n (RepositoriesService.ListCommits), après provider.ValidateRepo et, si
// opts.Branch n'est pas vide, provider.ValidateBranch.
//
//   - sha = opts.Branch (omis s'il est vide : branche par défaut), encodé dans la query par go-github
//     (« feature/login » devient « feature%2Flogin ») ; since = opts.Since en UTC (omis s'il est zéro ;
//     format RFC 3339, à la seconde) ; per_page = PerPage, toujours (clés de cache stables) ;
//   - les commits sont parcourus dans l'ordre de l'API (du plus récent au plus ancien) ; au premier
//     commit pour lequel opts.Stop renvoie true, la liste s'arrête : ce commit et les suivants sont
//     exclus, aucune page suivante n'est demandée ;
//   - limite effective : opts.Limit si 0 < Limit < provider.MaxCommits, sinon provider.MaxCommits ;
//     dès qu'elle est atteinte, aucune page suivante n'est demandée ;
//   - erreur enveloppant provider.ErrEmptyRepository (409, dépôt vide) → liste vide non nil, sans
//     erreur ; branche inconnue → 404 → erreur enveloppant provider.ErrNotFound (convertError).
//
// Conversion (toCommit, utilisée aussi par CompareCommits) depuis *RepositoryCommit : SHA = sha ;
// Message = commit.message ; URL = html_url ; Author = {commit.author.name, commit.author.email,
// author.login, author.id, author.avatar_url} (author est nul quand aucun compte GitHub n'est lié à
// l'e-mail : Login, ID et AvatarURL restent vides) ; Committer de même avec commit.committer et
// committer ; AuthoredAt = commit.author.date et CommittedAt = commit.committer.date, en UTC ;
// Parents = parents[].sha dans l'ordre (slice vide non nil pour un commit racine). Un commit sans SHA
// complet (ou dont un parent n'en a pas) → p.unexpected.
func (p *Provider) ListCommits(ctx context.Context, owner, name string, opts provider.ListCommitsOptions) ([]provider.Commit, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if opts.Branch != "" {
		if err := provider.ValidateBranch(opts.Branch); err != nil {
			return nil, err
		}
	}
	op := "liste des commits de " + owner + "/" + name
	limit := opts.Limit
	if limit <= 0 || limit > provider.MaxCommits {
		limit = provider.MaxCommits
	}
	lo := &gogithub.CommitsListOptions{
		SHA:         opts.Branch,
		ListOptions: gogithub.ListOptions{PerPage: PerPage, Page: 1},
	}
	if !opts.Since.IsZero() {
		lo.Since = opts.Since.UTC()
	}

	commits := make([]provider.Commit, 0)
	pages, stop := 0, stopLastPage
pagination:
	for {
		list, resp, err := p.client.Repositories.ListCommits(ctx, owner, name, lo)
		pages++
		if err != nil {
			err = p.convertError(ctx, op, resp, err)
			if errors.Is(err, provider.ErrEmptyRepository) {
				p.logList(ctx, "commits listés", owner, name, pages, 0, stopEmpty)
				return make([]provider.Commit, 0), nil
			}
			return nil, err
		}
		for _, rc := range list {
			c, ok := toCommit(rc)
			if !ok {
				return nil, p.unexpected(op, resp)
			}
			if opts.Stop != nil && opts.Stop(c.SHA) {
				stop = stopKnownSHA
				break pagination
			}
			commits = append(commits, c)
			if len(commits) >= limit {
				stop = stopLimit
				if limit == provider.MaxCommits {
					stop = stopCap
				}
				break pagination
			}
		}
		next, reason := nextPage(resp, lo.Page, pages, len(list), maxCommitPages)
		if next == 0 {
			stop = reason
			break
		}
		lo.Page = next
	}
	p.logList(ctx, "commits listés", owner, name, pages, len(commits), stop)
	return commits, nil
}

// CompareCommits implémente provider.Provider : GET /repos/{owner}/{name}/compare/{base}...{head}
// ?per_page=100&page=n (RepositoriesService.CompareCommits), après provider.ValidateRepo et
// provider.ValidateSHA sur base et head.
//
//   - base == head : liste vide non nil, sans requête ;
//   - statut « behind » ou « identical » (head n'apporte rien) : liste vide non nil, sans autre page ;
//   - les pages de commits sont lues dans l'ordre (l'API les donne du plus ANCIEN au plus récent, page
//     1 = les plus anciens) jusqu'à total_commits commits (champ de la première page ; s'il manque,
//     jusqu'à la dernière page annoncée par l'en-tête Link), au plus provider.MaxCommits (au-delà, ce
//     sont donc les plus anciens qui sont gardés) ; page suivante = resp.NextPage, ou la page d'après
//     si l'en-tête Link manque alors que total_commits n'est pas atteint et que la page était pleine ;
//     arrêt sur une page vide et au plus tard après maxCommitPages pages. L'ordre est ensuite inversé :
//     résultat du plus récent au plus ancien ;
//   - conversion de chaque commit par toCommit (voir ListCommits) ;
//   - 409 (dépôt vide) → erreur enveloppant provider.ErrEmptyRepository (renvoyée, pas masquée) ;
//   - 404 : GitHub répond « Not Found » aussi bien pour un SHA de base (ou de tête) inconnu, ou deux
//     commits sans ancêtre commun, que pour un dépôt inaccessible. Pour les distinguer, p.Repository
//     est interrogé (souvent servi en 304 depuis le cache, donc gratuit) : s'il réussit, l'erreur est
//     l'*APIError du compare dont la cause est provider.ErrUnknownBase (et PAS provider.ErrNotFound) ;
//     s'il échoue, son erreur est renvoyée (ErrNotFound, *SSORequiredError…). head venant de Branches
//     juste avant, un 404 sur un dépôt accessible signale en pratique une base disparue (force-push).
func (p *Provider) CompareCommits(ctx context.Context, owner, name, base, head string) ([]provider.Commit, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return nil, err
	}
	if err := provider.ValidateSHA(base); err != nil {
		return nil, err
	}
	if err := provider.ValidateSHA(head); err != nil {
		return nil, err
	}
	if base == head {
		return make([]provider.Commit, 0), nil
	}
	op := fmt.Sprintf("comparaison %s...%s dans %s/%s", provider.ShortSHA(base), provider.ShortSHA(head), owner, name)
	lo := &gogithub.ListOptions{PerPage: PerPage, Page: 1}

	commits := make([]provider.Commit, 0)
	pages, total, totalKnown, stop := 0, 0, false, stopLastPage
pagination:
	for {
		comp, resp, err := p.client.Repositories.CompareCommits(ctx, owner, name, base, head, lo)
		pages++
		if err != nil {
			return nil, p.compareError(ctx, op, owner, name, resp, err)
		}
		if comp == nil {
			return nil, p.unexpected(op, resp)
		}
		if pages == 1 {
			switch comp.GetStatus() {
			case compareBehind, compareIdentical:
				p.logCompare(ctx, owner, name, base, head, pages, 0, comp.GetStatus())
				return make([]provider.Commit, 0), nil
			}
			total, totalKnown = comp.GetTotalCommits(), comp.TotalCommits != nil
		}
		list := comp.Commits
		for _, rc := range list {
			c, ok := toCommit(rc)
			if !ok {
				return nil, p.unexpected(op, resp)
			}
			commits = append(commits, c)
			if len(commits) >= provider.MaxCommits {
				stop = stopCap
				break pagination
			}
		}
		if totalKnown && len(commits) >= total {
			break
		}
		next, reason := nextPage(resp, lo.Page, pages, len(list), maxCommitPages)
		if next == 0 && reason == stopLastPage && totalKnown && len(list) >= PerPage && pages < maxCommitPages {
			// Pas d'en-tête Link alors que total_commits n'est pas atteint et que la page était
			// pleine : on demande la page suivante (une page au-delà de la dernière est vide).
			next = lo.Page + 1
		}
		if next == 0 {
			stop = reason
			break
		}
		lo.Page = next
	}
	slices.Reverse(commits)
	p.logCompare(ctx, owner, name, base, head, pages, len(commits), stop)
	return commits, nil
}

// compareError convertit l'erreur d'une page de comparaison. Un 404 est départagé par p.Repository :
// dépôt accessible → cause provider.ErrUnknownBase ; sinon l'erreur de p.Repository.
func (p *Provider) compareError(ctx context.Context, op, owner, name string, resp *gogithub.Response, err error) error {
	err = p.convertError(ctx, op, resp, err)
	if !errors.Is(err, provider.ErrNotFound) {
		return err
	}
	if _, rerr := p.Repository(ctx, owner, name); rerr != nil {
		return rerr
	}
	unknown := &provider.APIError{Op: op, Method: http.MethodGet, Status: http.StatusNotFound}
	var apiErr *provider.APIError
	if errors.As(err, &apiErr) {
		*unknown = *apiErr
	}
	unknown.Err = provider.ErrUnknownBase
	return unknown
}

// logCompare journalise, au niveau Debug, la fin d'une comparaison : dépôt, SHA courts, nombre de
// pages et de commits, raison de l'arrêt (ou statut behind / identical).
func (p *Provider) logCompare(ctx context.Context, owner, name, base, head string, pages, n int, stop string) {
	p.logger.DebugContext(ctx, "commits comparés",
		slog.String("depot", owner+"/"+name),
		slog.String("base", provider.ShortSHA(base)),
		slog.String("tete", provider.ShortSHA(head)),
		slog.Int("pages", pages),
		slog.Int("elements", n),
		slog.String("arret", stop),
	)
}

// CommitStats implémente provider.Provider : GET /repos/{owner}/{name}/commits/{sha}?per_page=1
// (RepositoriesService.GetCommit), après provider.ValidateRepo et provider.ValidateSHA. Additions =
// stats.additions, Deletions = stats.deletions ; stats absent → p.unexpected. 409 → erreur enveloppant
// provider.ErrEmptyRepository (renvoyée, pas masquée).
//
// per_page=1 : seule la liste files de la réponse est paginée ; stats couvre tout le commit quelle que
// soit la page (documentation de GitHub, vérifié sur l'API réelle). Sans ce paramètre, GitHub renvoie
// jusqu'à 300 fichiers avec leur patch, c'est-à-dire le code du dépôt privé de l'étudiant, téléchargé et
// décodé pour n'en garder que deux entiers. Le transport ne met pas non plus cette réponse en cache
// (cacheablePath) : un commit n'est demandé qu'une fois.
//
// Un SHA inconnu du dépôt fait répondre 422 « No commit found for SHA: … » à GitHub (vérifié sur
// l'API réelle), que convertError classe en provider.ErrUnexpected. Dans ce cas précis seulement
// (code 422 ET message de GitHub commençant par « No commit found », sans tenir compte de la casse,
// lu sur l'erreur de go-github avant conversion), la cause devient une erreur enveloppant
// provider.ErrNotFound, pour que le moteur de synchro puisse traiter un commit disparu (force-push)
// comme un élément introuvable. Tout autre 422 reste provider.ErrUnexpected : GitHub répond aussi 422
// pour un diff trop long à calculer (« this diff is taking too long to generate », par exemple un
// node_modules commité) ou une validation refusée, et le commit existe bien.
func (p *Provider) CommitStats(ctx context.Context, owner, name, sha string) (provider.CommitStats, error) {
	if err := provider.ValidateRepo(owner, name); err != nil {
		return provider.CommitStats{}, err
	}
	if err := provider.ValidateSHA(sha); err != nil {
		return provider.CommitStats{}, err
	}
	op := fmt.Sprintf("statistiques du commit %s de %s/%s", provider.ShortSHA(sha), owner, name)
	rc, resp, err := p.client.Repositories.GetCommit(ctx, owner, name, sha, &gogithub.ListOptions{PerPage: 1})
	if err != nil {
		unknownSHA := isUnknownCommit(err)
		err = p.convertError(ctx, op, resp, err)
		var apiErr *provider.APIError
		if unknownSHA && errors.As(err, &apiErr) && apiErr.Status == http.StatusUnprocessableEntity &&
			errors.Is(apiErr.Err, provider.ErrUnexpected) {
			notFound := *apiErr
			notFound.Err = fmt.Errorf("commit %s inconnu de GitHub : %w", provider.ShortSHA(sha), provider.ErrNotFound)
			return provider.CommitStats{}, &notFound
		}
		return provider.CommitStats{}, err
	}
	stats := rc.GetStats()
	if stats == nil || stats.Additions == nil || stats.Deletions == nil {
		return provider.CommitStats{}, p.unexpected(op, resp)
	}
	return provider.CommitStats{Additions: stats.GetAdditions(), Deletions: stats.GetDeletions()}, nil
}

// unknownCommitMessage est le début (comparé sans tenir compte de la casse) du message d'erreur de
// GitHub pour un SHA inconnu du dépôt : « No commit found for SHA: … ».
const unknownCommitMessage = "no commit found"

// isUnknownCommit indique une erreur de go-github dont le message de GitHub signale un SHA inconnu du
// dépôt. Le message sert seulement à classer : il n'apparaît jamais dans une erreur de Watchoo.
func isUnknownCommit(err error) bool {
	msg := strings.ToLower(strings.TrimSpace(githubMessage(err)))
	return strings.HasPrefix(msg, unknownCommitMessage)
}

// toCommit convertit un commit de go-github (liste ou comparaison). false si le commit est absent,
// si son SHA n'est pas un SHA complet (provider.ValidateSHA) ou si l'un de ses parents n'en a pas.
func toCommit(rc *gogithub.RepositoryCommit) (provider.Commit, bool) {
	if rc == nil || provider.ValidateSHA(rc.GetSHA()) != nil {
		return provider.Commit{}, false
	}
	parents := make([]string, 0, len(rc.Parents))
	for _, parent := range rc.Parents {
		if provider.ValidateSHA(parent.GetSHA()) != nil {
			return provider.Commit{}, false
		}
		parents = append(parents, parent.GetSHA())
	}
	git := rc.GetCommit()
	return provider.Commit{
		SHA:         rc.GetSHA(),
		Message:     git.GetMessage(),
		URL:         rc.GetHTMLURL(),
		Author:      toPerson(git.GetAuthor(), rc.GetAuthor()),
		Committer:   toPerson(git.GetCommitter(), rc.GetCommitter()),
		AuthoredAt:  utc(git.GetAuthor().GetDate()),
		CommittedAt: utc(git.GetCommitter().GetDate()),
		Parents:     parents,
	}, true
}

// toPerson assemble l'identité Git (nom, e-mail, tirés de commit.author ou commit.committer) et le
// compte GitHub lié (login, id, avatar, tirés de author ou committer), nil quand aucun compte n'est lié
// à l'e-mail du commit.
func toPerson(git *gogithub.CommitAuthor, account *gogithub.User) provider.Person {
	return provider.Person{
		Name:      git.GetName(),
		Email:     git.GetEmail(),
		Login:     account.GetLogin(),
		ID:        account.GetID(),
		AvatarURL: account.GetAvatarURL(),
	}
}
