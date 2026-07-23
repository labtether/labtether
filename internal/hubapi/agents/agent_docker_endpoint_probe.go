package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

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

type pendingDockerEndpointTest struct {
	ExpectedConn     *agentmgr.AgentConn
	ExpectedAgentID  string
	ExpectedAssetID  string
	ExpectedEndpoint string
	ResultCh         chan agentmgr.DockerEndpointTestResultData
}

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
	data, err := json.Marshal(request)
	if err != nil {
		return agentmgr.DockerEndpointTestResultData{}, fmt.Errorf("marshal Docker endpoint test request: %w", err)
	}
	conn, ok := d.AgentMgr.Get(assetID)
	if !ok || conn == nil {
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestUnavailable
	}

	resultCh := make(chan agentmgr.DockerEndpointTestResultData, 1)
	d.DockerEndpointTestBridges.Store(requestID, pendingDockerEndpointTest{
		ExpectedConn:     conn,
		ExpectedAgentID:  assetID,
		ExpectedAssetID:  assetID,
		ExpectedEndpoint: endpoint,
		ResultCh:         resultCh,
	})
	defer d.DockerEndpointTestBridges.Delete(requestID)

	if err := conn.Send(agentmgr.Message{
		Type: agentmgr.MsgDockerEndpointTest,
		ID:   requestID,
		Data: data,
	}); err != nil {
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestUnavailable
	}

	timeout := d.DockerEndpointTestTimeout
	if timeout <= 0 {
		timeout = defaultDockerEndpointTestTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-resultCh:
		return result, nil
	case <-ctx.Done():
		return agentmgr.DockerEndpointTestResultData{}, ctx.Err()
	case <-timer.C:
		return agentmgr.DockerEndpointTestResultData{}, ErrDockerEndpointTestTimeout
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
	if err := json.Unmarshal(msg.Data, &result); err != nil || result.Validate() != nil {
		return
	}
	if msg.ID == "" || msg.ID != result.RequestID {
		return
	}
	raw, ok := d.DockerEndpointTestBridges.Load(result.RequestID)
	if !ok {
		return
	}
	pending, ok := raw.(pendingDockerEndpointTest)
	if !ok || pending.ResultCh == nil {
		return
	}
	if conn != pending.ExpectedConn ||
		conn.AssetID != pending.ExpectedAgentID ||
		result.AssetID != pending.ExpectedAssetID ||
		result.Endpoint != pending.ExpectedEndpoint {
		return
	}
	select {
	case pending.ResultCh <- result:
	default:
	}
}
