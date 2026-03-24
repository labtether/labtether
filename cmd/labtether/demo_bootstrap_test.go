package main

import (
	"testing"

	"github.com/labtether/labtether/internal/auth"
)

func TestBootstrapDemoUser_CreatesViewerUser(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = true

	err := s.bootstrapDemoUser()
	if err != nil {
		t.Fatalf("bootstrapDemoUser failed: %v", err)
	}

	user, found, err := s.authStore.GetUserByUsername("demo")
	if err != nil {
		t.Fatalf("GetUserByUsername failed: %v", err)
	}
	if !found {
		t.Fatal("demo user not created")
	}
	if user.Role != auth.RoleViewer {
		t.Fatalf("demo user should have role viewer, got %s", user.Role)
	}
}

func TestBootstrapDemoUser_Idempotent(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = true

	if err := s.bootstrapDemoUser(); err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if err := s.bootstrapDemoUser(); err != nil {
		t.Fatalf("second call should not fail: %v", err)
	}

	// Verify still only one user.
	users, err := s.authStore.ListUsers(10)
	if err != nil {
		t.Fatalf("ListUsers failed: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("expected 1 user after two bootstrap calls, got %d", len(users))
	}
}

func TestBootstrapDemoUser_RejectsExistingNonDemoUsers(t *testing.T) {
	s := newTestAPIServer(t)
	s.demoMode = true

	// Create a real admin user first.
	_, err := s.authStore.CreateUser("admin", "hashedpassword")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	err = s.bootstrapDemoUser()
	if err == nil {
		t.Fatal("bootstrapDemoUser should fail when non-demo users exist")
	}
}
