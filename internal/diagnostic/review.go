package diagnostic

// Revue humaine d'une piste (phase 7.4, spécification section 6.2) : clé stable d'une piste, règles
// d'une revue, débit quotidien de l'auteur et commits voisins pour la fiche de revue.

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// Décisions, causes et niveaux de confiance d'une revue. Valeurs stables (base, formulaires).
const (
	DecisionAssisted     = "assisted"
	DecisionNotAssisted  = "not_assisted"
	DecisionUndetermined = "undetermined"

	CauseInternalCopy   = "internal_copy"
	CauseToolGenerated  = "tool_generated"
	CauseExistingImport = "existing_import"
	CauseRefactoring    = "refactoring"
	CauseOther          = "other"

	ConfidenceLow    = "low"
	ConfidenceMedium = "medium"
	ConfidenceHigh   = "high"

	// MaxNote est la longueur maximale de la note libre, en caractères.
	MaxNote = 2000
)

// Valeurs acceptées, dans l'ordre d'affichage.
var (
	Decisions   = []string{DecisionAssisted, DecisionNotAssisted, DecisionUndetermined}
	Causes      = []string{CauseInternalCopy, CauseToolGenerated, CauseExistingImport, CauseRefactoring, CauseOther}
	Confidences = []string{ConfidenceLow, ConfidenceMedium, ConfidenceHigh}
)

// ReviewInput est la saisie d'une revue.
type ReviewInput struct {
	Decision   string
	Cause      string // seulement pour DecisionNotAssisted ; ignorée sinon
	Confidence string
	Note       string
}

// Key renvoie la clé stable d'une piste dans son dépôt : « commit/<sha> », « pull_request/<numéro> »
// ou « day/<auteur>/<début du jour UTC> ».
func (l Lead) Key() string {
	switch l.Kind {
	case KindCommit:
		return KindCommit + "/" + l.SHA
	case KindPullRequest:
		return KindPullRequest + "/" + strconv.Itoa(l.Number)
	default:
		return KindDay + "/" + strconv.FormatInt(l.AuthorID, 10) + "/" + l.Date.UTC().Format("20060102T150405Z")
	}
}

// FindLead renvoie la piste de clé key du dépôt repoID dans rep.
func FindLead(rep Report, repoID int64, key string) (Lead, bool) {
	i := slices.IndexFunc(rep.Leads, func(l Lead) bool { return l.RepoID == repoID && l.Key() == key })
	if i < 0 {
		return Lead{}, false
	}
	return rep.Leads[i], true
}

// NewReview vérifie la saisie in et renvoie la revue de la piste l, faite par reviewer à now. Erreur
// (dans la langue de tr) pour une décision, une cause ou une confiance inconnue, une cause absente pour
// « non assisté », ou une note trop longue. La note est gardée telle quelle, espaces autour retirés.
func NewReview(tr *i18n.Translator, l Lead, in ReviewInput, reviewer string, now time.Time) (model.DiagnosticReview, error) {
	if !slices.Contains(Decisions, in.Decision) {
		return model.DiagnosticReview{}, errors.New(tr.T("diagnostic.review.decision"))
	}
	cause := ""
	if in.Decision == DecisionNotAssisted {
		if !slices.Contains(Causes, in.Cause) {
			return model.DiagnosticReview{}, errors.New(tr.T("diagnostic.review.cause"))
		}
		cause = in.Cause
	}
	if !slices.Contains(Confidences, in.Confidence) {
		return model.DiagnosticReview{}, errors.New(tr.T("diagnostic.review.confidence"))
	}
	note := strings.TrimSpace(in.Note)
	if !utf8.ValidString(note) || utf8.RuneCountInString(note) > MaxNote {
		return model.DiagnosticReview{}, errors.New(tr.T("diagnostic.review.note", MaxNote))
	}
	signals := []string{}
	for _, h := range l.Hits {
		if !slices.Contains(signals, h.Signal) {
			signals = append(signals, h.Signal)
		}
	}
	slices.Sort(signals)
	return model.DiagnosticReview{
		RepoID: l.RepoID, LeadKey: l.Key(), Kind: l.Kind, LeadDate: l.Date.UTC(), Signals: signals,
		Decision: in.Decision, Cause: cause, Confidence: in.Confidence, Note: note,
		Reviewer: reviewer, ReviewedAt: now.UTC(),
	}, nil
}

// DayLines est le débit d'un jour : lignes ajoutées (fichiers retenus) par l'auteur.
type DayLines struct {
	Date      string `json:"date"`      // AAAA-MM-JJ, dans le fuseau d'affichage
	Count     int    `json:"count"`     // lignes ajoutées
	Highlight bool   `json:"highlight"` // jour de la piste
}

// Throughput renvoie le débit quotidien de l'auteur authorID dans le dépôt repoID sur days jours qui
// se terminent par le jour de end (fuseau loc), le jour de end mis en évidence.
func Throughput(views []store.CommitSignalView, repoID, authorID int64, end time.Time, days int, loc *time.Location) []DayLines {
	if loc == nil {
		loc = time.UTC
	}
	days = max(days, 1)
	last := dayOf(end, loc)
	first := last.AddDate(0, 0, -(days - 1))
	out := make([]DayLines, days)
	index := map[string]int{}
	for i := range days {
		d := first.AddDate(0, 0, i).Format("2006-01-02")
		out[i] = DayLines{Date: d}
		index[d] = i
	}
	out[days-1].Highlight = true
	for _, v := range views {
		if v.RepoID != repoID || v.AuthorID != authorID {
			continue
		}
		if i, ok := index[dayOf(v.AuthoredAt, loc).Format("2006-01-02")]; ok {
			out[i].Count += v.AdditionsKept
		}
	}
	return out
}

// Neighbors renvoie jusqu'à n commits mesurés de l'auteur avant et après le commit sha (même dépôt),
// dans l'ordre chronologique, le commit lui-même compris.
func Neighbors(views []store.CommitSignalView, repoID, authorID int64, sha string, n int) []store.CommitSignalView {
	var mine []store.CommitSignalView
	for _, v := range views {
		if v.RepoID == repoID && v.AuthorID == authorID {
			mine = append(mine, v)
		}
	}
	i := slices.IndexFunc(mine, func(v store.CommitSignalView) bool { return v.SHA == sha })
	if i < 0 {
		return nil
	}
	return mine[max(i-n, 0):min(i+n+1, len(mine))]
}
