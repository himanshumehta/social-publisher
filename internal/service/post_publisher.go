package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// Rate limiter and circuit breaker configuration.
const (
	// publishHTTPTimeout is the HTTP client timeout for outbound platform API calls.
	publishHTTPTimeout = 30 * time.Second

	// Rate limits per platform (requests per window).
	rateLimitTwitter   = 300 // per 3 minutes
	rateLimitThreads   = 250 // per day
	rateLimitFacebook  = 200 // per hour
	rateLimitInstagram = 25  // per day
	rateLimitLinkedIn  = 100 // per day

	// Circuit breaker settings.
	circuitBreakerThreshold = 5
	circuitBreakerReset     = 5 * time.Minute

	// Maximum concurrent platform publishes.
	maxConcurrentPublishes = 3
)

type PostPublisher struct {
	pool         *pgxpool.Pool
	logger       *slog.Logger
	tokenCrypto  *TokenCrypto
	httpClient   *http.Client
	rateLimiters map[string]*rate.Limiter
	breakers     map[string]*circuitBreaker
	adapters     map[string]PlatformAdapter
	dryRun       bool
}

type circuitBreaker struct {
	mu         sync.Mutex
	failures   int
	threshold  int
	lastFail   time.Time
	resetAfter time.Duration
}

func (cb *circuitBreaker) isOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.failures >= cb.threshold {
		if time.Since(cb.lastFail) > cb.resetAfter {
			cb.failures = cb.threshold - 1
			return false
		}
		return true
	}
	return false
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFail = time.Now()
}

func NewPostPublisher(
	pool *pgxpool.Pool,
	logger *slog.Logger,
	tokenCrypto *TokenCrypto,
) *PostPublisher {
	pp := &PostPublisher{
		pool:        pool,
		logger:      logger,
		tokenCrypto: tokenCrypto,
		httpClient:  &http.Client{Timeout: publishHTTPTimeout},
		rateLimiters: map[string]*rate.Limiter{
			"twitter":   rate.NewLimiter(rate.Every(time.Minute*3/rateLimitTwitter), 5),
			"threads":   rate.NewLimiter(rate.Every(24*time.Hour/rateLimitThreads), 5),
			"facebook":  rate.NewLimiter(rate.Every(time.Hour/rateLimitFacebook), 10),
			"instagram": rate.NewLimiter(rate.Every(24*time.Hour/rateLimitInstagram), 2),
			"linkedin":  rate.NewLimiter(rate.Every(24*time.Hour/rateLimitLinkedIn), 5),
		},
		breakers: map[string]*circuitBreaker{
			"twitter":   {threshold: circuitBreakerThreshold, resetAfter: circuitBreakerReset},
			"threads":   {threshold: circuitBreakerThreshold, resetAfter: circuitBreakerReset},
			"facebook":  {threshold: circuitBreakerThreshold, resetAfter: circuitBreakerReset},
			"instagram": {threshold: circuitBreakerThreshold, resetAfter: circuitBreakerReset},
			"linkedin":  {threshold: circuitBreakerThreshold, resetAfter: circuitBreakerReset},
		},
	}
	pp.adapters = map[string]PlatformAdapter{
		"twitter":   PlatformAdapterFunc(pp.publishToTwitter),
		"threads":   PlatformAdapterFunc(pp.publishToThreads),
		"facebook":  PlatformAdapterFunc(pp.publishToFacebook),
		"instagram": PlatformAdapterFunc(pp.publishToInstagram),
		"linkedin":  PlatformAdapterFunc(pp.publishToLinkedIn),
	}
	return pp
}

// EnableDryRun swaps every platform adapter for a DryRunAdapter. Channel
// resolution, token decryption, rate limiting and circuit breaking are skipped,
// so no credentials or connected channels are required.
func (pp *PostPublisher) EnableDryRun() {
	pp.dryRun = true
	pp.adapters = dryRunAdapters(pp.logger, SupportedPostPlatforms)
	pp.logger.Info("post publisher in DRY-RUN mode: no platform APIs will be called")
}

