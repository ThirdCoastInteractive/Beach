package beach

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// WebSocket support: the fourth typed handler shape (RFC 05). SSE carries
// hypermedia; a Socket carries payloads that are not hypermedia — high-rate
// binary state streams, upstream controller input, telemetry. A socket never
// patches the DOM through the framework; a page that also wants UI updates
// keeps a normal Stream alongside.

// MsgType is a WebSocket data-frame type. Values map to the wire opcodes.
type MsgType int

const (
	// MsgText is a UTF-8 text frame.
	MsgText = MsgType(websocket.MessageText)
	// MsgBinary is a binary frame.
	MsgBinary = MsgType(websocket.MessageBinary)
)

// ErrSocketClosed is returned by Socket.Write after the connection has closed.
var ErrSocketClosed = errors.New("beach: socket closed")

// SocketConfig tunes the framework-owned socket machinery. The zero value is
// usable; every field defaults sensibly.
type SocketConfig struct {
	// MaxMessageBytes caps a single incoming message. Default 1 MiB. A peer
	// exceeding it gets close 1009 (message too big).
	MaxMessageBytes int64

	// PingInterval is the framework-owned keepalive cadence. Default 20s.
	PingInterval time.Duration

	// PongTimeout bounds the wait for a pong before the peer is declared dead
	// and the connection torn down. Default 30s.
	PongTimeout time.Duration

	// WriteQueueLen bounds the ordered Write queue; a full queue blocks the
	// caller. Default 64. WriteLatest bypasses this queue (depth-1 mailbox).
	WriteQueueLen int

	// Origins lists additional allowed Origin host patterns (path.Match
	// syntax, e.g. "app.example.com"). Empty means same-origin only — browser
	// WS ignores CORS, so this check is the framework's.
	Origins []string
}

// withDefaults returns cfg with zero fields replaced by the documented defaults.
func (cfg SocketConfig) withDefaults() SocketConfig {
	if cfg.MaxMessageBytes <= 0 {
		cfg.MaxMessageBytes = 1 << 20
	}
	if cfg.PingInterval <= 0 {
		cfg.PingInterval = 20 * time.Second
	}
	if cfg.PongTimeout <= 0 {
		cfg.PongTimeout = 30 * time.Second
	}
	if cfg.WriteQueueLen <= 0 {
		cfg.WriteQueueLen = 64
	}
	return cfg
}

// Socket is an accepted (or dialed) WebSocket connection. The framework owns
// the wire: one writer goroutine serializes all outgoing frames, and a read
// pump keeps a reader active at all times so ping/pong and close frames are
// handled even for a write-only handler. Handlers use Read/Write/WriteLatest;
// they never see the underlying connection.
type Socket struct {
	conn *websocket.Conn
	cfg  SocketConfig

	ctx    context.Context
	cancel context.CancelFunc

	readCh  chan sockMsg
	writeCh chan sockMsg

	// The WriteLatest mailbox: only the newest undelivered binary frame is
	// kept. latestCh (cap 1) wakes the writer; latest holds the frame.
	latestMu sync.Mutex
	latest   []byte
	latestCh chan struct{}

	// The terminal read error (a peer close, a network failure). Stored before
	// cancel so Read can surface the real cause instead of a bare "closed".
	errMu   sync.Mutex
	readErr error

	closeOnce sync.Once
	closeErr  error
	closeCode int // status sent (or 1006 on an abnormal teardown), for the log
}

// sockMsg is one data frame moving through a pump.
type sockMsg struct {
	typ MsgType
	p   []byte
}

// newSocket wraps an upgraded connection with the pump machinery. parent
// carries request values (auth) but not HTTP cancellation — after a hijack the
// request context is unreliable, so lifetime is owned by s.cancel.
func newSocket(parent context.Context, conn *websocket.Conn, cfg SocketConfig) *Socket {
	ctx, cancel := context.WithCancel(parent)
	s := &Socket{
		conn:     conn,
		cfg:      cfg,
		ctx:      ctx,
		cancel:   cancel,
		readCh:   make(chan sockMsg),
		writeCh:  make(chan sockMsg, cfg.WriteQueueLen),
		latestCh: make(chan struct{}, 1),
	}
	conn.SetReadLimit(cfg.MaxMessageBytes)
	go s.readPump()
	go s.writePump()
	go s.keepalive()
	return s
}

// Context returns a context canceled when the socket closes (peer close, write
// failure, keepalive timeout, server shutdown). Push loops select on it.
func (s *Socket) Context() context.Context { return s.ctx }

