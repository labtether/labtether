package remoteaccess

import (
	"os"
	"os/exec"
	"testing"
)

func TestCollectDesktopDiagnosticBasicFields(t *testing.T) {
	diag := CollectDesktopDiagnostic(nil, nil)
	if envDisplay := os.Getenv("DISPLAY"); diag.EnvDisplay != envDisplay {
		t.Errorf("EnvDisplay=%q, want %q", diag.EnvDisplay, envDisplay)
	}
	if diag.ActiveVNCSessions != 0 {
		t.Errorf("ActiveVNCSessions=%d, want 0", diag.ActiveVNCSessions)
	}
	if diag.ActiveWebRTCSessions != 0 {
		t.Errorf("ActiveWebRTCSessions=%d, want 0", diag.ActiveWebRTCSessions)
	}
}

func TestCollectDesktopDiagnosticDetectsXterm(t *testing.T) {
	diag := CollectDesktopDiagnostic(nil, nil)
	_, xtermErr := exec.LookPath("xterm")
	wantXterm := xtermErr == nil
	if diag.XtermAvailable != wantXterm {
		t.Errorf("XtermAvailable=%v, want %v", diag.XtermAvailable, wantXterm)
	}
}
