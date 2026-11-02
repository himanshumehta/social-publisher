package model

import (
	"time"

	"github.com/google/uuid"
)

type EditorAlignment struct {
	ID        uuid.UUID `json:"id"`
	CreatorID uuid.UUID `json:"creator_id"`
	EditorID  uuid.UUID `json:"editor_id"`
	AlignedBy uuid.UUID `json:"aligned_by"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
}

type EditorAlignmentWithNames struct {
	EditorAlignment
	CreatorName   string `json:"creator_name"`
	EditorName    string `json:"editor_name"`
	AlignedByName string `json:"aligned_by_name"`
}