// Read blocks until the next data message arrives and returns it. Ping, pong
// and close frames are handled internally and never surface here. On close it
// returns the connection's terminal error (a clean peer close is still an
// error, as with io.EOF), or ErrSocketClosed when this side closed first.
func (s *Socket) Read() (MsgType, []byte, error) {
	// Drain a delivered message before honoring cancellation, so a frame that
	// raced the close is not lost.
	select {
	case m := <-s.readCh:
		return m.typ, m.p, nil
	default:
	}
	select {
	case m := <-s.readCh:
		return m.typ, m.p, nil
	case <-s.ctx.Done():
		return 0, nil, s.terminalErr()
	}
}

// setReadErr records the first terminal read error.
func (s *Socket) setReadErr(err error) {
	s.errMu.Lock()
	if s.readErr == nil {
		s.readErr = err
	}
	s.errMu.Unlock()
}

// terminalErr returns the recorded terminal error, or ErrSocketClosed when the
// socket died without one (this side closed, writer failed, keepalive timeout).
func (s *Socket) terminalErr() error {
	s.errMu.Lock()
	defer s.errMu.Unlock()
	if s.readErr != nil {
		return s.readErr
	}
	return ErrSocketClosed
}

// Write enqueues an ordered frame (input acks, control messages). It blocks
// while the bounded queue is full and returns ErrSocketClosed once the socket
// is down. Delivery is serialized with WriteLatest frames by the single writer.
func (s *Socket) Write(typ MsgType, p []byte) error {
	select {
	case s.writeCh <- sockMsg{typ: typ, p: p}:
		return nil
	case <-s.ctx.Done():
		return ErrSocketClosed
	}
}

// WriteLatest offers a binary frame in latest-state-wins mode: only the newest
// undelivered frame is kept, so a slow client skips frames instead of building
// a queue. It never blocks. This is the shape for simulation state streams.
func (s *Socket) WriteLatest(p []byte) {
	s.latestMu.Lock()
	s.latest = p
	s.latestMu.Unlock()
	select {
	case s.latestCh <- struct{}{}:
	default:
	}
}

// Close performs the close handshake with the given status code (e.g. 1000 for
// normal) and reason, then tears the socket down. Subsequent closes are no-ops
// returning the first result.
func (s *Socket) Close(code int, reason string) error {
	s.closeOnce.Do(func() {
		s.closeCode = code
		s.closeErr = s.conn.Close(websocket.StatusCode(code), reason)
		s.cancel()
	})
	return s.closeErr
}

// teardown force-closes the connection and stops the pumps. It is the
// unconditional cleanup path (write failure, keepalive timeout, final defer);
// polite closes go through Close first, in which case this only cleans up.
func (s *Socket) teardown() {
	s.closeOnce.Do(func() {
		s.closeCode = 1006 // abnormal closure: no close frame was sent
	})
	s.cancel()
	_ = s.conn.CloseNow()
}

// readPump keeps the library reader active for the connection's lifetime so
// control frames (ping/pong/close) are always processed, and hands data
// messages to Read. If the handler stops reading while the peer floods, the
// pump blocks handing off a message, control frames stall, and the keepalive
// declares the connection dead — bounded memory over politeness.
func (s *Socket) readPump() {
	for {
		typ, p, err := s.conn.Read(s.ctx)
		if err != nil {
			// Record the cause, then cancel — so a write-only handler's
			// s.Context() ends promptly on peer close and a blocked Read
			// surfaces the real error.
			s.setReadErr(err)
			s.cancel()
			return
		}
		select {
		case s.readCh <- sockMsg{typ: MsgType(typ), p: p}:
		case <-s.ctx.Done():
			return
		}
	}
}

// writePump is the single writer goroutine that owns the wire. It drains the
// ordered queue and the WriteLatest mailbox; a write failure kills the socket.
func (s *Socket) writePump() {
	for {
		select {
		case <-s.ctx.Done():
			return
		case m := <-s.writeCh:
			if s.conn.Write(s.ctx, websocket.MessageType(m.typ), m.p) != nil {
				s.cancel()
				return
			}
		case <-s.latestCh:
			s.latestMu.Lock()
			p := s.latest
			s.latest = nil
			s.latestMu.Unlock()
			if p == nil {
				continue
			}
			if s.conn.Write(s.ctx, websocket.MessageBinary, p) != nil {
				s.cancel()
				return
			}
		}
	}
}

