package protocols

import (
	"encoding/json"
	"testing"
)

func TestDecodeVNCAndARDConfigTransportOptIn(t *testing.T) {
	vnc, err := DecodeVNCConfig(json.RawMessage(`{"display_number":1,"allow_insecure_vnc":true}`))
	if err != nil || !vnc.AllowInsecureTransport || vnc.DisplayNumber != 1 {
		t.Fatalf("unexpected VNC config=%+v err=%v", vnc, err)
	}
	ard, err := DecodeARDConfig(json.RawMessage(`{"apple_dh":true,"allow_insecure_vnc":true}`))
	if err != nil || !ard.AllowInsecureTransport || !ard.AppleDH {
		t.Fatalf("unexpected ARD config=%+v err=%v", ard, err)
	}
	if _, err := DecodeVNCConfig(json.RawMessage(`{"allow_insecure_transport":true}`)); err == nil {
		t.Fatal("unknown VNC security field was accepted")
	}
}
