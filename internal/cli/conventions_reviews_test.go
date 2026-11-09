package cli

// Conventions, phase 8.6: the reviews cell of a contributor (PR4 of their merged pull requests and
// reviews received), the same rule as the web: a dash without any pull request read, the reviews received
// alone without a merged pull request read (never a "0/0" rate).

import (
	"testing"

	"github.com/studoo-app/watchoo/internal/conventions"
	"github.com/studoo-app/watchoo/internal/i18n"
)

func TestAuthorReviewsText(t *testing.T) {
	tests := []struct {
		name   string
		tally  conventions.PRTally
		en, fr string
	}{
		{"nothing read", conventions.PRTally{}, "-", "-"},
		{"open pull requests only", conventions.PRTally{Open: conventions.PRCount{ReviewKnown: 1, ReviewsReceived: 2}},
			"2 review(s) received", "2 relecture(s) reçue(s)"},
		{"open pull request read, no review", conventions.PRTally{Open: conventions.PRCount{ReviewKnown: 1}},
			"0 review(s) received", "0 relecture(s) reçue(s)"},
		{"merged and open", conventions.PRTally{
			Open:   conventions.PRCount{ReviewKnown: 1, ReviewsReceived: 1},
			Merged: conventions.PRCount{ReviewKnown: 2, Reviewed: 1, ReviewsReceived: 2}},
			"1/2, 3 review(s) received", "1/2, 3 relecture(s) reçue(s)"},
	}
	for _, tt := range tests {
		for lang, want := range map[i18n.Lang]string{i18n.English: tt.en, i18n.French: tt.fr} {
			restore := i18n.SetProcessForTest(lang)
			if got := authorReviewsText(tt.tally, "-"); got != want {
				t.Errorf("%s (%s): %q, want %q", tt.name, lang, got, want)
			}
			restore()
		}
	}
}
