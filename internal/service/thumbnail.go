package service

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/image/draw"

	"github.com/himanshumehta/social-publisher/internal/infra"
)

const (
	thumbnailWidth   = 320
	thumbnailHeight  = 180
	thumbnailQuality = 85
)

type ThumbnailService struct {
	pool *pgxpool.Pool
	s3   *infra.S3Client
}

func NewThumbnailService(pool *pgxpool.Pool, s3 *infra.S3Client) *ThumbnailService {
	return &ThumbnailService{pool: pool, s3: s3}
}

// GenerateThumbnail generates a thumbnail for a video or photo and uploads it to S3.
// For videos, it uses a presigned URL so ffmpeg reads directly from S3 (no full download).
// For images, it downloads the file since they are small enough to fit in memory.
func (s *ThumbnailService) GenerateThumbnail(ctx context.Context, videoID uuid.UUID) error {
	// Get video details
	var storageKey, mimeType, fileName string
	err := s.pool.QueryRow(ctx,
		`SELECT storage_key, mime_type, file_name FROM videos WHERE id = $1`,
		videoID).Scan(&storageKey, &mimeType, &fileName)
	if err != nil {
		return fmt.Errorf("get video details: %w", err)
	}

	// Determine if it's a video or photo
	isVideo := strings.HasPrefix(mimeType, "video/")
	isImage := strings.HasPrefix(mimeType, "image/")

	if !isVideo && !isImage {
		return fmt.Errorf("unsupported mime type for thumbnail generation: %s", mimeType)
	}

	var thumbnailData []byte
	if isVideo {
		// For videos: use presigned URL so ffmpeg reads directly from S3
		presignedURL, err := s.s3.PresignGetObject(ctx, storageKey, 15*time.Minute)
		if err != nil {
			return fmt.Errorf("presign video URL: %w", err)
		}
		thumbnailData, err = generateVideoThumbnailFromURL(presignedURL)
		if err != nil {
			return fmt.Errorf("generate video thumbnail: %w", err)
		}
	} else {
		// For images: download the file (images are small, typically < 20MB)
		fileData, err := s.s3.GetObject(ctx, storageKey)
		if err != nil {
			return fmt.Errorf("download image from S3: %w", err)
		}
		thumbnailData, err = generatePhotoThumbnail(fileData, mimeType)
		if err != nil {
			return fmt.Errorf("generate photo thumbnail: %w", err)
		}
	}

	// Generate thumbnail storage key
	thumbnailKey := generateThumbnailKey(storageKey)

	// Upload thumbnail to S3
	_, err = s.s3.UploadObject(ctx, thumbnailKey, "image/jpeg", thumbnailData)
	if err != nil {
		return fmt.Errorf("upload thumbnail to S3: %w", err)
	}

	// Update video record with thumbnail info
	_, err = s.pool.Exec(ctx,
		`UPDATE videos SET thumbnail_key = $1, updated_at = NOW()
		 WHERE id = $2`,
		thumbnailKey, videoID)
	if err != nil {
		return fmt.Errorf("update video with thumbnail: %w", err)
	}

	return nil
}

// generateVideoThumbnailFromURL extracts a frame from a video using ffmpeg with a presigned URL.
// ffmpeg reads directly from the URL, so we don't need to download the entire video file.
func generateVideoThumbnailFromURL(videoURL string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "thumbnail-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	framePath := filepath.Join(tmpDir, "frame.jpg")
	cmd := exec.Command("ffmpeg",
		"-ss", "00:00:01.000",
		"-i", videoURL,
		"-vframes", "1",
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black", thumbnailWidth, thumbnailHeight, thumbnailWidth, thumbnailHeight),
		"-q:v", "2",
		framePath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ffmpeg error: %w, output: %s", err, string(output))
	}

	thumbnailData, err := os.ReadFile(framePath)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail: %w", err)
	}

	return thumbnailData, nil
}

// generatePhotoThumbnail creates a thumbnail from a photo, optionally removing background using rembg.
func generatePhotoThumbnail(imageData []byte, mimeType string) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "thumbnail-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Determine file extension
	ext := ".jpg"
	if strings.Contains(mimeType, "png") {
		ext = ".png"
	}

	// Write original image
	inputPath := filepath.Join(tmpDir, "input"+ext)
	if err := os.WriteFile(inputPath, imageData, 0644); err != nil {
		return nil, fmt.Errorf("write input image: %w", err)
	}

	// Try to remove background using rembg (if available)
	processedPath := filepath.Join(tmpDir, "processed.png")
	cmd := exec.Command("rembg", "i", inputPath, processedPath)
	if err := cmd.Run(); err != nil {
		// If rembg fails or is not available, use original image
		processedPath = inputPath
	}

	// Read the processed image
	processedData, err := os.ReadFile(processedPath)
	if err != nil {
		return nil, fmt.Errorf("read processed image: %w", err)
	}

	// Decode image
	img, _, err := image.Decode(bytes.NewReader(processedData))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	// Create thumbnail with proper aspect ratio
	thumbnail := resizeImage(img, thumbnailWidth, thumbnailHeight)

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, thumbnail, &jpeg.Options{Quality: thumbnailQuality}); err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w", err)
	}

	return buf.Bytes(), nil
}

// resizeImage resizes an image to fit within the specified dimensions while maintaining aspect ratio.
func resizeImage(src image.Image, maxWidth, maxHeight int) image.Image {
	bounds := src.Bounds()
	srcWidth := bounds.Dx()
	srcHeight := bounds.Dy()

	// Calculate scaling factor
	scaleX := float64(maxWidth) / float64(srcWidth)
	scaleY := float64(maxHeight) / float64(srcHeight)
	scale := scaleX
	if scaleY < scaleX {
		scale = scaleY
	}

	// Calculate new dimensions
	dstWidth := int(float64(srcWidth) * scale)
	dstHeight := int(float64(srcHeight) * scale)

	// Create destination image
	dst := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))

	// Resize using high-quality algorithm
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)

	return dst
}

// generateThumbnailKey creates a storage key for the thumbnail based on the original video key.
func generateThumbnailKey(originalKey string) string {
	ext := filepath.Ext(originalKey)
	baseKey := strings.TrimSuffix(originalKey, ext)
	return baseKey + "_thumb.jpg"
}
