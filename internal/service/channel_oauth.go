package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/config"
	"github.com/himanshumehta/social-publisher/internal/model"
)

// memStateStore provides in-memory OAuth state storage as a fallback when Redis
// is unavailable. Entries auto-expire based on their TTL.
var memState = struct {
	sync.Mutex
	m map[string]memStateEntry
}{m: make(map[string]memStateEntry)}

type memStateEntry struct {
	value     string
	expiresAt time.Time
}

func memStateSet(key, value string, ttl time.Duration) {
	memState.Lock()
	defer memState.Unlock()
	// Lazy cleanup of expired entries
	now := time.Now()
	for k, v := range memState.m {
		if now.After(v.expiresAt) {
			delete(memState.m, k)
		}
	}
	memState.m[key] = memStateEntry{value: value, expiresAt: now.Add(ttl)}
}

func memStateGet(key string) (string, bool) {
	memState.Lock()
	defer memState.Unlock()
	e, ok := memState.m[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(memState.m, key)
		return "", false
	}
	return e.value, true
}

func memStateDel(key string) {
	memState.Lock()
	defer memState.Unlock()
	delete(memState.m, key)
}

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const (
	oauthStatePrefix = "oauth_state:"
	oauthStateTTL    = 10 * time.Minute
	pkcePrefix       = "oauth_pkce:"
	pkceTTL          = 10 * time.Minute

	// Token expiry warning threshold for CheckExpiringTokens.
	tokenExpiryWarningDays = 7

	// Platform identifiers used in oauth_provider column.
	providerGoogle    = "google"
	providerFacebook  = "facebook"
	providerInstagram = "instagram"
	providerThreads   = "threads"
	providerTwitter   = "twitter"
	providerLinkedIn  = "linkedin"

	// HTTP timeout for outbound platform API calls.
	platformHTTPTimeout = 15 * time.Second

	// Google OAuth scopes for YouTube.
	googleScopeYouTubeUpload   = "https://www.googleapis.com/auth/youtube.upload"
	googleScopeYouTube         = "https://www.googleapis.com/auth/youtube"
	googleScopeYouTubeReadonly = "https://www.googleapis.com/auth/youtube.readonly"
	googleScopeYTAnalytics     = "https://www.googleapis.com/auth/yt-analytics.readonly"
)

// ---------------------------------------------------------------------------
// ChannelTokenWarning is returned by CheckExpiringTokens.
// ---------------------------------------------------------------------------

