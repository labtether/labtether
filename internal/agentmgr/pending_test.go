package agentmgr

import (
	"context"
	"testing"
)

// Each case registers one waiter for "req-1" and offers one reply.
func checkDelivery(t *testing.T, call Call[testRequest, testReply], m Match, reply Reply[testReply]) bool {
	t.Helper()
	p := &Pending[testReply]{}
	done, stop := waitOn(t, p, call)
	if deliverWithin(t, p, m, reply) {
		if got := wait(t, done); got.err != nil || got.reply != reply.Value {
			t.Fatalf("Deliver reported true but the request ended with %+v", got)
		}
		return true
	}
	if got := stop(); RequestKind(got.err) != RequestCanceled {
		t.Fatalf("Deliver reported false but the request ended with %+v", got)
	}
	return false
}

// Fault: a key rule matches on the wrong ID, so a reply with a stale or
// foreign envelope or payload ID completes a request (or a valid one is lost).
func TestMatchKeyRules(t *testing.T) {
	cases := []struct {
		rule     KeyRule
		envelope string
		payload  string
		want     bool
	}{
		{KeyPayload, "req-1", "req-1", true},
		{KeyPayload, "other", "req-1", true},
		{KeyPayload, "", "req-1", true},
		{KeyPayload, "req-1", "other", false},
		{KeyPayload, "req-1", "", false},
		{KeyEnvelope, "req-1", "", true},
		{KeyEnvelope, "req-1", "other", true},
		{KeyEnvelope, "other", "req-1", false},
		{KeyEnvelope, "", "req-1", false},
		{KeyPayloadEnvelopeIfSet, "req-1", "req-1", true},
		{KeyPayloadEnvelopeIfSet, "", "req-1", true},
		{KeyPayloadEnvelopeIfSet, "other", "req-1", false},
		{KeyPayloadEnvelopeIfSet, "req-1", "other", false},
		{KeyBoth, "req-1", "req-1", true},
		{KeyBoth, "", "req-1", false},
		{KeyBoth, "other", "req-1", false},
		{KeyBoth, "req-1", "", false},
	}
	for _, tc := range cases {
		reply := Reply[testReply]{EnvelopeID: tc.envelope, PayloadID: tc.payload, Value: testReply{Value: "v"}}
		got := checkDelivery(t, baseCall("req-1", nil), Match{Key: tc.rule, Device: DeviceAny}, reply)
		if got != tc.want {
			t.Errorf("rule %d envelope %q payload %q: delivered=%v, want %v", tc.rule, tc.envelope, tc.payload, got, tc.want)
		}
	}
}

// Fault: TrimKey is ignored (process list and package replies trim their ID
// today) or applied when not asked (disk, cron and others do not trim).
func TestMatchTrimKey(t *testing.T) {
	padded := Reply[testReply]{EnvelopeID: " req-1 ", PayloadID: " req-1 "}
	for _, rule := range []KeyRule{KeyPayload, KeyEnvelope, KeyPayloadEnvelopeIfSet, KeyBoth} {
		if !checkDelivery(t, baseCall("req-1", nil), Match{Key: rule, TrimKey: true}, padded) {
			t.Errorf("rule %d with TrimKey: padded key not delivered", rule)
		}
		if checkDelivery(t, baseCall("req-1", nil), Match{Key: rule}, padded) {
			t.Errorf("rule %d without TrimKey: padded key delivered", rule)
		}
	}
	mixed := Reply[testReply]{EnvelopeID: "req-1", PayloadID: " req-1"}
	if !checkDelivery(t, baseCall("req-1", nil), Match{Key: KeyPayloadEnvelopeIfSet, TrimKey: true}, mixed) {
		t.Error("TrimKey must trim both IDs before comparing them")
	}
}

// Fault: a device rule accepts a reply from another device, or refuses the
// right one. Each existing site's rule is one row: disk/cron/users fold and
// accept no connection; journal, service, network, package, display and
// process fold and refuse it; clipboard trims and compares exactly; power and
// the Docker test compare exactly; the desktop diagnostic checks nothing.
func TestMatchDeviceRules(t *testing.T) {
	conns := []struct {
		name string
		conn *AgentConn
	}{
		{"nas01", &AgentConn{AssetID: "nas01"}},
		{"NAS01", &AgentConn{AssetID: "NAS01"}},
		{"padded", &AgentConn{AssetID: " nas01 "}},
		{"nas02", &AgentConn{AssetID: "nas02"}},
		{"no conn", nil},
	}
	cases := []struct {
		name  string
		match Match
		want  []bool // per conns entry
	}{
		{"any", Match{Device: DeviceAny}, []bool{true, true, true, true, true}},
		{"fold, no conn accepted", Match{Device: DeviceFold, AcceptNoConn: true}, []bool{true, true, true, false, true}},
		{"fold, no conn refused", Match{Device: DeviceFold}, []bool{true, true, true, false, false}},
		{"exact trim", Match{Device: DeviceExactTrim}, []bool{true, false, true, false, false}},
		{"exact", Match{Device: DeviceExact}, []bool{true, false, false, false, false}},
	}
	for _, tc := range cases {
		for i, c := range conns {
			got := checkDelivery(t, baseCall("req-1", nil), tc.match, payloadReply("req-1", "v", c.conn))
			if got != tc.want[i] {
				t.Errorf("%s, reply from %s: delivered=%v, want %v", tc.name, c.name, got, tc.want[i])
			}
		}
	}
	// A waiter that expects no device skips the comparison, but a reply with
	// no connection still follows AcceptNoConn.
	call := baseCall("req-1", nil)
	call.Device = ""
	if !checkDelivery(t, call, Match{Device: DeviceExact}, payloadReply("req-1", "v", &AgentConn{AssetID: "nas02"})) {
		t.Error("a waiter without a device refused a reply")
	}
	if checkDelivery(t, call, Match{Device: DeviceExact}, payloadReply("req-1", "v", nil)) {
		t.Error("a reply with no connection passed without AcceptNoConn")
	}
}

