-- Demo seed data for LabTether hub.
-- Run AFTER the hub has started and applied migrations:
--   psql "$DATABASE_URL" -f scripts/demo-seed.sql
--
-- Safe to re-run: TRUNCATEs demo tables first, never touches users/sessions/runtime_settings.

BEGIN;

-- ============================================================
-- 1. TRUNCATE demo-relevant tables (CASCADE handles dependents)
-- ============================================================
TRUNCATE
    asset_edges,          -- asset_dependencies is a view onto this table
    metric_samples,
    alert_instances,
    alert_rules,
    incident_events,
    incident_alert_links,
    incidents,
    log_events,
    audit_events,
    groups,
    action_runs,
    assets
CASCADE;

-- ============================================================
-- 2. GROUPS
-- ============================================================
INSERT INTO groups (id, name, slug, parent_group_id, icon, sort_order, timezone, location, metadata, created_at, updated_at) VALUES
    ('group-prod',    'Production',   'production',  NULL,          'server',  1, 'America/New_York', 'Rack A', '{}'::jsonb, NOW() - INTERVAL '90 days', NOW()),
    ('group-dev',     'Development',  'development', NULL,          'code',    2, 'America/New_York', 'Rack B', '{}'::jsonb, NOW() - INTERVAL '90 days', NOW()),
    ('group-storage', 'Storage',      'storage',     'group-prod',  'database',3, 'America/New_York', 'Rack A', '{}'::jsonb, NOW() - INTERVAL '60 days', NOW())
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 3. ASSETS — 10 realistic homelab nodes
-- ============================================================
INSERT INTO assets (id, type, name, source, status, platform, host, transport_type, group_id, tags, metadata, created_at, updated_at, last_seen_at) VALUES
    ('asset-proxmox-1',      'server',    'proxmox-1',       'agent', 'online',   'linux',   '10.0.1.1',  'agent',   'group-prod',    '["hypervisor","proxmox"]'::jsonb,   '{"os":"pve-8.3","arch":"amd64","agent_version":"1.4.2","cpu_cores":16,"ram_gb":64}'::jsonb,   NOW() - INTERVAL '180 days', NOW(), NOW() - INTERVAL '30 seconds'),
    ('asset-truenas-main',   'nas',       'truenas-main',    'agent', 'degraded', 'freebsd', '10.0.1.2',  'agent',   'group-storage', '["storage","zfs"]'::jsonb,          '{"os":"truenas-13.3","arch":"amd64","agent_version":"1.4.0","pool_size_tb":48}'::jsonb,       NOW() - INTERVAL '180 days', NOW(), NOW() - INTERVAL '5 minutes'),
    ('asset-docker-host-1',  'server',    'docker-host-1',   'agent', 'online',   'linux',   '10.0.1.3',  'agent',   'group-prod',    '["docker","containers"]'::jsonb,    '{"os":"debian-12","arch":"amd64","agent_version":"1.4.2","containers_running":14}'::jsonb,    NOW() - INTERVAL '120 days', NOW(), NOW() - INTERVAL '1 minute'),
    ('asset-pihole',         'appliance', 'pihole',          'agent', 'online',   'linux',   '10.0.1.4',  'agent',   'group-prod',    '["dns","adblock"]'::jsonb,          '{"os":"raspbian-12","arch":"arm64","agent_version":"1.4.2"}'::jsonb,                         NOW() - INTERVAL '200 days', NOW(), NOW() - INTERVAL '45 seconds'),
    ('asset-home-assistant', 'appliance', 'home-assistant',  'agent', 'online',   'linux',   '10.0.1.5',  'agent',   'group-prod',    '["automation","iot"]'::jsonb,       '{"os":"haos-13.2","arch":"amd64","agent_version":"1.4.2","integrations":47}'::jsonb,         NOW() - INTERVAL '150 days', NOW(), NOW() - INTERVAL '1 minute'),
    ('asset-dev-vm',         'vm',        'dev-vm',          'agent', 'offline',  'linux',   '10.0.1.6',  'agent',   'group-dev',     '["development","ci"]'::jsonb,       '{"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.3.9"}'::jsonb,                        NOW() - INTERVAL '30 days',  NOW() - INTERVAL '2 hours', NOW() - INTERVAL '2 hours'),
    ('asset-backup-server',  'server',    'backup-server',   'agent', 'online',   'linux',   '10.0.1.7',  'agent',   'group-storage', '["backup","pbs"]'::jsonb,           '{"os":"debian-12","arch":"amd64","agent_version":"1.4.2"}'::jsonb,                           NOW() - INTERVAL '90 days',  NOW(), NOW() - INTERVAL '3 minutes'),
    ('asset-media-server',   'server',    'media-server',    'agent', 'online',   'linux',   '10.0.1.8',  'agent',   'group-prod',    '["media","plex"]'::jsonb,           '{"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.4.1","streams_active":3}'::jsonb,     NOW() - INTERVAL '60 days',  NOW(), NOW() - INTERVAL '2 minutes'),
    ('asset-k3s-node-1',     'server',    'k3s-node-1',      'agent', 'online',   'linux',   '10.0.1.9',  'agent',   'group-prod',    '["kubernetes","k3s"]'::jsonb,       '{"os":"ubuntu-24.04","arch":"amd64","agent_version":"1.4.2","pods_running":22}'::jsonb,      NOW() - INTERVAL '45 days',  NOW(), NOW() - INTERVAL '40 seconds'),
    ('asset-monitoring',     'server',    'monitoring',       'agent', 'online',   'linux',   '10.0.1.10', 'agent',   'group-prod',    '["observability","grafana"]'::jsonb,'{"os":"debian-12","arch":"amd64","agent_version":"1.4.2"}'::jsonb,                           NOW() - INTERVAL '120 days', NOW(), NOW() - INTERVAL '1 minute')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 4. ASSET DEPENDENCIES (topology via asset_edges / asset_dependencies view)
