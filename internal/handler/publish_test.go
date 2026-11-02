package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreatePublishRecord_InvalidBody(t *testing.T) {
	body := bytes.NewBufferString(`{not valid}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/some-id/publish-records", body)

	var data map[string]interface{}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}
}

func TestCreatePublishRecord_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"channel_id":"00000000-0000-0000-0000-000000000001","platform_url":"https://youtube.com/watch?v=abc","title":"My Video","visibility":"public"}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/some-id/publish-records", body)

	var data struct {
		ChannelID   string `json:"channel_id"`
		PlatformURL string `json:"platform_url"`
		Title       string `json:"title"`
		Visibility  string `json:"visibility"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ChannelID != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected channel_id, got %q", data.ChannelID)
	}
	if data.PlatformURL != "https://youtube.com/watch?v=abc" {
		t.Errorf("expected platform_url, got %q", data.PlatformURL)
	}
	if data.Title != "My Video" {
		t.Errorf("expected title 'My Video', got %q", data.Title)
	}
	if data.Visibility != "public" {
		t.Errorf("expected visibility 'public', got %q", data.Visibility)
	}
}

func TestCreatePublishRecord_WithOptionalFields(t *testing.T) {
	body := bytes.NewBufferString(`{
		"channel_id":"00000000-0000-0000-0000-000000000001",
		"platform_url":"https://youtube.com/watch?v=abc",
		"platform_content_id":"abc123",
		"title":"Test Title",
		"description":"Test description",
		"visibility":"unlisted"
	}`)
	req := httptest.NewRequest("POST", "/api/v1/projects/some-id/publish-records", body)

	var data struct {
		ChannelID         string `json:"channel_id"`
		PlatformURL       string `json:"platform_url"`
		PlatformContentID string `json:"platform_content_id"`
		Title             string `json:"title"`
		Description       string `json:"description"`
		Visibility        string `json:"visibility"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.PlatformContentID != "abc123" {
		t.Errorf("expected platform_content_id 'abc123', got %q", data.PlatformContentID)
	}
	if data.Visibility != "unlisted" {
		t.Errorf("expected visibility 'unlisted', got %q", data.Visibility)
	}
}

func TestUpdatePublishRecord_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"title":"Updated Title","status":"removed"}`)
	req := httptest.NewRequest("PATCH", "/api/v1/publish-records/some-id", body)

	var data struct {
		Title  *string `json:"title"`
		Status *string `json:"status"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Title == nil || *data.Title != "Updated Title" {
		t.Error("expected title 'Updated Title'")
	}
	if data.Status == nil || *data.Status != "removed" {
		t.Error("expected status 'removed'")
	}
}

func TestUpdatePublishRecord_PartialBody(t *testing.T) {
	body := bytes.NewBufferString(`{"platform_url":"https://new-url.com"}`)
	req := httptest.NewRequest("PATCH", "/api/v1/publish-records/some-id", body)

	var data struct {
		PlatformURL *string `json:"platform_url"`
		Title       *string `json:"title"`
		Status      *string `json:"status"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.PlatformURL == nil || *data.PlatformURL != "https://new-url.com" {
		t.Error("expected platform_url")
	}
	if data.Title != nil {
		t.Error("expected title to be nil for partial update")
	}
	if data.Status != nil {
		t.Error("expected status to be nil for partial update")
	}
}

func TestListPublishRecords_EmptyResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, []interface{}{})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded []interface{}
	json.NewDecoder(w.Body).Decode(&decoded)
	if len(decoded) != 0 {
		t.Errorf("expected empty array, got %d items", len(decoded))
	}
}

func TestDeletePublishRecord_Response(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["status"] != "deleted" {
		t.Errorf("expected status 'deleted', got %v", decoded)
	}
}

func TestPublishRecord_ErrorResponse(t *testing.T) {
	tests := []struct {
		name   string
		status int
		msg    string
	}{
		{"bad_request", http.StatusBadRequest, "invalid project id"},
		{"bad_body", http.StatusBadRequest, "invalid request body"},
		{"server_error", http.StatusInternalServerError, "failed to list publish records"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeError(w, tt.status, tt.msg)

			if w.Code != tt.status {
				t.Errorf("expected %d, got %d", tt.status, w.Code)
			}

			var decoded map[string]string
			json.NewDecoder(w.Body).Decode(&decoded)
			if decoded["error"] != tt.msg {
				t.Errorf("expected error %q, got %q", tt.msg, decoded["error"])
			}
		})
	}
}

func TestSuggestedChannels_ResponseFormat(t *testing.T) {
	w := httptest.NewRecorder()
	data := []map[string]interface{}{
		{
			"channel":          map[string]string{"id": "abc", "channel_name": "Test", "platform": "youtube"},
			"matching_formats": []string{"youtube_16_9"},
			"reason":           "Matches youtube formats in this category",
		},
	}
	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&decoded)
	if len(decoded) != 1 {
		t.Fatalf("expected 1 suggestion, got %d", len(decoded))
	}
	if decoded[0]["reason"] != "Matches youtube formats in this category" {
		t.Errorf("unexpected reason: %v", decoded[0]["reason"])
	}
}
