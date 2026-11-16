package agentmgr

import (
	"context"
	"fmt"
	"testing"
)

// Fault: an agent that sends its reply and then disconnects (a reboot does
// this) gets "disconnected" instead of its reply, because the reply and the
// closed connection are both ready when the request starts waiting and the
// wait picks one at random.
func TestRequestPrefersADeliveredReplyOverADisconnect(t *testing.T) {
	for i := 0; i < 64; i++ {
		id := fmt.Sprintf("req-%d", i)
		p := &Pending[testReply]{}
		gone := make(chan struct{})
		rec := newRecorder()
		rec.hook = func(Message) {
			p.Deliver(Match{Key: KeyPayload, Device: DeviceAny}, Reply[testReply]{PayloadID: id, Value: testReply{RequestID: id, Value: "answer"}})
			close(gone)
		}
		call := baseCall(id, rec)
		call.Disconnect = gone
		reply, err := Request(context.Background(), p, call)
		if err != nil || reply.Value != "answer" {
			t.Fatalf("attempt %d: got %+v, %v; want the reply that came before the disconnect", i, reply, err)
		}
	}
}
