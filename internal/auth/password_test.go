package auth

import "testing"

func TestHashVerifyRoundTrip(t *testing.T) {
	hash, err := HashPassword("Sup3rSecret!pw")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	ok, err := VerifyPassword(hash, "Sup3rSecret!pw")
	if err != nil || !ok {
		t.Fatalf("expected match, got ok=%v err=%v", ok, err)
	}
	if ok, _ := VerifyPassword(hash, "wrong-password"); ok {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestHashUsesRandomSalt(t *testing.T) {
	a, _ := HashPassword("samepassword123")
	b, _ := HashPassword("samepassword123")
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	if _, err := VerifyPassword("not-a-valid-hash", "x"); err == nil {
		t.Fatal("expected error for malformed hash")
	}
}

func TestPasswordStrength(t *testing.T) {
	weak := []string{"short", "alllowercaseonly", "Ab1!", "nodigits-or-upper"}
	for _, w := range weak {
		if err := ValidatePasswordStrength(w); err == nil {
			t.Errorf("expected %q to be rejected", w)
		}
	}
	strong := []string{"CorrectHorse1!", "MyStr0ng-Pass", "anotherG00d#one"}
	for _, s := range strong {
		if err := ValidatePasswordStrength(s); err != nil {
			t.Errorf("expected %q to pass: %v", s, err)
		}
	}
}
