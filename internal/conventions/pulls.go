package conventions

import (
	"strings"

	"github.com/studoo-app/watchoo/internal/aisignature"
	"github.com/studoo-app/watchoo/internal/model"
)

// Catégories PR1 d'une pull request : ouvertes et fusionnées sont comptées à part.
const (
	PRCategoryOpen   = "open"
	PRCategoryMerged = "merged"
)

// Motifs qui sortent une pull request de PR1 et PR2.
const (
	PRSkipDraft  = "draft"  // brouillon
	PRSkipBot    = "bot"    // auteur bot (aisignature.IsBot)
	PRSkipClosed = "closed" // fermée sans fusion
)

// PRCategory classe une pull request pour PR1 : « open » ou « merged », sinon le motif qui l'exclut
// (fermée sans fusion, brouillon ouvert, auteur bot), ok faux. Une PR fusionnée n'est plus un brouillon.
func PRCategory(pr model.PullRequest) (category string, ok bool) {
	switch {
	case pr.Status() == model.PullRequestStatusClosed:
		return PRSkipClosed, false
	case aisignature.IsBot(pr.AuthorLogin):
		return PRSkipBot, false
	case pr.Status() == model.PullRequestStatusMerged:
		return PRCategoryMerged, true
	case pr.Draft:
		return PRSkipDraft, false
	}
	return PRCategoryOpen, true
}

// Codes PR2 d'un titre par défaut ou interdit (clés --json), en anglais.
const (
	PRTitleForbidden     = CodeForbidden     // titre de la liste interdite (ou préfixe wip, fixup!…)
	PRTitleGitHubDefault = CodeGitHubDefault // titre proposé par GitHub (« Update README.md »)
	PRTitleHeadBranch    = "head_branch"     // titre égal au nom de la branche de tête, après normalisation
)

// AllPRTitleCodes liste les codes PR2 dans leur ordre d'affichage.
var AllPRTitleCodes = []string{PRTitleForbidden, PRTitleGitHubDefault, PRTitleHeadBranch}

// PRTitleResult est l'analyse du titre d'une pull request. Il ne garde rien du titre.
type PRTitleResult struct {
	Type     string   // type lu dans l'en-tête ; vide si aucun
	Scope    string   // scope lu ; vide si aucun
	Breaking bool     // « ! » dans l'en-tête
	Codes    []string // PR1 : codes CC2 de l'en-tête (le titre n'a pas de corps), ordre d'AllCodes
	Default  []string // PR2 : codes d'AllPRTitleCodes ; vide non nil si aucun
}

// Conform indique un titre conventionnel (PR1) : aucun écart bloquant du parseur commun.
func (r PRTitleResult) Conform() bool {
	return Result{Codes: r.Codes}.Conform()
}

// IsDefault indique un titre par défaut ou interdit (PR2).
func (r PRTitleResult) IsDefault() bool { return len(r.Default) > 0 }

// CheckPRTitle analyse le titre d'une pull request : PR1 par le parseur commun, sur la seule première
// ligne (règles d'en-tête, ni corps ni pieds), et PR2 : titre interdit, proposé par GitHub, ou égal au
// nom de la branche de tête après normalisation (casse, « - » et « _ » en espaces). La comparaison avec le
// sujet du seul commit de la PR n'est pas faite : la base ne relie pas les commits aux PR.
func (p *Parser) CheckPRTitle(title, headBranch string) PRTitleResult {
	subject := Subject(title)
	res := p.Parse(subject)
	out := PRTitleResult{Type: res.Type, Scope: res.Scope, Breaking: res.Breaking, Codes: res.Codes, Default: []string{}}
	for _, c := range res.Codes {
		if c == CodeForbidden || c == CodeGitHubDefault {
			out.Default = append(out.Default, c)
		}
	}
	if b := normalizeTitle(headBranch); b != "" && normalizeTitle(StripSquashSuffix(subject)) == b {
		out.Default = append(out.Default, PRTitleHeadBranch)
	}
	return out
}

// normalizeTitle ramène un titre ou un nom de branche à sa forme de comparaison PR2 : minuscules, « - »
// et « _ » en espaces, espaces simples.
func normalizeTitle(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(strings.ToLower(s))
	return strings.Join(strings.Fields(s), " ")
}
