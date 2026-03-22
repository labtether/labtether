//go:build linux

package linux

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/labtether/labtether/internal/agentplatform/tailscale"
)

var (
	readOSMetadataFunc            = readOSMetadata
	readDMIInfoFunc               = readDMIInfo
	readCPUInfoFunc               = readCPUInfo
	readMemTotalBytesFunc         = readMemTotalBytes
	readDiskCapacityBytesFunc     = readDiskCapacityBytes
	readNetworkInterfaceCountFunc = readNetworkInterfaceCount
	readTailscaleMetadataFunc     = tailscale.ReadMetadata
	readCapabilityMetadataFunc    = readCapabilityMetadata
)

func collectHostMetadata(hostname, source string) map[string]string {
	metadata := map[string]string{
		"hostname":         hostname,
		"agent":            source,
		"cpu_architecture": runtime.GOARCH,
		"os":               "linux",
		"platform":         "linux",
	}

	for key, value := range readOSMetadataFunc() {
		metadata[key] = value
	}
	for key, value := range readDMIInfoFunc() {
		metadata[key] = value
	}
	for key, value := range readCPUInfoFunc() {
		metadata[key] = value
	}

	if totalBytes := readMemTotalBytesFunc(); totalBytes > 0 {
		metadata["memory_total_bytes"] = strconv.FormatUint(totalBytes, 10)
	}

	if totalBytes, availableBytes := readDiskCapacityBytesFunc("/"); totalBytes > 0 {
		metadata["disk_root_total_bytes"] = strconv.FormatUint(totalBytes, 10)
		metadata["disk_root_available_bytes"] = strconv.FormatUint(availableBytes, 10)
	}

	if ifaceCount := readNetworkInterfaceCountFunc(); ifaceCount > 0 {
		metadata["network_interface_count"] = strconv.Itoa(ifaceCount)
	}
	for key, value := range readTailscaleMetadataFunc() {
		metadata[key] = value
	}
	for key, value := range readCapabilityMetadataFunc() {
		metadata[key] = value
	}

	return metadata
}

func readCapabilityMetadata() map[string]string {
	return readCapabilityMetadataWith(exec.LookPath)
}

func readCapabilityMetadataWith(lookPath func(string) (string, error)) map[string]string {
	metadata := map[string]string{
		"cap_services":           "",
		"cap_packages":           "",
		"cap_logs":               "stored",
		"cap_schedules":          "list",
		"cap_network":            "list",
		"service_backend":        "none",
		"package_backend":        "none",
		"log_backend":            "none",
		"network_backend":        "ifconfig",
		"network_action_backend": "none",
	}

	if commandExists(lookPath, "systemctl") {
		metadata["cap_services"] = "list,action"
		metadata["service_backend"] = "systemd"
	}

	if backend, pkgCapability := detectLinuxPackageCapability(lookPath); backend != "" {
		metadata["package_backend"] = backend
		metadata["cap_packages"] = pkgCapability
	}

	if commandExists(lookPath, "journalctl") {
		metadata["cap_logs"] = "stored,query,stream"
		metadata["log_backend"] = "journalctl"
	}

	if commandExists(lookPath, "netplan") || commandExists(lookPath, "nmcli") {
		metadata["cap_network"] = "list,action"
		if commandExists(lookPath, "netplan") {
			metadata["network_action_backend"] = "netplan"
		} else {
			metadata["network_action_backend"] = "nmcli"
		}
	}

	return metadata
}

func commandExists(lookPath func(string) (string, error), name string) bool {
	path, err := lookPath(name)
	return err == nil && strings.TrimSpace(path) != ""
}

func detectLinuxPackageCapability(lookPath func(string) (string, error)) (backend string, capability string) {
	for _, candidate := range []string{"apt-get", "dnf", "yum", "zypper", "pacman", "apk"} {
		if commandExists(lookPath, candidate) {
			return candidate, "list,action"
		}
	}
	for _, candidate := range []string{"dpkg-query", "rpm"} {
		if commandExists(lookPath, candidate) {
			return candidate, "list"
		}
	}
	return "", ""
}

