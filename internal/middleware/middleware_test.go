package middleware

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testJWTSecret = "test-secret-key-for-unit-testing-minimum-32-chars"

func createTestJWT(userID uuid.UUID, email, role string, expiry time.Time) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   userID.String(),
		"email": email,
		"role":  role,
		"exp":   expiry.Unix(),
		"iat":   time.Now().Unix(),
	})
	signed, _ := token.SignedString([]byte(testJWTSecret))
	return signed
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	})
}

// ---------------------------------------------------------------------------
// Auth Middleware Tests
// ---------------------------------------------------------------------------

func TestRequireAuth_MissingHeader(t *testing.T) {
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidFormat(t *testing.T) {
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic abc123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	uid := uuid.New()
	token := createTestJWT(uid, "test@example.com", "creator", time.Now().Add(-1*time.Hour))
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_WrongSecret(t *testing.T) {
	uid := uuid.New()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": uid.String(), "email": "test@example.com", "role": "creator",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})
	signed, _ := token.SignedString([]byte("wrong-secret-key-that-is-different"))
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	uid := uuid.New()
	token := createTestJWT(uid, "test@example.com", "creator", time.Now().Add(time.Hour))
	mw := RequireAuth(testJWTSecret)

	var gotUserID uuid.UUID
	var gotEmail, gotRole string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = UserIDFromContext(r.Context())
		gotEmail, _ = r.Context().Value(ContextKeyEmail).(string)
		gotRole = RoleFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	handler := mw(inner)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if gotUserID != uid {
		t.Errorf("expected user ID %s, got %s", uid, gotUserID)
	}
	if gotEmail != "test@example.com" {
		t.Errorf("expected email test@example.com, got %s", gotEmail)
	}
	if gotRole != "creator" {
		t.Errorf("expected role creator, got %s", gotRole)
	}
}

func TestRequireAuth_BearerCaseInsensitive(t *testing.T) {
	uid := uuid.New()
	token := createTestJWT(uid, "test@example.com", "admin", time.Now().Add(time.Hour))
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "BEARER "+token)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for case-insensitive Bearer, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidSubjectUUID(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":   "not-a-uuid",
		"email": "test@example.com",
		"role":  "creator",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
	})
	signed, _ := token.SignedString([]byte(testJWTSecret))
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer "+signed)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid UUID subject, got %d", w.Code)
	}
}

func TestRequireAuth_EmptyBearerToken(t *testing.T) {
	mw := RequireAuth(testJWTSecret)
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty bearer token, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Context Helper Tests
// ---------------------------------------------------------------------------

func TestUserIDFromContext_Empty(t *testing.T) {
	_, ok := UserIDFromContext(context.Background())
	if ok {
		t.Error("expected false for empty context")
	}
}

func TestUserIDFromContext_Valid(t *testing.T) {
	uid := uuid.New()
	ctx := context.WithValue(context.Background(), ContextKeyUserID, uid)
	got, ok := UserIDFromContext(ctx)
	if !ok {
		t.Error("expected true for valid context")
	}
	if got != uid {
		t.Errorf("expected %s, got %s", uid, got)
	}
}

func TestRoleFromContext_Empty(t *testing.T) {
	role := RoleFromContext(context.Background())
	if role != "" {
		t.Errorf("expected empty string, got %s", role)
	}
}

func TestRoleFromContext_Valid(t *testing.T) {
	ctx := context.WithValue(context.Background(), ContextKeyRole, "admin")
	role := RoleFromContext(ctx)
	if role != "admin" {
		t.Errorf("expected admin, got %s", role)
	}
}

// ---------------------------------------------------------------------------
// RBAC Tests
// ---------------------------------------------------------------------------

func TestRequireRole_Allowed(t *testing.T) {
	mw := RequireRole("admin", "super_admin")
	ctx := context.WithValue(context.Background(), ContextKeyRole, "admin")
	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler := mw(okHandler())
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireRole_AllowedSecondRole(t *testing.T) {
	mw := RequireRole("admin", "super_admin")
	ctx := context.WithValue(context.Background(), ContextKeyRole, "super_admin")
	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler := mw(okHandler())
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for super_admin, got %d", w.Code)
	}
}

func TestRequireRole_Forbidden(t *testing.T) {
	mw := RequireRole("admin", "super_admin")
	ctx := context.WithValue(context.Background(), ContextKeyRole, "creator")
	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handler := mw(okHandler())
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireRole_NoRole(t *testing.T) {
	mw := RequireRole("admin")
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler := mw(okHandler())
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when no role in context, got %d", w.Code)
	}
}

func TestRequireRole_ForbiddenResponseBody(t *testing.T) {
	mw := RequireRole("admin")
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler := mw(okHandler())
	handler.ServeHTTP(w, req)
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body, got error: %v", err)
	}
	if body["error"] != "forbidden" {
		t.Errorf("expected error=forbidden, got %s", body["error"])
	}
}

