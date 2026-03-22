package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/protocols"
)

const (
	protocolHealthCheckInterval = 5 * time.Minute
	protocolHealthStaleAfter    = 30 * time.Minute
	protocolHealthMaxConcurrent = 5
)

// runProtocolHealthChecker periodically re-tests enabled protocol configs that
// haven't been tested recently. Follows the synthetic runner pattern.
func (s *apiServer) runProtocolHealthChecker(ctx context.Context) {
	// Wait for startup to settle.
	select {
	case <-ctx.Done():
		return
	case <-time.After(90 * time.Second):
	}

	ticker := time.NewTicker(protocolHealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkStaleProtocolConfigs(ctx)
		}
	}
}

func (s *apiServer) checkStaleProtocolConfigs(ctx context.Context) {
	if s.db == nil {
		return
	}

	configs, err := s.db.ListStaleProtocolConfigs(ctx, protocolHealthStaleAfter)
	if err != nil {
		log.Printf("protocol-health: failed to list stale configs: %v", err)
		return
	}
	if len(configs) == 0 {
		return
	}

	sem := make(chan struct{}, protocolHealthMaxConcurrent)
	var wg sync.WaitGroup

	for _, pc := range configs {
		select {
		case <-ctx.Done():
			break
		default:
		}

		sem <- struct{}{}
		wg.Add(1)
		go func(pc *protocols.ProtocolConfig) {
			defer wg.Done()
			defer func() { <-sem }()
			s.testProtocolConfig(ctx, pc)
		}(pc)
	}
	wg.Wait()
}

func (s *apiServer) testProtocolConfig(ctx context.Context, pc *protocols.ProtocolConfig) {
	// Resolve host.
	host := strings.TrimSpace(pc.Host)
	if host == "" && s.assetStore != nil {
		asset, ok, err := s.assetStore.GetAsset(pc.AssetID)
		if err == nil && ok {
			host = strings.TrimSpace(asset.Host)
		}
	}
	if host == "" {
		return // no host to test
	}

	// Decrypt credential if present.
	var password, privateKey string
	if pc.CredentialProfileID != "" && s.credentialStore != nil && s.secretsManager != nil {
		profile, ok, err := s.credentialStore.GetCredentialProfile(pc.CredentialProfileID)
		if err == nil && ok {
			secret, decErr := s.secretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
			if decErr == nil {
				switch profile.Kind {
				case credentials.KindSSHPassword, credentials.KindTelnetPassword,
					credentials.KindRDPPassword, credentials.KindVNCPassword:
					password = secret
				case credentials.KindSSHPrivateKey, credentials.KindHubSSHIdentity:
					privateKey = secret
				}
			}
		}
	}

	// Run the test.
	testCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var result *protocols.TestResult
	switch pc.Protocol {
	case protocols.ProtocolSSH:
		username := strings.TrimSpace(pc.Username)
		var hostKeyCallback ssh.HostKeyCallback
		var sshCfg protocols.SSHConfig
		if len(pc.Config) > 0 {
			_ = json.Unmarshal(pc.Config, &sshCfg)
		}
		if sshCfg.StrictHostKey && sshCfg.HostKey != "" {
			hostPub, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(sshCfg.HostKey))
			if parseErr == nil {
				hostKeyCallback = ssh.FixedHostKey(hostPub)
			}
		}
		result = protocols.TestSSH(testCtx, host, pc.Port, username, password, privateKey, hostKeyCallback)
	case protocols.ProtocolTelnet:
		result = protocols.TestTelnet(testCtx, host, pc.Port)
	case protocols.ProtocolVNC:
		result = protocols.TestVNC(testCtx, host, pc.Port)
	case protocols.ProtocolRDP:
		guacdHost := strings.TrimSpace(os.Getenv("GUACD_HOST"))
		guacdPort := strings.TrimSpace(os.Getenv("GUACD_PORT"))
		var guacdAddr string
		if guacdHost != "" {
			if guacdPort == "" {
				guacdPort = "4822"
			}
			guacdAddr = guacdHost + ":" + guacdPort
		}
		result = protocols.TestRDP(testCtx, host, pc.Port, guacdAddr)
	case protocols.ProtocolARD:
		result = protocols.TestARD(testCtx, host, pc.Port)
	default:
		return
	}

	// Persist result.
	status := "success"
	testErr := ""
	if !result.Success {
		status = "failed"
		testErr = result.Error
	}
	if updateErr := s.db.UpdateProtocolTestResult(ctx, pc.AssetID, pc.Protocol, status, testErr); updateErr != nil {
		log.Printf("protocol-health: failed to store result for %s/%s: %v", pc.AssetID, pc.Protocol, updateErr)
	}
}
