package service

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/infra"
)

// buildEmailSender creates an email sender from config. Returns nil if SMTP
// is not configured (dev mode), causing the auth service to log instead.
func buildEmailSender(cfg *config.Config) EmailSenderFunc {
	if cfg.SMTP.Host == "" {
		return nil
	}
	sender := infra.NewEmailSender(
		cfg.SMTP.Host, cfg.SMTP.Port,
		cfg.SMTP.Username, cfg.SMTP.Password,
		cfg.SMTP.From, cfg.SMTP.UseTLS,
	)
	return sender.Send
}

type Services struct {
	Auth          *AuthService
	User          *UserService
	Category      *CategoryService
	Audit         *AuditService
	Project       *ProjectService
	Upload        *UploadService
	Archive       *ArchiveService
	Video         *VideoService
	Thumbnail     *ThumbnailService
	Transition    *TransitionService
	Review        *ReviewService
	Dashboard     *DashboardService
	Payout        *PayoutService
	RawAsset      *RawAssetService
	Channel       *ChannelService
	ChannelTarget *ChannelTargetService
	ClipJob       *ClipJobService
	Publish       *PublishService
	Notification  *NotificationService
	// V2: Platform Publishing
	ChannelOAuth *ChannelOAuthService
	Orchestrator *UploadOrchestrator

	// V3: Post Publishing Pipeline
	Post          *PostService
	PostScanner   *PostScanner
	PostGenerator *PostGenerator
	PostReviewer  *PostReviewer
	PostPublisher *PostPublisher
	APIKey        *APIKeyService
	TokenCrypto   *TokenCrypto

	// Unified Pipeline
	ProjectPost *ProjectPostService

	// V4: Simplified Flows
	EditorAlignment *EditorAlignmentService

	// V5: Revenue Sharing
	Revenue *RevenueService

	// V6: AI Editing Plugin
	AIJob   *AIJobService
	Credits *CreditsService

	// V7: Editor Marketplace
	Marketplace *MarketplaceService

	// V8: New Features
	Calendar           *CalendarService
	Analytics          *AnalyticsService
	Repurpose          *RepurposeService
	BrandKit           *BrandKitService
	Virality           *ViralityService
	ABTest             *ABTestService
	Team               *TeamService
	Approval           *ApprovalService
	Portfolio          *PortfolioService
	RevenueAttribution *RevenueAttributionService
	Mobile             *MobileService
	Org                *OrgService
	Commerce           *CommerceService

	// V9: AI Dubbing
	Dub *DubService

	// V9: Smart Clips
	SmartClip *SmartClipService

	// V9: YouTube SEO
	SEO *SEOService

	// V9: Newsletter
	Newsletter *NewsletterService

	// V10: Project Assets (Creator Kit)
	ProjectAsset *ProjectAssetService
}

func New(cfg *config.Config, pool *pgxpool.Pool, rdb *redis.Client, hotS3, coldS3 *infra.S3Client) *Services {
	audit := NewAuditService(pool)
	emailFn := buildEmailSender(cfg)

	// V2: Shared infrastructure
	tokenCrypto, _ := NewTokenCrypto(cfg.TokenEncKey) // nil in dev mode (no encryption)
	quotaTracker := NewQuotaTracker(rdb)

	// V2: Platform upload services
	metaContainer := NewMetaContainerUploader(pool, tokenCrypto, hotS3)
	youtube := NewYouTubeUploadService(pool, hotS3, tokenCrypto, quotaTracker, cfg.OAuth.Google.ClientID, cfg.OAuth.Google.ClientSecret)
	instagram := NewInstagramUploadService(metaContainer)
	facebook := NewFacebookUploadService(pool, hotS3, tokenCrypto)
	threads := NewThreadsUploadService(metaContainer)
	twitter := NewTwitterUploadService(pool, hotS3, tokenCrypto)
	linkedin := NewLinkedInUploadService(pool, hotS3, tokenCrypto)

	orchestrator := NewUploadOrchestrator(pool, youtube, instagram, facebook, threads, twitter, linkedin, quotaTracker, tokenCrypto)

	// V3: Post publishing pipeline
	logger := slog.Default()
	postScanner := NewPostScanner(pool, rdb, logger)
	postGenerator := NewPostGenerator(pool, rdb, logger, cfg.Claude.APIKey)
	postReviewer := NewPostReviewer(logger)
	postPublisher := NewPostPublisher(pool, logger, tokenCrypto)
	if cfg.Publish.DryRun {
		postPublisher.EnableDryRun()
		orchestrator.EnableDryRun(logger)
	}
	postSvc := NewPostService(pool, postScanner, postGenerator, postReviewer, logger)
	apiKeySvc := NewAPIKeyService(pool, logger)

	projectPostSvc := NewProjectPostService(pool, postSvc, logger)
	transitionSvc := NewTransitionService(pool)
	transitionSvc.SetProjectPostService(projectPostSvc)

	return &Services{
		Auth:               NewAuthService(cfg, pool, rdb, emailFn),
		User:               NewUserService(pool, audit, hotS3, rdb),
		Category:           NewCategoryService(pool),
		Audit:              audit,
		Project:            NewProjectService(pool),
		Upload:             NewUploadService(pool, hotS3),
		Archive:            NewArchiveService(pool, hotS3, coldS3),
		Video:              NewVideoService(pool, hotS3),
		Thumbnail:          NewThumbnailService(pool, hotS3),
		Transition:         transitionSvc,
		Review:             NewReviewService(pool),
		Dashboard:          NewDashboardService(pool),
		Payout:             NewPayoutService(pool),
		RawAsset:           NewRawAssetService(pool, hotS3),
		Channel:            NewChannelService(pool),
		ChannelTarget:      NewChannelTargetService(pool),
		ClipJob:            NewClipJobService(pool),
		Publish:            NewPublishService(pool),
		Notification:       NewNotificationService(pool),
		ChannelOAuth:       NewChannelOAuthService(cfg, pool, rdb, tokenCrypto, quotaTracker),
		Orchestrator:       orchestrator,
		Post:               postSvc,
		PostScanner:        postScanner,
		PostGenerator:      postGenerator,
		PostReviewer:       postReviewer,
		PostPublisher:      postPublisher,
		APIKey:             apiKeySvc,
		TokenCrypto:        tokenCrypto,
		ProjectPost:        projectPostSvc,
		EditorAlignment:    NewEditorAlignmentService(pool),
		Revenue:            NewRevenueService(pool),
		AIJob:              NewAIJobService(pool),
		Credits:            NewCreditsService(pool),
		Marketplace:        NewMarketplaceService(pool),
		Calendar:           NewCalendarService(pool),
		Analytics:          NewAnalyticsService(pool),
		Repurpose:          NewRepurposeService(pool),
		BrandKit:           NewBrandKitService(pool),
		Virality:           NewViralityService(pool, NewScoringEngine()),
		ABTest:             NewABTestService(pool),
		Team:               NewTeamService(pool),
		Approval:           NewApprovalService(pool),
		Portfolio:          NewPortfolioService(pool),
		RevenueAttribution: NewRevenueAttributionService(pool),
		Mobile:             NewMobileService(pool),
		Org:                NewOrgService(pool),
		Commerce:           NewCommerceService(pool),
		Dub:                NewDubService(pool),
		SmartClip:          NewSmartClipService(pool),
		SEO:                NewSEOService(pool),
		Newsletter:         NewNewsletterService(pool),
		ProjectAsset:       NewProjectAssetService(pool),
	}
}
