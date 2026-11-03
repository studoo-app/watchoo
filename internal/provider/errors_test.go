package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSentinelsAreDistinct(t *testing.T) {
	all := []error{
		ErrNotFound, ErrUnauthorized, ErrForbidden, ErrRateLimited, ErrEmptyRepository, ErrUnknownBase,
		ErrWriteForbidden, ErrInvalidRef, ErrUnavailable, ErrUnexpected, ErrUnsupported, ErrAccessDenied,
	}
	for i, a := range all {
		if a.Error() == "" {
			t.Errorf("error %d without a message", i)
		}
		for j, b := range all {
			if i != j && (errors.Is(a, b) || a.Error() == b.Error()) {
				t.Errorf("%q and %q are not distinct", a, b)
			}
		}
	}
	if !strings.Contains(ErrUnauthorized.Error(), "gh auth login") {
		t.Errorf("ErrUnauthorized must advise gh auth login: %q", ErrUnauthorized)
	}
	if !strings.Contains(ErrNotFound.Error(), "without access") {
		t.Errorf("ErrNotFound must mention the lack of access: %q", ErrNotFound)
	}
	if !strings.HasPrefix(ErrAccessDenied.Error(), "access denied") {
		t.Errorf("ErrAccessDenied must start with \"access denied\": %q", ErrAccessDenied)
	}
}

func TestErrHostKeyUnknown(t *testing.T) {
	if !errors.Is(ErrHostKeyUnknown, ErrAccessDenied) {
		t.Error("ErrHostKeyUnknown must be an ErrAccessDenied")
	}
	wrapped := &APIError{Op: "update", Err: ErrHostKeyUnknown}
	if !errors.Is(wrapped, ErrHostKeyUnknown) || !errors.Is(wrapped, ErrAccessDenied) {
		t.Errorf("errors.Is through APIError: %v", wrapped)
	}
	if errors.Is(ErrAccessDenied, ErrHostKeyUnknown) {
		t.Error("ErrAccessDenied must not be an ErrHostKeyUnknown")
	}
	msg := ErrHostKeyUnknown.Error()
	if !strings.HasPrefix(msg, "access denied") || !strings.Contains(msg, "ssh -T git@github.com") ||
		msg == ErrAccessDenied.Error() {
		t.Errorf("ErrHostKeyUnknown = %q", msg)
	}
}

func TestRateLimitError(t *testing.T) {
	reset := time.Date(2026, 10, 26, 15, 4, 5, 0, time.FixedZone("CET", 3600))
	tests := []struct {
		name string
		err  *RateLimitError
		msg  string
	}{
		{"quota with reset", &RateLimitError{Reset: reset}, "GitHub API rate limit exhausted until 2026-10-26 14:04:05 UTC"},
		{"quota without reset", &RateLimitError{}, "GitHub API rate limit exhausted"},
		{"secondary with reset", &RateLimitError{Secondary: true, Reset: reset, RetryAfter: time.Minute}, "try again after 2026-10-26 14:04:05 UTC"},
		{"secondary with delay", &RateLimitError{Secondary: true, RetryAfter: 90 * time.Second}, "try again in 1m30s"},
		{"secondary without hint", &RateLimitError{Secondary: true}, "try again later"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.err.Error(), tt.msg) {
				t.Errorf("Error() = %q, want %q", tt.err.Error(), tt.msg)
			}
			if tt.err.Secondary && !strings.Contains(tt.err.Error(), "secondary") {
				t.Errorf("Error() = %q, want a mention of the secondary limit", tt.err.Error())
			}
			wrapped := fmt.Errorf("listing commits: %w", tt.err)
			if !errors.Is(wrapped, ErrRateLimited) {
				t.Error("errors.Is(err, ErrRateLimited) = false")
			}
			if errors.Is(wrapped, ErrForbidden) || errors.Is(wrapped, ErrUnavailable) {
				t.Error("RateLimitError must satisfy only ErrRateLimited")
			}
			var rl *RateLimitError
			if !errors.As(wrapped, &rl) || rl != tt.err {
				t.Error("errors.As(err, *RateLimitError) fails")
			}
		})
	}
}

