//go:build darwin

package sysconfig

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

func PlatformListDisplays() ([]agentmgr.DisplayInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := securityruntime.CommandContextOutput(ctx, "system_profiler", "SPDisplaysDataType", "-json")
	if err != nil {
		log.Printf("display: system_profiler failed: %v", err)
		return nil, err
	}

	type displayEntry struct {
		Name       string `json:"_name"`
		Resolution string `json:"_spdisplays_resolution"`
		Main       string `json:"spdisplays_main,omitempty"`
	}
	type gpuEntry struct {
		Displays []displayEntry `json:"spdisplays_ndrvs"`
	}
	var payload struct {
		GPUs []gpuEntry `json:"SPDisplaysDataType"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		log.Printf("display: parse system_profiler json failed: %v", err)
		return nil, err
	}

	displays := make([]agentmgr.DisplayInfo, 0, 4)
	for _, gpu := range payload.GPUs {
		for _, display := range gpu.Displays {
			width, height := ParseResolution(display.Resolution)
			displays = append(displays, agentmgr.DisplayInfo{
				Name:    strings.TrimSpace(display.Name),
				Width:   width,
				Height:  height,
				Primary: strings.EqualFold(strings.TrimSpace(display.Main), "spdisplays_yes"),
			})
		}
	}
	return displays, nil
}
