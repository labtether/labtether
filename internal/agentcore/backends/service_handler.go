package backends

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// ServiceManager handles service list and management requests from the hub.
type ServiceManager struct {
	Backend ServiceBackend
}

// NewServiceManager creates a ServiceManager with the OS-appropriate backend.
func NewServiceManager() *ServiceManager {
	return &ServiceManager{
		Backend: NewServiceBackendForOS(),
	}
}

// CloseAll is a no-op for ServiceManager — service requests are stateless
// and require no cleanup.
func (sm *ServiceManager) CloseAll() {}

// HandleServiceList collects service inventory and sends it to the hub.
func (sm *ServiceManager) HandleServiceList(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.ServiceListData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("service: invalid service.list request: %v", err)
		return
	}

	services, err := sm.Backend.ListServices()

	var errMsg string
	if err != nil {
		errMsg = err.Error()
		log.Printf("service: failed to collect services: %v", err)
	}

	data, marshalErr := json.Marshal(agentmgr.ServiceListedData{
		RequestID: req.RequestID,
		Services:  services,
		Error:     errMsg,
	})
	if marshalErr != nil {
		log.Printf("service: failed to marshal service.listed response: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgServiceListed,
		ID:   req.RequestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("service: failed to send service.listed for request %s: %v", req.RequestID, sendErr)
	}
}

// HandleServiceAction performs a service action on a named service.
func (sm *ServiceManager) HandleServiceAction(transport MessageSender, msg agentmgr.Message) {
	var req agentmgr.ServiceActionData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("service: invalid service.action request: %v", err)
		return
	}

	action := strings.TrimSpace(req.Action)
	if !ValidServiceAction(action) {
		sm.sendResult(transport, req.RequestID, false, "", fmt.Sprintf("invalid action %q: must be start, stop, restart, enable, or disable", req.Action))
		return
	}
	service := strings.TrimSpace(req.Service)
	if service == "" {
		sm.sendResult(transport, req.RequestID, false, "", "service name is required")
		return
	}

	output, err := sm.Backend.PerformAction(action, service)
	if err != nil {
		log.Printf("service: %s %s failed: %v", action, service, err)
		sm.sendResult(transport, req.RequestID, false, output, err.Error())
		return
	}

	sm.sendResult(transport, req.RequestID, true, output, "")
}

// ValidServiceAction returns true if the action is a known service action.
func ValidServiceAction(action string) bool {
	switch action {
	case "start", "stop", "restart", "enable", "disable":
		return true
	default:
		return false
	}
}

// sendResult marshals and sends a ServiceResultData message to the hub.
func (sm *ServiceManager) sendResult(transport MessageSender, requestID string, ok bool, output, errMsg string) {
	data, marshalErr := json.Marshal(agentmgr.ServiceResultData{
		RequestID: requestID,
		OK:        ok,
		Output:    output,
		Error:     errMsg,
	})
	if marshalErr != nil {
		log.Printf("service: failed to marshal service.result: %v", marshalErr)
		return
	}

	if sendErr := transport.Send(agentmgr.Message{
		Type: agentmgr.MsgServiceResult,
		ID:   requestID,
		Data: data,
	}); sendErr != nil {
		log.Printf("service: failed to send service.result for request %s: %v", requestID, sendErr)
	}
}
