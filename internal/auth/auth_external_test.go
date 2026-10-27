package auth_test

import (
	"testing"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
)

// Le faux gh et le Fake en mémoire doivent utiliser les mêmes valeurs factices, pour que la
// vérification « le faux token n'apparaît jamais » porte sur la même chaîne partout.
func TestFakeValuesMatchAuthtest(t *testing.T) {
	if auth.FakeTokenForTest != authtest.FakeToken {
		t.Errorf("faux token du faux gh = %q, authtest.FakeToken = %q", auth.FakeTokenForTest, authtest.FakeToken)
	}
	if auth.FakeLoginForTest != authtest.FakeLogin {
		t.Errorf("faux login du faux gh = %q, authtest.FakeLogin = %q", auth.FakeLoginForTest, authtest.FakeLogin)
	}
}

// Les deux implémentations satisfont l'interface, en valeur comme en pointeur pour GH.
var (
	_ auth.Client = auth.GH{}
	_ auth.Client = &auth.GH{}
	_ auth.Client = authtest.NewLoggedIn("")
)