type ChannelTokenWarning struct {
	ChannelID      uuid.UUID `json:"channel_id"`
	ChannelName    string    `json:"channel_name"`
	Platform       string    `json:"platform"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
	DaysRemaining  int       `json:"days_remaining"`
}

// ---------------------------------------------------------------------------
// Service struct
// ---------------------------------------------------------------------------

// ChannelOAuthService manages OAuth flows for all supported social platforms.
type ChannelOAuthService struct {
	pool   *pgxpool.Pool
	rdb    *redis.Client
	crypto *TokenCrypto
	cfg    *config.Config
	http   *http.Client
	quota  *QuotaTracker
}

// NewChannelOAuthService creates a new ChannelOAuthService.
func NewChannelOAuthService(cfg *config.Config, pool *pgxpool.Pool, rdb *redis.Client, crypto *TokenCrypto, quota *QuotaTracker) *ChannelOAuthService {
	return &ChannelOAuthService{
		pool:   pool,
		rdb:    rdb,
		crypto: crypto,
		cfg:    cfg,
		quota:  quota,
		http: &http.Client{
			Timeout: platformHTTPTimeout,
		},
	}
}

// ===========================================================================
// Internal helpers
// ===========================================================================

// generateState creates a cryptographic state token, stores it in Redis (or
// in-memory if Redis is unavailable), and returns the state string.
func (s *ChannelOAuthService) generateState(ctx context.Context, channelID uuid.UUID) (string, error) {
	nonce := uuid.New().String()
	state := channelID.String() + ":" + nonce
	key := oauthStatePrefix + state

	err := s.rdb.Set(ctx, key, channelID.String(), oauthStateTTL).Err()
	if err != nil {
		// Fallback to in-memory store when Redis is unavailable
		memStateSet(key, channelID.String(), oauthStateTTL)
	}
	return state, nil
}

// validateState checks the state parameter against Redis (or in-memory
// fallback) and returns the associated channel ID. State is single-use.
func (s *ChannelOAuthService) validateState(ctx context.Context, state string) (uuid.UUID, error) {
	key := oauthStatePrefix + state

	channelIDStr, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		// Try in-memory fallback
		val, ok := memStateGet(key)
		if !ok {
			return uuid.Nil, fmt.Errorf("invalid or expired oauth state")
		}
		channelIDStr = val
		memStateDel(key)
	} else {
		// Consume the state so it cannot be replayed.
		s.rdb.Del(ctx, key)
	}

	channelID, err := uuid.Parse(channelIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("parse channel id from state: %w", err)
	}
	return channelID, nil
}

// generatePKCE creates a code_verifier and code_challenge (S256) and stores
// the verifier in Redis (or in-memory fallback) keyed by state.
func (s *ChannelOAuthService) generatePKCE(ctx context.Context, state string) (codeChallenge string, err error) {
	verifier, err := randomString(64)
	if err != nil {
		return "", fmt.Errorf("generate pkce verifier: %w", err)
	}

	h := sha256.Sum256([]byte(verifier))
	codeChallenge = base64.RawURLEncoding.EncodeToString(h[:])

	key := pkcePrefix + state
	if redisErr := s.rdb.Set(ctx, key, verifier, pkceTTL).Err(); redisErr != nil {
		memStateSet(key, verifier, pkceTTL)
	}

	return codeChallenge, nil
}

// consumePKCEVerifier retrieves and deletes the PKCE code_verifier.
func (s *ChannelOAuthService) consumePKCEVerifier(ctx context.Context, state string) (string, error) {
	key := pkcePrefix + state
	verifier, err := s.rdb.Get(ctx, key).Result()
	if err != nil {
		// Try in-memory fallback
		val, ok := memStateGet(key)
		if !ok {
			return "", fmt.Errorf("pkce verifier not found or expired")
		}
		memStateDel(key)
		return val, nil
	}
	s.rdb.Del(ctx, key)
	return verifier, nil
}

// randomString generates a cryptographically random URL-safe string of the
// given byte length (output will be longer due to base64 encoding).
func randomString(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// storeTokens encrypts and persists OAuth tokens for a channel.
func (s *ChannelOAuthService) storeTokens(ctx context.Context, channelID uuid.UUID, provider, accessToken, refreshToken string, expiresAt time.Time) error {
	encAccess, err := s.crypto.Encrypt(accessToken)
	if err != nil {
		return fmt.Errorf("encrypt access token: %w", err)
	}

	encRefresh := ""
	if refreshToken != "" {
		encRefresh, err = s.crypto.Encrypt(refreshToken)
		if err != nil {
			return fmt.Errorf("encrypt refresh token: %w", err)
		}
	}

	_, err = s.pool.Exec(ctx,
		`UPDATE channels SET
			access_token = $1,
			refresh_token = $2,
			token_expires_at = $3,
			oauth_provider = $4,
			connection_status = 'connected',
			token_encrypted = TRUE,
			last_token_refresh = NOW(),
			updated_at = NOW()
		WHERE id = $5`,
		encAccess, strPtrOrNil(encRefresh), expiresAt, provider, channelID)
	if err != nil {
		return fmt.Errorf("store tokens for channel %s: %w", channelID, err)
	}
	return nil
}

// getDecryptedTokens retrieves and decrypts the access and refresh tokens for
// a channel. Returns empty strings if tokens are not set.
func (s *ChannelOAuthService) getDecryptedTokens(ctx context.Context, channelID uuid.UUID) (accessToken, refreshToken string, err error) {
	var encAccess, encRefresh string
	var encrypted bool

	err = s.pool.QueryRow(ctx,
		`SELECT COALESCE(access_token, ''), COALESCE(refresh_token, ''), COALESCE(token_encrypted, FALSE)
		 FROM channels WHERE id = $1`, channelID,
	).Scan(&encAccess, &encRefresh, &encrypted)
	if err == pgx.ErrNoRows {
		return "", "", fmt.Errorf("channel not found: %s", channelID)
	}
	if err != nil {
		return "", "", fmt.Errorf("query channel tokens: %w", err)
	}

	if !encrypted || encAccess == "" {
		return encAccess, encRefresh, nil
	}

	accessToken, err = s.crypto.Decrypt(encAccess)
	if err != nil {
		return "", "", fmt.Errorf("decrypt access token: %w", err)
	}

	if encRefresh != "" {
		refreshToken, err = s.crypto.Decrypt(encRefresh)
		if err != nil {
			return "", "", fmt.Errorf("decrypt refresh token: %w", err)
		}
	}

	return accessToken, refreshToken, nil
}

// updateChannelInfo updates the channel_name, channel_id, channel_url,
// scopes, and platform-specific metadata after a successful OAuth callback.
func (s *ChannelOAuthService) updateChannelInfo(ctx context.Context, channelID uuid.UUID, name, extID, extURL, platform string, scopes []string) error {
	// Build platform-specific metadata so upload services can find the ID.
	metadataMap := map[string]string{}
	switch platform {
	case "facebook":
		metadataMap["page_id"] = extID
	case "instagram":
		metadataMap["instagram_business_account_id"] = extID
	case "threads":
		metadataMap["threads_user_id"] = extID
	}

	metadataJSON, _ := json.Marshal(metadataMap)

	_, err := s.pool.Exec(ctx,
		`UPDATE channels SET
			channel_name = $1,
			channel_id = $2,
			channel_url = $3,
			platform = $4,
			scopes = $5,
			metadata = $6,
			updated_at = NOW()
		WHERE id = $7`,
		name, extID, extURL, platform, scopes, metadataJSON, channelID)
	if err != nil {
		return fmt.Errorf("update channel info: %w", err)
	}
	return nil
}

// getChannel is a small helper to load a channel model by ID.
func (s *ChannelOAuthService) getChannel(ctx context.Context, channelID uuid.UUID) (*model.Channel, error) {
	var c model.Channel
	err := s.pool.QueryRow(ctx,
		`SELECT `+channelColumns+` FROM channels WHERE id = $1`, channelID,
	).Scan(&c.ID, &c.CategoryID, &c.UserID, &c.ChannelType, &c.Platform, &c.ChannelName, &c.ChannelID,
		&c.ChannelURL, &c.OAuthProvider, &c.ConnectionStatus, &c.IsActive, &c.TokenExpiresAt, &c.CreatedAt, &c.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}
	if err != nil {
		return nil, fmt.Errorf("get channel: %w", err)
	}
	return &c, nil
}

// doJSONGet performs an HTTP GET and decodes the JSON response into dest.
func (s *ChannelOAuthService) doJSONGet(ctx context.Context, rawURL string, headers map[string]string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("http get %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http get %s returned %d: %s", rawURL, resp.StatusCode, truncate(string(body), 512))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode json from %s: %w", rawURL, err)
	}
	return nil
}

// doFormPost performs a POST with form-encoded body and decodes the JSON
// response into dest.
func (s *ChannelOAuthService) doFormPost(ctx context.Context, rawURL string, data url.Values, headers map[string]string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := s.http.Do(req)
	if err != nil {
		return fmt.Errorf("http post %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http post %s returned %d: %s", rawURL, resp.StatusCode, truncate(string(body), 512))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode json from %s: %w", rawURL, err)
	}
	return nil
}

// truncate trims a string to at most n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// ===========================================================================
// Common operations
// ===========================================================================

// DisconnectChannel clears the stored tokens and sets the connection status
// to disconnected for the given channel.
func (s *ChannelOAuthService) DisconnectChannel(ctx context.Context, channelID uuid.UUID) error {
	result, err := s.pool.Exec(ctx,
		`UPDATE channels SET
			access_token = NULL,
			refresh_token = NULL,
			token_expires_at = NULL,
			connection_status = 'disconnected',
			token_encrypted = FALSE,
			last_token_refresh = NULL,
			updated_at = NOW()
		WHERE id = $1`, channelID)
	if err != nil {
		return fmt.Errorf("disconnect channel %s: %w", channelID, err)
	}
	if result.RowsAffected() == 0 {
		return fmt.Errorf("channel not found: %s", channelID)
	}
	return nil
}

// CheckExpiringTokens returns channels whose tokens expire within the
// configured warning period (7 days). This is intended to be called by a
// background job or health-check endpoint.
func (s *ChannelOAuthService) CheckExpiringTokens(ctx context.Context) ([]ChannelTokenWarning, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, channel_name, platform, token_expires_at
		 FROM channels
		 WHERE connection_status = 'connected'
		   AND token_expires_at IS NOT NULL
		   AND token_expires_at > NOW()
		   AND token_expires_at <= NOW() + INTERVAL '1 day' * $1
		 ORDER BY token_expires_at ASC`,
		tokenExpiryWarningDays)
	if err != nil {
		return nil, fmt.Errorf("query expiring tokens: %w", err)
	}
	defer rows.Close()

	var warnings []ChannelTokenWarning
	now := time.Now()

	for rows.Next() {
		var w ChannelTokenWarning
		if err := rows.Scan(&w.ChannelID, &w.ChannelName, &w.Platform, &w.TokenExpiresAt); err != nil {
			return nil, fmt.Errorf("scan expiring token row: %w", err)
		}
		w.DaysRemaining = int(w.TokenExpiresAt.Sub(now).Hours() / 24)
		warnings = append(warnings, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate expiring tokens: %w", err)
	}

	if warnings == nil {
		warnings = []ChannelTokenWarning{}
	}
	return warnings, nil
}

