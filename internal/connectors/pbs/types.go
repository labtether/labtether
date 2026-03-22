package pbs

import (
	"encoding/json"
	"strings"
)

type Version struct {
	Release string `json:"release"`
	Version string `json:"version"`
	RepoID  string `json:"repoid"`
}

type PingResponse struct {
	Pong bool `json:"pong"`
}

type Datastore struct {
	Store       string `json:"store"`
	Comment     string `json:"comment,omitempty"`
	MountStatus string `json:"mount-status,omitempty"`
	Maintenance string `json:"maintenance,omitempty"`
}

type DatastoreStatus struct {
	Store       string             `json:"store"`
	Total       int64              `json:"total,omitempty"`
	Used        int64              `json:"used,omitempty"`
	Avail       int64              `json:"avail,omitempty"`
	MountStatus string             `json:"mount-status,omitempty"`
	GCStatus    *DatastoreGCStatus `json:"gc-status,omitempty"`
}

type DatastoreGCStatus struct {
	UPID          string `json:"upid,omitempty"`
	RemovedBytes  int64  `json:"removed-bytes,omitempty"`
	PendingBytes  int64  `json:"pending-bytes,omitempty"`
	RemovedChunks int64  `json:"removed-chunks,omitempty"`
	PendingChunks int64  `json:"pending-chunks,omitempty"`
}

