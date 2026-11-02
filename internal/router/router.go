package router

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/handler"
	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/service"
)

func New(cfg *config.Config, svc *service.Services, logger *slog.Logger, pool *pgxpool.Pool, rdb *redis.Client) http.Handler {
	h := handler.New(cfg, svc, logger)
	hh := handler.NewHealthHandler(pool, rdb)
	rl := middleware.NewRateLimiter(10, 30) // 10 req/s, burst 30

	mux := http.NewServeMux()

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok"}`)
	})
	mux.HandleFunc("GET /health/ready", hh.Ready)

	// Auth (public)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.RefreshToken)

	// Categories (public)
	mux.HandleFunc("GET /api/v1/categories", h.ListCategories)

	authMw := middleware.RequireAuth(cfg.JWT.Secret)
	adminMw := middleware.RequireRole("admin", "super_admin")

	// Auth (protected)
	mux.Handle("POST /api/v1/auth/logout", authMw(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /api/v1/auth/me", authMw(http.HandlerFunc(h.GetMe)))

	// ======================== Posts (text / image) ========================

	mux.Handle("POST /api/v1/posts", authMw(http.HandlerFunc(h.CreatePost)))
	mux.Handle("GET /api/v1/posts", authMw(http.HandlerFunc(h.ListPosts)))
	mux.Handle("GET /api/v1/posts/{id}", authMw(http.HandlerFunc(h.GetPost)))
	mux.Handle("PUT /api/v1/posts/{id}", authMw(http.HandlerFunc(h.UpdatePost)))
	mux.Handle("DELETE /api/v1/posts/{id}", authMw(http.HandlerFunc(h.DeletePost)))
	mux.Handle("POST /api/v1/posts/{id}/regenerate", authMw(http.HandlerFunc(h.RegeneratePost)))
	mux.Handle("POST /api/v1/posts/{id}/submit", authMw(http.HandlerFunc(h.SubmitPost)))

	// Scheduling
	mux.Handle("GET /api/v1/calendar", authMw(http.HandlerFunc(h.GetCalendar)))
	mux.Handle("PATCH /api/v1/posts/{id}/schedule", authMw(http.HandlerFunc(h.SchedulePost)))
	mux.Handle("POST /api/v1/posts/bulk-schedule", authMw(http.HandlerFunc(h.BulkSchedulePost)))
	mux.Handle("DELETE /api/v1/posts/{id}/schedule", authMw(http.HandlerFunc(h.UnschedulePost)))

	// Review and publish (admin)
	mux.Handle("GET /api/v1/admin/posts", authMw(adminMw(http.HandlerFunc(h.ListAdminPosts))))
	mux.Handle("GET /api/v1/admin/posts/review-queue", authMw(adminMw(http.HandlerFunc(h.GetAdminReviewQueue))))
	mux.Handle("POST /api/v1/admin/posts/{id}/approve", authMw(adminMw(http.HandlerFunc(h.ApprovePost))))
	mux.Handle("POST /api/v1/admin/posts/{id}/reject", authMw(adminMw(http.HandlerFunc(h.RejectPost))))
	mux.Handle("POST /api/v1/admin/posts/{id}/publish", authMw(adminMw(http.HandlerFunc(h.AdminPublishPost))))
	mux.Handle("GET /api/v1/admin/posts/{id}/publish-status", authMw(adminMw(http.HandlerFunc(h.GetPostPublishStatus))))

	// Machine clients (API key auth)
	apiKeyMw := middleware.RequireAPIKey(svc.APIKey)
	mux.Handle("POST /api/v1/external/posts", apiKeyMw(http.HandlerFunc(h.ExternalCreatePost)))
	mux.Handle("GET /api/v1/external/posts/{id}", apiKeyMw(http.HandlerFunc(h.ExternalGetPost)))

	// ======================== Videos ========================

	// Projects group uploaded videos.
	mux.Handle("POST /api/v1/admin/projects", authMw(adminMw(http.HandlerFunc(h.CreateProject))))
	mux.Handle("GET /api/v1/admin/projects", authMw(adminMw(http.HandlerFunc(h.ListAllProjects))))
	mux.Handle("DELETE /api/v1/admin/projects/{id}", authMw(adminMw(http.HandlerFunc(h.DeleteProject))))
	mux.Handle("GET /api/v1/projects/{id}", authMw(http.HandlerFunc(h.GetProject)))

	// Multipart upload straight to S3 via presigned URLs
	mux.Handle("POST /api/v1/uploads/initiate", authMw(http.HandlerFunc(h.InitiateUpload)))
	mux.Handle("POST /api/v1/uploads/complete", authMw(http.HandlerFunc(h.CompleteUpload)))
	mux.Handle("POST /api/v1/uploads/abort", authMw(http.HandlerFunc(h.AbortUpload)))
	mux.Handle("POST /api/v1/uploads/refresh-urls", authMw(http.HandlerFunc(h.RefreshUploadURLs)))

	mux.Handle("GET /api/v1/projects/{id}/videos", authMw(http.HandlerFunc(h.ListVideos)))
	mux.Handle("GET /api/v1/videos/{videoId}/playback", authMw(http.HandlerFunc(h.GetVideoPlaybackURL)))
	mux.Handle("DELETE /api/v1/videos/{videoId}", authMw(http.HandlerFunc(h.DeleteVideo)))

	// Publish a video to a connected channel (queued, processed by the worker)
	mux.Handle("POST /api/v1/videos/{id}/publish-to-platform", authMw(adminMw(http.HandlerFunc(h.PublishToPlatform))))
	mux.Handle("GET /api/v1/projects/{id}/publish-records", authMw(http.HandlerFunc(h.ListPublishRecords)))
	mux.Handle("GET /api/v1/publish-records/{id}/upload-status", authMw(http.HandlerFunc(h.GetUploadStatus)))
	mux.Handle("GET /api/v1/publish-records/{id}/upload-status/stream", authMw(http.HandlerFunc(h.StreamUploadStatus)))
	mux.Handle("GET /api/v1/admin/quota", authMw(adminMw(http.HandlerFunc(h.GetQuotaStatus))))
	mux.Handle("GET /api/v1/admin/publish-history", authMw(adminMw(http.HandlerFunc(h.ListPublishHistory))))
	mux.Handle("POST /api/v1/admin/publish-records/{id}/retry-thumbnail", authMw(adminMw(http.HandlerFunc(h.RetryThumbnail))))

	// ======================== Channels ========================

	mux.Handle("GET /api/v1/admin/channels", authMw(adminMw(http.HandlerFunc(h.ListChannels))))
	mux.Handle("POST /api/v1/admin/channels", authMw(adminMw(http.HandlerFunc(h.CreateChannel))))
	mux.Handle("PUT /api/v1/admin/channels/{id}", authMw(adminMw(http.HandlerFunc(h.UpdateChannel))))
	mux.Handle("DELETE /api/v1/admin/channels/{id}", authMw(adminMw(http.HandlerFunc(h.DeleteChannel))))

	// OAuth connect: the admin requests an authorization URL, the provider
	// redirects back to a public callback that is bound to the request by a
	// single-use state value.
	mux.Handle("GET /api/v1/admin/channels/{id}/oauth/{provider}/url", authMw(adminMw(http.HandlerFunc(h.GetChannelOAuthURL))))
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/google/callback", h.GoogleChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/facebook/callback", h.FacebookChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/instagram/callback", h.InstagramChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/threads/callback", h.ThreadsChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/twitter/callback", h.TwitterChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/linkedin/callback", h.LinkedInChannelCallback)

	mux.Handle("POST /api/v1/admin/channels/{id}/disconnect", authMw(adminMw(http.HandlerFunc(h.DisconnectChannel))))
	mux.Handle("GET /api/v1/admin/channels/{id}/status", authMw(adminMw(http.HandlerFunc(h.GetChannelStatus))))
	mux.Handle("POST /api/v1/admin/channels/{id}/refresh-token", authMw(adminMw(http.HandlerFunc(h.RefreshChannelToken))))
	mux.Handle("POST /api/v1/admin/channels/{id}/test-connection", authMw(adminMw(http.HandlerFunc(h.TestChannelConnection))))

	// Apply global middleware chain
	var chain http.Handler = mux
	chain = rl.Middleware(chain)
	chain = middleware.RequestID(chain)
	chain = middleware.CORS(cfg.App.FrontendURL)(chain)
	chain = middleware.Logging(logger)(chain)
	chain = middleware.Recovery(chain)

	return chain
}
