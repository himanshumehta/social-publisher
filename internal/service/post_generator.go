package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/model"
)

const (
	claudeAPIURL    = "https://api.anthropic.com/v1/messages"
	claudeModel     = "claude-sonnet-4-5-20250929"
	claudeMaxTokens = 4096
	genCacheTTL     = 1 * time.Hour
	genCachePrefix  = "post:gen:"
	genHTTPTimeout  = 60 * time.Second
	maxGenRetries   = 3
)

type PostGenerator struct {
	pool   *pgxpool.Pool
	rdb    *redis.Client
	logger *slog.Logger
	apiKey string
	client *http.Client
}

func NewPostGenerator(pool *pgxpool.Pool, rdb *redis.Client, logger *slog.Logger, claudeAPIKey string) *PostGenerator {
	return &PostGenerator{
		pool:   pool,
		rdb:    rdb,
		logger: logger,
		apiKey: claudeAPIKey,
		client: &http.Client{Timeout: genHTTPTimeout},
	}
}

type GenerateRequest struct {
	ScannedContent *model.ScannedContent `json:"scanned_content,omitempty"`
	ManualTitle    string                `json:"manual_title,omitempty"`
	ManualContent  string                `json:"manual_content,omitempty"`
	Platforms      []string              `json:"platforms"`
	LinkedVideoURL string                `json:"linked_video_url,omitempty"`
	SourceURL      string                `json:"source_url,omitempty"`
	CoverImageURL  string                `json:"cover_image_url,omitempty"`
	Tone           string                `json:"tone"`
	BrandVoice     string                `json:"brand_voice,omitempty"`
}

type GenerateResult struct {
	Platforms   map[string]*model.PlatformPost `json:"platforms"`
	TokensUsed  int                            `json:"tokens_used"`
	Model       string                         `json:"model"`
	GeneratedAt time.Time                      `json:"generated_at"`
}

// ErrNoGenerationKey is returned when copy generation is needed but
// CLAUDE_API_KEY is not configured. Posts that supply platform_posts never
// reach the generator.
var ErrNoGenerationKey = errors.New("CLAUDE_API_KEY is not set; supply platform_posts or configure a key")

func (g *PostGenerator) Generate(ctx context.Context, req *GenerateRequest) (*GenerateResult, error) {
	cacheKey := genCachePrefix + g.contentHash(req)
	if cached, err := g.rdb.Get(ctx, cacheKey).Bytes(); err == nil {
		var result GenerateResult
		if json.Unmarshal(cached, &result) == nil {
			g.logger.Info("post generator cache hit", "platforms", len(req.Platforms))
			return &result, nil
		}
	}

	if g.apiKey == "" {
		return nil, ErrNoGenerationKey // fail fast instead of three doomed API calls
	}

	systemPrompt := g.buildSystemPrompt(req)
	userPrompt := g.buildUserPrompt(req)

	var lastErr error
	for attempt := 0; attempt < maxGenRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt)) * 2 * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		result, err := g.callClaude(ctx, systemPrompt, userPrompt, req.Platforms)
		if err != nil {
			lastErr = err
			g.logger.Warn("claude API call failed, retrying",
				"attempt", attempt+1,
				"error", err,
			)
			continue
		}

		if data, err := json.Marshal(result); err == nil {
			g.rdb.Set(ctx, cacheKey, data, genCacheTTL)
		}

		return result, nil
	}

	return nil, fmt.Errorf("claude API failed after %d attempts: %w", maxGenRetries, lastErr)
}

func (g *PostGenerator) buildSystemPrompt(req *GenerateRequest) string {
	var sb strings.Builder
	sb.WriteString(`You are a social media content specialist for a vehicle review platform.
Generate engaging post content for each requested platform.

CRITICAL RULES:
1. Each platform post MUST respect its character limit (including hashtags and URL)
2. Include the source URL as a call-to-action in every post
3. Never fabricate statistics, specs, or claims not present in the source content
4. Hashtags should be relevant and platform-appropriate
5. Each platform should have a DIFFERENT hook/angle — do not repeat the same text

PLATFORM SPECIFICATIONS:
`)

	for _, p := range req.Platforms {
		limit := model.PlatformCharLimits[p]
		switch p {
		case "twitter":
			fmt.Fprintf(&sb, "- Twitter/X (max %d chars): Punchy, data-driven. Lead with a surprising stat or bold take. 3-5 hashtags max. URLs auto-shorten to 23 chars.\n", limit)
		case "threads":
			fmt.Fprintf(&sb, "- Threads (max %d chars): Conversational, opinion-forward. Ask a question or share a hot take. 5-8 hashtags.\n", limit)
		case "facebook":
			fmt.Fprintf(&sb, "- Facebook (max %d chars): Engaging question or relatable statement. Shareable. 3-5 hashtags. Use line breaks for readability.\n", limit)
		case "instagram":
			fmt.Fprintf(&sb, "- Instagram (max %d chars): Visual-first caption. Start with a hook. 20-30 relevant hashtags in a separate block. Use line breaks and emojis.\n", limit)
		case "linkedin":
			fmt.Fprintf(&sb, "- LinkedIn (max %d chars): Professional, insight-led. Lead with an industry observation. 3-5 hashtags. No emojis.\n", limit)
		}
	}

	if req.Tone != "" {
		fmt.Fprintf(&sb, "\nTONE: %s\n", req.Tone)
	}
	if req.BrandVoice != "" {
		fmt.Fprintf(&sb, "\nBRAND VOICE GUIDELINES:\n%s\n", req.BrandVoice)
	}

	sb.WriteString(`
OUTPUT FORMAT: Respond with a JSON object where each key is the platform name and the value is an object with:
- "text": the full post text (including CTA link)
- "hashtags": array of hashtags (without # prefix)
- "char_count": character count of text + hashtags

Example:
{
  "twitter": {
    "text": "The post text here https://example.com/review",
    "hashtags": ["bikes", "review"],
    "char_count": 65
  }
}
`)
	return sb.String()
}