func (pp *PostPublisher) Publish(ctx context.Context, post *model.Post) ([]model.PostPublishResult, error) {
	if len(post.PlatformPosts) == 0 {
		return nil, fmt.Errorf("no platform posts to publish")
	}

	channels, err := pp.resolveChannels(ctx, post)
	if err != nil {
		return nil, fmt.Errorf("resolving channels: %w", err)
	}

	// Load already-published platforms for idempotency on retries
	alreadyPublished := pp.getAlreadyPublished(ctx, post.ID)

	var (
		mu      sync.Mutex
		results []model.PostPublishResult
	)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrentPublishes)

	for _, platform := range post.TargetPlatforms {
		// Skip platforms already published successfully (idempotency)
		if prev, ok := alreadyPublished[platform]; ok {
			pp.logger.Info("skipping already published platform",
				"post_id", post.ID,
				"platform", platform,
				"platform_url", prev.PlatformURL,
			)
			mu.Lock()
			results = append(results, prev)
			mu.Unlock()
			continue
		}

		platformPost, ok := post.PlatformPosts[platform]
		if !ok {
			continue
		}

		channel, ok := channels[platform]
		if !ok && pp.dryRun {
			// Dry-run needs no connected channel; use a placeholder.
			channel, ok = &model.Channel{Platform: platform, ChannelName: "dry-run"}, true
		}
		if !ok {
			mu.Lock()
			results = append(results, model.PostPublishResult{
				Platform: platform,
				Status:   "failed",
				Error:    "no channel configured for platform",
			})
			mu.Unlock()
			continue
		}

		g.Go(func() error {
			result := pp.publishToPlatform(gctx, post, platformPost, channel)

			mu.Lock()
			results = append(results, result)
			mu.Unlock()

			pp.recordPublishLink(gctx, post.ID, result, channel.ID)

			return nil
		})
	}

	g.Wait()

	return results, nil
}

// getAlreadyPublished checks post_publish_links for platforms already published successfully.
// Used for idempotency on retries to avoid duplicate posts on platforms.
func (pp *PostPublisher) getAlreadyPublished(ctx context.Context, postID uuid.UUID) map[string]model.PostPublishResult {
	result := make(map[string]model.PostPublishResult)

	rows, err := pp.pool.Query(ctx, `
		SELECT platform, platform_post_id, platform_url, channel_id, published_at
		FROM post_publish_links
		WHERE post_id = $1 AND status = 'published'`,
		postID,
	)
	if err != nil {
		pp.logger.Warn("failed to check existing publish links", "post_id", postID, "error", err)
		return result
	}
	defer rows.Close()

	for rows.Next() {
		var platform string
		var platformPostID, platformURL *string
		var channelID *uuid.UUID
		var publishedAt *time.Time
		if err := rows.Scan(&platform, &platformPostID, &platformURL, &channelID, &publishedAt); err != nil {
			continue
		}
		var channelIDStr string
		if channelID != nil {
			channelIDStr = channelID.String()
		}
		result[platform] = model.PostPublishResult{
			Platform:       platform,
			PlatformPostID: derefStr(platformPostID),
			PlatformURL:    derefStr(platformURL),
			ChannelID:      channelIDStr,
			Status:         "published",
			PublishedAt:    publishedAt,
		}
	}

	return result
}

func (pp *PostPublisher) publishToPlatform(ctx context.Context, post *model.Post, platformPost *model.PlatformPost, channel *model.Channel) model.PostPublishResult {
	platform := platformPost.Platform
	result := model.PostPublishResult{
		Platform:    platform,
		ChannelName: channel.ChannelName,
		Status:      "pending",
	}
	if channel.ID != uuid.Nil {
		result.ChannelID = channel.ID.String()
	}

	adapter, ok := pp.adapters[platform]
	if !ok {
		result.Status = "failed"
		result.Error = fmt.Sprintf("unsupported platform: %s", platform)
		return result
	}

	if pp.dryRun {
		return pp.runAdapter(ctx, adapter, post, platformPost, channel, "", result)
	}

	if breaker, ok := pp.breakers[platform]; ok && breaker.isOpen() {
		result.Status = "failed"
		result.Error = "platform circuit breaker is open, too many recent failures"
		return result
	}

	if limiter, ok := pp.rateLimiters[platform]; ok {
		// Use a bounded wait so we never block the worker goroutine indefinitely.
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := limiter.Wait(waitCtx)
		cancel()
		if err != nil {
			result.Status = "failed"
			result.Error = fmt.Sprintf("rate limit exceeded for %s, retry later", platform)
			return result
		}
	}

	accessToken, err := pp.tokenCrypto.Open(channel.AccessToken, channel.TokenEncrypted)
	if err != nil {
		result.Status = "failed"
		result.Error = "failed to decrypt channel access token"
		return result
	}

	return pp.runAdapter(ctx, adapter, post, platformPost, channel, accessToken, result)
}

