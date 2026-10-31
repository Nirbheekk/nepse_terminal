package auth

import (
	"strings"
	"testing"
	"time"
)

func TestHashPasswordRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Fatalf("hash missing argon2id prefix: %q", hash)
	}
	if strings.Contains(hash, "correct horse") {
		t.Fatal("hash contains the plaintext password")
	}

	ok, err := VerifyPassword(hash, "correct horse battery staple")
	if err != nil || !ok {
		t.Fatalf("VerifyPassword(correct) = %v, %v; want true, nil", ok, err)
	}
	ok, err = VerifyPassword(hash, "correct horse battery stapl")
	if err != nil {
		t.Fatalf("VerifyPassword(wrong) error: %v", err)
	}
	if ok {
		t.Fatal("VerifyPassword accepted an incorrect password")
	}
}

// Two hashes of the same password must differ: a shared salt would let one
// rainbow table cover every account.
func TestHashPasswordUsesRandomSalt(t *testing.T) {
	a, err := HashPassword("same-password-123")
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashPassword("same-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two hashes of the same password are identical; salt is not random")
	}
	if ok, _ := VerifyPassword(b, "same-password-123"); !ok {
		t.Fatal("second hash does not verify")
	}
}

func TestVerifyPasswordRejectsMalformedHashes(t *testing.T) {
	for _, bad := range []string{
		"",
		"plaintext",
		"$argon2i$v=19$m=65536,t=2,p=4$c2FsdA$aGFzaA",
		"$argon2id$v=99$m=65536,t=2,p=4$c2FsdA$aGFzaA",
	} {
		if ok, err := VerifyPassword(bad, "anything"); ok || err == nil {
			t.Errorf("VerifyPassword(%q) = %v, %v; want false and an error", bad, ok, err)
		}
	}
}

func TestIssueAndParse(t *testing.T) {
	m := NewManager([]byte("0123456789abcdef0123456789abcdef"), "nepse-platform", time.Minute)
	token, exp, err := m.Issue("user-1", []string{"user", "admin"})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if time.Until(exp) > 2*time.Minute {
		t.Fatalf("expiry %v is further out than the configured TTL", exp)
	}

	claims, err := m.Parse(token)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if claims.Subject != "user-1" {
		t.Errorf("Subject = %q, want user-1", claims.Subject)
	}
	if len(claims.Roles) != 2 || claims.Roles[0] != "user" {
		t.Errorf("Roles = %v", claims.Roles)
	}
}

func TestParseRejectsWrongSecret(t *testing.T) {
	issuer := NewManager([]byte("0123456789abcdef0123456789abcdef"), "nepse-platform", time.Minute)
	token, _, err := issuer.Issue("user-1", []string{"user"})
	if err != nil {
		t.Fatal(err)
	}
	other := NewManager([]byte("ffffffffffffffffffffffffffffffff"), "nepse-platform", time.Minute)
	if _, err := other.Parse(token); err == nil {
		t.Fatal("a token signed with a different secret was accepted")
	}
}

func TestParseRejectsWrongIssuer(t *testing.T) {
	m := NewManager([]byte("0123456789abcdef0123456789abcdef"), "nepse-platform", time.Minute)
	token, _, err := m.Issue("user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	other := NewManager([]byte("0123456789abcdef0123456789abcdef"), "someone-else", time.Minute)
	if _, err := other.Parse(token); err == nil {
		t.Fatal("a token from a different issuer was accepted")
	}
}

func TestParseRejectsExpiredToken(t *testing.T) {
	m := NewManager([]byte("0123456789abcdef0123456789abcdef"), "nepse-platform", time.Minute)
	token, _, err := m.Issue("user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	// A manager with a negative TTL issues an already-expired token.
	expired := NewManager([]byte("0123456789abcdef0123456789abcdef"), "nepse-platform", -time.Minute)
	token, _, err = expired.Issue("user-1", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.Parse(token); err == nil {
		t.Fatal("an expired token was accepted")
	}
}

// alg=none and HS/RS confusion are the classic JWT bypasses; the parser pins
// the signing method explicitly.
func TestParseRejectsAlgNone(t *testing.T) {
	m := NewManager([]byte("0123456789abcdef0123456789abcdef"), "nepse-platform", time.Minute)
	unsigned := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.eyJzdWIiOiJhdHRhY2tlciIsImlzcyI6Im5lcHNlLXBsYXRmb3JtIn0."
	if _, err := m.Parse(unsigned); err == nil {
		t.Fatal("an unsigned (alg=none) token was accepted")
	}
}

func TestNewOpaqueTokenIsRandomAndHashed(t *testing.T) {
	a, digestA, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	b, digestB, err := NewOpaqueToken()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("two opaque tokens are identical")
	}
	if len(digestA) != 32 {
		t.Fatalf("digest length = %d, want 32 (SHA-256)", len(digestA))
	}
	if string(digestA) == string(digestB) {
		t.Fatal("two tokens produced the same digest")
	}
	if string(digestA) == a {
		t.Fatal("the stored digest is the token itself")
	}
	if string(HashSecret(a)) != string(digestA) {
		t.Fatal("HashSecret is not deterministic for the same token")
	}
}

func TestRandomHexLength(t *testing.T) {
	got, err := RandomHex(24)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 48 {
		t.Fatalf("RandomHex(24) produced %d chars, want 48", len(got))
	}
	other, _ := RandomHex(24)
	if got == other {
		t.Fatal("RandomHex returned the same value twice")
	}
}
