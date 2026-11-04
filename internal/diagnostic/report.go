package diagnostic

// Pistes de réflexion de niveau « Déclaré » (phase 7.2) : commits et pull requests qui portent au
// moins un signal déclaratif, avec les signaux et leurs valeurs, et le contexte D4 des dépôts. Le
// résultat est réservé au formateur qui supervise l'espace ; une piste n'est jamais un verdict.

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
)

// Niveaux et natures des pistes. Valeurs stables (sorties --json).
const (
	LevelDeclared   = "declared"     // au moins un signal D1, D2, D3 ou D5 (spécification, section 5.1)
	KindCommit      = "commit"       // piste portée par un commit
	KindPullRequest = "pull_request" // piste portée par une pull request
)

// Hit est un signal déclenché, avec sa valeur brute : outil déclaré (D1, D2), compte bot (D3), nom de
// branche (D5).
type Hit struct {
	Signal string // SignalCoAuthor, SignalSignature, SignalBot ou SignalAgentBranch
	Value  string
}

// Lead est une piste de réflexion : un commit ou une pull request et les signaux qui l'ont levée.
type Lead struct {
	Kind   string // KindCommit ou KindPullRequest
	Level  string // LevelDeclared
	RepoID int64
	store.RepoRef
	SHA         string    // commit (KindCommit) ; vide pour une PR
	Number      int       // numéro de PR (KindPullRequest) ; 0 pour un commit
	Title       string    // titre de la PR ; vide pour un commit (son message n'est pas repris)
	URL         string    // page du commit ou de la PR sur la forge
	AuthorName  string    // nom Git de l'auteur (commit) ; vide si inconnu
	AuthorLogin string    // login de forge de l'auteur ; vide si inconnu
	Date        time.Time // date d'écriture du commit, ou d'ouverture de la PR, UTC
	Hits        []Hit     // signaux déclenchés, triés par signal
}

// Report regroupe les pistes (de la plus récente à la plus ancienne) et le contexte D4 des dépôts.
type Report struct {
	Leads    []Lead
	Contexts []store.AgentConfigContext
}

// Level renvoie le niveau d'un ensemble de signaux : LevelDeclared s'il contient un signal
// déclaratif (D1, D2, D3, D5), vide sinon. D4 n'est que du contexte.
func Level(hits []Hit) string {
	for _, h := range hits {
		switch h.Signal {
		case SignalCoAuthor, SignalSignature, SignalBot, SignalAgentBranch:
			return LevelDeclared
		}
	}
	return ""
}

// CommitHits renvoie les signaux déclaratifs d'un commit mesuré : outils déclarés (D1, D2) et auteur
// bot (D3, valeur : login, sinon nom de l'auteur).
func CommitHits(v store.CommitSignalView) []Hit {
	hits := []Hit{}
	for _, d := range DecodeDeclarations(v.Tools) {
		hits = append(hits, Hit{Signal: d.Signal, Value: d.Tool})
	}
	if v.BotAuthor {
		hits = append(hits, Hit{Signal: SignalBot, Value: cmp.Or(v.AuthorLogin, v.AuthorName, "bot")})
	}
	sortHits(hits)
	return hits
}

// PullRequestHits renvoie les signaux déclaratifs d'une pull request : outils déclarés dans sa
// description (D2, mesurés à la synchro) et branche nommée par un agent (D5, valeur : la branche).
func PullRequestHits(p store.PullRequestView) []Hit {
	hits := []Hit{}
	for _, d := range DecodeDeclarations(p.ToolIDs) {
		hits = append(hits, Hit{Signal: d.Signal, Value: d.Tool})
	}
	if _, ok := AgentBranch(p.HeadBranch); ok {
		hits = append(hits, Hit{Signal: SignalAgentBranch, Value: p.HeadBranch})
	}
	sortHits(hits)
	return hits
}

// Declared renvoie les pistes de niveau « Déclaré » qui satisfont f, et le contexte D4 des dépôts
// (f.RepoID, sinon tous). Filtres des pull requests : dépôt, date d'ouverture (f.Since) et login de
// l'auteur (f.Author, sans tenir compte de la casse : une PR ne connaît que le login).
func Declared(ctx context.Context, st *store.Store, f store.DiagnosticFilter) (Report, error) {
	commits, err := st.ListDeclaredCommitSignals(ctx, f)
	if err != nil {
		return Report{}, err
	}
	leads := make([]Lead, 0, len(commits))
	for _, c := range commits {
		hits := CommitHits(c)
		if Level(hits) == "" {
			continue
		}
		leads = append(leads, Lead{
			Kind: KindCommit, Level: LevelDeclared, RepoID: c.RepoID, RepoRef: c.RepoRef,
			SHA: c.SHA, URL: c.URL, AuthorName: c.AuthorName, AuthorLogin: c.AuthorLogin,
			Date: c.AuthoredAt, Hits: hits,
		})
	}

	pulls, err := st.ListPullRequests(ctx, store.PullRequestFilter{RepoID: f.RepoID})
	if err != nil {
		return Report{}, err
	}
	author := strings.TrimSpace(f.Author)
	for _, p := range pulls {
		if !f.Since.IsZero() && p.CreatedAt.Before(f.Since) {
			continue
		}
		if author != "" && !strings.EqualFold(p.AuthorLogin, author) {
			continue
		}
		hits := PullRequestHits(p)
		if Level(hits) == "" {
			continue
		}
		leads = append(leads, Lead{
			Kind: KindPullRequest, Level: LevelDeclared, RepoID: p.RepoID, RepoRef: p.RepoRef,
			Number: p.Number, Title: p.Title, URL: p.URL, AuthorLogin: p.AuthorLogin,
			Date: p.CreatedAt, Hits: hits,
		})
	}
	slices.SortStableFunc(leads, func(a, b Lead) int {
		return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(a.RepoID, b.RepoID), cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.SHA, b.SHA), cmp.Compare(a.Number, b.Number))
	})

	contexts, err := st.AgentConfigContexts(ctx, f.RepoID)
	if err != nil {
		return Report{}, err
	}
	return Report{Leads: leads, Contexts: contexts}, nil
}

// sortHits trie par signal puis par valeur.
func sortHits(hits []Hit) {
	slices.SortFunc(hits, func(a, b Hit) int {
		return cmp.Or(cmp.Compare(a.Signal, b.Signal), cmp.Compare(a.Value, b.Value))
	})
}