func readOSMetadata() map[string]string {
	metadata := map[string]string{}

	if release, err := parseKeyValueFile("/etc/os-release"); err == nil {
		if value := strings.TrimSpace(release["PRETTY_NAME"]); value != "" {
			metadata["os_pretty_name"] = value
		}
		if value := strings.TrimSpace(release["NAME"]); value != "" {
			metadata["os_name"] = value
		}
		if value := strings.TrimSpace(release["ID"]); value != "" {
			metadata["os_id"] = value
		}
		if value := strings.TrimSpace(release["ID_LIKE"]); value != "" {
			metadata["os_id_like"] = value
		}
		if value := strings.TrimSpace(release["VERSION_ID"]); value != "" {
			metadata["os_version_id"] = value
		}
		if value := strings.TrimSpace(release["VERSION"]); value != "" {
			metadata["os_version"] = value
		}
	}

	if value := readTrimmedFile("/proc/sys/kernel/osrelease"); value != "" {
		metadata["kernel_release"] = value
	}
	if value := readTrimmedFile("/proc/sys/kernel/version"); value != "" {
		metadata["kernel_version"] = value
	}

	return metadata
}

func readDMIInfo() map[string]string {
	fields := []struct {
		Path string
		Key  string
	}{
		{Path: "/sys/class/dmi/id/sys_vendor", Key: "computer_vendor"},
		{Path: "/sys/class/dmi/id/product_name", Key: "computer_model"},
		{Path: "/sys/class/dmi/id/product_version", Key: "computer_version"},
		{Path: "/sys/class/dmi/id/board_vendor", Key: "motherboard_vendor"},
		{Path: "/sys/class/dmi/id/board_name", Key: "motherboard_model"},
		{Path: "/sys/class/dmi/id/board_version", Key: "motherboard_version"},
		{Path: "/sys/class/dmi/id/chassis_vendor", Key: "chassis_vendor"},
		{Path: "/sys/class/dmi/id/chassis_type", Key: "chassis_type"},
		{Path: "/sys/class/dmi/id/bios_vendor", Key: "bios_vendor"},
		{Path: "/sys/class/dmi/id/bios_version", Key: "bios_version"},
		{Path: "/sys/class/dmi/id/bios_date", Key: "bios_date"},
	}

	metadata := map[string]string{}
	for _, field := range fields {
		value := readTrimmedFile(field.Path)
		if value == "" {
			continue
		}
		if field.Key == "chassis_type" {
			value = normalizeChassisType(value)
		}
		metadata[field.Key] = value
	}

	return metadata
}

