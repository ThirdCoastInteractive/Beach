# RFC 05 — WebSocket support (`App.Socket`)

[← docs index](../README.md) · prev: [The ECS pitch](03-ecs.md) · next: [Accessibility](06-accessibility.md)

**Status: shipped.** `pkg/beach/socket.go` + the fourth shape in `routes.go`/`handlers.go`;
tests in `socket_test.go`; demo at driftbottle `/sockets`. Decisions taken at
implementation time are marked ⚑ below.

## Motivation

The sync unit for UI remains HTML fragments over SSE (RFC/architecture 20-sync). Some
applications additionally need a **high-rate, bidirectional, binary** channel that is not
UI sync: live simulation state at 60 Hz, controller/gamepad input upstream, binary
telemetry. SSE is text-framed, unidirectional, and gzip-oriented — wrong tool. The
framework had no path, forcing apps to mount foreign handlers behind `Raw` and
carry their own WS dependency. This RFC adds WebSockets as a first-class fourth
handler shape.

Doctrine update (20-sync.md): **SSE carries hypermedia; WebSocket carries payloads that
are not hypermedia.** A socket never patches the DOM through the framework; if a socket
consumer wants UI updates, the page keeps a normal `Stream` alongside.

## API

A fourth typed shape next to `Page`/`Action`/`Stream`:

```go
// routes.go
func (a *App) Socket(pattern string, h SocketFunc, guards ...Guard) // GET + Upgrade

// handlers.go
type SocketFunc func(c *Ctx, s *Socket) error
```

`SocketFunc` runs on its own goroutine after a successful upgrade. Returning nil closes
with 1000 (normal); returning an error closes with 1011 and logs. Guards run **before**
upgrade (a rejected guard returns plain HTTP, never a half-open socket).

```go
// socket.go
type Socket struct { /* opaque */ }

func (s *Socket) Context() context.Context           // canceled on close/shutdown
func (s *Socket) Read() (typ MsgType, p []byte, err error)   // blocks; ping/pong handled internally
func (s *Socket) Write(typ MsgType, p []byte) error          // serialized; blocks per policy
func (s *Socket) WriteLatest(p []byte)               // mailbox mode: coalesce — only the
                                                     // newest undelivered binary frame is
                                                     // kept; never blocks. For state streams.
func (s *Socket) Close(code int, reason string) error

type MsgType int // MsgText, MsgBinary
```

`WriteLatest` is the load-bearing feature for simulation streams: latest-state-wins
coalescing so a slow client skips frames instead of building a queue. `Write` is for
ordered messages (input acks, control) with a bounded queue.

## Config

```go
// beach.Config gains:
Sockets SocketConfig

type SocketConfig struct {
    MaxMessageBytes int64         // default 1 MiB
    PingInterval    time.Duration // default 20s; framework-owned keepalive
    PongTimeout     time.Duration // default 30s
    WriteQueueLen   int           // default 64 (Write path; WriteLatest is depth-1)
    Origins         []string      // default: same-origin only
}
```

Origin enforcement is same-origin by default (browser WS ignores CORS; the check is
ours). `csp.go` `AllowConnect` already anticipates `ws:`/`wss:` sources — wire the
documented behavior.

## Lifecycle & middleware

- Upgrade happens after the fixed middleware stack, so auth/session context is populated
  in `Ctx` exactly as for other shapes; `requestLog` logs upgrade, close code, and
  connection duration; `recover` maps a handler panic to close 1011.
- Server shutdown: `App.Start`'s graceful shutdown cancels all socket contexts, sends
  close 1001 (going away), waits up to `ShutdownTimeout`.
- One writer goroutine per connection owns the wire; `Write`/`WriteLatest` feed it.
  Reads are the handler's loop via `s.Read()`.

## Dependency decision

Recommended: vendor **`coder/websocket`** (MIT, zero transitive deps, actively
maintained, context-native, supports client-side dialing). It is the smallest correct
implementation available and the client-dial support matters (below).

Alternative if the no-new-deps rule must hold: a hand-rolled `pkg/ws` implementing RFC
6455 server framing (~600 lines: handshake, masking, fragmentation, control frames,
close handshake). Correctness risk is real (fragmentation + close semantics are where
hand-rolled implementations fail); take this path only with a conformance-test suite
(autobahn-testsuite in CI). Decision owner: Henry, at implementation time.

⚑ **Decision: vendored `coder/websocket` v1.8.15** (the recommendation). Zero transitive
deps confirmed in `vendor/modules.txt`; the hand-rolled path was not worth the
conformance risk. The library's `Ping` requires an active reader, so the framework owns
a per-connection read pump — that is what makes keepalive work for write-only handlers,
and it means a peer that floods an app that never `Read`s stalls the pump and is killed
by the keepalive (bounded memory over politeness).

⚑ **Keepalive under saturation.** Control frames cannot jump the TCP queue: when a
consumer is so slow the path is stuffed, the ping sits behind the backlog and the pong
misses `PongTimeout` — the server tears the connection down (1006) and the client
reconnects onto fresh state. This is intended: on a latest-state-wins stream, a consumer
lagging more than the keepalive window is better served by a fresh connection than by a
minutes-old backlog.

## Client dialing (proxy pattern)

Expose the dialer for the relay use-case: an app terminates the public socket via
`App.Socket` (auth, origin, one public surface) and relays frames to a private backend
worker socket:

```go
func DialSocket(ctx context.Context, url string, opts ...DialOpt) (*Socket, error)
func Relay(ctx context.Context, a, b *Socket) error // bidirectional pump,
                                                    // WriteLatest semantics for binary
```

## Not in scope

- No hub↔socket bridge: `hub` remains SSE-only fan-out of pre-rendered fragments.
- No Datastar integration; no DOM patching over WS.
- No message schema/serialization opinions (apps own their frames).
- No reconnection protocol server-side (clients reconnect; apps own resume semantics).

## Client helper (optional, small)

`view/static/js/beach-ws.js`: native `WebSocket` wrapper — `binaryType='arraybuffer'`,
exponential-backoff reconnect, `onFrame` callback, send-queue-while-connecting. No
dependencies; ES module served like the other framework assets.

## Demo & tests

- New example route in `driftbottle` or a new `demos` entry: `/ws/echo` (text echo) and
  `/ws/tick` (60 Hz binary counter via `WriteLatest`) with a page that graphs receive
  rate — doubles as the backpressure/coalescing demonstration.
- Tests: `httptest` + `DialSocket` — upgrade+guards, origin rejection, ping/pong
  keepalive, `MaxMessageBytes` enforcement, coalescing under a stalled reader, graceful
  shutdown close codes, panic → 1011, `Relay` bidirectional pump.

⚑ **Browser reality check** (learned building the demo): a classic browser `WebSocket`
exerts *no* receive backpressure — the network process drains TCP into an unbounded
task queue no matter how stalled the page is, so a busy-tab "slow consumer" never makes
the server coalesce; it just bloats renderer memory until the keepalive declares it
dead. The demo's stall therefore uses `WebSocketStream` (Chromium): pausing reads stops
the socket for real, and ~13 s of stall showed 929 frames coalesced server-side with the
client snapping to fresh state on release. Non-Chromium browsers get the stream without
the stall button. Native/Go consumers (and `DialSocket`) backpressure properly — the
handoff into `Read` is synchronous.
