package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
)

// ErrIdempotencyConflict means the Idempotency-Key was already used by the
// same owner for a request with a different payload.
var ErrIdempotencyConflict = errors.New("idempotency key was already used with a different payload")

// ErrInvalidIngest wraps every metadata validation failure, so the handler can
// map it to 400 without string matching.
var ErrInvalidIngest = errors.New("invalid ingest request")

const (
	maxIngestTitleChars = 300
	maxIngestTags       = 30
	maxIngestTagChars   = 100
	maxIngestHashtags   = 30
	// IngestRetention is how long an Idempotency-Key is remembered. The
	// worker's cleanup loop deletes older ingest_requests rows; a replay after
	// that creates a new post.
	IngestRetention = 30 * 24 * time.Hour
)

// ObjectStore is the slice of the S3 client the ingest path needs.
type ObjectStore interface {
	PutObject(ctx context.Context, key, contentType string, body io.ReadSeeker, size int64) error
	DeleteObject(ctx context.Context, key string) error
}

// IngestFile is one uploaded file, already spooled, size-checked, hashed and
// sniffed by the handler.
type IngestFile struct {
	Body        io.ReadSeeker
	Size        int64
	SHA256      string // hex digest of the bytes in Body
	ContentType string // allow-listed and confirmed by content sniffing
}

// VideoIngestRequest is a validated ingest call.
type VideoIngestRequest struct {
	IdempotencyKey string
	Metadata       model.VideoIngestMetadata
	Video          IngestFile
	Thumbnail      *IngestFile
}

// VideoIngestService turns an uploaded video into a project, a video row and
// a post that waits for admin review. It never approves anything itself.
type VideoIngestService struct {
	pool   *pgxpool.Pool
	store  ObjectStore
	posts  *PostService
	logger *slog.Logger
}

func NewVideoIngestService(pool *pgxpool.Pool, store ObjectStore, posts *PostService, logger *slog.Logger) *VideoIngestService {
	return &VideoIngestService{pool: pool, store: store, posts: posts, logger: logger}
}

// NormalizeIngestMetadata trims and validates m in place: a title, at least
// one supported video platform (deduplicated, order kept), and a non-empty
// caption within each platform's limit. Captions missing from PlatformPosts
// default to the description, then the title.
func NormalizeIngestMetadata(m *model.VideoIngestMetadata) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidIngest, fmt.Sprintf(format, args...))
	}
	m.Title = strings.TrimSpace(m.Title)
	m.Description = strings.TrimSpace(m.Description)
	if m.Title == "" {
		return invalid("title is required")
	}
	if utf8.RuneCountInString(m.Title) > maxIngestTitleChars {
		return invalid("title is longer than %d characters", maxIngestTitleChars)
	}
	if len(m.TargetPlatforms) == 0 {
		return invalid("at least one target platform is required")
	}
	seen := make(map[string]bool, len(m.TargetPlatforms))
	platforms := make([]string, 0, len(m.TargetPlatforms))
	for _, p := range m.TargetPlatforms {
		p = strings.ToLower(strings.TrimSpace(p))
		if _, ok := model.VideoCaptionLimits[p]; !ok {
			return invalid("unsupported platform %q", p)
		}
		if !seen[p] {
			seen[p] = true
			platforms = append(platforms, p)
		}
	}
	m.TargetPlatforms = platforms
	for p := range m.PlatformPosts {
		if !seen[p] {
			return invalid("platform_posts has %q, which is not a target platform", p)
		}
	}
	if len(m.Tags) > maxIngestTags {
		return invalid("at most %d tags", maxIngestTags)
	}
	for _, t := range m.Tags {
		if utf8.RuneCountInString(t) > maxIngestTagChars {
			return invalid("tag longer than %d characters", maxIngestTagChars)
		}
	}

	defaultText := m.Description
	if defaultText == "" {
		defaultText = m.Title
	}
	posts := make(map[string]*model.IngestPlatformText, len(platforms))
	for _, p := range platforms {
		pt := &model.IngestPlatformText{Text: defaultText}
		if given := m.PlatformPosts[p]; given != nil {
			pt = &model.IngestPlatformText{Text: strings.TrimSpace(given.Text), Hashtags: given.Hashtags}
		}
		if pt.Text == "" {
			return invalid("%s caption is empty", p)
		}
		if n, limit := utf8.RuneCountInString(pt.Text), model.VideoCaptionLimits[p]; n > limit {
			return invalid("%s caption is %d characters, limit is %d", p, n, limit)
		}
		if len(pt.Hashtags) > maxIngestHashtags {
			return invalid("%s has more than %d hashtags", p, maxIngestHashtags)
		}
		posts[p] = pt
	}
	m.PlatformPosts = posts
	return nil
}

