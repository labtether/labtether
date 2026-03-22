package wol

import (
	"fmt"
	"net"
	"strings"
)

const magicPacketLength = 102

// BuildMagicPacket constructs a Wake-on-LAN magic packet:
// 6 bytes of 0xFF followed by the target MAC repeated 16 times.
func BuildMagicPacket(mac net.HardwareAddr) []byte {
	if len(mac) != 6 {
		return nil
	}
	packet := make([]byte, magicPacketLength)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 0; i < 16; i++ {
		copy(packet[6+i*6:], mac)
	}
	return packet
}

// ParseMAC parses a MAC address in standard notation (colon or dash).
func ParseMAC(raw string) (net.HardwareAddr, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty MAC address")
	}
	mac, err := net.ParseMAC(trimmed)
	if err != nil {
		return nil, err
	}
	if len(mac) != 6 {
		return nil, fmt.Errorf("expected 48-bit MAC address, got %d-bit", len(mac)*8)
	}
	return mac, nil
}

// Send transmits a magic packet to the provided UDP broadcast address.
// Example broadcastAddr: "255.255.255.255:9"
func Send(mac net.HardwareAddr, broadcastAddr string) error {
	packet := BuildMagicPacket(mac)
	if len(packet) != magicPacketLength {
		return fmt.Errorf("invalid MAC address length")
	}
	conn, err := net.Dial("udp", strings.TrimSpace(broadcastAddr))
	if err != nil {
		return fmt.Errorf("failed to dial broadcast: %w", err)
	}
	defer conn.Close()
	_, err = conn.Write(packet)
	return err
}