// Fault: a reply from another connection of the same device (a reconnect)
// completes a request that must stay on its own socket.
func TestMatchExactConn(t *testing.T) {
	sent := &AgentConn{AssetID: "nas01"}
	call := baseCall("req-1", nil)
	call.ExactConn = sent
	m := Match{Key: KeyBoth, Device: DeviceExact}
	if checkDelivery(t, call, m, payloadReply("req-1", "v", &AgentConn{AssetID: "nas01"})) {
		t.Error("reply from another connection was delivered")
	}
	if checkDelivery(t, call, m, payloadReply("req-1", "v", nil)) {
		t.Error("reply with no connection was delivered")
	}
	if !checkDelivery(t, call, m, payloadReply("req-1", "v", sent)) {
		t.Error("reply from the exact connection was refused")
	}
}

// Fault: Accept is skipped, or a refused reply removes or uses up the waiter,
// so a later valid reply is lost.
func TestMatchAcceptKeepsWaiterOnRefusal(t *testing.T) {
	p := &Pending[testReply]{}
	call := baseCall("req-1", nil)
	call.Accept = func(r testReply) bool { return r.Value == "good" }
	done, _ := waitOn(t, p, call)
	if deliverWithin(t, p, anyDevice, payloadReply("req-1", "bad", nil)) {
		t.Fatal("a reply Accept refused was delivered")
	}
	if _, ok := p.Load("req-1"); !ok {
		t.Fatal("a refused reply removed the waiter")
	}
	if !deliverWithin(t, p, anyDevice, payloadReply("req-1", "good", nil)) {
		t.Fatal("the valid reply after a refused one was not delivered")
	}
	if got := wait(t, done); got.err != nil || got.reply.Value != "good" {
		t.Fatalf("request returned %+v", got)
	}
}

// Fault: a reserved key is reachable by a reply, clashes with a reply ID,
// or outlives the request. The package preview keeps its logical job key in
// a separate key type today, so a reply can never reach it.
func TestReservedKeyIsHeldButNeverDelivered(t *testing.T) {
	p := &Pending[testReply]{}
	call := baseCall("job-1", nil)
	call.Reserve = []string{"job-1"} // the preview uses the job ID for both when the attempt is not private
	done, _ := waitOn(t, p, call)
	if p.Len() != 2 {
		t.Fatalf("%d keys registered, want 2", p.Len())
	}
	if _, err := Request(context.Background(), p, func() Call[testRequest, testReply] {
		c := baseCall("job-2", newRecorder())
		c.Reserve = []string{"job-1"}
		return c
	}()); RequestKind(err) != RequestDuplicate {
		t.Fatalf("second holder of the reserved key got %v, want duplicate", err)
	}
	other := baseCall("wire-9", nil)
	other.Reserve = []string{"logical-9"}
	_, stopOther := waitOn(t, p, other)
	if deliverWithin(t, p, anyDevice, payloadReply("logical-9", "v", nil)) {
		t.Fatal("a reply reached a reserved key")
	}
	stopOther()
	if !deliverWithin(t, p, anyDevice, payloadReply("job-1", "v", nil)) {
		t.Fatal("the reply ID was not delivered next to its reserved twin")
	}
	wait(t, done)
	if p.Len() != 0 {
		t.Fatalf("%d keys left after the requests ended", p.Len())
	}
}

// Fault: strict decoding accepts unknown fields or a second object (power
// control refuses both), or lenient decoding refuses unknown fields (every
// simple site accepts them today).
func TestDecodeDataModes(t *testing.T) {
	cases := []struct {
		name string
		data string
		mode DecodeMode
		ok   bool
	}{
		{"strict exact", `{"request_id":"r","value":"v"}`, DecodeStrict, true},
		{"strict unknown field", `{"request_id":"r","extra":true}`, DecodeStrict, false},
		{"strict second object", `{"request_id":"r"}{"request_id":"s"}`, DecodeStrict, false},
		{"lenient unknown field", `{"request_id":"r","extra":true}`, DecodeLenient, true},
		{"lenient second object", `{"request_id":"r"}{"request_id":"s"}`, DecodeLenient, false},
	}
	for _, tc := range cases {
		var got testReply
		err := DecodeData([]byte(tc.data), &got, tc.mode)
		if (err == nil) != tc.ok {
			t.Errorf("%s: err=%v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}
