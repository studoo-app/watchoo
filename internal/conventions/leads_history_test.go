package conventions

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func TestCheckDOC9(t *testing.T) {
	at := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	for _, tc := range []struct {
		name   string
		s      DocStamps
		status string
		days   int
	}{
		{"fresh", DocStamps{Docs: at(10), LastDoc: at(10), Code: at(12), CommitsSince: 3}, LeadNone, 2},
		{"docs newer than code", DocStamps{Docs: at(20), LastDoc: at(20), Code: at(12)}, LeadNone, 0},
		{"many commits", DocStamps{LastDoc: at(1), Code: at(5), CommitsSince: DOC9MinCommits}, LeadReview, 4},
		{"old documentation", DocStamps{LastDoc: at(1), Code: at(31), CommitsSince: 2}, LeadReview, 30},
		{"just under", DocStamps{LastDoc: at(1), Code: at(30).Add(23 * time.Hour), CommitsSince: DOC9MinCommits - 1},
			LeadNone, 29},
	} {
		l := CheckDOC9(tc.s)
		if l.ID != CriterionDOC9 || l.Status != tc.status || l.Counts[CountDaysBehind] != tc.days ||
			l.Counts[CountCommitsSince] != tc.s.CommitsSince {
			t.Errorf("%s: %+v", tc.name, l)
		}
		if tc.status == LeadReview && !slices.Equal(l.Codes, []string{LeadDocsBehind}) {
			t.Errorf("%s: codes %v", tc.name, l.Codes)
		}
	}
	l := CheckDOC9(DocStamps{Docs: at(1), Index: at(2), LastDoc: at(2), Code: at(3)})
	if len(l.Dates) != 3 || !l.Dates[DateDocs].Equal(at(1)) || !l.Dates[DateIndex].Equal(at(2)) ||
		!l.Dates[DateCode].Equal(at(3)) {
		t.Errorf("dates: %+v", l.Dates)
	}
	if l := CheckDOC9(DocStamps{Code: at(3)}); l.Status != CriterionUnknown || l.Reason != UnknownNoFile {
		t.Errorf("no documentation commit: %+v", l)
	}
}

func TestDocPathsOf(t *testing.T) {
	files := goodDocs()
	files["CHANGELOG.md"] = testChangelog
	d, reason := docPathsOf(treeOf(files), DefaultFileRules())
	if reason != "" || d != (docPaths{Docs: "documentation", Index: "documentation/INDEX.md", Changelog: "CHANGELOG.md",
		Readme: "README.md"}) {
		t.Errorf("paths = %+v, %q", d, reason)
	}
	if got := d.all(); !slices.Equal(got, []string{"documentation", "documentation/INDEX.md", "CHANGELOG.md", "README.md"}) {
		t.Errorf("all = %v", got)
	}
	d, _ = docPathsOf(treeOf(map[string]string{"main.go": "x"}), DefaultFileRules())
	if len(d.all()) != 0 {
		t.Errorf("no documentation: %+v", d)
	}
	if _, reason := docPathsOf(FileTree{Truncated: true, Paths: []string{"a"}}, DefaultFileRules()); reason != UnknownTreeTruncated {
		t.Errorf("truncated: %q", reason)
	}
}

func TestDeclaredDate(t *testing.T) {
	for cell, want := range map[string]string{
		"2026-10-01":           "2026-10-01",
		"**2026/1/5**":         "2026-01-05",
		"le 05/01/2026":        "2026-01-05",
		"5.1.2026":             "2026-01-05",
		"01-10-2026":           "2026-10-01",
		"2026-02-30":           "",
		"31/02/2026":           "",
		"octobre 2026":         "",
		"":                     "",
		"v1.2.3":               "",
		"2026-10-01 (relu)":    "2026-10-01",
		"[2026-10-01](x.md)":   "2026-10-01",
		"2026-13-01":           "",
		"1/2/26":               "",
		"2026-10-01T10:00:00Z": "2026-10-01",
	} {
		got, ok := declaredDate(cell)
		if (want == "") == ok || (ok && got.Format("2006-01-02") != want) {
			t.Errorf("%q: %v %v; want %q", cell, got, ok, want)
		}
	}
}

