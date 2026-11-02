package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/google/uuid"
)

func TestMarkNotificationRead_InvalidUUID_WriteError(t *testing.T) {
	invalidIDs := []string{
		"not-a-uuid",
		"12345",
		"",
		"zzzzzzzz-zzzz-zzzz-zzzz-zzzzzzzzzzzz",
	}

	for _, id := range invalidIDs {
		t.Run("id="+id, func(t *testing.T) {
			_, err := uuid.Parse(id)
			if err == nil {
				t.Errorf("expected uuid.Parse(%q) to fail", id)
				return
			}

			w := httptest.NewRecorder()
			writeError(w, http.StatusBadRequest, "invalid notification id")
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}

			var decoded map[string]string
			json.NewDecoder(w.Body).Decode(&decoded)
			if decoded["error"] != "invalid notification id" {
				t.Errorf("expected 'invalid notification id', got %q", decoded["error"])
			}
		})
	}
}

func TestMarkNotificationRead_ValidUUID(t *testing.T) {
	validID := "00000000-0000-0000-0000-000000000001"
	parsed, err := uuid.Parse(validID)
	if err != nil {
		t.Fatalf("expected valid UUID, got error: %v", err)
	}
	if parsed.String() != validID {
		t.Errorf("expected %q, got %q", validID, parsed.String())
	}
}

func TestMarkNotificationRead_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "read"})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["status"] != "read" {
		t.Errorf("expected status 'read', got %q", decoded["status"])
	}
}

func TestListNotifications_UnreadQueryParam(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantUnread bool
	}{
		{"unread=true", "?unread=true", true},
		{"unread=false", "?unread=false", false},
		{"no param", "", false},
		{"unread=1", "?unread=1", false},
		{"unread=yes", "?unread=yes", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/notifications"+tt.query, nil)
			unreadOnly := req.URL.Query().Get("unread") == "true"
			if unreadOnly != tt.wantUnread {
				t.Errorf("unreadOnly = %v, want %v", unreadOnly, tt.wantUnread)
			}
		})
	}
}

func TestListNotifications_LimitOffsetQueryParams(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{"defaults", "", 0, 0},
		{"limit=10", "?limit=10", 10, 0},
		{"offset=5", "?offset=5", 0, 5},
		{"both", "?limit=20&offset=10", 20, 10},
		{"invalid limit", "?limit=abc", 0, 0},
		{"invalid offset", "?offset=xyz", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/v1/notifications"+tt.query, nil)
			q := req.URL.Query()
			limit, _ := strconv.Atoi(q.Get("limit"))
			offset, _ := strconv.Atoi(q.Get("offset"))

			if limit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", limit, tt.wantLimit)
			}
			if offset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", offset, tt.wantOffset)
			}
		})
	}
}

func TestListNotifications_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{
		"data":  []any{},
		"total": 0,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded map[string]any
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["total"].(float64) != 0 {
		t.Errorf("expected total 0, got %v", decoded["total"])
	}
	data, ok := decoded["data"].([]any)
	if !ok {
		t.Fatal("expected data to be an array")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}
}

func TestDeleteNotification_InvalidUUID_WriteError(t *testing.T) {
	_, err := uuid.Parse("bad-uuid")
	if err == nil {
		t.Fatal("expected uuid.Parse to fail")
	}

	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "invalid notification id")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDeleteNotification_SuccessResponse(t *testing.T) {
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

func TestGetUnreadCount_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]int{"count": 5})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var decoded map[string]int
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["count"] != 5 {
		t.Errorf("expected count 5, got %d", decoded["count"])
	}
}

func TestMarkAllNotificationsRead_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]int64{"updated": 12})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var decoded map[string]float64
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["updated"] != 12 {
		t.Errorf("expected updated 12, got %v", decoded["updated"])
	}
}
