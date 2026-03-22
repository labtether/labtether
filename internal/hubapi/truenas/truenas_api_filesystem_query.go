package truenas

import (
	"context"
	"fmt"
	"path"
	"strings"

	tnconnector "github.com/labtether/labtether/internal/connectors/truenas"
)

func NormalizeTrueNASFilesystemPath(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "/mnt"
	}
	if !strings.HasPrefix(trimmed, "/") {
		trimmed = "/" + trimmed
	}
	return path.Clean(trimmed)
}

func ParentTrueNASFilesystemPath(currentPath string) string {
	currentPath = NormalizeTrueNASFilesystemPath(currentPath)
	if currentPath == "/" {
		return ""
	}
	cleaned := strings.TrimRight(currentPath, "/")
	idx := strings.LastIndex(cleaned, "/")
	if idx <= 0 {
		return "/"
	}
	return cleaned[:idx]
}

func CallTrueNASListDir(ctx context.Context, client *tnconnector.Client, directoryPath string) ([]map[string]any, error) {
	directoryPath = NormalizeTrueNASFilesystemPath(directoryPath)

	attempts := [][]any{
		{directoryPath},
		{directoryPath, map[string]any{}},
		{map[string]any{"path": directoryPath}},
	}

	var lastErr error
	for _, params := range attempts {
		var result any
		err := client.Call(ctx, "filesystem.listdir", params, &result)
		if err != nil {
			lastErr = err
			if tnconnector.IsMethodCallError(err) {
				continue
			}
			return nil, err
		}

		entries, ok := NormalizeTrueNASListDirResult(result)
		if !ok {
			return nil, fmt.Errorf("filesystem.listdir returned unexpected payload")
		}
		return entries, nil
	}
	return nil, lastErr
}

func CallTrueNASListDirWithRetries(ctx context.Context, client *tnconnector.Client, directoryPath string) ([]map[string]any, error) {
	var (
		entries []map[string]any
		err     error
	)
	for attempt := 0; attempt < TrueNASMethodCallRetryAttempts; attempt++ {
		entries, err = CallTrueNASListDir(ctx, client, directoryPath)
		if err == nil || !tnconnector.IsMethodCallError(err) {
			return entries, err
		}
		if !WaitForTrueNASMethodRetry(ctx, attempt) {
			break
		}
	}
	return nil, err
}
