package topology

import (
	"strings"
	"testing"
)

// helpers

func assetByID(assets []AssetInfo, id string) (AssetInfo, bool) {
	for _, a := range assets {
		if a.ID == id {
			return a, true
		}
	}
	return AssetInfo{}, false
}

func zoneByLabel(zones []Zone, label string) (Zone, bool) {
	for _, z := range zones {
		if z.Label == label {
			return z, true
		}
	}
	return Zone{}, false
}

func membersInZone(members []ZoneMember, zoneID string) []ZoneMember {
	var result []ZoneMember
	for _, m := range members {
		if m.ZoneID == zoneID {
			result = append(result, m)
		}
	}
	return result
}

// Tests

func TestSeed_EmptyInput(t *testing.T) {
	result := Seed(SeedInput{TopologyID: "topo-1"})
	if len(result.Zones) != 0 {
		t.Errorf("expected 0 zones, got %d", len(result.Zones))
	}
	if len(result.Members) != 0 {
		t.Errorf("expected 0 members, got %d", len(result.Members))
	}
	if len(result.Connections) != 0 {
		t.Errorf("expected 0 connections, got %d", len(result.Connections))
	}
}

func TestSeed_AssetsWithGroups(t *testing.T) {
	input := SeedInput{
		TopologyID: "topo-1",
		Assets: []AssetInfo{
			{ID: "a1", Label: "Server A"},
			{ID: "a2", Label: "Server B"},
			{ID: "a3", Label: "Server C"},
		},
		Groups: map[string]string{
			"grp-prod": "Production",
			"grp-dev":  "Development",
		},
		AssetGroups: map[string]string{
			"a1": "grp-prod",
			"a2": "grp-prod",
			"a3": "grp-dev",
		},
	}

	result := Seed(input)

	if len(result.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(result.Zones))
	}

	prod, ok := zoneByLabel(result.Zones, "Production")
	if !ok {
		t.Fatal("expected zone labeled 'Production'")
	}
	dev, ok := zoneByLabel(result.Zones, "Development")
	if !ok {
		t.Fatal("expected zone labeled 'Development'")
	}

	// Verify all assets land in correct zones
	prodMembers := membersInZone(result.Members, prod.ID)
	if len(prodMembers) != 2 {
		t.Errorf("expected 2 members in Production zone, got %d", len(prodMembers))
	}
	devMembers := membersInZone(result.Members, dev.ID)
	if len(devMembers) != 1 {
		t.Errorf("expected 1 member in Development zone, got %d", len(devMembers))
	}

	// Zone IDs must reference the correct topology
	for _, z := range result.Zones {
		if z.TopologyID != "topo-1" {
			t.Errorf("zone %s has wrong topology ID %q", z.ID, z.TopologyID)
		}
		if !strings.HasPrefix(z.ID, "seed-zone-") {
			t.Errorf("zone ID %q does not start with 'seed-zone-'", z.ID)
		}
	}
}

