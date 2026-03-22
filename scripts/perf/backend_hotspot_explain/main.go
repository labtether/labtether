package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/labtether/labtether/internal/telemetry"
)

type explainScenario string

const (
	scenarioProjectedGroup  explainScenario = "projected-group"
	scenarioSourcesWindowed explainScenario = "sources-windowed"
	scenarioSnapshotMany    explainScenario = "snapshotmany-lateral"
)

type explainResult struct {
	CapturedAt string                 `json:"captured_at"`
	Scenario   string                 `json:"scenario"`
	Database   string                 `json:"database"`
	Options    []string               `json:"options"`
	Query      string                 `json:"query"`
	Args       []any                  `json:"args"`
	Summary    map[string]any         `json:"summary,omitempty"`
	Plan       any                    `json:"plan,omitempty"`
	Lines      []string               `json:"lines,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Meta       map[string]interface{} `json:"meta,omitempty"`
}

func main() {
	log.SetFlags(0)

	defaultDSN := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if defaultDSN == "" {
		defaultDSN = "postgres://labtether:labtether@localhost:5432/labtether?sslmode=disable" // #nosec G101 -- local profiling fallback DSN for developer environments.
	}

	var (
		dsn           = flag.String("dsn", defaultDSN, "Postgres DSN (defaults to DATABASE_URL)")
		scenario      = flag.String("scenario", string(scenarioProjectedGroup), "Scenario: projected-group|sources-windowed|snapshotmany-lateral")
		output        = flag.String("output", "", "Optional output path (defaults to stdout)")
		format        = flag.String("format", "json", "Output format: json|text")
		window        = flag.Duration("window", 24*time.Hour, "Lookback window for time-bounded scenarios")
		limit         = flag.Int("limit", 50, "Limit for log/source scenarios")
		groupID       = flag.String("group-id", "smoke-group", "Group ID used for projected-group scenario")
		groupAssetIDs = flag.String("group-asset-ids", "smoke-group-node-01,smoke-node-01", "Comma-separated group asset IDs for projected-group scenario")
		assetCount    = flag.Int("asset-count", 64, "Asset ID count for snapshotmany-lateral scenario")
		timeout       = flag.Duration("timeout", 45*time.Second, "Execution timeout")
		analyze       = flag.Bool("analyze", true, "Include ANALYZE in EXPLAIN")
		buffers       = flag.Bool("buffers", true, "Include BUFFERS in EXPLAIN (requires --analyze=true)")
	)
	flag.Parse()

	if *window <= 0 {
		fatal("--window must be > 0")
	}
	if *limit <= 0 {
		fatal("--limit must be > 0")
	}
	if *assetCount <= 0 {
		fatal("--asset-count must be > 0")
	}

	formatValue := strings.ToLower(strings.TrimSpace(*format))
	if formatValue != "json" && formatValue != "text" {
		fatal("--format must be json or text")
	}
	if !*analyze && *buffers {
		fatal("--buffers requires --analyze=true")
	}

	scenarioValue := explainScenario(strings.TrimSpace(strings.ToLower(*scenario)))
	if !validScenario(scenarioValue) {
		fatal("unsupported --scenario; use projected-group|sources-windowed|snapshotmany-lateral")
	}

	now := time.Now().UTC()
	from := now.Add(-*window)
	to := now

	query, args, meta, err := buildScenarioQuery(scenarioValue, from, to, *limit, strings.TrimSpace(*groupID), splitCSV(*groupAssetIDs), *assetCount)
	if err != nil {
		fatal(err.Error())
	}

	explainOptions := buildExplainOptions(*analyze, *buffers, formatValue)
	explainSQL := fmt.Sprintf("EXPLAIN (%s) %s", strings.Join(explainOptions, ", "), query)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, *dsn)
	if err != nil {
		fatal(fmt.Sprintf("connect failed: %v", err))
	}
	defer pool.Close()

	result := explainResult{
		CapturedAt: now.Format(time.RFC3339),
		Scenario:   string(scenarioValue),
		Database:   redactDSN(*dsn),
		Options:    explainOptions,
		Query:      query,
		Args:       renderArgs(args),
		Meta:       meta,
	}

	if formatValue == "json" {
		result.Plan, result.Summary, err = runExplainJSON(ctx, pool, explainSQL, args)
	} else {
		result.Lines, err = runExplainText(ctx, pool, explainSQL, args)
	}
	if err != nil {
		result.Error = err.Error()
		emit(result, *output)
		os.Exit(1)
	}

	emit(result, *output)
}

func runExplainJSON(ctx context.Context, pool *pgxpool.Pool, sql string, args []any) (any, map[string]any, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		if rows.Err() != nil {
			return nil, nil, rows.Err()
		}
		return nil, nil, errors.New("no EXPLAIN output rows")
	}

	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return nil, nil, err
	}

	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, nil, err
	}

	summary := map[string]any{}
	if plans, ok := parsed.([]any); ok && len(plans) > 0 {
		if first, ok := plans[0].(map[string]any); ok {
			for _, key := range []string{"Planning Time", "Execution Time", "Triggers"} {
				if value, exists := first[key]; exists {
					summary[strings.ToLower(strings.ReplaceAll(key, " ", "_"))] = value
				}
			}
			if root, exists := first["Plan"]; exists {
				if node, ok := root.(map[string]any); ok {
					summary["root_node_type"] = node["Node Type"]
					summary["root_relation"] = node["Relation Name"]
					summary["root_index"] = node["Index Name"]
				}
			}
		}
	}

	return parsed, summary, nil
}

func runExplainText(ctx context.Context, pool *pgxpool.Pool, sql string, args []any) ([]string, error) {
	rows, err := pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, 16)
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return nil, err
		}
		out = append(out, line)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	if len(out) == 0 {
		return nil, errors.New("no EXPLAIN output rows")
	}
	return out, nil
}

func buildScenarioQuery(scenario explainScenario, from, to time.Time, limit int, groupID string, groupAssetIDs []string, assetCount int) (string, []any, map[string]interface{}, error) {
	switch scenario {
	case scenarioProjectedGroup:
		if groupID == "" {
			groupID = "smoke-group"
		}
		if len(groupAssetIDs) == 0 {
			groupAssetIDs = []string{"smoke-group-node-01", "smoke-node-01"}
		}
		query := `SELECT id, asset_id, source, level, message,
			NULL::jsonb AS fields,
			CASE WHEN asset_id IS NULL THEN COALESCE(NULLIF(BTRIM(fields->>'group_id'), ''), '') ELSE '' END AS projected_group_id,
			timestamp
		FROM log_events
		WHERE timestamp >= $1
		  AND timestamp <= $2
		  AND (asset_id = ANY($3::text[]) OR NULLIF(BTRIM(fields->>'group_id'), '') = $4)
		ORDER BY timestamp DESC
		LIMIT $5`
		meta := map[string]interface{}{
			"from":            from.Format(time.RFC3339),
			"to":              to.Format(time.RFC3339),
			"group_id":        groupID,
			"group_asset_ids": groupAssetIDs,
			"limit":           limit,
		}
		return query, []any{from, to, groupAssetIDs, groupID, limit}, meta, nil
	case scenarioSourcesWindowed:
		query := `SELECT source, COUNT(*) AS event_count, MAX(timestamp) AS last_seen
		FROM log_events
		WHERE timestamp >= $1
		GROUP BY source
		ORDER BY last_seen DESC
		LIMIT $2`
		meta := map[string]interface{}{
			"from":  from.Format(time.RFC3339),
			"limit": limit,
		}
		return query, []any{from, limit}, meta, nil
	case scenarioSnapshotMany:
		assetIDs := make([]string, 0, assetCount)
		for i := 1; i <= assetCount; i++ {
			assetIDs = append(assetIDs, fmt.Sprintf("ci-asset-%03d", i))
		}
		metrics := canonicalMetricNames()
		query := `WITH asset_ids AS (
			SELECT UNNEST($1::text[]) AS asset_id
		),
		metrics AS (
			SELECT UNNEST($2::text[]) AS metric
		)
		SELECT a.asset_id, m.metric, latest.value
		FROM asset_ids a
		CROSS JOIN metrics m
		JOIN LATERAL (
			SELECT value
			FROM metric_samples
			WHERE asset_id = a.asset_id
			  AND metric = m.metric
			  AND collected_at <= $3
			ORDER BY collected_at DESC
			LIMIT 1
		) AS latest ON true`
		meta := map[string]interface{}{
			"asset_count":  len(assetIDs),
			"metric_count": len(metrics),
			"at":           to.Format(time.RFC3339),
		}
		return query, []any{assetIDs, metrics, to}, meta, nil
	default:
		return "", nil, nil, fmt.Errorf("unsupported scenario: %s", scenario)
	}
}

func canonicalMetricNames() []string {
	definitions := telemetry.CanonicalMetrics()
	out := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		metric := strings.TrimSpace(definition.Metric)
		if metric == "" {
			continue
		}
		out = append(out, metric)
	}
	sort.Strings(out)
	return out
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}

func buildExplainOptions(analyze, buffers bool, format string) []string {
	options := make([]string, 0, 4)
	if analyze {
		options = append(options, "ANALYZE")
	}
	if buffers {
		options = append(options, "BUFFERS")
	}
	if format == "json" {
		options = append(options, "FORMAT JSON")
	}
	return options
}

func renderArgs(args []any) []any {
	out := make([]any, 0, len(args))
	for _, arg := range args {
		switch value := arg.(type) {
		case time.Time:
			out = append(out, value.UTC().Format(time.RFC3339))
		default:
			out = append(out, value)
		}
	}
	return out
}

func redactDSN(dsn string) string {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return "redacted"
	}
	if u.User != nil {
		username := u.User.Username()
		u.User = url.User(username)
	}
	return u.String()
}

func validScenario(s explainScenario) bool {
	switch s {
	case scenarioProjectedGroup, scenarioSourcesWindowed, scenarioSnapshotMany:
		return true
	default:
		return false
	}
}

func emit(result explainResult, output string) {
	blob, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(fmt.Sprintf("marshal output failed: %v", err))
	}
	if strings.TrimSpace(output) == "" {
		fmt.Println(string(blob))
		return
	}
	if err := os.WriteFile(output, append(blob, '\n'), 0o600); err != nil {
		fatal(fmt.Sprintf("write output failed: %v", err))
	}
	fmt.Printf("wrote %s\n", output)
}

func fatal(msg string) {
	log.Printf("FAIL: %s", msg)
	os.Exit(1)
}
