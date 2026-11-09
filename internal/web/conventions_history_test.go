package web

// Conventions, phase 8.4: history of the tags and of the default branch, due dates, versions and freshness
// of the CHANGELOG on the repository page and in /conventions (overview column, SV3 in the milestone
// review); labels in both languages; never an attribution nor a commit message.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
	"github.com/studoo-app/watchoo/internal/model"
	"github.com/studoo-app/watchoo/internal/store"
)

// seedHistory records, for repository repoID, two complete reads of the tags (v1.0 moved to another
// commit in between) and a rewrite of the default branch.
func (e *testEnv) seedHistory(repoID int64, tagSHA string) {
	e.t.Helper()
	ctx := e.t.Context()
	other := strings.Repeat("d", 40)
	first, second := e.now.Add(-48*time.Hour), e.now.Add(-25*time.Minute)
	if err := e.store.ObserveTags(ctx, repoID, []model.Tag{{Name: "v1.0", SHA: other}, {Name: "v0.9", SHA: strings.Repeat("e", 40)}},
		first); err != nil {
		e.t.Fatal(err)
	}
	if err := e.store.ObserveTags(ctx, repoID, []model.Tag{{Name: "v1.0", SHA: tagSHA}, {Name: "v0.9", SHA: strings.Repeat("e", 40)}},
		second); err != nil {
		e.t.Fatal(err)
	}
	if err := e.store.RecordBranchRewrite(ctx, model.BranchRewrite{RepoID: repoID, Branch: "main", OldSHA: other, NewSHA: tagSHA,
		ObservedAt: second}); err != nil {
		e.t.Fatal(err)
	}
}

func TestRepoConventionsHistory(t *testing.T) {
	e, ids := conventionsPullsEnv(t)
	head := e.commitWrite(6, store.Identity{}, 0, "", "main").SHA
	e.seedHistory(ids.Alice, head)
	w := e.get(repoPath(ids.Alice))
	assertStatus(t, w, http.StatusOK)
	section := conventionsSection(t, w.Body.String())
	for _, want := range []string{
		`id="repo-conventions-history"`, "History of the tags and of the default branch, versions",
		// BR4: one rewrite, short SHAs, nobody named.
		`id="repo-conventions-rewrites"`, "1 rewrite(s) of the default branch observed, the last on",
		shortSHA(strings.Repeat("d", 40)), shortSHA(head),
		// SV3: v1.0 rewritten, v0.9 without rewrite observed.
		`id="repo-conventions-tag-history"`, "1 without rewrite observed since", "1 rewritten, 0 deleted",
		"v1.0 · rewritten (other commit)", "v0.9 · no rewrite observed",
		// SV4: no pre-release; SV5, SV7 and CL10: no local clone.
		`id="repo-conventions-prereleases"`, "no pre-release",
		`id="repo-conventions-bumps"`, `id="repo-conventions-precedence"`, `id="repo-conventions-freshness"`,
		"unknown (no local clone)", "never attributed to anyone",
	} {
		if !strings.Contains(section, want) {
			t.Errorf("history section without %q", want)
		}
	}
	assertNoConventionsLeak(t, "repository page (history)", section)
	if strings.Contains(section, "pushed") || strings.Contains(section, "never rewritten.") {
		t.Error("history attributes or overstates")
	}

	// /conventions: history column; SV3 in the milestone review (v1.0 designates v1.0.0 and was rewritten).
	s, ids2 := conventionsPullsEnv(t, func(e *testEnv) { e.cfg.Conventions.Since = "2000-01-01" })
	s.seedHistory(ids2.Alice, s.commitWrite(6, store.Identity{}, 0, "", "main").SHA)
	w = s.get("/conventions?milestone=v1.0.0")
	assertStatus(t, w, http.StatusOK)
	assertContains(t, w, "History and versions", "rewrites 1", "tags rewritten or deleted 1",
		"SV3 <span", "milestone tag without rewrite observed", "Tag: rewritten (other commit)", "/12 criteria conforming")
	assertNoConventionsLeak(t, "/conventions (history)", w.Body.String())

	fr, ids3 := conventionsPullsEnv(t, withLang("fr"))
	fr.seedHistory(ids3.Alice, fr.commitWrite(6, store.Identity{}, 0, "", "main").SHA)
	section = conventionsSection(t, fr.get(repoPath(ids3.Alice)).Body.String())
	for _, want := range []string{"Historique des tags et de la branche par défaut", "1 réécriture(s) de la branche par défaut observée(s)",
		"réécrit (autre commit)", "aucune réécriture observée", "jamais attribuée à une personne", "Réécritures de la branche par défaut :"} {
		if !strings.Contains(section, want) {
			t.Errorf("French history section without %q", want)
		}
	}
}

