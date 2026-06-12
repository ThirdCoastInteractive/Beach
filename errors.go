package beach

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Errors are part of the handler contract: handlers return errors, not error
// responses. The framework maps them to the driftwood error page on navigation and
// to a toast / inline-validation patch on a Datastar request. One error type,
// both renderings — no handler ever writes its own error HTML.

// Sentinel errors map to specific HTTP statuses. Wrap them with fmt.Errorf("%w",
// ...) to add context while keeping the status mapping (errors.Is sees through).
var (
	// ErrNotFound renders a 404. Return it when a looked-up resource is absent.
	ErrNotFound = errors.New("beach: not found")

	// ErrForbidden renders a 403. Guards return it; handlers may too.
	ErrForbidden = errors.New("beach: forbidden")

	// ErrUnauthorized renders a 401. Return it when authentication is required
	// and absent (guards normally handle this).
	ErrUnauthorized = errors.New("beach: unauthorized")

	// ErrBadRequest renders a 400. Bind returns it (wrapped in a ValidationError)
	// for malformed input.
	ErrBadRequest = errors.New("beach: bad request")
)

// ValidationError is a field-keyed validation failure returned by Bind and by
// handlers that validate input. On a Datastar request the framework renders it as
// inline field errors; on navigation it folds into the error page. It maps
// to HTTP 400.
type ValidationError struct {
	// Fields maps a form field name to its human-readable error. A general
	// (non-field) message uses the empty-string key.
	Fields map[string]string
}

// Error implements error.
func (e *ValidationError) Error() string {
	if len(e.Fields) == 0 {
		return "validation failed"
	}
	parts := make([]string, 0, len(e.Fields))
	for k, v := range e.Fields {
		if k == "" {
			parts = append(parts, v)
		} else {
			parts = append(parts, k+": "+v)
		}
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Is reports ValidationError as a bad-request error so errors.Is(err,
// ErrBadRequest) works for the status mapping.
func (e *ValidationError) Is(target error) bool { return target == ErrBadRequest }

// Invalid is a convenience constructor for a single-field ValidationError.
func Invalid(field, msg string) *ValidationError {
	return &ValidationError{Fields: map[string]string{field: msg}}
}

// statusForError maps a handler error to an HTTP status. Unknown errors are 500.
func statusForError(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	default:
		var ve *ValidationError
		if errors.As(err, &ve) {
			return http.StatusBadRequest
		}
		return http.StatusInternalServerError
	}
}

// Bind decodes and returns a typed value from the request. For a JSON or
// Datastar request it decodes the JSON body the client posts (Datastar posts its
// signals as a JSON object); for a normal form submission it maps URL-encoded
// values by `form:"name"` struct tags. A decode failure becomes a
// ValidationError (HTTP 400). If T has a Validate() error method it is called
// after binding, so a handler's domain validation rides along.
func Bind[T any](c *Ctx) (T, error) {
	var dst T

	ct := c.R.Header.Get("Content-Type")
	switch {
	case strings.Contains(ct, "application/json") || c.IsDatastar():
		// Read the whole body so a superset of signals does not hard-fail: we do
		// not DisallowUnknownFields, since Datastar may post more signals than the
		// handler's struct declares.
		body, err := io.ReadAll(io.LimitReader(c.R.Body, maxBindBody))
		if err != nil {
			return dst, &ValidationError{Fields: map[string]string{"": "could not read request body"}}
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &dst); err != nil {
				return dst, &ValidationError{Fields: map[string]string{"": "invalid JSON body"}}
			}
		}
	default:
		if err := c.R.ParseForm(); err != nil {
			return dst, &ValidationError{Fields: map[string]string{"": "malformed form"}}
		}
		setFormFields(c, &dst)
	}

	if v, ok := any(&dst).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return dst, err
		}
	}
	return dst, nil
}

// maxBindBody caps the request body Bind will read, a basic abuse guard.
const maxBindBody = 1 << 20 // 1 MiB
