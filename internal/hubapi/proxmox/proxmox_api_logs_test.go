package proxmox

import "testing"

func TestParseProxmoxSyslogLimit(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want int
	}{
		{name: "empty", raw: "", want: defaultProxmoxSyslogLimit},
		{name: "valid", raw: "750", want: 750},
		{name: "whitespace", raw: " 750 ", want: 750},
		{name: "zero", raw: "0", want: defaultProxmoxSyslogLimit},
		{name: "negative", raw: "-1", want: defaultProxmoxSyslogLimit},
		{name: "malformed suffix", raw: "30abc", want: defaultProxmoxSyslogLimit},
		{name: "exponent", raw: "1e3", want: defaultProxmoxSyslogLimit},
		{name: "oversized", raw: "999999", want: maxProxmoxSyslogLimit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseProxmoxSyslogLimit(tt.raw); got != tt.want {
				t.Fatalf("parseProxmoxSyslogLimit(%q) = %d, want %d", tt.raw, got, tt.want)
			}
		})
	}
}
