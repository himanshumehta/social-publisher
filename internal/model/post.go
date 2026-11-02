package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Post represents a publish-ready piece of content that can be distributed
// across multiple social media platforms.
type Post struct {
	ID         uuid.UUID  `json:"id"`
	CategoryID *uuid.UUID `json:"category_id,omitempty"`
	ProjectID  *uuid.UUID `json:"project_id,omitempty"`
	AuthorID   uuid.UUID  `json:"author_id"`

	// Core content
	Title         string `json:"title"`
	Slug          string `json:"slug"`
	Content       string `json:"content,omitempty"`
	Excerpt       string `json:"excerpt,omitempty"`
	CoverImageURL string `json:"cover_image_url,omitempty"`
	SubjectName   string `json:"subject_name,omitempty"`
	SubjectType   string `json:"subject_type,omitempty"`

	// Source URL scanning
	SourceURL      string          `json:"source_url,omitempty"`
	SourceMetadata *ScannedContent `json:"source_metadata,omitempty"`

	// AI generation
	AIProvider    string     `json:"ai_provider,omitempty"`
	AIModel       string     `json:"ai_model,omitempty"`
	AIPrompt      string     `json:"ai_prompt,omitempty"`
	AIGeneratedAt *time.Time `json:"ai_generated_at,omitempty"`
	IsAIGenerated bool       `json:"is_ai_generated"`

	// Platform-specific generated content
	PlatformPosts   map[string]*PlatformPost `json:"platform_posts,omitempty"`
	TargetPlatforms []string                 `json:"target_platforms"`

	// Video linking
	LinkedVideoURL string     `json:"linked_video_url,omitempty"`
	LinkedVideoID  *uuid.UUID `json:"linked_video_id,omitempty"`

	// Generation settings
	Tone       string `json:"tone"`
	BrandVoice string `json:"brand_voice,omitempty"`

	// System review
	SystemReviewStatus string     `json:"system_review_status"`
	SystemReviewNotes  string     `json:"system_review_notes,omitempty"`
	SystemReviewAt     *time.Time `json:"system_review_at,omitempty"`

	// Pipeline state
	Status              string     `json:"status"`
	ErrorMessage        string     `json:"error_message,omitempty"`
	RetryCount          int        `json:"retry_count"`
	ProcessingStartedAt *time.Time `json:"processing_started_at,omitempty"`
	AutoPublish         bool       `json:"auto_publish"`

	// Scheduling
	ScheduledAt       *time.Time `json:"scheduled_at,omitempty"`
	Timezone          string     `json:"timezone,omitempty"`
	PreScheduleStatus string     `json:"-"` // internal use only, not exposed via API

	// Brand Kit
	BrandKitID *uuid.UUID `json:"brand_kit_id,omitempty"`

	// Publish results
	PublishResults []PostPublishResult `json:"publish_results,omitempty"`

	// SEO
	MetaTitle       string   `json:"meta_title,omitempty"`
	MetaDescription string   `json:"meta_description,omitempty"`
	Tags            []string `json:"tags,omitempty"`

	// Timestamps
	PublishedAt *time.Time `json:"published_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ScannedContent holds structured data extracted from a source URL.
type ScannedContent struct {
	Title         string            `json:"title"`
	Description   string            `json:"description"`
	BodyText      string            `json:"body_text"`
	CoverImageURL string            `json:"cover_image_url"`
	Images        []string          `json:"images"`
	OGTags        map[string]string `json:"og_tags"`
	Author        string            `json:"author"`
	PublishedDate *time.Time        `json:"published_date,omitempty"`
	SiteName      string            `json:"site_name"`
	WordCount     int               `json:"word_count"`
	ScannedAt     time.Time         `json:"scanned_at"`
}

// PlatformPost holds the generated content for a single platform.
type PlatformPost struct {
	Platform   string   `json:"platform"`
	Text       string   `json:"text"`
	Hashtags   []string `json:"hashtags"`
	CharCount  int      `json:"char_count"`
	CharLimit  int      `json:"char_limit"`
	ImageURL   string   `json:"image_url,omitempty"`
	VideoURL   string   `json:"video_url,omitempty"`
	CTALink    string   `json:"cta_link,omitempty"`
	IsEdited   bool     `json:"is_edited"`
	EditedText string   `json:"edited_text,omitempty"`
}

// PostPublishResult tracks the outcome of publishing to a single platform.
type PostPublishResult struct {
	Platform       string     `json:"platform"`
	PlatformPostID string     `json:"platform_post_id,omitempty"`
	PlatformURL    string     `json:"platform_url,omitempty"`
	ChannelID      string     `json:"channel_id"`
	ChannelName    string     `json:"channel_name"`
	Status         string     `json:"status"`
	Error          string     `json:"error,omitempty"`
	PublishedAt    *time.Time `json:"published_at,omitempty"`
}

// PostPublishLink is the DB record linking a post to its platform publication.
type PostPublishLink struct {
	ID              uuid.UUID  `json:"id"`
	PostID          uuid.UUID  `json:"post_id"`
	PublishRecordID *uuid.UUID `json:"publish_record_id,omitempty"`
	Platform        string     `json:"platform"`
	ChannelID       *uuid.UUID `json:"channel_id,omitempty"`
	PlatformPostID  string     `json:"platform_post_id,omitempty"`
	PlatformURL     string     `json:"platform_url,omitempty"`
	Status          string     `json:"status"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	PublishedAt     *time.Time `json:"published_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// PostListItem is a lightweight view for list responses.
type PostListItem struct {
	ID                 uuid.UUID           `json:"id"`
	Title              string              `json:"title"`
	SourceURL          string              `json:"source_url,omitempty"`
	CoverImageURL      string              `json:"cover_image_url,omitempty"`
	TargetPlatforms    []string            `json:"target_platforms"`
	Status             string              `json:"status"`
	SystemReviewStatus string              `json:"system_review_status"`
	AuthorName         string              `json:"author_name"`
	PublishedAt        *time.Time          `json:"published_at,omitempty"`
	ScheduledAt        *time.Time          `json:"scheduled_at,omitempty"`
	Timezone           string              `json:"timezone,omitempty"`
	CreatedAt          time.Time           `json:"created_at"`
	PublishResults     []PostPublishResult `json:"publish_results,omitempty"`
}

// CreatePostRequest is the input for creating a new post.
type CreatePostRequest struct {
	SourceURL       string                       `json:"source_url,omitempty"`
	Title           string                       `json:"title,omitempty"`
	Content         string                       `json:"content,omitempty"`
	Excerpt         string                       `json:"excerpt,omitempty"`
	TargetPlatforms []string                     `json:"target_platforms"`
	LinkedVideoURL  string                       `json:"linked_video_url,omitempty"`
	CoverImageURL   string                       `json:"cover_image_url,omitempty"`
	Tone            string                       `json:"tone,omitempty"`
	BrandVoice      string                       `json:"brand_voice,omitempty"`
	AutoPublish     bool                         `json:"auto_publish"`
	CategoryID      string                       `json:"category_id,omitempty"`
	ProjectID       string                       `json:"project_id,omitempty"`
	Tags            []string                     `json:"tags,omitempty"`
	PlatformPosts   map[string]*PlatformPostEdit `json:"platform_posts,omitempty"`
}

// UpdatePostRequest allows editing generated content before publishing.
type UpdatePostRequest struct {
	Title           *string                      `json:"title,omitempty"`
	PlatformPosts   map[string]*PlatformPostEdit `json:"platform_posts,omitempty"`
	TargetPlatforms *[]string                    `json:"target_platforms,omitempty"`
	CoverImageURL   *string                      `json:"cover_image_url,omitempty"`
	Tags            *[]string                    `json:"tags,omitempty"`
}

// PlatformPostEdit is the editable subset of a PlatformPost.
type PlatformPostEdit struct {
	Text     string   `json:"text"`
	Hashtags []string `json:"hashtags,omitempty"`
	ImageURL string   `json:"image_url,omitempty"`
}

// AdminRejectRequest provides rejection context.
type AdminRejectRequest struct {
	Reason string `json:"reason"`
}

// PostListFilter supports paginated, filtered listing.
type PostListFilter struct {
	Status    string `json:"status,omitempty"`
	Platform  string `json:"platform,omitempty"`
	AuthorID  string `json:"author_id,omitempty"`
	Search    string `json:"search,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"page_size,omitempty"`
	SortBy    string `json:"sort_by,omitempty"`
	SortOrder string `json:"sort_order,omitempty"`
}

// MarshalPlatformPosts converts the map to JSONB for storage.
func MarshalPlatformPosts(posts map[string]*PlatformPost) ([]byte, error) {
	if posts == nil {
		return nil, nil
	}
	return json.Marshal(posts)
}

// UnmarshalPlatformPosts converts JSONB back to the map.
func UnmarshalPlatformPosts(data []byte) (map[string]*PlatformPost, error) {
	if data == nil {
		return nil, nil
	}
	var posts map[string]*PlatformPost
	err := json.Unmarshal(data, &posts)
	return posts, err
}

// PlatformCharLimits defines the character limits per platform.
var PlatformCharLimits = map[string]int{
	"twitter":   280,
	"threads":   500,
	"facebook":  63206,
	"instagram": 2200,
	"linkedin":  3000,
}

// PlatformSupportsImages returns whether a platform supports image attachments.
var PlatformSupportsImages = map[string]bool{
	"twitter":   true,
	"threads":   true,
	"facebook":  true,
	"instagram": true,
	"linkedin":  true,
}
