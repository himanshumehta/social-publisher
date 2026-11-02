package service

import (
	"bytes"
	"fmt"
	"image"
	_ "image/gif" // register decoders used by checkImageDimensions
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"time"

	"github.com/himanshumehta/social-publisher/internal/netguard"
)

// Limits for media fetched from user-supplied URLs (post images, thumbnails).
const (
	mediaFetchTimeout = 30 * time.Second
	maxThumbnailBytes = 10 << 20 // 10 MiB
	maxPostImageBytes = 20 << 20 // 20 MiB
	// maxDecodedImagePixels caps width*height before an image is decoded, so
	// a small file that declares huge dimensions cannot exhaust memory.
	maxDecodedImagePixels = 40_000_000
)

// newMediaClient returns the HTTP client for user-supplied media URLs. It
// refuses non-public destinations unless allowPrivate is set.
func newMediaClient(allowPrivate bool) *http.Client {
	return netguard.NewClient(mediaFetchTimeout, allowPrivate)
}

// checkImageDimensions rejects images whose header declares more than
// maxDecodedImagePixels, before any full decode.
func checkImageDimensions(data []byte) error {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("read image header: %w", err)
	}
	if int64(cfg.Width)*int64(cfg.Height) > maxDecodedImagePixels {
		return fmt.Errorf("image is %dx%d, over the %d pixel limit", cfg.Width, cfg.Height, maxDecodedImagePixels)
	}
	return nil
}
