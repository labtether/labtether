package portainer

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Redeploy called Portainer's git redeploy for every stack, so a stack made in
// Portainer's editor (no repository) could not be redeployed. Updates also sent
// no environment, and Portainer cleared the stack's variables.
func TestRedeployEditorStackRedeploysItsFileAndKeepsEnv(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	var put map[string]any
	var putPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/stacks/5":
			_, _ = w.Write([]byte(`{"Id":5,"Name":"web","EndpointId":1,"Env":[{"name":"DB_PASSWORD","value":"s3cret"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/stacks/5/file":
			_, _ = w.Write([]byte(`{"StackFileContent":"services:\n  app:\n    image: busybox\n"}`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/stacks/5":
			putPath = r.URL.String()
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &put)
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()
	client := NewClient(Config{BaseURL: server.URL, APIKey: "k", Timeout: 5 * time.Second})

	if err := client.RedeployStack(context.Background(), 5, 1, true); err != nil {
		t.Fatalf("RedeployStack: %v", err)
	}
	if putPath != "/api/stacks/5?endpointId=1" || put["StackFileContent"] != "services:\n  app:\n    image: busybox\n" || put["PullImage"] != true {
		t.Fatalf("redeploy sent %s %v, want the stored file again with a pull", putPath, put)
	}
	env, _ := json.Marshal(put["Env"])
	if string(env) != `[{"name":"DB_PASSWORD","value":"s3cret"}]` {
		t.Fatalf("redeploy sent env %s, want the stack's own", env)
	}

	put = nil
	if err := client.UpdateStackCompose(context.Background(), 5, 1, "services: {}\n"); err != nil {
		t.Fatalf("UpdateStackCompose: %v", err)
	}
	env, _ = json.Marshal(put["Env"])
	if put["StackFileContent"] != "services: {}\n" || string(env) != `[{"name":"DB_PASSWORD","value":"s3cret"}]` {
		t.Fatalf("compose edit sent %v, want the new file and the stack's env", put)
	}
}
