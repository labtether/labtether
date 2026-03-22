-- Demo seed data for App Store review.
-- Run AFTER the hub has started and applied migrations:
--   psql "$DATABASE_URL" -f scripts/demo-seed.sql

BEGIN;

-- Apple review account (viewer role, read-only)
INSERT INTO users (id, username, password_hash, role, auth_provider, created_at, updated_at)
VALUES (
    'demo-apple-review',
    'apple-review',
    '$2a$12$kCnzirpFXr7vvSXs0GzB1.AwnAIpOWf4aXINGAxKm8PjozU3LUiZO',
    'viewer',
    'local',
    NOW(),
    NOW()
) ON CONFLICT (id) DO NOTHING;

-- Sample assets (no real agents connected)
INSERT INTO assets (id, hostname, display_name, os, arch, agent_version, status, ip_addresses, created_at, updated_at, last_seen_at) VALUES
    ('asset-web-prod-1',    'web-prod-1',    'Web Server (Production)',      'ubuntu-24.04', 'amd64', '1.4.2', 'online',  '["10.0.1.10"]', NOW() - INTERVAL '30 days', NOW(), NOW() - INTERVAL '2 minutes'),
    ('asset-web-prod-2',    'web-prod-2',    'Web Server (Production) #2',   'ubuntu-24.04', 'amd64', '1.4.2', 'online',  '["10.0.1.11"]', NOW() - INTERVAL '30 days', NOW(), NOW() - INTERVAL '1 minute'),
    ('asset-db-primary',    'db-primary',    'PostgreSQL Primary',           'debian-12',    'amd64', '1.4.2', 'online',  '["10.0.2.10"]', NOW() - INTERVAL '90 days', NOW(), NOW() - INTERVAL '30 seconds'),
    ('asset-db-replica',    'db-replica',    'PostgreSQL Replica',           'debian-12',    'amd64', '1.4.2', 'online',  '["10.0.2.11"]', NOW() - INTERVAL '90 days', NOW(), NOW() - INTERVAL '45 seconds'),
    ('asset-cache-1',       'cache-1',       'Redis Cache',                  'alpine-3.20',  'amd64', '1.4.1', 'online',  '["10.0.3.10"]', NOW() - INTERVAL '60 days', NOW(), NOW() - INTERVAL '1 minute'),
    ('asset-monitor',       'monitor',       'Monitoring Stack',             'ubuntu-24.04', 'amd64', '1.4.2', 'online',  '["10.0.4.10"]', NOW() - INTERVAL '120 days', NOW(), NOW() - INTERVAL '2 minutes'),
    ('asset-nas-1',         'nas-1',         'TrueNAS Storage',             'truenas-13.3', 'amd64', '1.4.0', 'degraded','["10.0.5.10"]', NOW() - INTERVAL '180 days', NOW(), NOW() - INTERVAL '5 minutes'),
    ('asset-backup-1',      'backup-1',      'Proxmox Backup Server',       'debian-12',    'amd64', '1.4.2', 'online',  '["10.0.6.10"]', NOW() - INTERVAL '45 days', NOW(), NOW() - INTERVAL '3 minutes'),
    ('asset-dev-vm',        'dev-vm',        'Dev Environment VM',           'ubuntu-24.04', 'amd64', '1.3.9', 'offline', '["10.0.7.10"]', NOW() - INTERVAL '15 days', NOW() - INTERVAL '2 hours', NOW() - INTERVAL '2 hours'),
    ('asset-docker-host',   'docker-host',   'Docker Host',                  'debian-12',    'amd64', '1.4.2', 'online',  '["10.0.8.10"]', NOW() - INTERVAL '60 days', NOW(), NOW() - INTERVAL '1 minute')
ON CONFLICT (id) DO NOTHING;

