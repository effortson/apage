package totp

import (
	"testing"
	"time"
)

func TestCodeVerifyRoundTrip(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	code, err := Code(secret, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code must be 6 digits, got %q", code)
	}
	if !Verify(secret, code, now) {
		t.Error("a freshly computed code must verify")
	}
	if Verify(secret, "000000", now) && code != "000000" {
		t.Error("a wrong code must not verify")
	}
}

func TestVerifyClockSkew(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Unix(1_700_000_000, 0)
	// A code from the previous window must still verify (±1 step tolerance).
	prev, _ := Code(secret, now.Add(-30*time.Second))
	if !Verify(secret, prev, now) {
		t.Error("previous-window code should verify within skew tolerance")
	}
	// Two windows away must NOT verify.
	old, _ := Code(secret, now.Add(-90*time.Second))
	if old != prev && Verify(secret, old, now) {
		t.Error("a code two windows old must not verify")
	}
}

func TestDistinctSecrets(t *testing.T) {
	a, _ := GenerateSecret()
	b, _ := GenerateSecret()
	if a == b {
		t.Error("secrets must be random/distinct")
	}
	now := time.Unix(1_700_000_000, 0)
	ca, _ := Code(a, now)
	if Verify(b, ca, now) {
		t.Error("a code for secret A must not verify under secret B")
	}
}

func TestVerifyStepReturnsCounterAndIsStable(t *testing.T) {
	secret, _ := GenerateSecret()
	now := time.Unix(1_700_000_000, 0)
	code, _ := Code(secret, now)

	step, ok := VerifyStep(secret, code, now)
	if !ok {
		t.Fatal("freshly computed code must verify")
	}
	if want := now.Unix() / 30; step != want {
		t.Fatalf("step = %d, want %d", step, want)
	}
	// A code minted in the previous window matches the previous step, so a caller
	// keying replay protection on the step can distinguish the two windows.
	prevCode, _ := Code(secret, now.Add(-30*time.Second))
	prevStep, ok := VerifyStep(secret, prevCode, now)
	if !ok {
		t.Fatal("previous-window code must verify within skew tolerance")
	}
	if prevStep == step && prevCode != code {
		t.Error("distinct windows must yield distinct steps")
	}
	if _, ok := VerifyStep(secret, "000000", now); ok && code != "000000" {
		t.Error("a wrong code must not verify")
	}
}
