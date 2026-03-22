package apikeys

import (
	"strings"
	"testing"
)

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey() error: %v", err)
	}
	if !strings.HasPrefix(key.Raw, "lt_") {
		t.Errorf("key should start with lt_, got %q", key.Raw)
	}
	if key.Prefix == "" {
		t.Error("prefix should not be empty")
	}
	if len(key.Prefix) != 4 {
		t.Errorf("prefix should be 4 chars, got %d", len(key.Prefix))
	}
	if key.Hash == "" {
		t.Error("hash should not be empty")
	}
	key2, _ := GenerateKey()
	if key.Raw == key2.Raw {
		t.Error("two generated keys should differ")
	}
}

func TestHashKey(t *testing.T) {
	hash1 := HashKey("lt_abcd_somesecret")
	hash2 := HashKey("lt_abcd_somesecret")
	if hash1 != hash2 {
		t.Error("same input should produce same hash")
	}
	hash3 := HashKey("lt_abcd_different")
	if hash1 == hash3 {
		t.Error("different input should produce different hash")
	}
}

func TestExtractPrefix(t *testing.T) {
	prefix, ok := ExtractPrefix("lt_a1b2_x8k9m2p4q7rest")
	if !ok {
		t.Fatal("should extract prefix from valid key")
	}
	if prefix != "a1b2" {
		t.Errorf("expected a1b2, got %s", prefix)
	}
	_, ok = ExtractPrefix("notakey")
	if ok {
		t.Error("should return false for invalid format")
	}
	_, ok = ExtractPrefix("lt_short")
	if ok {
		t.Error("should return false for missing second underscore")
	}
}

func TestIsAPIKeyFormat(t *testing.T) {
	if !IsAPIKeyFormat("lt_a1b2_something") {
		t.Error("should recognize valid format")
	}
	if IsAPIKeyFormat("Bearer sometoken") {
		t.Error("should reject non-lt_ tokens")
	}
	if IsAPIKeyFormat("") {
		t.Error("should reject empty string")
	}
}
