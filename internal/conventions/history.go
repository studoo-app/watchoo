package conventions

// Historique des tags et de la branche par défaut (phase 8.4) : SV3 (tag réécrit ou supprimé) et
// échéances de SV2, d'après les observations de la synchro (tag_observations), et BR4 (réécritures
// observées de la branche par défaut, branch_rewrites). Fonctions pures : l'appelant fournit les
// observations lues en base, la liste des tags lue par la synchro et l'ascendance lue dans le clone
// (AncestryFunc). Rien n'est attribué à une personne : une réécriture est observée, jamais « poussée par ».

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

// Ancestry est le résultat d'un test d'ascendance entre deux commits du clone.
type Ancestry int

// Résultats d'un test d'ascendance.
const (
	// AncestryUnknown : pas de clone, clone illisible ou commit absent du clone. Jamais lu comme « non ».
	AncestryUnknown Ancestry = iota
	AncestryYes              // le premier commit est ancêtre du second (ou le même commit)
	AncestryNo               // le premier commit n'est pas ancêtre du second
)

// AncestryFunc indique si le commit ancestor est ancêtre de descendant (git merge-base --is-ancestor).
// Un commit est son propre ancêtre. nil : aucun clone, tout est AncestryUnknown.
type AncestryFunc func(ancestor, descendant string) Ancestry

// ancestry appelle anc ; un même commit est son propre ancêtre, sans lecture du clone.
func ancestry(anc AncestryFunc, ancestor, descendant string) Ancestry {
	switch {
	case ancestor == "" || descendant == "":
		return AncestryUnknown
	case ancestor == descendant:
		return AncestryYes
	case anc == nil:
		return AncestryUnknown
	}
	return anc(ancestor, descendant)
}

// isCommitSHA indique un SHA complet en hexadécimal minuscule (SHA-1 ou SHA-256).
func isCommitSHA(s string) bool {
	if len(s) != 40 && len(s) != 64 {
		return false
	}
	return !strings.ContainsFunc(s, func(r rune) bool { return (r < '0' || r > '9') && (r < 'a' || r > 'f') })
}

// Observations des tags (alimentées par la synchro, seulement si conventions.enabled et si la lecture
// des tags a réussi sans être tronquée).

// TagObservationClose ferme l'observation ouverte d'un couple (nom, SHA).
type TagObservationClose struct {
	Name   string
	SHA    string
	Reason string // model.TagEndRewritten ou model.TagEndDeleted
}

// TagObservationPlan est la mise à jour des observations après une lecture complète des tags. Toutes les
// listes sont triées par nom puis SHA, vides non nil si rien à faire.
type TagObservationPlan struct {
	Seen  []model.Tag           // couples ouverts revus : dernière observation à la date de la lecture
	Close []TagObservationClose // observations ouvertes à fermer à la date de la lecture
	Open  []model.Tag           // couples sans observation ouverte : nouvelle période, première observation à la date de la lecture
}

// Empty indique un plan sans écriture.
func (p TagObservationPlan) Empty() bool {
	return len(p.Seen) == 0 && len(p.Close) == 0 && len(p.Open) == 0
}

// PlanTagObservations compare les observations ouvertes du dépôt (open ; les fermées sont ignorées) à la
// liste complète des tags lue par la synchro (current) :
//   - même nom, même SHA : revu ;
//   - même nom, autre SHA : l'ancienne observation est fermée (réécrit) et une nouvelle est ouverte ;
//   - nom absent de la liste : fermée (supprimé) ;
//   - couple sans observation ouverte : ouvert.
//
// Un tag sans nom ou sans SHA complet est ignoré ; un nom en double ne compte qu'une fois (premier vu).
// L'appelant ne l'applique que si la lecture a réussi et n'est pas tronquée : sinon rien n'est fermé.
func PlanTagObservations(open []model.TagObservation, current []model.Tag) TagObservationPlan {
	plan := TagObservationPlan{Seen: []model.Tag{}, Close: []TagObservationClose{}, Open: []model.Tag{}}
	cur := map[string]model.Tag{}
	for _, t := range current {
		if t.Name == "" || !isCommitSHA(t.SHA) {
			continue
		}
		if _, dup := cur[t.Name]; !dup {
			cur[t.Name] = t
		}
	}
	kept := map[string]bool{} // noms dont l'observation ouverte est revue
	for _, o := range open {
		if !o.Open() {
			continue
		}
		t, seen := cur[o.Name]
		switch {
		case !seen:
			plan.Close = append(plan.Close, TagObservationClose{Name: o.Name, SHA: o.SHA, Reason: model.TagEndDeleted})
		case t.SHA != o.SHA:
			plan.Close = append(plan.Close, TagObservationClose{Name: o.Name, SHA: o.SHA, Reason: model.TagEndRewritten})
		case kept[o.Name]: // observation ouverte en double : déjà revue
		default:
			kept[o.Name] = true
			plan.Seen = append(plan.Seen, model.Tag{RepoID: o.RepoID, Name: o.Name, SHA: o.SHA})
		}
	}
	for name, t := range cur {
		if !kept[name] {
			plan.Open = append(plan.Open, t)
		}
	}
	byTag := func(a, b model.Tag) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.SHA, b.SHA))
	}
	slices.SortFunc(plan.Seen, byTag)
	slices.SortFunc(plan.Open, byTag)
	slices.SortFunc(plan.Close, func(a, b TagObservationClose) int {
		return cmp.Or(strings.Compare(a.Name, b.Name), strings.Compare(a.SHA, b.SHA))
	})
	return plan
}

