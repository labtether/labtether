package terminal

import (
	"regexp"
	"testing"
)

func TestTmuxSessionNameForIDIsStableAndDoesNotTruncateUniquenessSuffix(t *testing.T) {
	firstID := "pts_1784077900279951758_2204"
	secondID := "pts_1784077900279951758_2205"
	first := TmuxSessionNameForID(firstID)
	if first != TmuxSessionNameForID(firstID) {
		t.Fatal("same ID did not produce a stable tmux name")
	}
	if first == TmuxSessionNameForID(secondID) {
		t.Fatalf("IDs differing only in their uniqueness suffix collided as %q", first)
	}
	if !regexp.MustCompile(`^lt-[0-9a-f]{32}$`).MatchString(first) {
		t.Fatalf("tmux name %q is not in the bounded safe format", first)
	}
}
