package service

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ===========================================================================
// 1. Google / YouTube OAuth
// ===========================================================================

var googleScopes = []string{
	googleScopeYouTubeUpload,
	googleScopeYouTube,
	googleScopeYouTubeReadonly,
	googleScopeYTAnalytics,
}

// GetGoogleOAuthURL generates the Google OAuth 2.0 authorization URL for
// YouTube channel connection.
func (s *ChannelOAuthService) GetGoogleOAuthURL(ctx context.Context, channelID uuid.UUID) (string, error) {
	gcfg := s.cfg.OAuth.Google
	if gcfg.ClientID == "" {
		return "", fmt.Errorf("google oauth not configured")
	}

	state, err := s.generateState(ctx, channelID)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"client_id":     {gcfg.ClientID},
		"redirect_uri":  {gcfg.RedirectURL},
		"response_type": {"code"},
		"scope":         {strings.Join(googleScopes, " ")},
		"access_type":   {"offline"},
		"prompt":        {"consent"},
		"state":         {state},
	}

	return "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode(), nil
}

// googleTokenResponse represents the response from Google's token endpoint.
type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// youtubeChannelsResponse represents the YouTube Data API channels.list response.
type youtubeChannelsResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title       string `json:"title"`
			CustomURL   string `json:"customUrl"`
			Description string `json:"description"`
		} `json:"snippet"`
	} `json:"items"`
}

// HandleGoogleCallback exchanges the authorization code for tokens, fetches
// the YouTube channel info, and stores encrypted tokens.
func (s *ChannelOAuthService) HandleGoogleCallback(ctx context.Context, code, state string) (*model.Channel, error) {
	channelID, err := s.validateState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("validate google state: %w", err)
	}

	gcfg := s.cfg.OAuth.Google

	// Exchange authorization code for tokens.
	var tokenResp googleTokenResponse
	err = s.doFormPost(ctx, "https://oauth2.googleapis.com/token", url.Values{
		"code":          {code},
		"client_id":     {gcfg.ClientID},
		"client_secret": {gcfg.ClientSecret},
		"redirect_uri":  {gcfg.RedirectURL},
		"grant_type":    {"authorization_code"},
	}, nil, &tokenResp)
	if err != nil {
		return nil, fmt.Errorf("exchange google auth code: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("google token exchange returned empty access token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Fetch YouTube channel info.
	var ytResp youtubeChannelsResponse
	err = s.doJSONGet(ctx,
		"https://www.googleapis.com/youtube/v3/channels?part=snippet&mine=true",
		map[string]string{"Authorization": "Bearer " + tokenResp.AccessToken},
		&ytResp)
	if err != nil {
		return nil, fmt.Errorf("fetch youtube channel info: %w", err)
	}

	channelName := "YouTube Channel"
	channelExtID := ""
	channelURL := ""
	if len(ytResp.Items) > 0 {
		item := ytResp.Items[0]
		channelName = item.Snippet.Title
		channelExtID = item.ID
		if item.Snippet.CustomURL != "" {
			channelURL = "https://www.youtube.com/" + item.Snippet.CustomURL
		} else {
			channelURL = "https://www.youtube.com/channel/" + item.ID
		}
	}

	// Store encrypted tokens.
	if err := s.storeTokens(ctx, channelID, providerGoogle, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt); err != nil {
		return nil, err
	}

	// Update channel metadata.
	if err := s.updateChannelInfo(ctx, channelID, channelName, channelExtID, channelURL, "youtube", googleScopes); err != nil {
		return nil, err
	}

	return s.getChannel(ctx, channelID)
}

// RefreshGoogleToken uses the stored refresh token to obtain a new access
// token from Google.
func (s *ChannelOAuthService) RefreshGoogleToken(ctx context.Context, channelID uuid.UUID) error {
	_, refreshToken, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get google refresh token: %w", err)
	}
	if refreshToken == "" {
		return fmt.Errorf("no refresh token stored for channel %s", channelID)
	}

	gcfg := s.cfg.OAuth.Google

	var tokenResp googleTokenResponse
	err = s.doFormPost(ctx, "https://oauth2.googleapis.com/token", url.Values{
		"client_id":     {gcfg.ClientID},
		"client_secret": {gcfg.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}, nil, &tokenResp)
	if err != nil {
		return fmt.Errorf("refresh google token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("google token refresh returned empty access token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Google does not always return a new refresh token on refresh; keep the existing one.
	newRefresh := refreshToken
	if tokenResp.RefreshToken != "" {
		newRefresh = tokenResp.RefreshToken
	}

	return s.storeTokens(ctx, channelID, providerGoogle, tokenResp.AccessToken, newRefresh, expiresAt)
}
