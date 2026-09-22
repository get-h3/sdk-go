// Package harness provides the H3 HTTP handler, middleware, and Harness interface
// for building H3-compliant agent harnesses.
package harness

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
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
	SessionID           string
	Status              string
	StartedAt           time.Time
	LastActive          time.Time
	TurnCount           int
	CurrentDecisionID   string
	CurrentDecisionType protocol.DecisionType
	// LastResultDecisionID is the decision_id most recently accepted by
	// POST /v1/result. CurrentDecisionID already holds the in-flight decision,
	// so this second id is what makes a RETRY distinguishable from a stale or
	// invented id: req.DecisionID == LastResultDecisionID (and != in-flight)
	// means the result for that decision was already applied (GAP-049).
	LastResultDecisionID string
	// ResultClaimID is the decision_id of the POST /v1/result delivery that is
	// currently admitted for this session — i.e. the delivery whose OnResult
	// call is running (GAP-058). Admission checks and this claim are written in
	// ONE locked transaction before OnResult is invoked, so two concurrent
	// deliveries of the SAME decision_id can never both pass the check and run
	// the harness's side effect twice. It is cleared when the delivery finishes
	// (any path, including an error or a panic), which leaves a failed delivery
	// retryable.
	ResultClaimID string
	// deliveryMu serializes result deliveries for this session: one
	// POST /v1/result at a time, so a delivery that arrives while another one
	// for the same session is admitted waits for it and is then answered from
	// the state that delivery left behind (200 when the id is the in-flight
	// decision again, 400 when it was resolved) instead of racing its OnResult.
	// The lock lives with the entry, so it is per session incarnation.
	deliveryMu *sync.Mutex
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

// newSessionEntry builds a fresh session entry with a zeroed lifecycle: one
// process turn is counted by the caller that creates it.
func newSessionEntry(sessionID string, now time.Time) *sessionEntry {
	return &sessionEntry{
		SessionID:  sessionID,
		Status:     "active",
		StartedAt:  now,
		LastActive: now,
		TurnCount:  0,
		deliveryMu: &sync.Mutex{},
	}
}

// beginProcessTurn admits one POST /v1/process for sessionID and accounts for
// it, in a single locked transaction (GAP-059). It reproduces the documented
// session status machine (docs/api-reference.md § Session status machine):
//
//   - session not in the store      -> created, status "active", and this call
//     is turn 1 (started_at = now).
//   - session "active"              -> the call ACCUMULATES: turn_count++ and
//     started_at is preserved. A repeated POST /v1/process is a new turn of the
//     same session, not a new session; resetting started_at/turn_count here (as
//     this SDK used to, on every POST) buried the session's real history.
//   - session "completed"           -> re-opened: status back to "active" and
//     started_at/turn_count reset (the re-opening call is turn 1), matching the
//     documented re-open semantics.
//   - session "cancelled"           -> untouched: cancelled is terminal
//     (GAP-028). The handler still runs OnProcess (permissive), but a late
//     process must not revive the session or advance turn_count.
//
// Doing the check and the write under one lock also removes the old
// snapshot-then-create race, in which two concurrent process requests for the
// same new session could both "create" it and lose one call's turn.
func (s *sessionStore) beginProcessTurn(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	entry, ok := s.sessions[sessionID]
	if !ok {
		entry = newSessionEntry(sessionID, now)
		entry.TurnCount = 1
		s.sessions[sessionID] = entry
		return
	}

	if entry.Status == "cancelled" {
		return
	}

	if entry.Status == "completed" {
		// Re-open: a fresh lifecycle for the same session id. Decision
		// bookkeeping from the completed lifecycle is dropped as well, so a
		// result for the old lifecycle's last decision cannot be correlated
		// against the re-opened session (the pre-GAP-059 code recreated the
		// whole entry here, which cleared these fields too).
		entry.Status = "active"
		entry.StartedAt = now
		entry.LastActive = now
		entry.TurnCount = 1
		entry.CurrentDecisionID = ""
		entry.CurrentDecisionType = ""
		entry.LastResultDecisionID = ""
		entry.ResultClaimID = ""
		return
	}

	entry.LastActive = now
	entry.TurnCount++
}

