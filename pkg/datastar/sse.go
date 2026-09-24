package datastar

import (
	"net/http"

	sdk "github.com/starfederation/datastar-go/datastar"
)

// SSECompression selects the compression mode for an SSE stream. The cost of
// compression is per-connection compressor state, which at the 5k-connection
// budget is measured, not hoped — so the mode is an explicit choice per app.
type SSECompression int

const (
	// SSECompressionGzip is the default: gzip at the fastest level, sync-flushed
	// per event. Datastar fragment streams repeat tags, classes and selectors
	// endlessly and the compressor window persists across the whole stream, so
	// every event compresses against everything before it.
	SSECompressionGzip SSECompression = iota

	// SSECompressionLight is the low-memory fallback: gzip in Huffman-only mode,
	// near-zero CPU and ~40-60% savings, a fraction of the window memory.
	SSECompressionLight

	// SSECompressionOff disables compression entirely.
	SSECompressionOff
)

// gzipHuffmanOnly is compress/flate's HuffmanOnly level (-2). It is defined here
// so the light mode does not pull in the flate import surface directly.
const gzipHuffmanOnly = -2

// gzipBestSpeed mirrors compress/flate.BestSpeed (1) — level 1 is the default
// for the gzip mode.
const gzipBestSpeed = 1

// SSE wraps the upstream Datastar server-sent event generator. Only the methods
// Beach handlers need are surfaced; the embedded *sdk.ServerSentEventGenerator
// is exported so the genuinely weird can reach the full SDK.
type SSE struct {
	*sdk.ServerSentEventGenerator
}

// NewSSE upgrades w into a Datastar SSE stream, negotiating gzip from the
// request's Accept-Encoding and sync-flushing the compressor on every event so
// fragments arrive with the latency of an uncompressed stream. no-transform is
// set so proxies leave the encoding alone.
//
// Compression mode defaults to SSECompressionGzip; pass one SSECompression to
// override. Additional SDK options (e.g. context) follow.
func NewSSE(w http.ResponseWriter, r *http.Request, opts ...Option) *SSE {
	cfg := config{mode: SSECompressionGzip}
	for _, o := range opts {
		o(&cfg)
	}

	sdkOpts := make([]sdk.SSEOption, 0, len(cfg.sdkOpts)+1)
	switch cfg.mode {
	case SSECompressionGzip:
		sdkOpts = append(sdkOpts, sdk.WithCompression(
			sdk.WithGzip(sdk.WithGzipLevel(gzipBestSpeed)),
		))
	case SSECompressionLight:
		sdkOpts = append(sdkOpts, sdk.WithCompression(
			sdk.WithGzip(sdk.WithGzipLevel(gzipHuffmanOnly)),
		))
	case SSECompressionOff:
		// no compression option
	}
	sdkOpts = append(sdkOpts, cfg.sdkOpts...)

	gen := sdk.NewSSE(w, r, sdkOpts...)

	// Proxies (Cloudflare tunnels included) must leave SSE encoding alone.
	if cfg.mode != SSECompressionOff {
		w.Header().Set("Cache-Control", "no-cache, no-transform")
	}

	return &SSE{ServerSentEventGenerator: gen}
}

// Ping writes a no-op SSE event and flushes. Stream loops call this on a
// timer so a client that hung up is detected even when the hub is quiet —
// without a write, ctx.Done() may never fire and the handler leaks.
func (s *SSE) Ping() error {
	if s == nil || s.ServerSentEventGenerator == nil {
		return nil
	}
	return s.Send("ping", nil)
}

// config holds resolved NewSSE options.
type config struct {
	mode    SSECompression
	sdkOpts []sdk.SSEOption
}

// Option configures NewSSE.
type Option func(*config)

// WithCompression overrides the default gzip compression mode.
func WithCompression(mode SSECompression) Option {
	return func(c *config) { c.mode = mode }
}

// WithSDKOption threads a raw upstream SDK option through (e.g. a derived
// context for templ values). Escape hatch for the genuinely weird.
func WithSDKOption(o sdk.SSEOption) Option {
	return func(c *config) { c.sdkOpts = append(c.sdkOpts, o) }
}

// IsDatastar reports whether the request was issued by Datastar's fetch-based
// client, by reading the Datastar-Request header it sets on every backend call.
// Dual-purpose PageFuncs branch on this to choose full-document vs. fragment.
func IsDatastar(r *http.Request) bool {
	return r.Header.Get("Datastar-Request") == "true"
}
