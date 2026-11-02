package router

import (
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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
	mux.HandleFunc("GET /api/v1/auth/{provider}/url", h.GetOAuthURL)
	mux.HandleFunc("POST /api/v1/auth/{provider}/callback", h.OAuthCallback)
	mux.HandleFunc("POST /api/v1/auth/refresh", h.RefreshToken)

	// Auth - email/password (public)
	mux.HandleFunc("POST /api/v1/auth/signup", h.Signup)
	mux.HandleFunc("POST /api/v1/auth/login", h.Login)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", h.ForgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", h.ResetPassword)

	// Categories (public)
	mux.HandleFunc("GET /api/v1/categories", h.ListCategories)

	// Protected routes - wrap with auth middleware
	authMw := middleware.RequireAuth(cfg.JWT.Secret)
	adminMw := middleware.RequireRole("admin", "super_admin")
	editorMw := middleware.RequireRole("editor", "admin", "super_admin")

	// Auth (protected)
	mux.Handle("POST /api/v1/auth/logout", authMw(http.HandlerFunc(h.Logout)))
	mux.Handle("GET /api/v1/auth/me", authMw(http.HandlerFunc(h.GetMe)))

	// User (protected)
	mux.Handle("POST /api/v1/users/me/onboard", authMw(http.HandlerFunc(h.Onboard)))
	mux.Handle("PATCH /api/v1/users/me/onboard/draft", authMw(http.HandlerFunc(h.SaveOnboardDraft)))
	mux.Handle("GET /api/v1/users/me/profile", authMw(http.HandlerFunc(h.GetProfile)))
	mux.Handle("PUT /api/v1/users/me/profile", authMw(http.HandlerFunc(h.UpdateProfile)))
	mux.Handle("GET /api/v1/users/me/banking", authMw(http.HandlerFunc(h.GetBankingInfo)))
	mux.Handle("POST /api/v1/users/me/banking", authMw(http.HandlerFunc(h.SubmitBanking)))
	mux.Handle("GET /api/v1/users/me/avatar-upload-url", authMw(http.HandlerFunc(h.GetAvatarUploadURL)))
	mux.Handle("POST /api/v1/users/me/avatar", authMw(http.HandlerFunc(h.ConfirmAvatarUpload)))
	mux.Handle("GET /api/v1/users/me/onboarding-status", authMw(http.HandlerFunc(h.GetOnboardingStatus)))
	mux.Handle("GET /api/v1/creators/count", authMw(http.HandlerFunc(h.GetCreatorCountByCity)))

	// Assignments - Content Brief Feed (protected)
	mux.Handle("GET /api/v1/assignments/open", authMw(http.HandlerFunc(h.ListOpenAssignments)))
	mux.Handle("POST /api/v1/projects/{id}/claim", authMw(http.HandlerFunc(h.ClaimAssignment)))

	// Projects - Creator (protected)
	mux.Handle("GET /api/v1/projects/mine", authMw(http.HandlerFunc(h.ListMyProjects)))
	mux.Handle("GET /api/v1/projects/{id}", authMw(http.HandlerFunc(h.GetProject)))
	mux.Handle("POST /api/v1/projects/self", authMw(http.HandlerFunc(h.CreateSelfProject)))
	mux.Handle("PUT /api/v1/projects/{id}", authMw(http.HandlerFunc(h.UpdateProject)))

	// Project posts (protected)
	mux.Handle("GET /api/v1/projects/{id}/posts", authMw(http.HandlerFunc(h.ListProjectPosts)))

	// Editing flow (protected)
	mux.Handle("POST /api/v1/projects/{id}/request-editing", authMw(http.HandlerFunc(h.RequestEditing)))
	mux.Handle("POST /api/v1/projects/{id}/editing-done", authMw(http.HandlerFunc(h.MarkEditingDone)))

	// Creator channels (protected)
	mux.Handle("GET /api/v1/channels/mine", authMw(http.HandlerFunc(h.ListMyChannels)))
	mux.Handle("GET /api/v1/projects/{id}/available-channels", authMw(http.HandlerFunc(h.ListAvailableChannels)))
	mux.Handle("POST /api/v1/me/channels/connect", authMw(http.HandlerFunc(h.ConnectMyChannel)))
	mux.Handle("POST /api/v1/me/channels/{id}/disconnect", authMw(http.HandlerFunc(h.DisconnectMyChannel)))
	mux.Handle("GET /api/v1/me/channels/{id}/status", authMw(http.HandlerFunc(h.GetMyChannelStatus)))
	mux.Handle("DELETE /api/v1/me/channels/{id}", authMw(http.HandlerFunc(h.DeleteMyChannel)))

	// Creator aligned editors (protected)
	mux.Handle("GET /api/v1/editors/mine", authMw(http.HandlerFunc(h.ListMyAlignedEditors)))

	// Projects - Admin (protected + role)
	mux.Handle("POST /api/v1/admin/projects", authMw(adminMw(http.HandlerFunc(h.CreateProject))))
	mux.Handle("GET /api/v1/admin/projects", authMw(adminMw(http.HandlerFunc(h.ListAllProjects))))
	mux.Handle("POST /api/v1/admin/projects/{id}/archive", authMw(adminMw(http.HandlerFunc(h.ArchiveProject))))
	mux.Handle("GET /api/v1/admin/projects/{id}/archive", authMw(adminMw(http.HandlerFunc(h.GetArchiveStatus))))
	mux.Handle("DELETE /api/v1/admin/projects/{id}", authMw(adminMw(http.HandlerFunc(h.DeleteProject))))

	// Admin review queue (protected + admin)
	mux.Handle("GET /api/v1/admin/review-queue", authMw(adminMw(http.HandlerFunc(h.ListReviewQueue))))

	// Uploads (protected)
	mux.Handle("POST /api/v1/uploads/initiate", authMw(http.HandlerFunc(h.InitiateUpload)))
	mux.Handle("POST /api/v1/uploads/complete", authMw(http.HandlerFunc(h.CompleteUpload)))
	mux.Handle("POST /api/v1/uploads/abort", authMw(http.HandlerFunc(h.AbortUpload)))
	mux.Handle("POST /api/v1/uploads/refresh-urls", authMw(http.HandlerFunc(h.RefreshUploadURLs)))

	// Videos (protected)
	mux.Handle("GET /api/v1/projects/{id}/videos", authMw(http.HandlerFunc(h.ListVideos)))
	mux.Handle("GET /api/v1/videos/{videoId}/playback", authMw(http.HandlerFunc(h.GetVideoPlaybackURL)))
	mux.Handle("DELETE /api/v1/videos/{videoId}", authMw(http.HandlerFunc(h.DeleteVideo)))

	// Comments (protected)
	mux.Handle("POST /api/v1/comments", authMw(http.HandlerFunc(h.CreateComment)))
	mux.Handle("GET /api/v1/projects/{id}/comments", authMw(http.HandlerFunc(h.ListComments)))
	mux.Handle("GET /api/v1/videos/{videoId}/comments", authMw(http.HandlerFunc(h.ListVideoComments)))

	// Transitions (protected)
	mux.Handle("POST /api/v1/projects/{id}/transition", authMw(http.HandlerFunc(h.TransitionProject)))
	mux.Handle("GET /api/v1/projects/{id}/transitions", authMw(http.HandlerFunc(h.GetAllowedTransitions)))

	// Channel Targets (protected)
	mux.Handle("POST /api/v1/projects/{id}/channel-targets", authMw(http.HandlerFunc(h.CreateChannelTargets)))
	mux.Handle("GET /api/v1/projects/{id}/channel-targets", authMw(http.HandlerFunc(h.ListChannelTargets)))
	mux.Handle("DELETE /api/v1/channel-targets/{id}", authMw(http.HandlerFunc(h.DeleteChannelTarget)))

	// Creator-accessible: list available channels for a project
	mux.Handle("GET /api/v1/projects/{id}/channels", authMw(http.HandlerFunc(h.ListProjectChannels)))

	// Clip Jobs (protected)
	mux.Handle("POST /api/v1/projects/{id}/clip-jobs", authMw(http.HandlerFunc(h.CreateClipJob)))
	mux.Handle("GET /api/v1/projects/{id}/clip-jobs", authMw(http.HandlerFunc(h.ListClipJobs)))
	mux.Handle("GET /api/v1/clip-jobs/{id}/clips", authMw(http.HandlerFunc(h.ListClipsByJob)))

	// Raw Assets (protected)
	mux.Handle("GET /api/v1/projects/{id}/raw-assets", authMw(http.HandlerFunc(h.ListRawAssets)))
	mux.Handle("PATCH /api/v1/raw-assets/{id}/status", authMw(adminMw(http.HandlerFunc(h.UpdateRawAssetStatus))))

	// Dashboard (protected)
	mux.Handle("GET /api/v1/dashboard/stats", authMw(http.HandlerFunc(h.GetCreatorDashboardStats)))
	mux.Handle("GET /api/v1/projects/performance", authMw(http.HandlerFunc(h.ListProjectPerformance)))
	mux.Handle("GET /api/v1/projects/{id}/video-performance", authMw(http.HandlerFunc(h.GetVideoPerformance)))
	mux.Handle("GET /api/v1/videos/{videoId}/platform-stats", authMw(http.HandlerFunc(h.GetPlatformStats)))
	mux.Handle("GET /api/v1/dashboard/earnings-trend", authMw(http.HandlerFunc(h.GetEarningsTrend)))
	mux.Handle("GET /api/v1/dashboard/top-videos", authMw(http.HandlerFunc(h.GetTopVideos)))
	mux.Handle("GET /api/v1/dashboard/platform-breakdown", authMw(http.HandlerFunc(h.GetPlatformBreakdown)))
	mux.Handle("GET /api/v1/dashboard/activity-feed", authMw(http.HandlerFunc(h.GetActivityFeed)))
	mux.Handle("GET /api/v1/payouts/current-month", authMw(http.HandlerFunc(h.GetCurrentMonthPayout)))

	// Payouts - Creator (protected)
	mux.Handle("GET /api/v1/payouts/mine", authMw(http.HandlerFunc(h.GetMyPayouts)))

	// Editor (protected + role)
	mux.Handle("GET /api/v1/editor/projects", authMw(editorMw(http.HandlerFunc(h.ListEditorProjects))))

	// Admin - users management
	mux.Handle("GET /api/v1/admin/users", authMw(adminMw(http.HandlerFunc(h.ListAllUsers))))
	mux.Handle("GET /api/v1/admin/users/editors", authMw(adminMw(http.HandlerFunc(h.ListEditors))))
	mux.Handle("PUT /api/v1/admin/users/{id}/role", authMw(adminMw(http.HandlerFunc(h.ChangeUserRole))))

	// Admin Dashboard, Analytics & Payouts (protected + admin)
	mux.Handle("GET /api/v1/admin/dashboard/stats", authMw(adminMw(http.HandlerFunc(h.GetAdminDashboardStats))))
	mux.Handle("GET /api/v1/admin/projects/performance", authMw(adminMw(http.HandlerFunc(h.ListAllProjectPerformance))))
	mux.Handle("GET /api/v1/admin/analytics", authMw(adminMw(http.HandlerFunc(h.GetAdminAnalytics))))
	mux.Handle("GET /api/v1/admin/payouts", authMw(adminMw(http.HandlerFunc(h.ListAllPayouts))))

	// Revenue sharing (admin)
	mux.Handle("GET /api/v1/admin/revenue-config", authMw(adminMw(http.HandlerFunc(h.GetRevenueConfig))))
	mux.Handle("PUT /api/v1/admin/revenue-config", authMw(adminMw(http.HandlerFunc(h.UpdateRevenueConfig))))
	mux.Handle("POST /api/v1/admin/payouts/generate", authMw(adminMw(http.HandlerFunc(h.GenerateMonthlyPayouts))))
	mux.Handle("POST /api/v1/admin/payouts/{id}/approve", authMw(adminMw(http.HandlerFunc(h.ApprovePayout))))
	mux.Handle("POST /api/v1/admin/payouts/{id}/reject", authMw(adminMw(http.HandlerFunc(h.RejectPayout))))

	// Admin - Editor Alignments (protected + admin)
	mux.Handle("POST /api/v1/admin/editor-alignments", authMw(adminMw(http.HandlerFunc(h.CreateEditorAlignment))))
	mux.Handle("GET /api/v1/admin/editor-alignments/creator/{creatorId}", authMw(adminMw(http.HandlerFunc(h.ListEditorAlignmentsByCreator))))
	mux.Handle("GET /api/v1/admin/editor-alignments/editor/{editorId}", authMw(adminMw(http.HandlerFunc(h.ListEditorAlignmentsByEditor))))
	mux.Handle("DELETE /api/v1/admin/editor-alignments/{id}", authMw(adminMw(http.HandlerFunc(h.DeleteEditorAlignment))))

	// Channels - Admin (protected + admin)
	mux.Handle("GET /api/v1/admin/channels", authMw(adminMw(http.HandlerFunc(h.ListChannels))))
	mux.Handle("POST /api/v1/admin/channels", authMw(adminMw(http.HandlerFunc(h.CreateChannel))))
	mux.Handle("PUT /api/v1/admin/channels/{id}", authMw(adminMw(http.HandlerFunc(h.UpdateChannel))))
	mux.Handle("DELETE /api/v1/admin/channels/{id}", authMw(adminMw(http.HandlerFunc(h.DeleteChannel))))

	// Publish Records
	mux.Handle("POST /api/v1/projects/{id}/publish-records", authMw(adminMw(http.HandlerFunc(h.CreatePublishRecord))))
	mux.Handle("GET /api/v1/projects/{id}/publish-records", authMw(http.HandlerFunc(h.ListPublishRecords)))
	mux.Handle("PATCH /api/v1/publish-records/{id}", authMw(adminMw(http.HandlerFunc(h.UpdatePublishRecord))))
	mux.Handle("DELETE /api/v1/publish-records/{id}", authMw(adminMw(http.HandlerFunc(h.DeletePublishRecord))))
	mux.Handle("GET /api/v1/projects/{id}/suggested-channels", authMw(http.HandlerFunc(h.GetSuggestedChannels)))

	// Notifications (protected)
	mux.Handle("GET /api/v1/notifications", authMw(http.HandlerFunc(h.ListNotifications)))
	mux.Handle("GET /api/v1/notifications/unread-count", authMw(http.HandlerFunc(h.GetUnreadCount)))
	mux.Handle("POST /api/v1/notifications/read-all", authMw(http.HandlerFunc(h.MarkAllNotificationsRead)))
	mux.Handle("POST /api/v1/notifications/{id}/read", authMw(http.HandlerFunc(h.MarkNotificationRead)))
	mux.Handle("DELETE /api/v1/notifications/{id}", authMw(http.HandlerFunc(h.DeleteNotification)))

	// Creator analytics (protected)
	mux.Handle("GET /api/v1/analytics/mine", authMw(http.HandlerFunc(h.GetCreatorAnalytics)))

	// ======================== V2: Platform Publishing ========================

	// Channel OAuth callbacks (public — redirect targets from OAuth providers)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/google/callback", h.GoogleChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/facebook/callback", h.FacebookChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/instagram/callback", h.InstagramChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/threads/callback", h.ThreadsChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/twitter/callback", h.TwitterChannelCallback)
	mux.HandleFunc("GET /api/v1/admin/channels/oauth/linkedin/callback", h.LinkedInChannelCallback)

	// Channel OAuth management (protected + admin)
	mux.Handle("GET /api/v1/admin/channels/{id}/oauth/{provider}/url", authMw(adminMw(http.HandlerFunc(h.GetChannelOAuthURL))))
	mux.Handle("POST /api/v1/admin/channels/{id}/disconnect", authMw(adminMw(http.HandlerFunc(h.DisconnectChannel))))
	mux.Handle("GET /api/v1/admin/channels/{id}/status", authMw(adminMw(http.HandlerFunc(h.GetChannelStatus))))
	mux.Handle("POST /api/v1/admin/channels/{id}/refresh-token", authMw(adminMw(http.HandlerFunc(h.RefreshChannelToken))))
	mux.Handle("POST /api/v1/admin/channels/{id}/test-connection", authMw(adminMw(http.HandlerFunc(h.TestChannelConnection))))

	// Platform publishing (protected + admin)
	mux.Handle("POST /api/v1/videos/{id}/publish-to-platform", authMw(adminMw(http.HandlerFunc(h.PublishToPlatform))))
	mux.Handle("GET /api/v1/publish-records/{id}/upload-status", authMw(http.HandlerFunc(h.GetUploadStatus)))
	mux.Handle("GET /api/v1/publish-records/{id}/upload-status/stream", authMw(http.HandlerFunc(h.StreamUploadStatus)))

	// Quota & publish history (protected + admin)
	mux.Handle("GET /api/v1/admin/quota", authMw(adminMw(http.HandlerFunc(h.GetQuotaStatus))))
	mux.Handle("GET /api/v1/admin/publish-history", authMw(adminMw(http.HandlerFunc(h.ListPublishHistory))))
	mux.Handle("POST /api/v1/admin/publish-records/{id}/retry-thumbnail", authMw(adminMw(http.HandlerFunc(h.RetryThumbnail))))

	// ======================== Posts ========================

	// User-facing post routes (protected)
	mux.Handle("POST /api/v1/posts", authMw(http.HandlerFunc(h.CreatePost)))
	mux.Handle("GET /api/v1/posts", authMw(http.HandlerFunc(h.ListPosts)))
	mux.Handle("GET /api/v1/posts/{id}", authMw(http.HandlerFunc(h.GetPost)))
	mux.Handle("PUT /api/v1/posts/{id}", authMw(http.HandlerFunc(h.UpdatePost)))
	mux.Handle("DELETE /api/v1/posts/{id}", authMw(http.HandlerFunc(h.DeletePost)))
	mux.Handle("POST /api/v1/posts/{id}/regenerate", authMw(http.HandlerFunc(h.RegeneratePost)))
	mux.Handle("POST /api/v1/posts/{id}/submit", authMw(http.HandlerFunc(h.SubmitPost)))

	// Admin post management (protected + admin)
	mux.Handle("GET /api/v1/admin/posts", authMw(adminMw(http.HandlerFunc(h.ListAdminPosts))))
	mux.Handle("GET /api/v1/admin/posts/review-queue", authMw(adminMw(http.HandlerFunc(h.GetAdminReviewQueue))))
	mux.Handle("POST /api/v1/admin/posts/{id}/approve", authMw(adminMw(http.HandlerFunc(h.ApprovePost))))
	mux.Handle("POST /api/v1/admin/posts/{id}/reject", authMw(adminMw(http.HandlerFunc(h.RejectPost))))
	mux.Handle("POST /api/v1/admin/posts/{id}/publish", authMw(adminMw(http.HandlerFunc(h.AdminPublishPost))))
	mux.Handle("GET /api/v1/admin/posts/{id}/publish-status", authMw(adminMw(http.HandlerFunc(h.GetPostPublishStatus))))

	// ======================== AI Jobs & Credits ========================

	// AI Jobs (protected)
	mux.Handle("POST /api/v1/projects/{id}/ai-jobs", authMw(http.HandlerFunc(h.CreateAIJob)))
	mux.Handle("GET /api/v1/projects/{id}/ai-jobs", authMw(http.HandlerFunc(h.ListAIJobs)))
	mux.Handle("GET /api/v1/ai-jobs/{id}", authMw(http.HandlerFunc(h.GetAIJob)))
	mux.Handle("POST /api/v1/ai-jobs/{id}/cancel", authMw(http.HandlerFunc(h.CancelAIJob)))

	// AI Credits (protected)
	mux.Handle("GET /api/v1/ai-credits", authMw(http.HandlerFunc(h.GetCredits)))
	mux.Handle("GET /api/v1/ai-credits/transactions", authMw(http.HandlerFunc(h.ListCreditTransactions)))

	// ======================== V7: Editor Marketplace ========================

	// Marketplace - Creator (protected)
	mux.Handle("POST /api/v1/projects/{id}/editing-job", authMw(http.HandlerFunc(h.CreateEditingJob)))
	mux.Handle("GET /api/v1/projects/{id}/editing-job", authMw(http.HandlerFunc(h.GetEditingJob)))
	mux.Handle("PUT /api/v1/editing-jobs/{id}", authMw(http.HandlerFunc(h.UpdateEditingJob)))
	mux.Handle("DELETE /api/v1/editing-jobs/{id}", authMw(http.HandlerFunc(h.CancelEditingJob)))
	mux.Handle("GET /api/v1/editing-jobs/{id}/applications", authMw(http.HandlerFunc(h.ListJobApplications)))
	mux.Handle("POST /api/v1/job-applications/{id}/accept", authMw(http.HandlerFunc(h.AcceptApplication)))
	mux.Handle("POST /api/v1/job-applications/{id}/reject", authMw(http.HandlerFunc(h.RejectApplication)))
	mux.Handle("POST /api/v1/editing-assignments/{id}/accept-delivery", authMw(http.HandlerFunc(h.AcceptDelivery)))
	mux.Handle("POST /api/v1/editing-assignments/{id}/request-changes", authMw(http.HandlerFunc(h.RequestChanges)))
	mux.Handle("POST /api/v1/editing-assignments/{id}/rate", authMw(http.HandlerFunc(h.RateUser)))

	// Marketplace - Editor (protected)
	mux.Handle("GET /api/v1/editing-jobs", authMw(http.HandlerFunc(h.ListOpenJobs)))
	mux.Handle("POST /api/v1/editing-jobs/{id}/apply", authMw(http.HandlerFunc(h.ApplyForJob)))
	mux.Handle("DELETE /api/v1/job-applications/{id}", authMw(http.HandlerFunc(h.WithdrawApplication)))
	mux.Handle("GET /api/v1/me/editing-assignments", authMw(http.HandlerFunc(h.ListMyAssignments)))
	mux.Handle("POST /api/v1/editing-assignments/{id}/deliver", authMw(http.HandlerFunc(h.DeliverWork)))
	mux.Handle("GET /api/v1/me/editor-stats", authMw(http.HandlerFunc(h.GetEditorStats)))

	// Marketplace - Admin (protected + admin)
	mux.Handle("GET /api/v1/admin/editing-price-config", authMw(adminMw(http.HandlerFunc(h.GetPriceConfig))))

	// ======================== V8: Content Calendar & Scheduling ========================
	mux.Handle("GET /api/v1/calendar", authMw(http.HandlerFunc(h.GetCalendar)))
	mux.Handle("PATCH /api/v1/posts/{id}/schedule", authMw(http.HandlerFunc(h.SchedulePost)))
	mux.Handle("POST /api/v1/posts/bulk-schedule", authMw(http.HandlerFunc(h.BulkSchedulePost)))
	mux.Handle("DELETE /api/v1/posts/{id}/schedule", authMw(http.HandlerFunc(h.UnschedulePost)))

	// ======================== Cross-Platform Analytics ========================
	mux.Handle("GET /api/v1/analytics/overview", authMw(http.HandlerFunc(h.GetAnalyticsOverview)))
	mux.Handle("GET /api/v1/analytics/platform/{platform}", authMw(http.HandlerFunc(h.GetPlatformAnalytics)))
	mux.Handle("GET /api/v1/analytics/post/{id}", authMw(http.HandlerFunc(h.GetPostAnalyticsHandler)))
	mux.Handle("GET /api/v1/analytics/comparison", authMw(http.HandlerFunc(h.GetAnalyticsComparison)))

	// ======================== AI Content Repurposing ========================
	mux.Handle("POST /api/v1/posts/{id}/repurpose", authMw(http.HandlerFunc(h.CreateRepurposeJob)))
	mux.Handle("GET /api/v1/repurpose-jobs", authMw(http.HandlerFunc(h.ListRepurposeJobs)))
	mux.Handle("GET /api/v1/repurpose-jobs/{id}", authMw(http.HandlerFunc(h.GetRepurposeJob)))
	mux.Handle("GET /api/v1/repurpose-jobs/{id}/outputs", authMw(http.HandlerFunc(h.GetRepurposeOutputs)))
	mux.Handle("POST /api/v1/repurpose-outputs/{id}/accept", authMw(http.HandlerFunc(h.AcceptRepurposeOutput)))

	// ======================== Brand Kit & Templates ========================
	mux.Handle("POST /api/v1/brand-kits", authMw(http.HandlerFunc(h.CreateBrandKit)))
	mux.Handle("GET /api/v1/brand-kits", authMw(http.HandlerFunc(h.ListBrandKits)))
	mux.Handle("GET /api/v1/brand-kits/{id}", authMw(http.HandlerFunc(h.GetBrandKit)))
	mux.Handle("PUT /api/v1/brand-kits/{id}", authMw(http.HandlerFunc(h.UpdateBrandKit)))
	mux.Handle("DELETE /api/v1/brand-kits/{id}", authMw(http.HandlerFunc(h.DeleteBrandKit)))
	mux.Handle("POST /api/v1/brand-kits/{id}/default", authMw(http.HandlerFunc(h.SetDefaultBrandKit)))
	mux.Handle("POST /api/v1/brand-kits/{id}/templates", authMw(http.HandlerFunc(h.CreateTemplate)))
	mux.Handle("GET /api/v1/brand-kits/{id}/templates", authMw(http.HandlerFunc(h.ListTemplates)))
	mux.Handle("GET /api/v1/templates/{id}", authMw(http.HandlerFunc(h.GetTemplate)))
	mux.Handle("POST /api/v1/templates/{id}/render", authMw(http.HandlerFunc(h.RenderTemplate)))
	mux.Handle("DELETE /api/v1/templates/{id}", authMw(http.HandlerFunc(h.DeleteTemplate)))

	// ======================== Virality Scoring ========================
	mux.Handle("GET /api/v1/posts/{id}/virality-score", authMw(http.HandlerFunc(h.GetViralityScore)))
	mux.Handle("GET /api/v1/posts/{id}/suggestions", authMw(http.HandlerFunc(h.GetViralitySuggestions)))
	mux.Handle("GET /api/v1/trending-hashtags", authMw(http.HandlerFunc(h.GetTrendingHashtags)))

	// ======================== A/B Testing ========================
	mux.Handle("POST /api/v1/ab-tests", authMw(http.HandlerFunc(h.CreateABTest)))
	mux.Handle("GET /api/v1/ab-tests", authMw(http.HandlerFunc(h.ListABTests)))
	mux.Handle("GET /api/v1/ab-tests/{id}", authMw(http.HandlerFunc(h.GetABTest)))
	mux.Handle("POST /api/v1/ab-tests/{id}/start", authMw(http.HandlerFunc(h.StartABTest)))
	mux.Handle("GET /api/v1/ab-tests/{id}/evaluate", authMw(http.HandlerFunc(h.EvaluateABTest)))
	mux.Handle("POST /api/v1/ab-tests/{id}/winner", authMw(http.HandlerFunc(h.SelectABTestWinner)))
	mux.Handle("POST /api/v1/ab-tests/{id}/cancel", authMw(http.HandlerFunc(h.CancelABTest)))

	// ======================== Collaboration Workflows ========================
	mux.Handle("POST /api/v1/team/members", authMw(http.HandlerFunc(h.InviteMember)))
	mux.Handle("POST /api/v1/team/invites/{id}/accept", authMw(http.HandlerFunc(h.AcceptTeamInvite)))
	mux.Handle("DELETE /api/v1/team/members/{id}", authMw(http.HandlerFunc(h.RemoveTeamMember)))
	mux.Handle("GET /api/v1/team/members", authMw(http.HandlerFunc(h.ListTeamMembers)))
	mux.Handle("PATCH /api/v1/team/members/{id}/role", authMw(http.HandlerFunc(h.UpdateTeamMemberRole)))
	mux.Handle("POST /api/v1/workflows", authMw(http.HandlerFunc(h.CreateApprovalWorkflow)))
	mux.Handle("GET /api/v1/workflows/{id}", authMw(http.HandlerFunc(h.GetApprovalWorkflow)))
	mux.Handle("GET /api/v1/workflows", authMw(http.HandlerFunc(h.ListApprovalWorkflows)))
	mux.Handle("DELETE /api/v1/workflows/{id}", authMw(http.HandlerFunc(h.DeleteApprovalWorkflow)))
	mux.Handle("POST /api/v1/posts/{id}/submit-approval", authMw(http.HandlerFunc(h.SubmitForApproval)))
	mux.Handle("POST /api/v1/approvals/{id}/action", authMw(http.HandlerFunc(h.TakeApprovalAction)))
	mux.Handle("GET /api/v1/posts/{id}/approval-status", authMw(http.HandlerFunc(h.GetApprovalStatus)))
	mux.Handle("GET /api/v1/approvals/pending", authMw(http.HandlerFunc(h.ListPendingApprovals)))

	// ======================== Marketplace Enhancements ========================
	mux.Handle("POST /api/v1/me/portfolio", authMw(http.HandlerFunc(h.CreateEditorPortfolio)))
	mux.Handle("GET /api/v1/editors/{id}/portfolio", authMw(http.HandlerFunc(h.GetEditorPortfolio)))
	mux.Handle("PUT /api/v1/me/portfolio", authMw(http.HandlerFunc(h.UpdateEditorPortfolio)))
	mux.Handle("POST /api/v1/me/portfolio/items", authMw(http.HandlerFunc(h.AddPortfolioItem)))
	mux.Handle("DELETE /api/v1/me/portfolio/items/{id}", authMw(http.HandlerFunc(h.RemovePortfolioItem)))
	mux.Handle("PUT /api/v1/me/skills", authMw(http.HandlerFunc(h.SetEditorSkills)))
	mux.Handle("GET /api/v1/editors/search", authMw(http.HandlerFunc(h.SearchEditors)))
	mux.Handle("GET /api/v1/editing-jobs/{id}/match-editors", authMw(http.HandlerFunc(h.MatchEditorsForJob)))
	mux.Handle("POST /api/v1/marketplace/templates", authMw(http.HandlerFunc(h.CreateMarketplaceTemplate)))
	mux.Handle("GET /api/v1/marketplace/templates", authMw(http.HandlerFunc(h.ListMarketplaceTemplates)))
	mux.Handle("POST /api/v1/marketplace/templates/{id}/purchase", authMw(http.HandlerFunc(h.PurchaseMarketplaceTemplate)))

	// ======================== Revenue Attribution ========================
	mux.Handle("POST /api/v1/revenue/sources", authMw(http.HandlerFunc(h.AddRevenueSource)))
	mux.Handle("GET /api/v1/revenue/sources", authMw(http.HandlerFunc(h.ListRevenueSources)))
	mux.Handle("PUT /api/v1/revenue/sources/{id}", authMw(http.HandlerFunc(h.UpdateRevenueSource)))
	mux.Handle("DELETE /api/v1/revenue/sources/{id}", authMw(http.HandlerFunc(h.DeleteRevenueSource)))
	mux.Handle("GET /api/v1/revenue/breakdown", authMw(http.HandlerFunc(h.GetRevenueBreakdown)))
	mux.Handle("POST /api/v1/revenue/goals", authMw(http.HandlerFunc(h.CreateRevenueGoal)))
	mux.Handle("GET /api/v1/revenue/goals", authMw(http.HandlerFunc(h.ListRevenueGoals)))
	mux.Handle("GET /api/v1/revenue/goals/{id}/progress", authMw(http.HandlerFunc(h.GetGoalProgress)))
	mux.Handle("POST /api/v1/revenue/affiliate-links", authMw(http.HandlerFunc(h.CreateAffiliateLink)))
	mux.Handle("GET /api/v1/revenue/affiliate-links", authMw(http.HandlerFunc(h.ListAffiliateLinks)))
	mux.HandleFunc("GET /api/v1/r/{code}", h.TrackAffiliateClick)

	// ======================== Mobile API ========================
	mux.Handle("POST /api/v1/mobile/push-token", authMw(http.HandlerFunc(h.RegisterPushToken)))
	mux.Handle("DELETE /api/v1/mobile/push-token", authMw(http.HandlerFunc(h.UnregisterPushToken)))
	mux.Handle("GET /api/v1/mobile/sync", authMw(http.HandlerFunc(h.GetSyncEntries)))
	mux.Handle("GET /api/v1/mobile/notifications", authMw(http.HandlerFunc(h.GetMobileNotifications)))
	mux.Handle("POST /api/v1/mobile/quick-post", authMw(http.HandlerFunc(h.QuickCreatePost)))
	mux.Handle("GET /api/v1/mobile/dashboard", authMw(http.HandlerFunc(h.GetMobileDashboard)))

	// ======================== Agency / White-Label Mode ========================
	mux.Handle("POST /api/v1/organizations", authMw(http.HandlerFunc(h.CreateOrganization)))
	mux.Handle("GET /api/v1/organizations", authMw(http.HandlerFunc(h.ListMyOrganizations)))
	mux.Handle("GET /api/v1/organizations/{id}", authMw(http.HandlerFunc(h.GetOrganization)))
	mux.Handle("PUT /api/v1/organizations/{id}", authMw(http.HandlerFunc(h.UpdateOrganization)))
	mux.Handle("POST /api/v1/organizations/{id}/members", authMw(http.HandlerFunc(h.InviteOrgMember)))
	mux.Handle("GET /api/v1/organizations/{id}/members", authMw(http.HandlerFunc(h.ListOrgMembers)))
	mux.Handle("DELETE /api/v1/organizations/{id}/members/{memberId}", authMw(http.HandlerFunc(h.RemoveOrgMember)))
	mux.Handle("PATCH /api/v1/organizations/{id}/members/{memberId}/role", authMw(http.HandlerFunc(h.UpdateOrgMemberRole)))
	mux.Handle("POST /api/v1/organizations/invites/{id}/accept", authMw(http.HandlerFunc(h.AcceptOrgInvite)))
	mux.Handle("GET /api/v1/organizations/{id}/dashboard", authMw(http.HandlerFunc(h.GetOrgDashboard)))
	mux.Handle("GET /api/v1/organizations/{id}/projects", authMw(http.HandlerFunc(h.ListOrgProjects)))
	mux.Handle("POST /api/v1/organizations/{id}/switch", authMw(http.HandlerFunc(h.SwitchOrg)))

	// ======================== Platform Commerce ========================
	mux.Handle("POST /api/v1/commerce/integrations", authMw(http.HandlerFunc(h.ConnectCommerce)))
	mux.Handle("POST /api/v1/commerce/integrations/{id}/disconnect", authMw(http.HandlerFunc(h.DisconnectCommerce)))
	mux.Handle("GET /api/v1/commerce/integrations", authMw(http.HandlerFunc(h.ListCommerceIntegrations)))
	mux.Handle("GET /api/v1/commerce/stats", authMw(http.HandlerFunc(h.GetCommerceStats)))
	mux.Handle("GET /api/v1/commerce/transactions", authMw(http.HandlerFunc(h.ListCommerceTransactions)))
	mux.Handle("POST /api/v1/commerce/integrations/{id}/sync", authMw(http.HandlerFunc(h.SyncCommerceIntegration)))

	// ======================== AI Dubbing ========================
	mux.Handle("POST /api/v1/dub/jobs", authMw(http.HandlerFunc(h.CreateDubJob)))
	mux.Handle("POST /api/v1/dub/batch", authMw(http.HandlerFunc(h.BatchCreateDubJobs)))
	mux.Handle("GET /api/v1/dub/jobs/{id}", authMw(http.HandlerFunc(h.GetDubJob)))
	mux.Handle("GET /api/v1/dub/projects/{id}/jobs", authMw(http.HandlerFunc(h.ListDubJobsByProject)))
	mux.Handle("GET /api/v1/dub/jobs/{id}/outputs", authMw(http.HandlerFunc(h.ListDubOutputs)))
	mux.Handle("POST /api/v1/dub/jobs/{id}/cancel", authMw(http.HandlerFunc(h.CancelDubJob)))
	mux.Handle("GET /api/v1/dub/languages", authMw(http.HandlerFunc(h.GetDubLanguages)))
	mux.Handle("POST /api/v1/dub/estimate", authMw(http.HandlerFunc(h.EstimateDubCredits)))

	// ======================== Smart Clips (Better than OpusClip) ========================
	mux.Handle("POST /api/v1/smart-clips/jobs", authMw(http.HandlerFunc(h.CreateSmartClipJob)))
	mux.Handle("GET /api/v1/smart-clips/jobs/{id}", authMw(http.HandlerFunc(h.GetSmartClipJob)))
	mux.Handle("GET /api/v1/smart-clips/projects/{id}/jobs", authMw(http.HandlerFunc(h.ListSmartClipJobsByProject)))
	mux.Handle("GET /api/v1/smart-clips/jobs/{id}/clips", authMw(http.HandlerFunc(h.ListSmartClips)))
	mux.Handle("GET /api/v1/smart-clips/jobs/{id}/moments", authMw(http.HandlerFunc(h.ListClipMoments)))
	mux.Handle("GET /api/v1/smart-clips/jobs/{id}/top", authMw(http.HandlerFunc(h.GetTopSmartClips)))
	mux.Handle("POST /api/v1/smart-clips/jobs/{id}/cancel", authMw(http.HandlerFunc(h.CancelSmartClipJob)))
	mux.Handle("POST /api/v1/smart-clips/estimate", authMw(http.HandlerFunc(h.EstimateSmartClipCredits)))

	// ======================== YouTube SEO & Content Strategy ========================
	mux.Handle("POST /api/v1/seo/analyze", authMw(http.HandlerFunc(h.AnalyzeSEO)))
	mux.Handle("GET /api/v1/seo/analyses/{id}", authMw(http.HandlerFunc(h.GetSEOAnalysis)))
	mux.Handle("GET /api/v1/seo/projects/{id}/analyses", authMw(http.HandlerFunc(h.ListSEOAnalyses)))
	mux.Handle("POST /api/v1/seo/keywords", authMw(http.HandlerFunc(h.TrackSEOKeyword)))
	mux.Handle("GET /api/v1/seo/keywords", authMw(http.HandlerFunc(h.ListSEOKeywords)))
	mux.Handle("DELETE /api/v1/seo/keywords/{id}", authMw(http.HandlerFunc(h.DeleteSEOKeyword)))
	mux.Handle("POST /api/v1/seo/suggestions", authMw(http.HandlerFunc(h.CreateContentSuggestion)))
	mux.Handle("GET /api/v1/seo/suggestions", authMw(http.HandlerFunc(h.ListContentSuggestions)))
	mux.Handle("POST /api/v1/seo/suggestions/{id}/dismiss", authMw(http.HandlerFunc(h.DismissContentSuggestion)))
	mux.Handle("POST /api/v1/seo/competitors", authMw(http.HandlerFunc(h.AddCompetitor)))
	mux.Handle("GET /api/v1/seo/competitors", authMw(http.HandlerFunc(h.ListCompetitors)))
	mux.Handle("DELETE /api/v1/seo/competitors/{id}", authMw(http.HandlerFunc(h.RemoveCompetitor)))

	// ======================== Newsletter & Email Marketing ========================
	mux.Handle("POST /api/v1/newsletters", authMw(http.HandlerFunc(h.CreateNewsletter)))
	mux.Handle("GET /api/v1/newsletters", authMw(http.HandlerFunc(h.ListNewsletters)))
	mux.Handle("GET /api/v1/newsletters/{id}", authMw(http.HandlerFunc(h.GetNewsletter)))
	mux.Handle("PATCH /api/v1/newsletters/{id}", authMw(http.HandlerFunc(h.UpdateNewsletter)))
	mux.Handle("DELETE /api/v1/newsletters/{id}", authMw(http.HandlerFunc(h.DeleteNewsletter)))
	mux.Handle("POST /api/v1/newsletters/{id}/subscribers", authMw(http.HandlerFunc(h.AddNewsletterSubscriber)))
	mux.Handle("POST /api/v1/newsletters/{id}/subscribers/bulk", authMw(http.HandlerFunc(h.BulkAddSubscribers)))
	mux.Handle("GET /api/v1/newsletters/{id}/subscribers", authMw(http.HandlerFunc(h.ListNewsletterSubscribers)))
	mux.Handle("POST /api/v1/newsletters/{id}/unsubscribe", authMw(http.HandlerFunc(h.UnsubscribeNewsletter)))
	mux.Handle("DELETE /api/v1/newsletters/{id}/subscribers/{subId}", authMw(http.HandlerFunc(h.RemoveNewsletterSubscriber)))
	mux.Handle("POST /api/v1/newsletters/{id}/issues", authMw(http.HandlerFunc(h.CreateNewsletterIssue)))
	mux.Handle("GET /api/v1/newsletters/{id}/issues", authMw(http.HandlerFunc(h.ListNewsletterIssues)))
	mux.Handle("GET /api/v1/newsletter-issues/{id}", authMw(http.HandlerFunc(h.GetNewsletterIssue)))
	mux.Handle("POST /api/v1/newsletter-issues/{id}/send", authMw(http.HandlerFunc(h.SendNewsletterIssue)))
	mux.Handle("DELETE /api/v1/newsletters/{id}/issues/{issueId}", authMw(http.HandlerFunc(h.DeleteNewsletterIssue)))
	mux.Handle("GET /api/v1/newsletters/{id}/stats", authMw(http.HandlerFunc(h.GetNewsletterStats)))

	// ======================== Project Assets (Creator Kit) ========================

	// Asset CRUD (admin)
	mux.Handle("GET /api/v1/admin/projects/{id}/assets", authMw(adminMw(http.HandlerFunc(h.ListProjectAssets))))
	mux.Handle("GET /api/v1/admin/projects/{id}/assets/search", authMw(adminMw(http.HandlerFunc(h.SearchProjectAssets))))
	mux.Handle("POST /api/v1/admin/projects/{id}/assets", authMw(adminMw(http.HandlerFunc(h.CreateProjectAsset))))
	mux.Handle("DELETE /api/v1/admin/projects/{id}/assets/{assetId}", authMw(adminMw(http.HandlerFunc(h.DeleteProjectAsset))))
	mux.Handle("POST /api/v1/admin/projects/{id}/assets/bulk-delete", authMw(adminMw(http.HandlerFunc(h.BulkDeleteProjectAssets))))
	mux.Handle("POST /api/v1/admin/projects/{id}/assets/{assetId}/restore", authMw(adminMw(http.HandlerFunc(h.RestoreProjectAsset))))
	mux.Handle("PUT /api/v1/admin/projects/{id}/assets/{assetId}/rename", authMw(adminMw(http.HandlerFunc(h.RenameProjectAsset))))
	mux.Handle("PUT /api/v1/admin/projects/{id}/assets/{assetId}/move", authMw(adminMw(http.HandlerFunc(h.MoveProjectAsset))))

	// Deleted assets / trash (admin)
	mux.Handle("GET /api/v1/admin/projects/{id}/assets/deleted", authMw(adminMw(http.HandlerFunc(h.ListDeletedProjectAssets))))
	mux.Handle("DELETE /api/v1/admin/projects/{id}/assets/purge", authMw(adminMw(http.HandlerFunc(h.PurgeDeletedProjectAssets))))

	// Kit status & changelog (admin)
	mux.Handle("GET /api/v1/admin/projects/{id}/kit-status", authMw(adminMw(http.HandlerFunc(h.GetProjectKitStatus))))
	mux.Handle("PUT /api/v1/admin/projects/{id}/kit-status", authMw(adminMw(http.HandlerFunc(h.UpdateProjectKitStatus))))
	mux.Handle("GET /api/v1/admin/projects/{id}/assets/changelog", authMw(adminMw(http.HandlerFunc(h.ListProjectChangelog))))
	mux.Handle("POST /api/v1/admin/projects/{id}/assets/changelog", authMw(adminMw(http.HandlerFunc(h.AddProjectChangelog))))

	// External API (API key auth)
	apiKeyMw := middleware.RequireAPIKey(svc.APIKey)
	mux.Handle("POST /api/v1/external/posts", apiKeyMw(http.HandlerFunc(h.ExternalCreatePost)))
	mux.Handle("GET /api/v1/external/posts/{id}", apiKeyMw(http.HandlerFunc(h.ExternalGetPost)))

	// SPA catch-all: serve index.html for non-API, non-file routes
	// Static files
	fs := http.FileServer(http.Dir("web/dist"))
	mux.Handle("GET /assets/", fs)
	mux.Handle("GET /showcase/", fs)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// For API-like paths or file requests, 404
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "web/dist/index.html")
	})

	// Apply global middleware chain
	var chain http.Handler = mux
	chain = rl.Middleware(chain)
	chain = middleware.RequestID(chain)
	chain = middleware.CORS(cfg.App.FrontendURL)(chain)
	chain = middleware.Logging(logger)(chain)
	chain = middleware.Recovery(chain)

	return chain
}
