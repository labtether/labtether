package agents

import (
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

// The installer, the agent log and the Windows and Mac apps show these
// answers unchanged. One said "valid single-use device identity continuity
// proof is required". Every refusal must say what to do, in plain words.
func TestEnrollmentRefusalsArePlainAndSayWhatToDo(t *testing.T) {
	refused := []error{
		persistence.ErrEnrollmentTokenUsed, persistence.ErrEnrollmentTokenExpired,
		persistence.ErrEnrollmentTokenRevoked, persistence.ErrEnrollmentTokenInvalid,
		persistence.ErrEnrollmentTokenScopeMismatch, persistence.ErrAgentIdentityRetired,
		persistence.ErrAgentIdentityProofV2Required, persistence.ErrRecoveryRequiresSingleUseToken,
		persistence.ErrAgentIdentityContinuityConflict, persistence.ErrEnrollmentTokenPredatesRotation,
		persistence.ErrAgentFleetCapacityReached,
	}
	messages := []string{refusalBadRequest, refusalMissingKey, refusalBadHostname, refusalNoHostname,
		refusalBadProof, refusalServerError, refusalAdminRejected, refusalApproveNameUsed}
	for _, err := range refused {
		_, message, ok := enrollmentRefusal(err)
		if !ok {
			t.Fatalf("%v has no plain answer", err)
		}
		messages = append(messages, message)
	}
	internal := []string{"proof", "continuity", "identity", "operator", "asset id", "recovery", "normaliz", "_"}
	for _, message := range messages {
		lower := strings.ToLower(message)
		for _, word := range internal {
			if strings.Contains(lower, word) {
				t.Errorf("%q uses %q", message, word)
			}
		}
		if !strings.HasSuffix(message, ".") || len(strings.Fields(message)) < 6 {
			t.Errorf("%q is not a full sentence", message)
		}
	}
}
