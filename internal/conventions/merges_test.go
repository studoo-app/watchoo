package conventions

import (
	"reflect"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/model"
)

func intp(n int) *int { return &n }

func boolp(b bool) *bool { return &b }

func TestClassifyMerge(t *testing.T) {
	p := NewParser(DefaultRules())
	commit := func(parents *int, msg string) *model.ConventionCommit {
		return &model.ConventionCommit{SHA: "a", ParentCount: parents, Message: msg}
	}
	cases := []struct {
		name   string
		commit *model.ConventionCommit
		want   MergeResult
	}{
		{"absent", nil, MergeResult{Method: MergeMethodUnknown}},
		{"parents unknown", commit(nil, "feat: x (#4)"), MergeResult{Method: MergeMethodUnknown}},
		{"root", commit(intp(0), "feat: x (#4)"), MergeResult{Method: MergeMethodUnknown}},
		{"squash conform", commit(intp(1), "feat: ajoute la connexion (#4)\n\n* wip\n* fix"),
			MergeResult{Method: MergeMethodSquash, MessageConform: true, Conform: true}},
		{"squash not conform", commit(intp(1), "Update login (#4)"), MergeResult{Method: MergeMethodSquash}},
		{"squash no space", commit(intp(1), "fix: corrige le lien(#4)"),
			MergeResult{Method: MergeMethodSquash, MessageConform: true, Conform: true}},
		{"other pr suffix", commit(intp(1), "feat: x (#5)"), MergeResult{Method: MergeMethodUnknown}},
		{"rebase", commit(intp(1), "feat: dernier commit rebasé"), MergeResult{Method: MergeMethodRebase}},
		{"merge commit", commit(intp(2), "Merge pull request #4 from a/feat/x"), MergeResult{Method: MergeMethodMerge}},
		{"octopus", commit(intp(3), "feat: x (#4)"), MergeResult{Method: MergeMethodMerge}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyMerge(4, tc.commit, p)
			if got != tc.want {
				t.Errorf("ClassifyMerge = %+v, want %+v", got, tc.want)
			}
			if got.Known() != (tc.want.Method != MergeMethodUnknown) {
				t.Errorf("Known = %v", got.Known())
			}
		})
	}
}

// sha40 fabrique un SHA de test à partir d'un caractère hexadécimal.
func sha40(c byte) string {
	b := make([]byte, 40)
	for i := range b {
		b[i] = c
	}
	return string(b)
}