func (g *PostGenerator) buildUserPrompt(req *GenerateRequest) string {
	var sb strings.Builder

	if req.ScannedContent != nil {
		sc := req.ScannedContent
		fmt.Fprintf(&sb, "SOURCE ARTICLE:\nTitle: %s\n", sc.Title)
		if sc.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", sc.Description)
		}
		if sc.Author != "" {
			fmt.Fprintf(&sb, "Author: %s\n", sc.Author)
		}
		if sc.SiteName != "" {
			fmt.Fprintf(&sb, "Site: %s\n", sc.SiteName)
		}
		fmt.Fprintf(&sb, "Word Count: %d\n", sc.WordCount)
		if sc.BodyText != "" {
			fmt.Fprintf(&sb, "\nArticle Content:\n%s\n", sc.BodyText)
		}
	} else {
		fmt.Fprintf(&sb, "CONTENT:\nTitle: %s\n", req.ManualTitle)
		if req.ManualContent != "" {
			fmt.Fprintf(&sb, "Content: %s\n", req.ManualContent)
		}
	}

	if req.SourceURL != "" {
		fmt.Fprintf(&sb, "\nSOURCE URL (include in all posts): %s\n", req.SourceURL)
	}
	if req.LinkedVideoURL != "" {
		fmt.Fprintf(&sb, "VIDEO LINK: %s\n", req.LinkedVideoURL)
	}

	fmt.Fprintf(&sb, "\nGenerate posts for: %s", strings.Join(req.Platforms, ", "))

	return sb.String()
}

type claudeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type claudeRequest struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	System    string          `json:"system"`
	Messages  []claudeMessage `json:"messages"`
}

type claudeResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

func (g *PostGenerator) callClaude(ctx context.Context, systemPrompt, userPrompt string, platforms []string) (*GenerateResult, error) {
	reqBody := claudeRequest{
		Model:     claudeModel,
		MaxTokens: claudeMaxTokens,
		System:    systemPrompt,
		Messages: []claudeMessage{
			{Role: "user", Content: userPrompt},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", claudeAPIURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", g.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling Claude API: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude API returned %d: %s", resp.StatusCode, string(respBody))
	}

	var claudeResp claudeResponse
	if err := json.Unmarshal(respBody, &claudeResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	if len(claudeResp.Content) == 0 {
		return nil, fmt.Errorf("empty response from Claude")
	}

	responseText := claudeResp.Content[0].Text
	jsonStr := extractJSON(responseText)

	var platformPosts map[string]*model.PlatformPost
	if err := json.Unmarshal([]byte(jsonStr), &platformPosts); err != nil {
		return nil, fmt.Errorf("parsing platform posts JSON: %w", err)
	}

	platformPosts, err = validateGenerated(platformPosts, platforms)
	if err != nil {
		return nil, err
	}

	return &GenerateResult{
		Platforms:   platformPosts,
		TokensUsed:  claudeResp.Usage.InputTokens + claudeResp.Usage.OutputTokens,
		Model:       claudeModel,
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func extractJSON(text string) string {
	if idx := strings.Index(text, "```json"); idx != -1 {
		start := idx + 7
		end := strings.Index(text[start:], "```")
		if end != -1 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	if idx := strings.Index(text, "```"); idx != -1 {
		start := idx + 3
		end := strings.Index(text[start:], "```")
		if end != -1 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

func (g *PostGenerator) contentHash(req *GenerateRequest) string {
	h := sha256.New()
	if req.ScannedContent != nil {
		h.Write([]byte(req.ScannedContent.Title))
		h.Write([]byte(req.ScannedContent.BodyText))
	} else {
		h.Write([]byte(req.ManualTitle))
		h.Write([]byte(req.ManualContent))
	}
	for _, p := range req.Platforms {
		h.Write([]byte(p))
	}
	h.Write([]byte(req.Tone))
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// validateGenerated is the guardrail on model output. The model's JSON is
// untrusted: keep only the platforms that were requested, require every one
// of them with non-empty text, and recompute character counts from the text
// itself (the model's own char_count is ignored, so the reviewer's length
// check cannot be talked past). Length limits are enforced by PostReviewer,
// which flags over-long posts before an admin sees them.
func validateGenerated(posts map[string]*model.PlatformPost, requested []string) (map[string]*model.PlatformPost, error) {
	out := make(map[string]*model.PlatformPost, len(requested))
	for _, platform := range requested {
		post, ok := posts[platform]
		if !ok || post == nil || strings.TrimSpace(post.Text) == "" {
			return nil, fmt.Errorf("model output has no text for %s", platform)
		}
		post.Platform = platform
		post.CharLimit = model.PlatformCharLimits[platform]
		post.CharCount = utf8.RuneCountInString(post.Text)
		for _, tag := range post.Hashtags {
			post.CharCount += utf8.RuneCountInString(tag) + 2 // "#tag" plus a separator
		}
		out[platform] = post
	}
	return out, nil
}
