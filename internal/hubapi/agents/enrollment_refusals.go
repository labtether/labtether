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
	refusalUnknownKey      = "LabTether does not know this enrollment token. Check that it was copied whole, or make a new one in Devices, Add device."
	refusalUsedKey         = "This enrollment token was already used. Make a new one in Devices, Add device."
	refusalExpiredKey      = "This enrollment token expired. Make a new one in Devices, Add device."
	refusalRevokedKey      = "This enrollment token was revoked. Make a new one in Devices, Add device."
	refusalOtherPlacement  = "This enrollment token was made for another device or group. Make a new one for this device in Devices, Add device."
	refusalRemovedDevice   = "This device was removed from LabTether. Only a new single-use enrollment token made after the removal can add it back. Make one in Devices, Add device."
	refusalNameTaken       = "A device with this name is already in LabTether, and this machine's agent key does not match it. Remove that device in LabTether first, or rename this machine. Then make a new single-use token in Devices, Add device."
	refusalNeedsSingleUse  = "A device with this name is already in LabTether. To connect it again, use a single-use token. Make one in Devices, Add device."
	refusalAgentTooOld     = "A device with this name is already in LabTether, and this agent is too old to reconnect it. Install the current agent with a new single-use token from Devices, Add device."
	refusalBadProof        = "LabTether could not check this machine's agent key. Run the install command from Devices, Add device again."
	refusalNoHostname      = "This machine's hostname has no letters or digits LabTether can use. Set a hostname, then run the install command again."
	refusalServerError     = "LabTether could not add this device because of a server error. Run the install command again."
	refusalBadRequest      = "LabTether could not read this agent's request. Install the current agent with the command from Devices, Add device."
	refusalMissingKey      = "No enrollment token was given. Make one in Devices, Add device, then run the install command with it."
	refusalBadHostname     = "This machine's hostname cannot be used: it must be at most 253 bytes, with no control characters. Set another hostname, then run the install command again."
	refusalAdminRejected   = "An admin rejected this device in LabTether."
	refusalApproveNameUsed = "A device with this name is already in LabTether. Reinstall over that device's agent to keep it, or remove that device first."
	refusalBeforeRotation  = "This enrollment token was made before the device last enrolled. Make a new one in Devices, Add device."
	refusalFleetFull       = "LabTether has reached its device limit. Remove a device to add this one."
)

// enrollmentRefusal maps an enrollment failure to the answer the agent shows.
// ok is false for failures that are not the person's to fix (server errors).
func enrollmentRefusal(err error) (status int, message string, ok bool) {
	switch {
	case errors.Is(err, persistence.ErrEnrollmentTokenUsed):
		return http.StatusUnauthorized, refusalUsedKey, true
	case errors.Is(err, persistence.ErrEnrollmentTokenExpired):
		return http.StatusUnauthorized, refusalExpiredKey, true
	case errors.Is(err, persistence.ErrEnrollmentTokenRevoked):
		return http.StatusUnauthorized, refusalRevokedKey, true
	case errors.Is(err, persistence.ErrEnrollmentTokenInvalid):
		return http.StatusUnauthorized, refusalUnknownKey, true
	case errors.Is(err, persistence.ErrEnrollmentTokenScopeMismatch):
		return http.StatusUnauthorized, refusalOtherPlacement, true
	case errors.Is(err, persistence.ErrAgentIdentityRetired):
		return http.StatusConflict, refusalRemovedDevice, true
	case errors.Is(err, persistence.ErrAgentIdentityProofV2Required):
		return http.StatusConflict, refusalAgentTooOld, true
	case errors.Is(err, persistence.ErrRecoveryRequiresSingleUseToken):
		return http.StatusConflict, refusalNeedsSingleUse, true
	case errors.Is(err, persistence.ErrAgentIdentityContinuityConflict):
		return http.StatusConflict, refusalNameTaken, true
	case errors.Is(err, persistence.ErrEnrollmentTokenPredatesRotation):
		return http.StatusConflict, refusalBeforeRotation, true
	case errors.Is(err, persistence.ErrAgentFleetCapacityReached):
		return http.StatusConflict, refusalFleetFull, true
	}
	return 0, "", false
}
