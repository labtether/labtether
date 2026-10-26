package dockerpkg

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/labtether/labtether/internal/agentmgr"
)

func TestDockerExecExitFlushesFinalOutputAndClosesBrowser(t *testing.T) {
	for _, phase := range []string{"before_started", "while_reading_input"} {
		t.Run(phase, func(t *testing.T) {
			d, conn, peer, url, finished := newDockerExecHarness(t)
			browser := dialDockerExecBrowser(t, url+"?request=exit")
			start := readDockerExecStart(t, peer)
			if start.Type != agentmgr.MsgDockerExecStart {
				t.Fatalf("expected exec start: %+v", start)
			}
			if phase == "while_reading_input" {
				d.ProcessAgentDockerExecStarted(conn, dockerExecMessage(t, agentmgr.DockerExecStartedData{SessionID: start.ID}))
				if err := browser.WriteMessage(websocket.BinaryMessage, []byte("echo final")); err != nil {
					t.Fatal(err)
				}
				if input := readDockerExecWire(t, peer); input.Type != agentmgr.MsgDockerExecInput {
					t.Fatalf("browser did not reach the input loop: %+v", input)
				}
			}
			for _, data := range []string{"final output\r\n", "\u65e5\u672c\u8a9e \U0001f600\n"} {
				d.ProcessAgentDockerExecData(conn, dockerExecMessage(t, agentmgr.DockerExecDataPayload{
					SessionID: start.ID, Data: base64.StdEncoding.EncodeToString([]byte(data)),
				}))
			}
			d.ProcessAgentDockerExecClosed(conn, dockerExecMessage(t, agentmgr.DockerExecCloseData{SessionID: start.ID, Reason: "exec ended"}))
			readDockerExecOutput(t, browser, "final output\r\n")
			readDockerExecOutput(t, browser, "\u65e5\u672c\u8a9e \U0001f600\n")
			_ = browser.SetReadDeadline(time.Now().Add(3 * time.Second))
			if _, _, err := browser.ReadMessage(); err == nil {
				t.Fatal("browser remained open after exec exit")
			} else if !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				t.Fatalf("clean exec exit did not send a normal close: %v", err)
			}
			waitDockerExecFinished(t, finished, "exit")
			stop := readDockerExecWire(t, peer)
			var stopData agentmgr.DockerExecCloseData
			if err := json.Unmarshal(stop.Data, &stopData); err != nil || stop.Type != agentmgr.MsgDockerExecClose || stop.ID != start.ID || stopData.SessionID != start.ID {
				t.Fatalf("finalizer stopped the wrong exec: %+v", stop)
			}
			if _, exists := d.DockerExecBridges.Load(start.ID); exists {
				t.Fatal("finished exec retained wire dispatch")
			}
			if _, exists := d.dockerExecOwners.Load("public-session"); exists {
				t.Fatal("finished exec retained public ownership")
			}
		})
	}
}

func TestDockerExecOutputOverflowClosesInsteadOfSkippingBytes(t *testing.T) {
	bridge := &DockerExecBridge{OutputCh: make(chan []byte, 1), ClosedCh: make(chan struct{})}
	bridge.TrySendOutput([]byte("prefix"))
	bridge.TrySendOutput([]byte("overflow"))
	bridge.TrySendOutput([]byte("late bytes"))
	select {
	case <-bridge.ClosedCh:
	default:
		t.Fatal("full exec queue silently discarded bytes and kept the stream open")
	}
	if bridge.ReasonOr("") != "container terminal output buffer overflow" {
		t.Fatalf("wrong overflow reason: %q", bridge.ReasonOr(""))
	}
	if output := <-bridge.OutputCh; string(output) != "prefix" {
		t.Fatalf("queued prefix changed: %q", output)
	}
	select {
	case <-bridge.OutputCh:
		t.Fatal("closed exec accepted more output")
	default:
	}
}
