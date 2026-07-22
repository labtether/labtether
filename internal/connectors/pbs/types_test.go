package pbs

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestBackupSnapshotUnmarshalAcceptsObjectFileEntries(t *testing.T) {
	var snapshot BackupSnapshot
	payload := []byte(`{
		"backup-type":"vm",
		"backup-id":"100",
		"backup-time":1711111111,
		"files":[
			{"filename":"index.json.blob"},
			{"path":"vm/100/2024-03-22T00:00:00Z"},
			{"volid":"drive-scsi0"}
		]
	}`)

	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	want := []string{"index.json.blob", "vm/100/2024-03-22T00:00:00Z", "drive-scsi0"}
	if !reflect.DeepEqual(snapshot.Files, want) {
		t.Fatalf("unexpected files: got %#v want %#v", snapshot.Files, want)
	}
}

func TestBackupSnapshotUnmarshalAcceptsStringFiles(t *testing.T) {
	var snapshot BackupSnapshot
	payload := []byte(`{
		"backup-type":"vm",
		"backup-id":"100",
		"backup-time":1711111111,
		"files":["index.json.blob","drive-scsi0"]
	}`)

	if err := json.Unmarshal(payload, &snapshot); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	want := []string{"index.json.blob", "drive-scsi0"}
	if !reflect.DeepEqual(snapshot.Files, want) {
		t.Fatalf("unexpected files: got %#v want %#v", snapshot.Files, want)
	}
}

func TestPBSJobTypesPreserveSafetyAndRetentionFields(t *testing.T) {
	var verify VerifyJob
	if err := json.Unmarshal([]byte(`{"id":"weekly","store":"backup","ignore-verified":true,"outdated-after":30}`), &verify); err != nil {
		t.Fatalf("unmarshal verify job: %v", err)
	}
	if !verify.IgnoreVerified || verify.OutdatedAfter == nil || *verify.OutdatedAfter != 30 {
		t.Fatalf("verify safety fields lost: %+v", verify)
	}

	var syncJob SyncJob
	if err := json.Unmarshal([]byte(`{"id":"offsite","store":"backup","remote-store":"remote","verified-only":true,"remove-vanished":false,"transfer-last":3}`), &syncJob); err != nil {
		t.Fatalf("unmarshal sync job: %v", err)
	}
	if !syncJob.VerifiedOnly || syncJob.TransferLast == nil || *syncJob.TransferLast != 3 {
		t.Fatalf("sync safety fields lost: %+v", syncJob)
	}
}
