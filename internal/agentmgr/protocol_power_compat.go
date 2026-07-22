package agentmgr

import (
	"fmt"
	"strings"
)

// Power wire compatibility declarations mirror github.com/labtether/protocol's
// power.action/power.result contract. The hub intentionally keeps its published
// protocol v1.4.0 dependency until the next protocol release; these narrow
// declarations avoid a relative replace or dependency on an unpublished tag.
const (
	MsgPowerAction = "power.action"
	MsgPowerResult = "power.result"
)

type PowerAction string

const (
	PowerActionReboot   PowerAction = "reboot"
	PowerActionShutdown PowerAction = "shutdown"
)

func (a PowerAction) Valid() bool {
	switch a {
	case PowerActionReboot, PowerActionShutdown:
		return true
	default:
		return false
	}
}

type PowerResultStatus string

const (
	PowerResultAccepted    PowerResultStatus = "accepted"
	PowerResultUnsupported PowerResultStatus = "unsupported"
	PowerResultRejected    PowerResultStatus = "rejected"
	PowerResultFailed      PowerResultStatus = "failed"
)

func (s PowerResultStatus) Valid() bool {
	switch s {
	case PowerResultAccepted, PowerResultUnsupported, PowerResultRejected, PowerResultFailed:
		return true
	default:
		return false
	}
}

type PowerResultCode string

const (
	PowerResultCodeInvalidRequest      PowerResultCode = "invalid_request"
	PowerResultCodeAssetMismatch       PowerResultCode = "asset_mismatch"
	PowerResultCodeCapabilityDenied    PowerResultCode = "capability_denied"
	PowerResultCodeBusy                PowerResultCode = "busy"
	PowerResultCodeUnsupportedPlatform PowerResultCode = "unsupported_platform"
	PowerResultCodeExecutionFailed     PowerResultCode = "execution_failed"
	PowerResultCodeExecutionTimeout    PowerResultCode = "execution_timeout"
)

func (c PowerResultCode) Valid() bool {
	switch c {
	case PowerResultCodeInvalidRequest,
		PowerResultCodeAssetMismatch,
		PowerResultCodeCapabilityDenied,
		PowerResultCodeBusy,
		PowerResultCodeUnsupportedPlatform,
		PowerResultCodeExecutionFailed,
		PowerResultCodeExecutionTimeout:
		return true
	default:
		return false
	}
}

type PowerActionData struct {
	RequestID string      `json:"request_id"`
	AssetID   string      `json:"asset_id"`
	Action    PowerAction `json:"action"`
}

func (d PowerActionData) Validate() error {
	if strings.TrimSpace(d.RequestID) == "" || len(d.RequestID) > 128 {
		return fmt.Errorf("invalid request_id")
	}
	if strings.TrimSpace(d.AssetID) == "" || len(d.AssetID) > 256 {
		return fmt.Errorf("invalid asset_id")
	}
	if !d.Action.Valid() {
		return fmt.Errorf("invalid action")
	}
	return nil
}

type PowerResultData struct {
	RequestID string            `json:"request_id"`
	AssetID   string            `json:"asset_id"`
	Action    PowerAction       `json:"action"`
	Status    PowerResultStatus `json:"status"`
	Code      PowerResultCode   `json:"code,omitempty"`
	Message   string            `json:"message,omitempty"`
}

func (d PowerResultData) Validate() error {
	if err := (PowerActionData{RequestID: d.RequestID, AssetID: d.AssetID, Action: d.Action}).Validate(); err != nil {
		return err
	}
	if !d.Status.Valid() {
		return fmt.Errorf("invalid status")
	}
	if len(d.Message) > 256 {
		return fmt.Errorf("message too long")
	}

	switch d.Status {
	case PowerResultAccepted:
		if d.Code != "" {
			return fmt.Errorf("accepted result must not include a code")
		}
	case PowerResultUnsupported:
		if d.Code != PowerResultCodeUnsupportedPlatform {
			return fmt.Errorf("invalid unsupported result code")
		}
	case PowerResultRejected:
		switch d.Code {
		case PowerResultCodeInvalidRequest,
			PowerResultCodeAssetMismatch,
			PowerResultCodeCapabilityDenied,
			PowerResultCodeBusy:
		default:
			return fmt.Errorf("invalid rejected result code")
		}
	case PowerResultFailed:
		if d.Code != PowerResultCodeExecutionFailed && d.Code != PowerResultCodeExecutionTimeout {
			return fmt.Errorf("invalid failed result code")
		}
	}
	return nil
}
