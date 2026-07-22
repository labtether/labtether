package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/auth"
	"github.com/labtether/labtether/internal/credentials"
	"github.com/labtether/labtether/internal/persistence"
	"github.com/labtether/labtether/internal/secrets"
)

type failingHubKeyCredentialStore struct {
	persistence.CredentialStore
	err error
}

func (s failingHubKeyCredentialStore) UpdateCredentialProfileSecret(
	_ string,
	_ string,
	_ string,
	_ *time.Time,
) (credentials.Profile, error) {
	return credentials.Profile{}, s.err
}

func initializeTestHubSSHIdentity(t *testing.T, server *apiServer) *hubSSHIdentity {
	t.Helper()
	identity, err := ensureHubSSHIdentity(server)
	if err != nil {
		t.Fatalf("initialize hub SSH identity: %v", err)
	}
	server.setHubSSHIdentity(identity)
	if server.agentMgr == nil {
		server.agentMgr = agentmgr.NewManager()
	}
	return identity
}

func newHubSSHKeyRotationRequest(t *testing.T, actorID, body string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/settings/ssh-hub-key/rotate", strings.NewReader(body))
	request = request.WithContext(contextWithPrincipal(request.Context(), actorID, auth.RoleAdmin))
	request.Header.Set("Content-Type", "application/json")
	return request
}

func persistedHubSSHPublicKey(t *testing.T, server *apiServer, profileID string) string {
	t.Helper()
	profile, ok, err := server.credentialStore.GetCredentialProfile(profileID)
	if err != nil || !ok {
		t.Fatalf("load persisted hub profile: ok=%v err=%v", ok, err)
	}
	privateKeyPEM, err := server.secretsManager.DecryptString(profile.SecretCiphertext, profile.ID)
	if err != nil {
		t.Fatalf("decrypt persisted hub key: %v", err)
	}
	signer, err := ssh.ParsePrivateKey([]byte(privateKeyPEM))
	if err != nil {
		t.Fatalf("parse persisted hub key: %v", err)
	}
	return string(ssh.MarshalAuthorizedKey(signer.PublicKey()))
}

func readHubSSHKeyMessage(t *testing.T, conn *websocket.Conn, wantType string) string {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set websocket read deadline: %v", err)
	}
	var message agentmgr.Message
	if err := conn.ReadJSON(&message); err != nil {
		t.Fatalf("read %s message: %v", wantType, err)
	}
	if message.Type != wantType {
		t.Fatalf("message type=%q, want %q", message.Type, wantType)
	}
	var payload struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.Unmarshal(message.Data, &payload); err != nil {
		t.Fatalf("decode %s payload: %v", wantType, err)
	}
	return payload.PublicKey
}

