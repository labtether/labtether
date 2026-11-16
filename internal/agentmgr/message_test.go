package agentmgr

import (
	"encoding/json"
	"errors"
	"testing"
)

// Every build site outside this package goes through NewMessage, so its
// envelope is what agents receive: the type, the envelope ID they echo back,
// the payload bytes, and no data field at all for a message with no payload
// (terminal.probe, webservice.sync).
func TestNewMessageWireShape(t *testing.T) {
	for _, tc := range []struct {
		name, msgType, id string
		payload           any
		want              string
	}{
		{"payload and ID", MsgFileMkdir, "req-1", FileMkdirData{RequestID: "req-1", Path: "/srv"},
			`{"type":"file.mkdir","id":"req-1","data":{"request_id":"req-1","path":"/srv"}}`},
		{"no ID", MsgSSHKeyRemove, "", SSHKeyRemoveData{PublicKey: "ssh-ed25519 EXAMPLE"},
			`{"type":"ssh_key.remove","data":{"public_key":"ssh-ed25519 EXAMPLE"}}`},
		{"no payload", MsgTerminalProbe, "", nil, `{"type":"terminal.probe"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg, err := NewMessage(tc.msgType, tc.id, tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			got, err := json.Marshal(msg)
			if err != nil || string(got) != tc.want {
				t.Fatalf("sent %s (%v), want %s", got, err, tc.want)
			}
		})
	}
}

// Sites that ignored the marshal error sent the envelope without data; the
// others return json.Marshal's error, which some wrap into their own text.
func TestNewMessageEncodeErrorKeepsTheEnvelope(t *testing.T) {
	msg, err := NewMessage(MsgAlertNotify, "id-1", map[string]any{"bad": make(chan int)})
	var unsupported *json.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("err = %v, want json.Marshal's UnsupportedTypeError", err)
	}
	if msg.Type != MsgAlertNotify || msg.ID != "id-1" || msg.Data != nil {
		t.Fatalf("message on error = %+v, want type and ID with no data", msg)
	}
}

// Decode accepts what json.Unmarshal accepted, and DecodeStrictly refuses
// unknown fields, so moving a site keeps which replies it accepts.
func TestDecodeKeepsEachSitesRules(t *testing.T) {
	extra := Message{Type: MsgPowerResult, Data: json.RawMessage(`{"request_id":"r1","new_field":true}`)}
	got, err := Decode[PowerResultData](extra)
	if err != nil || got.RequestID != "r1" {
		t.Fatalf("Decode = %+v, %v; want the reply with the unknown field ignored", got, err)
	}
	if _, err := DecodeStrictly[PowerResultData](extra); err == nil {
		t.Fatal("DecodeStrictly accepted an unknown field")
	}
	if _, err := Decode[PowerResultData](Message{Data: json.RawMessage(`{"request_id":`)}); err == nil {
		t.Fatal("Decode accepted a broken payload")
	}
	// A site that logs and carries on sees what json.Unmarshal left behind.
	partial, err := Decode[FileResultData](Message{Data: json.RawMessage(`{"request_id":"r2","ok":"yes"}`)})
	var typeErr *json.UnmarshalTypeError
	if !errors.As(err, &typeErr) || partial.RequestID != "r2" {
		t.Fatalf("Decode = %+v, %v; want json.Unmarshal's type error and the fields before it", partial, err)
	}
}
