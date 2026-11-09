package conventions

import (
	"reflect"
	"testing"
	"time"
)

func day(d int) time.Time { return time.Date(2026, 9, d, 12, 0, 0, 0, time.UTC) }

func TestAggregate(t *testing.T) {
	commit := func(sha, author, msg string, parents int, d int) Commit {
		return Commit{SHA: sha, AuthorKey: author, AuthorName: author, Message: msg, ParentCount: parents, ParentsKnown: true, AuthoredAt: day(d)}
	}
	commits := []Commit{
		// Avant la date de début : compté à part.
		commit("a0", "alice", "update", 1, 1),
		// Commit racine « Initial commit » créé par GitHub : exclu.
		{SHA: "r1", AuthorKey: "alice", AuthorName: "alice", Message: "Initial commit", ParentsKnown: true, ParentCount: 0, AuthoredAt: day(5), GitHubCommitter: true},
		commit("c1", "alice", "feat: ajoute la connexion", 1, 6),
		commit("c2", "alice", "Update README.md", 1, 6), // édition web : évaluée
		commit("c3", "bob", "fix(api): corrige le jeton (#12)", 1, 7),
		commit("c4", "bob", "feat: Ajoute la page", 1, 7),          // avertissement seul
		commit("c5", "bob", "wip", 1, 8),                           // interdit
		commit("m1", "bob", "Merge branch 'main' into feat", 2, 8), // fusion
		// parent_count inconnu : repli sur le message de fusion par défaut.
		{SHA: "m2", AuthorKey: "bob", AuthorName: "bob", Message: "Merge pull request #3 from bob/x\n\nfeat: x", AuthoredAt: day(8)},
		// parent_count inconnu, message ordinaire : évalué.
		{SHA: "u1", AuthorKey: "bob", AuthorName: "bob", Message: "Merge dev into main", AuthoredAt: day(8)},
		{SHA: "b1", AuthorKey: "dependabot", AuthorLogin: "dependabot[bot]", Message: "Bump x", ParentsKnown: true, ParentCount: 1, AuthoredAt: day(9)},
		{SHA: "b2", AuthorKey: "copilot", AuthorName: "copilot-swe-agent", Message: "Initial plan", ParentsKnown: true, ParentCount: 1, AuthoredAt: day(9)},
		{SHA: "b3", AuthorKey: "ci", AuthorName: "ci", Bot: true, Message: "x", ParentsKnown: true, ParentCount: 1, AuthoredAt: day(9)},
		{SHA: "t1", AuthorKey: "prof", AuthorName: "Prof", AuthorEmails: []string{"Prof@School.example"}, Message: "maj", ParentsKnown: true, ParentCount: 1, AuthoredAt: day(9)},
		commit("v1", "alice", "Revert \"feat: ajoute la connexion\"\n\nThis reverts commit c1.", 1, 10), // revert compté
		commit("c1", "alice", "feat: ajoute la connexion", 1, 6),                                        // SHA en double : ignoré
	}
	rep := Aggregate(commits, Options{
		Rules:            DefaultRules(),
		Since:            day(2),
		ExcludedAccounts: []string{" prof@school.example "},
	})
	if rep.RulesVersion != RulesVersion {
		t.Errorf("RulesVersion = %d", rep.RulesVersion)
	}
	if rep.BeforeSince != 1 {
		t.Errorf("BeforeSince = %d", rep.BeforeSince)
	}
	// Évalués : c1, c2, c3, c4, c5, u1, v1 ; conformes : c1, c3, c4, v1.
	if rep.Evaluated != 7 || rep.Conform != 4 {
		t.Errorf("CC1 = %d/%d, want 4/7", rep.Conform, rep.Evaluated)
	}
	if rate, ok := rep.Rate(); !ok || rate != 4.0/7.0 {
		t.Errorf("Rate = %v, %v", rate, ok)
	}
	wantExcl := map[string]int{ExclusionMerge: 2, ExclusionBot: 3, ExclusionInitialCommit: 1, ExclusionAccount: 1}
	if !reflect.DeepEqual(rep.Excluded, wantExcl) || rep.ExcludedTotal() != 7 {
		t.Errorf("CC4 = %v", rep.Excluded)
	}
	wantIssues := []Issue{
		{Code: CodeForbidden, Severity: SeverityBlocking, Count: 1, SHAs: []string{"c5"}},
		{Code: CodeGitHubDefault, Severity: SeverityBlocking, Count: 1, SHAs: []string{"c2"}},
		{Code: CodeNoType, Severity: SeverityBlocking, Count: 1, SHAs: []string{"u1"}},
		{Code: CodeDescCapitalized, Severity: SeverityWarning, Count: 1, SHAs: []string{"c4"}},
		{Code: CodeGitRevert, Severity: SeverityWarning, Count: 1, SHAs: []string{"v1"}},
	}
	if !reflect.DeepEqual(rep.Issues, wantIssues) {
		t.Errorf("CC2 = %+v", rep.Issues)
	}
	// CC3 : par contributeur, triés ; un contributeur qui n'a que des commits avant la date n'apparaît pas.
	keys := []string{}
	for _, c := range rep.Contributors {
		keys = append(keys, c.AuthorKey)
	}
	if !reflect.DeepEqual(keys, []string{"alice", "bob", "ci", "copilot", "dependabot", "prof"}) {
		t.Errorf("contributors = %v", keys)
	}
	alice, bob := rep.Contributors[0], rep.Contributors[1]
	if alice.Evaluated != 3 || alice.Conform != 2 || alice.Excluded[ExclusionInitialCommit] != 1 {
		t.Errorf("alice = %+v", alice.Tally)
	}
	if bob.Evaluated != 4 || bob.Conform != 2 || bob.Excluded[ExclusionMerge] != 2 || len(bob.Issues) != 3 {
		t.Errorf("bob = %+v", bob.Tally)
	}
	if _, ok := rep.Contributors[2].Rate(); ok {
		t.Error("bot contributor has a rate")
	}
	// Résultats par commit : un par SHA, dans l'ordre.
	if len(rep.Commits) != 15 {
		t.Fatalf("%d commit results", len(rep.Commits))
	}
	byID := map[string]CommitResult{}
	for _, c := range rep.Commits {
		byID[c.SHA] = c
	}
	if c := byID["a0"]; !c.BeforeSince || c.Evaluated() || len(c.Codes) != 0 {
		t.Errorf("a0 = %+v", c)
	}
	if c := byID["m1"]; c.Excluded != ExclusionMerge || c.Evaluated() || c.Conform {
		t.Errorf("m1 = %+v", c)
	}
	if c := byID["c3"]; !c.Conform || len(c.Codes) != 0 {
		t.Errorf("c3 (squash suffix) = %+v", c)
	}
	if c := byID["c5"]; c.Conform || !reflect.DeepEqual(c.Codes, []string{CodeForbidden}) {
		t.Errorf("c5 = %+v", c)
	}
}

