package desktop

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/guacamole"
	"github.com/labtether/labtether/internal/servicehttp"
)

const (
	defaultGUACDHost = "127.0.0.1"
	defaultGUACDPort = 4822

	// rdpGatewayProbeTimeout bounds the check that the gateway accepts a
	// connection. A gateway that is there answers in milliseconds.
	rdpGatewayProbeTimeout = 2 * time.Second

	// rdpGatewayNotRunning is what a person reads when RDP cannot be served.
	// The gateway is an optional part of an install (the compose profile
	// "remote-desktop"), and the install guide calls it the RDP gateway
	// (guacd). This used to be a 502 "guacd unavailable", which reached people
	// as "An internal error occurred." or as a stream that never opened.
	rdpGatewayNotRunning = "RDP needs the Hub's RDP gateway (guacd), which is not running on this Hub."
)

// rdpGatewayAddress is where this Hub looks for its RDP gateway.
func rdpGatewayAddress() (string, int) {
	host := strings.TrimSpace(os.Getenv("GUACD_HOST"))
	if host == "" {
		host = defaultGUACDHost
	}
	port := defaultGUACDPort
	if raw := strings.TrimSpace(os.Getenv("GUACD_PORT")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			port = parsed
		}
	}
	return host, port
}

// rdpGatewayReachable reports whether this Hub can serve RDP now: its gateway
// accepts a connection. The address is always set (compose points it at the
// "guacd" service whether or not that service runs), so only a connection
// tells a Hub with a gateway from one without.
func rdpGatewayReachable(ctx context.Context) bool {
	host, port := rdpGatewayAddress()
	probeCtx, cancel := context.WithTimeout(ctx, rdpGatewayProbeTimeout)
	defer cancel()
	client, err := guacamole.ConnectContext(probeCtx, host, port)
	if err != nil {
		return false
	}
	_ = client.Close()
	return true
}

// writeRDPGatewayNotRunning answers an RDP request this Hub cannot serve.
// 409, like the other Remote View answers a person has to read: a 5xx is
// replaced with "An internal error occurred." on the way out.
func writeRDPGatewayNotRunning(w http.ResponseWriter) {
	servicehttp.WriteError(w, http.StatusConflict, rdpGatewayNotRunning)
}
