package guacamole

import (
	"fmt"
	"strconv"
	"strings"
)

// EncodeInstruction encodes one Guacamole protocol instruction.
// Format: "<len>.<opcode>,<len>.<arg>,...;"
func EncodeInstruction(opcode string, args ...string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d.%s", len(opcode), opcode))
	for _, arg := range args {
		b.WriteString(fmt.Sprintf(",%d.%s", len(arg), arg))
	}
	b.WriteByte(';')
	return b.String()
}

// ParseInstruction parses a single Guacamole instruction.
func ParseInstruction(raw string) (string, []string, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimSuffix(trimmed, ";")
	if trimmed == "" {
		return "", nil, fmt.Errorf("empty instruction")
	}

	parts := strings.Split(trimmed, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		dotIdx := strings.IndexByte(part, '.')
		if dotIdx < 0 {
			return "", nil, fmt.Errorf("invalid instruction element: %q", part)
		}
		length, err := strconv.Atoi(part[:dotIdx])
		if err != nil {
			return "", nil, fmt.Errorf("invalid length in %q: %w", part, err)
		}
		value := part[dotIdx+1:]
		if len(value) != length {
			return "", nil, fmt.Errorf("length mismatch for %q", part)
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return "", nil, fmt.Errorf("missing opcode")
	}
	return values[0], values[1:], nil
}