-- ============================================================
INSERT INTO asset_edges (id, source_asset_id, target_asset_id, relationship_type, direction, criticality, origin, confidence, match_signals, metadata, created_at, updated_at) VALUES
    -- proxmox-1 runs dev-vm
    (gen_random_uuid()::text, 'asset-proxmox-1',     'asset-dev-vm',         'runs_on',      'downstream', 'high',   'manual', 1.0, NULL, '{"note":"VM hosted on Proxmox"}'::jsonb,      NOW() - INTERVAL '30 days',  NOW()),
    -- docker-host-1 runs containers
    (gen_random_uuid()::text, 'asset-docker-host-1', 'asset-pihole',         'runs_on',      'downstream', 'medium', 'manual', 1.0, NULL, '{"note":"Pi-hole container"}'::jsonb,          NOW() - INTERVAL '120 days', NOW()),
    (gen_random_uuid()::text, 'asset-docker-host-1', 'asset-home-assistant', 'runs_on',      'downstream', 'medium', 'manual', 1.0, NULL, '{"note":"Home Assistant container"}'::jsonb,   NOW() - INTERVAL '120 days', NOW()),
    (gen_random_uuid()::text, 'asset-docker-host-1', 'asset-media-server',   'runs_on',      'downstream', 'low',    'manual', 1.0, NULL, '{"note":"Media server container"}'::jsonb,     NOW() - INTERVAL '60 days',  NOW()),
    -- truenas provides storage
    (gen_random_uuid()::text, 'asset-truenas-main',  'asset-proxmox-1',      'provides_to',  'downstream', 'critical','manual', 1.0, NULL, '{"note":"NFS datastore"}'::jsonb,             NOW() - INTERVAL '180 days', NOW()),
    (gen_random_uuid()::text, 'asset-truenas-main',  'asset-docker-host-1',  'provides_to',  'downstream', 'high',   'manual', 1.0, NULL, '{"note":"iSCSI volumes"}'::jsonb,              NOW() - INTERVAL '120 days', NOW()),
    (gen_random_uuid()::text, 'asset-truenas-main',  'asset-backup-server',  'provides_to',  'downstream', 'high',   'manual', 1.0, NULL, '{"note":"Backup target share"}'::jsonb,        NOW() - INTERVAL '90 days',  NOW()),
    -- monitoring depends on k3s
    (gen_random_uuid()::text, 'asset-monitoring',    'asset-k3s-node-1',     'depends_on',   'upstream',   'high',   'manual', 1.0, NULL, '{"note":"Grafana + Prometheus on k3s"}'::jsonb, NOW() - INTERVAL '45 days',  NOW())
ON CONFLICT DO NOTHING;

-- ============================================================
-- 5. ALERT RULES — 5 rules
-- ============================================================
INSERT INTO alert_rules (id, name, description, status, kind, severity, target_scope, cooldown_seconds, reopen_after_seconds, evaluation_interval_seconds, window_seconds, condition, labels, metadata, created_by, created_at, updated_at) VALUES
    ('rule-high-cpu',      'High CPU Usage',     'CPU usage above 80% for 5 minutes',              'active', 'metric_threshold',  'medium', 'global', 300, 60, 30, 300,
     '{"metric":"cpu_percent","operator":">","threshold":80}'::jsonb,
     '{"category":"performance"}'::jsonb, '{}'::jsonb, 'admin', NOW() - INTERVAL '60 days', NOW()),

    ('rule-disk-full',     'Disk Nearly Full',   'Disk usage above 90%',                           'active', 'metric_threshold',  'critical','global', 300, 60, 30, 60,
     '{"metric":"disk_percent","operator":">","threshold":90}'::jsonb,
     '{"category":"capacity"}'::jsonb, '{}'::jsonb, 'admin', NOW() - INTERVAL '60 days', NOW()),

    ('rule-memory-pressure','Memory Pressure',   'Memory usage above 85% for 10 minutes',          'active', 'metric_threshold',  'high',   'global', 600, 60, 30, 600,
     '{"metric":"memory_percent","operator":">","threshold":85}'::jsonb,
     '{"category":"performance"}'::jsonb, '{}'::jsonb, 'admin', NOW() - INTERVAL '60 days', NOW()),

    ('rule-node-offline',  'Node Offline',       'Node has not reported in 5 minutes',              'active', 'heartbeat_stale',   'critical','global', 300, 60, 60, 300,
     '{"stale_seconds":300}'::jsonb,
     '{"category":"availability"}'::jsonb, '{}'::jsonb, 'admin', NOW() - INTERVAL '60 days', NOW()),

    ('rule-service-down',  'Service Down',       'Critical service process not detected in logs',   'active', 'log_pattern',       'critical','global', 120, 30, 30, 120,
     '{"pattern":"service (stopped|crashed|exited)","level":"error"}'::jsonb,
     '{"category":"availability"}'::jsonb, '{}'::jsonb, 'admin', NOW() - INTERVAL '30 days', NOW())
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 6. ALERT INSTANCES — 4 (firing, acknowledged, pending, resolved)
-- ============================================================
INSERT INTO alert_instances (id, rule_id, fingerprint, status, severity, labels, annotations, started_at, resolved_at, last_fired_at, suppressed_by, created_at, updated_at) VALUES
    ('ai-truenas-disk',   'rule-disk-full',       'truenas-main:disk_percent',  'firing',        'critical',
     '{"asset_id":"asset-truenas-main","metric":"disk_percent"}'::jsonb,
     '{"summary":"Disk usage at 91.3% on truenas-main","value":"91.3"}'::jsonb,
     NOW() - INTERVAL '2 hours', NULL, NOW() - INTERVAL '5 minutes', NULL,
     NOW() - INTERVAL '2 hours', NOW()),

    ('ai-web-cpu',        'rule-high-cpu',        'docker-host-1:cpu_percent',  'acknowledged',  'medium',
     '{"asset_id":"asset-docker-host-1","metric":"cpu_percent"}'::jsonb,
     '{"summary":"CPU spike on docker-host-1 during container rebuild","value":"87.2"}'::jsonb,
     NOW() - INTERVAL '1 day', NULL, NOW() - INTERVAL '6 hours', NULL,
     NOW() - INTERVAL '1 day', NOW()),

    ('ai-media-mem',      'rule-memory-pressure', 'media-server:memory',        'pending',       'high',
     '{"asset_id":"asset-media-server","metric":"memory_percent"}'::jsonb,
     '{"summary":"Memory approaching threshold on media-server","value":"84.1"}'::jsonb,
     NOW() - INTERVAL '15 minutes', NULL, NOW() - INTERVAL '2 minutes', NULL,
     NOW() - INTERVAL '15 minutes', NOW()),

    ('ai-dev-offline',    'rule-node-offline',    'dev-vm:heartbeat',           'resolved',      'critical',
     '{"asset_id":"asset-dev-vm"}'::jsonb,
     '{"summary":"dev-vm has not reported in over 2 hours"}'::jsonb,
     NOW() - INTERVAL '4 hours', NOW() - INTERVAL '1 hour', NOW() - INTERVAL '2 hours', NULL,
     NOW() - INTERVAL '4 hours', NOW() - INTERVAL '1 hour')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 7. INCIDENTS — 3 with full timelines
