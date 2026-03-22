package terminal

import (
	"bytes"
	"regexp"
	"strings"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func StripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

const mergeOverlapWindow = 100

// MergeScrollback merges a stored Postgres scrollback buffer with a tmux
// capture-pane output. Both contain raw ANSI byte streams. Deduplication
// works by stripping ANSI from the trailing lines of stored and the leading
// lines of tmuxCapture, finding the longest suffix/prefix match, then
// splicing the raw streams at the match point.
func MergeScrollback(stored, tmuxCapture []byte) []byte {
	if len(tmuxCapture) == 0 {
		return stored
	}
	if len(stored) == 0 {
		return tmuxCapture
	}

	storedLines := splitLines(stored)
	tmuxLines := splitLines(tmuxCapture)

	// Take trailing window from stored, leading window from tmux
	storedTail := storedLines
	if len(storedTail) > mergeOverlapWindow {
		storedTail = storedTail[len(storedTail)-mergeOverlapWindow:]
	}
	tmuxHead := tmuxLines
	if len(tmuxHead) > mergeOverlapWindow {
		tmuxHead = tmuxHead[:mergeOverlapWindow]
	}

	// Strip ANSI for comparison
	storedPlain := make([]string, len(storedTail))
	for i, l := range storedTail {
		storedPlain[i] = StripANSI(string(l))
	}
	tmuxPlain := make([]string, len(tmuxHead))
	for i, l := range tmuxHead {
		tmuxPlain[i] = StripANSI(string(l))
	}

	// Find longest suffix of storedPlain that matches a prefix of tmuxPlain
	matchLen := 0
	for tryLen := min(len(storedPlain), len(tmuxPlain)); tryLen > 0; tryLen-- {
		suffix := storedPlain[len(storedPlain)-tryLen:]
		prefix := tmuxPlain[:tryLen]
		if slicesEqual(suffix, prefix) {
			matchLen = tryLen
			break
		}
	}

	if matchLen == 0 {
		// No overlap — concatenate with marker
		var buf bytes.Buffer
		buf.Write(stored)
		buf.WriteString("--- reconnected ---\n")
		buf.Write(tmuxCapture)
		return buf.Bytes()
	}

	// Splice: all of stored + tmux lines after the overlap
	var buf bytes.Buffer
	buf.Write(stored)
	remaining := tmuxLines[matchLen:]
	for _, line := range remaining {
		buf.Write(line)
	}
	return buf.Bytes()
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			lines = append(lines, data)
			break
		}
		lines = append(lines, data[:idx+1])
		data = data[idx+1:]
	}
	return lines
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimRight(a[i], "\r\n") != strings.TrimRight(b[i], "\r\n") {
			return false
		}
	}
	return true
}