func TestSSHHubKeyInfoAndRestartDerivePersistedType(t *testing.T) {
	server := newTestAPIServer(t)
	created := initializeTestHubSSHIdentity(t, server)
	if created.KeyType != "ed25519" {
		t.Fatalf("created key type=%q, want ed25519", created.KeyType)
	}

	server.setHubSSHIdentity(nil)
	loaded, err := ensureHubSSHIdentity(server)
	if err != nil {
		t.Fatalf("reload hub SSH identity: %v", err)
	}
	if loaded.KeyType != "ed25519" {
		t.Fatalf("reloaded key type=%q, want ed25519", loaded.KeyType)
	}
	server.setHubSSHIdentity(loaded)

	response := httptest.NewRecorder()
	server.handleHubSSHPublicKey(response, httptest.NewRequest(http.MethodGet, "/settings/ssh-hub-key", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("key info status=%d body=%s", response.Code, response.Body.String())
	}
	var info sshHubKeyInfo
	if err := json.Unmarshal(response.Body.Bytes(), &info); err != nil {
		t.Fatalf("decode key info: %v", err)
	}
	if info.PublicKey != loaded.PublicKey || info.KeyType != "ed25519" || !strings.HasPrefix(info.FingerprintSHA256, "SHA256:") {
		t.Fatalf("unexpected key info: %+v", info)
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache control=%q, want no-store", response.Header().Get("Cache-Control"))
	}
}

func TestSSHHubKeyRestartDerivesPersistedRSATypeInsteadOfEnvironmentDefault(t *testing.T) {
	server := newTestAPIServer(t)
	t.Setenv("LABTETHER_SSH_KEY_TYPE", "rsa")
	created := initializeTestHubSSHIdentity(t, server)
	if created.KeyType != "rsa" {
		t.Fatalf("created key type=%q, want rsa", created.KeyType)
	}

	server.setHubSSHIdentity(nil)
	t.Setenv("LABTETHER_SSH_KEY_TYPE", "ed25519")
	loaded, err := ensureHubSSHIdentity(server)
	if err != nil {
		t.Fatalf("reload RSA hub identity: %v", err)
	}
	if loaded.KeyType != "rsa" {
		t.Fatalf("reloaded key type=%q, want persisted rsa", loaded.KeyType)
	}
}

func TestSSHHubKeyRestartNeverDeletesIdentityOnDecryptionFailure(t *testing.T) {
	server := newTestAPIServer(t)
	created := initializeTestHubSSHIdentity(t, server)
	alternateKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
	alternateManager, err := secrets.NewManagerFromEncodedKey(alternateKey)
	if err != nil {
		t.Fatalf("initialize alternate secrets manager: %v", err)
	}
	server.secretsManager = alternateManager
	server.setHubSSHIdentity(nil)

	if _, err := ensureHubSSHIdentity(server); err == nil {
		t.Fatal("expected persisted identity decryption failure")
	}
	if _, ok, err := server.credentialStore.GetCredentialProfile(created.ProfileID); err != nil || !ok {
		t.Fatalf("existing identity was deleted after decryption failure: ok=%v err=%v", ok, err)
	}
}

func TestSSHHubKeyRotationStagesBeforePersistAndRemovesOldKeyAfterward(t *testing.T) {
	server := newTestAPIServer(t)
	oldIdentity := initializeTestHubSSHIdentity(t, server)

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	server.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-ssh-rotation", "linux"))
	defer server.agentMgr.Unregister("node-ssh-rotation")

	response := httptest.NewRecorder()
	request := newHubSSHKeyRotationRequest(t, "user-admin-1", `{"key_type":"ed25519","reason":"LTQA-245","confirm":"ROTATE"}`)
	server.handleSSHHubKeyRotate(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("rotation status=%d body=%s", response.Code, response.Body.String())
	}

	stagedKey := readHubSSHKeyMessage(t, clientConn, agentmgr.MsgSSHKeyInstall)
	removedKey := readHubSSHKeyMessage(t, clientConn, agentmgr.MsgSSHKeyRemove)
	if strings.TrimSpace(stagedKey) == strings.TrimSpace(oldIdentity.PublicKey) {
		t.Fatal("rotation staged the old public key")
	}
	if strings.TrimSpace(removedKey) != strings.TrimSpace(oldIdentity.PublicKey) {
		t.Fatalf("removed key does not match previous public key")
	}

	current := server.currentHubSSHIdentity()
	if current == nil || strings.TrimSpace(current.PublicKey) != strings.TrimSpace(stagedKey) {
		t.Fatalf("active identity does not match staged key: %+v", current)
	}
	if persisted := persistedHubSSHPublicKey(t, server, oldIdentity.ProfileID); strings.TrimSpace(persisted) != strings.TrimSpace(stagedKey) {
		t.Fatal("persisted private key does not match staged public key")
	}

	events, err := server.auditStore.List(10, 0)
	if err != nil || len(events) == 0 {
		t.Fatalf("load rotation audit event: count=%d err=%v", len(events), err)
	}
	event := events[0]
	if event.Type != sshHubKeyRotationAuditType || event.ActorID != "user-admin-1" || event.Decision != "applied" {
		t.Fatalf("unexpected audit event: %+v", event)
	}
	encodedEvent, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode audit event: %v", err)
	}
	for _, keyMaterial := range []string{strings.TrimSpace(oldIdentity.PublicKey), strings.TrimSpace(stagedKey), "OPENSSH PRIVATE KEY"} {
		if keyMaterial != "" && bytes.Contains(encodedEvent, []byte(keyMaterial)) {
			t.Fatalf("audit event contains SSH key material: %s", encodedEvent)
		}
	}
}

