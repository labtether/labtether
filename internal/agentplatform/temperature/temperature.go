package temperature

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/labtether/labtether/internal/securityruntime"
	"github.com/shirou/gopsutil/v4/sensors"
)

const (
	minPlausibleTemperatureCelsius = -40.0
	maxPlausibleTemperatureCelsius = 150.0
)

var (
	temperatureValuePattern = regexp.MustCompile(`(?i)([-+]?\d+(?:\.\d+)?)\s*°?\s*([cf])?`)
	linuxTemperatureGlobs   = []string{
		"/sys/class/thermal/thermal_zone*/temp",
		"/sys/class/hwmon/hwmon*/temp*_input",
		"/sys/class/hwmon/hwmon*/device/temp*_input",
	}
	darwinTemperatureCommands = []struct {
		name string
		args []string
	}{
		{name: "osx-cpu-temp", args: []string{}},
		{name: "istats", args: []string{"cpu", "temp", "--value-only"}},
	}
)

func ReadCelsius() (*float64, error) {
	value, err := readTemperatureFromGopsutil()
	if err == nil {
		return &value, nil
	}
	gopsutilErr := err

	switch runtime.GOOS {
	case "linux":
		value, err := readTemperatureFromLinuxSysfs()
		if err == nil {
			return &value, nil
		}
		return nil, fmt.Errorf("gopsutil temperature probe failed: %w; linux sysfs fallback failed: %v", gopsutilErr, err)
	case "darwin":
		value, err := readTemperatureFromDarwinCommands()
		if err == nil {
			return &value, nil
		}
		return nil, fmt.Errorf("gopsutil temperature probe failed: %w; darwin command fallback failed: %v", gopsutilErr, err)
	default:
		return nil, fmt.Errorf("gopsutil temperature probe failed on %s: %w", runtime.GOOS, gopsutilErr)
	}
}

func readTemperatureFromGopsutil() (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	stats, err := sensors.TemperaturesWithContext(ctx)
	if err != nil {
		return 0, err
	}

	value, _, ok := selectPreferredTemperature(stats)
	if !ok {
		return 0, errors.New("no usable temperature samples returned")
	}
	return value, nil
}

func selectPreferredTemperature(stats []sensors.TemperatureStat) (float64, string, bool) {
	bestPriority := -1
	bestTemp := 0.0
	bestSensor := ""

	for _, sample := range stats {
		temp := sample.Temperature
		if math.IsNaN(temp) || math.IsInf(temp, 0) || !isPlausibleTemperature(temp) {
			continue
		}

		priority := temperatureSensorPriority(sample.SensorKey)
		if priority > bestPriority || (priority == bestPriority && temp > bestTemp) {
			bestPriority = priority
			bestTemp = temp
			bestSensor = sample.SensorKey
		}
	}

	if bestPriority < 0 {
		return 0, "", false
	}
	return bestTemp, bestSensor, true
}

func temperatureSensorPriority(sensorKey string) int {
	key := strings.ToLower(strings.TrimSpace(sensorKey))
	switch {
	case containsAny(key, "cpu", "package", "tdie", "tctl", "peci", "core", "die", "soc", "ecpu", "pcpu"):
		return 4
	case containsAny(key, "gpu", "graphics", "igpu"):
		return 3
	case containsAny(key, "memory", "dram", "chipset", "pch", "ambient", "board"):
		return 2
	case key == "":
		return 1
	default:
		return 1
	}
}

func containsAny(value string, tokens ...string) bool {
	for _, token := range tokens {
		if strings.Contains(value, token) {
			return true
		}
	}
	return false
}

func readTemperatureFromLinuxSysfs() (float64, error) {
	paths := make([]string, 0, 16)
	for _, pattern := range linuxTemperatureGlobs {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			continue
		}
		paths = append(paths, matches...)
	}

	if len(paths) == 0 {
		return 0, errors.New("no linux temperature sensor files discovered")
	}

	found := false
	best := 0.0
	for _, path := range paths {
		raw, err := os.ReadFile(path) // #nosec G304 -- Paths come from bounded sensor/sysfs discovery, not arbitrary user input.
		if err != nil {
			continue
		}
		value, err := parseSysfsTemperatureValue(string(raw))
		if err != nil {
			continue
		}
		if !isPlausibleTemperature(value) {
			continue
		}
		if !found || value > best {
			best = value
			found = true
		}
	}

	if !found {
		return 0, errors.New("linux temperature sensors discovered but no valid readings")
	}
	return best, nil
}

func parseSysfsTemperatureValue(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, err
	}
	if math.Abs(value) > 1000 {
		value = value / 1000.0
	}
	return value, nil
}

func readTemperatureFromDarwinCommands() (float64, error) {
	reasons := make([]string, 0, len(darwinTemperatureCommands))
	for _, probe := range darwinTemperatureCommands {
		path, err := exec.LookPath(probe.name)
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("%s not installed", probe.name))
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		output, err := securityruntime.CommandContextOutput(ctx, path, probe.args...)
		cancel()
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("%s failed", probe.name))
			continue
		}

		value, err := parseTemperatureFromCommandOutput(string(output))
		if err != nil {
			reasons = append(reasons, fmt.Sprintf("%s output parse failed", probe.name))
			continue
		}
		if !isPlausibleTemperature(value) {
			reasons = append(reasons, fmt.Sprintf("%s output out of range", probe.name))
			continue
		}
		return value, nil
	}

	if len(reasons) == 0 {
		return 0, errors.New("no darwin temperature command probe available")
	}
	return 0, errors.New(strings.Join(reasons, "; "))
}

func parseTemperatureFromCommandOutput(output string) (float64, error) {
	matches := temperatureValuePattern.FindAllStringSubmatch(output, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(match[1], 64)
		if err != nil {
			continue
		}

		unit := ""
		if len(match) > 2 {
			unit = strings.ToLower(strings.TrimSpace(match[2]))
		}
		if unit == "f" {
			value = (value - 32.0) * 5.0 / 9.0
		}

		if isPlausibleTemperature(value) {
			return value, nil
		}
	}

	return 0, errors.New("no valid temperature value found")
}

func isPlausibleTemperature(value float64) bool {
	return value >= minPlausibleTemperatureCelsius && value <= maxPlausibleTemperatureCelsius
}