// get returns the stored entry pointer for existence checks only. Callers MUST
// NOT read or write fields through the returned pointer — that would be an
// unlocked access racing update() (GAP-043). Use snapshot() for any read of
// entry fields.
func (s *sessionStore) get(sessionID string) *sessionEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sessions[sessionID]
}

// snapshot returns a copy of the session entry's fields taken under the read
// lock. Handlers use it so that every read of a session entry is ordered
// against the locked writes performed by update(). sessionEntry holds only
// value types (strings, ints, time.Time) plus deliveryMu, a pointer to the
// entry's own result-delivery lock — copying that pointer is the point: a
// handler that must serialize against other deliveries of the same session
// takes the lock through the copy it already holds. The returned copy shares no
// OTHER mutable state with the stored entry and is safe to use after RUnlock.
func (s *sessionStore) snapshot(sessionID string) (sessionEntry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, ok := s.sessions[sessionID]
	if !ok {
		return sessionEntry{}, false
	}
	return *entry, true
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
	delete(s.sessions, sessionID)
}

// claimResultDelivery performs the atomic verify-and-claim for ONE
// POST /v1/result delivery (GAP-058). It runs inside sessionStore.update, i.e.
// under the session store's write lock, and either
//
//   - returns the message of the 400 INVALID_REQUEST the handler must write —
//     the delivery is refused and NOTHING was claimed, or
//   - records the delivery's claim on the entry and returns "", leaving the
//     caller to invoke OnResult.
//
// The claim is written BEFORE OnResult runs, in the same critical section as
// the check. That is the fix: the check and the act of claiming are one
// transaction, so two concurrent deliveries of the SAME decision_id can no
// longer both pass the check (as they did when the check and the
// LastResultDecisionID record were separate lock acquisitions with OnResult in
// between) and execute the harness's side effect twice.
//
// The correlation rules are unchanged (GAP-049): with a decision in flight,
// the in-flight id is accepted, the already-resolved id is refused as "already
// been resolved", and anything else is refused as a mismatch. A session with no
// decision in flight is still accepted permissively. The one added refusal is a
// delivery of an id another delivery currently holds claimed for this session.
func claimResultDelivery(e *sessionEntry, sessionID, decisionID string) string {
	if e.ResultClaimID == decisionID {
		// This id is admitted and its OnResult is still running. Refusing
		// here is what keeps the side effect single-shot even if a delivery
		// ever reaches this point without the session's delivery lock.
		return fmt.Sprintf("decision_id %q is already being processed for session %q",
			decisionID, sessionID)
	}

	if e.CurrentDecisionID != "" {
		switch decisionID {
		case e.CurrentDecisionID:
			// In-flight match — fall through to the claim below.
		case e.LastResultDecisionID:
			return fmt.Sprintf("decision_id %q has already been resolved for session %q",
				decisionID, sessionID)
		default:
			return fmt.Sprintf("decision_id %q does not match the session's in-flight decision %q",
				decisionID, e.CurrentDecisionID)
		}
	}

	e.ResultClaimID = decisionID
	// GAP-028: cancelled is terminal — a late result still reaches the harness
	// (permissive) but must not increment turn_count or rewrite status.
	if e.Status != "cancelled" {
		e.LastActive = time.Now()
		e.TurnCount++
	}
	return ""
}

// releaseResultClaim clears the claim recorded by claimResultDelivery for
// decisionID, leaving every other field alone. It is called on EVERY exit of a
// delivery — including OnResult returning an error, a decision that fails
// validation, and a panic inside OnResult — so a delivery that did not resolve
// its decision leaves the session retryable: the next delivery of that id is
// checked against the unchanged bookkeeping instead of being refused as
// still-claimed.
func (s *sessionStore) releaseResultClaim(sessionID, decisionID string) {
	s.update(sessionID, func(e *sessionEntry) {
		if e.ResultClaimID == decisionID {
			e.ResultClaimID = ""
		}
	})
}

