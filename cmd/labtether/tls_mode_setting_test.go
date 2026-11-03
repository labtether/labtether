package main

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// LABTETHER_TLS_MODE=bogus was served as "auto" with no log line, and the
// Hub kept "bogus" as its recorded mode.
func TestResolveTLSModeSettingWarnsAndUsesAuto(t *testing.T) {
	var output bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(originalWriter) })

	for raw, want := range map[string]string{"auto": "auto", " External ": "external", "disabled": "disabled", "": "auto", "bogus": "auto"} {
		if got := resolveTLSModeSetting(raw); got != want {
			t.Fatalf("resolveTLSModeSetting(%q) = %q, want %q", raw, got, want)
		}
	}
	if !strings.Contains(output.String(), `LABTETHER_TLS_MODE="bogus" is not auto, external or disabled; using auto`) ||
		strings.Count(output.String(), "LABTETHER_TLS_MODE") != 1 {
		t.Fatalf("log = %q, want one warning for the unknown value", output.String())
	}
}
