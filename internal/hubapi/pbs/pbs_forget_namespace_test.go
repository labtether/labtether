package pbs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pbsconnector "github.com/labtether/labtether/internal/connectors/pbs"
)

// Listings now include namespaced groups. A forget that leaves the namespace
// out means the root, so it must not delete a root group when the caller was
// looking at a namespaced one with the same type and ID.
func TestForgetWithoutNamespaceIsRefusedWhenTheGroupIsNamespaced(t *testing.T) {
	t.Setenv("LABTETHER_OUTBOUND_ALLOW_LOOPBACK", "true")
	t.Setenv("LABTETHER_ALLOW_INSECURE_TRANSPORT", "true")
	listed := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		listed++
		switch {
		case strings.HasSuffix(r.URL.Path, "/namespace"):
			_, _ = w.Write([]byte(`{"data":[{"ns":"site-a"}]}`))
		case strings.HasSuffix(r.URL.Path, "/groups") && r.URL.Query().Get("ns") == "site-a":
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"}]}`))
		default:
			_, _ = w.Write([]byte(`{"data":[{"backup-type":"vm","backup-id":"100"},{"backup-type":"vm","backup-id":"200"}]}`))
		}
	}))
	defer server.Close()
	client, err := pbsconnector.NewClient(pbsconnector.Config{BaseURL: server.URL + "/api2/json", TokenID: "qa@pbs!t", TokenSecret: "s", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	forget := func(query, id string) (string, bool, *httptest.ResponseRecorder) {
		rec := httptest.NewRecorder()
		ns, ok := forgetNamespace(context.Background(), rec, httptest.NewRequest(http.MethodDelete, "/x?"+query, nil), client, "store-a", "vm", id)
		return ns, ok, rec
	}

	if _, ok, rec := forget("store=store-a", "100"); ok || rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Name the namespace") {
		t.Fatalf("no ns, vm/100 also in site-a: ok=%v %d %s, want refused", ok, rec.Code, rec.Body.String())
	}
	if ns, ok, _ := forget("store=store-a", "200"); !ok || ns != "" {
		t.Fatalf("no ns, vm/200 only in the root: ns=%q ok=%v, want the root", ns, ok)
	}
	before := listed
	if ns, ok, _ := forget("store=store-a&ns=site-a", "100"); !ok || ns != "site-a" || listed != before {
		t.Fatalf("ns named: ns=%q ok=%v, extra PBS calls %d, want site-a with no listing", ns, ok, listed-before)
	}
	if ns, ok, _ := forget("store=store-a&ns=", "100"); !ok || ns != "" {
		t.Fatalf("empty ns: ns=%q ok=%v, want the root as named", ns, ok)
	}
}