// count returns the number of sessions currently present in the store — any
// status (active, completed or cancelled), because a session leaves the store
// only via DELETE /v1/sessions/{id}. It backs the SDK-filled active_sessions
// health field (GAP-050), so it reads the map under the same lock as every
// other accessor and is safe to call from concurrent health requests.
func (s *sessionStore) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// maxRequestBodyBytes is the cap the SDK applies to a request body before
// decoding it (GAP-060). Every POST handler used to hand r.Body straight to
// json.Decoder, so an unauthenticated caller could make the process allocate
// as much memory as it liked just by streaming a huge body (a body it never
// even had to finish sending). http.MaxBytesReader stops the decoder one byte
// past this limit, so the memory a request can cost is bounded by a constant
// instead of by the client.
const maxRequestBodyBytes int64 = 10 << 20 // 10 MiB

// server holds the harness and session store for HTTP handlers.
type server struct {
	harness  Harness
	sessions *sessionStore
	// startedAt is when NewHTTPServer built this handler. It is the server's
	// own clock for the SDK-filled uptime_seconds health field (GAP-050): the
	// harness cannot know how long the HTTP server has been serving, so the
	// server reports it.
	startedAt time.Time
	// maxBodyBytes overrides maxRequestBodyBytes for this server. Zero (the
	// value NewHTTPServer leaves, and the value of a server a test builds by
	// hand) means maxRequestBodyBytes; a test can set it to exercise the cap
	// with a small body instead of a 10 MiB one.
	maxBodyBytes int64
}

// bodyLimit is the effective POST body cap for this server.
func (s *server) bodyLimit() int64 {
	if s.maxBodyBytes > 0 {
		return s.maxBodyBytes
	}
	return maxRequestBodyBytes
}

// newServer builds the server value behind NewHTTPServer's handler. It is split
// out so tests can build a server and adjust its limits before wrapping it.
func newServer(h Harness) *server {
	return &server{
		harness:   h,
		sessions:  newSessionStore(),
		startedAt: time.Now(),
	}
}

// NewHTTPServer creates an http.Handler with all H3 endpoints.
// The returned handler is ready to use with http.ListenAndServe.
func NewHTTPServer(h Harness) http.Handler {
	return newServer(h).handler()
}

// handler returns the fully wired handler: every H3 route, the JSON 404/405
// interceptor, and the middleware chain.
func (srv *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", srv.healthHandler)
	mux.HandleFunc("POST /v1/process", srv.processHandler)
	mux.HandleFunc("POST /v1/result", srv.resultHandler)
	mux.HandleFunc("POST /v1/cancel", srv.cancelHandler)
	mux.HandleFunc("GET /v1/sessions/{id}", srv.getSessionHandler)
	mux.HandleFunc("DELETE /v1/sessions/{id}", srv.deleteSessionHandler)

	// GAP-034/GAP-035: ServeMux has no exported NotFound field in this
	// toolchain, so wrap the mux to detect its default text/plain 404 and 405
	// responses and replace them with JSON ErrorResponses. We cannot register a
	// "/" catch-all (it would intercept wrong-method requests that should get
	// 405), and delegating via mux.Handler would lose wildcard PathValue
	// population — so let the mux dispatch normally and intercept only its
	// default 404/405 output. Handler-level JSON errors (e.g.
	// session-not-found) pass through untouched.
	return withMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		iw := &notFoundInterceptor{ResponseWriter: w}
		mux.ServeHTTP(iw, r)
		if iw.intercepted {
			if iw.status == http.StatusMethodNotAllowed {
				writeError(w, http.StatusMethodNotAllowed, protocol.ErrMethodNotAllowed, "method not allowed")
			} else {
				writeError(w, http.StatusNotFound, protocol.ErrNotFound, "route not found")
			}
		}
	}))
}

// notFoundInterceptor wraps a ResponseWriter to detect the ServeMux's default
// text/plain 404 and 405 responses. When detected, it suppresses the write and
// records the intercepted status so the caller can write a JSON ErrorResponse
// instead. Handler-level JSON errors (Content-Type: application/json) pass
// through untouched.
type notFoundInterceptor struct {
	http.ResponseWriter
	wroteHeader bool
	intercepted bool
	status      int
}

