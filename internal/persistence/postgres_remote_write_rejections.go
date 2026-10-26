package persistence

import (
	"context"
	"fmt"

	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

// RejectRemoteWritePage stores the exact rejected request and advances replay
// together. All writers serialize on the existing singleton cursor row, including
// endpoint changes. No rejected data is evicted automatically to make space.
func (s *PostgresStore) RejectRemoteWritePage(ctx context.Context, fingerprint string, page remotewrite.RejectedPage) error {
	if s == nil || s.pool == nil || ctx == nil || !remoteWriteFingerprintPattern.MatchString(fingerprint) {
		return fmt.Errorf("remote write rejection persistence is unavailable or invalid")
	}
	if err := page.Validate(); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("remote write rejection transaction unavailable")
	}
	defer tx.Rollback(ctx)
	var owner string
	var current remotewrite.Cursor
	err = tx.QueryRow(ctx, `SELECT endpoint_fingerprint,asset_sample_id,hub_sample_id
	 FROM prometheus_remote_write_state WHERE singleton=TRUE FOR UPDATE`).Scan(&owner, &current.AssetSampleID, &current.HubSampleID)
	if err != nil {
		return fmt.Errorf("remote write rejection cursor lock failed")
	}
	if owner != fingerprint {
		return remotewrite.ErrRejectedOwnerChanged
	}
	if current != page.Previous {
		// The previous COMMIT may have succeeded even if its response was lost.
		// Only an exact retained disposition can acknowledge that retry. Matching
		// a cursor alone could falsely accept a different rejected payload.
		var exact bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM prometheus_remote_write_rejections
		 WHERE endpoint_fingerprint=$1 AND previous_asset_id=$2 AND previous_hub_id=$3
		 AND next_asset_id=$4 AND next_hub_id=$5 AND status_code=$6 AND sample_count=$7
		 AND payload=$8 AND rejected_at=$9)`, fingerprint,
			page.Previous.AssetSampleID, page.Previous.HubSampleID, page.Next.AssetSampleID, page.Next.HubSampleID,
			page.StatusCode, page.SampleCount, page.Payload, page.RejectedAt.UTC()).Scan(&exact)
		if err != nil {
			return fmt.Errorf("remote write rejection reconciliation failed")
		}
		if !exact || current.AssetSampleID < page.Next.AssetSampleID || current.HubSampleID < page.Next.HubSampleID {
			return remotewrite.ErrRejectedOwnerChanged
		}
		return nil
	}
	var count, bytes int64
	err = tx.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(octet_length(payload)),0)
	 FROM prometheus_remote_write_rejections`).Scan(&count, &bytes)
	if err != nil {
		return fmt.Errorf("remote write rejection capacity read failed")
	}
	if count >= remotewrite.MaxRejectedPages || bytes+int64(len(page.Payload)) > remotewrite.MaxRejectedPayloadBytes {
		return remotewrite.ErrRejectedStoreFull
	}
	_, err = tx.Exec(ctx, `INSERT INTO prometheus_remote_write_rejections
	 (endpoint_fingerprint,previous_asset_id,previous_hub_id,next_asset_id,next_hub_id,status_code,sample_count,payload,rejected_at)
	 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, fingerprint,
		page.Previous.AssetSampleID, page.Previous.HubSampleID, page.Next.AssetSampleID, page.Next.HubSampleID,
		page.StatusCode, page.SampleCount, page.Payload, page.RejectedAt.UTC())
	if err != nil {
		return fmt.Errorf("remote write rejection insert failed")
	}
	updated, err := tx.Exec(ctx, `UPDATE prometheus_remote_write_state
	 SET asset_sample_id=$1,hub_sample_id=$2,last_advanced_at=$3,updated_at=NOW() WHERE singleton=TRUE`,
		page.Next.AssetSampleID, page.Next.HubSampleID, page.RejectedAt.UTC())
	if err != nil {
		return fmt.Errorf("remote write rejected cursor update failed")
	}
	if updated.RowsAffected() != 1 {
		return fmt.Errorf("remote write rejected cursor was not advanced")
	}
	if err = tx.Commit(ctx); err != nil {
		// Do not assert rollback: the caller retries this exact disposition,
		// which the locked state and retained payload can reconcile next time.
		return fmt.Errorf("remote write rejection commit was not confirmed")
	}
	return nil
}
