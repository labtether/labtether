package agentcore

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
)

// handleSSHKeyInstall installs the hub's SSH public key into the current user's
// authorized_keys file, enabling zero-config SSH access from the hub.
func handleSSHKeyInstall(transport *wsTransport, msg agentmgr.Message) {
	var req agentmgr.SSHKeyInstallData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("ssh-key: invalid install request: %v", err)
		return
	}

	pubKey := strings.TrimSpace(req.PublicKey)
	if pubKey == "" {
		log.Printf("ssh-key: empty public key in install request")
		return
	}

	authKeysPath, err := resolveAuthorizedKeysPath()
	if err != nil {
		log.Printf("ssh-key: failed to resolve authorized_keys path: %v", err)
		return
	}

	if err := installPublicKey(authKeysPath, pubKey); err != nil {
		log.Printf("ssh-key: failed to install public key: %v", err)
		return
	}

	// Send confirmation back to hub.
	currentUser, _ := user.Current()
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()

	username := ""
	if currentUser != nil {
		username = currentUser.Username
	}

	resp := agentmgr.SSHKeyInstalledData{
		Username: username,
		Hostname: hostname,
		HomeDir:  homeDir,
	}
	data, _ := json.Marshal(resp)
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgSSHKeyInstalled,
		Data: data,
	})

	log.Printf("ssh-key: installed hub public key for user %s", username)
}

// handleSSHKeyRemove removes the hub's SSH public key from the current user's
// authorized_keys file, used during asset decommissioning.
func handleSSHKeyRemove(transport *wsTransport, msg agentmgr.Message) {
	var req agentmgr.SSHKeyRemoveData
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("ssh-key: invalid remove request: %v", err)
		return
	}

	pubKey := strings.TrimSpace(req.PublicKey)
	if pubKey == "" {
		log.Printf("ssh-key: empty public key in remove request")
		return
	}

	authKeysPath, err := resolveAuthorizedKeysPath()
	if err != nil {
		log.Printf("ssh-key: failed to resolve authorized_keys path: %v", err)
		return
	}

	if err := removePublicKey(authKeysPath, pubKey); err != nil {
		log.Printf("ssh-key: failed to remove public key: %v", err)
		return
	}

	resp := agentmgr.SSHKeyInstalledData{} // reuse for confirmation
	currentUser, _ := user.Current()
	hostname, _ := os.Hostname()
	homeDir, _ := os.UserHomeDir()
	if currentUser != nil {
		resp.Username = currentUser.Username
	}
	resp.Hostname = hostname
	resp.HomeDir = homeDir

	data, _ := json.Marshal(resp)
	_ = transport.Send(agentmgr.Message{
		Type: agentmgr.MsgSSHKeyRemoved,
		Data: data,
	})

	log.Printf("ssh-key: removed hub public key from %s", authKeysPath)
}

// removePublicKey removes a matching public key line from authorized_keys.
func removePublicKey(authKeysPath, pubKey string) error {
	existing, err := os.ReadFile(authKeysPath) // #nosec G304 -- authorized_keys path is derived from the managed SSH home directory.
	if err != nil {
		if os.IsNotExist(err) {
			return nil // nothing to remove
		}
		return fmt.Errorf("failed to read authorized_keys: %w", err)
	}

	keyLine := strings.TrimSpace(pubKey)
	lines := strings.Split(string(existing), "\n")
	filtered := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		if strings.TrimSpace(line) == keyLine {
			removed = true
			continue
		}
		filtered = append(filtered, line)
	}

	if !removed {
		return nil // key was not present
	}

	content := strings.Join(filtered, "\n")
	if err := os.WriteFile(authKeysPath, []byte(content), 0600); err != nil { // #nosec G703 -- authorized_keys path is derived from the managed SSH home directory.
		return fmt.Errorf("failed to write authorized_keys: %w", err)
	}
	return nil
}

// resolveAuthorizedKeysPath determines the appropriate authorized_keys file path.
func resolveAuthorizedKeysPath() (string, error) {
	if runtime.GOOS == "windows" {
		// Check if running as SYSTEM (common for Windows services).
		currentUser, err := user.Current()
		if err == nil && strings.EqualFold(currentUser.Username, "NT AUTHORITY\\SYSTEM") {
			return `C:\ProgramData\ssh\administrators_authorized_keys`, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		return filepath.Join(home, ".ssh", "authorized_keys"), nil
	}

	// Linux, macOS, FreeBSD
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".ssh", "authorized_keys"), nil
}

// installPublicKey idempotently appends a public key to authorized_keys.
func installPublicKey(authKeysPath, pubKey string) error {
	// Ensure .ssh directory exists with correct permissions.
	sshDir := filepath.Dir(authKeysPath)
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return fmt.Errorf("failed to create .ssh directory: %w", err)
	}

	// Read existing authorized_keys.
	existing, err := os.ReadFile(authKeysPath) // #nosec G304 -- authorized_keys path is derived from the managed SSH home directory.
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read authorized_keys: %w", err)
	}

	// Check if the key is already present.
	keyLine := strings.TrimSpace(pubKey)
	if len(existing) > 0 {
		for _, line := range strings.Split(string(existing), "\n") {
			if strings.TrimSpace(line) == keyLine {
				log.Printf("ssh-key: public key already present in %s", authKeysPath)
				return nil
			}
		}
	}

	// Append the key.
	content := string(existing)
	if len(content) > 0 && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += keyLine + "\n"

	if err := os.WriteFile(authKeysPath, []byte(content), 0600); err != nil { // #nosec G703 -- authorized_keys path is derived from the managed SSH home directory.
		return fmt.Errorf("failed to write authorized_keys: %w", err)
	}

	return nil
}
