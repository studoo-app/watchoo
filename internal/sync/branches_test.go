package sync_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/studoo-app/watchoo/internal/provider"
	wsync "github.com/studoo-app/watchoo/internal/sync"
	"github.com/studoo-app/watchoo/internal/workspace"
)

// names returns the names of the branches.
func names(bs []provider.Branch) []string {
	out := make([]string, len(bs))
	for i, b := range bs {
		out[i] = b.Name
	}
	return out
}

func TestSelectBranches(t *testing.T) {
	all := []provider.Branch{{Name: "dependabot/npm"}, {Name: "dev"}, {Name: "main"}, {Name: "release/1"},
		{Name: "release/2/fix"}, {Name: "wip"}}
	cases := []struct {
		name              string
		patterns, exclude []string
		want              []string
	}{
		{"all", nil, nil, []string{"dependabot/npm", "dev", "main", "release/1", "release/2/fix", "wip"}},
		{"exclusion", nil, []string{"dependabot/*", "wip"}, []string{"dev", "main", "release/1", "release/2/fix"}},
		{"patterns", []string{"main", "release/*"}, nil, []string{"main", "release/1"}},
		{"patterns and exclusion", []string{"release/*", "dependabot/*"}, []string{"dependabot/*"}, []string{"release/1"}},
		{"invalid pattern", []string{"[", "dev"}, []string{"["}, []string{"dev"}},
		{"none", []string{"prod"}, nil, []string{}},
	}
	for _, tc := range cases {
		got := wsync.SelectBranches(all, tc.patterns, tc.exclude)
		if got == nil || !slices.Equal(names(got), tc.want) {
			t.Errorf("%s: %v, want %v", tc.name, names(got), tc.want)
		}
	}
	if got := wsync.SelectBranches(nil, nil, nil); got == nil || len(got) != 0 {
		t.Errorf("empty list: %v", got)
	}
}

func TestReferenceBranch(t *testing.T) {
	tracked := []provider.Branch{{Name: "dev"}, {Name: "main"}}
	for _, tc := range []struct {
		def  string
		list []provider.Branch
		want int
	}{
		{"main", tracked, 1}, {"master", tracked, 0}, {"", tracked, 0}, {"main", nil, -1},
	} {
		if got := wsync.ReferenceBranch(tc.list, tc.def); got != tc.want {
			t.Errorf("ReferenceBranch(%v, %q) = %d, want %d", names(tc.list), tc.def, got, tc.want)
		}
	}
}

func TestAutoInterval(t *testing.T) {
	auto := workspace.SyncConfig{Auto: true, Interval: 30 * time.Minute}
	manual := workspace.SyncConfig{Auto: false, Interval: 30 * time.Minute}
	cases := []struct {
		name       string
		cfg        workspace.SyncConfig
		override   time.Duration
		overridden bool
		want       time.Duration
		err        string
	}{
		{"auto config", auto, 0, false, 30 * time.Minute, ""},
		{"manual config", manual, 0, false, 0, ""},
		{"--interval 0", auto, 0, true, 0, ""},
		{"--interval despite auto false", manual, time.Hour, true, time.Hour, ""},
		{"--interval minimum", auto, 15 * time.Minute, true, 15 * time.Minute, ""},
		{"--interval too short", auto, 10 * time.Minute, true, 0,
			"--interval: 10m0s is below the minimum of 15 min (0 for manual sync only)"},
		{"negative --interval", auto, -time.Minute, true, 0, "--interval: -1m0s is negative"},
		{"invalid config", workspace.SyncConfig{Auto: true, Interval: time.Minute}, 0, false, 0, "sync.interval: 1m0s"},
	}
	for _, tc := range cases {
		got, err := wsync.AutoInterval(tc.cfg, tc.override, tc.overridden)
		if tc.err != "" {
			if err == nil || !strings.HasPrefix(err.Error(), tc.err) {
				t.Errorf("%s: error %v, want %q", tc.name, err, tc.err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%s: %v, %v; want %v", tc.name, got, err, tc.want)
		}
	}
}
