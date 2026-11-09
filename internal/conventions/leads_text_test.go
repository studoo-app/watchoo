package conventions

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// keyTranslate renders a key with its arguments, to check which keys and values LeadDetail uses.
func keyTranslate(key string, args ...any) string {
	if len(args) == 0 {
		return key
	}
	return key + fmt.Sprintf("%v", args)
}

func TestLeadDetail(t *testing.T) {
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	header := map[string]int{CountComplete: 1, CountDocuments: 3}
	for i, f := range AllHeaderFields {
		header[HeaderCountKey(f)] = i
	}
	tests := []struct {
		lead LeadCheck
		want []string
	}{
		{LeadCheck{ID: CriterionCL11, Status: LeadReview, Counts: map[string]int{CountTitles: 3, CountLinked: 1}},
			[]string{"conv.lead_detail.CL11[1 3]"}},
		{LeadCheck{ID: CriterionCL12, Status: LeadNotApplicable, Reason: UnknownTooFew, Counts: map[string]int{CountBullets: 4,
			CountSubjectPercent: 75, CountSubjects: 9, CountShaMergePercent: 25}}, []string{"conv.lead_detail.CL12[4 75 9 25]"}},
		{LeadCheck{ID: CriterionRD5, Status: LeadReview, Counts: map[string]int{CountCommandBlocks: 2, CountEnvAnnounced: 1}},
			[]string{"conv.lead_detail.RD5[2]", "conv.lead_detail.env_example_absent"}},
		{LeadCheck{ID: CriterionRD5, Status: LeadNone, Counts: map[string]int{CountCommandBlocks: 1, CountEnvAnnounced: 1,
			CountEnvExample: 1}}, []string{"conv.lead_detail.env_example_present"}},
		{LeadCheck{ID: CriterionRD5, Status: LeadNone, Counts: map[string]int{CountCommandBlocks: 1}},
			[]string{"conv.lead_detail.RD5[1]"}},
		{LeadCheck{ID: CriterionDOC6, Status: LeadReview, Counts: header},
			[]string{"conv.lead_detail.DOC6[1 3]", "conv.lead_detail.DOC6_fields[conv.lead_header.title 0, conv.lead_header.objective 1, conv.lead_header.scope 2, conv.lead_header.author 3, conv.lead_header.date 4, conv.lead_header.version 5]"}},
		{LeadCheck{ID: CriterionDOC7, Status: LeadReview, Counts: map[string]int{CountImages: 2, CountBadges: 1, CountMermaid: 3}},
			[]string{"conv.lead_detail.DOC7[2 1 3]", "conv.lead_detail.media_absent"}},
		{LeadCheck{ID: CriterionDOC9, Status: LeadNone, Counts: map[string]int{CountCommitsSince: 4, CountDaysBehind: 2},
			Dates: map[string]time.Time{DateDocs: at, DateCode: at.Add(48 * time.Hour), DateIndex: {}}},
			[]string{"conv.lead_detail.DOC9[4 2]", "conv.lead_date.docs[2026-10-01]", "conv.lead_date.code[2026-10-03]"}},
		{LeadCheck{ID: CriterionDOC10, Status: LeadReview, Counts: map[string]int{CountRows: 60, CountCompared: 50,
			CountUnresolved: 2, CountOverLimit: 8}},
			[]string{"conv.lead_detail.DOC10[60 50 2]", "conv.lead_detail.DOC10_over[8 50]"}},
		{LeadCheck{ID: CriterionTL4, Status: LeadNone, Counts: map[string]int{CountWorkflows: 2, CountCIMask: 1}},
			[]string{"conv.lead_detail.TL4[2 commitlint]"}},
		{LeadCheck{ID: CriterionTL4, Status: LeadReview, Counts: map[string]int{CountWorkflows: 1}},
			[]string{"conv.lead_detail.TL4_none[1]"}},
		{LeadCheck{ID: CriterionTL5, Status: LeadReview, Counts: map[string]int{CountBefore: 10, CountBeforeConform: 5,
			CountBeforePercent: 50, CountAfter: 12, CountAfterConform: 6, CountAfterPercent: 50, CountSources: 1,
			CountUndatedSources: 1}, Dates: map[string]time.Time{DateIntroduced: at}},
			[]string{"conv.lead_detail.TL5[5 10 50 6 12 50]", "conv.lead_detail.TL5_sources[1 1]",
				"conv.lead_date.introduced[2026-10-01]"}},
	}
	for _, tt := range tests {
		got := LeadDetail(keyTranslate, day, tt.lead)
		for _, w := range tt.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: detail %q without %q", tt.lead.ID, got, w)
			}
		}
	}
	// The index date (zero) is not written; the parts are separated.
	got := LeadDetail(keyTranslate, day, tests[7].lead)
	if strings.Contains(got, "conv.lead_date.index") || !strings.Contains(got, "conv.lead_sep") {
		t.Errorf("DOC9 detail %q", got)
	}
	// An unknown lead, or a lead without counter, has no detail.
	for _, l := range []LeadCheck{unknownLead(CriterionDOC9, UnknownDetailOnly),
		{ID: CriterionTL5, Status: LeadNotApplicable, Reason: UnknownNoTooling, Counts: map[string]int{}}} {
		if got := LeadDetail(keyTranslate, day, l); got != "" {
			t.Errorf("%s %s: detail %q, want none", l.ID, l.Status, got)
		}
	}
}
