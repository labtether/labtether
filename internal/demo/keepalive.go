// Package demo runs the public demo: a pretend home lab whose machines send
// the hub the same messages real agents send, plus the upkeep that keeps a
// long-running demo database small.
package demo

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/schedules"
)

// metricHistoryDays is how much metric history the demo keeps. It matches the
// longest window the console offers and the history the seed loads.
const metricHistoryDays = 30

// RunKeepalive trims the demo's metric history and moves its schedules on
// every five minutes. It blocks until ctx is cancelled.
func RunKeepalive(ctx context.Context, pool *pgxpool.Pool) {
	log.Println("demo: keepalive started")
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	upkeep := func() {
		pruneMetrics(ctx, pool)
		rollSchedules(ctx, pool, time.Now())
	}
	upkeep()
	for {
		select {
		case <-ctx.Done():
			log.Println("demo: keepalive stopped")
			return
		case <-ticker.C:
			upkeep()
		}
	}
}

// pruneMetrics keeps the metric table at a steady size. Samples older than
// metricHistoryDays go. Samples older than six hours are thinned to the one
// that falls in the first 30 seconds of each 15 minutes, which is the spacing
// of the seeded history, so a demo that has run for a month holds about as
// many rows as a fresh one.
func pruneMetrics(ctx context.Context, pool *pgxpool.Pool) {
	_, err := pool.Exec(ctx,
		`DELETE FROM metric_samples WHERE collected_at < NOW() - $1::interval`,
		fmt.Sprintf("%d days", metricHistoryDays),
	)
	if err != nil {
		log.Printf("demo: prune metrics: %v", err)
	}
	_, err = pool.Exec(ctx,
		`DELETE FROM metric_samples
		 WHERE collected_at < NOW() - INTERVAL '6 hours'
		   AND collected_at > NOW() - INTERVAL '12 hours'
		   AND (EXTRACT(EPOCH FROM collected_at)::bigint % 900) >= 30`,
	)
	if err != nil {
		log.Printf("demo: thin metrics: %v", err)
	}
}

// rollSchedules keeps the Schedules page current. Demo mode does not run
// schedules, so a seeded "next run" would soon be in the past. Each enabled
// schedule whose time has come is moved to its next cron slot, and the slot
// that passed becomes its last run.
func rollSchedules(ctx context.Context, pool *pgxpool.Pool, now time.Time) {
	rows, err := pool.Query(ctx,
		`SELECT id, cron_expr, next_run_at FROM scheduled_tasks
		 WHERE enabled AND next_run_at IS NOT NULL AND next_run_at <= $1`, now)
	if err != nil {
		log.Printf("demo: roll schedules: %v", err)
		return
	}
	type due struct {
		id   string
		expr string
		at   time.Time
	}
	var passed []due
	for rows.Next() {
		var d due
		if err := rows.Scan(&d.id, &d.expr, &d.at); err == nil {
			passed = append(passed, d)
		}
	}
	rows.Close()

	for _, d := range passed {
		// The hub's own cron rules, so the time matches what a real run would set.
		next, err := schedules.NextRun(d.expr, now)
		if err != nil {
			continue
		}
		if _, err := pool.Exec(ctx,
			`UPDATE scheduled_tasks SET last_run_at = $2, next_run_at = $3 WHERE id = $1`,
			d.id, d.at, next,
		); err != nil {
			log.Printf("demo: roll schedule %s: %v", d.id, err)
		}
	}
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
