package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/demo"
)

const (
	demoFleetInterval = 30 * time.Second
	// The full Docker inventory goes out on the first cycle and then every
	// tenth one (five minutes).
	demoFleetInventoryEvery = 10
)

// runDemoFleet plays the demo's pretend machines. Every cycle each online
// host sends its messages through the same router that handles a real
// agent's WebSocket messages, so devices, charts, containers and services in
// the demo are produced by the code that produces them on a real hub.
//
// The hosts have no connection: nothing can be sent to them, and the hub
// refuses real agent connections in demo mode.
func (s *apiServer) runDemoFleet(ctx context.Context) {
	router := s.buildWSRouter()
	hosts := demo.OnlineHosts()
	conns := make(map[string]*agentmgr.AgentConn, len(hosts))
	for _, host := range hosts {
		conns[host.ID] = agentmgr.NewAgentConn(nil, host.ID, host.Platform)
	}
	log.Printf("demo: fleet started (%d hosts, every %s)", len(hosts), demoFleetInterval)

	ticker := time.NewTicker(demoFleetInterval)
	defer ticker.Stop()
	for cycle := 0; ; cycle++ {
		version := s.demoFleetAgentVersion()
		now := time.Now()
		for _, host := range hosts {
			for _, msg := range host.Messages(now, version, cycle%demoFleetInventoryEvery == 0) {
				if handle, ok := router[msg.Type]; ok {
					handle(conns[host.ID], msg)
				}
			}
		}
		select {
		case <-ctx.Done():
			log.Println("demo: fleet stopped")
			return
		case <-ticker.C:
		}
	}
}

// demoFleetConnectivity answers the device status view for the demo fleet. A
// device made by an agent reads as offline while that agent has no connection
// to this hub. The pretend hosts have no connection, so the view asks here
// instead: a host that sends messages is connected, the offline one is not.
//
// Only the status view uses this. The agent manager still holds no
// connection for these hosts, so nothing can be sent to them.
type demoFleetConnectivity struct {
	online map[string]bool
}

func newDemoFleetConnectivity() demoFleetConnectivity {
	online := make(map[string]bool)
	for _, host := range demo.OnlineHosts() {
		online[host.ID] = true
	}
	return demoFleetConnectivity{online: online}
}

func (c demoFleetConnectivity) IsConnected(assetID string) bool {
	return c.online[strings.TrimSpace(assetID)]
}

// demoFleetAgentVersion is the agent version this hub ships, or empty when
// the image carries no agent manifest.
func (s *apiServer) demoFleetAgentVersion() string {
	if s.agentCache == nil {
		return ""
	}
	manifest := s.agentCache.Manifest()
	if manifest == nil {
		return ""
	}
	return manifest.GoAgentVersion()
}
