package beach

import (
	"context"
	"errors"
	"net/http"

	"github.com/ThirdCoastInteractive/Beach/pkg/datastar"
	"github.com/ThirdCoastInteractive/Beach/pkg/i18n"
	"github.com/ThirdCoastInteractive/Beach/pkg/ui/driftwood"
)

// ToastTarget is the DOM id of the page's live region, where error toasts and
// Patch.Announce messages land. driftwood.Shell renders the region empty on
// every page — which is what makes patches into it announced, since a live
// region that arrives with its content is not read out. The kit owns the id
// because the kit renders the element; this is the alias apps name.
const ToastTarget = driftwood.ToastTarget

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
		alert := driftwood.ErrorAlert(ToastTarget+"-item", false, i18n.T(c.Context(), "framework.error.form"), "")
		_ = sse.PatchElementTempl(alert, patchOptions(ToastTarget, PatchAppend)...)
		return
	}

	alert := driftwood.ErrorAlert(ToastTarget+"-item", true, errorTitle(c.Context(), status), errorMessage(c.Context(), err, status))
	_ = sse.PatchElementTempl(alert, patchOptions(ToastTarget, PatchAppend)...)
}

// writeErrorPage renders driftwood's full error document on navigation. It is
// deliberately minimal and built from the same tokens as the rest of the app,
// so it inherits the active theme.
func (a *App) writeErrorPage(c *Ctx, err error, status int) {
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.WriteHeader(status)
	page := driftwood.ErrorPage(status, errorTitle(c.Context(), status), errorMessage(c.Context(), err, status))
	if rerr := page.Render(c.Context(), c.W); rerr != nil {
		a.log.Error("error page render", "err", rerr)
	}
}

// errorTitle is a short human title for a status. Like every other string the
// framework puts in front of a person, it comes from the catalog: an app served
// in Spanish should not hand back an English error page, and the page's own
// <html lang> would be lying if it did.
func errorTitle(ctx context.Context, status int) string {
	switch status {
	case http.StatusNotFound:
		return i18n.T(ctx, "framework.error.not_found")
	case http.StatusForbidden:
		return i18n.T(ctx, "framework.error.forbidden")
	case http.StatusUnauthorized:
		return i18n.T(ctx, "framework.error.unauthorized")
	case http.StatusBadRequest:
		return i18n.T(ctx, "framework.error.bad_request")
	default:
		return i18n.T(ctx, "framework.error.internal")
	}
}

// errorMessage returns a safe, user-facing message. Internal (500) errors never
// leak their detail to the client; 4xx errors with a message do (validation, etc).
func errorMessage(ctx context.Context, err error, status int) string {
	if status >= 500 {
		return i18n.T(ctx, "framework.error.internal.detail")
	}
	var ve *ValidationError
	if errors.As(err, &ve) {
		return ve.Error()
	}
	switch status {
	case http.StatusNotFound:
		return i18n.T(ctx, "framework.error.not_found.detail")
	case http.StatusForbidden:
		return i18n.T(ctx, "framework.error.forbidden.detail")
	case http.StatusUnauthorized:
		return i18n.T(ctx, "framework.error.unauthorized.detail")
	default:
		return i18n.T(ctx, "framework.error.bad_request.detail")
	}
}