func TestDOC10(t *testing.T) {
	index := "# Index\n\n| Document | Thème | Date |\n|---|---|---|\n" +
		"| [Vision](01-cadrage/vision.md) | Cadrage | 2026-10-05 |\n" + // newer than its commit (Oct 1)
		"| [Archi](02-conception/architecture.md) | Conception | 01/08/2026 |\n" + // older by > 30 days (Oct 2)
		"| [Guide](03-guide/guide.md) | Guide | 2026-10-03 |\n" + // same day in Paris
		"| [Plan](04-plan/plan.md) | Plan | bientôt |\n" + // unreadable
		"| [Absent](absent.md) | — | 2026-10-01 |\n" + // missing target
		"| [Vision](01-cadrage/vision.md) | Doublon | 2026-10-01 |\n" + // compared once
		"| [Guide](03-guide/guide.md) | Sans date |  |\n" + // empty date cell: not counted (DOC2)
		"| | | |\n" // blank row, ignored
	files := goodDocs()
	files["documentation/INDEX.md"] = index
	files["documentation/03-guide/guide.md"] = "# Guide\n"
	files["documentation/04-plan/plan.md"] = "# Plan\n"
	rows, l, reason := indexDates(treeOf(files), DefaultFileRules(), "promo/projet")
	if reason != "" || len(rows) != 3 {
		t.Fatalf("rows = %+v, %q", rows, reason)
	}
	if rows[0].Path != "documentation/01-cadrage/vision.md" || rows[1].Declared.Format("2006-01-02") != "2026-08-01" {
		t.Errorf("rows = %+v", rows)
	}
	paris, _ := time.LoadLocation("Europe/Paris")
	last := map[string]time.Time{
		"documentation/01-cadrage/vision.md":          time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC),
		"documentation/02-conception/architecture.md": time.Date(2026, 10, 2, 9, 0, 0, 0, time.UTC),
		// October 2 at 23:30 UTC is October 3 in Paris.
		"documentation/03-guide/guide.md": time.Date(2026, 10, 2, 23, 30, 0, 0, time.UTC),
	}
	got := checkDOC10(rows, l, last, paris)
	want := map[string]int{CountRows: 6, CountCompared: 3, CountUnresolved: 1, CountOverLimit: 0,
		LeadDeclaredNewer: 1, LeadDeclaredOlder: 1, LeadDateUnreadable: 1}
	for k, v := range want {
		if got.Counts[k] != v {
			t.Errorf("count %s = %d, want %d (%+v)", k, got.Counts[k], v, got.Counts)
		}
	}
	if got.Status != LeadReview || !slices.Equal(got.Codes, []string{LeadDeclaredNewer, LeadDeclaredOlder, LeadDateUnreadable}) {
		t.Errorf("DOC10 = %+v", got)
	}
	// A document never committed is counted unresolved.
	rows, l, _ = indexDates(treeOf(files), DefaultFileRules(), "promo/projet")
	if got := checkDOC10(rows, l, nil, nil); got.Counts[CountUnresolved] != 4 || got.Counts[CountCompared] != 0 {
		t.Errorf("never committed: %+v", got.Counts)
	}

	// At most DOC10MaxDocuments documents compared.
	var b strings.Builder
	b.WriteString("| Document | Date |\n|---|---|\n")
	big := map[string]string{}
	for i := range DOC10MaxDocuments + 5 {
		p := "d" + strings.Repeat("x", i) + ".md"
		big["documentation/"+p] = "# D\n"
		b.WriteString("| [D](" + p + ") | 2026-10-01 |\n")
	}
	big["documentation/INDEX.md"] = b.String()
	rows, l, _ = indexDates(treeOf(big), DefaultFileRules(), "promo/projet")
	if len(rows) != DOC10MaxDocuments || l.counts[CountOverLimit] != 5 {
		t.Errorf("limit: %d rows, %+v", len(rows), l.counts)
	}

	// No date column: n/a; no INDEX: unknown.
	files["documentation/INDEX.md"] = goodIndex[:strings.Index(goodIndex, "\n")] + "\n\n| Document | Thème |\n|---|---|\n"
	if _, _, reason := indexDates(treeOf(files), DefaultFileRules(), "promo/projet"); reason != UnknownNoDateColumn {
		t.Errorf("no date column: %q", reason)
	}
	delete(files, "documentation/INDEX.md")
	if _, _, reason := indexDates(treeOf(files), DefaultFileRules(), "promo/projet"); reason != UnknownNoFile {
		t.Errorf("no INDEX: %q", reason)
	}
}

