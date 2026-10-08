package persistence

// Schema changes owned by the collectors domain.

func schemaMigrationHubCollectors() schemaMigration {
	return schemaMigration{
		Version: 17,
		Name:    "hub_collectors",
		Statements: []string{
			`CREATE TABLE IF NOT EXISTS hub_collectors (
				id TEXT PRIMARY KEY,
				asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
				collector_type TEXT NOT NULL CHECK (collector_type IN ('ssh','winrm','api')),
				config JSONB NOT NULL DEFAULT '{}',
				enabled BOOLEAN NOT NULL DEFAULT true,
				interval_seconds INT NOT NULL DEFAULT 60,
				last_collected_at TIMESTAMPTZ,
				last_status TEXT DEFAULT '',
				last_error TEXT DEFAULT '',
				created_at TIMESTAMPTZ DEFAULT now(),
				updated_at TIMESTAMPTZ DEFAULT now(),
				UNIQUE(asset_id)
			)`,
		},
	}
}

func schemaMigrationHubCollectorsAddProxmox() schemaMigration {
	return schemaMigration{
		Version: 22,
		Name:    "hub_collectors_add_proxmox",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox'))`,
		},
	}
}

// Version 23 fills the development gap between 22 and 24; preserve this no-op.
func schemaMigrationNoopSequencePlaceholder() schemaMigration {
	return schemaMigration{
		Version:    23,
		Name:       "noop_sequence_placeholder",
		Statements: []string{},
	}
}

func schemaMigrationHubCollectorsAddTruenas() schemaMigration {
	return schemaMigration{
		Version: 24,
		Name:    "hub_collectors_add_truenas",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','truenas'))`,
		},
	}
}

func schemaMigrationHubCollectorsAddPortainerDocker() schemaMigration {
	return schemaMigration{
		Version: 25,
		Name:    "hub_collectors_add_portainer_docker",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','truenas','portainer','docker'))`,
		},
	}
}

func schemaMigrationHubCollectorsAddPbs() schemaMigration {
	return schemaMigration{
		Version: 26,
		Name:    "hub_collectors_add_pbs",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','pbs','truenas','portainer','docker'))`,
		},
	}
}

func schemaMigrationHubCollectorsAddHomeassistant() schemaMigration {
	return schemaMigration{
		Version: 27,
		Name:    "hub_collectors_add_homeassistant",
		Statements: []string{
			`ALTER TABLE hub_collectors DROP CONSTRAINT IF EXISTS hub_collectors_collector_type_check`,
			`ALTER TABLE hub_collectors
			 ADD CONSTRAINT hub_collectors_collector_type_check
			 CHECK (collector_type IN ('ssh','winrm','api','proxmox','pbs','truenas','portainer','docker','homeassistant'))`,
		},
	}
}

// Versions 97 and 98 are reserved by independent identity/enrollment fixes.
// Keep this cleanup at 99 to preserve schema version ownership.
func schemaMigrationScrubCollectorUrlUserinfo() schemaMigration {
	return schemaMigration{
		Version: 99,
		Name:    "scrub_collector_url_userinfo",
		Statements: []string{
			`CREATE FUNCTION labtether_migration_99_scrub_url_userinfo(input JSONB)
			 RETURNS JSONB
			 LANGUAGE plpgsql
			 IMMUTABLE
			 STRICT
			 AS $function$
			 DECLARE
			   scrubbed JSONB;
			   raw_value TEXT;
			 BEGIN
			   CASE jsonb_typeof(input)
			   WHEN 'object' THEN
			     SELECT COALESCE(jsonb_object_agg(entry_key, labtether_migration_99_scrub_url_userinfo(entry_value)), '{}'::jsonb)
			       INTO scrubbed
			       FROM jsonb_each(input) AS entry(entry_key, entry_value);
			     RETURN scrubbed;
			   WHEN 'array' THEN
			     SELECT COALESCE(jsonb_agg(labtether_migration_99_scrub_url_userinfo(entry_value) ORDER BY entry_index), '[]'::jsonb)
			       INTO scrubbed
			       FROM jsonb_array_elements(input) WITH ORDINALITY AS entry(entry_value, entry_index);
			     RETURN scrubbed;
			   WHEN 'string' THEN
			     raw_value := input #>> '{}';
			     IF raw_value ~ '^([[:space:]]*([[:alpha:]][[:alnum:]+.-]*:)?//)[^/?#]*@' THEN
			       RETURN to_jsonb(regexp_replace(
			         raw_value,
			         '^([[:space:]]*([[:alpha:]][[:alnum:]+.-]*:)?//)[^/?#]*@',
			         E'\\1'
			       ));
			     END IF;
			   END CASE;
			   RETURN input;
			 END
			 $function$`,
			`UPDATE hub_collectors
			 SET config = labtether_migration_99_scrub_url_userinfo(config),
			     enabled = FALSE,
			     last_status = 'error',
			     last_error = 'collector disabled because a URL contained embedded credentials; add a credential_id and re-enable it',
			     updated_at = NOW()
			 WHERE labtether_migration_99_scrub_url_userinfo(config) IS DISTINCT FROM config`,
			`UPDATE hub_collectors
			 SET last_error = regexp_replace(
			       last_error,
			       '([[:alpha:]][[:alnum:]+.-]*://)[^/?#[:cntrl:]]*@',
			       E'\\1',
			       'g'
			     ),
			     updated_at = NOW()
			 WHERE last_error ~ '([[:alpha:]][[:alnum:]+.-]*://)[^/?#[:cntrl:]]*@'`,
			`UPDATE credential_profiles
			 SET metadata = labtether_migration_99_scrub_url_userinfo(metadata),
			     updated_at = NOW()
			 WHERE metadata IS NOT NULL
			   AND labtether_migration_99_scrub_url_userinfo(metadata) IS DISTINCT FROM metadata`,
			`UPDATE assets
			 SET metadata = labtether_migration_99_scrub_url_userinfo(metadata),
			     updated_at = NOW()
			 WHERE metadata IS NOT NULL
			   AND labtether_migration_99_scrub_url_userinfo(metadata) IS DISTINCT FROM metadata`,
			`UPDATE asset_heartbeats
			 SET metadata = labtether_migration_99_scrub_url_userinfo(metadata)
			 WHERE metadata IS NOT NULL
			   AND labtether_migration_99_scrub_url_userinfo(metadata) IS DISTINCT FROM metadata`,
			`UPDATE provider_instances
			 SET metadata = labtether_migration_99_scrub_url_userinfo(metadata),
			     updated_at = NOW()
			 WHERE labtether_migration_99_scrub_url_userinfo(metadata) IS DISTINCT FROM metadata`,
			`UPDATE resource_external_refs
			 SET raw_locator = labtether_migration_99_scrub_url_userinfo(to_jsonb(raw_locator)) #>> '{}',
			     updated_at = NOW()
			 WHERE raw_locator IS NOT NULL
			   AND (labtether_migration_99_scrub_url_userinfo(to_jsonb(raw_locator)) #>> '{}') IS DISTINCT FROM raw_locator`,
			`DROP FUNCTION labtether_migration_99_scrub_url_userinfo(JSONB)`,
		},
	}
}
