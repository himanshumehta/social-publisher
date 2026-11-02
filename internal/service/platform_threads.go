package service

import (
	"context"

	"github.com/google/uuid"
)

type ThreadsUploadService struct {
	container *MetaContainerUploader
	config    MetaContainerConfig
}

type ThreadsUploadMeta struct {
	Text string // Threads uses "text" not "caption"
}

func NewThreadsUploadService(container *MetaContainerUploader) *ThreadsUploadService {
	return &ThreadsUploadService{
		container: container,
		config: MetaContainerConfig{
			BaseURL:           "graph.threads.net",
			APIVersion:        "v1.0",
			UserIDField:       "threads_user_id",
			MediaTypeField:    "VIDEO",
			CaptionField:      "text",
			ContainerEndpoint: "threads",
			StatusFields:      "status,error_message",
			PublishEndpoint:   "threads_publish",
			Platform:          "threads",
		},
	}
}

// Upload starts a Threads video upload via the container flow.
func (s *ThreadsUploadService) Upload(ctx context.Context, channelID, versionID, publishRecordID uuid.UUID, meta ThreadsUploadMeta) (string, error) {
	return s.container.Upload(ctx, s.config, channelID, versionID, publishRecordID, meta.Text, "")
}

// PollStatus checks the container processing status.
func (s *ThreadsUploadService) PollStatus(ctx context.Context, containerID, token string) (string, error) {
	return s.container.PollStatus(ctx, s.config, containerID, token)
}

// Publish publishes a finished container.
func (s *ThreadsUploadService) Publish(ctx context.Context, userID, containerID, token string) (*MetaPublishResult, error) {
	return s.container.PublishContainer(ctx, s.config, userID, containerID, token)
}
