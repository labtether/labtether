package guacamole

import (
	"bufio"
	"io"
	"net"
	"strings"
	"testing"
)

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

func TestParseInstructionHonorsLengthPrefixedDelimiters(t *testing.T) {
	raw := EncodeInstruction("log", "a,b;c")
	opcode, args, err := ParseInstruction(raw)
	if err != nil {
		t.Fatalf("ParseInstruction: %v", err)
	}
	if opcode != "log" || len(args) != 1 || args[0] != "a,b;c" {
		t.Fatalf("opcode=%q args=%v", opcode, args)
	}
}

func TestParseInstructionRejectsTrailingAndOversizedData(t *testing.T) {
	if _, _, err := ParseInstruction("4.args;garbage"); err == nil {
		t.Fatal("expected trailing data to be rejected")
	}
	if _, _, err := ParseInstruction("4.args,1048577."); err == nil {
		t.Fatal("expected oversized element to be rejected")
	}
}

func TestSendConnectUsesAdvertisedArgumentOrder(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}

	result := make(chan error, 1)
	go func() {
		result <- client.SendConnect(
			[]string{"password", "hostname", "future-parameter", "port"},
			map[string]string{
				"hostname": "rdp.example.com",
				"port":     "3389",
				"password": "synthetic-secret",
			},
		)
	}()
	raw, err := io.ReadAll(io.LimitReader(serverConn, int64(len("7.connect,16.synthetic-secret,15.rdp.example.com,0.,4.3389;"))))
	if err != nil {
		t.Fatalf("read instruction: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("SendConnect: %v", err)
	}
	want := "7.connect,16.synthetic-secret,15.rdp.example.com,0.,4.3389;"
	if string(raw) != want {
		t.Fatalf("SendConnect instruction=%q want %q", raw, want)
	}
}

func TestSendConnectNegotiatesAdvertisedProtocolVersion(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}

	result := make(chan error, 1)
	go func() {
		result <- client.SendConnect(
			[]string{"VERSION_9_1_0", "hostname", "port"},
			map[string]string{"hostname": "rdp.example.com", "port": "3389"},
		)
	}()
	want := "7.connect,13.VERSION_1_5_0,15.rdp.example.com,4.3389;"
	raw, err := io.ReadAll(io.LimitReader(serverConn, int64(len(want))))
	if err != nil {
		t.Fatalf("read instruction: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("SendConnect: %v", err)
	}
	if string(raw) != want {
		t.Fatalf("SendConnect instruction=%q want %q", raw, want)
	}
}

func TestSendHandshakeIncludesRequiredPreamble(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}

	result := make(chan error, 1)
	go func() {
		result <- client.SendHandshake(
			[]string{"VERSION_1_5_0", "hostname", "password"},
			map[string]string{"hostname": "192.0.2.10", "password": "synthetic-secret"},
			ClientInformation{
				Width:          1920,
				Height:         1080,
				DPI:            96,
				AudioMIMETypes: []string{"audio/L16", "audio/L8"},
				ImageMIMETypes: []string{"image/png", "image/jpeg"},
				Timezone:       "Australia/Sydney",
				Name:           "LabTether",
			},
		)
	}()
	want := strings.Join([]string{
		"4.size,4.1920,4.1080,2.96;",
		"5.audio,9.audio/L16,8.audio/L8;",
		"5.video;",
		"5.image,9.image/png,10.image/jpeg;",
		"8.timezone,16.Australia/Sydney;",
		"4.name,9.LabTether;",
		"7.connect,13.VERSION_1_5_0,10.192.0.2.10,16.synthetic-secret;",
	}, "")
	raw, err := io.ReadAll(io.LimitReader(serverConn, int64(len(want))))
	if err != nil {
		t.Fatalf("read handshake: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("SendHandshake: %v", err)
	}
	if string(raw) != want {
		t.Fatalf("SendHandshake instructions=%q want %q", raw, want)
	}
}

func TestSendHandshakeLegacyVersionOmitsOptionalInstructions(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()
	client := &Client{conn: clientConn, reader: bufio.NewReader(clientConn)}

	result := make(chan error, 1)
	go func() {
		result <- client.SendHandshake(
			[]string{"VERSION_1_0_0", "hostname"},
			map[string]string{"hostname": "legacy.example.com"},
			ClientInformation{Width: 1024, Height: 768, DPI: 96, Timezone: "UTC", Name: "LabTether"},
		)
	}()
	want := "4.size,4.1024,3.768,2.96;5.audio;5.video;5.image;7.connect,13.VERSION_1_0_0,18.legacy.example.com;"
	raw, err := io.ReadAll(io.LimitReader(serverConn, int64(len(want))))
	if err != nil {
		t.Fatalf("read legacy handshake: %v", err)
	}
	if err := <-result; err != nil {
		t.Fatalf("SendHandshake: %v", err)
	}
	if string(raw) != want {
		t.Fatalf("SendHandshake instructions=%q want %q", raw, want)
	}
}
