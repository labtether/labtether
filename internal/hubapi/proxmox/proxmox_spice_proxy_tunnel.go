package proxmox

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
)

const (
	maxProxmoxSPICEProxyHeaderBytes = 16 * 1024
	maxProxmoxSPICEProxyLineBytes   = 4096
)

var proxmoxSPICEProxyTicketPattern = regexp.MustCompile(`^pvespiceproxy:[a-f0-9]{8}:[0-9]+:[^\s:]+(?::[0-9]+)?::[a-f0-9]{40}$`)

type bufferedSPICEProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedSPICEProxyConn) Read(buffer []byte) (int, error) {
	return c.reader.Read(buffer)
}

func validateProxmoxSPICEProxyTicket(host string, port int) (string, error) {
	host = strings.TrimSpace(host)
	if port <= 0 || port > 65535 || !proxmoxSPICEProxyTicketPattern.MatchString(host) {
		return "", errors.New("invalid Proxmox SPICE proxy ticket")
	}
	return host + ":" + strconv.Itoa(port), nil
}

func parseProxmoxSPICEProxyEndpoint(rawProxy string) (string, int, error) {
	parsed, err := neturl.Parse(strings.TrimSpace(rawProxy))
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() == "" || parsed.User != nil ||
		(parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", 0, errors.New("invalid Proxmox SPICE proxy URL")
	}
	return securityruntime.ValidateOutboundHostPort(parsed.Hostname(), parsed.Port(), 80)
}

func dialProxmoxSPICEProxyTunnel(ctx context.Context, rawProxy, ticketHost string, tlsPort int) (net.Conn, error) {
	connectTarget, err := validateProxmoxSPICEProxyTicket(ticketHost, tlsPort)
	if err != nil {
		return nil, err
	}
	proxyHost, proxyPort, err := parseProxmoxSPICEProxyEndpoint(rawProxy)
	if err != nil {
		return nil, err
	}
	proxyConn, err := securityruntime.DialOutboundTCPContext(ctx, proxyHost, proxyPort, 10*time.Second)
	if err != nil {
		return nil, err
	}
	closeWithError := func(err error) (net.Conn, error) {
		_ = proxyConn.Close()
		return nil, err
	}
	if err := proxyConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return closeWithError(err)
	}
	request := "CONNECT " + connectTarget + " HTTP/1.1\r\nHost: " + connectTarget + "\r\nProxy-Connection: Keep-Alive\r\n\r\n"
	if _, err := proxyConn.Write([]byte(request)); err != nil {
		return closeWithError(fmt.Errorf("write Proxmox SPICE proxy request: %w", err))
	}

	reader := bufio.NewReaderSize(proxyConn, maxProxmoxSPICEProxyLineBytes)
	statusLine, total, err := readBoundedProxyLine(reader, 0)
	if err != nil {
		return closeWithError(err)
	}
	statusParts := strings.SplitN(statusLine, " ", 3)
	if len(statusParts) < 2 || (statusParts[0] != "HTTP/1.1" && statusParts[0] != "HTTP/1.0") {
		return closeWithError(errors.New("invalid Proxmox SPICE proxy response"))
	}
	statusCode, err := strconv.Atoi(statusParts[1])
	if err != nil {
		return closeWithError(errors.New("invalid Proxmox SPICE proxy response"))
	}
	for {
		line, nextTotal, err := readBoundedProxyLine(reader, total)
		if err != nil {
			return closeWithError(err)
		}
		total = nextTotal
		if line == "" {
			break
		}
	}
	if statusCode != 200 {
		return closeWithError(fmt.Errorf("Proxmox SPICE proxy rejected connection with HTTP %d", statusCode))
	}
	if err := proxyConn.SetDeadline(time.Time{}); err != nil {
		return closeWithError(err)
	}
	return &bufferedSPICEProxyConn{Conn: proxyConn, reader: reader}, nil
}

func readBoundedProxyLine(reader *bufio.Reader, total int) (string, int, error) {
	line, err := reader.ReadString('\n')
	total += len(line)
	if total > maxProxmoxSPICEProxyHeaderBytes || len(line) > maxProxmoxSPICEProxyLineBytes {
		return "", total, errors.New("Proxmox SPICE proxy response headers too large")
	}
	if err != nil {
		return "", total, errors.New("invalid Proxmox SPICE proxy response")
	}
	if !strings.HasSuffix(line, "\r\n") {
		return "", total, errors.New("invalid Proxmox SPICE proxy response")
	}
	return strings.TrimSuffix(line, "\r\n"), total, nil
}