func TestSSHHubKeyRotationPersistenceFailureKeepsOldIdentity(t *testing.T) {
	server := newTestAPIServer(t)
	oldIdentity := initializeTestHubSSHIdentity(t, server)
	baseStore := server.credentialStore
	server.credentialStore = failingHubKeyCredentialStore{
		CredentialStore: baseStore,
		err:             errors.New("injected persistence failure"),
	}

	serverConn, clientConn, cleanup := createWSPairForNetworkTest(t)
	defer cleanup()
	server.agentMgr.Register(agentmgr.NewAgentConn(serverConn, "node-ssh-persist-failure", "linux"))
	defer server.agentMgr.Unregister("node-ssh-persist-failure")

	response := httptest.NewRecorder()
	request := newHubSSHKeyRotationRequest(t, "user-admin-2", `{"reason":"failure ordering","confirm":"ROTATE"}`)
	server.handleSSHHubKeyRotate(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("rotation status=%d body=%s", response.Code, response.Body.String())
	}

	stagedKey := readHubSSHKeyMessage(t, clientConn, agentmgr.MsgSSHKeyInstall)
	rolledBackKey := readHubSSHKeyMessage(t, clientConn, agentmgr.MsgSSHKeyRemove)
	if strings.TrimSpace(stagedKey) != strings.TrimSpace(rolledBackKey) {
		t.Fatal("persistence failure did not roll back the newly staged key")
	}
	if strings.TrimSpace(stagedKey) == strings.TrimSpace(oldIdentity.PublicKey) {
		t.Fatal("test did not stage a distinct replacement key")
	}
	current := server.currentHubSSHIdentity()
	if current == nil || strings.TrimSpace(current.PublicKey) != strings.TrimSpace(oldIdentity.PublicKey) {
		t.Fatalf("active identity changed after persistence failure: %+v", current)
	}
	server.credentialStore = baseStore
	if persisted := persistedHubSSHPublicKey(t, server, oldIdentity.ProfileID); strings.TrimSpace(persisted) != strings.TrimSpace(oldIdentity.PublicKey) {
		t.Fatal("persisted identity changed after injected failure")
	}
}

func TestSSHHubKeyRotationSerializesConcurrentRequests(t *testing.T) {
	server := newTestAPIServer(t)
	identity := initializeTestHubSSHIdentity(t, server)

	const rotations = 2
	start := make(chan struct{})
	responses := make([]*httptest.ResponseRecorder, rotations)
	var wait sync.WaitGroup
	for index := 0; index < rotations; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			responses[index] = httptest.NewRecorder()
			request := newHubSSHKeyRotationRequest(t, "concurrent-admin", `{"reason":"concurrency proof","confirm":"ROTATE"}`)
			server.handleSSHHubKeyRotate(responses[index], request)
		}(index)
	}
	close(start)
	wait.Wait()

	for index, response := range responses {
		if response.Code != http.StatusOK {
			t.Fatalf("rotation %d status=%d body=%s", index, response.Code, response.Body.String())
		}
	}
	current := server.currentHubSSHIdentity()
	if current == nil || current.ProfileID != identity.ProfileID {
		t.Fatalf("unexpected active identity: %+v", current)
	}
	if persisted := persistedHubSSHPublicKey(t, server, identity.ProfileID); strings.TrimSpace(persisted) != strings.TrimSpace(current.PublicKey) {
		t.Fatal("concurrent rotations left memory and persistence out of sync")
	}
	events, err := server.auditStore.List(rotations, 0)
	if err != nil || len(events) != rotations {
		t.Fatalf("rotation audit count=%d, want %d (err=%v)", len(events), rotations, err)
	}
}

