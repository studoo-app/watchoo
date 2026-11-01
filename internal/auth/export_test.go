package auth

// Dummy values of the fake gh exposed to the external test package auth_test, which checks that they
// match those of authtest (the internal tests cannot import authtest).
const (
	FakeTokenForTest = fakeToken
	FakeLoginForTest = fakeLogin
)