// runAdapter builds the final post text, invokes the platform adapter and
// records the outcome (including circuit-breaker bookkeeping).
func (pp *PostPublisher) runAdapter(
	ctx context.Context,
	adapter PlatformAdapter,
	post *model.Post,
	platformPost *model.PlatformPost,
	channel *model.Channel,
	accessToken string,
	result model.PostPublishResult,
) model.PostPublishResult {
	platform := platformPost.Platform

	postText := platformPost.Text
	if platformPost.IsEdited && platformPost.EditedText != "" {
		postText = platformPost.EditedText
	}

	if len(platformPost.Hashtags) > 0 {
		hashtags := ""
		for _, tag := range platformPost.Hashtags {
			hashtags += " #" + tag
		}
		postText += "\n" + hashtags
	}

	pp.logger.Info("publishing post to platform",
		"post_id", post.ID,
		"platform", platform,
		"channel", channel.ChannelName,
		"text_length", len(postText),
	)

	platformPostID, platformURL, err := adapter.Publish(ctx, PublishRequest{
		Platform:    platform,
		PostID:      post.ID,
		Channel:     channel,
		AccessToken: accessToken,
		Text:        postText,
		ImageURL:    platformPost.ImageURL,
	})

	if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
		if breaker, ok := pp.breakers[platform]; ok {
			breaker.recordFailure()
		}
		pp.logger.Error("platform publish failed",
			"post_id", post.ID,
			"platform", platform,
			"error", err,
		)
	} else {
		now := time.Now().UTC()
		result.Status = "published"
		result.PlatformPostID = platformPostID
		result.PlatformURL = platformURL
		result.PublishedAt = &now
		if breaker, ok := pp.breakers[platform]; ok {
			breaker.recordSuccess()
		}
		pp.logger.Info("post published successfully",
			"post_id", post.ID,
			"platform", platform,
			"platform_url", platformURL,
		)
	}

	return result
}

