package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ===========================================================================
// 2b. Instagram Business Login (direct Instagram OAuth, separate from Meta flow)
// ===========================================================================

var igBusinessScopes = []string{
	"instagram_business_basic",
	"instagram_business_content_publish",
	"instagram_business_manage_comments",
}

// GetInstagramBusinessLoginURL generates the Instagram OAuth authorization URL
// using the Instagram Business Login flow (not the Facebook Login flow).
func (s *ChannelOAuthService) GetInstagramBusinessLoginURL(ctx context.Context, channelID uuid.UUID) (string, error) {
	igCfg := s.cfg.OAuth.Instagram
	if igCfg.ClientID == "" {
		return "", fmt.Errorf("instagram oauth not configured")
	}

	state, err := s.generateState(ctx, channelID)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"client_id":     {igCfg.ClientID},
		"redirect_uri":  {igCfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(igBusinessScopes, ",")},
		"state":         {state},
	}

	return "https://www.instagram.com/oauth/authorize?" + params.Encode(), nil
}

// igBusinessTokenResponse represents the Instagram token endpoint response.
type igBusinessTokenResponse struct {
	AccessToken string `json:"access_token"`
	UserID      int64  `json:"user_id"`
}

// igBusinessLongTokenResponse represents the long-lived token exchange response.
type igBusinessLongTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// igBusinessProfileResponse represents the Instagram user profile.
type igBusinessProfileResponse struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// HandleInstagramBusinessLoginCallback exchanges the code for tokens using the
// Instagram Business Login flow, obtains a long-lived token, and updates the channel.
func (s *ChannelOAuthService) HandleInstagramBusinessLoginCallback(ctx context.Context, code, state string) (*model.Channel, error) {
	channelID, err := s.validateState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("validate instagram state: %w", err)
	}

	igCfg := s.cfg.OAuth.Instagram

	// Step 1: Exchange code for short-lived access token (form-encoded POST).
	formData := url.Values{
		"client_id":     {igCfg.ClientID},
		"client_secret": {igCfg.ClientSecret},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {igCfg.RedirectURL},
		"code":          {code},
	}

	resp, err := s.http.PostForm("https://api.instagram.com/oauth/access_token", formData)
	if err != nil {
		return nil, fmt.Errorf("exchange instagram code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("instagram token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var shortResp igBusinessTokenResponse
	if err := json.Unmarshal(body, &shortResp); err != nil {
		return nil, fmt.Errorf("parse instagram short token: %w", err)
	}

	if shortResp.AccessToken == "" {
		return nil, fmt.Errorf("instagram token exchange returned empty access token")
	}

	// Step 2: Exchange short-lived token for long-lived token (60 days).
	// Meta changed this endpoint to require POST (previously GET).
	longTokenForm := url.Values{
		"grant_type":    {"ig_exchange_token"},
		"client_secret": {igCfg.ClientSecret},
		"access_token":  {shortResp.AccessToken},
	}
	var longResp igBusinessLongTokenResponse
	if err := s.doFormPost(ctx, "https://graph.instagram.com/v21.0/access_token", longTokenForm, nil, &longResp); err != nil {
		return nil, fmt.Errorf("exchange instagram long-lived token: %w", err)
	}

	if longResp.AccessToken == "" {
		return nil, fmt.Errorf("instagram long-lived token exchange returned empty access token")
	}

	expiresAt := time.Now().Add(60 * 24 * time.Hour)
	if longResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(longResp.ExpiresIn) * time.Second)
	}

	// Step 3: Get user profile.
	profileURL := fmt.Sprintf(
		"https://graph.instagram.com/v21.0/me?fields=user_id,username,name&access_token=%s",
		url.QueryEscape(longResp.AccessToken),
	)

	var profile igBusinessProfileResponse
	igName := "Instagram Account"
	igUsername := ""
	igUserID := fmt.Sprintf("%d", shortResp.UserID)
	if err := s.doJSONGet(ctx, profileURL, nil, &profile); err == nil {
		if profile.Name != "" {
			igName = profile.Name
		}
		if profile.Username != "" {
			igUsername = profile.Username
		}
		if profile.UserID != "" {
			igUserID = profile.UserID
		}
	}

	igURL := ""
	if igUsername != "" {
		igURL = "https://www.instagram.com/" + igUsername
	}

	// Store tokens and update channel info.
	if err := s.storeTokens(ctx, channelID, providerInstagram, longResp.AccessToken, longResp.AccessToken, expiresAt); err != nil {
		return nil, fmt.Errorf("store instagram tokens: %w", err)
	}

	if err := s.updateChannelInfo(ctx, channelID, igName, igUserID, igURL, "instagram", igBusinessScopes); err != nil {
		return nil, fmt.Errorf("update instagram channel info: %w", err)
	}

	return s.getChannel(ctx, channelID)
}

// RefreshInstagramBusinessToken refreshes a long-lived Instagram Business Login token.
func (s *ChannelOAuthService) RefreshInstagramBusinessToken(ctx context.Context, channelID uuid.UUID) error {
	accessToken, _, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get instagram tokens: %w", err)
	}

	// Meta changed this endpoint to require POST (previously GET).
	refreshForm := url.Values{
		"grant_type":   {"ig_refresh_token"},
		"access_token": {accessToken},
	}
	var tokenResp igBusinessLongTokenResponse
	if err := s.doFormPost(ctx, "https://graph.instagram.com/v21.0/refresh_access_token", refreshForm, nil, &tokenResp); err != nil {
		return fmt.Errorf("refresh instagram business token: %w", err)
	}

	expiresAt := time.Now().Add(60 * 24 * time.Hour)
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return s.storeTokens(ctx, channelID, providerInstagram, tokenResp.AccessToken, tokenResp.AccessToken, expiresAt)
}
