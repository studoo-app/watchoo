package semver_test

import (
	"testing"

	"github.com/studoo-app/watchoo/internal/semver"
)

func TestParseAndCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v0.5.0", "v0.6.0", -1},
		{"0.6.0", "v0.6.0", 0},
		{"v1.0.0", "v0.99.99", 1},
		{"v0.10.0", "v0.9.0", 1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-alpha.1", "v1.0.0-alpha.beta", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.11", -1},
		{"v1.0.0-rc.1", "v1.0.0-beta", 1},
		{"v1.0.0+build.1", "v1.0.0", 0},
	}
	for _, tt := range tests {
		a, okA := semver.ParseVersion(tt.a)
		b, okB := semver.ParseVersion(tt.b)
		if !okA || !okB {
			t.Fatalf("ParseVersion(%q / %q) refused", tt.a, tt.b)
		}
		if got := a.Compare(b); got != tt.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
		if got := b.Compare(a); got != -tt.want {
			t.Errorf("Compare(%s, %s) = %d, want %d", tt.b, tt.a, got, -tt.want)
		}
	}
	for _, bad := range []string{"", "dev", "v1.2", "v01.2.3", "v1.2.3-", "v1.2.3-01", "v1.2.3+", "1.2.3.4", "v1.x.3", "(devel)"} {
		if _, ok := semver.ParseVersion(bad); ok {
			t.Errorf("ParseVersion(%q) accepted", bad)
		}
	}
	if v, _ := semver.ParseVersion("1.2.3-rc.1+b"); v.String() != "v1.2.3-rc.1+b" {
		t.Errorf("String = %s", v)
	}
}
