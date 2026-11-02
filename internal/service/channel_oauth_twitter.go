package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ===========================================================================
// Twitter/X OAuth 2.0 with PKCE
// ===========================================================================

var twitterScopes = []string{
	"tweet.write",
	"tweet.read",
	"users.read",
	"media.write",
	"offline.access",
}

// GetTwitterOAuthURL generates the Twitter OAuth 2.0 authorization URL with
// PKCE (S256 code challenge).
func (s *ChannelOAuthService) GetTwitterOAuthURL(ctx context.Context, channelID uuid.UUID) (string, error) {
	twCfg := s.cfg.OAuth.Twitter
	if twCfg.ClientID == "" {
		return "", fmt.Errorf("twitter oauth not configured")
	}

	state, err := s.generateState(ctx, channelID)
	if err != nil {
		return "", err
	}

	codeChallenge, err := s.generatePKCE(ctx, state)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"response_type":         {"code"},
		"client_id":             {twCfg.ClientID},
		"redirect_uri":          {twCfg.RedirectURL},
		"scope":                 {strings.Join(twitterScopes, " ")},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},
	}

	return "https://x.com/i/oauth2/authorize?" + params.Encode(), nil
}

// twitterTokenResponse represents the Twitter token endpoint response.
type twitterTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// twitterUserResponse represents the Twitter users/me response.
type twitterUserResponse struct {
	Data struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Username string `json:"username"`
	} `json:"data"`
}

// HandleTwitterCallback exchanges the authorization code (with PKCE verifier)
// for tokens and fetches the user profile.
func (s *ChannelOAuthService) HandleTwitterCallback(ctx context.Context, code, state string) (*model.Channel, error) {
	channelID, err := s.validateState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("validate twitter state: %w", err)
	}

	codeVerifier, err := s.consumePKCEVerifier(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("get twitter pkce verifier: %w", err)
	}

	twCfg := s.cfg.OAuth.Twitter

	// Exchange code for tokens using Basic auth (client_id:client_secret).
	var tokenResp twitterTokenResponse
	err = s.doFormPost(ctx, "https://api.x.com/2/oauth2/token", url.Values{
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {twCfg.RedirectURL},
		"code_verifier": {codeVerifier},
		"client_id":     {twCfg.ClientID},
	}, map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString(
			[]byte(twCfg.ClientID+":"+twCfg.ClientSecret)),
	}, &tokenResp)
	if err != nil {
		return nil, fmt.Errorf("exchange twitter auth code: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("twitter token exchange returned empty access token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// Fetch user profile.
	var userResp twitterUserResponse
	err = s.doJSONGet(ctx,
		"https://api.x.com/2/users/me",
		map[string]string{"Authorization": "Bearer " + tokenResp.AccessToken},
		&userResp)
	if err != nil {
		return nil, fmt.Errorf("fetch twitter user profile: %w", err)
	}

	userName := userResp.Data.Name
	if userName == "" {
		userName = "X Account"
	}
	userID := userResp.Data.ID
	userURL := ""
	if userResp.Data.Username != "" {
		userURL = "https://x.com/" + userResp.Data.Username
	}

	// Store tokens. Twitter refresh tokens are single-use.
	if err := s.storeTokens(ctx, channelID, providerTwitter, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt); err != nil {
		return nil, err
	}

	if err := s.updateChannelInfo(ctx, channelID, userName, userID, userURL, "twitter", twitterScopes); err != nil {
		return nil, err
	}

	return s.getChannel(ctx, channelID)
}

// RefreshTwitterToken uses the stored single-use refresh token to obtain new
// access and refresh tokens. The new refresh token MUST be stored because
// Twitter refresh tokens are single-use.
func (s *ChannelOAuthService) RefreshTwitterToken(ctx context.Context, channelID uuid.UUID) error {
	_, refreshToken, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get twitter tokens: %w", err)
	}
	if refreshToken == "" {
		return fmt.Errorf("no refresh token stored for channel %s", channelID)
	}

	twCfg := s.cfg.OAuth.Twitter

	var tokenResp twitterTokenResponse
	err = s.doFormPost(ctx, "https://api.x.com/2/oauth2/token", url.Values{
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
		"client_id":     {twCfg.ClientID},
	}, map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString(
			[]byte(twCfg.ClientID+":"+twCfg.ClientSecret)),
	}, &tokenResp)
	if err != nil {
		return fmt.Errorf("refresh twitter token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("twitter token refresh returned empty access token")
	}

	// Twitter issues a NEW single-use refresh token with each refresh.
	if tokenResp.RefreshToken == "" {
		return fmt.Errorf("twitter token refresh did not return a new refresh token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return s.storeTokens(ctx, channelID, providerTwitter, tokenResp.AccessToken, tokenResp.RefreshToken, expiresAt)
}
