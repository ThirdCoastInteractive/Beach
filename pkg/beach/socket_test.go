package beach

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// wsURL rewrites an httptest server URL to the ws scheme.
func wsURL(t *testing.T, srv *httptest.Server, path string) string {
	t.Helper()
	return "ws" + strings.TrimPrefix(srv.URL, "http") + path
}

// dialT dials a socket and registers cleanup.
func dialT(t *testing.T, url string, opts ...DialOpt) *Socket {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := DialSocket(ctx, url, opts...)
	if err != nil {
		t.Fatalf("DialSocket(%s): %v", url, err)
	}
	t.Cleanup(s.teardown)
	return s
}

func TestSocketEcho(t *testing.T) {
	a, _ := testApp(t)
	a.Socket("/ws/echo", func(c *Ctx, s *Socket) error {
		for {
			typ, p, err := s.Read()
			if err != nil {
				return err
			}
			if err := s.Write(typ, p); err != nil {
				return err
			}
		}
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	s := dialT(t, wsURL(t, srv, "/ws/echo"))
	if err := s.Write(MsgText, []byte("hello beach")); err != nil {
		t.Fatalf("write: %v", err)
	}
	typ, p, err := s.Read()
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if typ != MsgText || string(p) != "hello beach" {
		t.Fatalf("echo = (%v, %q), want (MsgText, hello beach)", typ, p)
	}
	if err := s.Close(1000, ""); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestSocketGuardRejectsBeforeUpgrade(t *testing.T) {
	a, _ := testApp(t)
	a.Socket("/ws/private", func(c *Ctx, s *Socket) error {
		t.Error("handler ran despite failing guard")
		return nil
	}, a.Authed())
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := DialSocket(ctx, wsURL(t, srv, "/ws/private"))
	if err == nil {
		t.Fatal("dial succeeded, want plain-HTTP 401 rejection")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Fatalf("dial error = %v, want 401 rejection", err)
	}
}

func TestSocketOriginRejected(t *testing.T) {
	a, _ := testApp(t)
	a.Socket("/ws/echo", func(c *Ctx, s *Socket) error { return nil })
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	h := http.Header{}
	h.Set("Origin", "https://evil.example")
	_, err := DialSocket(ctx, wsURL(t, srv, "/ws/echo"), WithDialHeader(h))
	if err == nil {
		t.Fatal("cross-origin dial succeeded, want 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Fatalf("dial error = %v, want 403 rejection", err)
	}
}

func TestSocketOriginAllowlist(t *testing.T) {
	a := New(Config{Service: "test", Sockets: SocketConfig{Origins: []string{"app.example.com"}}})
	a.Socket("/ws/echo", func(c *Ctx, s *Socket) error {
		<-s.Context().Done()
		return nil
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	h := http.Header{}
	h.Set("Origin", "https://app.example.com")
	s := dialT(t, wsURL(t, srv, "/ws/echo"), WithDialHeader(h))
	s.teardown()
}

func TestSocketMaxMessageBytes(t *testing.T) {
	a := New(Config{Service: "test", Sockets: SocketConfig{MaxMessageBytes: 128}})
	a.Socket("/ws/echo", func(c *Ctx, s *Socket) error {
		_, _, err := s.Read()
		return err
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	s := dialT(t, wsURL(t, srv, "/ws/echo"))
	if err := s.Write(MsgBinary, make([]byte, 4096)); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := s.Read()
	if err == nil {
		t.Fatal("read succeeded, want close after oversize message")
	}
	if got := websocket.CloseStatus(err); got != websocket.StatusMessageTooBig {
		t.Fatalf("close status = %v (%v), want 1009 message too big", got, err)
	}
}

// TestSocketWriteLatestCoalesces is the backpressure demonstration: a slow
// consumer of a fast latest-state stream skips frames instead of building a
// queue, and still ends on the newest state.
func TestSocketWriteLatestCoalesces(t *testing.T) {
	const frames = 500
	const frameSize = 32 << 10

	a := New(Config{Service: "test", Sockets: SocketConfig{MaxMessageBytes: frameSize + 16}})
	a.Socket("/ws/tick", func(c *Ctx, s *Socket) error {
		buf := make([]byte, frameSize)
		for i := 0; i < frames; i++ {
			binary.BigEndian.PutUint32(buf, uint32(i))
			s.WriteLatest(append([]byte(nil), buf...))
		}
		// Keep the connection open until the client has caught up and closes.
		<-s.Context().Done()
		return nil
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	s := dialT(t, wsURL(t, srv, "/ws/tick"), WithDialSocketConfig(SocketConfig{MaxMessageBytes: frameSize + 16}))

	received := 0
	last := -1
	deadline := time.After(10 * time.Second)
	for last != frames-1 {
		select {
		case <-deadline:
			t.Fatalf("timed out; received %d frames, last %d", received, last)
		default:
		}
		_, p, err := s.Read()
		if err != nil {
			t.Fatalf("read after %d frames (last %d): %v", received, last, err)
		}
		received++
		last = int(binary.BigEndian.Uint32(p))
		time.Sleep(2 * time.Millisecond) // the deliberately slow consumer
	}
	if received >= frames/2 {
		t.Fatalf("received %d of %d frames — coalescing did not kick in", received, frames)
	}
	t.Logf("coalesced %d frames into %d deliveries", frames, received)
}

// TestSocketKeepaliveKillsDeafPeer: a peer that never reads never processes
// pings, so the framework declares it dead within PingInterval+PongTimeout and
// unblocks the handler.
func TestSocketKeepaliveKillsDeafPeer(t *testing.T) {
	a := New(Config{Service: "test", Sockets: SocketConfig{
		PingInterval: 30 * time.Millisecond,
		PongTimeout:  60 * time.Millisecond,
	}})
	done := make(chan error, 1)
	a.Socket("/ws/push", func(c *Ctx, s *Socket) error {
		<-s.Context().Done()
		done <- s.terminalErr()
		return nil
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	// A raw library client with no reader: pongs are only processed by an
	// active reader, so this peer is deaf to keepalive.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL(t, srv, "/ws/push"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.CloseNow()

	select {
	case <-done:
		// The handler unblocked: the framework tore the connection down.
	case <-time.After(3 * time.Second):
		t.Fatal("keepalive did not kill the deaf peer")
	}
}

func TestSocketShutdownGoingAway(t *testing.T) {
	a, _ := testApp(t)
	handlerDone := make(chan struct{})
	a.Socket("/ws/push", func(c *Ctx, s *Socket) error {
		defer close(handlerDone)
		<-s.Context().Done()
		return nil
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	s := dialT(t, wsURL(t, srv, "/ws/push"))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := a.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	select {
	case <-handlerDone:
	default:
		t.Fatal("Shutdown returned before the socket handler")
	}

	_, _, err := s.Read()
	if got := websocket.CloseStatus(err); got != websocket.StatusGoingAway {
		t.Fatalf("close status = %v (%v), want 1001 going away", got, err)
	}
}

func TestSocketPanicCloses1011(t *testing.T) {
	a, _ := testApp(t)
	a.Socket("/ws/boom", func(c *Ctx, s *Socket) error {
		panic("kaboom")
	})
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()

	s := dialT(t, wsURL(t, srv, "/ws/boom"))
	_, _, err := s.Read()
	if got := websocket.CloseStatus(err); got != websocket.StatusInternalError {
		t.Fatalf("close status = %v (%v), want 1011 internal error", got, err)
	}
}

// TestSocketRelay pumps a client through a front app relaying to a backend
// echo worker: client -> front -> backend -> front -> client.
func TestSocketRelay(t *testing.T) {
	backend, _ := testApp(t)
	backend.Socket("/worker", func(c *Ctx, s *Socket) error {
		for {
			typ, p, err := s.Read()
			if err != nil {
				return err
			}
			if typ == MsgBinary {
				s.WriteLatest(p)
				continue
			}
			if err := s.Write(typ, p); err != nil {
				return err
			}
		}
	})
	backendSrv := httptest.NewServer(backend.Handler())
	defer backendSrv.Close()

	front, _ := testApp(t)
	front.Socket("/ws/relay", func(c *Ctx, s *Socket) error {
		worker, err := DialSocket(s.Context(), wsURL(t, backendSrv, "/worker"))
		if err != nil {
			return err
		}
		return Relay(s.Context(), s, worker)
	})
	frontSrv := httptest.NewServer(front.Handler())
	defer frontSrv.Close()

	s := dialT(t, wsURL(t, frontSrv, "/ws/relay"))

	if err := s.Write(MsgText, []byte("through the relay")); err != nil {
		t.Fatalf("write text: %v", err)
	}
	typ, p, err := s.Read()
	if err != nil {
		t.Fatalf("read text echo: %v", err)
	}
	if typ != MsgText || string(p) != "through the relay" {
		t.Fatalf("text echo = (%v, %q)", typ, p)
	}

	if err := s.Write(MsgBinary, []byte{1, 2, 3, 4}); err != nil {
		t.Fatalf("write binary: %v", err)
	}
	typ, p, err = s.Read()
	if err != nil {
		t.Fatalf("read binary echo: %v", err)
	}
	if typ != MsgBinary || len(p) != 4 || p[0] != 1 || p[3] != 4 {
		t.Fatalf("binary echo = (%v, %v)", typ, p)
	}

	if err := s.Close(1000, ""); err != nil {
		t.Fatalf("close: %v", err)
	}
}
