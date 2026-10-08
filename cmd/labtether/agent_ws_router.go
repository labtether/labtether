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
		s.processAgentHeartbeat(conn, msg)
	}
	router[agentmgr.MsgTelemetry] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTelemetry(conn, msg)
	}
	router[agentmgr.MsgCommandResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentCommandResult(conn, msg)
	}
	router[agentmgr.MsgPowerResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPowerResult(conn, msg)
	}
	router[agentmgr.MsgPong] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPong(conn, msg)
	}
	router[agentmgr.MsgLogStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentLogStream(conn, msg)
	}
	router[agentmgr.MsgLogBatch] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentLogBatch(conn, msg)
	}
	router[agentmgr.MsgJournalEntries] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentJournalEntries(conn, msg)
	}
	router[agentmgr.MsgUpdateProgress] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentUpdateProgress(conn, msg)
	}
	router[agentmgr.MsgUpdateResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentUpdateResult(conn, msg)
	}
	router[agentmgr.MsgTerminalProbed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalProbed(conn, msg)
	}
	router[agentmgr.MsgTerminalStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalStarted(conn, msg)
	}
	router[agentmgr.MsgTerminalData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalData(conn, msg)
	}
	router[agentmgr.MsgTerminalClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentTerminalClosed(conn, msg)
	}
	router[agentmgr.MsgSSHKeyInstalled] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSSHKeyInstalled(conn, msg)
	}
	router[agentmgr.MsgSSHKeyRemoved] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSSHKeyRemoved(conn, msg)
	}
	router[agentmgr.MsgDesktopStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopStarted(conn, msg)
	}
	router[agentmgr.MsgDesktopData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopData(conn, msg)
	}
	router[agentmgr.MsgDesktopClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopClosed(conn, msg)
	}
	router[agentmgr.MsgDesktopDisplays] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopDisplays(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopAudioData(conn, msg)
	}
	router[agentmgr.MsgDesktopAudioState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopAudioState(conn, msg)
	}
	router[agentmgr.MsgWebRTCCapabilities] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCCapabilities(conn, msg)
	}
	router[agentmgr.MsgWebRTCStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCStarted(conn, msg)
	}
	router[agentmgr.MsgWebRTCAnswer] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCAnswer(conn, msg)
	}
	router[agentmgr.MsgWebRTCICE] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCICE(conn, msg)
	}
	router[agentmgr.MsgWebRTCStopped] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebRTCStopped(conn, msg)
	}
	router[agentmgr.MsgWoLResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWoLResult(conn, msg)
	}
	router[agentmgr.MsgFileListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileListed(conn, msg)
	}
	router[agentmgr.MsgFileData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileData(conn, msg)
	}
	router[agentmgr.MsgFileWritten] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileWritten(conn, msg)
	}
	router[agentmgr.MsgFileResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentFileResult(conn, msg)
	}
	router[agentmgr.MsgProcessListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentProcessListed(conn, msg)
	}
	router[agentmgr.MsgProcessKillResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentProcessKillResult(conn, msg)
	}
	router[agentmgr.MsgServiceListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentServiceListed(conn, msg)
	}
	router[agentmgr.MsgServiceResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentServiceResult(conn, msg)
	}
	router[agentmgr.MsgDiskListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDiskListed(conn, msg)
	}
	router[agentmgr.MsgNetworkListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentNetworkListed(conn, msg)
	}
	router[agentmgr.MsgNetworkResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentNetworkResult(conn, msg)
	}
	router[agentmgr.MsgPackageListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPackageListed(conn, msg)
	}
	router[agentmgr.MsgPackageResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentPackageResult(conn, msg)
	}
	router[agentmgr.MsgCronListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentCronListed(conn, msg)
	}
	router[agentmgr.MsgUsersListed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentUsersListed(conn, msg)
	}
	router[agentmgr.MsgConfigApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentConfigApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsApplied] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSettingsApplied(conn, msg)
	}
	router[agentmgr.MsgAgentSettingsState] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentSettingsState(conn, msg)
	}
	router[agentmgr.MsgDockerEndpointTestResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerEndpointTestResult(conn, msg)
	}
	router[agentmgr.MsgDockerDiscovery] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerDiscovery(conn, msg)
	}
	router[agentmgr.MsgDockerDiscoveryDelta] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerDiscoveryDelta(conn, msg)
	}
	router[agentmgr.MsgDockerStats] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerStats(conn, msg)
	}
	router[agentmgr.MsgDockerEvents] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerEvents(conn, msg)
	}
	router[agentmgr.MsgDockerActionResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerActionResult(conn, msg)
	}
	router[agentmgr.MsgDockerExecStarted] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerExecStartedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerExecDataMessage(conn, msg)
	}
	router[agentmgr.MsgDockerExecClosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerExecClosedMessage(conn, msg)
	}
	router[agentmgr.MsgDockerLogsStream] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerLogsStreamMessage(conn, msg)
	}
	router[agentmgr.MsgDockerComposeResult] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDockerComposeResult(conn, msg)
	}
	router[agentmgr.MsgWebServiceReport] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentWebServiceReport(conn, msg)
	}
	router[agentmgr.MsgClipboardData] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentClipboardData(conn, msg)
	}
	router[agentmgr.MsgClipboardSetAck] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentClipboardSetAck(conn, msg)
	}
	router[agentmgr.MsgDesktopDiagnosed] = func(conn *agentmgr.AgentConn, msg agentmgr.Message) {
		s.processAgentDesktopDiagnosed(conn, msg)
	}

	return router
}
