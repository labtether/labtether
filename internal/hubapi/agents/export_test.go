package agents

// Limits the external tests (package agents_test) check against. cmd's
// agents_bridge.go keeps copies of these for its own tests.
const (
	MaxPendingHostnameIDLen           = maxPendingHostnameIDLen
	MaxPendingEnrollmentAgents        = maxPendingEnrollmentAgents
	MaxPendingEnrollmentPerIP         = maxPendingEnrollmentPerIP
	MaxPendingEnrollmentTimeout       = maxPendingEnrollmentTimeout
	MaxPendingEnrollmentProofMessages = maxPendingEnrollmentProofMessages
)
