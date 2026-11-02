package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// SupportedPostPlatforms lists the platforms the text/image post pipeline can
// publish to. YouTube is handled by the video upload path (UploadOrchestrator).
var SupportedPostPlatforms = []string{"twitter", "threads", "facebook", "instagram", "linkedin"}

// SupportedVideoPlatforms lists the platforms the video upload path can publish to.
var SupportedVideoPlatforms = []string{"youtube", "instagram", "facebook", "threads", "twitter", "linkedin"}

// PublishRequest is the platform-agnostic input handed to a PlatformAdapter.
type PublishRequest struct {
	Platform    string
	PostID      uuid.UUID
	Channel     *model.Channel
	AccessToken string
	Text        string
	ImageURL    string
	// VideoKey is the storage key of the post's linked video, if any.
	VideoKey string
}

// PlatformAdapter publishes one piece of content to one platform and returns
// the platform-side content ID and its public URL.
type PlatformAdapter interface {
	Publish(ctx context.Context, req PublishRequest) (platformPostID, platformURL string, err error)
}

// PlatformAdapterFunc adapts a plain function to the PlatformAdapter interface.
type PlatformAdapterFunc func(ctx context.Context, req PublishRequest) (string, string, error)

// Publish calls f.
func (f PlatformAdapterFunc) Publish(ctx context.Context, req PublishRequest) (string, string, error) {
	return f(ctx, req)
}

// DryRunAdapter is a PlatformAdapter that never calls a platform API. It logs
// the publish it would have made and reports success with a synthetic ID, so
// the rest of the pipeline (status transitions, publish links, retries) runs
// exactly as it does in production.
type DryRunAdapter struct {
	Logger *slog.Logger
}

// dryRunCaptionPrefixLen is the number of characters of the caption logged.
const dryRunCaptionPrefixLen = 40

// Publish logs "would publish <platform> <post-id> <caption-prefix>".
func (d DryRunAdapter) Publish(_ context.Context, req PublishRequest) (string, string, error) {
	logger := d.Logger
	if logger == nil {
		logger = slog.Default()
	}
	prefix := captionPrefix(req.Text, dryRunCaptionPrefixLen)
	attrs := []any{"dry_run", true, "platform", req.Platform, "post_id", req.PostID}
	if req.VideoKey != "" {
		attrs = append(attrs, "video_key", req.VideoKey)
	}
	logger.Info(fmt.Sprintf("would publish %s %s %s", req.Platform, req.PostID, prefix), attrs...)
	id := fmt.Sprintf("dryrun-%s-%s", req.Platform, shortID(req.PostID))
	return id, fmt.Sprintf("https://dry-run.invalid/%s/%s", req.Platform, id), nil
}

// dryRunAdapters binds every listed platform to the same DryRunAdapter.
func dryRunAdapters(logger *slog.Logger, platformSets ...[]string) map[string]PlatformAdapter {
	adapter := DryRunAdapter{Logger: logger}
	m := make(map[string]PlatformAdapter)
	for _, platforms := range platformSets {
		for _, p := range platforms {
			m[p] = adapter
		}
	}
	return m
}

// captionPrefix returns the first n runes of text on a single line,
// with an ellipsis when truncated.
func captionPrefix(text string, n int) string {
	s := strings.Join(strings.Fields(text), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "..."
}

func shortID(id uuid.UUID) string {
	return id.String()[:8]
}
