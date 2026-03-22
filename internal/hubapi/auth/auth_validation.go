package auth

import "errors"

// LoginRequest holds the credentials for a login attempt.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` // #nosec G117 -- Login request carries runtime credential material.
}

const MinPasswordLength = 8

// ValidateLoginRequest validates the login credentials.
func ValidateLoginRequest(req LoginRequest) error {
	if req.Username == "" {
		return errors.New("username is required")
	}
	if len(req.Username) > 64 {
		return errors.New("username exceeds max length 64")
	}
	if req.Password == "" {
		return errors.New("password is required")
	}
	if len(req.Password) < MinPasswordLength {
		return errors.New("password must be at least 8 characters")
	}
	if len(req.Password) > 256 {
		return errors.New("password exceeds max length 256")
	}
	return nil
}
