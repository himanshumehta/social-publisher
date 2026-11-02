package router

import (
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/handler"
	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/service"
)

// Access is the authentication/authorization requirement of a route.
type Access int

const (
	// Public routes need no credentials (health, login, OAuth provider callbacks).
	Public Access = iota
	// User routes need a valid access token; handlers scope data to the caller.
	User
	// Admin routes need a valid access token with an admin role.
	Admin
	// APIKey routes need a scoped API key (machine clients).
	APIKey
)

// Route is one entry of the API surface. Keeping the surface in a table lets
// tests assert the access level of every route.
type Route struct {
	Pattern string // "METHOD /path" as understood by http.ServeMux
	Access  Access
	Handler http.HandlerFunc
}

const (
	apiRatePerSecond  = 10 // per client IP, all routes
	apiBurst          = 30
	authRatePerSecond = 10.0 / 60 // per client IP on credential routes: 10/min
	authBurst         = 5
)

// credentialRoutes accept secrets (passwords, refresh tokens) and get the
// stricter auth rate limit.
var credentialRoutes = map[string]bool{
	"POST /api/v1/auth/login":   true,
	"POST /api/v1/auth/refresh": true,
}

// Routes returns the complete API surface.
func Routes(h *handler.Handler, hh *handler.HealthHandler) []Route {
	return []Route{
		// Health
		{"GET /health", Public, handler.Health},
		{"GET /health/ready", Public, hh.Ready},

		// Auth
		{"POST /api/v1/auth/login", Public, h.Login},
		{"POST /api/v1/auth/refresh", Public, h.RefreshToken},
		{"POST /api/v1/auth/logout", User, h.Logout},
		{"GET /api/v1/auth/me", User, h.GetMe},
		{"GET /api/v1/categories", User, h.ListCategories},

		// Posts (text / image). Non-admin callers only see their own posts.
		{"POST /api/v1/posts", User, h.CreatePost},
		{"GET /api/v1/posts", User, h.ListPosts},
		{"GET /api/v1/posts/{id}", User, h.GetPost},
		{"PUT /api/v1/posts/{id}", User, h.UpdatePost},
		{"DELETE /api/v1/posts/{id}", User, h.DeletePost},
		{"POST /api/v1/posts/{id}/regenerate", User, h.RegeneratePost},
		{"POST /api/v1/posts/{id}/submit", User, h.SubmitPost},

		// Scheduling (approved posts only; see CalendarService.SchedulePost)
		{"GET /api/v1/calendar", User, h.GetCalendar},
		{"PATCH /api/v1/posts/{id}/schedule", User, h.SchedulePost},
		{"POST /api/v1/posts/bulk-schedule", User, h.BulkSchedulePost},
		{"DELETE /api/v1/posts/{id}/schedule", User, h.UnschedulePost},

		// Review and publish
		{"GET /api/v1/admin/posts", Admin, h.ListAdminPosts},
		{"GET /api/v1/admin/posts/review-queue", Admin, h.GetAdminReviewQueue},
		{"POST /api/v1/admin/posts/{id}/approve", Admin, h.ApprovePost},
		{"POST /api/v1/admin/posts/{id}/reject", Admin, h.RejectPost},
		{"POST /api/v1/admin/posts/{id}/publish", Admin, h.AdminPublishPost},
		{"GET /api/v1/admin/posts/{id}/publish-status", Admin, h.GetPostPublishStatus},

		// Machine clients
		{"POST /api/v1/external/posts", APIKey, h.ExternalCreatePost},
		{"GET /api/v1/external/posts/{id}", APIKey, h.ExternalGetPost},

		// Videos. This is an operator path, so every route is admin-only.
		{"POST /api/v1/admin/projects", Admin, h.CreateProject},
		{"GET /api/v1/admin/projects", Admin, h.ListAllProjects},
		{"GET /api/v1/projects/{id}", Admin, h.GetProject},
		{"DELETE /api/v1/admin/projects/{id}", Admin, h.DeleteProject},
		{"POST /api/v1/uploads/initiate", Admin, h.InitiateUpload},
		{"POST /api/v1/uploads/complete", Admin, h.CompleteUpload},
		{"POST /api/v1/uploads/abort", Admin, h.AbortUpload},
		{"POST /api/v1/uploads/refresh-urls", Admin, h.RefreshUploadURLs},
		{"GET /api/v1/projects/{id}/videos", Admin, h.ListVideos},
		{"GET /api/v1/videos/{videoId}/playback", Admin, h.GetVideoPlaybackURL},
		{"DELETE /api/v1/videos/{videoId}", Admin, h.DeleteVideo},
		{"POST /api/v1/videos/{id}/publish-to-platform", Admin, h.PublishToPlatform},
		{"GET /api/v1/projects/{id}/publish-records", Admin, h.ListPublishRecords},
		{"GET /api/v1/publish-records/{id}/upload-status", Admin, h.GetUploadStatus},
		{"GET /api/v1/publish-records/{id}/upload-status/stream", Admin, h.StreamUploadStatus},
		{"GET /api/v1/admin/quota", Admin, h.GetQuotaStatus},
		{"GET /api/v1/admin/publish-history", Admin, h.ListPublishHistory},
		{"POST /api/v1/admin/publish-records/{id}/retry-thumbnail", Admin, h.RetryThumbnail},

		// Channels
		{"GET /api/v1/admin/channels", Admin, h.ListChannels},
		{"POST /api/v1/admin/channels", Admin, h.CreateChannel},
		{"PUT /api/v1/admin/channels/{id}", Admin, h.UpdateChannel},
		{"DELETE /api/v1/admin/channels/{id}", Admin, h.DeleteChannel},
		{"POST /api/v1/admin/channels/{id}/disconnect", Admin, h.DisconnectChannel},
		{"GET /api/v1/admin/channels/{id}/status", Admin, h.GetChannelStatus},
		{"POST /api/v1/admin/channels/{id}/refresh-token", Admin, h.RefreshChannelToken},
		{"POST /api/v1/admin/channels/{id}/test-connection", Admin, h.TestChannelConnection},

		// Channel OAuth: an admin requests the authorization URL; the provider
		// redirects the browser to a public callback that is bound to that
		// request by a single-use, short-lived state value.
		{"GET /api/v1/admin/channels/{id}/oauth/{provider}/url", Admin, h.GetChannelOAuthURL},
		{"GET /api/v1/admin/channels/oauth/google/callback", Public, h.GoogleChannelCallback},
		{"GET /api/v1/admin/channels/oauth/facebook/callback", Public, h.FacebookChannelCallback},
		{"GET /api/v1/admin/channels/oauth/instagram/callback", Public, h.InstagramChannelCallback},
		{"GET /api/v1/admin/channels/oauth/threads/callback", Public, h.ThreadsChannelCallback},
		{"GET /api/v1/admin/channels/oauth/twitter/callback", Public, h.TwitterChannelCallback},
		{"GET /api/v1/admin/channels/oauth/linkedin/callback", Public, h.LinkedInChannelCallback},
	}
}

