// Package demo provides background processes for keeping demo instances alive
// with realistic, continuously refreshed data.
package demo

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"log"
	"math"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// assetProfile defines the per-asset metric generation parameters.
type assetProfile struct {
	cpuBase      float64
	cpuAmplitude float64
	memBase      float64
	diskBase     float64
}

var defaultProfile = assetProfile{cpuBase: 30, cpuAmplitude: 12, memBase: 55, diskBase: 50}

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

// refreshAssets updates last_seen_at for all online/degraded assets and keeps
// offline assets stale. Discovers assets from the DB rather than hardcoding.
func refreshAssets(ctx context.Context, pool *pgxpool.Pool) {
	// Refresh all online/degraded assets.
	rows, err := pool.Query(ctx,
		`SELECT id FROM assets WHERE status IN ('online', 'degraded')`,
	)
	if err != nil {
		log.Printf("demo: refreshAssets query: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		offset := 5 + randomIntN(55)
		_, err := pool.Exec(ctx,
			`UPDATE assets SET last_seen_at = NOW() - ($1 || ' seconds')::interval, updated_at = NOW() WHERE id = $2`,
			fmt.Sprintf("%d", offset), id,
		)
		if err != nil {
			log.Printf("demo: refreshAssets %s: %v", id, err)
		}
	}

	// Keep offline assets stale — bump last_seen_at to maintain a consistent
	// "offline for N hours" appearance without it drifting to days.
	_, err = pool.Exec(ctx,
		`UPDATE assets SET last_seen_at = NOW() - INTERVAL '3 hours', updated_at = NOW() WHERE status = 'offline'`,
	)
	if err != nil {
		log.Printf("demo: refreshAssets offline: %v", err)
	}
}

// insertMetrics inserts 4 metric rows (cpu, mem, disk, network) for each
// online/degraded asset. Discovers assets from the DB.
func insertMetrics(ctx context.Context, pool *pgxpool.Pool) {
	now := time.Now()
	rows, err := pool.Query(ctx,
		`SELECT id FROM assets WHERE status IN ('online', 'degraded')`,
	)
	if err != nil {
		log.Printf("demo: insertMetrics query: %v", err)
		return
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	for _, id := range ids {
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

// pruneOldMetrics removes metric samples older than 7 days.
func pruneOldMetrics(ctx context.Context, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx,
		`DELETE FROM metric_samples WHERE collected_at < NOW() - INTERVAL '7 days'`,
	)
	if err != nil {
		log.Printf("demo: pruneOldMetrics: %v", err)
	}
}

// insertActivity inserts a synthetic audit event using a random online asset.
func insertActivity(ctx context.Context, pool *pgxpool.Pool) {
	templates := []struct {
		eventType string
		reason    string
	}{
		{"health_check", "periodic health check completed"},
		{"container_sync", "container inventory synchronized"},
		{"dns_refresh", "DNS blocklist statistics refreshed"},
		{"automation_run", "automation engine cycle completed"},
		{"backup_verify", "backup integrity verification passed"},
		{"media_scan", "media library scan completed"},
		{"cluster_reconcile", "cluster state reconciled"},
		{"metric_collection", "metric collection cycle completed"},
		{"pool_refresh", "storage pool status refreshed"},
		{"network_scan", "network device discovery sweep completed"},
		{"certificate_check", "TLS certificate expiry check passed"},
		{"snapshot_cleanup", "old snapshots pruned"},
	}

	// Pick a random online asset as target.
	var target string
	err := pool.QueryRow(ctx,
		`SELECT id FROM assets WHERE status IN ('online', 'degraded') ORDER BY random() LIMIT 1`,
	).Scan(&target)
	if err != nil {
		log.Printf("demo: insertActivity target: %v", err)
		return
	}

	tmpl := templates[randomIntN(len(templates))]
	_, err = pool.Exec(ctx,
		`INSERT INTO audit_events (id, type, actor_id, target, session_id, command_id, decision, reason, details, timestamp)
		 VALUES (gen_random_uuid()::text, $1, 'system', $2, '', '', 'allow', $3, '{}'::jsonb, NOW())`,
		tmpl.eventType, target, tmpl.reason,
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
	p := profileFor(assetID)
	// 600-second period sinusoidal wave.
	phase := 2 * math.Pi * float64(t.Unix()) / 600.0
	noise := randomFloat64()*20 - 10
	v := p.cpuBase + p.cpuAmplitude*math.Sin(phase) + noise
	return clamp(v, 5, 85)
}

func generateMemory(assetID string) float64 {
	p := profileFor(assetID)
	noise := randomFloat64()*6 - 3
	return clamp(p.memBase+noise, 30, 90)
}

func generateDisk(assetID string) float64 {
	p := profileFor(assetID)
	noise := randomFloat64()*2 - 1
	return clamp(p.diskBase+noise, 10, 95)
}

func generateNetwork() float64 {
	if randomFloat64() < 0.85 {
		return 1 + randomFloat64()*29
	}
	return 50 + randomFloat64()*450
}

func profileFor(assetID string) assetProfile {
	if p, ok := profiles[assetID]; ok {
		return p
	}
	return defaultProfile
}

// profiles maps known asset IDs to metric generation parameters.
// Assets not in this map get the default profile.
var profiles = map[string]assetProfile{
	"asset-pve1":     {cpuBase: 35, cpuAmplitude: 12, memBase: 72, diskBase: 52},
	"asset-pve2":     {cpuBase: 28, cpuAmplitude: 10, memBase: 58, diskBase: 44},
	"asset-truenas":  {cpuBase: 25, cpuAmplitude: 10, memBase: 68, diskBase: 91},
	"asset-pbs":      {cpuBase: 10, cpuAmplitude: 8, memBase: 42, diskBase: 65},
	"asset-opnsense": {cpuBase: 8, cpuAmplitude: 4, memBase: 32, diskBase: 18},
	"asset-pihole":   {cpuBase: 6, cpuAmplitude: 3, memBase: 28, diskBase: 15},
	"asset-unifi":    {cpuBase: 12, cpuAmplitude: 5, memBase: 45, diskBase: 22},
	"asset-docker":   {cpuBase: 22, cpuAmplitude: 10, memBase: 62, diskBase: 48},
	"asset-k3s-m":    {cpuBase: 30, cpuAmplitude: 8, memBase: 58, diskBase: 40},
	"asset-k3s-w1":   {cpuBase: 35, cpuAmplitude: 10, memBase: 65, diskBase: 38},
	"asset-hass":     {cpuBase: 14, cpuAmplitude: 6, memBase: 48, diskBase: 35},
	"asset-media":    {cpuBase: 20, cpuAmplitude: 15, memBase: 78, diskBase: 55},
	"asset-mon":      {cpuBase: 18, cpuAmplitude: 5, memBase: 55, diskBase: 32},
	"asset-gitlab":   {cpuBase: 15, cpuAmplitude: 20, memBase: 40, diskBase: 52},
	"asset-mc":       {cpuBase: 8, cpuAmplitude: 15, memBase: 62, diskBase: 35},
	"asset-offsite":  {cpuBase: 5, cpuAmplitude: 3, memBase: 30, diskBase: 42},
	"asset-htpc":     {cpuBase: 5, cpuAmplitude: 10, memBase: 45, diskBase: 60},
}

// OnlineAssetIDs returns a copy of the online asset IDs slice for testing.
// Deprecated: keepalive now discovers assets from the database.
func OnlineAssetIDs() []string {
	ids := make([]string, 0, len(profiles))
	for id := range profiles {
		ids = append(ids, id)
	}
	return ids
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
	return min + time.Duration(randomInt64N(int64(max-min+1)))
}

// randomInt64N returns a crypto-backed integer in [0, n). Demo generation is
// not a security decision, but using the process CSPRNG keeps this helper safe
// if its output is ever reused in a security-sensitive context. OS entropy
// failure is unrecoverable here, so fail closed instead of substituting a weak
// or predictable source.
func randomInt64N(n int64) int64 {
	if n <= 1 {
		return 0
	}
	value, err := cryptorand.Int(cryptorand.Reader, big.NewInt(n))
	if err != nil {
		panic(fmt.Sprintf("demo: secure random source failed: %v", err))
	}
	return value.Int64()
}

func randomIntN(n int) int {
	if n <= 1 {
		return 0
	}
	return int(randomInt64N(int64(n)))
}

func randomFloat64() float64 {
	const floatPrecision = int64(1 << 53)
	return float64(randomInt64N(floatPrecision)) / float64(floatPrecision)
}
