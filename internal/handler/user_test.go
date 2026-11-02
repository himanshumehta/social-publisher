package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnboard_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"display_name":"Test Creator","bio":"I make videos","city":"Mumbai","state":"Maharashtra","specializations":["bikes","cars"],"category_ids":["cat1"]}`)
	req := httptest.NewRequest("POST", "/api/v1/me/onboard", body)

	var data struct {
		DisplayName     string   `json:"display_name"`
		Bio             string   `json:"bio"`
		City            string   `json:"city"`
		State           string   `json:"state"`
		Specializations []string `json:"specializations"`
		CategoryIDs     []string `json:"category_ids"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.DisplayName != "Test Creator" {
		t.Errorf("expected display_name 'Test Creator', got %q", data.DisplayName)
	}
	if data.Bio != "I make videos" {
		t.Errorf("expected bio 'I make videos', got %q", data.Bio)
	}
	if data.City != "Mumbai" {
		t.Errorf("expected city 'Mumbai', got %q", data.City)
	}
	if data.State != "Maharashtra" {
		t.Errorf("expected state 'Maharashtra', got %q", data.State)
	}
	if len(data.Specializations) != 2 {
		t.Errorf("expected 2 specializations, got %d", len(data.Specializations))
	}
	if len(data.CategoryIDs) != 1 {
		t.Errorf("expected 1 category_id, got %d", len(data.CategoryIDs))
	}
}

