// Package agentcaps decides which requests the Hub may send to a device's
// agent. An agent that sets agent_capability_mode=explicit in its heartbeat
// metadata is offered only what it advertises. Every other agent, which
// includes every Go agent released so far, is offered everything, exactly as
// before: the rule needs a positive signal and is never inferred from a
// missing key.
package agentcaps

import (
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// Heartbeat metadata keys and the only values the Hub acts on. Any other
// value behaves as if the key were absent.
const (
	// MetaMode says whether the agent's advertisement is complete.
	MetaMode     = "agent_capability_mode"
	ModeExplicit = "explicit"

	// MetaEdition names a limited build for display. It does not change
	// what the Hub sends; only MetaMode does.
	MetaEdition       = "agent_edition"
	EditionMonitoring = "monitoring"

	// MetaCapabilities is where the Hub records the heartbeat's
	// capabilities list for an explicit agent, so the stored device keeps
	// it. An explicit agent's own value for this key is replaced.
	MetaCapabilities = "agent_capabilities"
)

// need is one advertised value: a token in a comma-separated metadata value.
// key MetaCapabilities means the heartbeat capabilities list.
type need struct{ key, token string }

func listed(token string) need { return need{MetaCapabilities, token} }

// messageNeeds says what an explicit agent must advertise before the Hub
// sends it each request. A request type that is neither here nor in
// coreMessages is not sent to an explicit agent.
var messageNeeds = map[string]need{
	agentmgr.MsgDiskList:      {"cap_disks", "list"},
	agentmgr.MsgProcessList:   {"cap_processes", "list"},
	agentmgr.MsgProcessKill:   {"cap_processes", "kill"},
	agentmgr.MsgServiceList:   {"cap_services", "list"},
	agentmgr.MsgServiceAction: {"cap_services", "action"},
	agentmgr.MsgNetworkList:   {"cap_network", "list"},
	agentmgr.MsgNetworkAction: {"cap_network", "action"},
	agentmgr.MsgPackageList:   {"cap_packages", "list"},
	agentmgr.MsgPackageAction: {"cap_packages", "action"},
	agentmgr.MsgCronList:      {"cap_schedules", "list"},
	agentmgr.MsgUsersList:     {"cap_users", "list"},
	agentmgr.MsgJournalQuery:  {"cap_logs", "query"},

	agentmgr.MsgFileList:   listed("files"),
	agentmgr.MsgFileRead:   listed("files"),
	agentmgr.MsgFileWrite:  listed("files"),
	agentmgr.MsgFileMkdir:  listed("files"),
	agentmgr.MsgFileDelete: listed("files"),
	agentmgr.MsgFileRename: listed("files"),
	agentmgr.MsgFileCopy:   listed("files"),
	agentmgr.MsgFileSearch: listed("files"),

	agentmgr.MsgTerminalProbe:    listed("terminal"),
	agentmgr.MsgTerminalStart:    listed("terminal"),
	agentmgr.MsgTerminalData:     listed("terminal"),
	agentmgr.MsgTerminalResize:   listed("terminal"),
	agentmgr.MsgTerminalClose:    listed("terminal"),
	agentmgr.MsgTerminalTmuxKill: listed("terminal"),

	agentmgr.MsgDesktopStart:        listed("desktop"),
	agentmgr.MsgDesktopData:         listed("desktop"),
	agentmgr.MsgDesktopClose:        listed("desktop"),
	agentmgr.MsgDesktopListDisplays: listed("desktop"),
	agentmgr.MsgDesktopDiagnose:     listed("desktop"),
	agentmgr.MsgDesktopAudioStart:   listed("desktop"),
	agentmgr.MsgDesktopAudioStop:    listed("desktop"),
	agentmgr.MsgClipboardGet:        listed("desktop"),
	agentmgr.MsgClipboardSet:        listed("desktop"),

	agentmgr.MsgWebRTCStart: listed("webrtc"),
	agentmgr.MsgWebRTCOffer: listed("webrtc"),
	agentmgr.MsgWebRTCICE:   listed("webrtc"),
	agentmgr.MsgWebRTCInput: listed("webrtc"),
	agentmgr.MsgWebRTCStop:  listed("webrtc"),

	agentmgr.MsgPowerAction: listed("power"),

	agentmgr.MsgWoLSend:       {"cap_wol", "send"},
	agentmgr.MsgSSHKeyInstall: {"cap_ssh_keys", "manage"},
	agentmgr.MsgSSHKeyRemove:  {"cap_ssh_keys", "manage"},

	agentmgr.MsgDockerAction:          {"cap_docker", "manage"},
	agentmgr.MsgDockerComposeAction:   {"cap_docker", "manage"},
	agentmgr.MsgDockerComposeActionV2: {"cap_docker", "manage"},
	agentmgr.MsgDockerEndpointTest:    {"cap_docker", "manage"},
	agentmgr.MsgDockerLogsStart:       {"cap_docker", "manage"},
	agentmgr.MsgDockerLogsStop:        {"cap_docker", "manage"},
	agentmgr.MsgDockerExecStart:       {"cap_docker", "manage"},
	agentmgr.MsgDockerExecInput:       {"cap_docker", "manage"},
	agentmgr.MsgDockerExecResize:      {"cap_docker", "manage"},
	agentmgr.MsgDockerExecClose:       {"cap_docker", "manage"},

	agentmgr.MsgWebServiceSync:     {"cap_web_services", "sync"},
	agentmgr.MsgCommandRequest:     {"cap_commands", "run"},
	agentmgr.MsgAgentSettingsApply: {"cap_settings", "apply"},
	// update.request also depends on its mode: see OffersUpdate.
	agentmgr.MsgUpdateRequest: {"cap_updates", ""},
}

// coreMessages are sent to every agent: the connection itself, enrollment,
// runtime config and alert display. The Rust agent handles them in its core.
var coreMessages = map[string]bool{
	agentmgr.MsgPing:                true,
	agentmgr.MsgPong:                true,
	agentmgr.MsgConfigUpdate:        true,
	agentmgr.MsgAlertNotify:         true,
	agentmgr.MsgEnrollmentChallenge: true,
	agentmgr.MsgEnrollmentApproved:  true,
	agentmgr.MsgEnrollmentRejected:  true,
}

// Explicit reports whether metadata says the advertisement is complete.
func Explicit(metadata map[string]string) bool {
	return normalize(metadata[MetaMode]) == ModeExplicit
}

// Edition returns the agent's edition when the Hub knows it, else "".
func Edition(metadata map[string]string) string {
	if normalize(metadata[MetaEdition]) == EditionMonitoring {
		return EditionMonitoring
	}
	return ""
}

// Offers reports whether the Hub may send msgType to an agent whose
// heartbeat metadata is metadata.
func Offers(metadata map[string]string, msgType string) bool {
	if !Explicit(metadata) || coreMessages[msgType] {
		return true
	}
	n, ok := messageNeeds[msgType]
	if !ok || n.token == "" {
		return false
	}
	return HasToken(metadata[n.key], n.token)
}

// OffersUpdate reports whether the Hub may send update.request with mode:
// "self" needs cap_updates=self, any other mode cap_updates=packages.
func OffersUpdate(metadata map[string]string, mode string) bool {
	if !Explicit(metadata) {
		return true
	}
	token := "packages"
	if normalize(mode) == "self" {
		token = "self"
	}
	return HasToken(metadata["cap_updates"], token)
}

// Offered lists every request type the Hub may send to an explicit agent,
// sorted, so the console can follow the Hub's own rule. It is nil for an
// agent that is not explicit: such an agent is offered everything.
func Offered(metadata map[string]string) []string {
	if !Explicit(metadata) {
		return nil
	}
	out := make([]string, 0, len(messageNeeds))
	for msgType := range messageNeeds {
		if Offers(metadata, msgType) {
			out = append(out, msgType)
		}
	}
	for _, mode := range []string{"self", "packages"} {
		if OffersUpdate(metadata, mode) {
			out = append(out, agentmgr.MsgUpdateRequest+":"+mode)
		}
	}
	sort.Strings(out)
	return out
}

// CapabilityIDs returns the capability set entries an explicit agent earns
// beyond the cap_services, cap_packages, cap_network, cap_schedules and
// cap_logs entries every agent gets from its metadata. They replace the
// entries the Hub adds by default for an agent that is not explicit.
func CapabilityIDs(metadata map[string]string) []string {
	var out []string
	for _, entry := range []struct {
		id string
		n  need
	}{
		{"terminal.open", listed("terminal")},
		{"files.list", listed("files")},
		{"process.list", need{"cap_processes", "list"}},
		{"users.list", need{"cap_users", "list"}},
	} {
		if HasToken(metadata[entry.n.key], entry.n.token) {
			out = append(out, entry.id)
		}
	}
	return out
}

// HasToken reports whether a comma-separated value holds token.
func HasToken(value, token string) bool {
	token = normalize(token)
	for _, part := range strings.Split(value, ",") {
		if normalize(part) == token && token != "" {
			return true
		}
	}
	return false
}

// JoinCapabilities normalizes a heartbeat capabilities list for MetaCapabilities.
func JoinCapabilities(capabilities []string) string {
	out := make([]string, 0, len(capabilities))
	seen := make(map[string]bool, len(capabilities))
	for _, capability := range capabilities {
		value := normalize(capability)
		if value == "" || strings.Contains(value, ",") || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
