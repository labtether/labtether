package terminal

import (
	"net"
	"strings"
)

// agentPinnedHostKey returns the SSH host key the Hub holds for host:port on
// this device: the key on its saved Hub key login, which the authenticated
// agent reported for that address. It returns "" for any other address or
// port, so a key is only ever used for the machine it came from.
func (d *Deps) agentPinnedHostKey(assetID, host string, port int) string {
	if d.CredentialStore == nil || d.GetAssetTerminalConfig == nil {
		return ""
	}
	saved, ok, err := d.GetAssetTerminalConfig(assetID)
	if err != nil || !ok {
		return ""
	}
	savedPort := saved.Port
	if savedPort <= 0 {
		savedPort = 22
	}
	if savedPort != port || !sameSSHHost(saved.Host, host) {
		return ""
	}
	return strings.TrimSpace(saved.HostKey)
}

func sameSSHHost(left, right string) bool {
	left = strings.TrimSuffix(strings.Trim(strings.TrimSpace(left), "[]"), ".")
	right = strings.TrimSuffix(strings.Trim(strings.TrimSpace(right), "[]"), ".")
	if left == "" || right == "" {
		return false
	}
	leftIP, rightIP := net.ParseIP(left), net.ParseIP(right)
	if leftIP != nil || rightIP != nil {
		return leftIP != nil && rightIP != nil && leftIP.Equal(rightIP)
	}
	return strings.EqualFold(left, right)
}
