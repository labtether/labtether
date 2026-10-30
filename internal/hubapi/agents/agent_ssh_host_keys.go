package agents

import (
	"crypto/rsa"
	"strings"

	"golang.org/x/crypto/ssh"
)

const (
	// maxAgentSSHHostKeys and maxAgentSSHHostKeyBytes bound what an install
	// reply may carry. The byte limit matches the terminal config API's.
	maxAgentSSHHostKeys     = 16
	maxAgentSSHHostKeyBytes = 2048
	minAgentSSHHostRSABits  = 2048
)

// pinnedHostKeyFromAgent picks the host key the Hub pins for its SSH login
// from the host keys an authenticated agent read on its device. The saved
// login holds one key, so it takes the strongest valid one (ed25519, then
// ECDSA, then RSA of at least 2048 bits) in authorized_keys form without a
// comment. Invalid, certificate and other key types are dropped. It returns
// "" when none is usable.
func pinnedHostKeyFromAgent(hostKeys []string) string {
	best, bestRank := "", 0
	for index, raw := range hostKeys {
		if index >= maxAgentSSHHostKeys {
			break
		}
		if len(raw) > maxAgentSSHHostKeyBytes {
			continue
		}
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(raw))
		if err != nil {
			continue
		}
		if rank := agentHostKeyRank(key); rank > bestRank {
			best, bestRank = strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))), rank
		}
	}
	return best
}

func agentHostKeyRank(key ssh.PublicKey) int {
	switch key.Type() {
	case ssh.KeyAlgoED25519:
		return 3
	case ssh.KeyAlgoECDSA256, ssh.KeyAlgoECDSA384, ssh.KeyAlgoECDSA521:
		return 2
	case ssh.KeyAlgoRSA:
		cryptoKey, ok := key.(ssh.CryptoPublicKey)
		if !ok {
			return 0
		}
		if rsaKey, ok := cryptoKey.CryptoPublicKey().(*rsa.PublicKey); ok && rsaKey.N.BitLen() >= minAgentSSHHostRSABits {
			return 1
		}
	}
	return 0
}
