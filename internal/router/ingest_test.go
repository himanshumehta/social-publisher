package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/himanshumehta/social-publisher/internal/model"
	"github.com/himanshumehta/social-publisher/internal/service"
)

// These tests drive POST /api/v1/external/videos through the real router,
// API-key middleware and Postgres, with an in-memory object store. They need
// TEST_DATABASE_URL and skip without it.

type memStore struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func (m *memStore) PutObject(_ context.Context, key, _ string, body io.ReadSeeker, size int64) error {
	b, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	if int64(len(b)) != size {
		return fmt.Errorf("size %d != %d", len(b), size)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.objects[key] = b
	return nil
}

func (m *memStore) DeleteObject(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.objects, key)
	return nil
}

func (m *memStore) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.objects)
}

type ingestFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	h       http.Handler
	store   *memStore
	posts   *service.PostService
	ownerID uuid.UUID
	key     string // API key with posts:create
	readKey string // API key with posts:read only
	tmp     string // TMPDIR for spooled uploads
}

// ingestClientN gives each request its own client address, so the per-IP
// rate limiter does not interfere.
var ingestClientN atomic.Int64

// maxTestVideo keeps oversize tests small.
const maxTestVideo = 64 << 10

func newIngestFixture(t *testing.T) *ingestFixture {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Skipf("cannot connect to test database: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("cannot ping test database: %v", err)
	}

	email := fmt.Sprintf("ingest-%s@example.com", uuid.New().String()[:8])
	ownerID, err := service.CreateUser(ctx, pool, email, "", "creator", "ingest-test-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	key, err := service.IssueAPIKey(ctx, pool, email, "ingest-test", []string{"posts:create", "posts:read"}, time.Hour)
	if err != nil {
		t.Fatalf("issue key: %v", err)
	}
	readKey, err := service.IssueAPIKey(ctx, pool, email, "ingest-test-ro", []string{"posts:read"}, time.Hour)
	if err != nil {
		t.Fatalf("issue key: %v", err)
	}
	t.Cleanup(func() {
		// ingest_requests, posts' projects and videos go with the user.
		pool.Exec(ctx, `DELETE FROM ingest_requests WHERE owner_id = $1`, ownerID)
		pool.Exec(ctx, `DELETE FROM post_publish_links WHERE post_id IN (SELECT id FROM posts WHERE author_id = $1)`, ownerID)
		pool.Exec(ctx, `DELETE FROM posts WHERE author_id = $1`, ownerID)
		pool.Exec(ctx, `DELETE FROM videos WHERE uploaded_by = $1`, ownerID)
		pool.Exec(ctx, `DELETE FROM projects WHERE created_by = $1`, ownerID)
		pool.Exec(ctx, `DELETE FROM api_keys WHERE user_id = $1`, ownerID)
		pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, ownerID)
		pool.Close()
	})

	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &memStore{objects: map[string][]byte{}}
	posts := service.NewPostService(pool, nil, nil, service.NewPostReviewer(logger), logger)
	svc := &service.Services{
		APIKey:      service.NewAPIKeyService(pool, logger),
		Post:        posts,
		VideoIngest: service.NewVideoIngestService(pool, store, posts, logger),
	}
	cfg := testConfig()
	cfg.Ingest.MaxVideoBytes = maxTestVideo
	return &ingestFixture{
		ctx: ctx, pool: pool, h: New(cfg, svc, logger, pool, nil), store: store, posts: posts,
		ownerID: ownerID, key: key, readKey: readKey, tmp: tmp,
	}
}

// fakeMP4 is a minimal ISO-BMFF header that http.DetectContentType reports
// as video/mp4, padded to size bytes.
func fakeMP4(size int) []byte {
	head := []byte("\x00\x00\x00\x18ftypmp42\x00\x00\x00\x00mp42isom")
	if size < len(head) {
		size = len(head)
	}
	b := make([]byte, size)
	copy(b, head)
	return b
}

