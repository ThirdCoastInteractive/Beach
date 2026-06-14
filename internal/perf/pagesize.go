// Package perf holds the-beach's performance gates: the 14KB page-size budget
// (the [14KB rule]) and the SSE fan-out budget (the 5k-connection proxy). These
// are tests and helpers, not production code — they measure the framework's
// performance laws so a regression shows up as a red build and a diff in the
// JSONL perf history.
//
// Two gates live here:
//
//   - PageSize + TestHomePagesUnder14KB: render the example apps' home pages
//     through the real beach render path and assert each fits ≤14KB gzipped.
//   - BenchmarkFanout: subscribe N hub connections, publish M frames, and report
//     per-connection memory and throughput — the budget the hub must meet for
//     5k live SSE connections on one container.
//
// [14KB rule]: ../docs/architecture/06-ui.md
package perf

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
)

// Budget14KB is the compressed page-size budget from the 14KB rule: a fresh TCP
// connection's initial congestion window is ~10 segments ≈ 14KB, so a first
// response that fits arrives in one round trip. View.Page targets ≤14KB
// compressed (the HTML document only — the stylesheet is a separate cached
// request and does not count).
const Budget14KB = 14 * 1024

// PageSize drives handler with a plain GET of path through net/http/httptest and
// returns the response body size both raw and gzipped. It is the measurement
// behind the 14KB gate: the framework writes the page as uncompressed HTML
// (compression is a transport concern), so the gate compresses the body itself
// at gzip's default level to model what the wire actually carries.
//
// The request carries no Datastar header, so a beach PageFunc renders the full
// document (the navigation branch), not a fragment patch.
func PageSize(handler http.Handler, path string) (raw, gzipped int) {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.Bytes()
	return len(body), gzipLen(body)
}

// gzipLen returns the byte length of b compressed with gzip at the default
// compression level — the level a stock reverse proxy or Go's own gzip handler
// uses, so the number tracks the wire size a browser sees.
func gzipLen(b []byte) int {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(b); err != nil {
		return -1
	}
	if err := zw.Close(); err != nil {
		return -1
	}
	return buf.Len()
}

// PerfSample is one row of the JSONL perf history tracked in git. Ts is supplied
// by the caller (an RFC3339 string or build id passed as an arg, never read from
// the clock here) so the history is deterministic and diffable; trends show up
// in the diff rather than in a timestamp churn.
type PerfSample struct {
	Ts    string `json:"ts"`
	Route string `json:"route"`
	Bytes int    `json:"bytes"`
}

// AppendSample appends one {ts, route, bytes} record as a single JSON line to the
// JSONL file at path, creating it if absent. JSONL (one object per line, never
// rewritten) is append-only so a perf run never has to read-modify-write the
// whole history and concurrent runs can't corrupt earlier rows.
func AppendSample(path, ts, route string, bytes int) error {
	line, err := json.Marshal(PerfSample{Ts: ts, Route: route, Bytes: bytes})
	if err != nil {
		return fmt.Errorf("perf: marshal sample: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("perf: open history %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("perf: append history: %w", err)
	}
	return nil
}
