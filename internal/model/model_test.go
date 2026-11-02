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