func TestAggregateExclusions(t *testing.T) {
	tests := []struct {
		name string
		c    Commit
		want string
	}{
		{"two parents", Commit{Message: "feat: x", ParentCount: 2, ParentsKnown: true}, ExclusionMerge},
		{"octopus", Commit{Message: "x", ParentCount: 3, ParentsKnown: true}, ExclusionMerge},
		{"one parent with merge message", Commit{Message: "Merge branch 'main'", ParentCount: 1, ParentsKnown: true}, ""},
		{"unknown parents, merge message", Commit{Message: "Merge remote-tracking branch 'origin/main'"}, ExclusionMerge},
		{"unknown parents, ordinary", Commit{Message: "feat: x"}, ""},
		{"bot by name", Commit{AuthorName: "renovate[bot]", Message: "x"}, ExclusionBot},
		{"bot flag", Commit{Bot: true, Message: "x"}, ExclusionBot},
		{"agent account of the catalog stays evaluated", Commit{AuthorLogin: "copilot", Message: "x"}, ""},
		{"initial commit by GitHub", Commit{Message: "Initial commit", Root: true, GitHubCommitter: true}, ExclusionInitialCommit},
		{"initial commit, root from parent count", Commit{Message: "Initial commit\n", ParentsKnown: true, GitHubCommitter: true}, ExclusionInitialCommit},
		{"initial commit by a student", Commit{Message: "Initial commit", Root: true}, ""},
		{"initial commit by GitHub, parents unknown", Commit{Message: "Initial commit", GitHubCommitter: true}, ExclusionInitialCommit},
		{"initial commit by a student, parents unknown", Commit{Message: "Initial commit"}, ""},
		{"other subject by GitHub, parents unknown", Commit{Message: "Initial commit of the app", GitHubCommitter: true}, ""},
		{"initial commit not root", Commit{Message: "Initial commit", ParentCount: 1, ParentsKnown: true, GitHubCommitter: true}, ""},
		{"other root by GitHub", Commit{Message: "Create README.md", Root: true, GitHubCommitter: true}, ""},
		{"excluded login", Commit{AuthorLogin: "Prof-X", Message: "x"}, ExclusionAccount},
		{"excluded name", Commit{AuthorName: "prof-x", Message: "x"}, ExclusionAccount},
		{"merge before bot", Commit{AuthorName: "a[bot]", Message: "x", ParentCount: 2, ParentsKnown: true}, ExclusionMerge},
		{"git revert is evaluated", Commit{Message: "Revert \"feat: x\"", ParentCount: 1, ParentsKnown: true}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.c.SHA = "s"
			rep := Aggregate([]Commit{tt.c}, Options{Rules: DefaultRules(), ExcludedAccounts: []string{"", "prof-x"}})
			if got := rep.Commits[0].Excluded; got != tt.want {
				t.Errorf("exclusion = %q, want %q", got, tt.want)
			}
			if rep.Commits[0].Evaluated() != (tt.want == "") {
				t.Error("Evaluated mismatch")
			}
		})
	}
}