// New builds the HTTP handler: every route from Routes wrapped for its access
// level, behind the global middleware chain.
func New(cfg *config.Config, svc *service.Services, logger *slog.Logger, pool *pgxpool.Pool, rdb *redis.Client) http.Handler {
	h := handler.New(cfg, svc, logger)
	hh := handler.NewHealthHandler(pool, rdb)

	var apiKeys *service.APIKeyService
	if svc != nil {
		apiKeys = svc.APIKey
	}
	return build(cfg, logger, Routes(h, hh), apiKeys)
}

func build(cfg *config.Config, logger *slog.Logger, routes []Route, apiKeys *service.APIKeyService) http.Handler {
	authMw := middleware.RequireAuth(cfg.JWT.Secret)
	adminMw := middleware.RequireRole("admin", "super_admin")
	apiKeyMw := middleware.RequireAPIKey(apiKeys)

	// Credential endpoints get a much smaller budget than the API as a whole
	// to slow password guessing and refresh-token probing.
	authLimiter := middleware.NewRateLimiter(authRatePerSecond, authBurst, cfg.TrustProxyHeaders)

	mux := http.NewServeMux()
	for _, rt := range routes {
		var hnd http.Handler = rt.Handler
		switch rt.Access {
		case User:
			hnd = authMw(hnd)
		case Admin:
			hnd = authMw(adminMw(hnd))
		case APIKey:
			hnd = apiKeyMw(hnd)
		}
		if credentialRoutes[rt.Pattern] {
			hnd = authLimiter.Middleware(hnd)
		}
		mux.Handle(rt.Pattern, hnd)
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	})

	rl := middleware.NewRateLimiter(apiRatePerSecond, apiBurst, cfg.TrustProxyHeaders)

	var chain http.Handler = mux
	chain = rl.Middleware(chain)
	chain = middleware.RequestID(chain)
	chain = middleware.CORS(cfg.App.FrontendURL)(chain)
	chain = middleware.Logging(logger)(chain)
	chain = middleware.Recovery(chain)
	return chain
}