-- Sample metric snapshots (recent telemetry for dashboard)
INSERT INTO metric_samples (asset_id, metric, value, collected_at) VALUES
    ('asset-web-prod-1',  'cpu_percent',    32.5,  NOW() - INTERVAL '1 minute'),
    ('asset-web-prod-1',  'memory_percent', 67.2,  NOW() - INTERVAL '1 minute'),
    ('asset-web-prod-1',  'disk_percent',   45.0,  NOW() - INTERVAL '1 minute'),
    ('asset-web-prod-1',  'load_1m',        1.2,   NOW() - INTERVAL '1 minute'),
    ('asset-web-prod-2',  'cpu_percent',    28.1,  NOW() - INTERVAL '1 minute'),
    ('asset-web-prod-2',  'memory_percent', 52.8,  NOW() - INTERVAL '1 minute'),
    ('asset-db-primary',  'cpu_percent',    18.3,  NOW() - INTERVAL '30 seconds'),
    ('asset-db-primary',  'memory_percent', 74.6,  NOW() - INTERVAL '30 seconds'),
    ('asset-db-primary',  'disk_percent',   62.1,  NOW() - INTERVAL '30 seconds'),
    ('asset-db-replica',  'cpu_percent',    12.7,  NOW() - INTERVAL '45 seconds'),
    ('asset-db-replica',  'memory_percent', 58.3,  NOW() - INTERVAL '45 seconds'),
    ('asset-cache-1',     'cpu_percent',    5.2,   NOW() - INTERVAL '1 minute'),
    ('asset-cache-1',     'memory_percent', 88.4,  NOW() - INTERVAL '1 minute'),
    ('asset-nas-1',       'cpu_percent',    41.7,  NOW() - INTERVAL '5 minutes'),
    ('asset-nas-1',       'memory_percent', 72.0,  NOW() - INTERVAL '5 minutes'),
    ('asset-nas-1',       'disk_percent',   91.3,  NOW() - INTERVAL '5 minutes'),
    ('asset-docker-host', 'cpu_percent',    22.0,  NOW() - INTERVAL '1 minute'),
    ('asset-docker-host', 'memory_percent', 60.5,  NOW() - INTERVAL '1 minute');

-- Sample alert rules
INSERT INTO alert_rules (id, name, description, severity, kind, status, condition_metric, condition_op, condition_threshold, duration_seconds, created_at, updated_at) VALUES
    ('alert-high-cpu',     'High CPU Usage',         'CPU usage above 80% for 5 minutes',     'warning',  'threshold', 'enabled',  'cpu_percent',    '>', 80,  300, NOW() - INTERVAL '30 days', NOW()),
    ('alert-disk-full',    'Disk Nearly Full',        'Disk usage above 90%',                  'critical', 'threshold', 'enabled',  'disk_percent',   '>', 90,  60,  NOW() - INTERVAL '30 days', NOW()),
    ('alert-memory-high',  'Memory Pressure',         'Memory usage above 85% for 10 minutes', 'warning',  'threshold', 'enabled',  'memory_percent', '>', 85,  600, NOW() - INTERVAL '30 days', NOW()),
    ('alert-node-offline', 'Node Offline',            'Node has not reported in 5 minutes',    'critical', 'heartbeat', 'enabled',  '',               '',  0,   300, NOW() - INTERVAL '30 days', NOW())
ON CONFLICT (id) DO NOTHING;

-- Sample alert instances (active alerts)
INSERT INTO alert_instances (id, rule_id, fingerprint, asset_id, status, severity, summary, first_fired_at, last_fired_at, created_at, updated_at) VALUES
    ('ai-nas-disk',     'alert-disk-full',   'nas-1:disk_percent',   'asset-nas-1',    'firing',       'critical', 'Disk usage at 91.3% on nas-1',           NOW() - INTERVAL '2 hours',  NOW() - INTERVAL '5 minutes', NOW() - INTERVAL '2 hours', NOW()),
    ('ai-cache-mem',    'alert-memory-high', 'cache-1:memory',       'asset-cache-1',  'firing',       'warning',  'Memory at 88.4% on cache-1',             NOW() - INTERVAL '45 minutes', NOW() - INTERVAL '1 minute', NOW() - INTERVAL '45 minutes', NOW()),
    ('ai-dev-offline',  'alert-node-offline','dev-vm:heartbeat',     'asset-dev-vm',   'firing',       'critical', 'dev-vm has not reported in over 2 hours', NOW() - INTERVAL '2 hours',  NOW() - INTERVAL '2 hours',   NOW() - INTERVAL '2 hours', NOW()),
    ('ai-web-cpu-old',  'alert-high-cpu',    'web-prod-1:cpu_old',   'asset-web-prod-1','acknowledged','warning',  'CPU spike on web-prod-1 (resolved)',      NOW() - INTERVAL '1 day',    NOW() - INTERVAL '6 hours',   NOW() - INTERVAL '1 day', NOW())
ON CONFLICT (id) DO NOTHING;

-- Sample incidents
INSERT INTO incidents (id, title, description, severity, status, source, assignee, created_at, updated_at) VALUES
    ('inc-storage-cap',  'NAS Storage Capacity Critical',    'TrueNAS nas-1 disk usage has reached 91%. Immediate action required to prevent data loss. Consider expanding pool or archiving old backups.', 'critical', 'open',     'alert',  'admin', NOW() - INTERVAL '2 hours', NOW()),
    ('inc-dev-vm-down',  'Dev Environment Unreachable',      'The development VM has been offline for 2+ hours. May affect CI/CD pipelines. Last seen sending heartbeat at normal intervals before going silent.', 'high', 'investigating', 'alert', '', NOW() - INTERVAL '2 hours', NOW()),
    ('inc-web-latency',  'Web Tier Latency Degradation',     'Intermittent latency spikes on web-prod-1 and web-prod-2 during peak hours last week. Root cause identified as connection pool exhaustion. Fixed by tuning pool settings.', 'medium', 'resolved', 'manual', 'admin', NOW() - INTERVAL '7 days', NOW() - INTERVAL '5 days')