// keepalive pings on PingInterval and declares the peer dead when no pong
// lands within PongTimeout. The read pump processes the pongs, so keepalive
// works for write-only handlers too.
func (s *Socket) keepalive() {
	t := time.NewTicker(s.cfg.PingInterval)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(s.ctx, s.cfg.PongTimeout)
			err := s.conn.Ping(ctx)
			cancel()
			if err != nil {
				s.teardown()
				return
			}
		}
	}
}

// socketCloseIsNormal classifies a SocketFunc's returned error: a peer close
// (any close status), a canceled context, or a torn-down connection is the
// ordinary end of a socket's life, not a handler failure. Handlers can return
// their read loop's terminal error without tripping the 1011 path.
func socketCloseIsNormal(err error) bool {
	if err == nil {
		return true
	}
	if websocket.CloseStatus(err) != -1 {
		return true
	}
	return errors.Is(err, ErrSocketClosed) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF)
}

// --- server lifecycle ---

// socketRegistry tracks live sockets for graceful shutdown. Hijacked
// connections are invisible to http.Server.Shutdown, so the app closes them
// itself: 1001 (going away) to every peer, then wait for handlers to return.
type socketRegistry struct {
	mu  sync.Mutex
	set map[*Socket]struct{}
	wg  sync.WaitGroup
}

func (r *socketRegistry) add(s *Socket) {
	r.mu.Lock()
	if r.set == nil {
		r.set = make(map[*Socket]struct{})
	}
	r.set[s] = struct{}{}
	r.mu.Unlock()
	r.wg.Add(1)
}

func (r *socketRegistry) remove(s *Socket) {
	r.mu.Lock()
	delete(r.set, s)
	r.mu.Unlock()
	r.wg.Done()
}

// closeAll sends close 1001 to every live socket and cancels its context,
// unblocking handler loops.
func (r *socketRegistry) closeAll() {
	r.mu.Lock()
	live := make([]*Socket, 0, len(r.set))
	for s := range r.set {
		live = append(live, s)
	}
	r.mu.Unlock()
	for _, s := range live {
		_ = s.Close(int(websocket.StatusGoingAway), "server shutting down")
	}
}

// wait blocks until every socket handler has returned or ctx expires.
func (r *socketRegistry) wait(ctx context.Context) {
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// --- client dialing (proxy pattern) ---

// DialOpt configures DialSocket.
type DialOpt func(*dialConfig)

type dialConfig struct {
	header http.Header
	socket SocketConfig
}

// WithDialHeader sets extra HTTP headers on the handshake request (an Origin,
// a bearer token for a TokenAuthed backend).
func WithDialHeader(h http.Header) DialOpt {
	return func(dc *dialConfig) { dc.header = h }
}

// WithDialSocketConfig overrides the SocketConfig used for the dialed side
// (keepalive cadence, queue depth). The zero-value default applies otherwise.
func WithDialSocketConfig(cfg SocketConfig) DialOpt {
	return func(dc *dialConfig) { dc.socket = cfg }
}

// DialSocket dials a WebSocket URL (ws:// or wss://) and returns the same
// *Socket the server side gets, pumps and keepalive included. It backs the
// relay pattern — an app terminates the public socket via App.Socket and
// relays frames to a private backend worker socket — and the test suite.
// The caller owns the socket and must Close it.
func DialSocket(ctx context.Context, url string, opts ...DialOpt) (*Socket, error) {
	var dc dialConfig
	for _, o := range opts {
		o(&dc)
	}
	conn, resp, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: dc.header})
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	// The dial context bounds the handshake only; the socket's own lifetime is
	// owned by its cancel, like the server side.
	return newSocket(context.WithoutCancel(ctx), conn, dc.socket.withDefaults()), nil
}

// Relay pumps frames bidirectionally between two sockets until either side
// closes or ctx is canceled. Binary frames ride WriteLatest (latest-state-wins,
// matching the simulation-stream contract); text frames ride the ordered
// queue. A clean close on either side returns nil; both sockets are torn down
// on return.
func Relay(ctx context.Context, a, b *Socket) error {
	errc := make(chan error, 2)
	pump := func(src, dst *Socket) {
		for {
			typ, p, err := src.Read()
			if err != nil {
				errc <- err
				return
			}
			if typ == MsgBinary {
				dst.WriteLatest(p)
				continue
			}
			if err := dst.Write(typ, p); err != nil {
				errc <- err
				return
			}
		}
	}
	go pump(a, b)
	go pump(b, a)

	var err error
	select {
	case err = <-errc:
	case <-ctx.Done():
		err = ctx.Err()
	}
	// Tearing both down unblocks the surviving pump goroutine.
	a.teardown()
	b.teardown()
	if socketCloseIsNormal(err) {
		return nil
	}
	return err
}
