package main

import (
	"context"

	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/hubapi/dockerpkg"
	"github.com/labtether/labtether/internal/hubapi/shared"
)

// Resolve provider deps only inside callbacks: constructing the shared action
// map must not recursively initialize bridges which depend on one another.
func (s *apiServer) configuredConnectorActions() map[string]shared.ConfiguredConnectorAction {
	ha := shared.ConfiguredConnectorAction{
		Bind: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
			return s.ensureHomeAssistantDeps().BindAction(ctx, action, req)
		},
		Execute: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
			return s.ensureHomeAssistantDeps().ExecuteAction(ctx, action, req)
		},
	}
	return map[string]shared.ConfiguredConnectorAction{
		"home-assistant": ha,
		"homeassistant":  ha,
		"docker": {
			BindingParam: dockerpkg.ActionAgentBindingParam, UseCallerTimeout: true,
			Bind: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
				return s.ensureDockerDeps().BindAction(ctx, action, req)
			},
			Execute: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return s.ensureDockerDeps().ExecuteAction(ctx, action, req)
			},
		},
		"truenas": {Bind: s.bindTrueNASAction, Execute: s.executeTrueNASActionDirect},
		"pbs": {
			Bind: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
				return s.ensurePBSDeps().BindAction(ctx, action, req)
			},
			Execute: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return s.ensurePBSDeps().ExecuteAction(ctx, action, req)
			},
		},
		"portainer": {
			Bind: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionRequest, error) {
				return s.ensurePortainerDeps().BindAction(ctx, action, req)
			},
			Execute: func(ctx context.Context, action string, req connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
				return s.ensurePortainerDeps().ExecuteAction(ctx, action, req)
			},
		},
	}
}
