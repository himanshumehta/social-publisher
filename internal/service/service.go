package service

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/infra"
)

// Services is the dependency graph shared by the server and the worker.
type Services struct {
	Auth      *AuthService
	User      *UserService
	Category  *CategoryService
	Project   *ProjectService
	Upload    *UploadService
	Video     *VideoService
	Thumbnail *ThumbnailService
	Channel   *ChannelService
	Publish   *PublishService

	// Video publishing: OAuth-connected channels and the upload queue.
	ChannelOAuth *ChannelOAuthService
	Orchestrator *UploadOrchestrator

	// Text/image post pipeline: scan → generate → review → publish.
	Post          *PostService
	PostScanner   *PostScanner
	PostGenerator *PostGenerator
	PostReviewer  *PostReviewer
	PostPublisher *PostPublisher
	Calendar      *CalendarService

	APIKey      *APIKeyService
	TokenCrypto *TokenCrypto
}

// New wires every service. It fails if the token encryption keys are invalid,
// so a typo in TOKEN_ENCRYPTION_KEY stops start-up instead of silently storing
// OAuth tokens in plaintext.
func New(cfg *config.Config, pool *pgxpool.Pool, rdb *redis.Client, s3 *infra.S3Client) (*Services, error) {
	tokenCrypto, err := NewTokenCrypto(cfg.TokenEncKey, cfg.TokenEncKeysPrevious...)
	if err != nil {
		return nil, err
	}
	if !tokenCrypto.Enabled() {
		slog.Warn("TOKEN_ENCRYPTION_KEY not set: OAuth tokens will be stored unencrypted (development only)")
	}
	quotaTracker := NewQuotaTracker(rdb)

	// Per-platform video upload services.
	metaContainer := NewMetaContainerUploader(pool, tokenCrypto, s3)
	youtube := NewYouTubeUploadService(pool, s3, tokenCrypto, quotaTracker, cfg.OAuth.Google.ClientID, cfg.OAuth.Google.ClientSecret)
	instagram := NewInstagramUploadService(metaContainer)
	facebook := NewFacebookUploadService(pool, s3, tokenCrypto)
	threads := NewThreadsUploadService(metaContainer)
	twitter := NewTwitterUploadService(pool, s3, tokenCrypto)
	linkedin := NewLinkedInUploadService(pool, s3, tokenCrypto)

	orchestrator := NewUploadOrchestrator(pool, youtube, instagram, facebook, threads, twitter, linkedin, quotaTracker, tokenCrypto)

	logger := slog.Default()
	postScanner := NewPostScanner(pool, rdb, logger)
	postGenerator := NewPostGenerator(pool, rdb, logger, cfg.Claude.APIKey)
	postReviewer := NewPostReviewer(logger)
	postPublisher := NewPostPublisher(pool, logger, tokenCrypto)
	if cfg.Publish.DryRun {
		postPublisher.EnableDryRun()
		orchestrator.EnableDryRun(logger)
	}

	return &Services{
		Auth:          NewAuthService(cfg, pool),
		User:          NewUserService(pool),
		Category:      NewCategoryService(pool),
		Project:       NewProjectService(pool),
		Upload:        NewUploadService(pool, s3),
		Video:         NewVideoService(pool, s3),
		Thumbnail:     NewThumbnailService(pool, s3),
		Channel:       NewChannelService(pool),
		Publish:       NewPublishService(pool),
		ChannelOAuth:  NewChannelOAuthService(cfg, pool, rdb, tokenCrypto, quotaTracker),
		Orchestrator:  orchestrator,
		Post:          NewPostService(pool, postScanner, postGenerator, postReviewer, logger),
		PostScanner:   postScanner,
		PostGenerator: postGenerator,
		PostReviewer:  postReviewer,
		PostPublisher: postPublisher,
		Calendar:      NewCalendarService(pool),
		APIKey:        NewAPIKeyService(pool, logger),
		TokenCrypto:   tokenCrypto,
	}, nil
}
