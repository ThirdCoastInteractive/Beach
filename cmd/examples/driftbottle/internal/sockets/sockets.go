// Package sockets is driftbottle's WebSocket demo surface (RFC 05): the
// non-hypermedia channel next door to the SSE fan-out the rest of the app
// benchmarks. Two App.Socket routes — a text echo and a 60 Hz binary tick
// stream pushed with WriteLatest — and a page whose script graphs the receive
// rate. Stalling the page's consumer makes the tick counter skip instead of
// lag: the latest-state-wins coalescing demonstrated live.
package sockets

import (
	"encoding/binary"
	"time"

	beach "github.com/ThirdCoastInteractive/Beach/pkg/beach"
)

// tickFrame lays out one binary tick: a uint32 counter, an int64 server clock
// (unix nanos), padded to 64 KiB. The padding is the demonstration: ~3.8 MB/s
// of production saturates the TCP path within about a second of a stalled
// consumer, the writer blocks, and the WriteLatest mailbox starts skipping —
// with small frames the kernel and browser buffers would swallow minutes of
// backlog before any coalescing showed.
const tickFrameSize = 64 << 10

// Routes registers the demo page and its two sockets.
func Routes(app *beach.App) {
	app.Page("/sockets", page)
	app.Socket("/ws/echo", echoSocket)
	app.Socket("/ws/tick", tickSocket)
}

// page renders the demo document. All socket interaction is page script
// (static/js/sockets-demo.js) — a socket never patches the DOM through the
// framework, so there is nothing Datastar-shaped here.
func page(c *beach.Ctx) (beach.View, error) {
	return beach.View{Page: socketsDocument()}, nil
}

// echoSocket returns every data frame to its sender. The read loop's terminal
// error is the ordinary end of the connection (peer close), so returning it is
// a normal close, not a 1011.
func echoSocket(c *beach.Ctx, s *beach.Socket) error {
	for {
		typ, p, err := s.Read()
		if err != nil {
			return err
		}
		if err := s.Write(typ, p); err != nil {
			return err
		}
	}
}

// tickSocket pushes a 60 Hz binary counter with WriteLatest: a slow consumer
// skips frames instead of building a queue, and the page's counter-gap readout
// makes the skips visible. Write-only — the framework's read pump keeps
// keepalive working with no Read loop here.
func tickSocket(c *beach.Ctx, s *beach.Socket) error {
	t := time.NewTicker(time.Second / 60)
	defer t.Stop()
	buf := make([]byte, tickFrameSize)
	var counter uint32
	for {
		select {
		case <-s.Context().Done():
			return nil
		case <-t.C:
			binary.BigEndian.PutUint32(buf[0:4], counter)
			binary.BigEndian.PutUint64(buf[4:12], uint64(time.Now().UnixNano()))
			counter++
			s.WriteLatest(append([]byte(nil), buf...))
		}
	}
}