func TestToolingSources(t *testing.T) {
	files := map[string]string{
		"commitlint.config.js":          "module.exports = {}",
		"web/.husky/commit-msg":         "npx commitlint --edit",
		"lefthook.yml":                  "pre-commit:\n  x\n",     // no commit-msg entry
		".pre-commit-config.yaml":       "stages: [commit-msg]\n", // dated
		"cog.toml":                      "",
		"pyproject.toml":                "[tool.commitizen]\n", // undated
		"package.json":                  `{"devDependencies": {"@commitlint/cli": "19"}}`,
		".github/workflows/ci.yml":      "steps:\n  - uses: wagoid/commitlint-github-action@v6\n",
		".github/workflows/build.yml":   "steps:\n  - run: go test\n",
		".github/workflows/old.yml":     "# uses: amannn/action-semantic-pull-request\n",
		"a/b/c/commitlint.config.js":    "too deep",
		"node_modules/x/package.json":   `{"commitlint": {}}`,
		"src/main.go":                   "package main",
		"docs/pull_request_template.md": "# PR",
	}
	got, undated, reason := toolingSources(treeOf(files))
	want := []string{".github/workflows/ci.yml", ".pre-commit-config.yaml", "cog.toml", "commitlint.config.js",
		"web/.husky/commit-msg"}
	if reason != "" || !slices.Equal(got, want) || undated != 2 {
		t.Errorf("sources = %v, %d, %q; want %v, 2", got, undated, reason, want)
	}
	if got, undated, _ := toolingSources(treeOf(map[string]string{"main.go": "x"})); len(got) != 0 || undated != 0 {
		t.Errorf("no tooling: %v %d", got, undated)
	}
}

func TestCheckTL5(t *testing.T) {
	intro := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	commits := func(before, beforeOK, after, afterOK int) []DatedCommit {
		var out []DatedCommit
		for i := range before {
			out = append(out, DatedCommit{At: intro.Add(-time.Duration(i+1) * time.Hour), Conform: i < beforeOK})
		}
		for i := range after {
			out = append(out, DatedCommit{At: intro.Add(time.Duration(i) * time.Hour), Conform: i < afterOK})
		}
		return out
	}
	l := CheckTL5(intro, 2, 1, commits(10, 3, 12, 9))
	if l.Status != LeadNone || l.Counts[CountBefore] != 10 || l.Counts[CountBeforeConform] != 3 || l.Counts[CountAfter] != 12 ||
		l.Counts[CountAfterConform] != 9 || l.Counts[CountBeforePercent] != 30 || l.Counts[CountAfterPercent] != 75 ||
		l.Counts[CountSources] != 2 || l.Counts[CountUndatedSources] != 1 || !l.Dates[DateIntroduced].Equal(intro) {
		t.Errorf("gain: %+v", l)
	}
	// Same rate after: to review; the commit at the introduction instant counts after.
	if l := CheckTL5(intro, 1, 0, commits(10, 5, 10, 5)); l.Status != LeadReview || !slices.Equal(l.Codes, []string{LeadNoGain}) {
		t.Errorf("no gain: %+v", l)
	}
	if l := CheckTL5(intro, 1, 0, commits(9, 0, 30, 30)); l.Status != LeadNotApplicable || l.Reason != UnknownTooFew ||
		l.Counts[CountBefore] != 9 || len(l.Codes) != 0 {
		t.Errorf("too few: %+v", l)
	}
	if l := CheckTL5(time.Time{}, 0, 0, nil); l.Status != LeadNotApplicable || l.Reason != UnknownNoTooling {
		t.Errorf("no tooling: %+v", l)
	}
	if l := CheckTL5(time.Time{}, 0, 2, nil); l.Status != CriterionUnknown || l.Reason != UnknownIntroductionUnknown ||
		l.Counts[CountUndatedSources] != 2 {
		t.Errorf("undated: %+v", l)
	}
}

// leadClones adds LeadHistoryReader to fileClones: last commit by pathspec, count, first addition.
type leadClones struct {
	*fileClones
	last    map[string]time.Time // strings.Join(include, ",") + "!" + strings.Join(exclude, ",") → date
	since   int                  // CountCommits
	first   map[string]time.Time // file → first addition
	err     error
	cancel  context.CancelFunc
	queries []string
}

func leadKey(include, exclude []string) string {
	return strings.Join(include, ",") + "!" + strings.Join(exclude, ",")
}

