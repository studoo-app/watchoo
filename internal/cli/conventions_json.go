package cli

// Sortie JSON des indicateurs de la phase 8.2 (complétés en phase 8.5 : PR5, PR6, BR2, PR3 de la revue) de « watchoo conventions » : nommage des branches (BR1),
// titres de pull request (PR1, PR2 : numéro et codes, jamais le titre), tags et jalons (SV1, SV2), releases
// en information (SV6), formulation, pieds et langue des commits (CC5 à CC7), CC1 par intervalle entre
// tags, tendance de CC3 et revue de jalon. Clés en anglais, ajoutées à celles de la phase 8.1.

import (
	"github.com/studoo-app/watchoo/internal/conventions"
)

// conventionsTraitsJSON est CC5 et CC6 d'un dépôt ou d'un contributeur (informatifs).
type conventionsTraitsJSON struct {
	Evaluated      int            `json:"evaluated"`       // commits évalués comptés
	Wording        map[string]int `json:"wording"`         // CC5 : past_tense, progressive, first_person
	Footer         int            `json:"footer"`          // CC6 : commits avec un bloc de pieds valide
	IssueReference int            `json:"issue_reference"` // CC6 : commits qui citent une issue
}

// conventionsLanguageJSON est CC7 d'un dépôt (information).
type conventionsLanguageJSON struct {
	Expected string `json:"expected"` // conventions.commit.language : fr, en ou any
	FR       int    `json:"fr"`
	EN       int    `json:"en"`
	Unknown  int    `json:"unknown"`
	Other    int    `json:"other"` // commits reconnus dans l'autre langue ; 0 avec any
	Mixed    bool   `json:"mixed"` // les deux langues dépassent chacune 20 % des commits reconnus
}

// conventionsBranchesJSON est BR1 d'un dépôt.
type conventionsBranchesJSON struct {
	Source  string                      `json:"source"` // clone ou database (branches suivies, liste partielle)
	Checked int                         `json:"checked"`
	Conform int                         `json:"conform"`
	Review  int                         `json:"review"`
	Items   []conventionsBranchItemJSON `json:"items"`  // par nom
	Reused  conventionsReusedJSON       `json:"reused"` // BR2 (phase 8.5)
}

// conventionsBranchItemJSON est un nom de branche évalué.
type conventionsBranchItemJSON struct {
	Name         string       `json:"name"`
	Codes        []string     `json:"codes"`
	Conform      bool         `json:"conform"`
	Review       bool         `json:"review"`
	InSource     bool         `json:"in_source"`     // lue à la source (clone, ou base sans clone) ; faux : seulement branche de tête de PR
	PullRequests []int        `json:"pull_requests"` // PR dont c'est la branche de tête
	Authors      []authorJSON `json:"authors"`       // auteurs de ces PR (seule attribution d'une branche)
}

// conventionsPRCountJSON compte des PR évaluées (PR1, PR2 ; PR5 et PR6, phase 8.5).
type conventionsPRCountJSON struct {
	Evaluated    int `json:"evaluated"`
	Conform      int `json:"conform"`       // PR1 : titre conventionnel
	Default      int `json:"default"`       // PR2 : titre par défaut ou interdit
	BodyKnown    int `json:"body_known"`    // PR5 : corps lu (masque connu)
	BodyConform  int `json:"body_conform"`  // PR5 : sections attendues et mot-clé de fermeture
	MergeKnown   int `json:"merge_known"`   // PR6 : méthode de fusion déterminée (fusionnées)
	Squash       int `json:"squash"`        // PR6 : fusionnées en squash
	MergeConform int `json:"merge_conform"` // PR6 : squash et message conforme
}

// conventionsPullsJSON est PR1 et PR2 d'un dépôt.
type conventionsPullsJSON struct {
	Known   bool                      `json:"known"`
	Reason  *string                   `json:"reason"` // no_api si inconnu ; null sinon
	Open    conventionsPRCountJSON    `json:"open"`
	Merged  conventionsPRCountJSON    `json:"merged"`
	Skipped map[string]int            `json:"skipped"` // closed, draft, bot, fork, before_since
	Items   []conventionsPullItemJSON `json:"items"`   // numéro décroissant
	Methods map[string]int            `json:"methods"` // PR6 : fusionnées par méthode (squash, merge, rebase, unknown)
}

