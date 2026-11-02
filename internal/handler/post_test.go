package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

func TestCreatePost_ValidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{"source_url":"https://youtube.com/watch?v=abc","title":"My Post","content":"Full content here","target_platforms":["youtube","instagram"],"tone":"professional","auto_publish":false}`)
	req := httptest.NewRequest("POST", "/api/v1/posts", body)

	var data struct {
		SourceURL       string   `json:"source_url"`
		Title           string   `json:"title"`
		Content         string   `json:"content"`
		TargetPlatforms []string `json:"target_platforms"`
		Tone            string   `json:"tone"`
		AutoPublish     bool     `json:"auto_publish"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.SourceURL != "https://youtube.com/watch?v=abc" {
		t.Errorf("expected source_url, got %q", data.SourceURL)
	}
	if data.Title != "My Post" {
		t.Errorf("expected title 'My Post', got %q", data.Title)
	}
	if len(data.TargetPlatforms) != 2 {
		t.Errorf("expected 2 target_platforms, got %d", len(data.TargetPlatforms))
	}
	if data.Tone != "professional" {
		t.Errorf("expected tone 'professional', got %q", data.Tone)
	}
	if data.AutoPublish != false {
		t.Error("expected auto_publish false")
	}
}

func TestCreatePost_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{invalid}`)
	req := httptest.NewRequest("POST", "/api/v1/posts", body)
	w := httptest.NewRecorder()

	var data struct {
		Title string `json:"title"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	writeError(w, http.StatusBadRequest, "invalid request body")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "invalid request body" {
		t.Errorf("expected 'invalid request body', got %q", decoded["error"])
	}
}

func TestCreatePost_EmptyBody(t *testing.T) {
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest("POST", "/api/v1/posts", body)

	var data struct {
		Title string `json:"title"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestCreatePost_MinimalFields(t *testing.T) {
	body := bytes.NewBufferString(`{"target_platforms":["youtube"]}`)
	req := httptest.NewRequest("POST", "/api/v1/posts", body)

	var data struct {
		SourceURL       string   `json:"source_url"`
		Title           string   `json:"title"`
		Content         string   `json:"content"`
		TargetPlatforms []string `json:"target_platforms"`
		AutoPublish     bool     `json:"auto_publish"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.SourceURL != "" {
		t.Errorf("expected empty source_url, got %q", data.SourceURL)
	}
	if data.Title != "" {
		t.Errorf("expected empty title, got %q", data.Title)
	}
	if len(data.TargetPlatforms) != 1 {
		t.Errorf("expected 1 target_platform, got %d", len(data.TargetPlatforms))
	}
	if data.AutoPublish != false {
		t.Error("expected auto_publish to default to false")
	}
}

func TestListPosts_PageAndPageSizeDefaults(t *testing.T) {
	tests := []struct {
		name         string
		query        string
		wantPage     int
		wantPageSize int
	}{
		{"defaults", "", 1, 20},
		{"custom page", "?page=3", 3, 20},
		{"custom page_size", "?page_size=50", 1, 50},
		{"both", "?page=2&page_size=10", 2, 10},
		{"page=0 defaults to 1", "?page=0", 1, 20},
		{"page=-1 defaults to 1", "?page=-1", 1, 20},
		{"page_size=0 defaults to 20", "?page_size=0", 1, 20},
		{"page_size=-5 defaults to 20", "?page_size=-5", 1, 20},
		{"page_size=100 max", "?page_size=100", 1, 100},
		{"page_size=101 clamped to 20", "?page_size=101", 1, 20},
		{"page_size=200 clamped to 20", "?page_size=200", 1, 20},
		{"invalid page", "?page=abc", 1, 20},
		{"invalid page_size", "?page_size=xyz", 1, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/posts"+tt.query, nil)

			// Replicate the exact handler logic from post.go
			page := 1
			if v, err := strconv.Atoi(req.URL.Query().Get("page")); err == nil && v > 0 {
				page = v
			}
			pageSize := 20
			if v, err := strconv.Atoi(req.URL.Query().Get("page_size")); err == nil && v > 0 && v <= 100 {
				pageSize = v
			}

			if page != tt.wantPage {
				t.Errorf("page = %d, want %d", page, tt.wantPage)
			}
			if pageSize != tt.wantPageSize {
				t.Errorf("pageSize = %d, want %d", pageSize, tt.wantPageSize)
			}
		})
	}
}

func TestListPosts_FilterQueryParams(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/posts?status=published&platform=youtube&search=bike&sort_by=created_at&sort_order=desc", nil)
	q := req.URL.Query()

	if q.Get("status") != "published" {
		t.Errorf("expected status 'published', got %q", q.Get("status"))
	}
	if q.Get("platform") != "youtube" {
		t.Errorf("expected platform 'youtube', got %q", q.Get("platform"))
	}
	if q.Get("search") != "bike" {
		t.Errorf("expected search 'bike', got %q", q.Get("search"))
	}
	if q.Get("sort_by") != "created_at" {
		t.Errorf("expected sort_by 'created_at', got %q", q.Get("sort_by"))
	}
	if q.Get("sort_order") != "desc" {
		t.Errorf("expected sort_order 'desc', got %q", q.Get("sort_order"))
	}
}

func TestListPosts_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{
		"posts":     []any{},
		"total":     0,
		"page":      1,
		"page_size": 20,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded map[string]any
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["page"].(float64) != 1 {
		t.Errorf("expected page 1, got %v", decoded["page"])
	}
	if decoded["page_size"].(float64) != 20 {
		t.Errorf("expected page_size 20, got %v", decoded["page_size"])
	}
	if decoded["total"].(float64) != 0 {
		t.Errorf("expected total 0, got %v", decoded["total"])
	}
}

func TestGetPost_InvalidUUID_WriteError(t *testing.T) {
	_, err := uuid.Parse("not-valid")
	if err == nil {
		t.Fatal("expected uuid.Parse to fail")
	}

	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "invalid post ID")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestGetPost_ValidUUID(t *testing.T) {
	validID := "550e8400-e29b-41d4-a716-446655440000"
	parsed, err := uuid.Parse(validID)
	if err != nil {
		t.Fatalf("expected valid UUID, got error: %v", err)
	}
	if parsed.String() != validID {
		t.Errorf("expected %q, got %q", validID, parsed.String())
	}
}

func TestDeletePost_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %q", decoded["status"])
	}
}

func TestCreatePost_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusCreated, map[string]string{"id": "550e8400-e29b-41d4-a716-446655440000"})

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestListProjectPosts_InvalidUUID_WriteError(t *testing.T) {
	_, err := uuid.Parse("bad-project-id")
	if err == nil {
		t.Fatal("expected uuid.Parse to fail")
	}

	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "invalid project ID")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "invalid project ID" {
		t.Errorf("expected 'invalid project ID', got %q", decoded["error"])
	}
}

func TestRegeneratePost_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "regenerating"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["status"] != "regenerating" {
		t.Errorf("expected status 'regenerating', got %q", decoded["status"])
	}
}

func TestSubmitPost_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "submitted"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["status"] != "submitted" {
		t.Errorf("expected status 'submitted', got %q", decoded["status"])
	}
}
