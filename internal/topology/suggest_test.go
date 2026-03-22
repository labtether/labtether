package topology

import (
	"testing"
)

// helpers

func makeZone(id, label string) Zone {
	return Zone{ID: id, Label: label}
}

func makeMember(zoneID, assetID string) ZoneMember {
	return ZoneMember{ZoneID: zoneID, AssetID: assetID}
}

func makeAsset(id, source, typ string) AssetInfo {
	return AssetInfo{ID: id, Source: source, Type: typ}
}

// ---------------------------------------------------------------------------
// SuggestPlacements tests
// ---------------------------------------------------------------------------

func TestSuggestPlacements_EmptyInputs(t *testing.T) {
	result := SuggestPlacements(nil, nil, nil, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
}

func TestSuggestPlacements_ParentHostMatch(t *testing.T) {
	zones := []Zone{makeZone("z1", "Rack A")}
	parent := makeAsset("host1", "proxmox", "host")
	child := makeAsset("vm1", "proxmox", "vm")

	members := []ZoneMember{makeMember("z1", "host1")}
	memberAssets := map[string]AssetInfo{"host1": parent}
	parentMap := map[string]string{"vm1": "host1"}

	result := SuggestPlacements([]AssetInfo{child}, zones, members, memberAssets, parentMap)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result))
	}
	s := result[0]
	if s.AssetID != "vm1" {
		t.Errorf("expected AssetID vm1, got %s", s.AssetID)
	}
	if s.ZoneID != "z1" {
		t.Errorf("expected ZoneID z1, got %s", s.ZoneID)
	}
	if s.ZoneLabel != "Rack A" {
		t.Errorf("expected ZoneLabel 'Rack A', got %s", s.ZoneLabel)
	}
	if s.Reason != "parent_host" {
		t.Errorf("expected reason parent_host, got %s", s.Reason)
	}
}

func TestSuggestPlacements_SourceMajorityMatch(t *testing.T) {
	zones := []Zone{makeZone("z1", "Docker Zone")}
	// Three members share source "docker"; one has "other".
	memberAssets := map[string]AssetInfo{
		"a1": makeAsset("a1", "docker", "container"),
		"a2": makeAsset("a2", "docker", "container"),
		"a3": makeAsset("a3", "docker", "container"),
		"a4": makeAsset("a4", "other", "host"),
	}
	members := []ZoneMember{
		makeMember("z1", "a1"),
		makeMember("z1", "a2"),
		makeMember("z1", "a3"),
		makeMember("z1", "a4"),
	}
	unsorted := []AssetInfo{makeAsset("new1", "docker", "container")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result))
	}
	s := result[0]
	if s.ZoneID != "z1" {
		t.Errorf("expected ZoneID z1, got %s", s.ZoneID)
	}
	if s.Reason != "same_source" {
		t.Errorf("expected reason same_source, got %s", s.Reason)
	}
}

func TestSuggestPlacements_TypeMajorityMatch(t *testing.T) {
	zones := []Zone{makeZone("z1", "VM Zone")}
	// Majority type is "vm" but sources differ, so source won't match.
	memberAssets := map[string]AssetInfo{
		"a1": makeAsset("a1", "proxmox", "vm"),
		"a2": makeAsset("a2", "vmware", "vm"),
		"a3": makeAsset("a3", "kvm", "vm"),
	}
	members := []ZoneMember{
		makeMember("z1", "a1"),
		makeMember("z1", "a2"),
		makeMember("z1", "a3"),
	}
	// Unsorted asset has a unique source but type "vm".
	unsorted := []AssetInfo{makeAsset("new1", "hyper-v", "vm")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result))
	}
	s := result[0]
	if s.ZoneID != "z1" {
		t.Errorf("expected ZoneID z1, got %s", s.ZoneID)
	}
	if s.Reason != "same_type" {
		t.Errorf("expected reason same_type, got %s", s.Reason)
	}
}

