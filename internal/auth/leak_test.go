package auth

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTokenNeverLeaks makes gh fail in every known way and checks that the fake token (and its
// variants gho_fake_for_tests_*) appears in no error message, no returned Status and no log. The
// fake gh deliberately puts the token in its error output (error scenario) and in an unexpected
// standard output (garbage scenario).
func TestTokenNeverLeaks(t *testing.T) {
	scenarios := []string{
		"logged-in", "not-logged-in", "exit-not-logged-in", "multi-account", "env-token",
		"env-token-only", "invalid-token", "offline", "token-rotation", "empty-output", "garbage",
		"hang", "error",
	}
	users := []string{"", fakeLogin, otherLogin, strings.ToUpper(otherLogin), "inconnu"}

	var errs []error
	check := func(label string, st Status, err error) {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", label, err))
		}
		for _, s := range []string{fmt.Sprintf("%+v", st), fmt.Sprintf("%#v", st)} {
			if strings.Contains(s, fakeToken) {
				t.Errorf("%s: the returned Status contains the token: %s", label, s)
			}
		}
	}

	for _, scenario := range scenarios {
		for _, user := range users {
			if scenario == "hang" && user != "" {
				continue // a single pass is enough, every call waits for the timeout
			}
			t.Run(scenario+"/user="+user, func(t *testing.T) {
				useScenario(t, scenario)
				g := fakeGH()
				g.User = user
				if scenario == "hang" {
					g.Timeout = 300 * time.Millisecond
				}
				st, err := g.Status(context.Background())
				check(scenario+"/Status", st, err)
				_, err = g.Token(context.Background())
				check(scenario+"/Token", Status{}, err)
			})
		}
	}

	// gh missing and canceled context.
	missing := GH{Path: filepath.Join(t.TempDir(), "absent")}
	_, err := missing.Token(context.Background())
	check("absent/Token", Status{}, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = fakeGH().Token(ctx)
	check("canceled/Token", Status{}, err)

	if len(errs) < 10 {
		t.Fatalf("only %d errors obtained: the failure scenarios did not run", len(errs))
	}
	for _, err := range errs {
		for _, s := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
			if strings.Contains(s, fakeToken) {
				t.Errorf("the token appears in an error: %s", s)
			}
		}
	}

	logs := testLogs.String()
	if !strings.Contains(logs, "appel de gh") { // message of the Debug log of every call to gh
		t.Fatal("no log captured: the check of the logs proves nothing")
	}
	if strings.Contains(logs, fakeToken) {
		t.Errorf("the token appears in the logs:\n%s", logs)
	}
}