-- ============================================================
INSERT INTO incidents (id, title, summary, status, severity, source, group_id, primary_asset_id, assignee, created_by, opened_at, mitigated_at, resolved_at, closed_at, metadata, created_at, updated_at) VALUES
    ('inc-storage-cap', 'NAS Storage Capacity Critical',
     'TrueNAS truenas-main disk usage has reached 91%. Immediate action required to prevent data loss. Consider expanding pool or archiving old backups.',
     'open', 'critical', 'alert_auto', 'group-storage', 'asset-truenas-main', 'admin', 'system',
     NOW() - INTERVAL '2 hours', NULL, NULL, NULL, '{}'::jsonb,
     NOW() - INTERVAL '2 hours', NOW()),

    ('inc-dev-vm-down', 'Dev Environment Unreachable',
     'The development VM has been offline for 2+ hours. May affect CI/CD pipelines. Last seen sending heartbeat at normal intervals before going silent.',
     'investigating', 'high', 'alert_auto', 'group-dev', 'asset-dev-vm', '', 'system',
     NOW() - INTERVAL '4 hours', NULL, NULL, NULL, '{}'::jsonb,
     NOW() - INTERVAL '4 hours', NOW()),

    ('inc-web-latency', 'Docker Host Latency Degradation',
     'Intermittent latency spikes on docker-host-1 during peak hours. Root cause identified as container restart storm from a bad image pull. Fixed by pinning image digests.',
     'resolved', 'medium', 'manual', 'group-prod', 'asset-docker-host-1', 'admin', 'admin',
     NOW() - INTERVAL '7 days', NOW() - INTERVAL '6 days', NOW() - INTERVAL '5 days', NULL, '{}'::jsonb,
     NOW() - INTERVAL '7 days', NOW() - INTERVAL '5 days')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 8. INCIDENT ALERT LINKS
-- ============================================================
INSERT INTO incident_alert_links (id, incident_id, alert_rule_id, alert_instance_id, alert_fingerprint, link_type, created_by, created_at) VALUES
    ('ial-1', 'inc-storage-cap', 'rule-disk-full',    'ai-truenas-disk', 'truenas-main:disk_percent', 'trigger', 'system', NOW() - INTERVAL '2 hours'),
    ('ial-2', 'inc-dev-vm-down', 'rule-node-offline', 'ai-dev-offline',  'dev-vm:heartbeat',          'trigger', 'system', NOW() - INTERVAL '4 hours')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 9. INCIDENT EVENTS — timelines for all 3 incidents
