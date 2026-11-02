package service

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ===========================================================================
// Threads OAuth (separate from Meta)
// ===========================================================================

// GetThreadsOAuthURL generates the Threads OAuth authorization URL. Threads
// uses a separate Meta app (configured via THREADS_CLIENT_ID) with its own OAuth endpoints.
func (s *ChannelOAuthService) GetThreadsOAuthURL(ctx context.Context, channelID uuid.UUID) (string, error) {
	thCfg := s.cfg.OAuth.Threads
	if thCfg.ClientID == "" || thCfg.RedirectURL == "" {
		return "", fmt.Errorf("threads oauth not configured")
	}

	state, err := s.generateState(ctx, channelID)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"client_id":     {thCfg.ClientID},
		"redirect_uri":  {thCfg.RedirectURL},
		"scope":         {"threads_basic,threads_content_publish,threads_manage_replies"},
		"response_type": {"code"},
		"state":         {state},
	}

	return "https://threads.net/oauth/authorize?" + params.Encode(), nil
}

// threadsTokenResponse represents the Threads token endpoint response.
type threadsTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	UserID      string `json:"user_id"`
}

// threadsProfileResponse represents the Threads user profile.
type threadsProfileResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

// HandleThreadsCallback exchanges the authorization code for a short-lived
// token, then exchanges it for a long-lived token, and fetches the Threads
// profile.
func (s *ChannelOAuthService) HandleThreadsCallback(ctx context.Context, code, state string) (*model.Channel, error) {
	channelID, err := s.validateState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("validate threads state: %w", err)
	}

	thCfg := s.cfg.OAuth.Threads

	// Step 1: Exchange code for short-lived token.
	var shortResp threadsTokenResponse
	err = s.doFormPost(ctx, "https://graph.threads.net/oauth/access_token", url.Values{
		"client_id":     {thCfg.ClientID},
		"client_secret": {thCfg.ClientSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {thCfg.RedirectURL},
		"code":          {code},
	}, nil, &shortResp)
	if err != nil {
		return nil, fmt.Errorf("exchange threads auth code: %w", err)
	}

	if shortResp.AccessToken == "" {
		return nil, fmt.Errorf("threads token exchange returned empty access token")
	}

	// Step 2: Exchange short-lived for long-lived token.
	longTokenURL := fmt.Sprintf(
		"https://graph.threads.net/access_token?grant_type=th_exchange_token&client_secret=%s&access_token=%s",
		url.QueryEscape(thCfg.ClientSecret),
		url.QueryEscape(shortResp.AccessToken),
	)

	var longResp threadsTokenResponse
	if err := s.doJSONGet(ctx, longTokenURL, nil, &longResp); err != nil {
		return nil, fmt.Errorf("exchange threads short-lived for long-lived token: %w", err)
	}

	if longResp.AccessToken == "" {
		return nil, fmt.Errorf("threads long-lived token exchange returned empty access token")
	}

	expiresAt := time.Now().Add(60 * 24 * time.Hour)
	if longResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(longResp.ExpiresIn) * time.Second)
	}

	// Step 3: Fetch Threads profile.
	profileURL := "https://graph.threads.net/v1.0/me?fields=id,username&access_token=" + url.QueryEscape(longResp.AccessToken)

	var profile threadsProfileResponse
	threadName := "Threads Account"
	threadID := ""
	threadURL := ""

	if err := s.doJSONGet(ctx, profileURL, nil, &profile); err == nil {
		if profile.Username != "" {
			threadName = "@" + profile.Username
			threadURL = "https://www.threads.net/@" + profile.Username
		}
		threadID = profile.ID
	}

	// Store tokens. Threads long-lived tokens are used directly as access tokens.
	if err := s.storeTokens(ctx, channelID, providerThreads, longResp.AccessToken, longResp.AccessToken, expiresAt); err != nil {
		return nil, err
	}

	scopes := []string{"threads_basic", "threads_content_publish", "threads_manage_replies"}
	if err := s.updateChannelInfo(ctx, channelID, threadName, threadID, threadURL, "threads", scopes); err != nil {
		return nil, err
	}

	return s.getChannel(ctx, channelID)
}

// RefreshThreadsToken refreshes a Threads long-lived token. The token must be
// at least 24 hours old and not yet expired.
func (s *ChannelOAuthService) RefreshThreadsToken(ctx context.Context, channelID uuid.UUID) error {
	accessToken, _, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get threads token: %w", err)
	}
	if accessToken == "" {
		return fmt.Errorf("no threads token stored for channel %s", channelID)
	}

	refreshURL := fmt.Sprintf(
		"https://graph.threads.net/refresh_access_token?grant_type=th_refresh_token&access_token=%s",
		url.QueryEscape(accessToken),
	)

	var tokenResp threadsTokenResponse
	if err := s.doJSONGet(ctx, refreshURL, nil, &tokenResp); err != nil {
		return fmt.Errorf("refresh threads token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("threads token refresh returned empty access token")
	}

	expiresAt := time.Now().Add(60 * 24 * time.Hour)
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return s.storeTokens(ctx, channelID, providerThreads, tokenResp.AccessToken, tokenResp.AccessToken, expiresAt)
}
