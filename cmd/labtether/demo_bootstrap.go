package main

import (
	"fmt"
	"log"

	"github.com/labtether/labtether/internal/auth"
)

const demoUsername = "demo"
const demoPassword = "demo-viewer"

// bootstrapDemoUser creates a demo viewer user for the read-only demo instance.
// Refuses if non-demo users exist (prevents accidental activation on production).
func (s *apiServer) bootstrapDemoUser() error {
	// Safety: refuse if non-demo users already exist.
	users, err := s.authStore.ListUsers(10)
	if err != nil {
		return fmt.Errorf("demo bootstrap: list users: %w", err)
	}
	for _, u := range users {
		if u.Username != demoUsername {
			return fmt.Errorf("demo bootstrap: cannot enable demo mode — existing non-demo user %q detected", u.Username)
		}
	}

	// Check if demo user already exists.
	_, found, err := s.authStore.GetUserByUsername(demoUsername)
	if err != nil {
		return fmt.Errorf("demo bootstrap: get user: %w", err)
	}
	if found {
		log.Println("demo bootstrap: demo user already exists, skipping")
		return nil
	}

	// Create viewer user.
	hash, err := auth.HashPassword(demoPassword)
	if err != nil {
		return fmt.Errorf("demo bootstrap: hash: %w", err)
	}

	// Use CreateUserWithRole with RoleViewer, NOT CreateUser (which defaults to owner).
	_, err = s.authStore.CreateUserWithRole(demoUsername, hash, auth.RoleViewer, "local", "")
	if err != nil {
		return fmt.Errorf("demo bootstrap: create user: %w", err)
	}

	log.Println("demo bootstrap: created demo viewer user")
	return nil
}
