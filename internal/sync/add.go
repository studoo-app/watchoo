package sync

// Ajout de dépôts suivis avec vérification d'accès (« watchoo repo add » et « watchoo repo import »).

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/provider"
	"github.com/studoo-app/watchoo/internal/repoimport"
	"github.com/studoo-app/watchoo/internal/store"
)

// AddRequest est un dépôt à ajouter (ou à mettre à jour s'il est déjà suivi).
type AddRequest struct {
	Line  int            // numéro de ligne du fichier d'import ; 0 pour « repo add »
	Ref   repoimport.Ref // dépôt demandé (owner/name tels que saisis)
	Label string         // nom affiché ; vide = non fourni
	// BranchPatterns sont les motifs à suivre ; nil = non fourni (création : toutes les branches).
	BranchPatterns []string
	// FlemmeAfterDays et DeadAfterDays sont les seuils d'activité propres au dépôt ; nil = non fourni
	// (création : valeurs globales ; dépôt suivi : seuils gardés), 0 = revenir à la valeur globale.
	FlemmeAfterDays *int
	DeadAfterDays   *int
}

// AddOutcome est le résultat de l'ajout d'un dépôt. Valeurs stables (sorties --json).
type AddOutcome string

// Valeurs de AddOutcome.
const (
	OutcomeAdded          AddOutcome = "added"        // nouveau dépôt suivi
	OutcomeAlreadyTracked AddOutcome = "already"      // déjà suivi, rien à changer
	OutcomeUpdated        AddOutcome = "updated"      // déjà suivi : label, motifs ou seuils changés, ou suivi réactivé
	OutcomeInaccessible   AddOutcome = "inaccessible" // ErrNotFound, ErrForbidden (SSO compris) : dépôt introuvable ou sans accès
	OutcomeError          AddOutcome = "error"        // autre échec (réseau, quota, base…)
)

// AddResult est le résultat pour une AddRequest.
type AddResult struct {
	Request AddRequest
	Outcome AddOutcome
	// Repo est le dépôt tel qu'enregistré (ou tel qu'il le serait avec DryRun) : owner/name ACTUELS de
	// la forge (renommage suivi). Valeur zéro si Inaccessible ou Error.
	Repo model.Repository
	// Error est le message en français (Inaccessible, Error) ; vide sinon. Il ne cite jamais le token.
	Error string
}

// AddOptions règle AddRepositories.
type AddOptions struct {
	DryRun bool // vérifie l'accès et calcule les résultats sans rien écrire
	// Progress, si non nil, est appelée après chaque dépôt (done sur total), depuis la goroutine de
	// l'appelant.
	Progress func(done, total int)
}

