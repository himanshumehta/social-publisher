package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App         AppConfig
	DB          DBConfig
	Redis       RedisConfig
	S3          S3Config
	S3Cold      S3Config
	JWT         JWTConfig
	OAuth       OAuthConfig
	Platform    PlatformConfig
	Volume      VolumeConfig
	SMTP        SMTPConfig
	Upload      UploadConfig
	Quota       QuotaConfig
	TokenEncKey string // Hex-encoded 32-byte key for AES-256-GCM token encryption
	Claude      ClaudeConfig
	Publish     PublishConfig
	DevAdmin    DevAdminConfig
}

// PublishConfig controls outbound publishing to social platforms.
type PublishConfig struct {
	// DryRun routes every platform adapter to a mock that logs what it would
	// publish and marks the post published, without calling any platform API.
	// Defaults to true; set PUBLISH_DRY_RUN=false to post for real.
	DryRun bool
}

// DevAdminConfig bootstraps a local admin account at server start.
// Only honoured when APP_ENV=development and both fields are set.
type DevAdminConfig struct {
	Email    string
	Password string
}

type ClaudeConfig struct {
	APIKey string
}

type AppConfig struct {
	Env         string
	Port        string
	Host        string
	Secret      string
	BaseURL     string
	FrontendURL string
}

type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
	SSLMode  string
	MaxConns int
	MinConns int
}

func (c DBConfig) DSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=%s",
		c.User, c.Password, c.Host, c.Port, c.Name, c.SSLMode,
	)
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
	TLS      bool
}

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	UseSSL    bool
}

type JWTConfig struct {
	Secret          string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
}

type OAuthConfig struct {
	Google    OAuthProviderConfig
	Facebook  OAuthProviderConfig
	Instagram OAuthProviderConfig // Instagram Business Login (separate from Facebook flow)
	LinkedIn  OAuthProviderConfig
	Twitter   OAuthProviderConfig
	Threads   ThreadsConfig
}

type ThreadsConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type UploadConfig struct {
	MaxSizeMB   int
	ChunkSizeMB int
}

type QuotaConfig struct {
	YouTubeDailyUnits int
}

type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

type PlatformConfig struct {
	FeePercent     int
	TDSRatePercent int
}

type VolumeConfig struct {
	Path string
}

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	UseTLS   bool
}