// conventionsPullItemJSON est une PR évaluée, sans son titre.
type conventionsPullItemJSON struct {
	Number       int        `json:"number"`
	Category     string     `json:"category"` // open ou merged
	Author       authorJSON `json:"author"`
	HeadBranch   string     `json:"head_branch"`
	Codes        []string   `json:"codes"`         // PR1 : codes CC2 de l'en-tête
	DefaultCodes []string   `json:"default_codes"` // PR2 : forbidden, github_default, head_branch
	Conform      bool       `json:"conform"`
	// Phase 8.5.
	Body  conventionsBodyJSON   `json:"body"`  // PR5 (jamais le corps)
	Merge *conventionsMergeJSON `json:"merge"` // PR6 ; null pour une PR ouverte
}

// conventionsTagsJSON est SV1, SV2 et SV6 d'un dépôt.
type conventionsTagsJSON struct {
	Checked    int                            `json:"checked"`
	Conform    int                            `json:"conform"`
	Items      []conventionsTagItemJSON       `json:"items"`      // par nom
	Milestones []conventionsMilestoneItemJSON `json:"milestones"` // dans l'ordre de la configuration
	Releases   conventionsReleasesJSON        `json:"releases"`   // information seule
}

// conventionsTagItemJSON est un nom de tag évalué (SV1).
type conventionsTagItemJSON struct {
	Name    string   `json:"name"`
	Codes   []string `json:"codes"`
	Conform bool     `json:"conform"`
	Final   bool     `json:"final"` // conforme, sans pré-version ni métadonnées de build : seul un tag final borne un intervalle
}

// conventionsMilestoneItemJSON est la présence d'un jalon (SV2).
type conventionsMilestoneItemJSON struct {
	Name   string   `json:"name"`
	Due    *string  `json:"due"`    // AAAA-MM-JJ ; null sans échéance
	Status string   `json:"status"` // present, nonconforming_name ou absent
	Tag    *string  `json:"tag"`    // tag qui désigne le jalon ; null si absent
	Others []string `json:"others"` // autres noms non conformes qui le désignent
	// Deadline est l'échéance comparée à la pose du tag de nom exact (phase 8.4) ; null sans échéance jugée.
	Deadline *conventionsDeadlineJSON `json:"deadline"`
}

// conventionsReleasesJSON est SV6 d'un dépôt (information, hors conformité).
type conventionsReleasesJSON struct {
	Known  bool                     `json:"known"`
	Reason *string                  `json:"reason"`
	Notes  []conventionsReleaseNote `json:"notes"`
}

// conventionsReleaseNote est une remarque SV6.
type conventionsReleaseNote struct {
	Code string `json:"code"` // milestone_without_release, release_without_tag, prerelease_mismatch
	Tag  string `json:"tag"`
}

// conventionsIntervalsJSON est CC1 par intervalle entre tags finals d'un dépôt.
type conventionsIntervalsJSON struct {
	Known        bool                      `json:"known"`
	Reason       *string                   `json:"reason"`   // no_clone, clone_unreadable, filtered, no_final_tag
	LastTag      *string                   `json:"last_tag"` // dernier tag final ; null sans tag final
	Items        []conventionsIntervalJSON `json:"items"`
	SinceLastTag conventionsTallyJSON      `json:"since_last_tag"`
}

// conventionsIntervalJSON est CC1 sur un intervalle from..to.
type conventionsIntervalJSON struct {
	From        *string              `json:"from"` // null : depuis le début de l'historique
	To          string               `json:"to"`
	Known       bool                 `json:"known"`
	Commits     conventionsTallyJSON `json:"commits"`
	BeforeSince int                  `json:"before_since"`
	Missing     int                  `json:"missing"` // commits absents de la base, non évalués
}

