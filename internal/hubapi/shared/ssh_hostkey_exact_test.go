package shared

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHHostKeyPinsAreCaseSensitive(t *testing.T) {
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ssh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	for _, pin := range []string{ssh.FingerprintSHA256(key), base64.StdEncoding.EncodeToString(key.Marshal())} {
		callback, err := BuildSSHHostKeyCallback(true, pin)
		if err != nil || callback("host:22", nil, key) != nil {
			t.Fatalf("correct pin rejected: %v", err)
		}
		// Flip a letter in the key bytes, leaving the SHA256: prefix intact.
		start := strings.Index(pin, ":") + 1
		changed := []byte(pin)
		for i := start; i < len(changed); i++ {
			if changed[i] >= 'a' && changed[i] <= 'z' {
				changed[i] -= 'a' - 'A'
				break
			}
			if changed[i] >= 'A' && changed[i] <= 'Z' {
				changed[i] += 'a' - 'A'
				break
			}
		}
		callback, err = BuildSSHHostKeyCallback(true, string(changed))
		if err != nil {
			t.Fatal(err)
		}
		if callback("host:22", nil, key) == nil {
			t.Fatal("case-altered host key pin accepted")
		}
	}
}