// ===========================================================================
// Channel status, token refresh dispatch, connection testing
// ===========================================================================

// ChannelStatusResponse holds connection status details for a channel.
type ChannelStatusResponse struct {
	ChannelID        uuid.UUID  `json:"channel_id"`
	ConnectionStatus string     `json:"connection_status"`
	OAuthProvider    string     `json:"oauth_provider,omitempty"`
	TokenExpiresAt   *time.Time `json:"token_expires_at,omitempty"`
	LastTokenRefresh *time.Time `json:"last_token_refresh,omitempty"`
	IsActive         bool       `json:"is_active"`
}

// GetChannelStatus returns the OAuth connection status for a channel.
func (s *ChannelOAuthService) GetChannelStatus(ctx context.Context, channelID uuid.UUID) (*ChannelStatusResponse, error) {
	var resp ChannelStatusResponse
	var oauthProvider *string
	var lastRefresh *time.Time

	err := s.pool.QueryRow(ctx,
		`SELECT id, COALESCE(connection_status, 'disconnected'), oauth_provider,
			token_expires_at, last_token_refresh, is_active
		 FROM channels WHERE id = $1`, channelID,
	).Scan(&resp.ChannelID, &resp.ConnectionStatus, &oauthProvider,
		&resp.TokenExpiresAt, &lastRefresh, &resp.IsActive)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}
	if err != nil {
		return nil, fmt.Errorf("get channel status: %w", err)
	}

	if oauthProvider != nil {
		resp.OAuthProvider = *oauthProvider
	}
	resp.LastTokenRefresh = lastRefresh

	// Update connection_status based on current token state.
	if resp.ConnectionStatus == "connected" && resp.TokenExpiresAt != nil {
		if resp.TokenExpiresAt.Before(time.Now()) {
			resp.ConnectionStatus = "token_expired"
		} else if resp.TokenExpiresAt.Before(time.Now().Add(time.Duration(tokenExpiryWarningDays) * 24 * time.Hour)) {
			resp.ConnectionStatus = "token_expiring"
		}
	}

	return &resp, nil
}

