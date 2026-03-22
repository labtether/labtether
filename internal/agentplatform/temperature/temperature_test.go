package temperature

import (
	"math"
	"testing"

	"github.com/shirou/gopsutil/v4/sensors"
)

func TestSelectPreferredTemperaturePrefersCPUSensor(t *testing.T) {
	stats := []sensors.TemperatureStat{
		{SensorKey: "nvme_composite", Temperature: 43.2},
		{SensorKey: "coretemp_package_id_0", Temperature: 61.7},
		{SensorKey: "ambient", Temperature: 27.0},
	}

	value, sensor, ok := selectPreferredTemperature(stats)
	if !ok {
		t.Fatalf("expected a selected temperature")
	}
	if sensor != "coretemp_package_id_0" {
		t.Fatalf("expected CPU sensor to be selected, got %q", sensor)
	}
	if math.Abs(value-61.7) > 0.0001 {
		t.Fatalf("expected 61.7C, got %.4f", value)
	}
}

func TestSelectPreferredTemperatureDropsOutOfRangeSamples(t *testing.T) {
	stats := []sensors.TemperatureStat{
		{SensorKey: "coretemp_package_id_0", Temperature: 325.0},
		{SensorKey: "coretemp_core_0", Temperature: -80.0},
	}

	_, _, ok := selectPreferredTemperature(stats)
	if ok {
		t.Fatalf("expected no valid sample for out-of-range input")
	}
}

func TestParseSysfsTemperatureValue(t *testing.T) {
	value, err := parseSysfsTemperatureValue("52345\n")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if math.Abs(value-52.345) > 0.0001 {
		t.Fatalf("expected 52.345C, got %.4f", value)
	}

	value, err = parseSysfsTemperatureValue("56.2")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if math.Abs(value-56.2) > 0.0001 {
		t.Fatalf("expected 56.2C, got %.4f", value)
	}
}

func TestParseTemperatureFromCommandOutput(t *testing.T) {
	value, err := parseTemperatureFromCommandOutput("CPU die temperature: 129.2 F")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if math.Abs(value-54.0) > 0.2 {
		t.Fatalf("expected about 54.0C, got %.4f", value)
	}

	value, err = parseTemperatureFromCommandOutput("osx-cpu-temp: 47.6°C")
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if math.Abs(value-47.6) > 0.0001 {
		t.Fatalf("expected 47.6C, got %.4f", value)
	}
}
