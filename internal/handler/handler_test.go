package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}
	writeJSON(w, http.StatusCreated, data)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["hello"] != "world" {
		t.Errorf("expected hello=world, got %v", decoded)
	}
}

func TestWriteJSON_Array(t *testing.T) {
	w := httptest.NewRecorder()
	data := []string{"a", "b", "c"}
	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded []string
	json.NewDecoder(w.Body).Decode(&decoded)
	if len(decoded) != 3 {
		t.Errorf("expected 3 items, got %d", len(decoded))
	}
}

func TestWriteJSON_NilValue(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, nil)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	body := strings.TrimSpace(w.Body.String())
	if body != "null" {
		t.Errorf("expected null, got %s", body)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "something went wrong")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "something went wrong" {
		t.Errorf("expected error message, got %v", decoded)
	}
}

func TestWriteError_Unauthorized(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusUnauthorized, "not authenticated")

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestWriteError_InternalServerError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusInternalServerError, "server error")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestReadJSON_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"name":"test","value":42}`)
	req := httptest.NewRequest("POST", "/test", body)
	req.Header.Set("Content-Type", "application/json")

	var data struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if data.Name != "test" {
		t.Errorf("expected name=test, got %s", data.Name)
	}
	if data.Value != 42 {
		t.Errorf("expected value=42, got %d", data.Value)
	}
}

func TestReadJSON_InvalidBody(t *testing.T) {
	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest("POST", "/test", body)
	var data map[string]interface{}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestReadJSON_EmptyBody(t *testing.T) {
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest("POST", "/test", body)
	var data map[string]interface{}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestReadJSON_NestedStruct(t *testing.T) {
	body := bytes.NewBufferString(`{"display_name":"Test Creator","bio":"I make videos","city":"Mumbai","specializations":["bikes","cars"]}`)
	req := httptest.NewRequest("POST", "/test", body)

	var data struct {
		DisplayName     string   `json:"display_name"`
		Bio             string   `json:"bio"`
		City            string   `json:"city"`
		Specializations []string `json:"specializations"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.DisplayName != "Test Creator" {
		t.Errorf("expected display_name=Test Creator, got %s", data.DisplayName)
	}
	if len(data.Specializations) != 2 {
		t.Errorf("expected 2 specializations, got %d", len(data.Specializations))
	}
}

func TestWriteJSON_StatusCodes(t *testing.T) {
	tests := []struct {
		status int
		name   string
	}{
		{http.StatusOK, "200"},
		{http.StatusCreated, "201"},
		{http.StatusNoContent, "204"},
		{http.StatusBadRequest, "400"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "404"},
		{http.StatusTooManyRequests, "429"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeJSON(w, tt.status, map[string]bool{"ok": true})
			if w.Code != tt.status {
				t.Errorf("expected %d, got %d", tt.status, w.Code)
			}
		})
	}
}

func TestWriteError_RedactsTokens(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadGateway, `Post "https://graph.facebook.com/v21.0/123/videos?access_token=EAAsecret": timeout`)
	if strings.Contains(w.Body.String(), "EAAsecret") {
		t.Fatalf("token leaked to client: %s", w.Body.String())
	}
}
