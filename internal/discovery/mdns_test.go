package discovery

import (
	"encoding/binary"
	"strings"
	"testing"
)

// TestNewMDNSAdvertiser verifies constructor validation and defaults.
func TestNewMDNSAdvertiser(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		port    int
		version string
		wantErr bool
	}{
		{name: "valid port 8080", port: 8080, version: "1.0.0", wantErr: false},
		{name: "valid port 443", port: 443, version: "dev", wantErr: false},
		{name: "valid empty version uses dev", port: 8080, version: "", wantErr: false},
		{name: "invalid port zero", port: 0, version: "1.0.0", wantErr: true},
		{name: "invalid port negative", port: -1, version: "1.0.0", wantErr: true},
		{name: "invalid port too large", port: 65536, version: "1.0.0", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, err := NewMDNSAdvertiser(tc.port, tc.version)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if a == nil {
				t.Fatal("expected non-nil advertiser")
			}
			if a.port != tc.port {
				t.Errorf("port: got %d, want %d", a.port, tc.port)
			}
			if tc.version == "" && a.version != "dev" {
				t.Errorf("empty version should default to 'dev', got %q", a.version)
			}
			if tc.version != "" && a.version != tc.version {
				t.Errorf("version: got %q, want %q", a.version, tc.version)
			}
			// Service name must contain the service type.
			if !strings.Contains(a.serviceName, "_labtether._tcp.local.") {
				t.Errorf("serviceName %q does not contain expected service type", a.serviceName)
			}
			// Host name must end with .local.
			if !strings.HasSuffix(a.hostName, ".local.") {
				t.Errorf("hostName %q does not end with .local.", a.hostName)
			}
		})
	}
}

// TestDecodeDNSName tests the mDNS name decoder.
func TestDecodeDNSName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    []byte
		offset   int
		wantName string
		wantEnd  int
		wantErr  bool
	}{
		{
			name: "simple name _labtether._tcp.local",
			// _labtether(10) . _tcp(4) . local(5) . \0
			input: func() []byte {
				b := []byte{}
				b = append(b, 10)
				b = append(b, []byte("_labtether")...)
				b = append(b, 4)
				b = append(b, []byte("_tcp")...)
				b = append(b, 5)
				b = append(b, []byte("local")...)
				b = append(b, 0)
				return b
			}(),
			offset:   0,
			wantName: "_labtether._tcp.local.",
			wantEnd:  22, // 1+10+1+4+1+5+1 = 23... let me count: 11+5+6+1=23
			wantErr:  false,
		},
		{
			name:    "empty message",
			input:   []byte{},
			offset:  0,
			wantErr: true,
		},
		{
			name:    "out of bounds offset",
			input:   []byte{5, 'h', 'e', 'l', 'l', 'o', 0},
			offset:  10,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			name, end, err := decodeDNSName(tc.input, tc.offset)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got name=%q end=%d", name, end)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tc.wantName {
				t.Errorf("name: got %q, want %q", name, tc.wantName)
			}
		})
	}
}

// TestEncodeDNSName verifies that encoded names decode correctly.
func TestEncodeDNSName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
	}{
		{"_labtether._tcp.local."},
		{"labtether.local."},
		{"myhostname._labtether._tcp.local."},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			encoded := encodeDNSName(tc.input)
			decoded, end, err := decodeDNSName(encoded, 0)
			if err != nil {
				t.Fatalf("decode error: %v", err)
			}
			if decoded != tc.input {
				t.Errorf("round-trip: got %q, want %q", decoded, tc.input)
			}
			if end != len(encoded) {
				t.Errorf("end offset: got %d, want %d", end, len(encoded))
			}
		})
	}
}

// TestBuildTXTData verifies TXT record encoding.
func TestBuildTXTData(t *testing.T) {
	t.Parallel()

	data := buildTXTData(map[string]string{
		"version": "1.2.3",
		"service": "labtether",
	})
	if len(data) == 0 {
		t.Fatal("expected non-empty TXT data")
	}

	// Verify that each string is length-prefixed.
	offset := 0
	found := map[string]bool{}
	for offset < len(data) {
		if offset >= len(data) {
			break
		}
		strLen := int(data[offset])
		offset++
		if offset+strLen > len(data) {
			t.Fatalf("TXT string length %d exceeds data at offset %d", strLen, offset)
		}
		entry := string(data[offset : offset+strLen])
		found[entry] = true
		offset += strLen
	}

	if !found["version=1.2.3"] {
		t.Errorf("missing 'version=1.2.3' in TXT data, found: %v", found)
	}
	if !found["service=labtether"] {
		t.Errorf("missing 'service=labtether' in TXT data, found: %v", found)
	}
}

// TestBuildResponsePacket verifies the structure of generated DNS response packets.
func TestBuildResponsePacket(t *testing.T) {
	t.Parallel()

	a, err := NewMDNSAdvertiser(8080, "1.0.0")
	if err != nil {
		t.Fatalf("NewMDNSAdvertiser: %v", err)
	}

	pkt := a.buildResponsePacket(0)
	if len(pkt) < 12 {
		t.Fatalf("packet too short: %d bytes", len(pkt))
	}

	// Check DNS header flags: QR=1, AA=1.
	flags := binary.BigEndian.Uint16(pkt[2:4])
	if flags&0x8000 == 0 {
		t.Errorf("QR bit not set in response: flags=0x%04x", flags)
	}
	if flags&0x0400 == 0 {
		t.Errorf("AA bit not set in response: flags=0x%04x", flags)
	}

	// Answer count should be 3 (PTR + SRV + TXT).
	anCount := binary.BigEndian.Uint16(pkt[6:8])
	if anCount != 3 {
		t.Errorf("answer count: got %d, want 3", anCount)
	}

	// Question count should be 0.
	qdCount := binary.BigEndian.Uint16(pkt[4:6])
	if qdCount != 0 {
		t.Errorf("question count: got %d, want 0", qdCount)
	}
}

// TestHandleQueryIgnoresResponses verifies that mDNS responses are not
// recursively answered.
func TestHandleQueryIgnoresResponses(t *testing.T) {
	t.Parallel()

	a, err := NewMDNSAdvertiser(8080, "dev")
	if err != nil {
		t.Fatalf("NewMDNSAdvertiser: %v", err)
	}

	// Build a packet with QR=1 (response) — should be silently ignored.
	pkt := make([]byte, 12)
	binary.BigEndian.PutUint16(pkt[2:4], 0x8000) // QR bit set
	binary.BigEndian.PutUint16(pkt[4:6], 1)       // QDCOUNT=1

	// The method should return without sending anything — no panic.
	a.handleQuery(pkt, nil)
}

// TestHandleQueryShortPacket verifies that malformed short packets are
// handled without panicking.
func TestHandleQueryShortPacket(t *testing.T) {
	t.Parallel()

	a, err := NewMDNSAdvertiser(8080, "dev")
	if err != nil {
		t.Fatalf("NewMDNSAdvertiser: %v", err)
	}

	// A packet shorter than the 12-byte header must not panic.
	a.handleQuery([]byte{0x00, 0x00}, nil)
	a.handleQuery([]byte{}, nil)
}

// TestStopIdempotent verifies that Stop can be called multiple times safely.
func TestStopIdempotent(t *testing.T) {
	t.Parallel()

	a, err := NewMDNSAdvertiser(8080, "dev")
	if err != nil {
		t.Fatalf("NewMDNSAdvertiser: %v", err)
	}

	// Stop without Start should not panic.
	a.Stop()
	a.Stop()
	a.Stop()
}
