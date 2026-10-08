package notifications

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSignAPNsJWT(t *testing.T) {
	// Generate a test ECDSA P-256 key.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	now := time.Date(2026, 3, 2, 12, 0, 0, 0, time.UTC)
	token, err := signAPNsJWT(key, "KEYID12345", "TEAMID1234", now)
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	// Verify structure: three dot-separated parts.
	parts := splitJWT(token)
	if parts == nil {
		t.Fatal("JWT should have 3 parts")
	}

	// Decode and verify header.
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header map[string]string
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header["alg"] != "ES256" {
		t.Fatalf("expected alg ES256, got %s", header["alg"])
	}
	if header["kid"] != "KEYID12345" {
		t.Fatalf("expected kid KEYID12345, got %s", header["kid"])
	}

	// Decode and verify claims.
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["iss"] != "TEAMID1234" {
		t.Fatalf("expected iss TEAMID1234, got %v", claims["iss"])
	}
	iat, ok := claims["iat"].(float64)
	if !ok || int64(iat) != now.Unix() {
		t.Fatalf("expected iat %d, got %v", now.Unix(), claims["iat"])
	}

	// Verify the signature.
	if !verifyAPNsJWT(token, &key.PublicKey) {
		t.Fatal("JWT signature verification failed")
	}
}

func TestSignAPNsJWT_NilKey(t *testing.T) {
	_, err := signAPNsJWT(nil, "KEY", "TEAM", time.Now())
	if err == nil {
		t.Fatal("expected error for nil key")
	}
}

func TestSignAPNsJWTRejectsNonP256Key(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		t.Fatalf("generate P-384 key: %v", err)
	}
	if _, err := signAPNsJWT(key, "KEYID12345", "TEAMID1234", time.Now()); err == nil {
		t.Fatal("non-P-256 APNs key unexpectedly accepted as ES256")
	}
}

func TestAPNsAuthKeyReadErrorDoesNotExposeHostPath(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "private-host-layout", "AuthKey.p8")
	err := (&APNsAdapter{}).ensureAuthKey(secretPath)
	if err == nil {
		t.Fatal("missing APNs key unexpectedly loaded")
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(err.Error(), "private-host-layout") {
		t.Fatalf("APNs auth error exposed host path: %v", err)
	}
}

func TestAPNsAuthKeyRejectsNonRegularAndSymlinkPaths(t *testing.T) {
	directory := t.TempDir()
	tests := []struct {
		name string
		path string
	}{
		{name: "directory", path: directory},
	}

	target := filepath.Join(directory, "target.p8")
	if err := os.WriteFile(target, []byte("not-a-key"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	symlink := filepath.Join(directory, "linked.p8")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	tests = append(tests, struct {
		name string
		path string
	}{name: "symlink", path: symlink})

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := (&APNsAdapter{}).ensureAuthKey(test.path)
			if err == nil || !strings.Contains(err.Error(), "unavailable or invalid") {
				t.Fatalf("expected bounded non-regular-file error, got %v", err)
			}
			if strings.Contains(err.Error(), test.path) {
				t.Fatalf("APNs auth error exposed host path: %v", err)
			}
		})
	}
}

func TestAPNsAuthKeyRejectsOversizedFileBeforeParsing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.p8")
	if err := os.WriteFile(path, make([]byte, apnsMaxAuthKeyBytes+1), 0o600); err != nil {
		t.Fatalf("write oversized key: %v", err)
	}
	err := (&APNsAdapter{}).ensureAuthKey(path)
	if err == nil || !strings.Contains(err.Error(), "unavailable or invalid") {
		t.Fatalf("expected oversized-file rejection, got %v", err)
	}
	if strings.Contains(err.Error(), path) {
		t.Fatalf("APNs auth error exposed host path: %v", err)
	}
}

func TestEnsureAPNsJWTCacheIsScopedToKeyAndTeamIdentity(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	adapter := &APNsAdapter{authKey: key}

	first, err := adapter.ensureJWT("KEYID00001", "TEAMID0001")
	if err != nil {
		t.Fatalf("first JWT: %v", err)
	}
	cached, err := adapter.ensureJWT("KEYID00001", "TEAMID0001")
	if err != nil {
		t.Fatalf("cached JWT: %v", err)
	}
	if first != cached {
		t.Fatal("unchanged APNs identity should reuse the cached JWT")
	}

	changedKey, err := adapter.ensureJWT("KEYID00002", "TEAMID0001")
	if err != nil {
		t.Fatalf("changed-key JWT: %v", err)
	}
	if changedKey == cached || adapter.jwtKeyID != "KEYID00002" {
		t.Fatal("changing the APNs key ID must invalidate the cached JWT")
	}

	changedTeam, err := adapter.ensureJWT("KEYID00002", "TEAMID0002")
	if err != nil {
		t.Fatalf("changed-team JWT: %v", err)
	}
	if changedTeam == changedKey || adapter.jwtTeamID != "TEAMID0002" {
		t.Fatal("changing the APNs team ID must invalidate the cached JWT")
	}
}

func TestAPNsAuthorizationJWTKeySelectionIsAtomicAcrossChannels(t *testing.T) {
	t.Parallel()

	type channelKey struct {
		path string
		key  *ecdsa.PrivateKey
		kid  string
		team string
	}
	writeKey := func(name string) channelKey {
		t.Helper()
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("generate %s: %v", name, err)
		}
		encoded, err := x509.MarshalPKCS8PrivateKey(key)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		path := filepath.Join(t.TempDir(), name+".p8")
		if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: encoded}), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		return channelKey{path: path, key: key, kid: name + "KEYID", team: name + "TEAMID"}
	}

	channels := []channelKey{writeKey("FIRST"), writeKey("SECOND")}
	adapter := &APNsAdapter{}
	errCh := make(chan error, 80)
	var wg sync.WaitGroup
	for index := 0; index < 80; index++ {
		channel := channels[index%len(channels)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := adapter.authorizationJWT(channel.path, channel.kid, channel.team)
			if err != nil {
				errCh <- err
				return
			}
			if !verifyAPNsJWT(token, &channel.key.PublicKey) {
				errCh <- fmt.Errorf("JWT was not signed by the selected channel key")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func TestSplitJWT_Invalid(t *testing.T) {
	if splitJWT("no-dots") != nil {
		t.Fatal("expected nil for input without dots")
	}
	if splitJWT("one.dot") != nil {
		t.Fatal("expected nil for input with one dot")
	}
	if splitJWT("too.many.dots.here") != nil {
		t.Fatal("expected nil for input with three dots")
	}
}
