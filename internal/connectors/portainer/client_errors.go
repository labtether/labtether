package portainer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// APIError is a request Portainer answered with an error status. Message and
// Details are Portainer's own words from its JSON reply, when it sent them.
type APIError struct {
	StatusCode int
	Body       string
	Message    string
	Details    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("portainer api returned %d: %s", e.StatusCode, e.Body)
}

// Reason is Portainer's own explanation: its details, else its message.
func (e *APIError) Reason() string {
	if e.Details != "" {
		return e.Details
	}
	return e.Message
}

func newAPIError(statusCode int, payload []byte) *APIError {
	apiErr := &APIError{StatusCode: statusCode, Body: strings.TrimSpace(string(payload))}
	var reply struct {
		Message string `json:"message"`
		Details string `json:"details"`
	}
	if json.Unmarshal(payload, &reply) == nil {
		apiErr.Message = strings.TrimSpace(reply.Message)
		apiErr.Details = strings.TrimSpace(reply.Details)
	}
	return apiErr
}