var pngHeader = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")

type ingestPart struct {
	name, contentType string
	body              []byte
}

func defaultMetadata() map[string]any {
	return map[string]any{
		"title":            "Classic 350: five-year ownership cost",
		"description":      "What it costs to own, year by year.",
		"target_platforms": []string{"instagram", "youtube"},
	}
}

func (f *ingestFixture) post(t *testing.T, apiKey, idemKey string, meta map[string]any, parts ...ingestPart) (int, map[string]any) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if meta != nil {
		raw, _ := json.Marshal(meta)
		if err := mw.WriteField("metadata", string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range parts {
		hdr := textproto.MIMEHeader{}
		hdr.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename="upload"`, p.name))
		hdr.Set("Content-Type", p.contentType)
		w, err := mw.CreatePart(hdr)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(p.body)
	}
	mw.Close()

	r := httptest.NewRequest(http.MethodPost, "/api/v1/external/videos", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	if apiKey != "" {
		r.Header.Set("Authorization", "Bearer "+apiKey)
	}
	if idemKey != "" {
		r.Header.Set("Idempotency-Key", idemKey)
	}
	n := ingestClientN.Add(1) // concurrent callers: keep the counter race-free
	r.RemoteAddr = fmt.Sprintf("10.9.%d.%d:1234", n/250, n%250+1)
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func (f *ingestFixture) postCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM posts WHERE author_id = $1`, f.ownerID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *ingestFixture) assertNoTempFiles(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(f.tmp)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("%d spooled temp file(s) left behind", len(entries))
	}
}

func video(n int) ingestPart { return ingestPart{"video", "video/mp4", fakeMP4(n)} }

func TestIngest_CreatesPendingReviewPostAndReplays(t *testing.T) {
	f := newIngestFixture(t)
	key := "key-" + uuid.NewString()

	code, first := f.post(t, f.key, key, defaultMetadata(), video(4096), ingestPart{"thumbnail", "image/png", pngHeader})
	if code != http.StatusCreated {
		t.Fatalf("first ingest: %d %v", code, first)
	}
	if first["status"] != "pending_review" || first["replayed"] != false {
		t.Fatalf("first ingest body = %v", first)
	}
	if f.store.count() != 2 {
		t.Fatalf("stored %d objects, want video + thumbnail", f.store.count())
	}
	for k := range f.store.objects {
		if !strings.HasPrefix(k, "ingest/"+f.ownerID.String()+"/"+first["post_id"].(string)+"/") {
			t.Errorf("object key %q is not under the owner/post prefix", k)
		}
	}

	var linked uuid.UUID
	var platforms []string
	if err := f.pool.QueryRow(f.ctx,
		`SELECT linked_video_id, target_platforms FROM posts WHERE id = $1`, first["post_id"],
	).Scan(&linked, &platforms); err != nil {
		t.Fatal(err)
	}
	if linked.String() != first["video_id"] || strings.Join(platforms, ",") != "instagram,youtube" {
		t.Errorf("post row: linked_video_id=%s platforms=%v", linked, platforms)
	}

	// Same key, same payload: the original resource, nothing new stored.
	code, again := f.post(t, f.key, key, defaultMetadata(), video(4096), ingestPart{"thumbnail", "image/png", pngHeader})
	if code != http.StatusOK || again["replayed"] != true ||
		again["post_id"] != first["post_id"] || again["video_id"] != first["video_id"] {
		t.Fatalf("replay: %d %v, want 200 with the original ids", code, again)
	}
	if n := f.postCount(t); n != 1 {
		t.Errorf("%d posts after replay, want 1", n)
	}
	if f.store.count() != 2 {
		t.Errorf("replay stored new objects: %d", f.store.count())
	}

	// Same key, different payload: 409 and still one post.
	code, body := f.post(t, f.key, key, defaultMetadata(), video(4097))
	if code != http.StatusConflict {
		t.Fatalf("conflicting replay: %d %v, want 409", code, body)
	}
	if n := f.postCount(t); n != 1 {
		t.Errorf("%d posts after conflicting replay, want 1", n)
	}
	f.assertNoTempFiles(t)
}

func TestIngest_ConcurrentRetriesCreateOnePost(t *testing.T) {
	f := newIngestFixture(t)
	key := "key-" + uuid.NewString()
	const n = 6
	var wg sync.WaitGroup
	codes := make([]int, n)
	ids := make([]any, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code, body := f.post(t, f.key, key, defaultMetadata(), video(2048))
			codes[i], ids[i] = code, body["post_id"]
		}(i)
	}
	wg.Wait()
	created := 0
	for i := range codes {
		if codes[i] == http.StatusCreated {
			created++
		} else if codes[i] != http.StatusOK {
			t.Errorf("request %d: status %d", i, codes[i])
		}
		if ids[i] != ids[0] {
			t.Errorf("request %d got post %v, request 0 got %v", i, ids[i], ids[0])
		}
	}
	if created != 1 {
		t.Errorf("%d requests created a post, want exactly 1", created)
	}
	if got := f.postCount(t); got != 1 {
		t.Errorf("%d posts, want 1", got)
	}
	// Losers delete their own objects; only the winner's video remains.
	if f.store.count() != 1 {
		t.Errorf("%d objects stored, want 1", f.store.count())
	}
}