// conventionsTrendJSON est la tendance de CC3 d'un contributeur : deux taux.
type conventionsTrendJSON struct {
	Known        bool                 `json:"known"`
	Repositories int                  `json:"repositories"` // dépôts où la position a pu être lue
	Unmeasured   int                  `json:"unmeasured"`   // commits non placés (sans clone ni tag final)
	Before       conventionsTallyJSON `json:"before"`       // jusqu'au dernier tag final
	Since        conventionsTallyJSON `json:"since"`        // depuis
}

// conventionsAuthorPullsJSON compte les PR d'un contributeur.
type conventionsAuthorPullsJSON struct {
	Open   conventionsPRCountJSON `json:"open"`
	Merged conventionsPRCountJSON `json:"merged"`
}

// conventionsAuthorBranchesJSON compte les branches de tête des PR d'un contributeur.
type conventionsAuthorBranchesJSON struct {
	Checked int `json:"checked"`
	Conform int `json:"conform"`
	Review  int `json:"review"`
}

// conventionsMilestoneJSON est la revue d'un jalon (--milestone).
type conventionsMilestoneJSON struct {
	Name         string                         `json:"name"`
	Due          *string                        `json:"due"`
	Criteria     []string                       `json:"criteria"` // critères mesurés, dans leur ordre
	Repositories []conventionsMilestoneRepoJSON `json:"repositories"`
}

// conventionsMilestoneRepoJSON est la revue du jalon pour un dépôt (indicateurs de dépôt).
type conventionsMilestoneRepoJSON struct {
	Repository repoRefJSON                       `json:"repository"`
	Evaluable  bool                              `json:"evaluable"`
	Reason     *string                           `json:"reason"` // no_since si non évaluable
	Since      *string                           `json:"since"`
	Status     string                            `json:"status"` // SV2 : present, nonconforming_name, absent
	Tag        *string                           `json:"tag"`
	TagCodes   []string                          `json:"tag_codes"`   // SV1 du tag ; [] sans tag
	Interval   *conventionsMilestoneIntervalJSON `json:"interval"`    // null si non mesuré
	Branches   conventionsAuthorBranchesJSON     `json:"branches"`    // BR1, état courant
	Files      *conventionsMilestoneFilesJSON    `json:"files"`       // critères de fichiers au tag (8.3) ; null si non évaluable
	Deadline   *conventionsDeadlineJSON          `json:"deadline"`    // échéance de SV2 (8.4) ; null sans échéance jugée
	TagHistory *conventionsMilestoneHistoryJSON  `json:"tag_history"` // SV3 du tag (8.4) ; null s'il est absent ou jamais observé
	Arrivals   *conventionsArrivalsJSON          `json:"arrivals"`    // PR3 sur l'intervalle (8.5) ; null si non évaluable
	Criteria   []conventionsCriterionJSON        `json:"criteria"`
	Conform    int                               `json:"conform"`
	Unknown    int                               `json:"unknown"`
	Total      int                               `json:"total"`
}

// conventionsMilestoneIntervalJSON est l'intervalle des commits d'un jalon.
type conventionsMilestoneIntervalJSON struct {
	From        *string              `json:"from"`
	To          *string              `json:"to"`  // null si borné par l'échéance
	Due         *string              `json:"due"` // échéance qui borne l'intervalle (tag absent)
	Commits     conventionsTallyJSON `json:"commits"`
	BeforeSince int                  `json:"before_since"`
	Missing     int                  `json:"missing"`
}

// conventionsCriterionJSON est l'état d'un critère de la revue de jalon.
type conventionsCriterionJSON struct {
	ID     string  `json:"id"`
	Status string  `json:"status"` // conform, nonconform ou unknown
	Reason *string `json:"reason"`
}

// newConventionsTraitsJSON convertit CC5 et CC6.
func newConventionsTraitsJSON(t conventions.TraitsTally) conventionsTraitsJSON {
	out := conventionsTraitsJSON{Evaluated: t.Evaluated, Wording: map[string]int{}, Footer: t.Footer, IssueReference: t.Issue}
	for _, c := range conventions.AllWordingCodes {
		out.Wording[c] = t.Wording[c]
	}
	return out
}