-- ============================================================
INSERT INTO incident_events (id, incident_id, event_type, source_ref, summary, severity, metadata, occurred_at, created_at) VALUES
    -- Storage capacity incident
    ('ie-01', 'inc-storage-cap', 'alert_fired',     'ai-truenas-disk',            'Alert fired: Disk Nearly Full on truenas-main (91.3%)',                              'critical', '{}'::jsonb, NOW() - INTERVAL '2 hours',     NOW() - INTERVAL '2 hours'),
    ('ie-02', 'inc-storage-cap', 'audit',           'admin:note:storage-1',       'Investigating storage pool. ZFS scrub completed with no errors. Usage is legitimate growth from backup retention.', 'info', '{}'::jsonb, NOW() - INTERVAL '90 minutes',  NOW() - INTERVAL '90 minutes'),
    ('ie-03', 'inc-storage-cap', 'config_change',   'admin:config:retention',     'Adjusted snapshot retention from 30d to 14d to reclaim space.',                       'info',     '{}'::jsonb, NOW() - INTERVAL '60 minutes',  NOW() - INTERVAL '60 minutes'),

    -- Dev VM incident
    ('ie-04', 'inc-dev-vm-down', 'alert_fired',     'ai-dev-offline',             'Alert fired: Node Offline for dev-vm',                                                'critical', '{}'::jsonb, NOW() - INTERVAL '4 hours',     NOW() - INTERVAL '4 hours'),
    ('ie-05', 'inc-dev-vm-down', 'heartbeat_change','dev-vm:heartbeat:stale',     'Heartbeat status changed: online -> stale -> offline',                                 'high',     '{}'::jsonb, NOW() - INTERVAL '3 hours 55 minutes', NOW() - INTERVAL '3 hours 55 minutes'),
    ('ie-06', 'inc-dev-vm-down', 'audit',           'admin:investigate:dev-vm',   'Status changed to investigating. Checking Proxmox host console for VM state.',          'info',     '{}'::jsonb, NOW() - INTERVAL '2 hours',     NOW() - INTERVAL '2 hours'),
    ('ie-07', 'inc-dev-vm-down', 'alert_resolved',  'ai-dev-offline:resolved',    'VM restarted successfully, heartbeat restored.',                                        'info',     '{}'::jsonb, NOW() - INTERVAL '1 hour',      NOW() - INTERVAL '1 hour'),

    -- Docker host latency incident
    ('ie-08', 'inc-web-latency', 'metric_anomaly',  'docker-host-1:cpu:anomaly',  'CPU anomaly detected during container restart storm.',                                  'high',     '{"metric":"cpu_percent","peak_value":94.2}'::jsonb, NOW() - INTERVAL '7 days',  NOW() - INTERVAL '7 days'),
    ('ie-09', 'inc-web-latency', 'log_burst',       'docker-host-1:logs:burst',   'Burst of 340 error-level log events in 5 minutes from container runtime.',              'high',     '{"count":340,"window_seconds":300}'::jsonb,          NOW() - INTERVAL '6 days 23 hours', NOW() - INTERVAL '6 days 23 hours'),
    ('ie-10', 'inc-web-latency', 'config_change',   'admin:config:image-pins',    'Pinned all container images to digest references to prevent bad pulls.',                 'info',     '{}'::jsonb, NOW() - INTERVAL '6 days',  NOW() - INTERVAL '6 days'),
    ('ie-11', 'inc-web-latency', 'alert_resolved',  'docker-host-1:cpu:resolved', 'No further latency spikes after image pinning. Marking resolved.',                      'info',     '{}'::jsonb, NOW() - INTERVAL '5 days',  NOW() - INTERVAL '5 days')
ON CONFLICT (id) DO NOTHING;

