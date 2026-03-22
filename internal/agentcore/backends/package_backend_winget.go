package backends

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/labtether/labtether/internal/agentmgr"
	"github.com/labtether/labtether/internal/securityruntime"
)

// RunWindowsPackageCommand is the function used to run WinGet/choco commands. Overridable for tests.
var RunWindowsPackageCommand = securityruntime.CommandContextCombinedOutput

// WindowsPackageBackend implements PackageBackend using WinGet with Chocolatey fallback.
type WindowsPackageBackend struct {
	// backend is "winget" or "choco".
	backend string
}

// wingetPackageRow is an intermediate representation of a parsed WinGet table row.
type wingetPackageRow struct {
	name      string
	id        string
	version   string
	available string
}

// ListPackages lists installed packages via WinGet or Chocolatey.
func (b WindowsPackageBackend) ListPackages() ([]agentmgr.PackageInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), PackageActionCommandTimeout)
	defer cancel()

	switch b.backend {
	case "choco":
		out, err := RunWindowsPackageCommand(ctx, "choco", "list", "--local-only")
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("choco package listing timed out")
			}
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				return nil, fmt.Errorf("choco package listing failed: %s", trimmed)
			}
			return nil, fmt.Errorf("choco package listing failed: %w", err)
		}
		return parseChocoListOutput(out)

	default: // "winget"
		out, err := RunWindowsPackageCommand(ctx, "winget", "list",
			"--accept-source-agreements", "--disable-interactivity")
		if err != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, fmt.Errorf("winget package listing timed out")
			}
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				return nil, fmt.Errorf("winget package listing failed: %s", trimmed)
			}
			return nil, fmt.Errorf("winget package listing failed: %w", err)
		}
		rows, parseErr := parseWinGetListOutput(out)
		if parseErr != nil {
			return nil, parseErr
		}
		pkgs := make([]agentmgr.PackageInfo, 0, len(rows))
		for _, row := range rows {
			pkgs = append(pkgs, agentmgr.PackageInfo{
				Name:    row.name,
				Version: row.version,
				Status:  "installed",
			})
		}
		return pkgs, nil
	}
}

// PerformAction performs a package action (install, upgrade, uninstall) via WinGet or Chocolatey.
func (b WindowsPackageBackend) PerformAction(action string, packages []string) (PackageActionResult, error) {
	if len(packages) == 0 {
		return PackageActionResult{}, fmt.Errorf("no packages specified")
	}

	ctx, cancel := context.WithTimeout(context.Background(), PackageActionCommandTimeout)
	defer cancel()

	var combined bytes.Buffer

	for _, pkg := range packages {
		args, err := buildWindowsPackageActionArgs(b.backend, action, pkg)
		if err != nil {
			return PackageActionResult{}, err
		}

		var cmd string
		switch b.backend {
		case "choco":
			cmd = "choco"
		default:
			cmd = "winget"
		}

		out, runErr := RunWindowsPackageCommand(ctx, cmd, args...)
		if combined.Len() > 0 && len(out) > 0 {
			combined.WriteByte('\n')
		}
		combined.Write(out)

		result := PackageActionResult{
			Output:         TruncateCommandOutput(combined.Bytes(), MaxCommandOutputBytes),
			RebootRequired: false,
		}
		if runErr != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return result, fmt.Errorf("package action timed out")
			}
			return result, runErr
		}
	}

	return PackageActionResult{
		Output:         TruncateCommandOutput(combined.Bytes(), MaxCommandOutputBytes),
		RebootRequired: false,
	}, nil
}

func buildWindowsPackageActionArgs(backend, action, pkg string) ([]string, error) {
	switch backend {
	case "choco":
		switch action {
		case "install":
			return []string{"install", pkg, "-y"}, nil
		case "upgrade":
			return []string{"upgrade", pkg, "-y"}, nil
		case "uninstall", "remove":
			return []string{"uninstall", pkg, "-y"}, nil
		default:
			return nil, fmt.Errorf("unsupported package action %q for choco", action)
		}
	default: // winget
		switch action {
		case "install":
			return []string{"install", "--id", pkg,
				"--accept-package-agreements", "--accept-source-agreements", "--silent"}, nil
		case "upgrade":
			return []string{"upgrade", "--id", pkg,
				"--accept-package-agreements", "--accept-source-agreements", "--silent"}, nil
		case "uninstall", "remove":
			return []string{"uninstall", "--id", pkg, "--silent"}, nil
		default:
			return nil, fmt.Errorf("unsupported package action %q for winget", action)
		}
	}
}

