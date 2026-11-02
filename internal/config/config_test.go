package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env to get defaults
	os.Clearenv()
	cfg := Load()
	if cfg.App.Env != "development" {
		t.Errorf("expected development, got %s", cfg.App.Env)
	}
	if cfg.App.Port != "8081" {
		t.Errorf("expected 8081, got %s", cfg.App.Port)
	}
	if cfg.DB.User != "socialpub" {
		t.Errorf("expected socialpub, got %s", cfg.DB.User)
	}
	if cfg.DB.Name != "socialpub" {
		t.Errorf("expected socialpub, got %s", cfg.DB.Name)
	}
	if cfg.Redis.DB != 1 {
		t.Errorf("expected Redis DB 1, got %d", cfg.Redis.DB)
	}
	if cfg.S3.Bucket != "social-publisher-hot" {
		t.Errorf("expected social-publisher-hot, got %s", cfg.S3.Bucket)
	}
	if cfg.JWT.AccessTokenTTL != 15*time.Minute {
		t.Errorf("expected 15m access TTL, got %v", cfg.JWT.AccessTokenTTL)
	}
	if cfg.JWT.RefreshTokenTTL != 30*24*time.Hour {
		t.Errorf("expected 30d refresh TTL, got %v", cfg.JWT.RefreshTokenTTL)
	}
	if cfg.Platform.FeePercent != 30 {
		t.Errorf("expected 30%% fee, got %d", cfg.Platform.FeePercent)
	}
	if cfg.Platform.TDSRatePercent != 10 {
		t.Errorf("expected 10%% TDS, got %d", cfg.Platform.TDSRatePercent)
	}
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Clearenv()
	os.Setenv("APP_ENV", "production")
	os.Setenv("APP_PORT", "9090")
	os.Setenv("DB_HOST", "db.example.com")
	os.Setenv("REDIS_DB", "3")
	os.Setenv("PLATFORM_FEE_PERCENT", "25")
	defer os.Clearenv()

	cfg := Load()
	if cfg.App.Env != "production" {
		t.Errorf("expected production, got %s", cfg.App.Env)
	}
	if cfg.App.Port != "9090" {
		t.Errorf("expected 9090, got %s", cfg.App.Port)
	}
	if cfg.DB.Host != "db.example.com" {
		t.Errorf("expected db.example.com, got %s", cfg.DB.Host)
	}
	if cfg.Redis.DB != 3 {
		t.Errorf("expected Redis DB 3, got %d", cfg.Redis.DB)
	}
	if cfg.Platform.FeePercent != 25 {
		t.Errorf("expected 25%% fee, got %d", cfg.Platform.FeePercent)
	}
}

func TestIsDev(t *testing.T) {
	cfg := &Config{App: AppConfig{Env: "development"}}
	if !cfg.IsDev() {
		t.Error("expected IsDev() true for development")
	}
	if cfg.IsProd() {
		t.Error("expected IsProd() false for development")
	}
}

func TestIsProd(t *testing.T) {
	cfg := &Config{App: AppConfig{Env: "production"}}
	if !cfg.IsProd() {
		t.Error("expected IsProd() true for production")
	}
	if cfg.IsDev() {
		t.Error("expected IsDev() false for production")
	}
}

func TestDBConfig_DSN(t *testing.T) {
	db := DBConfig{
		Host: "localhost", Port: "5432", User: "socialpub",
		Password: "secret", Name: "socialpub", SSLMode: "disable",
	}
	expected := "postgres://socialpub:secret@localhost:5432/socialpub?sslmode=disable"
	if got := db.DSN(); got != expected {
		t.Errorf("expected DSN %s, got %s", expected, got)
	}
}

func TestValidate_DevSkipsValidation(t *testing.T) {
	cfg := &Config{App: AppConfig{Env: "development"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error in dev, got %v", err)
	}
}

func TestValidate_ProdRejectsDefaults(t *testing.T) {
	os.Clearenv()
	cfg := Load()
	cfg.App.Env = "production"
	err := cfg.Validate()
	if err == nil {
		t.Error("expected validation error for production with default values")
	}
}

func TestValidate_ProdAcceptsGoodConfig(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			Env:         "production",
			Secret:      "a-very-long-production-secret-that-is-at-least-32-chars",
			BaseURL:     "https://publisher.example.com",
			FrontendURL: "https://publisher.example.com",
		},
		DB: DBConfig{SSLMode: "require"},
		S3: S3Config{Endpoint: "s3.example.com"},
		JWT: JWTConfig{
			Secret: "another-very-long-jwt-secret-that-is-at-least-32-chars",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error for valid prod config, got %v", err)
	}
}

func TestEnvOrDefaultDuration(t *testing.T) {
	os.Clearenv()
	// No env set - fallback
	d := envOrDefaultDuration("TEST_DUR", 5*time.Minute)
	if d != 5*time.Minute {
		t.Errorf("expected 5m default, got %v", d)
	}
	// Set env
	os.Setenv("TEST_DUR", "30s")
	d = envOrDefaultDuration("TEST_DUR", 5*time.Minute)
	if d != 30*time.Second {
		t.Errorf("expected 30s from env, got %v", d)
	}
	os.Unsetenv("TEST_DUR")
}

func TestEnvOrDefaultBool(t *testing.T) {
	os.Clearenv()
	b := envOrDefaultBool("TEST_BOOL", false)
	if b != false {
		t.Error("expected false default")
	}
	os.Setenv("TEST_BOOL", "true")
	b = envOrDefaultBool("TEST_BOOL", false)
	if b != true {
		t.Error("expected true from env")
	}
	os.Unsetenv("TEST_BOOL")
}
