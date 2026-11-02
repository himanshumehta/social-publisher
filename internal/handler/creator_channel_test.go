package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderToPlatform(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		wantPlatform string
		wantOK       bool
	}{
		{"google maps to youtube", "google", "youtube", true},
		{"facebook maps to facebook", "facebook", "facebook", true},
		{"instagram maps to instagram", "instagram", "instagram", true},
		{"threads maps to threads", "threads", "threads", true},
		{"twitter maps to twitter", "twitter", "twitter", true},
		{"linkedin maps to linkedin", "linkedin", "linkedin", true},
		{"case insensitive Google", "Google", "youtube", true},
		{"uppercase TWITTER", "TWITTER", "twitter", true},
		{"unsupported tiktok", "tiktok", "", false},
		{"empty string", "", "", false},
		{"invalid provider", "invalid", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platform, ok := providerToPlatform(tt.provider)
			if platform != tt.wantPlatform {
				t.Errorf("providerToPlatform(%q) platform = %q, want %q", tt.provider, platform, tt.wantPlatform)
			}
			if ok != tt.wantOK {
				t.Errorf("providerToPlatform(%q) ok = %v, want %v", tt.provider, ok, tt.wantOK)
			}
		})
	}
}

func TestConnectMyChannel_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest("POST", "/api/v1/me/channels/connect", body)
	w := httptest.NewRecorder()

	var data struct {
		Provider string `json:"provider"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON body")
	}

	writeError(w, http.StatusBadRequest, "invalid request body")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "invalid request body" {
		t.Errorf("expected error message 'invalid request body', got %q", decoded["error"])
	}
}

func TestConnectMyChannel_ValidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{"provider":"google"}`)
	req := httptest.NewRequest("POST", "/api/v1/me/channels/connect", body)

	var data struct {
		Provider string `json:"provider"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Provider != "google" {
		t.Errorf("expected provider 'google', got %q", data.Provider)
	}
}

func TestConnectMyChannel_EmptyProvider(t *testing.T) {
	body := bytes.NewBufferString(`{"provider":""}`)
	req := httptest.NewRequest("POST", "/api/v1/me/channels/connect", body)

	var data struct {
		Provider string `json:"provider"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Provider != "" {
		t.Errorf("expected empty provider, got %q", data.Provider)
	}

	// Handler validates later via providerToPlatform
	_, ok := providerToPlatform(data.Provider)
	if ok {
		t.Error("expected providerToPlatform to reject empty provider")
	}
}

func TestConnectMyChannel_UnsupportedProvider_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "unsupported provider: tiktok")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "unsupported provider: tiktok" {
		t.Errorf("expected unsupported provider error, got %q", decoded["error"])
	}
}

func TestDisconnectMyChannel_InvalidUUID_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "invalid channel id")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "invalid channel id" {
		t.Errorf("expected 'invalid channel id', got %q", decoded["error"])
	}
}

func TestConnectMyChannel_WriteJSON_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{
		"url":        "https://accounts.google.com/o/oauth2/auth?...",
		"channel_id": "00000000-0000-0000-0000-000000000001",
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["url"] == "" {
		t.Error("expected url in response")
	}
	if decoded["channel_id"] != "00000000-0000-0000-0000-000000000001" {
		t.Errorf("expected channel_id, got %q", decoded["channel_id"])
	}
}
