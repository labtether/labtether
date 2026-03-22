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