func Load() *Config {
	return &Config{
		App: AppConfig{
			Env:         envOrDefault("APP_ENV", "development"),
			Port:        envOrDefault("APP_PORT", "8081"),
			Host:        envOrDefault("APP_HOST", "localhost"),
			Secret:      envOrDefault("APP_SECRET", "dev-secret-change-me-in-production"),
			BaseURL:     envOrDefault("APP_BASE_URL", "http://localhost:8081"),
			FrontendURL: envOrDefault("APP_FRONTEND_URL", "http://localhost:5173"),
		},
		DB: DBConfig{
			Host:     envOrDefault("DB_HOST", "localhost"),
			Port:     envOrDefault("DB_PORT", "5432"),
			User:     envOrDefault("DB_USER", "socialpub"),
			Password: envOrDefault("DB_PASSWORD", "socialpub"),
			Name:     envOrDefault("DB_NAME", "socialpub"),
			SSLMode:  envOrDefault("DB_SSLMODE", "disable"),
			MaxConns: envOrDefaultInt("DB_MAX_CONNS", 25),
			MinConns: envOrDefaultInt("DB_MIN_CONNS", 5),
		},
		Redis: RedisConfig{
			Addr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
			Password: envOrDefault("REDIS_PASSWORD", ""),
			DB:       envOrDefaultInt("REDIS_DB", 1),
			TLS:      envOrDefaultBool("REDIS_TLS", false),
		},
		S3: S3Config{
			Endpoint:  envOrDefault("S3_ENDPOINT", "localhost:9000"),
			AccessKey: envOrDefault("S3_ACCESS_KEY", "minioadmin"),
			SecretKey: envOrDefault("S3_SECRET_KEY", "minioadmin"),
			Bucket:    envOrDefault("S3_BUCKET", "social-publisher-hot"),
			Region:    envOrDefault("S3_REGION", "us-east-1"),
			UseSSL:    envOrDefaultBool("S3_USE_SSL", false),
		},
		S3Cold: S3Config{
			Endpoint:  envOrDefault("S3_COLD_ENDPOINT", envOrDefault("S3_ENDPOINT", "localhost:9000")),
			AccessKey: envOrDefault("S3_COLD_ACCESS_KEY", envOrDefault("S3_ACCESS_KEY", "minioadmin")),
			SecretKey: envOrDefault("S3_COLD_SECRET_KEY", envOrDefault("S3_SECRET_KEY", "minioadmin")),
			Bucket:    envOrDefault("S3_COLD_BUCKET", "social-publisher-cold"),
			Region:    envOrDefault("S3_COLD_REGION", envOrDefault("S3_REGION", "us-east-1")),
			UseSSL:    envOrDefaultBool("S3_COLD_USE_SSL", envOrDefaultBool("S3_USE_SSL", false)),
		},
		JWT: JWTConfig{
			Secret:          envOrDefault("JWT_SECRET", "dev-jwt-secret-change-me-in-production"),
			AccessTokenTTL:  envOrDefaultDuration("JWT_ACCESS_TOKEN_TTL", 15*time.Minute),
			RefreshTokenTTL: envOrDefaultDuration("JWT_REFRESH_TOKEN_TTL", 30*24*time.Hour),
		},
		OAuth: OAuthConfig{
			Google: OAuthProviderConfig{
				ClientID:     envOrDefault("GOOGLE_CLIENT_ID", ""),
				ClientSecret: envOrDefault("GOOGLE_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("GOOGLE_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/google/callback"),
			},
			Facebook: OAuthProviderConfig{
				ClientID:     envOrDefault("FACEBOOK_CLIENT_ID", ""),
				ClientSecret: envOrDefault("FACEBOOK_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("FACEBOOK_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/facebook/callback"),
			},
			Instagram: OAuthProviderConfig{
				ClientID:     envOrDefault("INSTAGRAM_CLIENT_ID", ""),
				ClientSecret: envOrDefault("INSTAGRAM_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("INSTAGRAM_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/instagram/callback"),
			},
			LinkedIn: OAuthProviderConfig{
				ClientID:     envOrDefault("LINKEDIN_CLIENT_ID", ""),
				ClientSecret: envOrDefault("LINKEDIN_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("LINKEDIN_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/linkedin/callback"),
			},
			Twitter: OAuthProviderConfig{
				ClientID:     envOrDefault("TWITTER_CLIENT_ID", ""),
				ClientSecret: envOrDefault("TWITTER_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("TWITTER_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/twitter/callback"),
			},
			Threads: ThreadsConfig{
				ClientID:     envOrDefault("THREADS_CLIENT_ID", ""),
				ClientSecret: envOrDefault("THREADS_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("THREADS_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/threads/callback"),
			},
		},
		Platform: PlatformConfig{
			FeePercent:     envOrDefaultInt("PLATFORM_FEE_PERCENT", 30),
			TDSRatePercent: envOrDefaultInt("PLATFORM_TDS_RATE_PERCENT", 10),
		},
		Volume: VolumeConfig{
			Path: envOrDefault("VOLUME_PATH", "/tmp/social-publisher-workspace"),
		},
		Upload: UploadConfig{
			MaxSizeMB:   envOrDefaultInt("MAX_UPLOAD_SIZE_MB", 5120),
			ChunkSizeMB: envOrDefaultInt("UPLOAD_CHUNK_SIZE_MB", 10),
		},
		Quota: QuotaConfig{
			YouTubeDailyUnits: envOrDefaultInt("YOUTUBE_DAILY_QUOTA", 10000),
		},
		TokenEncKey: envOrDefault("TOKEN_ENCRYPTION_KEY", ""),
		Claude: ClaudeConfig{
			APIKey: envOrDefault("CLAUDE_API_KEY", ""),
		},
		Publish: PublishConfig{
			DryRun: envOrDefaultBool("PUBLISH_DRY_RUN", true),
		},
		DevAdmin: DevAdminConfig{
			Email:    envOrDefault("DEV_ADMIN_EMAIL", ""),
			Password: envOrDefault("DEV_ADMIN_PASSWORD", ""),
		},
		SMTP: SMTPConfig{
			Host:     envOrDefault("SMTP_HOST", ""),
			Port:     envOrDefault("SMTP_PORT", "587"),
			Username: envOrDefault("SMTP_USERNAME", ""),
			Password: envOrDefault("SMTP_PASSWORD", ""),
			From:     envOrDefault("SMTP_FROM", "noreply@example.com"),
			UseTLS:   envOrDefaultBool("SMTP_USE_TLS", true),
		},
	}
}

func (c *Config) IsDev() bool {
	return c.App.Env == "development"
}

func (c *Config) IsProd() bool {
	return c.App.Env == "production"
}

func (c *Config) Validate() error {
	if !c.IsProd() {
		return nil // skip validation in development
	}

	var errs []string

	if c.App.Secret == "dev-secret-change-me-in-production" || len(c.App.Secret) < 32 {
		errs = append(errs, "APP_SECRET must be set to a random string (min 32 chars)")
	}

	if c.JWT.Secret == "dev-jwt-secret-change-me-in-production" || len(c.JWT.Secret) < 32 {
		errs = append(errs, "JWT_SECRET must be set to a random string (min 32 chars)")
	}

	if c.DB.SSLMode == "disable" {
		errs = append(errs, "DB_SSLMODE must not be 'disable' in production")
	}

	if c.App.BaseURL == "" || c.App.BaseURL == "http://localhost:8081" {
		errs = append(errs, "APP_BASE_URL must be set to the production domain")
	}

	if c.App.FrontendURL == "" || c.App.FrontendURL == "http://localhost:5173" {
		errs = append(errs, "APP_FRONTEND_URL must be set to the production frontend domain")
	}

	if c.S3.Endpoint == "" || c.S3.Endpoint == "localhost:9000" {
		errs = append(errs, "S3_ENDPOINT must be set for production storage")
	}

	if len(errs) > 0 {
		return fmt.Errorf("production config validation failed:\n  - %s",
			strings.Join(errs, "\n  - "))
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

func envOrDefaultBool(key string, fallback bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return fallback
}

func envOrDefaultDuration(key string, fallback time.Duration) time.Duration {
	if val := os.Getenv(key); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return fallback
}
