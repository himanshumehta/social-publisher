package router

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/handler"
)

const testSecret = "router-test-secret-that-is-long-enough-123"

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{Env: "development", FrontendURL: "http://localhost:5173"},
		JWT: config.JWTConfig{Secret: testSecret, AccessTokenTTL: time.Minute},
	}
}

func token(t *testing.T, role string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "11111111-1111-1111-1111-111111111111",
		"role": role,
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	s, err := tok.SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// reached replaces every handler so the test can tell whether a request got
// past the access middleware.
const reachedStatus = http.StatusTeapot

func stubbedRoutes() []Route {
	routes := Routes(handler.New(testConfig(), nil, slog.Default()), handler.NewHealthHandler(nil, nil))
	for i := range routes {
		routes[i].Handler = func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(reachedStatus) }
	}
	return routes
}

var (
	pathParam = regexp.MustCompile(`\{[^}]+\}`)
	clientN   int
)

// requestFor builds a request for the route from a fresh client address, so
// the global per-IP rate limiter does not interfere with this test.
func requestFor(rt Route) *http.Request {
	method, path, _ := strings.Cut(rt.Pattern, " ")
	path = pathParam.ReplaceAllString(path, "22222222-2222-2222-2222-222222222222")
	r := httptest.NewRequest(method, path, nil)
	clientN++
	r.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1234", clientN/250, clientN%250+1)
	return r
}

func serve(h http.Handler, r *http.Request) int {
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	_, _ = io.Copy(io.Discard, w.Result().Body)
	return w.Code
}

func TestRoutes_AccessLevels(t *testing.T) {
	routes := stubbedRoutes()
	h := build(testConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)), routes, nil)
	creator := token(t, "creator")
	admin := token(t, "admin")

	for _, rt := range routes {
		t.Run(rt.Pattern, func(t *testing.T) {
			anon := serve(h, requestFor(rt))
			switch rt.Access {
			case Public:
				if anon != reachedStatus {
					t.Fatalf("public route: status %d without credentials", anon)
				}
				return
			default:
				if anon != http.StatusUnauthorized {
					t.Fatalf("no credentials: status %d, want 401", anon)
				}
			}
			if rt.Access == APIKey {
				// A user JWT is not an API key.
				r := requestFor(rt)
				r.Header.Set("Authorization", "Bearer "+admin)
				if got := serve(h, r); got == reachedStatus {
					t.Fatal("API-key route accepted a user JWT")
				}
				return
			}

			r := requestFor(rt)
			r.Header.Set("Authorization", "Bearer "+creator)
			got := serve(h, r)
			if rt.Access == Admin && got != http.StatusForbidden {
				t.Fatalf("creator on admin route: status %d, want 403", got)
			}
			if rt.Access == User && got != reachedStatus {
				t.Fatalf("creator on user route: status %d, want handler", got)
			}

			r = requestFor(rt)
			r.Header.Set("Authorization", "Bearer "+admin)
			if got := serve(h, r); got != reachedStatus {
				t.Fatalf("admin: status %d, want handler", got)
			}
		})
	}
}

// TestRoutes_PublicSurface pins the exact set of unauthenticated routes, so a
// new public route is a deliberate, reviewed change.
func TestRoutes_PublicSurface(t *testing.T) {
	want := map[string]bool{
		"GET /health":                                         true,
		"GET /health/ready":                                   true,
		"POST /api/v1/auth/login":                             true,
		"POST /api/v1/auth/refresh":                           true,
		"GET /api/v1/admin/channels/oauth/google/callback":    true,
		"GET /api/v1/admin/channels/oauth/facebook/callback":  true,
		"GET /api/v1/admin/channels/oauth/instagram/callback": true,
		"GET /api/v1/admin/channels/oauth/threads/callback":   true,
		"GET /api/v1/admin/channels/oauth/twitter/callback":   true,
		"GET /api/v1/admin/channels/oauth/linkedin/callback":  true,
	}
	got := map[string]bool{}
	for _, rt := range stubbedRoutes() {
		if rt.Access == Public {
			got[rt.Pattern] = true
			if !want[rt.Pattern] {
				t.Errorf("unexpected public route %q", rt.Pattern)
			}
		}
	}
	for p := range want {
		if !got[p] {
			t.Errorf("expected public route %q is missing", p)
		}
	}
}

// TestRoutes_VideoPathIsAdminOnly guards against re-exposing project, upload,
// video or publish-record data to non-admin accounts (IDOR).
func TestRoutes_VideoPathIsAdminOnly(t *testing.T) {
	for _, rt := range stubbedRoutes() {
		_, path, _ := strings.Cut(rt.Pattern, " ")
		for _, prefix := range []string{"/api/v1/projects", "/api/v1/uploads", "/api/v1/videos", "/api/v1/publish-records", "/api/v1/admin/"} {
			if strings.HasPrefix(path, prefix) && rt.Access != Admin && !strings.Contains(path, "/oauth/") {
				t.Errorf("%s is %v, want Admin", rt.Pattern, rt.Access)
			}
		}
	}
}

func TestUnknownRouteIsJSON404(t *testing.T) {
	h := build(testConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)), stubbedRoutes(), nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "not found") {
		t.Fatalf("got %d %q", w.Code, w.Body.String())
	}
}

func TestCredentialRoutesHaveStricterRateLimit(t *testing.T) {
	h := build(testConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)), stubbedRoutes(), nil)
	// Login and refresh share one per-IP credential budget.
	for n, pattern := range []string{"POST /api/v1/auth/login", "POST /api/v1/auth/refresh"} {
		var codes []int
		for i := 0; i < authBurst+1; i++ {
			method, path, _ := strings.Cut(pattern, " ")
			r := httptest.NewRequest(method, path, nil)
			r.RemoteAddr = fmt.Sprintf("203.0.113.%d:5555", 50+n)
			codes = append(codes, serve(h, r))
		}
		if codes[authBurst-1] != reachedStatus || codes[authBurst] != http.StatusTooManyRequests {
			t.Errorf("%s: statuses %v, want %d allowed then 429", pattern, codes, authBurst)
		}
	}
}
