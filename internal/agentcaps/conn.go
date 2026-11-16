package agentcaps

import (
	"encoding/json"
	"errors"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ErrUnsupported is returned before anything is sent when the device's agent
// did not advertise the request.
var ErrUnsupported = errors.New("agent does not support this request")

// connMetaKey holds, on a connection, the metadata keys the rules read.
const connMetaKey = "agentcaps.advertisement"

// Record keeps on conn what the rules need from the device's latest
// heartbeat metadata, or from the stored device when it connects, so
// background work and routes can check a request without a store read.
func Record(conn *agentmgr.AgentConn, metadata map[string]string) {
	if conn == nil {
		return
	}
	kept := map[string]string{}
	keep := func(key string) {
		if value, ok := metadata[key]; ok {
			kept[key] = value
		}
	}
	keep(MetaMode)
	keep(MetaEdition)
	keep(MetaCapabilities)
	keep("cap_updates")
	for _, n := range messageNeeds {
		keep(n.key)
	}
	encoded, err := json.Marshal(kept)
	if err != nil {
		return
	}
	conn.SetMeta(connMetaKey, string(encoded))
}

// connMetadata returns what Record kept on conn; nil when nothing was kept.
func connMetadata(conn *agentmgr.AgentConn) map[string]string {
	if conn == nil {
		return nil
	}
	raw := conn.Meta(connMetaKey)
	if raw == "" {
		return nil
	}
	var metadata map[string]string
	if json.Unmarshal([]byte(raw), &metadata) != nil {
		return nil
	}
	return metadata
}

// ConnOffers reports whether the Hub may send msgType on conn. A connection
// with nothing recorded is treated as an agent that is not explicit.
func ConnOffers(conn *agentmgr.AgentConn, msgType string) bool {
	return Offers(connMetadata(conn), msgType)
}

// ConnOffersUpdate is OffersUpdate for a connection.
func ConnOffersUpdate(conn *agentmgr.AgentConn, mode string) bool {
	return OffersUpdate(connMetadata(conn), mode)
}

// Check returns ErrUnsupported when the Hub may not send msgType on conn.
func Check(conn *agentmgr.AgentConn, msgType string) error {
	if ConnOffers(conn, msgType) {
		return nil
	}
	return ErrUnsupported
}

// Manager is the part of agentmgr.AgentManager that GatedSender uses.
type Manager interface {
	Get(assetID string) (*agentmgr.AgentConn, bool)
	SendToAgent(assetID string, msg agentmgr.Message) error
}

// GatedSender sends through Manager only what the device's agent offers.
// A refused message returns ErrUnsupported and is not sent.
type GatedSender struct{ Manager Manager }

// SendToAgent sends msg unless the device's agent did not advertise it.
func (g GatedSender) SendToAgent(assetID string, msg agentmgr.Message) error {
	if conn, ok := g.Manager.Get(assetID); ok && !ConnOffers(conn, msg.Type) {
		return ErrUnsupported
	}
	return g.Manager.SendToAgent(assetID, msg)
}
