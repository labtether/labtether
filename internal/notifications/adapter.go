package notifications

import "context"

// Adapter defines how to send a notification through a specific channel type.
type Adapter interface {
	Type() string
	Send(ctx context.Context, config map[string]any, payload map[string]any) error
}
