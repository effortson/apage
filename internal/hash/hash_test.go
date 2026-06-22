package hash

import "testing"

func TestPasswordRoundTrip(t *testing.T) {
	enc, err := Password("correct-horse-battery-staple-1")
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPassword("correct-horse-battery-staple-1", enc)
	if err != nil || !ok {
		t.Fatalf("expected match, got ok=%v err=%v", ok, err)
	}
	bad, _ := VerifyPassword("wrong", enc)
	if bad {
		t.Fatal("expected mismatch for wrong password")
	}
}

func TestSecretHashConstantTime(t *testing.T) {
	h := SecretHash("aps_abc123")
	if !SecretEqual("aps_abc123", h) {
		t.Fatal("expected secret to match its hash")
	}
	if SecretEqual("aps_other", h) {
		t.Fatal("expected mismatch for different secret")
	}
	if SecretEqual("aps_abc123", "not-hex") {
		t.Fatal("invalid stored hash must not match")
	}
}
