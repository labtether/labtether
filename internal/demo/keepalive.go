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
)

// metricHistoryDays is how much metric history the demo keeps. It matches the
// longest window the console offers and the history the seed loads.
const metricHistoryDays = 30

// RunKeepalive trims the demo's metric history every five minutes. It blocks
// until ctx is cancelled.
func RunKeepalive(ctx context.Context, pool *pgxpool.Pool) {
	log.Println("demo: keepalive started")
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	pruneMetrics(ctx, pool)
	for {
		select {
		case <-ctx.Done():
			log.Println("demo: keepalive stopped")
			return
		case <-ticker.C:
			pruneMetrics(ctx, pool)
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
