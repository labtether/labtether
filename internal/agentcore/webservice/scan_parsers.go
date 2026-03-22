package webservice

import (
	"sort"
	"strconv"
	"strings"
)

func parsePortValue(raw string) int {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return 0
	}
	port, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0
	}
	if port <= 0 || port > 65535 {
		return 0
	}
	return port
}

func parsePortList(raw string) []int {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		switch r {
		case ',', ';', ' ', '\n', '\t':
			return true
		default:
			return false
		}
	})

	seen := make(map[int]struct{}, len(fields))
	ports := make([]int, 0, len(fields))
	for _, field := range fields {
		value := strings.TrimSpace(field)
		if value == "" {
			continue
		}
		port, err := strconv.Atoi(value)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		if _, ok := seen[port]; ok {
			continue
		}
		seen[port] = struct{}{}
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

// parseProcNetListeningPorts parses Linux /proc/net/tcp(/tcp6) data and returns
// TCP listening ports (state 0A).
func parseProcNetListeningPorts(raw string) []int {
	lines := strings.Split(raw, "\n")
	if len(lines) == 0 {
		return nil
	}

	ports := make([]int, 0, len(lines))
	for _, line := range lines[1:] { // skip header
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 4 {
			continue
		}
		// st column: 0A == LISTEN
		if !strings.EqualFold(fields[3], "0A") {
			continue
		}
		localAddr := fields[1]
		idx := strings.LastIndex(localAddr, ":")
		if idx < 0 || idx+1 >= len(localAddr) {
			continue
		}
		portHex := localAddr[idx+1:]
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		ports = append(ports, int(port))
	}
	return ports
}