-- ============================================================
-- 10. METRIC SAMPLES — per node: CPU, memory, disk, network_rx
-- ============================================================
INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at) VALUES
    -- proxmox-1
    ('asset-proxmox-1',     'cpu_percent',    '%',    38.5,   NOW() - INTERVAL '1 minute'),
    ('asset-proxmox-1',     'memory_percent', '%',    72.1,   NOW() - INTERVAL '1 minute'),
    ('asset-proxmox-1',     'disk_percent',   '%',    55.0,   NOW() - INTERVAL '1 minute'),
    ('asset-proxmox-1',     'network_rx_mbps','Mbps', 12.4,   NOW() - INTERVAL '1 minute'),

    -- truenas-main
    ('asset-truenas-main',  'cpu_percent',    '%',    41.7,   NOW() - INTERVAL '5 minutes'),
    ('asset-truenas-main',  'memory_percent', '%',    72.0,   NOW() - INTERVAL '5 minutes'),
    ('asset-truenas-main',  'disk_percent',   '%',    91.3,   NOW() - INTERVAL '5 minutes'),
    ('asset-truenas-main',  'network_rx_mbps','Mbps', 85.2,   NOW() - INTERVAL '5 minutes'),

    -- docker-host-1
    ('asset-docker-host-1', 'cpu_percent',    '%',    22.0,   NOW() - INTERVAL '1 minute'),
    ('asset-docker-host-1', 'memory_percent', '%',    60.5,   NOW() - INTERVAL '1 minute'),
    ('asset-docker-host-1', 'disk_percent',   '%',    48.3,   NOW() - INTERVAL '1 minute'),
    ('asset-docker-host-1', 'network_rx_mbps','Mbps', 5.8,    NOW() - INTERVAL '1 minute'),

    -- pihole
    ('asset-pihole',        'cpu_percent',    '%',    8.2,    NOW() - INTERVAL '45 seconds'),
    ('asset-pihole',        'memory_percent', '%',    34.1,   NOW() - INTERVAL '45 seconds'),
    ('asset-pihole',        'disk_percent',   '%',    22.0,   NOW() - INTERVAL '45 seconds'),
    ('asset-pihole',        'network_rx_mbps','Mbps', 0.3,    NOW() - INTERVAL '45 seconds'),

    -- home-assistant
    ('asset-home-assistant','cpu_percent',    '%',    15.6,   NOW() - INTERVAL '1 minute'),
    ('asset-home-assistant','memory_percent', '%',    52.8,   NOW() - INTERVAL '1 minute'),
    ('asset-home-assistant','disk_percent',   '%',    38.9,   NOW() - INTERVAL '1 minute'),
    ('asset-home-assistant','network_rx_mbps','Mbps', 1.1,    NOW() - INTERVAL '1 minute'),

    -- dev-vm (last readings before going offline)
    ('asset-dev-vm',        'cpu_percent',    '%',    65.3,   NOW() - INTERVAL '2 hours'),
    ('asset-dev-vm',        'memory_percent', '%',    78.9,   NOW() - INTERVAL '2 hours'),
    ('asset-dev-vm',        'disk_percent',   '%',    71.4,   NOW() - INTERVAL '2 hours'),
    ('asset-dev-vm',        'network_rx_mbps','Mbps', 2.1,    NOW() - INTERVAL '2 hours'),

    -- backup-server
    ('asset-backup-server', 'cpu_percent',    '%',    12.0,   NOW() - INTERVAL '3 minutes'),
    ('asset-backup-server', 'memory_percent', '%',    44.5,   NOW() - INTERVAL '3 minutes'),
    ('asset-backup-server', 'disk_percent',   '%',    67.8,   NOW() - INTERVAL '3 minutes'),
    ('asset-backup-server', 'network_rx_mbps','Mbps', 45.3,   NOW() - INTERVAL '3 minutes'),

    -- media-server
    ('asset-media-server',  'cpu_percent',    '%',    55.2,   NOW() - INTERVAL '2 minutes'),
    ('asset-media-server',  'memory_percent', '%',    84.1,   NOW() - INTERVAL '2 minutes'),
    ('asset-media-server',  'disk_percent',   '%',    58.0,   NOW() - INTERVAL '2 minutes'),
    ('asset-media-server',  'network_rx_mbps','Mbps', 92.7,   NOW() - INTERVAL '2 minutes'),

    -- k3s-node-1
    ('asset-k3s-node-1',    'cpu_percent',    '%',    29.4,   NOW() - INTERVAL '40 seconds'),
    ('asset-k3s-node-1',    'memory_percent', '%',    61.2,   NOW() - INTERVAL '40 seconds'),
    ('asset-k3s-node-1',    'disk_percent',   '%',    42.1,   NOW() - INTERVAL '40 seconds'),
    ('asset-k3s-node-1',    'network_rx_mbps','Mbps', 8.9,    NOW() - INTERVAL '40 seconds'),

    -- monitoring
    ('asset-monitoring',    'cpu_percent',    '%',    18.8,   NOW() - INTERVAL '1 minute'),
    ('asset-monitoring',    'memory_percent', '%',    56.3,   NOW() - INTERVAL '1 minute'),
    ('asset-monitoring',    'disk_percent',   '%',    35.5,   NOW() - INTERVAL '1 minute'),
    ('asset-monitoring',    'network_rx_mbps','Mbps', 3.2,    NOW() - INTERVAL '1 minute'),

    -- Historical samples for sparklines (proxmox-1 CPU over last hour)
    ('asset-proxmox-1',     'cpu_percent',    '%',    42.1,   NOW() - INTERVAL '60 minutes'),
    ('asset-proxmox-1',     'cpu_percent',    '%',    35.8,   NOW() - INTERVAL '50 minutes'),
    ('asset-proxmox-1',     'cpu_percent',    '%',    51.2,   NOW() - INTERVAL '40 minutes'),
    ('asset-proxmox-1',     'cpu_percent',    '%',    47.6,   NOW() - INTERVAL '30 minutes'),
    ('asset-proxmox-1',     'cpu_percent',    '%',    33.9,   NOW() - INTERVAL '20 minutes'),
    ('asset-proxmox-1',     'cpu_percent',    '%',    40.0,   NOW() - INTERVAL '10 minutes'),

    -- Historical samples for sparklines (truenas-main disk over last 6 hours)
    ('asset-truenas-main',  'disk_percent',   '%',    88.1,   NOW() - INTERVAL '6 hours'),
    ('asset-truenas-main',  'disk_percent',   '%',    89.0,   NOW() - INTERVAL '5 hours'),
    ('asset-truenas-main',  'disk_percent',   '%',    89.7,   NOW() - INTERVAL '4 hours'),
    ('asset-truenas-main',  'disk_percent',   '%',    90.2,   NOW() - INTERVAL '3 hours'),
    ('asset-truenas-main',  'disk_percent',   '%',    90.8,   NOW() - INTERVAL '2 hours'),
    ('asset-truenas-main',  'disk_percent',   '%',    91.1,   NOW() - INTERVAL '1 hour');