func TestAggregateEmpty(t *testing.T) {
	rep := Aggregate(nil, Options{Rules: DefaultRules()})
	if rep.Evaluated != 0 || rep.Issues == nil || rep.Commits == nil || rep.Contributors == nil || len(rep.Excluded) != len(AllExclusions) {
		t.Errorf("empty report = %+v", rep)
	}
	if _, ok := rep.Rate(); ok {
		t.Error("rate of nothing is known")
	}
}

func TestAggregateSinceBoundary(t *testing.T) {
	since := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	rep := Aggregate([]Commit{
		{SHA: "a", Message: "feat: x", AuthoredAt: since.Add(-time.Second)},
		{SHA: "b", Message: "feat: x", AuthoredAt: since},
	}, Options{Rules: DefaultRules(), Since: since})
	if rep.BeforeSince != 1 || rep.Evaluated != 1 {
		t.Errorf("before %d, evaluated %d", rep.BeforeSince, rep.Evaluated)
	}
}

func TestSince(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "",
		"  ":                   "",
		"2026-09-01":           "2026-09-01T00:00:00Z",
		"2026-09-01T00:00:00Z": "2026-09-01T00:00:00Z",
		"2026-09-01T15:04:05Z": "2026-09-01T00:00:00Z",
	} {
		if got, err := NormalizeSince(in); err != nil || got != want {
			t.Errorf("NormalizeSince(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"01/09/2026", "2026-13-01", "2026-9-1", "demain"} {
		if _, err := NormalizeSince(bad); err != ErrInvalidSince {
			t.Errorf("NormalizeSince(%q) err = %v", bad, err)
		}
		if _, err := SinceAt(bad, nil); err != ErrInvalidSince {
			t.Errorf("SinceAt(%q) err = %v", bad, err)
		}
	}
	for in, want := range map[string]string{"": "", " 2026-09-01 ": "2026-09-01", "2026-09-01T00:00:00Z": "2026-09-01"} {
		if got, err := SinceDate(in); err != nil || got != want {
			t.Errorf("SinceDate(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := SinceDate("2026-13-01"); err != ErrInvalidSince {
		t.Errorf("SinceDate invalid: %v", err)
	}
	paris, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Skip("no tzdata")
	}
	got, err := SinceAt("2026-09-01T00:00:00Z", paris)
	if err != nil || !got.Equal(time.Date(2026, 8, 31, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("SinceAt Paris = %v, %v", got, err)
	}
	if got, _ := SinceAt("2026-09-01", nil); !got.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("SinceAt UTC = %v", got)
	}
	if got, _ := SinceAt("", paris); !got.IsZero() {
		t.Errorf("SinceAt empty = %v", got)
	}
	if got, err := EffectiveSinceAt("2026-01-01", "2026-09-01", paris); err != nil ||
		!got.Equal(time.Date(2026, 8, 31, 22, 0, 0, 0, time.UTC)) {
		t.Errorf("EffectiveSinceAt own date = %v, %v", got, err)
	}
	if got, err := EffectiveSinceAt("2026-01-01", "", nil); err != nil || !got.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("EffectiveSinceAt workspace date = %v, %v", got, err)
	}
	if got, err := EffectiveSinceAt("", "", paris); err != nil || !got.IsZero() {
		t.Errorf("EffectiveSinceAt no date = %v, %v", got, err)
	}
	if EffectiveSince("2026-01-01", "") != "2026-01-01" || EffectiveSince("2026-01-01", "2026-03-01") != "2026-03-01" ||
		EffectiveSince("", " ") != "" {
		t.Error("EffectiveSince")
	}
}
