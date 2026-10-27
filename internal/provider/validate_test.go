package provider

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateOwner(t *testing.T) {
	valid := []string{"a", "alice", "Alice-Martin", "promo-2026", "0", strings.Repeat("a", 39), "a-b-c"}
	for _, owner := range valid {
		if err := ValidateOwner(owner); err != nil {
			t.Errorf("ValidateOwner(%q) = %v, veut nil", owner, err)
		}
	}
	invalid := []string{
		"", "-alice", "alice/bob", "alice bob", "alice_bob", "..", ".", "alice.bob", "élève",
		strings.Repeat("a", 40), "alice\n", "alice%2F", "a?b", "a#b",
	}
	for _, owner := range invalid {
		if err := ValidateOwner(owner); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateOwner(%q) = %v, veut ErrInvalidRef", owner, err)
		}
	}
}

func TestValidateName(t *testing.T) {
	valid := []string{"projet", "Projet-Web_2026", "a.b", ".github", "...", "x", strings.Repeat("r", 100), "-tiret"}
	for _, name := range valid {
		if err := ValidateName(name); err != nil {
			t.Errorf("ValidateName(%q) = %v, veut nil", name, err)
		}
	}
	invalid := []string{
		"", ".", "..", "a/b", "../x", "a b", strings.Repeat("r", 101), "projet\x00", "é", "a%2e", "a?b",
	}
	for _, name := range invalid {
		if err := ValidateName(name); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateName(%q) = %v, veut ErrInvalidRef", name, err)
		}
	}
}

func TestValidateRepo(t *testing.T) {
	if err := ValidateRepo("alice", "projet"); err != nil {
		t.Errorf("ValidateRepo valide : %v", err)
	}
	err := ValidateRepo("-alice", "projet")
	if !errors.Is(err, ErrInvalidRef) || !strings.Contains(err.Error(), "propriétaire") {
		t.Errorf("ValidateRepo, owner invalide : %v", err)
	}
	err = ValidateRepo("alice", "..")
	if !errors.Is(err, ErrInvalidRef) || !strings.Contains(err.Error(), "nom de dépôt") {
		t.Errorf("ValidateRepo, nom invalide : %v", err)
	}
}

func TestValidateSHA(t *testing.T) {
	sha1 := strings.Repeat("0123456789", 4)
	sha256 := strings.Repeat("abcdef0123456789", 4)
	for _, sha := range []string{sha1, sha256, strings.Repeat("f", 40)} {
		if err := ValidateSHA(sha); err != nil {
			t.Errorf("ValidateSHA(%q) = %v, veut nil", sha, err)
		}
	}
	invalid := []string{
		"", "abc1234", strings.ToUpper(strings.Repeat("ab", 20)), sha1[:39], sha1 + "0",
		strings.Repeat("g", 40), sha1[:39] + "\n", "main", strings.Repeat("a", 63), sha256 + "a",
	}
	for _, sha := range invalid {
		if err := ValidateSHA(sha); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateSHA(%q) = %v, veut ErrInvalidRef", sha, err)
		}
	}
}

func TestValidateBranch(t *testing.T) {
	valid := []string{"main", "dev", "feature/connexion", "release/1.2", "élève", "a b", "dependabot/npm_and_yarn/x-1.0",
		strings.Repeat("b", MaxBranchBytes), strings.Repeat("0123456789", 4)}
	for _, branch := range valid {
		if err := ValidateBranch(branch); err != nil {
			t.Errorf("ValidateBranch(%q) = %v, veut nil", branch, err)
		}
	}
	invalid := []string{"", "main\n", "a\tb", "x\x00", "\x7f", "a\u0085b", "\xff\xfe", strings.Repeat("b", MaxBranchBytes+1)}
	for _, branch := range invalid {
		if err := ValidateBranch(branch); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateBranch(%q) = %v, veut ErrInvalidRef", branch, err)
		}
	}
}

func TestValidateID(t *testing.T) {
	if err := ValidateID(1); err != nil {
		t.Errorf("ValidateID(1) = %v", err)
	}
	for _, id := range []int64{0, -1} {
		if err := ValidateID(id); !errors.Is(err, ErrInvalidRef) {
			t.Errorf("ValidateID(%d) = %v, veut ErrInvalidRef", id, err)
		}
	}
}

// TestValidationMessages vérifie que la valeur citée est échappée et raccourcie : un message d'erreur
// ne doit ni casser une ligne de log ni recopier une entrée démesurée.
func TestValidationMessages(t *testing.T) {
	err := ValidateName("a\nb")
	if err == nil || strings.ContainsAny(err.Error(), "\n") || !strings.Contains(err.Error(), `"a\nb"`) {
		t.Errorf("message avec saut de ligne : %v", err)
	}
	long := strings.Repeat("x", 500) + "/"
	err = ValidateName(long)
	if err == nil || strings.Contains(err.Error(), strings.Repeat("x", maxQuotedRunes+1)) || !strings.Contains(err.Error(), "…") {
		t.Errorf("message avec une valeur longue : %v", err)
	}
	err = ValidateBranch(strings.Repeat("b", MaxBranchBytes+1))
	if err == nil || strings.Contains(err.Error(), "bbbb") {
		t.Errorf("nom de branche trop long recopié : %v", err)
	}
	err = ValidateBranch("\xff")
	if err == nil || !strings.Contains(err.Error(), "invalide") {
		t.Errorf("UTF-8 invalide : %v", err)
	}
}

func TestShortSHA(t *testing.T) {
	tests := map[string]string{"": "", "abc": "abc", "0123456": "0123456", strings.Repeat("0123456789", 4): "0123456"}
	for in, want := range tests {
		if got := ShortSHA(in); got != want {
			t.Errorf("ShortSHA(%q) = %q, veut %q", in, got, want)
		}
	}
}

func TestRepositoryFullName(t *testing.T) {
	if got := (Repository{Owner: "alice", Name: "projet"}).FullName(); got != "alice/projet" {
		t.Errorf("FullName = %q", got)
	}
}

func TestValidLogin(t *testing.T) {
	for login, want := range map[string]bool{
		"etudiant-test":         true,
		"A1":                    true,
		strings.Repeat("a", 39): true,
		"":                      false,
		strings.Repeat("a", 40): false,
		"-tiret":                false,
		"dependabot[bot]":       false,
		"a/b":                   false,
		"../x":                  false,
		"a b":                   false,
		"élève":                 false,
		"x?y=1":                 false,
	} {
		if got := ValidLogin(login); got != want {
			t.Errorf("ValidLogin(%q) = %v, veut %v", login, got, want)
		}
	}
}