func (w *notFoundInterceptor) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	// Intercept the ServeMux default 404 and 405 (text/plain). Handler-level
	// JSON errors use application/json via writeError and must pass through.
	if (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) && w.Header().Get("Content-Type") == "text/plain; charset=utf-8" {
		w.intercepted = true
		w.status = code
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundInterceptor) Write(b []byte) (int, error) {
	if w.intercepted {
		return len(b), nil // discard the text/plain body
	}
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
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

// decodeBody caps r.Body and decodes it into v, reporting whether the handler
// may continue (GAP-060). It writes the error response itself when it returns
// false:
//
//   - the body passed this server's cap -> 413 INVALID_REQUEST naming the
//     limit. The cap is applied with http.MaxBytesReader BEFORE decoding, so
//     the decoder reads at most one byte past the limit and an oversized body
//     is refused from a bounded amount of memory rather than buffered whole.
//   - anything else (malformed JSON, wrong types, a truncated body) -> 400
//     INVALID_REQUEST with the same message every handler produced before the
//     cap existed.
func (s *server) decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, s.bodyLimit())

	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, protocol.ErrInvalidRequest,
				fmt.Sprintf("request body exceeds the %d-byte limit", tooLarge.Limit))
			return false
		}
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest,
			"failed to decode request body: "+err.Error())
		return false
	}
	return true
}

// healthHandler handles GET /v1/health.
//
// Supplier split (GAP-050): the harness owns the identity/capability fields of
// the body — status, version, transport, protocol_version, capabilities,
// degraded_reason and error — and the server passes them through verbatim. The
// SERVER owns uptime_seconds (it is the only party that knows how long this
// HTTP server has been serving) and active_sessions (it owns the session
// store), so it fills those two on the way out; whatever the harness set for
// them is overwritten.
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

	// Copy before filling. resp is the harness's own memory: a harness that
	// builds its health body once (a package-level value, or a struct field
	// returned on every call) would otherwise be written by concurrent health
	// requests — a data race the SDK must not introduce — and the harness's
	// struct would silently start reporting server-derived values.
	out := *resp
	out.UptimeSeconds = s.uptimeSeconds()
	out.ActiveSessions = s.sessions.count()
	writeJSON(w, http.StatusOK, &out)
}

// uptimeSeconds is this server's age in whole seconds. A zero startedAt (a
// server value not built by NewHTTPServer, as an in-package test may build)
// reports 0 rather than the age of the zero time, and a backwards clock is
// clamped at 0 so the field can never be negative.
func (s *server) uptimeSeconds() int {
	if s.startedAt.IsZero() {
		return 0
	}
	uptime := int(time.Since(s.startedAt).Seconds())
	if uptime < 0 {
		return 0
	}
	return uptime
}

// processHandler handles POST /v1/process.
func (s *server) processHandler(w http.ResponseWriter, r *http.Request) {
	var req protocol.ProcessRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}

	// Track the session turn. GAP-028: cancelled is terminal — a late POST
	// /v1/process for an already-cancelled session must not overwrite the entry
	// or increment turn_count; the harness callback still runs (permissive).
	// GAP-059: a repeated POST /v1/process on an ACTIVE session accumulates
	// (turn_count++, started_at preserved) and only a COMPLETED session is
	// re-opened with started_at/turn_count reset — the docs' semantics. The
	// whole decision is one locked transaction, so no concurrent process can
	// interleave a check with this write (GAP-043 read-side equivalent).
	s.sessions.beginProcessTurn(req.SessionID)

	decision, err := s.harness.OnProcess(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	// Auto-generate decision_id if harness didn't set one (H3 protocol §2.1).
	if decision.DecisionID == "" {
		decision.DecisionID = generateUUID()
	}

	if err := decision.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInvalidDecision, err.Error())
		return
	}

	// GAP-009: record the finalized decision so GET /v1/sessions/{id} and
	// POST /v1/cancel can report what was in flight.
	// GAP-DOG-003: transition session status to completed when the harness
	// returns DecisionEnd from OnProcess.
	// GAP-028: cancelled is terminal — do not rewrite lifecycle state for
	// already-cancelled sessions.
	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		if e.Status == "cancelled" {
			return
		}
		e.CurrentDecisionID = decision.DecisionID
		e.CurrentDecisionType = decision.Decision
		if decision.Decision == protocol.DecisionEnd {
			e.Status = "completed"
		}
	})

	writeJSON(w, http.StatusOK, decision)
}

