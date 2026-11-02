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
// Meta OAuth (Facebook + Instagram)
// ===========================================================================

var metaScopes = []string{
	"instagram_basic",
	"instagram_content_publish",
	"pages_read_engagement",
	"pages_manage_posts",
	"publish_video",
}

// GetMetaOAuthURL generates the Facebook OAuth dialog URL for connecting both
// Facebook Page and Instagram Business accounts.
func (s *ChannelOAuthService) GetMetaOAuthURL(ctx context.Context, channelID uuid.UUID) (string, error) {
	fbCfg := s.cfg.OAuth.Facebook
	if fbCfg.ClientID == "" {
		return "", fmt.Errorf("facebook/meta oauth not configured")
	}

	state, err := s.generateState(ctx, channelID)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"client_id":    {fbCfg.ClientID},
		"redirect_uri": {fbCfg.RedirectURL},
		"scope":        {strings.Join(metaScopes, ",")},
		"state":        {state},
	}

	return "https://www.facebook.com/v21.0/dialog/oauth?" + params.Encode(), nil
}

// metaTokenResponse represents the Facebook/Meta token endpoint response.
type metaTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// metaPagesResponse represents the /me/accounts response.
type metaPagesResponse struct {
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

// metaIGResponse represents the Instagram business account lookup response.
type metaIGResponse struct {
	InstagramBusinessAccount struct {
		ID string `json:"id"`
	} `json:"instagram_business_account"`
}

// metaIGProfileResponse represents the Instagram user profile.
type metaIGProfileResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// HandleMetaCallback exchanges the code for tokens, obtains a long-lived
// token, retrieves the Facebook Page and associated Instagram Business
// account, and returns up to 2 channel records (Facebook + Instagram).
func (s *ChannelOAuthService) HandleMetaCallback(ctx context.Context, code, state string) ([]*model.Channel, error) {
	channelID, err := s.validateState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("validate meta state: %w", err)
	}

	fbCfg := s.cfg.OAuth.Facebook

	// Step 1: Exchange code for short-lived user access token.
	shortTokenURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/oauth/access_token?client_id=%s&redirect_uri=%s&client_secret=%s&code=%s",
		url.QueryEscape(fbCfg.ClientID),
		url.QueryEscape(fbCfg.RedirectURL),
		url.QueryEscape(fbCfg.ClientSecret),
		url.QueryEscape(code),
	)

	var shortResp metaTokenResponse
	if err := s.doJSONGet(ctx, shortTokenURL, nil, &shortResp); err != nil {
		return nil, fmt.Errorf("exchange meta auth code for short-lived token: %w", err)
	}

	// Step 2: Exchange short-lived token for long-lived token.
	longTokenURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		url.QueryEscape(fbCfg.ClientID),
		url.QueryEscape(fbCfg.ClientSecret),
		url.QueryEscape(shortResp.AccessToken),
	)

	var longResp metaTokenResponse
	if err := s.doJSONGet(ctx, longTokenURL, nil, &longResp); err != nil {
		return nil, fmt.Errorf("exchange meta short-lived for long-lived token: %w", err)
	}

	if longResp.AccessToken == "" {
		return nil, fmt.Errorf("meta long-lived token exchange returned empty access token")
	}

	// Long-lived tokens last ~60 days.
	expiresAt := time.Now().Add(60 * 24 * time.Hour)
	if longResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(longResp.ExpiresIn) * time.Second)
	}

	// Step 3: Get user's Facebook pages.
	var pagesResp metaPagesResponse
	pagesURL := "https://graph.facebook.com/v21.0/me/accounts?access_token=" + url.QueryEscape(longResp.AccessToken)
	if err := s.doJSONGet(ctx, pagesURL, nil, &pagesResp); err != nil {
		return nil, fmt.Errorf("fetch meta pages: %w", err)
	}

	if len(pagesResp.Data) == 0 {
		return nil, fmt.Errorf("no facebook pages found for this account")
	}

	// Use the first page.
	page := pagesResp.Data[0]

	var channels []*model.Channel

	// -- Facebook Page channel --
	if err := s.storeTokens(ctx, channelID, providerFacebook, page.AccessToken, longResp.AccessToken, expiresAt); err != nil {
		return nil, fmt.Errorf("store facebook tokens: %w", err)
	}

	pageURL := "https://www.facebook.com/" + page.ID
	if err := s.updateChannelInfo(ctx, channelID, page.Name, page.ID, pageURL, "facebook", metaScopes); err != nil {
		return nil, fmt.Errorf("update facebook channel info: %w", err)
	}

	fbChannel, err := s.getChannel(ctx, channelID)
	if err != nil {
		return nil, err
	}
	channels = append(channels, fbChannel)

	// Step 4: Get Instagram Business Account linked to the page.
	igURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/%s?fields=instagram_business_account&access_token=%s",
		page.ID,
		url.QueryEscape(page.AccessToken),
	)

	var igResp metaIGResponse
	if err := s.doJSONGet(ctx, igURL, nil, &igResp); err != nil {
		// Instagram may not be connected; return just the Facebook channel.
		return channels, nil
	}

	if igResp.InstagramBusinessAccount.ID == "" {
		// No Instagram business account linked.
		return channels, nil
	}

	igAccountID := igResp.InstagramBusinessAccount.ID

	// Fetch Instagram profile info.
	igProfileURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/%s?fields=id,username,name&access_token=%s",
		igAccountID,
		url.QueryEscape(page.AccessToken),
	)

	var igProfile metaIGProfileResponse
	igName := "Instagram Account"
	igUsername := ""
	if err := s.doJSONGet(ctx, igProfileURL, nil, &igProfile); err == nil {
		if igProfile.Name != "" {
			igName = igProfile.Name
		}
		igUsername = igProfile.Username
	}

	// Create a separate channel record for Instagram.
	var igChannelID uuid.UUID
	var igCategoryID uuid.UUID

	// Get category_id and user_id from the parent channel to create the IG channel in the same context.
	var igUserID *uuid.UUID
	var igChannelType string
	err = s.pool.QueryRow(ctx, `SELECT category_id, user_id, channel_type FROM channels WHERE id = $1`, channelID).Scan(&igCategoryID, &igUserID, &igChannelType)
	if err != nil {
		return channels, fmt.Errorf("get category for ig channel: %w", err)
	}

	err = s.pool.QueryRow(ctx,
		`INSERT INTO channels (category_id, user_id, channel_type, platform, channel_name, channel_id, channel_url)
		 VALUES ($1, $2, $3, 'instagram', $4, $5, $6)
		 RETURNING id`,
		igCategoryID, igUserID, igChannelType, igName, igAccountID, "https://www.instagram.com/"+igUsername,
	).Scan(&igChannelID)
	if err != nil {
		return channels, fmt.Errorf("create instagram channel: %w", err)
	}

	// Store tokens for the Instagram channel (uses the page access token).
	if err := s.storeTokens(ctx, igChannelID, providerInstagram, page.AccessToken, longResp.AccessToken, expiresAt); err != nil {
		return channels, fmt.Errorf("store instagram tokens: %w", err)
	}

	igChannel, err := s.getChannel(ctx, igChannelID)
	if err != nil {
		return channels, err
	}
	channels = append(channels, igChannel)

	return channels, nil
}