// RefreshChannelToken looks up the channel's oauth_provider and dispatches to
// the correct provider-specific token refresh method.
func (s *ChannelOAuthService) RefreshChannelToken(ctx context.Context, channelID uuid.UUID) error {
	var provider *string
	err := s.pool.QueryRow(ctx,
		`SELECT oauth_provider FROM channels WHERE id = $1`, channelID,
	).Scan(&provider)
	if err == pgx.ErrNoRows {
		return fmt.Errorf("channel not found: %s", channelID)
	}
	if err != nil {
		return fmt.Errorf("get channel provider: %w", err)
	}
	if provider == nil || *provider == "" {
		return fmt.Errorf("channel %s has no oauth provider configured", channelID)
	}

	switch *provider {
	case providerGoogle:
		return s.RefreshGoogleToken(ctx, channelID)
	case providerFacebook:
		return s.RefreshMetaToken(ctx, channelID)
	case providerInstagram:
		// Determine if this Instagram channel was connected via Business Login
		// (scopes contain instagram_business_*) or via the Meta/Facebook flow.
		var scopes []string
		_ = s.pool.QueryRow(ctx, `SELECT COALESCE(scopes, '{}') FROM channels WHERE id = $1`, channelID).Scan(&scopes)
		for _, sc := range scopes {
			if strings.HasPrefix(sc, "instagram_business_") {
				return s.RefreshInstagramBusinessToken(ctx, channelID)
			}
		}
		return s.RefreshMetaToken(ctx, channelID)
	case providerThreads:
		return s.RefreshThreadsToken(ctx, channelID)
	case providerTwitter:
		return s.RefreshTwitterToken(ctx, channelID)
	case providerLinkedIn:
		return s.RefreshLinkedInToken(ctx, channelID)
	default:
		return fmt.Errorf("unsupported oauth provider: %s", *provider)
	}
}