func TestSuggestPlacements_PriorityParentOverSource(t *testing.T) {
	// Two zones: z1 has parent, z2 has source majority.
	// Asset has parent in z1 and source matching z2 → should prefer parent (z1).
	zones := []Zone{makeZone("z1", "Zone 1"), makeZone("z2", "Zone 2")}

	parentAsset := makeAsset("host1", "other", "host")
	memberAssets := map[string]AssetInfo{
		"host1": parentAsset,
		"b1":    makeAsset("b1", "docker", "container"),
		"b2":    makeAsset("b2", "docker", "container"),
		"b3":    makeAsset("b3", "docker", "container"),
	}
	members := []ZoneMember{
		makeMember("z1", "host1"),
		makeMember("z2", "b1"),
		makeMember("z2", "b2"),
		makeMember("z2", "b3"),
	}
	parentMap := map[string]string{"new1": "host1"}
	unsorted := []AssetInfo{makeAsset("new1", "docker", "container")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, parentMap)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result))
	}
	s := result[0]
	if s.ZoneID != "z1" {
		t.Errorf("expected ZoneID z1 (parent wins), got %s", s.ZoneID)
	}
	if s.Reason != "parent_host" {
		t.Errorf("expected reason parent_host, got %s", s.Reason)
	}
}

func TestSuggestPlacements_PrioritySourceOverType(t *testing.T) {
	// z1 has source majority "docker", z2 has type majority "vm".
	// Asset source is "docker" and type is "vm" → source wins → z1.
	zones := []Zone{makeZone("z1", "Docker Zone"), makeZone("z2", "VM Zone")}

	memberAssets := map[string]AssetInfo{
		"a1": makeAsset("a1", "docker", "container"),
		"a2": makeAsset("a2", "docker", "container"),
		"a3": makeAsset("a3", "docker", "container"),
		"b1": makeAsset("b1", "proxmox", "vm"),
		"b2": makeAsset("b2", "vmware", "vm"),
		"b3": makeAsset("b3", "kvm", "vm"),
	}
	members := []ZoneMember{
		makeMember("z1", "a1"),
		makeMember("z1", "a2"),
		makeMember("z1", "a3"),
		makeMember("z2", "b1"),
		makeMember("z2", "b2"),
		makeMember("z2", "b3"),
	}
	unsorted := []AssetInfo{makeAsset("new1", "docker", "vm")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result))
	}
	s := result[0]
	if s.ZoneID != "z1" {
		t.Errorf("expected ZoneID z1 (source wins over type), got %s", s.ZoneID)
	}
	if s.Reason != "same_source" {
		t.Errorf("expected reason same_source, got %s", s.Reason)
	}
}

func TestSuggestPlacements_NoMatch(t *testing.T) {
	zones := []Zone{makeZone("z1", "Zone 1")}
	memberAssets := map[string]AssetInfo{
		"a1": makeAsset("a1", "proxmox", "host"),
		"a2": makeAsset("a2", "docker", "container"),
	}
	members := []ZoneMember{
		makeMember("z1", "a1"),
		makeMember("z1", "a2"),
	}
	// Neither source nor type has a majority; no parent.
	unsorted := []AssetInfo{makeAsset("new1", "homeassistant", "sensor")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, nil)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion entry (no-match), got %d", len(result))
	}
	s := result[0]
	if s.AssetID != "new1" {
		t.Errorf("expected AssetID new1, got %s", s.AssetID)
	}
	if s.ZoneID != "" {
		t.Errorf("expected empty ZoneID for no-match, got %s", s.ZoneID)
	}
	if s.Reason != "" {
		t.Errorf("expected empty Reason for no-match, got %s", s.Reason)
	}
}

func TestSuggestPlacements_NoZones(t *testing.T) {
	unsorted := []AssetInfo{makeAsset("new1", "docker", "container")}
	result := SuggestPlacements(unsorted, nil, nil, nil, nil)
	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion (no-match), got %d", len(result))
	}
	if result[0].ZoneID != "" {
		t.Errorf("expected empty ZoneID with no zones, got %s", result[0].ZoneID)
	}
}

func TestSuggestPlacements_ParentNotInAnyZone(t *testing.T) {
	// Parent exists in parentMap but is not placed in any zone.
	zones := []Zone{makeZone("z1", "Zone 1")}
	memberAssets := map[string]AssetInfo{
		"a1": makeAsset("a1", "docker", "container"),
		"a2": makeAsset("a2", "docker", "container"),
		"a3": makeAsset("a3", "docker", "container"),
	}
	members := []ZoneMember{
		makeMember("z1", "a1"),
		makeMember("z1", "a2"),
		makeMember("z1", "a3"),
	}
	// Parent "unplaced" is not in any zone.
	parentMap := map[string]string{"new1": "unplaced"}
	unsorted := []AssetInfo{makeAsset("new1", "docker", "container")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, parentMap)

	if len(result) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(result))
	}
	s := result[0]
	// Should fall through to source match.
	if s.ZoneID != "z1" {
		t.Errorf("expected ZoneID z1 via source fallback, got %s", s.ZoneID)
	}
	if s.Reason != "same_source" {
		t.Errorf("expected reason same_source, got %s", s.Reason)
	}
}

