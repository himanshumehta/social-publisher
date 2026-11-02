package service

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/himanshumehta/social-publisher/internal/model"
)

func newTestReviewer() *PostReviewer {
	return NewPostReviewer(slog.Default())
}

func TestPostReviewer_Review(t *testing.T) {
	tests := []struct {
		name             string
		post             *model.Post
		platformPosts    map[string]*model.PlatformPost
		wantApproved     bool
		wantSeverity     ReviewSeverity
		wantFlagCount    int
		wantFlagRuleAny  string // at least one flag should match this rule
		wantNotesContain string // substring expected in Notes
	}{
		// --- 1. Blocklist matching (block severity) ---
		{
			name: "block severity: guaranteed in title",
			post: &model.Post{
				Title:   "This is guaranteed to work",
				Content: "Normal content here",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "guaranteed",
		},
		{
			name: "block severity: 100% in content",
			post: &model.Post{
				Title:   "Great deal",
				Content: "This is 100% effective",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "100%",
		},
		{
			name: "block severity: risk-free in platform post",
			post: &model.Post{
				Title: "Good offer",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "This is a risk-free investment",
					CharCount: 30,
					Hashtags:  []string{"#invest"},
				},
			},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "risk-free",
		},
		{
			name: "block severity: no-risk in title",
			post: &model.Post{
				Title:   "A no-risk opportunity",
				Content: "Details inside",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "no-risk",
		},

		// --- 2. Warning terms matching ---
		{
			name: "warning severity: zigwheels in content",
			post: &model.Post{
				Title:   "Bike review",
				Content: "According to zigwheels the mileage is great",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "zigwheels",
		},
		{
			name: "warning severity: bikedekho in title",
			post: &model.Post{
				Title: "BikedEkho says this bike is the best",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "bikedekho",
		},
		{
			name: "warning severity: cardekho in platform post",
			post: &model.Post{
				Title: "Car review",
			},
			platformPosts: map[string]*model.PlatformPost{
				"facebook": {
					Text:      "Check cardekho for more details",
					CharCount: 32,
				},
			},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "cardekho",
		},
		{
			name: "warning severity: autocar in content",
			post: &model.Post{
				Title:   "New launch",
				Content: "Autocar covered this launch extensively",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "autocar",
		},

		// --- 3. URL blocking ---
		{
			name: "blocked URL: bit.ly",
			post: &model.Post{
				Title:     "Check this out",
				SourceURL: "https://bit.ly/abc123",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocked_domain",
			wantNotesContain: "bit.ly",
		},
		{
			name: "blocked URL: tinyurl.com",
			post: &model.Post{
				Title:     "Short link",
				SourceURL: "https://tinyurl.com/xyz",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocked_domain",
			wantNotesContain: "tinyurl.com",
		},
		{
			name: "blocked URL: t.co",
			post: &model.Post{
				Title:     "Tweet link",
				SourceURL: "https://t.co/abc",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocked_domain",
			wantNotesContain: "t.co",
		},

		// --- 4. Safe URL passes ---
		{
			name: "safe URL: regular domain passes",
			post: &model.Post{
				Title:     "Our blog post",
				SourceURL: "https://example.com/post/123",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     true,
			wantSeverity:     SeverityInfo,
			wantFlagCount:    0,
			wantNotesContain: "",
		},

		// --- 5. Char limit warnings ---
		{
			name: "char limit: twitter exceeds 280",
			post: &model.Post{
				Title: "Tweet post",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "This is a very long tweet that exceeds the character limit for Twitter",
					CharCount: 300,
					Hashtags:  []string{"#test"},
				},
			},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "char_limit_exceeded",
			wantNotesContain: "twitter post is 300 chars",
		},
		{
			name: "char limit: threads exceeds 500",
			post: &model.Post{
				Title: "Threads post",
			},
			platformPosts: map[string]*model.PlatformPost{
				"threads": {
					Text:      "Long threads post content",
					CharCount: 550,
				},
			},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "char_limit_exceeded",
			wantNotesContain: "threads post is 550 chars",
		},
		{
			name: "char limit: within limit no flag",
			post: &model.Post{
				Title: "Short tweet",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "Hello world",
					CharCount: 11,
					Hashtags:  []string{"#hi"},
				},
			},
			wantApproved:  true,
			wantSeverity:  SeverityInfo,
			wantFlagCount: 0,
		},

		// --- 6. Excessive hashtags (info severity) ---
		{
			name: "excessive hashtags: twitter > 5",
			post: &model.Post{
				Title: "Hashtag test",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "Lots of tags",
					CharCount: 12,
					Hashtags:  []string{"#a", "#b", "#c", "#d", "#e", "#f"},
				},
			},
			wantApproved:     true,
			wantSeverity:     SeverityInfo,
			wantFlagCount:    1,
			wantFlagRuleAny:  "excessive_hashtags",
			wantNotesContain: "twitter has 6 hashtags",
		},
		{
			name: "excessive hashtags: instagram > 30",
			post: &model.Post{
				Title: "Instagram tags",
			},
			platformPosts: map[string]*model.PlatformPost{
				"instagram": {
					Text:      "Photo post",
					CharCount: 10,
					Hashtags:  make([]string, 31), // 31 hashtags
				},
			},
			wantApproved:     true,
			wantSeverity:     SeverityInfo,
			wantFlagCount:    1,
			wantFlagRuleAny:  "excessive_hashtags",
			wantNotesContain: "instagram has 31 hashtags",
		},
		{
			name: "hashtags within limit: no flag",
			post: &model.Post{
				Title: "Normal tags",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "Post with tags",
					CharCount: 14,
					Hashtags:  []string{"#a", "#b", "#c"},
				},
			},
			wantApproved:  true,
			wantSeverity:  SeverityInfo,
			wantFlagCount: 0,
		},

		// --- 7. Empty content blocking ---
		{
			name: "empty platform post text: blocked",
			post: &model.Post{
				Title: "Has a title",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "",
					CharCount: 0,
				},
			},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "empty_content",
			wantNotesContain: "twitter post has empty text",
		},
		{
			name: "whitespace-only platform post text: blocked",
			post: &model.Post{
				Title: "Has a title",
			},
			platformPosts: map[string]*model.PlatformPost{
				"facebook": {
					Text:      "   \t\n  ",
					CharCount: 0,
				},
			},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "empty_content",
			wantNotesContain: "facebook post has empty text",
		},

		// --- 8. Clean post passes review ---
		{
			name: "clean post: fully approved",
			post: &model.Post{
				Title:     "A lovely day for biking",
				Content:   "Today we explore some great routes in the mountains",
				SourceURL: "https://ourblog.com/article/42",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "Great day for biking! Check out our latest routes.",
					CharCount: 50,
					Hashtags:  []string{"#biking", "#routes"},
				},
				"instagram": {
					Text:      "A lovely day for biking in the mountains.",
					CharCount: 42,
					Hashtags:  []string{"#biking"},
				},
			},
			wantApproved:  true,
			wantSeverity:  SeverityInfo,
			wantFlagCount: 0,
		},

		// --- 9. Severity escalation: warning does not override block ---
		{
			name: "severity escalation: block + warning results in block",
			post: &model.Post{
				Title:   "Guaranteed review by zigwheels",
				Content: "Check it out",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    2, // guaranteed=block + zigwheels=warning
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "guaranteed",
		},

		// --- 11. Multiple flags combine correctly ---
		{
			name: "multiple flags: blocked URL + empty content",
			post: &model.Post{
				Title:     "Check this",
				SourceURL: "https://bit.ly/deal",
			},
			platformPosts: map[string]*model.PlatformPost{
				"twitter": {
					Text:      "",
					CharCount: 0,
				},
			},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    2, // blocked_domain + empty_content
			wantFlagRuleAny:  "blocked_domain",
			wantNotesContain: "bit.ly",
		},
		{
			name: "multiple warnings: char limit + warning term",
			post: &model.Post{
				Title: "Review from zigwheels on threads",
			},
			platformPosts: map[string]*model.PlatformPost{
				"threads": {
					Text:      "Content from zigwheels review",
					CharCount: 510,
				},
			},
			wantApproved:    true,
			wantSeverity:    SeverityWarning,
			wantFlagCount:   3, // char_limit + zigwheels in threads post + zigwheels in title
			wantFlagRuleAny: "blocklist_match",
		},

		// --- 12. checkText with punctuation-wrapped words ---
		{
			name: "punctuation: guaranteed! still matches",
			post: &model.Post{
				Title:   "This is guaranteed! you will love it",
				Content: "Regular content",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "guaranteed",
		},
		{
			name: "punctuation: (risk-free) still matches",
			post: &model.Post{
				Title:   "This is (risk-free) for everyone",
				Content: "Good stuff",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     false,
			wantSeverity:     SeverityBlock,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "risk-free",
		},
		{
			name: "punctuation: zigwheels. still matches as warning",
			post: &model.Post{
				Title:   "Read on zigwheels. The best reviews",
				Content: "More info",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "zigwheels",
		},
		{
			name: "punctuation: quoted \"autocar\" still matches",
			post: &model.Post{
				Title:   "Check out \"autocar\" for reviews",
				Content: "Detailed coverage",
			},
			platformPosts:    map[string]*model.PlatformPost{},
			wantApproved:     true,
			wantSeverity:     SeverityWarning,
			wantFlagCount:    1,
			wantFlagRuleAny:  "blocklist_match",
			wantNotesContain: "autocar",
		},
	}

	reviewer := newTestReviewer()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := reviewer.Review(ctx, tt.post, tt.platformPosts)

			if result.Approved != tt.wantApproved {
				t.Errorf("Approved = %v, want %v", result.Approved, tt.wantApproved)
			}

			if result.Severity != tt.wantSeverity {
				t.Errorf("Severity = %q, want %q", result.Severity, tt.wantSeverity)
			}

			if tt.wantFlagCount > 0 && len(result.Flags) < tt.wantFlagCount {
				t.Errorf("Flags count = %d, want at least %d", len(result.Flags), tt.wantFlagCount)
			}
			if tt.wantFlagCount == 0 && len(result.Flags) != 0 {
				t.Errorf("Flags count = %d, want 0; flags: %+v", len(result.Flags), result.Flags)
			}

			if tt.wantFlagRuleAny != "" {
				found := false
				for _, f := range result.Flags {
					if f.Rule == tt.wantFlagRuleAny {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected at least one flag with rule %q, got flags: %+v", tt.wantFlagRuleAny, result.Flags)
				}
			}

			if tt.wantNotesContain != "" {
				if !reviewTestContains(result.Notes, tt.wantNotesContain) {
					t.Errorf("Notes = %q, want it to contain %q", result.Notes, tt.wantNotesContain)
				}
			}
		})
	}
}

func TestReviewResultToDBFields(t *testing.T) {
	tests := []struct {
		name       string
		result     *ReviewResult
		wantStatus string
	}{
		{
			name: "approved with info severity returns passed",
			result: &ReviewResult{
				Approved: true,
				Severity: SeverityInfo,
				Notes:    "",
			},
			wantStatus: "passed",
		},
		{
			name: "approved with warning severity returns flagged",
			result: &ReviewResult{
				Approved: true,
				Severity: SeverityWarning,
				Notes:    "[warning] blocklist_match: Found blocked term 'zigwheels' in title",
			},
			wantStatus: "flagged",
		},
		{
			name: "not approved returns blocked",
			result: &ReviewResult{
				Approved: false,
				Severity: SeverityBlock,
				Notes:    "[block] blocklist_match: Found blocked term 'guaranteed' in title",
			},
			wantStatus: "blocked",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, notes, reviewedAt := ReviewResultToDBFields(tt.result)

			if status != tt.wantStatus {
				t.Errorf("status = %q, want %q", status, tt.wantStatus)
			}

			if notes != tt.result.Notes {
				t.Errorf("notes = %q, want %q", notes, tt.result.Notes)
			}

			if reviewedAt.IsZero() {
				t.Error("reviewedAt should not be zero")
			}
		})
	}
}

func TestPostReviewer_Review_MultipleFlagsCombine(t *testing.T) {
	reviewer := newTestReviewer()
	ctx := context.Background()

	// Post with a block term in title, a warning term in content, blocked URL,
	// an over-limit platform post with empty text, and excessive hashtags.
	post := &model.Post{
		Title:     "This is guaranteed by zigwheels",
		Content:   "Great bikedekho review",
		SourceURL: "https://bit.ly/deal",
	}
	platformPosts := map[string]*model.PlatformPost{
		"twitter": {
			Text:      "",
			CharCount: 0,
			Hashtags:  []string{"#1", "#2", "#3", "#4", "#5", "#6"},
		},
	}

	result := reviewer.Review(ctx, post, platformPosts)

	if result.Approved {
		t.Error("expected Approved=false when block-level flags present")
	}

	if result.Severity != SeverityBlock {
		t.Errorf("Severity = %q, want %q", result.Severity, SeverityBlock)
	}

	// Should have multiple flags: blocked_domain, empty_content, excessive_hashtags,
	// blocklist_match(guaranteed), blocklist_match(zigwheels), blocklist_match(bikedekho)
	if len(result.Flags) < 4 {
		t.Errorf("expected at least 4 flags, got %d: %+v", len(result.Flags), result.Flags)
	}

	// Verify the notes contain entries from multiple rules.
	for _, expected := range []string{"guaranteed", "bit.ly", "empty text"} {
		if !reviewTestContains(result.Notes, expected) {
			t.Errorf("Notes should contain %q, got: %q", expected, result.Notes)
		}
	}
}

func TestPostReviewer_Review_EmptyPlatformPosts(t *testing.T) {
	reviewer := newTestReviewer()
	ctx := context.Background()

	post := &model.Post{
		Title:   "Simple clean title",
		Content: "Nothing controversial here",
	}

	result := reviewer.Review(ctx, post, nil)

	if !result.Approved {
		t.Error("expected Approved=true for clean post with nil platformPosts")
	}
	if result.Severity != SeverityInfo {
		t.Errorf("Severity = %q, want %q", result.Severity, SeverityInfo)
	}
	if len(result.Flags) != 0 {
		t.Errorf("expected 0 flags, got %d: %+v", len(result.Flags), result.Flags)
	}
}

func TestPostReviewer_Review_CaseInsensitive(t *testing.T) {
	reviewer := newTestReviewer()
	ctx := context.Background()

	post := &model.Post{
		Title:   "This is GUARANTEED to be great",
		Content: "Visit BIKEDEKHO for details",
	}

	result := reviewer.Review(ctx, post, map[string]*model.PlatformPost{})

	if result.Approved {
		t.Error("expected Approved=false for uppercase GUARANTEED")
	}
	if result.Severity != SeverityBlock {
		t.Errorf("Severity = %q, want %q", result.Severity, SeverityBlock)
	}

	foundBlock := false
	foundWarning := false
	for _, f := range result.Flags {
		if reviewTestContains(f.Detail, "guaranteed") {
			foundBlock = true
		}
		if reviewTestContains(f.Detail, "bikedekho") {
			foundWarning = true
		}
	}
	if !foundBlock {
		t.Error("expected flag for 'guaranteed' (case-insensitive)")
	}
	if !foundWarning {
		t.Error("expected flag for 'bikedekho' (case-insensitive)")
	}
}

func reviewTestContains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && strings.Contains(s, sub)
}