func readCPUInfo() map[string]string {
	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return map[string]string{}
	}
	defer file.Close()

	metadata := map[string]string{}
	scanner := bufio.NewScanner(file)
	logicalCount := 0
	coresPerSocket := 0
	threadsPerSocket := 0
	physicalIDs := make(map[string]struct{})

	for scanner.Scan() {
		line := scanner.Text()
		key, rawValue, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value := strings.TrimSpace(rawValue)

		switch key {
		case "processor":
			logicalCount += 1
		case "physical id":
			if value != "" {
				physicalIDs[value] = struct{}{}
			}
		case "model name":
			if metadata["cpu_model"] == "" && value != "" {
				metadata["cpu_model"] = value
			}
		case "vendor_id":
			if metadata["cpu_vendor"] == "" && value != "" {
				metadata["cpu_vendor"] = value
			}
		case "cpu cores":
			if coresPerSocket == 0 {
				if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
					coresPerSocket = parsed
				}
			}
		case "siblings":
			if threadsPerSocket == 0 {
				if parsed, err := strconv.Atoi(value); err == nil && parsed > 0 {
					threadsPerSocket = parsed
				}
			}
		}
	}

	if logicalCount == 0 {
		logicalCount = runtime.NumCPU()
	}
	if logicalCount > 0 {
		metadata["cpu_threads_logical"] = strconv.Itoa(logicalCount)
	}

	socketCount := len(physicalIDs)
	if socketCount == 0 && logicalCount > 0 {
		socketCount = 1
	}
	if socketCount > 0 {
		metadata["cpu_sockets"] = strconv.Itoa(socketCount)
	}

	if coresPerSocket > 0 {
		metadata["cpu_cores_per_socket"] = strconv.Itoa(coresPerSocket)
		if socketCount > 0 {
			metadata["cpu_cores_physical"] = strconv.Itoa(coresPerSocket * socketCount)
		}
	}

	if threadsPerSocket > 0 {
		metadata["cpu_threads_per_socket"] = strconv.Itoa(threadsPerSocket)
	}

	if mhz := readCPUMaxFrequencyMHz(); mhz > 0 {
		metadata["cpu_max_mhz"] = fmt.Sprintf("%.0f", mhz)
	}
	if mhz := readCPUMinFrequencyMHz(); mhz > 0 {
		metadata["cpu_min_mhz"] = fmt.Sprintf("%.0f", mhz)
	}

	return metadata
}

func readCPUMaxFrequencyMHz() float64 {
	return readKiloHertzFileAsMHz("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_max_freq")
}

func readCPUMinFrequencyMHz() float64 {
	return readKiloHertzFileAsMHz("/sys/devices/system/cpu/cpu0/cpufreq/cpuinfo_min_freq")
}

func readKiloHertzFileAsMHz(path string) float64 {
	raw := readTrimmedFile(path)
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value / 1000.0
}

func readMemTotalBytes() uint64 {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "MemTotal:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0
		}
		totalKB, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0
		}
		return totalKB * 1024
	}
	return 0
}

func readDiskCapacityBytes(path string) (totalBytes uint64, availableBytes uint64) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0, 0
	}

	blockSize := uint64(stats.Bsize)
	if blockSize == 0 {
		return 0, 0
	}

	return stats.Blocks * blockSize, stats.Bavail * blockSize
}

func readNetworkInterfaceCount() int {
	file, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0
	}
	defer file.Close()

	count := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Inter-") || strings.HasPrefix(line, "face") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) != 2 {
			continue
		}
		iface := strings.TrimSpace(parts[0])
		if iface == "lo" {
			continue
		}
		count += 1
	}
	return count
}

func parseKeyValueFile(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	out := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		normalized := strings.Trim(strings.TrimSpace(value), `"'`)
		out[strings.TrimSpace(key)] = normalized
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func readTrimmedFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "None" {
		return ""
	}
	return value
}

func normalizeChassisType(raw string) string {
	code, err := strconv.Atoi(raw)
	if err != nil {
		return raw
	}

	labels := map[int]string{
		1:  "other",
		2:  "unknown",
		3:  "desktop",
		4:  "low profile desktop",
		5:  "pizza box",
		6:  "mini tower",
		7:  "tower",
		8:  "portable",
		9:  "laptop",
		10: "notebook",
		11: "handheld",
		12: "docking station",
		13: "all in one",
		14: "sub notebook",
		15: "space-saving",
		16: "lunch box",
		17: "main server chassis",
		18: "expansion chassis",
		19: "sub chassis",
		20: "bus expansion chassis",
		21: "peripheral chassis",
		22: "raid chassis",
		23: "rack mount chassis",
		24: "sealed-case pc",
		25: "multi-system chassis",
		26: "compact pci",
		27: "advanced tca",
		28: "blade",
		29: "blade enclosure",
		30: "tablet",
		31: "convertible",
		32: "detachable",
		33: "iot gateway",
		34: "embedded pc",
		35: "mini pc",
		36: "stick pc",
	}
	if label, ok := labels[code]; ok {
		return label
	}

	return raw
}
