package beach

import (
	"encoding/json"
	"html"

	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/Beach/pkg/hub"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
	sdk "github.com/starfederation/datastar-go/datastar"
)

// SSEMode re-exports the datastar package's compression mode so an app names a
// single beach.SSE* constant in Config rather than reaching across packages.
type SSEMode = datastar.SSECompression

const (
	// SSECompressionGzip is the default: gzip at the fastest level, sync-flushed
	// per event.
	SSECompressionGzip SSEMode = datastar.SSECompressionGzip
	// SSECompressionLight is the low-memory Huffman-only fallback.
	SSECompressionLight SSEMode = datastar.SSECompressionLight
	// SSECompressionOff disables compression.
	SSECompressionOff SSEMode = datastar.SSECompressionOff
)

// Patcher is the narrow interface a StreamFunc's CatchUp receives: it can patch a
// fragment into a target. It is satisfied by the live SSE stream, so a catch-up
// replay writes through the same compressed connection as live events. The
// interface keeps catch-up code from depending on the whole SSE surface.
type Patcher interface {
	// Patch renders a single Patch onto the stream.
	Patch(p Patch) error
}

// Sub is what a StreamFunc returns: a declaration of the hub topics to subscribe
// to and an optional catch-up replay. The framework owns the SSE loop —
// subscribe, run the since-cursor catch-up, select over the hub channel and the
// request context, flush per event, clean up — so a stream handler is a
// declaration, not 200 lines of plumbing.
type Sub struct {
	// Topics are the hub topics this stream subscribes to. Events published to any
	// of them are written to the client as they arrive.
	Topics []string

	// CatchUp, when non-nil, runs once after subscribe and before the live loop.
	// It replays missed patches given the client's ?since= cursor (read by the
	// handler from c.Query("since")), writing through the same Patcher the live
	// loop uses. This is the completeness half of the liveness/completeness split.
	CatchUp func(since string, p Patcher) error

	// Compression overrides the app-default SSE compression mode for this stream.
	// nil uses the app default.
	Compression *SSEMode
}

// streamPatcher adapts a live *datastar.SSE into a Patcher so CatchUp and the
// live loop share one patch path.
type streamPatcher struct {
	sse *datastar.SSE
}

// Patch renders p onto the SSE stream, honoring Target, Mode, Signals, Script,
// Announce and Redirect.
func (sp streamPatcher) Patch(p Patch) error {
	if p.Mode == PatchRemove && p.Target != "" {
		return sp.sse.RemoveElement("#" + p.Target)
	}
	// Announce first: the message describes the change, and appending it before
	// the fragment lands means the live region is updated even if rendering the
	// fragment fails.
	if p.Announce != "" {
		if err := sp.announce(p.Announce); err != nil {
			return err
		}
	}
	if p.Signals != nil {
		if err := sp.sse.MarshalAndPatchSignals(p.Signals); err != nil {
			return err
		}
	}
	if p.Script != "" {
		if err := sp.sse.ExecuteScript(p.Script); err != nil {
			return err
		}
	}
	if p.Fragment != nil {
		opts := patchOptions(p.Target, p.Mode)
		if p.ViewTransition {
			opts = append(opts, sdk.WithViewTransitions())
		}
		if err := sp.sse.PatchElementTempl(p.Fragment, opts...); err != nil {
			return err
		}
	}
	// Redirect last: any fragment/signal patches in the same Patch are flushed
	// before the client navigates away.
	if p.Redirect != "" {
		return sp.redirect(p.Redirect)
	}
	return nil
}

// announce appends a screen-reader message into the page's live region. The
// region is server-rendered and empty; appending into it is what makes the
// message announced rather than merely present.
func (sp streamPatcher) announce(msg string) error {
	return sp.sse.PatchElementTempl(
		driftwood.Announcement(msg),
		patchOptions(ToastTarget, PatchAppend)...,
	)
}

// redirect emits a CSP-safe client navigation. The SDK's Redirect helper
// appends an inline <script> that the framework's strict script-src CSP
// blocks, so instead we append a tiny fragment whose data-init expression
// (evaluated by the Datastar runtime under the CSP's 'unsafe-eval') assigns
// window.location. The URL is JSON-encoded into a JS string literal and then
// attribute-escaped, so quotes and angle brackets in it can't break out.
func (sp streamPatcher) redirect(url string) error {
	expr, err := json.Marshal(url)
	if err != nil {
		return err
	}
	el := `<div id="beach-redirect" data-init="` +
		html.EscapeString("window.location = "+string(expr)) + `"></div>`
	return sp.sse.PatchElements(el,
		sdk.WithSelector("body"), sdk.WithMode(sdk.ElementPatchModeAppend))
}

// patchOptions builds the SDK patch options for a target id and mode.
func patchOptions(target string, mode PatchMode) []sdk.PatchElementOption {
	opts := make([]sdk.PatchElementOption, 0, 3)
	if target != "" {
		opts = append(opts, sdk.WithSelector("#"+target))
	}
	if m := sdkMode(mode); m != "" {
		opts = append(opts, sdk.WithMode(m))
	}
	return opts
}

// sdkMode maps a beach PatchMode to the SDK's ElementPatchMode. The zero
// PatchMode maps to "" (SDK default outer morph).
func sdkMode(mode PatchMode) sdk.ElementPatchMode {
	switch mode {
	case PatchInner:
		return sdk.ElementPatchModeInner
	case PatchReplace:
		return sdk.ElementPatchModeReplace
	case PatchPrepend:
		return sdk.ElementPatchModePrepend
	case PatchAppend:
		return sdk.ElementPatchModeAppend
	case PatchBefore:
		return sdk.ElementPatchModeBefore
	case PatchAfter:
		return sdk.ElementPatchModeAfter
	case PatchRemove:
		return sdk.ElementPatchModeRemove
	default:
		return ""
	}
}

// hubEvent re-exports hub.Event so producers that publish into a Stream's topics
// from a beach handler do not import hub directly. It is a type alias, so a
// hub.Event passes wherever a beach.HubEvent is wanted and vice versa.
type HubEvent = hub.Event
