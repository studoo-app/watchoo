package model

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeContributorLabel(t *testing.T) {
	for _, tc := range []struct {
		in, want string
		err      error
	}{
		{"  Alice Martin ", "Alice Martin", nil},
		{"", "", nil},
		{"   ", "", nil},
		{"Élodie Dupré", "Élodie Dupré", nil},
		{strings.Repeat("é", MaxContributorLabelRunes), strings.Repeat("é", MaxContributorLabelRunes), nil},
		{strings.Repeat("é", MaxContributorLabelRunes+1), "", ErrContributorLabelTooLong},
		{"Alice\tMartin", "", ErrContributorLabelInvalid},
		{"Alice\x1b[31m", "", ErrContributorLabelInvalid},
		{"Alice\xff", "", ErrContributorLabelInvalid},
	} {
		got, err := NormalizeContributorLabel(tc.in)
		if got != tc.want || !errors.Is(err, tc.err) {
			t.Errorf("NormalizeContributorLabel(%q) = %q, %v; want %q, %v", tc.in, got, err, tc.want, tc.err)
		}
	}
}

func TestContributorDisplayNameAndCommitName(t *testing.T) {
	for _, tc := range []struct {
		c                 Contributor
		display, original string
	}{
		{Contributor{Label: "Alice Martin", Name: "alice", ProviderLogin: "alice42"}, "Alice Martin", "alice"},
		{Contributor{Label: "Alice", Name: "Alice", ProviderLogin: "alice42"}, "Alice", ""},
		{Contributor{Label: "Alice Martin", ProviderLogin: "alice42"}, "Alice Martin", ""},
		{Contributor{Name: "alice", ProviderLogin: "alice42"}, "alice", ""},
		{Contributor{ProviderLogin: "alice42"}, "alice42", ""},
	} {
		if got := tc.c.DisplayName(); got != tc.display {
			t.Errorf("%+v: DisplayName = %q, want %q", tc.c, got, tc.display)
		}
		if got := tc.c.CommitName(); got != tc.original {
			t.Errorf("%+v: CommitName = %q, want %q", tc.c, got, tc.original)
		}
	}
}