func TestSSHHubKeyRotationRejectsUnboundedOrUnconfirmedInput(t *testing.T) {
	server := newTestAPIServer(t)
	initializeTestHubSSHIdentity(t, server)

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{name: "missing confirmation", body: `{"key_type":"ed25519"}`, wantStatus: http.StatusBadRequest},
		{name: "inexact confirmation", body: `{"confirm":" ROTATE "}`, wantStatus: http.StatusBadRequest},
		{name: "unsupported key type", body: `{"key_type":"dsa","confirm":"ROTATE"}`, wantStatus: http.StatusBadRequest},
		{name: "unknown field", body: `{"confirm":"ROTATE","private_key":"must-not-be-accepted"}`, wantStatus: http.StatusBadRequest},
		{name: "control character in reason", body: `{"reason":"line one\nline two","confirm":"ROTATE"}`, wantStatus: http.StatusBadRequest},
		{name: "reason too long", body: `{"reason":"` + strings.Repeat("x", maxSSHHubKeyRotationReasonLen+1) + `","confirm":"ROTATE"}`, wantStatus: http.StatusBadRequest},
		{name: "body too large", body: `{"reason":"` + strings.Repeat("x", maxSSHHubKeyRotationBodyBytes+1) + `","confirm":"ROTATE"}`, wantStatus: http.StatusRequestEntityTooLarge},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			request := newHubSSHKeyRotationRequest(t, "bounded-admin", test.body)
			request.RemoteAddr = fmt.Sprintf("192.0.2.%d:1234", index+1)
			server.handleSSHHubKeyRotate(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status=%d, want %d body=%s", response.Code, test.wantStatus, response.Body.String())
			}
		})
	}
}

func TestSSHHubKeyRoutesEnforceGlobalAdminAuthorization(t *testing.T) {
	server := newTestAPIServer(t)
	initializeTestHubSSHIdentity(t, server)
	handlers := server.buildHTTPHandlers(nil, nil, nil)

	globalReadKey := createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"hub:read"}, nil)
	if response := invokeLegacyRoute(t, handlers["/settings/ssh-hub-key"], http.MethodGet, "/settings/ssh-hub-key", globalReadKey, ""); response.Code != http.StatusOK {
		t.Fatalf("global admin read status=%d body=%s", response.Code, response.Body.String())
	}

	restrictedReadKey := createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"hub:read"}, []string{"node-1"})
	for _, route := range []string{"/settings/ssh-hub-key", "/hub/ssh-public-key"} {
		response := invokeLegacyRoute(t, handlers[route], http.MethodGet, route, restrictedReadKey, "")
		if response.Code != http.StatusForbidden {
			t.Fatalf("restricted read route=%s status=%d body=%s", route, response.Code, response.Body.String())
		}
	}

	restrictedAdminKey := createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"hub:admin"}, []string{"node-1"})
	restrictedRotation := invokeLegacyRoute(
		t,
		handlers["/settings/ssh-hub-key/rotate"],
		http.MethodPost,
		"/settings/ssh-hub-key/rotate",
		restrictedAdminKey,
		`{"confirm":"ROTATE"}`,
	)
	if restrictedRotation.Code != http.StatusForbidden {
		t.Fatalf("restricted rotation status=%d body=%s", restrictedRotation.Code, restrictedRotation.Body.String())
	}

	globalAdminKey := createLegacyRouteAPIKeyWithRole(t, server, auth.RoleAdmin, []string{"hub:admin"}, nil)
	globalRotation := invokeLegacyRoute(
		t,
		handlers["/settings/ssh-hub-key/rotate"],
		http.MethodPost,
		"/settings/ssh-hub-key/rotate",
		globalAdminKey,
		`{"confirm":"ROTATE"}`,
	)
	if globalRotation.Code != http.StatusOK {
		t.Fatalf("global rotation status=%d body=%s", globalRotation.Code, globalRotation.Body.String())
	}
}
