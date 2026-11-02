package config

import (
	"encoding/hex"
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
	JWT         JWTConfig
	OAuth       OAuthConfig
	TokenEncKey string // Hex-encoded 32-byte key for AES-256-GCM token encryption
	// TokenEncKeysPrevious are retired keys still accepted for decryption
	// during a key rotation (TOKEN_ENCRYPTION_KEY_PREVIOUS, comma-separated).
	TokenEncKeysPrevious []string
	// TrustProxyHeaders makes the rate limiter key clients by CF-Connecting-IP
	// or X-Real-IP. Enable only behind a proxy that overwrites those headers.
	TrustProxyHeaders bool
	// OutboundAllowPrivate lets user-supplied URLs (source pages, images,
	// thumbnails) resolve to private/loopback addresses. Off by default; only
	// for development against local object storage.
	OutboundAllowPrivate bool
	Claude               ClaudeConfig
	Publish              PublishConfig
	Ingest               IngestConfig
	DevAdmin             DevAdminConfig
}

// IngestConfig bounds the API-key video ingest endpoint
// (POST /api/v1/external/videos).
type IngestConfig struct {
	// MaxVideoBytes caps one uploaded video (INGEST_MAX_VIDEO_BYTES). The
	// request body cap is this plus room for the thumbnail and form fields.
	MaxVideoBytes int64
}

// DefaultIngestMaxVideoBytes is the video size cap when INGEST_MAX_VIDEO_BYTES
// is unset: 512 MiB.
const DefaultIngestMaxVideoBytes = 512 << 20

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
	Threads   OAuthProviderConfig
}

type OAuthProviderConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

func Load() *Config {
	return &Config{
		App: AppConfig{
			Env:         envOrDefault("APP_ENV", "production"),
			Port:        envOrDefault("APP_PORT", "8081"),
			Host:        envOrDefault("APP_HOST", "localhost"),
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
		JWT: JWTConfig{
			Secret:          envOrDefault("JWT_SECRET", devJWTSecret),
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
			Threads: OAuthProviderConfig{
				ClientID:     envOrDefault("THREADS_CLIENT_ID", ""),
				ClientSecret: envOrDefault("THREADS_CLIENT_SECRET", ""),
				RedirectURL:  envOrDefault("THREADS_REDIRECT_URL", "http://localhost:8081/api/v1/admin/channels/oauth/threads/callback"),
			},
		},
		TokenEncKey:          envOrDefault("TOKEN_ENCRYPTION_KEY", ""),
		TokenEncKeysPrevious: envList("TOKEN_ENCRYPTION_KEY_PREVIOUS"),
		TrustProxyHeaders:    envOrDefaultBool("TRUST_PROXY_HEADERS", false),
		OutboundAllowPrivate: envOrDefaultBool("OUTBOUND_ALLOW_PRIVATE_NETWORKS", false),
		Claude: ClaudeConfig{
			APIKey: envOrDefault("CLAUDE_API_KEY", ""),
		},
		Publish: PublishConfig{
			DryRun: envOrDefaultBool("PUBLISH_DRY_RUN", true),
		},
		Ingest: IngestConfig{
			MaxVideoBytes: int64(envOrDefaultInt("INGEST_MAX_VIDEO_BYTES", DefaultIngestMaxVideoBytes)),
		},
		DevAdmin: DevAdminConfig{
			Email:    envOrDefault("DEV_ADMIN_EMAIL", ""),
			Password: envOrDefault("DEV_ADMIN_PASSWORD", ""),
		},
	}
}

// devJWTSecret is the fallback used when JWT_SECRET is unset. It is public
// (it is in this file), so it is only accepted in development.
const devJWTSecret = "dev-jwt-secret-change-me-in-production" // #nosec G101 -- public dev default, rejected outside development by Validate

// MinJWTSecretLen is the minimum HMAC key length accepted at startup.
const MinJWTSecretLen = 32

// IsDev reports whether the service runs in development mode. Development is
// opt-in: any APP_ENV other than "development" (including unset) is treated
// as production and gets the strict checks in Validate.
func (c *Config) IsDev() bool {
	return c.App.Env == "development"
}

// IsProd is the inverse of IsDev.
func (c *Config) IsProd() bool {
	return !c.IsDev()
}

// Validate checks the configuration at startup. Every binary calls it and
// exits on error. Some checks apply in all modes; the rest apply whenever
// APP_ENV is not "development".
func (c *Config) Validate() error {
	var errs []string

	if len(c.JWT.Secret) < MinJWTSecretLen {
		errs = append(errs, fmt.Sprintf("JWT_SECRET must be at least %d characters", MinJWTSecretLen))
	}
	if c.JWT.AccessTokenTTL <= 0 || c.JWT.RefreshTokenTTL <= 0 {
		errs = append(errs, "JWT_ACCESS_TOKEN_TTL and JWT_REFRESH_TOKEN_TTL must be positive durations")
	}
	if c.TokenEncKey != "" {
		if err := validateAESKey(c.TokenEncKey); err != nil {
			errs = append(errs, "TOKEN_ENCRYPTION_KEY: "+err.Error())
		}
	}
	for i, k := range c.TokenEncKeysPrevious {
		if err := validateAESKey(k); err != nil {
			errs = append(errs, fmt.Sprintf("TOKEN_ENCRYPTION_KEY_PREVIOUS[%d]: %s", i, err))
		}
	}
	if c.App.FrontendURL == "*" {
		errs = append(errs, "APP_FRONTEND_URL must be a single origin, not *")
	}
	if c.Ingest.MaxVideoBytes <= 0 {
		errs = append(errs, "INGEST_MAX_VIDEO_BYTES must be a positive number of bytes")
	}

	if !c.IsDev() {
		if c.JWT.Secret == devJWTSecret {
			errs = append(errs, "JWT_SECRET must be set to a random value (the built-in development secret is public)")
		}
		if c.TokenEncKey == "" {
			errs = append(errs, "TOKEN_ENCRYPTION_KEY is required outside development (64 hex chars, e.g. openssl rand -hex 32)")
		}
		if c.DB.SSLMode == "disable" {
			errs = append(errs, "DB_SSLMODE must not be 'disable' outside development")
		}
		if c.App.FrontendURL == "" || c.App.FrontendURL == "http://localhost:5173" {
			errs = append(errs, "APP_FRONTEND_URL must be set to the frontend origin")
		}
		if c.S3.Endpoint == "" || c.S3.Endpoint == "localhost:9000" {
			errs = append(errs, "S3_ENDPOINT must be set for production storage")
		}
		if c.DevAdmin.Email != "" || c.DevAdmin.Password != "" {
			errs = append(errs, "DEV_ADMIN_EMAIL/DEV_ADMIN_PASSWORD must not be set outside development; create accounts with cmd/admin")
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("invalid configuration (APP_ENV=%q):\n  - %s",
			c.App.Env, strings.Join(errs, "\n  - "))
	}
	return nil
}

// validateAESKey checks a hex-encoded AES-256 key: 32 bytes, not all one value.
func validateAESKey(hexKey string) error {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return fmt.Errorf("must be hex: %w", err)
	}
	if len(key) != 32 {
		return fmt.Errorf("must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	for _, b := range key[1:] {
		if b != key[0] {
			return nil
		}
	}
	return fmt.Errorf("must be random, not a repeated byte")
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

// envList reads a comma-separated list, dropping empty items.
func envList(key string) []string {
	var out []string
	for _, v := range strings.Split(os.Getenv(key), ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}