// ---------------------------------------------------------------------------
// CORS Tests
// ---------------------------------------------------------------------------

func TestCORS_MatchingOrigin(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:3000" {
		t.Errorf("expected Allow-Origin http://localhost:3000, got %s", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("expected Allow-Credentials true, got %s", got)
	}
}

func TestCORS_NonMatchingOrigin(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin for non-matching origin, got %s", got)
	}
}

func TestCORS_Wildcard(t *testing.T) {
	mw := CORS("*")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://anything.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// A wildcard with credentials would let any site read authenticated
	// responses; it must allow nothing (config.Validate also rejects it).
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("wildcard must not reflect arbitrary origins, got %s", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("wildcard must not allow credentials, got %s", got)
	}
}

func TestCORS_Preflight(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
	// Verify the inner handler was NOT called (preflight returns early)
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body for preflight, got %s", w.Body.String())
	}
}

func TestCORS_PreflightNonMatchingOrigin(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204 for preflight, got %d", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin for non-matching preflight, got %s", got)
	}
}

func TestCORS_AllowedMethods(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	methods := w.Header().Get("Access-Control-Allow-Methods")
	if methods != "GET, POST, PUT, PATCH, DELETE, OPTIONS" {
		t.Errorf("unexpected Allow-Methods: %s", methods)
	}
}

func TestCORS_AllowedHeaders(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	headers := w.Header().Get("Access-Control-Allow-Headers")
	if headers != "Content-Type, Authorization, X-Request-ID" {
		t.Errorf("unexpected Allow-Headers: %s", headers)
	}
}

func TestCORS_MaxAge(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("expected Max-Age 86400, got %s", got)
	}
}

