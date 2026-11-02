package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"image"
	"image/png"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/himanshumehta/social-publisher/internal/netguard"
)

// The scanner fetches the user-supplied source_url. It must not reach
// loopback/private services (here: an httptest server on 127.0.0.1).
func TestPostScanner_RefusesPrivateSourceURL(t *testing.T) {
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html><title>internal admin</title></html>"))
	}))
	defer internal.Close()

	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1"}) // cache miss path
	defer rdb.Close()
	scanner := NewPostScanner(nil, rdb, slog.Default())

	_, err := scanner.Scan(context.Background(), internal.URL)
	if !errors.Is(err, netguard.ErrBlockedAddress) {
		t.Fatalf("Scan(%s) err = %v, want ErrBlockedAddress", internal.URL, err)
	}
	for _, bad := range []string{"file:///etc/passwd", "gopher://127.0.0.1:6379/_FLUSHALL"} {
		if _, err := scanner.Scan(context.Background(), bad); err == nil {
			t.Errorf("Scan(%q) accepted", bad)
		}
	}
}

func TestCheckImageDimensions(t *testing.T) {
	var small bytes.Buffer
	_ = png.Encode(&small, image.NewRGBA(image.Rect(0, 0, 10, 10)))
	if err := checkImageDimensions(small.Bytes()); err != nil {
		t.Fatalf("small image rejected: %v", err)
	}
	// A PNG header that declares 100000x100000 pixels (a decompression bomb
	// shape); DecodeConfig reads only the header.
	var huge bytes.Buffer
	_ = png.Encode(&huge, image.NewRGBA(image.Rect(0, 0, 1, 1)))
	b := huge.Bytes()
	// IHDR width/height live at bytes 16..23; the chunk CRC (type+data)
	// follows at 29..32 and must be recomputed for the header to parse.
	copy(b[16:24], []byte{0, 1, 0x86, 0xA0, 0, 1, 0x86, 0xA0})
	binary.BigEndian.PutUint32(b[29:33], crc32.ChecksumIEEE(b[12:29]))
	err := checkImageDimensions(b)
	if err == nil || !strings.Contains(err.Error(), "pixel limit") {
		t.Fatalf("oversized image header: err = %v, want pixel limit error", err)
	}
}