// ConnectionTestResult holds the result of testing a channel's platform
// connection with a lightweight API call.
type ConnectionTestResult struct {
	Connected   bool   `json:"connected"`
	ChannelInfo any    `json:"channel_info,omitempty"`
	Error       string `json:"error,omitempty"`
}

// TestConnection makes a lightweight API call to the platform to verify that
// stored OAuth tokens are still valid.
func (s *ChannelOAuthService) TestConnection(ctx context.Context, channelID uuid.UUID) (*ConnectionTestResult, error) {
	var provider *string
	err := s.pool.QueryRow(ctx,
		`SELECT oauth_provider FROM channels WHERE id = $1`, channelID,
	).Scan(&provider)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}
	if err != nil {
		return nil, fmt.Errorf("get channel provider: %w", err)
	}
	if provider == nil || *provider == "" {
		return &ConnectionTestResult{Connected: false, Error: "channel not connected to any platform"}, nil
	}

	accessToken, _, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return &ConnectionTestResult{Connected: false, Error: "failed to retrieve tokens"}, nil
	}
	if accessToken == "" {
		return &ConnectionTestResult{Connected: false, Error: "no access token stored"}, nil
	}

	switch *provider {
	case providerGoogle:
		return s.testGoogleConnection(ctx, accessToken)
	case providerFacebook:
		return s.testFacebookConnection(ctx, accessToken)
	case providerInstagram:
		return s.testInstagramConnection(ctx, accessToken, channelID)
	case providerThreads:
		return s.testThreadsConnection(ctx, accessToken)
	case providerTwitter:
		return s.testTwitterConnection(ctx, accessToken)
	case providerLinkedIn:
		return s.testLinkedInConnection(ctx, accessToken)
	default:
		return &ConnectionTestResult{Connected: false, Error: "unsupported provider"}, nil
	}
}

func (s *ChannelOAuthService) testGoogleConnection(ctx context.Context, accessToken string) (*ConnectionTestResult, error) {
	var resp struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}

	err := s.doJSONGet(ctx,
		"https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true",
		map[string]string{"Authorization": "Bearer " + accessToken},
		&resp)
	if err != nil {
		return &ConnectionTestResult{Connected: false, Error: err.Error()}, nil
	}

	info := map[string]any{"platform": "youtube"}
	if len(resp.Items) > 0 {
		info["channel_id"] = resp.Items[0].ID
		info["channel_title"] = resp.Items[0].Snippet.Title
	}

	return &ConnectionTestResult{Connected: true, ChannelInfo: info}, nil
}

func (s *ChannelOAuthService) testFacebookConnection(ctx context.Context, accessToken string) (*ConnectionTestResult, error) {
	var resp struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	err := s.doJSONGet(ctx,
		"https://graph.facebook.com/v21.0/me?fields=id,name&access_token="+accessToken,
		nil, &resp)
	if err != nil {
		return &ConnectionTestResult{Connected: false, Error: err.Error()}, nil
	}

	return &ConnectionTestResult{
		Connected:   true,
		ChannelInfo: map[string]any{"platform": "facebook", "page_id": resp.ID, "page_name": resp.Name},
	}, nil
}

