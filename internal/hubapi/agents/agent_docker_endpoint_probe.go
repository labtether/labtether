package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentcaps"
	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

const (
	defaultDockerEndpointTestTimeout = 10 * time.Second
	maxDockerEndpointTestResultBytes = 4096
)

var (
	ErrDockerEndpointTestTimeout     = errors.New("Docker endpoint test timed out waiting for agent")
	ErrDockerEndpointTestUnavailable = errors.New("agent unavailable for Docker endpoint test")
)

// dockerEndpointTestMatch is the reply rule for Docker endpoint test results:
// the envelope ID must equal the payload request ID and the reply must come
// from the exact device ID the probe was sent to.
var dockerEndpointTestMatch = agentmgr.Match{Key: agentmgr.KeyBoth, Device: agentmgr.DeviceExact}

// RunDockerEndpointTest sends the dedicated typed probe to an agent and waits
// for a strictly correlated result. The pending entry is registered before the
// send so fast replies cannot race registration.
func (d *Deps) RunDockerEndpointTest(
	ctx context.Context,
	assetID string,
	endpoint string,
) (agentmgr.DockerEndpointTestResultData, error) {
	if d == nil || d.AgentMgr == nil {
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestUnavailable
	}
	assetID = strings.TrimSpace(assetID)
	endpoint = strings.TrimSpace(endpoint)
	requestID := shared.GenerateRequestID()
	request := agentmgr.DockerEndpointTestData{
		RequestID: requestID,
		AssetID:   assetID,
		Endpoint:  endpoint,
	}
	if err := request.Validate(); err != nil {
		return agentmgr.DockerEndpointTestResultData{}, fmt.Errorf("invalid Docker endpoint test request: %w", err)
	}
	conn, ok := d.AgentMgr.Get(assetID)
	if !ok || conn == nil {
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestUnavailable
	}
	if err := agentcaps.Check(conn, agentmgr.MsgDockerEndpointTest); err != nil {
		return agentmgr.DockerEndpointTestResultData{}, err
	}

	timeout := d.DockerEndpointTestTimeout
	if timeout <= 0 {
		timeout = defaultDockerEndpointTestTimeout
	}
	result, err := agentmgr.Request(ctx, &d.DockerEndpointTestBridges, agentmgr.Call[agentmgr.DockerEndpointTestData, agentmgr.DockerEndpointTestResultData]{
		Type:      agentmgr.MsgDockerEndpointTest,
		ID:        requestID,
		Payload:   request,
		Send:      conn.Send,
		Device:    assetID,
		ExactConn: conn,
		Accept: func(result agentmgr.DockerEndpointTestResultData) bool {
			return result.AssetID == assetID && result.Endpoint == endpoint
		},
		Timeout:    timeout,
		Disconnect: conn.Done(),
	})
	if err == nil {
		return result, nil
	}
	switch agentmgr.RequestKind(err) {
	case agentmgr.RequestInvalid:
		return agentmgr.DockerEndpointTestResultData{}, fmt.Errorf("marshal Docker endpoint test request: %w", errors.Unwrap(err))
	case agentmgr.RequestCanceled:
		return agentmgr.DockerEndpointTestResultData{}, errors.Unwrap(err)
	case agentmgr.RequestTimedOut:
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestTimeout
	default:
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestUnavailable
	}
}

// ProcessAgentDockerEndpointTestResult accepts only a fully valid result from
// the exact authenticated connection and request envelope that initiated the
// probe. Rejected messages neither consume nor delete the pending entry.
func (d *Deps) ProcessAgentDockerEndpointTestResult(conn *agentmgr.AgentConn, msg agentmgr.Message) {
	if d == nil || conn == nil || len(msg.Data) > maxDockerEndpointTestResultBytes {
		return
	}
	var result agentmgr.DockerEndpointTestResultData
	if err := agentmgr.DecodeData(msg.Data, &result, agentmgr.DecodeLenient); err != nil || result.Validate() != nil {
		return
	}
	d.DockerEndpointTestBridges.Deliver(dockerEndpointTestMatch, agentmgr.Reply[agentmgr.DockerEndpointTestResultData]{
		EnvelopeID: msg.ID,
		PayloadID:  result.RequestID,
		Conn:       conn,
		Value:      result,
	})
}
