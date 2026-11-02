package config

import (
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	// Clear env to get defaults
	os.Clearenv()
	cfg := Load()
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
}

func TestLoad_EnvOverrides(t *testing.T) {
	os.Clearenv()
	os.Setenv("APP_ENV", "production")
	os.Setenv("APP_PORT", "9090")
	os.Setenv("DB_HOST", "db.example.com")
	os.Setenv("REDIS_DB", "3")
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
}

func TestIsDev(t *testing.T) {
	cfg := &Config{App: AppConfig{Env: "development"}}
	if !cfg.IsDev() || cfg.IsProd() {
		t.Error("development should be dev and not prod")
	}
}

func TestIsProd_FailsClosed(t *testing.T) {
	for _, env := range []string{"production", "staging", "prod", ""} {
		cfg := &Config{App: AppConfig{Env: env}}
		if cfg.IsDev() || !cfg.IsProd() {
			t.Errorf("APP_ENV=%q must be treated as production", env)
		}
	}
}

func TestLoad_DefaultEnvIsProduction(t *testing.T) {
	os.Clearenv()
	if got := Load().App.Env; got != "production" {
		t.Fatalf("default APP_ENV = %q, want production (fail closed)", got)
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

func devConfig() *Config {
	os.Clearenv()
	cfg := Load()
	cfg.App.Env = "development"
	return cfg
}

// goodKey is a well-formed, non-repeating 32-byte test key (not a real secret).
var goodKey = hex.EncodeToString([]byte("config-test-fixture-32-bytes-ok!"))

func prodConfig() *Config {
	return &Config{
		App: AppConfig{Env: "production", FrontendURL: "https://publisher.example.com"},
		DB:  DBConfig{SSLMode: "require"},
		S3:  S3Config{Endpoint: "s3.example.com"},
		JWT: JWTConfig{
			Secret:          "another-very-long-jwt-secret-that-is-at-least-32-chars",
			AccessTokenTTL:  15 * time.Minute,
			RefreshTokenTTL: 24 * time.Hour,
		},
		TokenEncKey: goodKey,
	}
}

func TestValidate_DevDefaultsPass(t *testing.T) {
	if err := devConfig().Validate(); err != nil {
		t.Errorf("dev defaults should validate, got %v", err)
	}
}

func TestValidate_ProdRejectsDefaults(t *testing.T) {
	os.Clearenv()
	err := Load().Validate()
	if err == nil {
		t.Fatal("expected validation error for production with default values")
	}
	for _, want := range []string{"JWT_SECRET", "TOKEN_ENCRYPTION_KEY", "DB_SSLMODE", "APP_FRONTEND_URL", "S3_ENDPOINT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %s: %v", want, err)
		}
	}
}

func TestValidate_ProdAcceptsGoodConfig(t *testing.T) {
	if err := prodConfig().Validate(); err != nil {
		t.Errorf("expected no error for valid prod config, got %v", err)
	}
}

func TestValidate_Rules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"short jwt secret even in dev", func(c *Config) { c.App.Env = "development"; c.JWT.Secret = "short" }, "JWT_SECRET"},
		{"dev jwt secret in prod", func(c *Config) { c.JWT.Secret = devJWTSecret }, "JWT_SECRET"},
		{"bad hex key", func(c *Config) { c.TokenEncKey = "zz" }, "TOKEN_ENCRYPTION_KEY"},
		{"short key", func(c *Config) { c.TokenEncKey = "00ff" }, "32 bytes"},
		{"all-zero key", func(c *Config) { c.TokenEncKey = strings.Repeat("0", 64) }, "repeated byte"},
		{"bad previous key", func(c *Config) { c.TokenEncKeysPrevious = []string{"nothex"} }, "PREVIOUS"},
		{"wildcard cors", func(c *Config) { c.App.FrontendURL = "*" }, "APP_FRONTEND_URL"},
		{"dev admin in prod", func(c *Config) { c.DevAdmin.Email = "a@example.com" }, "DEV_ADMIN"},
		{"no ttl", func(c *Config) { c.JWT.AccessTokenTTL = 0 }, "TTL"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := prodConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() = %v, want error mentioning %q", err, tt.want)
			}
		})
	}
}

func TestEnvList(t *testing.T) {
	os.Clearenv()
	os.Setenv("TEST_LIST", " a, ,b ,")
	defer os.Unsetenv("TEST_LIST")
	got := envList("TEST_LIST")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("envList = %q", got)
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
