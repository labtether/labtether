//go:build !windows

package main

import "github.com/labtether/labtether/internal/agentcore"

func handleWindowsServiceArgs(_ []string) bool                                          { return false }
func isWindowsService() bool                                                            { return false }
func runAsWindowsService(_ agentcore.RuntimeConfig, _ agentcore.TelemetryProvider) error { return nil }