func TestConventionsDeadlineLabels(t *testing.T) {
	e := newTestEnv(t)
	tr := i18n.For(i18n.English)
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if st := deadlineStatus(tr, conventions.MilestoneDeadline{}); st != nil {
		t.Errorf("no due date: %+v", st)
	}
	late := conventions.MilestoneDeadline{Status: conventions.DeadlineLate, FirstSeenAt: at}
	if st := deadlineStatus(tr, late); st == nil || st.Class != "badge-error" || st.Label != "late" {
		t.Errorf("late: %+v", st)
	}
	if got := e.server.deadlineDetail(tr, late); !strings.HasPrefix(got, "first seen on ") {
		t.Errorf("late detail: %q", got)
	}
	undet := conventions.MilestoneDeadline{Status: conventions.DeadlineUndetermined, FirstSeenAt: at}
	if got := e.server.deadlineDetail(tr, undet); !strings.HasPrefix(got, "set between an unknown date and ") {
		t.Errorf("undetermined detail: %q", got)
	}
	if st := deadlineStatus(tr, conventions.MilestoneDeadline{Status: conventions.DeadlineOnTime}); st == nil || st.Class != "badge-success" {
		t.Errorf("on time: %+v", st)
	}
}

// Every code, status and reason of phase 8.4 has its label in both languages (keys built at run time).
func TestConventionsLabelsPhase84(t *testing.T) {
	families := map[string][]string{
		"conv.history.status.":       conventions.AllTagHistoryStatuses,
		"conv.deadline.":             conventions.AllDeadlineStatuses,
		"conv.prerelease_code.":      conventions.AllPrereleaseCodes,
		"conv.precedence_code.":      conventions.AllPrecedenceCodes,
		"conv.bump.status.":          conventions.AllBumpStatuses,
		"conv.indicator.":            {"BR4", "SV3", "SV4", "SV5", "SV7", "CL10"},
		"web.conv.criterion.":        {conventions.CriterionSV3},
		"web.conv.reason.":           {conventions.UnknownNotObserved, conventions.UnknownDeadline, conventions.UnknownNoBase, conventions.UnknownLowConform},
		"cli.conv.reason.":           {conventions.UnknownNotObserved, conventions.UnknownDeadline, conventions.UnknownNoBase, conventions.UnknownLowConform},
		"cli.conv.criterion_status.": {conventions.CriterionConform},
	}
	en, fr := i18n.For(i18n.English), i18n.For(i18n.French)
	for prefix, codes := range families {
		for _, c := range codes {
			key := prefix + c
			if en.T(key) == key || fr.T(key) == key {
				t.Errorf("no label for %s", key)
			} else if en.T(key) == fr.T(key) {
				t.Errorf("%s: same label in English and French", key)
			}
		}
	}
	for _, b := range []string{conventions.BumpPatch, conventions.BumpMinor, conventions.BumpMajor} {
		if key := "conv.bump." + b; en.T(key) == key {
			t.Errorf("no label for %s", key)
		}
	}
}
