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
		ErrWriteForbidden, ErrInvalidRef, ErrUnavailable, ErrUnexpected, ErrUnsupported,
	}
	for i, a := range all {
		if a.Error() == "" {
			t.Errorf("erreur %d sans message", i)
		}
		for j, b := range all {
			if i != j && (errors.Is(a, b) || a.Error() == b.Error()) {
				t.Errorf("%q et %q ne sont pas distinctes", a, b)
			}
		}
	}
	if !strings.Contains(ErrUnauthorized.Error(), "gh auth login") {
		t.Errorf("ErrUnauthorized doit conseiller gh auth login : %q", ErrUnauthorized)
	}
	if !strings.Contains(ErrNotFound.Error(), "sans accès") {
		t.Errorf("ErrNotFound doit évoquer l'absence d'accès : %q", ErrNotFound)
	}
}

func TestRateLimitError(t *testing.T) {
	reset := time.Date(2026, 10, 26, 15, 4, 5, 0, time.FixedZone("CET", 3600))
	tests := []struct {
		name string
		err  *RateLimitError
		msg  string
	}{
		{"quota avec reset", &RateLimitError{Reset: reset}, "quota de l'API GitHub épuisé jusqu'au 2026-10-26 14:04:05 UTC"},
		{"quota sans reset", &RateLimitError{}, "quota de l'API GitHub épuisé"},
		{"secondaire avec reset", &RateLimitError{Secondary: true, Reset: reset, RetryAfter: time.Minute}, "réessayez après le 2026-10-26 14:04:05 UTC"},
		{"secondaire avec délai", &RateLimitError{Secondary: true, RetryAfter: 90 * time.Second}, "réessayez dans 1m30s"},
		{"secondaire sans indication", &RateLimitError{Secondary: true}, "réessayez plus tard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.Contains(tt.err.Error(), tt.msg) {
				t.Errorf("Error() = %q, veut %q", tt.err.Error(), tt.msg)
			}
			if tt.err.Secondary && !strings.Contains(tt.err.Error(), "secondaire") {
				t.Errorf("Error() = %q, veut la mention de la limite secondaire", tt.err.Error())
			}
			wrapped := fmt.Errorf("liste des commits : %w", tt.err)
			if !errors.Is(wrapped, ErrRateLimited) {
				t.Error("errors.Is(err, ErrRateLimited) = false")
			}
			if errors.Is(wrapped, ErrForbidden) || errors.Is(wrapped, ErrUnavailable) {
				t.Error("RateLimitError ne doit satisfaire que ErrRateLimited")
			}
			var rl *RateLimitError
			if !errors.As(wrapped, &rl) || rl != tt.err {
				t.Error("errors.As(err, *RateLimitError) échoue")
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
	for _, want := range []string{"« ecole-dev »", "SSO", "gh auth refresh", "https://github.com/orgs/ecole-dev/sso"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, veut %q", msg, want)
		}
	}
	if strings.Contains(msg, "SECRETREQUEST") || strings.Contains(msg, "authorization_request") {
		t.Errorf("Error() reprend les paramètres du lien : %q", msg)
	}
	wrapped := fmt.Errorf("lecture du dépôt : %w", err)
	if !errors.Is(wrapped, ErrForbidden) {
		t.Error("errors.Is(err, ErrForbidden) = false")
	}
	if errors.Is(wrapped, ErrNotFound) || errors.Is(wrapped, ErrRateLimited) {
		t.Error("SSORequiredError ne doit satisfaire que ErrForbidden")
	}
	var sso *SSORequiredError
	if !errors.As(wrapped, &sso) || sso.Organization != "ecole-dev" {
		t.Error("errors.As(err, *SSORequiredError) échoue")
	}

	// Sans organisation ni lien exploitable.
	msg = (&SSORequiredError{URL: "javascript:alert(1)"}).Error()
	if !strings.Contains(msg, "de ce dépôt") || strings.Contains(msg, "javascript") || strings.Contains(msg, "ouvrez") {
		t.Errorf("Error() sans organisation = %q", msg)
	}
	msg = (&SSORequiredError{Organization: "x", URL: "https://user:pass@github.com/orgs/x/sso"}).Error()
	if strings.Contains(msg, "pass") {
		t.Errorf("Error() reprend des identifiants de l'URL : %q", msg)
	}
}

func TestAPIError(t *testing.T) {
	tests := []struct {
		err  *APIError
		want string
	}{
		{
			&APIError{Op: "lecture du dépôt alice/projet", Method: "GET", Path: "/repos/alice/projet", Status: 404, Err: ErrNotFound},
			"lecture du dépôt alice/projet (GET /repos/alice/projet, HTTP 404) : dépôt introuvable ou compte sans accès",
		},
		{
			&APIError{Op: "liste des branches de alice/projet", Method: "GET", Path: "/repos/alice/projet/branches", Err: ErrUnavailable},
			"liste des branches de alice/projet (GET /repos/alice/projet/branches) : " + ErrUnavailable.Error(),
		},
		{&APIError{Op: "état du quota", Err: ErrUnexpected}, "état du quota : " + ErrUnexpected.Error()},
		{&APIError{Status: 500}, "(HTTP 500)"},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("Error() = %q, veut %q", got, tt.want)
		}
	}

	// La cause reste accessible à travers plusieurs enveloppes.
	causes := []error{
		ErrNotFound,
		&RateLimitError{Reset: time.Unix(1_800_000_000, 0)},
		&SSORequiredError{Organization: "o"},
		context.Canceled,
	}
	for _, cause := range causes {
		err := fmt.Errorf("synchro : %w", &APIError{Op: "op", Method: "GET", Path: "/x", Status: 403, Err: cause})
		if !errors.Is(err, cause) {
			t.Errorf("errors.Is(%v) = false", cause)
		}
		var api *APIError
		if !errors.As(err, &api) || api.Status != 403 {
			t.Errorf("errors.As(*APIError) échoue pour %v", cause)
		}
	}
	err := fmt.Errorf("x : %w", &APIError{Op: "op", Err: &SSORequiredError{}})
	if !errors.Is(err, ErrForbidden) {
		t.Error("APIError > SSORequiredError doit satisfaire ErrForbidden")
	}
	err = &APIError{Op: "op", Err: &RateLimitError{}}
	if !errors.Is(err, ErrRateLimited) {
		t.Error("APIError > RateLimitError doit satisfaire ErrRateLimited")
	}
}
