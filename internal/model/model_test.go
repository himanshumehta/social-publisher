package model

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUserRole_Constants(t *testing.T) {
	tests := []struct {
		role     UserRole
		expected string
	}{
		{RoleSuperAdmin, "super_admin"},
		{RoleAdmin, "admin"},
		{RoleEditor, "editor"},
		{RoleCreator, "creator"},
	}
	for _, tt := range tests {
		if string(tt.role) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.role)
		}
	}
}

func TestOAuthProvider_Constants(t *testing.T) {
	tests := []struct {
		provider OAuthProvider
		expected string
	}{
		{ProviderGoogle, "google"},
		{ProviderFacebook, "facebook"},
		{ProviderLinkedIn, "linkedin"},
	}
	for _, tt := range tests {
		if string(tt.provider) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.provider)
		}
	}
}

func TestBankingStatus_Constants(t *testing.T) {
	tests := []struct {
		status   BankingStatus
		expected string
	}{
		{BankingPending, "pending"},
		{BankingSubmitted, "submitted"},
		{BankingVerified, "verified"},
		{BankingRejected, "rejected"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.expected {
			t.Errorf("expected %s, got %s", tt.expected, tt.status)
		}
	}
}

func TestUser_JSONSerialization(t *testing.T) {
	uid := uuid.New()
	now := time.Now().Truncate(time.Second)
	u := User{
		ID:          uid,
		Email:       "test@example.com",
		Name:        "Test User",
		DisplayName: "tester",
		Role:        RoleCreator,
		IsActive:    true,
		IsOnboarded: false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	data, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded["email"] != "test@example.com" {
		t.Errorf("expected email test@example.com, got %v", decoded["email"])
	}
	if decoded["role"] != "creator" {
		t.Errorf("expected role creator, got %v", decoded["role"])
	}
	if decoded["is_active"] != true {
		t.Errorf("expected is_active true")
	}
}

func TestOAuthAccount_TokensNotInJSON(t *testing.T) {
	oa := OAuthAccount{
		ID:           uuid.New(),
		UserID:       uuid.New(),
		Provider:     ProviderGoogle,
		ProviderID:   "google-123",
		AccessToken:  "secret-access-token",
		RefreshToken: "secret-refresh-token",
		RawProfile:   []byte(`{"email":"test@test.com"}`),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	data, _ := json.Marshal(oa)
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	// Sensitive fields should NOT appear in JSON
	if _, exists := decoded["access_token"]; exists {
		t.Error("access_token should not be in JSON output")
	}
	if _, exists := decoded["refresh_token"]; exists {
		t.Error("refresh_token should not be in JSON output")
	}
	if _, exists := decoded["raw_profile"]; exists {
		t.Error("raw_profile should not be in JSON output")
	}
}

func TestBankingInfo_SensitiveFieldsHidden(t *testing.T) {
	bi := BankingInfo{
		ID:            uuid.New(),
		UserID:        uuid.New(),
		AccountHolder: "Test User",
		BankName:      "HDFC Bank",
		AccountNumber: "1234567890",
		IFSCCode:      "HDFC0001234",
		UPIID:         "test@upi",
		PANNumber:     "ABCDE1234F",
		Status:        BankingSubmitted,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	data, _ := json.Marshal(bi)
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if _, exists := decoded["account_number"]; exists {
		t.Error("account_number should not be in JSON (tagged json:\"-\")")
	}
	if _, exists := decoded["pan_number"]; exists {
		t.Error("pan_number should not be in JSON (tagged json:\"-\")")
	}
	// UPI ID should be visible
	if decoded["upi_id"] != "test@upi" {
		t.Errorf("expected upi_id test@upi, got %v", decoded["upi_id"])
	}
}

func TestCreatorProfile_JSONOmitEmpty(t *testing.T) {
	cp := CreatorProfile{
		ID:       uuid.New(),
		UserID:   uuid.New(),
		Slug:     "testuser",
		IsPublic: true,
	}

	data, _ := json.Marshal(cp)
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	// Empty optional fields should be omitted
	if _, exists := decoded["city"]; exists {
		t.Error("empty city should be omitted")
	}
	if _, exists := decoded["youtube_channel"]; exists {
		t.Error("empty youtube_channel should be omitted")
	}
	if decoded["slug"] != "testuser" {
		t.Errorf("expected slug testuser, got %v", decoded["slug"])
	}
}

func TestAuditAction_Constants(t *testing.T) {
	actions := []AuditAction{
		AuditLogin, AuditLogout, AuditProfileUpdate, AuditBankingUpdate,
		AuditProjectCreate, AuditProjectUpdate, AuditProjectClaim,
		AuditVideoUpload, AuditReviewComment, AuditReviewApprove,
		AuditPayoutApprove, AuditPayoutComplete, AuditPostCreate, AuditPostPublish,
		AuditUserCreate, AuditUserRoleChange, AuditCategoryCreate, AuditChannelCreate,
	}
	for _, a := range actions {
		if string(a) == "" {
			t.Error("audit action should not be empty")
		}
	}
}

func TestCategory_JSONSerialization(t *testing.T) {
	c := Category{
		ID:           uuid.New(),
		Name:         "Demo Category",
		Slug:         "demo",
		Description:  "Motorcycles and cars",
		IsActive:     true,
		DisplayOrder: 0,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	data, _ := json.Marshal(c)
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if decoded["name"] != "Demo Category" {
		t.Errorf("expected name Demo Category, got %v", decoded["name"])
	}
	if decoded["slug"] != "demo" {
		t.Errorf("expected slug demo, got %v", decoded["slug"])
	}
}

func TestChannel_TokensHidden(t *testing.T) {
	catID := uuid.New()
	ch := Channel{
		ID:           uuid.New(),
		CategoryID:   &catID,
		ChannelType:  "system",
		Platform:     "youtube",
		ChannelName:  "demo_channel",
		AccessToken:  "secret-yt-token",
		RefreshToken: "secret-yt-refresh",
		IsActive:     true,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	data, _ := json.Marshal(ch)
	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if _, exists := decoded["access_token"]; exists {
		t.Error("channel access_token should not be in JSON")
	}
	if _, exists := decoded["refresh_token"]; exists {
		t.Error("channel refresh_token should not be in JSON")
	}
	if decoded["platform"] != "youtube" {
		t.Errorf("expected platform youtube, got %v", decoded["platform"])
	}
}