func (s *ChannelOAuthService) testInstagramConnection(ctx context.Context, accessToken string, channelID uuid.UUID) (*ConnectionTestResult, error) {
	// Get the Instagram account ID and scopes from the channel record.
	var igAccountID string
	var scopes []string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(channel_id, ''), COALESCE(scopes, '{}') FROM channels WHERE id = $1`, channelID,
	).Scan(&igAccountID, &scopes)
	if err != nil || igAccountID == "" {
		return &ConnectionTestResult{Connected: false, Error: "instagram account id not found"}, nil
	}

	var resp struct {
		ID       string `json:"id"`
		UserID   string `json:"user_id"`
		Username string `json:"username"`
	}

	// Check if this is an Instagram Business Login connection (uses graph.instagram.com).
	isBusinessLogin := false
	for _, sc := range scopes {
		if strings.HasPrefix(sc, "instagram_business_") {
			isBusinessLogin = true
			break
		}
	}

	var igURL string
	if isBusinessLogin {
		igURL = fmt.Sprintf(
			"https://graph.instagram.com/v21.0/me?fields=user_id,username&access_token=%s",
			accessToken)
	} else {
		igURL = fmt.Sprintf(
			"https://graph.facebook.com/v21.0/%s?fields=id,username&access_token=%s",
			igAccountID, accessToken)
	}

	if err := s.doJSONGet(ctx, igURL, nil, &resp); err != nil {
		return &ConnectionTestResult{Connected: false, Error: err.Error()}, nil
	}

	return &ConnectionTestResult{
		Connected:   true,
		ChannelInfo: map[string]any{"platform": "instagram", "ig_id": resp.ID, "username": resp.Username},
	}, nil
}

func (s *ChannelOAuthService) testThreadsConnection(ctx context.Context, accessToken string) (*ConnectionTestResult, error) {
	var resp struct {
		ID       string `json:"id"`
		Username string `json:"username"`
	}

	threadsURL := "https://graph.threads.net/v1.0/me?fields=id,username&access_token=" + accessToken
	if err := s.doJSONGet(ctx, threadsURL, nil, &resp); err != nil {
		return &ConnectionTestResult{Connected: false, Error: err.Error()}, nil
	}

	return &ConnectionTestResult{
		Connected:   true,
		ChannelInfo: map[string]any{"platform": "threads", "threads_id": resp.ID, "username": resp.Username},
	}, nil
}

func (s *ChannelOAuthService) testTwitterConnection(ctx context.Context, accessToken string) (*ConnectionTestResult, error) {
	var resp struct {
		Data struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"data"`
	}

	err := s.doJSONGet(ctx,
		"https://api.x.com/2/users/me",
		map[string]string{"Authorization": "Bearer " + accessToken},
		&resp)
	if err != nil {
		return &ConnectionTestResult{Connected: false, Error: err.Error()}, nil
	}

	return &ConnectionTestResult{
		Connected:   true,
		ChannelInfo: map[string]any{"platform": "twitter", "user_id": resp.Data.ID, "name": resp.Data.Name, "username": resp.Data.Username},
	}, nil
}

func (s *ChannelOAuthService) testLinkedInConnection(ctx context.Context, accessToken string) (*ConnectionTestResult, error) {
	var resp struct {
		Sub  string `json:"sub"`
		Name string `json:"name"`
	}

	err := s.doJSONGet(ctx,
		"https://api.linkedin.com/v2/userinfo",
		map[string]string{"Authorization": "Bearer " + accessToken},
		&resp)
	if err != nil {
		return &ConnectionTestResult{Connected: false, Error: err.Error()}, nil
	}

	return &ConnectionTestResult{
		Connected:   true,
		ChannelInfo: map[string]any{"platform": "linkedin", "member_id": resp.Sub, "name": resp.Name},
	}, nil
}

// ===========================================================================
// Quota status (delegates to QuotaTracker)
// ===========================================================================

// GetQuotaStatus returns the daily quota status for a platform. The handler
// calls this directly; it delegates to the shared QuotaTracker.
func (s *ChannelOAuthService) GetQuotaStatus(ctx context.Context, platform string, limitOverride int) (*QuotaStatus, error) {
	return s.quota.GetQuotaStatus(ctx, platform, limitOverride)
}

// ===========================================================================
// Platform publishing queue
// ===========================================================================

