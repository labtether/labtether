package sysconfig

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// XrandrMonitorRe matches xrandr --listmonitors output lines.
//
// Example:
//
//	0: +*DP-1 1920/530x1080/300+0+0  DP-1
var XrandrMonitorRe = regexp.MustCompile(
	`^\s*\d+:\s+\+(\*?)(\S+)\s+(\d+)/\d+x(\d+)/\d+\+(-?\d+)\+(-?\d+)`,
)

// ParseXrandrMonitors extracts display info from xrandr --listmonitors output.
func ParseXrandrMonitors(output string) []agentmgr.DisplayInfo {
	lines := strings.Split(output, "\n")
	displays := make([]agentmgr.DisplayInfo, 0, len(lines))
	for _, line := range lines {
		matches := XrandrMonitorRe.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		width, _ := strconv.Atoi(matches[3])
		height, _ := strconv.Atoi(matches[4])
		offsetX, _ := strconv.Atoi(matches[5])
		offsetY, _ := strconv.Atoi(matches[6])
		displays = append(displays, agentmgr.DisplayInfo{
			Name:    matches[2],
			Width:   width,
			Height:  height,
			Primary: matches[1] == "*",
			OffsetX: offsetX,
			OffsetY: offsetY,
		})
	}
	return displays
}

// ParseResolution parses a macOS resolution string like "1920 x 1080 @ 60 Hz".
func ParseResolution(raw string) (int, int) {
	parts := strings.SplitN(strings.TrimSpace(raw), " x ", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	width, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	right := strings.Fields(parts[1])
	if len(right) == 0 {
		return width, 0
	}
	height, _ := strconv.Atoi(strings.TrimSpace(right[0]))
	return width, height
}

func ParsePowerShellScreenDisplays(output string) []agentmgr.DisplayInfo {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	displays := make([]agentmgr.DisplayInfo, 0, len(lines))
	for _, line := range lines {
		fields := strings.Split(strings.TrimSpace(line), "|")
		if len(fields) != 6 {
			continue
		}
		width, _ := strconv.Atoi(strings.TrimSpace(fields[1]))
		height, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
		offsetX, _ := strconv.Atoi(strings.TrimSpace(fields[4]))
		offsetY, _ := strconv.Atoi(strings.TrimSpace(fields[5]))
		name := strings.TrimSpace(fields[0])
		if name == "" {
			name = fmt.Sprintf("Display %d", len(displays)+1)
		}
		displays = append(displays, agentmgr.DisplayInfo{
			Name:    name,
			Width:   width,
			Height:  height,
			Primary: strings.EqualFold(strings.TrimSpace(fields[3]), "true"),
			OffsetX: offsetX,
			OffsetY: offsetY,
		})
	}
	return displays
}
