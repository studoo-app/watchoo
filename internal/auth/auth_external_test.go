package auth_test

import (
	"testing"

	"github.com/studoo-app/watchoo/internal/auth"
	"github.com/studoo-app/watchoo/internal/auth/authtest"
)

// The fake gh and the in-memory Fake must use the same dummy values, so that the check "the fake
// token never appears" is about the same string everywhere.
func TestFakeValuesMatchAuthtest(t *testing.T) {
	if auth.FakeTokenForTest != authtest.FakeToken {
		t.Errorf("fake token of the fake gh = %q, authtest.FakeToken = %q", auth.FakeTokenForTest, authtest.FakeToken)
	}
	if auth.FakeLoginForTest != authtest.FakeLogin {
		t.Errorf("fake login of the fake gh = %q, authtest.FakeLogin = %q", auth.FakeLoginForTest, authtest.FakeLogin)
	}
}

// Both implementations satisfy the interface, as a value and as a pointer for GH.
var (
	_ auth.Client = auth.GH{}
	_ auth.Client = &auth.GH{}
	_ auth.Client = authtest.NewLoggedIn("")
)
