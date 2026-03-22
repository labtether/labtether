// Package discovery provides mDNS/DNS-SD service advertisement so that
// iOS companion apps can automatically discover the LabTether hub on the
// local network using NWBrowser with the "_labtether._tcp" service type.
package discovery

import (
	"encoding/binary"
	"fmt"
	"log"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	// mdnsMulticastAddr is the IPv4 mDNS multicast group address (RFC 6762).
	mdnsMulticastAddr = "224.0.0.251:5353"

	// dnsSDServiceType is the PTR name queried by NWBrowser on iOS.
	dnsSDServiceType = "_labtether._tcp.local."

	// recordTTL is the time-to-live for mDNS records in seconds.
	// RFC 6762 recommends 4500 s (75 min) for stable records; 120 s is
	// common for service records.
	recordTTL = 120

	// announceInterval is how often the hub proactively multicasts its
	// presence (unsolicited announcements).  Most discovery is driven by
	// query responses, but periodic announcements improve reliability.
	announceInterval = 60 * time.Second
)

// MDNSAdvertiser advertises the LabTether hub via mDNS/Bonjour (DNS-SD)
// so that iOS companion apps can discover it on the local network.
//
// It responds to DNS-SD PTR queries for "_labtether._tcp.local." and
// proactively multicasts its presence every announceInterval.
type MDNSAdvertiser struct {
	port        int
	version     string
	serviceName string // e.g. "labtether._labtether._tcp.local."
	hostName    string // e.g. "mymac.local."

	conn *net.UDPConn

	stopOnce sync.Once
	stopCh   chan struct{}
	wg       sync.WaitGroup
}

// NewMDNSAdvertiser creates a new advertiser for the LabTether hub.
//
// port is the hub's HTTP listen port.
// version is the application version string included in TXT records.
func NewMDNSAdvertiser(port int, version string) (*MDNSAdvertiser, error) {
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("discovery: invalid port %d", port)
	}
	if version == "" {
		version = "dev"
	}

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "labtether"
	}
	// Strip any existing .local suffix to avoid double-suffixing.
	hostname = strings.TrimSuffix(hostname, ".local")
	hostname = strings.TrimSuffix(hostname, ".local.")

	a := &MDNSAdvertiser{
		port:    port,
		version: version,
		// DNS-SD instance name: "<hostname>._labtether._tcp.local."
		serviceName: hostname + "." + dnsSDServiceType,
		// mDNS host name for SRV target: "<hostname>.local."
		hostName: hostname + ".local.",
		stopCh:   make(chan struct{}),
	}
	return a, nil
}

// Start begins advertising the LabTether service via mDNS/DNS-SD.
// It is safe to call Start only once; subsequent calls are no-ops.
func (a *MDNSAdvertiser) Start() error {
	addr, err := net.ResolveUDPAddr("udp4", mdnsMulticastAddr)
	if err != nil {
		return fmt.Errorf("discovery: resolve mDNS address: %w", err)
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return fmt.Errorf("discovery: listen mDNS multicast: %w", err)
	}
	a.conn = conn

	log.Printf("discovery: mDNS advertiser started — service %q on port %d", a.serviceName, a.port)

	a.wg.Add(2)
	go a.receiveLoop()
	go a.announceLoop()

	return nil
}

// Stop stops advertising and releases resources.
// It is safe to call Stop multiple times.
func (a *MDNSAdvertiser) Stop() {
	a.stopOnce.Do(func() {
		close(a.stopCh)
		if a.conn != nil {
			_ = a.conn.Close()
		}
		a.wg.Wait()
		log.Printf("discovery: mDNS advertiser stopped")
	})
}

// receiveLoop reads incoming mDNS packets and answers PTR/SRV/TXT queries.
func (a *MDNSAdvertiser) receiveLoop() {
	defer a.wg.Done()

	buf := make([]byte, 9000)
	for {
		n, src, err := a.conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-a.stopCh:
				return
			default:
				// Transient read error — log and continue.
				log.Printf("discovery: mDNS read error: %v", err)
				return
			}
		}

		msg := buf[:n]
		a.handleQuery(msg, src)
	}
}

// announceLoop sends unsolicited announcements at a regular interval.
func (a *MDNSAdvertiser) announceLoop() {
	defer a.wg.Done()

	// Send an initial announcement shortly after startup.
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()

	ticker := time.NewTicker(announceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-timer.C:
			a.sendAnnouncement()
		case <-ticker.C:
			a.sendAnnouncement()
		}
	}
}

