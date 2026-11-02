package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOAuthCallback_ValidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{"code":"auth_code_123","state":"random_state_456"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/google/callback", body)

	var data struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Code != "auth_code_123" {
		t.Errorf("expected code 'auth_code_123', got %q", data.Code)
	}
	if data.State != "random_state_456" {
		t.Errorf("expected state 'random_state_456', got %q", data.State)
	}
}

func TestOAuthCallback_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`not json at all`)
	req := httptest.NewRequest("POST", "/api/v1/auth/google/callback", body)

	var data struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestOAuthCallback_EmptyFields(t *testing.T) {
	body := bytes.NewBufferString(`{"code":"","state":""}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/google/callback", body)

	var data struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty strings parse successfully; the handler validates at service level
	if data.Code != "" {
		t.Errorf("expected empty code, got %q", data.Code)
	}
	if data.State != "" {
		t.Errorf("expected empty state, got %q", data.State)
	}
}

func TestLogin_ValidRequest(t *testing.T) {
	body := bytes.NewBufferString(`{"email":"user@example.com","password":"securepass123"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", body)

	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got %q", data.Email)
	}
	if data.Password != "securepass123" {
		t.Errorf("expected password 'securepass123', got %q", data.Password)
	}
}

func TestLogin_MissingFields(t *testing.T) {
	body := bytes.NewBufferString(`{"email":"user@example.com"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", body)

	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Missing password parses as empty string; handler validates at service level
	if data.Email != "user@example.com" {
		t.Errorf("expected email, got %q", data.Email)
	}
	if data.Password != "" {
		t.Errorf("expected empty password, got %q", data.Password)
	}
}

func TestLogin_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{bad json}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/login", body)
	w := httptest.NewRecorder()

	var data struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}

	writeError(w, http.StatusBadRequest, "invalid request body")
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestForgotPassword_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"email":"user@example.com"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/forgot-password", body)

	var data struct {
		Email string `json:"email"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got %q", data.Email)
	}
}

func TestForgotPassword_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`not-json`)
	req := httptest.NewRequest("POST", "/api/v1/auth/forgot-password", body)

	var data struct {
		Email string `json:"email"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestResetPassword_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"token":"reset_token_abc","password":"newpass456"}`)
	req := httptest.NewRequest("POST", "/api/v1/auth/reset-password", body)

	var data struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Token != "reset_token_abc" {
		t.Errorf("expected token 'reset_token_abc', got %q", data.Token)
	}
	if data.Password != "newpass456" {
		t.Errorf("expected password 'newpass456', got %q", data.Password)
	}
}

func TestResetPassword_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{broken`)
	req := httptest.NewRequest("POST", "/api/v1/auth/reset-password", body)

	var data struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSignup_WriteError_Forbidden(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusForbidden, "signups are currently disabled, ask an administrator for an account")

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}

	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "signups are currently disabled, ask an administrator for an account" {
		t.Errorf("expected signup disabled error, got %q", decoded["error"])
	}
}

func TestAuth_WriteJSON_SuccessResponse(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": "fake-access-token",
		"expires_at":   1700000000,
	})

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var decoded map[string]any
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["access_token"] != "fake-access-token" {
		t.Errorf("expected access_token in response, got %v", decoded)
	}
	if decoded["expires_at"] == nil {
		t.Error("expected expires_at in response")
	}
}
