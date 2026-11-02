package model

import (
	"time"

	"github.com/google/uuid"
)

type ProjectStatus string

const (
	ProjectOpen      ProjectStatus = "open"
	ProjectActive    ProjectStatus = "active"
	ProjectInReview  ProjectStatus = "in_review"
	ProjectPublished ProjectStatus = "published"
	ProjectArchived  ProjectStatus = "archived"
)

type Project struct {
	ID                 uuid.UUID     `json:"id"`
	CategoryID         *uuid.UUID    `json:"category_id,omitempty"`
	Title              string        `json:"name"`
	Slug               string        `json:"slug"`
	Description        string        `json:"description"`
	SubjectName        string        `json:"subject_name"`
	SubjectType        string        `json:"subject_type"`
	SubjectRefID       string        `json:"subject_ref_id,omitempty"`
	Requirements       []byte        `json:"requirements"`
	BudgetAmount       int64         `json:"budget_amount"`
	BudgetCurrency     string        `json:"budget_currency"`
	DeadlineAt         *time.Time    `json:"deadline_at,omitempty"`
	PublishTarget      *time.Time    `json:"publish_target,omitempty"`
	Status             ProjectStatus `json:"status"`
	CreatorID          *uuid.UUID    `json:"creator_id,omitempty"`
	EditorID           *uuid.UUID    `json:"editor_id,omitempty"`
	CreatedBy          uuid.UUID     `json:"created_by"`
	Priority           int           `json:"priority"`
	Tags               []string      `json:"tags,omitempty"`
	EditingRequested   bool          `json:"editing_requested"`
	EditingNotes       *string       `json:"editing_notes,omitempty"`
	EditingCompletedAt *time.Time    `json:"editing_completed_at,omitempty"`
	ProjectType        string        `json:"project_type"`
	PublishMode        string        `json:"publish_mode"`
	ClaimedAt          *time.Time    `json:"claimed_at,omitempty"`
	AgreementAccepted  bool          `json:"agreement_accepted"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}
