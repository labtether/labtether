package agentmgr

import (
	"encoding/json"
	"testing"
)

func TestPowerCompatibilityContract(t *testing.T) {
	if MsgPowerAction != "power.action" || MsgPowerResult != "power.result" {
		t.Fatal("power message names drifted from protocol contract")
	}

	action := PowerActionData{RequestID: "power-1", AssetID: "node-1", Action: PowerActionShutdown}
	if err := action.Validate(); err != nil {
		t.Fatalf("valid action rejected: %v", err)
	}
	raw, err := json.Marshal(action)
	if err != nil {
		t.Fatalf("marshal action: %v", err)
	}
	if string(raw) != `{"request_id":"power-1","asset_id":"node-1","action":"shutdown"}` {
		t.Fatalf("unexpected wire form: %s", raw)
	}

	result := PowerResultData{
		RequestID: "power-1",
		AssetID:   "node-1",
		Action:    PowerActionShutdown,
		Status:    PowerResultRejected,
		Code:      PowerResultCodeCapabilityDenied,
		Message:   "power capability denied",
	}
	if err := result.Validate(); err != nil {
		t.Fatalf("valid result rejected: %v", err)
	}
}

func TestPowerCompatibilityContractRejectsOpenEnums(t *testing.T) {
	tests := []PowerResultData{
		{RequestID: "r", AssetID: "a", Action: "restart", Status: PowerResultAccepted},
		{RequestID: "r", AssetID: "a", Action: PowerActionReboot, Status: "success"},
		{RequestID: "r", AssetID: "a", Action: PowerActionReboot, Status: PowerResultAccepted, Code: PowerResultCodeExecutionFailed},
		{RequestID: "r", AssetID: "a", Action: PowerActionReboot, Status: PowerResultRejected, Code: PowerResultCodeExecutionFailed},
	}
	for i, result := range tests {
		if err := result.Validate(); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}
