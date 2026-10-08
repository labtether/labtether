package main

import (
	"context"
	"encoding/json"
	"flag"
	"github.com/labtether/labtether/internal/assets"
	"github.com/labtether/labtether/internal/connectorsdk"
	"github.com/labtether/labtether/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var updateCanonicalContractFixtures = flag.Bool(
	"update-canonical-contract-fixtures",
	false,
	"update canonical connector contract fixture baselines",
)

type canonicalContractConnector struct {
	id      string
	display string
	caps    connectorsdk.Capabilities
	actions []connectorsdk.ActionDescriptor
	assets  []connectorsdk.Asset
}

func (c *canonicalContractConnector) ID() string { return c.id }

func (c *canonicalContractConnector) DisplayName() string {
	if strings.TrimSpace(c.display) != "" {
		return c.display
	}
	return c.id
}

func (c *canonicalContractConnector) Capabilities() connectorsdk.Capabilities { return c.caps }

func (c *canonicalContractConnector) Discover(context.Context) ([]connectorsdk.Asset, error) {
	out := make([]connectorsdk.Asset, len(c.assets))
	copy(out, c.assets)
	return out, nil
}

func (c *canonicalContractConnector) TestConnection(context.Context) (connectorsdk.Health, error) {
	return connectorsdk.Health{Status: "ok", Message: "ok"}, nil
}

func (c *canonicalContractConnector) Actions() []connectorsdk.ActionDescriptor {
	out := make([]connectorsdk.ActionDescriptor, len(c.actions))
	copy(out, c.actions)
	return out
}

func (c *canonicalContractConnector) ExecuteAction(context.Context, string, connectorsdk.ActionRequest) (connectorsdk.ActionResult, error) {
	return connectorsdk.ActionResult{Status: "ok", Message: "ok"}, nil
}

type canonicalContractSnapshot struct {
	ProviderInstance canonicalContractProviderInstanceFixture  `json:"provider_instance"`
	Assets           []canonicalContractAssetFixture           `json:"assets,omitempty"`
	ExternalRefs     []canonicalContractExternalRefFixture     `json:"external_refs,omitempty"`
	Relationships    []canonicalContractRelationshipFixture    `json:"relationships,omitempty"`
	CapabilitySets   []canonicalContractCapabilitySetFixture   `json:"capability_sets,omitempty"`
	TemplateBindings []canonicalContractTemplateBindingFixture `json:"template_bindings,omitempty"`
	Checkpoint       canonicalContractCheckpointFixture        `json:"checkpoint"`
	Reconciliation   canonicalContractReconciliationFixture    `json:"reconciliation"`
}

type canonicalContractProviderInstanceFixture struct {
	ID          string               `json:"id"`
	Kind        model.ProviderKind   `json:"kind"`
	Provider    string               `json:"provider"`
	DisplayName string               `json:"display_name"`
	Status      model.ProviderStatus `json:"status"`
	Scope       model.ProviderScope  `json:"scope"`
	ConfigRef   string               `json:"config_ref,omitempty"`
	Metadata    map[string]any       `json:"metadata,omitempty"`
}

