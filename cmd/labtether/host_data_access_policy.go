package main

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/servicehttp"
)

// Viewers are read-only observers. They may see inventory, status, alerts and
// topology, but must not open interactive sessions or read data from inside a
// managed host. GET requests and WebSocket upgrades (which are GETs) to the
// routes below would otherwise pass the method-based role gate, so every method
// on them requires operator or higher. API keys stay bound by their scopes and
// asset allow-lists in addition to this role floor.

const hostDataRoleDeniedMessage = "operator role required"

// hostDataRoutePrefixes are whole route families that expose host data or
// interactive sessions.
var hostDataRoutePrefixes = []string{
	"/terminal/",        // terminal and docker-exec sessions, streams, tickets, history, snippets
	"/api/v2/terminal/", // v2 terminal sessions, history, snippets
	"/desktop/",         // VNC/RDP/SPICE/WebRTC streams, audio, tickets, diagnostics
	"/recordings",       // session recordings and their downloads
	"/files/",           // agent file browsing and downloads
	"/api/v1/file-connections",
	"/api/v1/file-transfers",
	"/api/v1/remote-bookmarks",
	"/api/v2/file-transfers",
	"/processes/",    // process command lines can carry secrets
	"/cron/",         // scheduled commands can carry secrets
	"/users/",        // host operating-system accounts
	"/logs/journal/", // on-demand host journal reads
}

// requestRequiresOperatorRole reports whether the request path reads host data
// or opens an interactive session.
func requestRequiresOperatorRole(path string) bool {
	for _, prefix := range hostDataRoutePrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	if rest := pathSegmentsAfter(path, "/api/v2/assets/"); len(rest) >= 2 {
		switch sub := rest[1]; {
		case strings.HasPrefix(sub, "files"), strings.HasPrefix(sub, "processes"):
			return true
		case sub == "cron", sub == "users", sub == "logs", sub == "exec":
			return true
		}
	}
	if rest := pathSegmentsAfter(path, "/api/v1/nodes/"); len(rest) >= 2 && rest[1] == "clipboard" {
		return true
	}
	for _, prefix := range []string{"/api/v1/docker/containers/", "/api/v2/docker/containers/"} {
		if rest := pathSegmentsAfter(path, prefix); len(rest) >= 2 && rest[1] == "logs" {
			return true
		}
	}
	for _, prefix := range []string{"/portainer/assets/", "/api/v2/portainer/assets/"} {
		rest := pathSegmentsAfter(path, prefix)
		if len(rest) < 4 {
			continue
		}
		switch {
		case rest[1] == "containers" && (rest[3] == "exec" || rest[3] == "logs" || rest[3] == "inspect"):
			return true
		case rest[1] == "stacks" && rest[3] == "compose":
			return true
		}
	}
	return false
}

// pathSegmentsAfter splits the path after prefix into trimmed segments. The
// route handlers trim segments too, so " exec" must classify like "exec".
func pathSegmentsAfter(path, prefix string) []string {
	if !strings.HasPrefix(path, prefix) {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(path, prefix), "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

// rejectHostDataForRole writes 403 and returns true when role may not use a
// host-data or interactive-session route.
func rejectHostDataForRole(w http.ResponseWriter, r *http.Request, role string) bool {
	if r == nil || r.URL == nil || !requestRequiresOperatorRole(r.URL.Path) {
		return false
	}
	if auth.HasWritePrivileges(role) {
		return false
	}
	servicehttp.WriteError(w, http.StatusForbidden, hostDataRoleDeniedMessage)
	return true
}

// mcpAuthorizeHostDataRead applies the same role floor to MCP tools that read
// files, process command lines or container logs from a host.
func mcpAuthorizeHostDataRead(ctx context.Context) error {
	if !auth.HasWritePrivileges(userRoleFromContext(ctx)) {
		return errors.New("reading host data requires operator role")
	}
	return nil
}