// AddRepositories vérifie l'accès à chaque dépôt (une requête Provider.Repository par dépôt, l'une
// après l'autre) puis l'enregistre, et renvoie un résultat par requête, dans l'ordre :
//   - source choisie et Provider construit une fois (voir Source : API GitHub avec le token de gh, ou
//     clones Git locaux sans gh, dont Repository vérifie l'accès par « git ls-remote » sans rien
//     cloner) ; un échec ici est renvoyé comme erreur ;
//   - accès refusé (ErrNotFound, ErrForbidden ; ErrAccessDenied en clone Git local) :
//     OutcomeInaccessible, sans bloquer les autres ;
//     *RateLimitError : ce dépôt et les suivants en OutcomeError (message du quota) ;
//   - dépôt déjà en base, cherché par provider_id puis par owner/name actuels : label, motifs et seuils
//     d'activité mis à jour s'ils sont fournis et différents, suivi réactivé s'il était arrêté
//     (OutcomeUpdated), sinon OutcomeAlreadyTracked ;
//   - sinon création (store.CreateRepository) avec les métadonnées de la forge, Enabled, label, motifs
//     et seuils fournis : OutcomeAdded ;
//   - DryRun : aucune écriture, les mêmes résultats sont calculés.
//
// Les labels et les seuils sont d'abord vérifiés (repoimport.ValidateLabel, ValidateThresholds) : une
// valeur invalide est une erreur, avant tout appel à la forge. Aucune synchro n'est lancée. L'erreur
// renvoyée est réservée aux échecs globaux (label ou seuil invalide, token, ctx annulé).
func (e *Engine) AddRepositories(ctx context.Context, reqs []AddRequest, opts AddOptions) ([]AddResult, error) {
	for _, req := range reqs {
		if err := repoimport.ValidateLabel(req.Label); err != nil {
			return nil, fmt.Errorf("ajout du dépôt %s : %w", req.Ref, err)
		}
		if err := repoimport.ValidateThresholds(req.FlemmeAfterDays, req.DeadAfterDays); err != nil {
			return nil, fmt.Errorf("ajout du dépôt %s : %w", req.Ref, err)
		}
	}
	prov, redact, _, err := e.openProvider(ctx)
	if err != nil {
		return nil, fmt.Errorf("ajout de dépôts : %w", err)
	}
	results := make([]AddResult, 0, len(reqs))
	// planned retient, en DryRun, les dépôts qu'un ajout précédent du même lot aurait créés ou
	// modifiés (clé : plannedKey), pour que les résultats soient ceux d'un vrai ajout.
	planned := make(map[string]model.Repository)
	var quotaErr string // message du quota épuisé : ce dépôt et les suivants échouent
	for i, req := range reqs {
		if err := ctx.Err(); err != nil {
			return results, fmt.Errorf("ajout de dépôts : %w", err)
		}
		var res AddResult
		if quotaErr != "" {
			res = AddResult{Request: req, Outcome: OutcomeError, Error: quotaErr}
		} else {
			res, err = e.addOne(ctx, prov, req, opts.DryRun, planned)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return results, fmt.Errorf("ajout de dépôts : %w", ctxErr)
				}
				var rle *provider.RateLimitError
				if errors.As(err, &rle) {
					quotaErr = redact(err.Error())
				}
				res = AddResult{Request: req, Outcome: OutcomeError, Error: redact(err.Error())}
				if isInaccessible(err) {
					res.Outcome, res.Error = OutcomeInaccessible, redact(inaccessibleMessage(err))
				}
			}
		}
		e.logger.Info("ajout de dépôt", "repo", req.Ref.String(), "resultat", string(res.Outcome))
		results = append(results, res)
		if opts.Progress != nil {
			opts.Progress(i+1, len(reqs))
		}
	}
	return results, nil
}

// addOne vérifie l'accès au dépôt de req et l'enregistre (ou calcule ce qui le serait, avec dryRun).
// Une erreur de la forge ou du store est renvoyée telle quelle ; AddRepositories la classe.
func (e *Engine) addOne(ctx context.Context, prov provider.Provider, req AddRequest, dryRun bool,
	planned map[string]model.Repository) (AddResult, error) {
	meta, err := prov.Repository(ctx, req.Ref.Owner, req.Ref.Name)
	if err != nil {
		return AddResult{}, err
	}
	key := plannedKey(meta)
	existing, found := planned[key]
	if !found {
		existing, found, err = e.findExisting(ctx, prov.Name(), meta)
		if err != nil {
			return AddResult{}, err
		}
	}

	if !found {
		repo := model.Repository{
			Provider:        prov.Name(),
			ProviderID:      meta.ID,
			Owner:           meta.Owner,
			Name:            meta.Name,
			Label:           req.Label,
			DefaultBranch:   meta.DefaultBranch,
			URL:             meta.URL,
			Stars:           meta.Stars,
			OpenIssues:      meta.OpenIssues,
			Archived:        meta.Archived,
			Enabled:         true,
			BranchPatterns:  slices.Clone(req.BranchPatterns),
			FlemmeAfterDays: thresholdValue(req.FlemmeAfterDays, nil),
			DeadAfterDays:   thresholdValue(req.DeadAfterDays, nil),
		}
		if dryRun {
			planned[key] = repo
		} else if repo, err = e.store.CreateRepository(ctx, repo); err != nil {
			return AddResult{}, err
		}
		return AddResult{Request: req, Outcome: OutcomeAdded, Repo: repo}, nil
	}

	repo := existing
	settingsChanged := false
	if req.Label != "" && req.Label != repo.Label {
		repo.Label, settingsChanged = req.Label, true
	}
	if req.BranchPatterns != nil && !slices.Equal(req.BranchPatterns, repo.BranchPatterns) {
		repo.BranchPatterns, settingsChanged = slices.Clone(req.BranchPatterns), true
	}
	flemme := thresholdValue(req.FlemmeAfterDays, repo.FlemmeAfterDays)
	dead := thresholdValue(req.DeadAfterDays, repo.DeadAfterDays)
	thresholdsChanged := !equalThreshold(flemme, repo.FlemmeAfterDays) || !equalThreshold(dead, repo.DeadAfterDays)
	repo.FlemmeAfterDays, repo.DeadAfterDays = flemme, dead
	reenable := !repo.Enabled
	repo.Enabled = true
	if !settingsChanged && !thresholdsChanged && !reenable {
		return AddResult{Request: req, Outcome: OutcomeAlreadyTracked, Repo: repo}, nil
	}
	if dryRun {
		planned[key] = repo
		return AddResult{Request: req, Outcome: OutcomeUpdated, Repo: repo}, nil
	}
	if settingsChanged {
		if repo, err = e.store.UpdateRepositorySettings(ctx, repo.ID, repo.Label, repo.BranchPatterns); err != nil {
			return AddResult{}, err
		}
	}
	if thresholdsChanged {
		if repo, err = e.store.SetRepositoryThresholds(ctx, repo.ID, flemme, dead); err != nil {
			return AddResult{}, err
		}
	}
	if reenable {
		if err := e.store.SetRepositoryEnabled(ctx, repo.ID, true); err != nil {
			return AddResult{}, err
		}
		repo.Enabled = true
	}
	return AddResult{Request: req, Outcome: OutcomeUpdated, Repo: repo}, nil
}

