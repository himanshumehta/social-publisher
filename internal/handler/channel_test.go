package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateChannel_InvalidBody(t *testing.T) {
	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/channels", body)
	w := httptest.NewRecorder()

	// Use readJSON directly to verify it rejects invalid JSON
	var data map[string]interface{}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}

	// Verify writeError produces correct format
	writeError(w, http.StatusBadRequest, "invalid request body")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "invalid request body" {
		t.Errorf("expected error message, got %v", decoded)
	}
}

func TestCreateChannel_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"category_id":"00000000-0000-0000-0000-000000000001","platform":"youtube","channel_name":"Test Channel","channel_url":"https://youtube.com/@test"}`)
	req := httptest.NewRequest("POST", "/api/v1/admin/channels", body)

	var data struct {
		CategoryID  string `json:"category_id"`
		Platform    string `json:"platform"`
		ChannelName string `json:"channel_name"`
		ChannelURL  string `json:"channel_url"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ChannelName != "Test Channel" {
		t.Errorf("expected channel_name 'Test Channel', got %q", data.ChannelName)
	}
	if data.Platform != "youtube" {
		t.Errorf("expected platform 'youtube', got %q", data.Platform)
	}
	if data.ChannelURL != "https://youtube.com/@test" {
		t.Errorf("expected channel_url, got %q", data.ChannelURL)
	}
}

func TestUpdateChannel_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"channel_name":"Updated Name","is_active":false}`)
	req := httptest.NewRequest("PUT", "/api/v1/admin/channels/some-id", body)

	var data struct {
		ChannelName *string `json:"channel_name"`
		IsActive    *bool   `json:"is_active"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ChannelName == nil || *data.ChannelName != "Updated Name" {
		t.Errorf("expected channel_name 'Updated Name', got %v", data.ChannelName)
	}
	if data.IsActive == nil || *data.IsActive != false {
		t.Errorf("expected is_active false, got %v", data.IsActive)
	}
}

func TestUpdateChannel_PartialBody(t *testing.T) {
	body := bytes.NewBufferString(`{"channel_name":"Only Name"}`)
	req := httptest.NewRequest("PUT", "/api/v1/admin/channels/some-id", body)

	var data struct {
		ChannelName *string `json:"channel_name"`
		ChannelURL  *string `json:"channel_url"`
		IsActive    *bool   `json:"is_active"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.ChannelName == nil || *data.ChannelName != "Only Name" {
		t.Errorf("expected channel_name 'Only Name'")
	}
	if data.ChannelURL != nil {
		t.Error("expected channel_url to be nil for partial update")
	}
	if data.IsActive != nil {
		t.Error("expected is_active to be nil for partial update")
	}
}

func TestListChannels_WriteJSON_EmptyArray(t *testing.T) {
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

func TestDeleteChannel_WriteJSON_Response(t *testing.T) {
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