// newConventionsLanguageJSON convertit CC7.
func newConventionsLanguageJSON(l conventions.LanguageSummary) conventionsLanguageJSON {
	return conventionsLanguageJSON{Expected: l.Expected, FR: l.FR, EN: l.EN, Unknown: l.Unknown, Other: l.Other, Mixed: l.Mixed}
}

// newConventionsBranchesJSON convertit BR1.
func newConventionsBranchesJSON(b conventions.BranchReport) conventionsBranchesJSON {
	out := conventionsBranchesJSON{Source: b.Source, Checked: b.Checked, Conform: b.Conform, Review: b.Review,
		Items: make([]conventionsBranchItemJSON, 0, len(b.Branches)), Reused: newConventionsReusedJSON(b)}
	for _, l := range b.Branches {
		item := conventionsBranchItemJSON{Name: l.Name, Codes: nonNil(l.Codes), Conform: l.Conform(), Review: l.Review(),
			InSource: l.InClone, PullRequests: append([]int{}, l.PRs...), Authors: make([]authorJSON, 0, len(l.AuthorIDs))}
		for i := range l.AuthorIDs {
			item.Authors = append(item.Authors, conventionsAuthor(l.AuthorNames[i], l.AuthorLogins[i]))
		}
		out.Items = append(out.Items, item)
	}
	return out
}

// newPRCountJSON convertit un compte de PR.
func newPRCountJSON(c conventions.PRCount) conventionsPRCountJSON {
	return conventionsPRCountJSON{Evaluated: c.Evaluated, Conform: c.Conform, Default: c.Default, BodyKnown: c.BodyKnown,
		BodyConform: c.BodyConform, MergeKnown: c.MergeKnown, Squash: c.Squash, MergeConform: c.MergeConform}
}

// newConventionsPullsJSON convertit PR1 et PR2, sans aucun titre.
func newConventionsPullsJSON(p conventions.PullReport) conventionsPullsJSON {
	out := conventionsPullsJSON{Known: p.Known, Reason: optionalString(p.Reason), Open: newPRCountJSON(p.Open),
		Merged: newPRCountJSON(p.Merged), Skipped: map[string]int{}, Items: make([]conventionsPullItemJSON, 0, len(p.Pulls)),
		Methods: map[string]int{}}
	for _, s := range conventions.AllPRSkips {
		out.Skipped[s] = p.Skipped[s]
	}
	for _, m := range conventions.AllMergeMethods {
		out.Methods[m] = p.Methods[m]
	}
	for _, l := range p.Pulls {
		out.Items = append(out.Items, conventionsPullItemJSON{Number: l.Number, Category: l.Category,
			Author: conventionsAuthor(l.AuthorName, l.AuthorLogin), HeadBranch: l.HeadBranch, Codes: nonNil(l.Codes),
			DefaultCodes: nonNil(l.Default), Conform: l.Conform, Body: newConventionsBodyJSON(l.Body),
			Merge: newConventionsMergeJSON(l)})
	}
	return out
}

// newConventionsTagsJSON convertit SV1, SV2 et SV6.
func newConventionsTagsJSON(t conventions.TagReport) conventionsTagsJSON {
	out := conventionsTagsJSON{Checked: t.Checked, Conform: t.Conform, Items: make([]conventionsTagItemJSON, 0, len(t.Tags)),
		Milestones: make([]conventionsMilestoneItemJSON, 0, len(t.Milestones)),
		Releases: conventionsReleasesJSON{Known: t.Releases.Known, Reason: optionalString(t.Releases.Reason),
			Notes: make([]conventionsReleaseNote, 0, len(t.Releases.Notes))}}
	for _, tag := range t.Tags {
		out.Items = append(out.Items, conventionsTagItemJSON{Name: tag.Name, Codes: nonNil(tag.Codes), Conform: tag.Conform(),
			Final: tag.Final()})
	}
	for _, m := range t.Milestones {
		out.Milestones = append(out.Milestones, newMilestoneItemJSON(m))
	}
	for _, n := range t.Releases.Notes {
		out.Releases.Notes = append(out.Releases.Notes, conventionsReleaseNote{Code: n.Code, Tag: n.Tag})
	}
	return out
}

