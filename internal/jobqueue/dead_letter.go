package jobqueue

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

// DeadLetterEvent captures a job that exceeded retry limits.
type DeadLetterEvent struct {
	ID         string    `json:"id"`
	Component  string    `json:"component"`
	Subject    string    `json:"subject"`
	Deliveries uint64    `json:"deliveries"`
	Error      string    `json:"error"`
	PayloadB64 string    `json:"payload_b64,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// NewDeadLetterEvent creates a DeadLetterEvent from a failed job.
func NewDeadLetterEvent(component, subject string, deliveries uint64, payload []byte, err error) DeadLetterEvent {
	message := "processing failed"
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}

	payloadB64 := ""
	if len(payload) > 0 {
		payloadB64 = base64.StdEncoding.EncodeToString(payload)
	}

	now := time.Now().UTC()
	return DeadLetterEvent{
		ID:         fmt.Sprintf("dlq_%d", now.UnixNano()),
		Component:  strings.TrimSpace(component),
		Subject:    strings.TrimSpace(subject),
		Deliveries: deliveries,
		Error:      message,
		PayloadB64: payloadB64,
		CreatedAt:  now,
	}
}
