package proxmox

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDialProxmoxSPICEProxyTunnelSendsExactTicketAndPreservesBufferedBytes(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	ticket := "pvespiceproxy:68b8d480:101:pve01:61000::" + strings.Repeat("a", 40)
	wantRequestLine := "CONNECT " + ticket + ":61000 HTTP/1.1"
	requestLine := make(chan string, 1)
	serverErr := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverErr <- err
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, err := reader.ReadString('\n')
		if err != nil {
			serverErr <- err
			return
		}
		requestLine <- strings.TrimSuffix(line, "\r\n")
		for {
			line, err = reader.ReadString('\n')
			if err != nil {
				serverErr <- err
				return
			}
			if line == "\r\n" {
				break
			}
		}
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\nREADY"); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	proxyURL := "http://" + listener.Addr().String()
	conn, err := dialProxmoxSPICEProxyTunnel(context.Background(), proxyURL, ticket, 61000)
	if err != nil {
		t.Fatalf("dial SPICE proxy tunnel: %v", err)
	}
	defer conn.Close()

	if got := <-requestLine; got != wantRequestLine {
		t.Fatalf("proxy request line=%q, want %q", got, wantRequestLine)
	}
	marker := make([]byte, len("READY"))
	if _, err := io.ReadFull(conn, marker); err != nil {
		t.Fatalf("read buffered proxy bytes: %v", err)
	}
	if string(marker) != "READY" {
		t.Fatalf("buffered proxy bytes=%q", marker)
	}
	if err := <-serverErr; err != nil {
		t.Fatalf("proxy server: %v", err)
	}
}

func TestProxmoxSPICEProxyInputValidation(t *testing.T) {
	validTicket := "pvespiceproxy:68b8d480:101:pve01::" + strings.Repeat("b", 40)
	if target, err := validateProxmoxSPICEProxyTicket(validTicket, 61000); err != nil || target != validTicket+":61000" {
		t.Fatalf("valid proxy ticket target=%q err=%v", target, err)
	}

	for _, invalidTicket := range []string{
		"pvespiceproxy:68b8d480:101:pve01::" + strings.Repeat("b", 39),
		validTicket + "\r\nInjected: true",
		"node.example",
	} {
		if _, err := validateProxmoxSPICEProxyTicket(invalidTicket, 61000); err == nil {
			t.Fatalf("accepted invalid proxy ticket %q", invalidTicket)
		}
	}

	for _, invalidProxy := range []string{
		"https://pve.example:3128",
		"http://user@pve.example:3128",
		"http://pve.example:3128/path",
		"http://pve.example:3128?x=1",
	} {
		if _, _, err := parseProxmoxSPICEProxyEndpoint(invalidProxy); err == nil {
			t.Fatalf("accepted invalid proxy URL %q", invalidProxy)
		}
	}
}

func TestDialProxmoxSPICEProxyTunnelRejectsNonSuccess(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOWLIST_MODE", "false")
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		reader := bufio.NewReader(conn)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil || line == "\r\n" {
				break
			}
		}
		_, _ = fmt.Fprint(conn, "HTTP/1.1 403 Forbidden\r\n\r\n")
	}()

	ticket := "pvespiceproxy:68b8d480:101:pve01::" + strings.Repeat("c", 40)
	if conn, err := dialProxmoxSPICEProxyTunnel(context.Background(), "http://"+listener.Addr().String(), ticket, 61000); err == nil {
		_ = conn.Close()
		t.Fatal("expected proxy HTTP rejection")
	}
}
