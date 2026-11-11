package persistence

// reversedDiscoveryRunsOn matches runs_on links that link discovery stored
// from the host to the child: until 2.0 a device's agent_id or endpoint_id
// hint made a link from the host it names, the reverse of every other runs_on
// link. The hint names the host, so a link whose source is the named host is
// reversed, and one that was already turned no longer matches.
const reversedDiscoveryRunsOn = `e.relationship_type = 'runs_on'
	AND EXISTS (
		SELECT 1
		FROM jsonb_array_elements_text(
			CASE WHEN jsonb_typeof(e.match_signals->'signals') = 'array'
				THEN e.match_signals->'signals' ELSE '[]'::jsonb END) AS hint(value)
		LEFT JOIN assets host ON host.id = e.source_asset_id
		WHERE lower(hint.value) IN (
			'parent_hint:agent_id=' || lower(e.source_asset_id),
			'parent_hint:endpoint_id=' || lower(e.source_asset_id),
			'parent_hint:agent_id=' || lower(COALESCE(host.name, '')),
			'parent_hint:endpoint_id=' || lower(COALESCE(host.name, ''))))`

// schemaMigrationDiscoveryRunsOnChildToHost turns those links around, so a
// Docker container runs on its host instead of the host running on each of
// its containers (the device breadcrumb listed the containers as the host's
// parents). A link that already exists the right way round makes its reversed
// twin redundant, so the twin is removed instead of turned.
func schemaMigrationDiscoveryRunsOnChildToHost() schemaMigration {
	return schemaMigration{Version: 110, Name: "discovery_runs_on_child_to_host", Statements: []string{
		`DELETE FROM asset_edges e WHERE ` + reversedDiscoveryRunsOn + `
	AND EXISTS (
		SELECT 1 FROM asset_edges other
		WHERE other.source_asset_id = e.target_asset_id
		  AND other.target_asset_id = e.source_asset_id
		  AND other.relationship_type = 'runs_on')`,
		`UPDATE asset_edges e
	SET source_asset_id = e.target_asset_id, target_asset_id = e.source_asset_id, updated_at = NOW()
	WHERE ` + reversedDiscoveryRunsOn,
	}}
}
