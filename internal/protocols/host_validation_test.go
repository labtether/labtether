package protocols

import "testing"

func TestValidateManualDeviceHost(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		wantErr bool
	}{
		{"private 192.168", "192.168.1.50", false},
		{"private 10.x", "10.0.0.1", false},
		{"private 172.16", "172.16.0.1", false},
		{"private 172.31", "172.31.255.255", false},
		{"public IP", "8.8.8.8", false},
		{"hostname", "nas.local", false},
		{"fqdn", "server.example.com", false},
		{"loopback", "127.0.0.1", true},
		{"loopback full", "127.0.0.2", true},
		{"link-local", "169.254.1.1", true},
		{"metadata", "169.254.169.254", true},
		{"empty", "", true},
		{"localhost", "localhost", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManualDeviceHost(tt.host)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManualDeviceHost(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}
