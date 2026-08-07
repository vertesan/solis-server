package utils

import "testing"

func TestRandomHexStringRejectsInvalidLength(t *testing.T) {
	if _, err := RandomHexString(0); err == nil {
		t.Fatal("expected zero length to return an error")
	}
}

func TestRandomHexStringReturnsRequestedLength(t *testing.T) {
	value, err := RandomHexString(17)
	if err != nil {
		t.Fatalf("RandomHexString returned an error: %v", err)
	}
	if len(value) != 17 {
		t.Fatalf("RandomHexString returned %d characters, want 17", len(value))
	}
}
