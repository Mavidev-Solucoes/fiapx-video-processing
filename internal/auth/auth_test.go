package auth

import (
	"testing"
	"time"
)

const sampleID = "d80926ec-fb70-4d17-82a6-ae6efef1c854"

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("this-is-a-long-password")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "this-is-a-long-password" || !CheckPassword(hash, "this-is-a-long-password") {
		t.Fatal("password hash did not verify")
	}
	if CheckPassword(hash, "a-different-password") {
		t.Fatal("wrong password was accepted")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected short password rejection")
	}
	if _, err := HashPassword(string(make([]byte, 73))); err == nil {
		t.Fatal("expected bcrypt length rejection")
	}
}

func TestSignedTokensExpireAndRejectTampering(t *testing.T) {
	manager, err := NewTokens("a-secret-longer-than-thirty-two-characters-only-for-tests")
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	manager.now = func() time.Time { return fixed }
	token, err := manager.Issue(sampleID)
	if err != nil {
		t.Fatal(err)
	}
	subject, err := manager.Verify(token)
	if err != nil || subject != sampleID {
		t.Fatalf("token verification: subject %q, err %v", subject, err)
	}
	if _, err := manager.Verify(token + "x"); err == nil {
		t.Fatal("modified signature accepted")
	}
	manager.now = func() time.Time { return fixed.Add(61 * time.Minute) }
	if _, err := manager.Verify(token); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestSecretIsRequired(t *testing.T) {
	if _, err := NewTokens("short"); err == nil {
		t.Fatal("expected short JWT secret rejection")
	}
}
