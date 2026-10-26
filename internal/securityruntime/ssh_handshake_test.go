package securityruntime

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestSSHHandshakeHonorsCancellationAndDeadline(t *testing.T) {
	for _, explicitCancel := range []bool{true, false} {
		t.Run(map[bool]string{true: "cancel", false: "deadline"}[explicitCancel], func(t *testing.T) {
			client, peer := net.Pipe()
			defer peer.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			if explicitCancel {
				cancel()
			}
			result := make(chan error, 1)
			go func() {
				_, err := HandshakeSSHContext(ctx, client, "test:22", &ssh.ClientConfig{
					User: "test", HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error {
						return errors.New("unexpected host key from silent peer")
					},
				})
				result <- err
			}()
			select {
			case err := <-result:
				if !errors.Is(err, ctx.Err()) {
					t.Fatalf("handshake error=%v context=%v", err, ctx.Err())
				}
			case <-time.After(time.Second):
				_ = client.Close()
				t.Fatal("silent SSH peer kept handshake alive after cancellation")
			}
		})
	}
}

func TestSSHHandshakeTransfersConnectionOwnershipOnSuccess(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		config := &ssh.ServerConfig{NoClientAuth: true}
		config.AddHostKey(signer)
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			t.Errorf("server handshake: %v", err)
			return
		}
		defer server.Close()
		go func() {
			for ch := range channels {
				_ = ch.Reject(ssh.UnknownChannelType, "test only")
			}
		}()
		for req := range requests {
			_ = req.Reply(true, nil)
		}
	}()
	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	client, err := HandshakeSSHContext(ctx, conn, listener.Addr().String(), &ssh.ClientConfig{
		User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey()),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	cancel()
	// Cancelling the dial's context must not kill an established session.
	if ok, _, err := client.SendRequest("keepalive@test", true, nil); err != nil || !ok {
		t.Fatalf("established connection closed with handshake context: ok=%v err=%v", ok, err)
	}
	_ = client.Close()
	<-serverDone
}