// parseWinGetListOutput parses the fixed-width table output of
// `winget list --accept-source-agreements --disable-interactivity`.
//
// WinGet prints a header row, a separator row of dashes, then data rows. Each
// column is separated by two or more spaces; column widths are determined by
// the position of each header word.
func parseWinGetListOutput(raw []byte) ([]wingetPackageRow, error) {
	scanner := bufio.NewScanner(bytes.NewReader(raw))

	// Find the header line that contains the column names.
	var headerLine string
	for scanner.Scan() {
		line := scanner.Text()
		// WinGet may prefix output with BOM or progress chars on some Windows
		// versions; strip leading non-ASCII noise.
		cleaned := strings.TrimLeftFunc(line, func(r rune) bool { return r > 127 })
		if strings.Contains(cleaned, "Name") && strings.Contains(cleaned, "Id") &&
			strings.Contains(cleaned, "Version") {
			headerLine = cleaned
			break
		}
	}
	if headerLine == "" {
		return nil, nil
	}

	// Derive column start positions from the header.
	nameStart, idStart, versionStart, availableStart := wingetColumnOffsets(headerLine)
	if idStart < 0 || versionStart < 0 {
		// Cannot locate required columns; return empty rather than corrupt data.
		return nil, nil
	}

	// Consume the separator line (dashes).
	if scanner.Scan() {
		sep := scanner.Text()
		if !strings.HasPrefix(strings.TrimSpace(sep), "-") {
			// Not a separator — put it back conceptually by re-checking below,
			// but since Scanner doesn't support unread we just ignore this edge case.
			_ = sep
		}
	}

	var rows []wingetPackageRow
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Pad the line so substring extractions are safe.
		row := extractWinGetRow(line, nameStart, idStart, versionStart, availableStart)
		if row.name == "" && row.id == "" {
			continue
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// wingetColumnOffsets returns the byte offset of each column in the WinGet header.
// Returns -1 for columns that are not found.
func wingetColumnOffsets(header string) (nameStart, idStart, versionStart, availableStart int) {
	nameStart = strings.Index(header, "Name")
	idStart = strings.Index(header, "Id")
	versionStart = strings.Index(header, "Version")
	availableStart = strings.Index(header, "Available")
	return
}

// extractWinGetRow extracts fields from a single WinGet data row using column offsets.
func extractWinGetRow(line string, nameStart, idStart, versionStart, availableStart int) wingetPackageRow {
	safeSlice := func(s string, start, end int) string {
		if start < 0 || start >= len(s) {
			return ""
		}
		if end < 0 || end > len(s) {
			end = len(s)
		}
		return strings.TrimSpace(s[start:end])
	}

	name := safeSlice(line, nameStart, idStart)
	id := safeSlice(line, idStart, versionStart)

	var version, available string
	if availableStart >= 0 {
		version = safeSlice(line, versionStart, availableStart)
		// Everything after "Available" column start up to the "Source" column.
		// Source column is not critical; just read to end of line minus trailing
		// source token (e.g. "winget").
		rest := safeSlice(line, availableStart, -1)
		// The source token (e.g. "winget") is at the very end after whitespace.
		// Split on runs of spaces and take the non-source part.
		parts := strings.Fields(rest)
		if len(parts) >= 2 {
			// Last token is the source name; second-to-last or earlier is available.
			// WinGet sources are single-word identifiers; available version precedes it.
			available = parts[0]
		} else if len(parts) == 1 {
			// Could be just the source or just an available version — WinGet always
			// ends rows with the source name so a single token here is the source.
			available = ""
		}
	} else {
		version = safeSlice(line, versionStart, -1)
	}

	return wingetPackageRow{
		name:      name,
		id:        id,
		version:   version,
		available: available,
	}
}

// parseChocoListOutput parses the output of `choco list --local-only`.
//
// Format:
//
//	Chocolatey v1.4.0
//	<name> <version>
//	...
//	N packages installed.
func parseChocoListOutput(raw []byte) ([]agentmgr.PackageInfo, error) {
	var pkgs []agentmgr.PackageInfo
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Skip header/footer lines.
		if strings.HasPrefix(line, "Chocolatey v") ||
			strings.HasSuffix(line, "packages installed.") ||
			strings.HasSuffix(line, "package installed.") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pkgs = append(pkgs, agentmgr.PackageInfo{
			Name:    fields[0],
			Version: fields[1],
			Status:  "installed",
		})
	}
	return pkgs, nil
}
