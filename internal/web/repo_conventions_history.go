package web

// Conventions de la phase 8.4 dans le web : réécritures observées de la branche par défaut (BR4),
// historique des tags (SV3), échéances des jalons (SV2), pré-versions (SV4), sauts de version (SV5),
// précédence (SV7) et fraîcheur du CHANGELOG (CL10), sur la page du dépôt et en synthèse dans
// « /conventions ». Aucune attribution à une personne, aucun message de commit ; seules des classes déjà
// présentes dans static/app.css (la CSS n'est pas regénérée).

import (
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

// convRewriteRow est une réécriture observée de la branche par défaut (BR4).
type convRewriteRow struct {
	Branch     string
	OldSHA     string
	NewSHA     string
	ObservedAt time.Time
}

// convTagHistoryRow est SV3 d'un tag : pastille et première observation.
type convTagHistoryRow struct {
	Name      string
	Status    convStatus
	FirstSeen time.Time
}

// convNoteRow est un constat SV4, SV5 ou SV7 : pastille, tag et autre tag de la paire.
type convNoteRow struct {
	Status convStatus
	Tag    string
	Other  string // vide si aucun
	Detail string // précision traduite (saut attendu, raison) ; vide si aucune
}

// convHistoryView est la partie « historique et versions » de la section du dépôt.
type convHistoryView struct {
	// BR4 : RewritesReason traduit si inconnu ; Rewrites : les plus récentes, au plus repoConventionsList.
	RewritesKnown  bool
	RewritesReason string
	RewritesTotal  int
	RewritesLast   time.Time
	Rewrites       []convRewriteRow
	// SV3 : TagsReason traduit si aucune observation ; TagsSince : première observation du dépôt.
	TagsObserved   bool
	TagsReason     string
	TagsSince      time.Time
	TagsStable     int // sans réécriture observée (stables, ou supprimés puis reposés sur le même commit)
	TagsRewritten  int
	TagsDeleted    int
	TagHistory     []convTagHistoryRow // tags réécrits, supprimés ou reposés d'abord, au plus repoConventionsList
	TagHistoryMore int                 // tags non affichés
	// SV4 (avertissements).
	PreKnown   bool
	PreReason  string
	PreChecked int
	PreConform int
	PreNotes   []convNoteRow
	// SV7.
	PrecKnown      bool
	PrecReason     string
	PrecPairs      int
	PrecUnknown    int
	PrecConsistent bool
	PrecNotes      []convNoteRow
	// SV5 : paires qui ne sont pas cohérentes seulement.
	BumpsKnown                                           bool
	BumpsReason                                          string
	BumpsConsistent, BumpsUnder, BumpsUndet, BumpsUnread int
	BumpsTotal                                           int
	Bumps                                                []convNoteRow
	// CL10 (information).
	FreshKnown   bool
	FreshReason  string
	FreshSince   string
	FreshFeat    int
	FreshFix     int
	FreshMissing int
	FreshGrayed  bool
}

// convHistorySummary est la synthèse de la phase 8.4 d'un dépôt dans « /conventions ».
type convHistorySummary struct {
	Rewrites      int
	TagsObserved  bool
	TagsChanged   int // réécrits ou supprimés
	BumpsKnown    bool
	BumpsUnder    int
	PreWarnings   int
	PrecAncestry  int // versions supérieures posées sur un ancêtre d'une version inférieure
	FreshKnown    bool
	FreshFeatFix  int
	FreshGrayed   bool
	RewritesKnown bool
}

// tagHistoryStatus est la pastille SV3 d'un tag.
func tagHistoryStatus(tr *i18n.Translator, status string) convStatus {
	key := "conv.history.status." + status
	switch status {
	case conventions.TagHistoryStable, conventions.TagHistoryRestored:
		return statusConform(tr, key)
	default:
		return statusNonconform(tr, key)
	}
}

// deadlineStatus est la pastille de l'échéance d'un jalon (SV2) ; nil sans échéance jugée.
func deadlineStatus(tr *i18n.Translator, d conventions.MilestoneDeadline) *convStatus {
	key := "conv.deadline." + d.Status
	var st convStatus
	switch d.Status {
	case "":
		return nil
	case conventions.DeadlineOnTime:
		st = statusConform(tr, key)
	case conventions.DeadlineLate:
		st = statusNonconform(tr, key)
	default:
		st = statusUnknown(tr, key)
	}
	return &st
}

// deadlineDetail précise l'échéance d'un jalon : première observation, ou « posé entre le … et le … ».
func (s *server) deadlineDetail(tr *i18n.Translator, d conventions.MilestoneDeadline) string {
	date := func(t time.Time) string {
		if t.IsZero() {
			return tr.T("conv.deadline.unknown_date")
		}
		return s.formatLocal(t, tr.T("web.fmt.date"))
	}
	switch d.Status {
	case "":
		return ""
	case conventions.DeadlineUndetermined:
		return tr.T("conv.deadline.between", date(d.CommitAt), date(d.FirstSeenAt))
	}
	return tr.T("conv.deadline.first_seen", date(d.FirstSeenAt))
}

// versionNotes traduit des constats SV4 ou SV7 ; info : codes donnés en information (pastille neutre).
func versionNotes(tr *i18n.Translator, prefix string, notes []conventions.VersionNote, info map[string]bool) []convNoteRow {
	out := make([]convNoteRow, 0, len(notes))
	for _, n := range notes {
		st := statusWarning(tr, prefix+n.Code)
		if info[n.Code] {
			st = convStatus{Icon: "i", Label: tr.T(prefix + n.Code), Class: "badge-ghost"}
		}
		out = append(out, convNoteRow{Status: st, Tag: n.Tag, Other: n.Other})
	}
	return out
}

// newConvHistory traduit les indicateurs de la phase 8.4 d'un dépôt.
func newConvHistory(tr *i18n.Translator, repo conventions.RepoReport) convHistoryView {
	var v convHistoryView
	reason := func(r string) string { return tr.T("web.conv.reason." + r) }

	rw := repo.Rewrites
	v.RewritesKnown, v.RewritesTotal, v.RewritesLast = rw.Known, rw.Count, rw.Last
	if !rw.Known {
		v.RewritesReason = reason(rw.Reason)
	}
	for _, e := range rw.Events {
		if len(v.Rewrites) >= repoConventionsList {
			break
		}
		v.Rewrites = append(v.Rewrites, convRewriteRow{Branch: e.Branch, OldSHA: e.OldSHA, NewSHA: e.NewSHA, ObservedAt: e.ObservedAt})
	}

	h := repo.TagHistory
	v.TagsObserved, v.TagsSince = h.Observed, h.Since
	if !h.Observed {
		v.TagsReason = reason(h.Reason)
	}
	v.TagsStable = h.Counts[conventions.TagHistoryStable] + h.Counts[conventions.TagHistoryRestored]
	v.TagsRewritten, v.TagsDeleted = h.Counts[conventions.TagHistoryRewritten], h.Counts[conventions.TagHistoryDeleted]
	// Les tags qui ont changé d'abord, puis les autres, chacun par nom.
	for _, pass := range []bool{false, true} {
		for _, t := range h.Tags {
			if (t.Status == conventions.TagHistoryStable) != pass {
				continue
			}
			if len(v.TagHistory) >= repoConventionsList {
				v.TagHistoryMore++
				continue
			}
			v.TagHistory = append(v.TagHistory, convTagHistoryRow{Name: t.Name, Status: tagHistoryStatus(tr, t.Status),
				FirstSeen: t.FirstSeenAt})
		}
	}

	p := repo.Prereleases
	v.PreKnown, v.PreChecked, v.PreConform = p.Known, p.Checked, p.Conform
	if !p.Known {
		v.PreReason = reason(p.Reason)
	}
	v.PreNotes = versionNotes(tr, "conv.prerelease_code.", p.Notes, nil)

	pc := repo.Precedence
	v.PrecKnown, v.PrecPairs, v.PrecUnknown, v.PrecConsistent = pc.Known, pc.Pairs, pc.Unknown, pc.Conform()
	if pc.Reason != "" {
		v.PrecReason = reason(pc.Reason)
	}
	v.PrecNotes = versionNotes(tr, "conv.precedence_code.", pc.Notes, map[string]bool{conventions.PrecedenceSkip: true})

	b := repo.Bumps
	v.BumpsKnown = b.Known
	if !b.Known {
		v.BumpsReason = reason(b.Reason)
	}
	v.BumpsConsistent, v.BumpsUnder = b.Counts[conventions.BumpConsistent], b.Counts[conventions.BumpUnder]
	v.BumpsUndet, v.BumpsUnread = b.Counts[conventions.BumpUndetermined], b.Counts[conventions.BumpUnknown]
	v.BumpsTotal = len(b.Lines)
	for _, l := range b.Lines {
		if l.Status == conventions.BumpConsistent || len(v.Bumps) >= repoConventionsList {
			continue
		}
		row := convNoteRow{Tag: l.To, Other: l.From}
		switch l.Status {
		case conventions.BumpUnder:
			row.Status = statusNonconform(tr, "conv.bump.status."+l.Status)
			row.Detail = tr.T("conv.bump.expected", tr.T("conv.bump."+l.Actual), tr.T("conv.bump."+l.Expected))
		default:
			row.Status = statusUnknown(tr, "conv.bump.status."+l.Status)
			if l.Reason != "" {
				row.Detail = reason(l.Reason)
			}
		}
		v.Bumps = append(v.Bumps, row)
	}

	f := repo.Freshness
	v.FreshKnown, v.FreshSince, v.FreshFeat, v.FreshFix = f.Known, f.Since, f.Feat, f.Fix
	v.FreshMissing, v.FreshGrayed = f.Missing, f.Grayed
	if !f.Known {
		v.FreshReason = reason(f.Reason)
	}
	return v
}

// newConvHistorySummary résume les indicateurs de la phase 8.4 d'un dépôt pour « /conventions ».
func newConvHistorySummary(repo conventions.RepoReport) convHistorySummary {
	s := convHistorySummary{RewritesKnown: repo.Rewrites.Known, Rewrites: repo.Rewrites.Count,
		TagsObserved: repo.TagHistory.Observed, BumpsKnown: repo.Bumps.Known, BumpsUnder: repo.Bumps.Counts[conventions.BumpUnder],
		PreWarnings: len(repo.Prereleases.Notes), FreshKnown: repo.Freshness.Known, FreshGrayed: repo.Freshness.Grayed,
		FreshFeatFix: repo.Freshness.Feat + repo.Freshness.Fix}
	s.TagsChanged = repo.TagHistory.Counts[conventions.TagHistoryRewritten] + repo.TagHistory.Counts[conventions.TagHistoryDeleted]
	for _, n := range repo.Precedence.Notes {
		if n.Code == conventions.PrecedenceAncestry {
			s.PrecAncestry++
		}
	}
	return s
}
