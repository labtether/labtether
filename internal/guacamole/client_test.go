package guacamole

import "testing"

func TestEncodeInstruction(t *testing.T) {
	got := EncodeInstruction("select", "rdp")
	want := "6.select,3.rdp;"
	if got != want {
		t.Fatalf("EncodeInstruction mismatch: got %q want %q", got, want)
	}
}

func TestEncodeInstructionMultiArg(t *testing.T) {
	got := EncodeInstruction("connect", "hostname", "password")
	want := "7.connect,8.hostname,8.password;"
	if got != want {
		t.Fatalf("EncodeInstruction mismatch: got %q want %q", got, want)
	}
}

func TestParseInstruction(t *testing.T) {
	opcode, args, err := ParseInstruction("6.select,3.rdp;")
	if err != nil {
		t.Fatalf("ParseInstruction: %v", err)
	}
	if opcode != "select" {
		t.Fatalf("opcode=%q want select", opcode)
	}
	if len(args) != 1 || args[0] != "rdp" {
		t.Fatalf("args=%v", args)
	}
}