func TestClassifyFirstParent(t *testing.T) {
	merged := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)
	gh := func(sha string, parents *int, msg string, at time.Time) model.ConventionCommit {
		return model.ConventionCommit{SHA: sha, ParentCount: parents, Message: msg, CommitterLogin: "web-flow",
			CommittedAt: at, AuthorID: 7, AuthorName: "Alice", AuthorLogin: "alice"}
	}
	human := func(sha string, parents *int, msg string) model.ConventionCommit {
		return model.ConventionCommit{SHA: sha, ParentCount: parents, Message: msg, CommitterLogin: "bob",
			CommitterEmails: []string{"bob@example.test"}, CommittedAt: merged.Add(time.Hour), AuthorID: 8, AuthorLogin: "bob"}
	}
	pr := func(n int, sha string, at time.Time, base string) model.ConventionPullRequest {
		return model.ConventionPullRequest{PullRequest: model.PullRequest{Number: n, State: model.PullRequestClosed,
			Merged: !at.IsZero(), MergedAt: at, MergeCommitSHA: sha, BaseBranch: base}}
	}
	var (
		sSquash   = sha40('1') // squash de la PR 10 (merge_commit_sha)
		sRebase3  = sha40('2') // dernier commit rebasé de la PR 11 (merge_commit_sha)
		sRebase2  = sha40('3') // commit rebasé de la PR 11
		sRebase1  = sha40('4') // premier commit rebasé de la PR 11
		sPush     = sha40('5') // push direct
		sLocal    = sha40('6') // fusion locale poussée
		sWeb      = sha40('7') // édition web
		sGHMerge  = sha40('8') // fusion de GitHub sans merge_commit_sha connu
		sOldSq    = sha40('9') // squash d'une PR dont le merge_commit_sha est inconnu
		sMissing  = sha40('a') // absent de la base
		sNoCommit = sha40('b') // committer inconnu
		sMissPR   = sha40('c') // absent de la base mais merge_commit_sha d'une PR
		sOtherBr  = sha40('d') // merge_commit_sha d'une PR vers une autre branche
	)
	rebaseAt := merged.Add(2 * time.Hour)
	byKey := map[string]model.ConventionCommit{
		sSquash:   gh(sSquash, intp(1), "feat: x (#10)", merged),
		sRebase3:  gh(sRebase3, intp(1), "fix: c", rebaseAt.Add(300*time.Millisecond)),
		sRebase2:  gh(sRebase2, intp(1), "fix: b", rebaseAt),
		sRebase1:  gh(sRebase1, intp(1), "fix: a", rebaseAt),
		sPush:     human(sPush, intp(1), "fix: direct"),
		sLocal:    human(sLocal, intp(2), "Merge branch 'feat/x'"),
		sWeb:      gh(sWeb, intp(1), "Update README.md", merged.Add(5*time.Hour)),
		sGHMerge:  gh(sGHMerge, intp(2), "Merge pull request #13 from a/feat/y", merged.Add(6*time.Hour)),
		sOldSq:    gh(sOldSq, intp(1), "feat: y (#14)", merged.Add(7*time.Hour)),
		sNoCommit: {SHA: sNoCommit, ParentCount: intp(1), Message: "fix: z", AuthorID: 9},
		sOtherBr:  gh(sOtherBr, intp(1), "feat: w (#16)", merged.Add(8*time.Hour)),
	}
	pulls := []model.ConventionPullRequest{
		pr(10, sSquash, merged, "main"),
		pr(11, sRebase3, rebaseAt, "main"),
		pr(12, "", time.Time{}, "main"), // non fusionnée
		pr(14, "", merged.Add(7*time.Hour+time.Minute), "main"),
		pr(15, sMissPR, merged.Add(9*time.Hour), "main"),
		pr(16, sOtherBr, merged.Add(8*time.Hour), "develop"),
	}
	shas := []string{sOtherBr, sMissPR, sOldSq, sGHMerge, sWeb, sLocal, sPush, sRebase3, sRebase2, sRebase1, sSquash,
		sMissing, sNoCommit}

	line := func(sha, class, reason string, n int) ArrivalLine {
		l := ArrivalLine{SHA: sha, Class: class, Reason: reason, PR: n}
		if c, ok := byKey[sha]; ok {
			l.AuthorID, l.AuthorName, l.AuthorLogin = c.AuthorID, c.AuthorName, c.AuthorLogin
		}
		return l
	}
	t.Run("api complete", func(t *testing.T) {
		got := ClassifyFirstParent(shas, byKey, pulls, FirstParentOptions{DefaultBranch: "main", API: true, Complete: true})
		want := []ArrivalLine{
			line(sOtherBr, ArrivalOutsidePR, ArrivalWebEdit, 0),
			line(sMissPR, ArrivalByPR, ArrivalMergeCommitSHA, 15),
			line(sOldSq, ArrivalByPR, ArrivalSquashSuffix, 14),
			line(sGHMerge, ArrivalByPR, ArrivalGitHubMerge, 13),
			line(sWeb, ArrivalOutsidePR, ArrivalWebEdit, 0),
			line(sLocal, ArrivalOutsidePR, ArrivalLocalMerge, 0),
			line(sPush, ArrivalOutsidePR, ArrivalHumanCommitter, 0),
			line(sRebase3, ArrivalByPR, ArrivalMergeCommitSHA, 11),
			line(sRebase2, ArrivalByPR, ArrivalRebaseRun, 11),
			line(sRebase1, ArrivalByPR, ArrivalRebaseRun, 11),
			line(sSquash, ArrivalByPR, ArrivalMergeCommitSHA, 10),
			line(sMissing, ArrivalUndetermined, ArrivalNotInDatabase, 0),
			line(sNoCommit, ArrivalUndetermined, ArrivalCommitterUnknown, 0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent =\n%+v\nwant\n%+v", got, want)
		}
		tally := TallyArrivals(got, nil)
		if tally != (ArrivalTally{ByPR: 7, OutsidePR: 4, Undetermined: 2}) || tally.Total() != len(shas) {
			t.Errorf("TallyArrivals = %+v", tally)
		}
		bob := TallyArrivals(got, func(l ArrivalLine) bool { return l.AuthorID == 8 })
		if bob != (ArrivalTally{OutsidePR: 2}) {
			t.Errorf("TallyArrivals(bob) = %+v", bob)
		}
	})
	t.Run("api incomplete", func(t *testing.T) {
		got := ClassifyFirstParent([]string{sWeb, sPush}, byKey, pulls, FirstParentOptions{DefaultBranch: "main", API: true})
		want := []ArrivalLine{
			line(sWeb, ArrivalUndetermined, ArrivalUnmatched, 0),
			line(sPush, ArrivalOutsidePR, ArrivalHumanCommitter, 0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent = %+v, want %+v", got, want)
		}
	})
	t.Run("rebase run without merge sha", func(t *testing.T) {
		// La PR 11 sans merge_commit_sha : la suite est reconnue par merged_at seul.
		p := []model.ConventionPullRequest{pr(11, "", rebaseAt, "main"), pr(9, "", rebaseAt, "main")}
		got := ClassifyFirstParent([]string{sRebase2, sRebase1}, byKey, p, FirstParentOptions{DefaultBranch: "main", API: true})
		want := []ArrivalLine{
			line(sRebase2, ArrivalByPR, ArrivalRebaseRun, 9),
			line(sRebase1, ArrivalByPR, ArrivalRebaseRun, 9),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent = %+v, want %+v", got, want)
		}
	})
	t.Run("rebase run merged_at one second later", func(t *testing.T) {
		// GitHub a écrit la suite à rebaseAt, merged_at vaut rebaseAt + 1 s : la suite part de la tête.
		p := []model.ConventionPullRequest{pr(11, sRebase3, rebaseAt.Add(time.Second), "main")}
		k := map[string]model.ConventionCommit{
			sRebase3: gh(sRebase3, intp(1), "fix: c", rebaseAt),
			sRebase2: gh(sRebase2, intp(1), "fix: b", rebaseAt),
			sRebase1: gh(sRebase1, intp(1), "fix: a", rebaseAt),
			sWeb:     gh(sWeb, intp(1), "Update README.md", rebaseAt.Add(-time.Hour)),
		}
		for _, complete := range []bool{true, false} {
			got := ClassifyFirstParent([]string{sRebase3, sRebase2, sRebase1, sWeb}, k, p,
				FirstParentOptions{DefaultBranch: "main", API: true, Complete: complete})
			last := line(sWeb, ArrivalOutsidePR, ArrivalWebEdit, 0)
			if !complete {
				last = line(sWeb, ArrivalUndetermined, ArrivalUnmatched, 0)
			}
			want := []ArrivalLine{
				line(sRebase3, ArrivalByPR, ArrivalMergeCommitSHA, 11),
				line(sRebase2, ArrivalByPR, ArrivalRebaseRun, 11),
				line(sRebase1, ArrivalByPR, ArrivalRebaseRun, 11),
				last,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("complete=%v: ClassifyFirstParent =\n%+v\nwant\n%+v", complete, got, want)
			}
		}
	})
	t.Run("rebase run must be consecutive and end with the pr", func(t *testing.T) {
		// Une édition web à la seconde du merged_at d'une PR sans lien, séparée de la suite par un push
		// direct, reste une édition web ; un commit de GitHub au même instant mais plus récent que la tête
		// n'est pas rattaché.
		p := []model.ConventionPullRequest{pr(11, sRebase3, rebaseAt, "main")}
		k := map[string]model.ConventionCommit{
			sWeb:     gh(sWeb, intp(1), "Update README.md", rebaseAt),
			sRebase3: gh(sRebase3, intp(1), "fix: c", rebaseAt),
			sRebase2: gh(sRebase2, intp(1), "fix: b", rebaseAt),
			sPush:    human(sPush, intp(1), "fix: direct"),
			sRebase1: gh(sRebase1, intp(1), "Update docs", rebaseAt),
		}
		got := ClassifyFirstParent([]string{sWeb, sRebase3, sRebase2, sPush, sRebase1}, k, p,
			FirstParentOptions{DefaultBranch: "main", API: true, Complete: true})
		want := []ArrivalLine{
			line(sWeb, ArrivalOutsidePR, ArrivalWebEdit, 0),
			line(sRebase3, ArrivalByPR, ArrivalMergeCommitSHA, 11),
			line(sRebase2, ArrivalByPR, ArrivalRebaseRun, 11),
			line(sPush, ArrivalOutsidePR, ArrivalHumanCommitter, 0),
			line(sRebase1, ArrivalOutsidePR, ArrivalWebEdit, 0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent =\n%+v\nwant\n%+v", got, want)
		}
	})
	t.Run("rebase run without merge sha outside tolerance", func(t *testing.T) {
		p := []model.ConventionPullRequest{pr(11, "", rebaseAt.Add(3*time.Second), "main")}
		got := ClassifyFirstParent([]string{sRebase2, sRebase1}, byKey, p,
			FirstParentOptions{DefaultBranch: "main", API: true, Complete: true})
		want := []ArrivalLine{
			line(sRebase2, ArrivalOutsidePR, ArrivalWebEdit, 0),
			line(sRebase1, ArrivalOutsidePR, ArrivalWebEdit, 0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent = %+v, want %+v", got, want)
		}
		p = []model.ConventionPullRequest{pr(11, "", rebaseAt.Add(2*time.Second), "main")}
		got = ClassifyFirstParent([]string{sRebase2, sRebase1}, byKey, p,
			FirstParentOptions{DefaultBranch: "main", API: true, Complete: true})
		want = []ArrivalLine{
			line(sRebase2, ArrivalByPR, ArrivalRebaseRun, 11),
			line(sRebase1, ArrivalByPR, ArrivalRebaseRun, 11),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent = %+v, want %+v", got, want)
		}
	})
	t.Run("pr absent from the list", func(t *testing.T) {
		// Squash et fusion de GitHub d'une PR que la liste n'a pas encore (liste reportée pour quota) :
		// jamais une édition web, même liste complète.
		k := map[string]model.ConventionCommit{
			sOldSq:   gh(sOldSq, intp(1), "feat: y (#20)", merged),
			sGHMerge: gh(sGHMerge, intp(2), "Merge pull request #21 from a/feat/y", merged),
		}
		got := ClassifyFirstParent([]string{sOldSq, sGHMerge}, k, pulls,
			FirstParentOptions{DefaultBranch: "main", API: true, Complete: true})
		want := []ArrivalLine{
			line(sOldSq, ArrivalUndetermined, ArrivalUnmatched, 0),
			line(sGHMerge, ArrivalByPR, ArrivalGitHubMerge, 21),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent = %+v, want %+v", got, want)
		}
	})
	t.Run("merge with unknown parents", func(t *testing.T) {
		k := map[string]model.ConventionCommit{sGHMerge: gh(sGHMerge, nil, "Merge pull request #13 from a/b", time.Time{}),
			sLocal: human(sLocal, nil, "Merge remote-tracking branch 'origin/main'")}
		got := ClassifyFirstParent([]string{sGHMerge, sLocal}, k, nil, FirstParentOptions{DefaultBranch: "main", API: true})
		want := []ArrivalLine{
			line(sGHMerge, ArrivalByPR, ArrivalGitHubMerge, 13),
			line(sLocal, ArrivalOutsidePR, ArrivalLocalMerge, 0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent = %+v, want %+v", got, want)
		}
	})
	t.Run("clone only", func(t *testing.T) {
		email := func(c model.ConventionCommit) model.ConventionCommit {
			c.CommitterLogin, c.CommitterEmails = "", []string{"noreply@github.com"}
			return c
		}
		k := map[string]model.ConventionCommit{
			sSquash:   email(byKey[sSquash]),
			sGHMerge:  email(byKey[sGHMerge]),
			sRebase1:  email(byKey[sRebase1]),
			sWeb:      email(byKey[sWeb]),
			sPush:     byKey[sPush],
			sLocal:    byKey[sLocal],
			sNoCommit: byKey[sNoCommit],
		}
		got := ClassifyFirstParent([]string{sSquash, sGHMerge, sRebase1, sWeb, sPush, sLocal, sNoCommit, sMissPR}, k, pulls,
			FirstParentOptions{DefaultBranch: "main"})
		want := []ArrivalLine{
			line(sSquash, ArrivalByPR, ArrivalSquashSuffix, 10),
			line(sGHMerge, ArrivalByPR, ArrivalGitHubMerge, 13),
			line(sRebase1, ArrivalUndetermined, ArrivalCloneOnly, 0),
			line(sWeb, ArrivalUndetermined, ArrivalCloneOnly, 0),
			line(sPush, ArrivalOutsidePR, ArrivalHumanCommitter, 0),
			line(sLocal, ArrivalOutsidePR, ArrivalLocalMerge, 0),
			line(sNoCommit, ArrivalUndetermined, ArrivalCommitterUnknown, 0),
			line(sMissPR, ArrivalUndetermined, ArrivalNotInDatabase, 0),
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ClassifyFirstParent =\n%+v\nwant\n%+v", got, want)
		}
	})
	t.Run("empty", func(t *testing.T) {
		if got := ClassifyFirstParent(nil, nil, nil, FirstParentOptions{API: true}); got == nil || len(got) != 0 {
			t.Errorf("ClassifyFirstParent(nil) = %#v", got)
		}
	})
}

func TestReusedBranches(t *testing.T) {
	pr := func(n int, head, base string, merged bool, fork *bool, login string) model.ConventionPullRequest {
		return model.ConventionPullRequest{PullRequest: model.PullRequest{Number: n, HeadBranch: head, BaseBranch: base,
			Merged: merged, FromFork: fork, AuthorLogin: login}}
	}
	pulls := []model.ConventionPullRequest{
		pr(5, "feat/login", "main", true, boolp(false), "alice"),
		pr(2, "feat/login", "main", true, nil, "alice"),  // origine inconnue : comptée
		pr(9, "feat/login", "main", false, nil, "alice"), // non fusionnée
		pr(3, "fix/a", "main", true, nil, "bob"),
		pr(4, "fix/a", "develop", true, nil, "bob"), // autre base
		pr(6, "patch-1", "main", true, boolp(true), "eve"),
		pr(7, "patch-1", "main", true, boolp(true), "eve"), // forks
		pr(8, "deps", "main", true, nil, "dependabot[bot]"),
		pr(10, "deps", "main", true, nil, "dependabot[bot]"), // bot
		pr(11, "", "main", true, nil, "alice"),
		pr(12, "", "main", true, nil, "alice"), // branche inconnue
		pr(13, "dev", "main", true, nil, "carol"),
		pr(1, "dev", "main", true, nil, "dan"),
		pr(14, "dev", "main", true, nil, "carol"),
	}
	want := []ReusedBranch{{Name: "dev", PRs: []int{1, 13, 14}}, {Name: "feat/login", PRs: []int{2, 5}}}
	if got := ReusedBranches(pulls, "main"); !reflect.DeepEqual(got, want) {
		t.Errorf("ReusedBranches = %+v, want %+v", got, want)
	}
	if got := ReusedBranches(nil, "main"); got == nil || len(got) != 0 {
		t.Errorf("ReusedBranches(nil) = %#v", got)
	}
}

func TestIsFork(t *testing.T) {
	for _, tc := range []struct {
		fork *bool
		want bool
	}{{nil, false}, {boolp(false), false}, {boolp(true), true}} {
		if got := IsFork(model.PullRequest{FromFork: tc.fork}); got != tc.want {
			t.Errorf("IsFork(%v) = %v", tc.fork, got)
		}
	}
}

func TestCheckMergeMethods(t *testing.T) {
	tr, fa := boolp(true), boolp(false)
	cases := []struct {
		name                  string
		squash, merge, rebase *bool
		want                  MergeMethodsResult
	}{
		{"unknown", nil, nil, nil, MergeMethodsResult{}},
		{"partly unknown", tr, nil, fa, MergeMethodsResult{}},
		{"squash only", tr, fa, fa, MergeMethodsResult{Known: true, Squash: true, Conform: true}},
		{"github default", tr, tr, tr, MergeMethodsResult{Known: true, Squash: true, Merge: true, Rebase: true}},
		{"squash and rebase", tr, fa, tr, MergeMethodsResult{Known: true, Squash: true, Rebase: true}},
		{"merge only", fa, tr, fa, MergeMethodsResult{Known: true, Merge: true}},
		{"none", fa, fa, fa, MergeMethodsResult{Known: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CheckMergeMethods(tc.squash, tc.merge, tc.rebase); got != tc.want {
				t.Errorf("CheckMergeMethods = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestIsIdentifiedSquash(t *testing.T) {
	cases := []struct {
		name            string
		msg             string
		github          bool
		mergePR, parent int
		want            bool
	}{
		{"github squash", "feat: x (#4)", true, 0, -1, true},
		{"github no suffix", "feat: x", true, 0, -1, false},
		{"human suffix", "feat: x (#4)", false, 0, 1, false},
		{"merge commit sha", "feat: x (#4)", false, 4, 1, true},
		{"merge commit sha other suffix", "feat: x (#5)", false, 4, 1, false},
		{"merge commit two parents", "feat: x (#4)", false, 4, 2, false},
		{"merge commit parents unknown", "feat: x (#4)", false, 4, -1, false},
		{"empty", "", true, 4, 1, false},
	}
	for _, tc := range cases {
		if got := IsIdentifiedSquash(tc.msg, tc.github, tc.mergePR, tc.parent); got != tc.want {
			t.Errorf("%s: IsIdentifiedSquash = %v, want %v", tc.name, got, tc.want)
		}
	}
}
