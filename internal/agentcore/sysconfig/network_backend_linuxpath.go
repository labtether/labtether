package sysconfig

import "github.com/labtether/labtether/internal/agentmgr"

type LinuxNetworkBackend struct{}

func (LinuxNetworkBackend) ApplyAction(nm *NetworkManager, req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	return nm.ApplyActionLinux(req)
}

func (LinuxNetworkBackend) RollbackAction(nm *NetworkManager, req agentmgr.NetworkActionData) agentmgr.NetworkResultData {
	return nm.RollbackActionLinux(req)
}