// handleQuery parses an mDNS query packet and sends a response if it
// contains a PTR question for "_labtether._tcp.local.".
func (a *MDNSAdvertiser) handleQuery(msg []byte, src *net.UDPAddr) {
	if len(msg) < 12 {
		return
	}

	// DNS header: ID(2) FLAGS(2) QDCOUNT(2) ANCOUNT(2) NSCOUNT(2) ARCOUNT(2)
	flags := binary.BigEndian.Uint16(msg[2:4])
	// QR bit (bit 15): 0 = query, 1 = response. Skip responses.
	if flags&0x8000 != 0 {
		return
	}

	qdCount := binary.BigEndian.Uint16(msg[4:6])
	if qdCount == 0 {
		return
	}

	// Walk questions to see if any ask for our service type.
	offset := 12
	for i := uint16(0); i < qdCount; i++ {
		name, newOffset, err := decodeDNSName(msg, offset)
		if err != nil || newOffset+4 > len(msg) {
			return
		}
		qtype := binary.BigEndian.Uint16(msg[newOffset : newOffset+2])
		offset = newOffset + 4 // skip QTYPE(2) + QCLASS(2)

		// QTYPE 12 = PTR, 255 = ANY
		if (qtype == 12 || qtype == 255) && strings.EqualFold(name, dnsSDServiceType) {
			a.sendResponse(src)
			return
		}
		// Also respond to queries for our specific instance name (SRV/TXT).
		if (qtype == 33 || qtype == 16 || qtype == 255) && strings.EqualFold(name, a.serviceName) {
			a.sendResponse(src)
			return
		}
	}
}

// sendAnnouncement multicasts a DNS-SD response to the mDNS group.
func (a *MDNSAdvertiser) sendAnnouncement() {
	dst, err := net.ResolveUDPAddr("udp4", mdnsMulticastAddr)
	if err != nil {
		return
	}
	pkt := a.buildResponsePacket(0)
	if _, err := a.conn.WriteTo(pkt, dst); err != nil {
		// Non-fatal — the network may be briefly unavailable.
		log.Printf("discovery: mDNS announce error: %v", err)
	}
}

// sendResponse sends a unicast DNS-SD response to the querying host.
// For mDNS, responses may be multicast; we prefer unicast for efficiency.
func (a *MDNSAdvertiser) sendResponse(dst *net.UDPAddr) {
	pkt := a.buildResponsePacket(0)
	if _, err := a.conn.WriteTo(pkt, dst); err != nil {
		log.Printf("discovery: mDNS response error: %v", err)
	}
}

// buildResponsePacket constructs a minimal DNS-SD response with:
//   - PTR record: _labtether._tcp.local. → <instance>._labtether._tcp.local.
//   - SRV record: <instance>._labtether._tcp.local. → priority=0, weight=0, port, target
//   - TXT record: <instance>._labtether._tcp.local. → key=value pairs
//
// The packet is hand-assembled to avoid adding external dependencies.
func (a *MDNSAdvertiser) buildResponsePacket(queryID uint16) []byte {
	var b []byte

	txtData := buildTXTData(map[string]string{
		"version": a.version,
		"service": "labtether",
	})

	// --- DNS Header ---
	// ID: echo the query ID (0 for announcements)
	b = appendUint16(b, queryID)
	// FLAGS: QR=1 (response), AA=1 (authoritative), rest=0
	b = appendUint16(b, 0x8400)
	// QDCOUNT=0, ANCOUNT=3 (PTR + SRV + TXT), NSCOUNT=0, ARCOUNT=0
	b = appendUint16(b, 0)
	b = appendUint16(b, 3)
	b = appendUint16(b, 0)
	b = appendUint16(b, 0)

	// --- Answer 1: PTR record ---
	// NAME: _labtether._tcp.local.
	b = appendDNSName(b, dnsSDServiceType)
	// TYPE: PTR (12)
	b = appendUint16(b, 12)
	// CLASS: IN (1), with cache-flush bit clear (0x0001)
	b = appendUint16(b, 1)
	// TTL
	b = appendUint32(b, recordTTL)
	// RDATA: <instance>._labtether._tcp.local.
	rdata := encodeDNSName(a.serviceName)
	if len(rdata) > math.MaxUint16 {
		rdata = rdata[:math.MaxUint16]
	}
	b = appendUint16(b, uint16(len(rdata))) // #nosec G115 -- len(rdata) is capped to MaxUint16 above.
	b = append(b, rdata...)

	// --- Answer 2: SRV record ---
	// NAME: <instance>._labtether._tcp.local.
	b = appendDNSName(b, a.serviceName)
	// TYPE: SRV (33)
	b = appendUint16(b, 33)
	// CLASS: IN with cache-flush (0x8001)
	b = appendUint16(b, 0x8001)
	// TTL
	b = appendUint32(b, recordTTL)
	// RDATA: priority(2) + weight(2) + port(2) + target
	target := encodeDNSName(a.hostName)
	rdataLen := 6 + len(target)
	if rdataLen > math.MaxUint16 {
		return b
	}
	b = appendUint16(b, uint16(rdataLen))
	b = appendUint16(b, 0) // priority
	b = appendUint16(b, 0) // weight
	if a.port <= 0 || a.port > math.MaxUint16 {
		return b
	}
	b = appendUint16(b, uint16(a.port)) // port
	b = append(b, target...)

	// --- Answer 3: TXT record ---
	// NAME: <instance>._labtether._tcp.local.
	b = appendDNSName(b, a.serviceName)
	// TYPE: TXT (16)
	b = appendUint16(b, 16)
	// CLASS: IN with cache-flush (0x8001)
	b = appendUint16(b, 0x8001)
	// TTL
	b = appendUint32(b, recordTTL)
	// RDATA: length-prefixed strings
	if len(txtData) > math.MaxUint16 {
		txtData = txtData[:math.MaxUint16]
	}
	b = appendUint16(b, uint16(len(txtData))) // #nosec G115 -- len(txtData) is capped to MaxUint16 above.
	b = append(b, txtData...)

	return b
}