func TestCORS_NoOriginHeader(t *testing.T) {
	mw := CORS("http://localhost:3000")
	handler := mw(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	// No Origin header means origin="" which does not match frontendURL
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("expected no Allow-Origin when no Origin header, got %s", got)
	}
	// Should still pass through to the handler
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for normal GET without Origin, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// RequestID Tests
// ---------------------------------------------------------------------------

func TestRequestID_ExistingHeader(t *testing.T) {
	handler := RequestID(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "existing-id-123")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "existing-id-123" {
		t.Errorf("expected existing-id-123, got %s", got)
	}
}

func TestRequestID_CFRay(t *testing.T) {
	handler := RequestID(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("CF-Ray", "cf-ray-456")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "cf-ray-456" {
		t.Errorf("expected cf-ray-456, got %s", got)
	}
}

func TestRequestID_Generated(t *testing.T) {
	handler := RequestID(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	id := w.Header().Get("X-Request-ID")
	if id == "" {
		t.Error("expected generated request ID, got empty")
	}
	// Should be a valid UUID
	if _, err := uuid.Parse(id); err != nil {
		t.Errorf("expected UUID format, got %s: %v", id, err)
	}
}

func TestRequestID_PriorityOverCFRay(t *testing.T) {
	handler := RequestID(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "explicit-id")
	req.Header.Set("CF-Ray", "cf-ray-should-be-ignored")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if got := w.Header().Get("X-Request-ID"); got != "explicit-id" {
		t.Errorf("expected X-Request-ID to take priority over CF-Ray, got %s", got)
	}
}

func TestRequestID_SetOnRequest(t *testing.T) {
	var gotID string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = r.Header.Get("X-Request-ID")
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestID(inner)
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Request-ID", "propagated-id")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if gotID != "propagated-id" {
		t.Errorf("expected request header to contain propagated-id, got %s", gotID)
	}
}

// ---------------------------------------------------------------------------
// Recovery Tests
// ---------------------------------------------------------------------------

func TestRecovery_NoPanic(t *testing.T) {
	handler := Recovery(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRecovery_WithPanic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})
	handler := Recovery(panicHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRecovery_WithPanicResponseBody(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("something broke")
	})
	handler := Recovery(panicHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body, got error: %v", err)
	}
	if body["error"] != "internal server error" {
		t.Errorf("expected error=internal server error, got %s", body["error"])
	}
}

func TestRecovery_WithNonStringPanic(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(42)
	})
	handler := Recovery(panicHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for integer panic, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Logging Tests
// ---------------------------------------------------------------------------

func TestLogging_CapturesStatusAndSize(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	customHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("hello"))
	})
	handler := Logging(logger)(customHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if w.Body.String() != "hello" {
		t.Errorf("expected body hello, got %s", w.Body.String())
	}
}

func TestLogging_DefaultStatus(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	customHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Write without explicit WriteHeader - should default to 200
		w.Write([]byte("ok"))
	})
	handler := Logging(logger)(customHandler)
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 default status, got %d", w.Code)
	}
}

func TestResponseWriter_DefaultStatus(t *testing.T) {
	rw := &responseWriter{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	if rw.status != http.StatusOK {
		t.Errorf("expected default status 200, got %d", rw.status)
	}
}

func TestResponseWriter_WriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	rw.WriteHeader(http.StatusNotFound)
	if rw.status != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rw.status)
	}
}

func TestResponseWriter_WriteTracksSize(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, status: http.StatusOK}
	n, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected write error: %v", err)
	}
	if n != 5 {
		t.Errorf("expected 5 bytes written, got %d", n)
	}
	if rw.size != 5 {
		t.Errorf("expected size 5, got %d", rw.size)
	}
	// Write again to verify accumulation
	rw.Write([]byte(" world"))
	if rw.size != 11 {
		t.Errorf("expected accumulated size 11, got %d", rw.size)
	}
}

// ---------------------------------------------------------------------------
// Rate Limiter Tests
// ---------------------------------------------------------------------------

func TestRateLimiter_AllowsWithinBurst(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     10,
		burst:    5,
	}
	for i := range 5 {
		if !rl.allow("1.2.3.4") {
			t.Errorf("request %d should be allowed within burst", i+1)
		}
	}
}

func TestRateLimiter_BlocksAfterBurst(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     0.1, // very slow refill
		burst:    3,
	}
	// Exhaust burst
	for range 3 {
		rl.allow("1.2.3.4")
	}
	// Next should be blocked
	if rl.allow("1.2.3.4") {
		t.Error("expected request to be blocked after burst exhausted")
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     0.1,
		burst:    2,
	}
	// Exhaust IP1 burst
	rl.allow("1.1.1.1")
	rl.allow("1.1.1.1")
	if rl.allow("1.1.1.1") {
		t.Error("IP1 should be blocked")
	}
	// IP2 should still be allowed
	if !rl.allow("2.2.2.2") {
		t.Error("IP2 should be allowed (separate bucket)")
	}
}

