package conventions

import (
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/aisignature"
)

// Motifs d'exclusion CC4 (clés --json) : un commit exclu sort du dénominateur de CC1 et CC3 et est compté
// à part, sous un seul motif (le premier qui s'applique, dans l'ordre d'AllExclusions).
const (
	ExclusionMerge         = "merge"            // fusion : au moins deux parents, ou message de fusion par défaut si inconnu
	ExclusionBot           = "bot"              // auteur bot (aisignature.IsBot)
	ExclusionInitialCommit = "initial_commit"   // commit racine « Initial commit » créé par GitHub
	ExclusionAccount       = "excluded_account" // auteur de conventions.excluded_accounts
)

// AllExclusions liste les motifs d'exclusion CC4 dans leur ordre d'application et d'affichage.
var AllExclusions = []string{ExclusionMerge, ExclusionBot, ExclusionInitialCommit, ExclusionAccount}

// githubInitialSubject est le sujet du premier commit d'un dépôt créé sur GitHub avec un fichier.
const githubInitialSubject = "Initial commit"

// Commit est un commit à évaluer, lu par l'appelant (base et clone). Le message reste en mémoire : aucun
// résultat ne le recopie.
type Commit struct {
	SHA             string
	AuthorKey       string    // identifiant stable du contributeur (regroupement CC3) ; vide : inconnu
	AuthorName      string    // nom Git de l'auteur
	AuthorLogin     string    // login de forge de l'auteur ; vide si inconnu
	AuthorEmails    []string  // adresses Git de l'auteur (comparées aux comptes exclus)
	Message         string    // message complet (donnée personnelle, jamais recopié)
	ParentCount     int       // nombre de parents, si ParentsKnown
	ParentsKnown    bool      // faux : commits.parent_count est NULL
	AuthoredAt      time.Time // date d'auteur
	Bot             bool      // auteur bot déjà reconnu par l'appelant (s'ajoute à aisignature.IsBot)
	GitHubCommitter bool      // committer GitHub (aisignature.IsGitHubCommitter)
	Root            bool      // commit racine (sans parent) ; déduit aussi de ParentCount 0 connu
}

// Options règlent l'agrégation.
type Options struct {
	Rules            Rules
	Since            time.Time // date de début effective ; zéro : depuis le début
	ExcludedAccounts []string  // conventions.excluded_accounts : logins, noms ou adresses, casse ignorée
}

// Issue est un code d'écart CC2 et les commits qui l'ont reçu.
type Issue struct {
	Code     string
	Severity Severity
	Count    int
	SHAs     []string // dans l'ordre des commits donnés
}

// CommitResult est le résultat d'un commit : codes CC2 s'il est évalué, motif sinon.
type CommitResult struct {
	SHA         string
	AuthorKey   string
	BeforeSince bool     // antérieur à la date de début : ni évalué ni exclu par un motif
	Excluded    string   // motif CC4 ; vide si évalué ou antérieur à la date de début
	Codes       []string // codes CC2 ; vide si non évalué
	Conform     bool     // évalué et sans écart bloquant
	Traits      Traits   // CC5 à CC7 (informatifs) ; zéro si non évalué
}

// Evaluated indique un commit compté au dénominateur.
func (c CommitResult) Evaluated() bool { return !c.BeforeSince && c.Excluded == "" }

// Tally est un taux n/m et ses écarts : celui d'un dépôt (CC1) ou d'un contributeur (CC3).
type Tally struct {
	Evaluated int            // dénominateur : commits évalués (hors CC4 et hors date de début)
	Conform   int            // numérateur : commits évalués sans écart bloquant
	Excluded  map[string]int // CC4 : commits exclus par motif (toutes les clés d'AllExclusions)
	Issues    []Issue        // CC2 : codes reçus, dans l'ordre d'AllCodes, sans les codes à zéro
}

// Rate renvoie Conform / Evaluated ; ok est faux sans commit évalué (taux inconnu, pas 0 %).
func (t Tally) Rate() (rate float64, ok bool) {
	if t.Evaluated == 0 {
		return 0, false
	}
	return float64(t.Conform) / float64(t.Evaluated), true
}

// ExcludedTotal renvoie le nombre de commits exclus, tous motifs confondus.
func (t Tally) ExcludedTotal() int {
	n := 0
	for _, v := range t.Excluded {
		n += v
	}
	return n
}

// ContributorTally est le taux CC3 d'un contributeur.
type ContributorTally struct {
	AuthorKey string
	Tally
}

// Report est le résultat CC1 à CC4 d'un ensemble de commits (un dépôt, en général).
type Report struct {
	RulesVersion int
	Tally                           // CC1, CC2 par motif, CC4
	BeforeSince  int                // commits antérieurs à la date de début, comptés à part
	Contributors []ContributorTally // CC3, triés par AuthorKey
	Commits      []CommitResult     // un par commit donné, dans le même ordre
}