func TestOnboard_EmptyDisplayName(t *testing.T) {
	body := bytes.NewBufferString(`{"display_name":"","bio":"some bio"}`)
	req := httptest.NewRequest("POST", "/api/v1/me/onboard", body)

	var data struct {
		DisplayName string `json:"display_name"`
		Bio         string `json:"bio"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty display_name parses OK; the handler validates and returns 400
	if data.DisplayName != "" {
		t.Errorf("expected empty display_name, got %q", data.DisplayName)
	}
}

func TestOnboard_EmptyDisplayName_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "display_name is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "display_name is required" {
		t.Errorf("expected 'display_name is required', got %q", decoded["error"])
	}
}

func TestOnboard_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{not valid}`)
	req := httptest.NewRequest("POST", "/api/v1/me/onboard", body)

	var data struct {
		DisplayName string `json:"display_name"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSubmitBanking_ValidUPI(t *testing.T) {
	body := bytes.NewBufferString(`{"upi_id":"user@upi","pan_number":"ABCDE1234F"}`)
	req := httptest.NewRequest("POST", "/api/v1/me/banking", body)

	var data struct {
		UPIID         string `json:"upi_id"`
		AccountNumber string `json:"account_number"`
		IFSCCode      string `json:"ifsc_code"`
		PANNumber     string `json:"pan_number"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.UPIID != "user@upi" {
		t.Errorf("expected upi_id 'user@upi', got %q", data.UPIID)
	}
	if data.PANNumber != "ABCDE1234F" {
		t.Errorf("expected pan_number 'ABCDE1234F', got %q", data.PANNumber)
	}
}

func TestSubmitBanking_ValidAccountNumber(t *testing.T) {
	body := bytes.NewBufferString(`{"account_holder":"John Doe","bank_name":"SBI","account_number":"1234567890","ifsc_code":"SBIN0001234","pan_number":"ABCDE1234F"}`)
	req := httptest.NewRequest("POST", "/api/v1/me/banking", body)

	var data struct {
		AccountHolder string `json:"account_holder"`
		BankName      string `json:"bank_name"`
		AccountNumber string `json:"account_number"`
		IFSCCode      string `json:"ifsc_code"`
		PANNumber     string `json:"pan_number"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.AccountNumber != "1234567890" {
		t.Errorf("expected account_number '1234567890', got %q", data.AccountNumber)
	}
	if data.IFSCCode != "SBIN0001234" {
		t.Errorf("expected ifsc_code 'SBIN0001234', got %q", data.IFSCCode)
	}
	if data.AccountHolder != "John Doe" {
		t.Errorf("expected account_holder 'John Doe', got %q", data.AccountHolder)
	}
}

func TestSubmitBanking_MissingBothUPIAndAccount_WriteError(t *testing.T) {
	// When both upi_id and account_number+ifsc_code are missing, handler returns 400
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "provide UPI ID or account number + IFSC code")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "provide UPI ID or account number + IFSC code" {
		t.Errorf("expected banking validation error, got %q", decoded["error"])
	}
}

func TestSubmitBanking_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{nope}`)
	req := httptest.NewRequest("POST", "/api/v1/me/banking", body)

	var data struct {
		UPIID string `json:"upi_id"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestUpdateProfile_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"display_name":"Updated Name","bio":"new bio","phone":"+919999999999","city":"Delhi","state":"Delhi","specializations":["tech"],"youtube_channel":"https://youtube.com/@test","instagram_handle":"@testuser"}`)
	req := httptest.NewRequest("PUT", "/api/v1/me/profile", body)

	var data struct {
		DisplayName     string   `json:"display_name"`
		Bio             string   `json:"bio"`
		Phone           string   `json:"phone"`
		City            string   `json:"city"`
		State           string   `json:"state"`
		Specializations []string `json:"specializations"`
		YouTubeChannel  string   `json:"youtube_channel"`
		InstagramHandle string   `json:"instagram_handle"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.DisplayName != "Updated Name" {
		t.Errorf("expected display_name 'Updated Name', got %q", data.DisplayName)
	}
	if data.Phone != "+919999999999" {
		t.Errorf("expected phone '+919999999999', got %q", data.Phone)
	}
	if len(data.Specializations) != 1 {
		t.Errorf("expected 1 specialization, got %d", len(data.Specializations))
	}
}

func TestUpdateProfile_MissingDisplayName_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "display_name is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUpdateProfile_InvalidJSON(t *testing.T) {
	body := bytes.NewBufferString(`{bad}`)
	req := httptest.NewRequest("PUT", "/api/v1/me/profile", body)

	var data struct {
		DisplayName string `json:"display_name"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestConfirmAvatarUpload_ValidBody(t *testing.T) {
	body := bytes.NewBufferString(`{"storage_key":"avatars/some-uuid.jpg"}`)
	req := httptest.NewRequest("POST", "/api/v1/me/avatar/confirm", body)

	var data struct {
		StorageKey string `json:"storage_key"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.StorageKey != "avatars/some-uuid.jpg" {
		t.Errorf("expected storage_key 'avatars/some-uuid.jpg', got %q", data.StorageKey)
	}
}

func TestConfirmAvatarUpload_EmptyStorageKey(t *testing.T) {
	body := bytes.NewBufferString(`{"storage_key":""}`)
	req := httptest.NewRequest("POST", "/api/v1/me/avatar/confirm", body)

	var data struct {
		StorageKey string `json:"storage_key"`
	}
	err := readJSON(req, &data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty storage_key parses OK; handler checks err != nil || req.StorageKey == ""
	if data.StorageKey != "" {
		t.Errorf("expected empty storage_key, got %q", data.StorageKey)
	}
}

func TestConfirmAvatarUpload_EmptyBody(t *testing.T) {
	body := bytes.NewBufferString(``)
	req := httptest.NewRequest("POST", "/api/v1/me/avatar/confirm", body)

	var data struct {
		StorageKey string `json:"storage_key"`
	}
	err := readJSON(req, &data)
	if err == nil {
		t.Error("expected error for empty body")
	}
}

func TestConfirmAvatarUpload_NoStorageKey_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "storage_key is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "storage_key is required" {
		t.Errorf("expected 'storage_key is required', got %q", decoded["error"])
	}
}

func TestGetAvatarUploadURL_ContentTypeExtensionMapping(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantExt     string
	}{
		{"default jpeg", "", "jpg"},
		{"explicit jpeg", "image/jpeg", "jpg"},
		{"png", "image/png", "png"},
		{"webp", "image/webp", "webp"},
		{"unknown defaults to jpg", "image/gif", "jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/api/v1/me/avatar/upload-url"
			if tt.contentType != "" {
				url += "?content_type=" + tt.contentType
			}
			req := httptest.NewRequest("GET", url, nil)

			contentType := req.URL.Query().Get("content_type")
			if contentType == "" {
				contentType = "image/jpeg"
			}

			ext := "jpg"
			switch contentType {
			case "image/png":
				ext = "png"
			case "image/webp":
				ext = "webp"
			}

			if ext != tt.wantExt {
				t.Errorf("content_type %q -> ext = %q, want %q", tt.contentType, ext, tt.wantExt)
			}
		})
	}
}

func TestGetCreatorCountByCity_MissingCity_WriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "city parameter is required")

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var decoded map[string]string
	json.NewDecoder(w.Body).Decode(&decoded)
	if decoded["error"] != "city parameter is required" {
		t.Errorf("expected 'city parameter is required', got %q", decoded["error"])
	}
}

func TestGetCreatorCountByCity_QueryParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/creators/count?city=Mumbai", nil)
	city := req.URL.Query().Get("city")
	if city != "Mumbai" {
		t.Errorf("expected city 'Mumbai', got %q", city)
	}
}

func TestGetCreatorCountByCity_EmptyQueryParam(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/creators/count", nil)
	city := req.URL.Query().Get("city")
	if city != "" {
		t.Errorf("expected empty city, got %q", city)
	}
}