func TestIngest_RejectsBadRequests(t *testing.T) {
	f := newIngestFixture(t)
	cases := []struct {
		name   string
		apiKey string
		idem   string
		meta   map[string]any
		parts  []ingestPart
		want   int
	}{
		{"no API key", "", "k1", defaultMetadata(), []ingestPart{video(100)}, http.StatusUnauthorized},
		{"bad API key", "sp_not-a-real-key", "k1", defaultMetadata(), []ingestPart{video(100)}, http.StatusUnauthorized},
		{"read-only key", f.readKey, "k1", defaultMetadata(), []ingestPart{video(100)}, http.StatusForbidden},
		{"no idempotency key", f.key, "", defaultMetadata(), []ingestPart{video(100)}, http.StatusBadRequest},
		{"no metadata", f.key, "k2", nil, []ingestPart{video(100)}, http.StatusBadRequest},
		{"no video", f.key, "k3", defaultMetadata(), nil, http.StatusBadRequest},
		{"unknown platform", f.key, "k4", map[string]any{"title": "t", "target_platforms": []string{"myspace"}}, []ingestPart{video(100)}, http.StatusBadRequest},
		{"unknown metadata field", f.key, "k5", map[string]any{"title": "t", "target_platforms": []string{"youtube"}, "auto_publish": true}, []ingestPart{video(100)}, http.StatusBadRequest},
		{"oversize video", f.key, "k6", defaultMetadata(), []ingestPart{video(maxTestVideo + 1)}, http.StatusRequestEntityTooLarge},
		{"declared type not allowed", f.key, "k7", defaultMetadata(), []ingestPart{{"video", "video/quicktime", fakeMP4(100)}}, http.StatusUnsupportedMediaType},
		{"content is not mp4", f.key, "k8", defaultMetadata(), []ingestPart{{"video", "video/mp4", []byte("<html><script>x</script></html>")}}, http.StatusUnsupportedMediaType},
		{"thumbnail is not an image", f.key, "k9", defaultMetadata(), []ingestPart{video(100), {"thumbnail", "image/png", fakeMP4(100)}}, http.StatusUnsupportedMediaType},
		{"unexpected part", f.key, "k10", defaultMetadata(), []ingestPart{video(100), {"script", "text/plain", []byte("x")}}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, body := f.post(t, tc.apiKey, tc.idem, tc.meta, tc.parts...)
			if code != tc.want {
				t.Errorf("status %d (%v), want %d", code, body, tc.want)
			}
		})
	}
	if n := f.postCount(t); n != 0 {
		t.Errorf("%d posts created by rejected requests", n)
	}
	if f.store.count() != 0 {
		t.Errorf("%d objects stored by rejected requests", f.store.count())
	}
	f.assertNoTempFiles(t)
}