func TestSeed_AssetsWithSource(t *testing.T) {
	input := SeedInput{
		TopologyID: "topo-2",
		Assets: []AssetInfo{
			{ID: "a1", Label: "pve-node-1", Source: "Proxmox"},
			{ID: "a2", Label: "pve-node-2", Source: "Proxmox"},
			{ID: "a3", Label: "truenas-1", Source: "TrueNAS"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	if len(result.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d", len(result.Zones))
	}

	pve, ok := zoneByLabel(result.Zones, "Proxmox")
	if !ok {
		t.Fatal("expected zone labeled 'Proxmox'")
	}
	nas, ok := zoneByLabel(result.Zones, "TrueNAS")
	if !ok {
		t.Fatal("expected zone labeled 'TrueNAS'")
	}

	if len(membersInZone(result.Members, pve.ID)) != 2 {
		t.Errorf("expected 2 Proxmox members")
	}
	if len(membersInZone(result.Members, nas.ID)) != 1 {
		t.Errorf("expected 1 TrueNAS member")
	}
}

func TestSeed_Mixed(t *testing.T) {
	// a1 → group, a2 → source only, a3 → neither (unsorted)
	input := SeedInput{
		TopologyID: "topo-3",
		Assets: []AssetInfo{
			{ID: "a1", Label: "Server A", Source: "Proxmox"},
			{ID: "a2", Label: "Container B", Source: "Docker"},
			{ID: "a3", Label: "Orphan"},
		},
		Groups: map[string]string{
			"grp-infra": "Infrastructure",
		},
		AssetGroups: map[string]string{
			"a1": "grp-infra",
			// a2: source only (no group)
			// a3: neither
		},
	}

	result := Seed(input)

	// Expect 2 zones: "Infrastructure" (group) and "Docker" (source)
	if len(result.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d: %v", len(result.Zones), result.Zones)
	}

	_, hasInfra := zoneByLabel(result.Zones, "Infrastructure")
	if !hasInfra {
		t.Error("expected zone 'Infrastructure'")
	}
	_, hasDocker := zoneByLabel(result.Zones, "Docker")
	if !hasDocker {
		t.Error("expected zone 'Docker'")
	}

	// Total members: a1 in Infrastructure, a2 in Docker; a3 is unsorted
	if len(result.Members) != 2 {
		t.Errorf("expected 2 members total, got %d", len(result.Members))
	}

	// Verify a3 is not placed in any zone
	for _, m := range result.Members {
		if m.AssetID == "a3" {
			t.Errorf("a3 (unsorted) should not be assigned to a zone")
		}
	}
}

func TestSeed_EdgeImport(t *testing.T) {
	input := SeedInput{
		TopologyID: "topo-4",
		Assets: []AssetInfo{
			{ID: "a1", Label: "A", Source: "Docker"},
			{ID: "a2", Label: "B", Source: "Docker"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
		Edges: []EdgeInfo{
			{SourceAssetID: "a1", TargetAssetID: "a2", Relationship: "depends_on"},
			{SourceAssetID: "a2", TargetAssetID: "a1", Relationship: "provides_to"},
		},
	}

	result := Seed(input)

	if len(result.Connections) != 2 {
		t.Fatalf("expected 2 connections, got %d", len(result.Connections))
	}
	for _, c := range result.Connections {
		if c.UserDefined {
			t.Errorf("connection %s should have user_defined=false", c.ID)
		}
		if c.TopologyID != "topo-4" {
			t.Errorf("connection %s has wrong topology ID", c.ID)
		}
		if !strings.HasPrefix(c.ID, "seed-conn-") {
			t.Errorf("connection ID %q should start with 'seed-conn-'", c.ID)
		}
	}
}

func TestSeed_ContainsEdgesSkipped(t *testing.T) {
	input := SeedInput{
		TopologyID: "topo-5",
		Assets: []AssetInfo{
			{ID: "a1", Source: "Proxmox"},
			{ID: "a2", Source: "Proxmox"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
		Edges: []EdgeInfo{
			{SourceAssetID: "a1", TargetAssetID: "a2", Relationship: "contains"},
			{SourceAssetID: "a1", TargetAssetID: "a2", Relationship: "runs_on"},
		},
	}

	result := Seed(input)

	if len(result.Connections) != 1 {
		t.Fatalf("expected 1 connection (contains skipped), got %d", len(result.Connections))
	}
	if result.Connections[0].Relationship != "runs_on" {
		t.Errorf("expected relationship 'runs_on', got %q", result.Connections[0].Relationship)
	}
}

func TestSeed_UnknownRelationshipSkipped(t *testing.T) {
	input := SeedInput{
		TopologyID:  "topo-6",
		Assets:      []AssetInfo{{ID: "a1"}, {ID: "a2"}},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
		Edges: []EdgeInfo{
			{SourceAssetID: "a1", TargetAssetID: "a2", Relationship: "mystery_type"},
			{SourceAssetID: "a1", TargetAssetID: "a2", Relationship: "connected_to"},
		},
	}

	result := Seed(input)

	if len(result.Connections) != 1 {
		t.Fatalf("expected 1 connection (unknown type skipped), got %d", len(result.Connections))
	}
}

func TestSeed_ZoneGridPositions(t *testing.T) {
	// Create 4 assets across 4 different sources to get 4 zones.
	// Verify zones are NOT all at the same position.
	input := SeedInput{
		TopologyID: "topo-7",
		Assets: []AssetInfo{
			{ID: "a1", Source: "Proxmox"},
			{ID: "a2", Source: "Docker"},
			{ID: "a3", Source: "TrueNAS"},
			{ID: "a4", Source: "HomeAssistant"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	if len(result.Zones) != 4 {
		t.Fatalf("expected 4 zones, got %d", len(result.Zones))
	}

	// Collect all positions
	positions := map[[2]float64]int{}
	for _, z := range result.Zones {
		key := [2]float64{z.Position.X, z.Position.Y}
		positions[key]++
	}

	// All zones should be at distinct positions
	for pos, count := range positions {
		if count > 1 {
			t.Errorf("multiple zones share position (%v, %v)", pos[0], pos[1])
		}
	}

	// The 4th zone (index 3) should be on the second row (since 3 per row)
	// Find zones by sort order
	var row1, row2 []Zone
	for _, z := range result.Zones {
		if z.SortOrder < 3 {
			row1 = append(row1, z)
		} else {
			row2 = append(row2, z)
		}
	}
	if len(row2) != 1 {
		t.Errorf("expected 1 zone on row 2, got %d", len(row2))
	}
	if len(row1) != 3 {
		t.Errorf("expected 3 zones on row 1, got %d", len(row1))
	}

	// Row-2 zone should have a greater Y than all row-1 zones
	for _, z1 := range row1 {
		if row2[0].Position.Y <= z1.Position.Y {
			t.Errorf("row-2 zone Y (%v) should be greater than row-1 zone Y (%v)",
				row2[0].Position.Y, z1.Position.Y)
		}
	}
}

func TestSeed_MemberPositions(t *testing.T) {
	// Members within a zone should not all be at (0,0).
	input := SeedInput{
		TopologyID: "topo-8",
		Assets: []AssetInfo{
			{ID: "a1", Source: "Proxmox"},
			{ID: "a2", Source: "Proxmox"},
			{ID: "a3", Source: "Proxmox"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	if len(result.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result.Zones))
	}

	members := membersInZone(result.Members, result.Zones[0].ID)
	if len(members) != 3 {
		t.Fatalf("expected 3 members, got %d", len(members))
	}

	// No two members should share the same position
	positions := map[[2]float64]int{}
	for _, m := range members {
		key := [2]float64{m.Position.X, m.Position.Y}
		positions[key]++
	}
	for pos, count := range positions {
		if count > 1 {
			t.Errorf("multiple members share position (%v, %v)", pos[0], pos[1])
		}
	}

	// No member should be at (0, 0)
	for _, m := range members {
		if m.Position.X == 0 && m.Position.Y == 0 {
			t.Errorf("member %s is at origin (0,0); expected non-zero layout position", m.AssetID)
		}
	}
}

func TestSeed_ZoneIDsAreUnique(t *testing.T) {
	input := SeedInput{
		TopologyID: "topo-9",
		Assets: []AssetInfo{
			{ID: "a1", Source: "Proxmox"},
			{ID: "a2", Source: "Docker"},
			{ID: "a3", Source: "TrueNAS"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	seen := map[string]bool{}
	for _, z := range result.Zones {
		if seen[z.ID] {
			t.Errorf("duplicate zone ID: %s", z.ID)
		}
		seen[z.ID] = true
	}
}

func TestSeed_HostBasedZones(t *testing.T) {
	// Host-type assets should each get their own zone named after the asset label.
	input := SeedInput{
		TopologyID: "topo-host-1",
		Assets: []AssetInfo{
			{ID: "h1", Label: "pve-node-1", Type: "hypervisor", Source: "Proxmox"},
			{ID: "h2", Label: "nas-box", Type: "nas", Source: "TrueNAS"},
			{ID: "vm1", Label: "web-vm", Type: "vm", Source: "Proxmox"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	// Expect 3 zones: "pve-node-1", "nas-box" (host zones), and "Proxmox" (source zone for vm1)
	if len(result.Zones) != 3 {
		t.Fatalf("expected 3 zones, got %d: %+v", len(result.Zones), result.Zones)
	}

	h1Zone, ok := zoneByLabel(result.Zones, "pve-node-1")
	if !ok {
		t.Fatal("expected zone labeled 'pve-node-1' for hypervisor host")
	}
	h2Zone, ok := zoneByLabel(result.Zones, "nas-box")
	if !ok {
		t.Fatal("expected zone labeled 'nas-box' for NAS host")
	}
	_, ok = zoneByLabel(result.Zones, "Proxmox")
	if !ok {
		t.Fatal("expected fallback source zone 'Proxmox' for vm1")
	}

	// Each host zone should contain its host asset
	if len(membersInZone(result.Members, h1Zone.ID)) != 1 {
		t.Errorf("expected 1 member in pve-node-1 zone")
	}
	if len(membersInZone(result.Members, h2Zone.ID)) != 1 {
		t.Errorf("expected 1 member in nas-box zone")
	}
}

func TestSeed_HostBasedZonesChildExclusion(t *testing.T) {
	// Children linked via "contains" edges should land in the parent host's zone,
	// not in a separate source-based zone.
	input := SeedInput{
		TopologyID: "topo-host-2",
		Assets: []AssetInfo{
			{ID: "h1", Label: "pve-node-1", Type: "hypervisor", Source: "Proxmox"},
			{ID: "vm1", Label: "web-vm", Type: "vm", Source: "Proxmox"},
			{ID: "vm2", Label: "db-vm", Type: "vm", Source: "Proxmox"},
			{ID: "ct1", Label: "container-1", Type: "container", Source: "Docker"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
		Edges: []EdgeInfo{
			{SourceAssetID: "h1", TargetAssetID: "vm1", Relationship: "contains"},
			{SourceAssetID: "h1", TargetAssetID: "vm2", Relationship: "contains"},
			// ct1 has no contains edge to h1
		},
	}

	result := Seed(input)

	// Expect 2 zones: "pve-node-1" (host + 2 child VMs), "Docker" (ct1)
	if len(result.Zones) != 2 {
		t.Fatalf("expected 2 zones, got %d: %+v", len(result.Zones), result.Zones)
	}

	hostZone, ok := zoneByLabel(result.Zones, "pve-node-1")
	if !ok {
		t.Fatal("expected zone labeled 'pve-node-1'")
	}

	hostMembers := membersInZone(result.Members, hostZone.ID)
	if len(hostMembers) != 3 {
		t.Errorf("expected 3 members in host zone (h1 + vm1 + vm2), got %d", len(hostMembers))
	}

	dockerZone, ok := zoneByLabel(result.Zones, "Docker")
	if !ok {
		t.Fatal("expected zone labeled 'Docker'")
	}
	dockerMembers := membersInZone(result.Members, dockerZone.ID)
	if len(dockerMembers) != 1 {
		t.Errorf("expected 1 member in Docker zone, got %d", len(dockerMembers))
	}
}

func TestSeed_ColorPalette(t *testing.T) {
	// Zones should get colors from the palette, not a hardcoded value.
	input := SeedInput{
		TopologyID: "topo-colors",
		Assets: []AssetInfo{
			{ID: "a1", Source: "Proxmox"},
			{ID: "a2", Source: "Docker"},
			{ID: "a3", Source: "TrueNAS"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	if len(result.Zones) != 3 {
		t.Fatalf("expected 3 zones, got %d", len(result.Zones))
	}

	for _, z := range result.Zones {
		if z.Color == "#4f6bed" {
			t.Errorf("zone %q still uses hardcoded color #4f6bed", z.Label)
		}
		found := false
		for _, c := range zoneColorPalette {
			if z.Color == c {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("zone %q has color %q which is not in the palette", z.Label, z.Color)
		}
	}

	// Each zone should get a different color (3 zones, 6-color palette)
	colors := map[string]bool{}
	for _, z := range result.Zones {
		colors[z.Color] = true
	}
	if len(colors) != 3 {
		t.Errorf("expected 3 distinct colors, got %d", len(colors))
	}
}

func TestSeed_ZoneAutoSize(t *testing.T) {
	// Zones with multiple members should be auto-sized rather than using a fixed size.
	input := SeedInput{
		TopologyID: "topo-autosize",
		Assets: []AssetInfo{
			{ID: "a1", Source: "Docker"},
			{ID: "a2", Source: "Docker"},
			{ID: "a3", Source: "Docker"},
		},
		Groups:      map[string]string{},
		AssetGroups: map[string]string{},
	}

	result := Seed(input)

	if len(result.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result.Zones))
	}

	z := result.Zones[0]
	expected := zoneAutoSize(3)
	if z.Size.Width != expected.Width || z.Size.Height != expected.Height {
		t.Errorf("zone size = %vx%v, expected %vx%v", z.Size.Width, z.Size.Height, expected.Width, expected.Height)
	}
}

func TestSeed_GroupTakesPriorityOverHostType(t *testing.T) {
	// An asset with both a group and a host type should go into the group zone,
	// not get its own host zone.
	input := SeedInput{
		TopologyID: "topo-priority",
		Assets: []AssetInfo{
			{ID: "h1", Label: "pve-node-1", Type: "hypervisor", Source: "Proxmox"},
		},
		Groups:      map[string]string{"grp-infra": "Infrastructure"},
		AssetGroups: map[string]string{"h1": "grp-infra"},
	}

	result := Seed(input)

	if len(result.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result.Zones))
	}
	if result.Zones[0].Label != "Infrastructure" {
		t.Errorf("expected group zone 'Infrastructure', got %q", result.Zones[0].Label)
	}
}

func TestSeed_GroupLabelFallsBackToGroupID(t *testing.T) {
	// Group exists in AssetGroups but not in Groups map → use groupID as label.
	input := SeedInput{
		TopologyID: "topo-10",
		Assets: []AssetInfo{
			{ID: "a1", Label: "Server A"},
		},
		Groups: map[string]string{},
		AssetGroups: map[string]string{
			"a1": "grp-unknown",
		},
	}

	result := Seed(input)

	if len(result.Zones) != 1 {
		t.Fatalf("expected 1 zone, got %d", len(result.Zones))
	}
	if result.Zones[0].Label != "grp-unknown" {
		t.Errorf("expected label 'grp-unknown', got %q", result.Zones[0].Label)
	}
}
