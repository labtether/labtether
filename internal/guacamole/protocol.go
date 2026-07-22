package guacamole

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	// MaxInstructionBytes bounds handshake/control instructions accepted from
	// guacd. Interactive framebuffer payloads are relayed without this parser.
	MaxInstructionBytes    = 1024 * 1024
	maxInstructionElements = 4096
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
	reader := bufio.NewReader(strings.NewReader(raw))
	opcode, args, err := readInstruction(reader)
	if err != nil {
		return "", nil, err
	}
	if _, err := reader.ReadByte(); err != io.EOF {
		if err == nil {
			return "", nil, fmt.Errorf("trailing data after instruction")
		}
		return "", nil, err
	}
	return opcode, args, nil
}

func readInstruction(reader *bufio.Reader) (string, []string, error) {
	if reader == nil {
		return "", nil, fmt.Errorf("instruction reader is required")
	}
	values := make([]string, 0, 16)
	totalBytes := 0
	for {
		if len(values) >= maxInstructionElements {
			return "", nil, fmt.Errorf("instruction has too many elements")
		}
		length, prefixBytes, err := readElementLength(reader)
		if err != nil {
			return "", nil, err
		}
		totalBytes += prefixBytes
		if length > MaxInstructionBytes-totalBytes-1 {
			return "", nil, fmt.Errorf("instruction exceeds %d bytes", MaxInstructionBytes)
		}
		value := make([]byte, length)
		if _, err := io.ReadFull(reader, value); err != nil {
			return "", nil, fmt.Errorf("read instruction element: %w", err)
		}
		totalBytes += length
		delimiter, err := reader.ReadByte()
		if err != nil {
			return "", nil, fmt.Errorf("read instruction delimiter: %w", err)
		}
		totalBytes++
		values = append(values, string(value))
		switch delimiter {
		case ';':
			if len(values) == 0 || values[0] == "" {
				return "", nil, fmt.Errorf("missing opcode")
			}
			return values[0], values[1:], nil
		case ',':
			continue
		default:
			return "", nil, fmt.Errorf("invalid instruction delimiter %q", delimiter)
		}
	}
}

func readElementLength(reader *bufio.Reader) (length int, consumed int, err error) {
	digits := 0
	for {
		value, readErr := reader.ReadByte()
		if readErr != nil {
			return 0, consumed, fmt.Errorf("read instruction length: %w", readErr)
		}
		consumed++
		if value == '.' {
			if digits == 0 {
				return 0, consumed, fmt.Errorf("instruction length is missing")
			}
			return length, consumed, nil
		}
		if value < '0' || value > '9' {
			return 0, consumed, fmt.Errorf("invalid instruction length character %q", value)
		}
		digits++
		if digits > 9 {
			return 0, consumed, fmt.Errorf("instruction length is too large")
		}
		length = length*10 + int(value-'0')
		if length > MaxInstructionBytes {
			return 0, consumed, fmt.Errorf("instruction element exceeds %d bytes", MaxInstructionBytes)
		}
	}
}
