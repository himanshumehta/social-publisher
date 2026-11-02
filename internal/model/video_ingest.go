package model

import "github.com/google/uuid"

// VideoIngestMetadata is the JSON "metadata" part of
// POST /api/v1/external/videos. It becomes one post with a linked video.
type VideoIngestMetadata struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// TargetPlatforms are social-publisher platform names (see
	// VideoCaptionLimits); one post fans out to all of them.
	TargetPlatforms []string `json:"target_platforms"`
	// PlatformPosts optionally overrides the caption per platform. A target
	// without an entry uses Description, or Title when that is empty.
	PlatformPosts map[string]*IngestPlatformText `json:"platform_posts,omitempty"`
	Tags          []string                       `json:"tags,omitempty"`
}

// IngestPlatformText is the caption for one platform of an ingested video.
type IngestPlatformText struct {
	Text     string   `json:"text"`
	Hashtags []string `json:"hashtags,omitempty"`
}

// VideoIngestResult is the response body of the ingest endpoint, for both
// the first request (201) and an idempotent replay (200, Replayed=true).
type VideoIngestResult struct {
	PostID   uuid.UUID `json:"post_id"`
	VideoID  uuid.UUID `json:"video_id"`
	Status   string    `json:"status"`
	Replayed bool      `json:"replayed"`
}

// VideoCaptionLimits is the caption/description character limit of every
// platform a video post can target. It is PlatformCharLimits plus YouTube,
// whose video description limit is 5,000 characters; text posts cannot
// target YouTube, so it is kept out of PlatformCharLimits.
var VideoCaptionLimits = func() map[string]int {
	m := map[string]int{"youtube": 5000}
	for p, n := range PlatformCharLimits {
		m[p] = n
	}
	return m
}()
