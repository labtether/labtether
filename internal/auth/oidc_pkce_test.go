package auth

import (
	"strings"
	"testing"
)

func TestPKCECodeChallengeS256RFC7636Example(t *testing.T) {
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"

	got, err := PKCECodeChallengeS256(verifier)
	if err != nil {
		t.Fatalf("PKCECodeChallengeS256 returned error: %v", err)
	}
	if got != want {
		t.Fatalf("challenge = %q, want %q", got, want)
	}
	if err := ValidatePKCECodeChallenge(got); err != nil {
		t.Fatalf("generated challenge should validate: %v", err)
	}
}

func TestValidatePKCECodeVerifier(t *testing.T) {
	valid43 := strings.Repeat("a", 43)
	valid128 := strings.Repeat("A0-._~", 21) + "A0"
	if len(valid128) != 128 {
		t.Fatalf("test verifier length = %d", len(valid128))
	}

	tests := []struct {
		name     string
		verifier string
		wantErr  bool
	}{
		{name: "minimum", verifier: valid43},
		{name: "maximum and all punctuation", verifier: valid128},
		{name: "too short", verifier: strings.Repeat("a", 42), wantErr: true},
		{name: "too long", verifier: strings.Repeat("a", 129), wantErr: true},
		{name: "space", verifier: strings.Repeat("a", 42) + " ", wantErr: true},
		{name: "padding", verifier: strings.Repeat("a", 42) + "=", wantErr: true},
		{name: "unicode", verifier: strings.Repeat("a", 42) + "é", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePKCECodeVerifier(test.verifier)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidatePKCECodeVerifier() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidatePKCECodeChallengeRejectsNonCanonicalValues(t *testing.T) {
	valid, err := PKCECodeChallengeS256(strings.Repeat("v", 43))
	if err != nil {
		t.Fatal(err)
	}
	for _, challenge := range []string{
		valid + "=",
		strings.Repeat("a", 42),
		strings.Repeat("a", 44),
		strings.Repeat("*", 43),
		strings.Repeat("A", 42) + "B", // non-zero base64 trailing bits
	} {
		if err := ValidatePKCECodeChallenge(challenge); err == nil {
			t.Fatalf("expected challenge %q to be rejected", challenge)
		}
	}
}
