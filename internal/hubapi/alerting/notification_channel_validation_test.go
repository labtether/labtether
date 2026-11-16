package alerting_test

import (
	"testing"

	alertingpkg "github.com/labtether/labtether/internal/hubapi/alerting"
	"github.com/labtether/labtether/internal/notifications"
)

func TestValidateCreateChannelRequestAcceptsNtfyAndGotify(t *testing.T) {
	tests := []struct {
		channelType string
		config      map[string]any
	}{
		{
			channelType: notifications.ChannelTypeNtfy,
			config: map[string]any{
				"server_url": "https://ntfy.example.invalid",
				"topic":      "operations",
			},
		},
		{
			channelType: notifications.ChannelTypeGotify,
			config: map[string]any{
				"server_url": "https://gotify.example.invalid",
				"app_token":  "synthetic-test-token",
			},
		},
	}
	for _, test := range tests {
		err := alertingpkg.ValidateCreateChannelRequest(notifications.CreateChannelRequest{
			Name:   "ops",
			Type:   test.channelType,
			Config: test.config,
		})
		if err != nil {
			t.Fatalf("validateCreateChannelRequest(%s) returned error: %v", test.channelType, err)
		}
	}
}
