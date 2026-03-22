package terminal

import "testing"

func TestStripANSI(t *testing.T) {
	input := "\033[32mhello\033[0m world\n"
	got := StripANSI(input)
	if got != "hello world\n" {
		t.Fatalf("expected 'hello world\\n', got %q", got)
	}
}

func TestMergeScrollback_FullOverlap(t *testing.T) {
	stored := []byte("line1\nline2\nline3\n")
	tmuxCapture := []byte("line2\nline3\nline4\nline5\n")
	merged := MergeScrollback(stored, tmuxCapture)
	expected := "line1\nline2\nline3\nline4\nline5\n"
	if string(merged) != expected {
		t.Fatalf("expected %q, got %q", expected, string(merged))
	}
}

func TestMergeScrollback_NoOverlap(t *testing.T) {
	stored := []byte("aaa\nbbb\n")
	tmuxCapture := []byte("ccc\nddd\n")
	merged := MergeScrollback(stored, tmuxCapture)
	expected := "aaa\nbbb\n--- reconnected ---\nccc\nddd\n"
	if string(merged) != expected {
		t.Fatalf("expected %q, got %q", expected, string(merged))
	}
}

func TestMergeScrollback_EmptyTmux(t *testing.T) {
	stored := []byte("line1\nline2\n")
	merged := MergeScrollback(stored, nil)
	if string(merged) != string(stored) {
		t.Fatalf("expected stored buffer unchanged, got %q", string(merged))
	}
}

func TestMergeScrollback_EmptyStored(t *testing.T) {
	tmuxCapture := []byte("line1\nline2\n")
	merged := MergeScrollback(nil, tmuxCapture)
	if string(merged) != string(tmuxCapture) {
		t.Fatalf("expected tmux capture unchanged, got %q", string(merged))
	}
}

func TestMergeScrollback_ANSIOverlap(t *testing.T) {
	stored := []byte("\033[32mline1\033[0m\n\033[31mline2\033[0m\n")
	tmuxCapture := []byte("\033[31mline2\033[0m\n\033[34mline3\033[0m\n")
	merged := MergeScrollback(stored, tmuxCapture)
	// Should contain line1, line2 (from stored), line3 (from tmux)
	stripped := StripANSI(string(merged))
	if stripped != "line1\nline2\nline3\n" {
		t.Fatalf("expected merged plain text 'line1\\nline2\\nline3\\n', got %q", stripped)
	}
}
