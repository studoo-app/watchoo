package auth

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTokenNeverLeaks fait échouer gh de toutes les manières connues et vérifie que le faux token
// (et ses variantes gho_fake_for_tests_*) n'apparaît dans aucun message d'erreur, aucun Status
// renvoyé ni aucun log. Le faux gh place volontairement le token dans sa sortie d'erreur (scénario
// error) et dans une sortie standard inattendue (scénario garbage).
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
			errs = append(errs, fmt.Errorf("%s : %w", label, err))
		}
		for _, s := range []string{fmt.Sprintf("%+v", st), fmt.Sprintf("%#v", st)} {
			if strings.Contains(s, fakeToken) {
				t.Errorf("%s : le Status renvoyé contient le token : %s", label, s)
			}
		}
	}

	for _, scenario := range scenarios {
		for _, user := range users {
			if scenario == "hang" && user != "" {
				continue // un seul passage suffit, chaque appel attend le délai
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

	// gh absent et contexte annulé.
	missing := GH{Path: filepath.Join(t.TempDir(), "absent")}
	_, err := missing.Token(context.Background())
	check("absent/Token", Status{}, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = fakeGH().Token(ctx)
	check("annulé/Token", Status{}, err)

	if len(errs) < 10 {
		t.Fatalf("seulement %d erreurs obtenues : les scénarios d'échec n'ont pas tourné", len(errs))
	}
	for _, err := range errs {
		for _, s := range []string{err.Error(), fmt.Sprintf("%+v", err), fmt.Sprintf("%#v", err)} {
			if strings.Contains(s, fakeToken) {
				t.Errorf("le token apparaît dans une erreur : %s", s)
			}
		}
	}

	logs := testLogs.String()
	if !strings.Contains(logs, "appel de gh") {
		t.Fatal("aucun log capturé : la vérification des logs ne prouve rien")
	}
	if strings.Contains(logs, fakeToken) {
		t.Errorf("le token apparaît dans les logs :\n%s", logs)
	}
}
