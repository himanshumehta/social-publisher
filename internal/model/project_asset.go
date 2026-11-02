package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ProjectAsset struct {
	ID           uuid.UUID       `json:"id"`
	ProjectID    uuid.UUID       `json:"project_id"`
	Category     string          `json:"category"`
	Subcategory  string          `json:"subcategory"`
	Filename     string          `json:"filename"`
	FileURL      string          `json:"file_url"`
	ThumbnailURL *string         `json:"thumbnail_url,omitempty"`
	AltText      string          `json:"alt_text"`
	FileSize     int64           `json:"file_size"`
	MimeType     string          `json:"mime_type"`
	Metadata     json.RawMessage `json:"metadata"`
	SortOrder    int             `json:"sort_order"`
	Status       string          `json:"status"`
	Version      int             `json:"version"`
	UploadedBy   *uuid.UUID      `json:"uploaded_by,omitempty"`
	DeletedAt    *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ProjectAssetTree struct {
	Category    string         `json:"category"`
	DisplayName string         `json:"display_name"`
	Assets      []ProjectAsset `json:"assets"`
	TotalSize   int64          `json:"total_size"`
	Count       int            `json:"count"`
}

type ChangelogEntry struct {
	ID          uuid.UUID `json:"id"`
	ProjectID   uuid.UUID `json:"project_id"`
	Action      string    `json:"action"`
	Category    string    `json:"category"`
	AssetCount  int       `json:"asset_count"`
	Summary     string    `json:"summary"`
	TriggeredBy string    `json:"triggered_by"`
	CreatedAt   time.Time `json:"created_at"`
}

// CategoryDisplayNames maps category slugs to human-readable names
var CategoryDisplayNames = map[string]string{
	"data":        "Specs & Research",
	"narration":   "Narration",
	"images":      "Images",
	"templates":   "Templates",
	"branding":    "Branding",
	"racebox":     "Performance",
	"comparisons": "Comparisons",
}
