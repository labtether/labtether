package main

import (
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// buildWSRouter constructs a closure-based WebSocket message router.
// Each entry captures the apiServer receiver, so handlers use the
// shared.WSHandler signature: func(conn, msg) with no server argument.
func (s *apiServer) buildWSRouter() shared.WSRouter {
	router := make(shared.WSRouter, 64)

	router[agentmgr.MsgHeartbeat] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentHeartbeatInLifecycle(conn, msg)
	}
	router[agentmgr.MsgTelemetry] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentTelemetry(conn, msg)
	}
	router[agentmgr.MsgCommandResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentCommandResult(conn, msg)
	}
	router[agentmgr.MsgPowerResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPowerResult(conn, msg)
	}
	router[agentmgr.MsgPong] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPong(conn, msg)
	}
	router[agentmgr.MsgLogStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentLogStream(conn, msg)
	}
	router[agentmgr.MsgLogBatch] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentLogBatch(conn, msg)
	}
	router[agentmgr.MsgJournalEntries] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentJournalEntries(conn, msg)
	}
	router[agentmgr.MsgUpdateProgress] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentUpdateProgress(conn, msg)
	}
	router[agentmgr.MsgUpdateResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentUpdateResult(conn, msg)
	}
	router[agentmgr.MsgTerminalProbed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureTerminalDeps().ProcessAgentTerminalProbed(conn, msg)
	}
	router[agentmgr.MsgTerminalStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureTerminalDeps().ProcessAgentTerminalStarted(conn, msg)
	}
	router[agentmgr.MsgTerminalData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureTerminalDeps().ProcessAgentTerminalData(conn, msg)
	}
	router[agentmgr.MsgTerminalClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureTerminalDeps().ProcessAgentTerminalClosed(conn, msg)
	}
	router[agentmgr.MsgSSHKeyInstalled] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentSSHKeyInstalled(conn, msg)
	}
	router[agentmgr.MsgSSHKeyRemoved] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentSSHKeyRemoved(conn, msg)
	}
	router[agentmgr.MsgDesktopStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopStarted(conn, msg)
	}
	router[agentmgr.MsgDesktopData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopData(conn, msg)
	}
	router[agentmgr.MsgDesktopClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopClosed(conn, msg)
	}
	router[agentmgr.MsgDesktopDisplays] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopDisplays(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopAudioData(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopAudioState(conn, msg)
	}
	router[agentmgr.MsgWebRTCCapabilities] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentWebRTCCapabilities(conn, msg)
	}
	router[agentmgr.MsgWebRTCStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentWebRTCStarted(conn, msg)
	}
	router[agentmgr.MsgWebRTCAnswer] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentWebRTCAnswer(conn, msg)
	}
	router[agentmgr.MsgWebRTCICE] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentWebRTCICE(conn, msg)
	}
	router[agentmgr.MsgWebRTCStopped] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentWebRTCStopped(conn, msg)
	}
	router[agentmgr.MsgWoLResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentWoLResult(conn, msg)
	}
	router[agentmgr.MsgFileListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentFileListed(conn, msg)
	}
	router[agentmgr.MsgFileData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentFileData(conn, msg)
	}
	router[agentmgr.MsgFileWritten] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentFileWritten(conn, msg)
	}
	router[agentmgr.MsgFileResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentFileResult(conn, msg)
	}
	router[agentmgr.MsgProcessListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentProcessListed(conn, msg)
	}
	router[agentmgr.MsgProcessKillResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentProcessKillResult(conn, msg)
	}
	router[agentmgr.MsgServiceListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentServiceListed(conn, msg)
	}
	router[agentmgr.MsgServiceResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentServiceResult(conn, msg)
	}
	router[agentmgr.MsgDiskListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentDiskListed(conn, msg)
	}
	router[agentmgr.MsgNetworkListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentNetworkListed(conn, msg)
	}
	router[agentmgr.MsgNetworkResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentNetworkResult(conn, msg)
	}
	router[agentmgr.MsgPackageListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentPackageListed(conn, msg)
	}
	router[agentmgr.MsgPackageResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentPackageResult(conn, msg)
	}
	router[agentmgr.MsgCronListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentCronListed(conn, msg)
	}
	router[agentmgr.MsgUsersListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureResourcesDeps().ProcessAgentUsersListed(conn, msg)
	}
	router[agentmgr.MsgConfigApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentConfigApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentSettingsApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentSettingsState(conn, msg)
	}
	router[agentmgr.MsgDockerEndpointTestResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureAgentsDeps().ProcessAgentDockerEndpointTestResult(conn, msg)
	}
	router[agentmgr.MsgDockerDiscovery] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerDiscovery(conn, msg)
	}
	router[agentmgr.MsgDockerDiscoveryDelta] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerDiscoveryDelta(conn, msg)
	}
	router[agentmgr.MsgDockerStats] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerStats(conn, msg)
	}
	router[agentmgr.MsgDockerEvents] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerEvents(conn, msg)
	}
	router[agentmgr.MsgDockerActionResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerActionResult(conn, msg)
	}
	router[agentmgr.MsgDockerExecStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerExecStartedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerExecDataMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerExecClosedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerLogsStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerLogsStreamMessage(conn, msg)
	}
	router[agentmgr.MsgDockerComposeResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDockerDeps().ProcessAgentDockerComposeResult(conn, msg)
	}
	router[agentmgr.MsgWebServiceReport] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureCollectorsDeps().ProcessAgentWebServiceReport(conn, msg)
	}
	router[agentmgr.MsgClipboardData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentClipboardData(conn, msg)
	}
	router[agentmgr.MsgClipboardSetAck] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentClipboardSetAck(conn, msg)
	}
	router[agentmgr.MsgDesktopDiagnosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.ensureDesktopDeps().ProcessAgentDesktopDiagnosed(conn, msg)
	}

	return router
}