// UploadStatusResponse represents the current state of a platform upload.
type UploadStatusResponse struct {
	PublishRecordID   uuid.UUID  `json:"publish_record_id"`
	Status            string     `json:"status"`
	UploadProgressPct int        `json:"upload_progress_pct"`
	Platform          string     `json:"platform"`
	PlatformURL       string     `json:"platform_url,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
}

// GetUploadStatus queries the publish_records and upload_queue tables for the
// current upload/processing status of a publish record.
func (s *ChannelOAuthService) GetUploadStatus(ctx context.Context, recordID uuid.UUID) (*UploadStatusResponse, error) {
	var resp UploadStatusResponse
	var platformURL, errorMsg *string
	var platform string

	// First get publish record info.
	err := s.pool.QueryRow(ctx,
		`SELECT pr.id, pr.status, COALESCE(pr.upload_progress_pct, 0),
			COALESCE(c.platform, ''), pr.platform_url, pr.error_message,
			pr.upload_started_at, pr.upload_completed_at
		 FROM publish_records pr
		 LEFT JOIN channels c ON c.id = pr.channel_id
		 WHERE pr.id = $1`, recordID,
	).Scan(&resp.PublishRecordID, &resp.Status, &resp.UploadProgressPct,
		&platform, &platformURL, &errorMsg,
		&resp.StartedAt, &resp.CompletedAt)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("publish record not found: %s", recordID)
	}
	if err != nil {
		return nil, fmt.Errorf("get upload status: %w", err)
	}

	resp.Platform = platform
	if platformURL != nil {
		resp.PlatformURL = *platformURL
	}
	if errorMsg != nil {
		resp.ErrorMessage = *errorMsg
	}

	// If there is an upload_queue entry, use its more granular status.
	var queueStatus *string
	var queueStartedAt *time.Time
	var queueCompletedAt *time.Time
	var lastError *string

	err = s.pool.QueryRow(ctx,
		`SELECT status, started_at, completed_at, last_error
		 FROM upload_queue WHERE publish_record_id = $1
		 ORDER BY created_at DESC LIMIT 1`, recordID,
	).Scan(&queueStatus, &queueStartedAt, &queueCompletedAt, &lastError)
	if err == nil && queueStatus != nil {
		// upload_queue statuses are more granular; prefer them.
		resp.Status = *queueStatus
		if queueStartedAt != nil {
			resp.StartedAt = queueStartedAt
		}
		if queueCompletedAt != nil {
			resp.CompletedAt = queueCompletedAt
		}
		if lastError != nil && *lastError != "" {
			resp.ErrorMessage = *lastError
		}
	}

	return &resp, nil
}

// QueuePublish creates a publish_record and an upload_queue entry for the
// background worker to process. This is called by the handler's
// PublishToPlatform endpoint.
func (s *ChannelOAuthService) QueuePublish(ctx context.Context, videoID uuid.UUID, params QueuePublishParams) (*model.PublishRecord, error) {
	// Validate that the channel exists and is connected.
	var connectionStatus string
	var channelPlatform string
	err := s.pool.QueryRow(ctx,
		`SELECT COALESCE(connection_status, 'disconnected'), COALESCE(platform, '')
		 FROM channels WHERE id = $1`, params.ChannelID,
	).Scan(&connectionStatus, &channelPlatform)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("channel not found: %s", params.ChannelID)
	}
	if err != nil {
		return nil, fmt.Errorf("query channel: %w", err)
	}
	if connectionStatus != "connected" {
		return nil, fmt.Errorf("channel is not connected (status: %s)", connectionStatus)
	}

	// Look up the project_id from the video.
	var projectID uuid.UUID
	err = s.pool.QueryRow(ctx,
		`SELECT project_id FROM videos WHERE id = $1`, videoID,
	).Scan(&projectID)
	if err == pgx.ErrNoRows {
		return nil, fmt.Errorf("video not found: %s", videoID)
	}
	if err != nil {
		return nil, fmt.Errorf("query video: %w", err)
	}

	// Determine initial status based on scheduling.
	initialStatus := "pending"
	if params.ScheduledAt != nil {
		initialStatus = "scheduled"
	}

	// Start a transaction to insert both records atomically.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	// Insert publish_record.
	var record model.PublishRecord
	err = tx.QueryRow(ctx,
		`INSERT INTO publish_records (
			project_id, video_id, channel_id,
			title, description, tags, visibility, status, scheduled_at, thumbnail_url
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, project_id, video_id,
			channel_id, COALESCE(platform_content_id, ''), COALESCE(platform_url, ''),
			COALESCE(title, ''), COALESCE(description, ''),
			visibility, status, COALESCE(error_message, ''),
			scheduled_at, published_at, created_at, updated_at`,
		projectID, videoID, params.ChannelID,
		params.Title, params.Description, params.Tags, params.Visibility,
		initialStatus, params.ScheduledAt, params.ThumbnailURL,
	).Scan(
		&record.ID, &record.ProjectID, &record.VideoID,
		&record.ChannelID,
		&record.PlatformContentID, &record.PlatformURL,
		&record.Title, &record.Description,
		&record.Visibility, &record.Status, &record.ErrorMessage,
		&record.ScheduledAt, &record.PublishedAt,
		&record.CreatedAt, &record.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert publish record: %w", err)
	}

	// Insert upload_queue entry for the background worker.
	_, err = tx.Exec(ctx,
		`INSERT INTO upload_queue (
			publish_record_id, channel_id, video_id,
			platform, status, scheduled_at
		) VALUES ($1, $2, $3, $4, $5, $6)`,
		record.ID, params.ChannelID, videoID,
		params.Platform, initialStatus, params.ScheduledAt,
	)
	if err != nil {
		return nil, fmt.Errorf("insert upload queue entry: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &record, nil
}

// QueuePublishParams captures validated publish parameters from the handler.
type QueuePublishParams struct {
	Platform     string
	ChannelID    uuid.UUID
	Title        string
	Description  string
	Tags         []string
	Visibility   string
	ScheduledAt  *time.Time
	ThumbnailURL string
}

// ===========================================================================
// Publish history
// ===========================================================================

// PublishHistoryFilter holds query filters for listing publish history.
type PublishHistoryFilter struct {
	Platform  string
	Status    string
	ChannelID *uuid.UUID
	StartDate *time.Time
	EndDate   *time.Time
	Limit     int
	Offset    int
}

// ListPublishHistory returns a paginated list of publish records with channel
// information, filtered by the provided criteria.
func (s *ChannelOAuthService) ListPublishHistory(ctx context.Context, filter PublishHistoryFilter) ([]model.PublishRecordWithChannel, int, error) {
	limit, offset := ClampPagination(filter.Limit, filter.Offset)

	var args []interface{}
	var conditions []string
	argIdx := 1

	if filter.Platform != "" {
		conditions = append(conditions, fmt.Sprintf("c.platform = $%d", argIdx))
		args = append(args, filter.Platform)
		argIdx++
	}
	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("pr.status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.ChannelID != nil {
		conditions = append(conditions, fmt.Sprintf("pr.channel_id = $%d", argIdx))
		args = append(args, *filter.ChannelID)
		argIdx++
	}
	if filter.StartDate != nil {
		conditions = append(conditions, fmt.Sprintf("pr.created_at >= $%d", argIdx))
		args = append(args, *filter.StartDate)
		argIdx++
	}
	if filter.EndDate != nil {
		conditions = append(conditions, fmt.Sprintf("pr.created_at <= $%d", argIdx))
		args = append(args, *filter.EndDate)
		argIdx++
	}

	where := ""
	if len(conditions) > 0 {
		where = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total.
	var total int
	countQuery := fmt.Sprintf(
		`SELECT COUNT(*) FROM publish_records pr
		 JOIN channels c ON c.id = pr.channel_id%s`, where)
	if err := s.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count publish history: %w", err)
	}

	// Fetch records with channel info.
	query := fmt.Sprintf(
		`SELECT pr.id, pr.project_id,
			pr.channel_id, c.channel_name, c.platform,
			COALESCE(pr.platform_content_id, ''), COALESCE(pr.platform_url, ''),
			COALESCE(pr.title, ''), COALESCE(pr.description, ''),
			pr.visibility, pr.status, COALESCE(pr.error_message, ''),
			pr.published_at, pr.created_at, pr.updated_at
		 FROM publish_records pr
		 JOIN channels c ON c.id = pr.channel_id%s
		 ORDER BY pr.created_at DESC
		 LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("query publish history: %w", err)
	}
	defer rows.Close()

	var records []model.PublishRecordWithChannel
	for rows.Next() {
		var r model.PublishRecordWithChannel
		if err := rows.Scan(
			&r.ID, &r.ProjectID,
			&r.ChannelID, &r.ChannelName, &r.Platform,
			&r.PlatformContentID, &r.PlatformURL,
			&r.Title, &r.Description,
			&r.Visibility, &r.Status, &r.ErrorMessage,
			&r.PublishedAt, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("scan publish history row: %w", err)
		}
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate publish history: %w", err)
	}

	if records == nil {
		records = []model.PublishRecordWithChannel{}
	}

	return records, total, nil
}

// strPtrOrNil maps "" to SQL NULL.
func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
