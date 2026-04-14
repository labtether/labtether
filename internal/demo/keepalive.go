// Package demo provides background processes for keeping demo instances alive
// with realistic, continuously refreshed data.
package demo

import (
	"context"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// assetProfile defines the per-asset CPU generation parameters.
type assetProfile struct {
	cpuBase      float64
	cpuAmplitude float64
	memBase      float64
	diskBase     float64
}

// onlineAssetIDs are the 8 assets that are kept "online" (last_seen_at refreshed).
var onlineAssetIDs = []string{
	"asset-proxmox-1",
	"asset-docker-host-1",
	"asset-pihole",
	"asset-home-assistant",
	"asset-backup-server",
	"asset-media-server",
	"asset-k3s-node-1",
	"asset-monitoring",
}

var profiles = map[string]assetProfile{
	"asset-proxmox-1":      {cpuBase: 45, cpuAmplitude: 20, memBase: 72, diskBase: 68},
	"asset-docker-host-1":  {cpuBase: 35, cpuAmplitude: 15, memBase: 65, diskBase: 55},
	"asset-pihole":         {cpuBase: 12, cpuAmplitude: 8, memBase: 38, diskBase: 22},
	"asset-home-assistant": {cpuBase: 20, cpuAmplitude: 10, memBase: 52, diskBase: 45},
	"asset-backup-server":  {cpuBase: 25, cpuAmplitude: 18, memBase: 80, diskBase: 82},
	"asset-media-server":   {cpuBase: 55, cpuAmplitude: 20, memBase: 70, diskBase: 88},
	"asset-k3s-node-1":     {cpuBase: 40, cpuAmplitude: 15, memBase: 58, diskBase: 50},
	"asset-monitoring":     {cpuBase: 30, cpuAmplitude: 12, memBase: 48, diskBase: 35},
}

var defaultProfile = assetProfile{cpuBase: 30, cpuAmplitude: 12, memBase: 55, diskBase: 50}

// activityTemplates defines realistic synthetic audit events.
var activityTemplates = []struct {
	eventType string
	target    string
	reason    string
}{
	{"health_check", "asset-proxmox-1", "periodic health check completed"},
	{"container_sync", "asset-docker-host-1", "container inventory synchronized"},
	{"dns_refresh", "asset-pihole", "DNS blocklist statistics refreshed"},
	{"automation_run", "asset-home-assistant", "automation engine cycle completed"},
	{"backup_verify", "asset-backup-server", "backup integrity verification passed"},
	{"media_scan", "asset-media-server", "media library scan completed"},
	{"cluster_reconcile", "asset-k3s-node-1", "k3s cluster state reconciled"},
	{"metric_collection", "asset-monitoring", "metric collection cycle completed"},
	{"pool_refresh", "asset-proxmox-1", "storage pool status refreshed"},
	{"network_scan", "asset-pihole", "network device discovery sweep completed"},
	{"certificate_check", "asset-docker-host-1", "TLS certificate expiry check passed"},
	{"snapshot_cleanup", "asset-backup-server", "old snapshots pruned"},
}

// RunKeepalive starts the demo keepalive loop. It runs every 30 seconds
// and refreshes asset timestamps, inserts synthetic metrics, generates
// activity events, and cycles alert states. It blocks until ctx is cancelled.
func RunKeepalive(ctx context.Context, pool *pgxpool.Pool) {
	log.Println("demo: keepalive goroutine started")

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	nextActivity := time.Now().Add(randomDuration(2*time.Minute, 5*time.Minute))
	nextAlertCycle := time.Now().Add(randomDuration(30*time.Minute, 60*time.Minute))

	// Run once immediately on startup.
	runTick(ctx, pool)

	for {
		select {
		case <-ctx.Done():
			log.Println("demo: keepalive goroutine stopped")
			return
		case now := <-ticker.C:
			runTick(ctx, pool)

			if now.After(nextActivity) {
				insertActivity(ctx, pool)
				nextActivity = now.Add(randomDuration(2*time.Minute, 5*time.Minute))
			}

			if now.After(nextAlertCycle) {
				cycleAlerts(ctx, pool)
				nextAlertCycle = now.Add(randomDuration(30*time.Minute, 60*time.Minute))
			}
		}
	}
}

func runTick(ctx context.Context, pool *pgxpool.Pool) {
	refreshAssets(ctx, pool)
	insertMetrics(ctx, pool)
	pruneOldMetrics(ctx, pool)
}

// refreshAssets updates last_seen_at for online assets and keeps stale assets stale.
func refreshAssets(ctx context.Context, pool *pgxpool.Pool) {
	for _, id := range onlineAssetIDs {
		offset := 5 + rand.IntN(41) // 5-45 seconds
		_, err := pool.Exec(ctx,
			`UPDATE assets SET last_seen_at = NOW() - ($1 || ' seconds')::interval WHERE id = $2`,
			fmt.Sprintf("%d", offset), id,
		)
		if err != nil {
			log.Printf("demo: refreshAssets %s: %v", id, err)
		}
	}

	// Keep stale assets stale.
	staleAssets := []struct {
		id       string
		interval string
	}{
		{"asset-dev-vm", "2 hours"},
		{"asset-truenas-main", "4 minutes"},
	}
	for _, sa := range staleAssets {
		_, err := pool.Exec(ctx,
			`UPDATE assets SET last_seen_at = NOW() - $1::interval WHERE id = $2`,
			sa.interval, sa.id,
		)
		if err != nil {
			log.Printf("demo: refreshAssets stale %s: %v", sa.id, err)
		}
	}
}

// insertMetrics inserts 4 metric rows (cpu, mem, disk, network) for each online asset.
func insertMetrics(ctx context.Context, pool *pgxpool.Pool) {
	now := time.Now()
	for _, id := range onlineAssetIDs {
		cpu := GenerateCPU(id, now)
		mem := generateMemory(id)
		disk := generateDisk(id)
		net := generateNetwork()

		metrics := []struct {
			name  string
			unit  string
			value float64
		}{
			{"cpu_percent", "%", cpu},
			{"memory_percent", "%", mem},
			{"disk_percent", "%", disk},
			{"network_rx_mbps", "Mbps", net},
		}

		for _, m := range metrics {
			_, err := pool.Exec(ctx,
				`INSERT INTO metric_samples (asset_id, metric, unit, value, collected_at) VALUES ($1, $2, $3, $4, $5)`,
				id, m.name, m.unit, m.value, now,
			)
			if err != nil {
				log.Printf("demo: insertMetrics %s/%s: %v", id, m.name, err)
			}
		}
	}
}

// pruneOldMetrics removes metric samples older than 24 hours.
func pruneOldMetrics(ctx context.Context, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx,
		`DELETE FROM metric_samples WHERE collected_at < NOW() - INTERVAL '24 hours'`,
	)
	if err != nil {
		log.Printf("demo: pruneOldMetrics: %v", err)
	}
}