// newMilestoneItemJSON convertit la présence d'un jalon.
func newMilestoneItemJSON(m conventions.MilestoneResult) conventionsMilestoneItemJSON {
	return conventionsMilestoneItemJSON{Name: m.Name, Due: optionalString(m.Due), Status: m.Status,
		Tag: optionalString(m.Tag), Others: nonNil(m.Others), Deadline: newConventionsDeadlineJSON(m.Deadline)}
}

// newConventionsIntervalsJSON convertit CC1 par intervalle.
func newConventionsIntervalsJSON(iv conventions.IntervalReport) conventionsIntervalsJSON {
	out := conventionsIntervalsJSON{Known: iv.Known, Reason: optionalString(iv.Reason), LastTag: optionalString(iv.LastTag),
		Items: make([]conventionsIntervalJSON, 0, len(iv.Intervals)), SinceLastTag: newConventionsTallyJSON(iv.SinceLastTag)}
	for _, l := range iv.Intervals {
		out.Items = append(out.Items, conventionsIntervalJSON{From: optionalString(l.From), To: l.To, Known: l.Known,
			Commits: newConventionsTallyJSON(l.Tally), BeforeSince: l.BeforeSince, Missing: l.Missing})
	}
	return out
}

// newConventionsTrendJSON convertit la tendance de CC3.
func newConventionsTrendJSON(t conventions.Trend) conventionsTrendJSON {
	return conventionsTrendJSON{Known: t.Known(), Repositories: t.Repos, Unmeasured: t.Unmeasured,
		Before: newConventionsTallyJSON(t.Before), Since: newConventionsTallyJSON(t.Since)}
}

// newConventionsMilestoneJSON convertit la revue d'un jalon.
func newConventionsMilestoneJSON(rv conventions.MilestoneReview) *conventionsMilestoneJSON {
	out := &conventionsMilestoneJSON{Name: rv.Milestone.Name, Due: optionalString(rv.Milestone.Due),
		Criteria: append([]string{}, conventions.AllCriteria...), Repositories: make([]conventionsMilestoneRepoJSON, 0, len(rv.Repos))}
	for _, r := range rv.Repos {
		item := conventionsMilestoneRepoJSON{
			Repository: repoRefJSON{ID: r.Repo.ID, FullName: r.Repo.FullName(), DisplayName: r.Repo.DisplayName()},
			Evaluable:  r.Evaluable, Reason: optionalString(r.Reason), Since: optionalString(r.SinceDate),
			Status: r.Milestone.Status, Tag: optionalString(r.Milestone.Tag), TagCodes: []string{},
			Branches: conventionsAuthorBranchesJSON{Checked: r.Branches.Checked, Conform: r.Branches.Conform, Review: r.Branches.Review},
			Criteria: make([]conventionsCriterionJSON, 0, len(r.Criteria)), Conform: r.Conform, Unknown: r.Unknown,
			Total: len(r.Criteria), Files: newConventionsMilestoneFilesJSON(r.Files),
			Deadline: newConventionsDeadlineJSON(r.Milestone.Deadline), TagHistory: newConventionsMilestoneHistoryJSON(r.History),
		}
		if r.Tag != nil {
			item.TagCodes = nonNil(r.Tag.Codes)
		}
		if r.Arrivals != nil {
			a := newConventionsArrivalsJSON(*r.Arrivals)
			item.Arrivals = &a
		}
		if iv := r.Interval; iv != nil {
			item.Interval = &conventionsMilestoneIntervalJSON{From: optionalString(iv.From), To: optionalString(iv.To),
				Due: optionalString(iv.Due), Commits: newConventionsTallyJSON(iv.Tally), BeforeSince: iv.BeforeSince,
				Missing: iv.Missing}
		}
		for _, c := range r.Criteria {
			item.Criteria = append(item.Criteria, conventionsCriterionJSON{ID: c.ID, Status: c.Status, Reason: optionalString(c.Reason)})
		}
		out.Repositories = append(out.Repositories, item)
	}
	return out
}

// nonNil renvoie s, ou une liste vide non nil (« [] » en JSON plutôt que null).
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
