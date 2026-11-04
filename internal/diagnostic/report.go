package diagnostic

// Pistes de réflexion (phases 7.2 et 7.3) : commits, jours et pull requests qui portent au moins un
// signal déclaratif (niveau « Déclaré ») ou de rythme (« À regarder »), avec les signaux et leurs
// valeurs, le contexte D4 des dépôts et l'historique des auteurs. Le
// résultat est réservé au formateur qui supervise l'espace ; une piste n'est jamais un verdict.

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/store"
)

// Niveaux et natures des pistes. Valeurs stables (sorties --json).
const (
	LevelDeclared   = "declared"     // au moins un signal D1, D2, D3 ou D5 (spécification, section 5.1)
	LevelToReview   = "to_review"    // au moins un signal de rythme R, sans signal déclaratif
	KindCommit      = "commit"       // piste portée par un commit
	KindPullRequest = "pull_request" // piste portée par une pull request
	KindDay         = "day"          // piste portée par un jour de travail d'un auteur (R1, R6)
)

// Hit est un signal déclenché, avec sa valeur brute : outil déclaré (D1, D2), compte bot (D3), nom de
// branche (D5), ou mesure d'un signal de rythme (R1 à R6) avec la médiane personnelle et le score z.
type Hit struct {
	Signal    string  // identifiant du signal (D1… R6)
	Value     string  // valeur affichée : outil, compte, branche, ou mesure écrite (« 412 », « 0.97 »)
	Raw       float64 // mesure d'un signal de rythme ; 0 pour un signal déclaratif
	Median    float64 // médiane personnelle (R6 : médiane des 90 jours de référence) ; voir HasMedian
	Z         float64 // score z modifié ; voir HasZ
	HasMedian bool    // Median est renseignée
	HasZ      bool    // Z est renseigné
}

// Lead est une piste de réflexion : un commit ou une pull request et les signaux qui l'ont levée.
type Lead struct {
	Kind   string // KindCommit, KindPullRequest ou KindDay
	Level  string // LevelDeclared ou LevelToReview
	RepoID int64
	store.RepoRef
	AuthorID    int64     // contributeur auteur (commit, jour) ; 0 pour une PR ou si inconnu
	SHA         string    // commit (KindCommit) ; vide sinon
	Number      int       // numéro de PR (KindPullRequest) ; 0 pour un commit
	Title       string    // titre de la PR ; vide pour un commit (son message n'est pas repris)
	URL         string    // page du commit ou de la PR sur la forge
	AuthorName  string    // nom Git de l'auteur (commit) ; vide si inconnu
	AuthorLogin string    // login de forge de l'auteur ; vide si inconnu
	Date        time.Time // écriture du commit, ouverture de la PR, ou début du jour (KindDay)
	Hits        []Hit     // signaux déclenchés, triés par signal
}

// Report regroupe les pistes (de la plus récente à la plus ancienne), le contexte D4 des dépôts et
// l'historique des auteurs (signaux de rythme).
type Report struct {
	Leads     []Lead
	Contexts  []store.AgentConfigContext
	Histories []*AuthorHistory
}

