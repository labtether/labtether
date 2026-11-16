package openapi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/labtether/labtether/internal/persistence"
)

func TestV2OpenAPIFileTransferListContractIsImplementedAndBounded(t *testing.T) {
	var document struct {
		Paths map[string]map[string]any `json:"paths"`
	}
	if err := json.Unmarshal([]byte(Document), &document); err != nil {
		t.Fatalf("decode OpenAPI: %v", err)
	}
	operation, ok := document.Paths["/api/v2/file-transfers"]["get"].(map[string]any)
	if !ok {
		t.Fatal("GET /api/v2/file-transfers missing from OpenAPI")
	}
	responses, ok := operation["responses"].(map[string]any)
	if !ok || responses["200"] == nil || responses["501"] != nil {
		t.Fatalf("responses=%v, want implemented 200 contract without 501", operation["responses"])
	}
	parameters, ok := operation["parameters"].([]any)
	if !ok || len(parameters) != 3 {
		t.Fatalf("parameters=%v, want status, limit, and offset", operation["parameters"])
	}
	seen := make(map[string]map[string]any, len(parameters))
	for _, rawParameter := range parameters {
		parameter, _ := rawParameter.(map[string]any)
		name, _ := parameter["name"].(string)
		schema, _ := parameter["schema"].(map[string]any)
		seen[name] = schema
	}
	if seen["limit"]["maximum"] != float64(persistence.FileTransferListMaxLimit) || seen["offset"]["maximum"] != float64(persistence.FileTransferListMaxOffset) {
		t.Fatalf("pagination schemas=%v, want limits %d/%d", seen, persistence.FileTransferListMaxLimit, persistence.FileTransferListMaxOffset)
	}
	statusEnum, _ := seen["status"]["enum"].([]any)
	if len(statusEnum) != 4 {
		t.Fatalf("status schema=%v, want four persisted states", seen["status"])
	}
	if description, _ := operation["description"].(string); !strings.Contains(description, "authenticated actor") || !strings.Contains(description, "newest-first") {
		t.Fatalf("description=%q is missing isolation or ordering semantics", description)
	}
}
