package wol

import (
	"net"
	"testing"
)

func TestBuildMagicPacket(t *testing.T) {
	mac, err := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	if err != nil {
		t.Fatalf("parse mac: %v", err)
	}

	packet := BuildMagicPacket(mac)
	if len(packet) != 102 {
		t.Fatalf("expected 102 bytes, got %d", len(packet))
	}

	for i := 0; i < 6; i++ {
		if packet[i] != 0xFF {
			t.Fatalf("byte %d should be 0xFF, got 0x%02X", i, packet[i])
		}
	}

	for rep := 0; rep < 16; rep++ {
		offset := 6 + rep*6
		for j := 0; j < 6; j++ {
			if packet[offset+j] != mac[j] {
				t.Fatalf("rep %d byte %d mismatch", rep, j)
			}
		}
	}
}

func TestParseMACAddress(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{input: "AA:BB:CC:DD:EE:FF"},
		{input: "aa:bb:cc:dd:ee:ff"},
		{input: "AA-BB-CC-DD-EE-FF"},
		{input: "AA:BB:CC:DD:EE:FF:00:11", wantErr: true},
		{input: "invalid", wantErr: true},
		{input: "", wantErr: true},
	}

	for _, tc := range tests {
		_, err := ParseMAC(tc.input)
		if (err != nil) != tc.wantErr {
			t.Errorf("ParseMAC(%q) error=%v, wantErr=%v", tc.input, err, tc.wantErr)
		}
	}
}