// ObservationReads renvoie les instants de lecture complète des tags que les observations prouvent
// (première et dernière observation, fin), triés et sans doublon. Ce sont des lectures réussies, non
// tronquées, faites avec les conventions actives ; une lecture qui n'a ouvert, revu en dernier ni fermé
// aucune observation n'y figure pas (la dernière observation d'un couple est réécrite à chaque lecture).
func ObservationReads(obs []model.TagObservation) []time.Time {
	var out []time.Time
	for _, o := range obs {
		for _, at := range []time.Time{o.FirstSeenAt, o.LastSeenAt} {
			if !at.IsZero() {
				out = append(out, at.UTC())
			}
		}
		if o.EndedAt != nil && !o.EndedAt.IsZero() {
			out = append(out, o.EndedAt.UTC())
		}
	}
	slices.SortFunc(out, time.Time.Compare)
	return slices.CompactFunc(out, time.Time.Equal)
}

// États SV3 d'un tag (clés --json), en anglais.
const (
	TagHistoryStable    = "stable"    // aucune réécriture observée depuis la première observation
	TagHistoryRewritten = "rewritten" // le nom a désigné un autre commit (déplacé, ou supprimé puis reposé ailleurs)
	TagHistoryDeleted   = "deleted"   // le nom a disparu et n'est pas revenu
	TagHistoryRestored  = "restored"  // supprimé puis reposé sur le même commit
)

// AllTagHistoryStatuses liste les états SV3 dans leur ordre d'affichage.
var AllTagHistoryStatuses = []string{TagHistoryStable, TagHistoryRestored, TagHistoryRewritten, TagHistoryDeleted}

// TagPeriod est une période d'observation d'un tag sur un commit.
type TagPeriod struct {
	SHA         string
	FirstSeenAt time.Time
	LastSeenAt  time.Time
	EndedAt     time.Time // zéro tant que la période est ouverte
	EndReason   string    // model.TagEndRewritten ou model.TagEndDeleted ; vide si ouverte
}

// Open indique une période en cours.
func (p TagPeriod) Open() bool { return p.EndedAt.IsZero() }

// TagHistory est SV3 d'un tag : ses périodes d'observation et son état.
type TagHistory struct {
	Name   string
	Status string // TagHistoryStable, TagHistoryRestored, TagHistoryRewritten ou TagHistoryDeleted
	SHA    string // commit visé de la période en cours ; vide si le tag est supprimé
	// FirstSeenAt est la première observation du nom : « aucune réécriture observée depuis le … ».
	FirstSeenAt time.Time
	Periods     []TagPeriod // par première observation croissante
	Rewrites    int         // périodes fermées parce que le nom a changé de SHA
	Deletions   int         // périodes fermées parce que le nom a disparu
}

// Conform indique un tag sans réécriture ni suppression observée (stable). Un tag supprimé puis reposé sur
// le même commit (restored) ne l'est pas : SV3 relève toute réécriture ou suppression observée.
func (h TagHistory) Conform() bool {
	return h.Status == TagHistoryStable
}

// TagHistoryReport est SV3 d'un dépôt.
type TagHistoryReport struct {
	// Observed indique au moins une observation : sans elle (conventions désactivées, synchro à venir,
	// lectures toujours tronquées), SV3 est inconnu.
	Observed bool
	Reason   string       // UnknownNotObserved sans observation ; vide sinon
	Since    time.Time    // première observation du dépôt ; zéro sans observation
	Tags     []TagHistory // par nom ; vide non nil
	Counts   map[string]int
}

