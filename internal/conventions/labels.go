package conventions

// Noms donnés aux contributeurs dans l'interface web (1.8.0) : recopiés sur les lignes du rapport pour
// l'affichage, jamais lus par l'évaluation (comptes exclus, bots et attribution restent sur le nom Git
// et le login).

import "cmp"

// applyLabels recopie b.Labels sur les auteurs des lignes du rapport.
func (b *BuildReport) applyLabels() {
	if len(b.Labels) == 0 {
		return
	}
	for i := range b.Contributors {
		b.Contributors[i].AuthorLabel = b.Labels[b.Contributors[i].AuthorID]
	}
	for i := range b.Repos {
		rep := &b.Repos[i]
		for j := range rep.Commits {
			rep.Commits[j].AuthorLabel = b.Labels[rep.Commits[j].AuthorID]
		}
		for j := range rep.Pulls.Pulls {
			rep.Pulls.Pulls[j].AuthorLabel = b.Labels[rep.Pulls.Pulls[j].AuthorID]
		}
		for j := range rep.Arrivals.Lines {
			rep.Arrivals.Lines[j].AuthorLabel = b.Labels[rep.Arrivals.Lines[j].AuthorID]
		}
		for j := range rep.Branches.Branches {
			line := &rep.Branches.Branches[j]
			line.AuthorLabels = make([]string, len(line.AuthorIDs))
			for k, id := range line.AuthorIDs {
				line.AuthorLabels[k] = b.Labels[id]
			}
		}
	}
}

// DisplayAuthor renvoie le nom affiché de l'auteur du commit : nom donné, sinon nom, sinon login.
func (c CommitLine) DisplayAuthor() string { return cmp.Or(c.AuthorLabel, c.AuthorName, c.AuthorLogin) }

// DisplayAuthor renvoie le nom affiché du contributeur : nom donné, sinon nom, sinon login.
func (c ContributorReport) DisplayAuthor() string {
	return cmp.Or(c.AuthorLabel, c.AuthorName, c.AuthorLogin)
}

// DisplayAuthor renvoie le nom affiché de l'auteur de la pull request : nom donné, sinon nom, sinon login.
func (p PullLine) DisplayAuthor() string { return cmp.Or(p.AuthorLabel, p.AuthorName, p.AuthorLogin) }

// DisplayAuthor renvoie le nom affiché de l'auteur du commit : nom donné, sinon nom, sinon login.
func (l ArrivalLine) DisplayAuthor() string {
	return cmp.Or(l.AuthorLabel, l.AuthorName, l.AuthorLogin)
}

// AuthorLabel renvoie le nom donné au k-ième auteur de la branche ; vide si aucun.
func (l BranchLine) AuthorLabel(k int) string {
	if k < len(l.AuthorLabels) {
		return l.AuthorLabels[k]
	}
	return ""
}
