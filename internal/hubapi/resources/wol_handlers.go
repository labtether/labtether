package resources

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/idgen"
	"github.com/labtether/labtether/internal/servicehttp"
	"github.com/labtether/labtether/internal/wol"
)

var SendWakeOnLAN = wol.Send

func (d *Deps) HandleWakeOnLAN(w http.ResponseWriter, r *http.Request, assetID string) {
	if r.Method != http.MethodPost {
		servicehttp.WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	assetEntry, ok, err := d.AssetStore.GetAsset(assetID)
	if err != nil {
		servicehttp.WriteError(w, http.StatusInternalServerError, "failed to load asset")
		return
	}
	if !ok {
		servicehttp.WriteError(w, http.StatusNotFound, "asset not found")
		return
	}

	macStr := FindAssetMAC(assetEntry.Metadata)
	if macStr == "" {
		servicehttp.WriteError(w, http.StatusUnprocessableEntity, "no MAC address known for this asset")
		return
	}
	mac, err := wol.ParseMAC(macStr)
	if err != nil {
		servicehttp.WriteError(w, http.StatusUnprocessableEntity, fmt.Sprintf("invalid MAC address: %v", err))
		return
	}

	if method := d.tryAgentAssistedWoL(assetEntry.ID, macStr); method != "" {
		servicehttp.WriteJSON(w, http.StatusAccepted, map[string]string{
			"status": "sent",
			"method": method,
			"mac":    macStr,
		})
		return
	}

	// Fallback to hub-side broadcast.
	broadcastAddr := "255.255.255.255:9"
	if err := SendWakeOnLAN(mac, broadcastAddr); err != nil {
		log.Printf("wol: direct send failed asset=%s mac=%s: %v", assetEntry.ID, macStr, err)
		servicehttp.WriteError(w, http.StatusBadGateway, "failed to send magic packet")
		return
	}

	log.Printf("wol: direct magic packet sent asset=%s mac=%s", assetEntry.ID, macStr)
	servicehttp.WriteJSON(w, http.StatusAccepted, map[string]string{
		"status": "sent",
		"method": "direct",
		"mac":    macStr,
	})
}

func FindAssetMAC(metadata map[string]string) string {
	candidates := []string{
		strings.TrimSpace(metadata["mac_address"]),
		strings.TrimSpace(metadata["mac"]),
		strings.TrimSpace(metadata["primary_mac"]),
		strings.TrimSpace(metadata["guest_primary_mac"]),
	}
	if list := strings.TrimSpace(metadata["guest_mac_addresses"]); list != "" {
		for _, candidate := range strings.Split(list, ",") {
			candidates = append(candidates, strings.TrimSpace(candidate))
		}
	}

	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if _, err := wol.ParseMAC(candidate); err == nil {
			return candidate
		}
	}

	for _, candidate := range candidates {
		if candidate != "" {
			return candidate
		}
	}
	return ""
}

func (d *Deps) tryAgentAssistedWoL(targetAssetID, macAddr string) string {
	if d.AgentMgr == nil || d.AssetStore == nil {
		return ""
	}

	target, ok, err := d.AssetStore.GetAsset(targetAssetID)
	if err != nil || !ok {
		return ""
	}

	for _, relay := range d.EligibleWoLRelays(targetAssetID, target) {
		connectedID := relay.AssetID

		payload, _ := json.Marshal(agentmgr.WoLSendData{
			RequestID: idgen.New("wolreq"),
			MAC:       macAddr,
			Broadcast: "255.255.255.255:9",
		})
		err := d.AgentMgr.SendToAgent(connectedID, agentmgr.Message{
			Type: agentmgr.MsgWoLSend,
			ID:   targetAssetID,
			Data: payload,
		})
		if err != nil {
			log.Printf("wol: agent-assisted send failed relay=%s target=%s: %v", connectedID, targetAssetID, err)
			continue
		}
		log.Printf("wol: agent-assisted packet relayed via=%s target=%s mac=%s", connectedID, targetAssetID, macAddr)
		return "agent-assisted"
	}
	return ""
}

type WoLRelayCandidate struct {
	AssetID  string
	Platform string
}

func (d *Deps) EligibleWoLRelays(targetAssetID string, target assets.Asset) []WoLRelayCandidate {
	targetGroupID := strings.TrimSpace(target.GroupID)
	candidates := make([]WoLRelayCandidate, 0)
	for _, connectedID := range d.AgentMgr.ConnectedAssets() {
		if connectedID == targetAssetID {
			continue
		}
		agentAsset, aok, aerr := d.AssetStore.GetAsset(connectedID)
		if aerr != nil || !aok {
			continue
		}
		if targetGroupID != "" && !strings.EqualFold(strings.TrimSpace(agentAsset.GroupID), targetGroupID) {
			continue
		}
		candidates = append(candidates, WoLRelayCandidate{
			AssetID:  connectedID,
			Platform: strings.TrimSpace(agentAsset.Platform),
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		leftPriority := WoLRelayPlatformPriority(candidates[i].Platform)
		rightPriority := WoLRelayPlatformPriority(candidates[j].Platform)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}

		leftID := strings.ToLower(strings.TrimSpace(candidates[i].AssetID))
		rightID := strings.ToLower(strings.TrimSpace(candidates[j].AssetID))
		if leftID != rightID {
			return leftID < rightID
		}
		return strings.TrimSpace(candidates[i].AssetID) < strings.TrimSpace(candidates[j].AssetID)
	})

	return candidates
}

func WoLRelayPlatformPriority(platform string) int {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "linux":
		return 0
	case "":
		return 2
	default:
		return 1
	}
}

func (d *Deps) ProcessAgentWoLResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	var result agentmgr.WoLResultData
	if err := json.Unmarshal(msg.Data, &result); err != nil {
		log.Printf("wol: invalid wol.result payload from %s: %v", conn.AssetID, err)
		return
	}
	if result.OK {
		log.Printf("wol: relay %s confirmed packet send for mac=%s request=%s", conn.AssetID, result.MAC, result.RequestID)
		return
	}
	log.Printf("wol: relay %s failed packet send for mac=%s request=%s err=%s", conn.AssetID, result.MAC, result.RequestID, result.Error)
}