// BuildTagHistory calcule SV3 pour chaque tag observé du dépôt (les observations d'un même dépôt).
func BuildTagHistory(obs []model.TagObservation) TagHistoryReport {
	rep := TagHistoryReport{Tags: []TagHistory{}, Counts: map[string]int{}}
	for _, s := range AllTagHistoryStatuses {
		rep.Counts[s] = 0
	}
	byName := map[string][]TagPeriod{}
	for _, o := range obs {
		if o.Name == "" {
			continue
		}
		p := TagPeriod{SHA: o.SHA, FirstSeenAt: o.FirstSeenAt.UTC(), LastSeenAt: o.LastSeenAt.UTC()}
		if o.EndedAt != nil {
			p.EndedAt, p.EndReason = o.EndedAt.UTC(), o.EndReason
		}
		byName[o.Name] = append(byName[o.Name], p)
		if !rep.Observed || p.FirstSeenAt.Before(rep.Since) {
			rep.Since = p.FirstSeenAt
		}
		rep.Observed = true
	}
	for name, periods := range byName {
		h := tagHistory(name, periods)
		rep.Tags = append(rep.Tags, h)
		rep.Counts[h.Status]++
	}
	slices.SortFunc(rep.Tags, func(a, b TagHistory) int { return strings.Compare(a.Name, b.Name) })
	if !rep.Observed {
		rep.Reason = UnknownNotObserved
	}
	return rep
}

// tagHistory calcule l'état d'un tag d'après ses périodes.
func tagHistory(name string, periods []TagPeriod) TagHistory {
	slices.SortStableFunc(periods, func(a, b TagPeriod) int {
		return cmp.Or(a.FirstSeenAt.Compare(b.FirstSeenAt), strings.Compare(a.SHA, b.SHA))
	})
	h := TagHistory{Name: name, Periods: periods, FirstSeenAt: periods[0].FirstSeenAt}
	current := -1
	for i, p := range periods {
		switch {
		case p.Open():
			current = i // la plus récente des périodes ouvertes (une seule en régime normal)
		case p.EndReason == model.TagEndRewritten:
			h.Rewrites++
		default:
			h.Deletions++
		}
	}
	switch {
	case current < 0:
		h.Status = TagHistoryDeleted
	case slices.ContainsFunc(periods, func(p TagPeriod) bool { return p.SHA != periods[current].SHA }):
		h.Status, h.SHA = TagHistoryRewritten, periods[current].SHA
	case h.Rewrites+h.Deletions > 0:
		h.Status, h.SHA = TagHistoryRestored, periods[current].SHA
	default:
		h.Status, h.SHA = TagHistoryStable, periods[current].SHA
	}
	return h
}

// FindTagHistory renvoie SV3 du tag name ; faux s'il n'a jamais été observé.
func FindTagHistory(rep TagHistoryReport, name string) (TagHistory, bool) {
	i := slices.IndexFunc(rep.Tags, func(h TagHistory) bool { return h.Name == name })
	if i < 0 {
		return TagHistory{}, false
	}
	return rep.Tags[i], true
}

// Revue de jalon : SV3 du tag qui désigne le jalon.

// CriterionSV3 : aucune réécriture observée du tag du jalon (critère « tags de jalon jamais réécrits »,
// avec SV1 et SV2).
const CriterionSV3 = "SV3"

// UnknownNotObserved : le tag n'a jamais été vu par une lecture complète des tags (conventions activées
// après la dernière synchro, ou lectures tronquées) ; SV3 et l'échéance de SV2 ne peuvent pas être jugés.
const UnknownNotObserved = "not_observed"