func TestRateLimiter_RefillsOverTime(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     100, // fast refill for testing
		burst:    2,
	}
	rl.allow("1.1.1.1")
	rl.allow("1.1.1.1")
	// Simulate time passing
	rl.mu.Lock()
	rl.visitors["1.1.1.1"].lastSeen = time.Now().Add(-1 * time.Second)
	rl.mu.Unlock()
	// Should now be allowed (refilled)
	if !rl.allow("1.1.1.1") {
		t.Error("expected request to be allowed after token refill")
	}
}

func TestRateLimiter_BurstOfOne(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     0.001,
		burst:    1,
	}
	// First request should pass
	if !rl.allow("1.1.1.1") {
		t.Error("first request with burst=1 should be allowed")
	}
	// Second should fail
	if rl.allow("1.1.1.1") {
		t.Error("second request with burst=1 should be blocked")
	}
}

func TestRateLimiter_Middleware_HTTPStatus(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     0.001,
		burst:    1,
	}
	handler := rl.Middleware(okHandler())
	// First request OK
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("first request should be 200, got %d", w.Code)
	}
	// Second request blocked
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("second request should be 429, got %d", w.Code)
	}
}

func TestRateLimiter_Middleware_RateLimitResponseBody(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     0.001,
		burst:    1,
	}
	handler := rl.Middleware(okHandler())
	// Exhaust
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	// Now blocked
	req = httptest.NewRequest("GET", "/test", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body, got error: %v", err)
	}
	if body["error"] != "rate limit exceeded" {
		t.Errorf("expected error=rate limit exceeded, got %s", body["error"])
	}
}

func TestRateLimiter_Middleware_CFConnectingIP(t *testing.T) {
	rl := &RateLimiter{
		visitors:   make(map[string]*visitor),
		rate:       0.001,
		burst:      1,
		trustProxy: true,
	}
	handler := rl.Middleware(okHandler())
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("CF-Connecting-IP", "10.0.0.1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// Same CF IP, should be rate limited
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("CF-Connecting-IP", "10.0.0.1")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for same CF IP, got %d", w2.Code)
	}
}

func TestRateLimiter_Middleware_DifferentCFIPs(t *testing.T) {
	rl := &RateLimiter{
		visitors:   make(map[string]*visitor),
		rate:       0.001,
		burst:      1,
		trustProxy: true,
	}
	handler := rl.Middleware(okHandler())

	// First IP
	req1 := httptest.NewRequest("GET", "/test", nil)
	req1.Header.Set("CF-Connecting-IP", "10.0.0.1")
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Errorf("expected 200 for first CF IP, got %d", w1.Code)
	}

	// Different CF IP should also be allowed
	req2 := httptest.NewRequest("GET", "/test", nil)
	req2.Header.Set("CF-Connecting-IP", "10.0.0.2")
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Errorf("expected 200 for different CF IP, got %d", w2.Code)
	}
}

func TestRateLimiter_IgnoresProxyHeadersUnlessTrusted(t *testing.T) {
	rl := &RateLimiter{visitors: make(map[string]*visitor), rate: 0.001, burst: 1}
	handler := rl.Middleware(okHandler())
	// Same TCP peer, a different spoofed header on every request: one bucket.
	for i, ip := range []string{"10.0.0.1", "10.0.0.2"} {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "198.51.100.7:4000"
		req.Header.Set("CF-Connecting-IP", ip)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		want := http.StatusOK
		if i == 1 {
			want = http.StatusTooManyRequests
		}
		if w.Code != want {
			t.Fatalf("request %d: status %d, want %d (spoofed header must not reset the bucket)", i, w.Code, want)
		}
	}
}

