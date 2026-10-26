package maintenanceguard

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/hubapi/groupfeatures"
	"github.com/labtether/labtether/internal/servicehttp"
)

// BlockedActionError preserves the same policy result across binding,
// execution and HTTP handlers without requiring a response writer in a worker.
type BlockedActionError struct {
	Guard groupfeatures.GroupMaintenanceGuardrails
	// PublicReason is fixed policy text supplied by a trusted guard, never an
	// upstream or storage error. The default response remains unchanged.
	PublicReason string
}

func (e *BlockedActionError) Error() string {
	message := fmt.Sprintf("actions are blocked by active maintenance windows for group %s", e.Guard.GroupID)
	if e.PublicReason != "" {
		message += "; " + e.PublicReason
	}
	return message
}

func CheckAssetActions(targets []string, evaluate EvaluateAssetFunc) error {
	if evaluate == nil {
		return nil
	}
	seen := make(map[string]bool, len(targets))
	at := time.Now().UTC()
	for _, target := range targets {
		target = strings.TrimSpace(target)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		guard, err := evaluate(target, at)
		if err != nil {
			return err
		}
		if guard.BlockActions {
			return &BlockedActionError{Guard: guard}
		}
	}
	return nil
}

// WriteBlockedAction reports whether it handled a typed policy rejection.
// Other failures remain the caller's responsibility and are not exposed here.
func WriteBlockedAction(w http.ResponseWriter, err error) bool {
	var blocked *BlockedActionError
	if !errors.As(err, &blocked) {
		return false
	}
	message := "actions are blocked by active maintenance windows"
	if blocked.PublicReason != "" {
		message += "; " + blocked.PublicReason
	}
	servicehttp.WriteJSON(w, http.StatusLocked, map[string]any{
		"error": "maintenance_blocked", "message": message,
		"group_id": blocked.Guard.GroupID, "windows": blocked.Guard.ActiveWindows,
	})
	return true
}