// MilestoneSV3 calcule le critère SV3 de la revue de jalon pour le tag qui désigne le jalon (nom exact, ou
// premier nom non conforme). Tag absent : « inconnu » avec la raison absentReason (celle de SV1 :
// UnknownNoTag, UnknownNotDue ou UnknownTagAbsent ; vide : UnknownNoTag). Tag jamais observé : inconnu
// (UnknownNotObserved). Sinon conforme si aucune réécriture ni suppression n'a été observée (stable), non
// conforme s'il a été réécrit ou supprimé, même reposé ensuite sur le même commit.
func MilestoneSV3(ms MilestoneResult, hist TagHistoryReport, absentReason string) Criterion {
	if ms.Tag == "" {
		if absentReason == "" {
			absentReason = UnknownNoTag
		}
		return Criterion{ID: CriterionSV3, Status: CriterionUnknown, Reason: absentReason}
	}
	h, ok := FindTagHistory(hist, ms.Tag)
	if !ok {
		return Criterion{ID: CriterionSV3, Status: CriterionUnknown, Reason: UnknownNotObserved}
	}
	if h.Conform() {
		return Criterion{ID: CriterionSV3, Status: CriterionConform}
	}
	return Criterion{ID: CriterionSV3, Status: CriterionNonconform}
}

// Échéances de SV2 : la pose du tag de nom exact comparée à la fin de l'échéance du jalon.

// États de l'échéance d'un jalon (clés --json), en anglais.
const (
	DeadlineOnTime       = "on_time"      // première observation du tag avant la fin de l'échéance
	DeadlineLate         = "late"         // une lecture complète des tags faite à la fin de l'échéance ou après ne l'a pas vu
	DeadlineUndetermined = "undetermined" // observé après, sans lecture qui prouve son absence à la fin de l'échéance
)

// AllDeadlineStatuses liste les états d'échéance dans leur ordre d'affichage.
var AllDeadlineStatuses = []string{DeadlineOnTime, DeadlineLate, DeadlineUndetermined}

// MilestoneDeadline est l'échéance de SV2 d'un jalon dans un dépôt. Indéterminée, le tag a été posé
// entre SetAfter (si connue) et FirstSeenAt (première observation, si observé).
type MilestoneDeadline struct {
	Status      string    // DeadlineOnTime, DeadlineLate ou DeadlineUndetermined ; vide : sans échéance ou sans tag de nom exact
	DueEnd      time.Time // fin de l'échéance (borne exclue)
	FirstSeenAt time.Time // première observation du tag ; zéro s'il n'a jamais été observé
	CommitAt    time.Time // date du commit visé à la première observation du tag ; zéro si inconnue
	// SetAfter est la borne basse de la pose : la plus tardive entre CommitAt et la dernière lecture
	// complète des tags qui ne l'a pas vu, antérieures à FirstSeenAt ; zéro si aucune n'est connue.
	SetAfter time.Time
}

// CheckDeadline compare la pose du tag du jalon à son échéance (fin exclue : minuit du lendemain dans le
// fuseau loc, UTC si nil). Seul un tag de nom exact (MilestonePresent) est jugé :
//   - première observation avant la fin de l'échéance : à l'heure ;
//   - première observation après, et une lecture complète des tags (reads) a eu lieu à la fin de
//     l'échéance ou après, mais avant la première observation : en retard, cette lecture prouve que le tag
//     manquait encore après l'échéance ;
//   - sinon (jamais observé, aucune lecture dans cette fenêtre) : indéterminé. Une lecture antérieure à la
//     fin de l'échéance ne prouve rien : le tag a pu être posé entre elle et l'échéance.
//
// firstSeen est la première observation du nom (zéro : jamais observé) ; commitAt la date du commit visé à
// cette première observation (zéro : inconnue), qui ne sert qu'à la borne basse SetAfter.
func CheckDeadline(r MilestoneResult, loc *time.Location, firstSeen, commitAt time.Time, reads []time.Time) MilestoneDeadline {
	end, ok := r.DueEnd(loc)
	if !ok || r.Status != MilestonePresent {
		return MilestoneDeadline{}
	}
	d := MilestoneDeadline{Status: DeadlineUndetermined, DueEnd: end, FirstSeenAt: firstSeen, CommitAt: commitAt}
	if firstSeen.IsZero() {
		d.SetAfter = commitAt
		return d
	}
	if !commitAt.IsZero() && commitAt.Before(firstSeen) {
		d.SetAfter = commitAt
	}
	for _, at := range reads {
		if !at.Before(firstSeen) {
			continue
		}
		if at.After(d.SetAfter) {
			d.SetAfter = at
		}
		if !at.Before(end) {
			d.Status = DeadlineLate
		}
	}
	if firstSeen.Before(end) {
		d.Status = DeadlineOnTime
	}
	return d
}

// UnknownDeadline : SV2 de la revue, tag de nom exact présent mais pose indéterminée par rapport à
// l'échéance.
const UnknownDeadline = "deadline_undetermined"