// resultHandler handles POST /v1/result.
func (s *server) resultHandler(w http.ResponseWriter, r *http.Request) {
	var req protocol.ResultRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}

	snap, ok := s.sessions.snapshot(req.SessionID)
	if !ok {
		writeError(w, http.StatusNotFound, protocol.ErrSessionNotFound,
			"session not found: "+req.SessionID)
		return
	}

	// GAP-058: one result delivery per session at a time. Deliveries that
	// share a session serialise here, so a delivery arriving while another one
	// for the same session is mid-flight waits for it and is then judged
	// against the bookkeeping that delivery left behind — instead of racing its
	// OnResult. GAP-049's correlation alone could not prevent that race: the
	// check and the record it wrote were two separate lock acquisitions with
	// OnResult in between.
	if mu := snap.deliveryMu; mu != nil {
		mu.Lock()
		defer mu.Unlock()
	}

	// GAP-049: correlate the result with the session's in-flight decision
	// BEFORE OnResult runs, so at-least-once delivery (a client retry, or two
	// clients sharing one chat id) cannot re-execute the result's side effects.
	// Nothing below may call OnResult unless this block accepts the id.
	// GAP-043: read the ids through snapshot() — never dereference the raw
	// *sessionEntry from get(), which is unlocked against update().
	//
	// The rule, when a decision IS in flight (CurrentDecisionID != ""):
	//
	//   * decision_id == in-flight            -> accept (the normal path).
	//   * decision_id == last resolved id     -> 400 "already been resolved".
	//     This is the idempotency guard: a retrying client may treat that 400
	//     as "already applied" instead of replaying the side effect.
	//   * anything else                       -> 400 "does not match the
	//     session's in-flight decision" — an invented or long-stale id must not
	//     drive the loop.
	//
	// When NO decision is in flight (CurrentDecisionID == "", e.g. the session
	// was created and the harness has not answered yet) the result is accepted
	// as before: there is nothing to correlate against, and rejecting it would
	// break the permissive contract for a session with no outstanding work.
	//
	// GAP-058: that check and the CLAIM it implies are now ONE transaction
	// under the store's write lock (claimResultDelivery), and the claim is
	// recorded before OnResult is invoked below. Two deliveries of the same
	// decision_id therefore cannot both pass the check: the first claims the id
	// and advances the session, the second is judged against the state the
	// first left behind and refused with 400.
	reject := ""
	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		reject = claimResultDelivery(e, req.SessionID, req.DecisionID)
	})
	if reject != "" {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, reject)
		return
	}

	// The claim belongs to this delivery until it finishes. Releasing it on the
	// way out (every path — success, OnResult error, invalid decision, panic)
	// keeps a delivery that did NOT resolve its decision retryable: the retry
	// is checked against bookkeeping this delivery never advanced.
	defer s.sessions.releaseResultClaim(req.SessionID, req.DecisionID)

	decision, err := s.harness.OnResult(&req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	// Auto-generate decision_id if harness didn't set one (H3 protocol §2.1).
	if decision.DecisionID == "" {
		decision.DecisionID = generateUUID()
	}

	if err := decision.Validate(); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInvalidDecision, err.Error())
		return
	}

	// GAP-009: record the finalized decision so GET /v1/sessions/{id} and
	// POST /v1/cancel can report what was in flight.
	// GAP-DOG-003: transition session status to completed when the harness
	// returns DecisionEnd from OnResult.
	// GAP-028: cancelled is terminal — do not rewrite lifecycle state for
	// already-cancelled sessions.
	// GAP-049: also record the id this result was FOR. It is the retry
	// fingerprint: once CurrentDecisionID moves on (to the decision just
	// returned), a re-delivery of this same id matches LastResultDecisionID
	// instead of the in-flight decision and is rejected as already resolved.
	// GAP-058: this is also where the delivery's claim ends — the same
	// transaction that advances the session releases it, so no window exists in
	// which the session has advanced but the id still reads as claimed.
	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		if e.ResultClaimID == req.DecisionID {
			e.ResultClaimID = ""
		}
		if e.Status == "cancelled" {
			return
		}
		e.LastResultDecisionID = req.DecisionID
		e.CurrentDecisionID = decision.DecisionID
		e.CurrentDecisionType = decision.Decision
		if decision.Decision == protocol.DecisionEnd {
			e.Status = "completed"
		}
	})

	writeJSON(w, http.StatusOK, decision)
}

