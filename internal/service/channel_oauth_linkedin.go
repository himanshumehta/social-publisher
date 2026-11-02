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
// LinkedIn OAuth
// ===========================================================================

// GetLinkedInOAuthURL generates the LinkedIn OAuth 2.0 authorization URL.
func (s *ChannelOAuthService) GetLinkedInOAuthURL(ctx context.Context, channelID uuid.UUID) (string, error) {
	liCfg := s.cfg.OAuth.LinkedIn
	if liCfg.ClientID == "" {
		return "", fmt.Errorf("linkedin oauth not configured")
	}

	state, err := s.generateState(ctx, channelID)
	if err != nil {
		return "", err
	}

	params := url.Values{
		"response_type": {"code"},
		"client_id":     {liCfg.ClientID},
		"redirect_uri":  {liCfg.RedirectURL},
		"scope":         {"openid profile w_member_social w_organization_social"},
		"state":         {state},
	}

	return "https://www.linkedin.com/oauth/v2/authorization?" + params.Encode(), nil
}

// linkedInTokenResponse represents the LinkedIn token endpoint response.
type linkedInTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
}

// linkedInUserInfo represents the LinkedIn userinfo response.
type linkedInUserInfo struct {
	Sub     string `json:"sub"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

// linkedInOrgAclsResponse represents the LinkedIn organization ACLs response.
type linkedInOrgAclsResponse struct {
	Elements []struct {
		Organization string `json:"organization"`
	} `json:"elements"`
}

// linkedInOrgResponse represents a LinkedIn organization lookup response.
type linkedInOrgResponse struct {
	ID            int    `json:"id"`
	LocalizedName string `json:"localizedName"`
	VanityName    string `json:"vanityName"`
}

// HandleLinkedInCallback exchanges the authorization code for tokens, fetches
// user profile and organization info, and stores encrypted tokens.
func (s *ChannelOAuthService) HandleLinkedInCallback(ctx context.Context, code, state string) (*model.Channel, error) {
	channelID, err := s.validateState(ctx, state)
	if err != nil {
		return nil, fmt.Errorf("validate linkedin state: %w", err)
	}

	liCfg := s.cfg.OAuth.LinkedIn

	// Exchange code for tokens.
	var tokenResp linkedInTokenResponse
	err = s.doFormPost(ctx, "https://www.linkedin.com/oauth/v2/accessToken", url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {liCfg.RedirectURL},
		"client_id":     {liCfg.ClientID},
		"client_secret": {liCfg.ClientSecret},
	}, nil, &tokenResp)
	if err != nil {
		return nil, fmt.Errorf("exchange linkedin auth code: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("linkedin token exchange returned empty access token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	authHeaders := map[string]string{
		"Authorization":             "Bearer " + tokenResp.AccessToken,
		"Linkedin-Version":          "202601",
		"X-Restli-Protocol-Version": "2.0.0",
	}

	// Fetch user profile via OpenID userinfo.
	var userInfo linkedInUserInfo
	err = s.doJSONGet(ctx, "https://api.linkedin.com/v2/userinfo",
		map[string]string{"Authorization": "Bearer " + tokenResp.AccessToken},
		&userInfo)
	if err != nil {
		return nil, fmt.Errorf("fetch linkedin user profile: %w", err)
	}

	profileName := userInfo.Name
	if profileName == "" {
		profileName = "LinkedIn Account"
	}
	profileID := userInfo.Sub
	profileURL := "https://www.linkedin.com/in/" + profileID

	// Try to get organization info (for company pages).
	orgAclsURL := "https://api.linkedin.com/rest/organizationAcls?q=roleAssignee&role=ADMINISTRATOR&state=APPROVED"
	var orgAcls linkedInOrgAclsResponse
	if err := s.doJSONGet(ctx, orgAclsURL, authHeaders, &orgAcls); err == nil && len(orgAcls.Elements) > 0 {
		// Extract org URN from the first element. The organization field is a URN
		// like "urn:li:organization:12345".
		orgURN := orgAcls.Elements[0].Organization
		if orgURN != "" {
			// Try to fetch org details.
			orgID := extractLinkedInOrgID(orgURN)
			if orgID != "" {
				orgDetailURL := fmt.Sprintf("https://api.linkedin.com/rest/organizations/%s", orgID)
				var orgInfo linkedInOrgResponse
				if err := s.doJSONGet(ctx, orgDetailURL, authHeaders, &orgInfo); err == nil {
					if orgInfo.LocalizedName != "" {
						profileName = orgInfo.LocalizedName
					}
					if orgInfo.VanityName != "" {
						profileURL = "https://www.linkedin.com/company/" + orgInfo.VanityName
					}
					profileID = orgURN
				}
			}
		}
	}

	scopes := strings.Split(tokenResp.Scope, " ")
	if len(scopes) == 0 || (len(scopes) == 1 && scopes[0] == "") {
		scopes = []string{"openid", "profile", "w_member_social"}
	}

	// Store tokens. LinkedIn may not return a refresh token — fall back to access token.
	refreshToken := tokenResp.RefreshToken
	if refreshToken == "" {
		refreshToken = tokenResp.AccessToken
	}
	if err := s.storeTokens(ctx, channelID, providerLinkedIn, tokenResp.AccessToken, refreshToken, expiresAt); err != nil {
		return nil, err
	}

	if err := s.updateChannelInfo(ctx, channelID, profileName, profileID, profileURL, "linkedin", scopes); err != nil {
		return nil, err
	}

	return s.getChannel(ctx, channelID)
}

// RefreshLinkedInToken uses the stored refresh token to obtain a new access
// token from LinkedIn.
func (s *ChannelOAuthService) RefreshLinkedInToken(ctx context.Context, channelID uuid.UUID) error {
	_, refreshToken, err := s.getDecryptedTokens(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get linkedin tokens: %w", err)
	}
	if refreshToken == "" {
		return fmt.Errorf("no refresh token stored for channel %s", channelID)
	}

	liCfg := s.cfg.OAuth.LinkedIn

	var tokenResp linkedInTokenResponse
	err = s.doFormPost(ctx, "https://www.linkedin.com/oauth/v2/accessToken", url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {liCfg.ClientID},
		"client_secret": {liCfg.ClientSecret},
	}, nil, &tokenResp)
	if err != nil {
		return fmt.Errorf("refresh linkedin token: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return fmt.Errorf("linkedin token refresh returned empty access token")
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	newRefresh := refreshToken
	if tokenResp.RefreshToken != "" {
		newRefresh = tokenResp.RefreshToken
	}

	return s.storeTokens(ctx, channelID, providerLinkedIn, tokenResp.AccessToken, newRefresh, expiresAt)
}

// extractLinkedInOrgID extracts the numeric organization ID from a LinkedIn
// URN like "urn:li:organization:12345".
func extractLinkedInOrgID(urn string) string {
	parts := strings.Split(urn, ":")
	if len(parts) < 4 {
		return ""
	}
	return parts[3]
}
