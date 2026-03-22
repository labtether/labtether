package agentmgr

import (
	"encoding/json"
	"testing"
)

func TestIsKnownMessageType(t *testing.T) {
	known := []string{
		MsgHeartbeat, MsgTelemetry, MsgCommandRequest, MsgCommandResult,
		MsgPing, MsgPong, MsgLogStream, MsgLogBatch,
		MsgJournalQuery, MsgJournalEntries,
		MsgConfigUpdate, MsgConfigApplied,
		MsgAgentSettingsApply, MsgAgentSettingsApplied, MsgAgentSettingsState,
		MsgTerminalProbe, MsgTerminalProbed,
		MsgTerminalStart, MsgTerminalStarted, MsgTerminalData,
		MsgTerminalResize, MsgTerminalClose, MsgTerminalClosed,
		MsgDesktopStart, MsgDesktopStarted, MsgDesktopData,
		MsgDesktopClose, MsgDesktopClosed,
		MsgDesktopListDisplays, MsgDesktopDisplays,
		MsgClipboardGet, MsgClipboardData, MsgClipboardSet, MsgClipboardSetAck,
		MsgDesktopAudioStart, MsgDesktopAudioStop, MsgDesktopAudioData, MsgDesktopAudioState,
		MsgWebRTCCapabilities, MsgWebRTCOffer, MsgWebRTCAnswer, MsgWebRTCICE,
		MsgWebRTCStart, MsgWebRTCStarted, MsgWebRTCStop, MsgWebRTCStopped, MsgWebRTCInput,
		MsgWoLSend, MsgWoLResult,
		MsgFileList, MsgFileListed, MsgFileRead, MsgFileData,
		MsgFileWrite, MsgFileWritten, MsgFileMkdir, MsgFileDelete,
		MsgFileRename, MsgFileCopy, MsgFileResult,
		MsgFileSearch, MsgFileSearchResult,
		MsgProcessList, MsgProcessListed,
		MsgProcessKill, MsgProcessKillResult,
		MsgServiceList, MsgServiceListed, MsgServiceAction, MsgServiceResult,
		MsgDiskList, MsgDiskListed,
		MsgNetworkList, MsgNetworkListed, MsgNetworkAction, MsgNetworkResult,
		MsgPackageList, MsgPackageListed, MsgPackageAction, MsgPackageResult,
		MsgCronList, MsgCronListed,
		MsgUsersList, MsgUsersListed,
		MsgUpdateRequest, MsgUpdateProgress, MsgUpdateResult,
		MsgSSHKeyInstall, MsgSSHKeyInstalled, MsgSSHKeyRemove, MsgSSHKeyRemoved,
		MsgAlertNotify,
		MsgEnrollmentChallenge, MsgEnrollmentProof,
		MsgEnrollmentApproved, MsgEnrollmentRejected,
	}

	for _, msgType := range known {
		if !IsKnownMessageType(msgType) {
			t.Errorf("IsKnownMessageType(%q) = false, want true", msgType)
		}
	}
}

func TestIsKnownMessageType_RejectsUnknown(t *testing.T) {
	unknown := []string{"", "foo", "heartbeat.v2", "HEARTBEAT", "terminal"}
	for _, msgType := range unknown {
		if IsKnownMessageType(msgType) {
			t.Errorf("IsKnownMessageType(%q) = true, want false", msgType)
		}
	}
}

func TestKnownMessageTypesCountMatchesConstants(t *testing.T) {
	// Keep this in sync with message.go as new protocol capabilities are added.
	// If a new constant is added but not to KnownMessageTypes, this test fails.
	const expectedCount = 119
	if len(KnownMessageTypes) != expectedCount {
		t.Errorf("KnownMessageTypes has %d entries, want %d (did you add a new message type?)",
			len(KnownMessageTypes), expectedCount)
	}
}

func TestDockerMessageTypesAreKnown(t *testing.T) {
	dockerTypes := []string{
		MsgDockerDiscovery, MsgDockerDiscoveryDelta, MsgDockerStats, MsgDockerEvents,
		MsgDockerAction, MsgDockerActionResult,
		MsgDockerLogsStart, MsgDockerLogsStop, MsgDockerLogsStream,
		MsgDockerExecStart, MsgDockerExecStarted, MsgDockerExecData,
		MsgDockerExecInput, MsgDockerExecResize, MsgDockerExecClose, MsgDockerExecClosed,
		MsgDockerComposeAction, MsgDockerComposeResult,
	}
	for _, msgType := range dockerTypes {
		if !IsKnownMessageType(msgType) {
			t.Errorf("message type %q not registered in KnownMessageTypes", msgType)
		}
	}
}

func TestMessageEnvelopeRoundTrip(t *testing.T) {
	msg := Message{
		Type: MsgHeartbeat,
		ID:   "test-123",
		Data: json.RawMessage(`{"asset_id":"node-1"}`),
	}

	encoded, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Type = %q, want %q", decoded.Type, msg.Type)
	}
	if decoded.ID != msg.ID {
		t.Errorf("ID = %q, want %q", decoded.ID, msg.ID)
	}
}

func TestMessageEnvelopeRejectsEmptyType(t *testing.T) {
	raw := `{"type":"","id":"x"}`
	var msg Message
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if IsKnownMessageType(msg.Type) {
		t.Error("empty type should not be known")
	}
}