// cancelHandler handles POST /v1/cancel.
func (s *server) cancelHandler(w http.ResponseWriter, r *http.Request) {
	var req protocol.CancelRequest
	if !s.decodeBody(w, r, &req) {
		return
	}

	// GAP-048: validate BEFORE the session lookup — a malformed cancel must
	// surface as 400 INVALID_REQUEST, never as a 404 SESSION_NOT_FOUND with an
	// empty session name (which reads to a consumer as a vanished session).
	if err := req.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, protocol.ErrInvalidRequest, err.Error())
		return
	}

	if s.sessions.get(req.SessionID) == nil {
		writeError(w, http.StatusNotFound, protocol.ErrSessionNotFound,
			"session not found: "+req.SessionID)
		return
	}

	if err := s.harness.OnCancel(&req); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	// GAP-009: report the decision that was in flight when cancel arrived.
	// If no decision was ever finalized for the session this stays "" —
	// nothing was in flight, which is the correct empty value.
	// GAP-043: read the decision id through snapshot() (locked) instead of
	// dereferencing a raw *sessionEntry from get() with no lock held.
	cancelledDecisionID := ""
	if entry, ok := s.sessions.snapshot(req.SessionID); ok {
		cancelledDecisionID = entry.CurrentDecisionID
	}

	s.sessions.update(req.SessionID, func(e *sessionEntry) {
		e.Status = "cancelled"
	})

	writeJSON(w, http.StatusOK, protocol.CancelResponse{
		Cancelled:           true,
		CancelledDecisionID: cancelledDecisionID,
	})
}

// getSessionHandler handles GET /v1/sessions/{id}.
func (s *server) getSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")
	// GAP-043: every field read below comes from a locked snapshot rather than
	// a raw *sessionEntry dereferenced after get() returned.
	entry, ok := s.sessions.snapshot(sessionID)
	if !ok {
		writeError(w, http.StatusNotFound, protocol.ErrSessionNotFound,
			"session not found: "+sessionID)
		return
	}

	resp := &protocol.SessionResponse{
		SessionID:           entry.SessionID,
		StartedAt:           entry.StartedAt.Format(time.RFC3339),
		LastActive:          entry.LastActive.Format(time.RFC3339),
		TurnCount:           entry.TurnCount,
		Status:              protocol.SessionStatus(entry.Status),
		CurrentDecision:     entry.CurrentDecisionID,
		CurrentDecisionType: entry.CurrentDecisionType,
	}
	writeJSON(w, http.StatusOK, resp)
}

// deleteSessionHandler handles DELETE /v1/sessions/{id}.
func (s *server) deleteSessionHandler(w http.ResponseWriter, r *http.Request) {
	sessionID := r.PathValue("id")

	entry := s.sessions.get(sessionID)
	if entry == nil {
		writeError(w, http.StatusNotFound, protocol.ErrSessionNotFound,
			"session not found: "+sessionID)
		return
	}

	if err := s.harness.OnSessionTerminate(sessionID); err != nil {
		writeError(w, http.StatusInternalServerError, protocol.ErrInternalError, err.Error())
		return
	}

	s.sessions.delete(sessionID)
	writeJSON(w, http.StatusOK, protocol.SessionTerminateResponse{Terminated: true, SessionID: sessionID})
}

// generateUUID creates a UUIDv4 string using crypto/rand.
// Zero external dependencies — stdlib only.
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand.Read failure is catastrophic (system entropy exhausted).
		// In practice this never happens on Linux; the fallback keeps the
		// function from needing to return an error in the hot path.
		panic("harness: crypto/rand.Read failed: " + err.Error())
	}
	// Set UUID version 4 and variant bits.
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10xx
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