-- ============================================================
-- 11. LOG EVENTS — ~50 across nodes (mix of info/warn/error)
-- ============================================================
INSERT INTO log_events (id, asset_id, source, level, message, fields, timestamp) VALUES
    -- proxmox-1
    (gen_random_uuid()::text, 'asset-proxmox-1',     'agent',  'info',    'Heartbeat OK: cpu=38.5% mem=72.1% disk=55.0%',                    NULL, NOW() - INTERVAL '1 minute'),
    (gen_random_uuid()::text, 'asset-proxmox-1',     'agent',  'info',    'VM dev-vm: status=stopped, uptime=0s',                            NULL, NOW() - INTERVAL '30 minutes'),
    (gen_random_uuid()::text, 'asset-proxmox-1',     'agent',  'info',    'ZFS pool rpool: healthy, 55% used',                               NULL, NOW() - INTERVAL '15 minutes'),
    (gen_random_uuid()::text, 'asset-proxmox-1',     'agent',  'warning', 'SMART warning on /dev/sdc: reallocated sector count increasing',   '{"disk":"/dev/sdc","attribute":"Reallocated_Sector_Ct","value":8}'::jsonb, NOW() - INTERVAL '45 minutes'),

    -- truenas-main
    (gen_random_uuid()::text, 'asset-truenas-main',  'agent',  'error',   'ZFS pool tank: usage 91.3% exceeds 90% threshold',                 '{"pool":"tank","used_pct":91.3}'::jsonb, NOW() - INTERVAL '5 minutes'),
    (gen_random_uuid()::text, 'asset-truenas-main',  'agent',  'warning', 'Scrub completed with 0 errors, runtime 4h22m',                     NULL, NOW() - INTERVAL '3 hours'),
    (gen_random_uuid()::text, 'asset-truenas-main',  'agent',  'info',    'NFS exports: 4 active clients',                                    NULL, NOW() - INTERVAL '10 minutes'),
    (gen_random_uuid()::text, 'asset-truenas-main',  'agent',  'warning', 'Disk temperature /dev/da3: 48C (threshold 50C)',                    '{"disk":"/dev/da3","temp_c":48}'::jsonb, NOW() - INTERVAL '1 hour'),
    (gen_random_uuid()::text, 'asset-truenas-main',  'agent',  'info',    'Snapshot auto-created: tank/backups@auto-2026-03-23-06:00',         NULL, NOW() - INTERVAL '6 hours'),

    -- docker-host-1
    (gen_random_uuid()::text, 'asset-docker-host-1', 'agent',  'info',    'Docker containers: 14 running, 0 stopped, 2 paused',               '{"running":14,"stopped":0,"paused":2}'::jsonb, NOW() - INTERVAL '1 minute'),
    (gen_random_uuid()::text, 'asset-docker-host-1', 'agent',  'info',    'Container grafana: healthy, uptime 14d',                            NULL, NOW() - INTERVAL '5 minutes'),
    (gen_random_uuid()::text, 'asset-docker-host-1', 'agent',  'warning', 'Container nginx-proxy: restart count 3 in last hour',               '{"container":"nginx-proxy","restarts":3}'::jsonb, NOW() - INTERVAL '20 minutes'),
    (gen_random_uuid()::text, 'asset-docker-host-1', 'agent',  'error',   'Container redis-cache: OOMKilled, restarting',                      '{"container":"redis-cache","exit_code":137}'::jsonb, NOW() - INTERVAL '35 minutes'),
    (gen_random_uuid()::text, 'asset-docker-host-1', 'agent',  'info',    'Docker image prune: reclaimed 2.8 GB',                              NULL, NOW() - INTERVAL '2 hours'),

    -- pihole
    (gen_random_uuid()::text, 'asset-pihole',        'agent',  'info',    'DNS queries last hour: 12,847 (14.2% blocked)',                     '{"queries":12847,"blocked_pct":14.2}'::jsonb, NOW() - INTERVAL '45 seconds'),
    (gen_random_uuid()::text, 'asset-pihole',        'agent',  'info',    'Gravity database updated: 142,891 domains blocked',                 NULL, NOW() - INTERVAL '6 hours'),
    (gen_random_uuid()::text, 'asset-pihole',        'agent',  'warning', 'High query rate from 10.0.1.8: 2,400 queries in 5min',              '{"client":"10.0.1.8","queries":2400}'::jsonb, NOW() - INTERVAL '30 minutes'),

    -- home-assistant
    (gen_random_uuid()::text, 'asset-home-assistant', 'agent', 'info',    'Automation triggered: lights_off_away (away mode detected)',         NULL, NOW() - INTERVAL '2 hours'),
    (gen_random_uuid()::text, 'asset-home-assistant', 'agent', 'info',    'Integration reload: mqtt (47 entities updated)',                     NULL, NOW() - INTERVAL '4 hours'),
    (gen_random_uuid()::text, 'asset-home-assistant', 'agent', 'warning', 'Z-Wave device unresponsive: front_door_lock (node 14)',              '{"node_id":14,"device":"front_door_lock"}'::jsonb, NOW() - INTERVAL '1 hour'),
    (gen_random_uuid()::text, 'asset-home-assistant', 'agent', 'error',   'Integration error: hue_bridge connection refused (retrying in 30s)', '{"integration":"hue_bridge"}'::jsonb, NOW() - INTERVAL '25 minutes'),

    -- dev-vm (last logs before going offline)
    (gen_random_uuid()::text, 'asset-dev-vm',        'agent',  'info',    'CI runner: 3 jobs completed, 0 failed',                             NULL, NOW() - INTERVAL '2 hours 5 minutes'),
    (gen_random_uuid()::text, 'asset-dev-vm',        'agent',  'warning', 'Disk I/O latency elevated: avg 45ms (threshold 20ms)',               '{"avg_ms":45,"threshold_ms":20}'::jsonb, NOW() - INTERVAL '2 hours 3 minutes'),
    (gen_random_uuid()::text, 'asset-dev-vm',        'agent',  'error',   'Agent lost connection to hub, attempting reconnect',                  NULL, NOW() - INTERVAL '2 hours 1 minute'),
    (gen_random_uuid()::text, 'asset-dev-vm',        'agent',  'error',   'Heartbeat timeout: no response in 300s',                             NULL, NOW() - INTERVAL '2 hours'),

    -- backup-server
    (gen_random_uuid()::text, 'asset-backup-server', 'agent',  'info',    'Backup job completed: 5 VMs, 12.4 GB total, duration 42m',          '{"vms":5,"size_gb":12.4,"duration_min":42}'::jsonb, NOW() - INTERVAL '30 minutes'),
    (gen_random_uuid()::text, 'asset-backup-server', 'agent',  'info',    'Verification passed: all 5 VM backups integrity OK',                 NULL, NOW() - INTERVAL '25 minutes'),
    (gen_random_uuid()::text, 'asset-backup-server', 'agent',  'info',    'Pruning old backups: removed 3 snapshots, freed 8.1 GB',             NULL, NOW() - INTERVAL '3 hours'),
    (gen_random_uuid()::text, 'asset-backup-server', 'agent',  'warning', 'Backup datastore usage: 67.8% (retention policy: 14 days)',          NULL, NOW() - INTERVAL '30 minutes'),

    -- media-server
    (gen_random_uuid()::text, 'asset-media-server',  'agent',  'info',    'Active streams: 3 (2 direct play, 1 transcode)',                    '{"direct_play":2,"transcode":1}'::jsonb, NOW() - INTERVAL '2 minutes'),
    (gen_random_uuid()::text, 'asset-media-server',  'agent',  'info',    'Library scan completed: 247 new items indexed',                      NULL, NOW() - INTERVAL '4 hours'),
    (gen_random_uuid()::text, 'asset-media-server',  'agent',  'warning', 'Transcode buffer underrun on stream 3 (client: Apple TV)',           '{"stream_id":3,"client":"Apple TV"}'::jsonb, NOW() - INTERVAL '15 minutes'),
    (gen_random_uuid()::text, 'asset-media-server',  'agent',  'info',    'Hardware transcode: Intel QuickSync active, GPU utilization 62%',    NULL, NOW() - INTERVAL '10 minutes'),

    -- k3s-node-1
    (gen_random_uuid()::text, 'asset-k3s-node-1',   'agent',  'info',    'Pods: 22 running, 0 pending, 0 failed',                              '{"running":22,"pending":0,"failed":0}'::jsonb, NOW() - INTERVAL '40 seconds'),
    (gen_random_uuid()::text, 'asset-k3s-node-1',   'agent',  'info',    'Deployment rollout complete: ingress-nginx (3/3 replicas ready)',      NULL, NOW() - INTERVAL '1 hour'),
    (gen_random_uuid()::text, 'asset-k3s-node-1',   'agent',  'warning', 'Pod cert-manager-webhook: CrashLoopBackOff (restart count: 5)',       '{"pod":"cert-manager-webhook","restarts":5}'::jsonb, NOW() - INTERVAL '45 minutes'),
    (gen_random_uuid()::text, 'asset-k3s-node-1',   'agent',  'info',    'Certificate renewed: *.lab.local (expires in 89 days)',                NULL, NOW() - INTERVAL '1 day'),
    (gen_random_uuid()::text, 'asset-k3s-node-1',   'agent',  'error',   'Persistent volume claim pvc-logs: filesystem nearly full (94%)',       '{"pvc":"pvc-logs","used_pct":94}'::jsonb, NOW() - INTERVAL '30 minutes'),

    -- monitoring
    (gen_random_uuid()::text, 'asset-monitoring',    'agent',  'info',    'Prometheus: 847 active targets, 0 down',                              '{"targets":847,"down":0}'::jsonb, NOW() - INTERVAL '1 minute'),
    (gen_random_uuid()::text, 'asset-monitoring',    'agent',  'info',    'Grafana: 12 dashboards loaded, 3 alerts configured',                  NULL, NOW() - INTERVAL '5 minutes'),
    (gen_random_uuid()::text, 'asset-monitoring',    'agent',  'info',    'Loki: ingestion rate 2.1 MB/s, 14-day retention',                     NULL, NOW() - INTERVAL '10 minutes'),
    (gen_random_uuid()::text, 'asset-monitoring',    'agent',  'warning', 'Alertmanager: notification queue depth 47 (threshold 100)',            '{"queue_depth":47}'::jsonb, NOW() - INTERVAL '20 minutes'),

    -- Hub-level log events (no asset)
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'Alert rule evaluated: rule-disk-full -> firing for truenas-main',               NULL, NOW() - INTERVAL '5 minutes'),
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'Incident inc-storage-cap created from alert ai-truenas-disk',                   NULL, NOW() - INTERVAL '2 hours'),
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'User admin logged in from 192.168.1.100',                                       NULL, NOW() - INTERVAL '3 hours'),
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'Agent checkin: 9 of 10 assets reporting healthy',                               NULL, NOW() - INTERVAL '2 minutes'),
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'Scheduled job completed: metric_cleanup (removed 14,203 samples older than 30d)',NULL, NOW() - INTERVAL '6 hours'),
    (gen_random_uuid()::text, NULL, 'hub', 'warning', 'Rate limit triggered for API endpoint /api/v1/metrics (client 10.0.1.10)',       NULL, NOW() - INTERVAL '4 hours'),
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'TLS certificate auto-renewed for hub.lab.local, expires in 89 days',            NULL, NOW() - INTERVAL '1 day'),
    (gen_random_uuid()::text, NULL, 'hub', 'info',    'Database vacuum completed: 12 tables, freed 148 MB',                            NULL, NOW() - INTERVAL '12 hours');

