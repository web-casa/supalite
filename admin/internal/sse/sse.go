// Package sse provides a minimal Server-Sent Events writer.
//
// Usage:
//	w, err := sse.Start(rw, r)
//	if err != nil { return } // connection didn't support Flush
//	defer w.Close()
//	w.Send("log", line)
//	w.Heartbeat(15 * time.Second, ctx.Done())
//
// Design notes:
//   - Streaming requires http.Flusher; we bail early if the wrapped
//     ResponseWriter doesn't support it (shouldn't happen behind Caddy).
//   - Client disconnect is detected via r.Context().Done(); callers
//     should select on it to stop producing events.
//   - Heartbeat comments (`: ping\n\n`) keep intermediaries from closing
//     idle connections. Chose 15s to sit well under typical 30-60s
//     proxy idle timeouts.
package sse

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	closed  bool
}

// Start sends SSE headers and returns a Writer. Returns error if the
// underlying ResponseWriter doesn't support flushing.
func Start(w http.ResponseWriter, _ *http.Request) (*Writer, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, fmt.Errorf("streaming unsupported")
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("Connection", "keep-alive")
	// Disable buffering for nginx/any proxy that honors this hint.
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()
	return &Writer{w: w, flusher: flusher}, nil
}

// Send writes one event and flushes. `data` may contain newlines;
// each line is prefixed with "data: " per the SSE spec.
func (s *Writer) Send(event, data string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	var sb strings.Builder
	if event != "" {
		sb.WriteString("event: ")
		sb.WriteString(event)
		sb.WriteByte('\n')
	}
	for _, line := range strings.Split(data, "\n") {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
	if _, err := io.WriteString(s.w, sb.String()); err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// Ping writes an SSE comment to keep the connection alive.
func (s *Writer) Ping() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return io.ErrClosedPipe
	}
	if _, err := io.WriteString(s.w, ": ping\n\n"); err != nil {
		s.closed = true
		return err
	}
	s.flusher.Flush()
	return nil
}

// Close marks the writer closed. Subsequent Send/Ping return ErrClosedPipe.
// Doesn't close the underlying connection — that's the HTTP handler's job.
func (s *Writer) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}
