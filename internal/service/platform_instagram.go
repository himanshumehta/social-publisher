package service

import (
	"context"

	"github.com/google/uuid"
)

type InstagramUploadService struct {
	container *MetaContainerUploader
	config    MetaContainerConfig
}

type InstagramUploadMeta struct {
	Caption      string
	ShareToFeed  bool
	ThumbnailURL string // presigned S3 URL for cover image
}

func NewInstagramUploadService(container *MetaContainerUploader) *InstagramUploadService {
	return &InstagramUploadService{
		container: container,
		config: MetaContainerConfig{
			BaseURL:           "graph.instagram.com",
			APIVersion:        "v21.0",
			UserIDField:       "instagram_business_account_id",
			MediaTypeField:    "REELS",
			CaptionField:      "caption",
			ContainerEndpoint: "media",
			StatusFields:      "status_code,status",
			PublishEndpoint:   "media_publish",
			Platform:          "instagram",
		},
	}
}

// Upload starts an Instagram Reel upload via the container flow.
func (s *InstagramUploadService) Upload(ctx context.Context, channelID, versionID, publishRecordID uuid.UUID, meta InstagramUploadMeta) (string, error) {
	return s.container.Upload(ctx, s.config, channelID, versionID, publishRecordID, meta.Caption, meta.ThumbnailURL)
}

// PollStatus checks the container processing status.
func (s *InstagramUploadService) PollStatus(ctx context.Context, containerID, token string) (string, error) {
	return s.container.PollStatus(ctx, s.config, containerID, token)
}

// Publish publishes a finished container.
func (s *InstagramUploadService) Publish(ctx context.Context, userID, containerID, token string) (*MetaPublishResult, error) {
	return s.container.PublishContainer(ctx, s.config, userID, containerID, token)
}
