package agents

import (
	"errors"
	"net/http"

	"github.com/labtether/labtether/internal/persistence"
)

// The installer and the agent log show these words to the person adding a
// device, so each one says what happened and what to do, in plain text (no
// quotes or symbols that JSON would escape). "Devices, Add device" is where
// an admin makes a new token.
const (
	refusalTokenUnknown  = "LabTether does not know this enrollment token. Check that it was copied whole, or make a new one in Devices, Add device."
	refusalTokenUsed     = "This enrollment token was already used. Make a new one in Devices, Add device."
	refusalTokenExpired  = "This enrollment token expired. Make a new one in Devices, Add device."
	refusalTokenRevoked  = "This enrollment token was revoked. Make a new one in Devices, Add device."
	refusalTokenScope    = "This enrollment token was made for another device or group. Make a new one for this device in Devices, Add device."
	refusalRemovedDevice = "This device was removed from LabTether. Only a new single-use enrollment token made after the removal can add it back. Make one in Devices, Add device."
	refusalNameTaken     = "A device with this name is already enrolled. Reinstall over its agent to keep it, or remove that device in LabTether first."
	refusalTokenTooOld   = "This enrollment token was made before the device last enrolled. Make a new one in Devices, Add device."
	refusalFleetFull     = "LabTether has reached its device limit. Remove a device to add this one."
)

// enrollmentRefusal maps an enrollment failure to the answer the agent shows.
// ok is false for failures that are not the person's to fix (server errors).
func enrollmentRefusal(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, persistence.ErrEnrollmentTokenUsed):
		return http.StatusUnauthorized, refusalTokenUsed, true
	case errors.Is(err, persistence.ErrEnrollmentTokenExpired):
		return http.StatusUnauthorized, refusalTokenExpired, true
	case errors.Is(err, persistence.ErrEnrollmentTokenRevoked):
		return http.StatusUnauthorized, refusalTokenRevoked, true
	case errors.Is(err, persistence.ErrEnrollmentTokenInvalid):
		return http.StatusUnauthorized, refusalTokenUnknown, true
	case errors.Is(err, persistence.ErrEnrollmentTokenScopeMismatch):
		return http.StatusUnauthorized, refusalTokenScope, true
	case errors.Is(err, persistence.ErrAgentIdentityRetired):
		return http.StatusConflict, refusalRemovedDevice, true
	case errors.Is(err, persistence.ErrAgentIdentityProofV2Required),
		errors.Is(err, persistence.ErrRecoveryRequiresSingleUseToken),
		errors.Is(err, persistence.ErrAgentIdentityContinuityConflict):
		return http.StatusConflict, refusalNameTaken, true
	case errors.Is(err, persistence.ErrEnrollmentTokenPredatesRotation):
		return http.StatusConflict, refusalTokenTooOld, true
	case errors.Is(err, persistence.ErrAgentFleetCapacityReached):
		return http.StatusConflict, refusalFleetFull, true
	}
	return 0, "", false
}
