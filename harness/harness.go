package harness

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/get-h3/sdk-go/protocol"
)

// Harness is the interface that harness implementations must satisfy.
// It corresponds to S04 §2.3 of the H3 specification.
type Harness interface {
	// OnProcess is called when a new user message arrives.
	// Returns the first Decision in the agent loop.
	OnProcess(req *protocol.ProcessRequest) (*protocol.Decision, error)

	// OnResult is called after Hermes executes a Decision.
	// Returns the next Decision. Return DecisionEnd to finish.
	OnResult(req *protocol.ResultRequest) (*protocol.Decision, error)

	// OnCancel is called when the user interrupts.
	OnCancel(req *protocol.CancelRequest) error

	// OnSessionTerminate is called on DELETE /v1/sessions/:id.
	OnSessionTerminate(sessionID string) error

	// Health returns harness health status.
	Health() *protocol.HealthResponse
}

// sessionEntry tracks a single session lifecycle.
type sessionEntry struct {
	SessionID  string
	Status     string
	StartedAt  time.Time
	LastActive time.Time
	TurnCount  int
}

// sessionStore is a thread-safe in-memory session tracker.
type sessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*sessionEntry
}

func newSessionStore() *sessionStore {
	return &sessionStore{
		sessions: make(map[string]*sessionEntry),
	}
}

func (s *sessionStore) create(sessionID string) *sessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := &sessionEntry{
		SessionID:  sessionID,
		Status:     "active",
		StartedAt:  time.Now(),
		LastActive: time.Now(),
		TurnCount:  0,
	}
	s.sessions[sessionID] = entry
	return entry
}

func (s *sessionStore) get(sessionID string) *sessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

func (s *sessionStore) update(sessionID string, fn func(*sessionEntry)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.sessions[sessionID]; ok {
		fn(entry)
	}
}

func (s *sessionStore) delete(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.sessions[sessionID]; ok {
		entry.Status = "cancelled"
	}
}

// server holds the harness and session store for HTTP handlers.
type server struct {
	harness  Harness
	sessions *sessionStore
}

// NewHTTPServer creates an http.Handler with all H3 endpoints.
// The returned handler is ready to use with http.ListenAndServe.
func NewHTTPServer(h Harness) http.Handler {
	srv := &server{
		harness:  h,
		sessions: newSessionStore(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", srv.healthHandler)
	mux.HandleFunc("POST /v1/process", srv.processHandler)
	mux.HandleFunc("POST /v1/result", srv.resultHandler)
	mux.HandleFunc("POST /v1/cancel", srv.cancelHandler)
	mux.HandleFunc("GET /v1/sessions/{id}", srv.getSessionHandler)
	mux.HandleFunc("DELETE /v1/sessions/{id}", srv.deleteSessionHandler)

	return withMiddleware(mux)
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("harness: error encoding JSON response: %v", err)
	}
}

// writeError writes a standard H3 error response.
func writeError(w http.ResponseWriter, status int, code protocol.ErrorCode, message string) {
	writeJSON(w, status, &protocol.ErrorResponse{
		Error: protocol.ErrorDetail{
			Code:    code,
			Message: message,
		},
	})
}

// healthHandler handles GET /v1/health.
func (s *server) healthHandler(w http.ResponseWriter, r *http.Request) {
	resp := s.harness.Health()
	if resp == nil {
		resp = &protocol.HealthResponse{
			Status:          protocol.HealthOK,
			Version:         "1.0.0",
			Transport:       "rest",
			ProtocolVersion: "1.0",
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// processHandler handles POST /v1/process.
func (s *server) processHandler(w http.ResponseWriter, r *http.Request) {
	var req protocol.ProcessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest,
			"failed to decode request body: "+err.Error())
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}

	// Track session
	s.sessions.create(req.SessionID)
	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		e.LastActive = time.Now()
		e.TurnCount++
	})

	decision, err := s.harness.OnProcess(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	if err := decision.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInvalidDecision, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, decision)
}

// resultHandler handles POST /v1/result.
func (s *server) resultHandler(w http.ResponseWriter, r *http.Request) {
	var req protocol.ResultRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest,
			"failed to decode request body: "+err.Error())
		return
	}

	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		e.LastActive = time.Now()
		e.TurnCount++
	})

	decision, err := s.harness.OnResult(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	if err := decision.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInvalidDecision, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, decision)
}

// cancelHandler handles POST /v1/cancel.
func (s *server) cancelHandler(w http.ResponseWriter, r *http.Request) {
	var req protocol.CancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest,
			"failed to decode request body: "+err.Error())
		return
	}

	if err := s.harness.OnCancel(&req); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		e.Status = "cancelled"
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// getSessionHandler handles GET /v1/sessions/{id}.
func (s *server) getSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	entry := s.sessions.get(sessionID)
	if entry == nil {
		writeError(w, http.StatusNotFound, protocol.ErrSessionNotFound,
			"session not found: "+sessionID)
		return
	}

	resp := &protocol.SessionResponse{
		SessionID:  entry.SessionID,
		StartedAt:  entry.StartedAt.Format(time.RFC3339),
		LastActive: entry.LastActive.Format(time.RFC3339),
		TurnCount:  entry.TurnCount,
		Status:     protocol.SessionStatus(entry.Status),
	}
	writeJSON(w, http.StatusOK, resp)
}

// deleteSessionHandler handles DELETE /v1/sessions/{id}.
func (s *server) deleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	if err := s.harness.OnSessionTerminate(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	s.sessions.delete(sessionID)
	w.WriteHeader(http.StatusNoContent)
}