ON CONFLICT (id) DO NOTHING;

-- Link alerts to incidents
INSERT INTO incident_alert_links (id, incident_id, alert_instance_id, created_at) VALUES
    ('ial-1', 'inc-storage-cap', 'ai-nas-disk',    NOW() - INTERVAL '2 hours'),
    ('ial-2', 'inc-dev-vm-down', 'ai-dev-offline',  NOW() - INTERVAL '2 hours')
ON CONFLICT (id) DO NOTHING;

-- Incident timeline events
INSERT INTO incident_events (id, incident_id, event_type, summary, actor, created_at) VALUES
    ('ie-1', 'inc-storage-cap', 'created',      'Incident created from alert: Disk Nearly Full on nas-1',  'system',  NOW() - INTERVAL '2 hours'),
    ('ie-2', 'inc-storage-cap', 'note',         'Investigating storage pool. ZFS scrub completed with no errors. Usage is legitimate growth from backup retention.', 'admin', NOW() - INTERVAL '90 minutes'),
    ('ie-3', 'inc-dev-vm-down', 'created',      'Incident created from alert: Node Offline for dev-vm',    'system',  NOW() - INTERVAL '2 hours'),
    ('ie-4', 'inc-dev-vm-down', 'status_change','Status changed to investigating',                         'admin',   NOW() - INTERVAL '1 hour'),
    ('ie-5', 'inc-web-latency', 'created',      'Incident opened manually after user reports of slow page loads', 'admin', NOW() - INTERVAL '7 days'),
    ('ie-6', 'inc-web-latency', 'note',         'Connection pool tuned from 25 to 100 max connections. Monitoring.', 'admin', NOW() - INTERVAL '6 days'),
    ('ie-7', 'inc-web-latency', 'resolved',     'No further latency spikes after pool tuning. Marking resolved.',    'admin', NOW() - INTERVAL '5 days')
ON CONFLICT (id) DO NOTHING;

-- Sample log events
INSERT INTO log_events (id, source, level, message, asset_id, created_at) VALUES
    (gen_random_uuid()::text, 'agent',  'info',    'Heartbeat OK: cpu=32.5% mem=67.2% disk=45.0%',    'asset-web-prod-1',  NOW() - INTERVAL '1 minute'),
    (gen_random_uuid()::text, 'agent',  'info',    'Heartbeat OK: cpu=18.3% mem=74.6% disk=62.1%',    'asset-db-primary',  NOW() - INTERVAL '30 seconds'),
    (gen_random_uuid()::text, 'agent',  'warning', 'Disk usage above threshold: 91.3%',                'asset-nas-1',       NOW() - INTERVAL '5 minutes'),
    (gen_random_uuid()::text, 'agent',  'error',   'Heartbeat timeout: no response in 300s',           'asset-dev-vm',      NOW() - INTERVAL '2 hours'),
    (gen_random_uuid()::text, 'hub',    'info',    'Alert rule evaluated: alert-disk-full → firing',   '',                  NOW() - INTERVAL '5 minutes'),
    (gen_random_uuid()::text, 'hub',    'info',    'Incident inc-storage-cap created from alert',      '',                  NOW() - INTERVAL '2 hours'),
    (gen_random_uuid()::text, 'agent',  'info',    'Docker containers: 12 running, 0 stopped',         'asset-docker-host', NOW() - INTERVAL '1 minute'),
    (gen_random_uuid()::text, 'agent',  'info',    'Backup job completed: 3 VMs, 2.1 GB total',        'asset-backup-1',    NOW() - INTERVAL '30 minutes'),
    (gen_random_uuid()::text, 'hub',    'info',    'User admin logged in from 192.168.1.100',          '',                  NOW() - INTERVAL '3 hours'),
    (gen_random_uuid()::text, 'hub',    'info',    'TLS certificate auto-renewed, expires in 89 days', '',                  NOW() - INTERVAL '1 day')
ON CONFLICT DO NOTHING;

-- Sample groups
INSERT INTO groups (id, name, description, parent_id, created_at, updated_at) VALUES
    ('group-prod',    'Production',   'Production infrastructure',  NULL,          NOW() - INTERVAL '90 days', NOW()),
    ('group-dev',     'Development',  'Development and staging',    NULL,          NOW() - INTERVAL '90 days', NOW()),
    ('group-storage', 'Storage',      'Storage and backup systems', 'group-prod',  NOW() - INTERVAL '60 days', NOW())
ON CONFLICT (id) DO NOTHING;

COMMIT;
