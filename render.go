package beach

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/ThirdCoastInteractive/the-beach/datastar"
	"github.com/ThirdCoastInteractive/the-beach/ui"
	"github.com/a-h/templ"
)

// ToastTarget is the conventional DOM id the error toast appends into. A page's
// shell renders an empty <div id="beach-toast"> region; error patches append an
// alert there. Apps that want a different region can render their own and rely on
// the alert's own id morph instead.
const ToastTarget = "beach-toast"

// writeError is the single place a handler error becomes a response. One error
// type, both renderings: a toast/inline patch on a Datastar request, the kit's
// error page on navigation. No handler writes its own error HTML.
func (a *App) writeError(c *Ctx, err error) {
	status := statusForError(err)
	kit := c.Kit()

	if c.IsDatastar() {
		a.writeErrorPatch(c, kit, err, status)
		return
	}
	a.writeErrorPage(c, kit, err, status)
}

// writeErrorPatch streams the error as a Datastar patch. A ValidationError sets
// each field's inline error signal and patches a summary alert; other errors
// patch a single toast alert. The status is conveyed via the alert role, since an
// SSE response is always 200 at the HTTP layer.
func (a *App) writeErrorPatch(c *Ctx, kit ui.Kit, err error, status int) {
	sse := datastar.NewSSE(c.W, c.R, datastar.WithCompression(a.cfg.SSECompression))

	var ve *ValidationError
	if errors.As(err, &ve) {
		// Surface field messages as signals the form can bind to (e.g.
		// $errors.email), and patch a summary alert.
		if len(ve.Fields) > 0 {
			_ = sse.MarshalAndPatchSignals(map[string]any{"errors": ve.Fields})
		}
		alert := kit.Feedback().Alert(ui.AlertProps{
			Role:        ui.RoleWarn,
			Title:       "Please fix the errors below",
			Dismissible: true,
			Attrs:       datastar.Attrs{{Name: "id", Val: ToastTarget + "-item"}},
		})
		_ = sse.PatchElementTempl(toastWrap(alert), patchOptions(ToastTarget, PatchAppend)...)
		return
	}

	alert := kit.Feedback().Alert(ui.AlertProps{
		Role:        ui.RoleDanger,
		Title:       errorTitle(status),
		Dismissible: true,
		Attrs:       datastar.Attrs{{Name: "id", Val: ToastTarget + "-item"}},
	}, txtComponent(errorMessage(err, status)))
	_ = sse.PatchElementTempl(toastWrap(alert), patchOptions(ToastTarget, PatchAppend)...)
}

// writeErrorPage renders a full HTML error document on navigation. It is
// deliberately minimal — shell, heading, alert — and references only kit
// components, so it inherits the active theme.
func (a *App) writeErrorPage(c *Ctx, kit ui.Kit, err error, status int) {
	c.W.Header().Set("Content-Type", "text/html; charset=utf-8")
	c.W.WriteHeader(status)

	body := kit.Layout().Container(ui.ContainerProps{},
		kit.Layout().PageHeading(ui.PageHeadingProps{
			Title:    errorTitle(status),
			Subtitle: errorMessage(err, status),
		}),
		kit.Feedback().Alert(ui.AlertProps{Role: ui.RoleDanger, Title: http.StatusText(status)},
			txtComponent(errorMessage(err, status)),
		),
	)
	doc := errorDocument(errorTitle(status), body)
	if rerr := doc.Render(c.Context(), c.W); rerr != nil {
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

// toastWrap wraps an alert component in the toast item list-shape so appended
// alerts stack. It is just the alert itself today; the wrapper exists so the
// append target stays an element, not bare text.
func toastWrap(alert templ.Component) templ.Component { return alert }

// txtComponent renders escaped text as a templ component, for alert bodies.
func txtComponent(s string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := io.WriteString(w, templ.EscapeString(s))
		return err
	})
}

// errorDocument wraps a body component in a minimal HTML5 document for the
// navigation error page. The stylesheet is referenced by the conventional static
// path so the error page is themed even without the app's own shell.
func errorDocument(title string, body templ.Component) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		head := `<!doctype html><html lang="en"><head><meta charset="utf-8">` +
			`<meta name="viewport" content="width=device-width,initial-scale=1">` +
			`<title>` + templ.EscapeString(title) + `</title>` +
			`<link rel="stylesheet" href="/static/css/app.css"></head><body>`
		if _, err := io.WriteString(w, head); err != nil {
			return err
		}
		if body != nil {
			if err := body.Render(ctx, w); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, `</body></html>`)
		return err
	})
}