-- ============================================================
-- 12. AUDIT EVENTS — ~20
-- ============================================================
INSERT INTO audit_events (id, type, actor_id, target, session_id, decision, reason, details, timestamp) VALUES
    (gen_random_uuid()::text, 'login',           'admin',  NULL,                    'sess-001', 'allow', 'valid credentials',                   '{"ip":"192.168.1.100","method":"local"}'::jsonb,                         NOW() - INTERVAL '3 hours'),
    (gen_random_uuid()::text, 'login',           'admin',  NULL,                    'sess-002', 'allow', 'valid credentials',                   '{"ip":"192.168.1.100","method":"local"}'::jsonb,                         NOW() - INTERVAL '1 day'),
    (gen_random_uuid()::text, 'login_failed',    NULL,     NULL,                    NULL,       'deny',  'invalid password',                    '{"ip":"10.0.1.99","username":"root","method":"local"}'::jsonb,           NOW() - INTERVAL '8 hours'),
    (gen_random_uuid()::text, 'asset_create',    'admin',  'asset-pihole',          'sess-001', 'allow', 'manual enrollment',                   '{"name":"pihole","type":"appliance"}'::jsonb,                            NOW() - INTERVAL '200 days'),
    (gen_random_uuid()::text, 'asset_create',    'admin',  'asset-k3s-node-1',      'sess-001', 'allow', 'agent auto-registration',             '{"name":"k3s-node-1","type":"server"}'::jsonb,                           NOW() - INTERVAL '45 days'),
    (gen_random_uuid()::text, 'asset_update',    'admin',  'asset-truenas-main',    'sess-001', 'allow', 'tags updated',                        '{"added_tags":["zfs"]}'::jsonb,                                          NOW() - INTERVAL '7 days'),
    (gen_random_uuid()::text, 'asset_update',    'admin',  'asset-docker-host-1',   'sess-001', 'allow', 'group assignment changed',             '{"old_group":null,"new_group":"group-prod"}'::jsonb,                     NOW() - INTERVAL '14 days'),
    (gen_random_uuid()::text, 'alert_rule_create','admin', 'rule-high-cpu',         'sess-001', 'allow', 'rule created',                        '{"name":"High CPU Usage","severity":"medium"}'::jsonb,                   NOW() - INTERVAL '60 days'),
    (gen_random_uuid()::text, 'alert_rule_create','admin', 'rule-disk-full',        'sess-001', 'allow', 'rule created',                        '{"name":"Disk Nearly Full","severity":"critical"}'::jsonb,               NOW() - INTERVAL '60 days'),
    (gen_random_uuid()::text, 'alert_rule_create','admin', 'rule-service-down',     'sess-002', 'allow', 'rule created',                        '{"name":"Service Down","severity":"critical"}'::jsonb,                   NOW() - INTERVAL '30 days'),
    (gen_random_uuid()::text, 'alert_ack',       'admin',  'ai-web-cpu',            'sess-001', 'allow', 'alert acknowledged',                  '{"rule":"rule-high-cpu","asset":"asset-docker-host-1"}'::jsonb,          NOW() - INTERVAL '18 hours'),
    (gen_random_uuid()::text, 'incident_create', 'system', 'inc-storage-cap',       NULL,       'allow', 'auto-created from alert',             '{"alert":"ai-truenas-disk","severity":"critical"}'::jsonb,               NOW() - INTERVAL '2 hours'),
    (gen_random_uuid()::text, 'incident_create', 'system', 'inc-dev-vm-down',       NULL,       'allow', 'auto-created from alert',             '{"alert":"ai-dev-offline","severity":"critical"}'::jsonb,                NOW() - INTERVAL '4 hours'),
    (gen_random_uuid()::text, 'incident_create', 'admin',  'inc-web-latency',       'sess-002', 'allow', 'manually created',                    '{"severity":"medium"}'::jsonb,                                           NOW() - INTERVAL '7 days'),
    (gen_random_uuid()::text, 'incident_update', 'admin',  'inc-dev-vm-down',       'sess-001', 'allow', 'status changed to investigating',     '{"old_status":"open","new_status":"investigating"}'::jsonb,              NOW() - INTERVAL '2 hours'),
    (gen_random_uuid()::text, 'incident_resolve','admin',  'inc-web-latency',       'sess-002', 'allow', 'incident resolved',                   '{"resolution":"image digests pinned"}'::jsonb,                           NOW() - INTERVAL '5 days'),
    (gen_random_uuid()::text, 'group_create',    'admin',  'group-prod',            'sess-002', 'allow', 'group created',                       '{"name":"Production"}'::jsonb,                                           NOW() - INTERVAL '90 days'),
    (gen_random_uuid()::text, 'group_create',    'admin',  'group-dev',             'sess-002', 'allow', 'group created',                       '{"name":"Development"}'::jsonb,                                          NOW() - INTERVAL '90 days'),
    (gen_random_uuid()::text, 'group_create',    'admin',  'group-storage',         'sess-002', 'allow', 'group created as child of Production','{"name":"Storage","parent":"group-prod"}'::jsonb,                        NOW() - INTERVAL '60 days'),
    (gen_random_uuid()::text, 'settings_update', 'admin',  'retention_policy',      'sess-001', 'allow', 'metric retention changed',            '{"old_days":90,"new_days":30}'::jsonb,                                   NOW() - INTERVAL '14 days');

