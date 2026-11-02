package handler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"time"

	"github.com/himanshumehta/social-publisher/internal/middleware"
	"github.com/himanshumehta/social-publisher/internal/model"
	"github.com/himanshumehta/social-publisher/internal/service"
)

const (
	// IngestThumbnailMaxBytes caps the optional thumbnail part.
	IngestThumbnailMaxBytes = 10 << 20
	// ingestMetadataMaxBytes caps the JSON metadata part.
	ingestMetadataMaxBytes = 64 << 10
	// ingestFormOverheadBytes covers multipart boundaries and part headers.
	ingestFormOverheadBytes = 1 << 20
	// idempotencyKeyMaxLen bounds the Idempotency-Key header.
	idempotencyKeyMaxLen = 200
	// ingestTransferTimeout replaces the server-wide read/write timeouts for
	// this route only, so a large upload on a slow link is not cut off.
	ingestTransferTimeout = 15 * time.Minute
	// sniffLen is how many leading bytes http.DetectContentType inspects.
	sniffLen = 512
)

// IngestBodyLimit is the request body cap for the ingest route: one video,
// one thumbnail and the form overhead.
func IngestBodyLimit(maxVideoBytes int64) int64 {
	return maxVideoBytes + IngestThumbnailMaxBytes + ingestFormOverheadBytes
}

// ingestFileRule is the allow-list for one file part: the declared part
// Content-Type and the type sniffed from its first bytes must both be in it.
type ingestFileRule struct {
	maxBytes int64
	types    map[string]bool
}

// errTooLarge and errUnsupportedType map to 413 and 415.
var (
	errTooLarge        = errors.New("too large")
	errUnsupportedType = errors.New("unsupported media type")
)

// ExternalIngestVideo handles POST /api/v1/external/videos (API key auth).
//
// multipart/form-data parts:
//
//	metadata   JSON model.VideoIngestMetadata (required, send it first)
//	video      video/mp4 (required)
//	thumbnail  image/jpeg or image/png (optional)
//
// The Idempotency-Key header is required. The response is 201 for a new
// post and 200 with "replayed": true when the same key and payload were
// already accepted; the same key with another payload is 409.
func (h *Handler) ExternalIngestVideo(w http.ResponseWriter, r *http.Request) {
	apiKeyCtx := middleware.GetAPIKey(r)
	if apiKeyCtx == nil {
		writeError(w, http.StatusUnauthorized, "invalid API key")
		return
	}
	if !apiKeyCtx.HasScope("posts:create") {
		writeError(w, http.StatusForbidden, "API key lacks posts:create scope")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !validIdempotencyKey(key) {
		writeError(w, http.StatusBadRequest,
			fmt.Sprintf("Idempotency-Key header is required (1-%d printable ASCII characters)", idempotencyKeyMaxLen))
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		writeError(w, http.StatusUnsupportedMediaType, "body must be multipart/form-data")
		return
	}

	// Best effort: wrappers that do not expose the connection keep the
	// server-wide deadlines.
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(ingestTransferTimeout))
	_ = rc.SetWriteDeadline(time.Now().Add(ingestTransferTimeout + time.Minute))

	req, cleanup, status, err := h.readIngestForm(r)
	defer cleanup()
	if err != nil {
		writeError(w, status, err.Error())
		return
	}

	res, err := h.services.VideoIngest.Ingest(r.Context(), apiKeyCtx.UserID, req)
	switch {
	case errors.Is(err, service.ErrIdempotencyConflict):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		h.logger.Error("video ingest failed", "owner_id", apiKeyCtx.UserID, "error", err)
		writeError(w, http.StatusInternalServerError, "video ingest failed")
	case res.Replayed:
		writeJSON(w, http.StatusOK, res)
	default:
		writeJSON(w, http.StatusCreated, res)
	}
}

