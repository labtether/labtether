package truenas_test

import (
	"net/http/httptest"
	"testing"

	"github.com/labtether/labtether/internal/hubapi/testutil"
	"github.com/labtether/labtether/internal/hubapi/testutil/hubdeps"
	"github.com/labtether/labtether/internal/hubcollector"
	"github.com/labtether/labtether/internal/logs"
	"github.com/labtether/labtether/internal/persistence"
)

// TrueNAS fixtures with the signatures of the cmd/labtether helpers they
// replace, so the tests moved from cmd read as they did there. The shared
// parts are in testutil (truenas_fixtures.go); cmd keeps its own wrappers.

type trueNASRPCError = testutil.TrueNASRPCError

func newTrueNASRPCServer(t *testing.T, handler func(method string, params []any) (any, *trueNASRPCError)) *httptest.Server {
	t.Helper()
	return testutil.NewTrueNASRPCServer(t, handler)
}

func createTrueNASCredentialProfile(t *testing.T, sut *hubdeps.Hub, credentialID, secret, baseURL string) {
	t.Helper()
	testutil.CreateTrueNASCredentialProfile(t, sut.CredentialStore, sut.SecretsManager, credentialID, secret, baseURL)
}

func configureTrueNASCollectors(t *testing.T, sut *hubdeps.Hub, collectors ...hubcollector.Collector) {
	t.Helper()
	sut.HubCollectorStore = &testutil.StubHubCollectorStore{Collectors: collectors}
}

func seedTrueNASAsset(t *testing.T, sut *hubdeps.Hub, assetID, collectorID string) {
	t.Helper()
	testutil.SeedTrueNASAsset(t, sut.AssetStore, assetID, collectorID)
}

// failingTrueNASQueryLogStore is a copy of the cmd test type of the same
// name (cmd's truenas_api_handlers_helpers_test.go still uses its own).
type failingTrueNASQueryLogStore struct {
	persistence.LogStore
	err error
}

func (s *failingTrueNASQueryLogStore) QueryEvents(req logs.QueryRequest) ([]logs.Event, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.LogStore.QueryEvents(req)
}
