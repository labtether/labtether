package portainer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/assets"
	pnconnector "github.com/labtether/labtether/internal/connectors/portainer"
)

// A stack's environment variables are often secrets. The Hub reads them from
// Portainer to keep them across a redeploy or a compose edit, but its stacks
// list must never pass them on to the console.
func TestPortainerStacksListOmitsStackEnvironment(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/stacks" {
			t.Errorf("unexpected Portainer request %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"Id":7,"Name":"db","EndpointId":3,"Env":[{"name":"DB_PASSWORD","value":"s3cret"}]}]`))
	}))
	defer upstream.Close()
	runtime := &PortainerRuntime{Client: pnconnector.NewClient(pnconnector.Config{BaseURL: upstream.URL, APIKey: "k"})}
	asset := assets.Asset{ID: "portainer-endpoint-3", Metadata: map[string]string{"endpoint_id": "3"}}
	recorder := httptest.NewRecorder()

	(&Deps{}).HandlePortainerStacks(context.Background(), recorder,
		httptest.NewRequest(http.MethodGet, "/portainer/assets/portainer-endpoint-3/stacks", nil), asset, runtime, nil)

	body := recorder.Body.String()
	if recorder.Code != http.StatusOK || !strings.Contains(body, `"db"`) {
		t.Fatalf("stacks list = %d %s, want the db stack", recorder.Code, body)
	}
	if strings.Contains(body, "s3cret") || strings.Contains(body, "DB_PASSWORD") || strings.Contains(body, `"Env"`) {
		t.Fatalf("stacks list leaks the stack environment: %s", body)
	}
}
