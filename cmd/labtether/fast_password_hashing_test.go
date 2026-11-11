package main

import "github.com/labtether/labtether/internal/auth"

// These tests hash and check passwords hundreds of times. At the production
// bcrypt cost under the race detector that took about five minutes in CI.
// internal/auth tests the production cost itself.
func init() {
	auth.UseFastPasswordHashingInTests()
}