// IngestRequestHash is the payload fingerprint stored with an Idempotency-Key:
// sha256 over the normalized metadata (encoding/json sorts map keys, so the
// encoding is canonical) and the digests of the streamed files.
func IngestRequestHash(m *model.VideoIngestMetadata, videoSHA256, thumbnailSHA256 string) (string, error) {
	meta, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, part := range [][]byte{meta, []byte(videoSHA256), []byte(thumbnailSHA256)} {
		fmt.Fprintf(h, "%d:", len(part))
		h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Ingest stores the files and creates the project, video and post in one
// transaction, keyed by (owner, Idempotency-Key):
//
//   - a key seen before with the same payload returns the original IDs with
//     Replayed=true and uploads nothing;
//   - a key seen before with another payload returns ErrIdempotencyConflict;
//   - two concurrent requests with one key both upload under their own object
//     keys, but only one claim row can commit; the other rolls back, deletes
//     its objects and answers as a replay (or a conflict).
//
// The post is created in pending_review (or flagged by the automated review),
// so it is published only after an admin approves it.
func (s *VideoIngestService) Ingest(ctx context.Context, ownerID uuid.UUID, req *VideoIngestRequest) (*model.VideoIngestResult, error) {
	thumbSHA := ""
	if req.Thumbnail != nil {
		thumbSHA = req.Thumbnail.SHA256
	}
	reqHash, err := IngestRequestHash(&req.Metadata, req.Video.SHA256, thumbSHA)
	if err != nil {
		return nil, fmt.Errorf("hash request: %w", err)
	}

	if prev, err := s.lookup(ctx, ownerID, req.IdempotencyKey, reqHash); err != nil || prev != nil {
		return prev, err
	}

	postID, videoID, projectID := uuid.New(), uuid.New(), uuid.New()
	// Object keys are server-generated and unique per attempt; nothing from
	// the client (file names, the idempotency key) reaches the key.
	prefix := fmt.Sprintf("ingest/%s/%s", ownerID, postID)
	videoKey := prefix + "/video.mp4"
	var thumbKey string

	var stored []string
	discard := func() {
		// Use a fresh context: the request context may be what failed.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		for _, k := range stored {
			if err := s.store.DeleteObject(ctx, k); err != nil {
				s.logger.Warn("ingest: could not delete orphaned object", "key", k, "error", err)
			}
		}
	}

	if err := s.put(ctx, videoKey, &req.Video); err != nil {
		return nil, err
	}
	stored = append(stored, videoKey)
	if req.Thumbnail != nil {
		thumbKey = prefix + "/thumbnail" + imageExt(req.Thumbnail.ContentType)
		if err := s.put(ctx, thumbKey, req.Thumbnail); err != nil {
			discard()
			return nil, err
		}
		stored = append(stored, thumbKey)
	}

	status, err := s.createRecords(ctx, ownerID, req, reqHash, postID, videoID, projectID, videoKey, thumbKey)
	if errors.Is(err, errIngestKeyTaken) {
		// A concurrent request with the same key committed first.
		discard()
		prev, err := s.lookup(ctx, ownerID, req.IdempotencyKey, reqHash)
		if err == nil && prev == nil {
			err = fmt.Errorf("ingest: idempotency row vanished after conflict")
		}
		return prev, err
	}
	if err != nil {
		discard()
		return nil, err
	}

	s.logger.Info("video ingested",
		"post_id", postID,
		"video_id", videoID,
		"owner_id", ownerID,
		"status", status,
		"platforms", req.Metadata.TargetPlatforms,
		"video_bytes", req.Video.Size,
	)
	return &model.VideoIngestResult{PostID: postID, VideoID: videoID, Status: status}, nil
}

var errIngestKeyTaken = errors.New("idempotency key taken by a concurrent request")

func (s *VideoIngestService) put(ctx context.Context, key string, f *IngestFile) error {
	if _, err := f.Body.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("rewind upload: %w", err)
	}
	if err := s.store.PutObject(ctx, key, f.ContentType, f.Body, f.Size); err != nil {
		return fmt.Errorf("store upload: %w", err)
	}
	return nil
}

// lookup returns the stored result for (owner, key), nil when the key is new,
// or ErrIdempotencyConflict when it was used for a different payload.
func (s *VideoIngestService) lookup(ctx context.Context, ownerID uuid.UUID, key, reqHash string) (*model.VideoIngestResult, error) {
	var storedHash string
	res := &model.VideoIngestResult{Replayed: true}
	err := s.pool.QueryRow(ctx, `
		SELECT ir.request_hash, ir.post_id, ir.video_id, p.status::text
		FROM ingest_requests ir
		JOIN posts p ON p.id = ir.post_id
		WHERE ir.owner_id = $1 AND ir.idempotency_key = $2`,
		ownerID, key,
	).Scan(&storedHash, &res.PostID, &res.VideoID, &res.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("look up idempotency key: %w", err)
	}
	if storedHash != reqHash {
		return nil, ErrIdempotencyConflict
	}
	return res, nil
}

