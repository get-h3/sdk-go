// Package harness provides the H3 HTTP handler, middleware, and Harness interface
// for building H3-compliant agent harnesses.
package harness

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sync"
	"time"

	"github.com/get-h3/sdk-go/protocol"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode  int
	wroteHeader bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.wroteHeader {
		rw.statusCode = code
		rw.wroteHeader = true
	}
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// timeoutResponseWriter buffers the inner handler's response so that on
// deadline expiry the partial output can be discarded and a clean JSON
// ErrorResponse written instead. All writes are mutex-guarded: once timedOut
// is set, subsequent Write/WriteHeader calls from the still-running inner
// goroutine become no-ops — no partial/corrupted body can mix with the
// JSON error. This preserves the core safety property of http.TimeoutHandler
// (buffered, atomic flush) while replacing its text/plain 503 with a
// protocol-compliant H3 JSON ErrorResponse.
type timeoutResponseWriter struct {
	mu         sync.Mutex
	dst        http.ResponseWriter
	timedOut   bool
	header     http.Header
	buf        bytes.Buffer
	statusCode int
	wroteHead  bool
}

func newTimeoutResponseWriter(w http.ResponseWriter) *timeoutResponseWriter {
	return &timeoutResponseWriter{
		dst:        w,
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (tw *timeoutResponseWriter) Header() http.Header {
	return tw.header
}

func (tw *timeoutResponseWriter) WriteHeader(code int) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut || tw.wroteHead {
		return
	}
	tw.wroteHead = true
	tw.statusCode = code
}

func (tw *timeoutResponseWriter) Write(b []byte) (int, error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	if tw.timedOut {
		return 0, nil // discard late writes
	}
	if !tw.wroteHead {
		tw.wroteHead = true
	}
	return tw.buf.Write(b)
}

// markTimedOut atomically sets the timed-out flag so the inner handler's
// subsequent writes are discarded. Returns the body buffered so far (unused
// by the caller — kept in the signature for clarity / potential diagnostics).
func (tw *timeoutResponseWriter) markTimedOut() {
	tw.mu.Lock()
	tw.timedOut = true
	tw.mu.Unlock()
}

// flush copies the buffered response to the underlying ResponseWriter. It must
// only be called after the inner handler has completed (success path). On the
// timeout path markTimedOut is called instead and flush is never invoked, so
// the buffered body is silently discarded.
func (tw *timeoutResponseWriter) flush() error {
	tw.mu.Lock()
	if tw.timedOut {
		tw.mu.Unlock()
		return nil
	}
	tw.mu.Unlock()
	// Inner handler is done — no concurrent access to tw fields.
	for k, v := range tw.header {
		tw.dst.Header()[k] = v
	}
	tw.dst.WriteHeader(tw.statusCode)
	_, err := tw.dst.Write(tw.buf.Bytes())
	return err
}

// withMiddlewareTimeout wraps next with a custom timeout handler that writes a
// protocol-compliant JSON ErrorResponse (HARNESS_TIMEOUT, HTTP 504) when the
// deadline expires, replacing http.TimeoutHandler's text/plain 503. The
// deadline is parameterised so tests can exercise the timeout path with a
// short duration (the production default is wired via withMiddleware).
func withMiddlewareTimeout(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), d)
		defer cancel()
		r = r.WithContext(ctx)

		tw := newTimeoutResponseWriter(w)
		done := make(chan struct{})
		panicChan := make(chan any, 1)

		go func() {
			defer func() {
				if p := recover(); p != nil {
					panicChan <- p
				}
				close(done)
			}()
			next.ServeHTTP(tw, r)
		}()

		select {
		case p := <-panicChan:
			// Re-panic on the caller's goroutine so the outer
			// panic-recovery wrapper can catch and handle it.
			panic(p)
		case <-done:
			// Inner handler finished before the deadline — flush its
			// buffered response to the real ResponseWriter.
			if err := tw.flush(); err != nil {
				slog.Error("harness: error flushing timeout-buffered response", "error", err)
			}
		case <-ctx.Done():
			// Deadline expired. Discard the inner handler's partial output
			// and write a clean JSON ErrorResponse. The inner goroutine may
			// still be running; its subsequent writes are no-ops (timedOut).
			tw.markTimedOut()
			writeError(w, http.StatusGatewayTimeout, protocol.ErrHarnessTimeout,
				"harness did not respond within the timeout")
		}
	})
}

// withMiddleware wraps an http.Handler with logging, panic recovery, and timeout.
func withMiddleware(next http.Handler) http.Handler {
	// Apply timeout first (outermost), using the production 30s deadline.
	h := withMiddlewareTimeout(next, 30*time.Second)

	// Wrap with panic recovery + logging
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("harness: panic recovered",
					"error", fmt.Sprintf("%v", rec),
					"stack", string(debug.Stack()),
				)
				writeError(rw.ResponseWriter, http.StatusInternalServerError, protocol.ErrInternalError, "internal server error")
				rw.statusCode = http.StatusInternalServerError
			}
			slog.Info("request completed",
				"method", r.Method,
				"path", r.URL.Path,
				"status", rw.statusCode,
				"duration", time.Since(start).String(),
			)
		}()

		h.ServeHTTP(rw, r)
	})
}