type BackupGroup struct {
	BackupType  string   `json:"backup-type"`
	BackupID    string   `json:"backup-id"`
	BackupCount int64    `json:"backup-count,omitempty"`
	LastBackup  int64    `json:"last-backup,omitempty"`
	Comment     string   `json:"comment,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	Files       []string `json:"files,omitempty"`
}

type SnapshotVerification struct {
	State string `json:"state,omitempty"`
	UPID  string `json:"upid,omitempty"`
}

type BackupSnapshot struct {
	BackupType   string                `json:"backup-type"`
	BackupID     string                `json:"backup-id"`
	BackupTime   int64                 `json:"backup-time"`
	Comment      string                `json:"comment,omitempty"`
	Owner        string                `json:"owner,omitempty"`
	Protected    bool                  `json:"protected,omitempty"`
	Size         int64                 `json:"size,omitempty"`
	Files        []string              `json:"files,omitempty"`
	Verification *SnapshotVerification `json:"verification,omitempty"`
}

func (s *BackupSnapshot) UnmarshalJSON(data []byte) error {
	type rawBackupSnapshot struct {
		BackupType   string                `json:"backup-type"`
		BackupID     string                `json:"backup-id"`
		BackupTime   int64                 `json:"backup-time"`
		Comment      string                `json:"comment,omitempty"`
		Owner        string                `json:"owner,omitempty"`
		Protected    bool                  `json:"protected,omitempty"`
		Size         int64                 `json:"size,omitempty"`
		Files        json.RawMessage       `json:"files,omitempty"`
		Verification *SnapshotVerification `json:"verification,omitempty"`
	}

	var raw rawBackupSnapshot
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	files, err := decodePBSFileList(raw.Files)
	if err != nil {
		return err
	}

	*s = BackupSnapshot{
		BackupType:   raw.BackupType,
		BackupID:     raw.BackupID,
		BackupTime:   raw.BackupTime,
		Comment:      raw.Comment,
		Owner:        raw.Owner,
		Protected:    raw.Protected,
		Size:         raw.Size,
		Files:        files,
		Verification: raw.Verification,
	}
	return nil
}

func decodePBSFileList(raw json.RawMessage) ([]string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var stringList []string
	if err := json.Unmarshal(raw, &stringList); err == nil {
		return stringList, nil
	}

	var anyValue any
	if err := json.Unmarshal(raw, &anyValue); err != nil {
		return nil, err
	}

	switch value := anyValue.(type) {
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, nil
		}
		return []string{strings.TrimSpace(value)}, nil
	case []any:
		files := make([]string, 0, len(value))
		for _, item := range value {
			name := extractPBSFileName(item)
			if name == "" {
				continue
			}
			files = append(files, name)
		}
		return files, nil
	default:
		name := extractPBSFileName(value)
		if name == "" {
			return nil, nil
		}
		return []string{name}, nil
	}
}

func extractPBSFileName(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		for _, key := range []string{"filename", "path", "volid", "name", "id"} {
			if raw, ok := typed[key]; ok {
				if name := extractPBSFileName(raw); name != "" {
					return name
				}
			}
		}
	}
	return ""
}

type DatastoreUsage struct {
	Store       string `json:"store"`
	Total       int64  `json:"total,omitempty"`
	Used        int64  `json:"used,omitempty"`
	Avail       int64  `json:"avail,omitempty"`
	MountStatus string `json:"mount-status,omitempty"`
}

type Task struct {
	UPID       string `json:"upid"`
	Node       string `json:"node"`
	WorkerType string `json:"worker_type"`
	WorkerID   string `json:"worker_id,omitempty"`
	User       string `json:"user,omitempty"`
	StartTime  int64  `json:"starttime,omitempty"`
	EndTime    int64  `json:"endtime,omitempty"`
	Status     string `json:"status,omitempty"`
}

type TaskStatus struct {
	UPID       string `json:"upid"`
	Node       string `json:"node"`
	Type       string `json:"type,omitempty"`
	ID         string `json:"id,omitempty"`
	User       string `json:"user,omitempty"`
	Status     string `json:"status,omitempty"`
	ExitStatus string `json:"exitstatus,omitempty"`
	StartTime  int64  `json:"starttime,omitempty"`
	PStart     int64  `json:"pstart,omitempty"`
	PID        int64  `json:"pid,omitempty"`
}

type TaskLogLine struct {
	LineNo int    `json:"n"`
	Text   string `json:"t"`
}

type PruneOptions struct {
	DryRun      bool
	KeepLast    int
	KeepHourly  int
	KeepDaily   int
	KeepWeekly  int
	KeepMonthly int
	KeepYearly  int
}

// VerifyJob represents a PBS scheduled verify job config entry.
type VerifyJob struct {
	ID             string `json:"id"`
	Store          string `json:"store"`
	Schedule       string `json:"schedule,omitempty"`
	Comment        string `json:"comment,omitempty"`
	Disabled       bool   `json:"disable,omitempty"`
	IgnoreVerified bool   `json:"ignore-verified,omitempty"`
	MaxDepth       *int   `json:"max-depth,omitempty"`
}

// PruneJob represents a PBS scheduled prune job config entry.
type PruneJob struct {
	ID          string `json:"id"`
	Store       string `json:"store"`
	NS          string `json:"ns,omitempty"`
	Schedule    string `json:"schedule,omitempty"`
	Comment     string `json:"comment,omitempty"`
	Disabled    bool   `json:"disable,omitempty"`
	KeepLast    *int   `json:"keep-last,omitempty"`
	KeepHourly  *int   `json:"keep-hourly,omitempty"`
	KeepDaily   *int   `json:"keep-daily,omitempty"`
	KeepWeekly  *int   `json:"keep-weekly,omitempty"`
	KeepMonthly *int   `json:"keep-monthly,omitempty"`
	KeepYearly  *int   `json:"keep-yearly,omitempty"`
}

// SyncJob represents a PBS scheduled sync job config entry.
type SyncJob struct {
	ID             string `json:"id"`
	Store          string `json:"store"`
	Remote         string `json:"remote,omitempty"`
	RemoteStore    string `json:"remote-store,omitempty"`
	Schedule       string `json:"schedule,omitempty"`
	Comment        string `json:"comment,omitempty"`
	Disabled       bool   `json:"disable,omitempty"`
	RemoveVanished bool   `json:"remove-vanished,omitempty"`
}

// Remote represents a PBS remote (pull source) configuration entry.
type Remote struct {
	Name        string `json:"name"`
	Host        string `json:"host,omitempty"`
	Port        *int   `json:"port,omitempty"`
	AuthID      string `json:"authid,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Comment     string `json:"comment,omitempty"`
}

// TrafficRule represents a PBS traffic control rule.
type TrafficRule struct {
	Name    string `json:"name"`
	RateIn  string `json:"rate-in,omitempty"`
	RateOut string `json:"rate-out,omitempty"`
	Comment string `json:"comment,omitempty"`
}

// CertInfo represents a PBS node certificate entry.
type CertInfo struct {
	Filename    string   `json:"filename,omitempty"`
	Subject     string   `json:"subject,omitempty"`
	Issuer      string   `json:"issuer,omitempty"`
	NotBefore   *int64   `json:"notbefore,omitempty"`
	NotAfter    *int64   `json:"notafter,omitempty"`
	SANs        []string `json:"san,omitempty"`
	Fingerprint string   `json:"fingerprint,omitempty"`
}
