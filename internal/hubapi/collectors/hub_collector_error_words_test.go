package collectors

import (
	"strings"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/hubcollector"
)

type statusWordsStore struct {
	hubCollectorHandlerStore
	lastError string
}

func (s *statusWordsStore) UpdateHubCollectorStatus(_ string, _ string, lastError string, _ time.Time) error {
	s.lastError = lastError
	return nil
}

// Add device's "sync failed" line showed the collector's raw Go error, with
// the system's address and the transport detail.
func TestFailedCollectionIsStoredInWords(t *testing.T) {
	store := &statusWordsStore{}
	store.collector = hubcollector.Collector{ID: "c1", CollectorType: "portainer"}
	d := &Deps{HubCollectorStore: store}
	cases := map[string]string{
		`portainer discovery failed: portainer endpoints: Get "https://192.0.2.10:9443/api/endpoints?limit=10": dial tcp 192.0.2.10:9443: connect: connection refused`: "Could not reach Portainer. Check the connection's address and port.",
		`portainer discovery failed: portainer api returned 401: {"message":"Invalid API key"}`:                                                                        "Portainer refused the login.",
		`portainer discovery failed: portainer api returned 403: forbidden`:                                                                                            "Portainer refused part of the collection for the saved login.",
		`home assistant discovery failed: insecure url scheme "http" requires LABTETHER_ALLOW_INSECURE_TRANSPORT=true`:                                                 "over https only",
		`portainer discovery failed: portainer api returned 500: boom`:                                                                                                 "Portainer answered with an error during collection.",
	}
	for raw, want := range cases {
		d.UpdateCollectorStatus("c1", "error", raw)
		if !strings.Contains(store.lastError, want) || strings.Contains(store.lastError, "192.0.2.10") || strings.Contains(store.lastError, "dial tcp") {
			t.Errorf("%q stored as %q, want %q", raw, store.lastError, want)
		}
	}
	d.UpdateCollectorStatus("c1", "partial", "2 datastores could not be read")
	if store.lastError != "2 datastores could not be read" {
		t.Errorf("a message already in words changed to %q", store.lastError)
	}
}
