package main

import (
	"testing"

	"github.com/labtether/labtether/internal/notifications"
)

func TestValidateCreateChannelRequestAcceptsNtfyAndGotify(t *testing.T) {
	for _, channelType := range []string{notifications.ChannelTypeNtfy, notifications.ChannelTypeGotify} {
		err := validateCreateChannelRequest(notifications.CreateChannelRequest{
			Name: "ops",
			Type: channelType,
		})
		if err != nil {
			t.Fatalf("validateCreateChannelRequest(%s) returned error: %v", channelType, err)
		}
	}
}
