package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShareCACertSkipsMissingDefaultDirWhenEnvUnset(t *testing.T) {
	unsetEnvForTest(t, "LABTETHER_CA_SHARE_DIR")

	baseDir := t.TempDir()
	defaultShareDir := filepath.Join(baseDir, "missing-ca-share")

	shareCACert([]byte("test-ca"), defaultShareDir)

	if _, err := os.Stat(defaultShareDir); !os.IsNotExist(err) {
		t.Fatalf("expected default missing share dir to remain absent, err=%v", err)
	}
}

func TestShareCACertWritesToExistingDefaultDirWhenEnvUnset(t *testing.T) {
	unsetEnvForTest(t, "LABTETHER_CA_SHARE_DIR")

	defaultShareDir := filepath.Join(t.TempDir(), "ca-share")
	if err := os.MkdirAll(defaultShareDir, 0755); err != nil {
		t.Fatalf("mkdir default share dir: %v", err)
	}

	want := []byte("test-ca-default")
	shareCACert(want, defaultShareDir)

	gotPath := filepath.Join(defaultShareDir, "ca.crt")
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read written CA cert: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("unexpected CA content: got=%q want=%q", string(got), string(want))
	}
}

func TestShareCACertUsesExplicitEnvPath(t *testing.T) {
	explicitShareDir := filepath.Join(t.TempDir(), "explicit-ca-share")
	t.Setenv("LABTETHER_CA_SHARE_DIR", explicitShareDir)

	want := []byte("test-ca-explicit")
	shareCACert(want, filepath.Join(t.TempDir(), "ignored-default"))

	gotPath := filepath.Join(explicitShareDir, "ca.crt")
	got, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("read written explicit CA cert: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("unexpected explicit CA content: got=%q want=%q", string(got), string(want))
	}
}

func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	originalValue, hadOriginal := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			if err := os.Setenv(key, originalValue); err != nil {
				t.Fatalf("restore %s: %v", key, err)
			}
			return
		}
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("cleanup unset %s: %v", key, err)
		}
	})
}
