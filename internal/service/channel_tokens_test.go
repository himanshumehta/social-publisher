package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// DB-backed: channels.token_encrypted must describe what is actually stored.
func TestStoreTokens_EncryptedFlagIsTruthful(t *testing.T) {
	pool := setupTestPool(t)
	defer pool.Close()
	ctx := context.Background()

	for _, withKey := range []bool{false, true} {
		var crypto *TokenCrypto
		if withKey {
			crypto, _ = NewTokenCrypto(validHexKey(t))
		}
		svc := &ChannelOAuthService{pool: pool, crypto: crypto}

		var id uuid.UUID
		if err := pool.QueryRow(ctx,
			`INSERT INTO channels (platform, channel_name, channel_type) VALUES ('linkedin', 'token-flag-test', 'system') RETURNING id`,
		).Scan(&id); err != nil {
			t.Fatal(err)
		}
		defer pool.Exec(ctx, `DELETE FROM channels WHERE id = $1`, id)

		if err := svc.storeTokens(ctx, id, providerLinkedIn, "access-123", "refresh-456", time.Now().Add(time.Hour)); err != nil {
			t.Fatalf("storeTokens: %v", err)
		}
		var stored string
		var flag bool
		if err := pool.QueryRow(ctx, `SELECT access_token, token_encrypted FROM channels WHERE id = $1`, id).Scan(&stored, &flag); err != nil {
			t.Fatal(err)
		}
		if flag != withKey {
			t.Errorf("withKey=%v: token_encrypted = %v", withKey, flag)
		}
		if withKey && stored == "access-123" {
			t.Error("token stored in plaintext although a key is configured")
		}
		access, refresh, err := svc.getDecryptedTokens(ctx, id)
		if err != nil || access != "access-123" || refresh != "refresh-456" {
			t.Errorf("withKey=%v: round trip = %q, %q, %v", withKey, access, refresh, err)
		}
	}
}