-- ============================================================
-- 13. ACTION RUNS — 3 sample runs
-- ============================================================
INSERT INTO action_runs (id, type, actor_id, target, command, status, output, error, created_at, updated_at, completed_at) VALUES
    (gen_random_uuid()::text, 'restart_service', 'admin', 'asset-docker-host-1', 'docker restart nginx-proxy', 'completed', 'nginx-proxy\n', '', NOW() - INTERVAL '20 minutes', NOW() - INTERVAL '19 minutes', NOW() - INTERVAL '19 minutes'),
    (gen_random_uuid()::text, 'run_script',      'admin', 'asset-truenas-main',  'zpool status tank',          'completed', 'pool: tank\n  state: ONLINE\n  scan: scrub repaired 0B\n', '', NOW() - INTERVAL '90 minutes', NOW() - INTERVAL '89 minutes', NOW() - INTERVAL '89 minutes'),
    (gen_random_uuid()::text, 'restart_service', 'admin', 'asset-k3s-node-1',    'kubectl rollout restart deployment/cert-manager-webhook -n cert-manager', 'failed', '', 'error: deployment "cert-manager-webhook" not found in namespace "cert-manager"', NOW() - INTERVAL '40 minutes', NOW() - INTERVAL '40 minutes', NOW() - INTERVAL '40 minutes');

COMMIT;