func TestIngest_BodyCapAppliesBeforeHandler(t *testing.T) {
	f := newIngestFixture(t)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/external/videos", strings.NewReader("x"))
	r.ContentLength = 1 << 40
	r.Header.Set("Authorization", "Bearer "+f.key)
	r.Header.Set("Idempotency-Key", "k")
	r.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	r.RemoteAddr = "10.8.0.1:1234"
	if got := serve(f.h, r); got != http.StatusRequestEntityTooLarge {
		t.Fatalf("status %d, want 413", got)
	}
}

// TestIngest_ReviewGateHoldsUntilApproval walks the post through the real
// worker steps: it is not picked for publishing while pending review, and
// after approval the dry-run publisher publishes it to every target.
func TestIngest_ReviewGateHoldsUntilApproval(t *testing.T) {
	f := newIngestFixture(t)
	code, body := f.post(t, f.key, "gate-"+uuid.NewString(), defaultMetadata(), video(1024))
	if code != http.StatusCreated {
		t.Fatalf("ingest: %d %v", code, body)
	}
	postID := uuid.MustParse(body["post_id"].(string))

	pickOurs := func() *model.Post {
		// Other rows in a shared test database may be picked first.
		for i := 0; i < 50; i++ {
			p, err := f.posts.PickForPublish(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			if p == nil || p.ID == postID {
				return p
			}
		}
		return nil
	}
	if p := pickOurs(); p != nil {
		t.Fatal("publish worker picked an unapproved ingested post")
	}

	if err := f.posts.AdminApprove(f.ctx, postID, nil); err != nil {
		t.Fatalf("approve: %v", err)
	}
	p := pickOurs()
	if p == nil {
		t.Fatal("approved post was not picked for publishing")
	}

	var logs bytes.Buffer
	publisher := service.NewPostPublisher(f.pool, slog.New(slog.NewTextHandler(&logs, nil)), nil)
	publisher.EnableDryRun()
	results, err := publisher.Publish(f.ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.posts.CompletePublish(f.ctx, postID, results); err != nil {
		t.Fatal(err)
	}
	for _, platform := range []string{"instagram", "youtube"} {
		if want := "would publish " + platform + " " + postID.String(); !strings.Contains(logs.String(), want) {
			t.Errorf("log missing %q", want)
		}
	}
	if !strings.Contains(logs.String(), "video_key=ingest/") {
		t.Error("dry-run log does not name the linked video")
	}
	var status string
	f.pool.QueryRow(f.ctx, `SELECT status::text FROM posts WHERE id = $1`, postID).Scan(&status)
	if status != "published" {
		t.Errorf("status = %s, want published", status)
	}
}

func TestIngest_ExpiredKeysAreCleanedUp(t *testing.T) {
	f := newIngestFixture(t)
	key := "ttl-" + uuid.NewString()
	if code, body := f.post(t, f.key, key, defaultMetadata(), video(512)); code != http.StatusCreated {
		t.Fatalf("ingest: %d %v", code, body)
	}
	if _, err := f.pool.Exec(f.ctx,
		`UPDATE ingest_requests SET created_at = NOW() - make_interval(secs => $2) - INTERVAL '1 minute' WHERE owner_id = $1`,
		f.ownerID, service.IngestRetention.Seconds()); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ExpireIngestRequests(f.ctx, f.pool); err != nil {
		t.Fatal(err)
	}
	var left int
	f.pool.QueryRow(f.ctx, `SELECT COUNT(*) FROM ingest_requests WHERE owner_id = $1`, f.ownerID).Scan(&left)
	if left != 0 {
		t.Errorf("%d expired idempotency rows left", left)
	}
	if n := f.postCount(t); n != 1 {
		t.Errorf("cleanup removed the post: %d posts", n)
	}
}
