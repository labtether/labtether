package main

// Wiring pins hold the Hub's HTTP and runtime wiring still while code moves
// out of cmd/labtether. Each pin compares what the real wiring does with a
// golden file in testdata/wiring_pins.
//
// Regenerate every golden file with one command:
//
//	LABTETHER_UPDATE_WIRING_PINS=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether
//
// A regenerated file is a claim about behaviour, not a fix. Read its diff
// line by line and explain every changed line in review before committing
// it. A move that is meant to change nothing must leave these files
// byte-identical.

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const wiringPinUpdateEnv = "LABTETHER_UPDATE_WIRING_PINS"

const wiringPinUpdateCommand = wiringPinUpdateEnv + "=1 go test -count=1 -run 'TestWiringPin' ./cmd/labtether"

// wiringPinDir is resolved when the test binary starts, before any test
// changes the working directory.
var wiringPinDir = func() string {
	dir, err := filepath.Abs(filepath.Join("testdata", "wiring_pins"))
	if err != nil {
		panic(err)
	}
	return dir
}()

func wiringPinUpdating() bool {
	return os.Getenv(wiringPinUpdateEnv) == "1"
}

// compareWiringPin checks got against testdata/wiring_pins/<name>, or
// rewrites that file when LABTETHER_UPDATE_WIRING_PINS=1.
func compareWiringPin(t *testing.T, name string, got []byte) {
	t.Helper()
	path := filepath.Join(wiringPinDir, name)
	if wiringPinUpdating() {
		if err := os.MkdirAll(wiringPinDir, 0o755); err != nil {
			t.Fatalf("create %s: %v", wiringPinDir, err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("rewrote %s; review its diff before committing", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v\nCreate it with:\n  %s", path, err, wiringPinUpdateCommand)
	}
	if bytes.Equal(want, got) {
		return
	}
	t.Fatalf("wiring pin %s changed.\n%s\nIf the change is intended, regenerate and review the diff:\n  %s",
		name, describeWiringPinDiff(string(want), string(got)), wiringPinUpdateCommand)
}

// describeWiringPinDiff compares the files block by block. Golden files are
// blocks separated by a blank line, each named by its first line, so a
// failure names the route, list or document part that changed, then the
// lines only in the golden file (-) and only in this run (+).
func describeWiringPinDiff(want, got string) string {
	wantBlocks, wantOrder := splitWiringPinBlocks(want)
	gotBlocks, gotOrder := splitWiringPinBlocks(got)
	var b strings.Builder
	shown := 0
	note := func(format string, args ...any) {
		if shown < 30 {
			fmt.Fprintf(&b, format, args...)
		}
		shown++
	}
	for _, name := range wantOrder {
		gotBlock, ok := gotBlocks[name]
		if !ok {
			note("  block removed: %s\n", name)
			continue
		}
		for _, line := range linesOnlyIn(wantBlocks[name], gotBlock) {
			note("  in %s\n    - %s\n", name, line)
		}
		for _, line := range linesOnlyIn(gotBlock, wantBlocks[name]) {
			note("  in %s\n    + %s\n", name, line)
		}
	}
	for _, name := range gotOrder {
		if _, ok := wantBlocks[name]; !ok {
			note("  block added: %s\n", name)
		}
	}
	if shown == 0 {
		b.WriteString("  same blocks in a different order, or whitespace only\n")
	}
	if shown > 30 {
		fmt.Fprintf(&b, "  ... and %d more differences\n", shown-30)
	}
	return b.String()
}

// linesOnlyIn returns the lines of a that b lacks, counting repeats.
func linesOnlyIn(a, b string) []string {
	have := map[string]int{}
	for _, line := range strings.Split(b, "\n") {
		have[line]++
	}
	var out []string
	for _, line := range strings.Split(a, "\n") {
		if have[line] > 0 {
			have[line]--
			continue
		}
		out = append(out, line)
	}
	return out
}

func splitWiringPinBlocks(text string) (map[string]string, []string) {
	blocks := map[string]string{}
	var order []string
	for _, block := range strings.Split(text, "\n\n") {
		block = strings.Trim(block, "\n")
		if block == "" {
			continue
		}
		name := strings.SplitN(block, "\n", 2)[0]
		if _, dup := blocks[name]; dup {
			name = fmt.Sprintf("%s (#%d)", name, len(order))
		}
		blocks[name] = block
		order = append(order, name)
	}
	return blocks, order
}
