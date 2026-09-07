package email

import (
	"errors"
	"testing"
	"time"
)

func TestUnsubscribeTokenRoundTrip(t *testing.T) {
	const secret = "s3cr3t-key"
	now := time.Unix(1_700_000_000, 0)
	tok := SignUnsubscribe(secret, "user@example.com", now)

	got, err := VerifyUnsubscribe(secret, tok, now)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if got != "user@example.com" {
		t.Fatalf("email: got %q, want %q", got, "user@example.com")
	}
}

func TestUnsubscribeTokenWrongSecret(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := SignUnsubscribe("secret-a", "u@e.com", now)
	if _, err := VerifyUnsubscribe("secret-b", tok, now); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("got %v, want ErrInvalidToken", err)
	}
}

func TestUnsubscribeTokenTampered(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tok := SignUnsubscribe("secret", "u@e.com", now)
	repl := byte('A')
	if tok[len(tok)-1] == 'A' {
		repl = 'B'
	}
	bad := tok[:len(tok)-1] + string(repl)
	if _, err := VerifyUnsubscribe("secret", bad, now); err == nil {
		t.Fatal("tampered token verified; want error")
	}
}

func TestUnsubscribeTokenExpired(t *testing.T) {
	issued := time.Unix(1_700_000_000, 0)
	tok := SignUnsubscribe("secret", "u@e.com", issued)
	later := issued.Add(UnsubscribeTokenMaxAge + time.Hour)
	if _, err := VerifyUnsubscribe("secret", tok, later); !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("got %v, want ErrExpiredToken", err)
	}
}
