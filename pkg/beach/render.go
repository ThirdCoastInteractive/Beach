package beach

import (
	"errors"
	"net/http"

	"github.com/ThirdCoastInteractive/the-beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/the-beach/pkg/ui/driftwood"
)

// ToastTarget is the conventional DOM id the error toast appends into. A page's
// shell renders an empty <div id="beach-toast"> region; error patches append an
// alert there. Apps that want a different region can render their own and rely on
// the alert's own id morph instead.
const ToastTarget = "beach-toast"

// writeError is the single place a handler error becomes a response. One error
// type, both renderings: a toast/inline patch on a Datastar request, the
// driftwood error page on navigation. No handler writes its own error HTML.
func (a *App) writeError(c *Ctx, err error) {
	status := statusForError(err)
	if c.IsDatastar() {
		a.writeErrorPatch(c, err, status)
		return
	}
	a.writeErrorPage(c, err, status)
}

// writeErrorPatch streams the error as a Datastar patch. A ValidationError sets
// each field's inline error signal and patches a summary alert; other errors
// patch a single toast alert. The status is conveyed via the alert tone, since an
// SSE response is always 200 at the HTTP layer.
func (a *App) writeErrorPatch(c *Ctx, err error, status int) {
	sse := datastar.NewSSE(c.W, c.R, datastar.WithCompression(a.cfg.SSECompression))

	var ve *ValidationError
	if errors.As(err, &ve) {
		// Surface field messages as signals the form can bind to (e.g.
		// $errors.email), and patch a summary alert.
		if len(ve.Fields) > 0 {
			_ = sse.MarshalAndPatchSignals(map[string]any{"errors": ve.Fields})
		}
		alert := driftwood.ErrorAlert(ToastTarget+"-item", false, "Please fix the errors below", "")
		_ = sse.PatchElementTempl(alert, patchOptions(ToastTarget, PatchAppend)...)
		return
	}

	alert := driftwood.ErrorAlert(ToastTarget+"-item", true, errorTitle(status), errorMessage(err, status))
	_ = sse.PatchElementTempl(alert, patchOptions(ToastTarget, PatchAppend)...)
}

// writeErrorPage renders driftwood's full error document on navigation. It is
// deliberately minimal and built from the same tokens as the rest of the app,
// so it inherits the active theme.
func (a *App) writeErrorPage(c *Ctx, err error, status int) {
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.WriteHeader(status)
	page := driftwood.ErrorPage(status, errorTitle(status), errorMessage(err, status))
	if rerr := page.Render(c.Context(), c.W); rerr != nil {
		a.log.Error("error page render", "err", rerr)
	}
}

// errorTitle is a short human title for a status.
func errorTitle(status int) string {
	switch status {
	case http.StatusNotFound:
		return "Not found"
	case http.StatusForbidden:
		return "Forbidden"
	case http.StatusUnauthorized:
		return "Sign in required"
	case http.StatusBadRequest:
		return "Invalid request"
	default:
		return "Something went wrong"
	}
}

// errorMessage returns a safe, user-facing message. Internal (500) errors never
// leak their detail to the client; 4xx errors with a message do (validation, etc).
func errorMessage(err error, status int) string {
	if status >= 500 {
		return "An unexpected error occurred. Please try again."
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Error()
	}
	switch status {
	case http.StatusNotFound:
		return "The page you were looking for does not exist."
	case http.StatusForbidden:
		return "You do not have permission to view this page."
	case http.StatusUnauthorized:
		return "Please sign in to continue."
	default:
		return "The request could not be processed."
	}
}
