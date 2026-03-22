package sysconfig

import "testing"

func TestParseXrandrMonitors(t *testing.T) {
	output := `Monitors: 2
 0: +*DP-1 1920/530x1080/300+0+0  DP-1
 1: +HDMI-1 2560/600x1440/340+1920+0  HDMI-1`

	displays := ParseXrandrMonitors(output)
	if len(displays) != 2 {
		t.Fatalf("expected 2 displays, got %d", len(displays))
	}
	if displays[0].Name != "DP-1" {
		t.Fatalf("expected first display name DP-1, got %q", displays[0].Name)
	}
	if !displays[0].Primary {
		t.Fatalf("expected first display to be primary")
	}
	if displays[0].Width != 1920 || displays[0].Height != 1080 {
		t.Fatalf("expected first display 1920x1080, got %dx%d", displays[0].Width, displays[0].Height)
	}
	if displays[1].OffsetX != 1920 {
		t.Fatalf("expected second display offset_x=1920, got %d", displays[1].OffsetX)
	}
}

func TestParseXrandrMonitorsEmpty(t *testing.T) {
	displays := ParseXrandrMonitors("")
	if len(displays) != 0 {
		t.Fatalf("expected 0 displays, got %d", len(displays))
	}
}

func TestParseResolution(t *testing.T) {
	width, height := ParseResolution("1920 x 1080 @ 60 Hz")
	if width != 1920 || height != 1080 {
		t.Fatalf("expected 1920x1080, got %dx%d", width, height)
	}
}

func TestParsePowerShellScreenDisplays(t *testing.T) {
	output := `\\.\DISPLAY1|1920|1080|True|0|0
\\.\DISPLAY2|2560|1440|False|1920|0`

	displays := ParsePowerShellScreenDisplays(output)
	if len(displays) != 2 {
		t.Fatalf("expected 2 displays, got %d", len(displays))
	}
	if displays[0].Name != `\\.\DISPLAY1` {
		t.Fatalf("expected first display name \\\\.\\DISPLAY1, got %q", displays[0].Name)
	}
	if !displays[0].Primary {
		t.Fatalf("expected first display to be primary")
	}
	if displays[1].OffsetX != 1920 {
		t.Fatalf("expected second display offset_x=1920, got %d", displays[1].OffsetX)
	}
	if displays[1].Width != 2560 || displays[1].Height != 1440 {
		t.Fatalf("expected second display 2560x1440, got %dx%d", displays[1].Width, displays[1].Height)
	}
}