// RefreshMetaToken exchanges a long-lived token for a new long-lived token.
// Meta long-lived tokens can be refreshed once they are at least 24 hours old
// but before they expire (60 days). We recommend refreshing at ~50 days.
func (s *ChannelOAuthService) RefreshMetaToken(ctx context.Context, channelID uuid.UUID) error {
	_, refreshToken, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get meta tokens: %w", err)
	}

	// For Meta, the "refresh_token" field stores the user-level long-lived token.
	tokenToRefresh := refreshToken
	if tokenToRefresh == "" {
		return fmt.Errorf("no long-lived token stored for channel %s", channelID)
	}

	fbCfg := s.cfg.OAuth.Facebook

	refreshURL := fmt.Sprintf(
		"https://graph.facebook.com/v21.0/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		url.QueryEscape(fbCfg.ClientID),
		url.QueryEscape(fbCfg.ClientSecret),
		url.QueryEscape(tokenToRefresh),
	)

	var tokenResp metaTokenResponse
	if err := s.doJSONGet(ctx, refreshURL, nil, &tokenResp); err != nil {
		return fmt.Errorf("refresh meta long-lived token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("meta token refresh returned empty access token")
	}

	expiresAt := time.Now().Add(60 * 24 * time.Hour)
	if tokenResp.ExpiresIn > 0 {
		expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	// Determine provider from stored data.
	var provider string
	err = s.pool.QueryRow(ctx, `SELECT COALESCE(oauth_provider, 'facebook') FROM channels WHERE id = $1`, channelID).Scan(&provider)
	if err != nil {
		provider = providerFacebook
	}

	return s.storeTokens(ctx, channelID, provider, tokenResp.AccessToken, tokenResp.AccessToken, expiresAt)
}
