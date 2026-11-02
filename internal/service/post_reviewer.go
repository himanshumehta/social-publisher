package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/himanshumehta/social-publisher/internal/model"
)

type ReviewSeverity string

const (
	SeverityInfo    ReviewSeverity = "info"
	SeverityWarning ReviewSeverity = "warning"
	SeverityBlock   ReviewSeverity = "block"
)

type ReviewFlag struct {
	Rule     string         `json:"rule"`
	Severity ReviewSeverity `json:"severity"`
	Detail   string         `json:"detail"`
	Platform string         `json:"platform,omitempty"`
}

type ReviewResult struct {
	Approved bool           `json:"approved"`
	Flags    []ReviewFlag   `json:"flags"`
	Severity ReviewSeverity `json:"severity"`
	Notes    string         `json:"notes"`
}

type PostReviewer struct {
	logger       *slog.Logger
	blocklist    map[string]ReviewSeverity
	urlBlocklist map[string]bool
	maxHashtags  map[string]int
}

func NewPostReviewer(logger *slog.Logger) *PostReviewer {
	return &PostReviewer{
		logger:       logger,
		blocklist:    defaultBlocklist(),
		urlBlocklist: defaultURLBlocklist(),
		maxHashtags: map[string]int{
			"twitter":   5,
			"threads":   10,
			"facebook":  5,
			"instagram": 30,
			"linkedin":  5,
		},
	}
}

func (r *PostReviewer) Review(ctx context.Context, post *model.Post, platformPosts map[string]*model.PlatformPost) *ReviewResult {
	result := &ReviewResult{
		Approved: true,
		Severity: SeverityInfo,
	}

	if post.SourceURL != "" {
		r.checkURLSafety(post.SourceURL, result)
	}

	for platform, pp := range platformPosts {
		r.checkPlatformPost(platform, pp, result)
	}

	r.checkText("title", post.Title, result)
	if post.Content != "" {
		r.checkText("content", post.Content, result)
	}

	for _, flag := range result.Flags {
		if flag.Severity == SeverityBlock {
			result.Approved = false
			result.Severity = SeverityBlock
			break
		}
		if flag.Severity == SeverityWarning && result.Severity != SeverityBlock {
			result.Severity = SeverityWarning
		}
	}

	if len(result.Flags) > 0 {
		var notes []string
		for _, f := range result.Flags {
			notes = append(notes, fmt.Sprintf("[%s] %s: %s", f.Severity, f.Rule, f.Detail))
		}
		result.Notes = strings.Join(notes, "; ")
	}

	return result
}

func (r *PostReviewer) checkURLSafety(rawURL string, result *ReviewResult) {
	lower := strings.ToLower(rawURL)
	for domain := range r.urlBlocklist {
		if strings.Contains(lower, domain) {
			result.Flags = append(result.Flags, ReviewFlag{
				Rule:     "blocked_domain",
				Severity: SeverityBlock,
				Detail:   fmt.Sprintf("URL contains blocked domain: %s", domain),
			})
		}
	}
}

func (r *PostReviewer) checkPlatformPost(platform string, pp *model.PlatformPost, result *ReviewResult) {
	charLimit := model.PlatformCharLimits[platform]
	if charLimit > 0 && pp.CharCount > charLimit {
		result.Flags = append(result.Flags, ReviewFlag{
			Rule:     "char_limit_exceeded",
			Severity: SeverityWarning,
			Detail:   fmt.Sprintf("%s post is %d chars, limit is %d", platform, pp.CharCount, charLimit),
			Platform: platform,
		})
	}

	if maxTags, ok := r.maxHashtags[platform]; ok && len(pp.Hashtags) > maxTags {
		result.Flags = append(result.Flags, ReviewFlag{
			Rule:     "excessive_hashtags",
			Severity: SeverityInfo,
			Detail:   fmt.Sprintf("%s has %d hashtags, recommended max %d", platform, len(pp.Hashtags), maxTags),
			Platform: platform,
		})
	}

	r.checkText(platform+" post", pp.Text, result)

	if strings.TrimSpace(pp.Text) == "" {
		result.Flags = append(result.Flags, ReviewFlag{
			Rule:     "empty_content",
			Severity: SeverityBlock,
			Detail:   fmt.Sprintf("%s post has empty text", platform),
			Platform: platform,
		})
	}
}

func (r *PostReviewer) checkText(source, text string, result *ReviewResult) {
	lower := strings.ToLower(text)
	words := strings.Fields(lower)

	for _, word := range words {
		cleaned := strings.Trim(word, ".,!?;:'\"()-[]{}#@")
		if severity, found := r.blocklist[cleaned]; found {
			result.Flags = append(result.Flags, ReviewFlag{
				Rule:     "blocklist_match",
				Severity: severity,
				Detail:   fmt.Sprintf("Found blocked term '%s' in %s", cleaned, source),
			})
		}
	}
}

func defaultBlocklist() map[string]ReviewSeverity {
	blocklist := make(map[string]ReviewSeverity)

	for _, word := range []string{
		"guaranteed", "100%", "risk-free", "no-risk",
	} {
		blocklist[word] = SeverityBlock
	}

	for _, word := range []string{
		"zigwheels", "bikedekho", "cardekho", "autocar",
	} {
		blocklist[word] = SeverityWarning
	}

	return blocklist
}

func defaultURLBlocklist() map[string]bool {
	return map[string]bool{
		"bit.ly":      true,
		"tinyurl.com": true,
		"t.co":        true,
	}
}

func ReviewResultToDBFields(result *ReviewResult) (status string, notes string, reviewedAt time.Time) {
	reviewedAt = time.Now().UTC()
	if result.Approved {
		if result.Severity == SeverityWarning {
			return "flagged", result.Notes, reviewedAt
		}
		return "passed", result.Notes, reviewedAt
	}
	return "blocked", result.Notes, reviewedAt
}