// MilestoneSV2 calcule le critère SV2 de la revue de jalon à l'instant now : tag de nom exact présent,
// conforme s'il n'a pas d'échéance ou s'il a été posé à l'heure, non conforme en retard, inconnu
// (UnknownDeadline) si la pose est indéterminée ; tag absent avant la fin de l'échéance : inconnu
// (UnknownNotDue) ; sinon (absent, ou seulement sous un nom non conforme) : non conforme. d est le résultat
// de CheckDeadline ; un d vide (échéances non lues) juge la seule présence.
func MilestoneSV2(ms MilestoneResult, d MilestoneDeadline, loc *time.Location, now time.Time) Criterion {
	c := Criterion{ID: CriterionSV2, Status: CriterionNonconform}
	switch ms.Status {
	case MilestonePresent:
		switch d.Status {
		case DeadlineLate:
		case DeadlineUndetermined:
			c.Status, c.Reason = CriterionUnknown, UnknownDeadline
		default:
			c.Status = CriterionConform
		}
	case MilestoneAbsent:
		if end, ok := ms.DueEnd(loc); ok && now.Before(end) {
			c.Status, c.Reason = CriterionUnknown, UnknownNotDue
		}
	}
	return c
}

// BR4 : réécritures de la branche par défaut.

// HeadChange décrit la tête de la branche par défaut connue avant une synchro (Prev*) et lue par elle.
type HeadChange struct {
	PrevBranch  string // branche par défaut connue avant la synchro
	PrevSHA     string // sa tête connue
	PrevTracked bool   // suivie avant la synchro (sinon sa tête en base est périmée)
	Branch      string // branche par défaut lue par la synchro
	SHA         string // sa nouvelle tête
	Tracked     bool   // suivie après la synchro
}

// Comparable indique deux têtes comparables : même branche par défaut (un changement de branche par défaut
// n'est pas une réécriture), suivie avant et après, deux SHA complets et différents.
func (h HeadChange) Comparable() bool {
	return h.Branch != "" && h.PrevBranch == h.Branch && h.PrevTracked && h.Tracked &&
		isCommitSHA(h.PrevSHA) && isCommitSHA(h.SHA) && h.PrevSHA != h.SHA
}

// RewriteObserved indique une réécriture à enregistrer : têtes comparables et ancienne tête qui n'est
// plus ancêtre de la nouvelle (a, lu dans le clone : git merge-base --is-ancestor). Un ancien objet absent
// du clone donne AncestryUnknown : jamais une réécriture. En mode API sans clone, l'appelant traduit
// ErrUnknownBase (ancienne tête inconnue de la forge) en AncestryNo.
func RewriteObserved(h HeadChange, a Ancestry) bool {
	return h.Comparable() && a == AncestryNo
}

// RewriteEvent est une réécriture observée de la branche par défaut.
type RewriteEvent struct {
	Branch     string
	OldSHA     string
	NewSHA     string
	ObservedAt time.Time
}

// RewriteReport est BR4 d'un dépôt : les réécritures observées, sans attribution.
type RewriteReport struct {
	// Known est toujours vrai et Reason toujours vide : BR4 est lu en base, même dans un rapport restreint à
	// des contributeurs (indicateur de dépôt) ; les deux champs gardent la forme des autres indicateurs.
	// Aucune branche n'est jamais « jamais réécrite » : seulement « aucune réécriture observée ».
	Known  bool
	Reason string
	Count  int
	Events []RewriteEvent // de la plus récente à la plus ancienne ; vide non nil
	Last   time.Time      // date de la plus récente ; zéro si aucune
}

// BuildRewriteReport restitue les réécritures observées d'un dépôt.
func BuildRewriteReport(rows []model.BranchRewrite) RewriteReport {
	rep := RewriteReport{Known: true, Events: make([]RewriteEvent, 0, len(rows))}
	for _, r := range rows {
		rep.Events = append(rep.Events, RewriteEvent{Branch: r.Branch, OldSHA: r.OldSHA, NewSHA: r.NewSHA,
			ObservedAt: r.ObservedAt.UTC()})
	}
	slices.SortStableFunc(rep.Events, func(a, b RewriteEvent) int {
		return cmp.Or(b.ObservedAt.Compare(a.ObservedAt), strings.Compare(a.Branch, b.Branch),
			strings.Compare(a.OldSHA, b.OldSHA))
	})
	rep.Count = len(rep.Events)
	if rep.Count > 0 {
		rep.Last = rep.Events[0].ObservedAt
	}
	return rep
}
