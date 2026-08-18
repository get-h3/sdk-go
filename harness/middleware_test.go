package harness

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/get-h3/sdk-go/protocol"
)

// TestPanicRecovery_JSONErrorResponse verifies that a panicking handler
// wrapped in withMiddleware returns a JSON ErrorResponse (INTERNAL_ERROR,
// HTTP 500) instead of a text/plain body, and that the server keeps serving
// subsequent requests after the panic. Regression test for GAP-027.
func TestPanicRecovery_JSONErrorResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	ts := httptest.NewServer(withMiddleware(mux))
	defer ts.Close()

	// First request: panicking handler → must return JSON 500 INTERNAL_ERROR.
	resp, err := http.Post(ts.URL+"/v1/boom", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("POST /v1/boom: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	var errResp protocol.ErrorResponse
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("decode error response: %v (body: %s)", err, body)
	}
	if errResp.Error.Code != protocol.ErrInternalError {
		t.Errorf("expected error code %q, got %q", protocol.ErrInternalError, errResp.Error.Code)
	}
	if errResp.Error.Message != "internal server error" {
		t.Errorf("expected message %q, got %q", "internal server error", errResp.Error.Message)
	}

	// Second request: healthy handler → server must still be serving (200).
	healthResp, err := http.Get(ts.URL + "/v1/health")
	if err != nil {
		t.Fatalf("GET /v1/health after panic: %v", err)
	}
	defer func() { _ = healthResp.Body.Close() }()

	if healthResp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 from healthy handler after panic, got %d", healthResp.StatusCode)
	}
}