// readIngestForm streams the multipart body: metadata is decoded and
// validated as soon as it arrives, file parts are spooled to temp files with
// a size cap, a SHA-256 and a content sniff. cleanup closes and removes every
// temp file and must always be called. On error it returns the HTTP status.
func (h *Handler) readIngestForm(r *http.Request) (*service.VideoIngestRequest, func(), int, error) {
	var spooled []*os.File
	cleanup := func() {
		for _, f := range spooled {
			_ = f.Close()
			_ = os.Remove(f.Name()) // #nosec G703 -- name comes from os.CreateTemp, never from the request
		}
	}
	fail := func(status int, err error) (*service.VideoIngestRequest, func(), int, error) {
		return nil, cleanup, status, err
	}

	rules := map[string]ingestFileRule{
		"video":     {maxBytes: h.cfg.Ingest.MaxVideoBytes, types: map[string]bool{"video/mp4": true}},
		"thumbnail": {maxBytes: IngestThumbnailMaxBytes, types: map[string]bool{"image/jpeg": true, "image/png": true}},
	}

	mr, err := r.MultipartReader()
	if err != nil {
		return fail(http.StatusBadRequest, fmt.Errorf("invalid multipart body"))
	}
	req := &service.VideoIngestRequest{IdempotencyKey: r.Header.Get("Idempotency-Key")}
	var haveMeta, haveVideo bool
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fail(bodyErrorStatus(err), fmt.Errorf("invalid multipart body"))
		}
		name := part.FormName()
		switch {
		case name == "metadata":
			if haveMeta {
				return fail(http.StatusBadRequest, fmt.Errorf("duplicate metadata part"))
			}
			haveMeta = true
			if err := decodeIngestMetadata(part, &req.Metadata); err != nil {
				return fail(bodyErrorStatus(err), err)
			}
		case rules[name].maxBytes > 0:
			if (name == "video" && haveVideo) || (name == "thumbnail" && req.Thumbnail != nil) {
				return fail(http.StatusBadRequest, fmt.Errorf("duplicate %s part", name))
			}
			f, file, err := spool(part, part.Header.Get("Content-Type"), rules[name])
			if f != nil {
				spooled = append(spooled, f)
			}
			if err != nil {
				return fail(bodyErrorStatus(err), fmt.Errorf("%s: %w", name, err))
			}
			if name == "video" {
				haveVideo, req.Video = true, *file
			} else {
				req.Thumbnail = file
			}
		default:
			return fail(http.StatusBadRequest, fmt.Errorf("unexpected form part %q", name))
		}
		_ = part.Close()
	}
	if !haveMeta {
		return fail(http.StatusBadRequest, fmt.Errorf("metadata part is required"))
	}
	if !haveVideo {
		return fail(http.StatusBadRequest, fmt.Errorf("video part is required"))
	}
	return req, cleanup, http.StatusOK, nil
}

func decodeIngestMetadata(part io.Reader, m *model.VideoIngestMetadata) error {
	raw, err := io.ReadAll(io.LimitReader(part, ingestMetadataMaxBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > ingestMetadataMaxBytes {
		return fmt.Errorf("metadata: %w (limit %d bytes)", errTooLarge, ingestMetadataMaxBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(m); err != nil {
		return fmt.Errorf("%w: metadata is not valid JSON for this endpoint", service.ErrInvalidIngest)
	}
	return service.NormalizeIngestMetadata(m)
}

// spool copies one file part to a temp file. The declared Content-Type and
// the type sniffed from the first bytes must both be allowed, and the part
// must not exceed rule.maxBytes. The returned *os.File (possibly non-nil on
// error) belongs to the caller's cleanup.
func spool(body io.Reader, declared string, rule ingestFileRule) (*os.File, *service.IngestFile, error) {
	declaredType, _, _ := mime.ParseMediaType(declared)
	if !rule.types[declaredType] {
		return nil, nil, fmt.Errorf("%w: declared Content-Type %q is not allowed", errUnsupportedType, declared)
	}

	head := make([]byte, sniffLen)
	n, err := io.ReadFull(body, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, nil, err
	}
	head = head[:n]
	if n == 0 {
		return nil, nil, fmt.Errorf("file is empty")
	}
	if sniffed := sniffContentType(head); sniffed != declaredType {
		return nil, nil, fmt.Errorf("%w: content looks like %q, not %q", errUnsupportedType, sniffed, declaredType)
	}

	f, err := os.CreateTemp("", "ingest-*")
	if err != nil {
		return nil, nil, fmt.Errorf("spool: %w", err)
	}
	hash := sha256.New()
	w := io.MultiWriter(f, hash)
	if _, err := w.Write(head); err != nil {
		return f, nil, fmt.Errorf("spool: %w", err)
	}
	rest, err := io.Copy(w, io.LimitReader(body, rule.maxBytes-int64(n)+1))
	if err != nil {
		return f, nil, err
	}
	size := int64(n) + rest
	if size > rule.maxBytes {
		return f, nil, fmt.Errorf("%w (limit %d bytes)", errTooLarge, rule.maxBytes)
	}
	return f, &service.IngestFile{
		Body:        f,
		Size:        size,
		SHA256:      hex.EncodeToString(hash.Sum(nil)),
		ContentType: declaredType,
	}, nil
}

// sniffContentType is http.DetectContentType without parameters.
func sniffContentType(head []byte) string {
	t, _, _ := mime.ParseMediaType(http.DetectContentType(head))
	return t
}

// bodyErrorStatus maps a body/validation error to its HTTP status.
func bodyErrorStatus(err error) int {
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytes), errors.Is(err, errTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, errUnsupportedType):
		return http.StatusUnsupportedMediaType
	default:
		return http.StatusBadRequest
	}
}

// validIdempotencyKey accepts 1..idempotencyKeyMaxLen printable ASCII bytes.
func validIdempotencyKey(k string) bool {
	if k == "" || len(k) > idempotencyKeyMaxLen {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < 0x21 || k[i] > 0x7e {
			return false
		}
	}
	return true
}