// Aggregate évalue les commits et calcule CC1 à CC4. Chaque commit est compté une fois : un SHA déjà vu
// est ignoré.
func Aggregate(commits []Commit, opts Options) Report {
	p := NewParser(opts.Rules)
	rep := Report{RulesVersion: RulesVersion, Tally: newTally(), Commits: []CommitResult{}, Contributors: []ContributorTally{}}
	byAuthor := map[string]*Tally{}
	seen := map[string]bool{}
	for _, c := range commits {
		if seen[c.SHA] {
			continue
		}
		seen[c.SHA] = true
		r := CommitResult{SHA: c.SHA, AuthorKey: c.AuthorKey, Codes: []string{}}
		at := byAuthor[c.AuthorKey]
		if at == nil {
			t := newTally()
			at = &t
			byAuthor[c.AuthorKey] = at
		}
		switch {
		case !opts.Since.IsZero() && c.AuthoredAt.Before(opts.Since):
			r.BeforeSince = true
			rep.BeforeSince++
		default:
			r.Excluded = exclusion(c, opts.ExcludedAccounts)
		}
		if r.Evaluated() {
			res := p.Parse(c.Message)
			r.Codes, r.Conform = res.Codes, res.Conform()
			r.Traits = Inspect(c.Message)
		}
		if !r.BeforeSince {
			rep.add(r)
			at.add(r)
		}
		rep.Commits = append(rep.Commits, r)
	}
	keys := make([]string, 0, len(byAuthor))
	for k, t := range byAuthor {
		if t.Evaluated > 0 || t.ExcludedTotal() > 0 {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	for _, k := range keys {
		byAuthor[k].sortIssues()
		rep.Contributors = append(rep.Contributors, ContributorTally{AuthorKey: k, Tally: *byAuthor[k]})
	}
	rep.sortIssues()
	return rep
}

func newTally() Tally {
	t := Tally{Excluded: map[string]int{}, Issues: []Issue{}}
	for _, e := range AllExclusions {
		t.Excluded[e] = 0
	}
	return t
}

// add compte le résultat r (évalué ou exclu) dans t.
func (t *Tally) add(r CommitResult) {
	if r.Excluded != "" {
		t.Excluded[r.Excluded]++
		return
	}
	t.Evaluated++
	if r.Conform {
		t.Conform++
	}
	for _, code := range r.Codes {
		i := slices.IndexFunc(t.Issues, func(is Issue) bool { return is.Code == code })
		if i < 0 {
			t.Issues = append(t.Issues, Issue{Code: code, Severity: SeverityOf(code), SHAs: []string{}})
			i = len(t.Issues) - 1
		}
		t.Issues[i].Count++
		t.Issues[i].SHAs = append(t.Issues[i].SHAs, r.SHA)
	}
}

// sortIssues range les écarts dans l'ordre d'AllCodes.
func (t *Tally) sortIssues() {
	slices.SortStableFunc(t.Issues, func(a, b Issue) int {
		return slices.Index(AllCodes, a.Code) - slices.Index(AllCodes, b.Code)
	})
}

// exclusion renvoie le motif CC4 du commit c ; vide s'il est évalué. Les reverts de Git restent évalués
// (avertissement git_revert), comme les éditions web de GitHub et les comptes d'agents d'IA.
func exclusion(c Commit, excluded []string) string {
	switch {
	case isMerge(c):
		return ExclusionMerge
	case c.Bot || aisignature.IsBot(c.AuthorName) || aisignature.IsBot(c.AuthorLogin):
		return ExclusionBot
	// Nombre de parents inconnu (commit d'avant la migration sans clone, rattrapage en échec) : repli
	// sur le committer GitHub et le sujet exact, comme les fusions se replient sur le message.
	case (c.Root || !c.ParentsKnown || c.ParentCount == 0) && c.GitHubCommitter &&
		StripSquashSuffix(Subject(c.Message)) == githubInitialSubject:
		return ExclusionInitialCommit
	case isExcludedAccount(c, excluded):
		return ExclusionAccount
	}
	return ""
}

// isMerge indique une fusion : au moins deux parents, ou, si le nombre de parents est inconnu, un message
// de fusion par défaut de Git ou de GitHub.
func isMerge(c Commit) bool {
	if c.ParentsKnown {
		return c.ParentCount >= 2
	}
	return IsDefaultMergeMessage(c.Message)
}

// isExcludedAccount indique un auteur de la liste des comptes exclus : login, nom ou adresse, casse
// ignorée.
func isExcludedAccount(c Commit, excluded []string) bool {
	for _, x := range excluded {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if strings.EqualFold(x, strings.TrimSpace(c.AuthorLogin)) || strings.EqualFold(x, strings.TrimSpace(c.AuthorName)) {
			return true
		}
		for _, e := range c.AuthorEmails {
			if strings.EqualFold(x, strings.TrimSpace(e)) {
				return true
			}
		}
	}
	return false
}
