package protocols

import (
	"encoding/json"
	"testing"
)

func TestDecodeTelnetConfigTransportOptIn(t *testing.T) {
	cfg, err := DecodeTelnetConfig(json.RawMessage(`{"allow_insecure_telnet":true}`))
	if err != nil || !cfg.AllowInsecureTransport {
		t.Fatalf("unexpected Telnet config=%+v err=%v", cfg, err)
	}
	if _, err := DecodeTelnetConfig(json.RawMessage(`{"allow_insecure_transport":true}`)); err == nil {
		t.Fatal("unknown Telnet security field was accepted")
	}
}
