package docker

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestSplitDockerParamList(t *testing.T) {
	got := splitDockerParamList("FOO=bar, BAZ=qux\nQUUX=corge", ",")
	want := []string{"FOO=bar", "BAZ=qux", "QUUX=corge"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d = %q, want %q", i, got[i], want[i])
		}
	}

	got = splitDockerParamList("python -m http.server 8080", " ")
	want = []string{"python", "-m", "http.server", "8080"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestParseDockerPortBindings(t *testing.T) {
	got := parseDockerPortBindings("8080:80,9443:9443/tcp,5353:5353/udp,invalid,9000:")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}

	if got[0].HostPort != "8080" || got[0].ContainerPort != "80" || got[0].Protocol != "tcp" {
		t.Fatalf("binding[0] = %+v, want host=8080 container=80 proto=tcp", got[0])
	}
	if got[1].HostPort != "9443" || got[1].ContainerPort != "9443" || got[1].Protocol != "tcp" {
		t.Fatalf("binding[1] = %+v, want host=9443 container=9443 proto=tcp", got[1])
	}
	if got[2].HostPort != "5353" || got[2].ContainerPort != "5353" || got[2].Protocol != "udp" {
		t.Fatalf("binding[2] = %+v, want host=5353 container=5353 proto=udp", got[2])
	}
}

func TestDecodeDockerLogPayload(t *testing.T) {
	payload := bytes.NewBuffer(nil)
	writeFrame := func(stream byte, data string) {
		header := []byte{stream, 0, 0, 0, 0, 0, 0, 0}
		binary.BigEndian.PutUint32(header[4:], uint32(len(data)))
		_, _ = payload.Write(header)
		_, _ = payload.WriteString(data)
	}

	writeFrame(1, "hello\n")
	writeFrame(2, "world\n")

	got := decodeDockerLogPayload(payload.Bytes())
	if got != "hello\nworld\n" {
		t.Fatalf("decoded payload = %q, want %q", got, "hello\nworld\n")
	}

	plain := "plain text logs"
	if decodeDockerLogPayload([]byte(plain)) != plain {
		t.Fatalf("plain decode mismatch")
	}
}