func (pp *PostPublisher) publishToTwitter(ctx context.Context, in PublishRequest) (string, string, error) {
	accessToken, text := in.AccessToken, in.Text
	tweetBody := map[string]interface{}{
		"text": text,
	}

	payload, err := json.Marshal(tweetBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal tweet: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.x.com/2/tweets", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("create tweet request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("tweet request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read tweet response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("tweet creation returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("decode tweet response: %w", err)
	}
	if result.Data.ID == "" {
		return "", "", fmt.Errorf("tweet creation returned empty id")
	}

	tweetURL := fmt.Sprintf("https://x.com/i/status/%s", result.Data.ID)
	return result.Data.ID, tweetURL, nil
}

func (pp *PostPublisher) publishToThreads(ctx context.Context, in PublishRequest) (string, string, error) {
	accessToken, channel, text, imageURL := in.AccessToken, in.Channel, in.Text, in.ImageURL
	userID, _ := extractMetadataField(channel.Metadata, "threads_user_id")
	if userID == "" {
		userID = channel.ChannelID // Fall back to channel_id.
	}
	if userID == "" {
		return "", "", fmt.Errorf("channel has no threads_user_id or channel_id")
	}

	endpoint := fmt.Sprintf("https://graph.threads.net/v1.0/%s/threads", userID)
	form := url.Values{}
	form.Set("text", text)
	form.Set("access_token", accessToken)

	if imageURL != "" {
		form.Set("media_type", "IMAGE")
		form.Set("image_url", imageURL)
	} else {
		form.Set("media_type", "TEXT")
	}

	containerID, err := pp.metaContainerCreate(ctx, endpoint, form)
	if err != nil {
		return "", "", fmt.Errorf("threads create container: %w", err)
	}

	publishEndpoint := fmt.Sprintf("https://graph.threads.net/v1.0/%s/threads_publish", userID)
	publishForm := url.Values{}
	publishForm.Set("creation_id", containerID)
	publishForm.Set("access_token", accessToken)

	mediaID, permalink, err := pp.metaContainerPublish(ctx, publishEndpoint, publishForm)
	if err != nil {
		return "", "", fmt.Errorf("threads publish container: %w", err)
	}

	return mediaID, permalink, nil
}

func (pp *PostPublisher) publishToFacebook(ctx context.Context, in PublishRequest) (string, string, error) {
	accessToken, channel, text, imageURL := in.AccessToken, in.Channel, in.Text, in.ImageURL
	pageID, _ := extractMetadataField(channel.Metadata, "page_id")
	if pageID == "" {
		pageID = channel.ChannelID // Fall back to channel_id (often the page ID).
	}
	if pageID == "" {
		return "", "", fmt.Errorf("channel has no page_id in metadata or channel_id")
	}

	var endpoint string
	form := url.Values{}
	form.Set("access_token", accessToken)

	if imageURL != "" {
		endpoint = fmt.Sprintf("https://graph.facebook.com/v21.0/%s/photos", pageID)
		form.Set("url", imageURL)
		form.Set("caption", text)
	} else {
		endpoint = fmt.Sprintf("https://graph.facebook.com/v21.0/%s/feed", pageID)
		form.Set("message", text)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("create facebook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("facebook request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read facebook response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("facebook post returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID     string `json:"id"`
		PostID string `json:"post_id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("decode facebook response: %w", err)
	}

	postID := result.ID
	if result.PostID != "" {
		postID = result.PostID
	}

	postURL := fmt.Sprintf("https://www.facebook.com/%s", postID)
	return postID, postURL, nil
}

func (pp *PostPublisher) publishToInstagram(ctx context.Context, in PublishRequest) (string, string, error) {
	accessToken, channel, text, imageURL := in.AccessToken, in.Channel, in.Text, in.ImageURL
	if imageURL == "" {
		return "", "", fmt.Errorf("instagram requires an image")
	}

	userID, _ := extractMetadataField(channel.Metadata, "instagram_business_account_id")
	if userID == "" {
		userID, _ = extractMetadataField(channel.Metadata, "ig_user_id")
	}
	if userID == "" {
		userID = channel.ChannelID // Fall back to channel_id (often the IG business account ID).
	}
	if userID == "" {
		return "", "", fmt.Errorf("channel has no instagram_business_account_id or channel_id")
	}

	// Detect Instagram Business Login: tokens only work on graph.instagram.com.
	isBusinessLogin := false
	for _, sc := range channel.Scopes {
		if strings.HasPrefix(sc, "instagram_business_") {
			isBusinessLogin = true
			break
		}
	}

	apiBase := "https://graph.facebook.com/v21.0"
	if isBusinessLogin {
		apiBase = "https://graph.instagram.com/v21.0"
	}

	endpoint := fmt.Sprintf("%s/%s/media", apiBase, userID)
	form := url.Values{}
	form.Set("media_type", "IMAGE")
	form.Set("image_url", imageURL)
	form.Set("caption", text)
	form.Set("access_token", accessToken)

	containerID, err := pp.metaContainerCreate(ctx, endpoint, form)
	if err != nil {
		return "", "", fmt.Errorf("instagram create container: %w", err)
	}

	publishEndpoint := fmt.Sprintf("%s/%s/media_publish", apiBase, userID)
	publishForm := url.Values{}
	publishForm.Set("creation_id", containerID)
	publishForm.Set("access_token", accessToken)

	mediaID, permalink, err := pp.metaContainerPublish(ctx, publishEndpoint, publishForm)
	if err != nil {
		return "", "", fmt.Errorf("instagram publish container: %w", err)
	}

	return mediaID, permalink, nil
}

func (pp *PostPublisher) publishToLinkedIn(ctx context.Context, in PublishRequest) (string, string, error) {
	accessToken, channel, text, imageURL := in.AccessToken, in.Channel, in.Text, in.ImageURL
	// Determine the author URN: prefer organization, fall back to personal profile.
	var authorURN string
	orgID, _ := extractMetadataField(channel.Metadata, "organization_id")
	if orgID == "" {
		orgID, _ = extractMetadataField(channel.Metadata, "company_id")
	}
	if orgID != "" {
		authorURN = fmt.Sprintf("urn:li:organization:%s", orgID)
	} else {
		// Personal profile: use person_id from metadata or fall back to channel_id.
		personID, _ := extractMetadataField(channel.Metadata, "person_id")
		if personID == "" {
			personID, _ = extractMetadataField(channel.Metadata, "member_id")
		}
		if personID == "" {
			personID = channel.ChannelID
		}
		if personID == "" {
			return "", "", fmt.Errorf("channel has no organization_id, person_id, or channel_id for LinkedIn")
		}
		authorURN = fmt.Sprintf("urn:li:person:%s", personID)
	}

	postBody := map[string]interface{}{
		"author":         authorURN,
		"commentary":     text,
		"visibility":     "PUBLIC",
		"distribution":   map[string]string{"feedDistribution": "MAIN_FEED"},
		"lifecycleState": "PUBLISHED",
	}

	if imageURL != "" {
		imageURN, err := pp.linkedInUploadImage(ctx, accessToken, authorURN, imageURL)
		if err != nil {
			pp.logger.Warn("linkedin image upload failed, posting text only", "error", err)
		} else {
			postBody["content"] = map[string]interface{}{
				"media": map[string]interface{}{
					"id": imageURN,
				},
			}
		}
	}

	payload, err := json.Marshal(postBody)
	if err != nil {
		return "", "", fmt.Errorf("marshal linkedin post: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.linkedin.com/rest/posts", bytes.NewReader(payload))
	if err != nil {
		return "", "", fmt.Errorf("create linkedin request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LinkedIn-Version", "202601")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("linkedin post request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read linkedin response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("linkedin post returned %d: %s", resp.StatusCode, string(body))
	}

	postURN := resp.Header.Get("x-restli-id")
	if postURN == "" {
		var postResp struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(body, &postResp); err == nil && postResp.ID != "" {
			postURN = postResp.ID
		}
	}

	postURL := fmt.Sprintf("https://www.linkedin.com/feed/update/%s", postURN)
	return postURN, postURL, nil
}

func (pp *PostPublisher) metaContainerCreate(ctx context.Context, endpoint string, form url.Values) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create container request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("post container: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read container response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("container creation returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode container response: %w", err)
	}
	if result.ID == "" {
		return "", fmt.Errorf("container response missing id")
	}

	return result.ID, nil
}

func (pp *PostPublisher) metaContainerPublish(ctx context.Context, endpoint string, form url.Values) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", fmt.Errorf("create publish request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("publish container: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("read publish response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("publish returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID        string `json:"id"`
		Permalink string `json:"permalink"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", "", fmt.Errorf("decode publish response: %w", err)
	}
	if result.ID == "" {
		return "", "", fmt.Errorf("publish response missing id")
	}

	return result.ID, result.Permalink, nil
}

func (pp *PostPublisher) linkedInUploadImage(ctx context.Context, accessToken, ownerURN, imageURL string) (string, error) {
	initBody := map[string]interface{}{
		"initializeUploadRequest": map[string]interface{}{
			"owner": ownerURN,
		},
	}
	initPayload, _ := json.Marshal(initBody)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api.linkedin.com/rest/images?action=initializeUpload",
		bytes.NewReader(initPayload))
	if err != nil {
		return "", fmt.Errorf("create init request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("LinkedIn-Version", "202601")
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	resp, err := pp.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("init image upload: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read init response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("init image upload returned %d: %s", resp.StatusCode, string(body))
	}

	var initResp struct {
		Value struct {
			UploadURL string `json:"uploadUrl"`
			Image     string `json:"image"`
		} `json:"value"`
	}
	if err := json.Unmarshal(body, &initResp); err != nil {
		return "", fmt.Errorf("decode init response: %w", err)
	}

	if initResp.Value.UploadURL == "" || initResp.Value.Image == "" {
		return "", fmt.Errorf("init response missing uploadUrl or image URN")
	}

	imgResp, err := pp.httpClient.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}
	defer imgResp.Body.Close()

	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		initResp.Value.UploadURL, imgResp.Body)
	if err != nil {
		return "", fmt.Errorf("create upload request: %w", err)
	}
	uploadReq.Header.Set("Authorization", "Bearer "+accessToken)
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	uploadReq.Header.Set("LinkedIn-Version", "202601")
	uploadReq.Header.Set("X-Restli-Protocol-Version", "2.0.0")

	uploadResp, err := pp.httpClient.Do(uploadReq)
	if err != nil {
		return "", fmt.Errorf("upload image: %w", err)
	}
	defer uploadResp.Body.Close()

	if uploadResp.StatusCode != http.StatusCreated && uploadResp.StatusCode != http.StatusOK {
		uploadBody, _ := io.ReadAll(uploadResp.Body)
		return "", fmt.Errorf("image upload returned %d: %s", uploadResp.StatusCode, string(uploadBody))
	}

	return initResp.Value.Image, nil
}

func (pp *PostPublisher) resolveChannels(ctx context.Context, post *model.Post) (map[string]*model.Channel, error) {
	channels := make(map[string]*model.Channel)

	rows, err := pp.pool.Query(ctx, `
		SELECT id, category_id, user_id, channel_type, platform, channel_name, channel_id,
			access_token, COALESCE(token_encrypted, FALSE), connection_status, token_expires_at, metadata, scopes
		FROM channels
		WHERE platform = ANY($1)
		  AND connection_status = 'connected'
		  AND (token_expires_at IS NULL OR token_expires_at > NOW())
		ORDER BY
			CASE WHEN category_id = $2 THEN 0 ELSE 1 END,
			created_at ASC`,
		post.TargetPlatforms, post.CategoryID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var ch model.Channel
		var channelID, accessToken *string
		err := rows.Scan(
			&ch.ID, &ch.CategoryID, &ch.UserID, &ch.ChannelType, &ch.Platform, &ch.ChannelName,
			&channelID, &accessToken, &ch.TokenEncrypted, &ch.ConnectionStatus,
			&ch.TokenExpiresAt, &ch.Metadata, &ch.Scopes,
		)
		if err != nil {
			pp.logger.Warn("failed to scan channel", "error", err)
			continue
		}
		ch.ChannelID = derefStr(channelID)
		ch.AccessToken = derefStr(accessToken)
		if _, exists := channels[ch.Platform]; !exists {
			channels[ch.Platform] = &ch
		}
	}

	return channels, nil
}

func (pp *PostPublisher) recordPublishLink(ctx context.Context, postID uuid.UUID, result model.PostPublishResult, channelID uuid.UUID) {
	publishedAt := result.PublishedAt

	// Dry-run placeholder channels have no row in channels; store NULL.
	var channelRef *uuid.UUID
	if channelID != uuid.Nil {
		channelRef = &channelID
	}

	_, err := pp.pool.Exec(ctx, `
		INSERT INTO post_publish_links (
			id, post_id, platform, channel_id, platform_post_id,
			platform_url, status, error_message, published_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (post_id, platform) DO UPDATE SET
			platform_post_id = EXCLUDED.platform_post_id,
			platform_url = EXCLUDED.platform_url,
			status = EXCLUDED.status,
			error_message = EXCLUDED.error_message,
			published_at = EXCLUDED.published_at,
			updated_at = NOW()`,
		uuid.New(), postID, result.Platform, channelRef,
		nilIfEmptyPtr(result.PlatformPostID), nilIfEmptyPtr(result.PlatformURL),
		result.Status, nilIfEmptyPtr(result.Error), publishedAt,
	)
	if err != nil {
		pp.logger.Error("failed to record publish link",
			"post_id", postID,
			"platform", result.Platform,
			"error", err,
		)
	}
}
