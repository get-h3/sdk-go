// Package harness provides the H3 HTTP handler, middleware, and Harness interface
// for building H3-compliant agent harnesses.
package harness

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
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

// withMiddleware wraps an http.Handler with logging, panic recovery, and timeout.
func withMiddleware(next http.Handler) http.Handler {
	// Apply timeout first (outermost)
	h := http.TimeoutHandler(next, 30*time.Second, "request timeout")

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
				http.Error(rw.ResponseWriter, "internal server error", http.StatusInternalServerError)
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