// insertActivity inserts a synthetic audit event.
func insertActivity(ctx context.Context, pool *pgxpool.Pool) {
	tmpl := activityTemplates[rand.IntN(len(activityTemplates))]
	_, err := pool.Exec(ctx,
		`INSERT INTO audit_events (id, type, actor_id, target, session_id, command_id, decision, reason, details, timestamp)
		 VALUES (gen_random_uuid()::text, $1, 'system', $2, '', '', 'allow', $3, '{}'::jsonb, NOW())`,
		tmpl.eventType, tmpl.target, tmpl.reason,
	)
	if err != nil {
		log.Printf("demo: insertActivity: %v", err)
	}
}

// cycleAlerts resolves one firing alert and re-fires one resolved alert.
func cycleAlerts(ctx context.Context, pool *pgxpool.Pool) {
	// Resolve one random firing alert.
	_, err := pool.Exec(ctx,
		`UPDATE alert_instances SET status = 'resolved', resolved_at = NOW(), updated_at = NOW()
		 WHERE id = (SELECT id FROM alert_instances WHERE status = 'firing' ORDER BY random() LIMIT 1)`,
	)
	if err != nil {
		log.Printf("demo: cycleAlerts resolve: %v", err)
	}

	// Re-fire one random resolved alert.
	_, err = pool.Exec(ctx,
		`UPDATE alert_instances SET status = 'firing', resolved_at = NULL, last_fired_at = NOW(), updated_at = NOW()
		 WHERE id = (SELECT id FROM alert_instances WHERE status = 'resolved' ORDER BY random() LIMIT 1)`,
	)
	if err != nil {
		log.Printf("demo: cycleAlerts re-fire: %v", err)
	}
}

// GenerateCPU produces a realistic CPU percentage for the given asset using
// a sinusoidal wave with per-asset base/amplitude and random noise.
// Exported for testing.
func GenerateCPU(assetID string, t time.Time) float64 {
	p, ok := profiles[assetID]
	if !ok {
		p = defaultProfile
	}
	// 600-second period sinusoidal wave.
	phase := 2 * math.Pi * float64(t.Unix()) / 600.0
	noise := rand.Float64()*20 - 10 // rand(-10,10)
	v := p.cpuBase + p.cpuAmplitude*math.Sin(phase) + noise
	return clamp(v, 5, 85)
}

func generateMemory(assetID string) float64 {
	p, ok := profiles[assetID]
	if !ok {
		p = defaultProfile
	}
	noise := rand.Float64()*6 - 3 // rand(-3,3)
	return clamp(p.memBase+noise, 30, 90)
}

func generateDisk(assetID string) float64 {
	p, ok := profiles[assetID]
	if !ok {
		p = defaultProfile
	}
	noise := rand.Float64()*2 - 1 // rand(-1,1)
	return clamp(p.diskBase+noise, 10, 95)
}

func generateNetwork() float64 {
	if rand.Float64() < 0.85 {
		return 1 + rand.Float64()*29 // 1-30 Mbps
	}
	return 50 + rand.Float64()*450 // 50-500 Mbps
}

// clamp restricts v to the range [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// randomDuration returns a random duration in [min, max].
func randomDuration(min, max time.Duration) time.Duration {
	if min >= max {
		return min
	}
	return min + time.Duration(rand.Int64N(int64(max-min+1)))
}

// OnlineAssetIDs returns a copy of the online asset IDs slice for testing.
func OnlineAssetIDs() []string {
	out := make([]string, len(onlineAssetIDs))
	copy(out, onlineAssetIDs)
	return out
}
