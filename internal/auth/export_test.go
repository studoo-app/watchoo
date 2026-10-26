package auth

// Valeurs factices du faux gh exposées au package de test externe auth_test, qui vérifie qu'elles
// concordent avec celles de authtest (les tests internes ne peuvent pas importer authtest).
const (
	FakeTokenForTest = fakeToken
	FakeLoginForTest = fakeLogin
)