func TestSuggestPlacements_NoMajority(t *testing.T) {
	// Exactly 50/50 split — no strict majority.
	zones := []Zone{makeZone("z1", "Zone 1")}
	memberAssets := map[string]AssetInfo{
		"a1": makeAsset("a1", "docker", "container"),
		"a2": makeAsset("a2", "proxmox", "host"),
	}
	members := []ZoneMember{
		makeMember("z1", "a1"),
		makeMember("z1", "a2"),
	}
	unsorted := []AssetInfo{makeAsset("new1", "docker", "container")}

	result := SuggestPlacements(unsorted, zones, members, memberAssets, nil)

	// 1 match out of 2 = 50%, not > 50%, so no suggestion.
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[0].ZoneID != "" {
		t.Errorf("expected empty ZoneID (no strict majority), got %s", result[0].ZoneID)
	}
}

// ---------------------------------------------------------------------------
// CheckDismissedForChanges tests
// ---------------------------------------------------------------------------

func TestCheckDismissedForChanges_SourceChanged(t *testing.T) {
	dismissed := []DismissedAssetState{
		{TopologyID: "t1", AssetID: "a1", Source: "old-source", Type: "vm"},
	}
	current := map[string]AssetInfo{
		"a1": makeAsset("a1", "new-source", "vm"),
	}
	result := CheckDismissedForChanges(dismissed, current)
	if len(result) != 1 || result[0] != "a1" {
		t.Errorf("expected [a1], got %v", result)
	}
}

func TestCheckDismissedForChanges_TypeChanged(t *testing.T) {
	dismissed := []DismissedAssetState{
		{TopologyID: "t1", AssetID: "a1", Source: "proxmox", Type: "host"},
	}
	current := map[string]AssetInfo{
		"a1": makeAsset("a1", "proxmox", "vm"),
	}
	result := CheckDismissedForChanges(dismissed, current)
	if len(result) != 1 || result[0] != "a1" {
		t.Errorf("expected [a1], got %v", result)
	}
}

func TestCheckDismissedForChanges_NoChange(t *testing.T) {
	dismissed := []DismissedAssetState{
		{TopologyID: "t1", AssetID: "a1", Source: "docker", Type: "container"},
	}
	current := map[string]AssetInfo{
		"a1": makeAsset("a1", "docker", "container"),
	}
	result := CheckDismissedForChanges(dismissed, current)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %v", result)
	}
}

func TestCheckDismissedForChanges_AssetDeleted(t *testing.T) {
	dismissed := []DismissedAssetState{
		{TopologyID: "t1", AssetID: "a1", Source: "docker", Type: "container"},
	}
	// Asset no longer present in current map.
	current := map[string]AssetInfo{}
	result := CheckDismissedForChanges(dismissed, current)
	if len(result) != 0 {
		t.Errorf("expected empty result for deleted asset, got %v", result)
	}
}

func TestCheckDismissedForChanges_Mixed(t *testing.T) {
	dismissed := []DismissedAssetState{
		{TopologyID: "t1", AssetID: "a1", Source: "docker", Type: "container"}, // unchanged
		{TopologyID: "t1", AssetID: "a2", Source: "old", Type: "host"},         // source changed
		{TopologyID: "t1", AssetID: "a3", Source: "proxmox", Type: "host"},     // deleted
		{TopologyID: "t1", AssetID: "a4", Source: "proxmox", Type: "host"},     // type changed
	}
	current := map[string]AssetInfo{
		"a1": makeAsset("a1", "docker", "container"),
		"a2": makeAsset("a2", "new", "host"),
		// a3 absent
		"a4": makeAsset("a4", "proxmox", "vm"),
	}
	result := CheckDismissedForChanges(dismissed, current)
	if len(result) != 2 {
		t.Fatalf("expected 2 changed assets, got %d: %v", len(result), result)
	}
	found := map[string]bool{}
	for _, id := range result {
		found[id] = true
	}
	if !found["a2"] || !found["a4"] {
		t.Errorf("expected a2 and a4, got %v", result)
	}
}

func TestCheckDismissedForChanges_EmptyInputs(t *testing.T) {
	result := CheckDismissedForChanges(nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty result for nil inputs, got %v", result)
	}
}
