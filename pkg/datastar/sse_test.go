package datastar

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsDatastar(t *testing.T) {
	tests := []struct {
		name   string
		header string
		set    bool
		want   bool
	}{
		{"present true", "true", true, true},
		{"present false", "false", true, false},
		{"present empty", "", true, false},
		{"absent", "", false, false},
		{"present garbage", "yes", true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.set {
				r.Header.Set("Datastar-Request", tt.header)
			}
			if got := IsDatastar(r); got != tt.want {
				t.Errorf("IsDatastar() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewSSEGzipNegotiated(t *testing.T) {
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	sse := NewSSE(w, r)
	if sse == nil || sse.ServerSentEventGenerator == nil {
		t.Fatal("NewSSE returned nil generator")
	}

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-transform") {
		t.Errorf("Cache-Control = %q, want it to contain no-transform", cc)
	}
}

func TestNewSSECompressionOff(t *testing.T) {
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	NewSSE(w, r, WithCompression(SSECompressionOff))

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty (compression off)", got)
	}
}

func TestNewSSELightCompression(t *testing.T) {
	r := httptest.NewRequest("GET", "/stream", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()

	NewSSE(w, r, WithCompression(SSECompressionLight))

	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip (huffman-only is still gzip)", got)
	}
}

func TestNewSSENoAcceptEncoding(t *testing.T) {
	// No Accept-Encoding: the SDK negotiates nothing, response stays plain.
	r := httptest.NewRequest("GET", "/stream", nil)
	w := httptest.NewRecorder()

	NewSSE(w, r)

	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want empty when client sends no Accept-Encoding", got)
	}
}
