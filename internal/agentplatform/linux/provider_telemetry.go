//go:build linux

package linux

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

var (
	procStatPath    = "/proc/stat"
	procMeminfoPath = "/proc/meminfo"
	procNetDevPath  = "/proc/net/dev"
	statfsFunc      = syscall.Statfs
)

func readCPUSample() (cpuSample, error) {
	file, err := os.Open(procStatPath)
	if err != nil {
		return cpuSample{}, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "cpu ") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 5 {
			return cpuSample{}, fmt.Errorf("unexpected /proc/stat format")
		}

		var total uint64
		var idle uint64
		for i, token := range fields[1:] {
			parsed, err := strconv.ParseUint(token, 10, 64)
			if err != nil {
				return cpuSample{}, err
			}
			total += parsed
			switch i {
			case 3, 4: // idle + iowait
				idle += parsed
			}
		}

		return cpuSample{idle: idle, total: total}, nil
	}

	if err := scanner.Err(); err != nil {
		return cpuSample{}, err
	}
	return cpuSample{}, fmt.Errorf("cpu line not found")
}

func readMemoryUsagePercent() float64 {
	file, err := os.Open(procMeminfoPath)
	if err != nil {
		return 0
	}
	defer file.Close()

	var totalKB uint64
	var availableKB uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "MemTotal:":
			totalKB, _ = strconv.ParseUint(fields[1], 10, 64)
		case "MemAvailable:":
			availableKB, _ = strconv.ParseUint(fields[1], 10, 64)
		}
	}

	if totalKB <= 0 {
		return 0
	}
	if availableKB >= totalKB {
		return 0
	}
	used := totalKB - availableKB
	return (float64(used) / float64(totalKB)) * 100.0
}

func readDiskUsagePercent(path string) float64 {
	var stats syscall.Statfs_t
	if err := statfsFunc(path, &stats); err != nil {
		return 0
	}
	if stats.Blocks == 0 {
		return 0
	}

	total := float64(stats.Blocks)
	free := float64(stats.Bavail) // Bavail excludes reserved-for-root blocks; Bfree does not.
	used := total - free
	if used < 0 {
		used = 0
	}
	return (used / total) * 100.0
}

func readNetworkBytes() (float64, float64) {
	file, err := os.Open(procNetDevPath)
	if err != nil {
		return 0, 0
	}
	defer file.Close()

	var rxTotal uint64
	var txTotal uint64

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "Inter-") || strings.HasPrefix(line, "face") {
			continue
		}

		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		iface := strings.TrimSpace(line[:colon])
		if iface == "lo" {
			continue
		}

		fields := strings.Fields(line[colon+1:])
		if len(fields) < 16 {
			continue
		}

		rx, err := strconv.ParseUint(fields[0], 10, 64)
		if err == nil {
			rxTotal += rx
		}
		tx, err := strconv.ParseUint(fields[8], 10, 64)
		if err == nil {
			txTotal += tx
		}
	}

	return float64(rxTotal), float64(txTotal)
}