// createRecords inserts the project, video, post and idempotency claim in one
// transaction and returns the post status. The claim is inserted last with
// ON CONFLICT DO NOTHING: a concurrent insert of the same (owner, key) blocks
// on the unique index until the other transaction ends, and then returns no
// row, which is reported as errIngestKeyTaken.
func (s *VideoIngestService) createRecords(
	ctx context.Context, ownerID uuid.UUID, req *VideoIngestRequest, reqHash string,
	postID, videoID, projectID uuid.UUID, videoKey, thumbKey string,
) (string, error) {
	meta := &req.Metadata
	platformPosts := make(map[string]*model.PlatformPost, len(meta.PlatformPosts))
	for p, pt := range meta.PlatformPosts {
		platformPosts[p] = &model.PlatformPost{
			Platform:  p,
			Text:      pt.Text,
			Hashtags:  pt.Hashtags,
			CharCount: utf8.RuneCountInString(pt.Text),
			CharLimit: model.VideoCaptionLimits[p],
		}
	}
	post := &model.Post{
		ID:              postID,
		Title:           meta.Title,
		Content:         meta.Description,
		TargetPlatforms: meta.TargetPlatforms,
		LinkedVideoID:   &videoID,
	}

	// The automated review runs before the insert, so the post is created
	// directly in its review state and no window exists in which an admin
	// approval could be overwritten by a later review update.
	status, reviewStatus, reviewNotes := "pending_review", "pending", ""
	var reviewedAt *time.Time
	if s.posts != nil && s.posts.reviewer != nil {
		result := s.posts.reviewer.Review(ctx, post, platformPosts)
		rs, notes, at := ReviewResultToDBFields(result)
		reviewStatus, reviewNotes, reviewedAt = rs, notes, &at
		if rs == "blocked" {
			status = "flagged"
		}
	}

	platformPostsJSON, err := json.Marshal(platformPosts)
	if err != nil {
		return "", fmt.Errorf("encode platform posts: %w", err)
	}
	slug := generatePostSlug(meta.Title) + "-" + shortID(postID)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO projects (id, title, slug, description, created_by, status)
		VALUES ($1, $2, $3, $4, $5, 'active')`,
		projectID, meta.Title, "ingest-"+postID.String(), "Created by video ingest", ownerID,
	); err != nil {
		return "", fmt.Errorf("insert project: %w", err)
	}

	var thumb *string
	if thumbKey != "" {
		thumb = &thumbKey
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO videos (id, project_id, uploaded_by, format, version, storage_key, storage_type,
			file_name, file_size_bytes, mime_type, thumbnail_key, status)
		VALUES ($1, $2, $3, 'raw', 1, $4, 'spaces', 'video.mp4', $5, $6, $7, 'ready')`,
		videoID, projectID, ownerID, videoKey, req.Video.Size, req.Video.ContentType, thumb,
	); err != nil {
		return "", fmt.Errorf("insert video: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO posts (
			id, project_id, author_id, title, slug, content, target_platforms,
			platform_posts, linked_video_id, tags, status,
			system_review_status, system_review_notes, system_review_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::post_status, $12, $13, $14)`,
		postID, projectID, ownerID, meta.Title, slug, nilIfEmptyPtr(meta.Description),
		meta.TargetPlatforms, platformPostsJSON, videoID, meta.Tags, status,
		reviewStatus, nilIfEmptyPtr(reviewNotes), reviewedAt,
	); err != nil {
		return "", fmt.Errorf("insert post: %w", err)
	}

	var claimID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO ingest_requests (owner_id, idempotency_key, request_hash, post_id, video_id)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT ON CONSTRAINT ingest_requests_owner_key_unique DO NOTHING
		RETURNING id`,
		ownerID, req.IdempotencyKey, reqHash, postID, videoID,
	).Scan(&claimID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", errIngestKeyTaken
	}
	if err != nil {
		return "", fmt.Errorf("claim idempotency key: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit: %w", err)
	}
	return status, nil
}

// ExpireIngestRequests deletes idempotency rows older than IngestRetention and
// returns how many were removed. The posts and videos they point at stay.
func ExpireIngestRequests(ctx context.Context, pool *pgxpool.Pool) (int64, error) {
	tag, err := pool.Exec(ctx,
		`DELETE FROM ingest_requests WHERE created_at < NOW() - make_interval(secs => $1)`,
		IngestRetention.Seconds())
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func imageExt(contentType string) string {
	if contentType == "image/png" {
		return ".png"
	}
	return ".jpg"
}