type canonicalContractAssetFixture struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Name          string            `json:"name"`
	Source        string            `json:"source"`
	ResourceClass string            `json:"resource_class"`
	ResourceKind  string            `json:"resource_kind"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Attributes    map[string]any    `json:"attributes,omitempty"`
}

type canonicalContractExternalRefFixture struct {
	ResourceID         string `json:"resource_id"`
	ProviderInstanceID string `json:"provider_instance_id"`
	ExternalID         string `json:"external_id"`
	ExternalType       string `json:"external_type,omitempty"`
	ExternalParentID   string `json:"external_parent_id,omitempty"`
	RawLocator         string `json:"raw_locator,omitempty"`
}

type canonicalContractRelationshipFixture struct {
	SourceResourceID string                        `json:"source_resource_id"`
	TargetResourceID string                        `json:"target_resource_id"`
	Type             model.RelationshipType        `json:"type"`
	Direction        model.RelationshipDirection   `json:"direction"`
	Criticality      model.RelationshipCriticality `json:"criticality"`
	Inferred         bool                          `json:"inferred"`
	Confidence       int                           `json:"confidence"`
	Evidence         map[string]any                `json:"evidence,omitempty"`
}

type canonicalContractCapabilityFixture struct {
	ID             string                    `json:"id"`
	Scope          model.CapabilityScope     `json:"scope"`
	Stability      model.CapabilityStability `json:"stability,omitempty"`
	SupportsDryRun bool                      `json:"supports_dry_run,omitempty"`
	SupportsAsync  bool                      `json:"supports_async,omitempty"`
	RequiresTarget bool                      `json:"requires_target,omitempty"`
}

type canonicalContractCapabilitySetFixture struct {
	SubjectType  string                               `json:"subject_type"`
	SubjectID    string                               `json:"subject_id"`
	Capabilities []canonicalContractCapabilityFixture `json:"capabilities,omitempty"`
}

type canonicalContractTemplateBindingFixture struct {
	ResourceID string   `json:"resource_id"`
	TemplateID string   `json:"template_id"`
	Tabs       []string `json:"tabs,omitempty"`
	Operations []string `json:"operations,omitempty"`
}

type canonicalContractCheckpointFixture struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	Stream             string `json:"stream"`
	Cursor             string `json:"cursor"`
}

type canonicalContractReconciliationFixture struct {
	CreatedCount int `json:"created_count"`
	UpdatedCount int `json:"updated_count"`
	StaleCount   int `json:"stale_count"`
	ErrorCount   int `json:"error_count"`
}

type canonicalHeartbeatSnapshot struct {
	ProviderInstance canonicalContractProviderInstanceFixture `json:"provider_instance"`
	Asset            canonicalContractAssetFixture            `json:"asset"`
	ExternalRefs     []canonicalContractExternalRefFixture    `json:"external_refs,omitempty"`
	CapabilitySet    canonicalContractCapabilitySetFixture    `json:"capability_set"`
	TemplateBinding  canonicalContractTemplateBindingFixture  `json:"template_binding"`
	Checkpoint       canonicalContractCheckpointFixture       `json:"checkpoint"`
}

type canonicalStatusAggregateSnapshot struct {
	Registry         canonicalStatusRegistryFixture             `json:"registry"`
	Providers        []canonicalContractProviderInstanceFixture `json:"providers,omitempty"`
	CapabilitySets   []canonicalContractCapabilitySetFixture    `json:"capability_sets,omitempty"`
	TemplateBindings []canonicalContractTemplateBindingFixture  `json:"template_bindings,omitempty"`
	Reconciliation   []canonicalStatusReconciliationFixture     `json:"reconciliation,omitempty"`
}

type canonicalStatusRegistryFixture struct {
	CapabilityIDs []string `json:"capability_ids,omitempty"`
	OperationIDs  []string `json:"operation_ids,omitempty"`
	MetricIDs     []string `json:"metric_ids,omitempty"`
	EventIDs      []string `json:"event_ids,omitempty"`
	TemplateIDs   []string `json:"template_ids,omitempty"`
}

type canonicalStatusReconciliationFixture struct {
	ProviderInstanceID string `json:"provider_instance_id"`
	CreatedCount       int    `json:"created_count"`
	UpdatedCount       int    `json:"updated_count"`
	StaleCount         int    `json:"stale_count"`
	ErrorCount         int    `json:"error_count"`
}

func seedCanonicalContractAsset(t *testing.T, sut *apiServer, asset connectorsdk.Asset) {
	t.Helper()

	_, metadata := withCanonicalResourceMetadata(asset.Source, asset.Type, asset.Metadata)
	if _, err := sut.assetStore.UpsertAssetHeartbeat(assets.HeartbeatRequest{
		AssetID:  asset.ID,
		Type:     asset.Type,
		Name:     asset.Name,
		Source:   asset.Source,
		Status:   "online",
		Metadata: metadata,
	}); err != nil {
		t.Fatalf("seed asset %s: %v", asset.ID, err)
	}
}

func assertCanonicalContractSnapshotFixture(t *testing.T, providerID string, actual canonicalContractSnapshot) {
	t.Helper()
	assertCanonicalContractSnapshotFixtureInDir(t, "canonical_connector_contract", providerID, actual)
}

func assertCanonicalDiscoverContractSnapshotFixture(t *testing.T, caseID string, actual canonicalContractSnapshot) {
	t.Helper()
	assertCanonicalContractSnapshotFixtureInDir(t, "canonical_discover_contract", caseID, actual)
}

func assertCanonicalContractSnapshotFixtureInDir(t *testing.T, fixtureDir, caseID string, actual canonicalContractSnapshot) {
	t.Helper()
	assertCanonicalFixture(t, fixtureDir, caseID, actual)
}

func assertCanonicalHeartbeatSnapshotFixture(t *testing.T, platform string, actual canonicalHeartbeatSnapshot) {
	t.Helper()
	assertCanonicalFixture(t, "canonical_heartbeat_contract", platform, actual)
}

func assertCanonicalStatusAggregateSnapshotFixture(t *testing.T, caseID string, actual canonicalStatusAggregateSnapshot) {
	t.Helper()
	assertCanonicalFixture(t, "canonical_status_aggregate_contract", caseID, actual)
}

func assertCanonicalFixture[T any](t *testing.T, fixtureDir, caseID string, actual T) {
	t.Helper()

	fixturePath := filepath.Join("testdata", strings.TrimSpace(fixtureDir), strings.TrimSpace(caseID)+".json")
	actualJSON, err := json.MarshalIndent(actual, "", "  ")
	if err != nil {
		t.Fatalf("marshal canonical snapshot for %s/%s: %v", fixtureDir, caseID, err)
	}
	actualJSON = append(actualJSON, '\n')

	if *updateCanonicalContractFixtures {
		if err := os.MkdirAll(filepath.Dir(fixturePath), 0o755); err != nil {
			t.Fatalf("create fixture directory for %s/%s: %v", fixtureDir, caseID, err)
		}
		if err := os.WriteFile(fixturePath, actualJSON, 0o644); err != nil {
			t.Fatalf("write fixture for %s/%s: %v", fixtureDir, caseID, err)
		}
	}

	expectedJSON, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture for %s/%s: %v", fixtureDir, caseID, err)
	}

	var expected T
	if err := json.Unmarshal(expectedJSON, &expected); err != nil {
		t.Fatalf("decode fixture for %s/%s: %v", fixtureDir, caseID, err)
	}

	if !reflect.DeepEqual(expected, actual) {
		expectedPretty, marshalErr := json.MarshalIndent(expected, "", "  ")
		if marshalErr != nil {
			t.Fatalf("marshal expected fixture for %s/%s: %v", fixtureDir, caseID, marshalErr)
		}
		t.Fatalf(
			"canonical fixture mismatch for %s/%s\nexpected:\n%s\nactual:\n%s",
			fixtureDir,
			caseID,
			string(expectedPretty),
			string(actualJSON),
		)
	}
}

func canonicalContractNormalizeCursor(cursor string) string {
	cursor = strings.TrimSpace(cursor)
	if cursor == "" {
		return ""
	}
	if separator := strings.LastIndex(cursor, "@"); separator > 0 {
		prefix := strings.TrimSpace(cursor[:separator])
		if prefix == "" {
			return "@*"
		}
		return prefix + "@*"
	}
	segments := strings.Split(cursor, ";")
	for index, segment := range segments {
		trimmed := strings.TrimSpace(segment)
		if strings.HasPrefix(trimmed, "ts=") {
			segments[index] = "ts=*"
			continue
		}
		segments[index] = trimmed
	}
	return strings.Join(segments, ";")
}

func canonicalContractCloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func canonicalContractCloneAnyMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func containsTab(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