func (c *leadClones) LastCommit(_ context.Context, _, _, head string, include, exclude []string) (string, time.Time,
	bool, error) {
	c.queries = append(c.queries, "last:"+head+":"+leadKey(include, exclude))
	if c.cancel != nil {
		c.cancel()
	}
	if c.err != nil {
		return "", time.Time{}, false, c.err
	}
	at, ok := c.last[leadKey(include, exclude)]
	if !ok {
		return "", time.Time{}, false, nil
	}
	return "sha-" + at.Format("0102"), at, true, nil
}

func (c *leadClones) CountCommits(_ context.Context, _, _, head, since string, include, exclude []string) (int, error) {
	c.queries = append(c.queries, "count:"+since+":"+leadKey(include, exclude))
	return c.since, c.err
}

func (c *leadClones) FirstAdd(_ context.Context, _, _, head, file string) (string, time.Time, bool, error) {
	c.queries = append(c.queries, "first:"+file)
	at, ok := c.first[file]
	return "first", at, ok, c.err
}

func leadFixture(t *testing.T) (*fakeReader, *leadClones, BuildOptions) {
	t.Helper()
	r, cl, opts := fileFixture(t)
	head := cl.trees["h1"]
	head["commitlint.config.js"] = "module.exports = {extends: ['@commitlint/config-conventional']}"
	head[".github/workflows/ci.yml"] = "steps:\n  - uses: wagoid/commitlint-github-action@v6\n"
	oct := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	docs := []string{"documentation", "documentation/INDEX.md", "CHANGELOG.md", "README.md"}
	lc := &leadClones{fileClones: cl, since: 25,
		last: map[string]time.Time{
			leadKey([]string{"documentation"}, nil):                               oct(2),
			leadKey([]string{"documentation/INDEX.md"}, nil):                      oct(2),
			leadKey([]string{"CHANGELOG.md"}, nil):                                oct(3),
			leadKey(docs, nil):                                                    oct(3),
			leadKey(nil, docs):                                                    oct(20),
			leadKey([]string{"documentation/01-cadrage/vision.md"}, nil):          oct(1),
			leadKey([]string{"documentation/02-conception/architecture.md"}, nil): oct(2),
		},
		first: map[string]time.Time{"commitlint.config.js": oct(5), ".github/workflows/ci.yml": oct(4)},
	}
	r.files = map[int64][]model.RepositoryConventions{1: {cacheRow(t, r, cl, FileTarget{Kind: model.ConventionRevHead,
		SHA: "h1"}, opts)}}
	return r, lc, opts
}

func leadStatuses(r LeadReport) map[string]string {
	out := map[string]string{}
	for _, l := range r.Leads {
		out[l.ID] = l.Status + ":" + l.Reason
	}
	return out
}