// Level renvoie le niveau d'un ensemble de signaux (spécification, section 5.1, sans les signaux de forme
// de la phase 7.5) : LevelDeclared s'il contient un signal déclaratif (D1, D2, D3, D5), sinon
// LevelToReview s'il contient un signal de rythme (R1 à R6), vide sinon. D4 n'est que du contexte.
func Level(hits []Hit) string {
	level := ""
	for _, h := range hits {
		switch h.Signal {
		case SignalCoAuthor, SignalSignature, SignalBot, SignalAgentBranch:
			return LevelDeclared
		case SignalDailyLines, SignalCommitSize, SignalAddRatio, SignalSpeed, SignalFiles, SignalRegimeBreak:
			level = LevelToReview
		}
	}
	return level
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

// Build renvoie les pistes de réflexion qui satisfont f (niveaux « Déclaré » et « À regarder »), le
// contexte D4 des dépôts et l'historique des auteurs. Les signaux de rythme sont calculés sur tout
// l'historique mesuré (les 90 jours qui précèdent chaque événement), puis filtrés. Filtres : dépôt, date
// de la piste (f.Since) et auteur (f.Author : login, nom ou e-mail d'un contributeur ; login seulement
// pour une pull request, qui ne connaît que lui).
func Build(ctx context.Context, st *store.Store, f store.DiagnosticFilter, opts RhythmOptions) (Report, error) {
	views, err := st.CommitSignalViews(ctx, f.RepoID)
	if err != nil {
		return Report{}, err
	}
	author := strings.TrimSpace(f.Author)
	var authorIDs []int64
	if author != "" {
		if authorIDs, err = st.MatchingContributorIDs(ctx, author); err != nil {
			return Report{}, err
		}
	}
	keep := func(authorID int64, date time.Time) bool {
		if !f.Since.IsZero() && date.Before(f.Since) {
			return false
		}
		return author == "" || slices.Contains(authorIDs, authorID)
	}

	commits, days, histories := rhythm(views, opts)
	extra := map[string][]Hit{} // repo/SHA → signaux de rythme
	for _, c := range commits {
		extra[leadKey(c.view.RepoID, c.view.SHA)] = c.hits
	}
	leads := []Lead{}
	for _, v := range views {
		hits := append(CommitHits(v), extra[leadKey(v.RepoID, v.SHA)]...)
		sortHits(hits)
		level := Level(hits)
		if level == "" || !keep(v.AuthorID, v.AuthoredAt) {
			continue
		}
		leads = append(leads, Lead{
			Kind: KindCommit, Level: level, RepoID: v.RepoID, RepoRef: v.RepoRef, AuthorID: v.AuthorID,
			SHA: v.SHA, URL: v.URL, AuthorName: v.AuthorName, AuthorLogin: v.AuthorLogin,
			Date: v.AuthoredAt, Hits: hits,
		})
	}
	for _, d := range days {
		if !keep(d.group.AuthorID, d.day) {
			continue
		}
		sortHits(d.hits)
		leads = append(leads, Lead{
			Kind: KindDay, Level: Level(d.hits), RepoID: d.group.RepoID, RepoRef: d.group.RepoRef,
			AuthorID: d.group.AuthorID, AuthorName: d.group.AuthorName, AuthorLogin: d.group.AuthorLogin,
			Date: d.day.UTC(), Hits: d.hits,
		})
	}

	pulls, err := st.ListPullRequests(ctx, store.PullRequestFilter{RepoID: f.RepoID})
	if err != nil {
		return Report{}, err
	}
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
			Kind: KindPullRequest, Level: Level(hits), RepoID: p.RepoID, RepoRef: p.RepoRef,
			Number: p.Number, Title: p.Title, URL: p.URL, AuthorLogin: p.AuthorLogin,
			Date: p.CreatedAt, Hits: hits,
		})
	}
	slices.SortStableFunc(leads, func(a, b Lead) int {
		return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(a.RepoID, b.RepoID), cmp.Compare(a.Kind, b.Kind),
			cmp.Compare(a.SHA, b.SHA), cmp.Compare(a.Number, b.Number), cmp.Compare(a.AuthorID, b.AuthorID))
	})

	contexts, err := st.AgentConfigContexts(ctx, f.RepoID)
	if err != nil {
		return Report{}, err
	}
	kept := []*AuthorHistory{}
	for _, h := range histories {
		if author == "" || slices.Contains(authorIDs, h.AuthorID) {
			kept = append(kept, h)
		}
	}
	return Report{Leads: leads, Contexts: contexts, Histories: kept}, nil
}

// leadKey identifie un commit d'un dépôt.
func leadKey(repoID int64, sha string) string { return strconv.FormatInt(repoID, 10) + "/" + sha }

// sortHits trie par signal puis par valeur.
func sortHits(hits []Hit) {
	slices.SortFunc(hits, func(a, b Hit) int {
		return cmp.Or(cmp.Compare(a.Signal, b.Signal), cmp.Compare(a.Value, b.Value))
	})
}