// plannedKey identifie un dépôt dans un lot DryRun : son identifiant de forge, sinon (clone Git local,
// identifiant inconnu) owner/name en minuscules.
func plannedKey(meta provider.Repository) string {
	if meta.ID > 0 {
		return fmt.Sprintf("id:%d", meta.ID)
	}
	return "nom:" + strings.ToLower(meta.FullName())
}

// thresholdValue renvoie la valeur d'un seuil après une demande : current si req est nil (non fourni),
// nil (valeur globale) si req vaut 0, sinon une copie de req.
func thresholdValue(req, current *int) *int {
	switch {
	case req == nil:
		return current
	case *req == 0:
		return nil
	default:
		v := *req
		return &v
	}
}

// equalThreshold compare deux seuils (nil = valeur globale).
func equalThreshold(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// findExisting cherche le dépôt meta en base : par identifiant de forge (s'il est connu), puis par
// owner/name actuels.
func (e *Engine) findExisting(ctx context.Context, providerName string, meta provider.Repository) (model.Repository, bool, error) {
	repo, err := e.store.GetRepositoryByProviderID(ctx, providerName, meta.ID)
	if err == nil {
		return repo, true, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return model.Repository{}, false, err
	}
	repo, err = e.store.GetRepositoryByName(ctx, providerName, meta.Owner, meta.Name)
	if err == nil {
		return repo, true, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return model.Repository{}, false, err
	}
	return model.Repository{}, false, nil
}

// isInaccessible indique un dépôt introuvable ou sans accès (404, 403, SSO compris ; accès refusé à
// git pour un clone local).
func isInaccessible(err error) bool {
	return errors.Is(err, provider.ErrNotFound) || errors.Is(err, provider.ErrForbidden) ||
		errors.Is(err, provider.ErrAccessDenied)
}

// inaccessibleMessage renvoie le message d'un dépôt inaccessible : celui du SSO s'il est en cause
// (il dit quoi faire), sinon le message de l'erreur sentinelle, sans le détail de la requête.
func inaccessibleMessage(err error) string {
	var sso *provider.SSORequiredError
	switch {
	case errors.As(err, &sso):
		return sso.Error()
	case errors.Is(err, provider.ErrNotFound):
		return provider.ErrNotFound.Error()
	case errors.Is(err, provider.ErrAccessDenied):
		return provider.ErrAccessDenied.Error()
	default:
		return provider.ErrForbidden.Error()
	}
}