func TestBuildLeads(t *testing.T) {
	r, cl, opts := leadFixture(t)
	opts.FileDetail = true

	// Overview: tree leads from the cache, history leads only in the detail view; no history read.
	rep, err := Build(t.Context(), r, cl, Filter{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	leads := rep.Repos[0].Leads
	ids := make([]string, 0, len(leads.Leads))
	for _, l := range leads.Leads {
		ids = append(ids, l.ID)
	}
	if !slices.Equal(ids, LeadCriteria) {
		t.Fatalf("ids = %v", ids)
	}
	st := leadStatuses(leads)
	for _, id := range HistoryLeadCriteria {
		if st[id] != "unknown:detail_only" {
			t.Errorf("%s overview: %s", id, st[id])
		}
	}
	if st[CriterionTL4] != "none:" || st[CriterionCL11] == "" || strings.HasPrefix(st[CriterionCL11], "unknown") {
		t.Errorf("tree leads: %v", st)
	}
	if len(cl.queries) != 0 {
		t.Errorf("history read in the overview: %v", cl.queries)
	}

	// Detail of the repository: DOC9, DOC10 and TL5 computed; the tree read once (kept from buildFiles).
	cl.fakeFiles.asked = nil
	rep, err = Build(t.Context(), r, cl, Filter{RepoID: 1}, opts)
	if err != nil {
		t.Fatal(err)
	}
	leads = rep.Repos[0].Leads
	doc9, _ := leads.Find(CriterionDOC9)
	if doc9.Status != LeadReview || doc9.Counts[CountCommitsSince] != 25 || doc9.Counts[CountDaysBehind] != 17 ||
		len(doc9.Dates) != 4 {
		t.Errorf("DOC9 = %+v", doc9)
	}
	doc10, _ := leads.Find(CriterionDOC10)
	if doc10.Status != LeadNone || doc10.Counts[CountCompared] != 2 || doc10.Counts[CountRows] != 2 {
		t.Errorf("DOC10 = %+v", doc10)
	}
	tl5, _ := leads.Find(CriterionTL5)
	if tl5.Counts[CountSources] != 2 || !tl5.Dates[DateIntroduced].Equal(time.Date(2026, 10, 4, 9, 0, 0, 0, time.UTC)) ||
		tl5.Status != LeadNotApplicable || tl5.Reason != UnknownTooFew {
		t.Errorf("TL5 = %+v", tl5)
	}
	seen := map[string]int{}
	for _, p := range cl.fakeFiles.asked {
		seen[p]++
		if seen[p] > 1 {
			t.Errorf("blob %s read twice", p)
		}
	}
	if leads.ToReview() < 1 {
		t.Errorf("to review = %d", leads.ToReview())
	}
	// Leads never enter the grids.
	for _, fam := range rep.Repos[0].Files.Results.Families() {
		for _, c := range fam.Checks {
			if slices.Contains(LeadCriteria, c.ID) {
				t.Errorf("lead %s in the %s grid", c.ID, fam.Name)
			}
		}
	}

	// Clone unreadable: history leads unknown; canceled context: error.
	cl.err = errors.New("boom")
	rep, err = Build(t.Context(), r, cl, Filter{RepoID: 1}, opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range HistoryLeadCriteria {
		if s := leadStatuses(rep.Repos[0].Leads)[id]; s != "unknown:clone_unreadable" {
			t.Errorf("%s unreadable: %s", id, s)
		}
	}
	cl.err = nil
	ctx, cancel := context.WithCancel(t.Context())
	cl.cancel = cancel
	if _, err := Build(ctx, r, cl, Filter{RepoID: 1}, opts); !errors.Is(err, context.Canceled) {
		t.Errorf("canceled: %v", err)
	}
	cl.cancel = nil

	// A clone without the lead reader, or no clone: no_clone; a contributor filter: filtered.
	rep, _ = Build(t.Context(), r, cl.fileClones, Filter{RepoID: 1}, opts)
	if s := leadStatuses(rep.Repos[0].Leads)[CriterionDOC9]; s != "unknown:no_clone" {
		t.Errorf("no lead reader: %s", s)
	}
	// Without a clone, the tree leads stay those of the cache, like the file criteria.
	rep, _ = Build(t.Context(), r, nil, Filter{RepoID: 1}, opts)
	for _, id := range HistoryLeadCriteria {
		if s := leadStatuses(rep.Repos[0].Leads)[id]; s != "unknown:no_clone" {
			t.Errorf("%s without a clone: %s", id, s)
		}
	}
	files := r.files
	r.files = nil
	rep, _ = Build(t.Context(), r, nil, Filter{RepoID: 1}, opts)
	for id, s := range leadStatuses(rep.Repos[0].Leads) {
		if s != "unknown:no_clone" {
			t.Errorf("%s without a clone nor cache: %s", id, s)
		}
	}
	r.files = files
	rep, _ = Build(t.Context(), r, cl, Filter{RepoID: 1, AuthorIDs: []int64{1}}, opts)
	for id, s := range leadStatuses(rep.Repos[0].Leads) {
		if s != "unknown:filtered" {
			t.Errorf("%s filtered: %s", id, s)
		}
	}

	// Head of the default branch unknown in the database: the head measured by the sync (the cached tree)
	// stands for it; without a cached tree, nothing computed yet.
	r.branches[1][0].HeadSHA = ""
	rep, _ = Build(t.Context(), r, cl, Filter{RepoID: 1}, opts)
	st = leadStatuses(rep.Repos[0].Leads)
	if st[CriterionTL5] != "not_applicable:too_few" || st[CriterionDOC9] != "review:" {
		t.Errorf("unknown head, cached tree: %v", st)
	}
	r.files = nil
	rep, _ = Build(t.Context(), r, cl, Filter{RepoID: 1}, opts)
	for _, id := range HistoryLeadCriteria {
		if s := leadStatuses(rep.Repos[0].Leads)[id]; s != "unknown:not_computed" {
			t.Errorf("%s unknown head, no cache: %s", id, s)
		}
	}
}