func TestRateLimiter_KeysByHostNotPort(t *testing.T) {
	rl := &RateLimiter{visitors: make(map[string]*visitor), rate: 0.001, burst: 1}
	handler := rl.Middleware(okHandler())
	codes := []int{}
	for _, addr := range []string{"198.51.100.7:4000", "198.51.100.7:4001"} {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = addr
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		codes = append(codes, w.Code)
	}
	if codes[1] != http.StatusTooManyRequests {
		t.Fatalf("new source port got a fresh bucket: %v", codes)
	}
}

func TestClientIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[2001:db8::1]:443"
	req.Header.Set("X-Real-IP", "203.0.113.9")
	if got := ClientIP(req, false); got != "2001:db8::1" {
		t.Errorf("untrusted: %q", got)
	}
	if got := ClientIP(req, true); got != "203.0.113.9" {
		t.Errorf("trusted: %q", got)
	}
	req.Header.Set("X-Real-IP", "not-an-ip")
	if got := ClientIP(req, true); got != "2001:db8::1" {
		t.Errorf("invalid header should fall back to peer: %q", got)
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := NewRateLimiter(10.0, 5, false)
	if rl == nil {
		t.Fatal("expected non-nil rate limiter")
	}
	if rl.rate != 10.0 {
		t.Errorf("expected rate 10.0, got %f", rl.rate)
	}
	if rl.burst != 5.0 {
		t.Errorf("expected burst 5.0, got %f", rl.burst)
	}
	if rl.visitors == nil {
		t.Error("expected initialized visitors map")
	}
}

// ---------------------------------------------------------------------------
// API Key Context Tests
// ---------------------------------------------------------------------------

func TestAPIKeyContext_HasScope_Found(t *testing.T) {
	akc := &APIKeyContext{
		KeyID:  uuid.New(),
		UserID: uuid.New(),
		Scopes: []string{"read", "write", "admin"},
	}
	if !akc.HasScope("write") {
		t.Error("expected HasScope to return true for existing scope 'write'")
	}
}

func TestAPIKeyContext_HasScope_NotFound(t *testing.T) {
	akc := &APIKeyContext{
		KeyID:  uuid.New(),
		UserID: uuid.New(),
		Scopes: []string{"read", "write"},
	}
	if akc.HasScope("delete") {
		t.Error("expected HasScope to return false for non-existing scope 'delete'")
	}
}

func TestAPIKeyContext_HasScope_EmptyScopes(t *testing.T) {
	akc := &APIKeyContext{
		KeyID:  uuid.New(),
		UserID: uuid.New(),
		Scopes: []string{},
	}
	if akc.HasScope("read") {
		t.Error("expected HasScope to return false for empty scopes list")
	}
}

func TestAPIKeyContext_HasScope_MultipleScopes(t *testing.T) {
	akc := &APIKeyContext{
		KeyID:  uuid.New(),
		UserID: uuid.New(),
		Scopes: []string{"projects:read", "projects:write", "billing:read", "admin:full"},
	}
	if !akc.HasScope("billing:read") {
		t.Error("expected HasScope to find 'billing:read' among multiple scopes")
	}
	if !akc.HasScope("projects:read") {
		t.Error("expected HasScope to find 'projects:read' (first element)")
	}
	if !akc.HasScope("admin:full") {
		t.Error("expected HasScope to find 'admin:full' (last element)")
	}
	if akc.HasScope("billing:write") {
		t.Error("expected HasScope to return false for 'billing:write' which is not in scopes")
	}
}

func TestGetAPIKey_NilContext(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	akc := GetAPIKey(req)
	if akc != nil {
		t.Errorf("expected nil API key context for request without API key, got %+v", akc)
	}
}

