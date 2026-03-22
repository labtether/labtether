package terminal

import "testing"

func TestValidateQuickConnectHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		// Valid hosts
		{"private IPv4 192.168", "192.168.1.50", false},
		{"private IPv4 10.x", "10.0.0.1", false},
		{"hostname simple", "myserver", false},
		{"hostname fqdn", "server.home.lab", false},
		{"hostname with dashes", "my-server-01", false},
		{"public IPv4", "8.8.8.8", false},

		// Invalid hosts
		{"empty", "", true},
		{"spaces", "my server", true},
		{"link-local 169.254", "169.254.169.254", true},
		{"metadata AWS", "169.254.169.254", true},
		{"all zeros", "0.0.0.0", true},
		{"broadcast", "255.255.255.255", true},
		{"loopback IPv4", "127.0.0.1", true},
		{"loopback IPv6", "::1", true},
		{"localhost", "localhost", true},
		{"too long", string(make([]byte, 300)), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateQuickConnectHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateQuickConnectHost(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}