// buildTXTData encodes a map of key=value pairs as DNS TXT RDATA.
// Each string is prefixed by a single length byte (RFC 1035 §3.3.14).
func buildTXTData(pairs map[string]string) []byte {
	var buf []byte
	for k, v := range pairs {
		entry := k + "=" + v
		if len(entry) > 255 {
			entry = entry[:255]
		}
		entryLen := len(entry)
		if entryLen < 0 || entryLen > math.MaxUint8 {
			continue
		}
		buf = append(buf, byte(entryLen)) // #nosec G115 -- entryLen is bounded to MaxUint8 above.
		buf = append(buf, []byte(entry)...)
	}
	return buf
}

// decodeDNSName reads a DNS name from msg starting at offset.
// It handles pointer compression (RFC 1035 §4.1.4).
// Returns the decoded name (with trailing dot) and the offset after the name.
func decodeDNSName(msg []byte, offset int) (string, int, error) {
	var name strings.Builder
	visited := 0
	endOffset := -1

	for {
		if offset >= len(msg) {
			return "", 0, fmt.Errorf("discovery: DNS name decode out of bounds")
		}
		length := int(msg[offset])

		if length == 0 {
			// End of name.
			if endOffset == -1 {
				endOffset = offset + 1
			}
			break
		}

		if length&0xC0 == 0xC0 {
			// Pointer compression.
			if offset+1 >= len(msg) {
				return "", 0, fmt.Errorf("discovery: DNS pointer out of bounds")
			}
			if endOffset == -1 {
				endOffset = offset + 2
			}
			ptr := int(binary.BigEndian.Uint16(msg[offset:offset+2]) & 0x3FFF)
			offset = ptr
			visited++
			if visited > 10 {
				return "", 0, fmt.Errorf("discovery: DNS name pointer loop")
			}
			continue
		}

		if length&0xC0 != 0 {
			return "", 0, fmt.Errorf("discovery: unsupported DNS label type 0x%02x", length)
		}

		offset++
		if offset+length > len(msg) {
			return "", 0, fmt.Errorf("discovery: DNS label out of bounds")
		}
		if name.Len() > 0 {
			name.WriteByte('.')
		}
		name.Write(msg[offset : offset+length])
		offset += length
	}

	if name.Len() > 0 {
		name.WriteByte('.')
	} else {
		name.WriteByte('.')
	}

	return name.String(), endOffset, nil
}

// encodeDNSName encodes a DNS name (with trailing dot) into wire format.
// Pointer compression is not used; each label is encoded literally.
func encodeDNSName(name string) []byte {
	name = strings.TrimSuffix(name, ".")
	labels := strings.Split(name, ".")
	var buf []byte
	for _, label := range labels {
		if label == "" {
			continue
		}
		if len(label) > 63 {
			label = label[:63]
		}
		labelLen := len(label)
		if labelLen < 0 || labelLen > math.MaxUint8 {
			continue
		}
		buf = append(buf, byte(labelLen)) // #nosec G115 -- labelLen is bounded to MaxUint8 above.
		buf = append(buf, []byte(label)...)
	}
	buf = append(buf, 0) // root label
	return buf
}

// appendDNSName appends a DNS wire-format name to b.
func appendDNSName(b []byte, name string) []byte {
	return append(b, encodeDNSName(name)...)
}

func appendUint16(b []byte, v uint16) []byte {
	var out [2]byte
	binary.BigEndian.PutUint16(out[:], v)
	return append(b, out[:]...)
}

func appendUint32(b []byte, v uint32) []byte {
	var out [4]byte
	binary.BigEndian.PutUint32(out[:], v)
	return append(b, out[:]...)
}