func TestSSORequiredError(t *testing.T) {
	err := &SSORequiredError{
		Organization: "ecole-dev",
		URL:          "https://github.com/orgs/ecole-dev/sso?authorization_request=SECRETREQUEST",
	}
	msg := err.Error()
	for _, want := range []string{`"ecole-dev"`, "SSO", "gh auth refresh", "https://github.com/orgs/ecole-dev/sso"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, want %q", msg, want)
		}
	}
	if strings.Contains(msg, "SECRETREQUEST") || strings.Contains(msg, "authorization_request") {
		t.Errorf("Error() repeats the link parameters: %q", msg)
	}
	wrapped := fmt.Errorf("reading repository: %w", err)
	if !errors.Is(wrapped, ErrForbidden) {
		t.Error("errors.Is(err, ErrForbidden) = false")
	}
	if errors.Is(wrapped, ErrNotFound) || errors.Is(wrapped, ErrRateLimited) {
		t.Error("SSORequiredError must satisfy only ErrForbidden")
	}
	var sso *SSORequiredError
	if !errors.As(wrapped, &sso) || sso.Organization != "ecole-dev" {
		t.Error("errors.As(err, *SSORequiredError) fails")
	}

	// Without an organization or a usable link.
	msg = (&SSORequiredError{URL: "javascript:alert(1)"}).Error()
	if !strings.Contains(msg, "of this repository") || strings.Contains(msg, "javascript") || strings.Contains(msg, "open ") {
		t.Errorf("Error() without organization = %q", msg)
	}
	msg = (&SSORequiredError{Organization: "x", URL: "https://user:pass@github.com/orgs/x/sso"}).Error()
	if strings.Contains(msg, "pass") {
		t.Errorf("Error() repeats credentials from the URL: %q", msg)
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		err  *APIError
		want string
	}{
		{
			&APIError{Op: "reading repository alice/projet", Method: "GET", Path: "/repos/alice/projet", Status: 404, Err: ErrNotFound},
			"reading repository alice/projet (GET /repos/alice/projet, HTTP 404): repository not found or account without access",
		},
		{
			&APIError{Op: "listing branches of alice/projet", Method: "GET", Path: "/repos/alice/projet/branches", Err: ErrUnavailable},
			"listing branches of alice/projet (GET /repos/alice/projet/branches): " + ErrUnavailable.Error(),
		},
		{&APIError{Op: "quota state", Err: ErrUnexpected}, "quota state: " + ErrUnexpected.Error()},
		{&APIError{Status: 500}, "(HTTP 500)"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}

	// The cause stays reachable through several wrappers.
	causes := []error{
		ErrNotFound,
		&RateLimitError{Reset: time.Unix(1_800_000_000, 0)},
		&SSORequiredError{Organization: "o"},
		context.Canceled,
	}
	for _, cause := range causes {
		err := fmt.Errorf("sync: %w", &APIError{Op: "op", Method: "GET", Path: "/x", Status: 403, Err: cause})
		if !errors.Is(err, cause) {
			t.Errorf("errors.Is(%v) = false", cause)
		}
		var api *APIError
		if !errors.As(err, &api) || api.Status != 403 {
			t.Errorf("errors.As(*APIError) fails for %v", cause)
		}
	}
	err := fmt.Errorf("x: %w", &APIError{Op: "op", Err: &SSORequiredError{}})
	if !errors.Is(err, ErrForbidden) {
		t.Error("APIError > SSORequiredError must satisfy ErrForbidden")
	}
	err = &APIError{Op: "op", Err: &RateLimitError{}}
	if !errors.Is(err, ErrRateLimited) {
		t.Error("APIError > RateLimitError must satisfy ErrRateLimited")
	}
}