func TestGetAPIKey_ValidContext(t *testing.T) {
	keyID := uuid.New()
	userID := uuid.New()
	scopes := []string{"read", "write"}

	expected := &APIKeyContext{
		KeyID:  keyID,
		UserID: userID,
		Scopes: scopes,
	}

	ctx := context.WithValue(context.Background(), apiKeyContextKey{}, expected)
	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)

	akc := GetAPIKey(req)
	if akc == nil {
		t.Fatal("expected non-nil API key context")
	}
	if akc.KeyID != keyID {
		t.Errorf("expected KeyID %s, got %s", keyID, akc.KeyID)
	}
	if akc.UserID != userID {
		t.Errorf("expected UserID %s, got %s", userID, akc.UserID)
	}
	if len(akc.Scopes) != len(scopes) {
		t.Fatalf("expected %d scopes, got %d", len(scopes), len(akc.Scopes))
	}
	for i, s := range scopes {
		if akc.Scopes[i] != s {
			t.Errorf("expected scope[%d]=%s, got %s", i, s, akc.Scopes[i])
		}
	}
}

// ---------------------------------------------------------------------------
// JWT hardening
// ---------------------------------------------------------------------------

func TestRequireAuth_RejectsWeakTokens(t *testing.T) {
	secret := strings.Repeat("j", 40) // test-only HMAC key
	sub := "11111111-1111-1111-1111-111111111111"
	sign := func(method jwt.SigningMethod, claims jwt.MapClaims, key any) string {
		s, err := jwt.NewWithClaims(method, claims).SignedString(key)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	future := time.Now().Add(time.Hour).Unix()
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": sub, "exp": future}).
		SignedString(jwt.UnsafeAllowNoneSignatureType)

	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"valid HS256", sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub, "exp": future}, []byte(secret)), http.StatusOK},
		{"alg none", none, http.StatusUnauthorized},
		{"HS512 with right key", sign(jwt.SigningMethodHS512, jwt.MapClaims{"sub": sub, "exp": future}, []byte(secret)), http.StatusUnauthorized},
		{"missing exp", sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub}, []byte(secret)), http.StatusUnauthorized},
		{"expired", sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub, "exp": time.Now().Add(-time.Minute).Unix()}, []byte(secret)), http.StatusUnauthorized},
		{"issued in the future", sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub, "exp": future, "iat": future}, []byte(secret)), http.StatusUnauthorized},
		{"wrong key", sign(jwt.SigningMethodHS256, jwt.MapClaims{"sub": sub, "exp": future}, []byte(strings.Repeat("w", 40))), http.StatusUnauthorized},
	}
	h := RequireAuth(secret)(okHandler())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Fatalf("status %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestCORS_VaryOrigin(t *testing.T) {
	w := httptest.NewRecorder()
	CORS("http://localhost:3000")(okHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Header().Get("Vary") != "Origin" {
		t.Errorf("Vary = %q, want Origin (responses differ per origin)", w.Header().Get("Vary"))
	}
}

func TestSecurityHeaders(t *testing.T) {
	w := httptest.NewRecorder()
	SecurityHeaders(true)(okHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/posts", nil))
	for k, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"X-Frame-Options":           "DENY",
		"Referrer-Policy":           "no-referrer",
		"Content-Security-Policy":   "default-src 'none'; frame-ancestors 'none'",
		"Cache-Control":             "no-store",
		"Strict-Transport-Security": "max-age=31536000; includeSubDomains",
	} {
		if got := w.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
	w = httptest.NewRecorder()
	SecurityHeaders(false)(okHandler()).ServeHTTP(w, httptest.NewRequest("GET", "/health", nil))
	if w.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS set in development")
	}
}

func TestLimitBody(t *testing.T) {
	read := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, "too big", http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	h := LimitBody(16)(read)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader("small")))
	if w.Code != http.StatusOK {
		t.Fatalf("small body: %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("POST", "/", strings.NewReader(strings.Repeat("x", 17))))
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("declared oversize body: %d", w.Code)
	}
	// Chunked body with no Content-Length: the reader enforces the cap.
	req := httptest.NewRequest("POST", "/", io.NopCloser(strings.NewReader(strings.Repeat("x", 64))))
	req.ContentLength = -1
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("streamed oversize body: %d", w.Code)
	}
}
