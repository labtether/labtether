package persistence

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/labtether/labtether/internal/telemetry/remotewrite"
)

// The production row lock, byte accounting and atomic cursor write require real
// PostgreSQL. Never run this in an unowned database or infer proof from a skip.
func TestPostgresRemoteWriteRejectedPageDisposition(t *testing.T) {
	requireOwnedRecoveryDatabase(t)
	s := newTestPostgresStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var occupied int
	if err := s.pool.QueryRow(ctx, `SELECT (SELECT COUNT(*) FROM prometheus_remote_write_state)+
	 (SELECT COUNT(*) FROM prometheus_remote_write_rejections)`).Scan(&occupied); err != nil || occupied != 0 {
		t.Fatalf("rejection fixture requires empty owned tables: %d/%v", occupied, err)
	}
	fingerprint := fmt.Sprintf("%x", sha256.Sum256([]byte("reject-fixture:"+os.Getenv("LABTETHER_JOB_ID"))))
	other := fmt.Sprintf("%x", sha256.Sum256([]byte("other-reject-fixture:"+os.Getenv("LABTETHER_JOB_ID"))))
	owners := []string{fingerprint, other}
	constraintInstalled := false
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if constraintInstalled {
			if _, err := s.pool.Exec(cleanupCtx, `ALTER TABLE prometheus_remote_write_state DROP CONSTRAINT ltqa_rejection_rollback`); err != nil {
				t.Error(err)
			}
		}
		if _, err := s.pool.Exec(cleanupCtx, `DELETE FROM prometheus_remote_write_rejections WHERE endpoint_fingerprint=ANY($1::text[])`, owners); err != nil {
			t.Error(err)
		}
		if _, err := s.pool.Exec(cleanupCtx, `DELETE FROM prometheus_remote_write_state WHERE endpoint_fingerprint=ANY($1::text[])`, owners); err != nil {
			t.Error(err)
		}
		var remains int
		if err := s.pool.QueryRow(cleanupCtx, `SELECT (SELECT COUNT(*) FROM prometheus_remote_write_state WHERE endpoint_fingerprint=ANY($1::text[]))+
		 (SELECT COUNT(*) FROM prometheus_remote_write_rejections WHERE endpoint_fingerprint=ANY($1::text[]))`, owners).Scan(&remains); err != nil || remains != 0 {
			t.Errorf("owned rejection cleanup incomplete: %d/%v", remains, err)
		}
	})
	if _, err := s.LoadRemoteWriteCursor(ctx, fingerprint); err != nil {
		t.Fatal(err)
	}
	body, err := remotewrite.SerializeWriteRequest([]remotewrite.SampleWithLabels{{
		Labels: map[string]string{"__name__": "temperature"}, Value: 3, Timestamp: 1000,
	}})
	if err != nil {
		t.Fatal(err)
	}
	page := remotewrite.RejectedPage{Previous: remotewrite.Cursor{}, Next: remotewrite.Cursor{AssetSampleID: 1},
		StatusCode: 400, SampleCount: 1, Payload: body, RejectedAt: time.Now().UTC().Truncate(time.Microsecond)}
	assertState := func(wantCursor int64, wantRows int) {
		t.Helper()
		var cursor int64
		var rows int
		if e := s.pool.QueryRow(ctx, `SELECT asset_sample_id,(SELECT COUNT(*) FROM prometheus_remote_write_rejections)
		 FROM prometheus_remote_write_state WHERE singleton=TRUE`).Scan(&cursor, &rows); e != nil || cursor != wantCursor || rows != wantRows {
			t.Fatalf("partial disposition: cursor=%d rows=%d want=%d/%d err=%v", cursor, rows, wantCursor, wantRows, e)
		}
	}
	// Force an actual SQL failure after the INSERT but before cursor completion.
	if _, err = s.pool.Exec(ctx, `ALTER TABLE prometheus_remote_write_state ADD CONSTRAINT ltqa_rejection_rollback CHECK(asset_sample_id=0) NOT VALID`); err != nil {
		t.Fatal(err)
	}
	constraintInstalled = true
	if err = s.RejectRemoteWritePage(ctx, fingerprint, page); err == nil {
		t.Fatal("expected injected cursor write failure")
	}
	assertState(0, 0)
	if _, err = s.pool.Exec(ctx, `ALTER TABLE prometheus_remote_write_state DROP CONSTRAINT ltqa_rejection_rollback`); err != nil {
		t.Fatal(err)
	}
	constraintInstalled = false
	if err = s.RejectRemoteWritePage(ctx, fingerprint, page); err != nil {
		t.Fatal(err)
	}
	assertState(1, 1)
	var stored []byte
	var code int
	if err = s.pool.QueryRow(ctx, `SELECT payload,status_code FROM prometheus_remote_write_rejections WHERE endpoint_fingerprint=$1`, fingerprint).Scan(&stored, &code); err != nil || code != 400 || !bytes.Equal(stored, body) {
		t.Fatalf("retained payload/status differs: status=%d error=%v", code, err)
	}
	// Reconstruct a caller after commit/reply loss. A later accepted cursor is
	// also safe; only the exact already-retained rejection may be acknowledged.
	if err = s.RejectRemoteWritePage(ctx, fingerprint, page); err != nil {
		t.Fatal(err)
	}
	altered := page
	altered.Payload = append(bytes.Clone(body), 0)
	if err = s.RejectRemoteWritePage(ctx, fingerprint, altered); !errors.Is(err, remotewrite.ErrRejectedOwnerChanged) {
		t.Fatalf("changed payload accepted: %v", err)
	}
	altered = page
	altered.Previous.HubSampleID = 1
	altered.Next.HubSampleID = 1
	if err = s.RejectRemoteWritePage(ctx, fingerprint, altered); !errors.Is(err, remotewrite.ErrRejectedOwnerChanged) {
		t.Fatalf("changed cursor accepted: %v", err)
	}
	if err = s.RejectRemoteWritePage(ctx, other, page); !errors.Is(err, remotewrite.ErrRejectedOwnerChanged) {
		t.Fatalf("changed endpoint accepted: %v", err)
	}
	assertState(1, 1)

	next := page
	next.Previous, next.Next = page.Next, remotewrite.Cursor{AssetSampleID: 2}
	seedCapacity := func(count, bodyBytes int) {
		t.Helper()
		_, e := s.pool.Exec(ctx, `INSERT INTO prometheus_remote_write_rejections
		 (endpoint_fingerprint,previous_asset_id,previous_hub_id,next_asset_id,next_hub_id,status_code,sample_count,payload,rejected_at)
		 SELECT $1,1000+n,0,1001+n,0,400,1,decode(repeat('78',$3),'hex'),$4 FROM generate_series(1,$2) n`, fingerprint, count, bodyBytes, page.RejectedAt)
		if e != nil {
			t.Fatal(e)
		}
	}
	clearCapacity := func() {
		t.Helper()
		if _, e := s.pool.Exec(ctx, `DELETE FROM prometheus_remote_write_rejections WHERE endpoint_fingerprint=$1 AND previous_asset_id BETWEEN 1001 AND 1063 AND rejected_at=$2`, fingerprint, page.RejectedAt); e != nil {
			t.Fatal(e)
		}
	}
	seedCapacity(remotewrite.MaxRejectedPages-1, 1)
	if err = s.RejectRemoteWritePage(ctx, fingerprint, next); !errors.Is(err, remotewrite.ErrRejectedStoreFull) {
		t.Fatalf("row limit not enforced: %v", err)
	}
	assertState(1, remotewrite.MaxRejectedPages)
	clearCapacity()
	seedCapacity(16, remotewrite.MaxRequestBodyBytes)
	if _, err = s.pool.Exec(ctx, `UPDATE prometheus_remote_write_rejections SET payload=substring(payload FROM 1 FOR $2)
	 WHERE endpoint_fingerprint=$1 AND previous_asset_id=1016`, fingerprint, remotewrite.MaxRequestBodyBytes-len(body)); err != nil {
		t.Fatal(err)
	}
	if err = s.RejectRemoteWritePage(ctx, fingerprint, next); !errors.Is(err, remotewrite.ErrRejectedStoreFull) {
		t.Fatalf("byte limit not enforced: %v", err)
	}
	assertState(1, 17)
	clearCapacity()
	oversize := next
	oversize.Payload = make([]byte, remotewrite.MaxRequestBodyBytes+1)
	if err = s.RejectRemoteWritePage(ctx, fingerprint, oversize); err == nil {
		t.Fatal("oversize retained payload accepted")
	}
	assertState(1, 1)
	if err = s.RejectRemoteWritePage(ctx, fingerprint, next); err != nil {
		t.Fatal(err)
	}
	assertState(2, 2)
	if err = s.RejectRemoteWritePage(ctx, fingerprint, page); err != nil {
		t.Fatalf("exact older committed page not reconciled: %v", err)
	}
	loaded, err := s.LoadRemoteWriteCursor(ctx, fingerprint)
	if err != nil || loaded != next.Next {
		t.Fatalf("restarted replay position=%+v/%v", loaded, err)
	}
	if _, err = s.LoadRemoteWriteCursor(ctx, other); err != nil {
		t.Fatal(err)
	}
	if err = s.RejectRemoteWritePage(ctx, fingerprint, next); !errors.Is(err, remotewrite.ErrRejectedOwnerChanged) {
		t.Fatalf("stale endpoint disposition accepted: %v", err)
	}
}
